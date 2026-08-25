package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/Actify/echonote/apps/server/internal/database/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Resolve(ctx context.Context, issuer, subject, email string) (pgtype.UUID, error) {
	issuer, subject, email = strings.TrimSpace(issuer), strings.TrimSpace(subject), strings.TrimSpace(email)
	if issuer == "" || subject == "" {
		return pgtype.UUID{}, errors.New("authenticated issuer and subject are required")
	}
	var optionalEmail *string
	if email != "" {
		optionalEmail = &email
	}
	return db.New(r.pool).ResolveUser(ctx, db.ResolveUserParams{
		AuthIssuer: &issuer, AuthSubject: &subject, Email: optionalEmail,
	})
}

func (r *UserRepository) Bind(ctx context.Context, userID pgtype.UUID, issuer, subject string) error {
	issuer, subject = strings.TrimSpace(issuer), strings.TrimSpace(subject)
	if !userID.Valid || issuer == "" || subject == "" {
		return errors.New("user ID, issuer, and subject are required")
	}
	updated, err := db.New(r.pool).BindUserIdentity(ctx, db.BindUserIdentityParams{
		AuthIssuer: &issuer, AuthSubject: &subject, UserID: userID,
	})
	if err != nil {
		return err
	}
	if updated == 0 {
		return errors.New("user does not exist or already has an OAuth identity")
	}
	return nil
}
