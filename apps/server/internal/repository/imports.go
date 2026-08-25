package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	domain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ResolveEpisodeJobType = "resolve_episode"

var ErrImportCanceled = errors.New("import canceled")

type ImportRepository struct {
	pool *pgxpool.Pool
}

func NewImportRepository(pool *pgxpool.Pool) *ImportRepository {
	return &ImportRepository{pool: pool}
}

func (r *ImportRepository) Create(ctx context.Context, userID pgtype.UUID, rawURL string) (db.GetImportStatusRow, error) {
	rawURL = strings.TrimSpace(rawURL)
	if !userID.Valid || rawURL == "" {
		return db.GetImportStatusRow{}, errors.New("user ID and URL are required")
	}
	return withTx(ctx, r.pool, func(queries *db.Queries) (db.GetImportStatusRow, error) {
		created, err := queries.CreateImport(ctx, db.CreateImportParams{UserID: userID, SubmittedUrl: rawURL})
		if err != nil {
			return db.GetImportStatusRow{}, fmt.Errorf("create import: %w", err)
		}
		job, err := enqueue(ctx, queries, NewJob{
			UserID: userID, Type: ResolveEpisodeJobType, EntityType: "import", EntityID: created.ID,
		})
		if err != nil {
			return db.GetImportStatusRow{}, err
		}
		if err := queries.SetImportJob(ctx, db.SetImportJobParams{JobID: job.ID, ImportID: created.ID, UserID: userID}); err != nil {
			return db.GetImportStatusRow{}, fmt.Errorf("attach import job: %w", err)
		}
		return queries.GetImportStatus(ctx, db.GetImportStatusParams{ImportID: created.ID, UserID: userID})
	})
}

func (r *ImportRepository) Get(ctx context.Context, userID, importID pgtype.UUID) (db.GetImportStatusRow, error) {
	status, err := db.New(r.pool).GetImportStatus(ctx, db.GetImportStatusParams{ImportID: importID, UserID: userID})
	if err != nil {
		return db.GetImportStatusRow{}, fmt.Errorf("get transcription task: %w", err)
	}
	return status, nil
}

func (r *ImportRepository) Segments(ctx context.Context, userID, importID pgtype.UUID) ([]db.ListTranscriptionTaskSegmentsRow, error) {
	return db.New(r.pool).ListTranscriptionTaskSegments(ctx, db.ListTranscriptionTaskSegmentsParams{
		ImportID: importID, UserID: userID,
	})
}

