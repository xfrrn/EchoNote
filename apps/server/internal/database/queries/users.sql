-- name: ResolveUser :one
INSERT INTO users (auth_issuer, auth_subject, email)
VALUES (sqlc.arg(auth_issuer), sqlc.arg(auth_subject), sqlc.narg(email)::text)
ON CONFLICT (auth_issuer, auth_subject) WHERE auth_issuer IS NOT NULL AND auth_subject IS NOT NULL
DO UPDATE SET
    email = COALESCE(EXCLUDED.email, users.email),
    updated_at = now()
WHERE users.status = 'active'
RETURNING id;

-- name: BindUserIdentity :execrows
UPDATE users
SET auth_issuer = sqlc.arg(auth_issuer),
    auth_subject = sqlc.arg(auth_subject),
    updated_at = now()
WHERE id = sqlc.arg(user_id)
  AND auth_issuer IS NULL
  AND auth_subject IS NULL;
