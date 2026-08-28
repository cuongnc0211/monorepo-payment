# Task 02 — Portal: endpoints section on the Webhooks page

**Plan:** ./plan.md · **Depends on:** task-01 · **Blocks:** none

## Context
- Spec ACs: **AC1** (list), **AC2** (create form), **AC3** (disable button), **AC6** (secret entered,
  never shown back).
- Links: portal src/pages/Webhooks.tsx, src/api/queries.ts.

## Requirements
- On the Webhooks page, above the recent-deliveries table: an **Endpoints** section listing the
  merchant's endpoints (URL, active/disabled, created), a small **add form** (URL + secret), and a
  **Disable** action per active endpoint.
- Uses TanStack Query mutations over the generated client; invalidates the endpoints query on
  success. The secret is a password-type input and is never rendered back.

## Files to create / modify
```
nem_pay/portal/src/api/queries.ts        (useWebhookEndpoints; useCreateEndpoint; useDisableEndpoint)
nem_pay/portal/src/pages/Webhooks.tsx     (Endpoints section + form + disable)
nem_pay/portal/src/api/schema.ts          (regenerated)
```

## Implementation steps
1. Regenerate the client (`pnpm gen`).
2. Query hook `useWebhookEndpoints`; mutation hooks `useCreateEndpoint` / `useDisableEndpoint`
   (invalidate `["webhook_endpoints"]` on success).
3. Webhooks page: render the endpoints list (URL mono, active/disabled badge), an add form (URL +
   secret password field, submit disabled while pending, error surface), and a Disable button on
   active rows. Keep the existing recent-deliveries table below.

## Validation / tests
- Manual (browser): add an endpoint → it appears; disable → shows disabled; the secret field is a
  password input and no secret is shown in the list (**AC1/AC2/AC3/AC6**). `pnpm build` green.

## Risks & rollback
- Rollback: remove the endpoints section + hooks; the read-only Webhooks page remains.
