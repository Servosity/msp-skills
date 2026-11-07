#!/usr/bin/env python3
"""vuln_watch.py - catch a newly published Go vulnerability in any connector within a day.

Run daily by .github/workflows/vuln-watch.yml. Advisory only: it opens or edits
GitHub issues and never gates or changes code. Two halves:

  --scan    govulncheck (symbol scan, -format json) over every skills/*/cli module
            in this shard, once per release target (registry.TARGETS), one result
            file per connector, under the Go the module builds with. Any connector
            that cannot be scanned is an ERROR result, never a silent skip, and the
            run exits 1.
  --report  reads every result file, checks that EVERY connector the repository
            has was scanned cleanly (else exit 1, loudly, after the issues are
            written), and keeps ONE issue per vulnerability ID (label vuln-watch,
            title "vuln-watch: GO-YYYY-NNNN reachable in N connectors"). A new
            issue opens only when a vulnerable symbol is CALLED in at least one
            connector (the security gate's own bar); IDs that are only required or
            imported are listed in the run summary. The issue for an ID is found
            by its marker comment or the ID in its title, among issues that carry
            the label or that the workflow opened, in any state: an open one is
            edited in place when its text changed (and relabelled if the label was
            removed); a closed one is left closed and never duplicated. There is
            no closing logic: when an ID stops being reported its issue is left as
            it is, for a person to close.

    python3 -I tools/maintainer/vuln_watch.py --list
    python3 -I tools/maintainer/vuln_watch.py --scan --shard 0/8 --out DIR [--root TREE]
    python3 -I tools/maintainer/vuln_watch.py --report --results DIR --repo Servosity/msp-skills --dry-run
    python3 -I tools/maintainer/vuln_watch.py --report --results DIR --repo Servosity/msp-skills --apply
    python3 -I tools/maintainer/vuln_watch.py --self-test

Without --apply (or with --dry-run) nothing is written: each issue's title and body is printed.
Everything from govulncheck or the tree is untrusted text: IDs must match
^GO-[0-9]{4}-[0-9]+$, slugs, module paths and versions a strict charset, and the
issue text goes to GitHub as a JSON request body on stdin, never through a shell.
Stdlib only; GitHub calls go through the `gh` CLI (GH_TOKEN in CI).
"""

from __future__ import annotations

import argparse
import ast
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
LABEL = "vuln-watch"
LABEL_COLOR, LABEL_DESC = "b60205", "A Go vulnerability govulncheck reports in a connector (vuln-watch.yml)"


def _release_targets(path: Path | None = None) -> tuple[tuple[str, str], ...]:
    """Every (GOOS, GOARCH) the release ships: registry.TARGETS, the one definition
    release.yml builds from. First-party code carries *_windows.go and *_linux.go
    files and a file can be arch-only, so a scan of fewer targets could miss a call
    path that exists only in one shipped binary. Read as DATA (ast.literal_eval of
    the assignment), never imported, so nothing in registry.py runs here."""
    path = path or Path(__file__).resolve().parent / "registry.py"
    try:
        tree = ast.parse(path.read_text(encoding="utf-8"))
        vals = [n.value for n in tree.body if isinstance(n, ast.Assign)
                and any(isinstance(t, ast.Name) and t.id == "TARGETS" for t in n.targets)]
        raw = ast.literal_eval(vals[-1])
        out = tuple(sorted((t["goos"], t["goarch"]) for t in raw))
    except (OSError, SyntaxError, ValueError, IndexError, KeyError, TypeError) as e:
        raise SystemExit(f"cannot read TARGETS from {path.name}: {e!r}") from e
    if not out or not all(isinstance(x, str) and re.fullmatch(r"[a-z0-9]+", x) for t in out for x in t):
        raise SystemExit(f"TARGETS in {path.name} is empty or malformed")
    return out


# A vuln is reachable if ANY target calls it.
TARGETS = _release_targets()
SCAN_TIMEOUT = 900  # seconds per connector per target
ID_RE = re.compile(r"^GO-[0-9]{4}-[0-9]+$")
SLUG_RE = re.compile(r"^[a-z0-9][a-z0-9-]{0,63}$")
MODULE_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._~+/-]{0,199}$")
VERSION_RE = re.compile(r"^v?[0-9][0-9A-Za-z.+-]{0,99}$")
TITLE_ID_RE = re.compile(r"^vuln-watch: (GO-[0-9]{4}-[0-9]+)(?: |$)")
LEVELS = ("required", "imported", "called")  # ascending
WITHHELD = "(withheld: not a well-formed value)"


def marker(vid: str) -> str:
    return f"<!-- vuln-watch:{vid} -->"


# --------------------------------------------------------------------------- #
# Enumeration: every connector module, or an error naming the one that is not.
# --------------------------------------------------------------------------- #

