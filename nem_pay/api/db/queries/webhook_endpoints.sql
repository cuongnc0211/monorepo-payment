-- name: InsertWebhookEndpoint :one
INSERT INTO webhook_endpoints (merchant_id, url, secret)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListActiveEndpoints :many
-- Active (non-disabled) endpoints for a merchant — where its events are delivered.
SELECT * FROM webhook_endpoints
WHERE merchant_id = $1 AND disabled_at IS NULL
ORDER BY created_at;

-- name: ListEndpoints :many
-- All of a merchant's endpoints (including disabled), for the portal management view.
SELECT * FROM webhook_endpoints
WHERE merchant_id = $1
ORDER BY created_at DESC;

-- name: DisableEndpoint :one
-- Disable one of a merchant's endpoints. Scoped by merchant so another merchant's id matches no row.
UPDATE webhook_endpoints
SET disabled_at = now()
WHERE id = $1 AND merchant_id = $2 AND disabled_at IS NULL
RETURNING *;
