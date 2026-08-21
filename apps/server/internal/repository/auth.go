package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthRepository struct {
	pool *pgxpool.Pool
}

type SessionUser struct {
	ID        pgtype.UUID
	Username  string
	ExpiresAt time.Time
}

func NewAuthRepository(pool *pgxpool.Pool) *AuthRepository {
	return &AuthRepository{pool: pool}
}

func (r *AuthRepository) EnsurePlaceholderUser(ctx context.Context, userID pgtype.UUID) error {
	if !userID.Valid {
		return nil
	}
	if err := db.New(r.pool).EnsurePlaceholderUser(ctx, userID); err != nil {
		return fmt.Errorf("ensure development user: %w", err)
	}
	return nil
}

func (r *AuthRepository) CreateUser(ctx context.Context, username, normalized, passwordHash string) (db.User, error) {
	user, err := db.New(r.pool).CreateUser(ctx, db.CreateUserParams{
		Username: &username, UsernameNormalized: &normalized, PasswordHash: &passwordHash,
	})
	if err != nil {
		return db.User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

func (r *AuthRepository) ClaimUser(ctx context.Context, userID pgtype.UUID, username, normalized, passwordHash string) (db.User, error) {
	if !userID.Valid {
		return db.User{}, errors.New("user ID is required")
	}
	user, err := db.New(r.pool).ClaimUser(ctx, db.ClaimUserParams{
		UserID: userID, Username: &username, UsernameNormalized: &normalized, PasswordHash: &passwordHash,
	})
	if err != nil {
		return db.User{}, fmt.Errorf("claim user: %w", err)
	}
	return user, nil
}

func (r *AuthRepository) UserForLogin(ctx context.Context, normalized string) (db.User, error) {
	user, err := db.New(r.pool).GetUserForLogin(ctx, &normalized)
	if err != nil {
		return db.User{}, fmt.Errorf("get login user: %w", err)
	}
	return user, nil
}

func (r *AuthRepository) CreateSession(ctx context.Context, userID pgtype.UUID, tokenHash [32]byte, ttl time.Duration) (time.Time, error) {
	session, err := db.New(r.pool).CreateSession(ctx, db.CreateSessionParams{
		UserID: userID, TokenHash: tokenHash[:], SessionTtlMilliseconds: durationMilliseconds(ttl),
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("create session: %w", err)
	}
	return session.ExpiresAt.Time, nil
}

func (r *AuthRepository) AuthenticateSession(ctx context.Context, tokenHash [32]byte) (SessionUser, error) {
	row, err := db.New(r.pool).AuthenticateSession(ctx, tokenHash[:])
	if err != nil {
		return SessionUser{}, fmt.Errorf("authenticate session: %w", err)
	}
	if row.Username == nil || !row.ExpiresAt.Valid {
		return SessionUser{}, errors.New("authenticated session has invalid user data")
	}
	return SessionUser{ID: row.UserID, Username: *row.Username, ExpiresAt: row.ExpiresAt.Time}, nil
}

func (r *AuthRepository) RevokeSession(ctx context.Context, tokenHash [32]byte) error {
	if err := db.New(r.pool).RevokeSession(ctx, tokenHash[:]); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (r *AuthRepository) ResetPassword(ctx context.Context, normalized, passwordHash string) (db.User, error) {
	return withTx(ctx, r.pool, func(queries *db.Queries) (db.User, error) {
		user, err := queries.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{
			UsernameNormalized: &normalized, PasswordHash: &passwordHash,
		})
		if err != nil {
			return db.User{}, fmt.Errorf("update password: %w", err)
		}
		if err := queries.RevokeUserSessions(ctx, user.ID); err != nil {
			return db.User{}, fmt.Errorf("revoke user sessions: %w", err)
		}
		return user, nil
	})
}
