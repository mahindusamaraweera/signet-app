# Signet

A crypto/MLM platform built as a Go REST API + Vue 3 SPA, backed by MySQL.

```
signet/
├── backend/     Go REST API (chi + sqlx + MySQL)
├── frontend/    Vue 3 SPA (Vite + vue-router + Pinia + Bootstrap 5 + SweetAlert2)
└── docs/        Reference docs: database schema, API spec, financial engine, UI spec
```

The `docs/` folder is worth reading before making changes: `schema.md` covers
the database schema, `api_spec.md` is the full route/business-logic spec,
`financial_engine.md` covers the wallet/commission/rank math, `ui_spec.md` is
the page-by-page UI spec, and `ARCHITECTURE.md` covers the stack decisions.

## Prerequisites

- Go 1.24+
- Node.js 18+ / npm
- MySQL

## Backend — run instructions

```bash
cd backend
cp .env.example .env
# edit .env: point DB_HOST/DB_PORT/DB_DATABASE/DB_USERNAME/DB_PASSWORD at
# your MySQL database

# Applies pending migrations from backend/migrations (each runs once per
# database; an existing database gets 001 recorded as a baseline, not run).
# See backend/migrations/README.md.
go run ./cmd/migrate

# Start the API (default :8080)
go run ./cmd/api
```

Build a binary instead of `go run` for production:

```bash
go build -o signet-api ./cmd/api
./signet-api
```

The API reads all configuration from environment variables (see
`.env.example` for the full list and comments) — `DB_*` for the database
connection, `SESSION_SECRET`/`SESSION_LIFETIME` for the session cookie,
`MAIL_*` for outbound email, `MINING_WEBHOOK_*` for the optional mining
webhook job, `FRONTEND_ORIGIN` for CORS, and `ENABLE_SCHEDULER` to control
whether this process runs the recurring background jobs, including enabled
blockchain package activation (mining:update,
mining:send-webhook, packages:weekly-sum, share:calculate — leave `true`
for a single-instance deployment; set `false` on every instance but one if
you run more than one API process against the same database).

Package activation uses the in-process scheduler; it needs no separate cron
entry. See [automatic activation setup](DEPLOYMENT.md#automatic-package-activation)
for the custody-service settings, migrations and manual activation fallback.

Verify the backend builds and passes static checks any time you change it:

```bash
go build ./... && go vet ./...
```

### Authentication

Session-based auth: `POST /api/v1/login` sets an httpOnly `signet_session`
cookie (JWT-signed, `SameSite=Lax`); the Vue frontend's axios client sends
it automatically (`withCredentials: true`). A separate bearer-token API for
B2B integrations lives under `/api/v1/token`, `/api/v1/check-user`,
`/api/v1/external/*` — Sanctum-compatible token storage (`personal_access_tokens`
table).

## Frontend — run instructions

```bash
cd frontend
npm install
npm run dev
```

Opens the Vite dev server on `http://localhost:5173`. It proxies `/api` and
`/storage` requests to the Go backend (default `http://127.0.0.1:8080` —
override with `VITE_API_PROXY_TARGET` if the backend runs elsewhere), so
run the backend first.

Production build:

```bash
npm run build
```

Outputs static files to `frontend/dist/` — serve them with any static file
host/CDN, or behind the same reverse proxy that fronts the Go API. Set
`FRONTEND_ORIGIN` on the backend to the deployed frontend's origin so CORS
allows it.

## Project layout notes

- `backend/internal/handlers/` — one file per controller domain, each
  exposing a `RegisterXRoutes(r chi.Router, d *app.Deps)` mounted from
  `cmd/api/main.go`. Routes are `/api/v1/...`.
- `backend/internal/wallet/`, `backend/internal/tree/` — the financial
  engine (wallet crediting, MLM tree placement/rank/ROC).
- `backend/internal/jobs/` — the recurring background jobs, wired to run
  on tickers via `jobs.StartScheduler` (see `ENABLE_SCHEDULER` above).
- `frontend/src/views/` — one `.vue` file per page/route, grouped by
  role/domain to mirror `docs/ui_spec.md`'s section layout.
- `frontend/src/components/{layout,widgets,shared}/` — reusable chrome
  (dashboard sidebar/topbar, the rank/ROC/mining dashboard widgets, tables,
  pagination, flash alerts) shared across pages.

## What's out of scope / different by design

- Real-time updates use client polling (mining widget polls every 5s)
  rather than a websocket/broadcast layer.
- Outbound mail is best-effort synchronous/goroutine SMTP send (mail
  failures are logged, never block the triggering request).
- A handful of financially- or PII-sensitive endpoints (e.g.
  `/active-package`, `/kyc/{id}/verify`, `/toggle-vacation`,
  `/mining/user/{id}`) require a valid session.

Everything else — every calculation, validation rule, hardcoded threshold,
and status enum — is documented in `docs/`.
