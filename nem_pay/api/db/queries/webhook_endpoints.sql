-- name: InsertWebhookEndpoint :one
INSERT INTO webhook_endpoints (merchant_id, url, secret)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListActiveEndpoints :many
-- Active (non-disabled) endpoints for a merchant — where its events are delivered.
SELECT * FROM webhook_endpoints
WHERE merchant_id = $1 AND disabled_at IS NULL
ORDER BY created_at;
