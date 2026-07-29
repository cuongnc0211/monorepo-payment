# NemPay

A Stripe-style payment gateway, built from scratch for learning. This is the
**authoritative system for everything about money**. If a fact about money isn't in
NemPay's ledger, it isn't true.

## Structure

```
NemPay/
├── api/        Go service — the gateway itself
├── portal/     React app — merchant dashboard (consumes api/ over /v1) — DEFERRED
└── bank-sim/   fake acquirer/issuer for dev & tests (approved / declined / timeout)
```

This file governs all three. If `api/` and `portal/` grow large, split into
`api/CLAUDE.md` and `portal/CLAUDE.md` later; for now the split below is enough.

### Running NemPay (self-contained)
`NemPay/docker-compose.yml` is the whole gateway in one file: **PostgreSQL + Redis + the Go
api + bank-sim**, with migrations run automatically on boot. A single `docker-compose up`
inside `NemPay/` yields a working payment gateway with `/v1` listening on the host — no
other service required. The gateway must be **usable standalone** (exercised via `curl` or
the merchants); it never depends on a merchant being up. The two merchants are *not* part
of this compose — they are separate consumers that reach the api over HTTP. Keep it so.

---

## api/ — Go service

### Stack
- **Router:** Gin
- **DB:** PostgreSQL via **sqlc** (hand-written SQL → generated type-safe Go)
- **Migrations:** golang-migrate
- **Background jobs:** asynq (Redis) — webhook delivery, expiry sweeps, reconciliation.
  Redis stays here **on purpose**, even though the rest of the stack is minimal: a real
  gateway delivers webhooks out-of-band via a durable queue, and modelling that faithfully
  (outbox → queue → worker) is a core lesson. Do not "simplify" it into inline delivery.
- **Auth:** JWT for portal sessions; API keys (publishable + secret) for merchant calls

### Why sqlc, not GORM — and why sqlc *everywhere in NemPay*, not just the money path
Payment correctness lives in SQL you can *see*: `SELECT … FOR UPDATE`, explicit
transaction boundaries, the precise locking on capture. sqlc keeps that SQL explicit and
type-checks it against the schema at build time; an ORM generates SQL at runtime and hides
the exact thing we must be certain of. Concretely, GORM's defaults are *wrong for money*:
`db.First(&pi, id)` takes no lock (two concurrent captures both proceed → double-capture);
`Updates` silently drops zero-value fields (`Amount: 0`, a `false` flag → a write lost with
no error); soft-delete injects an invisible `WHERE deleted_at IS NULL` into every query
(reconciliation must `SUM` *all* entries); hooks fire hidden writes. In a ledger, code
review *is* the safety mechanism — you cannot review SQL you cannot see.

**Decision: NemPay uses sqlc for the whole service — the read/portal API too, not only the
money path.** Do not add a second ORM for "the simple CRUD parts". Reasons:
- The "sensitive vs not" line is fuzzy here: most of the read API *is* reading money
  (balances = `SUM(entries)` in `numeric`, transactions, refunds, payouts). Genuinely
  money-free CRUD (webhook-endpoint config, merchant profile) is a small remainder.
- A second ORM in the same process opens a **second write path into money tables** that
  bypasses the ledger + locking discipline — a boundary enforced only by "remember not to",
  which a money system must not rely on.
- Two data-access models = two sets of footguns (soft-delete, zero-value, hooks) living
  beside the carefully-locked code. It also blurs the lesson: the ORM boundary belongs
  **between systems** (NemPay = sqlc vs the merchants = ActiveRecord), never inside the
  money system.
- sqlc handles plain CRUD fine in Go. For the few genuinely dynamic list endpoints (optional
  filters, pagination), use `sqlc.narg` + `COALESCE`, or a query builder (squirrel) /
  `database/sql` for just those — **never GORM**. One driver, one mental model, zero magic.

Never introduce GORM into NemPay.

