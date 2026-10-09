#!/usr/bin/env bash
# configure-domain.sh — point the server at APP_DOMAIN. Called by
# remote-deploy.sh when the GitHub variable APP_DOMAIN is set; runs as root.
#
#   APP_DOMAIN     the domain the app is served on        (required)
#   OLD_DOMAINS    space/comma separated; 301 → APP_DOMAIN (optional)
#   CERTBOT_EMAIL  Let's Encrypt account e-mail            (optional)
#
# DNS is NOT touched: the A record must already point at this server
# (Cloudflare). What it does, idempotently — an unchanged domain is a no-op:
#   1. HTTPS certificate for APP_DOMAIN, only if none is valid yet
#      (webroot challenge; the live site is never taken down for it)
#   2. regenerates /etc/nginx/sites-available/signet — ONLY that file —
#      and reloads nginx; a config nginx rejects is rolled back
#   3. sets APP_URL / FRONTEND_ORIGIN in .env.production
# The caller restarts the API afterwards, so step 3 takes effect.
set -Eeuo pipefail

APP_DIR="${APP_DIR:-/opt/signet}"
ENV_FILE="${ENV_FILE:-$APP_DIR/backend/.env.production}"
SITE="${NGINX_SITE:-/etc/nginx/sites-available/signet}"
SITE_LINK="${NGINX_SITE_LINK:-/etc/nginx/sites-enabled/signet}"
BOOTSTRAP="${NGINX_BOOTSTRAP:-/etc/nginx/sites-enabled/signet-acme-bootstrap}"
WEBROOT="${ACME_WEBROOT:-/var/www/certbot}"
PORT="${PORT:-8080}"
MARKER="# Managed by deploy/configure-domain.sh"

log() { echo "domain: $*"; }
die() { echo "domain: ERROR: $*" >&2; exit 1; }

domain_re='^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$'
APP_DOMAIN="$(tr '[:upper:]' '[:lower:]' <<<"${APP_DOMAIN:-}")"
[[ "$APP_DOMAIN" =~ $domain_re ]] || die "APP_DOMAIN '${APP_DOMAIN}' is not a valid domain name"
old=()
for d in $(tr ',[:upper:]' ' [:lower:]' <<<"${OLD_DOMAINS:-}"); do
	[[ "$d" =~ $domain_re ]] || die "OLD_DOMAINS entry '$d' is not a valid domain name"
	[[ "$d" == "$APP_DOMAIN" ]] || old+=("$d")
done

# The DNS record is added by hand in Cloudflare — fail early and clearly if
# it isn't there yet (behind the orange cloud it resolves to Cloudflare, so
# only existence is checked, not the IP).
getent ahosts "$APP_DOMAIN" >/dev/null || die "$APP_DOMAIN does not resolve — add its DNS record in Cloudflare first, then re-run the deploy"

