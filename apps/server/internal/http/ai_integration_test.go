package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database"
	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
	podcastdomain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/Actify/echonote/apps/server/internal/service"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeLLMProvider struct {
	artifact          string
	segmentID         string
	artifactCalls     int
	chatCalls         int
	invalidNext       bool
	transientFailures int
}

func (*fakeLLMProvider) Model() string { return "fake-qwen-v1" }

func (provider *fakeLLMProvider) GenerateStructured(context.Context, aidomain.StructuredGenerationRequest) (aidomain.StructuredGenerationResult, error) {
	provider.artifactCalls++
	if provider.transientFailures > 0 {
		provider.transientFailures--
		return aidomain.StructuredGenerationResult{}, transientLLMError{}
	}
	return aidomain.StructuredGenerationResult{Content: provider.artifact, Usage: aidomain.Usage{InputTokens: 120, OutputTokens: 40}}, nil
}

type transientLLMError struct{}

func (transientLLMError) Error() string             { return "temporary provider failure" }
func (transientLLMError) Code() string              { return "AI_PROVIDER_FAILED" }
func (transientLLMError) Retryable() bool           { return true }
func (transientLLMError) ProviderName() string      { return "fake_llm" }
func (transientLLMError) ProviderOperation() string { return "generate" }
func (transientLLMError) ProviderStatus() int       { return http.StatusTooManyRequests }

func (provider *fakeLLMProvider) StreamChat(context.Context, aidomain.ChatRequest) (<-chan aidomain.ChatEvent, error) {
	provider.chatCalls++
	citationID := "segment:" + provider.segmentID
	if provider.invalidNext {
		provider.invalidNext = false
		citationID = "segment:00000000-0000-0000-0000-000000000000"
	}
	events := make(chan aidomain.ChatEvent, 3)
	events <- aidomain.ChatEvent{Delta: "根据资料，公司完成融资。"}
	events <- aidomain.ChatEvent{Delta: aidomain.CitationOpen + `{"ids":["` + citationID + `"]}` + aidomain.CitationClose}
	events <- aidomain.ChatEvent{Usage: &aidomain.Usage{InputTokens: 30, OutputTokens: 12}}
	close(events)
	return events, nil
}

