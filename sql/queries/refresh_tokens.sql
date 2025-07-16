-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (token, user_id, expires_at)
VALUES (
    $1,
    $2,
    $3
)
RETURNING *;

-- name: GetUserByRefreshToken :one
SELECT *
    FROM refresh_tokens
    WHERE token = $1;

-- name: RevokeRefreshToken :one
UPDATE refresh_tokens
    SET revoked_at = NOW(),
        updated_at = NOW()  
    WHERE token = $1
RETURNING *;



