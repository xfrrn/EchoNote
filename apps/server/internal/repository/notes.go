package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pendingEpisodeTitle = "待解析节目"

type NotesRepository struct {
	pool *pgxpool.Pool
}

type CaptureResult struct {
	Note     db.Note
	ImportID pgtype.UUID
	Created  bool
}

func NewNotesRepository(pool *pgxpool.Pool) *NotesRepository {
	return &NotesRepository{pool: pool}
}

func (r *NotesRepository) CreateForEpisode(
	ctx context.Context,
	userID, episodeID, clientNoteID pgtype.UUID,
	content string,
	createdAt time.Time,
) (CaptureResult, error) {
	content = strings.TrimSpace(content)
	if !userID.Valid || !episodeID.Valid || !clientNoteID.Valid || content == "" || createdAt.IsZero() {
		return CaptureResult{}, errors.New("user, episode, client note ID, content, and created time are required")
	}
	return withTx(ctx, r.pool, func(queries *db.Queries) (CaptureResult, error) {
		existing, found, err := findIdempotentNote(ctx, queries, userID, clientNoteID)
		if err != nil {
			return CaptureResult{}, err
		}
		if found {
			return CaptureResult{Note: existing}, nil
		}
		if _, err := queries.LockOwnedEpisodeForNote(ctx, db.LockOwnedEpisodeForNoteParams{
			EpisodeID: episodeID, UserID: userID,
		}); err != nil {
			return CaptureResult{}, fmt.Errorf("lock episode for note: %w", err)
		}
		note, err := queries.CreateNoteForEpisode(ctx, db.CreateNoteForEpisodeParams{
			UserID: userID, EpisodeID: episodeID, ClientNoteID: clientNoteID,
			Content: content, CreatedAt: timestamptz(createdAt),
		})
		if err != nil {
			return CaptureResult{}, fmt.Errorf("create note: %w", err)
		}
		if err := enqueueSearchBuild(ctx, queries, userID, episodeID); err != nil {
			return CaptureResult{}, err
		}
		return CaptureResult{Note: note, Created: true}, nil
	})
}

func (r *NotesRepository) CaptureURL(
	ctx context.Context,
	userID, clientNoteID pgtype.UUID,
	rawURL, content string,
	createdAt time.Time,
) (CaptureResult, error) {
	rawURL, content = strings.TrimSpace(rawURL), strings.TrimSpace(content)
	if !userID.Valid || !clientNoteID.Valid || rawURL == "" || len(rawURL) > 4096 || content == "" || createdAt.IsZero() {
		return CaptureResult{}, errors.New("user, client note ID, URL, content, and created time are required")
	}
	return withTx(ctx, r.pool, func(queries *db.Queries) (CaptureResult, error) {
		existing, found, err := findIdempotentNote(ctx, queries, userID, clientNoteID)
		if err != nil {
			return CaptureResult{}, err
		}
		if found {
			importID, importErr := queries.GetCaptureImport(ctx, db.GetCaptureImportParams{
				UserID: userID, EpisodeID: existing.EpisodeID, SubmittedUrl: rawURL,
			})
			if importErr != nil && !errors.Is(importErr, pgx.ErrNoRows) {
				return CaptureResult{}, fmt.Errorf("get capture import: %w", importErr)
			}
			return CaptureResult{Note: existing, ImportID: importID}, nil
		}

		episode, err := queries.CreatePendingEpisode(ctx, db.CreatePendingEpisodeParams{
			UserID: userID, Title: pendingEpisodeTitle,
		})
		if err != nil {
			return CaptureResult{}, fmt.Errorf("create pending episode: %w", err)
		}
		createdImport, err := queries.CreateImport(ctx, db.CreateImportParams{UserID: userID, SubmittedUrl: rawURL})
		if err != nil {
			return CaptureResult{}, fmt.Errorf("create capture import: %w", err)
		}
		job, err := enqueue(ctx, queries, NewJob{
			UserID: userID, Type: ResolveEpisodeJobType, EntityType: "import", EntityID: createdImport.ID,
		})
		if err != nil {
			return CaptureResult{}, err
		}
		if err := queries.SetImportJob(ctx, db.SetImportJobParams{
			JobID: job.ID, ImportID: createdImport.ID, UserID: userID,
		}); err != nil {
			return CaptureResult{}, fmt.Errorf("attach capture job: %w", err)
		}
		if err := queries.SetImportEpisode(ctx, db.SetImportEpisodeParams{
			EpisodeID: episode.ID, ImportID: createdImport.ID, UserID: userID,
		}); err != nil {
			return CaptureResult{}, fmt.Errorf("attach pending episode: %w", err)
		}
		note, err := queries.CreateNoteForEpisode(ctx, db.CreateNoteForEpisodeParams{
			UserID: userID, EpisodeID: episode.ID, ClientNoteID: clientNoteID,
			Content: content, CreatedAt: timestamptz(createdAt),
		})
		if err != nil {
			return CaptureResult{}, fmt.Errorf("create capture note: %w", err)
		}
		if err := enqueueSearchBuild(ctx, queries, userID, episode.ID); err != nil {
			return CaptureResult{}, err
		}
		return CaptureResult{Note: note, ImportID: createdImport.ID, Created: true}, nil
	})
}

