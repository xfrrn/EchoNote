package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	domain "github.com/Actify/echonote/apps/server/internal/domain/transcription"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
)

var languageHintPattern = regexp.MustCompile(`^[a-z]{2,3}(?:-[A-Z]{2})?$`)

func (s *Server) CreateTranscription(w http.ResponseWriter, r *http.Request, episodeID string) {
	parsedEpisodeID, err := parseUUID(episodeID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "episodeId must be a UUID")
		return
	}
	var request CreateTranscriptionRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain one transcription profile")
		return
	}
	config := repository.RunConfig{LanguageHint: strings.TrimSpace(valueOrEmpty(request.LanguageHint))}
	if request.SpeakerCount != nil {
		config.SpeakerCount = *request.SpeakerCount
	}
	if !request.Profile.Valid() || (config.LanguageHint != "" && !languageHintPattern.MatchString(config.LanguageHint)) ||
		(config.SpeakerCount != 0 && (config.SpeakerCount < 2 || config.SpeakerCount > 100)) {
		writeAPIError(w, http.StatusBadRequest, "INVALID_TRANSCRIPTION_CONFIG", "profile, language_hint, or speaker_count is invalid")
		return
	}
	if !s.transcriptionEnabled {
		writeAPIError(w, http.StatusServiceUnavailable, "TRANSCRIPTION_DISABLED", "transcription providers are not configured")
		return
	}
	run, err := s.transcriptions.Create(r.Context(), requestUserID(r), parsedEpisodeID, string(request.Profile), config)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeAPIError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "episode was not found")
	case errors.Is(err, repository.ErrEpisodeNotReady):
		writeAPIError(w, http.StatusConflict, "EPISODE_NOT_READY", "episode must finish resolving before transcription")
	case errors.Is(err, repository.ErrTranscriptionRunning):
		writeAPIError(w, http.StatusConflict, "TRANSCRIPTION_ALREADY_RUNNING", "episode already has an active transcription run")
	case err != nil:
		s.logTranscriptionError(r, "create transcription", episodeID, err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	default:
		writeJSON(w, http.StatusAccepted, transcriptionRunResponse(run))
	}
}

func (s *Server) GetTranscription(w http.ResponseWriter, r *http.Request, runID string) {
	parsedRunID, err := parseUUID(runID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_TRANSCRIPTION_ID", "runId must be a UUID")
		return
	}
	run, err := s.transcriptions.Get(r.Context(), requestUserID(r), parsedRunID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "TRANSCRIPTION_NOT_FOUND", "transcription was not found")
		return
	}
	if err != nil {
		s.logTranscriptionError(r, "get transcription", runID, err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, transcriptionRunResponse(run))
}

func (s *Server) RetryTranscription(w http.ResponseWriter, r *http.Request, runID string) {
	parsedRunID, err := parseUUID(runID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_TRANSCRIPTION_ID", "runId must be a UUID")
		return
	}
	if !s.transcriptionEnabled {
		writeAPIError(w, http.StatusServiceUnavailable, "TRANSCRIPTION_DISABLED", "transcription providers are not configured")
		return
	}
	run, err := s.transcriptions.Retry(r.Context(), requestUserID(r), parsedRunID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeAPIError(w, http.StatusNotFound, "TRANSCRIPTION_NOT_FOUND", "transcription was not found")
	case errors.Is(err, repository.ErrTranscriptionNotRetryable):
		writeAPIError(w, http.StatusConflict, "TRANSCRIPTION_NOT_RETRYABLE", "only failed transcription runs can be retried")
	case errors.Is(err, repository.ErrTranscriptionRunning):
		writeAPIError(w, http.StatusConflict, "TRANSCRIPTION_ALREADY_RUNNING", "episode already has an active transcription run")
	case err != nil:
		s.logTranscriptionError(r, "retry transcription", runID, err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	default:
		writeJSON(w, http.StatusAccepted, transcriptionRunResponse(run))
	}
}

func (s *Server) CancelTranscription(w http.ResponseWriter, r *http.Request, runID string) {
	parsedRunID, err := parseUUID(runID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_TRANSCRIPTION_ID", "runId must be a UUID")
		return
	}
	run, err := s.transcriptions.Cancel(r.Context(), requestUserID(r), parsedRunID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "TRANSCRIPTION_NOT_FOUND", "transcription was not found")
		return
	}
	if err != nil {
		s.logTranscriptionError(r, "cancel transcription", runID, err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, transcriptionRunResponse(run))
}

func (s *Server) GetTranscriptionEvents(w http.ResponseWriter, r *http.Request, runID string, params GetTranscriptionEventsParams) {
	parsedRunID, err := parseUUID(runID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_TRANSCRIPTION_ID", "runId must be a UUID")
		return
	}
	afterID := int64(0)
	if params.LastEventID != nil {
		afterID = *params.LastEventID
		if afterID < 0 {
			writeAPIError(w, http.StatusBadRequest, "INVALID_EVENT_ID", "Last-Event-ID must be a non-negative integer")
			return
		}
	}
	run, err := s.transcriptions.Get(r.Context(), requestUserID(r), parsedRunID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "TRANSCRIPTION_NOT_FOUND", "transcription was not found")
		return
	}
	if err != nil {
		s.logTranscriptionError(r, "open transcription events", runID, err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "STREAMING_UNAVAILABLE", "streaming is unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "retry: 1000\n\n")
	flusher.Flush()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		events, listErr := s.transcriptions.Events(r.Context(), requestUserID(r), parsedRunID, afterID)
		if listErr != nil {
			if r.Context().Err() != nil {
				return
			}
			s.logTranscriptionError(r, "stream transcription events", runID, listErr)
			return
		}
		for _, event := range events {
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.EventType, event.Data); err != nil {
				return
			}
			afterID = event.ID
		}
		if len(events) > 0 {
			flusher.Flush()
			continue
		}
		if terminalTranscriptionStatus(run.Status) {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			run, err = s.transcriptions.Get(r.Context(), requestUserID(r), parsedRunID)
			if err != nil {
				if r.Context().Err() != nil {
					return
				}
				s.logTranscriptionError(r, "refresh transcription events", runID, err)
				return
			}
		}
	}
}

