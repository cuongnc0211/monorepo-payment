# Task 02 — JWT session login + key-or-session auth

**Plan:** ./plan.md · **Depends on:** task-01 · **Blocks:** task-03, task-04, task-05

## Context
- Spec acceptance criteria covered: **AC1** (auth required), **AC3** (session bound to one merchant;
  scope from the session, not a client-supplied id), and enforces the "session can never move money"
  invariant behind **AC10**.
- Links: plan.md decisions (hand-rolled `golang-jwt/jwt/v5` + `bcrypt`; session accepted only on GET
  reads + login); middleware_auth.go (`apiKeyAuth`, `secretOnly`, `KeyKind`), context.go.

## Requirements
- `POST /v1/portal/login` (public): `{email, password}` → verify bcrypt → issue a short-lived HS256
  JWT carrying `merchant_id` (+ `sub`, `exp`). Response `{token, expires_at, merchant:{id,name}}`.
  No refresh token (re-login on expiry). Generic error on bad credentials (no user-enumeration).
- A `sessionAuth` verifier: parse/verify the JWT, attach `merchant_id` and kind `"session"` to the
  context (reusing `setAuth`). Rejects expired/invalid tokens with the standard error envelope.
- An `authAny` middleware for read routes: accept **either** a valid API key (existing `apiKeyAuth`)
  **or** a session JWT, resolving to `merchant_id` either way. Distinguish by token shape
  (`pk_`/`sk_` prefix → key lookup; otherwise verify as JWT).
- The session kind must be **refused** by `secretOnly` and `publishableOnly` (unchanged) so a
  session can only reach routes that opt into it.
- Config: `NEMPAY_JWT_SECRET` (dev default, documented) and `NEMPAY_JWT_TTL` (e.g. 60m).

## Files to create / modify
```
nem_pay/api/go.mod / go.sum                              (add golang-jwt/jwt/v5, x/crypto/bcrypt)
nem_pay/api/internal/config/config.go                    (JWTSecret, JWTTTL)
nem_pay/api/internal/httpapi/session.go                  (new: issueToken, sessionAuth, authAny, login handler)
nem_pay/api/internal/httpapi/router.go                   (mount POST /v1/portal/login)
nem_pay/api/internal/httpapi/session_test.go             (new)
```

## Implementation steps
1. Add deps; extend config with `JWTSecret`/`JWTTTL`.
2. `issueToken(merchantID, ttl)` → signed HS256 JWT with `merchant_id`, `exp`.
3. `login` handler: `GetUserByEmail` → `bcrypt.CompareHashAndPassword` → issue token; return token +
   expiry + merchant. Constant-ish failure path for unknown email vs bad password (same 401).
4. `sessionAuth`: verify signature + expiry, `setAuth(c, merchantID, "session")`.
5. `authAny`: if bearer looks like an api key (`pk_`/`sk_`), run the key path; else run `sessionAuth`.
   One of them must succeed or respond 401.
6. Mount login as a **public** route (no auth, but under the CORS layer). Do not yet attach
   `authAny` to any read route — task-03 wires the read group.

## Validation / tests
- `login` with a seeded email+password → 200 + a token whose claims carry that user's `merchant_id`;
  wrong password → 401; unknown email → 401 (same shape). (**AC1**)
- `sessionAuth` accepts a freshly issued token and rejects a tampered/expired one.
- `authAny` accepts both a secret key and a session token, attaching the right merchant. (**AC3**)
- A session token is **refused** by `secretOnly` (assert 403 on a route guarded by it), proving a
  session cannot reach money verbs. (guards **AC10**)

## Risks & rollback
- Risk: a JWT accepted on money routes. Mitigation: money routes keep `secretOnly`; test asserts
  refusal. Risk: weak dev secret. Mitigation: env-configurable, documented.
- Rollback: remove `session.go`, the login route, and config fields; `authAny` is not yet mounted
  elsewhere, so removal is local.
