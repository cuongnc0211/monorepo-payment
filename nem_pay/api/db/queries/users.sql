-- name: GetUserByEmail :one
-- The login lookup. Returns the bcrypt hash for the handler to compare; the raw password never
-- leaves the request.
SELECT * FROM users WHERE email = $1;

-- name: InsertUser :one
INSERT INTO users (merchant_id, email, password_hash)
VALUES ($1, $2, $3)
RETURNING *;

-- name: CountUsersByEmail :one
-- Used by the dev seed to stay idempotent (insert a user only if its email is absent).
SELECT count(*) FROM users WHERE email = $1;
