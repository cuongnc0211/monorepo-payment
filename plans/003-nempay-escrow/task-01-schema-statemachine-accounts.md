# Task 01 — Migration + mode-aware state machine + escrow account kinds

**Plan:** ./plan.md · **Depends on:** none · **Blocks:** task-02…07

## Context
- Spec acceptance criteria covered: **AC7** (state-machine integrity), **AC8** (direct edges
  unchanged) — foundation every later escrow task builds on.
- Links: [`../../nem_pay/CLAUDE.md`](../../nem_pay/CLAUDE.md) — escrow section; M1 guards #2 (typed +
  per-reference accounts) and #4 (one central allowed-edges map).

## Requirements
- **Migration `0006_escrow`** (additive): widen `payment_intents.status` CHECK to add
  `held_in_escrow`, `released`; widen `settlement_mode` CHECK to add `escrow`. No new columns
  (`payee_id`, `application_fee` already exist).
- **Mode-aware state machine**: `allowedEdges` keyed by settlement mode. Direct edges are **exactly**
  M1's; escrow edges are `created → requires_confirmation → authorized → captured → held_in_escrow →
  released`, `held_in_escrow → refunded`, and the pre-capture `→ failed` edges. Signature becomes
  `CanTransition(mode, from, to)`; add `StatusHeldInEscrow`, `StatusReleased`, `SettlementEscrow`.
- **Escrow ledger account-kind constants** (strings only — no schema): `KindSegregatedCash` (asset),
  `KindEscrowLiability` (liability), `KindPayableToPayee` (liability), `KindPlatformRevenue` (revenue).

## Files to create / modify
```
nem_pay/api/db/migrations/0006_escrow.up.sql / .down.sql
nem_pay/api/internal/statemachine/intent.go        (+ intent_test.go)
nem_pay/api/internal/ledger/accounts.go            (new kind constants)
nem_pay/api/internal/service/money.go              (update CanTransition callers to pass pi.SettlementMode)
```

## Implementation steps
1. Write `0006` widening both CHECK constraints (`ALTER TABLE … DROP CONSTRAINT … ADD CONSTRAINT …`).
2. Refactor `allowedEdges` → `map[mode]map[from][]to`; update `CanTransition` to take `mode`. Keep the
   `direct` edge set byte-identical to M1.
3. Add the status/mode constants and the escrow account-kind constants.
4. Update every existing `CanTransition` caller in `money.go` to pass `pi.SettlementMode` (mechanical;
   direct behaviour unchanged).

## Validation / tests
- Escrow edges: `authorized→captured→held_in_escrow→released` and `held_in_escrow→refunded` legal;
  illegal (e.g. `captured→settled` in escrow, `held_in_escrow→released` in direct) rejected. (**AC7**)
- Direct edges unchanged — the M1 statemachine tests still pass verbatim. (**AC8**)
- `make sqlc` unaffected; migration applies and the CHECKs accept the new values.

## Risks & rollback
- **Signature change touches M1 callers** → direct-mode regression risk. Mitigation: direct edge set
  identical; run the full money suite. Rollback: narrow the CHECKs (safe only before escrow rows exist).
