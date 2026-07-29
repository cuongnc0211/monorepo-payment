-- name: CreateMerchant :one
INSERT INTO merchants (name) VALUES ($1) RETURNING *;

-- name: GetMerchant :one
SELECT * FROM merchants WHERE id = $1;
