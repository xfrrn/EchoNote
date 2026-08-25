package asr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	domain "github.com/Actify/echonote/apps/server/internal/domain/transcription"
	"github.com/Actify/echonote/apps/server/internal/provider/safehttp"
)

const (
	maxAPIResponseBytes = 1 << 20
	maxResultBytes      = 64 << 20
)

type ProviderError struct {
	Operation     string
	StatusCode    int
	ProviderCode  string
	Message       string
	retryable     bool
	ambiguousCost bool
}

func (err *ProviderError) Error() string {
	if err.ProviderCode != "" {
		return fmt.Sprintf("aliyun ASR %s: %s: %s", err.Operation, err.ProviderCode, err.Message)
	}
	return fmt.Sprintf("aliyun ASR %s: %s", err.Operation, err.Message)
}

func (err *ProviderError) Code() string {
	if err.ambiguousCost {
		return "ASR_SUBMISSION_AMBIGUOUS"
	}
	if err.Operation == "fetch result" && (err.StatusCode == http.StatusForbidden || err.StatusCode == http.StatusNotFound) {
		return "ASR_RESULT_EXPIRED"
	}
	if err.Operation == "parse result" {
		return "ASR_RESULT_INVALID"
	}
	return "ASR_PROVIDER_ERROR"
}

func (err *ProviderError) Retryable() bool { return err.retryable }

func (err *ProviderError) ProviderName() string      { return "aliyun_asr" }
func (err *ProviderError) ProviderOperation() string { return err.Operation }
func (err *ProviderError) ProviderStatus() int       { return err.StatusCode }

func (err *ProviderError) AmbiguousCost() bool { return err.ambiguousCost }

type Aliyun struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	allowHTTP  bool
}

func NewAliyun(endpoint, apiKey string, client *http.Client) (*Aliyun, error) {
	return newAliyun(endpoint, apiKey, client, false)
}

func newAliyun(endpoint, apiKey string, client *http.Client, allowHTTP bool) (*Aliyun, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && !(allowHTTP && parsed.Scheme == "http")) {
		return nil, errors.New("Aliyun ASR endpoint must be an HTTPS URL without credentials")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("Aliyun ASR API key is required")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/api/v1"
	} else if path != "/api/v1" {
		return nil, errors.New("Aliyun ASR endpoint path must be empty or /api/v1")
	}
	parsed.Path, parsed.RawPath, parsed.RawQuery, parsed.Fragment = path, "", "", ""
	if client == nil {
		client = safehttp.NewClient(30 * time.Second)
	}
	return &Aliyun{baseURL: strings.TrimRight(parsed.String(), "/"), apiKey: strings.TrimSpace(apiKey), httpClient: client, allowHTTP: allowHTTP}, nil
}

