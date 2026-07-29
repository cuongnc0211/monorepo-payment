# Task 01 (M1.0) — Scaffolding + self-contained compose

**Milestone:** M1 (NemPay direct-capture gateway) · **Depends on:** nothing · **Blocks:** all other M1 phases

## Context
- Roadmap: [`plan.md`](./plan.md) · Repo rules: [`../../CLAUDE.md`](../../CLAUDE.md) · Gateway rules: [`../../nem_pay/CLAUDE.md`](../../nem_pay/CLAUDE.md)
- Goal of this phase: `docker-compose up` inside `NemPay/` brings up a healthy gateway skeleton
  (Postgres + Redis + api + bank-sim), migrations run on boot, `/v1/health` returns 200. No
  business logic yet — this is the runnable shell everything else lands in.

## Requirements
- One command (`docker-compose up` in `NemPay/`) yields a working gateway; nothing else required.
- Migrations apply automatically on api boot (or a one-shot migrate service that the api waits on).
- Redis present (used from M1.5); Postgres is the only app DB.
- Config via env only; sane defaults for local (`.env.example` committed, `.env` git-ignored).

## Files to create
```
NemPay/
├── docker-compose.yml
├── .env.example
├── api/
│   ├── go.mod                       # module github.com/nempay/api (Go 1.22+)
│   ├── Makefile                     # run · migrate · sqlc · test
│   ├── sqlc.yaml                    # engine: postgresql; out: internal/repository/db
│   ├── Dockerfile
│   ├── cmd/api/main.go              # Gin server, /v1/health, graceful shutdown
│   ├── internal/config/config.go    # env loader (DB_URL, REDIS_URL, PORT, …)
│   ├── internal/httpapi/router.go   # /v1 group, health handler, middleware wiring
│   ├── db/migrations/               # golang-migrate (empty for now)
│   └── db/queries/                  # sqlc inputs (empty for now)
└── bank-sim/
    ├── go.mod
    ├── Dockerfile
    └── cmd/banksim/main.go          # stub: /authorize /capture return 200 (filled in M1.4)
```

## Implementation steps
1. `go mod init` for `api/` and `bank-sim/`; add Gin, pgx/v5, golang-migrate, sqlc (dev).
2. `docker-compose.yml` services: `postgres` (with healthcheck), `redis`, `migrate`
   (one-shot: `migrate -path db/migrations -database $DB_URL up`), `api` (`depends_on:
   migrate` completed + postgres healthy), `bank-sim`. Expose api on host `:8080`.
3. `cmd/api/main.go`: load config, open pgxpool, mount router, `GET /v1/health` →
   `{"status":"ok"}`, graceful shutdown on SIGTERM.
4. Makefile targets; `sqlc.yaml` pointed at `db/queries` + `db/migrations` for schema.
5. Commit `.env.example` (`DB_URL=postgres://…`, `REDIS_URL=…`, `PORT=8080`).

## Validation / tests
- `docker-compose up` → all services healthy; `curl localhost:8080/v1/health` → 200.
- `docker-compose down && up` twice → migrations idempotent, no dirty state.
- api starts only after Postgres healthy and migrate completed (kill order test).

## Risks & rollback
- **Migrate/api race** → api boots before schema exists. Mitigate with the one-shot `migrate`
  service + `depends_on: condition: service_completed_successfully`.
- **golang-migrate "dirty" state** after a failed migration wedges boot. Document
  `migrate force <v>` recovery in the Makefile comment.
- Rollback: this phase adds only scaffolding; `docker-compose down -v` resets everything.
