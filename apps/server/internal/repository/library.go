package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LibraryRepository struct {
	pool *pgxpool.Pool
}

type LibraryEpisodeDetail struct {
	Episode db.GetLibraryEpisodeRow
	Sources []db.ListLibraryEpisodeSourcesRow
}

func NewLibraryRepository(pool *pgxpool.Pool) *LibraryRepository {
	return &LibraryRepository{pool: pool}
}

func (r *LibraryRepository) List(
	ctx context.Context,
	userID pgtype.UUID,
	limit, offset int32,
) ([]db.ListLibraryEpisodesRow, int64, error) {
	if !userID.Valid || limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, errors.New("valid user ID, limit, and offset are required")
	}
	queries := db.New(r.pool)
	items, err := queries.ListLibraryEpisodes(ctx, db.ListLibraryEpisodesParams{
		UserID: userID, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list episodes: %w", err)
	}
	total, err := queries.CountLibraryEpisodes(ctx, userID)
	if err != nil {
		return nil, 0, fmt.Errorf("count episodes: %w", err)
	}
	return items, total, nil
}

func (r *LibraryRepository) Get(ctx context.Context, userID, episodeID pgtype.UUID) (LibraryEpisodeDetail, error) {
	queries := db.New(r.pool)
	episode, err := queries.GetLibraryEpisode(ctx, db.GetLibraryEpisodeParams{EpisodeID: episodeID, UserID: userID})
	if err != nil {
		return LibraryEpisodeDetail{}, fmt.Errorf("get episode: %w", err)
	}
	sources, err := queries.ListLibraryEpisodeSources(ctx, db.ListLibraryEpisodeSourcesParams{EpisodeID: episodeID, UserID: userID})
	if err != nil {
		return LibraryEpisodeDetail{}, fmt.Errorf("list episode sources: %w", err)
	}
	return LibraryEpisodeDetail{Episode: episode, Sources: sources}, nil
}

func (r *LibraryRepository) Delete(ctx context.Context, userID, episodeID pgtype.UUID) error {
	_, err := withTx(ctx, r.pool, func(queries *db.Queries) (struct{}, error) {
		podcastID, err := queries.DeleteLibraryEpisode(ctx, db.DeleteLibraryEpisodeParams{EpisodeID: episodeID, UserID: userID})
		if err != nil {
			return struct{}{}, err
		}
		if podcastID.Valid {
			if err := queries.DeleteOrphanPodcast(ctx, db.DeleteOrphanPodcastParams{PodcastID: podcastID, UserID: userID}); err != nil {
				return struct{}{}, err
			}
		}
		return struct{}{}, nil
	})
	if err != nil {
		return fmt.Errorf("delete episode: %w", err)
	}
	return nil
}
