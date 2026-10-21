#!/usr/bin/env python3
"""Scope, attribution and verdict for tools/maintainer/verify_all.sh.

verify_all.sh is the local "would CI pass?" check. Before 2026-10-21 it ignored
every argument, so a one-connector change rebuilt all 84 connectors (22-28
minutes) and a shared /tmp/va.out let two sessions overwrite each other. This
helper does the three parts that need more than shell:

  scope      Which connectors to check. --changed reuses CI's own change-set
             classifier (release_matrix.classify_change), so a change to shared
             machinery checks the whole fleet exactly as CI does, a bot-only
             derived-file change checks nothing, and a Windows-ledger edit
             checks the skills whose exclusions moved. The security gate gets
             its own scope, mirroring security-gate.yml: every slug with any
             changed path under skills/.
  attribute  Whether a failed FLEET-wide check failed for a connector in
             scope. Each output line is attributed by its first location token
             (`[slug]`, `slug:`, `skills/<dir>/...`, `docs/skills/<slug>.md`,
             or any other repo path). A failure that names an in-scope
             connector or a repo-level file blocks. A failure that names only
             connectors outside the scope is listed under out_of_scope_failures
             in the verdict, never dropped. A failure that names nothing
             cannot be attributed and blocks (fail closed).
  verdict    The machine-readable result: checks, verdict, seconds, head and
             base SHAs, scope, out_of_scope_failures.

    python3 tools/maintainer/verify_scope.py scope [--slug S]... [--changed] [--base REF]
    python3 tools/maintainer/verify_scope.py attribute --scope FILE --log FILE
    python3 tools/maintainer/verify_scope.py verdict --scope FILE --records FILE --out FILE
    python3 tools/maintainer/verify_scope.py --self-test
"""

from __future__ import annotations

import argparse
import datetime as _dt
import json
import re
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import registry  # noqa: E402  (local tools/ module)
import release_matrix  # noqa: E402  (local tools/ module: CI's change-set classifier)

ROOT = registry.ROOT
DIR_RE = re.compile(r"\A[a-z0-9][a-z0-9-]*\Z")  # security-gate.yml's slug charset
# security-gate.yml's Scope step FAILS a PR that edits the gate's own logic,
# before it scans anything (it cannot approve itself). Read from the workflow
# so the two never drift; the list is the fallback, and --self-test asserts
# the workflow still declares it.
GATE_LOGIC_FALLBACK = (
    "tools/maintainer/check_security_gate.py", ".semgrep.yml", ".github/workflows/security-gate.yml",
    ".github/workflows/release.yml", ".github/CODEOWNERS",
)


def gate_logic_re() -> re.Pattern:
    try:
        text = (ROOT / ".github/workflows/security-gate.yml").read_text(encoding="utf-8")
        m = re.search(r"grep -qE '(\^\(tools/maintainer/check_security_gate[^']*\)\$)'", text)
        if m:
            return re.compile(m.group(1))
    except (OSError, re.error):
        pass
    return re.compile("^(" + "|".join(re.escape(p) for p in GATE_LOGIC_FALLBACK) + ")$")


def _git(*args: str) -> tuple[int, str]:
    p = subprocess.run(["git", "-C", str(ROOT), *args], capture_output=True, text=True)
    return p.returncode, p.stdout.strip()


def _rev(ref: str) -> str | None:
    rc, out = _git("rev-parse", "--verify", "--quiet", f"{ref}^{{commit}}")
    return out if rc == 0 and out else None


def skill_dirs() -> list[str]:
    return sorted(p.name for p in (ROOT / "skills").iterdir() if p.is_dir() and not p.name.startswith("."))


def connectors() -> dict[str, str]:
    """registry slug -> skills/<dir> for every connector (a Go CLI to build)."""
    out = {}
    for slug in registry.skills():
        if registry.is_markdown_only(slug):
            continue
        d = registry.source_dir(slug)
        if (ROOT / "skills" / d / "cli" / "go.mod").is_file():
            out[slug] = d
    return out


def _aliases(names: set[str]) -> set[str]:
    """A skill is named by its registry slug in some outputs and by its
    directory in others; carry both."""
    out = set(names)
    for slug in registry.skills():
        d = registry.source_dir(slug)
        if slug in names or d in names:
            out |= {slug, d}
    return out


def _touched_dirs(files: list[str]) -> set[str]:
    return {f.split("/")[1] for f in files if f.startswith("skills/") and f.count("/") >= 2}


