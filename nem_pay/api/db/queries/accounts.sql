-- name: GetOrCreateSingletonAccount :one
-- Insert-first get-or-create for a singleton account (reference_id IS NULL), e.g. platform_cash.
-- The INSERT runs first and the partial unique index arbitrates any race between concurrent
-- creators — never SELECT-then-INSERT (a TOCTOU race). On conflict we DO UPDATE (a no-op that
-- re-sets kind to itself) rather than DO NOTHING: DO UPDATE ... RETURNING always yields the live
-- row, whereas DO NOTHING + a UNION-ed SELECT returns ZERO rows when the losing txn's snapshot
-- predates the winner's commit (READ COMMITTED) — a real race that surfaced as a spurious
-- ErrNoRows on a merchant's first concurrent capture.
INSERT INTO accounts (merchant_id, type, kind, currency)
VALUES ($1, $2, $3, $4)
ON CONFLICT (merchant_id, kind, currency) WHERE reference_id IS NULL
DO UPDATE SET kind = EXCLUDED.kind
RETURNING *;

-- name: GetOrCreatePerRefAccount :one
-- Same insert-first discipline for a per-reference account (reference_id set), e.g. a per-intent
-- escrow_liability in M3. Same DO UPDATE ... RETURNING race-safety as above, different index.
INSERT INTO accounts (merchant_id, type, kind, currency, reference_id)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (merchant_id, kind, currency, reference_id) WHERE reference_id IS NOT NULL
DO UPDATE SET kind = EXCLUDED.kind
RETURNING *;

-- name: GetAccountBalance :one
-- Balance is DERIVED, never stored: Σdebit − Σcredit over the account's entries. Cast to
-- numeric (not float) so the type is exact and deterministic; asset accounts read positive
-- (debit-normal), liability & revenue negative.
SELECT (COALESCE(SUM(debit), 0) - COALESCE(SUM(credit), 0))::numeric AS balance
FROM entries
WHERE account_id = $1;

-- name: ListAccounts :many
SELECT * FROM accounts
WHERE merchant_id = $1
ORDER BY created_at;
