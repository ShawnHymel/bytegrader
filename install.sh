#!/usr/bin/env bash
# =============================================================================
# ByteGrader Unified Deployment Script
# =============================================================================
# Replaces setup-server.sh, deploy/deploy.sh, and setup-ssl.sh.
# Reads all configuration from config.yaml — no interactive prompts.
#
# Usage:
#   sudo bash install.sh [--skip-nginx] [--skip-build]
#
# Options:
#   --skip-nginx    Skip nginx and SSL setup entirely. App is reachable directly
#                   on port 8080 for pre-DNS testing (blue/green deployments).
#                   Once DNS is pointed at this server, re-run with --skip-build
#                   to configure nginx and obtain SSL certificates.
#   --skip-build    Skip Docker image builds. Re-runs nginx and SSL configuration
#                   using existing containers. Use this after DNS has propagated.
# =============================================================================

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'
info()    { echo -e "${BLUE}ℹ${NC}  $*"; }
success() { echo -e "${GREEN}✅${NC} $*"; }
warn()    { echo -e "${YELLOW}⚠${NC}  $*"; }
die()     { echo -e "${RED}❌ ERROR:${NC} $*" >&2; exit 1; }
header()  { echo -e "\n${BOLD}━━━ $* ━━━${NC}"; }

# ── Args ──────────────────────────────────────────────────────────────────────
SKIP_NGINX=false
SKIP_BUILD=false
for arg in "$@"; do
  case $arg in
    --skip-nginx) SKIP_NGINX=true ;;
    --skip-build) SKIP_BUILD=true ;;
    *) die "Unknown argument: $arg" ;;
  esac
done

[[ $EUID -ne 0 ]] && die "Run as root: sudo bash install.sh"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG="$SCRIPT_DIR/config.yaml"
[[ -f "$CONFIG" ]] || die "config.yaml not found at $SCRIPT_DIR"

# ── Step 1: Dependencies ──────────────────────────────────────────────────────
header "Step 1 — Installing Dependencies"

apt-get update -qq
apt-get install -y -qq \
  ca-certificates curl wget git vim htop ufw \
  nginx certbot python3-certbot-nginx python3-yaml \
  > /dev/null 2>&1
success "System packages ready"

if ! command -v docker &>/dev/null; then
  info "Installing Docker..."
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
    -o /etc/apt/keyrings/docker.asc 2>/dev/null
  chmod a+r /etc/apt/keyrings/docker.asc
  echo \
    "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
     https://download.docker.com/linux/ubuntu \
     $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}") stable" \
    | tee /etc/apt/sources.list.d/docker.list > /dev/null
  apt-get update -qq
  apt-get install -y -qq \
    docker-ce docker-ce-cli containerd.io \
    docker-buildx-plugin docker-compose-plugin \
    > /dev/null 2>&1
  success "Docker installed"
else
  success "Docker already installed"
fi

# ── Step 2: Read config ───────────────────────────────────────────────────────
header "Step 2 — Reading Configuration"

cfg() {
  python3 - "$1" <<'PYEOF'
import sys, yaml, os
os.chdir(os.path.dirname(os.path.abspath("config.yaml")))
key = sys.argv[1]
with open("config.yaml") as f:
    d = yaml.safe_load(f)
val = d.get(key, "")
if isinstance(val, list):
    print("\n".join(str(v) for v in val if v))
elif val is None:
    print("")
else:
    print(str(val))
PYEOF
}

cd "$SCRIPT_DIR"

DOMAIN=$(cfg "domain");           [[ -z "$DOMAIN" ]]      && die "domain is required in config.yaml"
SUBDOMAIN=$(cfg "subdomain");     [[ -z "$SUBDOMAIN" ]]   && die "subdomain is required in config.yaml"
SSL_EMAIL=$(cfg "ssl_email");     [[ -z "$SSL_EMAIL" ]]   && die "ssl_email is required in config.yaml"
[[ "$SSL_EMAIL" == "you@example.com" ]] && die "Please set a real ssl_email in config.yaml"

FULL_DOMAIN="${SUBDOMAIN}.${DOMAIN}"
APP_DIR=$(cfg "app_dir");         APP_DIR="${APP_DIR:-/home/bytegrader/app}"
BG_USER=$(cfg "bytegrader_user"); BG_USER="${BG_USER:-bytegrader}"
APP_PORT=$(cfg "app_port");       APP_PORT="${APP_PORT:-8080}"

