# Task 02 (M1.1) — Ledger core (double-entry source of truth)

**Milestone:** M1 · **Depends on:** M1.0 · **Blocks:** M1.4 (money flow posts here), M1.5, reconciliation

## Context
- Gateway rules: [`../../nem_pay/CLAUDE.md`](../../nem_pay/CLAUDE.md) — "Double-entry ledger (source of truth)".
- This phase builds the append-only ledger and one generic posting primitive. It carries
  **escrow-adaptability guards #2, #3, #6** — build the seams now, use only direct-mode accounts.

## Requirements
- Append-only `accounts` / `transactions` / `entries`. **No balance column anywhere.**
- Balance of an account = `SUM(entries.amount)`. Reports use `numeric`, not float.
- One primitive `PostTransaction(tx, kind, refID, entries[])` that **enforces Σdebit = Σcredit**
  and writes the transaction + entries in the caller's DB transaction.
- Accounts are **typed** (asset/liability/revenue) and can be **per-reference** (per intent /
  per payee) so `escrow_liability(intent)` in M3 is just another account — no schema change.

## Entry model & balance convention (deliberate choice: separate debit/credit columns)
- Each entry has **two non-negative columns, `debit` and `credit`**; exactly one is non-zero.
  This mirrors how accountants write a journal (Nợ/Có), removes all sign ambiguity, and keeps
  every amount non-negative. (Chosen over a single signed column for teaching clarity.)
- A transaction is balanced when **Σdebit = Σcredit** across its entries.
- Account balance = `SUM(debit) − SUM(credit)` (a debit-normal balance). Asset accounts carry a
  positive balance; liability & revenue accounts a negative one — display magnitude in the API.

## Schema (migration `0001_ledger`)
```sql
CREATE TABLE accounts (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id  uuid NOT NULL,
  type         text NOT NULL CHECK (type IN ('asset','liability','revenue')),
  kind         text NOT NULL,          -- 'platform_cash','acquirer_receivable','merchant_payable',
                                        --  later: 'escrow_liability','payable_to_payee','platform_revenue','refund_payable'
  currency     char(3) NOT NULL,
  reference_id uuid,                    -- e.g. payment_intent id for per-intent accounts; NULL = singleton
  created_at   timestamptz NOT NULL DEFAULT now()
);
-- Singleton accounts unique per (merchant, kind, currency); per-reference accounts unique incl. reference_id.
CREATE UNIQUE INDEX accounts_singleton_uq ON accounts (merchant_id, kind, currency)
  WHERE reference_id IS NULL;
CREATE UNIQUE INDEX accounts_perref_uq    ON accounts (merchant_id, kind, currency, reference_id)
  WHERE reference_id IS NOT NULL;

CREATE TABLE transactions (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id  uuid NOT NULL,
  kind         text NOT NULL,          -- 'capture','settle','refund', later 'release'
  reference_id uuid,                    -- payment_intent id
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE entries (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  transaction_id uuid NOT NULL REFERENCES transactions(id),
  account_id     uuid NOT NULL REFERENCES accounts(id),
  debit          bigint NOT NULL DEFAULT 0 CHECK (debit  >= 0),
  credit         bigint NOT NULL DEFAULT 0 CHECK (credit >= 0),
  currency       char(3) NOT NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  CHECK ( (debit = 0) <> (credit = 0) )  -- exactly one side non-zero (rejects 0/0 and both>0)
);
CREATE INDEX entries_account_idx ON entries (account_id);
```

## Files to create
```
api/db/migrations/0001_ledger.up.sql / .down.sql
api/db/queries/accounts.sql          -- GetOrCreateAccount, GetAccountBalance (SUM), ListAccounts
api/db/queries/ledger.sql            -- InsertTransaction, InsertEntry
api/internal/ledger/post.go          -- PostTransaction(ctx, q, kind, refID, []Entry) enforcing sum==0
api/internal/ledger/accounts.go      -- account-kind constants + typed helpers
```

## Implementation steps
1. Write migration `0001_ledger`; `make sqlc` to generate typed queries.
2. `GetOrCreateAccount` uses insert-first against the partial unique indexes (no SELECT-then-INSERT).
3. `PostTransaction`: validate every entry currency matches and each entry has exactly one side
   non-zero; assert `Σdebit == Σcredit` (return a typed `ErrUnbalanced` otherwise); insert the
   transaction then entries — all on the passed `*Queries` bound to the caller's tx (the money
   service owns the tx boundary; see M1.4).
4. `GetAccountBalance` = `SELECT COALESCE(SUM(debit),0) - COALESCE(SUM(credit),0) FROM entries
   WHERE account_id=$1`.

## Validation / tests
- Posting where `Σdebit ≠ Σcredit` → `ErrUnbalanced`, nothing written.
- An entry with both sides zero, or both sides non-zero → rejected by the CHECK.
- Balance after a balanced capture-shaped posting matches expectation; asset balance +, liability −.
- `GetOrCreateAccount` is idempotent under concurrency (race two goroutines → one row).
- No table has a mutable balance column (grep guard in test).

## Risks & rollback
- **NULL-in-unique pitfall**: a plain `UNIQUE(...,reference_id)` treats NULLs as distinct →
  duplicate singleton accounts. The two **partial** indexes above are mandatory, not optional.
- **Debit/credit drift**: if M1.4/M3 post to the wrong side the ledger silently skews. Lock the
  convention in the `ledger` package doc + a golden test asserting known postings' balances.
- Rollback: `0001_ledger.down.sql` drops the three tables; safe while no money flow depends on it.
