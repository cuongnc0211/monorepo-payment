# Task 03 — NemPay client + checkout orchestration

**Plan:** ./plan.md · **Depends on:** task-02 · **Blocks:** task-06

## Context
- Spec acceptance criteria covered: **AC1** (create→confirm→capture), **AC2** (declined → stays
  pending), **AC6** (idempotent checkout / double-submit), **AC8** (direct-only), and the **AC3/AC7**
  invariants at unit level (never mark paid from the API response; no PAN).
- Links: [`../../Nem_luxury/CLAUDE.md`](../../Nem_luxury/CLAUDE.md) — "NemPay integration"; NemPay `/v1`
  API from M1 (`nem_pay/CLAUDE.md`, "API conventions").

## Requirements
- **`NemPay::Client`** (thin `Net::HTTP` wrapper): `create_intent(amount:, currency:, metadata:, idempotency_key:)`,
  `confirm(intent_id:, token:, idempotency_key:)`, `capture(intent_id:, idempotency_key:)`, `get_intent(id)`.
  Sets `Authorization: Bearer <secret>`, `Idempotency-Key`, JSON content-type; returns a typed result
  (success + parsed body, `declined`, or `error`) — never raises for a business decline.
- **`NemPay::Checkout.call(order:, token:)`** orchestrates **create → confirm(token) → capture** with
  **deterministic per-operation** idempotency keys derived from the order's `checkout_token`
  (`co-<token>-create` / `-confirm` / `-capture`). Stores `nem_pay_intent_id` on the order after create.
  On `confirm` returning `failed` (declined) → **do not capture**, leave the order `pending_payment`,
  return a declined result. **Never marks the order `paid`** (that is the webhook's job — task-04).
- **`CheckoutsController#create`**: `Order.find_or_create_by(checkout_token:)` for the product (so a
  double-submit maps to one order), then `NemPay::Checkout.call`; redirect to the order status page;
  render a failure notice on declined/error. Re-submitting an order already driven reuses the same
  deterministic keys (NemPay dedupes) → no second charge.
- **Direct only**: `create_intent` sends `amount`, `currency`, `metadata:{order_id}` — **never** an
  `escrow` flag or `payee` (AC8).

## Files to create / modify
```
Nem_luxury/app/services/nem_pay/client.rb
Nem_luxury/app/services/nem_pay/checkout.rb
Nem_luxury/app/services/nem_pay/result.rb        (typed success/declined/error)
Nem_luxury/app/controllers/checkouts_controller.rb
Nem_luxury/config/routes.rb                      (post "/checkout")
```

## Implementation steps
1. `Client`: build requests to `NemPay.api_url` + `/v1/...`; JSON encode/decode; attach auth +
   `Idempotency-Key`; map HTTP/status + intent `status` to `Result` (`captured`/`authorized`/`failed`/error).
2. `Checkout`: create (store intent id) → confirm(token); if `failed` → return declined; else capture →
   return success. Deterministic keys per operation. No `paid` mutation here.
3. `CheckoutsController#create`: find_or_create the order on `checkout_token`; guard against re-driving a
   paid/settled order; call `Checkout`; redirect to `orders#show`; flash the decline/error message.
4. Wire the buy form (task-02) → `POST /checkout` with `product_id`, `checkout_token`, `payment_method` token.

## Validation / tests (WebMock-stubbed NemPay)
| Scenario | Expected | AC |
|---|---|---|
| valid method | create→confirm(tok_ok)→capture called with correct bodies + distinct Idempotency-Keys; order gets `nem_pay_intent_id`; order **still `pending_payment`** | AC1, AC3 |
| declined | confirm returns `failed` → **capture NOT called**; order stays `pending_payment`; controller shows failure | AC2 |
| double-submit | two `POST /checkout` with the same `checkout_token` → **one** order; `create` uses the same key (NemPay dedupes) → not double-charged | AC6 |
| request body | `create_intent` payload has amount+currency+metadata only, **no `escrow`/`payee`** | AC8 |
| no PAN | no request carries a card number; only the opaque token | AC7 |

## Risks & rollback
- **Shared idempotency key across operations** → NemPay `422`. Keep keys per (checkout, operation).
- **Partial failure** (create ok, confirm errors/network) → leave order `pending_payment`; the
  deterministic keys make a retry safe. Surface, don't mark paid.
- **Marking paid from the capture 200** — forbidden; only task-04's webhook sets `paid`.
- Rollback: remove the services + controller + route (money flow reverts; order stays pending).