func (r *NotesRepository) List(ctx context.Context, userID, episodeID pgtype.UUID) ([]db.Note, error) {
	queries := db.New(r.pool)
	if _, err := queries.GetOwnedEpisodeID(ctx, db.GetOwnedEpisodeIDParams{EpisodeID: episodeID, UserID: userID}); err != nil {
		return nil, fmt.Errorf("get episode for notes: %w", err)
	}
	notes, err := queries.ListEpisodeNotes(ctx, db.ListEpisodeNotesParams{EpisodeID: episodeID, UserID: userID})
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}
	return notes, nil
}

func (r *NotesRepository) Update(ctx context.Context, userID, noteID pgtype.UUID, content string) (db.Note, error) {
	content = strings.TrimSpace(content)
	if !userID.Valid || !noteID.Valid || content == "" {
		return db.Note{}, errors.New("user, note ID, and content are required")
	}
	note, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.Note, error) {
		note, err := queries.UpdateNote(ctx, db.UpdateNoteParams{Content: content, NoteID: noteID, UserID: userID})
		if err != nil {
			return db.Note{}, err
		}
		if err := enqueueSearchBuild(ctx, queries, userID, note.EpisodeID); err != nil {
			return db.Note{}, err
		}
		return note, nil
	})
	if err != nil {
		return db.Note{}, fmt.Errorf("update note: %w", err)
	}
	return note, nil
}

func (r *NotesRepository) Delete(ctx context.Context, userID, noteID pgtype.UUID) error {
	if !userID.Valid || !noteID.Valid {
		return errors.New("user and note ID are required")
	}
	_, err := withTx(ctx, r.pool, func(queries *db.Queries) (db.Note, error) {
		note, err := queries.DeleteNote(ctx, db.DeleteNoteParams{NoteID: noteID, UserID: userID})
		if err != nil {
			return db.Note{}, err
		}
		if err := enqueueSearchBuild(ctx, queries, userID, note.EpisodeID); err != nil {
			return db.Note{}, err
		}
		return note, nil
	})
	if err != nil {
		return fmt.Errorf("delete note: %w", err)
	}
	return nil
}

func findIdempotentNote(
	ctx context.Context,
	queries *db.Queries,
	userID, clientNoteID pgtype.UUID,
) (db.Note, bool, error) {
	lockKey := fmt.Sprintf("%x:%x", userID.Bytes, clientNoteID.Bytes)
	if err := queries.AcquireNoteClientIDLock(ctx, lockKey); err != nil {
		return db.Note{}, false, fmt.Errorf("lock client note ID: %w", err)
	}
	note, err := queries.GetNoteByClientID(ctx, db.GetNoteByClientIDParams{UserID: userID, ClientNoteID: clientNoteID})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Note{}, false, nil
	}
	if err != nil {
		return db.Note{}, false, fmt.Errorf("get idempotent note: %w", err)
	}
	return note, true, nil
}
