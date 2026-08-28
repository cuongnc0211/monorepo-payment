# Task 01 — Gateway: webhook-endpoint list/create/disable

**Plan:** ./plan.md · **Depends on:** none · **Blocks:** task-02

## Context
- Spec ACs: **AC1** (list scoped), **AC2** (create), **AC3** (disable), **AC4** (tenant isolation),
  **AC5** (non-money), **AC6** (secret write-only), **AC7** (validation).
- Links: db/queries/webhook_endpoints.sql, internal/httpapi/portal_reads.go, router.go.

## Requirements
- Three `authAny` (session or secret key), tenant-scoped endpoints; secret never returned:
  - `GET /v1/webhook_endpoints` → list all (incl disabled) for the merchant.
  - `POST /v1/webhook_endpoints` `{url, secret}` → create; 400 on missing/invalid url or empty secret.
  - `POST /v1/webhook_endpoints/{id}/disable` → disable; 404 if not this merchant's.
- No ledger write, no balance change (non-money). No Idempotency-Key (config, documented).

## Files to create / modify
```
nem_pay/api/db/queries/webhook_endpoints.sql   (ListEndpoints, DisableEndpoint)
nem_pay/api/internal/repository/db/*            (sqlc regen)
nem_pay/api/internal/httpapi/webhook_endpoints.go (new handlers)
nem_pay/api/internal/httpapi/router.go          (mount under authAny)
nem_pay/api/internal/httpapi/webhook_endpoints_test.go (new)
nem_pay/api/openapi.yaml                         (x-internal docs)
```

## Implementation steps
1. Queries: `ListEndpoints` (merchant, order created desc, incl disabled) and `DisableEndpoint`
   (UPDATE disabled_at=now WHERE id=$1 AND merchant_id=$2 RETURNING). `make sqlc`.
2. Handlers reading `MerchantID(c)`: list (map to `{id,url,active,created_at}`, omit secret),
   create (validate url http(s) + secret non-empty), disable (RETURNING → 404 if no row).
3. Router: a writes group `v1.Group("").Use(session.authAny())` (reuse the reads group or a sibling)
   with the three routes. Money routes unchanged.
4. OpenAPI: document the three (x-internal).

## Validation / tests
- Create → appears in list; disable → active=false (**AC1/AC2/AC3**).
- Two merchants: A cannot list/disable B's endpoint; B's id → 404 (**AC4**).
- List/create responses never contain the secret (**AC6**).
- Missing url / empty secret → 400 (**AC7**).
- A create+disable leaves balances unchanged (**AC5**); a session token is still refused on
  `POST /v1/payment_intents`.
- `make test-db` green.

## Risks & rollback
- Rollback: remove routes/handlers/queries. Additive; no schema.
