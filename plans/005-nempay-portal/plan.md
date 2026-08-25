# Plan — NemPay portal (merchant dashboard, read-only, multi-tenant)

**ID:** 005-nempay-portal · **Status:** done
**Spec:** ../../specs/005-nempay-portal/spec.md
**Constitution:** ../../CLAUDE.md (+ nem_pay/CLAUDE.md → "portal/ — React merchant dashboard")

> HOW only. Every choice satisfies the spec's acceptance criteria and obeys the constitution.

## Approach (HOW)
Two parts:
1. **Gateway (Go):** add **human session auth** and the **tenant-scoped read endpoints** the portal
   needs, all on the same public `/v1` surface. Read endpoints accept **either** a secret API key
   (machine) **or** a portal session JWT (human) — both resolve to a `merchant_id`. Money-mutating
   routes stay secret-key-only, untouched.
2. **Portal (React SPA)** in `nem_pay/portal/`: a read-only dashboard that consumes `/v1` through a
   generated TypeScript client, authenticated by a short-lived JWT held in memory.

Tenant isolation is enforced **server-side at the query layer** (every read filters by the
credential's `merchant_id`; every get-by-id includes `AND merchant_id = $auth`), and proven by
tests using two seeded merchants. This is the multi-tenant lesson: two identity types (session vs
API key) converging on one tenant scope, with isolation as a tested invariant.

## Architecture / components touched
**nem_pay/api/**
- `db/migrations/0007_portal_users.{up,down}.sql` — new `users` table.
- `internal/auth` (or `internal/httpapi`): JWT issue/verify (HS256) via `github.com/golang-jwt/jwt/v5`, password check via `golang.org/x/crypto/bcrypt` — hand-rolled, no auth framework.
- `internal/httpapi/`: `sessionAuth`/`authAny` middleware; `login` handler; new read handlers
  (balances, webhook events, api keys, payment ledger); router wiring; extend CORS to N origins.
- `internal/config`: `NEMPAY_JWT_SECRET`, `NEMPAY_JWT_TTL`, `NEMPAY_CORS_ORIGINS` (list).
- `internal/devseed`: seed a **second** merchant + one user per merchant (for isolation tests).
- `internal/repository` + `db/queries`: tenant-scoped reads (balances, webhook events/deliveries,
  api keys, entries for an intent).
- `openapi.yaml`: login + new read endpoints + a `Session` (bearer JWT) security scheme.

**nem_pay/portal/** (new)
- Vite + React + TS SPA; TanStack Query over a client generated from `openapi.yaml`
  (openapi-typescript / orval). Pages: Login, Payments (list), Payment detail (state + refunds +
  ledger), Balances, Webhooks, API keys. In-memory JWT + auth guard; a 401 sends the user to Login.

**nem_pay/docker-compose.yml** — portal runs via `npm run dev` (Vite :5173) for the first cut; a
compose service is deferred. CORS on the api must allow the portal origin.

## Key decisions & alternatives considered
- **Dual-credential on `/v1` reads (key OR session).** *Alt:* a separate `/v1/portal/*` namespace.
  *Why:* the constitution wants the portal to dogfood the exact public API; one surface, no backdoor.
- **Session = short-lived JWT in memory, NO refresh** (re-login on expiry). *Alt:* access + rotating
  refresh token in an httpOnly cookie. *Why:* user-chosen for the first cut; keeps the lesson on
  tenancy/isolation, not refresh mechanics. **Deliberate deviation** from the constitution's "JWT
  with refresh" — deferred to a later lesson, recorded here.
- **Auth is hand-rolled on `golang-jwt/jwt/v5` + `x/crypto/bcrypt`, not an auth framework.**
  *Alt:* `appleboy/gin-jwt` (Gin batteries) or an external IdP (Ory Kratos / SuperTokens). *Why:*
  matches the repo ethos — explicit, minimal deps, the session→tenant→isolation wiring is visible
  and reviewable (same rationale as sqlc-not-ORM); a framework/IdP would hide the very mechanics
  this feature exists to teach, and an IdP breaks the self-contained-gateway constraint.
- **Tenant scope comes from the credential, enforced at the query layer.** *Alt:* filter in handlers.
  *Why:* defense-in-depth (AC2/AC3); a leak can't happen by forgetting a handler filter.
- **Session credential is accepted ONLY on GET read routes + login; money POSTs stay secret-only.**
  *Why:* a browser session must never move money. `secretOnly`/`publishableOnly` reject the session
  kind, so sessions can reach only routes that explicitly opt in.
- **`users` with bcrypt password, seeded (no sign-up).** *Alt:* passwordless dev login. *Why:*
  realistic auth without onboarding scope; matches "seeded merchants/users" decision.
- **Generated TS client from OpenAPI.** *Alt:* hand-written fetch. *Why:* constitution; end-to-end
  types, client stays in step with the contract.
- **Webhook logs = `outbox` (event-level, has `merchant_id`) joined to `webhook_deliveries`
  (attempt-level).** *Why:* matches existing schema (0005 even notes "the portal shows this later").
- **Refunds surfaced from the intent's ledger refund entries, not a refunds table.** *Why:* the
  ledger is the source of truth; no new table needed for a read view.

## Data model / API changes
- **New table** `users(id uuid pk, merchant_id uuid fk→merchants, email text unique, password_hash
  text, created_at timestamptz)`.
- **New / changed endpoints** (tenant-scoped; accept secret key or session JWT unless noted):
  - `POST /v1/portal/login` *(public)* — `{email, password}` → `{token, expires_at, merchant:{id,name}}`.
  - `GET /v1/balances` — balances by account kind + currency (Σ entries) for the merchant.
  - `GET /v1/webhook_events` — outbox events + latest delivery status, newest-first, paginated.
  - `GET /v1/api_keys` — keys by kind + prefix + created/revoked, secret **masked**.
  - `GET /v1/payment_intents/:id/ledger` — the transaction(s)/entries backing the intent (+ refunds).
  - `GET /v1/payment_intents` and `/:id` — unchanged shape; auth extended to also accept a session.
- **OpenAPI:** add the `Session` bearer scheme and the endpoints above.
- **CORS:** `cors()` accepts a set of allowed origins (merchant site + portal dev origin).

## Risks & rollback
- **Risk — session reaching money routes.** *Mitigation:* session kind is rejected by
  `secretOnly`/`publishableOnly`; it is wired only into the GET read group. Test: a session token is
  refused (403/401) on `POST /v1/payment_intents` and the money verbs.
- **Risk — cross-tenant leakage.** *Mitigation:* query-layer scoping + isolation tests with two
  merchants for every read (list and get-by-id).
- **Risk — secret/PAN exposure.** *Mitigation:* api-keys endpoint returns only prefix (masked); no
  endpoint returns a token hash or a full key; PAN never exists in these paths. Test asserts.
- **Risk — JWT secret default in dev.** *Mitigation:* env-configurable; dev default clearly marked;
  document that prod must set it.
- **Risk — scope creep.** *Mitigation:* read-only, no refresh, no RBAC, no onboarding (spec).
- **Rollback:** all api changes are additive (new table, new endpoints, session accepted on reads);
  the money core is untouched. Revert = drop migration 0007, remove the session middleware/handlers
  and read endpoints, delete `nem_pay/portal/`. The gateway still runs standalone.

## Tasks (execute in order)
- [x] [task-01 — portal users table + multi-merchant seed](./task-01-users-and-seed.md)
- [x] [task-02 — JWT session login + key-or-session auth](./task-02-jwt-session-auth.md)
- [x] [task-03 — tenant-scoped read endpoints (+ OpenAPI, CORS)](./task-03-read-endpoints.md)
- [x] [task-04 — multi-tenant isolation & auth test suite](./task-04-isolation-tests.md)
- [x] [task-05 — portal scaffold: SPA, generated client, login + guard](./task-05-portal-scaffold-auth.md)
- [x] [task-06 — portal pages: payments, ledger, balances, webhooks, keys](./task-06-portal-pages.md)

**Order / dependencies:** 01 → 02 → 03 → 04 (04 verifies 02+03). Frontend: 05 depends on 02+03,
06 depends on 05. Backend (01–04) and the portal scaffold (05) can proceed in parallel once 03's
`openapi.yaml` is stable enough to generate the client.
