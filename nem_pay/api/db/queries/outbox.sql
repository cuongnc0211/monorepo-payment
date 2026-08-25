-- name: InsertOutbox :one
-- Called INSIDE the money tx (with the caller's *Queries bound to that tx) so the event is
-- written atomically with the state change it announces.
INSERT INTO outbox (merchant_id, event_id, event_type, payload)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ClaimDueOutbox :many
-- The dispatcher's claim: due rows (pending, next_attempt_at reached), skip-locked so multiple
-- dispatchers never grab the same row, and leased forward (next_attempt_at += lease) so a row
-- isn't re-claimed while its delivery is in flight. If delivery crashes, the lease lapses and the
-- row becomes due again — at-least-once by construction.
UPDATE outbox
SET next_attempt_at = now() + make_interval(secs => sqlc.arg('lease_seconds')::double precision)
WHERE id IN (
  SELECT id FROM outbox
  WHERE status = 'pending' AND next_attempt_at <= now()
  ORDER BY next_attempt_at
  FOR UPDATE SKIP LOCKED
  LIMIT sqlc.arg('lim')
)
RETURNING *;

-- name: GetOutbox :one
SELECT * FROM outbox WHERE id = $1;

-- name: MarkOutboxDelivered :exec
UPDATE outbox SET status = 'delivered' WHERE id = $1;

-- name: MarkOutboxRetry :exec
-- A failed attempt: bump attempts, schedule the next try (backoff computed by the caller), record
-- the error. Stays 'pending' so the dispatcher re-claims it after next_attempt_at.
UPDATE outbox
SET attempts = attempts + 1, next_attempt_at = $2, last_error = $3
WHERE id = $1;

-- name: MarkOutboxDead :exec
-- Give up after the max attempts: park the row in 'dead' (a poison message can't wedge the queue).
UPDATE outbox
SET attempts = attempts + 1, status = 'dead', last_error = $2
WHERE id = $1;

-- name: InsertDelivery :exec
-- One row per delivery attempt (the delivery log).
INSERT INTO webhook_deliveries (outbox_id, attempt, status_code, ok, error)
VALUES ($1, $2, $3, $4, $5);

-- name: ListWebhookEventsForMerchant :many
-- Portal webhook log: emitted events for a merchant, newest first. Delivery state is derived by
-- the caller from status + attempts (delivered / dead=failed / pending+attempts>0=retrying).
SELECT id, event_id, event_type, status, attempts, last_error, created_at
FROM outbox
WHERE merchant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;
