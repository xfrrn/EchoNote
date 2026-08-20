package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	searchdomain "github.com/Actify/echonote/apps/server/internal/domain/search"
	"github.com/Actify/echonote/apps/server/internal/repository"
	workerapp "github.com/Actify/echonote/apps/server/internal/worker"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type SearchService struct {
	repository *repository.SearchRepository
	provider   searchdomain.EmbeddingProvider
}

type SearchOutput struct {
	Items         []searchdomain.Candidate
	Mode          string
	SemanticError error
}

func NewSearchService(repository *repository.SearchRepository, provider searchdomain.EmbeddingProvider) *SearchService {
	return &SearchService{repository: repository, provider: provider}
}

func (service *SearchService) Reindex(ctx context.Context, userID, episodeID pgtype.UUID) (int, error) {
	return service.repository.EnqueueRebuild(ctx, userID, episodeID)
}

func (service *SearchService) Search(
	ctx context.Context,
	userID pgtype.UUID,
	query, scope string,
	episodeID pgtype.UUID,
	limit int,
) (SearchOutput, error) {
	query, scope = strings.TrimSpace(query), strings.TrimSpace(scope)
	if !userID.Valid || utf8.RuneCountInString(query) < 2 || utf8.RuneCountInString(query) > 500 || limit < 1 || limit > 50 {
		return SearchOutput{}, errors.New("valid user, query, and limit are required")
	}
	if scope != "library" && scope != "episode" {
		return SearchOutput{}, errors.New("scope must be library or episode")
	}
	if scope == "episode" {
		if !episodeID.Valid {
			return SearchOutput{}, errors.New("episode scope requires an episode ID")
		}
		if err := service.repository.EnsureOwnedEpisode(ctx, userID, episodeID); err != nil {
			return SearchOutput{}, err
		}
	} else {
		episodeID = pgtype.UUID{}
	}
	candidateLimit := int32(limit * 3)
	keyword, err := service.repository.Keyword(ctx, userID, episodeID, query, candidateLimit)
	if err != nil {
		return SearchOutput{}, err
	}
	output := SearchOutput{Mode: "keyword"}
	var semantic []searchdomain.Candidate
	if service.provider != nil {
		vectors, embedErr := service.provider.Embed(ctx, []string{query}, searchdomain.EmbeddingQuery)
		if embedErr != nil {
			output.SemanticError = embedErr
		} else if embedErr = validateEmbeddings(vectors, 1, service.provider.Dimensions()); embedErr != nil {
			output.SemanticError = embedErr
		} else {
			semantic, err = service.repository.Semantic(ctx, userID, episodeID, service.provider.Model(), vectors[0], candidateLimit)
			if err != nil {
				return SearchOutput{}, err
			}
			output.Mode = "hybrid"
		}
	}
	output.Items = searchdomain.Fuse(keyword, semantic, limit)
	for index := range output.Items {
		output.Items[index].Text = searchdomain.Snippet(output.Items[index].Text, query, 180)
	}
	return output, nil
}

type SearchWorkflow struct {
	repository *repository.SearchRepository
	provider   searchdomain.EmbeddingProvider
}

func NewSearchWorkflow(repository *repository.SearchRepository, provider searchdomain.EmbeddingProvider) *SearchWorkflow {
	return &SearchWorkflow{repository: repository, provider: provider}
}

func (workflow *SearchWorkflow) Handlers() map[string]workerapp.Handler {
	handlers := map[string]workerapp.Handler{
		repository.BuildKeywordIndexJobType: workflow.build,
	}
	if workflow.provider != nil {
		handlers[repository.GenerateEmbeddingsJobType] = workflow.generate
	}
	return handlers
}

func (workflow *SearchWorkflow) build(ctx context.Context, job db.Job) error {
	if job.EntityType != "episode" || !job.UserID.Valid || !job.EntityID.Valid {
		return errors.New("search build job has an invalid entity")
	}
	model := ""
	if workflow.provider != nil {
		model = workflow.provider.Model()
	}
	_, err := workflow.repository.BuildEpisode(ctx, job.UserID, job.EntityID, model)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func (workflow *SearchWorkflow) generate(ctx context.Context, job db.Job) error {
	if workflow.provider == nil || job.EntityType != repository.SearchDocumentEntity || !job.UserID.Valid || !job.EntityID.Valid {
		return errors.New("embedding job has an invalid entity or provider")
	}
	var payload struct {
		ContentHash string `json:"content_hash"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil || !validHash(payload.ContentHash) {
		return errors.New("embedding job has an invalid content hash")
	}
	for {
		batch, err := workflow.repository.PendingEmbeddingBatch(
			ctx, job.UserID, job.EntityID, payload.ContentHash, workflow.provider.Model(),
		)
		if err != nil || len(batch) == 0 {
			return err
		}
		texts, ids := make([]string, len(batch)), make([]pgtype.UUID, len(batch))
		for index, chunk := range batch {
			texts[index], ids[index] = chunk.Text, chunk.ID
		}
		vectors, err := workflow.provider.Embed(ctx, texts, searchdomain.EmbeddingDocument)
		if err != nil {
			return err
		}
		if err := validateEmbeddings(vectors, len(texts), workflow.provider.Dimensions()); err != nil {
			return fmt.Errorf("validate provider embeddings: %w", err)
		}
		if err := workflow.repository.StoreEmbeddingBatch(
			ctx, job.UserID, payload.ContentHash, workflow.provider.Model(), ids, vectors,
		); err != nil {
			return err
		}
	}
}

func validateEmbeddings(vectors [][]float32, count, dimensions int) error {
	if len(vectors) != count || dimensions <= 0 {
		return errors.New("embedding count does not match input count")
	}
	for _, vector := range vectors {
		if len(vector) != dimensions {
			return errors.New("embedding dimensions do not match provider contract")
		}
		nonzero := false
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return errors.New("embedding contains a non-finite value")
			}
			nonzero = nonzero || value != 0
		}
		if !nonzero {
			return errors.New("embedding must not be a zero vector")
		}
	}
	return nil
}

func validHash(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
