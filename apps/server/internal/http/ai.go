package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/Actify/echonote/apps/server/internal/service"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
)

func (s *Server) ListEpisodeAIArtifacts(w http.ResponseWriter, r *http.Request, episodeID string) {
	parsedEpisodeID, err := parseUUID(episodeID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "episodeId must be a UUID")
		return
	}
	artifacts, err := s.ai.Artifacts(r.Context(), s.userID, parsedEpisodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "episode was not found")
		return
	}
	if err != nil {
		s.logAIError(r, "list AI artifacts", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	items := make([]AIArtifact, len(artifacts))
	for index, artifact := range artifacts {
		items[index] = aiArtifactResponse(artifact)
	}
	writeJSON(w, http.StatusOK, AIArtifactList{Items: items})
}

func (s *Server) RequestEpisodeAIArtifact(w http.ResponseWriter, r *http.Request, episodeID string) {
	parsedEpisodeID, err := parseUUID(episodeID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_EPISODE_ID", "episodeId must be a UUID")
		return
	}
	artifact, err := s.ai.RequestArtifact(r.Context(), s.userID, parsedEpisodeID)
	switch {
	case errors.Is(err, service.ErrAIUnavailable):
		writeAPIError(w, http.StatusServiceUnavailable, "AI_DISABLED", "AI provider is not configured")
		return
	case errors.Is(err, repository.ErrAITranscriptNotReady):
		writeAPIError(w, http.StatusConflict, "TRANSCRIPT_NOT_READY", "episode has no active transcript")
		return
	case errors.Is(err, pgx.ErrNoRows):
		writeAPIError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "episode was not found")
		return
	case err != nil:
		s.logAIError(r, "request AI artifact", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	status := http.StatusAccepted
	if artifact.Cached {
		status = http.StatusOK
	}
	writeJSON(w, status, aiArtifactResponse(artifact.Artifact))
}

func (s *Server) CreateConversation(w http.ResponseWriter, r *http.Request) {
	var request CreateConversationRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return
	}
	if request.Scope != ConversationScopeEpisode {
		writeAPIError(w, http.StatusBadRequest, "AI_LIBRARY_SCOPE_UNAVAILABLE", "only episode conversations are available")
		return
	}
	episodeID, err := parseUUID(strings.TrimSpace(request.EpisodeId))
	if err != nil || (request.Title != nil && utf8.RuneCountInString(strings.TrimSpace(*request.Title)) > 200) {
		writeAPIError(w, http.StatusBadRequest, "INVALID_CONVERSATION", "episode_id or title is invalid")
		return
	}
	title := ""
	if request.Title != nil {
		title = strings.TrimSpace(*request.Title)
	}
	conversation, err := s.ai.CreateConversation(r.Context(), s.userID, episodeID, title)
	switch {
	case errors.Is(err, repository.ErrAITranscriptNotReady):
		writeAPIError(w, http.StatusConflict, "TRANSCRIPT_NOT_READY", "episode has no active transcript")
		return
	case errors.Is(err, pgx.ErrNoRows):
		writeAPIError(w, http.StatusNotFound, "EPISODE_NOT_FOUND", "episode was not found")
		return
	case err != nil:
		s.logAIError(r, "create conversation", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusCreated, conversationResponse(repository.ConversationDetail{Conversation: conversation}))
}

func (s *Server) GetConversation(w http.ResponseWriter, r *http.Request, conversationID string) {
	parsedConversationID, err := parseUUID(conversationID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_CONVERSATION_ID", "conversationId must be a UUID")
		return
	}
	detail, err := s.ai.Conversation(r.Context(), s.userID, parsedConversationID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeAPIError(w, http.StatusNotFound, "CONVERSATION_NOT_FOUND", "conversation was not found")
		return
	}
	if err != nil {
		s.logAIError(r, "get conversation", err)
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, conversationResponse(detail))
}

func (s *Server) StreamConversationMessage(w http.ResponseWriter, r *http.Request, conversationID string) {
	parsedConversationID, err := parseUUID(conversationID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_CONVERSATION_ID", "conversationId must be a UUID")
		return
	}
	var request CreateConversationMessageRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return
	}
	clientMessageID, idErr := parseUUID(strings.TrimSpace(request.ClientMessageId))
	content := strings.TrimSpace(request.Content)
	if idErr != nil || utf8.RuneCountInString(content) < 2 || utf8.RuneCountInString(content) > 2_000 {
		writeAPIError(w, http.StatusBadRequest, "INVALID_MESSAGE", "client_message_id or content is invalid")
		return
	}
	session, err := s.ai.PrepareChat(r.Context(), s.userID, parsedConversationID, clientMessageID, content)
	switch {
	case errors.Is(err, service.ErrAIUnavailable):
		writeAPIError(w, http.StatusServiceUnavailable, "AI_DISABLED", "AI provider is not configured")
		return
	case errors.Is(err, pgx.ErrNoRows):
		writeAPIError(w, http.StatusNotFound, "CONVERSATION_NOT_FOUND", "conversation was not found")
		return
	case errors.Is(err, repository.ErrChatMessageConflict):
		writeAPIError(w, http.StatusConflict, "MESSAGE_ID_CONFLICT", err.Error())
		return
	case errors.Is(err, repository.ErrChatTurnInProgress):
		writeAPIError(w, http.StatusConflict, "MESSAGE_IN_PROGRESS", err.Error())
		return
	case errors.Is(err, repository.ErrChatTurnFailed):
		writeAPIError(w, http.StatusConflict, "MESSAGE_RETRY_REQUIRES_NEW_ID", err.Error())
		return
	case errors.Is(err, repository.ErrAIContextNotFound):
		writeAPIError(w, http.StatusConflict, "AI_CONTEXT_NOT_FOUND", "no citable context was found")
		return
	case err != nil:
		s.logAIError(r, "prepare conversation message", err)
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
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	emit := func(event service.ChatStreamEvent) error {
		name, data, err := aiSSEEvent(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	if err := session.Stream(r.Context(), emit); err != nil {
		s.logAIError(r, "stream conversation message", err)
		raw, _ := json.Marshal(ErrorResponse{Code: "AI_STREAM_FAILED", Message: "AI response did not complete"})
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", raw)
		flusher.Flush()
	}
}

func aiArtifactResponse(artifact db.AiArtifact) AIArtifact {
	response := AIArtifact{
		Id: formatUUID(artifact.ID), EpisodeId: formatUUID(artifact.EpisodeID), TranscriptVersionId: formatUUID(artifact.TranscriptVersionID),
		ArtifactType: AIArtifactType(artifact.ArtifactType), Model: artifact.Model, PromptVersion: artifact.PromptVersion,
		Status: AIArtifactStatus(artifact.Status), InputTokens: int(artifact.InputTokens), OutputTokens: int(artifact.OutputTokens),
		CreatedAt: artifact.CreatedAt.Time, UpdatedAt: artifact.UpdatedAt.Time,
		ErrorCode: artifact.ErrorCode, ErrorMessage: artifact.ErrorMessage,
	}
	if artifact.JobID.Valid {
		value := formatUUID(artifact.JobID)
		response.JobId = &value
	}
	if artifact.CompletedAt.Valid {
		response.CompletedAt = &artifact.CompletedAt.Time
	}
	if len(artifact.Result) > 0 {
		var result AIArtifactResult
		if json.Unmarshal(artifact.Result, &result) == nil {
			response.Result = &result
		}
	}
	return response
}

func conversationResponse(detail repository.ConversationDetail) Conversation {
	conversation := detail.Conversation
	response := Conversation{
		Id: formatUUID(conversation.ID), Scope: ConversationScope(conversation.Scope), Title: conversation.Title,
		EpisodeTitle: conversation.EpisodeTitle, Messages: make([]ConversationMessage, len(detail.Messages)),
		CreatedAt: conversation.CreatedAt.Time, UpdatedAt: conversation.UpdatedAt.Time,
	}
	if conversation.EpisodeID.Valid {
		value := formatUUID(conversation.EpisodeID)
		response.EpisodeId = &value
	}
	for index, item := range detail.Messages {
		message := item.Message
		mapped := ConversationMessage{
			Id: formatUUID(message.ID), Role: AIMessageRole(message.Role), Status: AIMessageStatus(message.Status), Content: message.Content,
			Citations: make([]AICitation, len(item.Citations)), InputTokens: int(message.InputTokens), OutputTokens: int(message.OutputTokens),
			CreatedAt: message.CreatedAt.Time, UpdatedAt: message.UpdatedAt.Time,
			Model: message.Model, ErrorCode: message.ErrorCode, ErrorMessage: message.ErrorMessage,
		}
		if message.ReplyToMessageID.Valid {
			value := formatUUID(message.ReplyToMessageID)
			mapped.ReplyToMessageId = &value
		}
		if message.ClientMessageID.Valid {
			value := formatUUID(message.ClientMessageID)
			mapped.ClientMessageId = &value
		}
		if message.CompletedAt.Valid {
			mapped.CompletedAt = &message.CompletedAt.Time
		}
		for citationIndex, citation := range item.Citations {
			mapped.Citations[citationIndex] = aiCitation(citation.Source)
		}
		response.Messages[index] = mapped
	}
	return response
}

func aiCitation(source aidomain.CitationSource) AICitation {
	response := AICitation{
		SourceType: AICitationSourceType(source.SourceType), SourceId: source.SourceID,
		Excerpt: source.Excerpt, StartMs: source.StartMS, EndMs: source.EndMS,
	}
	if source.SpeakerID != "" {
		response.SpeakerId = &source.SpeakerID
	}
	if source.SpeakerName != "" {
		response.SpeakerName = &source.SpeakerName
	}
	return response
}

func aiSSEEvent(event service.ChatStreamEvent) (string, []byte, error) {
	switch event.Kind {
	case "delta":
		raw, err := json.Marshal(map[string]any{"text": event.Text, "replayed": event.Replayed})
		return "delta", raw, err
	case "citation":
		if event.Citation == nil {
			return "", nil, errors.New("citation event omitted citation")
		}
		raw, err := json.Marshal(aiCitation(*event.Citation))
		return "citation", raw, err
	case "done":
		raw, err := json.Marshal(map[string]any{"message_id": event.MessageID, "replayed": event.Replayed})
		return "done", raw, err
	default:
		return "", nil, errors.New("unknown AI stream event")
	}
}

func (s *Server) logAIError(r *http.Request, operation string, err error) {
	s.logger.ErrorContext(r.Context(), operation,
		"request_id", middleware.GetReqID(r.Context()), "user_id", formatUUID(s.userID), "error", err,
	)
}
