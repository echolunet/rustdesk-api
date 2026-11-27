#!/usr/bin/env bash
set -euo pipefail

REPO_SLUG="${RUSTDESK_REPO:-echolunet/rustdesk-api}"
BRANCH="${RUSTDESK_BRANCH:-main}"
RAW_BASE="https://raw.githubusercontent.com/${REPO_SLUG}/${BRANCH}"
GIT_URL="https://github.com/${REPO_SLUG}.git"

echo "=================================================="
echo "  RustDesk API + Official Server One-Click Setup  "
echo "=================================================="

if ! command -v docker >/dev/null 2>&1; then
  echo "Error: docker is not installed. Install Docker first: https://get.docker.com" >&2
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  echo "Error: docker compose v2 plugin is required." >&2
  exit 1
fi

if [ -f "./docker-compose.yml" ] && [ -f "./.env.example" ]; then
  INSTALL_DIR="$(pwd)"
else
  INSTALL_DIR="${RUSTDESK_DIR:-/opt/rustdesk-api}"
  mkdir -p "$INSTALL_DIR"
  cd "$INSTALL_DIR"
fi
echo "[*] Deploy directory: $INSTALL_DIR"

# Download required compose/script files when run outside a cloned repository
download_if_missing() {
  local rel="$1"
  local target="$INSTALL_DIR/$rel"
  if [ ! -f "$target" ]; then
    mkdir -p "$(dirname "$target")"
    echo "[*] Downloading $rel ..."
    curl -fsSL "$RAW_BASE/$rel" -o "$target"
  fi
}

download_if_missing "docker-compose.yml"
download_if_missing "docker-compose.build.yml"
download_if_missing ".env.example"
download_if_missing "scripts/update.sh"
download_if_missing "scripts/sync-webclient.sh"
chmod +x "$INSTALL_DIR/scripts/update.sh" "$INSTALL_DIR/scripts/sync-webclient.sh" 2>/dev/null || true

# Detect public IPv4 as default server address
DETECTED_IP="$(curl -fsS --max-time 4 https://api.ipify.org 2>/dev/null || curl -fsS --max-time 4 https://ifconfig.me 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}' || echo "127.0.0.1")"
DETECTED_IP="${DETECTED_IP:-127.0.0.1}"

PROMPT_IN=""
if [ "${NON_INTERACTIVE:-0}" != "1" ] && { [ -t 0 ] || [ -r /dev/tty ]; }; then
  PROMPT_IN="/dev/tty"
fi

ask() {
  local prompt="$1"
  local def="$2"
  local reply=""
  if [ -n "$PROMPT_IN" ]; then
    printf "%s [%s]: " "$prompt" "$def" > /dev/tty
    IFS= read -r reply < "$PROMPT_IN" || true
  fi
  reply="$(printf "%s" "$reply" | tr -d '\r' | xargs || true)"
  if [ -z "$reply" ]; then
    printf "%s" "$def"
  else
    printf "%s" "$reply"
  fi
}

SHOULD_WRITE_ENV="1"
if [ -f "$INSTALL_DIR/.env" ]; then
  if [ "${FORCE_OVERWRITE_ENV:-0}" = "1" ]; then
    SHOULD_WRITE_ENV="1"
  elif [ -n "$PROMPT_IN" ]; then
    OVERWRITE_ANS="$(ask "Detected existing .env, overwrite config (N keeps current .env)" "N")"
    case "$OVERWRITE_ANS" in
      y|Y|yes|YES) SHOULD_WRITE_ENV="1" ;;
      *) SHOULD_WRITE_ENV="0" ;;
    esac
  else
    SHOULD_WRITE_ENV="0"
  fi
fi

clamp_int() {
  local val="$1"
  local min="$2"
  local max="$3"
  local fb="$4"
  if ! printf "%s" "$val" | grep -Eq '^[0-9]+$'; then
    printf "%s" "$fb"
    return
  fi
  if [ "$val" -lt "$min" ]; then
    printf "%s" "$min"
  elif [ "$val" -gt "$max" ]; then
    printf "%s" "$max"
  else
    printf "%s" "$val"
  fi
}

set_env_kv() {
  local file="$1"
  local key="$2"
  local val="$3"
  local tmp="${file}.tmp"
  if grep -Eq "^${key}=" "$file"; then
    awk -v k="$key" -v v="$val" 'BEGIN{FS=OFS="="} $1==k {$0=k "=" v} {print}' "$file" > "$tmp"
    mv "$tmp" "$file"
  else
    printf "%s=%s\n" "$key" "$val" >> "$file"
  fi
}

