#!/usr/bin/env bash
# Sentinel: asserts Hawser protocol constants match what homelab-manager hardcodes.
# Run after reference/hawser is updated to catch breaking protocol changes.
set -euo pipefail

HAWSER_SERVER="reference/hawser/internal/server/http.go"
HAWSER_CONFIG="reference/hawser/internal/config/config.go"
CLIENT_SRC="docker/remote.go"

fail() { echo "FAIL: $*" >&2; exit 1; }
ok()   { echo "OK:   $*"; }

# 1. X-Hawser-Token header name used by the server
TOKEN_HEADER="X-Hawser-Token"
grep -q "$TOKEN_HEADER" "$HAWSER_SERVER" \
  || fail "$TOKEN_HEADER not found in $HAWSER_SERVER — auth header name may have changed"
grep -q "$TOKEN_HEADER" "$CLIENT_SRC" \
  || fail "$TOKEN_HEADER not found in $CLIENT_SRC — update hawserTokenHeader constant to match"
ok "token header: $TOKEN_HEADER"

# 2. Standard Mode default port config key present in Hawser config
grep -q '"PORT"' "$HAWSER_CONFIG" \
  || fail '"PORT" config key not found in Hawser config — port env var name may have changed'
ok "standard mode port env var: PORT"

# 3. Health endpoint path
HEALTH_PATH="/_hawser/health"
grep -q "$HEALTH_PATH" "$HAWSER_SERVER" \
  || fail "$HEALTH_PATH not found in $HAWSER_SERVER — health endpoint path may have changed"
ok "health endpoint: $HEALTH_PATH"

# 4. handleProxy is the catch-all route (Standard Mode proxy behaviour)
grep -q 'handleProxy' "$HAWSER_SERVER" \
  || fail "handleProxy not found in $HAWSER_SERVER — Standard Mode proxy handler may have been renamed"
ok "proxy handler: handleProxy"

echo ""
echo "All Hawser protocol checks passed."
