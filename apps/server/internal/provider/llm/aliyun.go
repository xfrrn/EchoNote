package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
	"github.com/Actify/echonote/apps/server/internal/provider/safehttp"
)

const (
	defaultModel           = "qwen-plus"
	maxResponseBytes       = 2 << 20
	maxStreamBytes         = 4 << 20
	maxStreamEventBytes    = 1 << 20
	chatCompletionsPath    = "/chat/completions"
	compatibleModeBasePath = "/compatible-mode/v1"
)

type ProviderError struct {
	StatusCode   int
	ProviderCode string
	Message      string
}

func (err *ProviderError) Error() string {
	if err.ProviderCode != "" {
		return fmt.Sprintf("aliyun LLM: %s: %s", err.ProviderCode, err.Message)
	}
	return "aliyun LLM: " + err.Message
}

func (err *ProviderError) Code() string    { return "AI_PROVIDER_FAILED" }
func (err *ProviderError) Retryable() bool { return false }

type Aliyun struct {
	baseURL    string
	model      string
	apiKey     string
	httpClient *http.Client
}

func NewAliyun(endpoint, model, apiKey string, client *http.Client) (*Aliyun, error) {
	return newAliyun(endpoint, model, apiKey, client, false)
}

func newAliyun(endpoint, model, apiKey string, client *http.Client, allowHTTP bool) (*Aliyun, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && !(allowHTTP && parsed.Scheme == "http")) {
		return nil, errors.New("Aliyun LLM endpoint must be an HTTPS URL without credentials")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = compatibleModeBasePath
	} else if path != compatibleModeBasePath {
		return nil, errors.New("Aliyun LLM endpoint path must be empty or /compatible-mode/v1")
	}
	model, apiKey = strings.TrimSpace(model), strings.TrimSpace(apiKey)
	if model == "" {
		model = defaultModel
	}
	if apiKey == "" {
		return nil, errors.New("Aliyun LLM API key is required")
	}
	parsed.Path, parsed.RawPath, parsed.RawQuery, parsed.Fragment = path, "", "", ""
	if client == nil {
		client = safehttp.NewClient(5 * time.Minute)
	}
	return &Aliyun{
		baseURL: strings.TrimRight(parsed.String(), "/"), model: model,
		apiKey: apiKey, httpClient: client,
	}, nil
}

func (provider *Aliyun) Model() string { return provider.model }

