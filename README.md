# System Design Playground

Sketch a system, send traffic, watch it break, then fix it. A browser-based simulator for learning
system design: drag servers, caches, databases, queues and more onto a canvas, wire them together,
press Play, and see queues fill, connection pools run dry, retries pile up and stale reads appear.
Guided challenges start from a broken design and score your fix from one to three stars.

```
frontend/   React + TypeScript + Vite + zustand + roughjs (hand-drawn canvas)
backend/    Go: deterministic discrete-event simulation engine, REST API, WebSocket live sessions
```

The simulation runs on the Go server. The browser edits the diagram, streams it to the server over a
WebSocket and renders the stats and request particles that come back, so edits apply to a running
simulation live.

## Quick start

### 1. Prerequisites

Pick **one** route:

- **Docker** (easiest, nothing else to install): any recent Docker Desktop or Engine, plus `make`.
- **From source**: Go 1.23+ and Node 22.12+ (macOS: `brew install go node`; check with `go version` and `node -v`), plus `make`.

### 2. Get the code

```sh
git clone https://github.com/tejas-k3/system_design_playground.git
cd system_design_playground
```

### 3. Run it

**Docker, in the background** (recommended for trying it out):

```sh
make up               # builds the image and starts it; restarts after a reboot or crash
# open http://localhost:8090
make logs             # follow the server log (Ctrl-C to stop following)
make down             # stop and remove it (shared designs are kept in the sdp-data volume)
```

**From source, one process** (builds the UI and serves it from the Go binary):

```sh
make install          # once: installs the frontend dependencies
make run              # builds both, then serves http://localhost:8090 (Ctrl-C to stop)
```

**For development** (live reload while you edit the frontend):

```sh
make install          # once
make dev              # Go API on :8090 + Vite on :5173; open http://localhost:5173
```

