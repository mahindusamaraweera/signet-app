# CI/CD with GitHub Actions

Two workflows live in `.github/workflows/`:

| Workflow | Runs when | Does |
|---|---|---|
| `ci.yml` | every pull request into `main` | gofmt, `go vet`, `go test`, frontend `npm run build` |
| `deploy.yml` | a pull request into `main` is **merged**, or by hand (Actions → Deploy → Run workflow) | CI again on the merged commit → build → deploy to the Lightsail server |

A PR closed **without** merging deploys nothing. Two merges in quick
succession deploy one after the other, never at the same time.

## What a deploy does

**On GitHub:**
1. Runs the full CI suite on the merged commit. If anything fails, nothing
   ships.
2. Builds `signet-api` and `migrate` as static Linux binaries, plus
   `frontend/dist`, and packs them with `deploy/` into one tarball. The
   server never compiles anything.

**On the server:** it copies the tarball over SSH and runs
`deploy/remote-deploy.sh` as root:
1. **Database backup.** Uploaded to S3 if `AWS_BUCKET` is set in
   `.env.production`, otherwise kept in `/var/backups/signet/`. If the
   backup fails, the deploy stops and nothing changes.
2. **Migrations.** Only pending ones run (see `backend/migrations/`). If a
   migration fails, the deploy stops before anything is swapped.
3. **Backend.** The new binary replaces `/opt/signet/backend/signet-api`
   (the old one is kept as `signet-api.prev`), then `systemctl restart
   signet-api`, then `/api/v1/health` must answer within 30 seconds. If it
   doesn't, **the previous binary is put back automatically** and the
   deploy fails.
4. **Frontend.** `/opt/signet/frontend/dist` is swapped (the old one is
   kept as `dist.prev`). This only happens after the backend is healthy.
5. **Housekeeping.** `/opt/signet/deploy/` is refreshed,
   `/opt/signet/REVISION` records the live commit, and the last 5 releases
   are kept in `/opt/signet/releases/`.

Finally, the workflow checks `https://go.signetint.net/api/v1/health` from
outside.

nginx and the systemd unit are not touched. The paths are the same as in
[DEPLOYMENT.md](../DEPLOYMENT.md).

---

## One-time setup

### 1. A deploy SSH key

Create a key used only by GitHub, on your own machine:

```bash
ssh-keygen -t ed25519 -C "github-actions-deploy" -f signet_deploy -N ""
```

Put the **public** key on the server, for the `ubuntu` user:

```bash
cat signet_deploy.pub | ssh ubuntu@<server-ip> 'cat >> ~/.ssh/authorized_keys'
```

The `ubuntu` user on Lightsail already has passwordless `sudo`, which the
deploy script needs.

### 2. The server's host key

GitHub only connects to a server whose identity it already knows, which
stops a man-in-the-middle from receiving your release. Get the key:

```bash
ssh-keyscan -t ed25519 <server-ip-or-domain>
```

Check the fingerprint matches the server's own key, then keep the whole
output line:

```bash
ssh-keygen -lf <(ssh-keyscan -t ed25519 <server>)          # from your PC
ssh ubuntu@<server> 'ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub'  # on the server
```

### 3. GitHub secrets

In the repo, go to **Settings → Environments → New environment →
`production`**. Under *Environment secrets*, add:

| Secret | Value |
|---|---|
| `DEPLOY_HOST` | server IP or hostname, e.g. the Lightsail static IP |
| `DEPLOY_USER` | `ubuntu` |
| `DEPLOY_SSH_KEY` | the whole contents of the **private** key file `signet_deploy` |
| `DEPLOY_KNOWN_HOSTS` | the `ssh-keyscan` line from step 2 |
| `DEPLOY_PORT` | *(optional)* only if SSH isn't on port 22 |

Optional **variables** (same screen, *Environment variables*):

| Variable | Default |
|---|---|
| `APP_DOMAIN` | unset: the deploy doesn't touch nginx. Set it to let deploys manage the domain; see [Changing the domain](#changing-the-domain) |
| `OLD_DOMAINS` | unset. Old domains that should redirect to `APP_DOMAIN` |
| `CERTBOT_EMAIL` | unset. E-mail for Let's Encrypt expiry notices |
| `DEPLOY_URL` | `https://<APP_DOMAIN>`, else `https://go.signetint.net`. Used for the final health check |
| `DEPLOY_GOARCH` | `amd64`; set to `arm64` for an ARM server |

Then delete `signet_deploy` and `signet_deploy.pub` from your PC, or keep
them in a password manager.

You can also add **Required reviewers** to the `production` environment.
Each deploy then waits for someone to click "Approve" in the Actions tab.

### 4. Protect `main`

