-- name: InsertAPIKey :one
INSERT INTO api_keys (merchant_id, kind, token_prefix, token_hash)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetKeysByPrefix :many
-- Narrow to candidate keys by the indexed, non-secret prefix; the caller then compares the
-- full-token hash in constant time. Revoked keys are excluded here so they can never match.
SELECT * FROM api_keys
WHERE token_prefix = $1 AND revoked_at IS NULL;

-- name: CountKeysForMerchant :one
-- Used by the dev seed to stay idempotent (only insert keys if a merchant has none yet).
SELECT count(*) FROM api_keys WHERE merchant_id = $1;

-- name: ListAPIKeysForMerchant :many
-- Portal API-keys view. Returns only the non-secret prefix (never token_hash) so a full key can
-- never be reconstructed from this endpoint.
SELECT id, kind, token_prefix, created_at, revoked_at
FROM api_keys
WHERE merchant_id = $1
ORDER BY created_at DESC;
