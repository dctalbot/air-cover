-- name: CreateMagicLink :exec
INSERT INTO magic_links (user_id, token_hash, expires_at) VALUES (?, ?, ?);

-- name: UseMagicLink :one
DELETE FROM magic_links
WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?
RETURNING id, user_id, token_hash, expires_at, used_at;

-- name: CreateSession :exec
INSERT INTO sessions (id, user_id, session_token, expires_at) VALUES (?, ?, ?, ?);

-- name: GetSessionByToken :one
SELECT id, user_id, session_token, expires_at FROM sessions WHERE session_token = ?;

-- name: DeleteSessionsByUserID :exec
DELETE FROM sessions WHERE user_id = ?;
