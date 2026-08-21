package httpapi

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ensureTestUsers(t *testing.T, pool *pgxpool.Pool, userIDs ...pgtype.UUID) func() {
	t.Helper()
	for _, userID := range userIDs {
		if _, err := pool.Exec(context.Background(), "INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING", userID); err != nil {
			t.Fatal(err)
		}
	}
	return func() {
		for _, userID := range userIDs {
			_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
		}
	}
}
