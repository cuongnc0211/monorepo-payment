# Task 03 — Tenant-scoped read endpoints (+ OpenAPI, CORS)

**Plan:** ./plan.md · **Depends on:** task-02 · **Blocks:** task-04, task-05, task-06

## Context
- Spec acceptance criteria covered: **AC4** (payment detail + ledger), **AC5** (balances), **AC6**
  (webhook logs), **AC7/AC8** (api keys masked, no secret leaked), **AC10** (read-only, GET only),
  **AC11** (served from `/v1`), and query-layer scoping underpins **AC2**.
- Links: 0001_ledger, 0004_payment_intents, 0005_outbox_webhooks, 0003 api_keys; router.go, cors.go.

## Requirements
- New **GET** endpoints under `/v1`, each guarded by `authAny` (key or session) and scoped to the
  credential's `merchant_id`. No mutation endpoints.
  - `GET /v1/balances` — balances by account kind + currency, derived as Σ(entries) in `numeric`.
  - `GET /v1/webhook_events` — `outbox` rows (merchant-scoped) with their latest delivery status
    from `webhook_deliveries`; newest-first, paginated.
  - `GET /v1/api_keys` — keys by kind, `token_prefix`, created/revoked. **Never** return
    `token_hash` or a full key.
  - `GET /v1/payment_intents/:id/ledger` — the transaction(s)/entries backing the intent, plus any
    refund entries. 404 if the intent is not this merchant's.
- Extend `GET /v1/payment_intents` and `/:id` to also accept a session (swap `secretOnly`→`authAny`
  on the read verbs only; the POST create/list-write stay secret-only). Response shapes unchanged.
- Every get-by-id query includes `AND merchant_id = $auth` so another merchant's id returns 404,
  never a disclosure.
- CORS: allow a **set** of origins (merchant site + portal dev origin) via `NEMPAY_CORS_ORIGINS`.
- OpenAPI documents the new endpoints and a `Session` (bearer JWT) security scheme.

## Files to create / modify
```
nem_pay/api/db/queries/{balances.sql, webhooks.sql, api_keys.sql, entries.sql}   (new)
nem_pay/api/internal/repository/db/*                                             (sqlc regen)
nem_pay/api/internal/httpapi/{balances.go, webhooks.go, api_keys.go, ledger.go}  (new handlers)
nem_pay/api/internal/httpapi/router.go                                          (read group → authAny; new routes)
nem_pay/api/internal/httpapi/cors.go                                            (multi-origin)
nem_pay/api/internal/config/config.go                                           (NEMPAY_CORS_ORIGINS)
nem_pay/api/openapi.yaml                                                        (endpoints + Session scheme)
```

## Implementation steps
1. Write the tenant-scoped SQL (all take `merchant_id`): balances aggregate; webhook events joined
   to latest delivery; api-keys list (no hash); entries/transactions for an intent. `make sqlc`.
2. Add handlers that read `MerchantID(c)` from the credential and call the scoped queries; hand-map
   responses (mask keys; format money as minor-unit int64 like the intent responses).
3. Split the router: a **reads** group uses `authAny`; the `payment_intents` GET verbs move under it,
   while POST create + money verbs stay under `secretOnly`. Mount the four new GET routes.
4. Generalise `cors()` to match any origin in the configured allow-set; keep preflight behaviour.
5. Update `openapi.yaml`.

## Validation / tests
- Each endpoint returns only the caller's rows; a get-by-id for another merchant's resource → 404
  (proves **AC2** at the endpoint layer).
- `GET /v1/api_keys` never includes `token_hash`/full token (**AC7/AC8**).
- Balances equal Σ(entries) computed independently in the test (**AC5**); intent ledger sums to zero
  per transaction (**AC4**).
- The read endpoints accept a session token and a secret key equally (**AC11**); no non-GET route is
  added (**AC10**).

## Risks & rollback
- Risk: forgetting a `merchant_id` filter. Mitigation: scope lives in the SQL, reviewed; isolation
  suite in task-04 exercises every endpoint.
- Rollback: drop new queries/handlers/routes; restore `secretOnly` on the intent GETs; revert CORS
  to a single origin. Additive-only, money core untouched.
