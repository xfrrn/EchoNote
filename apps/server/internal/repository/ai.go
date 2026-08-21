package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
	searchdomain "github.com/Actify/echonote/apps/server/internal/domain/search"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	GenerateAIArtifactJobType = "generate_ai_artifact"
	AIArtifactEntity          = "ai_artifact"
	maxCitationSources        = 12
	maxCitationContextRunes   = 20_000
)

var (
	ErrChatMessageConflict  = errors.New("client message ID was already used with different content")
	ErrChatTurnInProgress   = errors.New("chat turn is already in progress")
	ErrChatTurnFailed       = errors.New("chat turn already failed; use a new client message ID to retry")
	ErrAIContextNotFound    = errors.New("no citable AI context was found")
	ErrAITranscriptNotReady = errors.New("episode has no active transcript")
)

type AIRepository struct {
	pool *pgxpool.Pool
}

type ArtifactSchedule struct {
	Artifact db.AiArtifact
	Cached   bool
	Queued   bool
}

type ArtifactGeneration struct {
	Artifact db.AiArtifact
	Input    aidomain.EpisodeInput
	Run      bool
}

type ConversationRecord struct {
	ID           pgtype.UUID
	UserID       pgtype.UUID
	EpisodeID    pgtype.UUID
	Scope        string
	Title        string
	EpisodeTitle string
	CreatedAt    pgtype.Timestamptz
	UpdatedAt    pgtype.Timestamptz
}

type CitationRecord struct {
	MessageID pgtype.UUID
	Position  int32
	Source    aidomain.CitationSource
}

type ConversationMessage struct {
	Message   db.Message
	Citations []CitationRecord
}

type ConversationDetail struct {
	Conversation ConversationRecord
	Messages     []ConversationMessage
}

type ChatTurn struct {
	Conversation ConversationRecord
	UserMessage  db.Message
	Assistant    db.Message
	Replay       bool
}

func NewAIRepository(pool *pgxpool.Pool) *AIRepository {
	return &AIRepository{pool: pool}
}

