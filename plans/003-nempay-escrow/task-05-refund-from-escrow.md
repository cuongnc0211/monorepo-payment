# Task 05 — Full refund from escrow

**Plan:** ./plan.md · **Depends on:** task-03 · **Blocks:** task-07

## Context
- Spec acceptance criteria covered: **AC5** (full refund from held; reject after release).
- Links: M1 `Refund` (task-05/001); escrow refund posting in
  [`../../nem_pay/CLAUDE.md`](../../nem_pay/CLAUDE.md).

## Requirements
- Extend the existing `POST /v1/payment_intents/:id/refund` to be **mode-aware**. For an **escrow**
  intent, only a **full** refund is allowed and only from `held_in_escrow`: it posts
  `Dr escrow_liability A / Cr segregated_cash A` (full amount), moves the intent to `refunded`
  (terminal), and emits a refund webhook. Ledger-only (no bank call), as in M1.
- A refund of a **`released`** escrow intent (or any non-`held_in_escrow` state) is **rejected** with
  no posting. Direct-mode refund (partial/full) is unchanged.

## Files to create / modify
```
nem_pay/api/internal/service/money.go              (Refund: escrow branch — full, from held_in_escrow)
nem_pay/api/internal/httpapi/payment_intents.go    (refund handler stays; service decides by mode)
```

## Implementation steps
1. In `Refund`, branch on `pi.SettlementMode`. Escrow: require `pi.Status == held_in_escrow` and
   `amount == pi.Amount` (full); assert `CanTransition(escrow, held_in_escrow, refunded)`; post
   `Dr escrow_liability / Cr segregated_cash`; set `refunded`; `emit`.
2. Reject partial escrow refund (`ErrPartialRefundUnsupported` or the existing partial-refund guard)
   and refund-after-release (`ErrInvalidState`).

## Validation / tests
- Refund a held escrow intent (full) → `refunded`; `escrow_liability=0`, `segregated_cash=0`; balanced
  posting; one refund outbox row. (**AC5**)
- Refund a `released` intent → 409/no posting; partial escrow refund → rejected. (**AC5**, **AC7**)
- Direct refund (partial + full) unchanged. (**AC8**)

## Risks & rollback
- **Refund vs release race** on the same held intent → `FOR UPDATE` + state machine (whichever commits
  first wins; the other sees the new terminal state and is rejected). Rollback: additive.
