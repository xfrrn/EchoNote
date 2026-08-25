package httpapi

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
)

type ownerContextKey struct{}

func identify(ownerID pgtype.UUID) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ownerContextKey{}, ownerID)))
		})
	}
}

func requestUserID(r *http.Request) pgtype.UUID {
	ownerID, _ := r.Context().Value(ownerContextKey{}).(pgtype.UUID)
	return ownerID
}