func (repository *AIRepository) RequestArtifact(
	ctx context.Context,
	userID, episodeID pgtype.UUID,
	model string,
) (ArtifactSchedule, error) {
	if !userID.Valid || !episodeID.Valid || strings.TrimSpace(model) == "" {
		return ArtifactSchedule{}, errors.New("user, episode, and model are required")
	}
	result, err := withTx(ctx, repository.pool, func(queries *db.Queries) (ArtifactSchedule, error) {
		episode, err := queries.LockOwnedEpisodeForAI(ctx, db.LockOwnedEpisodeForAIParams{EpisodeID: episodeID, UserID: userID})
		if err != nil {
			return ArtifactSchedule{}, err
		}
		input, err := loadEpisodeAIInput(ctx, queries, userID, episodeID, episode.Title)
		if errors.Is(err, pgx.ErrNoRows) {
			return ArtifactSchedule{}, ErrAITranscriptNotReady
		}
		if err != nil {
			return ArtifactSchedule{}, err
		}
		params := db.GetCachedAIArtifactParams{
			UserID: userID, EpisodeID: episodeID, ArtifactType: aidomain.ArtifactTypeEpisodeSummary,
			TranscriptVersionID: parseIDOrZero(input.TranscriptVersionID), NotesRevision: input.NotesRevision(),
			InputHash: input.InputHash(), Model: model, PromptVersion: aidomain.ArtifactPromptVersion,
		}
		artifact, cacheErr := queries.GetCachedAIArtifact(ctx, params)
		if cacheErr == nil && (artifact.Status == "ready" || artifact.Status == "queued" || artifact.Status == "generating") {
			return ArtifactSchedule{Artifact: artifact, Cached: artifact.Status == "ready"}, nil
		}
		if cacheErr != nil && !errors.Is(cacheErr, pgx.ErrNoRows) {
			return ArtifactSchedule{}, cacheErr
		}
		exceptID := pgtype.UUID{}
		if cacheErr == nil {
			exceptID = artifact.ID
		}
		if err := staleOtherAIArtifacts(ctx, queries, userID, episodeID, exceptID); err != nil {
			return ArtifactSchedule{}, err
		}
		if cacheErr == nil && artifact.Status == "stale" && len(artifact.Result) > 0 {
			artifact, err = queries.ReactivateCachedAIArtifact(ctx, db.ReactivateCachedAIArtifactParams{ArtifactID: artifact.ID, UserID: userID})
			if err != nil {
				return ArtifactSchedule{}, err
			}
			if err := queries.SetEpisodeAIStatus(ctx, db.SetEpisodeAIStatusParams{Status: "completed", EpisodeID: episodeID, UserID: userID}); err != nil {
				return ArtifactSchedule{}, err
			}
			if err := enqueueSearchBuild(ctx, queries, userID, episodeID); err != nil {
				return ArtifactSchedule{}, err
			}
			return ArtifactSchedule{Artifact: artifact, Cached: true}, nil
		}
		if cacheErr == nil {
			artifact, err = queries.ResetAIArtifactForGeneration(ctx, db.ResetAIArtifactForGenerationParams{ArtifactID: artifact.ID, UserID: userID})
		} else {
			artifact, err = queries.CreateAIArtifact(ctx, db.CreateAIArtifactParams{
				UserID: userID, EpisodeID: episodeID, TranscriptVersionID: params.TranscriptVersionID,
				ArtifactType: params.ArtifactType, Model: model, PromptVersion: params.PromptVersion,
				NotesRevision: params.NotesRevision, InputHash: params.InputHash,
			})
		}
		if err != nil {
			return ArtifactSchedule{}, err
		}
		payload, _ := json.Marshal(map[string]string{"input_hash": artifact.InputHash})
		job, err := enqueue(ctx, queries, NewJob{
			UserID: userID, Type: GenerateAIArtifactJobType, EntityType: AIArtifactEntity,
			EntityID: artifact.ID, Payload: payload, MaxAttempts: 3,
		})
		if err != nil {
			return ArtifactSchedule{}, err
		}
		artifact, err = queries.AttachAIArtifactJob(ctx, db.AttachAIArtifactJobParams{JobID: job.ID, ArtifactID: artifact.ID, UserID: userID})
		if err != nil {
			return ArtifactSchedule{}, err
		}
		if err := queries.SetEpisodeAIStatus(ctx, db.SetEpisodeAIStatusParams{Status: "queued", EpisodeID: episodeID, UserID: userID}); err != nil {
			return ArtifactSchedule{}, err
		}
		return ArtifactSchedule{Artifact: artifact, Queued: true}, nil
	})
	if err != nil {
		return ArtifactSchedule{}, fmt.Errorf("request AI artifact: %w", err)
	}
	return result, nil
}

func (repository *AIRepository) ListArtifacts(ctx context.Context, userID, episodeID pgtype.UUID) ([]db.AiArtifact, error) {
	queries := db.New(repository.pool)
	if _, err := queries.GetOwnedEpisodeID(ctx, db.GetOwnedEpisodeIDParams{EpisodeID: episodeID, UserID: userID}); err != nil {
		return nil, err
	}
	if reconciled, err := queries.ReconcileFailedAIArtifacts(ctx, db.ReconcileFailedAIArtifactsParams{UserID: userID, EpisodeID: episodeID}); err != nil {
		return nil, fmt.Errorf("reconcile AI artifacts: %w", err)
	} else if reconciled > 0 {
		if err := queries.SetEpisodeAIStatus(ctx, db.SetEpisodeAIStatusParams{Status: "failed", EpisodeID: episodeID, UserID: userID}); err != nil {
			return nil, err
		}
	}
	artifacts, err := queries.ListEpisodeAIArtifacts(ctx, db.ListEpisodeAIArtifactsParams{UserID: userID, EpisodeID: episodeID})
	if err != nil {
		return nil, fmt.Errorf("list AI artifacts: %w", err)
	}
	return artifacts, nil
}