def resolve_scope(slugs: list[str], changed: bool, base: str) -> dict:
    conn = connectors()
    reg = registry.skills()
    dirs = set(skill_dirs())
    for s in slugs:
        if s not in reg and s not in dirs:
            raise SystemExit(f"verify_scope: unknown skill {s!r} (not in tools/maintainer/skills.json "
                             "and no skills/<dir> of that name)")
    head = _rev("HEAD")
    base_sha = _rev(base)
    mb = None
    if base_sha:
        rc, out = _git("merge-base", base_sha, "HEAD")
        mb = out if rc == 0 and out else None
    rc, porcelain = _git("status", "--porcelain", "--untracked-files=normal")
    dirty = rc != 0 or bool(porcelain)
    notes: list[str] = []
    if not base_sha:
        notes.append(f"base {base!r} does not resolve; base-relative checks report it")

    mode = "full" if not slugs and not changed else ("changed" if changed and not slugs else
                                                     "slug" if slugs and not changed else "slug+changed")
    full_fleet = mode == "full"
    reason = "no arguments: the full fleet (today's behavior)" if full_fleet else ""
    picked: set[str] = set()      # connector slugs for the per-connector checks
    touched: set[str] = set()     # skill dirs the change or the caller named
    files: list[str] = []

    for s in slugs:
        d = registry.source_dir(s) if s in reg else s
        touched.add(d)
        for cs, cd in conn.items():
            if cs == s or cd == s:
                picked.add(cs)
    cs = release_matrix.change_set(base, worktree=True) if base_sha else None
    if cs is not None:
        rx = gate_logic_re()
        gate_logic = sorted(f for f in cs[1] if rx.match(f))
    else:
        gate_logic = []
    if changed or full_fleet:
        if cs is None:
            if changed:
                full_fleet = True
                reason = (f"--changed: no change set against {base!r} could be computed; "
                          "FULL fleet (fail open, as CI does)")
        else:
            mb, files = cs
            touched |= _touched_dirs(files)
            if changed:
                why: list[str] = []
                rows = release_matrix.classify_change(mb, files, new_rev=None, reasons=why)
                if rows is None:
                    full_fleet = True
                    reason = "--changed: " + (why[0] if why else "CI's classifier chose the FULL matrix")
                else:
                    picked |= {s for s in conn if s in rows or conn[s] in rows}
                    reason = (reason + "; " if reason else "") + (
                        f"--changed: CI's classifier selected {len(rows & set(conn))} connector row(s) "
                        f"from {len(files)} changed file(s)")
    if full_fleet:
        picked = set(conn)
    if slugs and not reason:
        reason = f"--slug: {', '.join(sorted(slugs))}"
    security = sorted(d for d in touched if DIR_RE.match(d) and d in dirs)
    affected = set(dirs) if full_fleet else _aliases(touched | {conn[s] for s in picked} | picked)
    return {
        "schema": 1,
        "mode": mode,
        "base_ref": base,
        "base_sha": base_sha,
        "merge_base": mb,
        "head_sha": head,
        "dirty": dirty,
        "full_fleet": full_fleet,
        "reason": reason,
        "notes": notes,
        "connectors": sorted(picked),
        "connector_dirs": {s: conn[s] for s in sorted(picked)},
        "touched": sorted(touched),
        "security_gate_slugs": security,
        "affected": sorted(affected),
        "changed_files": len(files),
        "gate_logic_changed": gate_logic,
    }


# --------------------------------------------------------------------------- #
# Attribution of a failed fleet-wide check
# --------------------------------------------------------------------------- #

# A location: a repo path (top-level folder prefix, or a file name with a known
# extension, or an extensionless file the checks name: NOTICE, LICENSE,
# Makefile, a git hook).
_EXT = r"md|json|ya?ml|sh|ps1|py|txt|toml|go|mod|sum|html|png|jpe?g|gif|webp|mp4|svg|xml|csv|mcpb"
_PATH = re.compile(r"(?<![\w/.-])((?:skills|tools|docs|assets|scripts|\.github|\.claude-plugin)/[\w./@+-]*[\w@+-]"
                   rf"|(?:[\w.-]+/)*[\w-][\w.-]*\.(?:{_EXT})|(?:[\w.-]+/)*(?:NOTICE|LICENSE|Makefile))"
                   r"(?::\d+)?(?![\w])")
# A diagnostic that STARTS with its source: grep-style `path:line:` (any path,
# dotfiles included: `.gitignore:197:`), or a dotfile path.
_GREP_LOC = re.compile(r"^([^:\n]+?):\d+(?::|\s|$)")   # spaces allowed: `Release Notes.md:1:`
_DOT_LOC = re.compile(r"^(\.[A-Za-z][\w.-]*(?:/[\w.@+-]+)*)(?::|\s|$)")
_BRACKET = re.compile(r"\[([a-z0-9_][a-z0-9-]*)\]")
_MARKERS = re.compile(r"^\s*(?:(?:[-*]|FAIL:?|WARN:?|ERROR:?|\[BLOCK\]|R\d+)\s+)*")  # R1: install-docs rule id
_LEAD = re.compile(r"([a-z0-9_][a-z0-9-]*):\s")
# Complete lines that carry no finding: the headers and verdict lines the
# fleet checks print (each taken from a real failing run). Anything else with
# no location is UNKNOWN, and an unknown line blocks.
_BOILERPLATE = re.compile("|".join((
    r"^\s*$",
    r"^==> \d+\. .+$",                                         # ci_guards section headers
    r"^\s*hint: .+$",                                          # ci_guards explanations
    r"^(?:All CI guards passed\.|CI guards FAILED\. Fix the items above\.)$",
    r"^[A-Z][A-Za-z -]* check FAILED:$",                       # Skill contract / Markdown local-link / ...
    r"^check_[a-z_]+ FAILED:$",
    r"^check_[a-z_]+: FAIL$",
    r"^FAIL: manifest env schema does not match .*\(\d+ finding\(s\) across \d+ skill\(s\)\):$",
    r"^FAIL: hand-fix ledger regressions \(.*\):$",
    r"^FAIL: \d+ finding\(s\) - a copyright line is not the project's\.$",
    r"^\s*Expected in (?:Go|NOTICE/LICENSE): .+$",               # copyright's expected-line header
    r"^\d+ finding\(s\)\. .+$",                                # check_install_docs summary
    r"^check_cli_claims FAILED: \d+ finding\(s\)\.$",
    r"^\s*NOTE: .+$",
)))
# Remediation text the checks print AFTER their findings. From the first such
# line on, a line with no location is part of the trailer, not a finding.
_TRAILER = re.compile(r"^\s*(?:Fix:|See |Remove the marker|Banned terms:|Generate them|"
                      r"The MCP Registry answers|Contributor credit belongs|An UNREADABLE finding)")
