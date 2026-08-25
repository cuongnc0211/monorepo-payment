# Spec — NemPay escrow settlement mode (M3)

**ID:** 003-nempay-escrow · **Status:** active · **Author:** cuong.nguyen · **Date:** 2026-08-25
**Constitution:** ../../CLAUDE.md, ../../nem_pay/CLAUDE.md
**Plan:** ../../plans/003-nempay-escrow/plan.md

> This file captures **WHAT** and **WHY** only. No HOW (no tech choices, schema, or code) — that
> lives in the plan. Keep it stable; it is the contract.

## Why
NemPay today only does **direct capture** (M1): a merchant sells its own goods and money settles
straight to it. The *other* shape of money movement — the marketplace — has the platform broker a
**customer** who pays and a **third-party payee** who earns. There, the gateway must **hold** the
customer's captured money as a liability from capture until the platform confirms the job is done,
then **release** it to the payee minus a platform fee. Escrow is the second of the two contrasting
money flows this project exists to teach (direct vs escrow), and it introduces the **segregation /
reconciliation** discipline a real custodial system is legally required to uphold: customer money
held on behalf of others must be provably segregated, never commingled with the platform's own cash
and never counted as revenue until earned. The escrow *seams* were deliberately built (unused) in
M1, so this extends the **same** gateway additively rather than rewriting it.

## What — scope
**In scope**
- A new **`escrow` settlement mode** on the same `/v1` payment-intent API, chosen at creation via
  `{ "escrow": true, "payee": "<id>", "application_fee": <cents> }`. The **mode, payee, and fee are
  immutable** once set; a direct intent can never become an escrow one, or vice-versa. Absent the
  `escrow` flag, behaviour is exactly direct mode (unchanged).
- **Capture-into-escrow**: capturing an escrow intent **holds** the funds — the captured amount
  becomes an **escrow liability** held on behalf of the payee, and the corresponding cash is held in
  a **segregated** account, distinct from the platform's own cash. The intent reaches
  `held_in_escrow`.
- **Explicit, idempotent release**: `POST /v1/payment_intents/:id/release`, called by the platform
  only after the job is confirmed done. Release moves the held amount to the payee
  (**amount − fee**) and the flat **`application_fee`** to platform revenue; the intent becomes
  `released` (terminal). NemPay never auto-releases on a timer.
- **Full refund from escrow**: while held, the **full** held amount can be refunded to the payer;
  the intent becomes `refunded` (terminal). Refund is **not** allowed once released.
- **Payee as an opaque identifier**: the platform supplies the payee id; NemPay records it and
  **accrues** an amount payable to that payee on release. NemPay does **not** pay the payee out (as
  in direct mode, the payable simply stands).
- **Segregation reconciliation invariant**: at all times, the total of all escrow-liability balances
  equals the segregated escrow-cash balance. Held money is a liability, never revenue until released.
- Escrow **state changes emit webhooks** out-of-band (held / released / refunded events) via the
  existing outbox + worker, HMAC-signed, at-least-once, deduped on a stable `event_id`.
- **Idempotency** on every money-mutating escrow write (create, capture, release, refund), and
  **concurrency safety** (a held intent can never be double-released or released-and-refunded).

**Out of scope / non-goals**
- **Paying the payee out** to real bank rails — accrual only; payout is deferred (as in direct mode).
- A **payee entity / registration / KYC** — the payee is an opaque id this round.
- **Partial release, partial refund from escrow, and refund-after-release** — deferred.
- **Disputes / chargebacks**, **auto-release on timeout**, **multi-currency escrow** — deferred.
- The **NemTasker merchant integration** — that is M4; M3 is the gateway capability, exercised
  standalone (via `curl` / tests) with no merchant present.
- Any change to **direct-mode (M1) behaviour** — escrow is purely additive.

## Behaviours / user stories
- As a platform, when I create an intent with `escrow: true`, a `payee`, and an `application_fee`,
  then it is an escrow intent whose mode / payee / fee cannot afterwards change.
- As a platform, when an escrow intent is **captured**, then the customer's funds are pulled and
  recorded as an escrow **liability** owed to the payee; the intent is `captured` (the cash is
  in transit from the acquirer, not yet in hand). When those funds **settle**, they move into a
  **segregated** escrow account and the intent becomes `held_in_escrow`, with a *held_in_escrow*
  webhook delivered — the money is never the platform's and never revenue while held.
- As a platform, when I **release** a held intent, then the held amount **minus the flat fee** is
  accrued to the payee, the **fee** becomes platform revenue, the intent is `released`, and a
  *released* webhook is delivered. Releasing again does **not** move money twice.
- As a platform, when I **refund** a held intent, then the **full** held amount returns to the
  payer, the intent is `refunded`, and a *refund* webhook is delivered. Refunding a **released**
  intent is rejected.
- As an operator, for all funds **held in escrow**, the segregated escrow-cash on hand equals the
  total held escrow liability — customer money is provably set aside, never commingled.
