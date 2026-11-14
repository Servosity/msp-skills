#!/usr/bin/env python3
"""baseline_age.py - list every CI gate baseline entry that has sat unowned for too long.

A gate baseline lists known gaps the gate REPORTS instead of blocking. That is a
burn-down list, and a burn-down list nobody owns stops burning down: on
2026-10-23 a gate baselined 19 connectors "to adopt the fix later", and three
weeks later users reported the gap as issues #687 to #690. This tool names every
entry older than --days so the gap is owned again. Run weekly by
.github/workflows/baseline-age.yml, which keeps ONE issue with the list.
Advisory only: it exits 0 whatever it finds and gates nothing.

Baselines read (all under tools/maintainer/):
  security_gate_baseline.json  rules        -> {rule: [connector, ...]}
  verify_baseline.json         connectors   -> {connector: [check, ...]}
                               known_errors -> {connector: [check, ...]}
                                 (harness ERRORs the gate reports instead of
                                 failing; shrink-only like `connectors`, so it is
                                 a baseline and is read too)
  class_baseline.json          classes      -> {class: [connector, ...]}
Not read: security_suppressions.json (each entry is a deliberate, reasoned
suppression, not a burn-down list).

An entry's age runs from the start of its CURRENT continuous run of presence in
the file on the first-parent history of --ref: the walk goes back through every
commit that changed the file and stops at the first one without the entry. An
entry removed and later re-added ages from the re-add. Dates are committer dates
(when the change landed), ages are whole days before --today (UTC).

    python3 -I tools/maintainer/baseline_age.py                      # markdown report
    python3 -I tools/maintainer/baseline_age.py --days 14 --out body.md --status status.json
    python3 -I tools/maintainer/baseline_age.py --self-test

--status writes {"old": N, "complete": true|false}. complete is false when the
evidence could not be read whole (a shallow clone, a baseline unreadable at
--ref, a name outside the expected charset); the report says why, and the
workflow then never closes the issue on it. Stdlib only.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
MARKER = "<!-- baseline-age -->"
# (path, key, shape): "group->slugs" maps a rule or class to connectors,
# "slug->groups" maps a connector to the checks it fails.
BASELINES = (
    ("tools/maintainer/security_gate_baseline.json", "rules", "group->slugs"),
    ("tools/maintainer/verify_baseline.json", "connectors", "slug->groups"),
    ("tools/maintainer/verify_baseline.json", "known_errors", "slug->groups"),
    ("tools/maintainer/class_baseline.json", "classes", "group->slugs"),
)
NAME = re.compile(r"[a-z0-9][a-z0-9._-]{0,99}")  # always fullmatch: "$" admits a trailing newline
SHA = re.compile(r"[0-9a-f]{40}|[0-9a-f]{64}")
DATE = re.compile(r"[0-9]{4}-[0-9]{2}-[0-9]{2}")
PR_REF = re.compile(r"\(#([0-9]{1,7})\)\s*$")


class Unreadable(Exception):
    pass


def git(repo: Path, *args: str, check: bool = True) -> subprocess.CompletedProcess:
    r = subprocess.run(["git", "-C", str(repo), *args], capture_output=True, text=True, encoding="utf-8",
                       errors="replace", timeout=120)
    if check and r.returncode != 0:
        raise Unreadable(f"git {' '.join(args[:2])} failed: {r.stderr.strip()[:200]}")
    return r


def entries(text: str, key: str, shape: str) -> set[tuple[str, str]]:
    """The (group, connector) pairs under data[key]. Raises Unreadable on a bad shape."""
    try:
        data = json.loads(text)
    except ValueError:
        raise Unreadable("not JSON") from None
    if not isinstance(data, dict):
        raise Unreadable("top level is not an object")
    block = data.get(key, {})
    if not isinstance(block, dict):
        raise Unreadable(f"'{key}' is not an object")
    out: set[tuple[str, str]] = set()
    for k, v in block.items():
        if not isinstance(v, list) or not all(isinstance(x, str) for x in v):
            # The key is repository text: never echo it (only names that passed NAME reach the report).
            raise Unreadable(f"'{key}' holds a value that is not a list of names")
        for x in v:
            out.add((k, x) if shape == "group->slugs" else (x, k))
    return out


def file_at(repo: Path, rev: str, path: str) -> str | None:
    """The file at rev; None ONLY when rev exists and has no such file (deleted).
    Any other failure raises Unreadable, so a git error can never look like a removal."""
    listing = git(repo, "ls-tree", "-z", "--full-tree", rev, "--", path).stdout
    if not listing:
        return None
    return git(repo, "cat-file", "blob", f"{rev}:{path}").stdout


def history(repo: Path, ref: str, path: str) -> list[tuple[str, str, str]]:
    """(sha, committer date, PR number or "") of every first-parent commit that changed path, newest first."""
    # -z: records end in NUL, which no commit message can hold, so a subject cannot
    # forge a record (a newline, U+2028 or a tab in it stays inside its own field).
    out = git(repo, "log", "-z", "--first-parent", "--format=%H%x09%cs%x09%s", ref, "--", path).stdout
    rows = []
    for rec in out.split("\0"):
        if not rec:
            continue
        sha, date, subject = (rec.split("\t", 2) + ["", ""])[:3]
        if not (SHA.fullmatch(sha) and DATE.fullmatch(date)):
            raise Unreadable(f"unexpected git log record for {path}")
        m = PR_REF.search(subject)
        rows.append((sha, date, m.group(1) if m else ""))
    return rows


def ages(repo: Path, ref: str = "HEAD") -> tuple[list[dict], list[str]]:
    """Every current baseline entry with the commit its current run of presence started at,
    plus problems that make the result incomplete."""
    problems: list[str] = []
    if git(repo, "rev-parse", "--is-shallow-repository", check=False).stdout.strip() == "true":
        problems.append("the checkout is shallow, so every entry looks as young as the oldest fetched "
                        "commit (check out with fetch-depth 0)")
    rows: list[dict] = []
    cache: dict[tuple[str, str], str | None] = {}
    for path, key, shape in BASELINES:
        text = file_at(repo, ref, path)
        if text is None:
            problems.append(f"`{path}` is missing at {ref}")
            continue
        try:
            current = entries(text, key, shape)
        except Unreadable as e:
            problems.append(f"`{path}` is unreadable at {ref}: {e}")
            continue
        hist = history(repo, ref, path)
        versions: list[set[tuple[str, str]] | None] = []
        for sha, _, _ in hist:
            if (sha, path) not in cache:
                cache[(sha, path)] = file_at(repo, sha, path)
            body = cache[(sha, path)]
            if body is None:
                versions.append(set())  # the file did not exist after this commit
                continue
            try:
                versions.append(entries(body, key, shape))
            except Unreadable:
                versions.append(None)  # skipped: neither breaks nor extends a run
        for group, slug in sorted(current):
            if not (NAME.fullmatch(group) and NAME.fullmatch(slug)):
                problems.append(f"`{path}` `{key}` holds a name outside [a-z0-9._-]; not listed")
                continue
            start = None
            for (sha, date, pr), have in zip(hist, versions):
                if have is None:
                    continue
                if (group, slug) not in have:
                    break
                start = (sha, date, pr)
            if start is None:
                problems.append(f"`{path}`: no commit on {ref} shows `{group}` `{slug}`")
                continue
            rows.append({"file": path.rsplit("/", 1)[-1], "key": key, "group": group, "slug": slug,
                         "sha": start[0], "date": start[1], "pr": start[2]})
    return rows, problems


def report(rows: list[dict], problems: list[str], days: int, today: dt.date, ref_sha: str) -> tuple[str, int]:
    old = []
    for r in rows:
        age = (today - dt.date.fromisoformat(r["date"])).days
        if age > days:
            old.append({**r, "age": age})
    old.sort(key=lambda r: (-r["age"], r["file"], r["key"], r["group"], r["slug"]))
    lines = [MARKER]
    if old:
        lines += [f"{len(old)} gate baseline entries are older than {days} days as of {today} "
                  f"(main at {ref_sha[:12]}).",
                  "",
                  "Each entry is a known gap a CI gate reports instead of blocking. Fix it and remove the "
                  "entry, or name who owns it here. Written by `tools/maintainer/baseline_age.py` "
                  "(weekly, `baseline-age.yml`); advisory, gates nothing.",
                  "",
                  "| File | Rule or check | Connector | Added | Age (days) | Commit | PR |",
                  "| --- | --- | --- | --- | --- | --- | --- |"]
        for r in old:
            group = r["group"] if r["key"] in ("rules", "connectors", "classes") else f"{r['key']}: {r['group']}"
            lines.append(f"| `{r['file']}` | `{group}` | `{r['slug']}` | {r['date']} | {r['age']} | "
                         f"{r['sha'][:12]} | {'#' + r['pr'] if r['pr'] else ''} |")
    else:
        lines.append(f"No gate baseline entry is older than {days} days as of {today} (main at {ref_sha[:12]}).")
    if problems:
        lines += ["", "**Incomplete evidence** (this list may be missing entries):", ""]
        lines += [f"- {p}" for p in dict.fromkeys(problems)]
    return "\n".join(lines) + "\n", len(old)


def run(repo: Path, ref: str, days: int, today: dt.date) -> tuple[str, int, bool]:
    try:
        ref_sha = git(repo, "rev-parse", "--verify", f"{ref}^{{commit}}").stdout.strip()
        rows, problems = ages(repo, ref)
    except (Unreadable, OSError, subprocess.SubprocessError) as e:
        ref_sha, rows, problems = "unknown", [], [f"could not read the history: {e}"]
    body, n = report(rows, problems, days, today, ref_sha)
    return body, n, not problems


# ------------------------------------------------------------- self-test ----

def self_test() -> int:
    fails = 0

    def expect(ok: bool, what: str) -> None:
        nonlocal fails
        print(f"  [{'PASS' if ok else 'FAIL'}] {what}")
        fails += not ok

    tmp = Path(tempfile.mkdtemp(prefix="baseline-age-selftest-"))
    env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"), "HOME": str(tmp), "GIT_CONFIG_NOSYSTEM": "1",
           "GIT_CONFIG_GLOBAL": os.devnull, "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.invalid",
           "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.invalid", "GIT_TERMINAL_PROMPT": "0"}
    repo = tmp / "repo"
    repo.mkdir()

    def g(*a: str, date: str | None = None, cwd: Path = repo) -> str:
        e = dict(env)
        if date:
            e["GIT_AUTHOR_DATE"] = e["GIT_COMMITTER_DATE"] = f"{date}T12:00:00+0000"
        return subprocess.run(["git", *a], cwd=str(cwd), env=e, capture_output=True, text=True,
                              check=True, timeout=60).stdout.strip()

    def write(rules: dict, connectors: dict | None = None, known: dict | None = None,
              classes: dict | None = None) -> None:
        d = repo / "tools/maintainer"
        d.mkdir(parents=True, exist_ok=True)
        (d / "security_gate_baseline.json").write_text(json.dumps({"_comment": "x", "rules": rules}, indent=1))
        (d / "verify_baseline.json").write_text(json.dumps(
            {"_comment": "x", "connectors": connectors or {}, "known_errors": known or {}}, indent=1))
        (d / "class_baseline.json").write_text(json.dumps({"_comment": "x", "classes": classes or {}}, indent=1))

    def commit(date: str, msg: str) -> str:
        g("add", "-A")
        g("commit", "-q", "-m", msg, date=date)
        return g("rev-parse", "HEAD")

    try:
        g("init", "-q", "-b", "main")
        write({"r1": ["old", "gone"]}, {"vc": ["mcp-http"]}, {"ke": ["install"]}, {"c1": ["cl"]})
        c_seed = commit("2026-01-01", "seed (#11)")
        write({"r1": ["old"]}, {"vc": ["mcp-http"]}, {"ke": ["install"]}, {"c1": ["cl"]})
        commit("2026-01-10", "fix gone (#12)")
        (repo / "unrelated.txt").write_text("x")
        commit("2026-01-12", "unrelated")
        write({"r1": ["old", "gone"]}, {"vc": ["mcp-http"]}, {"ke": ["install"]}, {"c1": ["cl"]})
        c_readd = commit("2026-02-01", "re-add gone (#13)")
        write({"r1": ["old", "gone"], "r2": ["edge", "edge2"]}, {"vc": ["mcp-http"]}, {"ke": ["install"]},
              {"c1": ["cl"]})
        commit("2026-02-14", "edge entries")
        write({"r1": ["old", "gone"], "r2": ["edge", "edge2", "young"]},
              {"vc": ["mcp-http", "local-data"]}, {"ke": ["install"]}, {"c1": ["cl"]})
        commit("2026-02-25", "young entries")
        (repo / "tools/maintainer/security_gate_baseline.json").write_text("{ not json")
        commit("2026-02-26", "broken baseline")
        write({"r1": ["old", "gone"], "r2": ["edge", "edge2", "young"]},
              {"vc": ["mcp-http", "local-data"]}, {"ke": ["install"]}, {"c1": ["cl"]})
        commit("2026-02-27", "repaired baseline")
        # A side branch written long ago that lands on main late: it ages from the merge.
        g("checkout", "-q", "-b", "side")
        sec = repo / "tools/maintainer/security_gate_baseline.json"
        data = json.loads(sec.read_text())
        data["rules"]["r3"] = ["merged"]
        sec.write_text(json.dumps(data, indent=1))
        commit("2026-01-20", "side branch")
        g("checkout", "-q", "main")
        g("merge", "-q", "--no-ff", "-m", "merge side (#14)", "side", date="2026-02-28")
        # A subject holding U+2028, a tab and a forged record must stay one record.
        data = json.loads(sec.read_text())
        data["_comment"] = "y"
        sec.write_text(json.dumps(data, indent=1))
        commit("2026-02-28", "comment only\u2028" + "0" * 40 + "\t2026-02-28\tforged")

        rows, problems = ages(repo)
        by = {(r["key"], r["group"], r["slug"]): r for r in rows}
        expect(not problems, f"a full clone with readable baselines is complete (problems: {problems})")
        expect(by[("rules", "r1", "old")]["date"] == "2026-01-01" and by[("rules", "r1", "old")]["sha"] == c_seed
               and by[("rules", "r1", "old")]["pr"] == "11",
               "an entry present since the first commit ages from it, with its commit and PR")
        expect(by[("rules", "r1", "gone")]["date"] == "2026-02-01" and by[("rules", "r1", "gone")]["sha"] == c_readd,
               "a removed then re-added entry ages from the re-add, not the first add")
        expect(by[("rules", "r2", "young")]["date"] == "2026-02-25", "a young entry dates from its add")
        expect(by[("rules", "r3", "merged")]["date"] == "2026-02-28" and by[("rules", "r3", "merged")]["pr"] == "14",
               "an entry from a merged side branch ages from when it landed on main, not when it was written")
        expect(by[("connectors", "mcp-http", "vc")]["date"] == "2026-01-01"
               and by[("connectors", "local-data", "vc")]["date"] == "2026-02-25",
               "verify_baseline connectors are read per (check, connector)")
        expect(by[("known_errors", "install", "ke")]["date"] == "2026-01-01", "verify_baseline known_errors is read")
        expect(by[("classes", "c1", "cl")]["date"] == "2026-01-01", "class_baseline is read")
        expect(by[("rules", "r1", "old")]["date"] == "2026-01-01",
               "an unparseable historical version neither breaks nor restarts a run")
        expect(by[("rules", "r1", "old")]["date"] == "2026-01-01" and not problems,
               "a commit subject holding U+2028, a tab and a forged record cannot cut a run short")

        today = dt.date(2026, 3, 1)
        body, n, complete = run(repo, "HEAD", 14, today)
        listed = {line.split("|")[3].strip(" `") + "/" + line.split("|")[2].strip(" `")
                  for line in body.splitlines() if line.startswith("| `")}
        expect(complete and n == len(listed), f"the report counts what it lists ({n})")
        expect({"old/r1", "gone/r1", "cl/c1", "vc/mcp-http", "ke/known_errors: install", "edge/r2"} <= listed,
               "entries older than 14 days are listed (59, 28 and 15 days old)")
        expect("young/r2" not in listed and "vc/local-data" not in listed,
               "an entry 4 days old is not listed")
        _, n15, _ = run(repo, "HEAD", 15, today)
        expect(n15 == n - 2, "an entry exactly --days old is not listed (strictly older)")
        expect("| 2026-02-01 | 28 |" in body and "#13" in body, "the re-added entry shows its re-add date and PR")
        empty, n0, ok0 = run(repo, "HEAD", 400, today)
        expect(n0 == 0 and ok0 and "No gate baseline entry is older than 400 days" in empty,
               "nothing old gives the all-clear line and zero")

        write({"r1": ["old", "Bad Name", "old\n"]}, {}, {}, {})
        commit("2026-02-28", "bad name")
        rows, _ = ages(repo)
        expect({r["slug"] for r in rows if r["group"] == "r1"} == {"old"},
               "a name with a trailing newline or a space is never a row (fullmatch, not match)")
        body, n, complete = run(repo, "HEAD", 14, today)
        expect(not complete and "Bad Name" not in body and "outside [a-z0-9._-]" in body
               and "old\n`" not in body and body.count("| `old` |") == 1,
               "a name outside the charset (a space, a trailing newline) is withheld and makes the run incomplete")
        write({"r1": ["old"]}, {}, {}, {"@everyone": False})
        commit("2026-02-28", "malformed key")
        body, n, complete = run(repo, "HEAD", 14, today)
        expect(not complete and "@everyone" not in body and "not a list of names" in body,
               "a malformed baseline is reported without echoing its repository-controlled key")
        (repo / "tools/maintainer/verify_baseline.json").write_text("[]")
        commit("2026-02-28", "broken verify baseline at HEAD")
        body, n, complete = run(repo, "HEAD", 14, today)
        expect(not complete and "verify_baseline.json` is unreadable" in body and "| `old` |" in body
               and "`vc`" not in body, "a baseline unreadable at the ref is reported, the others still listed")

        shallow = tmp / "shallow"
        g("clone", "-q", "--depth", "1", f"file://{repo}", str(shallow), cwd=tmp)
        body, n, complete = run(shallow, "HEAD", 14, today)
        expect(not complete and "shallow" in body, "a shallow clone is reported as incomplete evidence")
    except subprocess.CalledProcessError as e:
        expect(False, f"fixture git failed: {e.stderr}")
    except (Unreadable, KeyError) as e:
        expect(False, f"the fixture history could not be read: {e!r}")
    finally:
        subprocess.run(["rm", "-rf", str(tmp)], check=False)
    print(f"baseline_age self-test: {'PASS' if not fails else f'FAIL ({fails})'}")
    return 1 if fails else 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--days", type=int, default=14, help="list entries strictly older than this (default 14)")
    ap.add_argument("--ref", default="HEAD", help="the commit whose baselines are read (default HEAD)")
    ap.add_argument("--repo", default=str(ROOT), help="the repository (default: this one)")
    ap.add_argument("--today", help="YYYY-MM-DD (default: today, UTC)")
    ap.add_argument("--out", help="write the markdown report here instead of stdout")
    ap.add_argument("--status", help='write {"old": N, "complete": bool} here')
    ap.add_argument("--self-test", action="store_true")
    a = ap.parse_args()
    if a.self_test:
        return self_test()
    today = dt.date.fromisoformat(a.today) if a.today else dt.datetime.now(dt.timezone.utc).date()
    body, n, complete = run(Path(a.repo), a.ref, a.days, today)
    if a.out:
        Path(a.out).write_text(body, encoding="utf-8")
    else:
        sys.stdout.write(body)
    if a.status:
        Path(a.status).write_text(json.dumps({"old": n, "complete": complete}) + "\n", encoding="utf-8")
    return 0  # advisory: never gates


if __name__ == "__main__":
    sys.exit(main())
