-- name: CreateIntent :one
-- Create a payment intent in its initial 'created' state. settlement_mode is 'direct' for all
-- of M1; the column exists so M3 can pass 'escrow' without a schema change.
INSERT INTO payment_intents (merchant_id, amount, currency, settlement_mode, metadata)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetIntent :one
-- Reads are always scoped by merchant_id so one merchant can never fetch another's intent,
-- even with a guessed id.
SELECT * FROM payment_intents
WHERE id = $1 AND merchant_id = $2;

-- name: ListIntents :many
-- List a merchant's intents, newest first, with an optional status filter and paging. The
-- status filter is a nullable arg (sqlc.narg): NULL means "no filter" — the dynamic-filter
-- pattern from nem_pay/CLAUDE.md, done in plain SQL, never GORM.
SELECT * FROM payment_intents
WHERE merchant_id = sqlc.arg('merchant_id')
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: LockIntentForUpdate :one
-- Row-lock an intent inside the money tx. Two concurrent captures both call this; the second
-- BLOCKS until the first commits, then reads the already-advanced status and is rejected by the
-- state machine — no double-capture, no double ledger post. Scoped by merchant for tenant safety.
SELECT * FROM payment_intents
WHERE id = $1 AND merchant_id = $2
FOR UPDATE;

-- name: UpdateIntentStatus :one
-- Advance an intent's status (and bump updated_at, which the settle/expiry sweeps scan on).
UPDATE payment_intents
SET status = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ListCapturedBefore :many
-- The settle sweep's selection: captured intents older than the settlement delay. Only these
-- settle; authorized/failed are untouched.
SELECT * FROM payment_intents
WHERE status = 'captured' AND updated_at < $1
ORDER BY updated_at
LIMIT $2;

-- name: ListStaleBefore :many
-- The expiry sweep's selection: PRE-AUTHORIZATION intents that have sat too long. Deliberately
-- excludes 'authorized' — an authorized intent holds real funds at the acquirer, and failing it
-- without a bank VOID would leak that hold. Voiding is a later lesson, so authorized intents are
-- left for capture (or a future void-and-expire path).
SELECT * FROM payment_intents
WHERE status IN ('created','requires_confirmation') AND updated_at < $1
ORDER BY updated_at
LIMIT $2;
