-- name: CreateUser :one
INSERT INTO users (username, username_normalized, password_hash, status)
VALUES (sqlc.arg(username), sqlc.arg(username_normalized), sqlc.arg(password_hash), 'active')
RETURNING *;

-- name: EnsurePlaceholderUser :exec
INSERT INTO users (id)
VALUES (sqlc.arg(user_id))
ON CONFLICT (id) DO NOTHING;

-- name: ClaimUser :one
UPDATE users
SET username = sqlc.arg(username),
    username_normalized = sqlc.arg(username_normalized),
    password_hash = sqlc.arg(password_hash),
    status = 'active',
    updated_at = now()
WHERE id = sqlc.arg(user_id)
  AND status = 'placeholder'
RETURNING *;

-- name: GetUserForLogin :one
SELECT *
FROM users
WHERE username_normalized = sqlc.arg(username_normalized)
  AND status = 'active';

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, expires_at)
VALUES (
    sqlc.arg(user_id),
    sqlc.arg(token_hash),
    now() + (sqlc.arg(session_ttl_milliseconds)::bigint * interval '1 millisecond')
)
RETURNING *;

-- name: AuthenticateSession :one
UPDATE sessions AS session
SET last_seen_at = GREATEST(session.last_seen_at, now())
FROM users AS app_user
WHERE session.token_hash = sqlc.arg(token_hash)
  AND session.revoked_at IS NULL
  AND session.expires_at > now()
  AND app_user.id = session.user_id
  AND app_user.status = 'active'
RETURNING app_user.id AS user_id, app_user.username, session.expires_at;

-- name: RevokeSession :exec
UPDATE sessions
SET revoked_at = COALESCE(revoked_at, now())
WHERE token_hash = sqlc.arg(token_hash);

-- name: UpdateUserPassword :one
UPDATE users
SET password_hash = sqlc.arg(password_hash),
    updated_at = now()
WHERE username_normalized = sqlc.arg(username_normalized)
  AND status = 'active'
RETURNING *;

-- name: RevokeUserSessions :exec
UPDATE sessions
SET revoked_at = COALESCE(revoked_at, now())
WHERE user_id = sqlc.arg(user_id)
  AND revoked_at IS NULL;
