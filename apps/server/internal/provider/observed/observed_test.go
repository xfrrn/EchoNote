package observed

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
	"github.com/Actify/echonote/apps/server/internal/logging"
)

type fakeLLM struct{}

func (fakeLLM) GenerateStructured(context.Context, aidomain.StructuredGenerationRequest) (aidomain.StructuredGenerationResult, error) {
	return aidomain.StructuredGenerationResult{Content: "private transcript", Usage: aidomain.Usage{InputTokens: 3, OutputTokens: 4}}, nil
}

func (fakeLLM) StreamChat(context.Context, aidomain.ChatRequest) (<-chan aidomain.ChatEvent, error) {
	events := make(chan aidomain.ChatEvent, 2)
	events <- aidomain.ChatEvent{Delta: "private answer"}
	events <- aidomain.ChatEvent{Usage: &aidomain.Usage{InputTokens: 5, OutputTokens: 6}}
	close(events)
	return events, nil
}

func (fakeLLM) Model() string { return "test-model" }

func TestLLMLogsUsageWithoutContent(t *testing.T) {
	var output bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&output, nil))
	ctx := logging.WithLogger(context.Background(), base.With("request_id", "request-1"))
	provider := WrapLLM(fakeLLM{}, base)

	if _, err := provider.GenerateStructured(ctx, aidomain.StructuredGenerationRequest{
		Messages: []aidomain.Message{{Role: "user", Content: "private question"}},
	}); err != nil {
		t.Fatal(err)
	}
	events, err := provider.StreamChat(ctx, aidomain.ChatRequest{Messages: []aidomain.Message{{Role: "user", Content: "private question"}}})
	if err != nil {
		t.Fatal(err)
	}
	for range events {
	}

	logs := output.String()
	for _, expected := range []string{`"request_id":"request-1"`, `"provider":"aliyun_llm"`, `"input_tokens":3`, `"output_tokens":4`, `"input_tokens":5`, `"output_tokens":6`, `"duration_ms":`} {
		if !strings.Contains(logs, expected) {
			t.Fatalf("logs missing %s: %s", expected, logs)
		}
	}
	for _, secret := range []string{"private question", "private transcript", "private answer"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("logs leaked %q: %s", secret, logs)
		}
	}
}