func (repository *AIRepository) BeginArtifactGeneration(
	ctx context.Context,
	userID, artifactID, jobID pgtype.UUID,
) (ArtifactGeneration, error) {
	artifact, err := db.New(repository.pool).GetAIArtifactForJob(ctx, db.GetAIArtifactForJobParams{
		ArtifactID: artifactID, UserID: userID, JobID: jobID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ArtifactGeneration{}, nil
	}
	if err != nil {
		return ArtifactGeneration{}, err
	}
	return withTx(ctx, repository.pool, func(queries *db.Queries) (ArtifactGeneration, error) {
		episode, err := queries.LockOwnedEpisodeForAI(ctx, db.LockOwnedEpisodeForAIParams{EpisodeID: artifact.EpisodeID, UserID: userID})
		if err != nil {
			return ArtifactGeneration{}, err
		}
		artifact, err = queries.StartAIArtifactGeneration(ctx, db.StartAIArtifactGenerationParams{
			ArtifactID: artifactID, UserID: userID, JobID: jobID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ArtifactGeneration{}, nil
		}
		if err != nil {
			return ArtifactGeneration{}, err
		}
		input, err := loadEpisodeAIInput(ctx, queries, userID, artifact.EpisodeID, episode.Title)
		if errors.Is(err, pgx.ErrNoRows) {
			_ = queries.MarkAIArtifactStale(ctx, db.MarkAIArtifactStaleParams{ArtifactID: artifactID, UserID: userID})
			_ = queries.SetEpisodeAIStatus(ctx, db.SetEpisodeAIStatusParams{Status: "waiting", EpisodeID: artifact.EpisodeID, UserID: userID})
			return ArtifactGeneration{}, nil
		}
		if err != nil {
			return ArtifactGeneration{Artifact: artifact, Run: true}, err
		}
		if artifact.TranscriptVersionID != parseIDOrZero(input.TranscriptVersionID) || artifact.NotesRevision != input.NotesRevision() || artifact.InputHash != input.InputHash() {
			if err := queries.MarkAIArtifactStale(ctx, db.MarkAIArtifactStaleParams{ArtifactID: artifactID, UserID: userID}); err != nil {
				return ArtifactGeneration{}, err
			}
			if err := queries.SetEpisodeAIStatus(ctx, db.SetEpisodeAIStatusParams{Status: "waiting", EpisodeID: artifact.EpisodeID, UserID: userID}); err != nil {
				return ArtifactGeneration{}, err
			}
			return ArtifactGeneration{}, nil
		}
		if err := queries.SetEpisodeAIStatus(ctx, db.SetEpisodeAIStatusParams{Status: "running", EpisodeID: artifact.EpisodeID, UserID: userID}); err != nil {
			return ArtifactGeneration{}, err
		}
		return ArtifactGeneration{Artifact: artifact, Input: input, Run: true}, nil
	})
}

func (repository *AIRepository) CompleteArtifact(
	ctx context.Context,
	artifact db.AiArtifact,
	result aidomain.ArtifactResult,
	usage aidomain.Usage,
) (bool, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return false, err
	}
	inputTokens, outputTokens, err := checkedTokens(usage)
	if err != nil {
		return false, err
	}
	completed, err := withTx(ctx, repository.pool, func(queries *db.Queries) (bool, error) {
		if _, err := queries.LockOwnedEpisodeForAI(ctx, db.LockOwnedEpisodeForAIParams{EpisodeID: artifact.EpisodeID, UserID: artifact.UserID}); err != nil {
			return false, err
		}
		stored, err := queries.CompleteAIArtifact(ctx, db.CompleteAIArtifactParams{
			Result: raw, SearchText: result.SearchText(), InputTokens: inputTokens, OutputTokens: outputTokens,
			ArtifactID: artifact.ID, UserID: artifact.UserID, InputHash: artifact.InputHash,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if err := queries.SetEpisodeAIStatus(ctx, db.SetEpisodeAIStatusParams{Status: "completed", EpisodeID: stored.EpisodeID, UserID: stored.UserID}); err != nil {
			return false, err
		}
		if err := enqueueSearchBuild(ctx, queries, stored.UserID, stored.EpisodeID); err != nil {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return false, fmt.Errorf("complete AI artifact: %w", err)
	}
	return completed, nil
}

func (repository *AIRepository) FailArtifact(ctx context.Context, artifact db.AiArtifact, code, message string) error {
	_, err := withTx(ctx, repository.pool, func(queries *db.Queries) (struct{}, error) {
		if _, err := queries.LockOwnedEpisodeForAI(ctx, db.LockOwnedEpisodeForAIParams{EpisodeID: artifact.EpisodeID, UserID: artifact.UserID}); err != nil {
			return struct{}{}, err
		}
		failed, err := queries.FailAIArtifact(ctx, db.FailAIArtifactParams{
			ErrorCode: nullableString(code), ErrorMessage: nullableString(message), ArtifactID: artifact.ID, UserID: artifact.UserID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return struct{}{}, nil
		}
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, queries.SetEpisodeAIStatus(ctx, db.SetEpisodeAIStatusParams{Status: "failed", EpisodeID: failed.EpisodeID, UserID: failed.UserID})
	})
	return err
}

func (repository *AIRepository) CreateConversation(
	ctx context.Context,
	userID, episodeID pgtype.UUID,
	title string,
) (ConversationRecord, error) {
	if !userID.Valid || !episodeID.Valid {
		return ConversationRecord{}, errors.New("user and episode IDs are required")
	}
	conversation, err := withTx(ctx, repository.pool, func(queries *db.Queries) (ConversationRecord, error) {
		episode, err := queries.LockOwnedEpisodeForAI(ctx, db.LockOwnedEpisodeForAIParams{EpisodeID: episodeID, UserID: userID})
		if err != nil {
			return ConversationRecord{}, err
		}
		if _, err := queries.GetActiveTranscriptVersion(ctx, db.GetActiveTranscriptVersionParams{EpisodeID: episodeID, UserID: userID}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ConversationRecord{}, ErrAITranscriptNotReady
			}
			return ConversationRecord{}, err
		}
		title = strings.TrimSpace(title)
		if title == "" {
			title = "问：" + episode.Title
		}
		created, err := queries.CreateConversation(ctx, db.CreateConversationParams{
			UserID: userID, EpisodeID: episodeID, Scope: "episode", Title: title,
		})
		if err != nil {
			return ConversationRecord{}, err
		}
		return conversationFrom(created, episode.Title), nil
	})
	if err != nil {
		return ConversationRecord{}, fmt.Errorf("create conversation: %w", err)
	}
	return conversation, nil
}

func (repository *AIRepository) Conversation(ctx context.Context, userID, conversationID pgtype.UUID) (ConversationDetail, error) {
	queries := db.New(repository.pool)
	conversation, err := queries.GetOwnedConversation(ctx, db.GetOwnedConversationParams{ConversationID: conversationID, UserID: userID})
	if err != nil {
		return ConversationDetail{}, err
	}
	messages, err := queries.ListConversationMessages(ctx, db.ListConversationMessagesParams{ConversationID: conversationID, UserID: userID})
	if err != nil {
		return ConversationDetail{}, err
	}
	citations, err := queries.ListConversationCitations(ctx, db.ListConversationCitationsParams{ConversationID: conversationID, UserID: userID})
	if err != nil {
		return ConversationDetail{}, err
	}
	byMessage := make(map[pgtype.UUID][]CitationRecord, len(citations))
	for _, citation := range citations {
		byMessage[citation.MessageID] = append(byMessage[citation.MessageID], citationFromRow(citation))
	}
	items := make([]ConversationMessage, len(messages))
	for index, message := range messages {
		items[index] = ConversationMessage{Message: message, Citations: byMessage[message.ID]}
	}
	return ConversationDetail{Conversation: conversationFromRow(conversation), Messages: items}, nil
}

func (repository *AIRepository) StartChatTurn(
	ctx context.Context,
	userID, conversationID, clientMessageID pgtype.UUID,
	content, model string,
) (ChatTurn, error) {
	return withTx(ctx, repository.pool, func(queries *db.Queries) (ChatTurn, error) {
		conversation, err := queries.LockOwnedConversation(ctx, db.LockOwnedConversationParams{ConversationID: conversationID, UserID: userID})
		if err != nil {
			return ChatTurn{}, err
		}
		if conversation.Scope != "episode" || !conversation.EpisodeID.Valid {
			return ChatTurn{}, errors.New("only episode conversations are supported")
		}
		existing, err := queries.GetUserMessageByClientID(ctx, db.GetUserMessageByClientIDParams{
			ConversationID: conversationID, UserID: userID, ClientMessageID: clientMessageID,
		})
		if err == nil {
			if existing.Content != content {
				return ChatTurn{}, ErrChatMessageConflict
			}
			assistant, err := queries.GetAssistantReply(ctx, db.GetAssistantReplyParams{UserMessageID: existing.ID, UserID: userID})
			if err != nil {
				return ChatTurn{}, err
			}
			switch assistant.Status {
			case "completed":
				return ChatTurn{Conversation: conversationFromLockRow(conversation), UserMessage: existing, Assistant: assistant, Replay: true}, nil
			case "streaming":
				return ChatTurn{}, ErrChatTurnInProgress
			default:
				return ChatTurn{}, ErrChatTurnFailed
			}
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return ChatTurn{}, err
		}
		userMessage, err := queries.CreateUserMessage(ctx, db.CreateUserMessageParams{
			ConversationID: conversationID, ClientMessageID: clientMessageID, Content: content,
		})
		if err != nil {
			return ChatTurn{}, err
		}
		assistant, err := queries.CreateAssistantMessage(ctx, db.CreateAssistantMessageParams{
			ConversationID: conversationID, UserMessageID: userMessage.ID, Model: &model,
		})
		if err != nil {
			return ChatTurn{}, err
		}
		if err := queries.TouchConversation(ctx, db.TouchConversationParams{ConversationID: conversationID, UserID: userID}); err != nil {
			return ChatTurn{}, err
		}
		return ChatTurn{Conversation: conversationFromLockRow(conversation), UserMessage: userMessage, Assistant: assistant}, nil
	})
}

func (repository *AIRepository) ConversationHistory(
	ctx context.Context,
	userID, conversationID, currentUserMessageID pgtype.UUID,
) ([]aidomain.Message, error) {
	rows, err := db.New(repository.pool).ListConversationHistory(ctx, db.ListConversationHistoryParams{
		ConversationID: conversationID, UserID: userID, CurrentUserMessageID: currentUserMessageID,
	})
	if err != nil {
		return nil, err
	}
	slices.Reverse(rows)
	messages := make([]aidomain.Message, len(rows))
	for index, row := range rows {
		messages[index] = aidomain.Message{Role: row.Role, Content: row.Content}
	}
	return messages, nil
}

func (repository *AIRepository) CitationSources(
	ctx context.Context,
	userID, episodeID pgtype.UUID,
	candidates []searchdomain.Candidate,
) ([]aidomain.CitationSource, error) {
	chunkIDs, noteIDs := candidateIDs(candidates)
	queries := db.New(repository.pool)
	segmentsByChunk := make(map[string][]aidomain.CitationSource, len(chunkIDs))
	if len(chunkIDs) > 0 {
		segments, err := queries.ListAISegmentsForSearchChunks(ctx, db.ListAISegmentsForSearchChunksParams{
			SearchChunkIds: chunkIDs, UserID: userID, EpisodeID: episodeID,
		})
		if err != nil {
			return nil, err
		}
		for _, segment := range segments {
			source := segmentCitation(segment.ID, segment.SpeakerID, segment.SpeakerName, segment.StartMs, segment.EndMs, segment.Text)
			key := idText(segment.SearchChunkID)
			segmentsByChunk[key] = append(segmentsByChunk[key], source)
		}
	}
	notesByID := make(map[string]aidomain.CitationSource, len(noteIDs))
	if len(noteIDs) > 0 {
		notes, err := queries.ListAINotesByIDs(ctx, db.ListAINotesByIDsParams{NoteIds: noteIDs, UserID: userID, EpisodeID: episodeID})
		if err != nil {
			return nil, err
		}
		for _, note := range notes {
			notesByID[idText(note.ID)] = noteCitation(note.ID, note.Content)
		}
	}
	sources := make([]aidomain.CitationSource, 0, maxCitationSources)
	seen, runes := make(map[string]struct{}, maxCitationSources), 0
	appendSource := func(source aidomain.CitationSource) {
		if source.Key == "" || len(sources) >= maxCitationSources {
			return
		}
		if _, duplicate := seen[source.Key]; duplicate {
			return
		}
		count := utf8.RuneCountInString(source.Excerpt)
		if runes+count > maxCitationContextRunes {
			return
		}
		seen[source.Key], runes = struct{}{}, runes+count
		sources = append(sources, source)
	}
	for _, candidate := range candidates {
		switch candidate.DocumentType {
		case "transcript":
			for _, source := range segmentsByChunk[candidate.ID] {
				appendSource(source)
			}
		case "note":
			appendSource(notesByID[candidate.SourceID])
		}
	}
	if len(sources) < 3 {
		fallbackSegments, err := queries.ListFallbackAISegments(ctx, db.ListFallbackAISegmentsParams{
			UserID: userID, EpisodeID: episodeID, CandidateLimit: maxCitationSources,
		})
		if err != nil {
			return nil, err
		}
		for _, segment := range fallbackSegments {
			appendSource(segmentCitation(segment.ID, segment.SpeakerID, segment.SpeakerName, segment.StartMs, segment.EndMs, segment.Text))
		}
		fallbackNotes, err := queries.ListFallbackAINotes(ctx, db.ListFallbackAINotesParams{
			UserID: userID, EpisodeID: episodeID, CandidateLimit: 4,
		})
		if err != nil {
			return nil, err
		}
		for _, note := range fallbackNotes {
			appendSource(noteCitation(note.ID, note.Content))
		}
	}
	if len(sources) == 0 {
		return nil, ErrAIContextNotFound
	}
	return sources, nil
}

func (repository *AIRepository) ReplayCitations(
	ctx context.Context,
	userID, conversationID, messageID pgtype.UUID,
) ([]aidomain.CitationSource, error) {
	rows, err := db.New(repository.pool).ListConversationCitations(ctx, db.ListConversationCitationsParams{
		ConversationID: conversationID, UserID: userID,
	})
	if err != nil {
		return nil, err
	}
	result := make([]aidomain.CitationSource, 0)
	for _, row := range rows {
		if row.MessageID == messageID {
			result = append(result, citationFromRow(row).Source)
		}
	}
	return result, nil
}

func (repository *AIRepository) CompleteChat(
	ctx context.Context,
	userID pgtype.UUID,
	turn ChatTurn,
	content string,
	citations []aidomain.CitationSource,
	usage aidomain.Usage,
) (db.Message, error) {
	inputTokens, outputTokens, err := checkedTokens(usage)
	if err != nil {
		return db.Message{}, err
	}
	return withTx(ctx, repository.pool, func(queries *db.Queries) (db.Message, error) {
		if _, err := queries.LockOwnedConversation(ctx, db.LockOwnedConversationParams{
			ConversationID: turn.Conversation.ID, UserID: userID,
		}); err != nil {
			return db.Message{}, err
		}
		message, err := queries.CompleteAssistantMessage(ctx, db.CompleteAssistantMessageParams{
			Content: content, InputTokens: inputTokens, OutputTokens: outputTokens,
			MessageID: turn.Assistant.ID, UserID: userID,
		})
		if err != nil {
			return db.Message{}, err
		}
		for index, citation := range citations {
			params := db.CreateMessageCitationParams{
				MessageID: message.ID, Position: int32(index), Excerpt: citation.Excerpt,
			}
			parsed, err := parseID(citation.SourceID)
			if err != nil {
				return db.Message{}, err
			}
			if citation.SourceType == "transcript" {
				params.TranscriptSegmentID = parsed
			} else if citation.SourceType == "note" {
				params.NoteID = parsed
			} else {
				return db.Message{}, errors.New("unsupported citation source")
			}
			if _, err := queries.CreateMessageCitation(ctx, params); err != nil {
				return db.Message{}, err
			}
		}
		if err := queries.TouchConversation(ctx, db.TouchConversationParams{ConversationID: turn.Conversation.ID, UserID: userID}); err != nil {
			return db.Message{}, err
		}
		return message, nil
	})
}

func (repository *AIRepository) FailChat(ctx context.Context, userID, messageID pgtype.UUID, code, message string) error {
	rows, err := db.New(repository.pool).FailAssistantMessage(ctx, db.FailAssistantMessageParams{
		ErrorCode: nullableString(code), ErrorMessage: nullableString(message), MessageID: messageID, UserID: userID,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return nil
	}
	return nil
}

func loadEpisodeAIInput(
	ctx context.Context,
	queries *db.Queries,
	userID, episodeID pgtype.UUID,
	episodeTitle string,
) (aidomain.EpisodeInput, error) {
	version, err := queries.GetActiveTranscriptVersion(ctx, db.GetActiveTranscriptVersionParams{EpisodeID: episodeID, UserID: userID})
	if err != nil {
		return aidomain.EpisodeInput{}, err
	}
	speakers, err := queries.ListTranscriptSpeakers(ctx, db.ListTranscriptSpeakersParams{TranscriptID: version.ID, UserID: userID})
	if err != nil {
		return aidomain.EpisodeInput{}, err
	}
	segments, err := queries.ListTranscriptSegmentsForSearch(ctx, db.ListTranscriptSegmentsForSearchParams{TranscriptID: version.ID, UserID: userID})
	if err != nil {
		return aidomain.EpisodeInput{}, err
	}
	notes, err := queries.ListEpisodeNotes(ctx, db.ListEpisodeNotesParams{EpisodeID: episodeID, UserID: userID})
	if err != nil {
		return aidomain.EpisodeInput{}, err
	}
	input := aidomain.EpisodeInput{
		EpisodeID: idText(episodeID), EpisodeTitle: episodeTitle, TranscriptVersionID: idText(version.ID),
		Speakers: make([]aidomain.Speaker, len(speakers)), Segments: make([]aidomain.Segment, len(segments)), Notes: make([]aidomain.Note, len(notes)),
	}
	for index, speaker := range speakers {
		input.Speakers[index] = aidomain.Speaker{ID: idText(speaker.ID), Name: speaker.DisplayName, Role: speaker.Role}
	}
	for index, segment := range segments {
		input.Segments[index] = aidomain.Segment{
			ID: idText(segment.ID), SpeakerID: idText(segment.SpeakerID), StartMS: segment.StartMs, EndMS: segment.EndMs, Text: segment.Text,
		}
	}
	for index, note := range notes {
		input.Notes[index] = aidomain.Note{ID: idText(note.ID), Content: note.Content}
	}
	if err := input.Validate(); err != nil {
		return aidomain.EpisodeInput{}, err
	}
	return input, nil
}

func markEpisodeAIStale(ctx context.Context, queries *db.Queries, userID, episodeID pgtype.UUID) error {
	if _, err := queries.MarkEpisodeAIArtifactsStale(ctx, db.MarkEpisodeAIArtifactsStaleParams{UserID: userID, EpisodeID: episodeID}); err != nil {
		return err
	}
	if err := cancelAIJobsExcept(ctx, queries, userID, episodeID, pgtype.UUID{}); err != nil {
		return err
	}
	return queries.SetEpisodeAIStatus(ctx, db.SetEpisodeAIStatusParams{Status: "waiting", EpisodeID: episodeID, UserID: userID})
}

func staleOtherAIArtifacts(ctx context.Context, queries *db.Queries, userID, episodeID, exceptID pgtype.UUID) error {
	if err := queries.MarkOtherAIArtifactsStale(ctx, db.MarkOtherAIArtifactsStaleParams{
		UserID: userID, EpisodeID: episodeID, ArtifactType: aidomain.ArtifactTypeEpisodeSummary, ExceptArtifactID: exceptID,
	}); err != nil {
		return err
	}
	return cancelAIJobsExcept(ctx, queries, userID, episodeID, exceptID)
}

func cancelAIJobsExcept(ctx context.Context, queries *db.Queries, userID, episodeID, exceptID pgtype.UUID) error {
	jobs, err := queries.CancelEpisodeAIJobsExcept(ctx, db.CancelEpisodeAIJobsExceptParams{
		UserID: userID, EpisodeID: episodeID, ExceptArtifactID: exceptID,
	})
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err := createEvent(ctx, queries, job, "canceled"); err != nil {
			return err
		}
	}
	return nil
}

func candidateIDs(candidates []searchdomain.Candidate) ([]pgtype.UUID, []pgtype.UUID) {
	chunks, notes := make([]pgtype.UUID, 0), make([]pgtype.UUID, 0)
	for _, candidate := range candidates {
		var value string
		switch candidate.DocumentType {
		case "transcript":
			value = candidate.ID
		case "note":
			value = candidate.SourceID
		default:
			continue
		}
		id, err := parseID(value)
		if err != nil {
			continue
		}
		if candidate.DocumentType == "transcript" {
			chunks = append(chunks, id)
		} else {
			notes = append(notes, id)
		}
	}
	return chunks, notes
}

func segmentCitation(id, speakerID pgtype.UUID, speakerName string, startMS, endMS int64, excerpt string) aidomain.CitationSource {
	start, end := startMS, endMS
	identifier := idText(id)
	return aidomain.CitationSource{
		Key: "segment:" + identifier, SourceType: "transcript", SourceID: identifier,
		Excerpt: excerpt, SpeakerID: idText(speakerID), SpeakerName: speakerName, StartMS: &start, EndMS: &end,
	}
}

func noteCitation(id pgtype.UUID, excerpt string) aidomain.CitationSource {
	identifier := idText(id)
	return aidomain.CitationSource{Key: "note:" + identifier, SourceType: "note", SourceID: identifier, Excerpt: excerpt}
}

func citationFromRow(row db.ListConversationCitationsRow) CitationRecord {
	source := aidomain.CitationSource{Excerpt: row.Excerpt}
	if row.TranscriptSegmentID.Valid {
		source.SourceType, source.SourceID = "transcript", idText(row.TranscriptSegmentID)
		source.Key = "segment:" + source.SourceID
		source.SpeakerID, source.SpeakerName = idText(row.SpeakerID), row.SpeakerName
		source.StartMS, source.EndMS = row.StartMs, row.EndMs
	} else {
		source.SourceType, source.SourceID = "note", idText(row.NoteID)
		source.Key = "note:" + source.SourceID
	}
	return CitationRecord{MessageID: row.MessageID, Position: row.Position, Source: source}
}

func conversationFrom(value db.Conversation, episodeTitle string) ConversationRecord {
	return ConversationRecord{
		ID: value.ID, UserID: value.UserID, EpisodeID: value.EpisodeID, Scope: value.Scope,
		Title: value.Title, EpisodeTitle: episodeTitle, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func conversationFromRow(value db.GetOwnedConversationRow) ConversationRecord {
	title := ""
	if value.EpisodeTitle != nil {
		title = *value.EpisodeTitle
	}
	return ConversationRecord{
		ID: value.ID, UserID: value.UserID, EpisodeID: value.EpisodeID, Scope: value.Scope,
		Title: value.Title, EpisodeTitle: title, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func conversationFromLockRow(value db.LockOwnedConversationRow) ConversationRecord {
	title := ""
	if value.EpisodeTitle != nil {
		title = *value.EpisodeTitle
	}
	return ConversationRecord{
		ID: value.ID, UserID: value.UserID, EpisodeID: value.EpisodeID, Scope: value.Scope,
		Title: value.Title, EpisodeTitle: title, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func checkedTokens(usage aidomain.Usage) (int32, int32, error) {
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.InputTokens > math.MaxInt32 || usage.OutputTokens > math.MaxInt32 {
		return 0, 0, errors.New("LLM token usage is invalid")
	}
	return int32(usage.InputTokens), int32(usage.OutputTokens), nil
}

func parseIDOrZero(value string) pgtype.UUID {
	id, _ := parseID(value)
	return id
}
