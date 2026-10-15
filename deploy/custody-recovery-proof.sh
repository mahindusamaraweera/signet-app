#!/usr/bin/env bash
# Read-only proof generator. Run on the production API host after stopping API.
set -Eeuo pipefail
umask 077
APP_DIR="${APP_DIR:-/opt/signet}"
SERVICE="${SERVICE:-signet-api}"
ENV_FILE="${ENV_FILE:-$APP_DIR/backend/.env.production}"
if systemctl is-active --quiet "$SERVICE"; then
  echo "Stop $SERVICE before taking a recovery snapshot." >&2
  exit 1
fi
[[ -r "$ENV_FILE" ]] || { echo "Cannot read API environment file: $ENV_FILE" >&2; exit 1; }
# Parse plain/quoted KEY=value without evaluating shell substitutions.
while IFS= read -r line || [[ -n "$line" ]]; do
  line="${line%$'\r'}"
  [[ "$line" =~ ^[[:space:]]*(#|$) ]] && continue
  [[ "$line" =~ ^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)=(.*)$ ]] || continue
  key="${BASH_REMATCH[1]}" value="${BASH_REMATCH[2]}"
  if [[ "$value" =~ ^\"(.*)\"$ || "$value" =~ ^\'(.*)\'$ ]]; then value="${BASH_REMATCH[1]}"; fi
  export "$key=$value"
done < "$ENV_FILE"
exec "$APP_DIR/backend/custody-recovery-proof" "$@"
