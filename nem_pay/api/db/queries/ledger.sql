-- name: InsertTransaction :one
-- One row per money event (capture/settle/refund, later release). The balanced entries
-- that make it sum to zero are inserted separately, all inside the caller's DB tx.
INSERT INTO transactions (merchant_id, kind, reference_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: InsertEntry :one
-- One leg of a transaction. The table's CHECK guarantees exactly one of debit/credit is
-- non-zero and both are non-negative — malformed legs are impossible to persist.
INSERT INTO entries (transaction_id, account_id, debit, credit, currency)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: LedgerForIntent :many
-- The transaction(s)/entries backing one intent (capture, settle, refund, ...), for the portal's
-- payment detail. Scoped by merchant so another merchant's intent yields no rows.
SELECT t.id AS transaction_id, t.kind AS transaction_kind, t.created_at AS transaction_created_at,
       e.id AS entry_id, a.type AS account_type, a.kind AS account_kind,
       e.debit, e.credit, e.currency
FROM transactions t
JOIN entries e   ON e.transaction_id = t.id
JOIN accounts a  ON a.id = e.account_id
WHERE t.merchant_id = $1 AND t.reference_id = $2
ORDER BY t.created_at, e.created_at;