if [ "$SHOULD_WRITE_ENV" = "1" ]; then
  if [ -f "$INSTALL_DIR/.env" ]; then
    cp "$INSTALL_DIR/.env" "$INSTALL_DIR/.env.bak.$(date +%Y%m%d%H%M%S)"
  fi
  cp "$INSTALL_DIR/.env.example" "$INSTALL_DIR/.env"

  SERVER_HOST="${SERVER_HOST:-}"
  if [ -z "$SERVER_HOST" ]; then
    SERVER_HOST="$(ask "Server public IP or domain (for RustDesk ID/Relay/API)" "$DETECTED_IP")"
  fi
  SERVER_HOST="${SERVER_HOST#http://}"
  SERVER_HOST="${SERVER_HOST#https://}"
  SERVER_HOST="${SERVER_HOST%%/*}"

  API_PORT="${API_PORT:-}"
  if [ -z "$API_PORT" ]; then
    API_PORT="$(ask "API & Web Admin listen port" "21114")"
  fi
  API_PORT="$(clamp_int "$API_PORT" 1 65535 21114)"

  FPS_VAL="${DEFAULT_FPS:-}"
  QUALITY_VAL="${DEFAULT_QUALITY:-}"
  if [ -z "$FPS_VAL" ]; then
    if [ -n "$PROMPT_IN" ]; then
      printf "\nSelect default remote desktop frame rate preset:\n" > /dev/tty
      printf "  1) 60 FPS Smooth   (recommended, custom quality 75%%)\n" > /dev/tty
      printf "  2) 120 FPS High    (high refresh gaming/design, quality 85%%)\n" > /dev/tty
      printf "  3) 30 FPS Balanced (bandwidth saver, quality 60%%)\n" > /dev/tty
      printf "  4) Custom FPS      (5-120)\n" > /dev/tty
      FPS_CHOICE="$(ask "Choose [1-4]" "1")"
      case "$FPS_CHOICE" in
        2) FPS_VAL="120"; QUALITY_VAL="85" ;;
        3) FPS_VAL="30"; QUALITY_VAL="60" ;;
        4)
          FPS_VAL="$(ask "Enter custom FPS (5-120)" "60")"
          QUALITY_VAL="$(ask "Enter custom image quality % (10-100)" "75")"
          ;;
        *) FPS_VAL="60"; QUALITY_VAL="75" ;;
      esac
    else
      FPS_VAL="60"
      QUALITY_VAL="75"
    fi
  fi
  FPS_VAL="$(clamp_int "$FPS_VAL" 5 120 60)"
  QUALITY_VAL="$(clamp_int "${QUALITY_VAL:-75}" 10 100 75)"

  set_env_kv "$INSTALL_DIR/.env" "API_PORT" "$API_PORT"
  set_env_kv "$INSTALL_DIR/.env" "RUSTDESK_ID_SERVER" "${SERVER_HOST}:21116"
  set_env_kv "$INSTALL_DIR/.env" "RUSTDESK_RELAY_SERVER" "${SERVER_HOST}:21117"
  set_env_kv "$INSTALL_DIR/.env" "RUSTDESK_API_SERVER" "http://${SERVER_HOST}:${API_PORT}"
  set_env_kv "$INSTALL_DIR/.env" "CLIENT_STRATEGY_ENABLED" "true"
  set_env_kv "$INSTALL_DIR/.env" "CLIENT_DEFAULT_IMAGE_QUALITY" "custom"
  set_env_kv "$INSTALL_DIR/.env" "CLIENT_DEFAULT_FPS" "$FPS_VAL"
  set_env_kv "$INSTALL_DIR/.env" "CLIENT_DEFAULT_QUALITY" "$QUALITY_VAL"
  if [ -n "${API_IMAGE:-}" ]; then
    set_env_kv "$INSTALL_DIR/.env" "API_IMAGE" "$API_IMAGE"
  fi
  echo "[*] Wrote configuration to $INSTALL_DIR/.env (Default FPS: ${FPS_VAL})"
