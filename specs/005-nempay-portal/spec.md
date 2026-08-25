# Spec — NemPay portal (merchant dashboard, read-only, multi-tenant)

**ID:** 005-nempay-portal · **Status:** draft · **Author:** cuongnc0211 · **Date:** 2026-08-25
**Constitution:** ../../CLAUDE.md (+ nem_pay/CLAUDE.md → "portal/ — React merchant dashboard")
**Plan:** ../../plans/005-nempay-portal/plan.md

> WHAT and WHY only. No HOW (no tech choices, schema, or code) — that lives in the plan. The
> constitution already fixes several HOW choices for the portal (React SPA over `/v1`, generated
> client, JWT sessions); those are honoured by the plan, not re-decided here.

## Why
NemPay is the authoritative system for money, but the only way to observe that money today is
`curl` against `/v1` or `psql` into the ledger. There is no human-facing surface to see which
payments exist, what state they are in, what the balances are, or whether webhooks are being
delivered.

This feature has **two deliberate learning goals**:
1. **A read-only observability dashboard** — a real consumer that dogfoods the public `/v1` API and
   forces it to be complete.
2. **Multi-tenant design** — the reason to build it now. NemPay is already multi-tenant in its
   data (a `merchants` root, everything scoped by merchant), but it only has machine auth (API
   keys). The portal adds the missing half: **human authentication via login sessions** that
   resolve to a tenant, and **strict tenant isolation** enforced server-side and proven by tests —
   the same two-identities model (dashboard session vs API key) that a gateway like Stripe uses.

The first cut is intentionally narrow so "multi-tenant" is the lesson, not "clone all of Stripe":
read-only surface, no write actions, no feature breadth.

## What — scope

**In scope**
- **Authentication (human sessions).** The portal is behind a login. A signed-in user is tied to
  exactly **one** merchant; their session resolves to that merchant for every request. Nothing is
  visible without signing in.
- **Tenant isolation as a first-class invariant.** Every read is scoped to the session's merchant,
  enforced on the server (not by the client remembering to filter). At least **two** merchants
  exist (dev-seeded) specifically so isolation can be exercised and tested.
- **Payments / transactions (read).** A list of payment intents, newest first, with status, amount
  + currency, settlement mode, created time; filterable by status and paginated. A detail view for
  one payment: status, settlement mode, metadata, associated refunds, and the double-entry ledger
  transaction(s)/entries backing it.
- **Ledger & balances (read).** Balances **derived from the ledger** (sum of entries), by account
  kind and currency, matching what the API computes.
- **Refunds (read).** Refunds associated with payments (amount, status, time).
- **Webhook delivery logs (read).** Events NemPay emitted and their delivery attempts: event type,
  delivery status (delivered / retrying / failed / dead-lettered), timestamps.
- **API keys (read).** The merchant's keys by kind (publishable / secret) and prefix, secret
  **masked** — enough to identify a key, never enough to use it.

**Out of scope / non-goals**
- **Self-service onboarding:** sign-up, creating a merchant, provisioning keys from the UI. First
  cut logs into **seeded** merchants/users; onboarding is a later cut.
- **RBAC / multiple users per tenant / teams / invitations / roles.** First cut is **one user ↔
  one merchant**, no roles. Multi-user tenancy is its own later lesson.
- **Any write / money-moving action:** issuing refunds, capture/release, creating or revoking keys,
  creating or editing webhook endpoints, resending webhooks.
- **Escrow-specific views** (held funds, releases, segregation proof) — deferred until escrow (M3)
  is implemented; first cut targets the direct-capture surface (M1/M2).
- **Stripe feature breadth:** Connect, Billing, Radar/fraud, disputes, payouts to bank rails, tax,
  reporting/Sigma, Terminal — all out.
- Card tokenization or card entry (lives in the checkout surface, not here).

## Behaviours / user stories
- As merchant staff, when I open the portal without signing in, then I am asked to authenticate and
  see no merchant data until I do.
