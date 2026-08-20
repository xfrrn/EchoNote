package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	searchdomain "github.com/Actify/echonote/apps/server/internal/domain/search"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	BuildKeywordIndexJobType  = "build_keyword_index"
	GenerateEmbeddingsJobType = "generate_embeddings"
	SearchDocumentEntity      = "search_document"
)

type SearchRepository struct {
	pool *pgxpool.Pool
}

type SearchBuildResult struct {
	Documents int
	Changed   int
	Deleted   int
}

type searchDocumentInput struct {
	documentType string
	sourceID     pgtype.UUID
	content      string
	contentHash  string
	chunks       []searchdomain.Chunk
}

func NewSearchRepository(pool *pgxpool.Pool) *SearchRepository {
	return &SearchRepository{pool: pool}
}

func (r *SearchRepository) BuildEpisode(
	ctx context.Context,
	userID, episodeID pgtype.UUID,
	embeddingModel string,
) (SearchBuildResult, error) {
	if !userID.Valid || !episodeID.Valid {
		return SearchBuildResult{}, errors.New("user and episode IDs are required")
	}
	result, err := withTx(ctx, r.pool, func(queries *db.Queries) (SearchBuildResult, error) {
		if _, err := queries.LockOwnedEpisodeForSearch(ctx, db.LockOwnedEpisodeForSearchParams{EpisodeID: episodeID, UserID: userID}); err != nil {
			return SearchBuildResult{}, err
		}
		inputs, err := searchInputs(ctx, queries, userID, episodeID)
		if err != nil {
			return SearchBuildResult{}, err
		}
		existing, err := queries.ListSearchDocumentsForEpisode(ctx, db.ListSearchDocumentsForEpisodeParams{UserID: userID, EpisodeID: episodeID})
		if err != nil {
			return SearchBuildResult{}, err
		}
		current := make(map[string]db.SearchDocument, len(existing))
		for _, document := range existing {
			current[searchDocumentKey(document.DocumentType, document.SourceID)] = document
		}

		result := SearchBuildResult{Documents: len(inputs)}
		for _, input := range inputs {
			key := searchDocumentKey(input.documentType, input.sourceID)
			document, found := current[key]
			changed := !found || document.ContentHash != input.contentHash
			if !found {
				document, err = queries.CreateSearchDocument(ctx, db.CreateSearchDocumentParams{
					UserID: userID, EpisodeID: episodeID, DocumentType: input.documentType,
					SourceID: input.sourceID, Content: input.content, ContentHash: input.contentHash, Metadata: []byte(`{}`),
				})
			} else if changed {
				document, err = queries.UpdateSearchDocument(ctx, db.UpdateSearchDocumentParams{
					EpisodeID: episodeID, Content: input.content, ContentHash: input.contentHash,
					Metadata: []byte(`{}`), DocumentID: document.ID, UserID: userID,
				})
				if err == nil {
					err = queries.DeleteSearchDocumentChunks(ctx, document.ID)
				}
			}
			if err != nil {
				return SearchBuildResult{}, err
			}
			if changed {
				for _, chunk := range input.chunks {
					params, paramErr := searchChunkParams(document.ID, input.documentType, chunk)
					if paramErr != nil {
						return SearchBuildResult{}, paramErr
					}
					if err := queries.CreateSearchChunk(ctx, params); err != nil {
						return SearchBuildResult{}, err
					}
				}
				result.Changed++
			}
			if embeddingModel != "" {
				pending, err := queries.CountPendingSearchEmbeddings(ctx, db.CountPendingSearchEmbeddingsParams{
					DocumentID: document.ID, UserID: userID, EmbeddingModel: &embeddingModel,
				})
				if err != nil {
					return SearchBuildResult{}, err
				}
				if pending > 0 {
					if err := enqueueEmbeddingJob(ctx, queries, userID, document.ID, document.ContentHash); err != nil {
						return SearchBuildResult{}, err
					}
				}
			}
			delete(current, key)
		}
		for _, stale := range current {
			if err := queries.DeleteSearchDocument(ctx, db.DeleteSearchDocumentParams{DocumentID: stale.ID, UserID: userID}); err != nil {
				return SearchBuildResult{}, err
			}
			result.Deleted++
		}
		return result, nil
	})
	if err != nil {
		return SearchBuildResult{}, fmt.Errorf("build episode search index: %w", err)
	}
	return result, nil
}

func (r *SearchRepository) EnqueueRebuild(ctx context.Context, userID, episodeID pgtype.UUID) (int, error) {
	if !userID.Valid {
		return 0, errors.New("user ID is required")
	}
	count, err := withTx(ctx, r.pool, func(queries *db.Queries) (int, error) {
		episodeIDs := []pgtype.UUID{episodeID}
		if !episodeID.Valid {
			var err error
			episodeIDs, err = queries.ListOwnedEpisodeIDsForSearch(ctx, userID)
			if err != nil {
				return 0, err
			}
		} else if _, err := queries.LockOwnedEpisodeForSearch(ctx, db.LockOwnedEpisodeForSearchParams{EpisodeID: episodeID, UserID: userID}); err != nil {
			return 0, err
		}
		for _, id := range episodeIDs {
			if _, err := enqueue(ctx, queries, searchBuildJob(userID, id)); err != nil {
				return 0, err
			}
		}
		return len(episodeIDs), nil
	})
	if err != nil {
		return 0, fmt.Errorf("enqueue search rebuild: %w", err)
	}
	return count, nil
}

