# Task 04 — Webhook endpoint: verify + dedup + paid transition

**Plan:** ./plan.md · **Depends on:** task-03 · **Blocks:** task-06

## Context
- Spec acceptance criteria covered: **AC1** (`paid` on captured), **AC3** (`paid` set ONLY here),
  **AC4** (duplicate deduped), **AC5** (bad signature rejected).
- Links: [`../../Nem_luxury/CLAUDE.md`](../../Nem_luxury/CLAUDE.md) — "Webhooks"; NemPay signing from M1
  (`X-NemPay-Signature: sha256=…`, `X-NemPay-Event-Id`, `X-NemPay-Event-Type`).

## Requirements
- One endpoint `POST /webhooks/nem_pay`, **CSRF-exempt**, handled **synchronously** (no job), returning `200` on success.
- **Verify first**: recompute `sha256=hex(HMAC-SHA256(webhook_secret, RAW body))` and constant-time
  compare to `X-NemPay-Signature`. On mismatch → **reject `400`**, change nothing (AC5). Read the **raw**
  body before any parsing.
- **Dedup**: `ProcessedWebhookEvent` with `UNIQUE(event_id)`; insert-first inside the update transaction.
  A duplicate `event_id` → no-op, still `200` (AC4).
- **Apply**: `payment_intent.captured` → find the order by `nem_pay_intent_id` → `mark_paid!` (guarded,
  idempotent). Other event types → acknowledged `200`, no-op in M2 (refunds deferred). Unknown intent →
  verify+dedup, log, `200` (so NemPay stops retrying) — never crash.
- `paid` is set **only** here — never from a checkout response or redirect (AC3).

## Files to create / modify
```
Nem_luxury/db/migrate/*_create_processed_webhook_events.rb
Nem_luxury/app/models/processed_webhook_event.rb
Nem_luxury/app/services/nem_pay/webhook_verifier.rb
Nem_luxury/app/services/nem_pay/webhook_handler.rb
Nem_luxury/app/controllers/webhooks/nem_pay_controller.rb
Nem_luxury/config/routes.rb                       (post "/webhooks/nem_pay")
```

## Implementation steps
1. Migration/model for `processed_webhook_events` (`event_id` UNIQUE, `event_type`, `order_id?`).
2. `WebhookVerifier.verify(raw_body, signature_header, secret)` using
   `ActiveSupport::SecurityUtils.secure_compare` over the recomputed `sha256=…` string.
3. Controller: `skip_forgery_protection`; read `request.raw_post`; verify → `400` on mismatch; parse JSON;
   pull `event_id`/`event_type` from headers (and the intent id from the payload).
4. `WebhookHandler.call(event)`: in a transaction, insert the `ProcessedWebhookEvent` (rescue
   unique-violation → return `:duplicate`); on `payment_intent.captured` find order by `nem_pay_intent_id`
   and `mark_paid!`; return status. Controller renders `200` for handled/duplicate.

## Validation / tests
| Scenario | Expected | AC |
|---|---|---|
| valid sig + captured for a pending order | order → `paid`; one `ProcessedWebhookEvent` | AC1, AC3 |
| duplicate (same `event_id`) | order transitions **once**; 2nd delivery `200` no-op; still one event row | AC4 |
| bad signature | `400`; order unchanged; **no** event row | AC5 |
| non-captured event (authorized/settled) | `200` no-op; order unchanged | AC3 |
| captured for unknown intent | `200`, logged, no crash | (robustness) |
| already-paid order, captured again (new event_id) | `mark_paid!` no-op; `200` | AC4 |

## Risks & rollback
- **Raw-body consumption** by middleware/params → read `request.raw_post` before parsing; verify over
  exact bytes (NemPay signs the delivered bytes).
- **Signature mismatch due to secret drift** (task-05) → log clearly; both sides must share the value.
- **Order lookup**: match on the stored `nem_pay_intent_id` (set at checkout) / payload intent id.
- Rollback: drop the migration; remove verifier/handler/controller/route (money flow still works; orders
  simply never leave `pending_payment`).
