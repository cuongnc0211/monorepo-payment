# Task 04 (M1.3) — Payment intent, state machine, auth & API surface

**Milestone:** M1 · **Depends on:** M1.0, M1.2 · **Blocks:** M1.4 (money flow drives these states)

## Context
- Gateway rules: [`../../nem_pay/CLAUDE.md`](../../nem_pay/CLAUDE.md) — "Payment state machine", "API conventions".
- Carries **escrow-adaptability guards #1 and #4**: the `settlement_mode`/`payee_id`/
  `application_fee` columns exist now (direct-only), and transitions live in one central map.

## Requirements
- `payment_intents` table with escrow seams present but unused (mode = `direct` only).
- **State transitions defined in ONE place** (a Go allowed-edges map), reused by every handler.
- Auth: API keys — **publishable** (client/tokenization) vs **secret** (server-to-server). A
  publishable key must never perform a secret-key action. JWT for portal is deferred with the portal.
- Consistent error shape everywhere: `{ "error": { "type", "code", "message", "param" } }`.
- Emit an OpenAPI spec (oapi-codegen) — the M2/M4 merchants and (later) the portal generate clients from it.

## State machine (direct mode only in M1)
```
created → requires_confirmation → authorized → captured → settled
authorized → failed
captured | settled → refunded | partially_refunded
```
Escrow edges (`captured → held_in_escrow → released`, `held_in_escrow → refunded`) are added
in M3 by **appending to the same map** — do not scatter transition checks across handlers.

## Schema (migration `0003_payment_intents`, `0004_merchants_api_keys`)
```sql
-- 0003
CREATE TABLE payment_intents (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id     uuid NOT NULL,
  amount          bigint NOT NULL CHECK (amount > 0),
  currency        char(3) NOT NULL,
  status          text NOT NULL DEFAULT 'created'
    CHECK (status IN ('created','requires_confirmation','authorized','captured',
                      'settled','failed','refunded','partially_refunded')),  -- M3 adds escrow states
  settlement_mode text NOT NULL DEFAULT 'direct'
    CHECK (settlement_mode IN ('direct')),                                   -- M3 adds 'escrow'
  payee_id        uuid,        -- NULL for direct; used by escrow in M3
  application_fee bigint,      -- NULL for direct; used by escrow in M3
  metadata        jsonb NOT NULL DEFAULT '{}',
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

-- 0004
CREATE TABLE merchants (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE api_keys (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id  uuid NOT NULL REFERENCES merchants(id),
  kind         text NOT NULL CHECK (kind IN ('publishable','secret')),
  token_prefix text NOT NULL,          -- e.g. 'pk_live_' / 'sk_live_' for lookup
  token_hash   text NOT NULL,          -- store a hash, never the raw secret
  created_at   timestamptz NOT NULL DEFAULT now(),
  revoked_at   timestamptz
);
CREATE INDEX api_keys_lookup ON api_keys (token_prefix);
```
> `status`/`settlement_mode` are **text + CHECK**, not native enum — deliberate: M3 extends the
> allowed set by altering the CHECK, avoiding Postgres enum-migration gymnastics (`ALTER TYPE …
> ADD VALUE` can't run in a tx and can't be undone).

## API surface (`/v1`, direct scope)
| Method | Path | Key | Idem-Key | Notes |
|---|---|---|---|---|
| POST | `/v1/payment_intents` | secret | ✅ | create (`amount`,`currency`,`metadata`) |
| POST | `/v1/payment_intents/:id/confirm` | secret | ✅ | → authorize (M1.4) |
| POST | `/v1/payment_intents/:id/capture` | secret | ✅ | (M1.4) |
| POST | `/v1/payment_intents/:id/refund` | secret | ✅ | (M1.4) |
| GET | `/v1/payment_intents/:id` | secret | — | |
| GET | `/v1/payment_intents` | secret | — | list; dynamic filters → `sqlc.narg`/squirrel, **no GORM** |

## Files to create
```
api/db/migrations/0003_payment_intents.*  0004_merchants_api_keys.*
api/db/queries/payment_intents.sql  merchants.sql  api_keys.sql
api/internal/statemachine/intent.go       -- allowedEdges map + CanTransition(from,to)
api/internal/httpapi/middleware_auth.go   -- API-key auth → merchant_id + key kind in ctx
api/internal/httpapi/payment_intents.go   -- handlers (create + read/list now; money verbs in M1.4)
api/internal/service/payment_intent.go    -- create logic, tx boundary, uses statemachine
api/internal/httpapi/errors.go            -- the error envelope + typed → HTTP mapping
api/openapi.yaml (+ oapi-codegen config)
```

## Implementation steps
1. Migrations `0003`,`0004`; `make sqlc`.
2. `statemachine/intent.go`: `map[string][]string` of allowed edges + `CanTransition`; unit-tested.
3. Auth middleware: parse key, look up by prefix, compare hash, attach `merchant_id`+`kind`;
   enforce secret-only on mutating routes.
4. Create handler: validate, insert intent (`status='created'`, `settlement_mode='direct'`),
   wrapped in M1.2 idempotency. Read/list handlers.
5. Author `openapi.yaml` for the above; wire oapi-codegen into `make`.

## Validation / tests
- Create → 200 with intent in `created`; publishable key on create → 401/403.
- `CanTransition` rejects illegal edges (e.g. `created → captured`).
- Reused secret key hash mismatch → rejected; revoked key → rejected.
- Error envelope shape is identical across a validation error, an auth error, and a 404.
- OpenAPI spec generates a client without hand-editing.

## Risks & rollback
- **Enum vs CHECK**: chose CHECK for M3 extensibility — don't switch to native enum later without
  weighing the migration cost.
- **Guard erosion**: if `settlement_mode`/`payee_id`/`application_fee` are dropped "because unused",
  M3 needs an intent-semantics migration. Keep them.
- **Scattered transitions**: any transition check written inline in a handler instead of via
  `statemachine` reintroduces the drift M3 will pay for. Enforce in review.
- Rollback: drop `0003`/`0004`; safe until M1.4 attaches money flow.
