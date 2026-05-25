-- name: CreateUser :execresult
INSERT INTO users (email, role) VALUES (?, ?);

-- name: GetUserByID :one
SELECT id, email, role, is_enabled, created_at FROM users WHERE id = ?;

-- name: GetUserByEmail :one
SELECT id, email, role, is_enabled, created_at FROM users WHERE email = ?;

-- name: ListUsers :many
SELECT id, email, role, is_enabled, created_at FROM users ORDER BY created_at DESC;

-- name: ListActiveUsers :many
SELECT id, email, role, is_enabled, created_at FROM users WHERE is_enabled = true ORDER BY email ASC;

-- name: UpdateUserRole :execresult
UPDATE users SET role = ? WHERE id = ?;

-- name: UpdateUserEnabled :execresult
UPDATE users SET is_enabled = ? WHERE id = ?;

-- name: UpdateUserRoleAndEnabled :execresult
UPDATE users SET role = ?, is_enabled = ? WHERE id = ?;

-- name: ImportUser :exec
INSERT INTO users (email, role) VALUES (?, 'member') ON CONFLICT (email) DO NOTHING;
