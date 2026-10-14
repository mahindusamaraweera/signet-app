# Deployment

Signet ships as two independently deployable pieces:

- **`backend/`** — a single Go binary (no runtime dependencies beyond MySQL)
- **`frontend/`** — a Vue 3 SPA that builds to static files

They talk to each other over HTTP; nothing in the backend serves the
frontend's files, so any reverse proxy (nginx, Caddy, a CDN) that routes
`/api` and `/storage` to the Go process and everything else to the static
files works.

## 1. Prerequisites

- A Linux (or Windows) server with outbound access to your MySQL instance
- Go 1.24+ installed **only if building on the server**; otherwise build
  elsewhere and copy the binary over (Go binaries are static/self-contained
  — no Go runtime needed on the target machine)
- Node.js 18+ **only for building the frontend**; the deployed artifact is
  static HTML/CSS/JS, no Node needed at runtime
- MySQL 8+ reachable from the backend
- A domain (or subdomain) with DNS pointed at the server, if you want TLS

## 2. Database

Point the backend at your MySQL instance and run the migrations. Each
migration in `backend/migrations/` runs once per database and is recorded in
`schema_migrations`, so re-running only applies new ones:

```bash
cd backend
DB_HOST=... DB_PORT=3306 DB_DATABASE=... DB_USERNAME=... DB_PASSWORD=... \
  go run ./cmd/migrate
```

On a database that already has the Signet tables, the first run records
`001_initial.sql` as a baseline **without executing it**, so no existing
table is touched. On an empty database it creates the schema. Run
`go run ./cmd/migrate status` first to see which case applies. Details are
in [backend/migrations/README.md](backend/migrations/README.md).

## 3. Backend

### Build

```bash
cd backend
GOOS=linux GOARCH=amd64 go build -o signet-api ./cmd/api
```

Drop `GOOS`/`GOARCH` if you're building directly on the target machine.

### Configure

