#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET_DIR="${ROOT_DIR}/data/webclient"
mkdir -p "$TARGET_DIR"

usage() {
  cat <<EOF
Usage:
  $0 --build-official [git-ref]   Build official RustDesk Flutter Web Client via Docker (default ref: master)
  $0 --from-url <archive-url>     Download & extract a prebuilt web client .tar.gz or .zip into ./data/webclient
EOF
}

if [ $# -lt 1 ]; then
  usage
  exit 1
fi

case "$1" in
  --build-official)
    REF="${2:-master}"
    echo "Building official RustDesk Web Client from rustdesk/rustdesk@${REF}..."
    docker build -f "${ROOT_DIR}/Dockerfile.webclient" --build-arg "RUSTDESK_REF=${REF}" -t rustdesk-webclient-export:tmp "${ROOT_DIR}"
    CID=$(docker create rustdesk-webclient-export:tmp)
    trap 'docker rm -f "$CID" >/dev/null 2>&1 || true' EXIT
    rm -rf "${TARGET_DIR:?}/"*
    docker cp "${CID}:/." "${TARGET_DIR}/"
    echo "Official Web Client synced to ${TARGET_DIR}"
    ;;
  --from-url)
    URL="${2:-}"
    if [ -z "$URL" ]; then
      echo "Missing archive URL" >&2
      exit 1
    fi
    TMP_FILE=$(mktemp)
    trap 'rm -f "$TMP_FILE"' EXIT
    echo "Downloading ${URL}..."
    curl -fsSL "$URL" -o "$TMP_FILE"
    rm -rf "${TARGET_DIR:?}/"*
    case "$URL" in
      *.zip)
        unzip -oq "$TMP_FILE" -d "$TARGET_DIR"
        ;;
      *.tar.gz|*.tgz)
        tar -xzf "$TMP_FILE" -C "$TARGET_DIR"
        ;;
      *)
        echo "Unsupported archive extension (expected .zip, .tar.gz, or .tgz)" >&2
        exit 1
        ;;
    esac
    echo "Web Client extracted to ${TARGET_DIR}"
    ;;
  *)
    usage
    exit 1
    ;;
esac
