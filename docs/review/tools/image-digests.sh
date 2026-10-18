#!/usr/bin/env bash
# Print the current digests of the base images the Dockerfile pins, so they can be refreshed with
# `sed` or by hand. Needs only curl and python3 (no Docker daemon), so it works when `docker pull` stalls.
# usage: image-digests.sh
set -euo pipefail
ACCEPT="application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json"
hub() { # repo tag
  tok=$(curl -fsS -m 30 "https://auth.docker.io/token?service=registry.docker.io&scope=repository:$1:pull" | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')
  d=$(curl -fsSI -m 30 -H "Authorization: Bearer $tok" -H "Accept: $ACCEPT" "https://registry-1.docker.io/v2/$1/manifests/$2" | tr -d '\r' | awk -F': ' 'tolower($1)=="docker-content-digest"{print $2}')
  echo "$1:$2@$d"
}
hub docker/dockerfile 1
hub library/node 22-alpine
hub library/golang 1.23-alpine
d=$(curl -fsSI -m 30 -H "Accept: $ACCEPT" https://gcr.io/v2/distroless/static-debian12/manifests/nonroot | tr -d '\r' | awk -F': ' 'tolower($1)=="docker-content-digest"{print $2}')
echo "gcr.io/distroless/static-debian12:nonroot@$d"
