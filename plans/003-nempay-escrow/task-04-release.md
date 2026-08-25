# Task 04 — Release to payee minus fee

**Plan:** ./plan.md · **Depends on:** task-03 · **Blocks:** task-06, task-07

## Context
- Spec acceptance criteria covered: **AC3** (release minus fee), **AC4** (idempotent & concurrency-safe).
- Links: escrow release in [`../../nem_pay/CLAUDE.md`](../../nem_pay/CLAUDE.md); M1 idempotency wrapper
  (task-03/001) + `LockIntentForUpdate` (task-05/001).

## Requirements
- New endpoint `POST /v1/payment_intents/:id/release` — **secret key + `Idempotency-Key`**, wrapped in
  the M1 idempotency middleware, `SELECT … FOR UPDATE` on the intent.
- Only a `held_in_escrow` intent may be released. Release posts **one balanced** transaction
  (**Option A** — all cash leaves segregation):
  `Dr escrow_liability A · Dr platform_cash A / Cr segregated_cash A · Cr payable_to_payee(payee) (A−fee)
  · Cr platform_revenue fee`, then moves the intent to `released` (terminal).
- `payable_to_payee` is a per-reference liability keyed by `payee_id`; `platform_revenue` is a revenue
  singleton. Emit the **`escrow.released`** webhook event (add the explicit `released → escrow.released`
  case to `eventTypeFor`).
- Release is **not** a bank call — it moves money between NemPay's own accounts (payout deferred).

## Files to create / modify
```
nem_pay/api/internal/service/money.go              (Release: lock → assert → 5-leg post → status → emit)
nem_pay/api/internal/httpapi/payment_intents.go    (release handler + error mapping)
nem_pay/api/internal/httpapi/router.go             (POST /:id/release, secretOnly + WithIdempotency)
```

## Implementation steps
1. `Release(ctx, id, merchantID)`: `dbCtx := WithoutCancel`; tx → `LockIntentForUpdate` → assert
   `CanTransition(escrow, held_in_escrow, released)`; get/create the four accounts; `PostTransaction`
   the 5-leg posting; `UpdateIntentStatus(released)`; `emit`; commit.
2. Handler maps `ErrInvalidState` → 409, success → 200 intent; wrap route in `secretOnly` +
   `WithIdempotency`.
3. Add the `released` case to `eventTypeFor`.

## Validation / tests
- Release a held intent → `released`; balances: `escrow_liability=0`, `segregated_cash=0`,
  `platform_cash += A`, `payable_to_payee(payee) = -(A−fee)`, `platform_revenue = -fee`; the release
  transaction sums to zero. (**AC3**)
- Fee edges: `fee==amount` → payee accrues 0; `fee==0` → no revenue.
- Retried release (same Idempotency-Key) → replayed, one posting; N concurrent releases → exactly one
  posting; releasing an already-`released` intent → 409, no posting. (**AC4**)
- One `escrow.released` outbox row.

## Risks & rollback
- **5-leg sign errors** skew balances silently → rely on the sum-zero property + golden-balance tests.
- **Double-release / release-vs-refund race** → `FOR UPDATE` + state machine. Rollback: remove the
  route; ledger append-only.
