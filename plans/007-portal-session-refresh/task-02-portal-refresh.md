# Task 02 — Portal: in-memory refresh + transparent refresh-and-retry

**Plan:** ./plan.md · **Depends on:** task-01 · **Blocks:** none

## Context
- Spec ACs covered: **AC5** (transparent refresh + retry; stay on page), **AC6** (refresh fail →
  login), **AC7** (memory-only; reload logs out).
- Links: portal src/auth/token.ts, src/api/client.ts, src/auth/auth.tsx.

## Requirements
- Store the access **and** refresh tokens in memory (never localStorage/cookie).
- On a `401` caused by an expired access token, the client refreshes once (calls `/v1/portal/refresh`
  with the refresh token) and retries the original request with the new access token. The user stays
  on the current page.
- If refresh fails (or there is no refresh token), clear the session and route to login.
- Never attempt to refresh the refresh call itself (no loop); retry at most once.

## Files to create / modify
```
nem_pay/portal/src/auth/token.ts     (hold refreshToken; setSession(access, refresh, merchant))
nem_pay/portal/src/api/client.ts     (custom fetch: inject bearer + 401→refresh→retry once)
nem_pay/portal/src/auth/auth.tsx     (login stores both tokens from the response)
nem_pay/portal/src/api/schema.ts     (regenerated from openapi.yaml)
```

## Implementation steps
1. token.ts: add `refreshToken`; `setSession(access, refresh, merchant)`; `getRefreshToken`.
2. client.ts: pass a custom `fetch` to `createClient` that injects `Authorization: Bearer <access>`,
   and on `401` (when a refresh token exists and the URL is not the refresh endpoint) calls refresh,
   updates the access token, and retries once; on refresh failure clears tokens and fires the
   unauthorized handler.
3. auth.tsx: on login, store `token` + `refresh_token` from the response.
4. Regenerate the client (`pnpm gen`) so the login/refresh types are current.

## Validation / tests
- Manual (browser) with a **very short** access TTL (env): work in the portal past the access
  expiry; a page action still loads data without a redirect to login (**AC5**).
- With refresh also expired/cleared, the next action returns to login (**AC6**).
- Confirm tokens are not in localStorage; a full reload logs out (**AC7**).

## Risks & rollback
- Risk: refresh loop or double-retry. Mitigation: guard on the refresh URL + a single retry flag.
- Rollback: revert client.ts to the prior middleware (bearer + 401→login) and token.ts to
  access-only.
