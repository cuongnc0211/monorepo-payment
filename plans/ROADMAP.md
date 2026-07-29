# Roadmap — monorepo-payment (program overview)

**Status:** active · **Updated:** 2026-08-25

Build order is a **vertical slice**: get one money flow (direct capture) working end to end first,
then extend the *same* gateway to escrow. Escrow is designed for from day one but built last.

| # | Milestone | Spec | Plan |
|---|---|---|---|
| M1 ✅ | NemPay — direct-capture gateway (escrow-ready) — **done** (all 8 ACs verified, 49 tests + docker e2e) | [spec](../specs/001-direct-capture-gateway/spec.md) | [plan](./001-direct-capture-gateway/plan.md) |
| M2 ✅ | NemLuxury — direct-capture integration — **done** (8 ACs; 37 specs + live e2e) | [spec](../specs/002-nemluxury-direct-capture/spec.md) | [plan](./002-nemluxury-direct-capture/plan.md) |
| M3 | NemPay — escrow mode | [spec](../specs/003-nempay-escrow/spec.md) | [plan](./003-nempay-escrow/plan.md) |
| M4 | NemTasker — escrow integration | _tbd_ | _tbd_ |

**Dependencies:** M1 → M2, M1 → M3, M3 → M4. (M2 and M3 both depend only on M1, so they *could* run
in parallel; we do them sequentially to keep cognitive load low.)

Each milestone becomes its own `specs/<NNN>-<slug>/` + `plans/<NNN>-<slug>/` when we reach it, via
`/sdd:spec` → `/sdd:plan` → `/sdd:tasks`. Until then, M2–M4 summaries live here:

- **M2 — NemLuxury (direct-capture integration).** Rails + SQLite store, no background jobs. Checkout
  creates an intent (secret key + `Idempotency-Key`); card is tokenized client-side; a verified
  `payment_intent.captured` webhook — handled **synchronously** — marks the order paid. Acceptance:
  buy a supercar end to end; declined card; missing webhook keeps `pending_payment`; duplicate webhook
  deduped; double-submit doesn't double-charge.
- **M3 — Escrow mode (NemPay).** `settlement_mode='escrow'`; `escrow_liability(intent)`;
  `held_in_escrow`; explicit idempotent `release` with a flat `application_fee`; refund from escrow;
  reconciliation invariant `Σ escrow-liability == segregated funds`. **Core lifecycle only** —
  disputes, partial release/refund, auto-release-timeout, payout batching, multi-currency deferred.
- **M4 — NemTasker (escrow integration).** Rails + SQLite marketplace. Fund-on-offer-accept →
  release-on-completion → refund-on-cancel; synchronous webhooks. Acceptance: full journey (post →
  accept → fund → complete → release, tasker paid minus fee → reconcile); failure paths incl.
  refund-after-release rejected.

## Cross-cutting (matures across milestones)
- **Reconciliation** starts in M1 (ledger vs bank) and deepens in M3 (segregation invariant).
- **Testing** — every milestone exercises its failure paths against bank-sim; never a real processor.
- **Docs** — update the relevant `CLAUDE.md` only when behaviour/contracts actually change.
