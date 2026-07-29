# NemTasker

A dummy Airtasker-style task marketplace — a customer posts a task ("assemble this
wardrobe", "move a sofa"), taskers make offers, one is accepted and does the work — built
in Rails. It exists for one reason: to be the **escrow** reference merchant that
integrates **NemPay**. It owns tasks, offers, and assignments; it owns nothing about how
money moves.

This is the **escrow** counterpart to NemLuxury's direct-capture integration. The
difference is the whole lesson: NemLuxury sells its own goods (money settles straight to
the merchant), whereas NemTasker brokers between a *customer who pays* and a *third-party
tasker who earns* — so the platform must **hold the customer's money until the job is
done**, then **release it to the tasker minus a platform fee**. Read the two apps together.

Rails is the right tool *here* because this app owns a real domain (tasks, offers,
assignments) — exactly where ActiveRecord + conventions shine. It is a *consumer* of
NemPay, never an owner of money.

## Stack
- **Ruby on Rails** (full-stack; server-rendered views are fine — this app owns a domain).
- **SQLite** — its own file-based database. No shared DB with NemPay or NemLuxury. It's a
  dummy merchant with a simple domain, so SQLite is plenty and keeps the stack minimal.
- **No background jobs.** Inbound NemPay webhooks are handled **synchronously** in the
  controller (see below). Fine for a dummy merchant, and it keeps Redis/Sidekiq out.

## Domain (what Rails owns)
- **Taskers & customers:** the two sides of the marketplace. A tasker must be onboarded as
  a **payee in NemPay** before they can receive a release (NemTasker stores only the
  opaque NemPay payee id, never bank details).
- **Task:** what needs doing, its budget (integer cents, never floats), and its owner.
- **Offer:** a tasker's bid on a task. Accepting one assigns the tasker.
- **Task lifecycle** — mirrors the money flow but is a *separate* state machine owned here:
  ```
  posted → offer_accepted → funded → in_progress → completed → paid_out
  posted | offer_accepted → cancelled
  funded | in_progress → cancelled → refunded
  ```
  The `funded`, `paid_out`, and `refunded` transitions are **caused by verified NemPay
  webhooks**, never by NemTasker deciding on its own or trusting a browser redirect.

## The platform fee (NemTasker's decision, NemPay's math)
NemTasker decides *how much* its service fee is and passes it as `application_fee` (cents)
when creating the payment intent. NemPay does the actual money math — splitting the released
amount into `payable-to-payee` and `platform-revenue`. NemTasker never computes the split
itself.

## NemPay integration (the whole point of this app)

Keep all NemPay interaction behind a service layer (`app/services/nem_pay/…`). No gateway
calls from controllers or models directly.

**Server-to-server — fund the task into escrow.**
- When the customer accepts an offer, call NemPay `POST /v1/payment_intents` with:
  `{ escrow: true, payee: <tasker's payee id>, application_fee: <cents>, amount, currency,
  metadata: { task_id } }` and a fresh **`Idempotency-Key`**. Derive one key per funding
  attempt and reuse it on retries, so a double-submit cannot double-charge.
- Use the **secret key** here. It lives in Rails credentials / ENV — never in the repo,
  never sent to the browser.

**Client-side — collect the card.**
- The customer enters card details into NemPay's tokenization SDK / iframe. Raw card
  numbers (PAN) MUST NOT touch NemTasker servers — this is the entire PCI-scope argument.
- NemTasker receives only a `payment_method` token / `client_secret`.

**Server-to-server — release when the job is done.**
- When the customer confirms the task complete, call `POST /v1/payment_intents/:id/release`
  with an **`Idempotency-Key`**. This is the only thing that moves money out of escrow.
- Release is **idempotent by construction**: a customer double-clicking "confirm" must not
  pay the tasker twice. Reuse one key per task's release.
- NemTasker never releases money on its own ledger — it only *asks* NemPay to release.

**Cancel — refund from escrow.**
- If the task is cancelled while funds are held, request a refund; NemPay moves the money
  back out of `escrow-liability` to the customer. Never refund a task that already released.

**Webhooks — react to the outcome (the source of truth).**
- Expose one endpoint, e.g. `POST /webhooks/nem_pay`.
- **Verify the HMAC signature** against the shared webhook secret before trusting the
  payload. Reject on mismatch.
- Handle the event **synchronously** in the controller, then return `200`: verify the
  signature → dedupe → advance the task → respond. No background job. NemPay's delivery
  worker is given a generous timeout to accommodate this.
- Delivery is **at-least-once** → handlers must still be **idempotent** (dedupe on NemPay's
  `event_id`), synchronous or not.
- `payment_intent.held_in_escrow` → mark the task `funded`. `escrow.released` → `paid_out`.
  Refund events → `refunded`. Never advance the task from a client redirect alone.

## Boundaries (do not cross)
- NemTasker never computes balances, never stores card data, never stores tasker bank
  details, and never assumes a payment succeeded without a verified webhook.
- NemTasker never moves or holds money itself. It asks NemPay to hold (fund) and release;
  the escrow-liability, the segregated funds, and the split are entirely NemPay's.
- The meaning of the work (assembling a wardrobe vs moving a sofa) stays here. NemPay only
  ever sees an amount, a currency, a payee, a fee, and opaque metadata.

## Testing
- Stub NemPay in unit tests; run against the local NemPay + `bank-sim` for end-to-end.
- Exercise the escrow-specific failure paths deliberately:
  - declined card at funding (does the task correctly stay `offer_accepted`?),
  - a `held_in_escrow` webhook that never arrives (does the task stay unfunded?),
  - a double-submitted funding (does the reused `Idempotency-Key` prevent a second hold?),
  - a double-clicked release (does the reused `Idempotency-Key` pay the tasker only once?),
  - a refund requested *after* release (is it correctly rejected — money already gone?),
  - a duplicate webhook (does the handler dedupe on `event_id`?).

## Running
NemTasker runs **independently** of NemPay's compose — `bin/rails s` locally is simplest
(an optional Dockerfile is fine too). Start the gateway first (`docker-compose up` inside
`NemPay/`), then point this app at it via `NEMPAY_API_URL` (e.g. `http://localhost:8080`)
plus the API keys, held in Rails credentials / ENV. If NemTasker itself runs in a
container, reach the host gateway via `host.docker.internal` rather than `localhost`.

## Commands
- `bin/setup` · `bin/rails s` · `bin/rails db:migrate` · `bundle exec rspec`