func TestAIArtifactAndConversationHTTP(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-ai-http-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID, otherUserID := randomHTTPUUID(t), randomHTTPUUID(t)
	defer ensureTestUsers(t, pool, userID, otherUserID)()
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM imports WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM episodes WHERE user_id IN ($1, $2)", userID, otherUserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM podcasts WHERE user_id IN ($1, $2)", userID, otherUserID)
	}()

	imports := repository.NewImportRepository(pool)
	audioURL := fmt.Sprintf("https://cdn.example.com/ai-%x.mp3", userID.Bytes)
	createdImport, err := imports.Create(ctx, userID, audioURL)
	if err != nil {
		t.Fatal(err)
	}
	episodeID, err := imports.SaveResolved(ctx, userID, createdImport.ID, audioURL, &podcastdomain.ResolvedEpisode{
		SourceType: podcastdomain.SourceDirectAudio, EpisodeTitle: "AI 验收节目", CanonicalURL: audioURL, AudioURL: audioURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	notes := repository.NewNotesRepository(pool)
	note, err := notes.CreateForEpisode(ctx, userID, episodeID, randomHTTPUUID(t), "关注海外扩张", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	speakerID, segmentID := createAITranscript(t, ctx, pool, userID, episodeID)

	searchRepository := repository.NewSearchRepository(pool)
	if build, err := searchRepository.BuildEpisode(ctx, userID, episodeID, ""); err != nil || build.Documents != 2 {
		t.Fatalf("initial search build=%+v err=%v", build, err)
	}
	provider := &fakeLLMProvider{segmentID: formatUUID(segmentID)}
	provider.artifact = fmt.Sprintf(`{"one_sentence_summary":"artifactonlytoken 融资扩张","key_points":["完成融资"],"speaker_views":[{"speaker_id":"%s","points":["拓展海外市场"]}],"worth_reviewing":[{"transcript_segment_id":"%s","reason":"核心事实"}],"note_connections":[{"note_id":"%s","insight":"与海外扩张相关"}]}`, formatUUID(speakerID), formatUUID(segmentID), formatUUID(note.Note.ID))
	aiRepository := repository.NewAIRepository(pool)
	searchService := service.NewSearchService(searchRepository, nil)
	aiService := service.NewAIService(aiRepository, searchService, provider)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := NewRouter(pool, imports, repository.NewLibraryRepository(pool), notes, nil, searchService, aiService, nil, false, userID, logger)

	artifactPath := "/api/v1/episodes/" + formatUUID(episodeID) + "/ai/artifacts"
	response := serveAIRequest(router, http.MethodPost, artifactPath, "")
	if response.Code != http.StatusAccepted {
		t.Fatalf("request artifact status=%d body=%s", response.Code, response.Body.String())
	}
	queue := repository.NewJobQueue(pool)
	job, found, err := queue.Claim(ctx, "ai-test-worker", []string{repository.GenerateAIArtifactJobType})
	if err != nil || !found {
		var jobs string
		_ = pool.QueryRow(ctx, "SELECT COALESCE(string_agg(type || ':' || status || ':' || (run_after - now())::text, ','), '') FROM jobs WHERE user_id = $1", userID).Scan(&jobs)
		t.Fatalf("claim artifact job found=%t err=%v jobs=%s", found, err, jobs)
	}
	if err := service.NewAIWorkflow(aiRepository, provider).Handlers()[repository.GenerateAIArtifactJobType](ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := queue.Complete(ctx, job.ID, "ai-test-worker"); err != nil {
		t.Fatal(err)
	}

	response = serveAIRequest(router, http.MethodGet, artifactPath, "")
	var artifacts AIArtifactList
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &artifacts) != nil || len(artifacts.Items) != 1 || string(artifacts.Items[0].Status) != "ready" || artifacts.Items[0].Result == nil || artifacts.Items[0].Result.WorthReviewing[0].Quote != "公司完成新一轮融资，准备拓展海外市场" {
		t.Fatalf("artifact status=%d result=%+v body=%s", response.Code, artifacts, response.Body.String())
	}
	response = serveAIRequest(router, http.MethodPost, artifactPath, "")
	if response.Code != http.StatusOK || provider.artifactCalls != 1 {
		t.Fatalf("cached artifact status=%d calls=%d body=%s", response.Code, provider.artifactCalls, response.Body.String())
	}
	if build, err := searchRepository.BuildEpisode(ctx, userID, episodeID, ""); err != nil || build.Documents != 3 {
		t.Fatalf("artifact search build=%+v err=%v", build, err)
	}
	searchResult, err := searchService.Search(ctx, userID, "artifactonlytoken", "episode", episodeID, 10)
	if err != nil || len(searchResult.Items) != 1 || searchResult.Items[0].DocumentType != "ai_artifact" {
		t.Fatalf("artifact search=%+v err=%v", searchResult, err)
	}

	if _, err := notes.Update(ctx, userID, note.Note.ID, "更新后的海外扩张观察"); err != nil {
		t.Fatal(err)
	}
	response = serveAIRequest(router, http.MethodGet, artifactPath, "")
	artifacts = AIArtifactList{}
	if json.Unmarshal(response.Body.Bytes(), &artifacts) != nil || len(artifacts.Items) != 1 || string(artifacts.Items[0].Status) != "stale" {
		t.Fatalf("stale artifact status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := searchRepository.BuildEpisode(ctx, userID, episodeID, ""); err != nil {
		t.Fatal(err)
	}
	searchResult, err = searchService.Search(ctx, userID, "artifactonlytoken", "episode", episodeID, 10)
	if err != nil || len(searchResult.Items) != 0 {
		t.Fatalf("stale artifact remained searchable: result=%+v err=%v", searchResult, err)
	}

	provider.transientFailures = 1
	response = serveAIRequest(router, http.MethodPost, artifactPath, "")
	if response.Code != http.StatusAccepted {
		t.Fatalf("request retry artifact status=%d body=%s", response.Code, response.Body.String())
	}
	job, found, err = queue.Claim(ctx, "ai-test-worker", []string{repository.GenerateAIArtifactJobType})
	if err != nil || !found {
		t.Fatalf("claim retry artifact found=%t err=%v", found, err)
	}
	handler := service.NewAIWorkflow(aiRepository, provider).Handlers()[repository.GenerateAIArtifactJobType]
	if err := handler(ctx, job); err == nil {
		t.Fatal("transient provider failure was not returned")
	}
	if status, err := queue.RetryOrFail(ctx, job.ID, "ai-test-worker", "AI_PROVIDER_FAILED", "temporary provider failure", time.Millisecond, true); err != nil || status != "queued" {
		t.Fatalf("retry artifact job status=%q err=%v", status, err)
	}
	time.Sleep(5 * time.Millisecond)
	job, found, err = queue.Claim(ctx, "ai-test-worker", []string{repository.GenerateAIArtifactJobType})
	if err != nil || !found {
		t.Fatalf("reclaim retry artifact found=%t err=%v", found, err)
	}
	if err := handler(ctx, job); err != nil {
		t.Fatal(err)
	}
	if err := queue.Complete(ctx, job.ID, "ai-test-worker"); err != nil {
		t.Fatal(err)
	}
	response = serveAIRequest(router, http.MethodGet, artifactPath, "")
	artifacts = AIArtifactList{}
	if json.Unmarshal(response.Body.Bytes(), &artifacts) != nil || len(artifacts.Items) != 2 || string(artifacts.Items[0].Status) != "ready" {
		t.Fatalf("retried artifact status=%d body=%s", response.Code, response.Body.String())
	}

	conversationBody := `{"scope":"episode","episode_id":"` + formatUUID(episodeID) + `"}`
	response = serveAIRequest(router, http.MethodPost, "/api/v1/conversations", conversationBody)
	var conversation Conversation
	if response.Code != http.StatusCreated || json.Unmarshal(response.Body.Bytes(), &conversation) != nil || conversation.Id == "" {
		t.Fatalf("create conversation status=%d body=%s", response.Code, response.Body.String())
	}
	messageID := formatUUID(randomHTTPUUID(t))
	messageBody := `{"client_message_id":"` + messageID + `","content":"融资重点是什么？"}`
	messagePath := "/api/v1/conversations/" + conversation.Id + "/messages"
	response = serveAIRequest(router, http.MethodPost, messagePath, messageBody)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "event: citation") || !strings.Contains(response.Body.String(), "event: done") || strings.Contains(response.Body.String(), aidomain.CitationOpen) {
		t.Fatalf("chat stream status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveAIRequest(router, http.MethodGet, "/api/v1/conversations/"+conversation.Id, "")
	conversation = Conversation{}
	if json.Unmarshal(response.Body.Bytes(), &conversation) != nil || len(conversation.Messages) != 2 || string(conversation.Messages[0].Role) != "user" || string(conversation.Messages[1].Role) != "assistant" || string(conversation.Messages[1].Status) != "completed" || len(conversation.Messages[1].Citations) != 1 {
		t.Fatalf("stored conversation status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveAIRequest(router, http.MethodPost, messagePath, messageBody)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"replayed":true`) || provider.chatCalls != 1 {
		t.Fatalf("chat replay status=%d calls=%d body=%s", response.Code, provider.chatCalls, response.Body.String())
	}
	response = serveAIRequest(router, http.MethodPost, messagePath, `{"client_message_id":"`+messageID+`","content":"不同问题"}`)
	if response.Code != http.StatusConflict {
		t.Fatalf("message conflict status=%d body=%s", response.Code, response.Body.String())
	}

	provider.invalidNext = true
	badBody := `{"client_message_id":"` + formatUUID(randomHTTPUUID(t)) + `","content":"请给出虚构引用"}`
	response = serveAIRequest(router, http.MethodPost, messagePath, badBody)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "event: error") || strings.Contains(response.Body.String(), "event: done") {
		t.Fatalf("invalid citation stream status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveAIRequest(router, http.MethodGet, "/api/v1/conversations/"+conversation.Id, "")
	conversation = Conversation{}
	if json.Unmarshal(response.Body.Bytes(), &conversation) != nil || len(conversation.Messages) != 4 || string(conversation.Messages[3].Status) != "failed" {
		t.Fatalf("failed message was not persisted: status=%d body=%s", response.Code, response.Body.String())
	}
	conversationID, _ := parseUUID(conversation.Id)
	history, err := aiRepository.ConversationHistory(ctx, userID, conversationID, randomHTTPUUID(t))
	if err != nil || len(history) != 2 || history[0].Role != "user" || history[1].Role != "assistant" {
		t.Fatalf("conversation history=%+v err=%v", history, err)
	}

	otherRouter := NewRouter(pool, nil, nil, nil, nil, nil, aiService, nil, false, otherUserID, logger)
	response = serveAIRequest(otherRouter, http.MethodGet, "/api/v1/conversations/"+conversation.Id, "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("conversation isolation status=%d body=%s", response.Code, response.Body.String())
	}
}

func createAITranscript(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID, episodeID pgtype.UUID) (pgtype.UUID, pgtype.UUID) {
	t.Helper()
	var runID, chunkID, transcriptID, speakerID, segmentID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO transcription_runs (user_id, episode_id, profile, provider, model, status, stage, total_chunks, completed_chunks, completed_at) VALUES ($1, $2, 'economy', 'test', 'test', 'completed', 'completed', 1, 1, now()) RETURNING id`, userID, episodeID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO transcription_chunks (transcription_run_id, sequence, core_start_ms, core_end_ms, render_start_ms, render_end_ms, status, completed_at) VALUES ($1, 0, 0, 60000, 0, 60000, 'completed', now()) RETURNING id`, runID).Scan(&chunkID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO transcript_versions (user_id, episode_id, transcription_run_id, version, is_active) VALUES ($1, $2, $3, 1, true) RETURNING id`, userID, episodeID, runID).Scan(&transcriptID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO transcript_speakers (transcript_version_id, stable_key, display_name) VALUES ($1, 'global-1', '主讲人') RETURNING id`, transcriptID).Scan(&speakerID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO transcript_segments (transcript_version_id, speaker_id, sequence, start_ms, end_ms, text, source_chunk_id) VALUES ($1, $2, 0, 1000, 5000, '公司完成新一轮融资，准备拓展海外市场', $3) RETURNING id`, transcriptID, speakerID, chunkID).Scan(&segmentID); err != nil {
		t.Fatal(err)
	}
	return speakerID, segmentID
}

func serveAIRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
