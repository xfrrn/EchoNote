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
	applogging "github.com/Actify/echonote/apps/server/internal/logging"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	retryDelay              = 5 * time.Second
	workerHeartbeatInterval = time.Minute
)

type Queue interface {
	Claim(context.Context, string, []string) (db.Job, bool, error)
	Heartbeat(context.Context, pgtype.UUID, string) error
	Complete(context.Context, pgtype.UUID, string) error
	Reschedule(context.Context, pgtype.UUID, string, time.Duration) error
	RetryOrFail(context.Context, pgtype.UUID, string, string, string, time.Duration, bool) (string, error)
	RequeueStale(context.Context, time.Duration) (int, error)
}

type Handler func(context.Context, db.Job) error

type rescheduleError struct{ delay time.Duration }

func (err rescheduleError) Error() string                  { return "job waiting for external task" }
func (err rescheduleError) RescheduleAfter() time.Duration { return err.delay }

func RescheduleAfter(delay time.Duration) error {
	if delay <= 0 {
		delay = time.Second
	}
	return rescheduleError{delay: delay}
}

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
	workerHeartbeat := time.NewTicker(workerHeartbeatInterval)
	defer poll.Stop()
	defer recoverLeases.Stop()
	defer workerHeartbeat.Stop()

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
		case <-workerHeartbeat.C:
			w.logger.InfoContext(ctx, "worker heartbeat", "worker_id", w.workerID, "state", "idle")
		case <-recoverLeases.C:
			if err := w.requeueStale(ctx); err != nil {
				w.logger.ErrorContext(ctx, "recover stale jobs", "error", err)
			}
		}
	}
}

func (w *Worker) process(ctx context.Context, job db.Job) {
	started := time.Now()
	correlationLogger, logger := w.jobLoggers(job)
	logger.InfoContext(ctx, "job started")
	rawJobContext, cancelJob := context.WithCancel(ctx)
	jobContext := applogging.WithLogger(rawJobContext, correlationLogger)
	result := make(chan error, 1)
	go func() {
		result <- w.runHandler(jobContext, job, logger)
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
					logger.Error("complete job", "error", err)
				} else {
					logger.Info("job completed", "duration_ms", elapsedMilliseconds(started))
				}
				return
			}
			var reschedule interface{ RescheduleAfter() time.Duration }
			if errors.As(err, &reschedule) {
				if transitionErr := w.queue.Reschedule(transitionContext, job.ID, w.workerID, reschedule.RescheduleAfter()); transitionErr != nil {
					logger.Error("reschedule job", "error", transitionErr)
				} else {
					logger.Info("job rescheduled", "duration_ms", elapsedMilliseconds(started), "delay_ms", reschedule.RescheduleAfter().Milliseconds())
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
				logger.Error("record job failure", "error", transitionErr)
				return
			}
			attributes := []any{"status", status, "error_code", errorCode, "duration_ms", elapsedMilliseconds(started)}
			var provider interface {
				ProviderName() string
				ProviderOperation() string
				ProviderStatus() int
			}
			if errors.As(err, &provider) {
				if providerForJob(job.Type) == "" {
					attributes = append(attributes, "provider", provider.ProviderName())
				}
				attributes = append(attributes, "provider_operation", provider.ProviderOperation())
				if provider.ProviderStatus() > 0 {
					attributes = append(attributes, "provider_status", provider.ProviderStatus())
				}
			}
			logger.Warn("job handler failed", attributes...)
			return
		case <-heartbeat.C:
			heartbeatContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			err := w.queue.Heartbeat(heartbeatContext, job.ID, w.workerID)
			cancel()
			if err != nil {
				cancelJob()
				logger.Error("heartbeat job", "error", err)
				return
			}
			logger.Info("worker heartbeat", "state", "busy")
		}
	}
}

func (w *Worker) runHandler(ctx context.Context, job db.Job, logger *slog.Logger) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("job handler panic",
				"panic", recovered,
				"stack", string(debug.Stack()),
			)
			err = fmt.Errorf("job handler panic: %v", recovered)
		}
	}()
	return w.handlers[job.Type](ctx, job)
}

func (w *Worker) jobLoggers(job db.Job) (*slog.Logger, *slog.Logger) {
	attributes := []any{"worker_id", w.workerID, "job_id", uuidText(job.ID)}
	if job.UserID.Valid {
		attributes = append(attributes, "user_id", uuidText(job.UserID))
	}
	switch job.EntityType {
	case "episode", "deleted_episode_audio":
		attributes = append(attributes, "episode_id", uuidText(job.EntityID))
	case "transcription_run":
		attributes = append(attributes, "transcription_run_id", uuidText(job.EntityID))
	default:
		attributes = append(attributes, "entity_type", job.EntityType, "entity_id", uuidText(job.EntityID))
	}
	correlation := w.logger.With(attributes...)
	jobAttributes := []any{"operation", job.Type, "attempt", job.Attempt, "max_attempts", job.MaxAttempts}
	if provider := providerForJob(job.Type); provider != "" {
		jobAttributes = append(jobAttributes, "provider", provider)
	}
	return correlation, correlation.With(jobAttributes...)
}

func providerForJob(jobType string) string {
	switch jobType {
	case "submit_asr", "poll_asr", "ingest_asr_result":
		return "aliyun_asr"
	case "cleanup_audio":
		return "aliyun_oss"
	default:
		return ""
	}
}

func uuidText(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	value := id.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}

func elapsedMilliseconds(started time.Time) int64 {
	if elapsed := time.Since(started).Milliseconds(); elapsed > 0 {
		return elapsed
	}
	return 1
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
