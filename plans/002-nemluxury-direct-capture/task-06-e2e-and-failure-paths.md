# Task 06 — End-to-end + failure-path tests

**Plan:** ./plan.md · **Depends on:** task-03, task-04, task-05 · **Blocks:** none (completes M2)

## Context
- Spec acceptance criteria covered: **AC1–AC8** exercised together, including the async webhook round-trip.
- Links: [`../../Nem_luxury/CLAUDE.md`](../../Nem_luxury/CLAUDE.md) — "Testing" (exercise the failure paths
  deliberately); [`../../specs/002-nemluxury-direct-capture/spec.md`](../../specs/002-nemluxury-direct-capture/spec.md).

## Requirements
- **Request/integration specs** (WebMock-stubbed NemPay) that tie the checkout + webhook halves together
  deterministically — the bulk of the AC coverage, fast and CI-friendly.
- One **live end-to-end** spec (tagged, run manually) against the **real local NemPay + bank-sim**: start
  the gateway (`docker-compose up` in `NemPay/` with the dev webhook endpoint pointing at the test app),
  drive a real checkout, and **poll the order** until the async `captured` webhook flips it to `paid`.
- Every failure path from the constitution + spec is asserted.

## Files to create / modify
```
Nem_luxury/spec/requests/checkout_spec.rb            (happy, declined, double-submit)
Nem_luxury/spec/requests/webhooks/nem_pay_spec.rb    (captured, duplicate, bad-sig, missing)
Nem_luxury/spec/system/e2e_purchase_spec.rb          (tagged :e2e — live NemPay + bank-sim)
Nem_luxury/spec/support/nem_pay_stubs.rb, webhook_helpers.rb
Nem_luxury/README.md                                 (how to run unit vs :e2e)
```

## Validation / tests (matrix — the definition of done)
| Scenario | Expected | AC |
|---|---|---|
| happy purchase (valid method) | intent created→confirmed→captured; after a **verified captured webhook** the order is `paid` | AC1 |
| declined card | order stays `pending_payment`; customer shown failure; no capture | AC2 |
| missing webhook | capture happened but **no** webhook delivered → order **stays `pending_payment`** (never paid from redirect) | AC3 |
| duplicate webhook | same `event_id` twice → order `paid` **once**; 2nd is `200` no-op | AC4 |
| bad signature | tampered/wrong-secret webhook → `400`; order unchanged | AC5 |
| double-submit checkout | same `checkout_token` twice → one order, one charge | AC6 |
| PCI boundary | no card-number field/param anywhere; only opaque token | AC7 |
| direct-only | outbound create intent has no `escrow`/`payee`; only amount+currency+metadata | AC8 |
| **live e2e** (`:e2e`) | against real NemPay+bank-sim: `tok_ok` → poll → `paid`; `tok_declined` → stays pending | AC1, AC2 |

## Implementation steps
1. Shared stubs/helpers: WebMock stubs for NemPay create/confirm/capture; a helper that POSTs a
   correctly-signed (and a tampered) webhook to the app.
2. Request specs for the deterministic scenarios above.
3. `:e2e` system spec: assume the stack is up + the dev webhook endpoint targets the test app; perform a
   real checkout; poll `orders#show` (bounded timeout) until `paid`; assert the declined path stays pending.
4. README: run `bundle exec rspec` for unit; `bundle exec rspec --tag e2e` with NemPay up for live.

## Risks & rollback
- **Async timing** in the live e2e → poll with a bounded timeout, not a fixed sleep; keep it tagged/manual
  so CI isn't flaky.
- **Environment coupling** (needs the running stack) → the deterministic request specs carry the real AC
  coverage; `:e2e` is confirmation.
- Rollback: specs are additive; removing them doesn't change app behaviour.
