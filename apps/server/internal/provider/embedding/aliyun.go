package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	searchdomain "github.com/Actify/echonote/apps/server/internal/domain/search"
	"github.com/Actify/echonote/apps/server/internal/provider/safehttp"
)

const (
	AliyunModel      = "text-embedding-v4"
	AliyunDimensions = 1024
	maxBatchSize     = 10
	maxResponseBytes = 2 << 20
)

type ProviderError struct {
	StatusCode   int
	ProviderCode string
	Message      string
}

func (err *ProviderError) Error() string {
	if err.ProviderCode != "" {
		return fmt.Sprintf("aliyun embedding: %s: %s", err.ProviderCode, err.Message)
	}
	return "aliyun embedding: " + err.Message
}

func (err *ProviderError) Code() string    { return "EMBEDDING_PROVIDER_ERROR" }
func (err *ProviderError) Retryable() bool { return false }

type Aliyun struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewAliyun(endpoint, apiKey string, client *http.Client) (*Aliyun, error) {
	return newAliyun(endpoint, apiKey, client, false)
}

func newAliyun(endpoint, apiKey string, client *http.Client, allowHTTP bool) (*Aliyun, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && !(allowHTTP && parsed.Scheme == "http")) {
		return nil, errors.New("Aliyun embedding endpoint must be an HTTPS URL without credentials")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("Aliyun embedding API key is required")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/api/v1"
	} else if path != "/api/v1" {
		return nil, errors.New("Aliyun embedding endpoint path must be empty or /api/v1")
	}
	parsed.Path, parsed.RawPath, parsed.RawQuery, parsed.Fragment = path, "", "", ""
	if client == nil {
		client = safehttp.NewClient(30 * time.Second)
	}
	return &Aliyun{baseURL: strings.TrimRight(parsed.String(), "/"), apiKey: strings.TrimSpace(apiKey), httpClient: client}, nil
}

func (*Aliyun) Model() string   { return AliyunModel }
func (*Aliyun) Dimensions() int { return AliyunDimensions }

func (provider *Aliyun) Embed(
	ctx context.Context,
	texts []string,
	inputType searchdomain.EmbeddingInputType,
) ([][]float32, error) {
	if len(texts) == 0 || len(texts) > maxBatchSize {
		return nil, fmt.Errorf("embedding batch must contain 1-%d texts", maxBatchSize)
	}
	if inputType != searchdomain.EmbeddingQuery && inputType != searchdomain.EmbeddingDocument {
		return nil, errors.New("embedding input type must be query or document")
	}
	for _, text := range texts {
		if strings.TrimSpace(text) == "" {
			return nil, errors.New("embedding text must not be empty")
		}
	}
	payload := struct {
		Model string `json:"model"`
		Input struct {
			Texts []string `json:"texts"`
		} `json:"input"`
		Parameters struct {
			TextType  searchdomain.EmbeddingInputType `json:"text_type"`
			Dimension int                             `json:"dimension"`
			Output    string                          `json:"output_type"`
		} `json:"parameters"`
	}{Model: AliyunModel}
	payload.Input.Texts = texts
	payload.Parameters.TextType = inputType
	payload.Parameters.Dimension = AliyunDimensions
	payload.Parameters.Output = "dense"
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		provider.baseURL+"/services/embeddings/text-embedding/text-embedding",
		bytes.NewReader(body),
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
	defer response.Body.Close()
	raw, err := readResponse(response.Body)
	if err != nil {
		return nil, &ProviderError{StatusCode: response.StatusCode, Message: err.Error()}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, providerError(response.StatusCode, raw)
	}
	var decoded struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Output  struct {
			Embeddings []struct {
				Embedding []float32 `json:"embedding"`
				TextIndex int       `json:"text_index"`
			} `json:"embeddings"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, &ProviderError{StatusCode: response.StatusCode, Message: "invalid JSON response"}
	}
	if decoded.Code != "" {
		return nil, &ProviderError{StatusCode: response.StatusCode, ProviderCode: decoded.Code, Message: decoded.Message}
	}
	if len(decoded.Output.Embeddings) != len(texts) {
		return nil, &ProviderError{StatusCode: response.StatusCode, Message: "response count does not match input count"}
	}
	vectors := make([][]float32, len(texts))
	for _, item := range decoded.Output.Embeddings {
		if item.TextIndex < 0 || item.TextIndex >= len(vectors) || vectors[item.TextIndex] != nil || len(item.Embedding) != AliyunDimensions {
			return nil, &ProviderError{StatusCode: response.StatusCode, Message: "response contains an invalid embedding"}
		}
		for _, value := range item.Embedding {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return nil, &ProviderError{StatusCode: response.StatusCode, Message: "response contains a non-finite embedding"}
			}
		}
		vectors[item.TextIndex] = item.Embedding
	}
	return vectors, nil
}

func readResponse(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("response exceeds size limit")
	}
	return body, nil
}

func providerError(status int, body []byte) *ProviderError {
	var decoded struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &decoded)
	if decoded.Message == "" {
		decoded.Message = http.StatusText(status)
	}
	return &ProviderError{StatusCode: status, ProviderCode: decoded.Code, Message: decoded.Message}
}
