-- name: CreateSubRequest :execresult
INSERT INTO sub_requests (show_id, posted_by_user_id, taken_by_user_id, start_time, end_time, notes, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListSubRequests :many
SELECT sr.id, sr.show_id, sr.posted_by_user_id, sr.taken_by_user_id, u.email, COALESCE(u2.email, '') AS taker_email, sr.start_time, sr.end_time, sr.notes, sr.created_at, sr.updated_at
FROM sub_requests sr
JOIN users u ON sr.posted_by_user_id = u.id
LEFT JOIN users u2 ON sr.taken_by_user_id = u2.id
ORDER BY sr.start_time ASC;

-- name: GetSubRequestByID :one
SELECT id, show_id, posted_by_user_id, taken_by_user_id, start_time, end_time, notes, created_at, updated_at
FROM sub_requests
WHERE id = ?;

-- name: GetSubRequestDetailByID :one
SELECT sr.id, sr.show_id, sr.posted_by_user_id, sr.taken_by_user_id, u.email AS requester_email, COALESCE(u2.email, '') AS taker_email, sr.start_time, sr.end_time, sr.notes, sr.created_at, sr.updated_at
FROM sub_requests sr
JOIN users u ON sr.posted_by_user_id = u.id
LEFT JOIN users u2 ON sr.taken_by_user_id = u2.id
WHERE sr.id = ?;

-- name: DeleteSubRequest :execresult
DELETE FROM sub_requests WHERE id = ?;

-- name: TakeSubRequest :execresult
UPDATE sub_requests SET taken_by_user_id = ?, updated_at = ? WHERE id = ? AND taken_by_user_id IS NULL;

-- name: UntakeSubRequest :execresult
UPDATE sub_requests SET taken_by_user_id = NULL, updated_at = ? WHERE id = ? AND taken_by_user_id = ?;
