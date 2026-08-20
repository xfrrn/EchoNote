package asr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domain "github.com/Actify/echonote/apps/server/internal/domain/transcription"
)

func TestAliyunAsyncFlow(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/api/v1/") && request.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization=%q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/api/v1/services/audio/asr/transcription":
			if request.Method != http.MethodPost || request.Header.Get("X-DashScope-Async") != "enable" {
				t.Errorf("submit method=%s async=%q", request.Method, request.Header.Get("X-DashScope-Async"))
			}
			var body struct {
				Model      string         `json:"model"`
				Parameters map[string]any `json:"parameters"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Model != "paraformer-v2" || body.Parameters["diarization_enabled"] != true || body.Parameters["timestamp_alignment_enabled"] != true {
				t.Errorf("submit body=%+v", body)
			}
			_, _ = response.Write([]byte(`{"output":{"task_status":"PENDING","task_id":"task-1"}}`))
		case "/api/v1/tasks/task-1":
			_, _ = response.Write([]byte(`{"output":{"task_status":"SUCCEEDED","results":[{"subtask_status":"SUCCEEDED","transcription_url":"` + server.URL + `/result.json"}]}}`))
		case "/result.json":
			_, _ = response.Write([]byte(`{"transcripts":[{"sentences":[{"begin_time":100,"end_time":900,"text":"hello","speaker_id":1,"words":[{"begin_time":100,"end_time":900,"text":"hello","punctuation":"."}]}]}]}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	provider, err := newAliyun(server.URL, "secret", server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	task, err := provider.Submit(context.Background(), domain.Request{AudioURL: server.URL + "/audio.flac", Model: "paraformer-v2"})
	if err != nil || task.ID != "task-1" {
		t.Fatalf("task=%+v err=%v", task, err)
	}
	status, err := provider.Poll(context.Background(), task.ID)
	if err != nil || status.State != domain.TaskSucceeded {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	result, err := provider.FetchResult(context.Background(), status.ResultURL)
	if err != nil || len(result.Segments) != 1 || result.Segments[0].LocalSpeaker != "1" || len(result.Segments[0].Words) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestSubmitServerFailureIsAmbiguous(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusInternalServerError)
		_, _ = response.Write([]byte(`{"code":"InternalError","message":"unknown outcome"}`))
	}))
	defer server.Close()
	provider, err := newAliyun(server.URL, "secret", server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Submit(context.Background(), domain.Request{AudioURL: server.URL + "/audio.flac", Model: "fun-asr"})
	var providerError *ProviderError
	if !errors.As(err, &providerError) || !providerError.AmbiguousCost() || providerError.Retryable() || !strings.Contains(err.Error(), "InternalError") {
		t.Fatalf("error=%T %v", err, err)
	}
}

func TestParseResultUsesDefaultSpeakerForNullID(t *testing.T) {
	segments, err := parseResult([]byte(`{"transcripts":[{"sentences":[{"begin_time":0,"end_time":10,"text":"hello","speaker_id":null}]}]}`))
	if err != nil || len(segments) != 1 || segments[0].LocalSpeaker != "0" {
		t.Fatalf("segments=%+v err=%v", segments, err)
	}
}

func TestFetchExpiredResultHasStableCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	provider, err := newAliyun(server.URL, "secret", server.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.FetchResult(context.Background(), server.URL+"/expired.json")
	var providerError *ProviderError
	if !errors.As(err, &providerError) || providerError.Code() != "ASR_RESULT_EXPIRED" || providerError.Retryable() {
		t.Fatalf("error=%T %v", err, err)
	}
}