def connectors(root: Path) -> tuple[list[str], list[str]]:
    """(slugs to scan, problems). A slug is a skills/<dir> with cli/go.mod. Every
    registry entry that is not markdown_only must have one, and a cli/ without a
    go.mod is a problem, so a connector can never drop out of the scan quietly."""
    problems: list[str] = []
    skills = root / "skills"
    slugs = set()
    for d in sorted(skills.iterdir()) if skills.is_dir() else ():
        if not d.is_dir() or not (d / "cli").exists():
            continue
        if not (d / "cli" / "go.mod").is_file():
            problems.append(f"skills/{d.name}/cli has no go.mod, so it cannot be scanned")
        elif not SLUG_RE.match(d.name):
            problems.append(f"skills/{d.name!r} is not a well-formed connector directory name")
        else:
            slugs.add(d.name)
    try:
        reg = json.loads((root / "tools" / "maintainer" / "skills.json").read_text(encoding="utf-8"))["skills"]
    except (OSError, ValueError, KeyError, TypeError) as e:
        problems.append(f"tools/maintainer/skills.json unreadable: {e!r}")
        reg = {}
    for slug, meta in sorted(reg.items()):
        if not isinstance(meta, dict) or meta.get("markdown_only"):
            continue
        d = str(meta.get("source_dir", slug))
        if d not in slugs and not any(p.startswith(f"skills/{d}/") or p.startswith(f"skills/{d!r}")
                                      for p in problems):
            problems.append(f"registry skill {slug!r} has no skills/{d}/cli/go.mod to scan")
    if not slugs:
        problems.append("no connector modules found under skills/*/cli")
    return sorted(slugs), problems


def shard_of(slugs: list[str], spec: str) -> list[str]:
    m = re.fullmatch(r"([0-9]+)/([1-9][0-9]*)", spec)
    if not m or int(m.group(1)) >= int(m.group(2)):
        raise ValueError(f"--shard wants I/N with 0 <= I < N, got {spec!r}")
    i, n = int(m.group(1)), int(m.group(2))
    return [s for k, s in enumerate(slugs) if k % n == i]


# --------------------------------------------------------------------------- #
# Scanning one module.
# --------------------------------------------------------------------------- #

def parse_stream(text: str) -> list[dict]:
    """govulncheck -format json is a stream of JSON objects. Raises ValueError."""
    dec, i, out = json.JSONDecoder(), 0, []
    while True:
        while i < len(text) and text[i] in " \t\r\n":
            i += 1
        if i >= len(text):
            return out
        obj, i = dec.raw_decode(text, i)
        if not isinstance(obj, dict):
            raise ValueError("a message that is not a JSON object")
        out.append(obj)


def findings_of(msgs: list[dict]) -> dict[str, dict]:
    """{id: {"level", "modules": {module: {"version", "fixed"}}}} from one scan.
    Raises ValueError on a stream that is not a complete govulncheck run."""
    if sum(1 for m in msgs if "config" in m) != 1:
        raise ValueError("no govulncheck config message: not a complete scan")
    out: dict[str, dict] = {}
    for m in msgs:
        f = m.get("finding")
        if f is None:
            continue
        if not isinstance(f, dict) or not isinstance(f.get("osv"), str):
            raise ValueError("a finding without an osv id")
        trace = f.get("trace")
        if not isinstance(trace, list) or not trace or not isinstance(trace[0], dict):
            raise ValueError(f"finding {f['osv'][:40]!r} has no trace")
        top = trace[0]
        level = "called" if top.get("function") else "imported" if top.get("package") else "required"
        v = out.setdefault(f["osv"], {"level": "required", "modules": {}})
        if LEVELS.index(level) > LEVELS.index(v["level"]):
            v["level"] = level
        mod = str(top.get("module") or "")
        v["modules"].setdefault(mod, {"version": str(top.get("version") or ""),
                                      "fixed": str(f.get("fixed_version") or "")})
    return out


def release_go(info: dict) -> str | None:
    """The Go that builds this module, from `go mod edit -json` (the go command's
    own go.mod parser, so comments and spacing cannot hide a directive): the
    larger of the go and toolchain directives, as goX.Y.Z. None when that cannot
    be established exactly (a minor-only go and no toolchain, or a toolchain that
    is not goX.Y.Z), and the caller then refuses to scan."""
    full = []
    go = info.get("Go")
    if isinstance(go, str) and re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", go):
        full.append(tuple(int(x) for x in go.split(".")))
    tc = info.get("Toolchain")
    if tc is not None:
        m = re.fullmatch(r"go([0-9]+)\.([0-9]+)\.([0-9]+)", tc) if isinstance(tc, str) else None
        if not m:
            return None
        full.append(tuple(int(x) for x in m.groups()))
    return "go" + ".".join(map(str, max(full))) if full else None


def gomod_info(mod_dir: Path) -> dict:
    """`go mod edit -json` of this module. Raises ValueError when it cannot be read."""
    try:
        r = subprocess.run(["go", "mod", "edit", "-json"], cwd=str(mod_dir), capture_output=True, text=True,
                           timeout=60, env={**os.environ, "GOTOOLCHAIN": "local"})
    except (OSError, subprocess.TimeoutExpired) as e:
        raise ValueError(f"go mod edit -json: {e}") from e
    if r.returncode != 0:
        raise ValueError(f"go mod edit -json exit {r.returncode}: {r.stderr.strip()[:200]}")
    try:
        info = json.loads(r.stdout)
    except ValueError as e:
        raise ValueError(f"go mod edit -json output unreadable: {e}") from e
    if not isinstance(info, dict):
        raise ValueError("go mod edit -json output is not an object")
    return info


def graph_roots(mod_dir: Path) -> list[str]:
    """`./...` plus every cmd/ dir by explicit path (check_security_gate.py's rule:
    `./...` skips names starting with _ or . and symlinked dirs)."""
    cmd = mod_dir / "cmd"
    return ["./..."] + sorted(f"./cmd/{d.name}" for d in (cmd.iterdir() if cmd.is_dir() else ()) if d.is_dir())


