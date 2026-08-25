# Task 05 — Portal scaffold: SPA, generated client, login + guard

**Plan:** ./plan.md · **Depends on:** task-02, task-03 · **Blocks:** task-06

## Context
- Spec acceptance criteria covered: **AC1** (login gate), **AC3** (session-scoped), **AC9** (no PAN),
  **AC11** (all data via the generated `/v1` client).
- Links: nem_pay/CLAUDE.md "portal" (Vite+React+TS, TanStack Query, generated client, JWT in memory
  not localStorage).

## Requirements
- A new `nem_pay/portal/` Vite + React + TypeScript app.
- A TypeScript client **generated** from `openapi.yaml` (openapi-typescript / orval); treated as the
  contract, never hand-patched.
- Auth: a Login page → `POST /v1/portal/login`; the JWT is held **in memory** (React context/store),
  never in localStorage. All API calls send `Authorization: Bearer <jwt>`.
- An auth guard: unauthenticated users are routed to Login; a `401` from any call clears the token
  and returns to Login (re-login on expiry — no refresh).
- App shell: nav to the (empty for now) sections + a sign-out that drops the in-memory token.
- Configurable API base URL (env, default `http://localhost:8080`).

## Files to create / modify
```
nem_pay/portal/                         (new: Vite React-TS project)
  package.json, vite.config.ts, tsconfig.json, index.html
  src/main.tsx, src/App.tsx
  src/api/ (generated client + a thin fetch wrapper injecting the bearer token)
  src/auth/ (in-memory token store, AuthProvider, RequireAuth guard)
  src/pages/Login.tsx
nem_pay/portal/README.md                (how to run: npm i / npm run gen / npm run dev)
```

## Implementation steps
1. Scaffold Vite React-TS in `nem_pay/portal/`; add TanStack Query + the OpenAPI client generator.
2. Add a `gen` script that regenerates the client from `../api/openapi.yaml`.
3. Build the in-memory auth store + `AuthProvider`; the fetch wrapper attaches the bearer and, on
   401, clears the token.
4. Login page posts credentials, stores the returned token in memory, redirects to the app shell.
5. `RequireAuth` wraps the app routes; unauthenticated → Login.
6. Verify against the running gateway: sign in with a seeded user, land on the (empty) shell; a
   forced-expired/invalid token bounces back to Login.

## Validation / tests
- Manual (browser): unauthenticated visit shows Login and no data (**AC1**); signing in with a
  seeded user succeeds and the token is in memory only (not in localStorage) (**AC3**); no card
  field exists anywhere (**AC9**); network calls go to `/v1` via the generated client (**AC11**).
- (Optional) a component test for the guard: no token → Login; 401 → token cleared.

## Risks & rollback
- Risk: token in localStorage by habit. Mitigation: store in memory only; review + the manual check.
- Risk: CORS from the portal origin. Mitigation: task-03 added the portal origin to the allow-set.
- Rollback: delete `nem_pay/portal/`; nothing else depends on it.
