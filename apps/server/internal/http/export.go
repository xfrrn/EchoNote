package httpapi

import (
	"errors"
	"net/http"

	exportdomain "github.com/Actify/echonote/apps/server/internal/domain/export"
	"github.com/Actify/echonote/apps/server/internal/service"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) CreateEpisodeExport(w http.ResponseWriter, r *http.Request, episodeID string) {
	parsedEpisodeID, err := parseUUID(episodeID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "episodeId must be a UUID")
		return
	}
	var request CreateExportRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return
	}
	segmentIDs, err := parseExportSegmentIDs(request.TranscriptSegmentIds)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_EXPORT_OPTIONS", "transcript_segment_ids must contain unique UUIDs")
		return
	}
	organized := request.Mode == OrganizedNote
	options := exportdomain.Options{
		Mode:                  string(request.Mode),
		IncludeUserNotes:      exportBool(request.IncludeUserNotes, organized),
		IncludeSummary:        exportBool(request.IncludeSummary, organized),
		IncludeKeyPoints:      exportBool(request.IncludeKeyPoints, organized),
		IncludeWorthReviewing: exportBool(request.IncludeWorthReviewing, organized),
		IncludeTranscript:     exportBool(request.IncludeTranscript, false),
	}
	result, err := s.exports.Export(r.Context(), s.userID, parsedEpisodeID, service.ExportRequest{Options: options, SegmentIDs: segmentIDs})
	switch {
	case errors.Is(err, exportdomain.ErrInvalidOptions):
		writeAPIError(w, http.StatusBadRequest, "INVALID_EXPORT_OPTIONS", "export mode and options are inconsistent")
		return
	case errors.Is(err, exportdomain.ErrSegmentsNotFound):
		writeAPIError(w, http.StatusBadRequest, "TRANSCRIPT_SEGMENTS_NOT_FOUND", "one or more transcript segments do not belong to the active transcript")
		return
	case errors.Is(err, exportdomain.ErrArtifactNotReady):
		writeAPIError(w, http.StatusConflict, "AI_ARTIFACT_NOT_READY", "a current AI artifact is required for the requested sections")
		return
	case errors.Is(err, exportdomain.ErrTranscriptNotReady):
		writeAPIError(w, http.StatusConflict, "TRANSCRIPT_NOT_READY", "an active transcript is required for this export")
		return
	case errors.Is(err, exportdomain.ErrNoContent):
		writeAPIError(w, http.StatusConflict, "EXPORT_CONTENT_EMPTY", "the requested export has no content")
		return
	case errors.Is(err, exportdomain.ErrTooLarge):
		writeAPIError(w, http.StatusRequestEntityTooLarge, "EXPORT_TOO_LARGE", "the generated export exceeds the synchronous size limit")
		return
	case errors.Is(err, pgx.ErrNoRows):
		writeAPIError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "episode was not found")
		return
	case err != nil:
		s.logger.ErrorContext(r.Context(), "create Episode export",
			"request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "error", err,
		)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, ExportContent{
		Title: result.Title, Text: result.Text, Markdown: result.Markdown, SuggestedFilename: result.SuggestedFilename,
	})
}

func parseExportSegmentIDs(values *[]string) ([]pgtype.UUID, error) {
	if values == nil {
		return nil, nil
	}
	result := make([]pgtype.UUID, len(*values))
	seen := make(map[[16]byte]struct{}, len(*values))
	for index, value := range *values {
		id, err := parseUUID(value)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[id.Bytes]; duplicate {
			return nil, exportdomain.ErrInvalidOptions
		}
		seen[id.Bytes] = struct{}{}
		result[index] = id
	}
	return result, nil
}

func exportBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
