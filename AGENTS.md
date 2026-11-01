# Working in this repo

For coding agents and new contributors. It says how the repo works and how changes are made. What to build lives in the issues; what exists is in `README.md` (features) and `docs/COMPONENTS.md` (every component and setting). If something here is wrong, fix it in the same PR.

## What it is

System Design Playground: a browser app where a learner draws a system, sends simulated traffic, watches it fail and fixes the design. Guided *challenges* start from a broken design and score one to three stars.

- **Backend** (Go, module `sysdesign`, `backend/`): a deterministic discrete-event simulator (same diagram + seed gives an identical result), a REST API and a WebSocket live session. The simulation runs on the server; the browser never simulates.
- **Frontend** (React, Vite, TypeScript, zustand, canvas, vitest in a node environment, `frontend/`): edits the diagram, streams it to the server, draws stats.

## Map

```
backend/internal/engine/      the simulator (no I/O). Orientation:
  types.go registry.go        node kinds, wire types, failure reasons; one NodeSpec per component with its settings (num/sel/flag, .adv() .fixed() .needs() .showIf()) and cost;
                              the specs themselves are one function per kind in registry_specs.go, built once on first use (no init())
  sanitize.go                 clamps and validates a diagram from the registry
  sim.go clients.go flow*.go  the Sim, traffic generation and retries, the request path (flow.go hops and calls, flow_station.go, flow_pool.go, flow_balancer.go, flow_cdn.go)
  data*.go chaos.go           per-kind behaviour (data.go app server, data_cache.go, data_database.go, data_redis.go; queue...) and crash/slow/partition/failover
  workflow.go stream.go ...   one file per feature area (sagas and commits, stream jobs, auth, sessions, gossip, ranking, graph...)
  stats.go scoring.go lint.go statistics (RunStats), Goals and Evaluate (checks, stars), design lint rules
backend/internal/scenarios/   challenges: defs_*.go, scenarios_open*.go (blank-canvas briefs), alternatives.go (measured "what if" rows),
                              levels.go, concepts.go, questions_*.go, studydeck.go, testdata/catalog.golden
backend/internal/api/         routes, WebSocket session, limits
frontend/src/model/           pure, unit-tested logic (goals, glossary, memo, estimates...)
frontend/src/{state,sim,draw,ui,api}/   store, bridge to the server, canvas drawing, components, wire types (api/types.ts mirrors the Go JSON)
docs/                         COMPONENTS.md, DEPLOYMENT.md, ACCESSIBILITY_TESTING.md are published; everything else in docs/ is local notes and never committed
```

The UI settings panel is rendered from `GET /api/registry`: a setting added in `registry_specs.go` appears with no frontend code.

## Rules of the engine

- **Determinism is a hard requirement.** Randomness comes from `s.rng`, `s.keyRng` and `s.chaosRng`. A feature that is off by default must not consume a random draw when off, so existing results stay bit-identical. Never let map iteration order affect a result. Verdicts such as "is this request affected" use hashes of the request id, not draws.
- Traces are observation only and must never change results.
- Defaults must not change existing results: a new setting at its default is equal to the design that never mentions it (write that test).
- Keep per-request and per-node state small. `make perf-guard` measures a large design and fails above 125 MB or 2.0M allocations (today about 115 MB and 1.86M). Prefer deriving a number from counters that exist.
- Models are coarse and documented as such. A number in a doc or an alternatives row must be one the tests re-measure.

## Recipes

**Add a setting**: the kind's function in `registry_specs.go` (group, default, range, help text, `.adv()` for advanced), read it with `n.f/n.s/n.b`, test in `engine/*_test.go` using `builder{}` and `run(diagram, Options, seconds)`. Add the row to `docs/COMPONENTS.md`.

**Add a goal**: field in `Goals` and a check in `Evaluate` (`scoring.go`), the counter in `RunStats` (`stats.go`), the mirror in `frontend/src/api/types.ts`, a row in `model/goals.ts` with a test row in `goals.test.ts`.