_TRUNCATED = re.compile(r"\.\.\.\s*(?:and\s+)?\d+\s+more|\band \d+ more\b")
_ASSET_KINDS = ("social", "video")


def _skill_of(path: str) -> str | None:
    parts = path.split("/")
    if len(parts) >= 3 and parts[0] == "skills":
        return parts[1]
    if len(parts) == 3 and parts[0] == "docs" and parts[1] == "skills":
        return parts[2].rsplit(".", 1)[0].split(":", 1)[0]
    if len(parts) >= 5 and parts[:2] == ["docs", "assets"] and parts[2] in _ASSET_KINDS:
        return parts[3]   # docs/assets/social/<slug>/..., docs/assets/video/<slug>/...
    return None


def _location(path: str) -> tuple[str, str | None]:
    who = _skill_of(path.split(":", 1)[0])
    return ("skill", who) if who else ("repo", None)


def classify_line(line: str, known: set[str], affected: set[str]) -> tuple[str, str | None]:
    """("skill", name), ("repo", None), ("blank", None) or ("unknown", None).

    The SOURCE location wins: a line that starts with a path is about that
    path, whatever its message mentions (`skills/zammad/README.md:3: [avanan]
    ...` is zammad's; `README.md: broken link -> skills/avanan/x.md` and
    `.gitignore:197: ... skills/zammad ...` are repo-level). Then a leading
    `[slug]` tag or `slug:`. Then, for a line that starts with prose, the first
    skill path it names ("missing skills/zeta/social.png"). A line that also
    mentions an IN-SCOPE skill anywhere is that skill's, so a mixed line
    always blocks. A truncation marker ("... and 12 more") is unknown: the
    findings it hides cannot be attributed."""
    line = re.sub(r"(?<![\w.])\./", "", line)
    if _TRUNCATED.search(line):
        return ("unknown", None)
    mentioned = {p for p in (_skill_of(m.group(1)) for m in _PATH.finditer(line)) if p}
    mentioned |= {m.group(1) for m in _BRACKET.finditer(line) if m.group(1) in known}
    hit = sorted(mentioned & affected)
    if hit:
        return ("skill", hit[0])
    body = line[_MARKERS.match(line).end():]
    for rx in (_GREP_LOC, _DOT_LOC, _PATH):
        m = rx.match(body)
        if m:
            return _location(m.group(1))
    m = _BRACKET.match(body)
    if m and m.group(1) in known:
        return ("skill", m.group(1))
    m = _LEAD.match(body)
    if m and m.group(1) in known:
        return ("skill", m.group(1))
    for pm in _PATH.finditer(body):
        who = _skill_of(pm.group(1))
        if who:
            return ("skill", who)
    if not line.strip():
        return ("blank", None)
    return ("unknown", None)


def attribute_lines(text: str, known: set[str], affected: set[str]) -> list[tuple[str, str | None, str]]:
    """Every line as (kind, who, line), kind one of skill / repo / info /
    unknown. A line with no location is info only when it is recognised
    boilerplate or sits in a remediation trailer."""
    out = []
    trailer = False
    for line in text.splitlines():
        kind, who = classify_line(line, known, affected)
        if kind == "blank":
            kind = "info"
        elif kind == "unknown":
            if _TRAILER.match(line):
                trailer = True
            if not _TRUNCATED.search(line) and (trailer or _BOILERPLATE.match(line)):
                kind = "info"
        out.append((kind, who, line))
    return out


def attribute_line(line: str, known: set[str], affected: set[str] | None = None) -> str | None:
    """One line on its own: the skill it is about, "<repo>", "<unknown>", or
    None for recognised boilerplate (no trailer context)."""
    kind, who, _ = attribute_lines(line, known, affected or set())[0] if line else ("info", None, "")
    return {"skill": who, "repo": "<repo>", "unknown": "<unknown>"}.get(kind)


def attribute(text: str, affected: set[str], known: set[str]) -> dict:
    blocking, other = [], []
    for kind, who, line in attribute_lines(text, known, affected):
        if kind == "info":
            continue
        if kind == "skill" and who not in affected and who in known:
            other.append((who, line))
        else:
            blocking.append(line)
    if blocking or not other:
        why = ("names an in-scope skill, a repo-level file, or a failure it cannot attribute"
               if blocking else "names no file or connector, so it cannot be attributed (fail closed)")
        return {"classification": "blocking", "why": why, "slugs": [], "lines": blocking[:40]}
    return {"classification": "out_of_scope", "why": "every failure names a connector outside the scope",
            "slugs": sorted({w for w, _ in other}), "lines": [ln for _, ln in other][:40]}


_NORM_NUM = re.compile(r":\d+")
_ANSI = re.compile(r"\x1b\[[0-9;]*m")


def _norm(line: str) -> str:
    line = _ANSI.sub("", line)
    line = re.sub(r"(?<![\w.])\./", "", line)
    return " ".join(_NORM_NUM.sub(":N", line).split())