def run_govulncheck(mod_dir: Path, goos: str, goarch: str) -> tuple[int, str, str]:
    env = {**os.environ, "GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0", "GOTOOLCHAIN": "local"}
    try:
        r = subprocess.run(["govulncheck", "-scan", "symbol", "-format", "json", *graph_roots(mod_dir)],
                           cwd=str(mod_dir), env=env, capture_output=True, text=True, timeout=SCAN_TIMEOUT)
    except subprocess.TimeoutExpired:
        return 124, "", f"timed out after {SCAN_TIMEOUT}s"
    except OSError as e:
        return 127, "", f"cannot run govulncheck: {e}"
    return r.returncode, r.stdout, r.stderr


def scanner_go() -> str:
    try:
        r = subprocess.run(["go", "env", "GOVERSION"], capture_output=True, text=True, timeout=60,
                           env={**os.environ, "GOTOOLCHAIN": "local"})
        return r.stdout.strip() if r.returncode == 0 else ""
    except (OSError, subprocess.TimeoutExpired):
        return ""


def scan_one(root: Path, slug: str, gover: str, runner=run_govulncheck) -> dict:
    """One connector's result record. status "error" on ANY failure."""
    mod = root / "skills" / slug / "cli"
    res = {"slug": slug, "status": "ok", "go": gover, "targets": [], "vulns": {}}
    try:
        want = release_go(gomod_info(mod))
    except ValueError as e:
        return {**res, "status": "error", "error": f"go.mod unreadable: {e}"}
    # govulncheck judges stdlib findings by the scanning Go, so a scan under a Go
    # the release does not build with would report the wrong stdlib vulns.
    if want != gover:
        return {**res, "status": "error",
                "error": (f"go.mod builds with {want} but the scanner Go is {gover or 'unknown'}" if want else
                          "cannot establish exactly which Go builds this module (go.mod needs a goX.Y.Z "
                          "go or toolchain directive)")}
    for goos, goarch in TARGETS:
        tgt = f"{goos}/{goarch}"
        code, out, err = runner(mod, goos, goarch)
        if code != 0:
            return {**res, "status": "error", "error": f"govulncheck {tgt} exit {code}: {(err or out).strip()[-300:]}"}
        try:
            found = findings_of(parse_stream(out))
        except ValueError as e:
            return {**res, "status": "error", "error": f"govulncheck {tgt} output unreadable: {e}"}
        res["targets"].append(tgt)
        for vid, v in found.items():
            cur = res["vulns"].setdefault(vid, {"level": "required", "modules": {}, "called_on": []})
            if LEVELS.index(v["level"]) > LEVELS.index(cur["level"]):
                cur["level"] = v["level"]
            if v["level"] == "called":
                cur["called_on"].append(tgt)
            for mname, mv in v["modules"].items():
                cur["modules"].setdefault(mname, mv)
    return res


# --------------------------------------------------------------------------- #
# Report: results -> one issue per ID.
# --------------------------------------------------------------------------- #

def clean(value, rx: re.Pattern) -> str:
    return value if isinstance(value, str) and rx.match(value) else WITHHELD


def load_results(rdir: Path, expected: list[str]) -> tuple[dict[str, dict], list[str]]:
    """({slug: record} for clean scans, problems). A connector with no result, an
    error result or a malformed one is a problem; so is a vuln ID that is not
    well formed (it cannot be reported, so it must not pass silently)."""
    problems: list[str] = []
    got: dict[str, dict] = {}
    for slug in expected:
        p = rdir / f"{slug}.json"
        try:
            rec = json.loads(p.read_text(encoding="utf-8"))
        except FileNotFoundError:
            problems.append(f"{slug}: no scan result (its shard did not run, failed or was cancelled)")
            continue
        except (OSError, ValueError) as e:
            problems.append(f"{slug}: scan result unreadable: {e!r}")
            continue
        if not isinstance(rec, dict) or rec.get("slug") != slug:
            problems.append(f"{slug}: scan result is for a different connector")
            continue
        if rec.get("status") != "ok":
            problems.append(f"{slug}: scan failed: {str(rec.get('error'))[:300]}")
            continue
        vulns = rec.get("vulns")
        if not isinstance(vulns, dict) or sorted(rec.get("targets") or []) != sorted(f"{a}/{b}" for a, b in TARGETS):
            problems.append(f"{slug}: scan result does not cover every target")
            continue
        ok = {}
        for vid, v in vulns.items():
            if not isinstance(vid, str) or not ID_RE.match(vid):
                problems.append(f"{slug}: govulncheck reported an ID that is not GO-YYYY-NNNN: {vid!r:.60}")
                continue
            if not isinstance(v, dict) or v.get("level") not in LEVELS or not isinstance(v.get("modules"), dict):
                problems.append(f"{slug}: malformed finding for {vid}")
                continue
            ok[vid] = v
        got[slug] = {**rec, "vulns": ok}
    return got, problems


def aggregate(results: dict[str, dict]) -> dict[str, list[dict]]:
    """{id: [row per (connector, module)]} sorted, with every untrusted value cleaned."""
    out: dict[str, list[dict]] = {}
    for slug in sorted(results):
        for vid, v in results[slug]["vulns"].items():
            mods = v["modules"] or {"": {}}
            for mname in sorted(mods):
                mv = mods[mname] if isinstance(mods[mname], dict) else {}
                out.setdefault(vid, []).append({
                    "slug": slug, "module": clean(mname, MODULE_RE),
                    "version": clean(mv.get("version"), VERSION_RE),
                    "fixed": clean(mv.get("fixed"), VERSION_RE) if mv.get("fixed") else "none yet",
                    "level": v["level"],
                    "called_on": [t for t in (v.get("called_on") or [])
                                  if t in {f"{a}/{b}" for a, b in TARGETS}]})
    return out


