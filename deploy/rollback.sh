#!/usr/bin/env bash
# rollback.sh — put the previous backend binary and frontend back. Runs ON
# THE SERVER as root:  sudo bash /opt/signet/deploy/rollback.sh
#
# Swaps signet-api <-> signet-api.prev and dist <-> dist.prev, so running it
# twice returns to the newer release. Database migrations are NOT undone —
# see backend/migrations/README.md (`migrate rollback`) if one must be.
set -Eeuo pipefail

APP_DIR="${APP_DIR:-/opt/signet}"
SERVICE="${SERVICE:-signet-api}"
PORT="$(grep -E '^PORT=' "$APP_DIR/backend/.env.production" 2>/dev/null | cut -d= -f2 || true)"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:${PORT:-8080}/api/v1/health}"

die() { echo "rollback: ERROR: $*" >&2; exit 1; }
[[ $EUID -eq 0 ]] || die "run as root (sudo)"
exec 9>"${LOCK_FILE:-/var/lock/signet-deploy.lock}"
flock -n 9 || die "a deploy is running"

bin="$APP_DIR/backend/signet-api"
dist="$APP_DIR/frontend/dist"
[[ -f "$bin.prev" ]] || die "no previous backend binary ($bin.prev)"

mv "$bin" "$bin.tmp" && mv "$bin.prev" "$bin" && mv "$bin.tmp" "$bin.prev"
if [[ -d "$dist.prev" ]]; then
	mv "$dist" "$dist.tmp" && mv "$dist.prev" "$dist" && mv "$dist.tmp" "$dist.prev"
fi
rev="$APP_DIR/REVISION"
if [[ -f "$rev.prev" ]]; then
	mv "$rev" "$rev.tmp" && mv "$rev.prev" "$rev" && mv "$rev.tmp" "$rev.prev"
fi
systemctl restart "$SERVICE"

for _ in $(seq 1 30); do
	if curl -fsS --max-time 2 "$HEALTH_URL" 2>/dev/null | grep -q '"ok"'; then
		echo "rollback: done — previous release is serving"
		exit 0
	fi
	sleep 1
done
journalctl -u "$SERVICE" -n 30 --no-pager >&2 || true
die "service not healthy after rollback"