else
  # Read display defaults from existing .env
  API_PORT="$(grep -E '^API_PORT=' "$INSTALL_DIR/.env" | head -n1 | cut -d= -f2- || echo "21114")"
  API_PORT="$(clamp_int "${API_PORT:-21114}" 1 65535 21114)"
  API_URL_LINE="$(grep -E '^RUSTDESK_API_SERVER=' "$INSTALL_DIR/.env" | head -n1 | cut -d= -f2- || echo "")"
  FPS_VAL="$(grep -E '^CLIENT_DEFAULT_FPS=' "$INSTALL_DIR/.env" | head -n1 | cut -d= -f2- || echo "60")"
fi

echo "[*] Pulling official RustDesk server images (hbbs & hbbr)..."
docker compose pull hbbs hbbr

USE_LOCAL_BUILD="0"
echo "[*] Pulling prebuilt rustdesk-api image..."
if ! docker compose pull rustdesk-api; then
  echo "[!] Prebuilt GHCR image pull failed (package may not be built yet or still private)."
  if [ -f "$INSTALL_DIR/Dockerfile" ] && [ -f "$INSTALL_DIR/go.mod" ]; then
    USE_LOCAL_BUILD="1"
  elif command -v git >/dev/null 2>&1; then
    echo "[*] Cloning source to $INSTALL_DIR/src for automatic fallback build..."
    rm -rf "$INSTALL_DIR/src"
    git clone --depth 1 -b "$BRANCH" "$GIT_URL" "$INSTALL_DIR/src"
    cp -n "$INSTALL_DIR/.env" "$INSTALL_DIR/src/.env" 2>/dev/null || cp "$INSTALL_DIR/.env" "$INSTALL_DIR/src/.env"
    mkdir -p "$INSTALL_DIR/data"
    cd "$INSTALL_DIR/src"
    USE_LOCAL_BUILD="1"
  else
    echo "Error: Could not pull GHCR image and git is unavailable for fallback build." >&2
    echo "Hint: Set GHCR package ghcr.io/${REPO_SLUG} to Public on GitHub, or install git to build locally." >&2
    exit 1
  fi
fi

echo "[*] Starting containers..."
if [ "$USE_LOCAL_BUILD" = "1" ]; then
  docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
else
  docker compose up -d
fi

echo "[*] Waiting for API health and official hbbs public key..."
PUB_KEY=""
for _ in $(seq 1 20); do
  if curl -fsS "http://127.0.0.1:${API_PORT}/health" >/dev/null 2>&1 && [ -f "./data/server/id_ed25519.pub" ]; then
    PUB_KEY="$(tr -d '\r\n' < ./data/server/id_ed25519.pub || true)"
    break
  fi
  sleep 1
done

INIT_PASS="$(docker compose logs rustdesk-api 2>&1 | grep -iE 'Initial Admin|password:' | tail -n 3 || true)"
ENV_API_URL="${API_URL_LINE:-http://${SERVER_HOST:-$DETECTED_IP}:${API_PORT}}"
ENV_ID_SRV="$(grep -E '^RUSTDESK_ID_SERVER=' .env | head -n1 | cut -d= -f2- || echo "${DETECTED_IP}:21116")"
ENV_RELAY_SRV="$(grep -E '^RUSTDESK_RELAY_SERVER=' .env | head -n1 | cut -d= -f2- || echo "${DETECTED_IP}:21117")"

echo ""
echo "=================================================="
echo "  RustDesk Stack Deployed Successfully!"
echo "=================================================="
echo "  Web Admin Console : ${ENV_API_URL}/_admin/"
echo "  Web Client        : ${ENV_API_URL}/webclient/"
echo "  ID Server         : ${ENV_ID_SRV}"
echo "  Relay Server      : ${ENV_RELAY_SRV}"
echo "  API Server        : ${ENV_API_URL}"
echo "  Official Key      : ${PUB_KEY:-Pending in ./data/server/id_ed25519.pub}"
echo "  Default FPS Push  : ${FPS_VAL} FPS (configurable in Admin -> 客户端策略)"
echo "--------------------------------------------------"
echo "  Initial Admin Info (first boot only):"
if [ -n "$INIT_PASS" ]; then
  printf "%s\n" "$INIT_PASS" | sed 's/^/    /'
else
  echo "    Check with: docker compose logs rustdesk-api | grep -i password"
fi
echo "--------------------------------------------------"
echo "  Firewall ports to open:"
echo "    TCP: ${API_PORT}, 21115, 21116, 21117, 21118, 21119"
echo "    UDP: 21116"
echo "  Update command:"
echo "    ./scripts/update.sh --pull-api"
echo "=================================================="
