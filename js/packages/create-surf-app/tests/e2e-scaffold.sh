#!/usr/bin/env bash
#
# End-to-end scaffold tests for create-surf-app.
# Tests both Vite and Next.js templates: scaffold, install, dev, build, start.
#
# Prerequisites:
#   - bun installed
#   - SDK built locally (js/packages/sdk)
#
# Usage:
#   bash tests/e2e-scaffold.sh [--sdk-path /path/to/sdk]

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

PASS=0
FAIL=0
SDK_PATH=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --sdk-path) SDK_PATH="$2"; shift 2 ;;
    *) echo "Unknown arg: $1"; exit 1 ;;
  esac
done

if [[ -z "$SDK_PATH" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
  SDK_PATH="$(cd "$SCRIPT_DIR/../../sdk" && pwd)"
fi

CLI_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TMPDIR_BASE="$(mktemp -d)"

trap 'cleanup' EXIT

cleanup() {
  for port in 3000 3001 4000 4001 4002 5173; do
    lsof -ti ":$port" 2>/dev/null | xargs kill 2>/dev/null || true
  done
  sleep 1
  rm -rf "$TMPDIR_BASE"
}

pass() {
  echo -e "  ${GREEN}✓${NC} $1"
  PASS=$((PASS + 1))
}

fail() {
  echo -e "  ${RED}✗${NC} $1"
  FAIL=$((FAIL + 1))
}

wait_for_port() {
  local port=$1
  local timeout=${2:-15}
  local i=0
  while ! curl -s "http://localhost:$port" >/dev/null 2>&1; do
    sleep 1
    i=$((i + 1))
    if [[ $i -ge $timeout ]]; then
      return 1
    fi
  done
}

kill_port() {
  lsof -ti ":$1" 2>/dev/null | xargs kill 2>/dev/null || true
  sleep 1
}

# Helper: create .env from .env.example with overrides
# Usage: make_env <env_example_path> <key=value> ...
make_env() {
  local src="$1"; shift
  local dst="${src%.example}"
  cp "$src" "$dst"
  for kv in "$@"; do
    local key="${kv%%=*}"
    local val="${kv#*=}"
    sed -i '' "s|^${key}=.*|${key}=${val}|" "$dst"
  done
}

# ── Build CLI ──────────────────────────────────────────────────────────────

echo ""
echo -e "${YELLOW}Building create-surf-app CLI...${NC}"
cd "$CLI_DIR"
bun run build >/dev/null 2>&1
echo -e "${GREEN}CLI built.${NC}"
echo ""

# ══════════════════════════════════════════════════════════════════════════
# NEXT.JS TEMPLATE
# ══════════════════════════════════════════════════════════════════════════

NEXTJS_DIR="$TMPDIR_BASE/nextjs-app"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo " Next.js Template"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

node "$CLI_DIR/dist/cli.js" "$NEXTJS_DIR" --template nextjs >/dev/null 2>&1

# Check .env.example exists (not .env)
if [[ ! -f "$NEXTJS_DIR/.env" ]] && \
   grep -q "FRONTEND_PORT=" "$NEXTJS_DIR/.env.example" && \
   grep -q "BASE_PATH=" "$NEXTJS_DIR/.env.example" && \
   grep -q "SURF_API_KEY=" "$NEXTJS_DIR/.env.example"; then
  pass "scaffold: .env.example has all required vars, no .env"
else
  fail "scaffold: .env.example check failed"
fi

# Check key files exist
MISSING=""
for f in CLAUDE.md instrumentation.ts next.config.ts app/layout.tsx app/page.tsx app/providers.tsx \
         app/api/health/route.ts app/api/cron/route.ts \
         app/api/market/price/route.ts db/schema.ts db/index.ts lib/boot.ts \
         components/ui/button.tsx scripts/check-env.js .env.example; do
  [[ -f "$NEXTJS_DIR/$f" ]] || MISSING="$MISSING $f"
done
if [[ -z "$MISSING" ]]; then
  pass "scaffold: all expected files exist"
else
  fail "scaffold: missing files:$MISSING"
fi

# No Vite artifacts
if [[ ! -d "$NEXTJS_DIR/frontend" ]] && [[ ! -d "$NEXTJS_DIR/backend" ]]; then
  pass "scaffold: no Vite artifacts (frontend/, backend/)"
else
  fail "scaffold: Vite artifacts found"
fi

# Install
cd "$NEXTJS_DIR"
npm link "$SDK_PATH" >/dev/null 2>&1
bun install >/dev/null 2>&1

# Dev — no .env at all should block
DEV_OUT="$TMPDIR_BASE/dev-out.txt"
bun run dev >"$DEV_OUT" 2>&1 || true
if grep -q "Missing required env vars" "$DEV_OUT"; then
  pass "dev: blocked without .env"
else
  fail "dev: did NOT block without .env"
  cat "$DEV_OUT"
  kill_port 3000
fi

# Create .env without SURF_API_KEY — build should work, dev should block
make_env "$NEXTJS_DIR/.env.example" "FRONTEND_PORT=4000" "BASE_PATH=/preview/test"
bun run dev >"$DEV_OUT" 2>&1 || true
if grep -q "Missing required env vars" "$DEV_OUT"; then
  pass "dev: blocked without SURF_API_KEY"
else
  fail "dev: did NOT block without SURF_API_KEY"
  cat "$DEV_OUT"
  kill_port 4000
fi

# Build without SURF_API_KEY — should succeed (not needed at build time)
if bun run build >/dev/null 2>&1; then
  pass "build: succeeds without SURF_API_KEY"
else
  fail "build: failed without SURF_API_KEY"
fi

# Set API key
make_env "$NEXTJS_DIR/.env.example" "FRONTEND_PORT=4000" "BASE_PATH=/preview/test" "SURF_API_KEY=testkey"

# Build — should succeed
if bun run build >/dev/null 2>&1; then
  pass "build: succeeds with all env vars"
else
  fail "build: failed with all env vars"
fi

# Start (production) — check health with basePath
bun run start >/dev/null 2>&1 &
if wait_for_port 4000; then
  HEALTH=$(curl -s "http://localhost:4000/preview/test/api/health")
  if [[ "$HEALTH" == '{"status":"ok"}' ]]; then
    pass "start: /preview/test/api/health returns ok"
  else
    fail "start: health returned '$HEALTH'"
  fi

  HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:4000/api/health")
  if [[ "$HTTP_CODE" == "404" ]]; then
    pass "start: /api/health (no basePath) returns 404"
  else
    fail "start: /api/health returned $HTTP_CODE (expected 404)"
  fi
else
  fail "start: server did not start on port 4000"
fi
kill_port 4000

# Dev — check health with basePath
bun run dev >/dev/null 2>&1 &
if wait_for_port 4000; then
  HEALTH=$(curl -s "http://localhost:4000/preview/test/api/health")
  if [[ "$HEALTH" == '{"status":"ok"}' ]]; then
    pass "dev: /preview/test/api/health returns ok"
  else
    fail "dev: health returned '$HEALTH'"
  fi
else
  fail "dev: server did not start on port 4000"
fi
kill_port 4000

echo ""

# ══════════════════════════════════════════════════════════════════════════
# VITE TEMPLATE
# ══════════════════════════════════════════════════════════════════════════

VITE_DIR="$TMPDIR_BASE/vite-app"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo " Vite Template"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

node "$CLI_DIR/dist/cli.js" "$VITE_DIR" >/dev/null 2>&1

# Check .env.example files (not .env)
if [[ ! -f "$VITE_DIR/backend/.env" ]] && \
   grep -q "BACKEND_PORT=" "$VITE_DIR/backend/.env.example" && \
   grep -q "SURF_API_KEY=" "$VITE_DIR/backend/.env.example"; then
  pass "scaffold: backend/.env.example has BACKEND_PORT, SURF_API_KEY"
else
  fail "scaffold: backend env check failed"
fi

if [[ ! -f "$VITE_DIR/frontend/.env" ]] && \
   grep -q "FRONTEND_PORT=" "$VITE_DIR/frontend/.env.example" && \
   grep -q "BACKEND_PORT=" "$VITE_DIR/frontend/.env.example" && \
   grep -q "BASE_PATH=" "$VITE_DIR/frontend/.env.example"; then
  pass "scaffold: frontend/.env.example has FRONTEND_PORT, BACKEND_PORT, BASE_PATH"
else
  fail "scaffold: frontend env check failed"
fi

# Check key files
MISSING=""
for f in CLAUDE.md package.json backend/server.js backend/db/schema.js \
         backend/.env.example backend/scripts/check-env.js \
         frontend/vite.config.ts frontend/.env.example frontend/scripts/check-env.cjs \
         frontend/src/App.tsx frontend/src/lib/api.ts frontend/src/components/ui/button.tsx; do
  [[ -f "$VITE_DIR/$f" ]] || MISSING="$MISSING $f"
done
if [[ -z "$MISSING" ]]; then
  pass "scaffold: all expected files exist"
else
  fail "scaffold: missing files:$MISSING"
fi

# No Next.js artifacts
if [[ ! -f "$VITE_DIR/next.config.ts" ]] && [[ ! -f "$VITE_DIR/instrumentation.ts" ]]; then
  pass "scaffold: no Next.js artifacts"
else
  fail "scaffold: Next.js artifacts found"
fi

# Install at workspace root (single npm install)
cd "$VITE_DIR"
npm link "$SDK_PATH" --workspace backend >/dev/null 2>&1 || npm link "$SDK_PATH" >/dev/null 2>&1
bun install >/dev/null 2>&1

# Create .env files from examples
make_env "$VITE_DIR/backend/.env.example" "SURF_API_KEY=testkey"
make_env "$VITE_DIR/frontend/.env.example" "BASE_PATH=/preview/vtest/"

# Backend dev
cd "$VITE_DIR/backend"
bun run dev >/dev/null 2>&1 &
if wait_for_port 3001; then
  HEALTH=$(curl -s "http://localhost:3001/api/health")
  if [[ "$HEALTH" == '{"status":"ok"}' ]]; then
    pass "backend dev: /api/health returns ok"
  else
    fail "backend dev: health returned '$HEALTH'"
  fi
else
  fail "backend dev: server did not start on port 3001"
fi

# Frontend dev — proxy to backend with BASE_PATH
cd "$VITE_DIR/frontend"
bun run dev >/dev/null 2>&1 &
if wait_for_port 5173; then
  sleep 2

  HEALTH=$(curl -s "http://localhost:5173/preview/vtest/api/health")
  if [[ "$HEALTH" == '{"status":"ok"}' ]]; then
    pass "frontend dev: proxy /preview/vtest/api/health returns ok"
  else
    fail "frontend dev: proxy returned '$HEALTH'"
  fi

  HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:5173/preview/vtest/")
  if [[ "$HTTP_CODE" == "200" ]]; then
    pass "frontend dev: page at /preview/vtest/ returns 200"
  else
    fail "frontend dev: page returned $HTTP_CODE (expected 200)"
  fi

  HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:5173/api/health")
  if [[ "$HTTP_CODE" != "200" ]]; then
    pass "frontend dev: /api/health (no base) does not proxy"
  else
    fail "frontend dev: /api/health should not proxy without base"
  fi
else
  fail "frontend dev: server did not start on port 5173"
fi

kill_port 5173
kill_port 3001

echo ""

# ══════════════════════════════════════════════════════════════════════════
# TEMPLATE ISOLATION
# ══════════════════════════════════════════════════════════════════════════

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo " Template Isolation"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

TMPL_OUT="$TMPDIR_BASE/template-out.txt"
node "$CLI_DIR/dist/cli.js" "$TMPDIR_BASE/bad-template" --template nope >"$TMPL_OUT" 2>&1 || true
if grep -q "Unknown template" "$TMPL_OUT"; then
  pass "unknown template rejected"
else
  fail "unknown template was NOT rejected"
  cat "$TMPL_OUT"
fi

DEFAULT_DIR="$TMPDIR_BASE/default-app"
node "$CLI_DIR/dist/cli.js" "$DEFAULT_DIR" >/dev/null 2>&1
if [[ -f "$DEFAULT_DIR/backend/server.js" ]] && [[ -f "$DEFAULT_DIR/frontend/vite.config.ts" ]]; then
  pass "default template is Vite"
else
  fail "default template is NOT Vite"
fi

echo ""

# ══════════════════════════════════════════════════════════════════════════
# SUMMARY
# ══════════════════════════════════════════════════════════════════════════

TOTAL=$((PASS + FAIL))
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if [[ $FAIL -eq 0 ]]; then
  echo -e " ${GREEN}All $TOTAL tests passed${NC}"
else
  echo -e " ${RED}$FAIL/$TOTAL tests failed${NC}"
fi
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

exit $FAIL
