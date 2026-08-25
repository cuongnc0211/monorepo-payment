# Plan — NemPay escrow settlement mode (M3)

**ID:** 003-nempay-escrow · **Status:** complete (all tasks done; escrow suite + direct regression green + live docker e2e)
**Spec:** ../../specs/003-nempay-escrow/spec.md
**Constitution:** ../../CLAUDE.md, ../../nem_pay/CLAUDE.md

> This file is the **HOW**. Every choice satisfies the spec's acceptance criteria and obeys the
> constitution. Tasks are decomposed separately in `/sdd:tasks`.

## Approach (HOW)
Add an **`escrow` settlement mode** to the existing gateway using the six **escrow-adaptability
seams** built (unused) in M1 — so M3 is *additive, not a rewrite*. Escrow reuses the direct
**money plane** (`authorize → capture → settle`) with the ledger **destination chosen by mode**,
then adds an explicit, idempotent **release** and a **full refund-from-escrow**, plus a
**segregation reconciliation** invariant. The customer's money is pulled at capture (→ `captured`),
settled into a **segregated** account (→ `held_in_escrow`), and on release moves to the payee minus
a flat fee (→ `released`). One migration widens two `CHECK` constraints; no new account *type*, no
new tables (`payee_id` / `application_fee` already exist), and **bank-sim is unchanged**.

## Architecture / components touched (all in `nem_pay/api`)
- `internal/statemachine/intent.go` — make the allowed-edges map **mode-aware**; add `held_in_escrow`,
  `released` statuses, `escrow` settlement mode, and the escrow edge set.
- `internal/ledger/accounts.go` — add escrow account **kind** constants (string only, no schema):
  `segregated_cash` (asset), `escrow_liability` (liability, per-intent), `payable_to_payee`
  (liability, per-payee), `platform_revenue` (revenue).
- `db/migrations/0006_escrow.*` — widen the `payment_intents.status` CHECK (`+held_in_escrow`,
  `+released`) and `settlement_mode` CHECK (`+escrow`).
- `internal/service/payment_intent.go` (Create) — accept `{escrow, payee, application_fee}`; validate.
- `internal/service/money.go` — mode-aware postings: **Capture** (credit destination by mode),
  extend the **settle** sweep (escrow → `held_in_escrow` into segregated cash), **Refund** (escrow
  branch, full, from `held_in_escrow` only), new **Release**; escrow event emission.
- `internal/httpapi/payment_intents.go` + `router.go` — create accepts escrow fields; new
  `POST /v1/payment_intents/:id/release` (secret key + Idempotency-Key); refund handler mode-aware.
- Reconciliation — add the segregation check (`Σ escrow_liability(held) == segregated_cash`).
- `nem_pay/CLAUDE.md` — refine the escrow-ledger wording (`Dr platform-cash` → segregated cash),
  the conscious refinement the spec called out.

