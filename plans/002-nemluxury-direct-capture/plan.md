# Plan — NemLuxury: direct-capture integration with NemPay (M2)

**ID:** 002-nemluxury-direct-capture · **Status:** complete (all tasks done; 37 specs green + live docker e2e)
**Spec:** ../../specs/002-nemluxury-direct-capture/spec.md
**Constitution:** ../../CLAUDE.md, ../../Nem_luxury/CLAUDE.md, ../../nem_pay/CLAUDE.md

## Decisions & fixes during implementation
- **Paid decided off the SIGNED payload status, not the unsigned event-type header** (task-04): the HMAC
  covers only the body, so keying the paid transition off `X-NemPay-Event-Type` let a relabelled
  authorized event flip an order to paid. Now gates on `payload["status"] == "captured"` + amount match.
- **Webhook dedup is DB-index insert-first** (no model-level uniqueness validation, which would TOCTOU
  and raise RecordInvalid instead of RecordNotUnique).
- **Single-drive guard made app-authoritative** (task-03): only the request that inserts the order row
  drives payment; a race-loser resets its `created` flag and doesn't re-drive.
- **Rails host authorization** allows `host.docker.internal` in development (task-06): webhooks from the
  NemPay container were 403-blocked otherwise — found only by the live e2e.
- **Captured-for-non-pending order is acknowledged, not raised** (task-04): avoids a 5xx redelivery loop.

## Known M2 scope limits (documented in code)
- Refunds deferred; partial gateway-failure mid-checkout is not auto-retried from the UI; dedup integrity
  rides on side-effect idempotency because NemPay's event_id is header-sourced (a future NemPay contract
  improvement should sign it); orders are unauthenticated (demo merchant, amount is server-side).

> This file is the **HOW**. Every choice satisfies the spec's acceptance criteria and obeys the
> constitution. Tasks are decomposed separately in `/sdd:tasks`.

## Approach (HOW)
A fresh **Rails 8 app** in `Nem_luxury/` owns a small luxury catalogue and a **single-item "buy now"**
checkout. All gateway interaction lives behind a service layer (`app/services/nem_pay/…`) — no NemPay
calls from controllers or models. On checkout, the service orchestrates NemPay's direct flow
**create → confirm → capture** server-to-server (secret key; deterministic per-checkout Idempotency-
Keys). The customer picks a **test payment method** that maps to a bank-sim magic token — no PAN ever
exists. The order is created `pending_payment` and becomes `paid` **only** when a **verified, deduped**
`payment_intent.captured` webhook arrives — never from the synchronous capture response or a redirect.
A small **dev-only change to NemPay's seed** registers NemLuxury's webhook endpoint (url + shared
secret) so events can flow; NemPay's public API and money logic are untouched.

## Architecture / components touched
**`Nem_luxury/` — new Rails 8 app** (SQLite, RSpec, server-rendered ERB, **no** Sidekiq/Redis):
- **Models:** `Product` (catalogue), `Order` (lifecycle + intent id), `ProcessedWebhookEvent` (dedup).
- **Services (`app/services/nem_pay/`):**
  - `Client` — thin `Net::HTTP` wrapper: `create_intent`, `confirm`, `capture`, `get_intent`; sets
    `Authorization: Bearer <secret>`, `Idempotency-Key`, JSON; maps errors to typed results.
  - `Checkout` — orchestrates create → confirm(token) → capture with deterministic idempotency keys.
  - `WebhookVerifier` — recompute HMAC-SHA256 over the **raw** body, constant-time compare.
  - `WebhookHandler` — dedup on `event_id`, then apply the state transition.
- **Controllers:** `ProductsController` (index/show+buy form), `CheckoutsController#create`,
  `OrdersController#show` (status page), `Webhooks::NemPayController#create`.
- **Views:** catalogue, product/buy (with a test-payment-method selector), order status.
- **Config:** `NEMPAY_API_URL`, `NEMPAY_SECRET_KEY`, `NEMPAY_WEBHOOK_SECRET` via Rails credentials/ENV.

**`nem_pay/` — dev-only wiring (additive, no API/money change):**
- Extend `internal/devseed` to insert a `webhook_endpoints` row for the dev merchant from
  `NEMPAY_DEV_WEBHOOK_URL` + `NEMPAY_DEV_WEBHOOK_SECRET` (idempotent; skipped if unset).