**Add a challenge**: build the start and the reference with `bld{}` in a `defs_*.go` (one `func (c *catalog) addXxx()` per challenge that calls `c.register`, listed in its section's `registerXxx` method; there is no `init()`; if you add a new file of challenges, call it from `buildCatalog` in `scenarios.go`), plus goals, three hints, an answer, a prediction, a debrief and `ConfigOnly` if the topology is fixed. Then: an entry in `levels.go` and `concepts.go`, stored alternatives in `alternatives.go` (numbers come from the failing test output), `UPDATE_GOLDEN=1 go test ./internal/scenarios -run TestCatalogGolden`. Quality gates you will meet: three hints, star targets strictly stricter than limits, reference earns two or more stars on seeds 1-3 and the same stars on seeds 1-8, prediction options within 1.6x in length, at most 24 challenges without a glossary term (`{{id|words}}`; the id must be listed in `glossary.ts`, and challenge ids in `glossary.test.ts`), the Hard count pinned in `debrief_test.go`, component and open-challenge counts pinned in the API test.

**Add a component kind**: constant in `types.go`, `NodeSpec` function in `registry_specs.go` (and its line in `buildSpecs`, `registry.go`), `arrive`/`process` in `flow_station.go`, a `proc<Kind>`, stats; frontend icon in `draw/icons.ts`, group in `model/library.ts`.

## Working agreement

- One branch and one PR per item. Test first: write the failing test, watch it fail for the right reason, then implement. Never loosen, delete or skip a test to get green; report the disappointing number instead.
- Before every push run `make check` (markdown allow-list, tests, lint, perf guard, frontend build; it stops at the first failure; the browser checks run in CI) and wait for it. If you run the steps yourself, join them with `&&`, never `;`.
- Review the diff with the `code-review` skill and fix what it finds, then wait for green CI and merge with a merge commit (`gh pr merge N --merge`). No direct pushes to `main`, no force pushes. Commit trailers and PR footers follow the session's attribution reminder.
- Comment on a finished ticket and close it. The owner runs the 900x700 browser check; a look at 1280x820 in a real browser is fine for UI changes (`make browser-check` runs the automated layout checks).
- Published markdown is an allow-list (`README.md`, `AGENTS.md`, `CLAUDE.md`, `.claude/skills/*/SKILL.md`, `docs/COMPONENTS.md`, `docs/DEPLOYMENT.md`, `docs/ACCESSIBILITY_TESTING.md`). The pre-commit hook (`make hooks`) and CI enforce it. Never commit `docs/review/`.
- Accessibility is a requirement: new UI needs an accessible name, a keyboard path, 4.5:1 text contrast in both themes and must not rely on colour alone.
- Report honestly in PRs: what was measured, what was not tested.

## Skills and hooks (in `.claude/`)

- Skills: `ship-pr` (check, local review, PR, CI, merge, ticket, cleanup), `add-challenge`, `add-setting-or-goal`. Use them instead of working from memory.
- Claude Code hooks (`.claude/settings.json`, scripts in `.claude/hooks/`): gofmt after Go edits; refusal (judged per git command, in the directory a preceding `cd` moved to) of force pushes, pushes to `main`, `git add -f`, staging `node_modules` or `docs/review/`, and `git worktree add` onto an existing path.
- Git hooks (`make hooks` once per clone): pre-commit (markdown allow-list, staged `node_modules`, gofmt) and pre-push (refuses `main`).

## Pitfalls that already cost time

- Check that a file name is free before creating it (`ls`): two existing files were overwritten by a new file with the same name.
- `git worktree list` before `git worktree add ../name`: old worktrees may already own the name. A `ln -s` of `frontend/node_modules` into a worktree must never be committed.
- Use `;` only for commands that may fail independently; a pinned-count test failed twice after a chained push.
- zsh expands unquoted globs: quote `'*.go'` in `grep --include`. macOS `sed -i` needs an extension; use a small Python edit instead.
- `showIf` compares `String(value)`, so it suits selects and flags, not numbers.
- A chaos event's downtime field is `DownSec` (not `DurationSec`) for a crash.
- A scenario star bar that sits on the reference solution's own number flips with the seed; keep a margin.
- A background review runs on the diff of the shell's current worktree: give it the path explicitly.
