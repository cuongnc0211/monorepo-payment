# Task 03 (M1.2) — Idempotency infrastructure

**Milestone:** M1 · **Depends on:** M1.0 · **Blocks:** M1.3/M1.4 (every money-mutating POST wraps in this)

## Context
- Gateway rules: [`../../nem_pay/CLAUDE.md`](../../nem_pay/CLAUDE.md) — "Idempotency ... insert-first".
- Cross-cutting principle #1: retries must be safe **by construction**, never check-then-act.
- This is infra, not an endpoint — a reusable wrapper the money handlers call in M1.3+.

## Requirements
- Every money-mutating POST requires an `Idempotency-Key` header (reject with 400 if absent).
- **Insert-first** against `UNIQUE(merchant_id, idem_key)`; let the constraint arbitrate the race.
- Behaviour on the second caller:
  - row `in_flight` → `409 Conflict` (original still running).
  - row `completed` + matching request fingerprint → **replay stored response** (same code/body).
  - row exists + **different** fingerprint → `422` (key reused for a different request).
- Keys scoped per merchant; expire ~24h. A sweeper resets orphaned `in_flight` rows (crash recovery).

## Schema (migration `0002_idempotency`)
```sql
CREATE TABLE idempotency_keys (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id   uuid NOT NULL,
  idem_key      text NOT NULL,
  request_hash  text NOT NULL,        -- sha256(method + path + canonical body)
  status        text NOT NULL CHECK (status IN ('in_flight','completed')),
  response_code int,
  response_body jsonb,
  locked_at     timestamptz NOT NULL DEFAULT now(),   -- for the orphan sweeper
  created_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (merchant_id, idem_key)
);
CREATE INDEX idempotency_sweep_idx ON idempotency_keys (status, locked_at);
```

## Files to create
```
api/db/migrations/0002_idempotency.up.sql / .down.sql
api/db/queries/idempotency.sql        -- InsertInFlight, MarkCompleted, GetByKey, ResetOrphans
api/internal/httpapi/idempotency.go   -- WithIdempotency(handler) middleware/wrapper
api/internal/service/idempotency.go   -- fingerprinting + the insert-first decision logic
```

## Implementation steps
1. Middleware reads `Idempotency-Key` + `merchant_id` (from API key, M1.3).
2. Compute `request_hash`; attempt `INSERT ... status='in_flight'`.
   - Insert OK → you are first: run the handler; on success `MarkCompleted(code, body)`; return.
   - `UniqueViolation` → `GetByKey`: branch to 409 / replay / 422 per rules above.
3. Wrap the handler so its response is captured (buffered) for storing on the `completed` row.
4. Sweeper: a periodic job (`ResetOrphans`: `in_flight` older than N minutes → delete or reset)
   runs from the worker (M1.5) or a ticker. Document the N.

## Validation / tests
- Two concurrent identical POSTs (same key) → exactly one processes, the other 409 or replays;
  **never both execute the side effect** (assert one ledger transaction, not two).
- Same key + different body → 422.
- Completed key replayed → identical status code + body, no re-execution.
- Missing header on a money POST → 400.
- Orphaned `in_flight` (simulate crash mid-request) → sweeper frees it; retry then succeeds.

## Risks & rollback
- **TOCTOU** if anyone "optimises" to SELECT-then-INSERT — forbidden; the unique constraint is
  the arbiter. Add a test that hammers concurrency to lock this in.
- **Wedged retries**: without the sweeper, a crash mid-request pins every retry at 409 forever.
  The sweeper is required, not optional.
- **Fingerprint over-strictness** (e.g. hashing volatile headers) → false 422s. Hash only
  method + path + canonicalised body.
- Rollback: drop the table (`0002…down`); safe until M1.3 wires it into handlers.
