package service

import (
	"context"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	domain "github.com/Actify/echonote/apps/server/internal/domain/podcast"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/Actify/echonote/apps/server/internal/worker"
)

func NewResolveImportHandler(
	imports *repository.ImportRepository,
	transcriptions *repository.TranscriptionRepository,
	resolver domain.EpisodeResolver,
) worker.Handler {
	return func(ctx context.Context, job db.Job) error {
		if job.EntityType != "import" || !job.EntityID.Valid || !job.UserID.Valid {
			return domain.NewResolveError("IMPORT_JOB_INVALID", "Import job is invalid", false, nil)
		}
		status, err := imports.Get(ctx, job.UserID, job.EntityID)
		if err != nil {
			return err
		}
		episodeID := status.EpisodeID
		if !episodeID.Valid {
			resolved, err := resolver.Resolve(ctx, status.SubmittedUrl)
			if err != nil {
				return err
			}
			episodeID, err = imports.SaveResolved(ctx, job.UserID, job.EntityID, status.SubmittedUrl, resolved)
			if err != nil {
				return err
			}
		}
		_, err = transcriptions.Create(ctx, job.UserID, episodeID)
		return err
	}
}
