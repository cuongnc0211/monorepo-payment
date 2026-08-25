# Task 01 — Rails 8 scaffold + config

**Plan:** ./plan.md · **Depends on:** none · **Blocks:** task-02, task-03, task-04

## Context
- Spec acceptance criteria covered: **none proven yet** — this is the runnable shell every later task builds on.
- Links: [`../../Nem_luxury/CLAUDE.md`](../../Nem_luxury/CLAUDE.md) — "Stack", "Running", "Commands".

## Requirements
- A fresh **Rails 8** app living directly in `Nem_luxury/`, backed by **SQLite**, server-rendered.
- **RSpec** as the test framework (`bundle exec rspec`); **WebMock** available for stubbing NemPay in unit tests.
- **No background-job / Redis infrastructure** — remove/disable Solid Queue etc.; webhooks are synchronous.
- NemPay configuration read from **ENV / Rails credentials**, never hard-coded: `NEMPAY_API_URL`,
  `NEMPAY_SECRET_KEY`, `NEMPAY_WEBHOOK_SECRET`. The secret key is server-side only.
- App boots (`bin/rails s`) and `bundle exec rspec` is green.

## Files to create / modify
```
Nem_luxury/                      (rails new . -d sqlite3 -T)
Nem_luxury/Gemfile               (+ rspec-rails, webmock; ensure no sidekiq/redis)
Nem_luxury/config/initializers/nem_pay.rb   (NemPay config object from ENV)
Nem_luxury/.env.example          (NEMPAY_API_URL / NEMPAY_SECRET_KEY / NEMPAY_WEBHOOK_SECRET)
Nem_luxury/config/routes.rb      (root route → products#index, added in task-02)
Nem_luxury/spec/…                (rspec install + a smoke spec)
Nem_luxury/README.md             (run against local NemPay)
```

## Implementation steps
1. `rails new Nem_luxury -d sqlite3 -T` (skip Minitest). Keep the folder name `Nem_luxury`.
2. Add `rspec-rails`, `webmock` (test group) to the Gemfile; `bundle`; `rails g rspec:install`.
3. Strip background-job infra so nothing needs Redis/Sidekiq; confirm the async job adapter is inline/none.
4. Add `NemPay` config (`NemPay.api_url`, `.secret_key`, `.webhook_secret`) reading ENV with sane local
   defaults for `api_url` (`http://localhost:8080`); secrets have **no** default (must be set).
5. `.env.example` documents the three vars; ensure real values are git-ignored.
6. A trivial smoke spec asserts the app + NemPay config load.

## Validation / tests
- `bundle exec rspec` passes (smoke spec).
- Config spec: `NemPay.api_url/secret_key/webhook_secret` resolve from ENV.
- `bin/rails s` boots with no Redis/Sidekiq dependency.

## Risks & rollback
- **Rails 8 defaults** may add Solid Queue/Cache expecting extra services — disable what isn't needed so
  the app runs on SQLite alone.
- Rollback: delete `Nem_luxury/` app files (the folder's `CLAUDE.md` stays).
