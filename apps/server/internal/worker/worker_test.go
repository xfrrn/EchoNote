package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeQueue struct {
	completed bool
	retried   bool
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

func (q *fakeQueue) RetryOrFail(context.Context, pgtype.UUID, string, string, string, time.Duration) (string, error) {
	q.retried = true
	return "queued", nil
}

func (*fakeQueue) RequeueStale(context.Context, time.Duration) (int, error) {
	return 0, nil
}

func TestWorkerProcessesJobOutcome(t *testing.T) {
	tests := []struct {
		name         string
		handler      Handler
		wantComplete bool
		wantRetry    bool
	}{
		{name: "success", handler: func(context.Context, db.Job) error { return nil }, wantComplete: true},
		{name: "failure", handler: func(context.Context, db.Job) error { return errors.New("failed") }, wantRetry: true},
		{name: "panic", handler: func(context.Context, db.Job) error { panic("boom") }, wantRetry: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			queue := &fakeQueue{}
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			process := New(queue, logger, "worker-test", time.Millisecond, time.Second, map[string]Handler{
				"test": test.handler,
			})
			process.process(context.Background(), db.Job{
				ID:   pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
				Type: "test",
			})

			if queue.completed != test.wantComplete || queue.retried != test.wantRetry {
				t.Fatalf("completed=%v retried=%v", queue.completed, queue.retried)
			}
		})
	}
}