def reachable_count(rows: list[dict]) -> int:
    return len({r["slug"] for r in rows if r["level"] == "called"})


def render(vid: str, rows: list[dict]) -> tuple[str, str]:
    n = reachable_count(rows)
    title = f"vuln-watch: {vid} reachable in {n} connector{'' if n == 1 else 's'}"
    why = {"called": "yes", "imported": "no (package imported, no vulnerable symbol called)",
           "required": "no (module required only)"}
    lines = [marker(vid), "",
             f"**{vid}**: https://pkg.go.dev/vuln/{vid}", "",
             f"govulncheck reports this vulnerability in {len({r['slug'] for r in rows})} connector(s); "
             f"a vulnerable symbol is called in {n}.", "",
             "| Connector | Module | Version | Fixed in | Reachable | Called on |",
             "|---|---|---|---|---|---|"]
    for r in sorted(rows, key=lambda r: (r["level"] != "called", r["slug"], r["module"])):
        lines.append(f"| {r['slug']} | `{r['module']}` | `{r['version']}` | `{r['fixed']}` | "
                     f"{why[r['level']]} | {', '.join(r['called_on']) or '-'} |")
    lines += ["", "Fix: move the module to the fixed version in each reachable connector "
                  "(`go get <module>@<fixed>` in `skills/<slug>/cli`), then release it.", "",
              "Written by `.github/workflows/vuln-watch.yml` (govulncheck symbol scan, daily). "
              "This body is replaced when the affected set changes; the issue is never closed "
              "automatically."]
    return title, "\n".join(lines) + "\n"


def issue_id(issue: dict) -> str | None:
    m = TITLE_ID_RE.match(issue.get("title") or "")
    if m:
        return m.group(1)
    found = re.search(r"<!-- vuln-watch:(GO-[0-9]{4}-[0-9]+) -->", issue.get("body") or "")
    return found.group(1) if found else None


BOT = "github-actions[bot]"


def ours(issue: dict) -> bool:
    """An issue this watch may edit: it carries the label (only people with triage
    rights can label) or the workflow's own token opened it. An unlabelled issue a
    person opened is never ours, so nobody can make the bot edit their issue or
    pre-empt an ID with one."""
    return LABEL in (issue.get("labels") or []) or issue.get("author") == BOT


def plan(issues: list[dict], agg: dict[str, list[dict]]) -> tuple[list[tuple], list[str]]:
    """(actions, notes). Pure. `issues` is every issue of ours in ANY state. An ID
    with an open issue gets an edit when its text changed (and its label back if a
    person removed it); an ID with only a closed issue is left alone (a person
    closed it; it is never reopened or duplicated); an ID with no issue gets a
    create only when reachable somewhere."""
    by_id: dict[str, list[dict]] = {}
    for i in sorted((i for i in issues if ours(i)), key=lambda i: i["number"]):
        vid = issue_id(i)
        if vid:
            by_id.setdefault(vid, []).append(i)
    acts, notes = [], []
    for vid in sorted(agg):
        title, body = render(vid, agg[vid])
        have = by_id.get(vid, [])
        live = [i for i in have if i.get("state") == "open"]
        if live:
            keep = live[0]
            relabel = LABEL not in (keep.get("labels") or [])
            if relabel or keep.get("title") != title or (keep.get("body") or "") != body:
                acts.append(("edit", keep["number"], title, body, relabel))
        elif have:
            if reachable_count(agg[vid]):
                notes.append(f"{vid}: reachable in {reachable_count(agg[vid])}, but #{have[0]['number']} for it "
                             "is closed; not reopened (reopen it to have it kept current)")
        elif reachable_count(agg[vid]):
            acts.append(("create", vid, title, body))
    return acts, notes


class GhIssues:
    def __init__(self, repo: str):
        if not re.fullmatch(r"[A-Za-z0-9-]+/[A-Za-z0-9._-]+", repo):
            raise ValueError(f"not an owner/repo: {repo!r}")
        self.repo = repo

    def _api(self, *args: str, inp: dict | None = None) -> str:
        argv = ["gh", "api", *args] + (["--input", "-"] if inp is not None else [])
        r = subprocess.run(argv, input=json.dumps(inp) if inp is not None else None,
                           capture_output=True, text=True, timeout=120)
        if r.returncode != 0:
            raise RuntimeError(f"gh api {' '.join(args[:3])} failed: {r.stderr.strip()[:300]}")
        return r.stdout

    def ensure_label(self) -> None:
        try:
            self._api(f"repos/{self.repo}/labels/{LABEL}")
        except RuntimeError:
            self._api("-X", "POST", f"repos/{self.repo}/labels",
                      inp={"name": LABEL, "color": LABEL_COLOR, "description": LABEL_DESC})

    def list_issues(self) -> list[dict]:
        """Every issue, open or closed, that carries the label OR that the workflow's
        token opened (so removing the label cannot make the next run duplicate it).
        One compact JSON object per line (@json), so a body holding a newline or
        "][" can never be read as a page boundary."""
        seen: dict[int, dict] = {}
        for filt in (f"labels={LABEL}", f"creator={BOT}"):
            out = self._api("-X", "GET", "--paginate", f"repos/{self.repo}/issues",
                            "-f", filt, "-f", "state=all", "-f", "per_page=100",
                            "-q", ".[] | select(.pull_request == null) | {number, title, body, state, "
                                  "labels: [.labels[].name], author: .user.login} | @json")
            for ln in out.splitlines():
                if ln.strip():
                    i = json.loads(ln)
                    seen[int(i["number"])] = i
        return list(seen.values())

    def create(self, title: str, body: str) -> int:
        return json.loads(self._api("-X", "POST", f"repos/{self.repo}/issues",
                                    inp={"title": title, "body": body, "labels": [LABEL]}))["number"]

    def edit(self, number: int, title: str, body: str, relabel: bool) -> None:
        self._api("-X", "PATCH", f"repos/{self.repo}/issues/{int(number)}", inp={"title": title, "body": body})
        if relabel:
            self._api("-X", "POST", f"repos/{self.repo}/issues/{int(number)}/labels", inp={"labels": [LABEL]})


