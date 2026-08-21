package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestValidateRuntimeRoleRejectsSuperuser(t *testing.T) {
	databaseURL := os.Getenv("ECHONOTE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ECHONOTE_TEST_DATABASE_URL to run the PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := Open(ctx, databaseURL, "echonote-runtime-role-test")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var superuser bool
	if err := pool.QueryRow(ctx, `SELECT rolsuper FROM pg_roles WHERE rolname = current_user`).Scan(&superuser); err != nil {
		t.Fatal(err)
	}
	if !superuser {
		t.Skip("test connection is not a superuser")
	}
	if err := ValidateRuntimeRole(ctx, pool, "echonote_test"); err == nil || !strings.Contains(err.Error(), "administrative privileges") {
		t.Fatalf("expected superuser rejection, got %v", err)
	}
}
