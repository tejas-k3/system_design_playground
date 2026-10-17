# System Design Playground

Sketch a system, send traffic, watch it break, then fix it. A browser-based simulator for learning
system design: drag servers, caches, databases, queues and more onto a canvas, wire them together,
press Play, and see queues fill, connection pools run dry, retries pile up and stale reads appear.
Thirteen guided challenges start from a broken design and score your fix from one to three stars.

```
frontend/   React + TypeScript + Vite + zustand + roughjs (hand-drawn canvas)
backend/    Go: deterministic discrete-event simulation engine, REST API, WebSocket live sessions
```

The simulation runs on the Go server. The browser edits the diagram, streams it to the server over a
WebSocket and renders the stats and request particles that come back, so edits apply to a running
simulation live.

## Quick start

Requires Go 1.23+ and Node 20+.

```sh
make install          # once: frontend dependencies
make dev              # Go API on :8090 + Vite on :5173, open http://localhost:5173
```

Or as a single binary serving the built UI:

```sh
make run              # builds both, then http://localhost:8090
```

Other targets: `make test`, `make bench`, `make build`. Use `PORT=9000 make dev` to change the port.

## How to play

1. **Build.** Click or drag components onto the canvas. Drag from the round handle on a component's
   right edge to connect it to another component (invalid connections are refused with a message).
2. **Run.** Press **Play** (or Space). Bars show how busy each component is; the metrics drawer shows
   throughput, latency, success rate and a breakdown of *why* requests failed.
3. **Break it.** Select a component and press **Crash it**, or raise the traffic.
4. **Fix it.** Change settings or structure while it runs.

Open **Challenges** for guided scenarios. Each has goals (success rate, latency, cost, double
bookings, stale reads…), three hints, a prediction question, a reference solution you can reveal after
the hints, and a debrief. Some are *configuration only*: the architecture is fixed.

Shortcuts: `Space` play/pause · `V` select · `C` connect · `Delete` remove · `Ctrl/⌘ Z` undo ·
`Shift+Z` redo · `F` fit view · `+`/`-` zoom · double-click empty space for a note · `Esc` cancel.

## What is simulated

| Area | Behaviour |
|---|---|
| Traffic | Poisson arrivals; constant / ramp / spike / wave or authored curves; read/write mix; Zipf, uniform or scan key popularity; hot key |
| Clients | Timeouts, retries with exponential backoff + jitter or immediate retry; abandoned ("ghost") attempts keep consuming resources downstream |
| App servers | Thread pool, bounded FIFO/LIFO queue, queue-wait timeout, call timeouts, circuit breaker, idempotency keys |
| Connection pools | Per server per database; databases refuse connections beyond their limit |
| Caches / CDN | LRU, LFU, FIFO, random eviction; TTL; warm start; read-through or cache-aside; invalidate / write-through |
| Replication | Replica serves the version from `now − lag`; strong / bounded / eventual reads and a stale-read count |
| Load balancing | Round robin, least connections, random; health-check delay; outlier ejection |
| Queues | Bounded queue + workers, retries, backlog and job-latency metrics |
| Rate limiter | Token bucket |
| Sharding / geo / leaderboard / booking | Shard routing by key; spatial index; Redis sorted-set style ops; row locks and Redis locks with TTL, double-booking detection |
| Chaos | Crash (auto-restart) and slow-down events, scheduled or manual |

Runs are **deterministic**: the same diagram and seed always produce the same result.

## Architecture

```
backend/
  cmd/server/            HTTP server entry point (flags / env: ADDR, STATIC_DIR, ALLOWED_ORIGINS)
  internal/engine/       the simulator (no I/O, fully unit tested)
    sim.go flow.go data.go   event loop, request flow, per-node behaviour
    registry.go              node kinds, parameters, connection rules, cost
    sanitize.go              validates untrusted diagrams
    scoring.go stats.go      goals, stars and the snapshots the UI renders
  internal/scenarios/    challenges as data (+ tests: start must fail, solution must pass)
  internal/api/          REST + WebSocket session
frontend/src/
  api/ model/ state/     wire types, diagram helpers + sanitizer, zustand store, persistence
  sim/                   WebSocket client and the store ⇄ server bridge
  draw/                  canvas renderer (roughjs), geometry and hit-testing, icons
  ui/                    Board, Library, Inspector, Charts, Scenario panels, modals
```

The engine is a priority-queue discrete-event simulation. Each request is a chain of callbacks across
nodes; capacity-limited nodes (app threads, DB connections, Redis, workers) are *stations* with a
bounded queue. A 100 ms tick snapshots per-node counters into ring buffers that become the live stats.
A 30 s run of a 1,000 rps, 20-node design takes about 90 ms (`make bench`).

### HTTP API

| Method & path | Purpose |
|---|---|
| `GET /api/registry` | Node kinds with parameter specs; the inspector is rendered from this |
| `GET /api/scenarios`, `GET /api/scenarios/{id}` | Challenge list / one challenge (without its solution) |
| `GET /api/scenarios/{id}/solution` | Reference solution diagram |
| `POST /api/validate` | Sanitize a diagram, returns `{diagram, adjusted, costPerHour}` |
| `POST /api/run` | Headless run `{diagram, scenarioId?, seed?, durationSec?}` → stats (+ score for a scenario) |
| `GET /api/sim` (WebSocket) | Live simulation session |
| `GET /api/healthz` | Liveness |

### WebSocket protocol (`/api/sim`)

Client → server: `init {diagram, scenarioId?, seed?, speed?}`, `diagram {diagram}` (live edit),
`play`, `pause`, `step`, `reset`, `speed {speed}`, `crash {nodeId}`, `restart {nodeId}`.
Server → client: `frame {status, speed, stats?, particles?, notices?}` (~25 Hz, stats ~10 Hz),
`result {scenarioId, score}` when a challenge finishes, `error {message}`.

The server advances at most 100 ms of wall time per tick (scaled by speed, in ≤10 ms slices under a
12 ms CPU budget) and warns when it has to slow down. Configuration-only challenges reject topology
changes on the server as well as in the UI.

## Authoring a challenge

Challenges live in `backend/internal/scenarios/defs.go`: a starting `Diagram`, a reference `Solution`,
`Goals`, optional `Traffic` curves and `Chaos` events, three hints, a prediction and a debrief.
`go test ./internal/scenarios` fails unless the starting design fails the goals and the solution passes
them across several seeds, so a mistuned scenario is caught immediately. `VERBOSE=1 go test -v
./internal/scenarios` prints every check.

## Persistence

The sandbox diagram, theme, view position and challenge stars are stored in the browser's
`localStorage` (`sdp:v1`), validated on load. There is no account system and the server keeps no state
between sessions.

## Tests

`make test` runs `go vet` + Go tests (engine mechanisms, scenario calibration, API and WebSocket
sessions) and the frontend type-check + unit tests.
