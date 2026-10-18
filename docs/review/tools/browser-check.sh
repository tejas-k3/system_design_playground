#!/usr/bin/env bash
# Run every browser regression check against a freshly started server. Needs the app built
# (make build) and Chrome (set CHROME_BIN if it is not found). exit 0 = all checks pass.
# usage: browser-check.sh [port]   (default 8091)
set -uo pipefail
PORT=${1:-8091}
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../../.." && pwd)
"$ROOT/backend/bin/server" -addr ":$PORT" -static "$ROOT/frontend/dist" >/dev/null 2>&1 &
SERVER=$!
trap 'kill $SERVER 2>/dev/null' EXIT
for i in $(seq 1 30); do curl -fs "http://localhost:$PORT/api/healthz" >/dev/null && break; sleep 1; done
export URL="http://localhost:$PORT"
fail=0
run() { echo "== $*"; "$@" || { echo "FAILED: $*"; fail=1; }; }
run bash "$HERE/check-layout.sh"
run bash "$HERE/check-drag.sh"
run bash "$HERE/check-fit.sh" 1440 900
run node "$HERE/ax-audit.mjs" "$URL/"
[ -n "${SKIP_STRESS:-}" ] || run node "$HERE/stress-frames.mjs"
[ $fail -eq 0 ] && echo "ALL BROWSER CHECKS PASSED" || echo "BROWSER CHECKS FAILED"
exit $fail