- Add those env vars to the `api` service in `docker-compose.yml`
  (`NEMPAY_DEV_WEBHOOK_URL` default `http://host.docker.internal:3000/webhooks/nem_pay`).

## Key decisions & alternatives considered
- **Fresh Rails 8, SQLite, RSpec, server-rendered ERB.** *Alt:* API-only + SPA. *Why:* the constitution
  says full-stack Rails is right where the app owns a domain; server-rendered is explicitly fine.
- **NemPay client = `Net::HTTP` wrapper.** *Alt:* Faraday. *Why:* four calls don't justify a gem;
  minimal deps, the exact request is visible (chosen "minimal/stdlib" posture).
- **Money = integer-cents columns + `currency` (char 3).** *Alt:* money-rails. *Why:* KISS; "integer
  cents, never floats" needs no gem; luxury prices fit int64/bigint.
- **Order state = plain `status` enum + guarded transition methods** (`mark_paid!` only from
  `pending_payment`, raise on illegal edge). *Alt:* AASM. *Why:* small state set; explicit and visible
  beats a gem. States: `pending_payment → paid → fulfilled | cancelled` (fulfilled/cancelled unused).
- **Orchestrate create → confirm → capture in the service, each with its OWN deterministic
  Idempotency-Key** (`co-<token>-create|confirm|capture`). *Alt:* one shared key (→ NemPay 422 on
  differing fingerprints); or auto-capture (M1 has none). *Why:* M1 requires an explicit capture; a key
  per (checkout, operation) makes retries and double-submit safe (AC6).
- **`paid` set ONLY by a verified `captured` webhook** — never from the capture `200` or a redirect.
  *Alt:* mark paid on the capture response. *Why:* constitution + the core lesson; webhook is the
  source of truth (AC1, AC3). The synchronous capture success is deliberately ignored for `paid`.
- **Double-submit idempotency via a per-checkout token.** The buy form embeds a `checkout_token` minted
  at render; a resubmit carries the same token → `Order.find_or_create_by(checkout_token:)` yields one
  order → the create Idempotency-Key derives from it → NemPay returns the same intent → one charge.
  *Alt:* dedupe on nothing (two clicks → two orders → two charges). *Why:* AC6.
- **Webhook dedup = `ProcessedWebhookEvent` with `UNIQUE(event_id)`, insert-first inside the order-
  update transaction;** duplicate insert → no-op, still `200`. *Alt:* check-then-act. *Why:* at-least-
  once delivery, insert-first mirrors NemPay's own discipline (AC4).
- **HMAC verify over the RAW request body**, constant-time compare to `X-NemPay-Signature`, reject
  `400` before parsing/trusting. *Alt:* parse then verify. *Why:* the signature is over exact bytes;
  read `request.raw_post` before JSON parsing (AC5).
- **Webhook registration = A1 dev-seed wiring** (shared secret in both configs). *Alt:* a `POST
  /v1/webhook_endpoints` API. *Why:* keeps NemPay's frozen M1 API surface intact; dev-only concern.
- **Test payment methods.** The buy form offers a fixed list — "Valid card" → `tok_ok`, "Declined
  card" → `tok_declined` (and "Bank timeout" → `tok_timeout` for the failure demo). The selected token
  is passed to `confirm`. **No card-number field exists anywhere** (AC7).

## Data model / API changes
**NemLuxury tables (SQLite):**
- `products` — `name`, `description`, `amount_cents` (integer), `currency` (char 3), timestamps. Seeded
  with a few luxury items.
- `orders` — `product_id` (FK), `amount_cents`, `currency`, `status` (default `pending_payment`),
  `checkout_token` (UNIQUE, the double-submit natural key), `nem_pay_intent_id` (nullable), timestamps.
- `processed_webhook_events` — `event_id` (UNIQUE), `event_type`, `order_id` (nullable), `created_at`.

