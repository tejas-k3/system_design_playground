#!/bin/sh
# Drag a component and check that nothing of its body stays behind at the old position (the stale-drawing bug).
# usage: URL=http://localhost:8091 sh check-drag.sh     (1440x900, "Load an example", drags Users straight down)
URL=${URL:-http://localhost:8091}
DIR=$(dirname "$0")
COUNT='(()=>{const c=document.querySelector("canvas");const x=c.getContext("2d");const d=x.getImageData(78,230,160,84).data;let n=0;for(let i=0;i<d.length;i+=4){if(d[i]<90&&d[i+1]<90&&d[i+2]<90)n++}return n})()'
OUT=$(node "$DIR/walk.mjs" 1440 900 /tmp/check-drag "[{\"goto\":\"$URL/\",\"wait\":2000},{\"click\":\"Load an example\",\"wait\":1500},{\"eval\":$(printf '%s' "$COUNT" | python3 -c 'import json,sys;print(json.dumps(sys.stdin.read()))'),\"log\":\"dark pixels where Users sits, before\"},{\"drag\":[370,335,370,560]},{\"wait\":500},{\"eval\":$(printf '%s' "$COUNT" | python3 -c 'import json,sys;print(json.dumps(sys.stdin.read()))'),\"log\":\"dark pixels where Users sat, after\"},{\"shot\":\"after-drag\"}]" 2>&1)
echo "$OUT" | grep "dark pixels"
BEFORE=$(echo "$OUT" | grep "sits, before" | sed 's/.* //')
AFTER=$(echo "$OUT" | grep "sat, after" | sed 's/.* //')
if [ "${BEFORE:-0}" -gt 100 ] && [ "${AFTER:-999}" -lt 20 ]; then echo "PASS: nothing left behind"; else echo "FAIL: stale drawing at the old position (before=$BEFORE after=$AFTER)"; exit 1; fi
