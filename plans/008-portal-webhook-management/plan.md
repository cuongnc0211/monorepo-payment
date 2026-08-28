# Plan — Portal webhook endpoint management

**ID:** 008-portal-webhook-management · **Status:** draft
**Spec:** ../../specs/008-portal-webhook-management/spec.md
**Constitution:** ../../CLAUDE.md (+ nem_pay/CLAUDE.md)

> HOW only. Satisfies the spec's ACs and obeys the constitution.

## Approach (HOW)
Three tenant-scoped endpoints on `/v1`, guarded by `authAny` (portal session OR secret key) — the
portal's first writes, and deliberately **non-money** (no ledger, no balance), so 005's "a session
cannot move money" invariant is untouched. The portal gets an endpoints section on the Webhooks
page: list, an add form, and disable.

## Key decisions & alternatives considered
- **Config writes accept a session (authAny), not secretOnly.** *Why:* they are configuration, not
  money; the money guard (secretOnly) is unchanged, so a session still cannot reach money routes.
  Tests assert these routes write no ledger entry / change no balance (AC5).
- **No Idempotency-Key on these config POSTs.** *Alt:* require it (API-conventions line). *Why:* the
  binding idempotency rule is about *money* ("every POST that moves or reserves money"); a
  double-created endpoint is a harmless config nuisance, not a money bug. **Deliberate, documented.**
- **Signing secret is write-only.** *Alt:* show-once. *Why:* the merchant already holds the secret
  (they verify signatures with it); the portal never needs to read it back. Reads omit it (AC6).
- **Create + disable, no edit-in-place.** *Why:* an endpoint's identity is its URL+secret; changing
  it = disable old, add new. Simpler and auditable.
- **Portal surfaces endpoints on the existing Webhooks page** (endpoints on top, recent deliveries
  below). *Why:* one place for "webhooks", fewer nav items.

## Data model / API changes
- **No schema change** (webhook_endpoints table exists: id, merchant_id, url, secret, created_at,
  disabled_at).
- New queries: `ListEndpoints` (all for a merchant, incl disabled), `DisableEndpoint`
  (set disabled_at where id + merchant). `InsertWebhookEndpoint` already exists.
- New endpoints (authAny, tenant-scoped, secret never returned):
  - `GET /v1/webhook_endpoints` → list `{ id, url, active, created_at }`.
  - `POST /v1/webhook_endpoints` `{ url, secret }` → create → `{ id, url, active, created_at }`.
  - `POST /v1/webhook_endpoints/{id}/disable` → `{ id, url, active:false, ... }`; 404 if not this merchant's.
- Validation: url required + http(s) + parseable; secret required non-empty (400 otherwise).
- OpenAPI: document the three (x-internal — portal/config surface).

## Risks & rollback
- **Risk — a config write drifts into money.** *Mitigation:* handlers only touch webhook_endpoints;
  a test asserts balances are unchanged after create/disable (AC5).
- **Risk — cross-tenant.** *Mitigation:* every query filters by merchant; disable-by-id → 404 for
  another merchant; isolation test.
- **Risk — secret leak.** *Mitigation:* list/response types omit `secret`; test asserts absence.
- **Rollback:** additive — drop the three routes/handlers/queries and the portal section. No schema.

## Tasks (execute in order)
- [ ] [task-01 — gateway: webhook-endpoint list/create/disable + tests](./task-01-gateway-endpoints.md)
- [ ] [task-02 — portal: endpoints section on the Webhooks page](./task-02-portal-endpoints.md)
