#!/usr/bin/env python3
"""The catalog bot's own push-time checks, run before it pushes to main.

    python3 tools/maintainer/bot_self_check.py --base <sha>
    python3 tools/maintainer/bot_self_check.py --self-test

catalog.yml marks its derived-files commits [skip ci], so CI never runs on
them. Most CI checks read the whole tree and still see the bot's output on the
next push. The ones that compare a push with its parent (github.event.before)
do not: the next push's `before` IS the bot commit, so the bot's own change
would never be compared with what it replaced. That is how a live-verified
badge was silently lost before (#248). So the bot runs exactly those checks
itself, on its commit (HEAD, checked out) against the commit it builds on
(--base), with the arguments ci.yml gives them, and does not push if any fails.

It also answers whether [skip ci] is allowed at all: only when the change-set
builds no connector row (release_matrix.py --changed-since, the same scoping
ci.yml's `prepare` job uses). A bot commit that stamps a sealed release builds
that connector's row today, so it keeps CI.

stdout is one line, `skip-ci: yes` or `skip-ci: no`; the checks' own output
goes to stderr. Exit 0 = every check passed, 1 = a check failed (do not push),
2 = usage or environment error.

--self-test proves the inventory guard both ways: every step of a
push-triggered workflow that reads github.event.before must run a checker
listed in CHECKS (or the matrix scoping, or a named exemption), so a new
parent-comparing check added to CI cannot silently miss bot commits.
"""
from __future__ import annotations

import json
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
TOOLS = "tools/maintainer"

# The checks ci.yml runs on a push with --base github.event.before, in the
# same form. Inventory and reasons: .review/inventory.md of the change that
# added this file.
CHECKS = {
    "check_registry_state": ["--base", "{base}"],
    "check_env_schema": ["--base", "{base}"],
    "check_press_metadata": ["--base", "{base}"],
    "check_pinned_artifacts": ["--base", "{base}", "--no-network"],
}
# Runs too, but decides [skip ci] rather than passing or failing.
MATRIX = "release_matrix"

# Steps that read github.event.before without running a checker, each with the
# reason a skipped bot commit cannot hide anything from it.
EXEMPT = {
    ("credkit.yml", "Did the kit itself change"):
        "push trigger is path-filtered to the kit, its vendoring tool, its workflow and "
        "skills/*/cli/go.mod; the bot writes none of those, so its commit never triggers credkit",
}

# check_env_schema.py takes about three minutes, and every merge waits for the
# catalog bot to finish (merge_when_green.py), so it runs only when the
# change-set touches something outside these paths. It reads skills.json, each
# connector's cli/ sources, manifest.json, server.json and mcp-install.md, the
# credential kit and its own rules file: none of these. Anything not listed
# here, an unreadable diff included, runs it (fails closed).
ENV_SCHEMA_BLIND = re.compile(
    r"^(docs/.*|catalog\.json|README\.md|skills/[^/]+/(README|CHANGELOG)\.md"
    r"|\.github/ISSUE_TEMPLATE/[^/]+\.yml)$")

CHECK_TIMEOUT = 900


def changed_files(base: str) -> list[str] | None:
    try:
        out = subprocess.run(["git", "diff", "--no-renames", "--name-only", "-z", base, "HEAD"],
                             cwd=ROOT, capture_output=True, text=True, timeout=120)
    except subprocess.TimeoutExpired:
        return None
    return [f for f in out.stdout.split("\0") if f] if out.returncode == 0 else None


def env_schema_blind(files: list[str] | None) -> bool:
    return files is not None and all(ENV_SCHEMA_BLIND.match(f) for f in files)


def run_checks(base: str) -> int:
    failed = []
    files = changed_files(base)
    for name, args in CHECKS.items():
        if name == "check_env_schema" and env_schema_blind(files):
            print("== bot self-check: check_env_schema.py not needed: the change-set touches only "
                  "paths it never reads", file=sys.stderr, flush=True)
            continue
        argv = [sys.executable, f"{TOOLS}/{name}.py"] + [a.format(base=base) for a in args]
        print(f"== bot self-check: {' '.join(argv[1:])}", file=sys.stderr, flush=True)
        try:
            rc = subprocess.run(argv, cwd=ROOT, stdout=sys.stderr, timeout=CHECK_TIMEOUT).returncode
        except subprocess.TimeoutExpired:
            rc = 124
        if rc != 0:
            failed.append(f"{name} (exit {rc})")
    try:
        out = subprocess.run([sys.executable, f"{TOOLS}/{MATRIX}.py", "--skills-only", "--changed-since", base],
                             cwd=ROOT, capture_output=True, text=True, timeout=CHECK_TIMEOUT)
        rows = [s["name"] for s in json.loads(out.stdout)["skill"]] if out.returncode == 0 else None
    except (subprocess.TimeoutExpired, ValueError, KeyError, TypeError):
        rows = None
    if rows is None:
        # release_matrix.py itself fails open to the full matrix; so does this.
        print("== bot self-check: build scope unknown, CI must run", file=sys.stderr)
    else:
        print(f"== bot self-check: build rows for this change-set: {rows or 'none'}", file=sys.stderr)
    if failed:
        print("::error::the bot's commit fails a check that compares a push with its parent: "
              + ", ".join(failed) + ". Not pushing.", file=sys.stderr)
        return 1
    print("skip-ci: " + ("yes" if rows == [] else "no"))
    return 0


