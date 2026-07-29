-- name: InsertInFlight :one
-- Insert-first claim of an idempotency key. Runs BEFORE the handler; the UNIQUE constraint
-- arbitrates concurrent retries (the loser gets 23505, never a raced SELECT). Committed on
-- its own so the claim is visible immediately to a competing request — that visibility is
-- the whole point, and the reason this is not folded into the handler's money tx.
INSERT INTO idempotency_keys (merchant_id, idem_key, request_hash, status)
VALUES ($1, $2, $3, 'in_flight')
RETURNING *;

-- name: GetByKey :one
-- Read the existing claim after a 23505, to decide replay (completed + same hash) vs
-- 409 (still in_flight) vs 422 (same key, different request).
SELECT * FROM idempotency_keys
WHERE merchant_id = $1 AND idem_key = $2;

-- name: MarkCompleted :exec
-- Store the first caller's final response (raw bytes) so retries replay it byte-for-byte.
-- Scoped by id (the row THIS request inserted), so it can never stamp a different row that a
-- sweeper deleted-and-a-retry-recreated under the same (merchant_id, idem_key).
UPDATE idempotency_keys
SET status = 'completed', response_code = $2, response_body = $3
WHERE id = $1;

-- name: DeleteKey :exec
-- Release a claim whose handler produced a transient (5xx/panic) result, so a retry can re-run
-- rather than being wedged replaying a server error. Scoped by id for the same reason as above.
DELETE FROM idempotency_keys
WHERE id = $1;

-- name: ResetOrphans :exec
-- Crash recovery: free rows stuck 'in_flight' past the cutoff. Without this, a crash mid
-- request pins every later retry at 409 forever.
DELETE FROM idempotency_keys
WHERE status = 'in_flight' AND locked_at < $1;

-- name: ExpireCompleted :exec
-- Enforce the ~24h key lifetime (see nem_pay/CLAUDE.md): drop completed rows past the cutoff
-- so the table stays bounded and a key legitimately reused after its lifetime runs fresh
-- rather than replaying a stale response.
DELETE FROM idempotency_keys
WHERE status = 'completed' AND created_at < $1;