func (r *SearchRepository) EnsureOwnedEpisode(ctx context.Context, userID, episodeID pgtype.UUID) error {
	_, err := db.New(r.pool).GetOwnedEpisodeID(ctx, db.GetOwnedEpisodeIDParams{EpisodeID: episodeID, UserID: userID})
	return err
}

func (r *SearchRepository) PendingEmbeddingBatch(
	ctx context.Context,
	userID, documentID pgtype.UUID,
	contentHash, model string,
) ([]db.ListPendingSearchEmbeddingsRow, error) {
	rows, err := db.New(r.pool).ListPendingSearchEmbeddings(ctx, db.ListPendingSearchEmbeddingsParams{
		DocumentID: documentID, UserID: userID, ContentHash: contentHash, EmbeddingModel: &model, BatchLimit: 10,
	})
	if err != nil {
		return nil, fmt.Errorf("list pending embeddings: %w", err)
	}
	return rows, nil
}

func (r *SearchRepository) StoreEmbeddingBatch(
	ctx context.Context,
	userID pgtype.UUID,
	contentHash, model string,
	chunkIDs []pgtype.UUID,
	vectors [][]float32,
) error {
	if len(chunkIDs) != len(vectors) {
		return errors.New("embedding count does not match chunk count")
	}
	_, err := withTx(ctx, r.pool, func(queries *db.Queries) (struct{}, error) {
		for index, chunkID := range chunkIDs {
			rows, err := queries.SetSearchChunkEmbedding(ctx, db.SetSearchChunkEmbeddingParams{
				Embedding: vectorLiteral(vectors[index]), EmbeddingModel: &model,
				ChunkID: chunkID, UserID: userID, ContentHash: contentHash,
			})
			if err != nil {
				return struct{}{}, err
			}
			if rows == 0 {
				return struct{}{}, nil
			}
		}
		return struct{}{}, nil
	})
	if err != nil {
		return fmt.Errorf("store embeddings: %w", err)
	}
	return nil
}