Copy `backend/.env.example` to wherever you keep server config and fill in
real values. The binary reads configuration from **environment variables**,
not from a `.env` file directly — export them or use a process manager
that loads an env file (systemd's `EnvironmentFile=` does this, see below).

| Variable | Purpose |
|---|---|
| `APP_NAME`, `APP_URL` | display name / base URL, used in outbound emails |
| `PORT` | port the API listens on (default `8080`) |
| `DB_HOST`, `DB_PORT`, `DB_DATABASE`, `DB_USERNAME`, `DB_PASSWORD` | MySQL connection |
| `SESSION_SECRET` | **must** be a long random value in production — signs the session JWT |
| `SESSION_LIFETIME` | session cookie lifetime, in minutes |
| `MAIL_HOST`, `MAIL_PORT`, `MAIL_USERNAME`, `MAIL_PASSWORD`, `MAIL_FROM_ADDRESS` | outbound SMTP |
| `MINING_WEBHOOK_ENABLED`, `MINING_WEBHOOK_URL`, `MINING_WEBHOOK_SECRET` | optional mining webhook job |
| `FRONTEND_ORIGIN` | the deployed frontend's origin — required for CORS with credentials |
| `ENABLE_SCHEDULER` | runs the in-process recurring jobs (mining/pool/share calculations) — see below |

Generate a real `SESSION_SECRET` rather than leaving the placeholder:

```bash
openssl rand -hex 32
```

### Run as a service (systemd example)

```ini
# /etc/systemd/system/signet-api.service
[Unit]
Description=Signet API
After=network.target

[Service]
Type=simple
User=signet
WorkingDirectory=/opt/signet/backend
EnvironmentFile=/opt/signet/backend/.env.production
ExecStart=/opt/signet/backend/signet-api
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

`.env.production` is a plain `KEY=value` file (systemd's `EnvironmentFile`
format is compatible with `.env`-style files — no `export`/quoting needed).
Keep it outside version control and readable only by the service user
(`chmod 600`).

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now signet-api
sudo journalctl -u signet-api -f   # tail logs
```

### Running more than one backend instance

If you run multiple API processes behind a load balancer (for horizontal
scaling), set `ENABLE_SCHEDULER=false` on every instance **except one**.
The scheduler runs recurring jobs (mining updates, weekly pool sums, share
calculations) in-process on a ticker — running it on every instance would
duplicate those jobs against the shared database.

### Automatic package activation

No separate cron entry is needed for the Go API. It reconciles purchases on
startup and every minute while the API service stays running. On the one API
instance that runs background jobs, configure its `EnvironmentFile`:

```dotenv
ENABLE_SCHEDULER=true
BLOCKCHAIN_ENABLED=true
BLOCKCHAIN_SERVICE_URL=https://your-custody-service.example
BLOCKCHAIN_SERVICE_TOKEN=your-server-only-service-credential
```

Use your actual custody service URL and credential. The custody service must
also have its own deposit scanner, confirmation processing and purchase queues
running; the Go scheduler cannot replace them. Top-ups additionally require
`BLOCKCHAIN_TOPUP_ENABLED=true` and the matching custody top-up/commission
processing. Keep `REGISTRATION_EXPIRY_ENABLED` independent from activation;
turning on activation does not require enabling unpaid-account deletion.

Before restarting the API, check/apply the repository migrations with the
configured database environment. In particular, migration
`033_purchase_sync_diagnostics.sql` supplies the fields shown on the funding
page. From `backend/`:

```bash
go run ./cmd/migrate status
go run ./cmd/migrate up
SIGNET_API_UNIT=signet-staging.service
sudo systemctl restart "$SIGNET_API_UNIT"
sudo systemctl status "$SIGNET_API_UNIT" --no-pager
sudo journalctl -u "$SIGNET_API_UNIT" --since '10 minutes ago' --no-pager
```

`signet-staging.service` is the unit identified in the supplied staging logs.
For production or another host, set `SIGNET_API_UNIT` to that deployment's actual
unit before running these commands.

After a confirmed deposit, use **Refresh wallet** to check the available USDT,
selected package and service fee. **Activate package** is an optional fallback:
it checks/resumes the same purchase intent, verifies ownership and delegates
balance reservation to custody. It can run with the Go scheduler disabled,
but still needs custody processing. Completed purchases hide the action;
processing purchases offer **Check activation**. Insufficient confirmed funds
never authorize activation. Retrying does not create another purchase or debit.

If a funded package stays pending, inspect the user's latest
`package_purchase_intents` row: `status`, `deposit_seen`, `available_usdt`,
`remaining_usdt`, `last_attempt_at` and `last_sync_error`. No recorded attempt
points to scheduler/service setup; a recorded error requires checking the API
and custody logs. A placement review or replacement confirmation must be
resolved through its supported flow. Do not mark `user_packages` active with a
manual SQL update: that bypasses custody confirmation, commissions and vesting.

### Tier-gated SIGNET allocation

Deploy the matching Signet-token Tier-gated allocation change with this frontend.
Package activation confirms custody funding, swap and purchaser bonus, but does
not start vesting. Purchased SIGNET plus bonus stays locked until a qualifying
Tier Option 1/2 is reviewed and confirmed. Existing external-package schedules
without either option stop future accrual; already released funds remain intact.

Custody counts settled commission/reward SIGNET credits cumulatively, including
amounts already withdrawn. When those earnings reach the original allocation,
it returns only the remaining locked allocation to the purchase's treasury.
Deposits, purchaser bonuses, transfers, vesting releases and the separate Tier
account's earnings do not count. Tier eligibility uses invested principal only.

Ensure the existing Laravel scheduler and `blockchain` worker run the new
`signet-app:allocation-recovery` minute task. Returns remain pending until a
matching on-chain receipt is confirmed. This change needs no new migration;
refresh custody config cache and restart the correct Horizon/queue service.
Deploy the frontend to show waiting, recovery-pending and recovered states.

### Live commission price credential

Custody requests `/pancakeswap/price` using the signer's separate read-only
credential. Set `SIGNER_PRICE_API_KEY` to the same long random secret in both
Laravel's runtime environment and the Node wallet signer's runtime environment.
Keep it separate from `SIGNER_API_KEY`; never use a manual token price to bypass
live commission pricing. A missing Node credential returns
`price_api_not_configured`; a mismatched credential returns `unauthorized`.
Deploy the custody price-header fix, refresh Laravel configuration with
`php artisan config:cache`, restart the actual Node signer process, and run
`php artisan horizon:terminate` so Supervisor starts workers with new config.
Verify read-only pricing with:

```bash
php artisan tinker --execute='dump(app(\App\Services\Blockchain\SignerClient::class)->pancakeSwapPrice("SIGNET", "USDT"));'
```

Existing commission outbox rows retry automatically; preserve their identities.
A signer circuit breaker may take its configured open interval to close.
The company sale waits for confirmed commission delivery.

### Commission remainder paid to company

Keep the existing commission percentages and source USD/cap accounting; there
is no fixed 60/40 split. New custody orders snapshot
`commission_basis=confirmed_swap_output_v1`. Each commission token entitlement
is calculated as `commission_usd * confirmed_swap_signet / package_price_usdt`,
rounded down to the token's base unit. Purchaser bonus and service fee are
excluded. There is no fresh live token price lookup for these entitlements.
For example, a $1,000 purchase with 10,000 confirmed swap SIGNET and $400 of
system-calculated commission pays 4,000 SIGNET, regardless of later price changes.
Actual commission percentages continue to come from Signet-app's rules.

For a first purchase, custody waits for the sealed source commission list and
all confirmed deliveries, then sells treasury SIGNET equal to the confirmed
swap output minus allocated commission SIGNET. The buyer's purchased SIGNET
and bonus remain locked and untouched. Existing income-cap/company-diversion
instructions retain their treatment and count as allocated commission tokens;
they are not paid a second time.

For top-ups, the purchase swap sends SIGNET to treasury. Commission deliveries
use the same purchase swap basis, and the remaining swap SIGNET is sold to
`SIGNET_APP_TOPUP_COMPANY_USDT_ADDRESS`. The buyer receives no new token
allocation, bonus or vesting. The reverse sale still uses a live PancakeSwap
execution quote with slippage protection; actual company USDT can differ from
the original nominal USD remainder.

Existing orders without the new basis retain their previous policy and live
conversion behavior. Existing authorized, confirming, confirmed or sealed
financial instructions are never silently repriced. Do not edit old policies or
reset payout identities to change a purchase already in progress. Reconcile
any requested historical conversion separately. New swap-based entitlement
quotes do not expire because their purchase evidence is immutable.

Deploy custody first. Set these server flags (preserve existing credentials and
approved destinations):

```dotenv
# signet-token
SIGNET_APP_COMMISSION_PREPARATION_ENABLED=true
SIGNET_APP_FIRST_COMMISSION_DELIVERY_ENABLED=true
SIGNET_APP_FIRST_COMPANY_SETTLEMENT_ENABLED=true
SIGNET_APP_TOPUP_ENABLED=true
PANCAKESWAP_ENABLED=true
# SIGNET_APP_TOPUP_COMPANY_USDT_ADDRESS must contain the approved company address.
```

Run `php artisan config:cache` and restart the correct Horizon service with
`php artisan horizon:terminate` under Supervisor. Keep the signer and the
`blockchain` queue running. No new Laravel migration is needed; existing
settlement columns store the swap identity, amounts and confirmation.
Saved per-order company opt-ins are no longer required. When runtime settlement
workers are enabled, completed first purchases previously marked `skipped` are
checked again. Custody seals the complete commission list and sells the actual
remaining token amount after confirmed payouts, including older orders whose
saved company flag was false or missing. Previously confirmed/broadcast swaps
keep their identities and amounts; never reset them. Existing address snapshots
are preserved; missing destinations are captured from the configured addresses
before a new settlement is prepared.

Then configure Signet-app:

```dotenv
ENABLE_SCHEDULER=true
BLOCKCHAIN_ENABLED=true
BLOCKCHAIN_TOPUP_ENABLED=true
BLOCKCHAIN_COMMISSION_PREPARATION_ENABLED=true
BLOCKCHAIN_COMMISSION_DELIVERY_ENABLED=true
BLOCKCHAIN_COMPANY_SETTLEMENT_ENABLED=true
```

Apply migration `034_first_company_settlement.sql` from `backend/` with
`go run ./cmd/migrate up`, rebuild/deploy the API, and restart the actual API
service (`signet-staging.service` on the supplied staging host). The source
worker rotates pending manifests every minute; retrying preserves the same
commission list and signer identity. A disputed/unprepared commission blocks
company settlement instead of assuming a smaller payout. Company settlement
failure does not deactivate a funded first package or spend its locked tokens.

Legacy secondary company accounting entries are not additional commission
entitlements. New entries are recorded as `accounting_only`. For pending
settlements, the worker can reclassify old unsubmitted `requires_reconciliation`
company rows only when a matching confirmed non-company payout and original
earn logs corroborate the role, USD amount, description and timestamp. Earn
logs and wallet/cap balances are preserved. Accounting-only rows are excluded
from the custody commission manifest; unmatched disputed rows remain blocked.
Never mark every reconciliation row confirmed or delete payment history to
unblock a sale. No new schema migration is required for this classification.

Verify a new staging first purchase with nonzero remainder: compare the
`blockchain_company_settlements` state (`pending`, `sealed`, `confirmed`),
custody `policy.company_remainder_usd`, `settlement_signet`,
`settlement_tx_hash`, `settlement_usdt`, and `settlement_confirmed_at`.
The confirmed receipt must prove the treasury SIGNET debit and company USDT
credit. Check that retrying sends no second swap and buyer locked balances
remain unchanged. Test the top-up remainder flow separately. Never replace
an ambiguous or reverted signer operation with a fresh purchase/key.

## 4. Frontend

### Build

```bash
cd frontend
npm install
npm run build
```

Outputs static files to `frontend/dist/`.

### Serve

Any static file host works — upload `frontend/dist/` to:
- a CDN/static host (S3+CloudFront, Cloudflare Pages, Netlify, Vercel — as
  a static site, not a Node app), or
- the same server as the backend, served by nginx/Caddy directly.

Whichever you choose, the frontend needs `/api` and `/storage` requests
proxied through to the backend (see the nginx example below) — it's a
single-page app that talks to those paths with `withCredentials: true`, so
they must be same-origin (or CORS-configured via `FRONTEND_ORIGIN`) for the
session cookie to work.

### Configure

`VITE_API_PROXY_TARGET` only affects the **Vite dev server** (`npm run
dev`); it has no effect on the production build. For production, the
reverse proxy is what routes `/api`/`/storage` to the backend — see below.

## 5. Reverse proxy (nginx example)

Puts the frontend and backend behind one domain, so no CORS is needed at
all (same-origin):

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.com;

    ssl_certificate     /etc/letsencrypt/live/your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;

    # Vue SPA static files
    root /opt/signet/frontend/dist;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;  # SPA client-side routing
    }

    # Go API
    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /storage/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
    }
}