## Key decisions & alternatives considered
- **Mode-aware state machine.** `allowedEdges` becomes keyed by settlement mode. *Direct* edges are
  exactly M1's; *escrow* edges are `created → requires_confirmation → authorized → captured →
  held_in_escrow → released`, `held_in_escrow → refunded`, and the pre-capture `→ failed` edges.
  `CanTransition(mode, from, to)`. *Alt:* one permissive map + service-enforced mode. *Why:* escrow
  states genuinely differ by mode; keying by mode blocks illegal cross-mode edges structurally.
  M1 callers change minimally — they already hold `pi.SettlementMode`.
- **Reuse the capture→settle money plane; destination by mode (guard #3 extended).** Capture posts
  `Dr acquirer_receivable / Cr {merchant_payable | escrow_liability(intent)}` → `captured`; the
  settle sweep posts `Dr {platform_cash | segregated_cash} / Cr acquirer_receivable` →
  `{settled | held_in_escrow}`. *Alt:* escrow capture straight into segregated cash. *Why:* faithful
  (capture pulls to a receivable, settle lands the cash), reuses M1 machinery, and makes `captured`
  a real in-transit state (the resolved spec decision).
- **Release moves ALL cash out of segregation (Option A — chosen).** One balanced 5-leg posting:
  `Dr escrow_liability A · Dr platform_cash A / Cr segregated_cash A · Cr payable_to_payee (A−fee) ·
  Cr platform_revenue fee` → `released`. *Alt:* keep the payee's cash segregated until payout
  (Option B). *Why:* keeps `Σ escrow_liability == segregated_cash` exact (spec AC6), mirrors
  direct-mode `merchant_payable` (payout deferred), and matches real marketplaces (post-transfer
  funds are a normal balance, not escrow).
- **`payable_to_payee` per-payee; `escrow_liability` per-intent.** Both are per-reference accounts
  (guard #2): the liability is summed per intent for the invariant; the payable accrues per payee
  across intents. *Alt:* per-intent payable. *Why:* a payee earns across many intents — realistic.
- **Refund from escrow: full-only, from `held_in_escrow`.** `Dr escrow_liability A / Cr
  segregated_cash A` → `refunded` (ledger-only, as in M1). Refund after `released` is rejected by the
  state machine. *Why:* spec scope (partial and refund-after-release deferred).
- **Release is explicit, idempotent, and locked.** New endpoint wrapped in the task-03 idempotency
  middleware; `SELECT … FOR UPDATE` on the intent. A retry replays; a concurrent double-release
  blocks then sees `released` and is rejected. No auto-release.
- **Fee validation at create.** `0 ≤ application_fee ≤ amount`; `payee` required and a valid UUID
  when `escrow: true`. Mode / payee / fee immutable after create (the columns are set once).
- **Events.** `eventTypeFor` gains explicit cases: `held_in_escrow → payment_intent.held_in_escrow`,
  `released → escrow.released`. Emitted to the outbox in the same tx (M1 machinery, guard #5).

## Data model / API changes
**Migration `0006_escrow` (additive):**
- `payment_intents.status` CHECK gains `held_in_escrow`, `released`.
- `payment_intents.settlement_mode` CHECK gains `escrow`.
- No new columns/tables — `payee_id` (uuid) and `application_fee` (bigint) already exist (M1 guard #1).

**New ledger account kinds** (string constants; `accounts.type`/per-reference already support them):
`segregated_cash`, `escrow_liability`, `payable_to_payee`, `platform_revenue`.

**Escrow ledger postings** (`A` = amount, `fee` = application_fee):

| Event | Debit | Credit | Result |
|---|---|---|---|
| capture | `acquirer_receivable` A | `escrow_liability(intent)` A | `captured` |
| settle | `segregated_cash` A | `acquirer_receivable` A | `held_in_escrow` |
| release | `escrow_liability` A · `platform_cash` A | `segregated_cash` A · `payable_to_payee(payee)` (A−fee) · `platform_revenue` fee | `released` |
| refund (from held) | `escrow_liability` A | `segregated_cash` A | `refunded` |

**API:**
- `POST /v1/payment_intents` accepts `{ escrow: true, payee: "<uuid>", application_fee: <cents> }`.
- `POST /v1/payment_intents/:id/release` — secret key + `Idempotency-Key`.
- `POST /v1/payment_intents/:id/refund` — mode-aware (escrow: full, from held).

## Acceptance coverage (spec AC ↔ plan)
| Spec AC | Covered by |
|---|---|
| AC1 create escrow + validate/immutable | Create validation + `settlement_mode` CHECK |
| AC2 capture → captured (escrow liability) | mode-aware Capture posting |
| AC2b settle → held_in_escrow (segregated) | mode-aware settle sweep |
| AC3 release minus fee | Release 5-leg posting + event |
| AC4 idempotent/concurrent release | idempotency wrapper + FOR UPDATE + state machine |
| AC5 full refund from held; reject after release | mode-aware Refund + state machine |
| AC6 segregation invariant | per-intent `escrow_liability` vs `segregated_cash` + reconciliation check |
| AC7 state-machine integrity | mode-aware `CanTransition`; illegal edges rejected pre-posting |
| AC8 direct mode unchanged | direct edge set = M1's; regression tests |

## Risks & rollback
- **Invariant during the transit window.** Between `capture` and `settle`, `escrow_liability` exists
  but `segregated_cash` does not (funds are an `acquirer_receivable`). The invariant is scoped to
  **held** funds; reconciliation sums `escrow_liability` for `held_in_escrow` intents vs
  `segregated_cash`. Document the transit window (a captured liability is backed by the receivable).
- **Mode-aware state machine touches M1 callers.** Passing the mode risks direct-mode regressions.
  *Mitigation:* the direct edge set is byte-identical to M1; AC8 regression suite.
- **Release sign errors** in a 5-leg posting skew balances silently. *Mitigation:* the M1 sum-zero
  property test + golden-balance tests + the segregation-invariant test all run on escrow flows.
- **Fee edges:** `fee == amount` → payee accrues 0 (allowed); `fee == 0` → no revenue (allowed);
  `fee > amount` or negative → rejected at create. Tested.
- **Concurrency:** double-release and release-vs-refund races resolved by `FOR UPDATE` + the state
  machine (`held_in_escrow` is the only source for both, and the first commit wins).
- **CHECK migration** is additive; a rollback that narrows the CHECK is only safe before any escrow
  rows exist. The ledger is append-only — never delete to undo; post reversing entries.
- **Rollback:** escrow is purely additive — not creating escrow intents and removing the release
  route reverts behaviour; direct mode is untouched.

## Tasks (execute in order)
- [x] [task-01 — Migration + mode-aware state machine + account kinds](./task-01-schema-statemachine-accounts.md)
- [x] [task-02 — Create escrow intent + mode-aware capture](./task-02-create-escrow-and-capture.md)
- [x] [task-03 — Settle into segregation (held_in_escrow)](./task-03-settle-into-escrow.md)
- [x] [task-04 — Release to payee minus fee](./task-04-release.md)
- [x] [task-05 — Full refund from escrow](./task-05-refund-from-escrow.md)
- [x] [task-06 — Segregation reconciliation + constitution refinement](./task-06-segregation-reconciliation.md)
- [x] [task-07 — Lifecycle tests + direct regression + e2e](./task-07-tests-and-e2e.md)

**Dependency order:** 01 → 02 → 03 → {04, 05} → 06 → 07. Tasks 04 (release) and 05 (refund) both
depend only on 03 and can be done in either order.

**Commit convention:** each task lands as **one atomic commit** once its tests pass — conventional,
component-scoped (`feat(nempay): …`, `test(nempay): …`, `docs(nempay): …`), no cross-task mixing.

## Acceptance coverage (task ↔ spec AC)
| Task | Covers |
|---|---|
| task-01 | AC7, AC8 (foundation) |
| task-02 | AC1, AC2 |
| task-03 | AC2b |
| task-04 | AC3, AC4 |
| task-05 | AC5 |
| task-06 | AC6 |
| task-07 | AC1–AC8 (integration + regression) |
