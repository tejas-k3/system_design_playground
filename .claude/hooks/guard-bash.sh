#!/bin/sh
# PreToolUse (Bash): refuse the commands that have gone wrong in this repo. Exit 2 blocks the call and shows the message.
cmd=$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("tool_input",{}).get("command",""))' 2>/dev/null)
block() { echo "guard-bash: $1" >&2; exit 2; }

case "$cmd" in
  *"git push"*)
    branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null)
    case "$cmd" in
      *" --force"*|*" -f "*|*" -f"|*"--force-with-lease"*) block "no force pushes" ;;
      *"origin main"*|*":main"*|*"HEAD:main"*) block "no direct pushes to main: open a PR" ;;
    esac
    [ "$branch" = "main" ] && block "you are on main: push a branch and open a PR"
    ;;
esac

case "$cmd" in
  *"git add"*)
    case "$cmd" in
      *" -f "*|*" --force"*) block "git add -f is not allowed (it bypasses the markdown and ignore rules)" ;;
    esac
    # a path that names node_modules or docs/review, other than as an exclusion pathspec (':!...')
    for word in $cmd; do
      case "$word" in
        ":!"*|":(exclude)"*|"'"":!"*|"\":!"*) continue ;;
        *node_modules*|*docs/review*) block "never stage frontend/node_modules (the worktree symlink) or docs/review (local notes)" ;;
      esac
    done
    ;;
esac

case "$cmd" in
  *"git worktree add"*)
    # the path is the first argument after "add" that does not start with a dash
    path=$(printf '%s' "$cmd" | python3 -c 'import sys,shlex; a=shlex.split(sys.stdin.read()); i=a.index("add"); print(next((x for x in a[i+1:] if not x.startswith("-")),""))' 2>/dev/null)
    [ -n "$path" ] && [ -e "$path" ] && block "$path already exists: run 'git worktree list' and pick a free name"
    ;;
esac
exit 0
