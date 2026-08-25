# Task 03 — Settle into segregation (held_in_escrow)

**Plan:** ./plan.md · **Depends on:** task-02 · **Blocks:** task-04, task-05, task-06

## Context
- Spec acceptance criteria covered: **AC2b** (settle → `held_in_escrow` into segregated cash).
- Links: M1 `SettleDueIntents` sweep (task-05 of feature 001); guard #3 (destination by mode).

## Requirements
- Extend the existing **settle sweep** so a `captured` **escrow** intent settles into a **segregated
  escrow-cash** account and becomes `held_in_escrow`, while a `captured` **direct** intent still
  settles into `platform_cash` and becomes `settled`. Destination + target status chosen by mode.
- Escrow settle posts `Dr segregated_cash / Cr acquirer_receivable` (full amount), balanced, in the
  intent's tx.
- Emit the **`payment_intent.held_in_escrow`** webhook event on this transition (add the explicit
  `held_in_escrow → payment_intent.held_in_escrow` case to `eventTypeFor`).

## Files to create / modify
```
nem_pay/api/internal/service/money.go     (settleOne: destination + target status by mode; emit)
nem_pay/api/db/queries/payment_intents.sql (ListCapturedBefore already selects captured regardless of mode)
```

## Implementation steps
1. In `settleOne`, branch on `pi.SettlementMode`: direct → `Dr platform_cash / Cr acquirer_receivable`,
   target `settled`; escrow → `Dr segregated_cash / Cr acquirer_receivable`, target `held_in_escrow`.
2. Assert `CanTransition(mode, captured, target)`; post; update status; `emit`.
3. Add the `held_in_escrow` case to `eventTypeFor`.

## Validation / tests
- Escrow capture then force-run the sweep → `held_in_escrow`; `segregated_cash = amount`,
  `acquirer_receivable = 0`, `escrow_liability(intent) = amount` still standing. (**AC2b**)
- Sweep selectivity: a `held_in_escrow` intent is not re-settled; direct `captured` intents still
  settle to `platform_cash`/`settled`. (**AC8**)
- Exactly one `payment_intent.held_in_escrow` outbox row written for the transition.

## Risks & rollback
- **Invariant timing**: after settle, `escrow_liability(intent) == segregated_cash` for that intent;
  before settle it is backed by the receivable (documented, not a bug). Rollback: additive on task-02.
