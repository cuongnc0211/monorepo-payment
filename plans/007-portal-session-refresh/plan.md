# Plan — Portal session refresh

**ID:** 007-portal-session-refresh · **Status:** draft
**Spec:** ../../specs/007-portal-session-refresh/spec.md
**Constitution:** ../../CLAUDE.md (+ nem_pay/CLAUDE.md)

> HOW only. Satisfies the spec's ACs and obeys the constitution (memory-only portal auth).

## Approach (HOW)
Two HS256 JWTs signed with the same secret, distinguished by a `typ` claim: **access** (short) and
**refresh** (long). Login issues both. A new `POST /v1/portal/refresh` exchanges a valid refresh
token for a fresh access token, scoped to the same merchant. The token type is enforced everywhere:
`authAny`/session accepts only `typ=access`; the refresh endpoint accepts only `typ=refresh`. The
portal keeps both tokens in memory and, on a 401 caused by an expired access token, transparently
refreshes once and retries the original request; if refresh fails it clears the session and routes
to login.

## Key decisions & alternatives considered
- **Two token types via a `typ` claim, one signing secret.** *Alt:* opaque refresh tokens in a
  server-side store. *Why:* stateless, minimal, and the type check gives non-interchangeability
  (AC4) without new storage; matches the hand-rolled-jwt decision from 005.
- **No rotation, no server-side revocation (first cut).** *Alt:* rotate the refresh token each use +
  a revocation/denylist. *Why:* keeps it stateless; the short access TTL bounds exposure. **Deviation
  noted:** a stolen refresh token is valid until its TTL; rotation/revocation is a later lesson.
- **TTLs:** access ~15m, refresh ~12h (env-tunable, dev defaults). Short access so refresh is
  actually exercised in a session.
- **Login response stays backward-compatible:** keep `token` as the access token and **add**
  `refresh_token` (+ `expires_at`). Existing clients/tests keep working.
- **Portal: transparent refresh in a custom `fetch`** passed to the openapi-fetch client (bearer
  injection + 401→refresh→retry in one place). *Alt:* onResponse middleware (cannot re-issue the
  request cleanly). Both tokens stay in memory (constitution); reload still logs out (out of scope).

## Data model / API changes
- **No DB changes** (stateless tokens).
- `POST /v1/portal/login` → `{ token, refresh_token, expires_at, merchant }` (adds refresh_token).
- **New** `POST /v1/portal/refresh` *(public)* → body `{ refresh_token }` → `{ token, expires_at }`.
- OpenAPI: document `/v1/portal/refresh` (x-internal, portal-only); refresh_token added to login
  response.

## Risks & rollback
- **Risk — refresh token accepted as access (or vice-versa).** *Mitigation:* `typ` check in verify;
  tests assert both refusals (AC4).
- **Risk — refresh loop.** *Mitigation:* the portal never refreshes on the refresh call itself and
  retries at most once.
- **Risk — stolen refresh token usable until TTL.** *Mitigation:* short-ish refresh TTL; rotation/
  revocation deferred and documented.
- **Rollback:** additive — remove the refresh route/handler, drop `typ` enforcement back to
  access-only, revert login to `token`-only, and the portal to the previous re-login-on-expiry
  behaviour. No migrations.

## Tasks (execute in order)
- [ ] [task-01 — gateway: typed access/refresh tokens + /v1/portal/refresh + tests](./task-01-gateway-refresh.md)
- [ ] [task-02 — portal: in-memory refresh token + transparent refresh-and-retry](./task-02-portal-refresh.md)
