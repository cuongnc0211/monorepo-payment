# Plan 004 — Card tokenization & multi-step checkout

Spec: `specs/004-tokenization-checkout/spec.md`. Status: done — verified end-to-end (browser happy
path + PAN-never-reaches-Rails check; NemPay `go test` and NemLuxury `rspec` green).

## Design decisions
- **Token = passthrough (test-gateway simplification).** `/v1/tokens` maps a test PAN to the
  existing bank-sim magic token (`tok_ok` / `tok_declined` / `tok_timeout`) and returns it as the
  token id. This keeps the money core, `confirm`, and bank-sim untouched (no new storage, no
  migration in the api). In a production gateway the token would be opaque and single-use via a
  vault; noted in code. The response still carries `brand`/`last4` like a real token object.
- **PaymentIntent created at step 1** (contact form), mirroring Stripe (create intent server-side,
  confirm later with the tokenized method). Card page tokenizes; Pay confirms + captures.
- **Publishable-key auth + CORS** for `/v1/tokens` only. Secret key is refused there.
- Idempotency keys stay derived from `order.checkout_token` (`co-<token>-{create,confirm,capture}`).

## Phase A — NemPay: tokenization endpoint
Files: `internal/httpapi/tokens.go` (new), `internal/httpapi/cors.go` (new),
`internal/httpapi/router.go`, `internal/httpapi/tokens_test.go` (new), `openapi.yaml`.
- `publishableOnly()` guard (mirror of `secretOnly`).
- CORS middleware (engine-level): sets `Access-Control-Allow-*` for the configured merchant origin;
  answers `OPTIONS` preflight with 204 before auth runs. Origin from env
  (`NEMPAY_CORS_ORIGIN`, default `http://localhost:3000`).
- `POST /v1/tokens`: validate card fields (present + light Luhn/length), derive brand/last4, map
  PAN → magic token, return `{ id, object:"token", card:{ brand, last4, exp_month, exp_year } }`.
- Tests: publishable ok, secret refused, preflight ok, each test card → expected token, bad body → 400.

## Phase B — NemLuxury: data + flow
Files: migration add contact fields, `config/initializers/nem_pay.rb` (publishable key + js url),
`example.env`, routes, `checkouts_controller.rb` (refactor), new `payments_controller.rb`,
`app/services/nem_pay/checkout.rb` (split create vs confirm+capture), views.
- Migration: `customer_name`, `customer_address`, `customer_phone` on `orders`.
- `NemPay.publishable_key` + `NemPay.js_url` (browser-facing base, default api_url).
- Routes: `GET /products/:id/checkout` (contact form), `POST /checkout` (create order+intent →
  redirect to payment), `GET /orders/:id/payment` (card page), `POST /orders/:id/payment`
  (token → confirm+capture → redirect to order), `GET /orders/:id` (success/status).
- Split `NemPay::Checkout`: `create_intent!(order)` and `pay!(order, token)`.

## Phase C — NemLuxury: Stripe-like UI
- Contact form view; card page view (card fields + test-card panel) with a small JS module that
  POSTs to `${NEMPAY_JS_URL}/v1/tokens` with the publishable key, then submits the returned token to
  `POST /orders/:id/payment`. PAN stays in the browser.
- Enhance the paid order view into a detailed success page (contact + product + amount + ids).

## Verification
- NemPay: `go test ./internal/httpapi/...` (token cases, auth, preflight) + `curl` publishable vs secret.
- NemLuxury: request specs for the new steps; browser walk-through of the happy path (4242) and a
  decline (4000…0002); grep logs/DB to confirm no PAN leaks.