class FakeIssues:
    """Stand-in for the issues API. An entry defaults to open, labelled, bot-opened."""

    def __init__(self, issues: list[dict] | None = None):
        self.issues = [{"state": "open", "labels": [LABEL], "author": BOT, **i} for i in (issues or [])]
        self.labelled = False

    def ensure_label(self) -> None:
        self.labelled = True

    def list_issues(self) -> list[dict]:
        return [dict(i) for i in self.issues]

    def create(self, title: str, body: str) -> int:
        n = max([i["number"] for i in self.issues] or [0]) + 1
        self.issues.append({"number": n, "title": title, "body": body, "state": "open",
                            "labels": [LABEL], "author": BOT})
        return n

    def edit(self, number: int, title: str, body: str, relabel: bool) -> None:
        for i in self.issues:
            if i["number"] == number:
                i.update(title=title, body=body)
                if relabel:
                    i["labels"] = [*i["labels"], LABEL]


def report(root: Path, rdir: Path, backend, apply: bool, out=sys.stdout) -> int:
    slugs, problems = connectors(root)
    results, bad = load_results(rdir, slugs)
    problems += bad
    agg = aggregate(results)
    w = out.write
    w(f"## vuln-watch\n\nScanned cleanly: {len(results)} of {len(slugs)} connectors "
      f"({', '.join(f'{a}/{b}' for a, b in TARGETS)}).\n\n")
    for vid in sorted(agg):
        rows = agg[vid]
        w(f"- {vid}: reachable in {reachable_count(rows)}, reported in "
          f"{len({r['slug'] for r in rows})} connector(s)\n")
    if not agg:
        w("- no vulnerabilities reported\n")
    acts, notes = plan(backend.list_issues(), agg) if backend is not None else ([], [])
    w("\n### Issue actions" + ("" if apply else " (dry run: nothing is written)") + "\n\n")
    for a in acts:
        w(f"- {a[0]} {'#' + str(a[1]) if a[0] == 'edit' else a[1]}: {a[2]}\n")
        if not apply:
            w("\n```text\n" + a[3].replace("```", "'''") + "```\n\n")
            continue
        if a[0] == "create":
            backend.ensure_label()
            w(f"  opened #{backend.create(a[2], a[3])}\n")
        else:
            backend.edit(a[1], a[2], a[3], a[4])
    if not acts:
        w("- no change\n")
    for n in notes:
        w(f"- note: {n}\n")
    if problems:
        w("\n### ERRORS: these connectors were NOT scanned, so a vulnerability in them would be missed\n\n")
        for p in problems:
            w(f"- {p}\n")
    return 1 if problems else 0


# --------------------------------------------------------------------------- #
# Self-test (offline: fake govulncheck streams, fake issues API).
# --------------------------------------------------------------------------- #

def _stream(*findings) -> str:
    msgs = [{"config": {"protocol_version": "v1.0.0", "scanner_name": "govulncheck"}}, {"progress": {"message": "x"}}]
    for vid, mod, ver, fixed, pkg, fn in findings:
        top = {"module": mod, "version": ver}
        if pkg:
            top["package"] = pkg
        if fn:
            top["function"] = fn
        msgs.append({"finding": {"osv": vid, "fixed_version": fixed, "trace": [top]}})
    return "\n".join(json.dumps(m, indent=2) for m in msgs)


def _tree(base: Path, mods: dict[str, str], registry: dict | None = None) -> Path:
    for slug, gomod in mods.items():
        (base / "skills" / slug / "cli").mkdir(parents=True)
        if gomod is not None:
            (base / "skills" / slug / "cli" / "go.mod").write_text(gomod, encoding="utf-8")
    (base / "tools" / "maintainer").mkdir(parents=True, exist_ok=True)
    reg = registry if registry is not None else {s: {} for s in mods}
    (base / "tools" / "maintainer" / "skills.json").write_text(json.dumps({"skills": reg}), encoding="utf-8")
    return base


