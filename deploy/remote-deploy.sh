#!/usr/bin/env bash
# remote-deploy.sh — installs one release tarball built by
# .github/workflows/deploy.yml. Runs ON THE SERVER, as root:
#
#   sudo bash remote-deploy.sh /tmp/signet-<sha>.tar.gz
#
# Order matters — each step only runs if everything before it succeeded:
#   1. database backup        (abort if it fails: never migrate without one)
#   2. migrations             (only pending ones; see backend/migrations)
#   3. backend binary swap + restart + health check
#        → health check fails: previous binary restored, deploy fails
#   4. frontend swap          (only after the new backend is healthy)
#   5. deploy/ scripts refreshed, old releases pruned
#
# Layout it maintains (same as DEPLOYMENT.md, so nginx/systemd are untouched):
#   $APP_DIR/backend/signet-api        (+ signet-api.prev for rollback)
#   $APP_DIR/frontend/dist             (+ dist.prev for rollback)
#   $APP_DIR/releases/<sha>/           last $KEEP_RELEASES unpacked releases
#   $APP_DIR/REVISION                  commit currently deployed
set -Eeuo pipefail
umask 022

APP_DIR="${APP_DIR:-/opt/signet}"
SERVICE="${SERVICE:-signet-api}"
ENV_FILE="${ENV_FILE:-$APP_DIR/backend/.env.production}"
KEEP_RELEASES="${KEEP_RELEASES:-5}"

log() { echo "deploy: $*"; }
die() { echo "deploy: ERROR: $*" >&2; exit 1; }
trap 'rc=$?; echo "deploy: ERROR: failed at line $LINENO (exit $rc)" >&2' ERR

[[ $EUID -eq 0 ]] || die "run as root (sudo)"
tarball="${1:-}"
[[ -f "$tarball" ]] || die "usage: remote-deploy.sh <release tarball>"
[[ -f "$ENV_FILE" ]] || die "$ENV_FILE not found"

# One deploy at a time, even if two workflow runs reach the server.
exec 9>"${LOCK_FILE:-/var/lock/signet-deploy.lock}"
flock -n 9 || die "another deploy is already running"

# Read KEY=value lines like systemd's EnvironmentFile does — no shell
# evaluation, so a "$" or space in a password can't break or inject.
load_env() {
	local line k v
	while IFS= read -r line || [[ -n "$line" ]]; do
		line="${line%$'\r'}"
		[[ "$line" =~ ^[[:space:]]*(#|$) ]] && continue
		[[ "$line" =~ ^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]] || continue
		k="${BASH_REMATCH[1]}" v="${BASH_REMATCH[2]}"
		if [[ "$v" =~ ^\"(.*)\"$ || "$v" =~ ^\'(.*)\'$ ]]; then v="${BASH_REMATCH[1]}"; fi
		export "$k=$v"
	done <"$1"
}
load_env "$ENV_FILE"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:${PORT:-8080}/api/v1/health}"

# --- unpack -------------------------------------------------------------
rev="$(tar -xzOf "$tarball" ./REVISION | tr -d '[:space:]')"
[[ "$rev" =~ ^[0-9a-f]{7,40}$ ]] || die "tarball has no valid REVISION"
rel="$APP_DIR/releases/$rev"
rm -rf "$rel"
mkdir -p "$rel"
tar -xzf "$tarball" -C "$rel" --no-same-owner
rm -f "$tarball"
for f in signet-api migrate frontend/index.html; do
	[[ -e "$rel/$f" ]] || die "release is missing $f"
done
log "release $rev unpacked → $rel"

# The binary keeps whatever owner the service already uses.
owner="$(stat -c %U:%G "$APP_DIR/backend/signet-api" 2>/dev/null || stat -c %U:%G "$APP_DIR/backend")"

# --- 1. backup ------------------------------------------------------------
# Uploads to S3 when the bucket is configured, otherwise keeps it local.
upload=false
[[ -n "${AWS_BUCKET:-}" || -n "${BACKUP_S3_URI:-}" ]] && command -v aws >/dev/null && upload=true
log "pre-deploy database backup (upload to S3: $upload)"
BACKUP_UPLOAD="$upload" BACKUP_LABEL=predeploy bash "$rel/deploy/backup/signet-db-backup.sh"

# --- 2. migrations --------------------------------------------------------
log "running migrations"
if ! (cd "$APP_DIR/backend" && "$rel/migrate"); then
	die "migrations failed — nothing was swapped, the previous release is still live"
fi

# --- 2b. domain (only when the GitHub variable APP_DOMAIN is set) ----------
# Before the restart below, so a changed APP_URL is picked up by it.
if [[ -n "${APP_DOMAIN:-}" ]]; then
	log "configuring domain $APP_DOMAIN"
	if ! APP_DIR="$APP_DIR" ENV_FILE="$ENV_FILE" PORT="${PORT:-8080}" bash "$rel/deploy/configure-domain.sh"; then
		die "domain setup failed — nothing was swapped, the previous release is still live"
	fi
fi

# --- 3. backend -----------------------------------------------------------
bin="$APP_DIR/backend/signet-api"
if [[ -f "$bin" ]]; then
	cp -p "$bin" "$bin.prev"
fi
install -o "${owner%%:*}" -g "${owner##*:}" -m 0755 "$rel/signet-api" "$bin.new"
mv -f "$bin.new" "$bin"
log "restarting $SERVICE"
systemctl restart "$SERVICE"

healthy() {
	local i
	for i in $(seq 1 30); do
		if curl -fsS --max-time 2 "$HEALTH_URL" 2>/dev/null | grep -q '"ok"'; then
			return 0
		fi
		sleep 1
	done
	return 1
}

if ! healthy; then
	echo "deploy: ERROR: $SERVICE is not healthy at $HEALTH_URL after 30s — recent log:" >&2
	journalctl -u "$SERVICE" -n 30 --no-pager >&2 || true
	if [[ -f "$bin.prev" ]]; then
		log "rolling back to the previous binary"
		mv -f "$bin.prev" "$bin"
		systemctl restart "$SERVICE"
		healthy && log "previous binary is serving again" || echo "deploy: ERROR: previous binary is not healthy either" >&2
	fi
	die "deploy of $rev failed; frontend left unchanged. Migrations already applied stay applied."
fi
log "backend healthy"

# --- 4. frontend ------------------------------------------------------------
dist="$APP_DIR/frontend/dist"
mkdir -p "$APP_DIR/frontend"
rm -rf "$dist.new"
cp -a "$rel/frontend" "$dist.new"
chmod -R a+rX "$dist.new"
if [[ -d "$dist" ]]; then
	rm -rf "$dist.prev"
	mv "$dist" "$dist.prev"
fi
mv "$dist.new" "$dist"
log "frontend swapped"

# --- 5. housekeeping --------------------------------------------------------
mkdir -p "$APP_DIR/deploy"
cp -a "$rel/deploy/." "$APP_DIR/deploy/"
chmod +x "$APP_DIR"/deploy/*.sh "$APP_DIR"/deploy/backup/*.sh
[[ -f "$APP_DIR/REVISION" ]] && cp "$APP_DIR/REVISION" "$APP_DIR/REVISION.prev"
echo "$rev" >"$APP_DIR/REVISION"

ls -1dt "$APP_DIR"/releases/*/ 2>/dev/null | tail -n +"$((KEEP_RELEASES + 1))" | xargs -r rm -rf
log "done — $rev is live"
