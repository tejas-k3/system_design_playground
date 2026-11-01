#!/bin/sh
# PostToolUse (Edit|Write): keep Go files formatted, so `make lint` never fails on gofmt.
f=$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("tool_input",{}).get("file_path",""))' 2>/dev/null)
case "$f" in
  *.go) [ -f "$f" ] && gofmt -w "$f" ;;
esac
exit 0