mapfile -t API_KEY_ARRAY < <(cfg "api_keys")
API_KEYS_CSV=$(IFS=,; echo "${API_KEY_ARRAY[*]:-}")
PRIMARY_API_KEY="${API_KEY_ARRAY[0]:-}"

mapfile -t IP_WHITELIST < <(cfg "ip_whitelist")

GRADERS_PATH=$(cfg "graders_local_path"); [[ -z "$GRADERS_PATH" ]] && die "graders_local_path is required in config.yaml"

# Print config info
info "Domain:       $FULL_DOMAIN"
info "App dir:      $APP_DIR"
info "User:         $BG_USER"
info "API keys:     ${#API_KEY_ARRAY[@]} configured"
info "IP whitelist: ${#IP_WHITELIST[@]} entries (0 = open)"
info "Graders path: $GRADERS_PATH"

# Validate graders path
[[ -d "$GRADERS_PATH" ]] || die "graders_local_path does not exist: $GRADERS_PATH"
[[ -f "$GRADERS_PATH/registry.yaml" ]] || die "registry.yaml not found in $GRADERS_PATH"
[[ -f "$GRADERS_PATH/build.sh" ]] || die "build.sh not found in $GRADERS_PATH"
success "Graders path validated: $GRADERS_PATH"

# ── Step 3: System user ───────────────────────────────────────────────────────
header "Step 3 — System User"

if ! id "$BG_USER" &>/dev/null; then
  adduser --disabled-password --gecos "" "$BG_USER"
  success "Created user: $BG_USER"
else
  success "User $BG_USER already exists"
fi
usermod -aG docker "$BG_USER"

if [[ -f /root/.ssh/authorized_keys ]]; then
  mkdir -p "/home/$BG_USER/.ssh"
  cp /root/.ssh/authorized_keys "/home/$BG_USER/.ssh/"
  chown -R "$BG_USER:$BG_USER" "/home/$BG_USER/.ssh"
  chmod 700 "/home/$BG_USER/.ssh"
  chmod 600 "/home/$BG_USER/.ssh/authorized_keys"
fi

# ── Step 4: Environment file ──────────────────────────────────────────────────
header "Step 4 — Environment File"

ENV_FILE="/home/$BG_USER/.bytegrader_env"
cat > "$ENV_FILE" <<EOF
# Generated by deploy.sh — edit config.yaml and re-run to update
BYTEGRADER_DOMAIN=${DOMAIN}
BYTEGRADER_SUBDOMAIN=${SUBDOMAIN}
BYTEGRADER_FULL_DOMAIN=${FULL_DOMAIN}
BYTEGRADER_SSL_EMAIL=${SSL_EMAIL}
BYTEGRADER_API_KEYS=${API_KEYS_CSV}
BYTEGRADER_APP_PORT=${APP_PORT}
BYTEGRADER_APP_DIR=${APP_DIR}
BYTEGRADER_USER=${BG_USER}
BYTEGRADER_REQUIRE_API_KEY=$([[ -n "$API_KEYS_CSV" ]] && echo "true" || echo "false")
BYTEGRADER_VALID_API_KEYS=${API_KEYS_CSV}
EOF
chown "$BG_USER:$BG_USER" "$ENV_FILE"
chmod 600 "$ENV_FILE"
success "Environment file written"

# ── Step 5: Deploy app ────────────────────────────────────────────────────────
header "Step 5 — Deploying Application"

mkdir -p "$APP_DIR"
chown "$BG_USER:$BG_USER" "$APP_DIR"

