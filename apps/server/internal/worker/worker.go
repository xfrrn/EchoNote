package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sort"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

const retryDelay = 5 * time.Second

type Queue interface {
	Claim(context.Context, string, []string) (db.Job, bool, error)
	Heartbeat(context.Context, pgtype.UUID, string) error
	Complete(context.Context, pgtype.UUID, string) error
	RetryOrFail(context.Context, pgtype.UUID, string, string, string, time.Duration, bool) (string, error)
	RequeueStale(context.Context, time.Duration) (int, error)
}

type Handler func(context.Context, db.Job) error

type Worker struct {
	queue        Queue
	logger       *slog.Logger
	workerID     string
	pollInterval time.Duration
	leaseTimeout time.Duration
	handlers     map[string]Handler
	jobTypes     []string
}

func New(
	queue Queue,
	logger *slog.Logger,
	workerID string,
	pollInterval time.Duration,
	leaseTimeout time.Duration,
	handlers map[string]Handler,
) *Worker {
	jobTypes := make([]string, 0, len(handlers))
	for jobType := range handlers {
		jobTypes = append(jobTypes, jobType)
	}
	sort.Strings(jobTypes)
	return &Worker{
		queue:        queue,
		logger:       logger,
		workerID:     workerID,
		pollInterval: pollInterval,
		leaseTimeout: leaseTimeout,
		handlers:     handlers,
		jobTypes:     jobTypes,
	}
}

func (w *Worker) Run(ctx context.Context) error {
	w.logger.InfoContext(ctx, "worker started", "worker_id", w.workerID, "job_types", w.jobTypes)
	if err := w.requeueStale(ctx); err != nil {
		return err
	}

	poll := time.NewTicker(w.pollInterval)
	recoverEvery := w.leaseTimeout / 2
	if recoverEvery < time.Millisecond {
		recoverEvery = time.Millisecond
	}
	recoverLeases := time.NewTicker(recoverEvery)
	defer poll.Stop()
	defer recoverLeases.Stop()

	// ponytail: one active job per process keeps leases simple; add bounded concurrency when throughput requires it.
	for {
		if len(w.jobTypes) > 0 {
			job, found, err := w.queue.Claim(ctx, w.workerID, w.jobTypes)
			if err != nil {
				w.logger.ErrorContext(ctx, "claim job", "error", err)
			} else if found {
				w.process(ctx, job)
				continue
			}
		}

		select {
		case <-ctx.Done():
			w.logger.Info("worker stopped", "worker_id", w.workerID)
			return nil
		case <-poll.C:
		case <-recoverLeases.C:
			if err := w.requeueStale(ctx); err != nil {
				w.logger.ErrorContext(ctx, "recover stale jobs", "error", err)
			}
		}
	}
}

func (w *Worker) process(ctx context.Context, job db.Job) {
	jobID := fmt.Sprintf("%x", job.ID.Bytes)
	jobContext, cancelJob := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() {
		result <- w.runHandler(jobContext, job)
	}()

	heartbeatEvery := w.leaseTimeout / 3
	if heartbeatEvery < time.Millisecond {
		heartbeatEvery = time.Millisecond
	}
	heartbeat := time.NewTicker(heartbeatEvery)
	defer heartbeat.Stop()
	defer cancelJob()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-result:
			transitionContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if err == nil {
				if err := w.queue.Complete(transitionContext, job.ID, w.workerID); err != nil {
					w.logger.Error("complete job", "job_id", jobID, "job_type", job.Type, "error", err)
				}
				return
			}

			errorCode := "JOB_HANDLER_FAILED"
			errorMessage := "job handler failed"
			retryable := true
			var classified interface {
				Code() string
				Retryable() bool
			}
			if errors.As(err, &classified) {
				errorCode = classified.Code()
				errorMessage = err.Error()
				retryable = classified.Retryable()
			}
			status, transitionErr := w.queue.RetryOrFail(
				transitionContext,
				job.ID,
				w.workerID,
				errorCode,
				errorMessage,
				retryDelay,
				retryable,
			)
			if transitionErr != nil {
				w.logger.Error("record job failure", "job_id", jobID, "job_type", job.Type, "error", transitionErr)
				return
			}
			w.logger.Warn("job handler failed", "job_id", jobID, "job_type", job.Type, "status", status, "error", err, "cause", errors.Unwrap(err))
			return
		case <-heartbeat.C:
			heartbeatContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			err := w.queue.Heartbeat(heartbeatContext, job.ID, w.workerID)
			cancel()
			if err != nil {
				cancelJob()
				w.logger.Error("heartbeat job", "job_id", jobID, "job_type", job.Type, "error", err)
				return
			}
		}
	}
}

func (w *Worker) runHandler(ctx context.Context, job db.Job) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			w.logger.Error("job handler panic",
				"job_id", fmt.Sprintf("%x", job.ID.Bytes),
				"job_type", job.Type,
				"panic", recovered,
				"stack", string(debug.Stack()),
			)
			err = fmt.Errorf("job handler panic: %v", recovered)
		}
	}()
	return w.handlers[job.Type](ctx, job)
}

func (w *Worker) requeueStale(ctx context.Context) error {
	count, err := w.queue.RequeueStale(ctx, w.leaseTimeout)
	if err != nil {
		return err
	}
	if count > 0 {
		w.logger.WarnContext(ctx, "recovered stale jobs", "count", count)
	}
	return nil
}
