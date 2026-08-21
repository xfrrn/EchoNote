package repository

import (
	"context"
	"crypto/rand"
	"os"
	"testing"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database"
	"github.com/Actify/echonote/apps/server/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestJobQueueLifecycle(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	if err := database.MigrateUp(databaseURL); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, databaseURL, "echonote-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	queue := NewJobQueue(pool)
	jobType := "phase1_test_" + time.Now().Format("20060102150405.000000000")
	job, err := queue.Enqueue(ctx, NewJob{
		Type:       jobType,
		EntityType: "phase1_test",
		EntityID:   randomUUID(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE id = $1", job.ID)
	}()

	claimed, found, err := queue.Claim(ctx, "worker-test", []string{jobType})
	if err != nil || !found {
		t.Fatalf("claim found=%v err=%v", found, err)
	}
	if claimed.Attempt != 1 || claimed.Status != "running" {
		t.Fatalf("claimed job = %+v", claimed)
	}
	if err := queue.Complete(ctx, claimed.ID, "worker-test"); err != nil {
		t.Fatal(err)
	}

	stored, err := db.New(pool).GetJob(ctx, claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	events, err := db.New(pool).ListJobEvents(ctx, claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "succeeded" || len(events) != 3 {
		t.Fatalf("status=%q events=%d", stored.Status, len(events))
	}

	retryType := jobType + "_retry"
	retryJob, err := queue.Enqueue(ctx, NewJob{
		Type:        retryType,
		EntityType:  "phase1_test",
		EntityID:    randomUUID(t),
		MaxAttempts: 2,
		RunAfter:    time.Now().Add(-time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE id = $1", retryJob.ID)
	}()

	firstAttempt, found, err := queue.Claim(ctx, "worker-test", []string{retryType})
	if err != nil || !found {
		t.Fatalf("first retry claim found=%v err=%v", found, err)
	}
	status, err := queue.RetryOrFail(ctx, firstAttempt.ID, "worker-test", "TEST_ERROR", "retry once", time.Millisecond, true)
	if err != nil || status != "queued" {
		t.Fatalf("first failure status=%q err=%v", status, err)
	}
	time.Sleep(5 * time.Millisecond)

	secondAttempt, found, err := queue.Claim(ctx, "worker-test", []string{retryType})
	if err != nil || !found {
		t.Fatalf("second retry claim found=%v err=%v", found, err)
	}
	status, err = queue.RetryOrFail(ctx, secondAttempt.ID, "worker-test", "TEST_ERROR", "attempts exhausted", time.Millisecond, true)
	if err != nil || status != "failed" {
		t.Fatalf("final failure status=%q err=%v", status, err)
	}
	retryEvents, err := db.New(pool).ListJobEvents(ctx, retryJob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(retryEvents) != 5 {
		t.Fatalf("retry events=%d, want 5", len(retryEvents))
	}

	permanentJob, err := queue.Enqueue(ctx, NewJob{
		Type: jobType + "_permanent", EntityType: "phase1_test", EntityID: randomUUID(t), MaxAttempts: 3,
		RunAfter: time.Now().Add(-time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE id = $1", permanentJob.ID) }()
	permanentAttempt, found, err := queue.Claim(ctx, "worker-test", []string{jobType + "_permanent"})
	if err != nil || !found {
		t.Fatalf("permanent claim found=%v err=%v", found, err)
	}
	status, err = queue.RetryOrFail(ctx, permanentAttempt.ID, "worker-test", "PERMANENT", "do not retry", time.Millisecond, false)
	if err != nil || status != "failed" {
		t.Fatalf("permanent failure status=%q err=%v", status, err)
	}

	pollJob, err := queue.Enqueue(ctx, NewJob{
		Type: jobType + "_poll", EntityType: "phase1_test", EntityID: randomUUID(t), MaxAttempts: 1,
		RunAfter: time.Now().Add(-time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), "DELETE FROM jobs WHERE id = $1", pollJob.ID) }()
	pollAttempt, found, err := queue.Claim(ctx, "worker-test", []string{jobType + "_poll"})
	if err != nil || !found || pollAttempt.Attempt != 1 {
		t.Fatalf("poll claim=%+v found=%v err=%v", pollAttempt, found, err)
	}
	if err := queue.Reschedule(ctx, pollAttempt.ID, "worker-test", time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	pollAttempt, found, err = queue.Claim(ctx, "worker-test", []string{jobType + "_poll"})
	if err != nil || !found || pollAttempt.Attempt != 1 {
		t.Fatalf("rescheduled poll claim=%+v found=%v err=%v", pollAttempt, found, err)
	}
	if err := queue.Complete(ctx, pollAttempt.ID, "worker-test"); err != nil {
		t.Fatal(err)
	}
}

func randomUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return pgtype.UUID{Bytes: id, Valid: true}
}