func (provider *Aliyun) Submit(ctx context.Context, request domain.Request) (domain.ExternalTask, error) {
	if request.Model != "paraformer-v2" && request.Model != "fun-asr" {
		return domain.ExternalTask{}, errors.New("unsupported Aliyun ASR model")
	}
	if parsed, err := url.Parse(request.AudioURL); err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && !(provider.allowHTTP && parsed.Scheme == "http")) {
		return domain.ExternalTask{}, errors.New("ASR audio URL must be HTTPS")
	}
	parameters := map[string]any{
		"channel_id":          []int{0},
		"diarization_enabled": true,
	}
	if request.Model == "paraformer-v2" {
		parameters["timestamp_alignment_enabled"] = true
	}
	if request.LanguageHint != "" {
		parameters["language_hints"] = []string{request.LanguageHint}
	}
	if request.SpeakerCount >= 2 && request.SpeakerCount <= 100 {
		parameters["speaker_count"] = request.SpeakerCount
	}
	payload := struct {
		Model      string         `json:"model"`
		Input      map[string]any `json:"input"`
		Parameters map[string]any `json:"parameters"`
	}{Model: request.Model, Input: map[string]any{"file_urls": []string{request.AudioURL}}, Parameters: parameters}
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.ExternalTask{}, err
	}
	httpRequest, err := provider.request(ctx, http.MethodPost, provider.baseURL+"/services/audio/asr/transcription", bytes.NewReader(body))
	if err != nil {
		return domain.ExternalTask{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-DashScope-Async", "enable")
	var response struct {
		Output struct {
			TaskID string `json:"task_id"`
		} `json:"output"`
	}
	if err := provider.doJSON(httpRequest, &response, true); err != nil {
		return domain.ExternalTask{}, err
	}
	if response.Output.TaskID == "" {
		return domain.ExternalTask{}, &ProviderError{Operation: "submit", Message: "response omitted task_id", ambiguousCost: true}
	}
	return domain.ExternalTask{ID: response.Output.TaskID}, nil
}

func (provider *Aliyun) Poll(ctx context.Context, taskID string) (domain.ExternalTaskStatus, error) {
	if strings.TrimSpace(taskID) == "" {
		return domain.ExternalTaskStatus{}, errors.New("ASR task ID is required")
	}
	httpRequest, err := provider.request(ctx, http.MethodGet, provider.baseURL+"/tasks/"+url.PathEscape(taskID), nil)
	if err != nil {
		return domain.ExternalTaskStatus{}, err
	}
	var response struct {
		Output struct {
			TaskStatus string `json:"task_status"`
			Code       string `json:"code"`
			Message    string `json:"message"`
			Results    []struct {
				SubtaskStatus    string `json:"subtask_status"`
				TranscriptionURL string `json:"transcription_url"`
				Code             string `json:"code"`
				Message          string `json:"message"`
			} `json:"results"`
		} `json:"output"`
	}
	if err := provider.doJSON(httpRequest, &response, false); err != nil {
		return domain.ExternalTaskStatus{}, err
	}
	switch strings.ToUpper(response.Output.TaskStatus) {
	case "PENDING":
		return domain.ExternalTaskStatus{State: domain.TaskPending}, nil
	case "RUNNING":
		return domain.ExternalTaskStatus{State: domain.TaskRunning}, nil
	case "FAILED", "UNKNOWN":
		return domain.ExternalTaskStatus{State: domain.TaskFailed, Code: response.Output.Code, Message: response.Output.Message}, nil
	case "CANCELED":
		return domain.ExternalTaskStatus{State: domain.TaskCanceled}, nil
	case "SUCCEEDED":
		if len(response.Output.Results) != 1 {
			return domain.ExternalTaskStatus{}, &ProviderError{Operation: "poll", Message: "successful task did not return exactly one result"}
		}
		result := response.Output.Results[0]
		if strings.ToUpper(result.SubtaskStatus) != "SUCCEEDED" || result.TranscriptionURL == "" {
			return domain.ExternalTaskStatus{State: domain.TaskFailed, Code: result.Code, Message: result.Message}, nil
		}
		return domain.ExternalTaskStatus{State: domain.TaskSucceeded, ResultURL: result.TranscriptionURL}, nil
	default:
		return domain.ExternalTaskStatus{}, &ProviderError{Operation: "poll", Message: "unknown task status " + strconv.Quote(response.Output.TaskStatus), retryable: true}
	}
}

func (provider *Aliyun) FetchResult(ctx context.Context, resultURL string) (domain.RawResult, error) {
	parsed, err := url.Parse(strings.TrimSpace(resultURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && !(provider.allowHTTP && parsed.Scheme == "http")) {
		return domain.RawResult{}, errors.New("ASR result URL must be HTTPS")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return domain.RawResult{}, err
	}
	response, err := provider.httpClient.Do(request)
	if err != nil {
		return domain.RawResult{}, &ProviderError{Operation: "fetch result", Message: err.Error(), retryable: true}
	}
	defer response.Body.Close()
	raw, readErr := readLimited(response.Body, maxResultBytes)
	if readErr != nil {
		return domain.RawResult{}, &ProviderError{Operation: "fetch result", Message: readErr.Error(), retryable: true}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.RawResult{}, responseError("fetch result", response.StatusCode, raw, false)
	}
	segments, err := parseResult(raw)
	if err != nil {
		return domain.RawResult{}, &ProviderError{Operation: "parse result", Message: err.Error()}
	}
	return domain.RawResult{Raw: raw, Segments: segments}, nil
}

func (provider *Aliyun) request(ctx context.Context, method, target string, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+provider.apiKey)
	request.Header.Set("Accept", "application/json")
	return request, nil
}

func (provider *Aliyun) doJSON(request *http.Request, destination any, submission bool) error {
	response, err := provider.httpClient.Do(request)
	if err != nil {
		return &ProviderError{Operation: operationName(request), Message: err.Error(), ambiguousCost: submission}
	}
	defer response.Body.Close()
	body, err := readLimited(response.Body, maxAPIResponseBytes)
	if err != nil {
		return &ProviderError{Operation: operationName(request), Message: err.Error(), retryable: !submission, ambiguousCost: submission}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		providerError := responseError(operationName(request), response.StatusCode, body, submission && (response.StatusCode == http.StatusRequestTimeout || response.StatusCode >= 500))
		return providerError
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return &ProviderError{Operation: operationName(request), Message: "invalid JSON response", retryable: !submission, ambiguousCost: submission}
	}
	return nil
}

func responseError(operation string, status int, body []byte, ambiguous bool) *ProviderError {
	var decoded struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Output  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"output"`
	}
	_ = json.Unmarshal(body, &decoded)
	if decoded.Code == "" {
		decoded.Code, decoded.Message = decoded.Output.Code, decoded.Output.Message
	}
	if decoded.Message == "" {
		decoded.Message = http.StatusText(status)
	}
	return &ProviderError{
		Operation: operation, StatusCode: status, ProviderCode: decoded.Code, Message: decoded.Message,
		retryable:     status == http.StatusTooManyRequests || (!ambiguous && (status == http.StatusRequestTimeout || status >= 500)),
		ambiguousCost: ambiguous,
	}
}

func operationName(request *http.Request) string {
	if request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/transcription") {
		return "submit"
	}
	return strings.ToLower(request.Method)
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("response exceeds size limit")
	}
	return body, nil
}

type speakerID string

func (id *speakerID) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*id = ""
		return nil
	}
	var text string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
	} else {
		text = string(data)
	}
	*id = speakerID(strings.TrimSpace(text))
	return nil
}

func parseResult(raw []byte) ([]domain.Segment, error) {
	var decoded struct {
		Transcripts []struct {
			Sentences []struct {
				BeginTime int64     `json:"begin_time"`
				EndTime   int64     `json:"end_time"`
				Text      string    `json:"text"`
				SpeakerID speakerID `json:"speaker_id"`
				Words     []struct {
					BeginTime   int64  `json:"begin_time"`
					EndTime     int64  `json:"end_time"`
					Text        string `json:"text"`
					Punctuation string `json:"punctuation"`
				} `json:"words"`
			} `json:"sentences"`
		} `json:"transcripts"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	var segments []domain.Segment
	for _, transcript := range decoded.Transcripts {
		for _, sentence := range transcript.Sentences {
			speaker := string(sentence.SpeakerID)
			if speaker == "" {
				speaker = "0"
			}
			words := make([]domain.Word, 0, len(sentence.Words))
			for _, word := range sentence.Words {
				words = append(words, domain.Word{StartMS: word.BeginTime, EndMS: word.EndTime, Text: word.Text, Punctuation: word.Punctuation})
			}
			segments = append(segments, domain.Segment{
				LocalSequence: len(segments), LocalSpeaker: speaker,
				StartMS: sentence.BeginTime, EndMS: sentence.EndTime, Text: sentence.Text, Words: words,
			})
		}
	}
	if len(segments) == 0 {
		return nil, errors.New("result contains no sentences")
	}
	return segments, nil
}
