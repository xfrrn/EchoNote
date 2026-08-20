package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrJobLeaseLost = errors.New("job lease lost")

type NewJob struct {
	UserID      pgtype.UUID
	Type        string
	EntityType  string
	EntityID    pgtype.UUID
	Payload     json.RawMessage
	Priority    int16
	MaxAttempts int32
	RunAfter    time.Time
}

type JobQueue struct {
	pool *pgxpool.Pool
}

func NewJobQueue(pool *pgxpool.Pool) *JobQueue {
	return &JobQueue{pool: pool}
}

func (q *JobQueue) Enqueue(ctx context.Context, input NewJob) (db.Job, error) {
	if len(input.Payload) == 0 {
		input.Payload = json.RawMessage(`{}`)
	}
	if input.MaxAttempts == 0 {
		input.MaxAttempts = 3
	}
	if input.RunAfter.IsZero() {
		input.RunAfter = time.Now()
	}

	return withTx(ctx, q.pool, func(queries *db.Queries) (db.Job, error) {
		job, err := queries.EnqueueJob(ctx, db.EnqueueJobParams{
			UserID:      input.UserID,
			JobType:     input.Type,
			EntityType:  input.EntityType,
			EntityID:    input.EntityID,
			Payload:     input.Payload,
			Stage:       "queued",
			Priority:    input.Priority,
			MaxAttempts: input.MaxAttempts,
			RunAfter:    timestamptz(input.RunAfter),
		})
		if err != nil {
			return db.Job{}, fmt.Errorf("enqueue job: %w", err)
		}
		if err := createEvent(ctx, queries, job, "queued"); err != nil {
			return db.Job{}, err
		}
		return job, nil
	})
}

func (q *JobQueue) Claim(ctx context.Context, workerID string, jobTypes []string) (db.Job, bool, error) {
	if len(jobTypes) == 0 {
		return db.Job{}, false, nil
	}

	job, err := withTx(ctx, q.pool, func(queries *db.Queries) (db.Job, error) {
		job, err := queries.ClaimJob(ctx, db.ClaimJobParams{
			WorkerID: &workerID,
			JobTypes: jobTypes,
		})
		if err != nil {
			return db.Job{}, err
		}
		if err := createEvent(ctx, queries, job, "running"); err != nil {
			return db.Job{}, err
		}
		return job, nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Job{}, false, nil
	}
	if err != nil {
		return db.Job{}, false, fmt.Errorf("claim job: %w", err)
	}
	return job, true, nil
}

func (q *JobQueue) Heartbeat(ctx context.Context, jobID pgtype.UUID, workerID string) error {
	rows, err := db.New(q.pool).HeartbeatJob(ctx, db.HeartbeatJobParams{
		JobID:    jobID,
		WorkerID: &workerID,
	})
	if err != nil {
		return fmt.Errorf("heartbeat job: %w", err)
	}
	if rows != 1 {
		return ErrJobLeaseLost
	}
	return nil
}

func (q *JobQueue) Complete(ctx context.Context, jobID pgtype.UUID, workerID string) error {
	_, err := withTx(ctx, q.pool, func(queries *db.Queries) (db.Job, error) {
		job, err := queries.CompleteJob(ctx, db.CompleteJobParams{
			JobID:    jobID,
			WorkerID: &workerID,
		})
		if err != nil {
			return db.Job{}, err
		}
		if err := createEvent(ctx, queries, job, "succeeded"); err != nil {
			return db.Job{}, err
		}
		return job, nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrJobLeaseLost
	}
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	return nil
}

func (q *JobQueue) RetryOrFail(
	ctx context.Context,
	jobID pgtype.UUID,
	workerID string,
	errorCode string,
	errorMessage string,
	retryDelay time.Duration,
) (string, error) {
	job, err := withTx(ctx, q.pool, func(queries *db.Queries) (db.Job, error) {
		job, err := queries.RetryOrFailJob(ctx, db.RetryOrFailJobParams{
			RetryDelayMilliseconds: durationMilliseconds(retryDelay),
			ErrorCode:              &errorCode,
			ErrorMessage:           &errorMessage,
			JobID:                  jobID,
			WorkerID:               &workerID,
		})
		if err != nil {
			return db.Job{}, err
		}
		if err := createEvent(ctx, queries, job, job.Status); err != nil {
			return db.Job{}, err
		}
		return job, nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrJobLeaseLost
	}
	if err != nil {
		return "", fmt.Errorf("retry or fail job: %w", err)
	}
	return job.Status, nil
}

func (q *JobQueue) RequeueStale(ctx context.Context, leaseTimeout time.Duration) (int, error) {
	jobs, err := withTx(ctx, q.pool, func(queries *db.Queries) ([]db.Job, error) {
		jobs, err := queries.RequeueStaleJobs(ctx, durationMilliseconds(leaseTimeout))
		if err != nil {
			return nil, err
		}
		for _, job := range jobs {
			if err := createEvent(ctx, queries, job, job.Status); err != nil {
				return nil, err
			}
		}
		return jobs, nil
	})
	if err != nil {
		return 0, fmt.Errorf("requeue stale jobs: %w", err)
	}
	return len(jobs), nil
}

func createEvent(ctx context.Context, queries *db.Queries, job db.Job, eventType string) error {
	_, err := queries.CreateJobEvent(ctx, db.CreateJobEventParams{
		JobID:     job.ID,
		EventType: eventType,
		Stage:     job.Stage,
		Data:      []byte(`{}`),
	})
	if err != nil {
		return fmt.Errorf("create job event: %w", err)
	}
	return nil
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func durationMilliseconds(value time.Duration) int64 {
	if milliseconds := value.Milliseconds(); milliseconds > 0 {
		return milliseconds
	}
	return 1
}

func withTx[T any](
	ctx context.Context,
	pool *pgxpool.Pool,
	fn func(*db.Queries) (T, error),
) (result T, err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, rollbackErr)
		}
	}()

	result, err = fn(db.New(tx))
	if err != nil {
		return result, err
	}
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}
