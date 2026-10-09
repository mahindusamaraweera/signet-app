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
