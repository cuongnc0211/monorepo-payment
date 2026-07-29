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
