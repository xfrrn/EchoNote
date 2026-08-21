package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	searchdomain "github.com/Actify/echonote/apps/server/internal/domain/search"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) SearchContent(w http.ResponseWriter, r *http.Request, params SearchContentParams) {
	query := strings.TrimSpace(params.Q)
	scope, limit := SearchScopeLibrary, 20
	if params.Scope != nil {
		scope = *params.Scope
	}
	if params.Limit != nil {
		limit = *params.Limit
	}
	if !scope.Valid() || utf8.RuneCountInString(query) < 2 || utf8.RuneCountInString(query) > 500 || limit < 1 || limit > 50 {
		writeAPIError(w, http.StatusBadRequest, "INVALID_SEARCH", "q must contain 2-500 characters and scope must be library or episode")
		return
	}
	episodeID := pgtype.UUID{}
	if scope == SearchScopeEpisode {
		if params.EpisodeId == nil {
			writeAPIError(w, http.StatusBadRequest, "INVALID_SEARCH_SCOPE", "episode scope requires episode_id")
			return
		}
		var err error
		episodeID, err = parseUUID(strings.TrimSpace(*params.EpisodeId))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "episode_id must be a UUID")
			return
		}
	} else if params.EpisodeId != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_SEARCH_SCOPE", "episode_id is only valid for episode scope")
		return
	}
	output, err := s.searches.Search(r.Context(), requestUserID(r), query, string(scope), episodeID, limit)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "episode was not found")
		return
	}
	if err != nil {
		s.logger.ErrorContext(r.Context(), "search content", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(requestUserID(r)), "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if output.SemanticError != nil {
		attributes := []any{"request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(requestUserID(r))}
		attributes = append(attributes, errorLogAttributes(output.SemanticError)...)
		s.logger.WarnContext(r.Context(), "semantic search unavailable", attributes...)
	}
	items := make([]SearchResult, len(output.Items))
	for index, item := range output.Items {
		items[index] = searchResult(item)
	}
	writeJSON(w, http.StatusOK, SearchResponse{Items: items, Mode: SearchMode(output.Mode)})
}

func (s *Server) ReindexSearch(w http.ResponseWriter, r *http.Request) {
	var request ReindexSearchRequest
	if err := decodeJSON(w, r, &request); err != nil || !request.Scope.Valid() {
		writeAPIError(w, http.StatusBadRequest, "INVALID_SEARCH_SCOPE", "scope must be library or episode")
		return
	}
	episodeID := pgtype.UUID{}
	if request.Scope == SearchScopeEpisode {
		if request.EpisodeId == nil {
			writeAPIError(w, http.StatusBadRequest, "INVALID_SEARCH_SCOPE", "episode scope requires episode_id")
			return
		}
		var err error
		episodeID, err = parseUUID(strings.TrimSpace(*request.EpisodeId))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "episode_id must be a UUID")
			return
		}
	} else if request.EpisodeId != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_SEARCH_SCOPE", "episode_id is only valid for episode scope")
		return
	}
	queued, err := s.searches.Reindex(r.Context(), requestUserID(r), episodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "episode was not found")
		return
	}
	if err != nil {
		s.logger.ErrorContext(r.Context(), "reindex search", "request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(requestUserID(r)), "error", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusAccepted, ReindexSearchResponse{Queued: queued})
}

func searchResult(item searchdomain.Candidate) SearchResult {
	result := SearchResult{
		Id: item.ID, DocumentType: SearchDocumentType(item.DocumentType), SourceId: item.SourceID,
		EpisodeId: item.EpisodeID, EpisodeTitle: item.EpisodeTitle, PodcastTitle: item.PodcastTitle,
		Snippet: item.Text, Score: item.Score, StartMs: item.StartMS, EndMs: item.EndMS,
	}
	if item.SpeakerID != "" {
		result.SpeakerId = &item.SpeakerID
	}
	if item.SpeakerName != "" {
		result.SpeakerName = &item.SpeakerName
	}
	return result
}
