# Task 06 — Portal pages: payments, ledger, balances, webhooks, keys

**Plan:** ./plan.md · **Depends on:** task-05 (and the endpoints from task-03) · **Blocks:** none

## Context
- Spec acceptance criteria covered: **AC4** (payment detail + ledger), **AC5** (balances), **AC6**
  (webhook logs), **AC7/AC8** (keys masked), **AC9** (no PAN), **AC10** (no mutate controls),
  **AC11** (via `/v1`).
- Links: task-03 endpoints; nem_pay/CLAUDE.md "What it shows".

## Requirements
- Read-only pages, each fetching via TanStack Query + the generated client, showing only the signed-
  in merchant's data:
  - **Payments list** — newest-first, status + amount/currency, filter by status, pagination.
  - **Payment detail** — status, settlement mode, metadata, refunds, and the backing ledger entries
    (from `/:id/ledger`); amounts shown exactly as the API returns them (no client money math).
  - **Balances** — by currency/kind, from `/v1/balances`.
  - **Webhooks** — events + delivery status (delivered/retrying/failed distinguished).
  - **API keys** — kind + prefix, secret masked.
- No control anywhere mutates gateway state (no refund/capture/key/webhook buttons). Empty and
  loading states for each list.

## Files to create / modify
```
nem_pay/portal/src/pages/{Payments.tsx, PaymentDetail.tsx, Balances.tsx, Webhooks.tsx, ApiKeys.tsx}
nem_pay/portal/src/api/queries.ts        (typed query hooks over the generated client)
nem_pay/portal/src/App.tsx               (routes for the pages)
```

## Implementation steps
1. Query hooks for each endpoint (list intents w/ status+page params, intent + ledger, balances,
   webhook events, api keys).
2. Payments list with a status filter and pagination; row → detail.
3. Payment detail rendering state, refunds, and the ledger entries table (debits/credits per
   account); display amounts verbatim from the API.
4. Balances, Webhooks (status badges), API keys (masked) pages.
5. Consistent loading/empty/error states; format money from minor units for display only.

## Validation / tests
- Manual (browser, against the running gateway with seeded data): each page shows the signed-in
  merchant's data and nothing from the other merchant (**AC2/AC11**); amounts match the API/`curl`
  values (**AC4/AC5**); webhook statuses render (**AC6**); keys show masked, no full secret (**AC7/AC8**);
  no card data anywhere (**AC9**); there is no button that mutates state (**AC10**).
- Cross-check one payment's ledger entries sum to zero and match the API.

## Risks & rollback
- Risk: client recomputing money. Mitigation: render API-provided amounts only; review.
- Risk: accidental write affordance. Mitigation: pages are read-only by construction; review against
  AC10.
- Rollback: delete the page files/routes; the scaffold (task-05) still stands.