if [[ "$SKIP_BUILD" == false ]]; then
  info "Building and deploying containers..."

  # Copy server source files
  mkdir -p "$APP_DIR/server"
  cp "$SCRIPT_DIR/server"/*.go "$APP_DIR/server/"
  cp "$SCRIPT_DIR/server/go.mod" "$APP_DIR/server/"
  cp "$SCRIPT_DIR/server/go.sum" "$APP_DIR/server/"
  cp "$SCRIPT_DIR/VERSION" "$APP_DIR/"
  success "Copied server source files"

  # Copy graders
  mkdir -p "$APP_DIR/graders"
  cp -r "$GRADERS_PATH/." "$APP_DIR/graders/"

  # If graders path is not the main bytegrader repo, copy framework files
  BYTEGRADER_GRADERS="$SCRIPT_DIR/graders"
  if [[ "$GRADERS_PATH" != "$BYTEGRADER_GRADERS" ]]; then
    info "Copying framework files (main.py, result.py) from bytegrader repo..."
    [[ -f "$BYTEGRADER_GRADERS/main.py" ]] || die "main.py not found in $BYTEGRADER_GRADERS"
    [[ -f "$BYTEGRADER_GRADERS/result.py" ]] || die "result.py not found in $BYTEGRADER_GRADERS"
    cp "$BYTEGRADER_GRADERS/main.py" "$APP_DIR/graders/"
    cp "$BYTEGRADER_GRADERS/result.py" "$APP_DIR/graders/"
    success "Copied framework files"
  fi
  success "Copied graders from $GRADERS_PATH"

  # Get git commit info
  GIT_COMMIT=$(cd "$SCRIPT_DIR" && git rev-parse --short HEAD 2>/dev/null || echo "unknown")
  info "Git commit: $GIT_COMMIT"

  # Get user/group IDs
  DOCKER_USER_ID=$(id -u "$BG_USER")
  DOCKER_GROUP_ID=$(getent group docker | cut -d: -f3)
  info "User ID: $DOCKER_USER_ID, Docker Group ID: $DOCKER_GROUP_ID"

  # Copy and process docker-compose.yaml
  export COURSE_SUBDOMAIN="$SUBDOMAIN"
  export BYTEGRADER_REQUIRE_API_KEY=$([[ -n "$API_KEYS_CSV" ]] && echo "true" || echo "false")
  export BYTEGRADER_VALID_API_KEYS="$API_KEYS_CSV"
  export BYTEGRADER_ALLOWED_IPS=$(IFS=,; echo "${IP_WHITELIST[*]:-}")
  export BYTEGRADER_MAX_FILE_SIZE_MB=${BYTEGRADER_MAX_FILE_SIZE_MB:-50}
  export BYTEGRADER_GRADING_TIMEOUT_MIN=${BYTEGRADER_GRADING_TIMEOUT_MIN:-10}
  export BYTEGRADER_MAX_CONCURRENT_JOBS=${BYTEGRADER_MAX_CONCURRENT_JOBS:-2}
  export BYTEGRADER_RATE_LIMIT_ENABLED=${BYTEGRADER_RATE_LIMIT_ENABLED:-true}
  export BYTEGRADER_RATE_LIMIT_REQUESTS=${BYTEGRADER_RATE_LIMIT_REQUESTS:-100}
  export BYTEGRADER_RATE_LIMIT_WINDOW_MIN=${BYTEGRADER_RATE_LIMIT_WINDOW_MIN:-5}
  export BYTEGRADER_CLEANUP_INTERVAL_HOURS=${BYTEGRADER_CLEANUP_INTERVAL_HOURS:-1}
  export BYTEGRADER_COMPLETED_JOB_TTL_HOURS=${BYTEGRADER_COMPLETED_JOB_TTL_HOURS:-24}
  export BYTEGRADER_FAILED_JOB_TTL_HOURS=${BYTEGRADER_FAILED_JOB_TTL_HOURS:-24}
  export BYTEGRADER_OLD_FILE_TTL_HOURS=${BYTEGRADER_OLD_FILE_TTL_HOURS:-48}
  export DOCKER_USER_ID
  export DOCKER_GROUP_ID
  envsubst < "$SCRIPT_DIR/deploy/docker-compose.yaml" > "$APP_DIR/docker-compose.yaml"
  success "Generated docker-compose.yaml"

  # Copy Dockerfile
  cp "$SCRIPT_DIR/deploy/Dockerfile" "$APP_DIR/"
  success "Copied Dockerfile"

  # Create runtime directories
  mkdir -p "$APP_DIR/uploads" "$APP_DIR/logs" "$APP_DIR/workspace"
  chown -R "$DOCKER_USER_ID:$DOCKER_GROUP_ID" "$APP_DIR/workspace" || true
  success "Created runtime directories"

  # Build grader images
  info "Building grader images..."
  cd "$APP_DIR/graders"
  chmod +x build.sh
  bash build.sh
  cd "$SCRIPT_DIR"
  success "Built grader images"

  # Build and start app container
  cd "$APP_DIR"
  docker compose down 2>/dev/null || true
  docker compose build --no-cache \
    --build-arg USER_ID="$DOCKER_USER_ID" \
    --build-arg GROUP_ID="$DOCKER_GROUP_ID" \
    --build-arg GIT_COMMIT="$GIT_COMMIT"

  # Fix volume permissions
  docker volume create bytegrader-workspace 2>/dev/null || true
  docker run --rm -v bytegrader-workspace:/workspace alpine sh -c \
    "chown -R $DOCKER_USER_ID:$DOCKER_GROUP_ID /workspace && chmod -R 755 /workspace"

  # Copy health check script
  cp "$SCRIPT_DIR/deploy/health-check.sh" "$APP_DIR/"
  chmod +x "$APP_DIR/health-check.sh"

  docker compose up -d
  cd "$SCRIPT_DIR"
  success "App container built and started"

  # Patch docker-compose.yaml for Blue/Green support
  sed -i 's|"127.0.0.1:8080:8080"|"127.0.0.1:${APP_PORT:-8080}:8080"|g' "$APP_DIR/docker-compose.yaml"
  sed -i 's|container_name: bytegrader-.*|container_name: ${CONTAINER_NAME:-bytegrader-app}|g' "$APP_DIR/docker-compose.yaml"
  success "Patched docker-compose.yaml for Blue/Green support"

else
  warn "--skip-build: bringing up existing containers"
  cd "$APP_DIR" && docker compose up -d 2>/dev/null || true
fi

# Wait for healthy
info "Waiting for service..."
for i in $(seq 1 30); do
  if curl -sf "http://localhost:${APP_PORT}/health" &>/dev/null; then
    success "App is healthy"
    break
  fi
  sleep 2
  [[ $i -eq 30 ]] && die "App did not become healthy. Check: cd $APP_DIR && docker compose logs"
done

# ── Step 6: nginx ─────────────────────────────────────────────────────────────
if [[ "$SKIP_NGINX" == true ]]; then
  header "Step 6 — Nginx (skipped)"
  warn "Nginx configuration skipped. App is reachable directly at:"
  warn "  http://<SERVER_IP>:${APP_PORT}/health"
  warn "Once DNS is pointed at this server, re-run to configure nginx + SSL:"
  warn "  sudo bash install.sh --skip-build"
else
  header "Step 6 — Configuring Nginx"

  # Build IP restriction block
  IP_BLOCK=""
  if [[ ${#IP_WHITELIST[@]} -gt 0 && -n "${IP_WHITELIST[0]}" ]]; then
    for ip in "${IP_WHITELIST[@]}"; do
      [[ -n "$ip" ]] && IP_BLOCK+="        allow ${ip};\n"
    done
    IP_BLOCK+="        deny all;\n"
  fi

  mkdir -p /var/www/html

  # Write HTTP-only config initially (certbot needs HTTP to work)
  cat > /etc/nginx/sites-available/bytegrader <<NGINX_HTTP
# ByteGrader nginx config — generated by install.sh

server {
    listen 80;
    server_name ${DOMAIN} www.${DOMAIN};
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://github.com/ShawnHymel/bytegrader; }
}

server {
    listen 80;
    server_name ${FULL_DOMAIN};
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / {
        proxy_pass http://localhost:${APP_PORT};
        proxy_set_header Host \$host;
        proxy_read_timeout 300s;
        client_max_body_size 100M;
    }
}
NGINX_HTTP

  ln -sf /etc/nginx/sites-available/bytegrader /etc/nginx/sites-enabled/bytegrader
  rm -f /etc/nginx/sites-enabled/default
  nginx -t && systemctl reload nginx
  success "Nginx configured (HTTP)"

# ── Step 7: Firewall ──────────────────────────────────────────────────────────
header "Step 7 — Firewall"

ufw allow OpenSSH > /dev/null 2>&1
if [[ "$SKIP_NGINX" == true ]]; then
  # Expose app port directly for pre-DNS testing
  ufw allow "${APP_PORT}/tcp" > /dev/null 2>&1
  success "Firewall: SSH + port ${APP_PORT} open (nginx skipped)"
else
  ufw allow 'Nginx Full' > /dev/null 2>&1
  # Close direct app port if it was previously opened
  ufw delete allow "${APP_PORT}/tcp" > /dev/null 2>&1 || true
  success "Firewall: SSH + HTTP/HTTPS open"
fi
ufw --force enable > /dev/null 2>&1

# ── Step 8: SSL ───────────────────────────────────────────────────────────
  header "Step 8 — SSL Certificate"

  SERVER_IP=$(curl -4 -sf https://ifconfig.me 2>/dev/null || true)
  RESOLVED_IP=$(getent hosts "$FULL_DOMAIN" 2>/dev/null | awk '{print $1}' || true)

  if [[ "$SERVER_IP" != "$RESOLVED_IP" ]]; then
    warn "DNS not ready: server is $SERVER_IP but $FULL_DOMAIN resolves to ${RESOLVED_IP:-nothing}"
    warn "Point your DNS A record to $SERVER_IP, wait for propagation, then re-run:"
    warn "  sudo bash install.sh --skip-build"
  else
    certbot certonly \
      --nginx \
      --non-interactive \
      --agree-tos \
      -m "$SSL_EMAIL" \
      -d "$FULL_DOMAIN" || { warn "certbot failed — re-run after DNS propagates: sudo bash install.sh --skip-build"; }

    if [[ -f "/etc/letsencrypt/live/${FULL_DOMAIN}/fullchain.pem" ]]; then

      # Write full HTTPS config now that cert exists
      cat > /etc/nginx/sites-available/bytegrader <<NGINX_FULL
# ByteGrader nginx config — generated by install.sh

server {
    listen 80;
    server_name ${DOMAIN} www.${DOMAIN};
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://github.com/ShawnHymel/bytegrader; }
}

server {
    listen 80;
    server_name ${FULL_DOMAIN};
    location /.well-known/acme-challenge/ { root /var/www/html; }
    location / { return 301 https://\$server_name\$request_uri; }
}

server {
    listen 443 ssl http2;
    server_name ${FULL_DOMAIN};

    ssl_certificate     /etc/letsencrypt/live/${FULL_DOMAIN}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/${FULL_DOMAIN}/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;

    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload";

    location / {
$(printf '%b' "$IP_BLOCK")
        proxy_hide_header Access-Control-Allow-Origin;
        set \$cors_origin "";
        if (\$http_origin ~* "^https?://(localhost|127\\.0\\.0\\.1)(:[0-9]+)?\$") {
            set \$cors_origin \$http_origin;
        }
        add_header Access-Control-Allow-Origin \$cors_origin always;
        add_header Access-Control-Allow-Headers "X-API-Key, X-Username, Content-Type" always;
        add_header Access-Control-Allow-Methods "GET, POST, OPTIONS" always;

        if (\$request_method = OPTIONS) { return 204; }

        proxy_pass         http://localhost:${APP_PORT};
        proxy_set_header   Host              \$host;
        proxy_set_header   X-Real-IP         \$remote_addr;
        proxy_set_header   X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto \$scheme;
        proxy_read_timeout 300s;
        proxy_connect_timeout 10s;
        client_max_body_size 100M;
    }
}
NGINX_FULL
      nginx -t && systemctl reload nginx
      success "HTTPS enabled for https://$FULL_DOMAIN"

      # Set up automatic cert renewal
      (crontab -l 2>/dev/null | grep -v certbot; echo "0 12 * * * /usr/bin/certbot renew --quiet") | crontab -
      success "SSL auto-renewal configured"
    fi
  fi
fi

# ── Done ──────────────────────────────────────────────────────────────────────
header "Installation Complete"

LOCAL=$(curl -sf "http://localhost:${APP_PORT}/health" || echo "FAILED")
echo ""
echo -e "  ${GREEN}Local health:${NC}  $LOCAL"
echo ""

if [[ "$SKIP_NGINX" == true ]]; then
  SERVER_IP=$(curl -4 -sf https://ifconfig.me 2>/dev/null || echo "<SERVER_IP>")
  echo -e "  ${BOLD}Test endpoint:${NC}  http://${SERVER_IP}:${APP_PORT}/health"
  echo ""
  echo -e "  ${YELLOW}Next steps:${NC}"
  echo -e "  1. Test the server directly via the URL above"
  echo -e "  2. Point DNS A record to: ${SERVER_IP}"
  echo -e "  3. Wait for DNS propagation, then run:"
  echo -e "     sudo bash install.sh --skip-build"
else
  if [[ -f "/etc/letsencrypt/live/${FULL_DOMAIN}/fullchain.pem" ]]; then
    REMOTE=$(curl -sf "https://${FULL_DOMAIN}/health" 2>/dev/null || echo "not yet reachable")
    echo -e "  ${GREEN}Remote health:${NC} $REMOTE"
    echo ""
    echo -e "  ${BOLD}Endpoint:${NC}  https://${FULL_DOMAIN}"
  else
    echo -e "  ${YELLOW}SSL not yet configured.${NC} Re-run after DNS propagates:"
    echo -e "  sudo bash install.sh --skip-build"
    echo ""
    echo -e "  ${BOLD}Endpoint (HTTP only):${NC}  http://${FULL_DOMAIN}"
  fi
fi

echo ""
echo -e "  ${BOLD}Logs:${NC}  cd $APP_DIR && docker compose logs -f"
echo ""
success "ByteGrader is live!"