# Another enabled site claiming the name would shadow this one.
for f in "${NGINX_ENABLED_DIR:-/etc/nginx/sites-enabled}"/*; do
	[[ -e "$f" && "$f" != "$SITE_LINK" && "$f" != "$BOOTSTRAP" && "$(readlink -f "$f")" != "$(readlink -f "$SITE")" ]] || continue
	if grep -qE "server_name[^;]*[[:space:]]${APP_DOMAIN//./\\.}([[:space:];])" "$f"; then
		die "$f already serves $APP_DOMAIN — remove it from that file first"
	fi
done

# cert_paths NAME → "fullchain privkey" of a currently valid certificate
# covering NAME (whatever certbot named it), or nothing.
cert_paths() {
	certbot certificates -d "$1" 2>/dev/null | awk '
		/Certificate Name:/ { valid = 0; c = ""; k = "" }
		/Expiry Date:/ && /VALID/ && !/INVALID/ { valid = 1 }
		/Certificate Path:/ { c = $3 }
		/Private Key Path:/ { k = $4; if (valid && c != "") { print c, k; exit } }'
}

# --- 1. certificate ----------------------------------------------------------
read -r cert key < <(cert_paths "$APP_DOMAIN" || true) || true
if [[ -z "${cert:-}" ]]; then
	log "no valid certificate for $APP_DOMAIN yet — requesting one"
	mkdir -p "$WEBROOT"
	# Temporary port-80 site for the challenge only; the current site keeps
	# serving untouched meanwhile.
	cat >"$BOOTSTRAP" <<-EOF
		$MARKER (temporary)
		server {
		    listen 80;
		    listen [::]:80;
		    server_name $APP_DOMAIN;
		    location ^~ /.well-known/acme-challenge/ { root $WEBROOT; default_type text/plain; }
		    location / { return 404; }
		}
	EOF
	nginx -t >/dev/null 2>&1 || { rm -f "$BOOTSTRAP"; die "nginx rejected the temporary challenge site"; }
	systemctl reload nginx
	email=(--register-unsafely-without-email)
	[[ -n "${CERTBOT_EMAIL:-}" ]] && email=(-m "$CERTBOT_EMAIL")
	if ! certbot certonly --webroot -w "$WEBROOT" --cert-name "$APP_DOMAIN" -d "$APP_DOMAIN" \
		--non-interactive --agree-tos "${email[@]}" --keep-until-expiring \
		--deploy-hook "systemctl reload nginx"; then
		rm -f "$BOOTSTRAP"
		systemctl reload nginx
		die "certificate request for $APP_DOMAIN failed. If the Cloudflare record is proxied (orange cloud), switch it to DNS only (grey) — or turn off 'Always Use HTTPS' — for this first deploy, then switch it back"
	fi
	rm -f "$BOOTSTRAP"
	read -r cert key < <(cert_paths "$APP_DOMAIN") || die "certbot succeeded but no certificate for $APP_DOMAIN was found"
fi
log "certificate: $cert"

# --- 2. nginx ------------------------------------------------------------------
acme="location ^~ /.well-known/acme-challenge/ { root $WEBROOT; default_type text/plain; }"
ssl_opts=""
[[ -f /etc/letsencrypt/options-ssl-nginx.conf ]] && ssl_opts="include /etc/letsencrypt/options-ssl-nginx.conf;"
[[ -f /etc/letsencrypt/ssl-dhparams.pem ]] && ssl_opts+=" ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;"
mkdir -p "$WEBROOT"

new_site="$(mktemp)"
trap 'rm -f "$new_site"' EXIT
{
	echo "$MARKER — edits here are overwritten on every deploy."
	echo "# Domain: GitHub variable APP_DOMAIN (old names: OLD_DOMAINS). See docs/CICD.md."
	cat <<-EOF

		server {
		    listen 80;
		    listen [::]:80;
		    server_name $APP_DOMAIN${old[*]:+ ${old[*]}};
		    $acme
		    location / { return 301 https://$APP_DOMAIN\$request_uri; }
		}

		server {
		    listen 443 ssl http2;
		    listen [::]:443 ssl http2;
		    server_name $APP_DOMAIN;

		    ssl_certificate     $cert;
		    ssl_certificate_key $key;
		    $ssl_opts

		    # KYC uploads are up to 20MB per form.
		    client_max_body_size 25m;

		    root $APP_DIR/frontend/dist;
		    index index.html;

		    $acme

		    location / {
		        try_files \$uri \$uri/ /index.html;
		    }

		    location /api/ {
		        proxy_pass http://127.0.0.1:$PORT;
		        proxy_set_header Host \$host;
		        proxy_set_header X-Real-IP \$remote_addr;
		        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
		        proxy_set_header X-Forwarded-Proto \$scheme;
		    }

		    location /storage/ {
		        proxy_pass http://127.0.0.1:$PORT;
		        proxy_set_header Host \$host;
		    }
		}
	EOF
	# Old names redirect over HTTPS too, using whatever certificate they
	# already have; without one they only redirect on port 80.
	for d in "${old[@]}"; do
		oc="" ok=""
		read -r oc ok < <(cert_paths "$d" || true) || true
		if [[ -z "$oc" ]]; then
			echo "# $d: no valid certificate — redirected on port 80 only"
			continue
		fi
		cat <<-EOF

			server {
			    listen 443 ssl http2;
			    listen [::]:443 ssl http2;
			    server_name $d;
			    ssl_certificate     $oc;
			    ssl_certificate_key $ok;
			    $ssl_opts
			    $acme
			    location / { return 301 https://$APP_DOMAIN\$request_uri; }
			}
		EOF
	done
} >"$new_site"

if [[ -f "$SITE" ]] && cmp -s "$new_site" "$SITE"; then
	log "nginx already configured for $APP_DOMAIN"
else
	backup=""
	if [[ -f "$SITE" ]]; then
		backup="$SITE.bak-$(date -u +%Y%m%dT%H%M%SZ)"
		cp -p "$SITE" "$backup"
		grep -qF "$MARKER" "$SITE" || log "taking over the hand-written $SITE (saved as $backup)"
		diff -u "$backup" "$new_site" | sed 's/^/domain:   /' || true
	fi
	install -m 0644 "$new_site" "$SITE"
	ln -sfn "$SITE" "$SITE_LINK"
	if ! out="$(nginx -t 2>&1)"; then
		sed 's/^/domain: nginx: /' <<<"$out" >&2
		if [[ -n "$backup" ]]; then cp -p "$backup" "$SITE"; else rm -f "$SITE" "$SITE_LINK"; fi
		die "nginx rejected the generated config — previous config restored"
	fi
	systemctl reload nginx
	log "nginx now serves $APP_DOMAIN${old[*]:+ (redirecting ${old[*]})}"
fi

# --- 3. app URLs -------------------------------------------------------------------
set_env() {
	local k="$1" v="$2"
	if grep -qE "^$k=" "$ENV_FILE"; then
		grep -qxF "$k=$v" "$ENV_FILE" && return 0
		sed -i "s#^$k=.*#$k=$v#" "$ENV_FILE"
	else
		printf '%s=%s\n' "$k" "$v" >>"$ENV_FILE"
	fi
	log "$k=$v"
}
set_env APP_URL "https://$APP_DOMAIN"
set_env FRONTEND_ORIGIN "https://$APP_DOMAIN"
