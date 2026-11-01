---
name: add-setting-or-goal
description: Add a new simulator setting (a node parameter), a new scoring goal, or a new run statistic to System Design Playground, test first, without changing existing results. Use when asked to model a new behaviour, lever, parameter or goal.
---

# Add a setting, a goal or a statistic

Ship it with the `ship-pr` skill.

## The rules that apply to everything

- **Determinism**: same diagram and seed give the same result. A feature that is off by default must not consume a random draw when off. Decide "is this request affected" with a hash of the request id (see `secondaryLookup`, `rangeRead`), never with `s.rng`. Never let map iteration order affect a result.
- **Defaults must not change existing results.** Write the test that a setting at its default, or named explicitly, equals the design that never mentions it. `TestCatalogGolden` and the alternatives test will also tell you.
- **Keep state small.** Prefer deriving a number from counters that exist (the ledger goal is a sum of two). `make perf-guard` must stay under 125 MB and 2.0M allocations.
- A setting that does nothing without another one must say so in its help text. A setting that could let a learner skip a challenge must be `.fixed()` there, and every route to the same cost (a replica, a second path) must pay it.

## A setting

1. **Test first**: in `backend/internal/engine/<feature>_test.go` (check the name is free with `ls`), using `builder{}` (`b.node(id, kind, Params{...}).edge(from, to)`), `Sanitize`, `New(d, Options{Seed: n})`, `s.Advance(ms)` and `s.RunSnapshot()` or `s.Snapshot().Nodes[id]`. Compare against a formula you can write down, or compare two settings by the extra time over a baseline. Watch it fail for the right reason.
2. **Registry**: `registry.go`, in the kind's `Params`: `num(key, label, group, default, min, max, step, unit, help)`, `sel(...)`, `flag(...)`; `.adv()` hides it under advanced, `.fixed()` locks it in challenges, `.showIf(key, "value")` is for selects and flags only (it compares strings), `.needs(kinds...)` for settings that need a neighbour. If it raises the cost, add it to the kind's `cost` function.
3. **Engine**: read it with `n.f("key")`, `n.s("key")`, `n.b("key")` (defaults are filled in). Put the behaviour in the file for its area (a new file per feature area is normal).
4. **Lint**: if misuse is common, add a rule in `lint.go` with a test.
5. **Docs**: the row in `docs/COMPONENTS.md` for the component and a feature row in the README with the measured numbers. The settings panel itself needs no frontend code.

## A goal

1. Field in `Goals` (`scoring.go`) with a JSON tag, a check in `Evaluate` (label, target, actual, pass), and the counter it reads in `RunStats` (`stats.go`) if it is new; compute derived values from the parts inside `Evaluate` so they cannot go stale.
2. Frontend: the mirror in `frontend/src/api/types.ts` (the goal and the stat), a row in `model/goals.ts`, and a test row in `model/goals.test.ts`. If the stat is a failure behaviour, add it to `model/memo.ts` (`runEvidence`) with a test.
3. A challenge that uses it comes through `add-challenge`.

## A statistic

Counter on `Sim` (`sim.go`), increment only when `c.counted` (past warm-up), expose it in `RunStats` (`stats.go`, in the literal in `Snapshot`), mirror in `api/types.ts`. A statistic is observation only; it must never change a result.

## Done when

The new test fails without the change and passes with it, the defaults test passes, `make check` passes, the PR states the measured numbers and the perf guard figures. Then `ship-pr`.
