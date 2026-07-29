-- 0005_outbox_webhooks — the notification plane, kept OUT of the money/ledger tx.
--
-- A state change writes an outbox row IN THE SAME tx as the change (so an event exists iff the
-- change committed — no lost, no phantom events). A separate worker then delivers it out-of-band:
-- at-least-once, exponential backoff, dead-letter. event_type is a free string, so M3's
-- 'escrow.released' needs no schema change (escrow-adaptability guard #5).
CREATE TABLE webhook_endpoints (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  url         text NOT NULL,
  secret      text NOT NULL,            -- HMAC signing secret, shared with the merchant
  created_at  timestamptz NOT NULL DEFAULT now(),
  disabled_at timestamptz
);
CREATE INDEX webhook_endpoints_merchant_idx ON webhook_endpoints (merchant_id) WHERE disabled_at IS NULL;

CREATE TABLE outbox (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id     uuid NOT NULL,
  event_id        uuid NOT NULL UNIQUE,   -- stable across redeliveries so receivers dedupe
  event_type      text NOT NULL,          -- 'payment_intent.captured', later 'escrow.released'
  payload         jsonb NOT NULL,
  status          text NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','delivered','dead')),
  attempts        int NOT NULL DEFAULT 0,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  last_error      text,
  created_at      timestamptz NOT NULL DEFAULT now()
);
-- The dispatcher scans this: due = pending AND next_attempt_at <= now.
CREATE INDEX outbox_dispatch_idx ON outbox (status, next_attempt_at);

CREATE TABLE webhook_deliveries (               -- per-attempt log (the portal shows this later)
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  outbox_id   uuid NOT NULL REFERENCES outbox(id),
  attempt     int NOT NULL,
  status_code int,
  ok          boolean NOT NULL,
  error       text,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX webhook_deliveries_outbox_idx ON webhook_deliveries (outbox_id);
