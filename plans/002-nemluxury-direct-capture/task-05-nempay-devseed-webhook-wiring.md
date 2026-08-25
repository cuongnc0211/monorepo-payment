# Task 05 — NemPay dev-seed webhook wiring + docker networking

**Plan:** ./plan.md · **Depends on:** none (NemPay side) · **Blocks:** task-06

## Context
- Spec acceptance criteria covered: **enables** the end-to-end delivery behind **AC1/AC4/AC5** — without a
  registered endpoint, NemPay has nowhere to deliver.
- Links: [`../../nem_pay/CLAUDE.md`](../../nem_pay/CLAUDE.md) — "Webhooks (outbox pattern)"; existing
  `internal/devseed`, `InsertWebhookEndpoint` query, `docker-compose.yml`.

## Requirements
- **Additive, dev-only** change to NemPay (its public `/v1` API and money logic stay frozen).
- Extend `internal/devseed` so that, when `NEMPAY_DEV_WEBHOOK_URL` **and** `NEMPAY_DEV_WEBHOOK_SECRET`
  are set, it inserts **one** `webhook_endpoints` row for the dev merchant (idempotent — only if the
  merchant has none). Skip cleanly when the env is unset (standalone gateway unchanged).
- Wire the env into `docker-compose.yml` `api`: `NEMPAY_DEV_WEBHOOK_URL` default
  `http://host.docker.internal:3000/webhooks/nem_pay`, `NEMPAY_DEV_WEBHOOK_SECRET` a known dev value.
- NemLuxury's `NEMPAY_WEBHOOK_SECRET` (task-01) must equal `NEMPAY_DEV_WEBHOOK_SECRET`. Document the
  single shared value and the `host.docker.internal` reachability note in both READMEs.

## Files to create / modify
```
nem_pay/api/internal/devseed/devseed.go     (insert webhook endpoint from env, idempotent)
nem_pay/api/internal/config/config.go       (read NEMPAY_DEV_WEBHOOK_URL/_SECRET, optional)
nem_pay/docker-compose.yml                  (api env: NEMPAY_DEV_WEBHOOK_URL/_SECRET)
nem_pay/.env.example, Nem_luxury/.env.example, Nem_luxury/README.md (shared secret + networking note)
```

## Implementation steps
1. In `devseed.Run`, after seeding keys: if URL+secret present and the dev merchant has no endpoint,
   `InsertWebhookEndpoint(merchant, url, secret)`. Log the registered URL.
2. Read the two vars in `config` (empty = disabled).
3. Add the env to compose `api`; document `host.docker.internal:3000` for reaching the host-run merchant.
4. Document that NemLuxury and NemPay must share the same webhook secret.

## Validation / tests
- Go DB test (in `devseed`): with the env set, exactly one endpoint row is created for the dev merchant,
  and a second `Run` does not duplicate it (idempotent). With the env unset, no endpoint is created.
- Manual: `docker-compose up` in `NemPay/` logs the registered webhook URL; the `webhook_endpoints` table
  has the row.

## Risks & rollback
- **`host.docker.internal`** resolves on Docker Desktop (macOS/Windows); on Linux it needs an
  `extra_hosts` mapping — document it.
- **Secret drift** between NemPay seed and NemLuxury ENV → every signature fails (task-04). One value.
- **Idempotency**: never insert a duplicate endpoint on re-seed.
- Rollback: unset the env (endpoint not seeded) — NemPay reverts to standalone; the code path is inert.
