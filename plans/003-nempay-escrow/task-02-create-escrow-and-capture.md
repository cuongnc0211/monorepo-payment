# Task 02 — Create escrow intent + mode-aware capture

**Plan:** ./plan.md · **Depends on:** task-01 · **Blocks:** task-03…07

## Context
- Spec acceptance criteria covered: **AC1** (create escrow + validate + immutable), **AC2** (capture →
  `captured` with an escrow liability).
- Links: escrow API in [`../../nem_pay/CLAUDE.md`](../../nem_pay/CLAUDE.md); M1 guard #1
  (`settlement_mode`/`payee_id`/`application_fee`) and guard #3 (capture destination by mode).

## Requirements
- `POST /v1/payment_intents` accepts `{ "escrow": true, "payee": "<uuid>", "application_fee": <cents> }`.
  When `escrow` is true: `payee` is required and a valid UUID; `application_fee` satisfies
  `0 ≤ fee ≤ amount`. Persist `settlement_mode='escrow'`, `payee_id`, `application_fee`. Absent
  `escrow`, behaviour is exactly direct. Mode/payee/fee are set once and never mutated.
- **Capture (mode-aware)**: escrow capture posts `Dr acquirer_receivable / Cr escrow_liability(intent)`
  (a per-reference liability keyed by the intent id) and moves the intent to `captured`. Direct capture
  is unchanged (`Cr merchant_payable`). The held amount is a **liability**, never revenue/cash.

## Files to create / modify
```
nem_pay/api/internal/service/payment_intent.go     (Create: accept + validate escrow fields)
nem_pay/api/internal/httpapi/payment_intents.go    (create request struct + binding)
nem_pay/api/internal/service/money.go              (Capture: credit destination by mode)
nem_pay/api/db/queries/payment_intents.sql         (CreateIntent already sets mode/payee/fee — extend if needed)
```

## Implementation steps
1. Extend the create request/DTO with `escrow`, `payee`, `application_fee`; validate per the rules
   above; map to `CreateIntentParams` (settlement_mode/payee_id/application_fee).
2. In `Capture`, choose the credit account by `pi.SettlementMode`: direct → `merchant_payable`
   (singleton); escrow → `escrow_liability` (per-reference via `GetOrCreatePerRefAccount`, ref = intent
   id). Post `Dr acquirer_receivable / Cr <dest>`; assert `CanTransition(mode, status, captured)`.

## Validation / tests
- Create escrow → 200, `settlement_mode=escrow`, payee/fee stored; missing/blank payee → 400;
  `fee<0` or `fee>amount` → 400; a later attempt to change mode/payee/fee has no effect. (**AC1**)
- Escrow capture → `captured`; ledger shows `escrow_liability(intent) = amount` (liability) and
  `acquirer_receivable = amount`; no revenue/cash. (**AC2**)
- Direct create + capture unchanged (regression). (**AC8**)

## Risks & rollback
- **Fee edge cases** (`fee==amount`, `fee==0`) accepted; only `<0`/`>amount` rejected — test both.
- **Per-reference account** must key on intent id (guard #2). Rollback: additive on task-01.
