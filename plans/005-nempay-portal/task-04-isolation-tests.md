# Task 04 — Multi-tenant isolation & auth test suite

**Plan:** ./plan.md · **Depends on:** task-02, task-03 · **Blocks:** none

## Context
- Spec acceptance criteria covered: **AC1**, **AC2** (the core isolation AC), **AC3**, **AC8**
  (no secret leak), **AC10** (session cannot mutate). This task is the *proof* the feature is safe.
- Links: spec "Behaviours" (merchant A never sees B); plan "Risks".

## Requirements
- An automated test suite (Go, against the throwaway test DB) that treats tenant isolation as an
  invariant across **every** read endpoint, using two merchants A and B each with seeded data.
- Cover the auth boundary: unauthenticated access denied; session refused on money routes; keys and
  tokens never disclosed.

## Files to create / modify
```
nem_pay/api/internal/httpapi/portal_isolation_test.go   (new)
nem_pay/api/internal/httpapi/*_test.go                   (extend fixtures for 2 merchants + a user each)
```

## Implementation steps
1. Extend the API fixture to seed two merchants, one user + one secret key each, and a little data
   for both (an intent, a webhook event) — enough to tell them apart.
2. Table-driven test over the read endpoints (`/v1/payment_intents`, `/:id`, `/:id/ledger`,
   `/v1/balances`, `/v1/webhook_events`, `/v1/api_keys`): sign in as A, assert responses contain A's
   data and **none** of B's; request B's resource by id → 404.
3. Auth-boundary tests: no credential → 401; a session token on `POST /v1/payment_intents` and the
   money verbs → 403/401 (session cannot move money); `GET /v1/api_keys` body contains no
   `token_hash` and no `sk_`/`pk_` full value.
4. Run via `make test-db` (serialised, `-p 1`).

## Validation / tests
- Green suite is the acceptance evidence: **AC2** (cross-tenant reads blocked, incl. by-id),
  **AC1** (unauth denied), **AC3** (scope from session), **AC10** (session cannot mutate),
  **AC8** (no secret in responses).

## Risks & rollback
- Risk: tests assert current shapes and could be brittle. Mitigation: assert on presence/absence of
  tenant-owned ids and forbidden fields, not full payloads.
- Rollback: delete the test file(s); no production code depends on them.
