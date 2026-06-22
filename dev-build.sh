#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FRONTEND_DIR="$ROOT_DIR/frontend"
MODE="${1:-stable}"
WAILS_DEVSERVER_HOST="${WAILS_DEVSERVER_HOST:-127.0.0.1}"
WAILS_DEVSERVER_PORT="${WAILS_DEVSERVER_PORT:-34115}"
FRONTEND_PORT="${FRONTEND_PORT:-5218}"
WAILS_VERSION="${WAILS_VERSION:-v2.12.0}"

usage() {
  cat <<'USAGE'
Usage:
  ./dev-build.sh [stable|live|build|help]

Modes:
  stable  Install deps, build frontend, then start Wails with frontend/dist. Default.
  live    Install deps, start Vite dev server, then start Wails connected to Vite.
  build   Install deps and build frontend only.
  help    Show this help.

Optional env:
  DEV_PROXY_URL           Proxy for npm/go downloads, e.g. http://127.0.0.1:7890
  DEV_NO_PROXY            NO_PROXY value
  DEV_GOPROXY             GOPROXY override, default https://goproxy.cn,direct
  WAILS_VERSION           Wails CLI version, default v2.12.0
  WAILS_DEVSERVER_PORT    Wails internal dev server start port, default 34115
  FRONTEND_PORT           Vite port for live mode, default 5218
USAGE
}

log() { printf '\033[1;36m[dev-build]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[dev-build][WARN]\033[0m %s\n' "$*" >&2; }
err() { printf '\033[1;31m[dev-build][ERROR]\033[0m %s\n' "$*" >&2; }

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    err "Missing required command: $cmd"
    exit 1
  fi
}

setup_proxy_env() {
  if [[ -n "${DEV_PROXY_URL:-}" ]]; then
    export HTTP_PROXY="$DEV_PROXY_URL"
    export HTTPS_PROXY="$DEV_PROXY_URL"
    export http_proxy="$DEV_PROXY_URL"
    export https_proxy="$DEV_PROXY_URL"
  fi

  if [[ -n "${DEV_NO_PROXY:-}" ]]; then
    export NO_PROXY="$DEV_NO_PROXY"
    export no_proxy="$DEV_NO_PROXY"
  fi

  if [[ -n "${DEV_GOPROXY:-}" ]]; then
    export GOPROXY="$DEV_GOPROXY"
  elif [[ -z "${GOPROXY:-}" ]]; then
    export GOPROXY="https://goproxy.cn,direct"
  fi
}

ensure_wails() {
  local gobin
  gobin="$(go env GOPATH)/bin"
  export PATH="$PATH:$gobin"

  if command -v wails >/dev/null 2>&1; then
    log "Wails found: $(command -v wails)"
    return
  fi

  log "Wails CLI not found, installing github.com/wailsapp/wails/v2/cmd/wails@$WAILS_VERSION ..."
  go install "github.com/wailsapp/wails/v2/cmd/wails@$WAILS_VERSION"

  if ! command -v wails >/dev/null 2>&1; then
    err "Wails was installed to $gobin but is still not in PATH"
    exit 1
  fi
}

port_busy() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
    return $?
  fi
  if command -v nc >/dev/null 2>&1; then
    nc -z "$WAILS_DEVSERVER_HOST" "$port" >/dev/null 2>&1
    return $?
  fi
  return 1
}

resolve_wails_devserver() {
  local port="$WAILS_DEVSERVER_PORT"
  while port_busy "$port"; do
    port=$((port + 1))
  done
  export WAILS_DEVSERVER_ADDRESS="$WAILS_DEVSERVER_HOST:$port"
}

install_frontend_deps() {
  cd "$FRONTEND_DIR"
  if [[ -f package-lock.json ]]; then
    log "Installing frontend dependencies with npm ci ..."
    npm ci
  else
    log "Installing frontend dependencies with npm install ..."
    npm install
  fi
}

build_frontend() {
  cd "$FRONTEND_DIR"
  log "Building frontend ..."
  npm run build:clean
}

prepare() {
  require_cmd node
  require_cmd npm
  require_cmd go
  setup_proxy_env
  ensure_wails
  resolve_wails_devserver
  log "Root: $ROOT_DIR"
  log "Wails dev server: http://$WAILS_DEVSERVER_ADDRESS"
}

run_stable() {
  prepare
  install_frontend_deps
  build_frontend
  cd "$ROOT_DIR"
  log "Starting Wails in stable mode ..."
  exec wails dev -m -noreload -s -skipbindings -assetdir frontend/dist -devserver "$WAILS_DEVSERVER_ADDRESS"
}

run_live() {
  local frontend_pid=""
  trap 'if [[ -n "$frontend_pid" ]] && kill -0 "$frontend_pid" >/dev/null 2>&1; then kill "$frontend_pid" >/dev/null 2>&1 || true; fi' EXIT

  prepare
  install_frontend_deps

  cd "$FRONTEND_DIR"
  log "Starting Vite dev server at http://127.0.0.1:$FRONTEND_PORT ..."
  npm run dev:raw -- --host 127.0.0.1 --port "$FRONTEND_PORT" &
  frontend_pid="$!"

  cd "$ROOT_DIR"
  log "Starting Wails in live mode ..."
  exec wails dev -m -s -skipbindings -frontenddevserverurl "http://127.0.0.1:$FRONTEND_PORT" -viteservertimeout 60 -devserver "$WAILS_DEVSERVER_ADDRESS"
}

run_build_only() {
  prepare
  install_frontend_deps
  build_frontend
  log "Build complete: $FRONTEND_DIR/dist"
}

case "$MODE" in
  stable) run_stable ;;
  live) run_live ;;
  build) run_build_only ;;
  help|-h|--help) usage ;;
  *)
    err "Unsupported mode: $MODE"
    usage >&2
    exit 1
    ;;
esac