func (s *Server) GetEpisodeTranscript(w http.ResponseWriter, r *http.Request, episodeID string) {
	parsedEpisodeID, err := parseUUID(episodeID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "episodeId must be a UUID")
		return
	}
	active, err := s.transcriptions.ActiveTranscript(r.Context(), requestUserID(r), parsedEpisodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "TRANSCRIPT_NOT_FOUND", "episode has no active transcript")
		return
	}
	if err != nil {
		s.logTranscriptionError(r, "get active transcript", episodeID, err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, transcriptResponse(active))
}

func (s *Server) ListTranscriptSegments(w http.ResponseWriter, r *http.Request, transcriptID string, params ListTranscriptSegmentsParams) {
	parsedTranscriptID, err := parseUUID(transcriptID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_TRANSCRIPT_ID", "transcriptId must be a UUID")
		return
	}
	limit, offset := 100, 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	if params.Offset != nil {
		offset = *params.Offset
	}
	if limit < 1 || limit > 500 || offset < 0 {
		writeAPIError(w, http.StatusBadRequest, "INVALID_PAGINATION", "limit must be 1-500 and offset must be non-negative")
		return
	}
	segments, total, err := s.transcriptions.Segments(r.Context(), requestUserID(r), parsedTranscriptID, int32(limit), int32(offset))
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "TRANSCRIPT_NOT_FOUND", "transcript was not found")
		return
	}
	if err != nil {
		s.logTranscriptionError(r, "list transcript segments", transcriptID, err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	items := make([]TranscriptSegment, len(segments))
	for index, segment := range segments {
		items[index] = transcriptSegmentResponse(segment)
	}
	writeJSON(w, http.StatusOK, TranscriptSegmentList{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (s *Server) UpdateTranscriptSpeaker(w http.ResponseWriter, r *http.Request, transcriptID, speakerID string) {
	parsedTranscriptID, transcriptErr := parseUUID(transcriptID)
	parsedSpeakerID, speakerErr := parseUUID(speakerID)
	if transcriptErr != nil || speakerErr != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_SPEAKER_ID", "transcriptId and speakerId must be UUIDs")
		return
	}
	var request UpdateTranscriptSpeakerRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain a display_name")
		return
	}
	displayName, role := strings.TrimSpace(request.DisplayName), strings.TrimSpace(valueOrEmpty(request.Role))
	if displayName == "" || len(displayName) > 200 || len(role) > 100 {
		writeAPIError(w, http.StatusBadRequest, "INVALID_SPEAKER", "display_name or role is invalid")
		return
	}
	speaker, err := s.transcriptions.RenameSpeaker(r.Context(), requestUserID(r), parsedTranscriptID, parsedSpeakerID, displayName, role)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "SPEAKER_NOT_FOUND", "speaker was not found")
		return
	}
	if err != nil {
		s.logTranscriptionError(r, "update transcript speaker", transcriptID, err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, transcriptSpeakerResponse(speaker))
}

func (s *Server) MergeTranscriptSpeakers(w http.ResponseWriter, r *http.Request, transcriptID string) {
	parsedTranscriptID, err := parseUUID(transcriptID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_TRANSCRIPT_ID", "transcriptId must be a UUID")
		return
	}
	var request MergeTranscriptSpeakersRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body must contain source and target speaker IDs")
		return
	}
	sourceID, sourceErr := parseUUID(request.SourceSpeakerId)
	targetID, targetErr := parseUUID(request.TargetSpeakerId)
	if sourceErr != nil || targetErr != nil || sourceID == targetID {
		writeAPIError(w, http.StatusBadRequest, "INVALID_SPEAKER_MERGE", "source and target must be different speaker UUIDs")
		return
	}
	target, err := s.transcriptions.MergeSpeakers(r.Context(), requestUserID(r), parsedTranscriptID, sourceID, targetID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "SPEAKER_NOT_FOUND", "source or target speaker was not found")
		return
	}
	if err != nil {
		s.logTranscriptionError(r, "merge transcript speakers", transcriptID, err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, transcriptSpeakerResponse(target))
}