func (r *SearchRepository) Keyword(
	ctx context.Context,
	userID, episodeID pgtype.UUID,
	query string,
	limit int32,
) ([]searchdomain.Candidate, error) {
	rows, err := db.New(r.pool).KeywordSearch(ctx, db.KeywordSearchParams{
		Query: query, UserID: userID, EpisodeID: episodeID, CandidateLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("keyword search: %w", err)
	}
	items := make([]searchdomain.Candidate, len(rows))
	for index, row := range rows {
		items[index] = candidate(
			row.ChunkID, row.DocumentType, row.SourceID, row.EpisodeID, row.EpisodeTitle,
			row.PodcastTitle, row.SpeakerID, row.SpeakerName, row.StartMs, row.EndMs, row.Text, row.RankScore,
		)
	}
	return items, nil
}

func (r *SearchRepository) Semantic(
	ctx context.Context,
	userID, episodeID pgtype.UUID,
	model string,
	embedding []float32,
	limit int32,
) ([]searchdomain.Candidate, error) {
	rows, err := db.New(r.pool).SemanticSearch(ctx, db.SemanticSearchParams{
		QueryEmbedding: vectorLiteral(embedding), UserID: userID, EpisodeID: episodeID,
		EmbeddingModel: &model, CandidateLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("semantic search: %w", err)
	}
	items := make([]searchdomain.Candidate, len(rows))
	for index, row := range rows {
		items[index] = candidate(
			row.ChunkID, row.DocumentType, row.SourceID, row.EpisodeID, row.EpisodeTitle,
			row.PodcastTitle, row.SpeakerID, row.SpeakerName, row.StartMs, row.EndMs, row.Text, row.RankScore,
		)
	}
	return items, nil
}

func searchInputs(ctx context.Context, queries *db.Queries, userID, episodeID pgtype.UUID) ([]searchDocumentInput, error) {
	notes, err := queries.ListEpisodeNotes(ctx, db.ListEpisodeNotesParams{EpisodeID: episodeID, UserID: userID})
	if err != nil {
		return nil, err
	}
	inputs := make([]searchDocumentInput, 0, len(notes)+2)
	for _, note := range notes {
		chunk := searchdomain.Chunk{Index: 0, Text: note.Content}
		inputs = append(inputs, searchDocumentInput{
			documentType: "note", sourceID: note.ID, content: note.Content,
			contentHash: searchdomain.DocumentHash("note", idText(note.ID), note.Content, []searchdomain.Chunk{chunk}),
			chunks:      []searchdomain.Chunk{chunk},
		})
	}

	version, err := queries.GetActiveTranscriptVersion(ctx, db.GetActiveTranscriptVersionParams{EpisodeID: episodeID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return inputs, nil
	}
	if err != nil {
		return nil, err
	}
	segments, err := queries.ListTranscriptSegmentsForSearch(ctx, db.ListTranscriptSegmentsForSearchParams{TranscriptID: version.ID, UserID: userID})
	if err != nil {
		return nil, err
	}
	domainSegments := make([]searchdomain.Segment, len(segments))
	for index, segment := range segments {
		domainSegments[index] = searchdomain.Segment{
			SpeakerID: idText(segment.SpeakerID), StartMS: segment.StartMs, EndMS: segment.EndMs, Text: segment.Text,
		}
	}
	chunks := searchdomain.TranscriptChunks(domainSegments)
	if len(chunks) > 0 {
		parts := make([]string, len(domainSegments))
		for index, segment := range domainSegments {
			parts[index] = segment.Text
		}
		content := strings.Join(parts, "\n\n")
		inputs = append(inputs, searchDocumentInput{
			documentType: "transcript", sourceID: version.ID, content: content,
			contentHash: searchdomain.DocumentHash("transcript", idText(version.ID), content, chunks), chunks: chunks,
		})
	}
	artifact, err := queries.GetReadyAIArtifactForSearch(ctx, db.GetReadyAIArtifactForSearchParams{UserID: userID, EpisodeID: episodeID})
	if errors.Is(err, pgx.ErrNoRows) {
		return inputs, nil
	}
	if err != nil {
		return nil, err
	}
	chunk := searchdomain.Chunk{Index: 0, Text: artifact.SearchText}
	inputs = append(inputs, searchDocumentInput{
		documentType: "ai_artifact", sourceID: artifact.ID, content: artifact.SearchText,
		contentHash: searchdomain.DocumentHash("ai_artifact", idText(artifact.ID), artifact.SearchText, []searchdomain.Chunk{chunk}),
		chunks:      []searchdomain.Chunk{chunk},
	})
	return inputs, nil
}

func searchChunkParams(documentID pgtype.UUID, documentType string, chunk searchdomain.Chunk) (db.CreateSearchChunkParams, error) {
	params := db.CreateSearchChunkParams{DocumentID: documentID, ChunkIndex: int32(chunk.Index), Text: chunk.Text}
	if documentType == "transcript" {
		params.StartMs, params.EndMs = &chunk.StartMS, &chunk.EndMS
		if err := params.SpeakerID.Scan(chunk.SpeakerID); err != nil {
			return db.CreateSearchChunkParams{}, fmt.Errorf("parse search speaker ID: %w", err)
		}
	}
	return params, nil
}

func enqueueSearchBuild(ctx context.Context, queries *db.Queries, userID, episodeID pgtype.UUID) error {
	_, err := enqueue(ctx, queries, searchBuildJob(userID, episodeID))
	return err
}

func searchBuildJob(userID, episodeID pgtype.UUID) NewJob {
	return NewJob{UserID: userID, Type: BuildKeywordIndexJobType, EntityType: "episode", EntityID: episodeID, MaxAttempts: 3}
}

func enqueueEmbeddingJob(ctx context.Context, queries *db.Queries, userID, documentID pgtype.UUID, contentHash string) error {
	payload, err := json.Marshal(map[string]string{"content_hash": contentHash})
	if err != nil {
		return err
	}
	_, err = enqueue(ctx, queries, NewJob{
		UserID: userID, Type: GenerateEmbeddingsJobType, EntityType: SearchDocumentEntity,
		EntityID: documentID, Payload: payload, MaxAttempts: 1,
	})
	return err
}

func searchDocumentKey(documentType string, sourceID pgtype.UUID) string {
	return documentType + ":" + idText(sourceID)
}

func vectorLiteral(vector []float32) string {
	buffer := make([]byte, 0, len(vector)*12)
	buffer = append(buffer, '[')
	for index, value := range vector {
		if index > 0 {
			buffer = append(buffer, ',')
		}
		buffer = strconv.AppendFloat(buffer, float64(value), 'g', -1, 32)
	}
	buffer = append(buffer, ']')
	return string(buffer)
}

func candidate(
	chunkID pgtype.UUID,
	documentType string,
	sourceID, episodeID pgtype.UUID,
	episodeTitle, podcastTitle string,
	speakerID pgtype.UUID,
	speakerName string,
	startMS, endMS *int64,
	text string,
	score float64,
) searchdomain.Candidate {
	return searchdomain.Candidate{
		ID: idText(chunkID), DocumentType: documentType, SourceID: idText(sourceID), EpisodeID: idText(episodeID),
		EpisodeTitle: episodeTitle, PodcastTitle: podcastTitle, SpeakerID: idText(speakerID), SpeakerName: speakerName,
		StartMS: startMS, EndMS: endMS, Text: text, Score: score,
	}
}
