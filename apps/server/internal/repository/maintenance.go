package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	completedRetention = 30 * 24 * time.Hour
	failedRetention    = 90 * 24 * time.Hour
)

type RetentionReport struct {
	CompletedJobs int64 `json:"completed_jobs"`
	FailedJobs    int64 `json:"failed_jobs"`
	JobEvents     int64 `json:"job_events"`
}

type MaintenanceRepository struct{ pool *pgxpool.Pool }

func NewMaintenanceRepository(pool *pgxpool.Pool) *MaintenanceRepository {
	return &MaintenanceRepository{pool: pool}
}

func (repository *MaintenanceRepository) Retain(ctx context.Context, now time.Time, apply bool) (report RetentionReport, err error) {
	if now.IsZero() {
		return report, errors.New("retention time is required")
	}
	completedBefore, failedBefore := now.Add(-completedRetention), now.Add(-failedRetention)
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return report, fmt.Errorf("begin retention: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, rollbackErr)
		}
	}()

	if err := tx.QueryRow(ctx, `
SELECT
    (SELECT count(*) FROM jobs
     WHERE status IN ('succeeded', 'canceled') AND completed_at < $1),
    (SELECT count(*) FROM jobs
     WHERE status = 'failed' AND completed_at < $2),
    (SELECT count(*) FROM job_events AS event
     JOIN jobs AS job ON job.id = event.job_id
	     WHERE (job.status IN ('succeeded', 'canceled') AND job.completed_at < $1)
	        OR (job.status = 'failed' AND job.completed_at < $2))`, completedBefore, failedBefore).Scan(
		&report.CompletedJobs, &report.FailedJobs, &report.JobEvents,
	); err != nil {
		return report, fmt.Errorf("preview retention: %w", err)
	}
	if !apply {
		return report, tx.Commit(ctx)
	}

	completed, err := tx.Exec(ctx, `DELETE FROM jobs
WHERE status IN ('succeeded', 'canceled') AND completed_at < $1`, completedBefore)
	if err != nil {
		return report, fmt.Errorf("delete completed jobs: %w", err)
	}
	failed, err := tx.Exec(ctx, `DELETE FROM jobs
WHERE status = 'failed' AND completed_at < $1`, failedBefore)
	if err != nil {
		return report, fmt.Errorf("delete failed jobs: %w", err)
	}
	report.CompletedJobs = completed.RowsAffected()
	report.FailedJobs = failed.RowsAffected()
	if err := tx.Commit(ctx); err != nil {
		return report, fmt.Errorf("commit retention: %w", err)
	}
	return report, nil
}
