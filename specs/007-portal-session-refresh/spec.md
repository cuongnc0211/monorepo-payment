# Spec — Portal session refresh

**ID:** 007-portal-session-refresh · **Status:** draft · **Author:** cuongnc0211 · **Date:** 2026-08-26
**Constitution:** ../../CLAUDE.md (+ nem_pay/CLAUDE.md → "Auth: JWT for portal sessions")
**Plan:** ../../plans/007-portal-session-refresh/plan.md

> WHAT and WHY only. No HOW (no tech choices, schema, or code) — that lives in the plan.

## Why
The portal (spec 005) uses a short-lived access token held in memory, with **no refresh**: when the
token expires mid-session, the next request 401s and the user is bounced to the login screen even
though they were actively working. That is the deliberate first-cut simplification we said we would
lift later. This feature adds the refresh half so a session survives access-token expiry without
re-entering credentials — the standard access-plus-refresh pattern, and the auth lesson the
constitution points at ("JWT ... held in memory with refresh").

It stays **memory-only** (no localStorage): the tokens live only in the running page. Surviving a
full page reload would require persisting a credential (a cookie or storage) and is **out of scope**
here — this feature is specifically about not being kicked out when the access token expires while
the tab is open.

## What — scope

**In scope**
- Login issues **two** tokens: a short-lived **access** token and a longer-lived **refresh** token.
- A **refresh endpoint** exchanges a valid refresh token for a new access token, preserving the
  merchant scope. It is a distinct operation from login (no credentials).
- The portal **transparently refreshes** the access token when it has expired: an in-flight request
  that fails on expiry triggers a refresh and a retry, and the user stays on the page.
- When the refresh token itself is invalid/expired, the portal clears the session and returns to
  login.

**Out of scope / non-goals**
- **Reload-persistence** — surviving a full browser reload (tokens are memory-only; a reload logs
  the user out, same as today).
- Persisting either token in localStorage/sessionStorage or a cookie.
- Changing what a session can *do* (still read-only; still cannot reach money routes).
- RBAC, multi-user, "remember me" across devices, or idle-timeout policy tuning.

## Behaviours / user stories
- As merchant staff, when my access token expires while I am using the portal, then my next action
  silently renews the session and succeeds — I am **not** sent to the login screen.
- As merchant staff, when my refresh token has also expired (a long-idle session), then I am
  returned to login and can sign in again.
- As merchant staff, when I sign out, then both my access and refresh tokens are dropped.

## Acceptance criteria (testable — the definition of done)
- [ ] **AC1** Login returns both a short-lived access token and a longer-lived refresh token.
- [ ] **AC2** The refresh endpoint, given a valid refresh token, returns a **new access token**
  scoped to the **same merchant**; no credentials are required.
- [ ] **AC3** The refresh endpoint rejects (401) an expired, invalid, or non-refresh token and
  issues nothing.
- [ ] **AC4** Token types are distinct and non-interchangeable: a **refresh** token is refused on
  the read/money routes (it is not an access token), and an **access** token is refused at the
  refresh endpoint.
- [ ] **AC5** In the portal, an expired access token during a session triggers a transparent refresh
  and retry of the original request; the user stays on the current page and the data loads (no
  redirect to login).
- [ ] **AC6** When refresh fails (refresh token expired/invalid), the portal clears the session and
  routes to login.
- [ ] **AC7** Both tokens are held in memory only — never written to localStorage, sessionStorage,
  or a cookie; a full page reload logs the user out (reload-persistence is out of scope).
- [ ] **AC8** A refreshed access token still cannot perform money-mutating actions (the session
  privilege is unchanged by refresh).

## Constraints (from the constitution)
- Portal auth is **held in memory**, not localStorage (nem_pay/CLAUDE.md).
- Refresh must not widen a session's authority: a browser session still cannot move money.
- One consistent error envelope for the refresh endpoint, like the rest of `/v1`.

## Open questions
- **Rotation:** does each refresh also issue a new refresh token (rotation), or is the refresh token
  static until it expires? (Plan.)
- **TTLs:** concrete access vs refresh token lifetimes. (Plan.)
- **Revocation on logout:** is logout purely client-side (drop tokens; rely on short access TTL), or
  server-side (invalidate the refresh token)? (Plan.)
- Confirm reload-persistence stays out of scope for this cut.