def pre_existing(head_text: str, base_text: str, affected: set[str], known: set[str]) -> dict:
    """Is a failure that names only out-of-scope connectors ALREADY on the
    base? Attribution alone cannot prove a failure is someone else's: deleting
    a connector's social image from docs/assets/, tightening vocab.json or a
    checker, or removing a file another skill links to all make OTHER
    connectors fail. So an out-of-scope failure is exempt only when the merge
    base, run with its own scripts, already prints every one of those
    findings (line numbers ignored). Anything new blocks."""
    base = {_norm(ln) for ln in base_text.splitlines()}
    new = [ln for kind, who, ln in attribute_lines(head_text, known, affected)
           if kind == "skill" and who not in affected and _norm(ln) not in base]
    return {"pre_existing": not new, "new_lines": new[:40]}


def known_names() -> set[str]:
    return set(skill_dirs()) | set(registry.skills())


# --------------------------------------------------------------------------- #
# Verdict
# --------------------------------------------------------------------------- #


def _tail(path: str, n: int = 40) -> list[str]:
    try:
        lines = Path(path).read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError:
        return []
    return lines[-n:]


def build_verdict(scope: dict, records: list[dict], seconds: float, started: str,
                  known: set[str] | None = None) -> dict:
    affected = set(scope["affected"])
    known = known_names() if known is None else known
    checks, blocking, out_of_scope = [], [], []
    for r in records:
        c = {"name": r["name"], "kind": r["kind"], "slug": r.get("slug") or None,
             "status": r["status"], "seconds": r["seconds"], "blocking": False}
        if r["status"] == "out_of_scope":
            # verify_all.sh decided this: the failure names only connectors
            # outside the scope AND the merge base already fails the same way.
            try:
                text = Path(r["log"]).read_text(encoding="utf-8", errors="replace")
            except OSError:
                text = ""
            if scope["full_fleet"]:
                c["status"], c["blocking"] = "fail", True
                c["detail"] = _tail(r["log"])
                blocking.append(r["name"])
            else:
                a = attribute(text, affected, known)
                slugs = a["slugs"] or ([c["slug"]] if c["slug"] else [])
                out_of_scope.append({"check": r["name"], "slugs": slugs,
                                     "lines": a["lines"] if a["slugs"] else _tail(r["log"]),
                                     "pre_existing_on_base": True})
        elif r["status"] == "fail":
            c["blocking"] = True
            if c["blocking"]:
                c["detail"] = _tail(r["log"])
                blocking.append(r["name"] + (f" ({c['slug']})" if c["slug"] and c["slug"] not in r["name"] else ""))
        checks.append(c)
    return {
        "schema": 1,
        "tool": "tools/maintainer/verify_all.sh",
        "verdict": "fail" if blocking else "pass",
        "started_at": started,
        "finished_at": _dt.datetime.now(_dt.timezone.utc).replace(microsecond=0).isoformat(),
        "seconds": round(seconds, 1),
        "head_sha": scope["head_sha"],
        "dirty": scope["dirty"],
        "base_ref": scope["base_ref"],
        "base_sha": scope["base_sha"],
        "merge_base": scope["merge_base"],
        "scope": {k: scope.get(k) for k in ("mode", "full_fleet", "reason", "notes", "connectors", "touched",
                                            "security_gate_slugs", "changed_files", "gate_logic_changed")},
        "counts": {s: sum(1 for c in checks if c["status"] == s)
                   for s in ("pass", "fail", "warn", "skip", "out_of_scope")},
        "blocking_failures": blocking,
        "out_of_scope_failures": out_of_scope,
        "checks": checks,
    }


def read_records(path: Path) -> list[dict]:
    out = []
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        name, kind, slug, status, seconds, log = (line.split("\t") + [""] * 6)[:6]
        try:
            secs = float(seconds)
        except ValueError:
            secs = 0.0
        out.append({"name": name, "kind": kind, "slug": slug, "status": status, "seconds": secs, "log": log})
    return out


# --------------------------------------------------------------------------- #
# The Go the CI security gate runs on
# --------------------------------------------------------------------------- #

_GO_SPEC = re.compile(r"^\s*go-version:\s*[\"']?([0-9]+\.[0-9]+)[\"']?\s*$", re.M)


def gate_go_spec() -> str | None:
    """security-gate.yml's `go-version:` spec (for example "1.26")."""
    try:
        text = (ROOT / ".github/workflows/security-gate.yml").read_text(encoding="utf-8")
    except OSError:
        return None
    m = _GO_SPEC.search(text)
    return m.group(1) if m else None


def newest_patch(spec: str, releases: list[dict]) -> str | None:
    """The newest stable goX.Y.Z for spec "X.Y", the way setup-go resolves a
    bare minor: the latest patch release."""
    best: tuple[int, ...] | None = None
    for r in releases:
        v = r.get("version", "") if isinstance(r, dict) else ""
        if not r.get("stable", False) or not v.startswith(f"go{spec}."):
            continue
        try:
            key = tuple(int(x) for x in v[2:].split("."))
        except ValueError:
            continue
        best = key if best is None or key > best else best
    return "go" + ".".join(str(x) for x in best) if best else None


def gate_go() -> str | None:
    """The toolchain security-gate.yml would run today: setup-go installs the
    newest patch of its `go-version:` spec, and GOTOOLCHAIN=auto never
    switches DOWN to go.mod's older toolchain line. Locally `go` would select
    go.mod's toolchain instead, so a stdlib advisory fixed in a newer patch
    blocks locally while CI passes. None when the release index is unreachable
    (the caller then falls back to go.mod's toolchain, the stricter choice)."""
    spec = gate_go_spec()
    if not spec:
        return None
    import urllib.request
    try:
        with urllib.request.urlopen("https://go.dev/dl/?mode=json&include=all", timeout=20) as resp:
            releases = json.load(resp)
    except (OSError, ValueError):
        return None
    return newest_patch(spec, releases if isinstance(releases, list) else [])


