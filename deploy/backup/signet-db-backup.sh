#!/usr/bin/env bash
# signet-db-backup.sh — dump the Signet MySQL database, verify the dump,
# and upload it to S3. Run daily by signet-db-backup.timer; see
# docs/S3_SETUP.md.
#
# Configuration comes from the environment (the systemd unit loads
# /opt/signet/backend/.env.production):
#   DB_HOST, DB_PORT, DB_DATABASE, DB_USERNAME, DB_PASSWORD   required
#   AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_DEFAULT_REGION
#   BACKUP_S3_URI            default s3://$AWS_BUCKET/db-backups
#   BACKUP_DIR               local copies, default /var/backups/signet
#   BACKUP_KEEP_LOCAL_DAYS   default 3 (S3 retention is a lifecycle rule)
#   BACKUP_IGNORE_TABLES     optional, comma-separated tables to skip
#   BACKUP_UPLOAD            default true; false keeps the dump local only
#                            (no AWS needed) — used for pre-deploy backups
#   BACKUP_LABEL             optional tag in the file name, e.g. "predeploy"
#
# The dump is written locally first and only uploaded after it is
# verified complete, so a failed mysqldump never lands in S3 as a
# truncated "backup". Any failure exits non-zero, which systemd records.
set -Eeuo pipefail
umask 077

log() { echo "backup: $*"; }
die() { echo "backup: ERROR: $*" >&2; exit 1; }
trap 'rc=$?; echo "backup: ERROR: command failed at line $LINENO (exit $rc) — nothing was uploaded after this point" >&2' ERR

for v in DB_DATABASE DB_USERNAME; do
	[[ -n "${!v:-}" ]] || die "$v is not set"
done
DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-3306}"
BACKUP_UPLOAD="${BACKUP_UPLOAD:-true}"
if [[ "$BACKUP_UPLOAD" == true && -z "${BACKUP_S3_URI:-}" ]]; then
	[[ -n "${AWS_BUCKET:-}" ]] || die "set BACKUP_S3_URI or AWS_BUCKET (or BACKUP_UPLOAD=false)"
	BACKUP_S3_URI="s3://${AWS_BUCKET}/db-backups"
fi
BACKUP_S3_URI="${BACKUP_S3_URI:-}"
BACKUP_S3_URI="${BACKUP_S3_URI%/}"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/signet}"
BACKUP_KEEP_LOCAL_DAYS="${BACKUP_KEEP_LOCAL_DAYS:-3}"

required=(mysqldump gzip)
[[ "$BACKUP_UPLOAD" == true ]] && required+=(aws)
for cmd in "${required[@]}"; do
	command -v "$cmd" >/dev/null || die "$cmd not found in PATH ($PATH)"
done

mkdir -p "$BACKUP_DIR"
stamp="$(date -u +%Y-%m-%dT%H%M%SZ)"
name="${DB_DATABASE}_${BACKUP_LABEL:+${BACKUP_LABEL}_}${stamp}.sql.gz"
file="$BACKUP_DIR/$name"
partial="$file.partial"

# Credentials go in a private option file, never on the command line
# (where any user could read them from the process list).
cnf="$(mktemp)"
cleanup() { rm -f "$cnf" "$partial"; }
trap cleanup EXIT
esc() { local s="${1//\\/\\\\}"; printf '%s' "${s//\"/\\\"}"; }
{
	echo "[client]"
	echo "host=\"$(esc "$DB_HOST")\""
	echo "port=$DB_PORT"
	echo "user=\"$(esc "$DB_USERNAME")\""
	echo "password=\"$(esc "${DB_PASSWORD:-}")\""
} >"$cnf"

ignore=()
IFS=',' read -r -a skip <<<"${BACKUP_IGNORE_TABLES:-}"
for t in "${skip[@]}"; do
	t="${t// /}"
	[[ -n "$t" ]] && ignore+=("--ignore-table=${DB_DATABASE}.${t}")
done

log "dumping ${DB_DATABASE}@${DB_HOST} → $file${ignore:+ (skipping ${BACKUP_IGNORE_TABLES})}"
start=$SECONDS
# --single-transaction: consistent InnoDB snapshot without locking the app.
mysqldump --defaults-extra-file="$cnf" \
	--single-transaction --quick --hex-blob \
	--routines --triggers --events --no-tablespaces \
	"${ignore[@]}" "$DB_DATABASE" | gzip -c >"$partial"

gzip -t "$partial" || die "gzip integrity check failed"
tail_line="$(gzip -dc "$partial" | tail -n 1)"
[[ "$tail_line" == "-- Dump completed"* ]] || die "dump is incomplete (last line: ${tail_line:0:80})"
mv "$partial" "$file"
size="$(stat -c %s "$file")"
log "dump ok: $size bytes in $((SECONDS - start))s"

dest="$file"
if [[ "$BACKUP_UPLOAD" == true ]]; then
	dest="$BACKUP_S3_URI/$name"
	log "uploading → $dest"
	aws s3 cp "$file" "$dest" --only-show-errors --sse AES256

	remote_size="$(aws s3 ls "$dest" | awk '{print $3}')"
	[[ "$remote_size" == "$size" ]] || die "uploaded size '${remote_size}' != local size '${size}'"
	log "upload verified ($remote_size bytes)"
else
	log "upload skipped (BACKUP_UPLOAD=false)"
fi

find "$BACKUP_DIR" -maxdepth 1 -name "${DB_DATABASE}_*.sql.gz" -mtime +"$BACKUP_KEEP_LOCAL_DAYS" -print -delete |
	sed 's/^/backup: removed old local copy /'
log "done: $dest"
