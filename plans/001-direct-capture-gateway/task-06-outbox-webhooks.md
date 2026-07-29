# Task 06 (M1.5) — Outbox + async webhook delivery

**Milestone:** M1 · **Depends on:** M1.4 (state changes to emit) · **Blocks:** M2 (merchant reacts to webhooks); completes M1

## Context
- Gateway rules: [`../../nem_pay/CLAUDE.md`](../../nem_pay/CLAUDE.md) — "Webhooks (outbox pattern)".
- Repo principle: keep the **notification plane** separate from money/ledger. Redis/asynq stays
  in NemPay **on purpose** (real gateways deliver out-of-band). Carries **guard #5**: `event_type`
  is a free string, so M3's `held_in_escrow`/`escrow.released` need no schema change.

## Requirements
- On every state change, write an `outbox` row **in the same DB tx** as the change (M1.4 tx).
- A **separate asynq worker** delivers: at-least-once, exponential backoff, dead-letter after N attempts.
- Sign each payload with the merchant's webhook secret (**HMAC-SHA256**) in a header.
- Include a stable `event_id` so receivers dedupe (delivery is at-least-once).
- Delivery HTTP client uses a **generous timeout** — the M2/M4 merchants process webhooks
  synchronously, so give them room.

## Schema (migration `0005_outbox_webhooks`)
```sql
CREATE TABLE webhook_endpoints (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  url         text NOT NULL,
  secret      text NOT NULL,            -- HMAC signing secret (shared with the merchant)
  created_at  timestamptz NOT NULL DEFAULT now(),
  disabled_at timestamptz
);

CREATE TABLE outbox (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id     uuid NOT NULL,
  event_id        uuid NOT NULL UNIQUE,   -- sent to receiver for dedupe
  event_type      text NOT NULL,          -- 'payment_intent.captured', later 'escrow.released' — free string
  payload         jsonb NOT NULL,
  status          text NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending','delivered','failed','dead')),
  attempts        int NOT NULL DEFAULT 0,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  last_error      text,
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX outbox_dispatch_idx ON outbox (status, next_attempt_at);

CREATE TABLE webhook_deliveries (               -- delivery log (portal shows this later)
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  outbox_id   uuid NOT NULL REFERENCES outbox(id),
  attempt     int NOT NULL,
  status_code int,
  ok          boolean NOT NULL,
  error       text,
  created_at  timestamptz NOT NULL DEFAULT now()
);
```

## Files to create
```
api/db/migrations/0005_outbox_webhooks.up.sql / .down.sql
api/db/queries/outbox.sql            -- InsertOutbox, ClaimDue, MarkDelivered, MarkFailed/Dead
api/db/queries/webhook_endpoints.sql
api/internal/outbox/emit.go          -- EmitEvent(ctx, q, merchantID, type, payload) — INSERT in caller's tx
api/internal/outbox/dispatcher.go    -- enqueue due rows onto asynq (or poll) 
api/internal/webhook/sign.go         -- HMAC-SHA256 header
api/internal/webhook/deliver.go      -- HTTP POST + backoff + delivery-log write
api/cmd/worker/main.go               -- asynq worker process (added to docker-compose)
```

## Implementation steps
1. Migration `0005`; `make sqlc`.
2. In M1.4's money service, after the status change (same tx) call `outbox.EmitEvent` with a fresh
   `event_id` and `event_type` (e.g. `payment_intent.captured`). **Same tx** — non-negotiable.
3. Dispatcher: a scheduler enqueues due `outbox` rows (`status='pending' AND next_attempt_at<=now`)
   as asynq tasks; the worker delivers.
4. `deliver.go`: POST payload to the endpoint URL with `X-NemPay-Signature: sha256=…`; on 2xx →
   `MarkDelivered`; else increment `attempts`, set `next_attempt_at` (exponential backoff), log a
   `webhook_deliveries` row; after N attempts → `dead`.
5. Add the `worker` service to `docker-compose.yml`.

## Validation / tests
- A captured intent writes exactly one `outbox` row in the **same tx** (rollback the tx → no outbox row).
- Worker delivers; endpoint receives a payload whose HMAC verifies against the secret.
- Endpoint returning 500 → retried with growing `next_attempt_at`; after N → `dead`, logged.
- Redelivery carries the **same `event_id`** (receiver can dedupe).
- Killing the worker mid-run → on restart, due rows are re-claimed and delivered (at-least-once).

## Risks & rollback
- **Dual-write trap**: emitting the event via a direct HTTP call from the money tx (instead of the
  outbox) collapses the planes and loses events if the merchant is down. The outbox INSERT-in-same-tx
  is the whole point — do not "simplify" it.
- **Poison messages** wedging the queue → the `dead` state + delivery log prevent infinite retry.
- **Clock/backoff** using wall-clock in tests is flaky — inject a clock.
- Rollback: drop `0005` + remove the worker service; money flow (M1.4) still works without notifications.

## Definition of done for M1
With M1.0–M1.5 complete: `docker-compose up` in `NemPay/` gives a gateway where a direct payment
intent goes create → authorize → capture → settle, idempotently, posting a balanced ledger, and a
signed webhook is delivered out-of-band — all exercisable by `curl` with no merchant present. Ready
for M2 (NemLuxury) to integrate.
