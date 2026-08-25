# Task 02 — Domain: Product + Order + catalogue

**Plan:** ./plan.md · **Depends on:** task-01 · **Blocks:** task-03, task-04

## Context
- Spec acceptance criteria covered: **AC7** (no card data in the domain), **AC8** (order carries only
  amount/currency, no domain meaning leaks to NemPay) — foundational for **AC1/AC2/AC3**.
- Links: [`../../Nem_luxury/CLAUDE.md`](../../Nem_luxury/CLAUDE.md) — "Domain", "Order lifecycle".

## Requirements
- **Product** catalogue: `name`, `description`, `amount_cents` (integer), `currency` (char 3). Prices are
  **integer cents**, never floats. Seed a few luxury items.
- **Order**: belongs to a product; carries `amount_cents`, `currency`, `status`, a UNIQUE `checkout_token`
  (the double-submit natural key, minted at buy-form render), and a nullable `nem_pay_intent_id`.
- **Order lifecycle** as an explicit, guarded state set: `pending_payment → paid → fulfilled | cancelled`.
  `mark_paid!` transitions **only** from `pending_payment` and is idempotent/safe if already `paid`
  (no-op); illegal transitions raise. `fulfilled`/`cancelled` exist but have **no** workflow in M2.
- Catalogue + product pages; a **buy form** that embeds a fresh `checkout_token` (hidden) and a
  **test-payment-method selector** (values map to magic tokens: valid→`tok_ok`, declined→`tok_declined`,
  timeout→`tok_timeout`). **No card-number field anywhere.** An order-status page.

## Files to create / modify
```
Nem_luxury/db/migrate/*_create_products.rb, *_create_orders.rb
Nem_luxury/app/models/product.rb, order.rb
Nem_luxury/db/seeds.rb                         (luxury catalogue)
Nem_luxury/app/controllers/products_controller.rb   (index, show)
Nem_luxury/app/controllers/orders_controller.rb     (show — status page)
Nem_luxury/app/views/products/{index,show}.html.erb (buy form + method selector)
Nem_luxury/app/views/orders/show.html.erb
Nem_luxury/app/helpers/money_helper.rb              (cents → display)
Nem_luxury/config/routes.rb                         (products, order show)
```

## Implementation steps
1. Migrations + models; `orders.status` a plain enum/string with a constant list; `checkout_token` UNIQUE.
2. `Order#mark_paid!`: guard `pending_payment → paid`; no-op if already `paid`; raise on any other source.
3. `db/seeds.rb` with a handful of high-value items (integer cents).
4. `products#index` (catalogue) and `products#show` (buy form: hidden `checkout_token` generated at render,
   a `<select>` of test methods, submit → `POST /checkout` wired in task-03).
5. `orders#show` status page (renders `pending_payment` / `paid`).
6. `MoneyHelper` formats integer cents for display (never introduces floats into logic).

## Validation / tests
- Model specs: `mark_paid!` succeeds from `pending_payment`; is a no-op from `paid`; raises from other
  states (proves the guarded lifecycle underpinning **AC1/AC3**). `checkout_token` uniqueness enforced.
- `amount_cents` is an integer; money is never stored/compared as float (**AC8** posture).
- Request spec: catalogue + product page render; the buy form contains a `checkout_token` and a method
  selector and **no** input that accepts a card number (**AC7**).

## Risks & rollback
- **Float leakage** in price math → use integer cents everywhere; helper is display-only.
- **Enum choice**: keep it explicit and visible; illegal transitions must raise, not silently pass.
- Rollback: drop the two migrations; remove models/controllers/views (additive on task-01).