server {
    listen 80;
    server_name your-domain.com;
    return 301 https://$host$request_uri;
}
```

With this setup, leave `FRONTEND_ORIGIN` pointed at `https://your-domain.com`
(same origin as the frontend) and the browser never needs cross-origin
credentials at all.

Get a certificate with [certbot](https://certbot.eff.org/) (`certbot
--nginx -d your-domain.com`) or your CDN's built-in TLS if you're not
self-hosting nginx.

## 5b. File storage and backups

KYC uploads go to local disk (`backend/storage/kyc/`) by default, or to a
private S3 bucket with `STORAGE_DRIVER=s3`. A daily `mysqldump` to the
same bucket runs from a systemd timer. For setup, migrating the existing
images and restoring a backup, see [docs/S3_SETUP.md](docs/S3_SETUP.md).

## 6. Updating a deployment

Merging a pull request into `main` deploys automatically through GitHub
Actions (backup, migrations, health check, automatic rollback). See
[docs/CICD.md](docs/CICD.md) for setup. The manual steps below are the
fallback.

```bash
# Backend
cd backend
git pull
go build -o signet-api ./cmd/api
sudo systemctl restart signet-api

# Frontend
cd frontend
git pull
npm install
npm run build
# re-upload dist/ to your static host, or it's already in place if nginx
# serves it directly from the repo checkout
```

Run `go run ./cmd/migrate` from `backend/` after every pull. It applies only
the migrations in `backend/migrations/` that this database hasn't run yet
(tracked in `schema_migrations`), so it is a no-op when nothing is new.
`go run ./cmd/migrate status` shows what is pending. See
[backend/migrations/README.md](backend/migrations/README.md).

## 7. Health check

The API doesn't expose a dedicated `/health` route yet — use any
authentication-free `GET` (e.g. `/api/v1/login` with an empty body, which
returns a `422` rather than connection-refused) as a liveness probe, or add
a proper health endpoint if your infra requires one.
