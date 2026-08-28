# Task 01 — Gateway: typed access/refresh tokens + refresh endpoint

**Plan:** ./plan.md · **Depends on:** none · **Blocks:** task-02

## Context
- Spec ACs covered: **AC1** (login issues both), **AC2** (refresh → new access, same merchant),
  **AC3** (bad refresh → 401), **AC4** (token types non-interchangeable), **AC8** (privilege unchanged).
- Links: internal/httpapi/session.go (issueToken/verifyToken/login/authAny), router.go, openapi.yaml.

## Requirements
- Access and refresh tokens carry a `typ` claim (`access` / `refresh`); both signed with the JWT
  secret. Access TTL short (~15m), refresh TTL long (~12h), env-tunable.
- Login returns `{ token, refresh_token, expires_at, merchant }` (adds refresh_token; `token` stays
  the access token for back-compat).
- `POST /v1/portal/refresh` *(public)* `{ refresh_token }` → verify `typ=refresh` + not expired →
  `{ token, expires_at }` (new access, same merchant). Reject anything else with 401.
- `verifyToken` (used by `authAny` sessions) accepts only `typ=access`; the refresh handler accepts
  only `typ=refresh`. No rotation, no server-side revocation (documented).

## Files to create / modify
```
nem_pay/api/internal/httpapi/session.go        (typ claim; issueRefreshToken; refresh handler; verify enforces typ)
nem_pay/api/internal/httpapi/router.go         (POST /v1/portal/refresh)
nem_pay/api/internal/httpapi/session_test.go   (refresh cases)
nem_pay/api/openapi.yaml                        (document /v1/portal/refresh, x-internal; refresh_token in login)
nem_pay/api/internal/config (or session env)    (NEMPAY_JWT_TTL access; NEMPAY_JWT_REFRESH_TTL)
```

## Implementation steps
1. Add `typ` to issued claims; `issueToken` → access (typ=access, access TTL); add `issueRefreshToken`
   (typ=refresh, refresh TTL). Login issues both; response gains `refresh_token`.
2. `verifyToken` returns (merchant, typ) or enforces `typ=access`; add a refresh-verify path requiring
   `typ=refresh`.
3. Refresh handler: bind `{refresh_token}`, verify refresh, issue a new access, return it. 401 on any
   failure (expired/invalid/wrong typ) with the standard error envelope.
4. Mount `POST /v1/portal/refresh` as public (no auth middleware), under CORS.
5. OpenAPI: add the endpoint (x-internal) and refresh_token to the login response.

## Validation / tests
- Login returns a non-empty `token` and `refresh_token` (**AC1**).
- Refresh with the login's refresh token → 200 + a new access token whose merchant matches (**AC2**);
  the new access token authorizes a read route.
- Refresh with an access token → 401; a session route with a refresh token → 401 (**AC3/AC4**).
- A refreshed access token is still refused on `POST /v1/payment_intents` (**AC8**).
- `make test-db` green.

## Risks & rollback
- Risk: breaking existing session tests. Mitigation: keep `token` as access; issueToken keeps its
  signature/behaviour (now stamped typ=access). Rollback: remove refresh route/handler + typ checks.
