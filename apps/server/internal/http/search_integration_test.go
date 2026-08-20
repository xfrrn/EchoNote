package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database"
	podcastdomain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/Actify/echonote/apps/server/internal/service"
)

func TestSearchHTTPKeywordAndReindex(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-search-http-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	userID := randomHTTPUUID(t)
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM imports WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM episodes WHERE user_id = $1", userID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM podcasts WHERE user_id = $1", userID)
	}()

	imports := repository.NewImportRepository(pool)
	audioURL := fmt.Sprintf("https://cdn.example.com/http-search-%x.mp3", userID.Bytes)
	createdImport, err := imports.Create(ctx, userID, audioURL)
	if err != nil {
		t.Fatal(err)
	}
	episodeID, err := imports.SaveResolved(ctx, userID, createdImport.ID, audioURL, &podcastdomain.ResolvedEpisode{
		SourceType: podcastdomain.SourceDirectAudio, EpisodeTitle: "HTTP Search Episode",
		CanonicalURL: audioURL, AudioURL: audioURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	notes := repository.NewNotesRepository(pool)
	if _, err := notes.CreateForEpisode(ctx, userID, episodeID, randomHTTPUUID(t), "HTTP 关键字验收", time.Now()); err != nil {
		t.Fatal(err)
	}
	searchRepository := repository.NewSearchRepository(pool)
	if _, err := searchRepository.BuildEpisode(ctx, userID, episodeID, ""); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(
		pool, imports, repository.NewLibraryRepository(pool), notes, nil,
		service.NewSearchService(searchRepository, nil), nil, nil, false, userID,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	query := url.Values{"q": {"关键字"}, "scope": {"episode"}, "episode_id": {formatUUID(episodeID)}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/search?"+query.Encode(), nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("search status=%d body=%s", response.Code, response.Body.String())
	}
	var result SearchResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Mode != Keyword || len(result.Items) != 1 || result.Items[0].DocumentType != SearchDocumentTypeNote {
		t.Fatalf("search result=%+v err=%v", result, err)
	}

	reindexBody := `{"scope":"episode","episode_id":"` + formatUUID(episodeID) + `"}`
	request = httptest.NewRequest(http.MethodPost, "/api/v1/search/reindex", strings.NewReader(reindexBody))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"queued":1`) {
		t.Fatalf("reindex status=%d body=%s", response.Code, response.Body.String())
	}
}
