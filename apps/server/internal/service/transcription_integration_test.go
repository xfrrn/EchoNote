package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database"
	"github.com/Actify/echonote/apps/server/internal/database/db"
	podcastdomain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
	transcriptiondomain "github.com/Actify/echonote/apps/server/internal/domain/transcription"
	"github.com/Actify/echonote/apps/server/internal/repository"
	workerapp "github.com/Actify/echonote/apps/server/internal/worker"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestTranscriptionWorkflowCompletesLongAudioWithSwappedSpeakers(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-transcription-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID := randomServiceUUID(t)
	defer ensureTestUsers(t, pool, userID)()
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM imports WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM episodes WHERE user_id = $1", userID)
	}()

	imports := repository.NewImportRepository(pool)
	createdImport, err := imports.Create(ctx, userID, "https://cdn.example.com/three-hours.mp3")
	if err != nil {
		t.Fatal(err)
	}
	episodeID, err := imports.SaveResolved(ctx, userID, createdImport.ID, createdImport.SubmittedUrl, &podcastdomain.ResolvedEpisode{
		SourceType: podcastdomain.SourceDirectAudio, EpisodeTitle: "Three hour conversation",
		CanonicalURL: createdImport.SubmittedUrl, AudioURL: createdImport.SubmittedUrl,
		DurationMS: 2 * transcriptiondomain.CoreWindowMS,
	})
	if err != nil {
		t.Fatal(err)
	}

	transcriptions := repository.NewTranscriptionRepository(pool, "fun-asr")
	run, err := transcriptions.Create(ctx, userID, episodeID)
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeObjectStore{objects: make(map[string][]byte)}
	asr := &fakeASR{polls: make(map[string]int)}
	workflow := NewTranscriptionWorkflow(transcriptions, fakeDownloader{}, fakeProcessor{}, store, asr, time.Millisecond)
	queue := repository.NewJobQueue(pool)
	process := workerapp.New(
		queue,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		"transcription-test-worker",
		time.Millisecond,
		time.Second,
		workflow.Handlers(),
	)
	workerContext, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	go func() { workerDone <- process.Run(workerContext) }()

	completed := waitForRun(t, ctx, transcriptions, userID, run.ID, "completed")
	waitForRunCleanup(t, ctx, transcriptions, userID, run.ID)
	waitForStoreEmpty(t, ctx, store)
	stopWorker()
	if err := <-workerDone; err != nil {
		t.Fatal(err)
	}
	if completed.TotalChunks != 2 || completed.CompletedChunks != 2 {
		t.Fatalf("run=%+v", completed)
	}
	active, err := transcriptions.ActiveTranscript(ctx, userID, episodeID)
	if err != nil {
		t.Fatal(err)
	}
	if active.Version.Version != 1 || !active.Version.IsActive || len(active.Speakers) != 2 {
		t.Fatalf("active=%+v", active)
	}
	segments, total, err := transcriptions.Segments(ctx, userID, active.Version.ID, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || len(segments) != 5 {
		t.Fatalf("segments=%d total=%d values=%+v", len(segments), total, segments)
	}
	boundaryCount := 0
	for _, segment := range segments {
		if segment.Text == "boundary" {
			boundaryCount++
		}
	}
	if boundaryCount != 1 {
		t.Fatalf("boundary segment count=%d", boundaryCount)
	}
	chunks, err := transcriptions.Chunks(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var firstMap, secondMap map[string]string
	if json.Unmarshal(chunks[0].SpeakerMap, &firstMap) != nil || json.Unmarshal(chunks[1].SpeakerMap, &secondMap) != nil {
		t.Fatal("speaker maps are not valid JSON")
	}
	if firstMap["0"] != secondMap["9"] || firstMap["1"] != secondMap["7"] {
		t.Fatalf("first=%v second=%v", firstMap, secondMap)
	}
	sameRun, err := transcriptions.Create(ctx, userID, episodeID)
	if err != nil {
		t.Fatal(err)
	}
	if sameRun.ID != run.ID {
		t.Fatalf("duplicate run=%v, want %v", sameRun.ID, run.ID)
	}
}

func TestCleanupDeletesExplicitObjects(t *testing.T) {
	store := &fakeObjectStore{objects: map[string][]byte{"source": {}, "chunk": {}}}
	workflow := &TranscriptionWorkflow{store: store}
	job := db.Job{Payload: json.RawMessage(`{"scope":"objects","keys":["source","chunk"]}`)}
	if err := workflow.cleanup(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if store.count() != 0 {
		t.Fatalf("objects remaining=%d", store.count())
	}
}

func waitForRun(
	t *testing.T,
	ctx context.Context,
	repository *repository.TranscriptionRepository,
	userID, runID pgtype.UUID,
	want string,
) db.TranscriptionRun {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		run, err := repository.Get(ctx, userID, runID)
		if err != nil {
			t.Fatal(err)
		}
		if run.Status == want {
			return run
		}
		if run.Status == "failed" {
			t.Fatalf("run failed: code=%v message=%v", run.ErrorCode, run.ErrorMessage)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}

func waitForStoreEmpty(t *testing.T, ctx context.Context, store *fakeObjectStore) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for store.count() != 0 {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}

func waitForRunCleanup(
	t *testing.T,
	ctx context.Context,
	repository *repository.TranscriptionRepository,
	userID, runID pgtype.UUID,
) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		run, err := repository.Get(ctx, userID, runID)
		if err != nil {
			t.Fatal(err)
		}
		if run.AudioCleanedAt.Valid && run.ChunksCleanedAt.Valid {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}

type fakeDownloader struct{}

func (fakeDownloader) Download(_ context.Context, _ string, _ map[string]string, destination string) (string, error) {
	data := []byte("source-audio")
	return digest(data), os.WriteFile(destination, data, 0o600)
}

type fakeProcessor struct{}

func (fakeProcessor) Prepare(_ context.Context, _ string, destination string) (int64, string, error) {
	data := []byte("prepared-audio")
	return 2 * transcriptiondomain.CoreWindowMS, digest(data), os.WriteFile(destination, data, 0o600)
}

func (fakeProcessor) Render(_ context.Context, _ string, startMS, endMS int64, destination string) (string, error) {
	data := []byte(strings.Join([]string{"chunk", time.Duration(startMS).String(), time.Duration(endMS).String()}, "-"))
	return digest(data), os.WriteFile(destination, data, 0o600)
}

type fakeObjectStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (store *fakeObjectStore) Put(_ context.Context, key string, reader io.Reader) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.objects[key] = data
	return nil
}

func (store *fakeObjectStore) Delete(_ context.Context, key string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.objects, key)
	return nil
}

func (*fakeObjectStore) SignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://objects.example/" + url.PathEscape(key), nil
}

