package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Server) CreateTranscription(w http.ResponseWriter, r *http.Request) {
	var request CreateTranscriptionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain one URL")
		return
	}
	parsedURL, err := parseHTTPURL(request.Url)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_URL", "url must be an HTTP or HTTPS URL")
		return
	}
	status, err := s.imports.Create(r.Context(), requestUserID(r), parsedURL)
	if err != nil {
		s.logTaskError(r, "create transcription", "", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	response, err := s.taskResponse(r, status)
	if err != nil {
		s.logTaskError(r, "render transcription task", formatUUID(status.ID), err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusAccepted, response)
}

func (s *Server) GetTranscription(w http.ResponseWriter, r *http.Request, taskID uuid.UUID) {
	parsedID := dbUUID(taskID)
	status, err := s.imports.Get(r.Context(), requestUserID(r), parsedID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "TRANSCRIPTION_NOT_FOUND", "transcription was not found")
		return
	}
	if err != nil {
		s.logTaskError(r, "get transcription", taskID.String(), err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	response, err := s.taskResponse(r, status)
	if err != nil {
		s.logTaskError(r, "render transcription task", taskID.String(), err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) taskResponse(r *http.Request, task db.GetImportStatusRow) (TranscriptionTask, error) {
	status, stage := taskStatus(task)
	response := TranscriptionTask{
		Id: uuid.UUID(task.ID.Bytes), SourceUrl: task.SubmittedUrl, Status: status, Stage: stage,
		CompletedChunks: int(task.CompletedChunks), TotalChunks: int(task.TotalChunks),
		CreatedAt: task.CreatedAt.Time, UpdatedAt: task.UpdatedAt.Time,
	}
	if task.Title != nil && strings.TrimSpace(*task.Title) != "" {
		response.Title = task.Title
	}
	code, message := task.ErrorCode, task.ErrorMessage
	if task.TranscriptionErrorCode != nil {
		code, message = task.TranscriptionErrorCode, task.TranscriptionErrorMessage
	}
	if status == Failed {
		response.Error = &TranscriptionError{Code: valueOr(code, "TRANSCRIPTION_FAILED"), Message: valueOr(message, "transcription failed")}
	}
	if status == Completed {
		segments, err := s.imports.Segments(r.Context(), requestUserID(r), task.ID)
		if err != nil {
			return TranscriptionTask{}, err
		}
		durationMS := int64(0)
		if task.DurationMs != nil {
			durationMS = *task.DurationMs
		}
		markdown := transcriptMarkdown(valueOr(task.Title, "Transcript"), task.SubmittedUrl, durationMS, segments)
		response.Markdown = &markdown
	}
	return response, nil
}

func taskStatus(task db.GetImportStatusRow) (TranscriptionTaskStatus, string) {
	if task.TranscriptionRunID.Valid {
		status := TranscriptionTaskStatus(task.TranscriptionStatus)
		if status == "canceled" {
			return Failed, "canceled"
		}
		return status, task.TranscriptionStage
	}
	switch task.ImportStatus {
	case "queued":
		return Queued, task.ImportStage
	case "failed", "canceled":
		return Failed, task.ImportStage
	default:
		return Resolving, task.ImportStage
	}
}

func transcriptMarkdown(title, sourceURL string, durationMS int64, segments []db.ListTranscriptionTaskSegmentsRow) string {
	var result strings.Builder
	result.WriteString("# ")
	result.WriteString(escapeMarkdown(strings.TrimSpace(title)))
	result.WriteString("\n\n> Source: <")
	result.WriteString(strings.ReplaceAll(sourceURL, ">", "%3E"))
	result.WriteString(">")
	if durationMS > 0 {
		result.WriteString("\n> Duration: ")
		result.WriteString(formatTimestamp(durationMS))
	}
	result.WriteString("\n\n## Transcript")
	for _, segment := range segments {
		result.WriteString("\n\n**")
		result.WriteString(escapeMarkdown(segment.SpeakerName))
		result.WriteString(" · ")
		result.WriteString(formatTimestamp(segment.StartMs))
		result.WriteString("**\n\n")
		result.WriteString(strings.TrimSpace(segment.Text))
	}
	return strings.TrimSpace(result.String())
}

func formatTimestamp(milliseconds int64) string {
	seconds := max(milliseconds, 0) / 1000
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}

func escapeMarkdown(value string) string {
	return strings.NewReplacer("\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]").Replace(value)
}

func valueOr(value *string, fallback string) string {
	if value != nil && strings.TrimSpace(*value) != "" {
		return *value
	}
	return fallback
}

func (s *Server) logTaskError(r *http.Request, operation, taskID string, err error) {
	s.logger.ErrorContext(r.Context(), operation,
		"request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(requestUserID(r)),
		"transcription_task_id", taskID, "error", err,
	)
}
