# Task 07 — Full escrow lifecycle tests + direct-mode regression + e2e

**Plan:** ./plan.md · **Depends on:** task-01…06 · **Blocks:** none (completes M3)

## Context
- Spec acceptance criteria covered: **AC1–AC8** exercised together, with emphasis on **AC6**
  (invariant) and **AC8** (direct-mode regression).
- Links: M1 money/service test patterns (`make test-db`, `-p 1`); the spec's acceptance matrix.

## Requirements
- A **service-level escrow lifecycle** test that drives create → capture → settle → release (and a
  parallel refund path) against the real test Postgres, asserting every ledger balance and the
  segregation invariant at each step.
- **Failure/edge** coverage: illegal transitions (release non-held, refund released, partial escrow
  refund), idempotent + concurrent release (exactly one posting), fee edges (`0`, `==amount`).
- **Direct-mode regression**: the full M1 suite still passes unchanged.
- A **`curl` docker end-to-end**: `docker-compose up` in `NemPay/`, then drive an escrow intent
  create → confirm → capture → (settle sweep) → release entirely over `/v1`, verifying the
  `held_in_escrow` and `escrow.released` webhooks are emitted and the invariant holds.

## Files to create / modify
```
nem_pay/api/internal/service/escrow_test.go        (lifecycle + invariant + edges + concurrency)
nem_pay/api/internal/httpapi/escrow_flow_test.go   (create/release/refund over HTTP, idempotency)
nem_pay/api/…                                       (any shared test helpers)
```

## Validation / tests (matrix — the definition of done)
| Scenario | Expected | AC |
|---|---|---|
| create escrow → capture → settle → release | states advance; balances exact; fee to revenue, rest to payee | AC1–AC3, AC2b |
| segregation invariant after each step | `Σ escrow_liability(held) == segregated_cash` | AC6 |
| idempotent release / concurrent release | exactly one release posting | AC4 |
| full refund from held; refund after release | refunded / rejected | AC5, AC7 |
| illegal edges (release non-held, partial escrow refund) | rejected, no posting | AC7 |
| direct-mode lifecycle (M1 suite) | unchanged | AC8 |
| docker e2e over /v1 | held_in_escrow + escrow.released webhooks; invariant holds | AC1–AC6 |

## Implementation steps
1. Write the service lifecycle + invariant + edge + concurrency tests.
2. Write the HTTP-level escrow flow tests (create/release/refund, idempotency headers).
3. Confirm `make test-db` (with `-p 1`) is green including the M1 regression.
4. Run the docker `curl` e2e and record the outcome.

## Risks & rollback
- **Invariant assertion** must scope to held intents (see task-06). **Concurrency** tests must not be
  flaky — reuse M1's pattern (bounded goroutines, real DB, `-p 1`). Rollback: tests are additive.
