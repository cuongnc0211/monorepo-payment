# Spec — Direct-capture payment gateway (M1)

**ID:** 001-direct-capture-gateway · **Status:** active · **Author:** cuong.nguyen · **Date:** 2026-07-17
**Constitution:** ../../CLAUDE.md, ../../nem_pay/CLAUDE.md
**Plan:** ../../plans/001-direct-capture-gateway/plan.md

## Why
NemPay must exist as a runnable, self-contained gateway before any merchant can integrate and
before escrow can be added. **Direct capture** (a merchant sells its own goods; money settles to
the merchant) is the simplest complete money flow and the foundation every later capability extends.

## What — scope
**In scope**
- A gateway that runs standalone via `docker-compose up` inside `NemPay/`, with `/v1` listening.
- A *direct-mode* payment-intent lifecycle: create → authorize → capture → settle, plus refund.
- Double-entry ledger as the source of truth; balances **derived**, never stored.
- Idempotency on every money-mutating POST.
- Signed webhooks delivered **out-of-band** — decoupled from the money transaction, so a merchant
  outage never blocks or rolls back a money movement.
- Bank interactions via **bank-sim** (approved / declined / timeout).

**Out of scope / non-goals**
- Escrow / marketplace mode → feature `003`.
- Merchant apps → features `002` (NemLuxury), `004` (NemTasker); React portal → deferred.
- Payout to the merchant, disputes/chargebacks, multi-currency aggregation.

## Behaviours
- As a merchant server, when I POST a payment intent with a secret key + `Idempotency-Key`, then a
  direct intent is created and I can confirm → capture it.
- As the gateway, when a state change happens, then a signed webhook for it is delivered at-least-once.
- As an operator, when the acquirer settles captured funds, then the intent moves to `settled`
  **asynchronously on its own schedule**, not in response to a client call.

## Acceptance criteria (definition of done)
- [x] **AC1** `docker-compose up` in `NemPay/` → gateway healthy, migrations applied, `/v1` reachable,
  with **no merchant present**.
- [x] **AC2** `curl` drives a payment intent create → authorize → capture; the intent then reaches
  `settled` **on its own, without any client call**. Balances are **derived from the ledger entries**
  (never stored) and are correct at each step.
- [x] **AC3** Retrying a capture with the same `Idempotency-Key` does **not** double-charge — exactly
  one balanced ledger transaction results.
- [x] **AC4** A **declined** authorize posts **no** ledger entries and fails the intent; an authorize
  **timeout** leaves the intent in a safe, reconcilable state with **no partial posting**.
- [x] **AC5** Each state change delivers a webhook the receiver can **verify as authentic**, at-least-once;
  redelivery carries the same stable `event_id` so the receiver can dedupe.
- [x] **AC6** Every ledger transaction is **balanced** (its entries net to zero); a balance is only ever
  a sum over entries, never a stored column.
- [x] **AC7** A **full or partial refund** of a captured intent posts the correct reversing entries and
  moves the intent to `refunded` / `partially_refunded`; refunding more than captured is rejected.
- [x] **AC8** A **publishable** key can never perform a secret-key (money-mutating) action; secret keys
  are required for those and are never exposed to the browser.

## Constraints
- Money = `int64` minor units + ISO-4217 currency; never float.
- NemPay money path must be **concurrency-safe** (a capture is never double-applied under concurrent
  requests) and its correctness must be **reviewable in the SQL**. (The mechanism — sqlc + explicit
  `SELECT … FOR UPDATE`, never GORM — is fixed by the constitution.)
- **Escrow-adaptability**: the design MUST let escrow (feature `003`) be added as an *extension, not a
  rewrite*. The concrete seams are specified in the plan; this spec only requires the constraint hold.
- Never call a real processor — all external money movement goes through bank-sim.

## Open questions
- _(resolved)_ Ledger representation, settle trigger, and settle accounting model are all decided —
  these are HOW, so they live in the plan's Key decisions, not here.
