# Plan — Direct-capture payment gateway (M1)

**ID:** 001-direct-capture-gateway · **Status:** complete (all tasks done, 49 tests green)
**Spec:** ../../specs/001-direct-capture-gateway/spec.md
**Constitution:** ../../CLAUDE.md, ../../nem_pay/CLAUDE.md

## Decisions made during implementation (refinements to the HOW)
- **Idempotency response storage = `bytea`, not `jsonb`** (task-03): jsonb re-serialises (key order,
  whitespace) and broke byte-identical replay. Raw bytes preserve the exact original response.
- **State machine gained `created → failed` and `requires_confirmation → failed`** (task-05): a
  declined authorization and the expiry sweep need a pre-capture intent to reach `failed` without a
  semantically-wrong hop through `authorized`. The constitution drew only `authorized → failed`.
- **API-key lookup prefix widened to 16 chars** (task-04): an 8-char label (`sk_test_`) carried no
  entropy, so the "narrow by prefix" auth lookup scanned all secret keys. The prefix now reaches
  into the token's random part.
- **Partial refund is rejected before settlement** (task-05): a pre-settle partial refund would
  strand `acquirer_receivable` (settle only selects `captured`), diverging ledger from bank. Full
  pre-settle and any post-settle refund are unaffected.
- **Expiry sweep skips `authorized` intents** (task-05): failing an authorized intent without a bank
  void would leak the acquirer hold. Void is deferred, so only pre-authorization states expire.
- **Get-or-create account uses `ON CONFLICT DO UPDATE … RETURNING`** (task-05 fix to task-02): the
  DO NOTHING + UNION-ed SELECT returned zero rows under a concurrent first-create race.
- **`make test-db` uses `go test -p 1`**: DB-backed packages share one database and TRUNCATE shared
  tables, so parallel package execution let them wipe each other mid-test.
- **Webhook delivery: DB outbox is the single retry authority; asynq handler always returns nil**
  (task-06) and enqueues with per-row `TaskID`, so asynq neither double-retries nor archive-blocks
  re-dispatch.

## Deferred to later milestones / lessons (documented in code)
- Bank-sim capture is not idempotent (fake processor); a real gateway needs an idempotent capture.
- Single refund per intent (partially_refunded is terminal); cumulative partials deferred.
- No `payment_intent.created` event; no webhook void-on-expiry; at most one active endpoint per merchant.

## Approach (HOW)
Build the gateway bottom-up: runnable shell → ledger → idempotency → intent + state machine → money
flow (bank-sim) → outbox/webhooks. Each layer is one task with its own DDL and tests. **Direct mode
only**; every seam escrow needs is built but left unused.

## Architecture / components touched
- `NemPay/api` — Go/Gin, **sqlc** over PostgreSQL, golang-migrate, asynq worker.
- `NemPay/bank-sim` — fake acquirer (approved / declined / timeout).
- `NemPay/docker-compose.yml` — Postgres + Redis + api + bank-sim; migrations on boot.
- Layering: handler → service (owns the tx boundary) → repository (sqlc). See `nem_pay/CLAUDE.md`.

## Key decisions & alternatives considered
- **Ledger sign model = separate `debit`/`credit` columns** (not one signed column). Why: teaching
  clarity, no negative-balance ambiguity; `Σdebit = Σcredit` is an obvious invariant.
- **Settle = faithful model** with an `acquirer_receivable` account (capture ≠ cash in hand); settle
  converts receivable → `platform_cash`. **Settle ≠ payout** (payout deferred).
- **Settle trigger = periodic sweep**, not an endpoint. Why: models real async/batched settlement;
  the transition fn is directly callable for deterministic tests.
- **`status`/`settlement_mode` = text + CHECK**, not native enum. Why: M3 extends the allowed set by
  altering the CHECK, avoiding Postgres enum-migration limits.

## Escrow-adaptability guards (build now, leave unused — the reason M3 stays additive)
1. `payment_intents.settlement_mode` (`direct` now; `escrow` in M3) + **nullable** `payee_id`, `application_fee`.
2. **Typed** chart of accounts (asset/liability/revenue) + per-reference accounts → `escrow_liability(intent)` later.
3. Capture credit **destination decided by mode** (M1: `merchant_payable`; M3: `escrow_liability`).
4. State transitions in **one central allowed-edges map**.
5. Webhook `event_type` is a **free string** over a generic outbox.
6. Reconciliation compares **any account balance vs a bank balance** generically.

## Data model / API changes
Migrations: `0001` ledger · `0002` idempotency · `0003` intents · `0004` merchants/api_keys ·
`0005` outbox/webhooks. API: `/v1/payment_intents` (+ confirm/capture/refund) and reads. Per-task DDL
in the task files.
- **Webhook signing contract** (the spec keeps this behaviour-level; the mechanism lives here): each
  delivery is signed **HMAC-SHA256** over the body with the merchant's webhook secret, in an
  `X-NemPay-Signature` header, and carries a stable `event_id` for dedupe. — satisfies AC5.
- **Refund**: full or partial; posts reversing entries (task-05) and sets `refunded` /
  `partially_refunded`; refunding more than captured is rejected. — satisfies AC7.
- **Auth**: publishable vs secret API keys; secret required for every money-mutating POST;
  publishable never performs a secret action. — satisfies AC8.

## Risks & rollback
- **Plane collapse** (emitting the webhook inside the money tx) — forbidden; outbox only (task 06).
- **Guard erosion** — dropping the unused seams turns M3 into a rewrite. Keep them.
- Ledger is append-only — never delete to undo; post reversing entries.

## Acceptance coverage (spec ↔ task traceability)
| Spec AC | Covered by |
|---|---|
| AC1 compose up / gateway healthy | task-01 |
| AC2 lifecycle + derived balances | task-02, task-04, task-05 |
| AC3 idempotent capture (no double-charge) | task-03, task-05 |
| AC4 declined / timeout handling | task-05 |
| AC5 signed webhook + stable event_id | task-06 |
| AC6 balanced transactions / derived balances | task-02 |
| AC7 full & partial refund | task-05 |
| AC8 publishable vs secret keys | task-04 |

Each escrow-adaptability guard also ships a **structural test**, so "escrow-ready" is *evidenced* in
M1, not merely promised: guard 1 → `settlement_mode` column exists (task-04); guard 2 → `accounts.type`
present (task-02); guard 4 → transitions live in one central map (task-04).

## Tasks (execute in order)
- [x] [task-01 — Scaffolding + compose](./task-01-scaffolding-compose.md)
- [x] [task-02 — Ledger core](./task-02-ledger-core.md)
- [x] [task-03 — Idempotency](./task-03-idempotency.md)
- [x] [task-04 — Payment intent, state machine, auth & API](./task-04-payment-intent-and-api.md)
- [x] [task-05 — Money flow + bank-sim](./task-05-money-flow-bank-sim.md)
- [x] [task-06 — Outbox + webhooks](./task-06-outbox-webhooks.md)
