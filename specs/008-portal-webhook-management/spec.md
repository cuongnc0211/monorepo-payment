# Spec — Portal webhook endpoint management

**ID:** 008-portal-webhook-management · **Status:** draft · **Author:** cuongnc0211 · **Date:** 2026-08-26
**Constitution:** ../../CLAUDE.md (+ nem_pay/CLAUDE.md)
**Plan:** ../../plans/008-portal-webhook-management/plan.md

> WHAT and WHY only. No HOW — that lives in the plan.

## Why
The portal (005) is read-only. But a merchant needs to configure **where** NemPay delivers webhooks
— the endpoint URL and the HMAC signing secret — and today only the dev seed can create one. This
adds the **first portal write**, deliberately confined to **non-money configuration**: managing
webhook endpoints. It preserves the core invariant from 005 that a portal session **cannot move
money** — these operations touch no ledger, no payment, no balance.

## What — scope

**In scope**
- **List** the authenticated merchant's webhook endpoints (URL, created, active/disabled).
- **Create** a webhook endpoint (URL + signing secret); it then receives future events.
- **Disable** a webhook endpoint (it stops receiving events).
- All scoped to the merchant; callable with a portal **session** or a secret key.

**Out of scope / non-goals**
- Any money movement (refunds, capture, release) — the "session cannot move money" invariant stays.
- API-key management (a separate feature), editing an endpoint's URL in place (disable + create a new
  one instead), resending/retrying past deliveries, and per-event-type subscriptions.

## Behaviours / user stories
- As merchant staff, I can see my webhook endpoints and whether each is active or disabled.
- As merchant staff, I can add an endpoint by entering a URL and a signing secret; new events then
  deliver there.
- As merchant staff, I can disable an endpoint I no longer want to receive events.
- As staff of merchant A, I can never see, create against, or disable merchant B's endpoints.

## Acceptance criteria (testable — the definition of done)
- [ ] **AC1** The portal lists only the authenticated merchant's endpoints, each with URL, created
  time, and active/disabled state.
- [ ] **AC2** Creating an endpoint (URL + secret) persists it for the merchant and it appears in the
  list; subsequent events are delivered to it.
- [ ] **AC3** Disabling an endpoint marks it disabled; it stops receiving events and shows disabled.
- [ ] **AC4** Tenant isolation: a merchant cannot list, create against, or disable another merchant's
  endpoints; addressing another merchant's endpoint by id returns 404.
- [ ] **AC5** These writes accept a portal session but are **not** money routes — they write no
  ledger entry and change no balance; a session performing them never moves money (005's invariant
  holds).
- [ ] **AC6** The signing **secret is write-only** to the portal: it is required to create, but the
  list/read never returns the full secret.
- [ ] **AC7** A create with a missing/invalid URL or an empty secret is rejected (400).

## Constraints (from the constitution)
- Tenant scoping is enforced **server-side** (defense in depth), like every other read/write.
- The webhook signing secret is never returned by a read (mirrors API-key masking).
- One consistent error envelope.
- These endpoints must not be money routes: no ledger write, no balance change (so the session that
  can call them still cannot move money).

## Open questions
- **Idempotency-Key:** the money-idempotency rule targets money POSTs; do these config POSTs also
  require an Idempotency-Key, or are they exempt as non-money config? (Plan.)
- Confirm edit-in-place is out (create + disable is the model).
- Confirm the secret is purely write-only (never shown back), since the merchant already holds it to
  verify signatures on their side.
