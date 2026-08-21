package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
)

func TestAliyunStructuredGenerationContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/compatible-mode/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("path=%q authorization=%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "qwen-plus" || body.Stream || body.ResponseFormat == nil || body.ResponseFormat.Type != "json_object" || body.MaxTokens != 100 {
			t.Errorf("body=%+v", body)
		}
		_, _ = fmt.Fprint(response, `{"choices":[{"message":{"content":"{\"answer\":\"ok\"}"}}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`)
	}))
	defer server.Close()
	provider, err := newAliyun(server.URL, "qwen-plus", "secret", server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.GenerateStructured(context.Background(), aidomain.StructuredGenerationRequest{
		Messages: []aidomain.Message{{Role: "system", Content: "return JSON"}}, MaxTokens: 100,
	})
	if err != nil || result.Content != `{"answer":"ok"}` || result.Usage.InputTokens != 3 || result.Usage.OutputTokens != 4 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestAliyunChatStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var body chatRequest
		_ = json.NewDecoder(request.Body).Decode(&body)
		if !body.Stream || body.StreamOptions == nil || !body.StreamOptions.IncludeUsage {
			t.Errorf("body=%+v", body)
		}
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(response, "data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n")
		_, _ = fmt.Fprint(response, "data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n")
		_, _ = fmt.Fprint(response, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1}}\n\n")
		_, _ = fmt.Fprint(response, "data: [DONE]\n\n")
	}))
	defer server.Close()
	provider, err := newAliyun(server.URL, "qwen-plus", "secret", server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	events, err := provider.StreamChat(context.Background(), aidomain.ChatRequest{
		Messages: []aidomain.Message{{Role: "user", Content: "hello"}}, MaxTokens: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	var usage aidomain.Usage
	for event := range events {
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		text += event.Delta
		if event.Usage != nil {
			usage = *event.Usage
		}
	}
	if strings.TrimSpace(text) != "你好" || usage.InputTokens != 2 || usage.OutputTokens != 1 {
		t.Fatalf("text=%q usage=%+v", text, usage)
	}
}

func TestProviderErrorRetryability(t *testing.T) {
	for status, expected := range map[int]bool{0: true, http.StatusBadRequest: false, http.StatusUnauthorized: false, http.StatusRequestTimeout: true, http.StatusTooManyRequests: true, http.StatusServiceUnavailable: true} {
		if actual := (&ProviderError{StatusCode: status}).Retryable(); actual != expected {
			t.Fatalf("status=%d retryable=%v", status, actual)
		}
	}
}