**NemPay `/v1` API consumed (no change to NemPay's API):**
- `POST /v1/payment_intents` `{amount, currency, metadata:{order_id}}` + `Bearer <secret>` + `Idempotency-Key`.
- `POST /v1/payment_intents/:id/confirm` `{token}` + `Idempotency-Key`.
- `POST /v1/payment_intents/:id/capture` + `Idempotency-Key`.
- `GET /v1/payment_intents/:id` (status/debug).
- **Inbound webhook** `POST /webhooks/nem_pay` with `X-NemPay-Signature`, `X-NemPay-Event-Id`,
  `X-NemPay-Event-Type`; `payment_intent.captured` → order `paid`. Other event types acknowledged `200`,
  no-op in M2 (refunds deferred).

**NemPay dev-seed change (additive, dev-only):** insert one `webhook_endpoints` row for the dev
merchant from env; add `NEMPAY_DEV_WEBHOOK_URL`/`NEMPAY_DEV_WEBHOOK_SECRET` to compose `api`.

## Acceptance coverage (spec AC ↔ plan)
| Spec AC | Covered by |
|---|---|
| AC1 happy path e2e | Checkout orchestration + webhook handler (`paid` on captured) + dev-seed wiring |
| AC2 declined → stays pending_payment | Checkout: confirm=`failed` → no capture, surface error, order untouched |
| AC3 missing webhook → stays pending | `paid` only in the webhook handler; capture response never marks paid |
| AC4 duplicate webhook → once | `ProcessedWebhookEvent` UNIQUE(event_id), insert-first in the update tx |
| AC5 bad signature → rejected | `WebhookVerifier` over raw body; `400` before any order change |
| AC6 idempotent checkout | `checkout_token` → one order → deterministic create Idempotency-Key |
| AC7 PCI boundary | Test-method selector only; no PAN field/param; nothing card-shaped stored/logged |
| AC8 direct-only | `Client.create_intent` never sends `escrow`/`payee`; sends amount+currency+opaque metadata |

## Risks & rollback
- **Docker→host networking:** NemPay (in compose) must reach NemLuxury (on host) via
  `host.docker.internal`. *Mitigation:* configurable `NEMPAY_DEV_WEBHOOK_URL`; documented in both READMEs.
- **Raw-body verification:** Rails middleware/params parsing can consume the body. *Mitigation:* read
  `request.raw_post` in the controller before parsing; verify, then parse.
- **Shared-secret drift** between NemPay's seed and NemLuxury's ENV → every signature fails.
  *Mitigation:* one documented value both sides read; a clear log on verify failure.
- **Per-operation idempotency keys:** reusing one key across create/confirm/capture → NemPay `422`.
  *Mitigation:* distinct deterministic keys per operation.
- **Order marked paid twice** under duplicate/again webhook: prevented by the UNIQUE event row + the
  guarded `mark_paid!` (idempotent from `pending_payment`, no-op/ignore if already `paid`).
- **Rollback:** NemLuxury is a separate app — not running it leaves NemPay working. The only NemPay
  change is an additive, env-guarded dev-seed row (harmless when the env vars are unset).

## Tasks (execute in order)
- [x] [task-01 — Rails 8 scaffold + config](./task-01-rails-scaffold-config.md)
- [x] [task-02 — Domain: Product + Order + catalogue](./task-02-domain-catalogue-orders.md)
- [x] [task-03 — NemPay client + checkout orchestration](./task-03-nempay-client-checkout.md)
- [x] [task-04 — Webhook endpoint: verify + dedup + paid](./task-04-webhook-verify-dedup-paid.md)
- [x] [task-05 — NemPay dev-seed webhook wiring + networking](./task-05-nempay-devseed-webhook-wiring.md)
- [x] [task-06 — End-to-end + failure-path tests](./task-06-e2e-and-failure-paths.md)

**Dependency order:** 01 → 02 → 03 → 04 → 06; task-05 (NemPay side) is independent but blocks 06.
`task-05` can be done any time before the live e2e in `task-06`.

## Acceptance coverage (task ↔ spec AC)
| Task | Covers |
|---|---|
| task-01 | (foundation) |
| task-02 | AC7, AC8 (domain posture) |
| task-03 | AC1, AC2, AC6, AC8 (+ AC3/AC7 invariants at unit level) |
| task-04 | AC1, AC3, AC4, AC5 |
| task-05 | enables AC1/AC4/AC5 delivery |
| task-06 | AC1–AC8 end to end |