func (r *ImportRepository) SaveResolved(
	ctx context.Context,
	userID, importID pgtype.UUID,
	rawURL string,
	resolved *domain.ResolvedEpisode,
) (pgtype.UUID, error) {
	if !userID.Valid || !importID.Valid {
		return pgtype.UUID{}, errors.New("user ID and import ID are required")
	}
	if resolved == nil || strings.TrimSpace(resolved.EpisodeTitle) == "" ||
		strings.TrimSpace(resolved.AudioURL) == "" || strings.TrimSpace(resolved.CanonicalURL) == "" {
		return pgtype.UUID{}, errors.New("resolver returned incomplete episode metadata")
	}
	identityKeys := episodeIdentityKeys(resolved)
	if len(identityKeys) == 0 {
		return pgtype.UUID{}, errors.New("resolver returned no episode identity")
	}

	return withTx(ctx, r.pool, func(queries *db.Queries) (pgtype.UUID, error) {
		importRecord, err := queries.GetImportForResolve(ctx, db.GetImportForResolveParams{ImportID: importID, UserID: userID})
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("lock import: %w", err)
		}
		if importRecord.JobStatus == "canceled" {
			return pgtype.UUID{}, ErrImportCanceled
		}
		if importRecord.EpisodeID.Valid {
			return importRecord.EpisodeID, nil
		}

		locks := make([]string, len(identityKeys))
		for index, key := range identityKeys {
			locks[index] = fmt.Sprintf("%x:%s", userID.Bytes, key)
		}
		sort.Strings(locks)
		for _, key := range locks {
			if err := queries.AcquireEpisodeIdentityLock(ctx, key); err != nil {
				return pgtype.UUID{}, fmt.Errorf("lock episode identity: %w", err)
			}
		}

		episodeID, err := queries.FindEpisodeByIdentityKeys(ctx, db.FindEpisodeByIdentityKeysParams{
			UserID: userID, IdentityKeys: identityKeys,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, fmt.Errorf("find episode identity: %w", err)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			episode, createErr := queries.CreateEpisode(ctx, db.CreateEpisodeParams{
				UserID: userID, Title: strings.TrimSpace(resolved.EpisodeTitle),
				Description: strings.TrimSpace(resolved.Description), PublishedAt: nullableTimestamptz(resolved.PublishedAt),
				DurationMs: max(resolved.DurationMS, 0), CoverUrl: strings.TrimSpace(resolved.PodcastCoverURL),
			})
			if createErr != nil {
				return pgtype.UUID{}, fmt.Errorf("create episode: %w", createErr)
			}
			episodeID = episode.ID
		} else if _, err := queries.EnrichEpisode(ctx, db.EnrichEpisodeParams{
			Title: strings.TrimSpace(resolved.EpisodeTitle), Description: strings.TrimSpace(resolved.Description),
			PublishedAt: nullableTimestamptz(resolved.PublishedAt), DurationMs: max(resolved.DurationMS, 0),
			CoverUrl: strings.TrimSpace(resolved.PodcastCoverURL), EpisodeID: episodeID, UserID: userID,
		}); err != nil {
			return pgtype.UUID{}, fmt.Errorf("enrich episode: %w", err)
		}

		for _, key := range identityKeys {
			if err := queries.AddEpisodeIdentityKey(ctx, db.AddEpisodeIdentityKeyParams{
				UserID: userID, IdentityKey: key, EpisodeID: episodeID,
			}); err != nil {
				return pgtype.UUID{}, fmt.Errorf("save episode identity: %w", err)
			}
		}
		if err := queries.AddEpisodeSource(ctx, db.AddEpisodeSourceParams{
			UserID: userID, EpisodeID: episodeID, SourceType: resolved.SourceType,
			ExternalID: nullableString(resolved.ExternalID), SourceUrl: strings.TrimSpace(rawURL),
			CanonicalUrl: strings.TrimSpace(resolved.CanonicalURL), AudioUrl: strings.TrimSpace(resolved.AudioURL),
			RssGuid: nullableString(resolved.RSSGUID),
		}); err != nil {
			return pgtype.UUID{}, fmt.Errorf("save episode source: %w", err)
		}
		if err := queries.SetImportEpisode(ctx, db.SetImportEpisodeParams{
			EpisodeID: episodeID, ImportID: importID, UserID: userID,
		}); err != nil {
			return pgtype.UUID{}, fmt.Errorf("complete import: %w", err)
		}
		return episodeID, nil
	})
}

func episodeIdentityKeys(resolved *domain.ResolvedEpisode) []string {
	keys := make([]string, 0, 4)
	if externalID := strings.TrimSpace(resolved.ExternalID); externalID != "" && resolved.SourceType == domain.SourceApple {
		keys = append(keys, identityKey("platform", resolved.SourceType+":"+externalID))
	}
	if guid := strings.TrimSpace(resolved.RSSGUID); guid != "" {
		keys = append(keys, identityKey("rss", guid))
	}
	if audioURL := normalizeURL(resolved.AudioURL); audioURL != "" {
		keys = append(keys, identityKey("audio", audioURL))
	}
	if resolved.PublishedAt != nil && resolved.DurationMS > 0 {
		metadata := strings.ToLower(strings.Join(strings.Fields(resolved.EpisodeTitle), " ")) + "|" +
			resolved.PublishedAt.UTC().Format(time.RFC3339) + "|" + strconv.FormatInt(resolved.DurationMS, 10)
		keys = append(keys, identityKey("metadata", metadata))
	}
	return keys
}

func identityKey(kind, value string) string {
	digest := sha256.Sum256([]byte(value))
	return kind + ":" + hex.EncodeToString(digest[:])
}

func normalizeURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" {
		return strings.TrimSpace(rawURL)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := parsed.Port()
	if port != "" && !((parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443")) {
		host = net.JoinHostPort(strings.ToLower(parsed.Hostname()), port)
	}
	parsed.Host = host
	parsed.Fragment = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	parsed.RawQuery = parsed.Query().Encode()
	return parsed.String()
}

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func nullableTimestamptz(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return timestamptz(*value)
}
