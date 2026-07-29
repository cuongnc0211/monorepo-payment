# Task 05 (M1.4) — Money flow: authorize / capture / settle + bank-sim

**Milestone:** M1 · **Depends on:** M1.1 (ledger), M1.2 (idempotency), M1.3 (intents/state) · **Blocks:** M1.5, M2

## Context
- Gateway rules: [`../../nem_pay/CLAUDE.md`](../../nem_pay/CLAUDE.md) — "Money handling", "The bank simulator",
  layering (service owns the tx boundary; idempotency + ledger write together in one tx).
- This is where the **money plane** and **ledger plane** wire together. Carries
  **escrow-adaptability guard #3**: capture posts to a destination **decided by mode**.

## Requirements
- `confirm` (authorize), `capture`, `refund`, and `settle` transitions, each:
  - checked via `statemachine`, wrapped in idempotency (M1.2),
  - performed with `SELECT … FOR UPDATE` on the intent row inside a single DB tx,
  - writing the ledger transaction **in the same tx** as the status change.
- All external money movement goes through **bank-sim** (approved / declined / timeout). Never a
  real processor. The authorize **timeout** path is a first-class, tested scenario.
- Money is `int64` minor units + currency; reports/sums use `numeric`.

## The capture race — the core correctness lesson
```sql
-- db/queries/payment_intents.sql
-- name: LockIntentForUpdate :one
SELECT * FROM payment_intents WHERE id = $1 FOR UPDATE;
```
Two concurrent captures both call this; the second **blocks** until the first commits, then reads
`status='captured'` and is rejected. No double-capture, no double ledger post.

## Ledger postings (direct mode) — guard #3 in action
Amounts are non-negative; each line is a debit or a credit; **Σdebit = Σcredit** (Convention B,
see M1.1). Capture's **credit destination** is chosen by `settlement_mode`: `merchant_payable`
in M1; M3 points the same code at `escrow_liability(intent)`.
```
capture (A):  debit acquirer_receivable A  ;  credit merchant_payable A
              (captured: the acquirer now owes us A, we owe the merchant A — no cash moved yet)
settle  (A):  debit platform_cash        A  ;  credit acquirer_receivable A
              (acquirer actually pays out in a batch: the receivable becomes real cash)
refund  (R):  debit merchant_payable      R  ;  credit acquirer_receivable R   (pre-settle)
           or debit merchant_payable      R  ;  credit platform_cash        R   (post-settle)
```
Each posting is one balanced `PostTransaction` written in the intent's tx. Note: **settle is not
payout** — after settle we hold the cash but `merchant_payable` still stands; paying the merchant
out is a separate payout step (deferred, not in M1).

## bank-sim contract
```
POST /authorize  { intent_id, amount, currency, token }  → 200 {status: approved|declined}
POST /capture    { intent_id, amount }                    → 200 {status: approved}
```
Outcome is driven by magic tokens for deterministic tests:
`tok_ok` → approved · `tok_declined` → declined · `tok_timeout` → sleeps past the client timeout.

## Files to create / modify
```
api/bank-sim/cmd/banksim/main.go          -- implement /authorize /capture with magic-token outcomes
api/internal/banksim/client.go            -- HTTP client w/ context timeout; typed outcomes
api/internal/service/payment_intent.go    -- add Authorize/Capture/Refund + SettleDueIntents sweep (tx + FOR UPDATE + ledger)
api/internal/httpapi/payment_intents.go   -- wire confirm/capture/refund handlers
api/db/queries/payment_intents.sql        -- LockIntentForUpdate, UpdateStatus
```
> **Settle runs as a periodic sweep, not an endpoint** (decision). A scheduled job finds
> `captured` intents older than a configured settlement delay (simulating T+1) and transitions
> them to `settled`, posting the settle entry above. This models settlement's real
> asynchronous, batched nature — the money plane on its own clock. The transition logic lives in
> `SettleDueIntents(ctx)` in the money service and is **directly callable**, so tests force-run it
> deterministically (no wall-clock waiting). It is scheduled on the worker (M1.5). A similar
> sweep expires stale `created`/`authorized` intents.

## Implementation steps
1. Implement bank-sim outcomes; `banksim.Client` with a context deadline so `tok_timeout` surfaces
   as a Go `context.DeadlineExceeded`.
2. `Authorize`: tx → `LockIntentForUpdate` → assert `CanTransition(status,'authorized')` → call
   bank-sim → on approved set `authorized` (declined → `failed`); commit.
   - **Timeout handling**: if the authorize call times out, the intent must NOT silently succeed —
     leave it in a safe state (`requires_confirmation`/pending) and make the outcome reconcilable;
     document the chosen policy ("did the bank receive it?").
3. `Capture`: tx → lock → assert transition → `PostTransaction(capture, …)` (destination by mode)
   → set `captured` → commit. Idempotent via M1.2.
4. `Refund`: partial or full; set `refunded`/`partially_refunded`; balanced reversal posting.
5. `SettleDueIntents(ctx)`: for each due `captured` intent (older than the settlement delay), in
   one tx per intent, `LockIntentForUpdate` → assert transition → post the settle entry (debit
   `platform_cash` / credit `acquirer_receivable`) → set `settled`. Callable directly for tests;
   scheduled on the worker in M1.5.

## Validation / tests (matrix)
| Scenario | Expected |
|---|---|
| happy path create→confirm→capture, then force-run settle sweep | states advance to `settled`; `acquirer_receivable` clears into `platform_cash`; `merchant_payable` still stands (payout deferred) |
| settle sweep selectivity | only `captured` intents past the delay settle; `authorized`/`failed` untouched |
| declined authorize (`tok_declined`) | intent `failed`; no ledger post |
| **authorize timeout** (`tok_timeout`) | intent left safe/reconcilable; documented policy holds |
| double capture (concurrent, same key) | one capture; one ledger tx (FOR UPDATE + idempotency) |
| capture from illegal state | rejected by statemachine; no post |
| full + partial refund | correct `refunded`/`partially_refunded`; balances reverse exactly |
| sum of all entries per tx | always 0 (property test) |

## Risks & rollback
- **Collapsing planes**: do NOT call the merchant webhook here — that's M1.5's outbox. Status +
  ledger only in this tx.
- **Timeout ambiguity** is the classic double-charge source. Its policy must be explicit and tested,
  not incidental.
- **Wrong-sign postings** skew balances silently — rely on the M1.1 sum-zero assertion + golden tests.
- Rollback: money verbs are additive on top of M1.3; disabling the routes reverts behaviour. Ledger
  rows are append-only — never delete to "undo"; post a reversing transaction instead.
