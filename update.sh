#!/usr/bin/env bash
# =============================================================================
# ByteGrader Update Script
# =============================================================================
# Zero-downtime Blue/Green container swap.
# Auto-falls back to Stop-then-Start on low-memory hosts (e.g. Raspberry Pi).
#
# Usage:
#   sudo bash update.sh [--force-stop-start] [--dry-run]
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
FORCE_STOP_START=false
DRY_RUN=false
for arg in "$@"; do
  case $arg in
    --force-stop-start) FORCE_STOP_START=true ;;
    --dry-run)          DRY_RUN=true ;;
    *) die "Unknown argument: $arg" ;;
  esac
done

[[ $EUID -ne 0 ]] && die "Run as root: sudo bash update.sh"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG="$SCRIPT_DIR/config.yaml"
[[ -f "$CONFIG" ]] || die "config.yaml not found. Run from the ByteGrader repo root."

# ── YAML parser ───────────────────────────────────────────────────────────────
cfg() {
  python3 - "$1" <<'PYEOF'
import sys, yaml
key = sys.argv[1]
with open("config.yaml") as f:
    d = yaml.safe_load(f)
val = d.get(key, "")
print("" if val is None else str(val))
PYEOF
}

cd "$SCRIPT_DIR"

APP_DIR=$(cfg "app_dir");   APP_DIR="${APP_DIR:-/home/bytegrader/app}"
APP_PORT=$(cfg "app_port"); APP_PORT="${APP_PORT:-8080}"
FULL_DOMAIN="$(cfg subdomain).$(cfg domain)"
NGINX_CONF="/etc/nginx/sites-available/bytegrader"

# ── Slot helpers ──────────────────────────────────────────────────────────────
SLOT_FILE="$APP_DIR/.active_slot"
BLUE_PORT=8080
GREEN_PORT=8081

get_active_slot()   { [[ -f "$SLOT_FILE" ]] && cat "$SLOT_FILE" || echo "blue"; }
get_inactive_slot() { [[ $(get_active_slot) == "blue" ]] && echo "green" || echo "blue"; }
slot_port()         { [[ "$1" == "blue" ]] && echo $BLUE_PORT || echo $GREEN_PORT; }

# ── RAM check ─────────────────────────────────────────────────────────────────
MIN_FREE_MB=512
check_ram() {
  local free_mb
  free_mb=$(awk '/MemAvailable/ {printf "%d", $2/1024}' /proc/meminfo)
  info "Available RAM: ${free_mb}MB (need ${MIN_FREE_MB}MB for Blue/Green)"
  [[ $free_mb -ge $MIN_FREE_MB ]]
}

# ── Health check ──────────────────────────────────────────────────────────────
health_check() {
  local port="$1" retries="${2:-20}" i
  for i in $(seq 1 "$retries"); do
    curl -sf "http://localhost:${port}/health" &>/dev/null && return 0
    sleep 3
  done
  return 1
}

# ── nginx swap ────────────────────────────────────────────────────────────────
swap_nginx() {
  local new_port="$1"
  sed -i "s|proxy_pass http://localhost:[0-9]*;|proxy_pass http://localhost:${new_port};|g" "$NGINX_CONF"
  sed -i "s|proxy_pass http://127.0.0.1:[0-9]*;|proxy_pass http://127.0.0.1:${new_port};|g" "$NGINX_CONF"
  nginx -t && systemctl reload nginx
}

# ── Pull ──────────────────────────────────────────────────────────────────────
header "ByteGrader Update"

info "Pulling latest from git..."
[[ "$DRY_RUN" == false ]] && \
  git -C "$SCRIPT_DIR" pull --ff-only 2>/dev/null || warn "git pull failed — using local version"

info "Building updated Docker image..."
if [[ "$DRY_RUN" == false ]]; then
  APP_PORT=$BLUE_PORT CONTAINER_NAME="bytegrader-blue" \
    docker compose --project-directory "$APP_DIR" --project-name "bytegrader-${BLUE_PORT}" \
    build --pull 2>/dev/null || true
fi

# ── Strategy ──────────────────────────────────────────────────────────────────
header "Selecting Update Strategy"

USE_BLUEGREEN=false
if [[ "$FORCE_STOP_START" == true ]]; then
  warn "--force-stop-start: using Stop-then-Start"
elif check_ram; then
  USE_BLUEGREEN=true
  info "Using Blue/Green zero-downtime swap"