def self_test() -> int:
    bad: list[str] = []

    def expect(cond, label: str) -> None:
        print(("  ok   " if cond else "  FAIL ") + label)
        if not cond:
            bad.append(label)

    GM = "module x\n\ngo 1.26\n\ntoolchain go1.26.10\n"
    BROTLI = ("GO-2026-6888", "github.com/andybalholm/brotli", "v1.2.1", "v1.2.7")
    called = _stream(BROTLI + ("", ""), BROTLI + ("github.com/andybalholm/brotli", ""),
                     BROTLI + ("github.com/andybalholm/brotli", "Read"))
    modonly = _stream(("GO-2026-5942", "golang.org/x/net", "v0.55.0", "v0.56.0", "", ""))

    def runner_for(streams: dict):
        def run(mod, goos, goarch):
            v = streams.get((mod.parent.name, f"{goos}/{goarch}"), streams.get(mod.parent.name))
            return v if isinstance(v, tuple) else (0, v if v is not None else _stream(), "")
        return run

    print("parsing and levels:")
    f = findings_of(parse_stream(called))
    expect(f["GO-2026-6888"]["level"] == "called", "a finding with a function in trace[0] is called (reachable)")
    expect(findings_of(parse_stream(modonly))["GO-2026-5942"]["level"] == "required", "module-only is required")
    for label, text in (("no config message", '{"progress": {}}'), ("truncated JSON", called[:-5]),
                        ("a non-object message", "[1]"), ("a finding with no trace",
                         _stream() + '{"finding": {"osv": "GO-2026-1-1"}}')):
        try:
            findings_of(parse_stream(text))
            expect(False, f"{label} is rejected")
        except ValueError:
            expect(True, f"{label} is rejected")

    with tempfile.TemporaryDirectory() as td:
        print("enumeration (no connector drops out quietly):")
        root = _tree(Path(td) / "a", {"axcient": GM, "cipp": GM, "zz": GM})
        slugs, probs = connectors(root)
        expect(slugs == ["axcient", "cipp", "zz"] and not probs, "every skills/*/cli/go.mod is a connector")
        r2 = _tree(Path(td) / "b", {"axcient": GM, "broken": None}, {"axcient": {}, "broken": {}, "gone": {},
                                                                     "md": {"markdown_only": True}})
        s2, p2 = connectors(r2)
        expect(s2 == ["axcient"] and any("broken/cli has no go.mod" in p for p in p2)
               and any("'gone'" in p for p in p2) and not any("md" in p for p in p2),
               "a cli/ without go.mod and a registry skill with no module are errors; markdown_only is not")
        expect(shard_of(slugs, "0/2") + shard_of(slugs, "1/2") == ["axcient", "zz", "cipp"],
               "shards partition the connector list")
        try:
            shard_of(slugs, "2/2")
            expect(False, "an out-of-range shard is refused")
        except ValueError:
            expect(True, "an out-of-range shard is refused")

        print("scanning (fail loudly, never skip):")
        ok = scan_one(root, "axcient", "go1.26.10", runner_for({"axcient": called}))
        expect(ok["status"] == "ok" and ok["vulns"]["GO-2026-6888"]["level"] == "called"
               and len(ok["vulns"]["GO-2026-6888"]["called_on"]) == len(TARGETS), "reachable on every target")
        win = scan_one(root, "axcient", "go1.26.10",
                       runner_for({("axcient", "windows/amd64"): called, "axcient": modonly}))
        expect(win["vulns"]["GO-2026-6888"]["called_on"] == ["windows/amd64"],
               "a call path only a windows build has is still reachable")
        e1 = scan_one(root, "axcient", "go1.26.10", runner_for({("axcient", "darwin/arm64"): (1, "", "load failed")}))
        expect(e1["status"] == "error" and "darwin/arm64" in e1["error"], "a failing target makes the connector an error")
        e2 = scan_one(root, "axcient", "go1.26.10", runner_for({"axcient": "not json"}))
        expect(e2["status"] == "error", "unreadable output is an error, not zero findings")
        e3 = scan_one(root, "axcient", "go1.26.9", runner_for({}))
        expect(e3["status"] == "error" and "go1.26.10" in e3["error"],
               "a scanner Go other than the one the release builds with is an error")
        e4 = scan_one(root, "axcient", "go1.26.10", runner_for({"axcient": (3, called, "")}))
        expect(e4["status"] == "error", "a nonzero govulncheck exit is an error even with output")

        print("report (completeness, validation, one issue per ID):")
        rd = Path(td) / "res"
        rd.mkdir()

        def write(*recs):
            for p in rd.iterdir():
                p.unlink()
            for rec in recs:
                (rd / f"{rec['slug']}.json").write_text(json.dumps(rec), encoding="utf-8")

        cl = {s: scan_one(root, s, "go1.26.10", runner_for({"axcient": called, "cipp": called, "zz": modonly}))
              for s in slugs}
        write(*cl.values())
        fake = FakeIssues()
        buf = _Buf()
        code = report(root, rd, fake, True, buf)
        titles = sorted(i["title"] for i in fake.issues)
        expect(code == 0 and titles == ["vuln-watch: GO-2026-6888 reachable in 2 connectors"] and fake.labelled,
               "a reachable ID opens one issue; a module-only ID opens none")
        body = fake.issues[0]["body"]
        expect(marker("GO-2026-6888") in body and "| axcient | `github.com/andybalholm/brotli` | `v1.2.1` | `v1.2.7` | yes" in body,
               "the body names the module, versions and reachability per connector")
        expect(report(root, rd, fake, True, _Buf()) == 0 and len(fake.issues) == 1 and fake.issues[0]["body"] == body,
               "a second run with the same set changes nothing and opens nothing")
        cl2 = dict(cl, cipp=scan_one(root, "cipp", "go1.26.10", runner_for({"cipp": modonly})))
        write(*cl2.values())
        report(root, rd, fake, True, _Buf())
        expect(len(fake.issues) == 1 and fake.issues[0]["title"].endswith("reachable in 1 connector"),
               "a changed affected set edits the one issue in place")
        human = FakeIssues([{"number": 7, "title": "vuln-watch: GO-2026-6888 reachable in 9 connectors",
                             "body": "edited by a person"}])
        report(root, rd, human, True, _Buf())
        expect(len(human.issues) == 1 and marker("GO-2026-6888") in human.issues[0]["body"],
               "an open issue found by the ID in its title is edited, never duplicated")
        dup = FakeIssues([{"number": 3, "title": "renamed", "body": marker("GO-2026-6888")},
                          {"number": 9, "title": "vuln-watch: GO-2026-6888 reachable in 1 connector", "body": ""}])
        acts, _ = plan(dup.list_issues(), aggregate(load_results(rd, slugs)[0]))
        expect([a[:2] for a in acts] == [("edit", 3)], "with two open copies only the oldest is edited; no create")
        expect(plan([], {"GO-2026-5942": aggregate(load_results(rd, slugs)[0])["GO-2026-5942"]}) == ([], []),
               "an ID reachable nowhere with no open issue opens nothing")
        dry = FakeIssues()
        out = _Buf()
        report(root, rd, dry, False, out)
        expect(dry.issues == [] and not dry.labelled and "reachable in 1 connector" in out.text,
               "--dry-run prints the title and body and writes nothing")

        evil = dict(cl["axcient"])
        evil["vulns"] = {"GO-2026-6888": {"level": "called", "called_on": ["linux/amd64", "$(id)"],
                                          "modules": {"x.io/m`|@everyone\n| y |": {"version": "v1.0.0\n<img>",
                                                                                  "fixed": "v1.2.7"}}},
                         "GO-2026-1 ; rm -rf /": {"level": "called", "modules": {}}}
        write(evil, cl["cipp"], cl["zz"])
        out = _Buf()
        code = report(root, rd, FakeIssues(), False, out)
        expect(code == 1 and "not GO-YYYY-NNNN" in out.text and "rm -rf" not in out.text.split("ERRORS")[0],
               "a malformed ID is a loud error and never reaches an issue")
        expect("@everyone" not in out.text and "<img>" not in out.text and "$(id)" not in out.text
               and WITHHELD in out.text, "an untrusted module path, version or target is withheld, not rendered")

        write(cl["axcient"], cl["zz"])
        out = _Buf()
        fk = FakeIssues()
        code = report(root, rd, fk, True, out)
        expect(code == 1 and "cipp: no scan result" in out.text and len(fk.issues) == 1,
               "a connector with no result fails the run loudly; the others are still reported")
        write(dict(cl["axcient"], status="error", error="govulncheck linux/amd64 exit 1"), cl["cipp"], cl["zz"])
        expect(report(root, rd, FakeIssues(), False, _Buf()) == 1, "an error result fails the run")
        write(dict(cl["axcient"], targets=["linux/amd64"]), cl["cipp"], cl["zz"])
        expect(report(root, rd, FakeIssues(), False, _Buf()) == 1, "a result missing a target fails the run")
        write(dict(cl["axcient"], slug="cipp"), cl["cipp"], cl["zz"])
        (rd / "axcient.json").write_text(json.dumps(dict(cl["axcient"], slug="cipp")), encoding="utf-8")
        expect(report(root, rd, FakeIssues(), False, _Buf()) == 1, "a result file naming another connector fails")

    print("round-1 classes (codex):")
    expect(sorted(TARGETS) == sorted((o, a) for o in ("darwin", "linux", "windows") for a in ("amd64", "arm64")),
           "the scan covers exactly the release targets (an arm64-only call path is scanned)")
    rg = globals().get("release_go")
    expect(rg is not None and rg({"Go": "1.26"}) is None and rg({"Go": "1.26.9"}) == "go1.26.9"
           and rg({"Go": "1.26", "Toolchain": "go1.26.10"}) == "go1.26.10"
           and rg({"Go": "1.26.11", "Toolchain": "go1.26.10"}) == "go1.26.11",
           "the release compiler is max(go, toolchain); a minor-only go with no toolchain is unresolved")
    with tempfile.TemporaryDirectory() as td:
        rt = _tree(Path(td), {"c1": "module x\n\ngo 1.26.9 // release compiler\n", "c2": "module x\n\ngo 1.26\n"})
        for slug in ("c1", "c2"):
            r = scan_one(rt, slug, "go1.26.10", runner_for({}))
            expect(r["status"] == "error", f"go.mod {slug} ({'a commented older go' if slug == 'c1' else 'minor-only go'}) "
                                           "is an error, never a scan under the wrong Go")
    agg1 = {"GO-2026-6888": [{"slug": "axcient", "module": "m", "version": "v1.0.0", "fixed": "v1.1.0",
                              "level": "called", "called_on": ["linux/amd64"]}]}
    t1, b1 = render("GO-2026-6888", agg1["GO-2026-6888"])
    unl = FakeIssues([{"number": 7, "title": t1, "body": b1, "labels": []}])
    report_acts = plan(unl.list_issues(), agg1)[0]
    expect([a[:2] for a in report_acts] == [("edit", 7)] and report_acts[0][4] is True,
           "a bot issue whose label was removed is still found, edited and relabelled; no second issue")
    cls = FakeIssues([{"number": 8, "state": "closed", "title": "old", "body": marker("GO-2026-6888")}])
    acts_c, notes_c = plan(cls.list_issues(), agg1)
    expect(acts_c == [] and "#8" in " ".join(notes_c),
           "a closed issue for the ID is never duplicated, edited or reopened; the run notes it")
    hum = FakeIssues([{"number": 9, "title": t1, "body": "mine", "labels": [], "author": "someone"}])
    expect([a[0] for a in plan(hum.list_issues(), agg1)[0]] == ["create"],
           "an unlabelled issue a person opened with the same title is never edited")
    calls: list[tuple] = []

    def fake_api(self, *args, inp=None):
        calls.append(args)
        filt = args[args.index("-f") + 1]
        row = {"number": 7 if "creator" in filt else 8, "title": t1, "body": "", "state": "closed",
               "labels": [], "author": BOT}
        return json.dumps(row) + "\n"

    real_api = GhIssues._api
    GhIssues._api = fake_api
    try:
        got = GhIssues("Servosity/msp-skills").list_issues()
    finally:
        GhIssues._api = real_api
    flat = [" ".join(c) for c in calls]
    expect(len(calls) == 2 and all("state=all" in c for c in flat) and any(f"labels={LABEL}" in c for c in flat)
           and any(f"creator={BOT}" in c for c in flat) and sorted(i["number"] for i in got) == [7, 8],
           "the API listing reads every state, by label AND by the bot as creator")

    print("round-2 class (codex): repository files are data, never executed:")
    with tempfile.TemporaryDirectory() as td:
        probe = Path(td) / "executed"
        reg = Path(td) / "registry.py"
        reg.write_text(f"import pathlib\npathlib.Path({str(probe)!r}).write_text('x')\n"
                       "TARGETS = [{'goos': 'linux', 'goarch': 'amd64'}, {'goos': 'darwin', 'goarch': 'arm64'}]\n",
                       encoding="utf-8")
        try:
            got_t = _release_targets(reg)
        except (SystemExit, TypeError) as e:
            got_t = repr(e)
        expect(not probe.exists() and got_t == (("darwin", "arm64"), ("linux", "amd64")),
               "registry.py is parsed for TARGETS, never run")
        for label, text in (("a computed TARGETS", "TARGETS = make()\n"), ("no TARGETS", "X = 1\n"),
                            ("a bad GOOS", "TARGETS = [{'goos': 'li nux', 'goarch': 'amd64'}]\n")):
            reg.write_text(text, encoding="utf-8")
            try:
                _release_targets(reg)
                expect(False, f"{label} is refused")
            except (SystemExit, TypeError):
                expect(True, f"{label} is refused")

    print("live repository enumeration:")
    live, lp = connectors(ROOT)
    expect(len(live) > 50 and not lp, f"this tree: {len(live)} connectors, no enumeration problem")

    print(f"\n{'PASS' if not bad else 'FAIL'}: vuln_watch self-test ({len(bad)} failure(s))")
    return 1 if bad else 0


