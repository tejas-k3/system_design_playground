# Deploying System Design Playground

The whole app is one small Go binary that serves the built UI, the API and the simulation WebSocket.
A container image is the easiest way to run it.

## Run with Docker

```sh
make docker-run                     # builds the image and serves http://localhost:8090
# or by hand
docker build -t system-design-playground .
docker run -d --name sdp -p 8090:8090 -v sdp-data:/data system-design-playground
```

The image runs as a non-root user, stores shared designs in `/data` (mount a volume to keep them across
restarts) and reports its own health (`docker ps` shows `healthy`), using the binary's `-healthcheck` mode.

## Configuration

Every setting is a flag or an environment variable (the flag wins).

| Flag | Environment | Default | Meaning |
|---|---|---|---|
| `-addr` | `ADDR` | `:8090` | Listen address |
| `-static` | `STATIC_DIR` | *(none; `/app/web` in the image)* | Built frontend to serve; without it only the API runs |
| `-data-dir` | `DATA_DIR` | *(none; `/data` in the image)* | Where shared designs are stored; empty keeps them in memory only |
| `-max-designs` | `MAX_DESIGNS` | `5000` | Designs kept before the oldest are dropped |
| `-max-sessions` | `MAX_SESSIONS` | `64` | Concurrent live simulations; more get `503` with `Retry-After` |
| `-idle-timeout` | | `10m` | Close a stopped or paused session after this much silence |
| `-origins` | `ALLOWED_ORIGINS` | `localhost:*,127.0.0.1:*` | Extra WebSocket origins to accept (comma-separated host patterns) |
| `-healthcheck` | | | Probe a running server on `-addr` and exit 0 only if healthy |

## Behind a reverse proxy

Put TLS, compression and any outer rate limiting in front of the app. Requirements:

- **Forward WebSockets** on `/api/sim` (`Upgrade` and `Connection` headers) and give them a long read timeout.
- **Preserve the `Host` header.** The WebSocket accepts a browser whose `Origin` matches the request host
  automatically; with the original `Host` forwarded no origin configuration is needed. If the proxy rewrites
  `Host`, list the public hostname in `ALLOWED_ORIGINS`. Any other origin is refused with `403`.
- **Compress responses.** The server does not gzip; the main script is about 100 KB compressed and 320 KB raw.

Minimal nginx:

```nginx
location / {
    proxy_pass http://127.0.0.1:8090;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;   # map $http_upgrade $connection_upgrade { default upgrade; '' close; }
    proxy_read_timeout 1h;
}
gzip on;
gzip_types text/css application/javascript image/svg+xml;
```

The design-saving rate limit (20 a minute) is per connecting address. Behind a proxy every request appears to come
from the proxy, so apply a per-client limit at the proxy as well and treat the app's limit as a backstop.

## Capacity

A simulation runs on the server while it is playing, using up to one CPU core at high speed with a large design (a
1,000 requests-per-second, 20-node design simulates about 300 times faster than real time, so ordinary use is far
lighter). `MAX_SESSIONS` is therefore the knob that bounds CPU: size it to roughly the number of cores you can spare
for simulations. Idle sessions are closed automatically, and the browser reconnects when its user presses Play.
Stopped and paused sessions cost almost nothing.

## Observability

- `GET /api/healthz` returns `{"status":"ok"}`; use it for load balancer and orchestrator probes.
- `GET /api/metrics` is Prometheus text: `sdp_sessions_active`, `sdp_sessions_total`, `sdp_sessions_refused_total`
  (a rising value means `MAX_SESSIONS` is too low or you are being hammered), `sdp_runs_total`,
  `sdp_designs_saved_total`, `sdp_designs_rate_limited_total` and `sdp_designs_stored`.
  Expose it only to your monitoring network.
- Requests under `/api` are logged with method, path, status and duration.

## Data, security and backups

- **Shared designs** are the only server-side data: one small JSON file per design under `DATA_DIR`. Back that
  directory up if the links matter; deleting a file removes that link.
- There are **no accounts**. A design's 13-character random id is its only protection, so share links only with people
  who may see the design. The server sanitizes everything it stores and returns, and caps uploads at 256 KB.
- Responses carry a strict Content-Security-Policy (own scripts, styles, fonts and connections only; no inline
  scripts; no framing), `nosniff`, and a referrer and permissions policy. Hashed build files under `/assets/` are
  cached for a year; `index.html` is always revalidated so a new deploy takes effect immediately.
- CI builds the image and smoke-tests that it starts and answers its health check on every push.