else
  warn "Low RAM — falling back to Stop-then-Start"
fi

# ── Blue/Green ────────────────────────────────────────────────────────────────
do_bluegreen() {
  local active inactive active_port inactive_port
  active=$(get_active_slot)
  inactive=$(get_inactive_slot)
  active_port=$(slot_port "$active")
  inactive_port=$(slot_port "$inactive")

  info "Active:  $active (port $active_port, container: bytegrader-$active)"
  info "Staging: $inactive (port $inactive_port, container: bytegrader-$inactive)"

  if [[ "$DRY_RUN" == true ]]; then
    info "[DRY RUN] Would start bytegrader-$inactive on port $inactive_port"
    info "[DRY RUN] Would health check, swap nginx, stop bytegrader-$active"
    return 0
  fi

  # Clean up any leftover container on the inactive port
  docker rm -f "bytegrader-$inactive" 2>/dev/null || true

  info "Starting bytegrader-$inactive on port $inactive_port..."
  APP_PORT=$inactive_port CONTAINER_NAME="bytegrader-$inactive" \
    docker compose \
      --project-directory "$APP_DIR" \
      --project-name "bytegrader-$inactive_port" \
      up -d --build \
    || die "Failed to start container on port $inactive_port"

  info "Health checking port $inactive_port..."
  if ! health_check "$inactive_port" 20; then
    warn "Health check failed — rolling back"
    APP_PORT=$inactive_port CONTAINER_NAME="bytegrader-$inactive" \
      docker compose \
        --project-directory "$APP_DIR" \
        --project-name "bytegrader-$inactive_port" \
        down 2>/dev/null || true
    die "Update aborted. Check: cd $APP_DIR && docker compose logs"
  fi
  success "New container healthy on port $inactive_port"

  info "Swapping nginx to port $inactive_port..."
  swap_nginx "$inactive_port"
  success "Nginx routing to $inactive (port $inactive_port)"

  echo "$inactive" > "$SLOT_FILE"
  sleep 3

  info "Stopping old container (bytegrader-$active, port $active_port)..."
  APP_PORT=$active_port CONTAINER_NAME="bytegrader-$active" \
    docker compose \
      --project-directory "$APP_DIR" \
      --project-name "bytegrader-$active_port" \
      down 2>/dev/null || true
  success "Old container stopped"
}

# ── Stop-then-Start ───────────────────────────────────────────────────────────
do_stop_start() {
  if [[ "$DRY_RUN" == true ]]; then
    info "[DRY RUN] Would stop then restart container"
    return 0
  fi

  local active active_port
  active=$(get_active_slot)
  active_port=$(slot_port "$active")

  info "Stopping current container..."
  APP_PORT=$active_port CONTAINER_NAME="bytegrader-$active" \
    docker compose --project-directory "$APP_DIR" \
    --project-name "bytegrader-$active_port" down 2>/dev/null || true

  info "Starting updated container..."
  APP_PORT=$active_port CONTAINER_NAME="bytegrader-$active" \
    docker compose --project-directory "$APP_DIR" \
    --project-name "bytegrader-$active_port" up -d \
    || die "Failed to start container"

  health_check "$active_port" 20 || die "Container unhealthy. Check: cd $APP_DIR && docker compose logs"
  success "Container is healthy"
}

# ── Run ───────────────────────────────────────────────────────────────────────
header "Running Update"
[[ "$USE_BLUEGREEN" == true ]] && do_bluegreen || do_stop_start

# ── Verify ────────────────────────────────────────────────────────────────────
header "Verification"

ACTIVE_PORT=$(slot_port "$(get_active_slot)")
LOCAL=$(curl -sf "http://localhost:${ACTIVE_PORT}/health" 2>/dev/null || echo "FAILED")
echo -e "  ${GREEN}Local health:${NC}  $LOCAL"

if [[ "$DRY_RUN" == false ]]; then
  REMOTE=$(curl -sf "https://${FULL_DOMAIN}/health" 2>/dev/null || echo "check DNS/SSL")
  echo -e "  ${GREEN}Remote health:${NC} $REMOTE"
fi

echo ""
GIT_COMMIT=$(git -C "$SCRIPT_DIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")
success "Update complete! Running commit: $GIT_COMMIT"
[[ "$DRY_RUN" == true ]] && { echo ""; warn "Dry run — no changes made."; }
