package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

type testProviderFailure struct{}

func (testProviderFailure) Error() string             { return "private provider response" }
func (testProviderFailure) Code() string              { return "TEST_PROVIDER_FAILED" }
func (testProviderFailure) ProviderName() string      { return "test" }
func (testProviderFailure) ProviderOperation() string { return "generate" }
func (testProviderFailure) ProviderStatus() int       { return 429 }

func TestProviderFailureClassification(t *testing.T) {
	if !isProviderFailure(fmt.Errorf("wrapped: %w", testProviderFailure{})) {
		t.Fatal("wrapped provider failure was not classified")
	}
	if isProviderFailure(errors.New("database failed")) {
		t.Fatal("non-provider error was classified as provider failure")
	}
}

func TestProviderFailureLogAttributesExcludeErrorText(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	logger.Warn("provider failed", errorLogAttributes(fmt.Errorf("wrapped: %w", testProviderFailure{}))...)
	logs := output.String()
	for _, expected := range []string{`"provider":"test"`, `"provider_operation":"generate"`, `"provider_status":429`, `"error_code":"TEST_PROVIDER_FAILED"`} {
		if !strings.Contains(logs, expected) {
			t.Fatalf("logs missing %s: %s", expected, logs)
		}
	}
	if strings.Contains(logs, "private provider response") {
		t.Fatalf("logs leaked provider error: %s", logs)
	}
}
