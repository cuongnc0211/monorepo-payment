# NemLuxury

A dummy luxury e-commerce store (Rails 8 + SQLite) that integrates **NemPay** as its payment
gateway — the **direct-capture** reference merchant. It owns products and orders; it owns nothing
about how money moves. See [`CLAUDE.md`](./CLAUDE.md) for the rules and
[`../specs/002-nemluxury-direct-capture/spec.md`](../specs/002-nemluxury-direct-capture/spec.md) for
the contract.

## Setup
```
bin/setup            # install gems, prepare the SQLite DB
cp .env.example .env # then fill in the NemPay secrets
```

## Running (against the local gateway)
1. Start NemPay first: `docker-compose up` inside `../NemPay/`.
2. Point this app at it via `.env` (`NEMPAY_API_URL`, `NEMPAY_SECRET_KEY`, `NEMPAY_WEBHOOK_SECRET`).
   `NEMPAY_WEBHOOK_SECRET` must match the secret NemPay is seeded with for this merchant's webhook
   endpoint. If NemLuxury runs in a container, set `NEMPAY_API_URL` to `http://host.docker.internal:8080`.
3. `bin/rails s`

## Tests
```
bundle exec rspec               # unit + request specs (NemPay stubbed via WebMock)
bundle exec rspec --tag e2e     # live outbound checkout against a running NemPay + bank-sim
```

## Manual end-to-end (both halves, live)
Verifies the full loop including the async inbound webhook (NemPay → this app → order `paid`).

1. Start the gateway with its webhook pointed at this app (defaults already do this):
   ```
   cd ../NemPay && docker-compose up --build
   ```
   The api logs `dev webhook endpoint: http://host.docker.internal:3000/webhooks/nem_pay` and the
   dev secret key.
2. Start this app on port 3000 with matching secrets:
   ```
   NEMPAY_API_URL=http://localhost:8080 \
   NEMPAY_SECRET_KEY=sk_test_nempay_secret \
   NEMPAY_WEBHOOK_SECRET=whsec_nemluxury_dev \
   bin/rails server -p 3000
   ```
3. Open http://localhost:3000, buy an item with the "Valid test card", and watch the order page
   move from *Payment pending* to *Paid* once the webhook is delivered (a few seconds).

> Note: development mode allows the `host.docker.internal` Host header (see
> `config/environments/development.rb`) so webhooks delivered from the NemPay container aren't
> blocked by Rails host authorization (they'd 403 otherwise).

## Stack
Ruby on Rails 8, SQLite, RSpec, server-rendered ERB. **No background jobs** — inbound NemPay webhooks
are handled synchronously in the controller. Gateway interaction lives behind `app/services/nem_pay/`.