class _Buf:
    def __init__(self):
        self.text = ""

    def write(self, s: str) -> None:
        self.text += s


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    mode = ap.add_mutually_exclusive_group(required=True)
    mode.add_argument("--list", action="store_true", help="print the connector slugs as JSON")
    mode.add_argument("--scan", action="store_true")
    mode.add_argument("--report", action="store_true")
    mode.add_argument("--self-test", action="store_true")
    ap.add_argument("--root", default=str(ROOT), help="repository tree to read (default: this checkout)")
    ap.add_argument("--shard", default="0/1", help="I/N: scan every Nth connector from I")
    ap.add_argument("--only", action="append", default=[], help="scan just this slug (local proof runs)")
    ap.add_argument("--out", help="--scan: directory for one <slug>.json per connector")
    ap.add_argument("--results", help="--report: the directory --scan wrote")
    ap.add_argument("--repo", help="--report: owner/repo whose issues to read (and write with --apply)")
    w = ap.add_mutually_exclusive_group()
    w.add_argument("--apply", action="store_true", help="--report: write the issues")
    w.add_argument("--dry-run", action="store_true", help="--report: print each issue's title and body, "
                                                          "write nothing (the default)")
    a = ap.parse_args(argv)
    if a.self_test:
        return self_test()
    root = Path(a.root)
    slugs, problems = connectors(root)
    if a.list:
        print(json.dumps(slugs))
        for p in problems:
            print(f"ERROR: {p}", file=sys.stderr)
        return 1 if problems else 0
    if a.scan:
        if not a.out:
            ap.error("--scan needs --out")
        todo = [s for s in slugs if s in a.only] if a.only else shard_of(slugs, a.shard)
        if a.only and sorted(set(a.only)) != todo:
            ap.error(f"--only names a slug that is not a connector: {sorted(set(a.only) - set(todo))}")
        outd = Path(a.out)
        outd.mkdir(parents=True, exist_ok=True)
        gover, failed = scanner_go(), 0
        for slug in todo:
            rec = scan_one(root, slug, gover)
            (outd / f"{slug}.json").write_text(json.dumps(rec, indent=1, sort_keys=True) + "\n", encoding="utf-8")
            called = sorted(k for k, v in rec["vulns"].items() if v["level"] == "called")
            print(f"{slug}: {rec['status']}" + (f" ({rec['error']})" if rec["status"] != "ok" else
                                               f", {len(rec['vulns'])} reported, called: {', '.join(called) or 'none'}"),
                  flush=True)
            failed += rec["status"] != "ok"
        for p in problems:
            print(f"ERROR: {p}")
        return 1 if failed or problems else 0
    if not a.results:
        ap.error("--report needs --results")
    backend = GhIssues(a.repo) if a.repo else None
    if a.apply and backend is None:
        ap.error("--apply needs --repo")
    return report(root, Path(a.results), backend, a.apply)


if __name__ == "__main__":
    sys.exit(main())