# --------------------------------------------------------------------------- #
# CLI
# --------------------------------------------------------------------------- #


def main(argv: list[str]) -> int:
    if argv[:1] == ["--self-test"]:
        return _self_test()
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    sub = ap.add_subparsers(dest="cmd", required=True)
    sp = sub.add_parser("scope")
    sp.add_argument("--slug", action="append", default=[])
    sp.add_argument("--changed", action="store_true")
    sp.add_argument("--base", default="origin/main")
    ap_a = sub.add_parser("attribute")
    ap_a.add_argument("--scope", required=True)
    ap_a.add_argument("--log", required=True)
    vp = sub.add_parser("verdict")
    vp.add_argument("--scope", required=True)
    vp.add_argument("--records", required=True)
    vp.add_argument("--out", required=True)
    vp.add_argument("--seconds", type=float, default=0.0)
    vp.add_argument("--started", default="")
    sub.add_parser("gate-go")
    bp = sub.add_parser("baseline")
    bp.add_argument("--scope", required=True)
    bp.add_argument("--head-log", required=True)
    bp.add_argument("--base-log", required=True)
    a = ap.parse_args(argv)
    if a.cmd == "gate-go":
        print(gate_go() or "")
        return 0
    if a.cmd == "baseline":
        sc = json.loads(Path(a.scope).read_text(encoding="utf-8"))
        r = pre_existing(Path(a.head_log).read_text(encoding="utf-8", errors="replace"),
                         Path(a.base_log).read_text(encoding="utf-8", errors="replace"),
                         set(sc["affected"]), known_names())
        print(json.dumps(r, indent=1))
        return 0 if r["pre_existing"] else 1
    if a.cmd == "scope":
        print(json.dumps(resolve_scope(a.slug, a.changed, a.base), indent=1))
        return 0
    scope = json.loads(Path(a.scope).read_text(encoding="utf-8"))
    if a.cmd == "attribute":
        text = Path(a.log).read_text(encoding="utf-8", errors="replace")
        print(json.dumps(attribute(text, set(scope["affected"]), known_names()), indent=1))
        return 0
    v = build_verdict(scope, read_records(Path(a.records)), a.seconds, a.started)
    out = Path(a.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(v, indent=1) + "\n", encoding="utf-8")
    for o in v["out_of_scope_failures"]:
        print(f"  OUT OF SCOPE  {o['check']}: fails for {', '.join(o['slugs'])} (listed in the verdict, "
              "not blocking this change)")
    print(f"verdict: {v['verdict'].upper()} - {len(v['blocking_failures'])} blocking, "
          f"{len(v['out_of_scope_failures'])} out of scope, {v['counts']['pass']} passed "
          f"({v['seconds']:.0f}s) -> {out}")
    return 1 if v["verdict"] == "fail" else 0


