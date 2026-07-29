# monorepo-payment

A learning monorepo for payment-system design. It contains a from-scratch payment
gateway and *two contrasting merchants* that integrate it, so the entire payment
lifecycle — including escrow — can be exercised end to end on one machine.

This is a teaching project: correctness and clarity beat cleverness. Every architectural
choice here is a deliberate lesson, not an accident of convenience.

## Layout

```
monorepo-payment/
├── CLAUDE.md            (this file — repo-wide rules)
├── NemPay/              payment gateway (Go API + React portal + bank simulator)
│   └── CLAUDE.md
├── NemLuxury/           dummy luxury e-commerce merchant (Rails) — DIRECT capture
│   └── CLAUDE.md        (sells its own goods → money settles straight to the merchant)
└── NemTasker/           dummy Airtasker-style marketplace (Rails) — ESCROW
    └── CLAUDE.md        (brokers customer ↔ third-party tasker → funds held in escrow)
```

Read the folder-level CLAUDE.md before working inside a folder. Its rules win for code
in that folder; this file governs how the folders relate.

## The three systems and what each owns

- **NemPay** owns everything about *money*: payment intents, authorization, capture,
  the double-entry ledger, escrow, payouts, and webhooks. It never knows what a
  "supercar", an "order", or a "task" is.
- **NemLuxury** owns everything about *products and orders*: catalogue, cart, checkout,
  fulfilment. It is a *consumer* of NemPay — a reference integration. It never
  computes balances and never stores card data.
- **NemTasker** owns everything about *tasks and taskers*: task posting, offers,
  assignment, completion. It is the *second* reference integration. Same rules: never
  computes balances, never stores card data.

Keeping these ownerships clean is the point. Domain leakage across the boundary is a bug.

### Why two merchants (this is the escrow lesson)

The two merchants exist to exercise the **two shapes of money movement** through the same
gateway — that contrast *is* the syllabus:

- **NemLuxury — direct capture.** The merchant sells *its own* goods. There is no third
  party, so money flows straight through: `authorize → capture → settle` to the merchant.
  No funds are ever held on anyone's behalf. This is the simple path.
- **NemTasker — escrow (marketplace).** The platform brokers between a *customer* who pays
  and a *third-party tasker* who earns. NemPay must **hold the customer's money** from
  capture until the job is done, then **release it to the tasker minus a platform fee**:
  `authorize → capture → held_in_escrow → released`. Held funds are a liability NemPay
  carries and must prove against a segregated bank balance at all times.

Escrow is *not* an add-on bolted onto NemLuxury — it is the reason NemTasker exists. If
you ever find yourself escrowing money in NemLuxury, the model is wrong.

## How the pieces talk

```
DIRECT-CAPTURE flow (NemLuxury):
NemLuxury (Rails) ── server-to-server: POST /v1/payment_intents ─────▶ NemPay API (Go)
   ▲                 (secret key + Idempotency-Key)                        │
   └──────────── webhook (HMAC-signed: payment_intent.captured) ◀──────────┘

ESCROW flow (NemTasker):
NemTasker (Rails) ─ POST /v1/payment_intents { escrow: true, payee: tasker } ─▶ NemPay
   ▲                                                                              │
   │◀── webhook: payment_intent.held_in_escrow  (money captured, held) ──────────┤
   │                                                                              │
   └─ POST /v1/payment_intents/:id/release ──▶ NemPay  ── release to tasker ──────┘
      (job confirmed done; Idempotency-Key)      then webhook: escrow.released

Customer browser ── card entered into NemPay tokenization SDK ──▶ NemPay API
   (raw card number / PAN NEVER reaches a merchant server — PCI scope)

Merchant staff ──▶ NemPay portal (React) ──▶ NemPay API   (the same public /v1 API)
```

Three independent databases (NemPay on PostgreSQL; each merchant on its own file-based
SQLite). The merchants integrate with NemPay over HTTP only, exactly as separate companies
would — never a shared table, never a direct DB read across the boundary. Both merchants
speak the *same* `/v1` API; escrow is a mode of that API, not a separate one.

## Three planes (shared mental model)

A single transaction moves through three planes at different speeds. Keep them separate
in code and in your head:

