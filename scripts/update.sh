#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

REBUILD_API=false
GIT_PULL=false
for arg in "$@"; do
  case "$arg" in
    --rebuild-api) REBUILD_API=true ;;
    --git-pull) GIT_PULL=true ;;
    --all)
      GIT_PULL=true
      REBUILD_API=true
      ;;
    -h|--help)
      echo "Usage: $0 [--git-pull] [--rebuild-api] [--all]"
      echo "  Pulls the latest official rustdesk/rustdesk-server image for hbbs & hbbr and recreates updated containers."
      echo "  --git-pull     Pulls latest repository commits via 'git pull --ff-only' before updating containers."
      echo "  --rebuild-api  Also rebuilds the custom rustdesk-api image from local source."
      echo "  --all          Shorthand for --git-pull --rebuild-api."
      exit 0
      ;;
  esac
done

if [ "$GIT_PULL" = true ] && [ -d .git ]; then
  echo "[0/4] Pulling latest repository changes from Git..."
  git pull --ff-only
fi

echo "[1/4] Pulling latest official RustDesk server image (hbbs & hbbr)..."
docker compose pull hbbs hbbr

if [ "$REBUILD_API" = true ]; then
  echo "[2/4] Rebuilding custom rustdesk-api image..."
  docker compose build --pull rustdesk-api
else
  echo "[2/4] Keeping existing rustdesk-api build (pass --rebuild-api to rebuild)."
fi

echo "[3/4] Restarting services cleanly..."
docker compose up -d --remove-orphans

echo "[4/4] Verifying health and image versions..."
sleep 2
docker compose ps
if command -v curl >/dev/null 2>&1; then
  API_PORT_VAL="${API_PORT:-21114}"
  if [ -f .env ]; then
    ENV_PORT=$(grep -E '^API_PORT=' .env | tail -n1 | cut -d= -f2- || true)
    if [ -n "${ENV_PORT:-}" ]; then
      API_PORT_VAL="$ENV_PORT"
    fi
  fi
  echo "Health check (http://127.0.0.1:${API_PORT_VAL}/health):"
  curl -fsS "http://127.0.0.1:${API_PORT_VAL}/health" || true
  echo ""
fi
echo "Update completed."
