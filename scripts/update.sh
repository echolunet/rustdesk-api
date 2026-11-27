#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

REBUILD_API=0
PULL_API=0
GIT_PULL=0

for arg in "$@"; do
  case "$arg" in
    --rebuild-api) REBUILD_API=1 ;;
    --pull-api) PULL_API=1 ;;
    --git-pull) GIT_PULL=1 ;;
    --all) GIT_PULL=1; PULL_API=1 ;;
    *)
      echo "Unknown option: $arg"
      echo "Usage: $0 [--pull-api] [--rebuild-api] [--git-pull] [--all]"
      exit 1
      ;;
  esac
done

COMPOSE_FILES=(-f docker-compose.yml)
CURRENT_API_IMAGE="$(docker inspect --format '{{.Config.Image}}' rustdesk-api 2>/dev/null || true)"
if [ "$REBUILD_API" -eq 1 ]; then
  if [ ! -f "$ROOT_DIR/Dockerfile" ] || [ ! -f "$ROOT_DIR/docker-compose.build.yml" ]; then
    echo "Error: --rebuild-api requires Dockerfile and docker-compose.build.yml in $ROOT_DIR" >&2
    exit 1
  fi
  COMPOSE_FILES=(-f docker-compose.yml -f docker-compose.build.yml)
elif [[ "$CURRENT_API_IMAGE" == *"rustdesk-api-custom"* ]] && [ -f "$ROOT_DIR/docker-compose.build.yml" ]; then
  COMPOSE_FILES=(-f docker-compose.yml -f docker-compose.build.yml)
fi

if [ "$GIT_PULL" -eq 1 ]; then
  if [ -d .git ]; then
    echo "[0/4] Pulling latest repository changes..."
    git pull --ff-only
  else
    echo "[0/4] Not a git repository, skipping git pull."
  fi
fi

echo "[1/4] Pulling latest official RustDesk server image (hbbs & hbbr)..."
docker compose "${COMPOSE_FILES[@]}" pull hbbs hbbr

if [ "$REBUILD_API" -eq 1 ]; then
  echo "[2/4] Rebuilding rustdesk-api image from local source..."
  docker compose "${COMPOSE_FILES[@]}" build --pull rustdesk-api
elif [ "$PULL_API" -eq 1 ]; then
  echo "[2/4] Pulling latest prebuilt rustdesk-api image..."
  docker compose "${COMPOSE_FILES[@]}" pull rustdesk-api
else
  echo "[2/4] Skipping rustdesk-api update (use --pull-api or --rebuild-api to update)."
fi

echo "[3/4] Restarting containers..."
docker compose "${COMPOSE_FILES[@]}" up -d --remove-orphans

echo "[4/4] Checking service health..."
sleep 2
docker compose "${COMPOSE_FILES[@]}" ps
if curl -fsS "http://127.0.0.1:${API_PORT:-21114}/health" >/dev/null 2>&1; then
  echo "Health check OK: http://127.0.0.1:${API_PORT:-21114}/health"
else
  echo "Notice: health endpoint not yet responding on 127.0.0.1:${API_PORT:-21114}; check 'docker compose logs -f'."
fi
