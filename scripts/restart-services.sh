#!/usr/bin/env bash
#
# restart-services.sh — start ulang backend Go (8081) dan frontend Angular (4200)
# dengan proteksi terhadap core dump yang bisa memenuhi disk.
#
# Latar belakang: kernel di environment ini memakai core_pattern="core" dan
# `ulimit -c` default "unlimited". Ketika Node.js / Go crash, core dump
# berukuran 2-4 GB ditulis ke $HOME dan akhirnya memenuhi disk ("no space
# left on device") sehingga `go run` tidak bisa membuat work dir.
#
# Mitigasi:
#   1. ulimit -c 0  -> proses tidak menghasilkan core dump.
#   2. NODE_OPTIONS --max-old-space-size -> batas heap Vite/Angular dev server
#      supaya crash karena OOM lebih kecil dan lebih jarang.
#
# Usage: ./scripts/restart-services.sh [--no-frontend] [--no-backend]

set -uo pipefail

BACKEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../backend" && pwd)"
FRONTEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../frontend" && pwd)"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

API_LOG="${API_LOG:-/tmp/tbt-api.log}"
WEB_LOG="${WEB_LOG:-/tmp/tbt-frontend.log}"
API_PORT="${API_PORT:-8081}"
WEB_PORT="${WEB_PORT:-4200}"
SERVE_PATH="${SERVE_PATH:-/absproxy/4200/}"

START_BACKEND=1
START_FRONTEND=1
for arg in "$@"; do
  case "$arg" in
    --no-frontend) START_FRONTEND=0 ;;
    --no-backend)  START_BACKEND=0 ;;
    *) echo "unknown option: $arg" >&2; exit 1 ;;
  esac
done

log() { echo "[restart-services] $*"; }

stop_matching() {
  # kill proses yang argv-nya cocok pola, tanpa menyalin pattern ke command
  # line shell ini sendiri (pkill -f bisa match ke dirinya sendiri).
  local pattern="$1"
  local pids
  pids="$(ps -eo pid,args | grep -F -- "$pattern" | grep -v grep | awk '{print $1}')"
  if [ -n "$pids" ]; then
    log "stopping: $pattern ($pids)"
    # shellcheck disable=SC2086
    kill -9 $pids 2>/dev/null
    sleep 2
  fi
}

start_backend() {
  stop_matching "go run ./cmd/api"
  stop_matching "go-build"
  log "starting backend on :$API_PORT"
  (
    cd "$BACKEND_DIR" || exit 1
    [ -f "$ROOT_DIR/.env" ] && export $(grep -v '^#' "$ROOT_DIR/.env" | xargs)
    ulimit -c 0
    HTTP_ADDR=":$API_PORT" setsid nohup go run ./cmd/api > "$API_LOG" 2>&1 < /dev/null &
  )
}

start_frontend() {
  stop_matching "ng serve"
  log "starting frontend on :$WEB_PORT ($SERVE_PATH)"
  (
    cd "$FRONTEND_DIR" || exit 1
    ulimit -c 0
    NODE_OPTIONS="--max-old-space-size=2048" setsid nohup bunx ng serve \
      --host 0.0.0.0 --port "$WEB_PORT" \
      --serve-path "$SERVE_PATH" --disable-host-check > "$WEB_LOG" 2>&1 < /dev/null &
  )
}

wait_for() {
  local url="$1" name="$2" tries="${3:-45}"
  for _ in $(seq 1 "$tries"); do
    if [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "$url" 2>/dev/null)" = "200" ]; then
      log "$name ready"
      return 0
    fi
    sleep 2
  done
  log "$name FAILED to become ready (see $API_LOG / $WEB_LOG)"
  return 1
}

[ "$START_BACKEND" = "1" ] && start_backend
[ "$START_FRONTEND" = "1" ] && start_frontend

[ "$START_BACKEND" = "1" ] && wait_for "http://localhost:$API_PORT/health/ready" "backend"
[ "$START_FRONTEND" = "1" ] && wait_for "http://localhost:$WEB_PORT$SERVE_PATH" "frontend"

log "disk: $(df -h / | tail -1)"
log "done"
