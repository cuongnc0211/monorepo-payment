-- 0004_payment_intents — the payment intent: the object a merchant creates and drives through
-- its lifecycle. M1 is direct mode only; the escrow seams (settlement_mode='escrow', payee_id,
-- application_fee) exist now but stay unused, so M3 adds escrow by widening CHECKs, not by an
-- intent-semantics migration (escrow-adaptability guard #1).
--
-- merchant_id is a real FK to merchants (created in 0003): a money table must not hold intents
-- for a merchant that doesn't exist — tenant integrity is enforced by the DB, not by convention.
CREATE TABLE payment_intents (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id     uuid NOT NULL REFERENCES merchants(id),
  amount          bigint NOT NULL CHECK (amount > 0),          -- int64 minor units; never float
  currency        char(3) NOT NULL,
  status          text NOT NULL DEFAULT 'created'
    CHECK (status IN ('created','requires_confirmation','authorized','captured',
                      'settled','failed','refunded','partially_refunded')),  -- M3 adds escrow states
  settlement_mode text NOT NULL DEFAULT 'direct'
    CHECK (settlement_mode IN ('direct')),                     -- M3 adds 'escrow'
  payee_id        uuid,        -- NULL for direct; the third-party payee for escrow (M3)
  application_fee bigint,      -- NULL for direct; the platform fee for escrow (M3)
  metadata        jsonb NOT NULL DEFAULT '{}',
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX payment_intents_merchant_idx ON payment_intents (merchant_id, created_at DESC);
