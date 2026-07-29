-- 0001_ledger — the double-entry ledger: NemPay's source of truth for all money.
--
-- Append-only by design. There is deliberately NO balance column on any table:
-- an account's balance is always DERIVED as SUM(debit) - SUM(credit) over its entries.
-- A balance you can UPDATE is a balance you can corrupt; a balance you must SUM is
-- provable at all times. This is the core lesson, not an optimisation choice.

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

-- Uniqueness is split into two PARTIAL indexes on purpose. A plain
-- UNIQUE(..., reference_id) treats every NULL as distinct, which would allow
-- duplicate singleton accounts (two 'platform_cash' rows) and silently split the
-- ledger across them. Splitting on reference_id NULL / NOT NULL closes that hole
-- and lets GetOrCreateAccount rely on insert-first (no SELECT-then-INSERT race).
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