- As an integrator using **direct** mode, nothing changes.

## Acceptance criteria (testable — the definition of done)
- [ ] **AC1 — Create escrow intent.** `{escrow:true, payee, application_fee}` creates an escrow-mode
  intent; mode / payee / fee are immutable thereafter. A missing/blank payee, or an
  `application_fee` that is negative or greater than the amount, is **rejected** at creation.
- [ ] **AC2 — Capture (escrow).** Capturing an escrow intent posts **one balanced** transaction that
  records the pulled funds as an **escrow liability** owed to the payee (backed by an in-transit
  escrow receivable) and moves the intent to `captured`. The amount is a **liability**, never revenue
  or platform cash.
- [ ] **AC2b — Settle into segregation.** The settlement step moves an escrow intent's funds into a
  **segregated escrow-cash** account (the same money-plane settlement as direct mode, with the
  destination chosen by mode), moves the intent to `held_in_escrow`, and delivers a *held_in_escrow*
  webhook. Each posting is balanced.
- [ ] **AC3 — Release to payee minus fee.** Releasing a `held_in_escrow` intent posts **one
  balanced** transaction: debit `escrow-liability` (full), credit `payable-to-payee` (amount − fee),
  credit `platform-revenue` (fee); moves the intent to `released`; delivers an *escrow.released*
  webhook. Fee arithmetic is exact (integer cents).
- [ ] **AC4 — Idempotent & concurrency-safe release.** A retried release (same `Idempotency-Key`)
  and a concurrent double-release each result in **exactly one** release posting; a distinct release
  attempt on an already-`released` intent is rejected with **no** new posting.
- [ ] **AC5 — Full refund from escrow.** Refunding a `held_in_escrow` intent posts **one balanced**
  transaction debiting `escrow-liability` and crediting `refund-to-payer` for the full amount, moves
  the intent to `refunded`, and delivers a *refund* webhook. Refunding a `released` intent is
  **rejected** with no posting.
- [ ] **AC6 — Segregation invariant.** An escrow liability is **always fully backed**: by an
  in-transit escrow receivable before settlement, and by **segregated escrow cash** once settled.
  In particular, for all funds in `held_in_escrow`, the summed `escrow-liability` **equals** the
  segregated escrow-cash balance (both derived from the ledger). Released fees appear in
  `platform-revenue` only **after** release; held money is never revenue.
- [ ] **AC7 — State-machine integrity.** Only the escrow edges are permitted; illegal operations
  (release a non-held intent, refund a released one, release/refund a direct intent, re-hold an
  already-held intent) are **rejected with no ledger posting**.
- [ ] **AC8 — Direct mode unaffected.** The existing direct-capture lifecycle, ledger postings, and
  reconciliation are unchanged — a regression check across M1's behaviour passes.

## Constraints (from the constitution or product)
- Money is **integer minor units + currency**, never floats; aggregate reports computed in the DB's
  `numeric` type.
- Escrow **held funds are a liability**; once settled, the matching cash is held in a **segregated**
  account, **not** commingled with `platform_cash`. Segregated cash is **sourced by settling** the
  escrow receivable created at capture (the same money-plane capture→settle as direct mode, with the
  settlement destination chosen by mode) — never conjured. *(This consciously resolves the
  constitution's escrow ledger toward its own segregation invariant.)*
- Every escrow money event is **one balanced transaction written in the same DB transaction** as the
  state change it represents; webhooks are emitted to the outbox in that same transaction and
  delivered **out-of-band**.
- **Idempotency** is required on every money-mutating escrow POST; the money path is
  **concurrency-safe** (a held intent is never double-released, released-then-refunded, etc.).
- `application_fee` is a **single flat amount**, `0 ≤ fee ≤ captured amount`, fixed at creation.
- No real payout and no real processor — money movement is simulated via `bank-sim`.
- **Additive & backward-compatible**: direct-mode intents and the existing public API behave exactly
  as in M1.

## Resolved decisions (were open questions)
- _(resolved)_ **Keep the `captured` state for escrow.** Escrow flows
  `authorized → captured → held_in_escrow → released` (and `held_in_escrow → refunded`). Capture
  pulls the customer's funds (→ `captured`); a settlement step then moves them into segregation
  (→ `held_in_escrow`). `captured` is meaningful — it is the in-transit window between charging the
  customer and the funds arriving in the segregated account, mirroring direct mode.
- _(resolved)_ **Segregated cash is sourced by settling the escrow receivable.** Capture posts to an
  escrow receivable; the settlement sweep moves it into segregated cash (destination chosen by mode).
  The segregation invariant (AC6) therefore holds for **settled/held** funds; a captured-not-yet-
  settled liability is backed by the in-transit receivable.
- _(resolved / constitution refinement)_ Escrow-held cash lives in a **segregated** account (not
  `platform_cash`); `nem_pay/CLAUDE.md`'s "Dr platform-cash" wording is refined toward its own
  segregation invariant during planning.
