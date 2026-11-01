#!/usr/bin/env python3
"""PreToolUse (Bash): refuse the git commands that have gone wrong in this repo.

Reads the hook JSON on stdin, splits the command into simple commands (so `a && b` is judged per command, in the
directory a preceding `cd` moved to) and inspects every `git` call. Exit 2 blocks the call and shows the message."""
import json, os, shlex, subprocess, sys

def block(msg):
    print("guard-bash: " + msg, file=sys.stderr)
    sys.exit(2)

def segments(cmd):
    lex = shlex.shlex(cmd, posix=True, punctuation_chars=True)
    lex.whitespace_split = True
    seg, out = [], []
    try:
        for tok in lex:
            if tok in (";", "&&", "||", "|", "&", "\n"):
                if seg: out.append(seg)
                seg = []
            else:
                seg.append(tok)
    except ValueError:
        return []  # unparseable (an unclosed quote): let the shell report it
    if seg: out.append(seg)
    return out

def short_flags(arg):
    return arg[1:] if arg.startswith("-") and not arg.startswith("--") else ""

def branch_of(cwd):
    r = subprocess.run(["git", "-C", cwd, "rev-parse", "--abbrev-ref", "HEAD"], capture_output=True, text=True)
    return r.stdout.strip()

def check_git(args, cwd):
    # skip `git -C dir` and other global options
    i = 0
    while i < len(args) and args[i].startswith("-"):
        i += 2 if args[i] in ("-C", "-c") else 1
        if args[i - 2:i - 1] == ["-C"]: cwd = os.path.join(cwd, args[i - 1])
    if i >= len(args): return
    sub, rest = args[i], args[i + 1:]
    if sub == "push":
        for a in rest:
            if a.startswith("--force") or a == "--delete" or "f" in short_flags(a) or (not a.startswith("-") and a.startswith("+")):
                block("no force pushes (a '+' refspec, -f and --force are all force)")
        refs = [a for a in rest if not a.startswith("-")]
        if any(r in ("main", "HEAD:main") or r.endswith(":main") or r.endswith(":refs/heads/main") for r in refs):
            block("no direct pushes to main: open a PR")
        if len(refs) < 2 and branch_of(cwd) == "main":
            block("you are on main: push a branch and open a PR")
    elif sub == "add":
        paths = []
        after_dd = False
        for a in rest:
            if a == "--" and not after_dd: after_dd = True; continue
            if not after_dd and (a == "--force" or "f" in short_flags(a)):
                block("git add -f is not allowed (it bypasses the markdown and ignore rules)")
            if after_dd or not a.startswith("-"): paths.append(a)
        for p in paths:
            if p.startswith(":!") or p.startswith(":(exclude)"): continue
            if "node_modules" in p or "docs/review" in p or p in ("docs", "docs/"):
                block("never stage frontend/node_modules (the worktree symlink) or docs/review (local notes); name the files instead of 'docs'")
    elif sub == "worktree" and rest[:1] == ["add"]:
        pos, k = [], 1
        while k < len(rest):
            a = rest[k]
            if a in ("-b", "-B", "--reason"): k += 2; continue
            if a.startswith("-"): k += 1; continue
            pos.append(a); k += 1
        if pos and os.path.exists(os.path.join(cwd, pos[0])):
            block(pos[0] + " already exists: run 'git worktree list' and pick a free name")

def main():
    try:
        cmd = json.load(sys.stdin).get("tool_input", {}).get("command", "")
    except Exception:
        return
    cwd = os.getcwd()
    for seg in segments(cmd):
        while seg and "=" in seg[0] and not seg[0].startswith("-") and seg[0].split("=")[0].isidentifier():
            seg = seg[1:]  # VAR=value prefixes
        if not seg: continue
        if seg[0] == "cd" and len(seg) > 1:
            cwd = os.path.normpath(os.path.join(cwd, os.path.expanduser(seg[1])))
        elif seg[0] == "git":
            check_git(seg[1:], cwd)

main()
