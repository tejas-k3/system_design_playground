---
name: ship-pr
description: Ship a finished change in this repo end to end - full local check, local code review, PR, CI watch, merge, ticket comment and local cleanup. Use when a branch's work is done and should become a merged PR, or when asked to "ship", "raise a PR" or "merge" a change.
---

# Ship a change

Run these in order and stop at the first failure. Never skip a step or loosen a test to get past it. Do not push to `main`; a hook refuses it.

## 0. Before you start (any time you begin a branch)

- `git worktree list`: pick a free name for `../sdp-<topic>` (an old worktree may own it) and branch `work/<ticket>-<topic>`.
- Create it with `git worktree add ../sdp-<topic> -b work/<ticket>-<topic> origin/main`, then `ln -s <main repo>/frontend/node_modules ../sdp-<topic>/frontend/node_modules`. The symlink must never be staged (a hook refuses it).
- Test first: the failing test, seen failing for the right reason, comes before the code.

## 1. Full local check

`make check` (markdown allow-list, tests, lint, perf guard, frontend build; stops at the first failure. The browser checks run in CI only). It takes a few minutes, so start it with `run_in_background` and wait for its notification, or run `make test`, `make lint`, `make perf-guard` and `cd frontend && npm run build` as separate commands joined with `&&`, never `;`.

If the perf guard moved, say so in the PR (ceilings: 125 MB and 2.0M allocations; the engine sits near 115 MB and 1.86M). If a golden file changed on purpose, regenerate it with `UPDATE_GOLDEN=1 go test ./internal/scenarios -run TestCatalogGolden` and read the diff.

## 2. Commit

- Stage by path (`git add backend frontend/src docs/COMPONENTS.md README.md`), never the bare `docs` directory (it holds local review tools), never `-f`, never `node_modules` or `docs/review/`.
- Subject under about 72 characters, a body that says why and what was measured, and the session's attribution trailer.
- The pre-commit hook checks the markdown allow-list, a staged `node_modules` and gofmt.

## 3. Local code review before the PR is raised

Run the `code-review` skill on this worktree, passing the path explicitly (a review started from another shell directory reviews that directory's diff):

`/code-review medium <the worktree's absolute path>` (from `git rev-parse --show-toplevel`)

Triage every finding: fix the real ones, answer the others in the PR description. Typical real ones here: a default that changed, a random draw added to a default path, prose or alternatives text that contradicts the measured numbers, a bypass (a replica, a second route) around a new cost, docs or README not updated. After fixing, go back to step 1.

## 4. Push and open the PR

`git push -u origin <branch>`, then `gh pr create` with: what changed, what was measured, what was not tested, what stays open. End the body with the session's PR attribution line.

## 5. Wait for CI, then merge

Watch with a Monitor (`gh pr checks N` until nothing is pending), not with sleeps. If main moved, `git fetch` and merge `origin/main` into the branch, re-run step 1 and push again. When every check is green: `gh pr merge N --merge` (merge commit, no squash). Confirm with `gh pr view N --json state`.

## 6. Close the loop on the ticket

Comment on the issue with what was built and the measured numbers; close it if nothing remains. Say plainly what was deliberately not built.

## 7. Local cleanup

From the main checkout (not the worktree you are removing):

- `git checkout main && git pull --ff-only origin main`
- `git worktree remove --force ../sdp-<topic>` (it holds only the node_modules symlink and the merged branch)
- `git branch -d <branch>` (use `-D` only if the PR is merged but git cannot tell)
- `git remote prune origin`; `git worktree prune`
- delete scratch files you made in the job's tmp directory

Leave the remote branch alone unless the owner asks.

## Hooks that run along the way

- Claude Code (`.claude/settings.json`): gofmt after every Go edit; refusal of force pushes, pushes to main, `git add -f`, staging `node_modules` or `docs/review/`, and `git worktree add` onto an existing path.
- Git (`make hooks` once per clone): pre-commit (markdown allow-list, staged `node_modules`, gofmt) and pre-push (refuses `main`).

## Report

End with what was merged (PR number), the measured result, what is still open, and anything the owner has to do (for example the 900x700 browser check).
