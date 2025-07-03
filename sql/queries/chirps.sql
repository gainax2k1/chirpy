-- name: CreateChirp :one
INSERT INTO chirps (body, user_id)
VALUES (
    $1,
    $2
)

RETURNING *;

-- name: GetChirps :many
SELECT *
    FROM chirps
    ORDER BY chirps.created_at ASC;

-- name: GetChirpByChirpUUID :one
SELECT *
    FROM chirps
    WHERE ID = $1;