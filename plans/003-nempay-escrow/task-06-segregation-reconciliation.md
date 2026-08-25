# Task 06 — Segregation reconciliation invariant + constitution refinement

**Plan:** ./plan.md · **Depends on:** task-04, task-05 · **Blocks:** task-07

## Context
- Spec acceptance criteria covered: **AC6** (segregation invariant).
- Links: M1 guard #6 (reconcile any account balance vs a bank balance generically); the escrow
  invariant in [`../../nem_pay/CLAUDE.md`](../../nem_pay/CLAUDE.md).

## Requirements
- A **reconciliation** helper/query that proves the segregation invariant from the ledger: the summed
  balance of all `escrow_liability` accounts for intents currently in `held_in_escrow` **equals** the
  `segregated_cash` balance. Held money never appears as `platform_revenue` before release.
- **Refine `nem_pay/CLAUDE.md`**: change the escrow capture posting wording from `Dr platform-cash` to
  the segregated model actually implemented (`Dr acquirer_receivable` at capture; `Dr segregated_cash`
  at settle), so the constitution matches the code (the conscious refinement the spec called out).

## Files to create / modify
```
nem_pay/api/internal/ledger/… or internal/reconcile/…   (segregation check helper + query)
nem_pay/api/db/queries/…                                 (sum escrow_liability(held) and segregated_cash)
nem_pay/CLAUDE.md                                        (escrow ledger wording refinement)
```

## Implementation steps
1. Add a query/helper that returns `Σ escrow_liability` (held intents) and `segregated_cash`, computed
   in `numeric`, and asserts equality.
2. Update `nem_pay/CLAUDE.md`'s escrow ledger bullet to the segregated model and note the transit
   window (a captured-not-settled liability is backed by the acquirer receivable).

## Validation / tests
- After a mix of escrow operations (capture, settle, release, refund across several intents), the
  invariant holds: `Σ escrow_liability(held) == segregated_cash`; released fees are in
  `platform_revenue` only after release. (**AC6**)
- Property check: no `escrow_liability` remains for a `released`/`refunded` intent, and its cash has
  left `segregated_cash`.

## Risks & rollback
- **Scoping the sum** to held intents (not released/refunded) is essential — a naïve `Σ all
  escrow_liability` already nets to zero for closed intents, so scope by state or by non-zero balance.
  Rollback: the helper is additive; the CLAUDE.md edit is documentation.