func (store *fakeObjectStore) count() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.objects)
}

type fakeASR struct {
	mu    sync.Mutex
	polls map[string]int
}

func (provider *fakeASR) Submit(_ context.Context, request transcriptiondomain.Request) (transcriptiondomain.ExternalTask, error) {
	parsed, err := url.Parse(request.AudioURL)
	if err != nil {
		return transcriptiondomain.ExternalTask{}, err
	}
	decoded, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil {
		return transcriptiondomain.ExternalTask{}, err
	}
	sequence := strings.TrimSuffix(path.Base(decoded), ".flac")
	return transcriptiondomain.ExternalTask{ID: "task-" + sequence}, nil
}

func (provider *fakeASR) Poll(_ context.Context, taskID string) (transcriptiondomain.ExternalTaskStatus, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.polls[taskID]++
	if provider.polls[taskID] == 1 {
		return transcriptiondomain.ExternalTaskStatus{State: transcriptiondomain.TaskRunning}, nil
	}
	return transcriptiondomain.ExternalTaskStatus{State: transcriptiondomain.TaskSucceeded, ResultURL: "https://asr.example/results/" + strings.TrimPrefix(taskID, "task-")}, nil
}

func (*fakeASR) FetchResult(_ context.Context, resultURL string) (transcriptiondomain.RawResult, error) {
	sequence := path.Base(resultURL)
	boundary, overlap := transcriptiondomain.CoreWindowMS, transcriptiondomain.OverlapMS
	segments := []transcriptiondomain.Segment{}
	if sequence == "0000" {
		segments = []transcriptiondomain.Segment{
			{LocalSequence: 0, LocalSpeaker: "0", StartMS: 1000, EndMS: 4000, Text: "first"},
			{LocalSequence: 1, LocalSpeaker: "0", StartMS: boundary - overlap + 10_000, EndMS: boundary - overlap + 14_000, Text: "host overlap"},
			{LocalSequence: 2, LocalSpeaker: "1", StartMS: boundary + 10_000, EndMS: boundary + 14_000, Text: "guest overlap"},
			{LocalSequence: 3, LocalSpeaker: "0", StartMS: boundary - 1000, EndMS: boundary + 1000, Text: "boundary"},
		}
	} else {
		segments = []transcriptiondomain.Segment{
			{LocalSequence: 0, LocalSpeaker: "9", StartMS: 10_000, EndMS: 14_000, Text: "host overlap"},
			{LocalSequence: 1, LocalSpeaker: "7", StartMS: overlap + 10_000, EndMS: overlap + 14_000, Text: "guest overlap"},
			{LocalSequence: 2, LocalSpeaker: "9", StartMS: overlap - 1000, EndMS: overlap + 1000, Text: "boundary"},
			{LocalSequence: 3, LocalSpeaker: "7", StartMS: overlap + 20_000, EndMS: overlap + 24_000, Text: "second"},
		}
	}
	return transcriptiondomain.RawResult{Raw: []byte(`{"transcripts":[]}`), Segments: segments}, nil
}

func digest(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
