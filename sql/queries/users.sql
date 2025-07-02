-- name: CreateUser :one
INSERT INTO users (email)
VALUES (
    $1
 
)
RETURNING *;

-- name: Reset :exec
DELETE FROM users;