func (provider *Aliyun) GenerateStructured(
	ctx context.Context,
	request aidomain.StructuredGenerationRequest,
) (aidomain.StructuredGenerationResult, error) {
	if err := validateRequest(request.Messages, request.MaxTokens); err != nil {
		return aidomain.StructuredGenerationResult{}, err
	}
	body, err := provider.requestBody(request.Messages, request.MaxTokens, false, true)
	if err != nil {
		return aidomain.StructuredGenerationResult{}, err
	}
	response, err := provider.do(ctx, body)
	if err != nil {
		return aidomain.StructuredGenerationResult{}, err
	}
	defer response.Body.Close()
	raw, err := readLimited(response.Body, maxResponseBytes)
	if err != nil {
		return aidomain.StructuredGenerationResult{}, &ProviderError{StatusCode: response.StatusCode, Message: err.Error()}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return aidomain.StructuredGenerationResult{}, providerError(response.StatusCode, raw)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage usageResponse `json:"usage"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || len(decoded.Choices) != 1 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
		return aidomain.StructuredGenerationResult{}, &ProviderError{StatusCode: response.StatusCode, Message: "invalid structured response"}
	}
	return aidomain.StructuredGenerationResult{
		Content: decoded.Choices[0].Message.Content,
		Usage:   decoded.Usage.domain(),
	}, nil
}

func (provider *Aliyun) StreamChat(
	ctx context.Context,
	request aidomain.ChatRequest,
) (<-chan aidomain.ChatEvent, error) {
	if err := validateRequest(request.Messages, request.MaxTokens); err != nil {
		return nil, err
	}
	body, err := provider.requestBody(request.Messages, request.MaxTokens, true, false)
	if err != nil {
		return nil, err
	}
	response, err := provider.do(ctx, body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		raw, readErr := readLimited(response.Body, maxResponseBytes)
		if readErr != nil {
			return nil, &ProviderError{StatusCode: response.StatusCode, Message: readErr.Error()}
		}
		return nil, providerError(response.StatusCode, raw)
	}
	events := make(chan aidomain.ChatEvent, 8)
	go provider.readStream(ctx, response.Body, events)
	return events, nil
}

type chatRequest struct {
	Model          string             `json:"model"`
	Messages       []aidomain.Message `json:"messages"`
	MaxTokens      int                `json:"max_tokens"`
	Temperature    float64            `json:"temperature"`
	Stream         bool               `json:"stream"`
	ResponseFormat *struct {
		Type string `json:"type"`
	} `json:"response_format,omitempty"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

func (provider *Aliyun) requestBody(messages []aidomain.Message, maxTokens int, stream, structured bool) ([]byte, error) {
	request := chatRequest{
		Model: provider.model, Messages: messages, MaxTokens: maxTokens,
		Temperature: 0.2, Stream: stream,
	}
	if structured {
		request.ResponseFormat = &struct {
			Type string `json:"type"`
		}{Type: "json_object"}
	}
	if stream {
		request.StreamOptions = &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true}
	}
	return json.Marshal(request)
}

func (provider *Aliyun) do(ctx context.Context, body []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, provider.baseURL+chatCompletionsPath, bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+provider.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := provider.httpClient.Do(request)
	if err != nil {
		return nil, &ProviderError{Message: err.Error()}
	}
	return response, nil
}

func (provider *Aliyun) readStream(ctx context.Context, body io.ReadCloser, events chan<- aidomain.ChatEvent) {
	defer close(events)
	defer body.Close()
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64<<10), maxStreamEventBytes)
	total, done := 0, false
	for scanner.Scan() {
		line := scanner.Text()
		total += len(line)
		if total > maxStreamBytes {
			sendEvent(ctx, events, aidomain.ChatEvent{Err: &ProviderError{Message: "stream exceeds size limit"}})
			return
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			done = true
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage usageResponse `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			sendEvent(ctx, events, aidomain.ChatEvent{Err: &ProviderError{Message: "invalid stream event"}})
			return
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			if !sendEvent(ctx, events, aidomain.ChatEvent{Delta: chunk.Choices[0].Delta.Content}) {
				return
			}
		}
		if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			usage := chunk.Usage.domain()
			if !sendEvent(ctx, events, aidomain.ChatEvent{Usage: &usage}) {
				return
			}
		}
	}
	if err := scanner.Err(); err != nil {
		sendEvent(ctx, events, aidomain.ChatEvent{Err: &ProviderError{Message: err.Error()}})
		return
	}
	if !done {
		sendEvent(ctx, events, aidomain.ChatEvent{Err: &ProviderError{Message: "stream ended without a completion marker"}})
	}
}

type usageResponse struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

func (usage usageResponse) domain() aidomain.Usage {
	return aidomain.Usage{InputTokens: usage.PromptTokens, OutputTokens: usage.CompletionTokens}
}

func validateRequest(messages []aidomain.Message, maxTokens int) error {
	if len(messages) == 0 || len(messages) > 50 || maxTokens < 1 || maxTokens > 32_768 {
		return errors.New("LLM request messages or max tokens are invalid")
	}
	for _, message := range messages {
		if (message.Role != "system" && message.Role != "user" && message.Role != "assistant") || strings.TrimSpace(message.Content) == "" {
			return errors.New("LLM request contains an invalid message")
		}
	}
	return nil
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

func providerError(status int, body []byte) *ProviderError {
	var decoded struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &decoded)
	if decoded.Error.Message == "" {
		decoded.Error.Message = http.StatusText(status)
	}
	return &ProviderError{StatusCode: status, ProviderCode: decoded.Error.Code, Message: decoded.Error.Message}
}

func sendEvent(ctx context.Context, events chan<- aidomain.ChatEvent, event aidomain.ChatEvent) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}
