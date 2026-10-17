#!/usr/bin/env bash
# Browser regression check for F-010: with a challenge open, the canvas must stay usably wide at laptop sizes,
# both just after opening it and after selecting a component (which opens the inspector).
# Needs the app on $URL (default http://localhost:8090) and Chrome.  exit 0 = every size passes.
# usage: check-layout.sh [minCanvasPx]   (default 480)
set -uo pipefail
URL=${URL:-http://localhost:8090}; MIN=${1:-480}
HERE=$(cd "$(dirname "$0")" && pwd); fail=0
CANVAS='Math.round(document.querySelector(\".board\").getBoundingClientRect().width)'
for size in 1024:768 1280:800 1366:768 1440:900 1920:1080; do
  W=${size%%:*}; H=${size##*:}
  STEPS='[{"goto":"'$URL'/","wait":2000},{"click":"Challenges","wait":600},{"click":"Hello, database","wait":2000},{"eval":"'$CANVAS'","log":"OPEN"},{"click":"Cache","wait":800},{"eval":"'$CANVAS'","log":"SELECTED"},{"eval":"document.documentElement.scrollWidth <= window.innerWidth","log":"NOSCROLL"}]'
  OUT=$(node "$HERE/walk.mjs" "$W" "$H" /tmp/review/check-layout "$STEPS")
  open=$(echo "$OUT" | grep '^OPEN' | awk '{print $2}'); sel=$(echo "$OUT" | grep '^SELECTED' | awk '{print $2}'); ns=$(echo "$OUT" | grep '^NOSCROLL' | awk '{print $2}')
  ok=PASS; { [ "${open:-0}" -ge "$MIN" ] && [ "${sel:-0}" -ge "$MIN" ] && [ "$ns" = "true" ]; } || { ok=FAIL; fail=1; }
  printf '%s  %sx%s  canvas after opening=%spx  after selecting=%spx  no horizontal scroll=%s  (need >= %s)\n' "$ok" "$W" "$H" "$open" "$sel" "$ns" "$MIN"
done
exit $fail