- As merchant staff, when I sign in, then every screen shows **only my merchant's** data.
- As staff of merchant A, I can never see any payment, balance, refund, webhook, or key belonging
  to merchant B — even by guessing an id or calling an endpoint directly.
- As merchant staff, when I sign in, I see my payments newest-first, can filter by status, and page
  through them.
- As merchant staff, when I open one payment, I see its full state, its refunds, and the ledger
  entries backing it — amounts exactly as the API reports them.
- As merchant staff, I can view my balances (derived from the ledger), my webhook delivery history,
  and my API keys with secrets masked.

## Acceptance criteria (testable — the definition of done)
- [ ] **AC1** The portal requires authentication; an unauthenticated visitor can retrieve **no**
  merchant data from any portal-facing request.
- [ ] **AC2 (isolation — the core multi-tenant AC)** With two seeded merchants A and B, a session
  for A returns only A's payments, balances, refunds, webhook logs, and keys; requesting a specific
  resource that belongs to B (e.g. by its id) is refused/not-found, never disclosed. Enforced
  server-side and covered by an automated test.
- [ ] **AC3** A session is bound to exactly one merchant; the tenant scope comes from the session,
  not from any client-supplied merchant identifier.
- [ ] **AC4** The payments list shows intents newest-first with status and amount+currency
  formatted from integer minor units (no floats); supports filtering by status and pagination.
- [ ] **AC5** A payment detail shows status, settlement mode, metadata, associated refunds, and the
  backing ledger transaction(s)/entries; every displayed amount equals the value returned by the
  API (the portal performs **no** money arithmetic of its own).
- [ ] **AC6** The balances view shows balances derived from the ledger (sum of entries) per
  currency, matching the API's computed figures.
- [ ] **AC7** Webhook deliveries are listed with event type, delivery status, and timestamps;
  delivered vs retrying vs failed are visually distinguishable.
- [ ] **AC8** API keys are listed by kind and prefix with the secret masked; a full secret is never
  rendered in the UI nor returned by any request the portal makes.
- [ ] **AC9** No card PAN and no full API secret appears anywhere in the portal UI or its network
  traffic.
- [ ] **AC10** The portal is read-only: it exposes no control that mutates gateway state.
- [ ] **AC11** Every figure shown is fetched from the public `/v1` API — the portal has no direct
  database access and no private backdoor.

## Constraints (from the constitution)
- The portal is a **consumer** of the public `/v1` API — no shared database, no direct DB reads, no
  private backdoor (nem_pay/CLAUDE.md).
- The portal owns no money domain: it **displays** amounts the API computes and never recomputes
  balances client-side.
- Raw card data must never be held or shown by the portal.
- Authentication and tenant scoping are enforced **server-side** (defense in depth); the client is
  never trusted to scope its own data.
- The portal is a separate app from the gateway's money core; standing it up or breaking it must
  never affect the gateway (the gateway stays usable standalone).

## Open questions
- **Session auth mechanism.** The constitution names JWT portal sessions. The plan must decide the
  concrete login + session scheme (credential storage, token issuance/refresh, how the session is
  presented to `/v1`) and how it coexists with the existing API-key auth. (HOW → plan.)
- **User ↔ merchant seed model.** First cut logs into seeded users. What is the minimal user record
  (email + credential) and how is it tied to a merchant in the seed? (HOW → plan.)
- **Missing read endpoints.** `/v1` today exposes payment-intent list/get. Balances, refunds list,
  webhook-delivery logs, and an API-keys list likely need new **read** endpoints/fields, all
  tenant-scoped. Which does the plan add?
- **State-change timeline.** Does the payment detail also want a chronological state history
  (created → authorized → captured → …), and can the gateway expose it (from the outbox/events)?
- **Landing view.** A summary/overview (counts, totals) for the signed-in merchant, or just the
  entity lists?
- **Escrow readiness.** Confirmed: escrow views are deferred until after M3 lands.
