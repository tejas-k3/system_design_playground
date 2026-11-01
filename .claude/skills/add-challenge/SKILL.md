---
name: add-challenge
description: Add a new guided challenge (start design, reference solution, goals, hints, alternatives) to System Design Playground and pass every quality gate on the first or second try. Use when asked to add, build or calibrate a challenge, scenario or brief.
---

# Add a challenge

Work test first and calibrate by measuring, never by guessing. Ship it with the `ship-pr` skill.

## 1. Write the lesson before the numbers

One idea per challenge. Decide: the failure the learner sees, the reference fix, and two or three wrong levers that look reasonable and fail (these become the stored alternatives). If a wrong lever passes, say so honestly in its note, or change the design until it does not.

## 2. Build the diagrams

In the fitting `backend/internal/scenarios/defs_*.go` (check the name list with `ls`; open blank-canvas briefs live in `scenarios_open*.go`):

- a method `func (c *catalog) addYourChallenge()`, called from the section's `registerXxx` method (or from `buildCatalog` for a new file); in it a function `design := func(fixed bool) engine.Diagram` using `(&bld{}).n(id, kind, col, row, P{...}).e(from, to).done()`; the start and the solution differ only in the fix;
- `c.register(&Scenario{ID, Title, Difficulty, ConfigOnly (true when the topology is fixed), Summary, Story, Diagram, Solution, Chaos, Goals, DurationSec, WarmupSec, Speed, Hints [3], Answer, Predictions, Debrief})`;
- `Deeper` is required for every Hard challenge (at least 200 characters).

Add the id to `levels.go` (sde1, sde2 or sde3 list) and `concepts.go` (existing concept ids only).

## 3. Measure, then set the goals

Start with `UPDATE_GOLDEN=1 go test ./internal/scenarios -run TestCatalogGolden` and read the new line in `testdata/catalog.golden`: `start=pass/stars/success/p95 solution=pass/stars/success/p95`. The start must fail; the reference must pass with at least two stars on seeds 1 to 3, with the same stars on seeds 1 to 8. Find the cost the reference actually has from `go test ./internal/scenarios -run 'TestReferenceSolutionsEarnTwoStars/<id>' -v` and set `MaxCostPerHour` a little above it and `StarCostPerHour` between the two. Star targets must be strictly stricter than the limits. A star bar that sits on the reference's own number flips with the seed; keep a margin.

## 4. Stored alternatives

Add `"<id>": { alt(label, passes, 0, 0, 0, "x", Change{node, key, value}), ... }` to `alternatives.go` and run `go test ./internal/scenarios -run Alternatives`. The failure message prints the measured success, p95 and cost: paste them in, then write the note so that it matches those numbers (no claim the numbers do not show, and no reason you have not verified; say what happened, not why, if you do not know why). A wrong lever that passes is allowed, but its `pass` flag must be true.

## 5. The gates you will meet

- three hints, an answer, a prediction whose options are within 1.6x in length (the explanation must be true);
- at most 24 challenges may lack a glossary term: use `{{id|words}}` in the story or debrief with an id that exists in `frontend/src/model/glossary.ts` (`p95` works anywhere), and add any new challenge id used by a glossary entry to `CHALLENGE_IDS` in `glossary.test.ts`;
- the Hard count is pinned in `debrief_test.go`: change it when you add a Hard challenge;
- the API test pins component kinds and the number of open briefs;
- regenerate the golden file last, after every text or number change;
- the README `Levels` row counts per band and the glossary count are checked by eye: update them.

## 6. Docs

Add a README feature row with the measured numbers (start result, reference result, the wrong levers) and, if a new setting was used, its row in `docs/COMPONENTS.md`.

## 7. Done when

`make check` passes, the numbers in the PR are the measured ones, and the wrong levers are listed. Then `ship-pr`.