Use `PORT=9000 make up` (or `run` / `dev`) to change the port. Other targets: `make test`, `make lint`
(gofmt + staticcheck), `make bench`, `make build`, `make docker-run` (Docker in the foreground); `make help` lists all.
For configuration, reverse proxy, metrics and capacity notes see [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

### 4. First steps in the app

1. On the welcome screen choose **Load an example**, then press **Play** (or Space) and watch traffic flow.
2. Or open **Challenges** and start with *Hello, database*: each challenge starts broken and tells you its goals.
3. Everything you build is saved in your browser; **Share** (toolbar) copies a link to the current design.

### Troubleshooting

| Symptom | Fix |
|---|---|
| Page says "Can't reach the simulator" | The Go server is not running. Use `make up`, `make run`, or `make dev` (which starts both). |
| `address already in use` / port 8090 taken | Pick another: `PORT=9000 make up`. |
| `go: command not found` or an old Go | Install Go 1.23+ (`brew install go`) or use the Docker route. |
| `npm install` warns about the Node version | Install Node 22.12+ or use the Docker route. |
| Status dot is red / "Disconnected after being idle" | Idle sessions close after 10 minutes; press Play and it reconnects. |
| `make up` / `docker build` sits for minutes at "load metadata" or "pulling" | Docker Hub or gcr.io is unreachable or throttling you. Check `curl -sI https://registry-1.docker.io/v2/` first. The Dockerfile pins every base image by digest, so once the images are cached a rebuild needs no network for them; if the pull keeps stalling, build without Docker (`make run`) or retry later. To refresh the pins deliberately: `bash docs/review/tools/image-digests.sh`. |
| Want a clean slate | Clear the site's local storage in your browser; for Docker designs also `docker volume rm sdp-data` after `make down`. |

See [docs/COMPONENTS.md](docs/COMPONENTS.md) for what each component does, exactly when it fails, and why those rules are believable.

## How to play

1. **Build.** Click or drag components onto the canvas. Drag from the round handle on a component's
   right edge to connect it to another component (invalid connections are refused with a message).
2. **Run.** Press **Play** (or Space). Bars show how busy each component is; the metrics drawer shows
   throughput, latency, success rate and a breakdown of *why* requests failed.
3. **Break it.** Select a component and press **Crash it**, or raise the traffic.
4. **Fix it.** Change settings or structure while it runs.
5. **Follow a request.** Open the **Request traces** tab under the canvas, pick a failed or successful request
   and read its waterfall: hatched bars are time spent waiting in a queue, red marks where it failed, and
   faded bars are work that kept running after the user gave up. A one-line diagnosis says where the time
   or the failure came from.

Open **Challenges** for guided scenarios. Each has goals (success rate, latency, cost, double
bookings, stale reads…), three hints, a prediction question, a reference solution you can reveal after
the hints, and a debrief. Some are *configuration only*: the architecture is fixed.

Shortcuts: `Space` play/pause · `V` select · `C` connect · `Delete` remove · arrow keys move (Shift: far) · `Ctrl/⌘ Z` undo ·
`Shift+Z` redo · `F` fit view · `+`/`-` zoom · double-click empty space for a note · `Esc` cancel.

**Without a mouse or on a phone:** the first Tab stop on the canvas is a list of every component (Enter selects one);
the inspector's "Sends requests to" section connects and disconnects components from a dropdown, so nothing
requires dragging. On a touch screen, pinch to zoom and drag to pan; below 980 px wide the panels start closed
and the inspector becomes a bottom sheet.

## What is simulated

| Area | Behaviour |
|---|---|
| Traffic | Independent (Poisson), bursty (short rushes, same average) or closed-loop users (a fixed crowd that waits and thinks); Poisson arrivals; constant / ramp / spike / wave or authored curves; read/write mix; Zipf, uniform or scan key popularity; hot key |
| Clients | Timeouts, retries with exponential backoff + jitter or immediate retry; abandoned ("ghost") attempts keep consuming resources downstream |
| Service times | Steady by default; an advanced *service-time spread* setting (app server, database, worker, external API) switches to exponential or heavy-tailed, same mean, to show how variability lengthens queues and tails |
| Autoscaling | Optional per app server: a fleet of identical servers that grows when the last 30 s of thread utilisation is above a target, after a boot delay, and shrinks one server at a time after a quiet period; cost is the average number of servers that ran |
| App servers | Thread pool, bounded FIFO/LIFO queue, queue-wait timeout, call timeouts, circuit breaker, idempotency keys |
| Connection pools | Per server per database; databases refuse connections beyond their limit |
| Caches / CDN | LRU, LFU, FIFO, random eviction; TTL (with optional jitter); warm start; optional sharing of concurrent misses; read-through or cache-aside; invalidate / write-through |
| Replication | Replica serves the version from `now − lag`; strong / bounded / eventual reads and a stale-read count; optional failover: promote a replica after a detection and promotion time, losing writes newer than its lag (or nothing, with a synchronous replica that slows every write); an unfenced old primary that returns loses the writes made since |
| Load balancing | Round robin, least connections, random; health-check delay; outlier ejection |
| Queues | Bounded queue + workers, retries, backlog and job-latency metrics |
| Rate limiter | Token bucket |
| Sharding / geo / leaderboard / booking | Shard routing by key; spatial index; Redis sorted-set style ops; row locks and Redis locks with TTL, double-booking detection |
| Chaos | Crash (auto-restart), slow-down (steady or pulsing) and network-partition events, scheduled or manual |
| Question formats | Beyond fixing a broken design: **estimate** (numbers graded within 30% / 2x, then built into a fleet and tested against the real peak), **diagnose** (a dashboard and one failed trace generated from a seeded run, pick the cause), **trade-off** (run each candidate design, then choose against a stated requirement), **review** (tick the flawed components; scored by found, missed and false marks) and **open** challenges (blank canvas, hidden load and failures, no solution to reveal). Every answer key stays on the server until you submit |
| Interview mode | A timed mock interview in seven stages (requirements, estimation, API, data model, high-level design, deep dive, trade-offs): a countdown computed from timestamps, so a hidden tab cannot drift it; a calculator (users to QPS, peak, storage, bandwidth, servers) with a warning when a payload is too big for a message queue and a *Build a starter* button; requirements, API and data-model pads with interviewer-style prompts; a test of your design against your own availability and latency targets; a self-rating beside evidence from the lint and the last run; PNG and Markdown export; and a present mode driven by the arrow keys. Everything is kept in this browser (localStorage), nothing is sent to the server |
| Daily challenge | The welcome screen offers one challenge a day: every challenge takes a turn before any repeats, never the same one two days running, chosen from the date alone. A streak counts consecutive days it was solved, kept in this browser only (localStorage), with no accounts and no leaderboard |
| Skill map | Every challenge is tagged with the ideas it practises (ten concepts: capacity, caching, replication, consistency, queues, failure handling, sharding, overload control, storage and payloads, autoscaling). The skill map lists one row per concept with the stars earned out of those available and a button for the challenge to try next (fewest stars first, easiest first), computed from the stars kept in this browser |
| Glossary | Twenty-nine terms (percentiles, utilisation, Little's law, circuit breaker, retry storm, idempotency, stampede, replication lag, split brain, fencing, sharding, hot key, backpressure, load shedding, SLO and more) with a one-sentence and a fuller definition. In challenge stories, hints and debriefs a marked word is keyboard-focusable and shows its definition on hover or focus (read out by screen readers, closed with Escape), with a button to a challenge about it |
| Feed fan-out | A **feed** workload with Zipf-skewed followers and an app-server strategy: push on write (a post costs one insert per follower, so celebrities flood the database), pull on read, or a hybrid that pulls the few huge accounts; the overview counts timeline inserts and followed authors read |
| Overload control | Users with distinct accounts and a rate limiter that limits per user or for everyone, with token bucket, fixed window and sliding window algorithms; bot traffic (a scraper that defeats caches and never retries); a client retry budget (stops retry storms); per-dependency bulkheads; deadline propagation (skip work whose client has gone); priority shedding when the queue is long. The customers-only score sets bots aside |
| Hot keys | One item taking most of the reads (hot item share) can pile onto a single cache node; an app server can keep its hottest items in a small local cache (size and lifetime set), which shows as local cache hits and takes that load off the cache node. A write through that server drops its own copy; other servers keep theirs until they expire . A shard router can spread by key (one hot map cell lands on one shard) or by request (even load; reads of one key would then need every shard, not modelled) |
| Long-lived connections | Connected users hold a connection open to an app server (setup cost, a slot each). A server holds as many as its connection limit and its memory allow (memory per connection, after the base footprint and thread working memory), independent of threads; the rest are refused. When a server dies its users reconnect after a random wait, so with no spread every user returns at once (a reconnect storm). Messages travel as ordinary requests and are not tied to the server holding the connection |
| Messaging | Queues with attempts per job, a dead-letter queue, poison jobs, visibility timeouts (at-least-once redelivery and the duplicates it causes), urgent-first priorities and delayed delivery; a **Topic** (every subscriber gets its own copy with its own backlog, a slow one falls behind alone); a partitioned **Log** with consumer groups (one reader per partition per group, per-key ordering with key partitioning, out-of-order messages with round robin, idle consumers beyond the partition count) |
| Search fan-out | A Search Shard component and an app-server **fan-out** setting: ask every shard and wait for all (latency is the slowest of N), answer on the first K, hedge slow shards, allow partial results; the overview counts hedged queries and partial answers |
| Payload and links | Request and response sizes on the users; link bandwidth as a shared capacity (bytes queue behind bytes, so a saturated link shows its limit and latency climbs); a queue's message-size limit ("Message too large"); an object store and the claim-check pattern (upload the body, queue a pointer); downloads from the store with usage-based cost (egress per GB, requests, storage and a projected year of growth); byte-limited caches and CDNs, chained CDN tiers with a collapsing shield, and a purge event |
| Failure modes | Stop-the-world GC pauses (random gaps, a latency tail with an unmoved median); secondary indexes (each costs 30% on writes) and the full-scan cost of a lookup with none; LSM compaction stalls; transaction isolation levels (lost updates, write skew, aborts, deadlocks); coarse data-model presets (document, wide-column, time-series); CPU burst credits; memory limits with out-of-memory kills; disk IOPS; data larger than the buffer pool (reads missing to disk); connection setup cost; link packet loss, jitter and bandwidth; one-way and two-way link failures |
| Investigation | | Investigation | Capacity curve (load sweep with the knee marked), your design versus the reference solution, side-by-side comparison of two designs with overlaid series, and a rewind slider that replays a run exactly (the engine is deterministic, so a seek reproduces the live run, including edits and crashes you made) |
| Observation | Charts mark crashes, restarts, partitions, failover steps, split brain and autoscaler steps at the time they happened; latency distribution with p50 / p95 / p99; per-component 1 Hz timelines; countdown chips for failover and booting servers; an advisory design review (lint), a live "what is limiting it" coach and an availability error budget with burn rate. Observation only: none of it changes a result |

Runs are **deterministic**: the same diagram and seed always produce the same result.

## Architecture

```
backend/
  cmd/server/            HTTP server entry point (flags / env: ADDR, STATIC_DIR, ALLOWED_ORIGINS)
  internal/engine/       the simulator (no I/O, fully unit tested)
    sim.go                   core types, event loop, 100 ms tick
    topology.go clients.go   building and live-editing the graph; traffic, retries
    flow.go data.go          request flow, pools, queues; per-node behaviour
    chaos.go versions.go     failure injection; item versions and staleness
    trace.go                 sampled per-request traces (observation only)
    registry.go sanitize.go  node kinds and parameters; validation of untrusted diagrams
    scoring.go stats.go      goals and stars; the snapshots the UI renders
  internal/scenarios/    challenges as data (+ tests: start must fail, solution must pass)
  internal/api/          REST + WebSocket session
frontend/src/
  api/ model/ state/     wire types, diagram helpers + sanitizer, zustand store, persistence
  sim/                   WebSocket client and the store ⇄ server bridge
  draw/                  canvas renderer (roughjs), geometry and hit-testing, icons
  ui/                    Board, Toolbar, Library, Inspector, Charts, challenge panel, dialogs
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
| `POST /api/sweep` | Same design at several loads (`loads`: multiples of every client's rate) or one setting at several values (`param`, `values`, clamped to the setting's range); up to 20 points, `durationSec` ≤ 60, one run slot and a 20 s deadline for the whole sweep |
| `GET /api/questions`, `GET /api/questions/{id}` | The non-fix question formats, without their answer keys |
| `POST /api/questions/{id}/answer`, `/run`, `/review`, `/build` | Grade an estimate or a choice, run a trade-off candidate, score a review, or build and test an estimate; keys are returned only with the result |
| `GET /api/sim` (WebSocket) | Live simulation session |
| `POST /api/designs`, `GET /api/designs/{id}` | Share a design: save returns `{id}` (rate-limited, sanitized); the id opens it |
| `GET /api/healthz` | Liveness |

### WebSocket protocol (`/api/sim`)

Client → server: `init {diagram, scenarioId?, seed?, speed?}`, `diagram {diagram}` (live edit),
`play`, `pause`, `step`, `reset`, `speed {speed}`, `traces`, `crash {nodeId}`, `restart {nodeId}`.
Server → client: `frame {status, speed, stats?, particles?, notices?}` (~25 Hz, stats ~10 Hz),
`traces {failed, ok}` (answer to `traces`: the most recent traced requests, newest first),
`result {scenarioId, score}` when a challenge finishes, `error {message}`.

Limits: at most 64 live simulations at once (`-max-sessions` / `MAX_SESSIONS`; further connections get
`503` with `Retry-After`), and a stopped or paused session is closed after 10 minutes of silence
(`-idle-timeout`). The client then goes dormant and reconnects the next time you press Play.

The server advances at most 100 ms of wall time per tick (scaled by speed, in ≤10 ms slices under a
12 ms CPU budget) and warns when it has to slow down. Configuration-only challenges reject topology
changes on the server as well as in the UI.

## Authoring a challenge

Challenges live in `backend/internal/scenarios/defs.go`: a starting `Diagram`, a reference `Solution`,
`Goals`, optional `Traffic` curves and `Chaos` events, three hints, a prediction and a debrief.
`go test ./internal/scenarios` fails unless the starting design fails the goals and the solution passes
them across several seeds, so a mistuned scenario is caught immediately. `VERBOSE=1 go test -v
./internal/scenarios` prints every check.

## Persistence and sharing

The sandbox diagram, theme, view position and challenge stars are stored in the browser's
`localStorage` (`sdp:v1`), validated on load.

The **Share** button saves the current design on the server and copies a link such as `/#design=3ibupvsc57zfg`;
opening it loads the design into your sandbox as an undoable step. There are no accounts: the 13-character
random id is the only access control, so anyone with the link can open that design and nothing else is
readable. Designs live in memory unless the server is started with `-data-dir` (`DATA_DIR`), in which case they
are also written there and survive restarts. At most 5,000 are kept (`-max-designs`), oldest dropped first,
and one client may share 20 a minute.

## Tests

`make test` runs `go vet` + Go tests (engine mechanisms, scenario calibration, API and WebSocket
sessions) and the frontend type-check + unit tests.

## License

[MIT](LICENSE) © 2026 Tejas Kothari.
