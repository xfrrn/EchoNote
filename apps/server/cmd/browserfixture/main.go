package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	authn "github.com/Actify/echonote/apps/server/internal/auth"
	"github.com/Actify/echonote/apps/server/internal/config"
	"github.com/Actify/echonote/apps/server/internal/database"
	aidomain "github.com/Actify/echonote/apps/server/internal/domain/ai"
	podcastdomain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type fixture struct {
	EpisodeID    string   `json:"episode_id"`
	RunID        string   `json:"run_id"`
	TranscriptID string   `json:"transcript_id"`
	SpeakerIDs   []string `json:"speaker_ids"`
	SegmentIDs   []string `json:"segment_ids"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("browser fixture exited", "error", err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	if len(args) != 2 || args[0] != "seed" {
		return fmt.Errorf("usage: browserfixture seed <username>")
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Environment != "test" {
		return fmt.Errorf("browser fixtures require APP_ENV=test")
	}
	if err := validateTestDatabase(cfg.DatabaseURL); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, cfg.DatabaseURL, "echonote-browser-fixture")
	if err != nil {
		return err
	}
	defer pool.Close()

	_, normalized, err := authn.NormalizeUsername(args[1])
	if err != nil {
		return err
	}
	user, err := repository.NewAuthRepository(pool).UserForLogin(ctx, normalized)
	if err != nil {
		return err
	}
	audioURL := "https://example.com/browser-ci/" + url.PathEscape(normalized) + ".mp3"
	imports := repository.NewImportRepository(pool)
	createdImport, err := imports.Create(ctx, user.ID, audioURL)
	if err != nil {
		return err
	}
	publishedAt := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	episodeID, err := imports.SaveResolved(ctx, user.ID, createdImport.ID, audioURL, &podcastdomain.ResolvedEpisode{
		SourceType: podcastdomain.SourceDirectAudio, PodcastTitle: "Browser CI Podcast",
		EpisodeTitle: "Browser CI full-flow episode", Description: "Production browser gate fixture",
		PublishedAt: &publishedAt, DurationMS: 90_000, CanonicalURL: audioURL, AudioURL: audioURL,
	})
	if err != nil {
		return err
	}
	note, err := repository.NewNotesRepository(pool).CreateForEpisode(
		ctx, user.ID, episodeID, newPGUUID(), "browserproof indexed note", publishedAt,
	)
	if err != nil {
		return err
	}

	var runID, chunkID, transcriptID, firstSpeakerID, secondSpeakerID, firstSegmentID, secondSegmentID, pageTwoSegmentID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcription_runs (
			user_id, episode_id, profile, provider, model, status, stage,
			total_chunks, completed_chunks, started_at, completed_at
		) VALUES ($1, $2, 'quality', 'browser-fixture', 'browser-fixture', 'completed', 'completed', 1, 1, now(), now())
		RETURNING id`, user.ID, episodeID).Scan(&runID); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO transcription_events (transcription_run_id, event_type, data)
		VALUES ($1, 'started', '{"stage":"transcribing"}'), ($1, 'completed', '{"stage":"completed"}')`, runID); err != nil {
		return err
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcription_chunks (
			transcription_run_id, sequence, core_start_ms, core_end_ms, render_start_ms, render_end_ms,
			status, normalized_result, speaker_map, completed_at
		) VALUES ($1, 0, 0, 90000, 0, 90000, 'completed', '{"segments":[]}', '{"0":"speaker-a","1":"speaker-b"}', now())
		RETURNING id`, runID).Scan(&chunkID); err != nil {
		return err
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcript_versions (user_id, episode_id, transcription_run_id, version, is_active)
		VALUES ($1, $2, $3, 1, true) RETURNING id`, user.ID, episodeID, runID).Scan(&transcriptID); err != nil {
		return err
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcript_speakers (transcript_version_id, stable_key, display_name)
		VALUES ($1, 'speaker-a', 'Speaker A') RETURNING id`, transcriptID).Scan(&firstSpeakerID); err != nil {
		return err
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcript_speakers (transcript_version_id, stable_key, display_name)
		VALUES ($1, 'speaker-b', 'Speaker B') RETURNING id`, transcriptID).Scan(&secondSpeakerID); err != nil {
		return err
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcript_segments (
			transcript_version_id, speaker_id, sequence, start_ms, end_ms, text, source_chunk_id
		) VALUES ($1, $2, 0, 1000, 5000, 'browserproof transcript evidence from Speaker A', $3)
		RETURNING id`, transcriptID, firstSpeakerID, chunkID).Scan(&firstSegmentID); err != nil {
		return err
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcript_segments (
			transcript_version_id, speaker_id, sequence, start_ms, end_ms, text, source_chunk_id
		) VALUES ($1, $2, 1, 6000, 10000, 'second browser transcript segment from Speaker B', $3)
		RETURNING id`, transcriptID, secondSpeakerID, chunkID).Scan(&secondSegmentID); err != nil {
		return err
	}

	artifact := aidomain.ArtifactResult{
		OneSentenceSummary: "browserproof AI summary",
		KeyPoints:          []string{"Browser CI verifies the complete user path"},
		SpeakerViews: []aidomain.SpeakerView{
			{SpeakerID: formatUUID(firstSpeakerID), SpeakerName: "Speaker A", Points: []string{"First verified view"}},
			{SpeakerID: formatUUID(secondSpeakerID), SpeakerName: "Speaker B", Points: []string{"Second verified view"}},
		},
		WorthReviewing: []aidomain.WorthReviewing{{
			TranscriptSegmentID: formatUUID(firstSegmentID), SpeakerID: formatUUID(firstSpeakerID),
			SpeakerName: "Speaker A", StartMS: 1000, EndMS: 5000,
			Quote: "browserproof transcript evidence from Speaker A", Reason: "Browser release evidence",
		}},
		NoteConnections: []aidomain.NoteConnection{{
			NoteID: formatUUID(note.Note.ID), Note: note.Note.Content, Insight: "The fixture note is traceable",
		}},
	}
	rawArtifact, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO transcript_segments (
			transcript_version_id, speaker_id, sequence, start_ms, end_ms, text, source_chunk_id
		)
		SELECT $1, $2, sequence, 10000 + sequence * 500, 10400 + sequence * 500,
			'browser filler transcript segment ' || sequence, $3
		FROM generate_series(2, 99) AS sequence`, transcriptID, secondSpeakerID, chunkID); err != nil {
		return err
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO transcript_segments (
			transcript_version_id, speaker_id, sequence, start_ms, end_ms, text, source_chunk_id
		) VALUES ($1, $2, 100, 60000, 60400, 'browserproof transcript page two evidence', $3)
		RETURNING id`, transcriptID, secondSpeakerID, chunkID).Scan(&pageTwoSegmentID); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_artifacts (
			user_id, episode_id, transcript_version_id, artifact_type, model, prompt_version,
			notes_revision, input_hash, status, result, search_text, completed_at
		) VALUES ($1, $2, $3, 'episode_summary', 'browser-fixture', 'browser-v1', $4, $5,
			'ready', $6, 'browserproof AI summary complete user path', now())`,
		user.ID, episodeID, transcriptID, strings.Repeat("a", 64), strings.Repeat("b", 64), rawArtifact,
	); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		UPDATE episodes SET transcription_status = 'completed', ai_status = 'completed', updated_at = now()
		WHERE id = $1 AND user_id = $2`, episodeID, user.ID); err != nil {
		return err
	}
	if _, err := repository.NewSearchRepository(pool).BuildEpisode(ctx, user.ID, episodeID, ""); err != nil {
		return err
	}

	return json.NewEncoder(output).Encode(fixture{
		EpisodeID: formatUUID(episodeID), RunID: formatUUID(runID), TranscriptID: formatUUID(transcriptID),
		SpeakerIDs: []string{formatUUID(firstSpeakerID), formatUUID(secondSpeakerID)},
		SegmentIDs: []string{formatUUID(firstSegmentID), formatUUID(secondSegmentID), formatUUID(pageTwoSegmentID)},
	})
}

func validateTestDatabase(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" && parsed.Scheme != "pgx5") {
		return fmt.Errorf("DATABASE_URL must be a PostgreSQL URL")
	}
	name := strings.TrimPrefix(parsed.Path, "/")
	if name != "echonote_test" && !strings.HasPrefix(name, "echonote_test_") && !strings.HasPrefix(name, "echonote_browser_") {
		return fmt.Errorf("browser fixtures refuse non-test database: %s", name)
	}
	return nil
}

func newPGUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte(uuid.New()), Valid: true}
}

func formatUUID(id pgtype.UUID) string {
	return uuid.UUID(id.Bytes).String()
}
