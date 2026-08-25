# Spec — NemLuxury: direct-capture integration with NemPay (M2)

**ID:** 002-nemluxury-direct-capture · **Status:** done · **Author:** cuong.nguyen · **Date:** 2026-08-25
**Constitution:** ../../CLAUDE.md, ../../Nem_luxury/CLAUDE.md, ../../nem_pay/CLAUDE.md
**Plan:** ../../plans/002-nemluxury-direct-capture/plan.md

> This file captures **WHAT** and **WHY** only. No HOW (no tech choices, schema, or code) — that
> lives in the plan. Keep it stable; it is the contract.

## Why
NemPay (M1) is a working gateway but has no consumer, so nothing yet proves it is *integrable* by a
real merchant. NemLuxury is that proof: the **direct-capture** reference integration — a store that
sells its *own* goods, so money settles straight to it with no third party and no escrow. Building it
exercises the whole direct money flow end to end (create → pay → order marked paid by a verified
webhook) and, crucially, the **failure modes** that justify NemPay's idempotency and webhook design:
declined cards, missing webhooks, duplicate webhooks, and double-submitted checkouts. It is the first
of two contrasting integrations; its escrow counterpart is NemTasker (M4).

## What — scope
**In scope**
- A minimal but real Rails storefront: a small luxury catalogue and a **single-item "buy now"**
  checkout that produces an order. The store owns products and orders; it owns nothing about money.
- An **order lifecycle** whose `paid` transition is caused **only** by a verified NemPay
  `payment_intent.captured` webhook — never by a browser redirect or NemLuxury's own decision. A
  **declined** payment leaves the order in `pending_payment` (the customer can retry). The order also
  carries `fulfilled` and `cancelled` terminal states for realism, with **no** workflow driving them
  in M2 (no fulfilment or cancellation process is built).
- **Server-to-server payment creation**: on checkout, NemLuxury creates a *direct* NemPay payment
  intent for the order amount (integer cents) + currency, with an order reference in metadata and a
  per-checkout **`Idempotency-Key`** reused across retries, using the **secret** API key.
- **Payment-method selection without a PAN**: the customer picks a **test payment method** that maps
  to a bank-sim magic token (a valid card, a declined card). NemLuxury forwards the opaque token to
  NemPay to complete the payment; no card number (PAN) is ever entered, stored, or logged.
- **One inbound webhook endpoint** that: verifies the HMAC signature against the shared secret,
  dedupes on NemPay's `event_id`, updates the order, and responds — all **synchronously**, no
  background jobs. `payment_intent.captured` → order `paid`.
- Runs **independently** of NemPay's compose, reaching the gateway over HTTP via a configured
  `NEMPAY_API_URL` + API keys (held in Rails credentials / ENV, never in the repo).

**Out of scope / non-goals**
- **Escrow** — NemLuxury sells its own goods and always creates direct intents. Escrow is NemTasker
  (M4). Reaching for it here means the domain model is wrong.
- **Refunds / partial refunds** — deferred to a later pass (order `refunded` state and refund-webhook
  handling are not built in M2).
- **Multi-item cart** — checkout is single-item "buy now" this round.
- **A real card-tokenization SDK / real PAN entry** — deferred with NemPay's portal/SDK; M2 uses test
  payment methods (magic tokens) as the payment method.
- **Fulfilment workflow** (shipping, inventory) — an order may have a post-`paid` terminal state, but
  no fulfilment process is built.
- **Balances, payouts, settlement, ledger** — all NemPay's domain; NemLuxury never computes them.
- **Async/background webhook processing** — handled synchronously in-controller by design.

## Behaviours / user stories
- As a customer, when I buy a product with a **valid** test payment method, then my order becomes
  `paid` **only after** NemLuxury receives a verified `payment_intent.captured` webhook for it.
- As a customer, when my payment is **declined**, then no order is ever marked `paid`, the order
  stays `pending_payment` so I can retry, and I am told the payment did not go through.
- As a customer, when I **double-submit** the same checkout, then I am **not charged twice** — the
  reused `Idempotency-Key` yields a single payment.
- As NemLuxury, when the **same webhook** is delivered more than once (same `event_id`), then the
  order transitions **exactly once**; later duplicates are acknowledged (`200`) as no-ops.
- As NemLuxury, when a webhook's **signature does not verify**, then I reject it and change no order.
- As NemLuxury, when the captured webhook **never arrives**, then the order stays `pending_payment` —
  it is never marked `paid` on a client redirect/confirmation alone.

## Acceptance criteria (testable — the definition of done)
- [x] **AC1 — Happy path (end to end).** Buying a catalogue item with a valid test payment method
  creates a **direct** payment intent (secret key + `Idempotency-Key`), drives it through to
  **captured** at NemPay, and the resulting **verified** `payment_intent.captured` webhook marks the
  order `paid`. Verified against the local NemPay + bank-sim, not a stub.
- [x] **AC2 — Declined card.** A declined test payment method results in **no** `paid` order; the
  order **stays `pending_payment`** (retryable) and the customer is shown a failure. No captured
  webhook occurs.
- [x] **AC3 — Missing webhook.** If the `captured` webhook is not delivered, the order remains
  `pending_payment`. It is **never** marked `paid` from the browser redirect/confirmation alone.
- [x] **AC4 — Duplicate webhook.** The same event (same `event_id`) delivered twice transitions the
  order **once**; the second delivery is a no-op returning `200`.
- [x] **AC5 — Signature verification.** A webhook whose HMAC signature does not match the shared
  secret is **rejected** (non-2xx) and changes no order state.
- [x] **AC6 — Idempotent checkout.** Double-submitting the same checkout (same reused
  `Idempotency-Key`) results in **exactly one** payment / one charge — the order is not double-charged.
- [x] **AC7 — PCI boundary.** No card PAN is ever accepted, stored, or logged by NemLuxury; only
  opaque payment-method tokens and NemPay identifiers are handled. (No card-number input exists.)
- [x] **AC8 — Boundary integrity (direct only).** NemLuxury never creates an escrow intent (no
  `escrow` flag / `payee`), never computes balances, and sends NemPay only an amount, a currency, and
  opaque metadata — never domain meaning (a "yacht" vs a "watch").

## Constraints (from the constitution or product)
- Money is **integer cents + currency**, never floats. Luxury prices are large.
- The **secret** API key is server-side only (Rails credentials / ENV); never in the repo or browser.
- Webhooks are handled **synchronously** then answered `200`; delivery is **at-least-once**, so the
  handler must be **idempotent** (dedupe on `event_id`) and verify the **HMAC signature** first.
- The order's `paid` transition is caused **solely** by a verified `captured` webhook.
- NemLuxury talks to NemPay **only** over the public `/v1` HTTP API — no shared database, no direct
  cross-boundary DB reads. It keeps its own file-based store.
- No background-job infrastructure (no Redis/Sidekiq) — synchronous by design for a dummy merchant.

## Open questions
- _(resolved)_ **Declined-payment order state** → the order **stays `pending_payment`** (retryable);
  it never advances to `paid`.
- _(resolved)_ **Post-`paid` terminal states** → keep `fulfilled` and `cancelled` on the order model
  for realism, with **no** workflow driving them in M2.
