# 004 — Card tokenization & multi-step checkout

## Why
Today NemLuxury's buy form makes the customer pick a raw bank-sim token from a dropdown, and the
purchase is one page. We want a Stripe-like checkout: collect contact details, then enter card
data on a dedicated page, then a payment-success page with the order details.

The lesson this preserves: **a card number (PAN) must never reach a merchant server.** A faithful
Stripe clone honours that by having the card fields talk *directly to the gateway*, which returns
an opaque token; the merchant only ever handles the token. So this feature adds real tokenization
to NemPay and rebuilds NemLuxury's checkout on top of it.

## Scope

### NemPay (gateway)
- **New endpoint `POST /v1/tokens`** — accepts card fields from the browser, returns a single-use
  payment-method **token**. Authenticated with a **publishable** key (safe to expose in a browser),
  never the secret key.
- The PAN is used only to derive `brand` + `last4` and to select the test outcome; it is **never
  stored**.
- **CORS**: the endpoint is browser-callable cross-origin from the merchant site; it must answer
  preflight and allow the configured merchant origin.
- Test cards map to the existing bank-sim outcomes:
  - `4242 4242 4242 4242` → success
  - `4000 0000 0000 0002` → declined
  - `4000 0000 0000 0069` → bank timeout
- Money core (intents, ledger, confirm/capture, webhooks) is **unchanged**.

### NemLuxury (merchant)
- Order gains contact fields: `customer_name`, `customer_address`, `customer_phone`.
- **Step 1 — contact**: "Acquire now" opens a form (name, address, phone). Submitting creates the
  order and a NemPay PaymentIntent, then goes to the card page.
- **Step 2 — card (Stripe-like)**: card fields + a panel listing the test cards and what each does.
  On "Pay", the browser tokenizes **directly against NemPay** (publishable key), then submits only
  the returned **token** to NemLuxury, which confirms + captures server-to-server.
- **Step 3 — success**: the order page shows a detailed confirmation once the capture webhook lands
  (declines return to the card page with a message).

## Acceptance criteria
1. `POST /v1/tokens` with a publishable key + `4242…` returns `200` and a token; the same call with
   a **secret** key is refused (publishable-only), and a browser preflight (`OPTIONS`) succeeds.
2. The PAN never appears in any NemLuxury request, log, or DB row — only the token, brand, last4.
3. Full happy path: Acquire → contact form → card page → Pay (4242) → success page shows the order
   (product, amount, contact details) once `payment_intent.captured` is delivered.
4. Declined card (`4000…0002`) returns to the card page with a clear message and no order marked
   paid; timeout card leaves the order recoverable.
5. Money core behaviour and existing NemPay/NemLuxury tests are unaffected.

## Out of scope
- Persisting/reusing saved cards, 3-D Secure, multiple line items, real PAN validation beyond a
  light Luhn/format check, escrow (M3).
