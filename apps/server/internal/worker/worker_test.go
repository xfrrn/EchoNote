package worker

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeQueue struct {
	completed   bool
	retried     bool
	retryable   bool
	errorCode   string
	rescheduled bool
}

func (*fakeQueue) Claim(context.Context, string, []string) (db.Job, bool, error) {
	return db.Job{}, false, nil
}

func (*fakeQueue) Heartbeat(context.Context, pgtype.UUID, string) error {
	return nil
}

func (q *fakeQueue) Complete(context.Context, pgtype.UUID, string) error {
	q.completed = true
	return nil
}

func (q *fakeQueue) Reschedule(context.Context, pgtype.UUID, string, time.Duration) error {
	q.rescheduled = true
	return nil
}

func (q *fakeQueue) RetryOrFail(_ context.Context, _ pgtype.UUID, _ string, errorCode string, _ string, _ time.Duration, retryable bool) (string, error) {
	q.retried = true
	q.retryable = retryable
	q.errorCode = errorCode
	return "queued", nil
}

func (*fakeQueue) RequeueStale(context.Context, time.Duration) (int, error) {
	return 0, nil
}

func TestWorkerProcessesJobOutcome(t *testing.T) {
	tests := []struct {
		name           string
		handler        Handler
		wantComplete   bool
		wantRetry      bool
		wantCode       string
		wantRetryable  bool
		wantReschedule bool
	}{
		{name: "success", handler: func(context.Context, db.Job) error { return nil }, wantComplete: true},
		{name: "failure", handler: func(context.Context, db.Job) error { return errors.New("failed") }, wantRetry: true, wantCode: "JOB_HANDLER_FAILED", wantRetryable: true},
		{name: "panic", handler: func(context.Context, db.Job) error { panic("boom") }, wantRetry: true, wantCode: "JOB_HANDLER_FAILED", wantRetryable: true},
		{name: "permanent", handler: func(context.Context, db.Job) error { return classifiedError{} }, wantRetry: true, wantCode: "PERMANENT", wantRetryable: false},
		{name: "reschedule", handler: func(context.Context, db.Job) error { return RescheduleAfter(time.Second) }, wantReschedule: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			queue := &fakeQueue{}
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			process := New(queue, logger, "worker-test", time.Millisecond, time.Second, map[string]Handler{
				"test": test.handler,
			})
			process.process(context.Background(), db.Job{
				ID:         pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
				UserID:     pgtype.UUID{Bytes: [16]byte{2}, Valid: true},
				Type:       "test",
				EntityType: "transcription_run",
				EntityID:   pgtype.UUID{Bytes: [16]byte{3}, Valid: true},
			})

			if queue.completed != test.wantComplete || queue.retried != test.wantRetry {
				t.Fatalf("completed=%v retried=%v", queue.completed, queue.retried)
			}
			if queue.rescheduled != test.wantReschedule {
				t.Fatalf("rescheduled=%v", queue.rescheduled)
			}
			if queue.retried && (queue.errorCode != test.wantCode || queue.retryable != test.wantRetryable) {
				t.Fatalf("errorCode=%q retryable=%v", queue.errorCode, queue.retryable)
			}
			for _, field := range []string{`"msg":"job started"`, `"job_id":`, `"user_id":`, `"transcription_run_id":`, `"operation":"test"`, `"duration_ms":`} {
				if !strings.Contains(logs.String(), field) {
					t.Fatalf("logs missing %s: %s", field, logs.String())
				}
			}
			if queue.retried && !strings.Contains(logs.String(), `"error_code":"`+test.wantCode+`"`) {
				t.Fatalf("logs missing error code: %s", logs.String())
			}
		})
	}
}

type classifiedError struct{}

func (classifiedError) Error() string   { return "permanent" }
func (classifiedError) Code() string    { return "PERMANENT" }
func (classifiedError) Retryable() bool { return false }