func transcriptionRunResponse(run db.TranscriptionRun) TranscriptionRun {
	var storedConfig repository.RunConfig
	_ = json.Unmarshal(run.Config, &storedConfig)
	config := TranscriptionConfig{}
	if storedConfig.LanguageHint != "" {
		config.LanguageHint = &storedConfig.LanguageHint
	}
	if storedConfig.SpeakerCount != 0 {
		config.SpeakerCount = &storedConfig.SpeakerCount
	}
	response := TranscriptionRun{
		Id: formatUUID(run.ID), EpisodeId: formatUUID(run.EpisodeID), Profile: TranscriptionProfile(run.Profile),
		Provider: run.Provider, Model: run.Model, Status: TranscriptionRunStatus(run.Status), Stage: run.Stage,
		WorkflowVersion: int(run.Version), Config: config, DurationMs: run.DurationMs,
		TotalChunks: int(run.TotalChunks), CompletedChunks: int(run.CompletedChunks),
		CreatedAt: run.CreatedAt.Time, UpdatedAt: run.UpdatedAt.Time,
	}
	if run.StartedAt.Valid {
		startedAt := run.StartedAt.Time
		response.StartedAt = &startedAt
	}
	if run.CompletedAt.Valid {
		completedAt := run.CompletedAt.Time
		response.CompletedAt = &completedAt
	}
	if run.ErrorCode != nil {
		response.Error = &TranscriptionError{Code: *run.ErrorCode, Message: valueOr(run.ErrorMessage, "transcription failed")}
	}
	return response
}

func transcriptResponse(active repository.ActiveTranscript) Transcript {
	speakers := make([]TranscriptSpeaker, len(active.Speakers))
	for index, speaker := range active.Speakers {
		speakers[index] = transcriptSpeakerResponse(speaker)
	}
	version := active.Version
	return Transcript{
		Id: formatUUID(version.ID), EpisodeId: formatUUID(version.EpisodeID),
		TranscriptionRunId: formatUUID(version.TranscriptionRunID), Version: int(version.Version),
		IsActive: version.IsActive, Status: version.Status, Speakers: speakers, CreatedAt: version.CreatedAt.Time,
	}
}

func transcriptSpeakerResponse(speaker db.TranscriptSpeaker) TranscriptSpeaker {
	response := TranscriptSpeaker{
		Id: formatUUID(speaker.ID), StableKey: speaker.StableKey, DisplayName: speaker.DisplayName,
		Role: speaker.Role, CreatedAt: speaker.CreatedAt.Time, UpdatedAt: speaker.UpdatedAt.Time,
	}
	if speaker.SpeakerProfileID.Valid {
		profileID := formatUUID(speaker.SpeakerProfileID)
		response.SpeakerProfileId = &profileID
	}
	return response
}

func transcriptSegmentResponse(segment db.TranscriptSegment) TranscriptSegment {
	var words []domain.Word
	_ = json.Unmarshal(segment.Words, &words)
	resultWords := make([]TranscriptWord, len(words))
	for index, word := range words {
		resultWords[index] = TranscriptWord{StartMs: word.StartMS, EndMs: word.EndMS, Text: word.Text}
		if word.Punctuation != "" {
			resultWords[index].Punctuation = &word.Punctuation
		}
	}
	return TranscriptSegment{
		Id: formatUUID(segment.ID), SpeakerId: formatUUID(segment.SpeakerID), Sequence: int(segment.Sequence),
		StartMs: segment.StartMs, EndMs: segment.EndMs, Text: segment.Text, Words: resultWords,
		SourceChunkId: formatUUID(segment.SourceChunkID),
	}
}

func terminalTranscriptionStatus(status string) bool {
	return status == "completed" || status == "failed" || status == "canceled"
}

func valueOr(value *string, fallback string) string {
	if value != nil && strings.TrimSpace(*value) != "" {
		return *value
	}
	return fallback
}

func (s *Server) logTranscriptionError(r *http.Request, operation, entityID string, err error) {
	s.logger.ErrorContext(r.Context(), operation,
		"request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(requestUserID(r)),
		"transcription_entity_id", entityID, "error", err,
	)
}
