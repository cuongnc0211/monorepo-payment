# NemLuxury

A dummy luxury e-commerce store — supercars, high-end watches, yachts, private jets —
built in Rails. It exists for one reason: to be the reference merchant that integrates
**NemPay**. It owns products and orders; it owns nothing about how money moves.

This is the **direct-capture** reference integration: NemLuxury sells its *own* goods, so
money settles straight to the merchant — there is no third party and therefore no escrow.
Its sibling **NemTasker** is the escrow counterpart. Read them together to see the two
money-movement shapes side by side.

Rails is the right tool *here* because this app owns a real domain (catalogue, cart,
orders) — which is exactly where Rails' ActiveRecord + conventions shine. Contrast with
the NemPay portal, which owns no domain and is therefore React.

## Stack
- **Ruby on Rails** (full-stack; server-rendered views are fine — this app owns a domain).
- **SQLite** — its own file-based database. No shared DB with NemPay. It's a dummy merchant
  with a simple domain, so SQLite is plenty and keeps the stack minimal.
- **No background jobs.** Inbound NemPay webhooks are handled **synchronously** in the
  controller (see below). Fine for a dummy merchant, and it keeps Redis/Sidekiq out.

## Domain (what Rails owns)
- **Products:** high-value catalogue items. Prices are large — store as integer cents
  (a cents column or money-rails), never floats.
- **Cart / Checkout / Order.**
- **Order lifecycle:** `pending_payment → paid → fulfilled | cancelled | refunded`. The
  `paid` transition is **caused by a verified NemPay webhook**, never by NemLuxury
  deciding on its own or trusting a browser redirect.

## NemPay integration (the whole point of this app)

Keep all NemPay interaction behind a service layer (`app/services/nem_pay/…`). No gateway
calls from controllers or models directly.

**Server-to-server — create the payment.**
- On checkout, call NemPay `POST /v1/payment_intents` with the amount (cents), currency,
  an order reference in `metadata`, and a fresh **`Idempotency-Key`**. Derive one key per
  checkout attempt and reuse it on retries, so a double-submit cannot double-charge.
- Use the **secret key** here. It lives in Rails credentials / ENV — never in the repo,
  never sent to the browser.

**Client-side — collect the card.**
- The customer enters card details into NemPay's tokenization SDK / iframe. Raw card
  numbers (PAN) MUST NOT touch NemLuxury servers — this is the entire PCI-scope argument.
  Do not add a form that posts card numbers to Rails.
- NemLuxury receives only a `payment_method` token / `client_secret`.

**Webhooks — react to the outcome (the source of truth).**
- Expose one endpoint, e.g. `POST /webhooks/nem_pay`.
- **Verify the HMAC signature** against the shared webhook secret before trusting the
  payload. Reject on mismatch.
- Handle the event **synchronously** in the controller, then return `200`: verify the
  signature → dedupe → update the order → respond. No background job. NemPay's delivery
  worker is given a generous timeout to accommodate this.
- Delivery is **at-least-once** → handlers must still be **idempotent** (dedupe on NemPay's
  `event_id`), synchronous or not.
- `payment_intent.captured` / succeeded → mark the order `paid`. Refund events → update
  accordingly. Never mark an order paid from the client redirect alone.

## Boundaries (do not cross)
- NemLuxury never computes balances, never stores card data, and never assumes a payment
  succeeded without a verified webhook.
- NemLuxury never uses escrow. It sells its own goods, so it always creates *direct*
  payment intents (no `escrow` flag, no `payee`). Escrow lives in NemTasker; if you reach
  for it here, the domain model is wrong.
- The meaning of the goods (a yacht vs a watch) stays here. NemPay only ever sees an
  amount, a currency, and opaque metadata.

## Testing
- Stub NemPay in unit tests; run against the local NemPay + `bank-sim` for end-to-end.
- Exercise the failure paths deliberately:
  - declined card,
  - a webhook that never arrives (does the order correctly stay `pending_payment`?),
  - a duplicate webhook (does the handler dedupe on `event_id`?),
  - a double-submitted checkout (does the reused `Idempotency-Key` prevent a second charge?).

## Running
NemLuxury runs **independently** of NemPay's compose — `bin/rails s` locally is simplest
(an optional Dockerfile is fine too). Start the gateway first (`docker-compose up` inside
`NemPay/`), then point this app at it via `NEMPAY_API_URL` (e.g. `http://localhost:8080`)
plus the API keys, held in Rails credentials / ENV. If NemLuxury itself runs in a
container, reach the host gateway via `host.docker.internal` rather than `localhost`.

## Commands
- `bin/setup` · `bin/rails s` · `bin/rails db:migrate` · `bundle exec rspec`