def _push_triggered(text: str) -> bool:
    on = re.search(r"^on:\n((?:[ #].*\n|\n)*)", text, re.M)
    return bool(on and re.search(r"^  push:", on.group(1), re.M))


def uncovered(workflows: dict[str, str]) -> list[str]:
    """Steps of push-triggered workflows that read github.event.before but run
    no checker this tool runs and are not exempt."""
    known = set(CHECKS) | {MATRIX}
    bad = []
    for fname, text in sorted(workflows.items()):
        if not _push_triggered(text):
            continue
        for step in re.split(r"^(?=\s*- name:|  \S)", text, flags=re.M):
            code = "\n".join(ln for ln in step.splitlines() if not ln.lstrip().startswith("#"))
            if "github.event.before" not in code:
                continue
            m = re.search(r"- name:\s*(.+)", step)
            title = m.group(1).strip() if m else "(job header)"
            used = set(re.findall(rf"{TOOLS}/([\w-]+)\.py", code))
            if (fname, title) in EXEMPT or used & known:
                continue
            bad.append(f"{fname}: {title} (runs {sorted(used) or 'no checker'})")
    return bad


def _self_test() -> int:
    wf = {p.name: p.read_text(encoding="utf-8") for p in sorted((ROOT / ".github/workflows").glob("*.yml"))}
    fails = []
    got = uncovered(wf)
    print(f"  [{'ok' if not got else 'FAIL'}] every parent-comparing step in today's workflows is covered")
    if got:
        fails.append("uncovered: " + "; ".join(got))
    rogue = dict(wf)
    rogue["ci.yml"] = wf["ci.yml"] + (
        "      - name: A new parent-comparing gate\n"
        "        env:\n"
        "          BASE: ${{ github.event.before }}\n"
        "        run: python3 tools/maintainer/check_new_thing.py --base \"$BASE\"\n")
    got = uncovered(rogue)
    hit = any("A new parent-comparing gate" in g for g in got)
    print(f"  [{'ok' if hit else 'FAIL'}] a new step comparing with github.event.before is flagged")
    if not hit:
        fails.append("a new github.event.before step was not flagged")
    quiet = dict(wf)
    quiet["ci.yml"] = wf["ci.yml"] + "      # BASE: ${{ github.event.before }} in a comment only\n"
    calm = uncovered(quiet) == uncovered(wf)
    if not calm:
        fails.append("a comment mentioning github.event.before was flagged")
    print(f"  [{'ok' if calm else 'FAIL'}] a comment mentioning it is not a step")
    nopush = {"x.yml": "on:\n  pull_request:\njobs:\n  a:\n    steps:\n      - name: s\n"
                       "        run: echo ${{ github.event.before }}\n"}
    pushy = {"x.yml": nopush["x.yml"].replace("  pull_request:\n", "  push:\n    branches: [main]\n")}
    ok = not uncovered(nopush) and uncovered(pushy)
    print(f"  [{'ok' if ok else 'FAIL'}] only push-triggered workflows are held to it")
    if not ok:
        fails.append("push-trigger detection is wrong")
    blind = ["docs/_data/pending.json", "README.md", "catalog.json", "skills/hudu/README.md",
             "skills/hudu/CHANGELOG.md", "docs/skills/hudu.md", ".github/ISSUE_TEMPLATE/it-works.yml"]
    seen = ["skills/hudu/mcp-install.md", "skills/hudu/manifest.json", "skills/hudu/server.json",
            "tools/maintainer/skills.json", "tools/maintainer/env_schema_internal.json",
            "skills/hudu/cli/main.go", "skills/hudu/docs/README.md"]
    ok = env_schema_blind(blind) and not env_schema_blind(None) and not any(
        env_schema_blind(blind + [f]) for f in seen)
    print(f"  [{'ok' if ok else 'FAIL'}] env schema is skipped only for derived-only change-sets")
    if not ok:
        fails.append("ENV_SCHEMA_BLIND skips a path the env schema reads, or an unknown diff")
    for name in CHECKS:
        if not (ROOT / TOOLS / f"{name}.py").is_file():
            fails.append(f"{name}.py does not exist")
    if fails:
        print("bot_self_check self-test FAILED:\n  - " + "\n  - ".join(fails), file=sys.stderr)
        return 1
    print("bot_self_check self-test passed")
    return 0


def main(argv: list[str]) -> int:
    if argv == ["--self-test"]:
        return _self_test()
    if len(argv) != 2 or argv[0] != "--base" or not argv[1]:
        print("usage: bot_self_check.py --base <sha> | --self-test", file=sys.stderr)
        return 2
    return run_checks(argv[1])


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
