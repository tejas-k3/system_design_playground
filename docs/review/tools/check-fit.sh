#!/usr/bin/env bash
# Browser regression check for F-003: opening a challenge must fit the design exactly as pressing F does,
# i.e. every component is on screen. Needs the app running on $URL (default http://localhost:8090) and Chrome.
# usage: check-fit.sh [width height]    exit 0 = auto-fit equals manual fit, 1 = they differ (components clipped)
set -euo pipefail
URL=${URL:-http://localhost:8090}; W=${1:-1440}; H=${2:-900}
HERE=$(cd "$(dirname "$0")" && pwd)
STEPS='[{"goto":"'$URL'/","wait":2000},{"click":"Challenges","wait":600},{"click":"Hello, database","wait":2000},{"eval":"document.querySelector(\".zoom-val\").textContent","log":"AUTO"},{"eval":"window.dispatchEvent(new KeyboardEvent(\"keydown\",{key:\"f\",bubbles:true}))","wait":700},{"eval":"document.querySelector(\".zoom-val\").textContent","log":"MANUAL"}]'
OUT=$(node "$HERE/walk.mjs" "$W" "$H" /tmp/review/check-fit "$STEPS")
echo "$OUT" | grep -E "AUTO|MANUAL"
AUTO=$(echo "$OUT" | grep AUTO | sed -E 's/.*"([0-9]+%)".*/\1/'); MANUAL=$(echo "$OUT" | grep MANUAL | sed -E 's/.*"([0-9]+%)".*/\1/')
if [ "$AUTO" = "$MANUAL" ]; then echo "PASS: auto-fit $AUTO == manual fit $MANUAL at ${W}x${H}"; else echo "FAIL: auto-fit $AUTO != manual fit $MANUAL at ${W}x${H} (components are clipped)"; exit 1; fi