Go to **Settings → Branches → Add rule** for `main`:
- Require a pull request before merging.
- Require status checks to pass, and select **Backend (Go)** and
  **Frontend (Vue)**. They appear after CI has run once.

This way nothing reaches `main`, and nothing gets deployed, without going
through review and CI.

### 5. Firewall

GitHub's runners connect from changing IP addresses. If the Lightsail
firewall limits SSH (port 22) to your own IP, the deploy can't connect.
Either allow SSH from anywhere (key-only login, with password login
disabled, is the default on Lightsail), or use a self-hosted runner.

### 6. First run

Run it by hand once: **Actions → Deploy → Run workflow → main**. Watch the
*Deploy to production* step. A good run ends with:

```
deploy: backend healthy
deploy: frontend swapped
deploy: done — <commit> is live
✓ https://go.signetint.net is healthy
```

---

## Day to day

- **Ship a change:** open a PR into `main`, wait for CI to pass, then merge.
  The deploy starts automatically.
- **See what's live:** run `cat /opt/signet/REVISION` on the server, or
  look at the latest Deploy run.
- **Logs of a deploy:** the Actions run. Everything the server script
  prints shows up there.

## Changing the domain

The domain is a GitHub variable. The server's IP doesn't change; you add
the DNS record yourself in Cloudflare.

1. **Cloudflare:** add an `A` record for the new name pointing at the
   server's static IP. **Set it to DNS only (grey cloud) for now.** The
   first certificate request needs to reach the server directly.
2. **GitHub → Settings → Environments → production → Variables:**
   - `APP_DOMAIN` = the new domain, for example `app.example.com`
   - `OLD_DOMAINS` = the previous domain(s), for example `go.signetint.net`.
     They keep working and redirect to the new one, so shared referral
     links don't break.
3. **Actions → Deploy → Run workflow** (or merge any PR).
4. When the run is green, switch the Cloudflare record to **Proxied
   (orange)** if you want, with **SSL/TLS mode Full (strict)**.

What the deploy does, in `deploy/configure-domain.sh`:
- Stops early if the name doesn't resolve yet. Nothing is changed.
- Gets a Let's Encrypt certificate for the new name if it has none. The
  live site keeps serving while it does. Renewal is automatic and also
  works behind the Cloudflare proxy.
- Regenerates **only** `/etc/nginx/sites-available/signet` and reloads
  nginx. Other sites (phpMyAdmin) are untouched. The previous file is
  saved next to it as `signet.bak-<time>`. If nginx rejects the new
  config, the old one is put back and the deploy fails.
- Sets `APP_URL` and `FRONTEND_ORIGIN` in `.env.production`, so referral
  links and password-reset e-mails use the new domain.

With `APP_DOMAIN` set to the current domain, deploys just keep the config
in sync. The **first** such deploy replaces the hand-written nginx file
with the generated one; the run log shows the diff.

Users have to log in once on the new domain, because browsers keep the
login cookie per domain. Google Authenticator codes are not affected.

## Rolling back

**Automatic:** if the new backend fails its health check, the deploy puts
the previous binary back by itself.

**By hand,** if a release is healthy but wrong. On the server:

```bash
sudo bash /opt/signet/deploy/rollback.sh
```

This swaps the backend binary, the frontend and `REVISION` back to the
previous release. Running it a second time switches forward again.

Or revert the PR on GitHub and merge the revert. That deploys the old code
as a new release.

**Database migrations are never undone automatically.** Write migrations
so the previous release still works with the new schema (add columns and
tables; don't rename or drop in the same release). If one must be undone,
see `migrate rollback` in
[backend/migrations/README.md](../backend/migrations/README.md). The
pre-deploy backup is in `/var/backups/signet/*_predeploy_*.sql.gz` (and in
S3 if configured).

## Troubleshooting

| Symptom | Fix |
|---|---|
| `Host key verification failed` | `DEPLOY_KNOWN_HOSTS` doesn't match the server, e.g. after rebuilding the instance. Redo step 2. |
| `Permission denied (publickey)` | The public key isn't in `~ubuntu/.ssh/authorized_keys`, or `DEPLOY_SSH_KEY` is missing a line. Paste the whole file, including the `BEGIN`/`END` lines. |
| `sudo: a password is required` | `DEPLOY_USER` has no passwordless sudo. Use `ubuntu`, or add a sudoers rule. |
| Connection timed out | Lightsail firewall, see step 5. |
| `another deploy is already running` | A previous deploy is still running, or crashed holding the lock. Wait, or check `ps aux \| grep remote-deploy`. |
| Backup fails | Check `DB_*` in `.env.production`. The deploy refuses to migrate without a backup. |
| `not healthy ... after 30s` | The new release crashed on start. The log tail is in the Actions output, and the previous binary has already been restored. |
