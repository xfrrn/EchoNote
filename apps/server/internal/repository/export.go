package repository

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
	exportdomain "github.com/Actify/echonote/apps/server/internal/domain/export"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ExportRepository struct {
	pool *pgxpool.Pool
}

type ExportSnapshotRequest struct {
	NeedNotes      bool
	NeedArtifact   bool
	NeedTranscript bool
	SegmentIDs     []pgtype.UUID
	SegmentLimit   int32
}

func NewExportRepository(pool *pgxpool.Pool) *ExportRepository {
	return &ExportRepository{pool: pool}
}

func (repository *ExportRepository) Snapshot(
	ctx context.Context,
	userID, episodeID pgtype.UUID,
	request ExportSnapshotRequest,
) (exportdomain.Input, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return exportdomain.Input{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := db.New(tx)
	episode, err := queries.GetLibraryEpisode(ctx, db.GetLibraryEpisodeParams{EpisodeID: episodeID, UserID: userID})
	if err != nil {
		return exportdomain.Input{}, err
	}
	sources, err := queries.ListLibraryEpisodeSources(ctx, db.ListLibraryEpisodeSourcesParams{EpisodeID: episodeID, UserID: userID})
	if err != nil {
		return exportdomain.Input{}, err
	}
	input := exportdomain.Input{Episode: exportdomain.Episode{
		Title: episode.Title, DurationMS: episode.DurationMs, SourceURL: preferredSourceURL(sources),
	}}
	if episode.PodcastTitle != nil {
		input.Episode.PodcastTitle = *episode.PodcastTitle
	}
	if episode.PublishedAt.Valid {
		published := episode.PublishedAt.Time
		input.Episode.PublishedAt = &published
	}
	if request.NeedNotes {
		notes, err := queries.ListEpisodeNotes(ctx, db.ListEpisodeNotesParams{EpisodeID: episodeID, UserID: userID})
		if err != nil {
			return exportdomain.Input{}, err
		}
		slices.Reverse(notes)
		input.Notes = make([]exportdomain.Note, len(notes))
		for index, note := range notes {
			input.Notes[index] = exportdomain.Note{Content: note.Content, CreatedAt: note.CreatedAt.Time}
		}
	}
	if request.NeedArtifact {
		raw, err := queries.GetReadyAIArtifactForExport(ctx, db.GetReadyAIArtifactForExportParams{UserID: userID, EpisodeID: episodeID})
		if errors.Is(err, pgx.ErrNoRows) {
			return exportdomain.Input{}, exportdomain.ErrArtifactNotReady
		}
		if err != nil {
			return exportdomain.Input{}, err
		}
		var artifact aidomain.ArtifactResult
		if err := json.Unmarshal(raw, &artifact); err != nil {
			return exportdomain.Input{}, err
		}
		input.Artifact = &artifact
	}
	if request.NeedTranscript {
		segmentIDs := request.SegmentIDs
		if segmentIDs == nil {
			segmentIDs = []pgtype.UUID{}
		}
		version, err := queries.GetActiveTranscriptVersion(ctx, db.GetActiveTranscriptVersionParams{EpisodeID: episodeID, UserID: userID})
		if errors.Is(err, pgx.ErrNoRows) {
			return exportdomain.Input{}, exportdomain.ErrTranscriptNotReady
		}
		if err != nil {
			return exportdomain.Input{}, err
		}
		stats, err := queries.GetTranscriptExportStats(ctx, db.GetTranscriptExportStatsParams{
			TranscriptID: version.ID, UserID: userID, SegmentIds: segmentIDs, SegmentLimit: request.SegmentLimit,
		})
		if err != nil {
			return exportdomain.Input{}, err
		}
		if len(request.SegmentIDs) > 0 && int(stats.SegmentCount) != len(request.SegmentIDs) {
			return exportdomain.Input{}, exportdomain.ErrSegmentsNotFound
		}
		if stats.SegmentCount == 0 {
			return exportdomain.Input{}, exportdomain.ErrTranscriptNotReady
		}
		if stats.ContentBytes > exportdomain.MaxContentBytes {
			return exportdomain.Input{}, exportdomain.ErrTooLarge
		}
		segments, err := queries.ListTranscriptSegmentsForExport(ctx, db.ListTranscriptSegmentsForExportParams{
			TranscriptID: version.ID, UserID: userID, SegmentIds: segmentIDs, SegmentLimit: request.SegmentLimit,
		})
		if err != nil {
			return exportdomain.Input{}, err
		}
		input.Segments = make([]exportdomain.Segment, len(segments))
		for index, segment := range segments {
			input.Segments[index] = exportdomain.Segment{
				SpeakerName: segment.SpeakerName, StartMS: segment.StartMs, Text: segment.Text,
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return exportdomain.Input{}, err
	}
	return input, nil
}

func preferredSourceURL(sources []db.ListLibraryEpisodeSourcesRow) string {
	for _, source := range sources {
		if value := strings.TrimSpace(source.CanonicalUrl); value != "" {
			return value
		}
	}
	for _, source := range sources {
		if value := strings.TrimSpace(source.SourceUrl); value != "" {
			return value
		}
	}
	return ""
}