def _self_test() -> int:
    import contextlib
    import io
    import tempfile

    failures: list[str] = []

    def expect(cond: bool, label: str) -> None:
        print(("  ok   " if cond else "  FAIL ") + label)
        if not cond:
            failures.append(label)

    known = {"acme", "zeta", "_meta", "msp-skills-concierge"}
    print("attribute_line:")
    for line, want in (
        ("skills/acme/README.md:12:an em-dash here", "acme"),
        ("  - zeta: missing required file guide.md", "zeta"),
        ("  - skills/zeta/README.md: broken local link -> ../../CONTRIBUTING.md", "zeta"),
        ("  - missing skills/zeta/assets/social.png (expected 1200x630 PNG)", "zeta"),
        ("  - docs/skills/zeta.md: missing `title` front matter", "zeta"),
        ("  - docs/assets/social/zeta/wide-1200x630.png is not a valid PNG", "zeta"),
        ("  docs/assets/video/zeta/demo.mp4 is 12 bytes (expected > 1000)", "zeta"),
        ("  - [zeta] ZETA_TOKEN is read by the binary but NOT declared", "zeta"),
        ("  - R1 skills/zeta/mcp-install.md:194: names `mcp-remote`", "zeta"),
        ("skills/zeta/install.sh:498:6: warning: x is referenced but not assigned. [SC2154]", "zeta"),
        ("README.md:3: [zeta] zeta-pp-cli -> | zeta |", "<repo>"),
        ("skills/zeta/README.md:3: [other] the source location wins over a tag", "zeta"),
        ("README.md: broken local link -> skills/zeta/missing.md", "<repo>"),
        (".gitignore:197:# ignore skills/zeta/generated", "<repo>"),
        ("./skills/zeta/cli/go.mod selects go1.26.1, below the go1.26.9 floor", "zeta"),
        ("  skills/zeta/cli/NOTICE: copyright is not the project's", "zeta"),
        ("skills/zeta/README.md:1: cross-reference to skills/acme/README.md", "acme"),
        ("Release Notes.md:1: See skills/zeta/README.md for an example", "<repo>"),
        ("CONTRIBUTING.md:40:an em-dash here", "<repo>"),
        ("tools/maintainer/hooks/pre-push:3: something", "<repo>"),
        ("  - tools/maintainer/skills.json: not valid JSON", "<repo>"),
        ("Skill contract check FAILED:", None),
        ("check_install_docs FAILED:", None),
        ("check_registry_state: FAIL", None),
        ("FAIL: 2 finding(s) - a copyright line is not the project's.", None),
        ("  hint: em-dash found; use ' - ', a colon, or parentheses", None),
        ("==> 2. No em-dash in human-authored files", None),
        ("See docs/reprint-survival.md for how the ledger + this gate work.", None),
        ("  ... and 12 more", "<unknown>"),
        ("  - unknownthing: validation failed", "<unknown>"),
        ("registry has 3 orphan entries:", "<unknown>"),
    ):
        got = attribute_line(line, known, {"acme"})
        expect(got == want, f"{line.strip()[:60]!r} -> {want} (got {got})")

    print("attribute (real failing output of each fleet check, slug renamed to zeta):")
    real = {
        "ci_guards": "==> 1. No unresolved owner placeholder token anywhere\n==> 2. No em-dash in human-authored files\n"
                     "skills/zeta/README.md:235:x - y\n  hint: em-dash found; use ' - ', a colon, or parentheses "
                     "(CONTRIBUTING style rule)\n==> 3. No personal filesystem paths in human-authored files\n"
                     "CI guards FAILED. Fix the items above.\n",
        "copyright": "FAIL: 1 finding(s) - a copyright line is not the project's.\n  Expected in Go:             "
                     "// Copyright 2026 Servosity Inc. See LICENSE.\n  Expected in NOTICE/LICENSE: Copyright 2026 "
                     "Servosity Inc.\n  skills/zeta/cli/internal/cli/agent_context.go: // Copyright 2026 Someone Else\n"
                     "\n  Contributor credit belongs in cli/NOTICE ('contributed by ...')\n  and SKILL.md `author`, "
                     "not in per-file headers. See CONTRIBUTING.md.\n  An UNREADABLE finding is a gate failure, not "
                     "a skip: fix the file's\n  permissions or encoding so the line can actually be checked.\n",
        "env_schema": "FAIL: manifest env schema does not match what the binaries read (2 finding(s) across 1 "
                      "skill(s)):\n\n  - [zeta] ZETA_API_TOKEN is read by the binary but NOT declared in "
                      "manifest.json\n  - [zeta] server.json (mcpb) lists ZETA_API_TOKEN\n\n  Fix: a credential must "
                      "be declared identically on all three install channels - skills/<slug>/manifest.json\n",
        "handfixes": "FAIL: hand-fix ledger regressions (a reprint may have clobbered a live-validated fix):\n\n"
                     "  - [zeta] handfix 'x' REGRESSED in cli/internal/x.go: expected 1x `y` in the file, found 0.\n"
                     "\nSee docs/reprint-survival.md for how the ledger + this gate work.\n",
        "install_docs": "check_install_docs FAILED:\n  - R1 skills/zeta/mcp-install.md:194: names `mcp-remote`\n"
                        "\n1 finding(s). Every one is a doc describing an install or transport path the shipped "
                        "artifact does not provide.\n",
        "mcp_registry": "check_mcp_registry FAILED:\n  - [zeta] description: 150 characters, the registry allows "
                        "at most 100.\n\nThe MCP Registry answers 422 on these and the publish step runs AFTER the "
                        "release assets\nare uploaded, so the release looks complete while the connector never "
                        "reaches the registry\nor the catalogues that fan out from it.\n",
        "md_links": "Markdown local-link check FAILED:\n  - skills/zeta/README.md: broken local link -> ./nope.md\n",
        "media_block": "check_media_block FAILED:\n  - zeta: skills/zeta/README.md has no <!-- media:start --> block\n"
                       "\nFix: re-run the publish skill's media injection for the named skill so its README media "
                       "block + demo assets are present and in budget.\n",
        "no_todos": "check_no_todos FAILED:\n  skills/zeta/SKILL.md:625: [TODO] TODO: fill this\n\nRemove the "
                    "marker, or add 'no-todos:ignore' on the line if it is a legitimate structural reference.\n",
        "pinned": "check_pinned_artifacts: FAIL\n  - skills/zeta/README.md: zeta-v0.0.1/zeta-mcp.mcpb - new pin for "
                  "an uncut tag\n",
        "registry_state": "check_registry_state: FAIL\n  - zeta: live_verified DOWNGRADED from 'live-verified'\n",
        "release_contract": "Release contract check FAILED:\n  - zeta: install.sh CLI_BIN is 'wrong-cli', expected "
                            "'zeta-cli' (from skills.json)\n",
        "social_assets": "Social assets check FAILED:\n  - docs/assets/social/zeta/wide-1200x630.png is not a valid "
                         "PNG\n\nGenerate them (msp-skills-publish Step 6.5):\n  ~/.claude/skills/social-preview-"
                         "generator/scripts/generate.py --repo . --mode both --out docs/assets/social\n",
        "vocabulary": "check_vocabulary FAILED:\n  skills/zeta/README.md:3: banned term 'pp-cli': x\n\nBanned "
                      "terms: rephrase in words an MSP would search for, or add\n'vocab:ignore' on the line ONLY "
                      "if the term is quoted on purpose.\n",
        "shellcheck_gcc": "skills/zeta/install.sh:498:6: warning: undefined_var_x is referenced but not assigned. "
                          "[SC2154]\nskills/zeta/install.sh:499:10: error: Iterating over ls output is fragile. "
                          "[SC2045]\n",
    }
    for name, text in real.items():
        a = attribute(text, {"acme"}, known)
        expect(a["classification"] == "out_of_scope" and a["slugs"] == ["zeta"],
               f"{name}: a failure only in zeta, scope acme -> out_of_scope (got {a['classification']} "
               f"{a.get('slugs')} {a['lines'][:1]})")
        a = attribute(text, {"zeta"}, known)
        expect(a["classification"] == "blocking", f"{name}: the same failure, scope zeta -> blocking")
    for name, text, why in (
        ("surface_coverage", "check_surface_coverage FAILED:\n  - README (/abs/README.md): the <!-- catalog:start -->"
         " / <!-- catalog:end --> marker pair is missing\n", "the root README"),
        ("gitleaks", "6:49PM INF scan completed in 7.96s\n6:49PM WRN leaks found: 1\n", "a secret scan names no file"),
        ("an unknown line among other-connector lines", "Skill contract check FAILED:\n  - zeta: x\n"
         "  - unknownthing: validation failed\n", "an unattributable line"),
        ("a header with a colon is not a free pass", "Skill contract check FAILED:\n  - zeta: x\n"
         "orphans found:\n", "an unrecognised header"),
        ("an in-scope NOTICE among other failures", "FAIL: 2 finding(s) - a copyright line is not the project's.\n"
         "  skills/zeta/cli/x.go: bad\n  skills/acme/cli/NOTICE: bad\n", "acme's NOTICE"),
        ("a truncated list", "check_copyright FAILED:\n  - zeta: x\n  ... and 3 more\n", "hidden findings"),
        ("a dotfile among other failures", "==> 2. No em-dash\nskills/zeta/README.md:1:x\n.gitignore:197:# "
         "skills/zeta y\nCI guards FAILED. Fix the items above.\n", "a repo dotfile"),
        ("nothing but headers", "Skill contract check FAILED:\n", "no attributable finding"),
    ):
        a = attribute(text, {"acme"}, known)
        expect(a["classification"] == "blocking", f"{name} -> blocking ({why})")

    print("pre_existing (the merge base must already fail the same way):")
    head = "Skill contract check FAILED:\n  - zeta: missing required file guide.md\n"
    expect(pre_existing(head, head, {"acme"}, known)["pre_existing"],
           "the base prints the same finding -> pre-existing")
    expect(pre_existing("check_no_todos FAILED:\n  skills/zeta/SKILL.md:625: [TODO] x\n",
                        "check_no_todos FAILED:\n  skills/zeta/SKILL.md:611: [TODO] x\n", {"acme"}, known)["pre_existing"],
           "the same finding at another line number -> pre-existing")
    r = pre_existing(head, "Skill contract check passed for 87 skill(s).\n", {"acme"}, known)
    expect(not r["pre_existing"] and r["new_lines"],
           "the base passes (this change broke zeta: a deleted asset, a tighter vocab.json) -> NEW, blocks")
    r = pre_existing(head + "  - yotta: missing x\n", head, {"acme"}, known | {"yotta"})
    expect(not r["pre_existing"], "one more out-of-scope finding than the base -> NEW, blocks")

    print("build_verdict:")
    with tempfile.TemporaryDirectory() as tmp:
        log_other = Path(tmp) / "other.log"
        log_other.write_text("Skill contract check FAILED:\n  - zeta: missing\n", encoding="utf-8")
        log_mine = Path(tmp) / "mine.log"
        log_mine.write_text("Skill contract check FAILED:\n  - acme: missing\n", encoding="utf-8")
        scope = {"affected": ["acme"], "full_fleet": False, "head_sha": "h", "dirty": False, "base_ref": "b",
                 "base_sha": "b", "merge_base": "m", "mode": "slug", "reason": "", "notes": [],
                 "connectors": ["acme"], "touched": ["acme"], "security_gate_slugs": ["acme"], "changed_files": 0}

        def rec(name: str, kind: str, slug: str, status: str, log: Path) -> dict:
            return {"name": name, "kind": kind, "slug": slug, "status": status, "seconds": 1.0, "log": str(log)}

        v = build_verdict(scope, [rec("build+vet", "connector", "acme", "pass", log_mine),
                                  rec("skill contract", "fleet", "", "out_of_scope", log_other)], 2, "t", known)
        expect(v["verdict"] == "pass" and v["out_of_scope_failures"][0]["slugs"] == ["zeta"],
               "a confirmed out-of-scope fleet failure -> PASS, listed under out_of_scope_failures")
        v = build_verdict(scope, [rec("skill contract", "fleet", "", "fail", log_other)], 2, "t", known)
        expect(v["verdict"] == "fail",
               "a fleet failure verify_all did NOT confirm as pre-existing -> FAIL, even if it names zeta")
        v = build_verdict(scope, [rec("skill contract", "fleet", "", "fail", log_mine)], 2, "t", known)
        expect(v["verdict"] == "fail" and v["blocking_failures"] == ["skill contract"],
               "this connector's fleet failure -> FAIL")
        v = build_verdict(scope, [rec("go test", "connector", "acme", "fail", log_mine)], 2, "t", known)
        expect(v["verdict"] == "fail", "a per-connector check failure -> FAIL")
        v = build_verdict(scope, [rec("plugin validate", "fleet", "zeta", "out_of_scope", log_other)], 2, "t", known)
        expect(v["verdict"] == "pass" and v["out_of_scope_failures"][0]["slugs"] == ["zeta"],
               "a per-skill fleet check for an out-of-scope skill -> listed, not blocking")
        v = build_verdict(scope, [rec("catalog idempotency", "repo", "", "fail", log_other)], 2, "t", known)
        expect(v["verdict"] == "fail", "a repo-level check failure -> FAIL")
        v = build_verdict(dict(scope, full_fleet=True), [rec("skill contract", "fleet", "", "out_of_scope", log_other)],
                          2, "t", known)
        expect(v["verdict"] == "fail" and not v["out_of_scope_failures"],
               "full fleet: every fleet failure blocks, nothing is out of scope")
        v = build_verdict(scope, [rec("shellcheck", "fleet", "", "warn", log_other)], 2, "t", known)
        expect(v["verdict"] == "pass" and v["counts"]["warn"] == 1, "a WARN is recorded, never blocking")

    print("resolve_scope (CI's classifier injected):")
    conn = connectors()
    some = sorted(conn)[0]
    mdonly = next((s for s in registry.skills() if registry.is_markdown_only(s)), None)
    real_cs, real_cc = release_matrix.change_set, release_matrix.classify_change
    try:
        def fake(files: list[str], rows: set[str] | None, why: str = "") -> None:
            release_matrix.change_set = lambda base, direct=False, worktree=False: ("mb", files)

            def cc(mb: str, f: list[str], new_rev: str | None = "HEAD", reasons: list | None = None) -> set | None:
                if rows is None and reasons is not None:
                    reasons.append(why)
                return rows
            release_matrix.classify_change = cc

        with contextlib.redirect_stderr(io.StringIO()):
            fake(["docs/reprint-survival.md"], set())
            sc = resolve_scope([], True, "HEAD")
            expect(sc["connectors"] == [] and not sc["full_fleet"] and sc["security_gate_slugs"] == [],
                   "--changed, docs only -> no connector, no security-gate slug")
            fake([f"skills/{conn[some]}/cli/internal/x.go"], {some})
            sc = resolve_scope([], True, "HEAD")
            expect(sc["connectors"] == [some] and sc["security_gate_slugs"] == [conn[some]],
                   f"--changed, one connector file -> exactly {some}, and the security gate scans it")
            fake([f"skills/{conn[some]}/README.md"], set())
            sc = resolve_scope([], True, "HEAD")
            expect(sc["connectors"] == [] and sc["security_gate_slugs"] == [conn[some]],
                   "--changed, a derived .mcpb pin re-mint -> no build row, but the security gate still "
                   "scans the slug (security-gate.yml scopes on any skills/<slug>/ path)")
            fake([".semgrep.yml"], set())
            sc = resolve_scope([], True, "HEAD")
            expect(sc["gate_logic_changed"] == [".semgrep.yml"] and sc["connectors"] == [],
                   "--changed, a security-gate LOGIC file -> recorded (CI's gate fails it by design)")
            fake([".semgrep.yml"], set())
            sc = resolve_scope([some], False, "HEAD")
            expect(sc["gate_logic_changed"] == [".semgrep.yml"], "--slug sees a gate-logic change in the branch too")
            fake(["tools/maintainer/check_mcp_gate.py"], None, "machinery changed (tools/maintainer/check_mcp_gate.py)")
            sc = resolve_scope([], True, "HEAD")
            expect(sc["full_fleet"] and sc["connectors"] == sorted(conn) and "machinery" in sc["reason"],
                   "--changed, shared machinery -> the FULL fleet, with CI's reason")
            release_matrix.change_set = lambda base, direct=False, worktree=False: None
            sc = resolve_scope([], True, "HEAD")
            expect(sc["full_fleet"], "--changed, the change set cannot be computed -> FULL fleet (fail open)")
            sc = resolve_scope([some], False, "HEAD")
            expect(sc["connectors"] == [some] and not sc["full_fleet"] and sc["security_gate_slugs"] == [conn[some]],
                   f"--slug {some} -> that connector only")
            if mdonly:
                sc = resolve_scope([mdonly], False, "HEAD")
                expect(sc["connectors"] == [] and mdonly in sc["affected"],
                       f"--slug {mdonly} (markdown-only) -> no Go build, still in scope for fleet checks")
            fake([], set())
            sc = resolve_scope([], False, "HEAD")
            expect(sc["full_fleet"] and sc["connectors"] == sorted(conn), "no arguments -> the full fleet")
            try:
                resolve_scope(["no-such-skill-zz"], False, "HEAD")
                expect(False, "an unknown --slug is refused")
            except SystemExit:
                expect(True, "an unknown --slug is refused")
    finally:
        release_matrix.change_set, release_matrix.classify_change = real_cs, real_cc

    print("newest_patch (setup-go's reading of a bare minor):")
    rel = [{"version": "go1.27.3", "stable": True}, {"version": "go1.26.10", "stable": True},
           {"version": "go1.26.9", "stable": True}, {"version": "go1.26rc1", "stable": False},
           {"version": "go1.26.11", "stable": False}, {"version": "go1.2.6", "stable": True}]
    expect(newest_patch("1.26", rel) == "go1.26.10", "1.26 -> go1.26.10 (newest stable patch, not an rc)")
    expect(newest_patch("1.25", rel) is None, "a minor with no release -> None (caller falls back)")
    expect(gate_go_spec() is not None, "security-gate.yml declares a go-version spec")
    rx = gate_logic_re()
    expect(all(rx.match(p) for p in GATE_LOGIC_FALLBACK) and rx.pattern != "^(" + "|".join(
        re.escape(p) for p in GATE_LOGIC_FALLBACK) + ")$",
        "security-gate.yml's own gate-logic pattern is read, and covers every gate-logic file")
    expect(not rx.match("tools/maintainer/security_suppressions.json") and not rx.match("skills/x/README.md"),
           "the gate-logic pattern does not fire on a data file or a skill")

    if failures:
        print(f"\nFAIL: verify_scope self-test - {len(failures)} case(s)")
        return 1
    print("\nPASS: verify_scope - scope, attribution and verdict each discriminate")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