### Layering: handler → service → repository
- **handler/** — HTTP only. Parse and validate the request, call one service, render the
  response. No business logic, no SQL.
- **service/** — business logic, state-machine transitions, orchestration. **Owns the DB
  transaction boundary.** Idempotency handling and ledger writes live here, together, in
  one transaction.
- **repository/** — sqlc-generated queries plus thin wrappers. No business logic.

Rule of thumb: decides *what* happens → service. Decides *how to speak HTTP* → handler.
Decides *how to speak SQL* → repository.

### Core domain rules (non-negotiable)

**Idempotency.** Every POST that moves or reserves money requires an `Idempotency-Key`
header. Implementation is **insert-first**:
- `INSERT` into `idempotency_keys` with `UNIQUE(merchant_id, key)`, `status='in_flight'`.
- Insert succeeds → you are the first request: process, store the response, mark
  `completed`.
- `UniqueViolation` → someone arrived first: compare a request fingerprint (`422` on
  mismatch), return `409` if `in_flight`, or replay the stored response if `completed`.
- **Never** `SELECT`-then-`INSERT` (TOCTOU race — two concurrent requests both see
  "absent" and both proceed). Let the unique constraint arbitrate.
- Keys are scoped per merchant and expire (~24h). A sweeper must reset orphaned
  `in_flight` rows (a crash mid-request otherwise wedges every retry at `409`).

**Double-entry ledger (source of truth).** Append-only. Authoritative for all balances.
- Tables: `accounts`, `transactions`, `entries`. A transaction has ≥2 entries that sum
  to zero.
- **Never `UPDATE` a balance column.** Balance = `SUM(entries)` for an account. You may
  materialize a snapshot for performance, but entries remain the truth.
- Every money event writes one balanced transaction **inside the same DB tx** as the
  state change it represents.

**Payment state machine.** A payment intent runs in one of two **settlement modes**,
chosen at creation and immutable thereafter. Transition only along the edges for that
mode; reject anything else.

*Direct mode* (NemLuxury — merchant sells its own goods, money settles to the merchant):
```
created → requires_confirmation → authorized → captured → settled
created | requires_confirmation | authorized → failed   (declined authorize, or expiry sweep)
captured | settled → refunded | partially_refunded
```
*Escrow mode* (NemTasker — platform holds the customer's money for a third-party payee):
```
created → requires_confirmation → authorized → captured → held_in_escrow → released
authorized → failed
held_in_escrow → refunded
```
Which mode applies is decided by the create request (see **Escrow** below), never inferred
later. `held_in_escrow → disputed`, partial release, and auto-release-on-timeout are
**later lessons — deliberately out of scope for now.** Wire only the edges above.

**Escrow (a first-class settlement mode, not an afterthought).** When a payment intent is
created in escrow mode, NemPay holds the customer's captured money on behalf of a
third-party payee until the platform (NemTasker) explicitly releases it. Held money is a
**liability account** on NemPay's books ("funds held on behalf of"), never revenue.

- *Enabling it.* `POST /v1/payment_intents` accepts `{ "escrow": true, "payee": "<id>",
  "application_fee": <cents> }`. Absent `escrow`, the intent is direct mode. The mode and
  payee are immutable once set — a direct intent can never become an escrow one.
- *Releasing it.* Release is an **explicit, idempotent** call — `POST
  /v1/payment_intents/:id/release` — made by the platform only after the job is confirmed
  done. NemPay never auto-releases on a timer yet (that is a later lesson).
- *The ledger (the core of the lesson):*
  - Capture → `Dr platform-cash`, `Cr escrow-liability(intent)`.
  - Release → `Dr escrow-liability`, `Cr payable-to-payee`, `Cr platform-revenue(fee)`.
  - Refund  → `Dr escrow-liability`, `Cr refund-to-payer`.
  - Each is one balanced transaction written **inside the same DB tx** as the state change.
- *The invariant that must always hold:* the total balance of all `escrow-liability`
  accounts MUST equal the balance of the segregated bank account (the money NemPay is
  holding for others). Reconciliation proves this continuously — it is a compliance
  requirement, not a nicety. If the two ever diverge, that is a P0.

**Scope for now (core lifecycle first).** Implement exactly: capture-into-escrow, explicit
release (with a single flat `application_fee`), and full refund from escrow — each proven
by the invariant above. **Deferred to later lessons, do not build yet:** disputes /
chargebacks, partial release, partial refund from escrow, auto-release on timeout, payout
batching to real bank rails, and multi-currency escrow.

**Webhooks (outbox pattern).** On every state change:
- Write an event row to an `outbox` table **in the same tx** as the state change.
- A separate asynq worker delivers it: at-least-once, exponential backoff, dead-letter.
- Sign each payload with the merchant's webhook secret (HMAC-SHA256) in a header.
- Include a stable `event_id` so receivers can dedupe (delivery is at-least-once).

**Money handling.** `int64` minor units (cents). Never float. Always pair an amount with
an ISO-4217 currency. Luxury prices are large — beware overflow when SUMming aggregates;
compute reports in the DB's `numeric` type.

**The bank simulator.** All external money movement goes through `bank-sim/`, which can be
told to return approved / declined / timeout. Every failure path — especially the
authorize **timeout** ("did the bank receive it?") — must be exercised against it. Never
call a real processor.

### API conventions
- Versioned base path: `/v1`.
- Error shape: `{ "error": { "type", "code", "message", "param" } }`, consistent everywhere.
- `Idempotency-Key` required on all mutating POSTs.
- **Publishable key** (frontend/tokenization) vs **secret key** (server-to-server): a
  publishable key must never perform a secret-key action.
- Emit an OpenAPI spec (oapi-codegen). The portal's TypeScript client is generated from it.

### Commands (fill in as scaffolded)
- `make run` · `make migrate` · `make sqlc` · `make test`

---

## portal/ — React merchant dashboard

> **Status: DEFERRED.** Not built until the escrow core lifecycle works end to end. The two
> merchants + `curl` are enough to exercise escrow for now. The design below is the plan for
> when it *is* built — keep it, don't act on it yet.

### Why React, not Rails
The Go API is the single source of truth. A portal is a *consumer* of that API, not the
*owner* of a domain — so Rails would arrive with ActiveRecord and migrations holstered,
reduced to an HTTP client with no models of its own. React over the API is the honest
fit, matches reality (Stripe's dashboard is a JS SPA over the Stripe API), and makes the
portal dogfood the exact public API merchants use — which forces that API to be complete.

### Stack
- **Vite + React + TypeScript.** SPA behind auth; no SEO need, so no SSR required.
- **Data layer:** TanStack Query over a **generated** TS client
  (openapi-typescript / orval, from NemPay's OpenAPI spec). End-to-end types; no
  hand-written fetch glue.
- **Auth:** JWT from `api/`, held in memory with refresh — not `localStorage`.

### What it shows
Transactions, payment intents, refunds, balances / ledger view, payouts, API keys,
webhook endpoints and delivery logs. Everything read and written through `/v1` — no
private backdoor into the DB.

### Conventions
- Never hold raw card data in the portal. Card entry uses NemPay's tokenization SDK.
- Treat the generated client as the contract: regenerate it when the API changes; never
  hand-patch it.
- Keep components dumb about money math — the API returns computed amounts; the portal
  displays them.