- **Money plane** — authorize / capture / settle (talks to the bank simulator).
- **Ledger plane** — the internal double-entry source of truth.
- **Notification plane** — asynchronous webhooks out to merchants.

Never collapse them. A state change on the money plane *simultaneously* writes to the
ledger plane and emits an event to the notification plane — but each has its own timing
and failure model.

## Cross-cutting principles (the syllabus)

These live in NemPay and shape how NemLuxury integrates. Each is a system-design lesson:

1. **Idempotency on every money-mutating write.** Insert-first against a UNIQUE
   constraint; never check-then-act. Retries must be safe by construction.
2. **Double-entry ledger as source of truth.** Append-only entries; a balance is
   *derived by summing entries*, never a mutable column.
3. **Explicit state machines.** Payment and escrow states move only along allowed
   edges. No implicit jumps.
4. **Outbox + async webhooks.** State changes write an event to an outbox in the same
   transaction; a separate worker delivers at-least-once with backoff and HMAC signing.
5. **Money is integer minor units + a currency.** Never floats. Luxury prices are large
   — use int64 cents and mind overflow when aggregating.
6. **Reconciliation is mandatory.** The internal ledger must be provable against the
   external "bank" at all times; escrow held must equal segregated funds on hand.

## Spec-driven development (how we work here)

This repo is built **spec-first**: the spec is the source of truth, code is derived from it.
Three artifact layers, three homes:

- **Constitution** — the `CLAUDE.md` files. Non-negotiable rules; auto-loaded every session.
- **Spec** — `specs/<NNN>-<slug>/spec.md`. The WHAT and WHY: scope, behaviours, acceptance
  criteria. **No HOW.** Stable — this is the contract.
- **Plan & tasks** — `plans/<NNN>-<slug>/plan.md` + `task-NN-*.md`. The HOW: architecture,
  decisions, and code-level task breakdown with test matrices. Evolves as implementation learns.
  `plans/ROADMAP.md` is the program-level index across features.

`specs/` and `plans/` share the same `<NNN>-<slug>` per feature so the two halves line up.
Templates live in `specs/templates/` and `plans/templates/`.

Workflow — each step is a slash command in `.claude/commands/sdd/`:
`/sdd:spec` → `/sdd:plan` → `/sdd:tasks` → `/sdd:implement` → `/sdd:verify`.

Four discipline rules (these make it spec-driven, not just documented):
1. **Spec is authoritative.** If code and spec disagree, fix the code — or consciously update the
   spec. Never let them silently diverge.
2. **Spec-first change.** When requirements change, edit `spec.md` first, then re-plan and
   re-implement. Do not patch code to a behaviour the spec doesn't describe.
3. **Verify against the spec.** "Done" means the spec's acceptance criteria pass, not "it runs".
4. **Spec travels with code.** One feature = one `specs/` folder + one `plans/` folder = one PR;
   the spec is reviewed alongside the diff.

## Repo conventions

- Conventional Commits, scoped by component: `feat(nempay): …`, `fix(nemluxury): …`,
  `feat(nemtasker): …`, `chore(repo): …`.
- Secrets via `.env` / Rails credentials, always git-ignored. Never commit keys.
- **NemPay ships as a self-contained gateway.** `NemPay/docker-compose.yml` brings up
  everything the gateway needs — PostgreSQL, Redis, the Go api, and bank-sim — and runs
  migrations on boot, so a single `docker-compose up` inside `NemPay/` gives you a working
  payment gateway with the `/v1` API listening locally. Nothing else is required to run it.
- **The merchants are separate consumers, not part of that compose.** NemLuxury and
  NemTasker run independently — either their own container or just `bin/rails s` locally —
  and reach NemPay over HTTP via a `NEMPAY_API_URL` (+ API keys) in their own env. This
  mirrors reality: the gateway is a service you stand up; merchants are separate companies
  that point at it. Each merchant keeps its own file-based SQLite — no DB container, no Redis.
- Redis lives **only** in NemPay, on purpose — a real gateway delivers webhooks out-of-band,
  and modelling that faithfully is part of the lesson. The dummy merchants have no such
  need, so they handle inbound webhooks synchronously. The NemPay portal is deferred.
- No cross-component imports. If a merchant needs a NemPay type, it talks to the API.
