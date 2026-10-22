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
  cgo-context  Whether turning cgo on changes what Go compiles for the target
             in GOOS/GOARCH: every file list `go list -deps -test` reports, for
             the module's packages and every non-standard dependency. A check
             that ran with cgo off speaks for a cgo-on CI job only when nothing
             differs.
  path-collisions  Committed or working-tree paths that a case-insensitive or
             normalization-insensitive disk (macOS, Windows) folds into one.

    python3 tools/maintainer/verify_scope.py scope [--slug S]... [--changed] [--base REF]
    python3 tools/maintainer/verify_scope.py attribute --scope FILE --log FILE
    python3 tools/maintainer/verify_scope.py verdict --scope FILE --records FILE --out FILE
    python3 tools/maintainer/verify_scope.py cgo-context --dir skills/<dir>/cli
    python3 tools/maintainer/verify_scope.py path-collisions
    python3 tools/maintainer/verify_scope.py --self-test
"""

from __future__ import annotations

import argparse
import datetime as _dt
import json
import os
import re
import subprocess
import sys
import unicodedata
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
        # The scope step's Python: logic = re.compile(r"..." r"...", re.S)
        m = re.search(r'logic = re\.compile\(((?:\s*r"[^"\n]*")+)', text)
        if m:
            return re.compile("".join(re.findall(r'r"([^"\n]*)"', m.group(1))), re.S)
        m = re.search(r"grep -qE '(\^\(tools/maintainer/check_security_gate[^']*\)\$)'", text)  # before #421
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
    """CI's build matrix, read from the function ci.yml's prepare job uses
    (release_matrix.skill_entries): slug -> the folder under skills/ its row
    builds. Never filtered by what is on disk: a row whose module is missing
    fails in CI, and fails here."""
    return {e["name"]: e["dir"].split("/")[1] for e in release_matrix.skill_entries(with_assets=False)}


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
    snapshot = None
    if os.environ.get("VERIFY_ALL_REAL_HEAD"):
        # verify_all.sh runs on a clean checkout of the commit `git add -A`
        # would make; report the developer's own HEAD, and whether that
        # commit carries uncommitted changes.
        snapshot, head = head, os.environ["VERIFY_ALL_REAL_HEAD"]
        dirty = dirty or os.environ.get("VERIFY_ALL_DIRTY") == "1"
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
    branch_dirs = _touched_dirs(cs[1]) if cs is not None else set()
    if full_fleet:
        picked = set(conn)
    if slugs and not reason:
        reason = f"--slug: {', '.join(sorted(slugs))}"
    # The security gate's scope is the BRANCH diff, whatever build scope was
    # asked for: security-gate.yml scans every slug the PR touches.
    security = sorted(d for d in (touched | branch_dirs) if DIR_RE.match(d) and d in dirs)
    affected = set(dirs) if full_fleet else _aliases(touched | {conn[s] for s in picked} | picked)
    return {
        "schema": 1,
        "mode": mode,
        "base_ref": base,
        "base_sha": base_sha,
        "merge_base": mb,
        "head_sha": head,
        "dirty": dirty,
        "snapshot_sha": snapshot,
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
# `Release Notes.md: ...` or `README (/abs/README.md): ...` - a leading source
# that carries spaces, recognised by ending in a file extension right before
# the first ": " (so `Expected in NOTICE/LICENSE:` is not one).
_COLON_LOC = re.compile(rf"^([^:\n]*\.(?:{_EXT})\)?)(?::\s|:$)")
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
    m = _COLON_LOC.match(body)
    if m:
        return _location(m.group(1).strip())
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
    for line in log_lines(text):
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


def read_log(path: str | Path) -> str:
    """A check's output exactly as written: no newline translation, so a
    carriage return the check printed survives into the comparison."""
    try:
        # surrogateescape: lossless, so two different invalid bytes never
        # compare equal (errors="replace" mapped both to U+FFFD).
        with open(path, encoding="utf-8", errors="surrogateescape", newline="") as fh:
            return fh.read()
    except OSError:
        return ""


def log_lines(text: str) -> list[str]:
    """Split on \\n only (str.splitlines also splits on \\r and friends)."""
    lines = text.split("\n")
    return lines[:-1] if lines and lines[-1] == "" else lines


def _norm(line: str) -> str:
    """Identity, on purpose. The comparison is EXACT: every normalisation
    tried (line numbers, columns, whitespace) erased finding content in some
    format and let a changed failure pass as pre-existing. Exactness costs
    nothing real: a finding's line number moves only when its own file
    changed, and then the failure is plausibly this change's, so it blocks."""
    return line


def lines_pre_existing(head_text: str, base_text: str) -> bool:
    """Every non-blank line of head_text, as a multiset, already in base_text
    (location line numbers ignored). For per-skill checks with no
    attribution of their own."""
    from collections import Counter
    head = Counter(_norm(ln) for ln in log_lines(head_text))
    base = Counter(_norm(ln) for ln in log_lines(base_text))
    return all(base[k] >= n for k, n in head.items())


def pre_existing(head_text: str, base_text: str, affected: set[str], known: set[str]) -> dict:
    """Is a failure that names only out-of-scope connectors ALREADY on the
    base? Attribution alone cannot prove a failure is someone else's: deleting
    a connector's social image from docs/assets/, tightening vocab.json or a
    checker, or removing a file another skill links to all make OTHER
    connectors fail. So the exemption needs proof that does not depend on
    attribution at all: EVERY line the check printed at HEAD (headers,
    findings, continuations, summaries) must already be in the base's output,
    byte for byte, counted as a multiset. Anything new, anywhere, blocks."""
    from collections import Counter

    base = Counter(log_lines(base_text))
    new = []
    for ln in log_lines(head_text):
        if base[ln] > 0:
            base[ln] -= 1
        else:
            new.append(ln)   # blank and whitespace-only lines are lines too
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
        if r["status"] == "pass":
            # Reported, not blocking: ledgered Windows failures the lint still
            # sees, and -pp- leaks the ratchet grandfathers. Kept visible.
            known_lines = [ln for ln in _tail(r["log"], 400) if _KNOWN.search(ln)]
            if known_lines:
                c["known_findings"] = known_lines[:40]
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
        "snapshot_sha": scope.get("snapshot_sha"),
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


_KNOWN = re.compile(r"\(known Windows failure, ledgered\)|\(pre-existing at base, not blocking\)")


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


_SETUP_GO_MANIFEST = "https://raw.githubusercontent.com/actions/go-versions/main/versions-manifest.json"
_GO_DEV_INDEX = "https://go.dev/dl/?mode=json&include=all"


_PIN_KEYS = ("SCANNER_GO", "GOSEC_VERSION", "GOVULNCHECK_VERSION", "OSV_SCANNER_VERSION", "SEMGREP_VERSION",
             "GOSEC_SHA256", "GOVULNCHECK_SHA256", "OSV_SCANNER_SHA256")


def scanner_pins(text: str | None = None) -> dict[str, str]:
    """security-gate.yml's scanner pins (its top-level env block), the single
    source both CI and verify_all.sh build the scanners from. Every key must be
    present with a well-formed value, else {} (the caller fails closed)."""
    if text is None:
        try:
            text = (ROOT / ".github/workflows/security-gate.yml").read_text(encoding="utf-8")
        except OSError:
            return {}
    out = {}
    for k in _PIN_KEYS:
        m = re.search(rf'^  {k}:\s*"([^"\n]+)"\s*$', text, re.M)
        ok = m and (re.fullmatch(r"[0-9a-f]{64}", m.group(1)) if k.endswith("SHA256")
                    else re.fullmatch(r"v?[0-9]+(?:\.[0-9]+)*", m.group(1)))
        if not ok:
            return {}
        out[k] = m.group(1)
    return out


def go_releases(path: str | None = None) -> list[dict]:
    """The release index setup-go resolves a version spec against: its own
    manifest (actions/go-versions), else go.dev's, as [{"version": "goX.Y.Z",
    "stable": bool}]; from a file a previous call saved, or fetched. [] when
    neither is reachable."""
    def norm(data: object) -> list[dict]:
        out = []
        for r in data if isinstance(data, list) else []:
            v = r.get("version", "") if isinstance(r, dict) else ""
            if isinstance(v, str) and v:
                out.append({"version": v if v.startswith("go") else "go" + v, "stable": bool(r.get("stable"))})
        return out
    if path:
        try:
            return norm(json.loads(Path(path).read_text(encoding="utf-8")))
        except (OSError, ValueError):
            return []
    import urllib.request
    for url in (_SETUP_GO_MANIFEST, _GO_DEV_INDEX):
        try:
            with urllib.request.urlopen(url, timeout=20) as resp:
                rel = norm(json.load(resp))
        except (OSError, ValueError):
            continue
        if rel:
            return rel
    return []


def setup_go_installs(spec: str, releases: list[dict]) -> str | None:
    """The Go actions/setup-go@v5 installs for a version spec: an exact X.Y.Z
    as written, a bare X.Y as its newest stable patch. The go command then
    selects the toolchain itself (GOTOOLCHAIN=auto: a newer `toolchain` line
    in go.mod or go.work upgrades it), so callers run go with
    GOTOOLCHAIN=<this>+auto, which is that same selection."""
    if re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", spec):
        return "go" + spec
    if re.fullmatch(r"[0-9]+\.[0-9]+", spec):
        return newest_patch(spec, releases)
    return None


_GOMOD_GO = re.compile(r"^go ([0-9]+(?:\.[0-9]+)*)", re.M)  # setup-go's own parse of go-version-file


def ci_go(gomod: str, releases: list[dict]) -> str | None:
    """What CI's build and windows rows install for a connector
    (setup-go `go-version-file: <its go.mod>`, which reads the `go` line only)."""
    try:
        m = _GOMOD_GO.search(Path(gomod).read_text(encoding="utf-8"))
    except OSError:
        return None
    return setup_go_installs(m.group(1), releases) if m else None


def gate_go(releases: list[dict] | None = None) -> str | None:
    """What security-gate.yml's setup-go installs (`go-version: "1.26"`): its
    newest patch. None when the release index is unreachable; the caller then
    fails closed, since which Go CI scans with is unknown."""
    spec = gate_go_spec()
    if not spec:
        return None
    return setup_go_installs(spec, go_releases() if releases is None else releases)


# --------------------------------------------------------------------------- #
# Build contexts and disks
# --------------------------------------------------------------------------- #


def go_list(d: str, cgo: str) -> tuple[dict[str, dict], str]:
    """`go list -e -json -deps -test ./...` in d with CGO_ENABLED=cgo (the rest
    of the environment, GOOS/GOARCH and the toolchain, is the caller's). Every
    package by import path, or an error when go listed nothing."""
    p = subprocess.run(["go", "list", "-e", "-json", "-deps", "-test", "./..."], cwd=d,
                       env=dict(os.environ, CGO_ENABLED=cgo), capture_output=True, text=True)
    pkgs: dict[str, dict] = {}
    dec, s, i = json.JSONDecoder(), p.stdout, 0
    try:
        while True:
            while i < len(s) and s[i].isspace():
                i += 1
            if i >= len(s):
                break
            obj, i = dec.raw_decode(s, i)
            pkgs[obj.get("ImportPath", "")] = obj
    except ValueError:
        return {}, "go list printed unparseable output"
    if not pkgs:
        return {}, (p.stderr.strip() or f"go list exited {p.returncode} and listed no package")
    return pkgs, ""


def build_inputs(pkgs: dict[str, dict]) -> dict[str, dict]:
    """What the compiler is handed per package: every file list go reports
    (each key ending in "Files": Go, cgo, assembly, C, syso, embed, test and
    ignored files, including kinds added later) and any load error. The
    standard library is left out: the same Go release builds it either way and
    its API does not change with cgo."""
    out: dict[str, dict] = {}
    for path, p in pkgs.items():
        if p.get("Standard"):
            continue
        rec = {k: sorted(v) for k, v in p.items() if k.endswith("Files") and isinstance(v, list)}
        rec["Error"] = json.dumps([p.get("Error"), p.get("DepsErrors")], sort_keys=True)
        out[path] = rec
    return out


def context_diff(off: dict[str, dict], on: dict[str, dict]) -> list[str]:
    """Every difference between the two contexts' build inputs, both ways."""
    lines: list[str] = []
    for path in sorted(set(off) | set(on)):
        if path not in on or path not in off:
            lines.append(f"  {path}: built only with cgo {'off' if path not in on else 'on'}")
            continue
        a, b = off[path], on[path]
        for k in sorted(set(a) | set(b)):
            if a.get(k) == b.get(k):
                continue
            if k == "Error":
                lines.append(f"  {path}: loads differently with cgo on and off")
                continue
            for f in sorted(set(b.get(k) or []) - set(a.get(k) or [])):
                lines.append(f"  {path}: {k} {f} only with cgo on")
            for f in sorted(set(a.get(k) or []) - set(b.get(k) or [])):
                lines.append(f"  {path}: {k} {f} only with cgo off")
    return lines


def listing_errors(pkgs: dict[str, dict]) -> list[str]:
    """Packages go could not load completely: a comparison over them proves nothing."""
    out = []
    for path, p in sorted(pkgs.items()):
        for e in [p.get("Error")] + list(p.get("DepsErrors") or []):
            if e:
                out.append(f"  {path}: {e.get('Err', e) if isinstance(e, dict) else e}")
    return out


def cgo_context(d: str) -> int:
    off, err_off = go_list(d, "0")
    on, err_on = go_list(d, "1")
    if err_off or err_on:
        print(err_off or err_on)
        return 2
    errs = listing_errors(off) + listing_errors(on)
    if errs:
        print("go could not load every package, so the two contexts cannot be compared:")
        print("\n".join(sorted(set(errs))))
        return 2
    diff = context_diff(build_inputs(off), build_inputs(on))
    if diff:
        print("turning cgo on changes what Go compiles here:")
        print("\n".join(diff))
        return 1
    print(f"cgo on and cgo off compile the same files ({len(build_inputs(on))} packages)")
    return 0


def ci_gowork(d: str, root: Path = ROOT) -> str:
    """The workspace file Go would use for module folder d in CI's checkout:
    the nearest go.work at or above d inside the repository that is in that
    checkout (committed, or new and not ignored; .gitignore lists go.work), as
    a path relative to the repository root, else "off". A go.work above the repository, or an
    ignored one, is never in CI's checkout, so GOWORK must not discover it."""
    root = root.resolve()
    cur = (root / d).resolve()
    if cur != root and root not in cur.parents:
        return "off"
    while True:
        cand = cur / "go.work"
        if cand.is_file():
            rel = cand.relative_to(root).as_posix()
            p = subprocess.run(["git", "ls-files", "-c", "-o", "--exclude-standard", "--", rel],
                               cwd=root, capture_output=True, text=True)
            if p.returncode != 0:
                raise OSError(f"git ls-files failed: {p.stderr.strip()}")
            if p.stdout.strip():
                return rel
        if cur == root:
            return "off"
        cur = cur.parent


def windows_tree(slug: str, out: str, root: Path = ROOT) -> str:
    """CI's Windows job's checkout, built at out: actions/checkout with a
    cone-mode sparse-checkout of skills/<slug> and tools/maintainer, which also
    holds the files directly in each cone's ancestors (the root, skills/ and
    tools/). Copied from this
    checkout (committed, staged, unstaged and new unignored files; nothing
    ignored), so a go.work, or a replace directive, that reaches outside that
    tree fails here as it does there. Returns the go.work Go finds for
    skills/<slug>/cli inside the tree (relative to it), or "off"."""
    cone = [f"skills/{slug}", "tools/maintainer"]
    # Cone mode also holds the files directly in every ancestor of a cone.
    tops = {"/".join(c.split("/")[:n]) for c in cone for n in range(len(c.split("/")))}
    spec = cone + [":(glob)" + (f"{t}/*" if t else "*") for t in sorted(tops)]
    p = subprocess.run(["git", "ls-files", "-z", "-c", "-o", "--exclude-standard", "--", *spec],
                       cwd=root, capture_output=True)
    if p.returncode != 0:
        raise OSError(f"git ls-files failed: {p.stderr.decode(errors='replace').strip()}")
    import shutil
    dst = Path(out)
    for raw in sorted(set(p.stdout.split(b"\0"))):
        if not raw:
            continue
        rel = os.fsdecode(raw)
        src = root / rel
        if not (src.is_file() or src.is_symlink()):
            continue  # deleted in the working tree
        (dst / rel).parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(src, dst / rel, follow_symlinks=False)
    cur = dst / "skills" / slug / "cli"
    while True:
        if (cur / "go.work").is_file():
            return (cur / "go.work").relative_to(dst).as_posix()
        if cur == dst:
            return "off"
        cur = cur.parent


def _fold(p: str) -> str:
    return unicodedata.normalize("NFD", p).casefold()


def path_collisions(paths: list[str]) -> list[list[str]]:
    """Groups of distinct paths (or parent folders) that one folded name
    covers on a case- or normalization-insensitive disk."""
    groups: dict[str, set[str]] = {}
    for p in paths:
        parts = p.split("/")
        for n in range(1, len(parts) + 1):
            pre = "/".join(parts[:n])
            groups.setdefault(_fold(pre), set()).add(pre)
    return sorted(sorted(g) for g in groups.values() if len(g) > 1)


_WIN_RESERVED = ({"AUX", "CON", "CONIN$", "CONOUT$", "NUL", "PRN"}
                 | {f"{d}{n}" for d in ("COM", "LPT") for n in list("123456789") + ["\u00b9", "\u00b2", "\u00b3"]})


def windows_invalid(paths: list[str]) -> list[str]:
    """Paths Git for Windows refuses to check out (its is_valid_win32_path):
    a reserved device name in any component, with or without an extension
    (aux.go, CON, com1.txt), an NTFS alias of .git (git~1), a component
    ending in a space or a dot, or one of
    < > : " | ? * \\ or a control character. CI's Windows job could not
    materialize them, so no copy of its checkout here can either."""
    bad = []
    for p in paths:
        for c in p.split("/"):
            if c in (".", ".."):
                continue
            stem, low = c.split(".", 1)[0].rstrip(" "), c.lower()
            # core.protectNTFS (on by default for Git for Windows): the NTFS
            # aliases of .git are refused too (git~1, .git. , .git::$INDEX_ALLOCATION).
            ntfs_dotgit = low.rstrip(". ") == ".git" or low == "git~1" or low.startswith(".git::$index_allocation")
            if (stem.upper() in _WIN_RESERVED or c.endswith((" ", ".")) or ntfs_dotgit
                    or any(ch in '<>:"|?*\\' or ord(ch) < 32 for ch in c)):
                bad.append(p)
                break
    return bad


def repo_paths() -> list[str]:
    out: set[str] = set()
    for args in (["ls-tree", "-r", "-z", "--name-only", "HEAD"], ["ls-files", "-z", "-c", "-o", "--exclude-standard"]):
        p = subprocess.run(["git", *args], cwd=ROOT, capture_output=True)
        if p.returncode != 0:
            raise OSError(f"git {' '.join(args)} failed: {p.stderr.decode(errors='replace').strip()}")
        out |= {x.decode("utf-8", "surrogateescape") for x in p.stdout.split(b"\0") if x}
    return sorted(out)


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
    vp.add_argument("--expect", type=int, default=None,
                    help="how many check results the caller wrote; any other count fails the verdict")
    vp.add_argument("--record-error", action="store_true",
                    help="the caller could not write a check result; fails the verdict")
    gg = sub.add_parser("gate-go")
    gg.add_argument("--releases")
    gr = sub.add_parser("go-releases")
    gr.add_argument("--out", required=True)
    cg = sub.add_parser("ci-go")
    cg.add_argument("--gomod", action="append", required=True)
    cg.add_argument("--releases")
    cp = sub.add_parser("cgo-context")
    cp.add_argument("--dir", required=True)
    sub.add_parser("path-collisions")
    gw = sub.add_parser("gowork")
    gw.add_argument("--dir", required=True)
    sub.add_parser("scanner-pins")
    wt = sub.add_parser("windows-tree")
    wt.add_argument("--slug", required=True)
    wt.add_argument("--out", required=True)
    bp = sub.add_parser("baseline")
    bp.add_argument("--scope", required=True)
    bp.add_argument("--head-log", required=True)
    bp.add_argument("--base-log", required=True)
    bp.add_argument("--all-lines", action="store_true",
                    help="compare every line (a per-skill check), not only attributed findings")
    a = ap.parse_args(argv)
    if a.cmd == "gate-go":
        print(gate_go(go_releases(a.releases) if a.releases else None) or "")
        return 0
    if a.cmd == "go-releases":
        rel = go_releases()
        if not rel:
            print("could not read setup-go's release manifest or go.dev's index", file=sys.stderr)
            return 1
        Path(a.out).write_text(json.dumps(rel), encoding="utf-8")
        return 0
    if a.cmd == "ci-go":
        rel = go_releases(a.releases)
        bad = 0
        for g in a.gomod:
            v = ci_go(g, rel)
            bad |= v is None
            print(f"{v or '-'}\t{g}")
        return 1 if bad else 0
    if a.cmd == "cgo-context":
        return cgo_context(a.dir)
    if a.cmd == "gowork":
        try:
            print(ci_gowork(a.dir))
        except OSError as e:
            print(e, file=sys.stderr)
            return 2
        return 0
    if a.cmd == "scanner-pins":
        pins = scanner_pins()
        if not pins:
            print("security-gate.yml does not declare every scanner pin", file=sys.stderr)
            return 1
        print("\n".join(f"{k}={v}" for k, v in pins.items()))
        return 0
    if a.cmd == "windows-tree":
        if not DIR_RE.match(a.slug):
            print(f"{a.slug!r} is not a skill slug", file=sys.stderr)
            return 2
        try:
            print(windows_tree(a.slug, a.out))
        except OSError as e:
            print(e, file=sys.stderr)
            return 2
        return 0
    if a.cmd == "path-collisions":
        try:
            paths = repo_paths()
        except OSError as e:
            print(e)
            return 2
        groups, bad = path_collisions(paths), windows_invalid(paths)
        for g in groups:
            print("  one file on a case-insensitive disk (macOS, Windows): " + "  ".join(g))
        for b in bad:
            print(f"  {b}: Windows cannot check this path out (a reserved device name, a trailing space or dot,"
                  " or a character Windows forbids)")
        if groups or bad:
            print("rename these: a macOS or Windows checkout cannot hold them as committed, so no check run")
            print("here (and not CI's Windows job) sees what Linux sees")
            return 1
        print("every path checks out as committed on Linux, macOS and Windows")
        return 0
    if a.cmd == "baseline":
        sc = json.loads(Path(a.scope).read_text(encoding="utf-8"))
        if a.all_lines:
            ok = lines_pre_existing(read_log(a.head_log), read_log(a.base_log))
            print(json.dumps({"pre_existing": ok}))
            return 0 if ok else 1
        r = pre_existing(read_log(a.head_log), read_log(a.base_log),
                         set(sc["affected"]), known_names())
        print(json.dumps(r, indent=1))
        return 0 if r["pre_existing"] else 1
    if a.cmd == "scope":
        print(json.dumps(resolve_scope(a.slug, a.changed, a.base), indent=1))
        return 0
    scope = json.loads(Path(a.scope).read_text(encoding="utf-8"))
    if a.cmd == "attribute":
        text = read_log(a.log)
        print(json.dumps(attribute(text, set(scope["affected"]), known_names()), indent=1))
        return 0
    records = read_records(Path(a.records))
    if a.record_error or (a.expect is not None and len(records) != a.expect):
        # A result that never reached the record file is not a pass.
        records.append({"name": f"verify harness: {a.expect} check result(s) written, {len(records)} recorded"
                        + (", and a write failed" if a.record_error else ""),
                        "kind": "repo", "slug": "", "status": "fail", "seconds": 0.0, "log": "/dev/null"})
    v = build_verdict(scope, records, a.seconds, a.started)
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
        ("Release Notes.md: broken local link -> skills/zeta/missing.md", "<repo>"),
        ("  - README (/abs/r/README.md): the catalog marker pair is missing", "<repo>"),
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

    print("pre_existing (the merge base must already print every line):")
    linkfail = "Markdown local-link check FAILED:\n  - skills/zeta/README.md: broken local link -> missing.md\n"
    expect(not pre_existing(linkfail + " \nSee new description\n", linkfail + "See new description\n",
                            {"acme"}, known)["pre_existing"], "an extra whitespace-only line -> new")
    expect(not pre_existing(linkfail + "See new description\n", linkfail, {"acme"}, known)["pre_existing"],
           "a changed continuation line (text the base never printed) -> new, even if it reads like a hint")
    with tempfile.TemporaryDirectory() as tmp3:
        a_, b_ = Path(tmp3) / "a.log", Path(tmp3) / "b.log"
        a_.write_bytes(b"skills/zeta/README.md:1:x \xff\n")
        b_.write_bytes(b"skills/zeta/README.md:1:x \xfe\n")
        expect(not pre_existing(read_log(a_), read_log(b_), {"acme"}, known)["pre_existing"],
               "two different invalid bytes stay different (lossless decoding)")
    head = "Skill contract check FAILED:\n  - zeta: missing required file guide.md\n"
    expect(pre_existing(head, head, {"acme"}, known)["pre_existing"],
           "the base prints the same finding -> pre-existing")
    expect(not pre_existing("check_no_todos FAILED:\n  skills/zeta/SKILL.md:625: [TODO] x\n",
                            "check_no_todos FAILED:\n  skills/zeta/SKILL.md:611: [TODO] x\n", {"acme"}, known)["pre_existing"],
           "the same finding at another line number -> new (its file changed; the comparison is exact)")
    r = pre_existing(head, "Skill contract check passed for 87 skill(s).\n", {"acme"}, known)
    expect(not r["pre_existing"] and r["new_lines"],
           "the base passes (this change broke zeta: a deleted asset, a tighter vocab.json) -> NEW, blocks")
    r = pre_existing(head + "  - yotta: missing x\n", head, {"acme"}, known | {"yotta"})
    expect(not r["pre_existing"], "one more out-of-scope finding than the base -> NEW, blocks")
    expect(not pre_existing("check_no_todos FAILED:\n  skills/zeta/SKILL.md:625: [TODO] TODO ticket:101\n",
                            "check_no_todos FAILED:\n  skills/zeta/SKILL.md:625: [TODO] TODO ticket:100\n",
                            {"acme"}, known)["pre_existing"],
           "a number INSIDE a finding is content: ticket:100 -> ticket:101 is new")
    expect(not pre_existing("x\nskills/zeta/README.md:12:101: a - b\n", "x\nskills/zeta/README.md:12:100: a - b\n",
                            {"acme"}, known)["pre_existing"],
           "grep output: digits right after path:line are the file's text, not a column -> new")
    expect(not pre_existing("x\nskills/zeta/README.md:12:100: warning: a - b\n",
                            "x\nskills/zeta/README.md:12:101: warning: a - b\n", {"acme"}, known)["pre_existing"],
           "digits that only LOOK like a column are content -> new")
    expect(not pre_existing("x\nskills/zeta/README.md:3:a  - b\n", "x\nskills/zeta/README.md:3:a - b\n",
                            {"acme"}, known)["pre_existing"], "a whitespace change inside a finding -> new")
    expect(not pre_existing("x\nskills/zeta/README.md:3:a - b\r\n", "x\nskills/zeta/README.md:3:a - b\n",
                            {"acme"}, known)["pre_existing"], "a line that gained a carriage return -> new")
    with tempfile.TemporaryDirectory() as tmp2:
        crlf = Path(tmp2) / "crlf.log"
        crlf.write_bytes(b"skills/zeta/README.md:3:a - b\r\n")
        expect(read_log(crlf).endswith("\r\n"), "logs are read without newline translation")
    expect(not lines_pre_existing("plugin zeta: bad field x\nplugin zeta: bad field x\n", "plugin zeta: bad field x\n"),
           "per-skill lines are a multiset too")
    one = "check_install_docs FAILED:\n  - R1 skills/zeta/mcp-install.md:10: names `mcp-remote`\n"
    two = one + "  - R1 skills/zeta/mcp-install.md:20: names `mcp-remote`\n"
    expect(not pre_existing(two, one, {"acme"}, known)["pre_existing"],
           "a SECOND occurrence of a finding the base has once -> NEW, blocks (counts, not a set)")
    expect(pre_existing(two, two, {"acme"}, known)["pre_existing"], "the same two occurrences on the base -> pre-existing")

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
        log_known = Path(tmp) / "known.log"
        log_known.write_text("skills/acme/cli/x_test.go:9: [acme] mode-bits in TestX: y (known Windows failure, "
                             "ledgered)\ncheck_unix_test_asserts: 1 connector(s), 0 new finding(s)\n", encoding="utf-8")
        v = build_verdict(scope, [rec("unix-only test lint", "connector", "acme", "pass", log_known)], 2, "t", known)
        expect(v["verdict"] == "pass" and len(v["checks"][0].get("known_findings", [])) == 1,
               "a passing check keeps its ledgered known findings in the verdict")

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
            other = sorted(conn)[1]
            fake([f"skills/{conn[other]}/cli/internal/x.go"], {other})
            sc = resolve_scope([some], False, "HEAD")
            expect(sc["connectors"] == [some] and sc["security_gate_slugs"] == sorted({conn[some], conn[other]}),
                   "--slug: builds only the named connector, but the security gate scans everything the branch "
                   "touched (as CI does)")
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

    print("verdict inventory:")
    with tempfile.TemporaryDirectory() as td:
        sc, rc_, out = Path(td) / "scope.json", Path(td) / "records.tsv", Path(td) / "v.json"
        sc.write_text(json.dumps({"affected": [], "full_fleet": False, "head_sha": "h", "dirty": False,
                                  "base_ref": "b", "base_sha": "b", "merge_base": "m", "mode": "slug", "reason": "",
                                  "notes": [], "connectors": [], "touched": [], "security_gate_slugs": [],
                                  "changed_files": 0}), encoding="utf-8")
        rc_.write_text("a\trepo\t\tpass\t1\t/dev/null\n", encoding="utf-8")
        args = ["verdict", "--scope", str(sc), "--records", str(rc_), "--out", str(out)]
        with contextlib.redirect_stdout(io.StringIO()):
            ok_rc, lost_rc, err_rc = (main(args + ["--expect", "1"]), main(args + ["--expect", "2"]),
                                      main(args + ["--expect", "1", "--record-error"]))
        expect(ok_rc == 0, "every written result recorded -> the verdict stands")
        expect(lost_rc == 1, "a result written but missing from the record file -> FAIL")
        expect(err_rc == 1, "a result that could not be written -> FAIL")

    print("cgo context (build inputs, both directions):")
    def pk(files=None, **kw):
        d = {"ImportPath": "m/p", "GoFiles": files or ["a.go"]}
        d.update(kw)
        return d
    base_pk = {"m/p": pk(), "fmt": {"ImportPath": "fmt", "Standard": True, "GoFiles": ["x.go"]}}
    same = build_inputs(base_pk)
    expect(context_diff(same, build_inputs(base_pk)) == [], "identical inputs -> no difference")
    expect(context_diff(same, build_inputs({"m/p": pk(["a.go", "c_windows.go"])})) ==
           ["  m/p: GoFiles c_windows.go only with cgo on"], "a file only cgo-on compiles is found")
    expect(context_diff(build_inputs({"m/p": pk(["a.go", "stub.go"])}), same) ==
           ["  m/p: GoFiles stub.go only with cgo off"], "a file only cgo-OFF compiles is found too")
    expect(any("SFiles" in x for x in context_diff(same, build_inputs({"m/p": pk(SFiles=["x_amd64.s"])}))),
           "assembly (and any other *Files list) is compared")
    expect(context_diff(same, build_inputs({**base_pk, "dep/q": {"ImportPath": "dep/q", "GoFiles": ["q.go"]}})) ==
           ["  dep/q: built only with cgo on"], "a dependency package only one context builds is found")
    expect(context_diff(same, build_inputs({"m/p": pk(Error={"Err": "boom"})})) ==
           ["  m/p: loads differently with cgo on and off"], "a load error in one context is a difference")
    expect(context_diff(same, build_inputs({**base_pk, "fmt": {"ImportPath": "fmt", "Standard": True,
                                                                "GoFiles": ["x.go", "cgo.go"]}})) == [],
           "the standard library is left out (same release, same API either way)")
    print("CI's workspace file:")
    with tempfile.TemporaryDirectory() as td:
        outer = Path(td) / "outer"
        repo = outer / "repo"
        (repo / "skills/x/cli").mkdir(parents=True)
        subprocess.run(["git", "init", "-q", str(repo)], check=True)
        (repo / ".gitignore").write_text("go.work\n", encoding="utf-8")
        (outer / "go.work").write_text("go 1.26\n", encoding="utf-8")
        expect(ci_gowork("skills/x/cli", repo) == "off", "a go.work ABOVE the repository is not CI's -> off")
        (repo / "go.work").write_text("go 1.26\n", encoding="utf-8")
        expect(ci_gowork("skills/x/cli", repo) == "off", "an IGNORED go.work in the repository is not CI's -> off")
        (repo / "skills/go.work").write_text("go 1.26\n", encoding="utf-8")
        subprocess.run(["git", "-C", str(repo), "add", "-f", "skills/go.work"], check=True)
        expect(ci_gowork("skills/x/cli", repo) == "skills/go.work",
               "a committed go.work above the module is CI's")

    pins = scanner_pins()
    expect(pins.get("GOSEC_VERSION", "").startswith("v") and len(pins) == len(_PIN_KEYS),
           "security-gate.yml declares every scanner pin")
    expect(scanner_pins('  GOSEC_VERSION: "v2.29.0"\n') == {}, "a missing pin is refused, never defaulted")

    print("CI's sparse Windows checkout:")
    with tempfile.TemporaryDirectory() as td:
        repo, out = Path(td) / "repo", Path(td) / "win"
        for f in ("README.md", "skills/list.md", "skills/x/cli/go.mod", "skills/x/cli/a.go", "skills/y/cli/go.mod",
                  "tools/maintainer/t.py", "tools/go.mod", "tools/other/o.py", "docs/d.md", "skills/x/cli/ignored.log"):
            (repo / f).parent.mkdir(parents=True, exist_ok=True)
            (repo / f).write_text("x\n", encoding="utf-8")
        (repo / ".gitignore").write_text("*.log\n", encoding="utf-8")
        subprocess.run(["git", "init", "-q", str(repo)], check=True)
        work = windows_tree("x", str(out), repo)
        got = sorted(p.relative_to(out).as_posix() for p in out.rglob("*") if p.is_file())
        expect(got == [".gitignore", "README.md", "skills/list.md", "skills/x/cli/a.go", "skills/x/cli/go.mod",
                       "tools/go.mod", "tools/maintainer/t.py"] and work == "off",
               "the two cones and the files directly in root, skills/ and tools/; never a sibling, docs/ or an ignored file")

    expect(listing_errors({"m/p": pk(Error={"Err": "cannot find package"})}) != [] and
           listing_errors({"m/p": pk(DepsErrors=[{"Err": "missing"}])}) != [] and listing_errors(base_pk) == [],
           "a package go could not load (itself or a dependency) is an error, never a match")

    print("path collisions:")
    expect(path_collisions(["a/Case_linux.go", "a/case_linux.go"]) == [["a/Case_linux.go", "a/case_linux.go"]],
           "two files that differ only by case collide")
    expect(path_collisions(["internal/Foo/a.go", "internal/foo/b.go"]) == [["internal/Foo", "internal/foo"]],
           "two folders that differ only by case collide (one package on macOS, two on Linux)")
    expect(path_collisions(["caf\u00e9.md", "cafe\u0301.md"]) == [["cafe\u0301.md", "caf\u00e9.md"]],
           "NFC and NFD spellings of one name collide")
    expect(path_collisions(["a/x.go", "a/y.go", "b/x.go"]) == [], "distinct paths do not")
    expect(windows_invalid(["cli/internal/cli/aux.go", "x/CON", "y/com1.txt", "z/nul .md", "a/b./c", "q/a:b",
                            "t/x "]) == ["cli/internal/cli/aux.go", "x/CON", "y/com1.txt", "z/nul .md", "a/b./c",
                                         "q/a:b", "t/x "], "every Windows-reserved shape is refused")
    expect(windows_invalid(["assets/git~1/example.txt", "a/GIT~1", "b/.git::$INDEX_ALLOCATION/x"]) ==
           ["assets/git~1/example.txt", "a/GIT~1", "b/.git::$INDEX_ALLOCATION/x"], "NTFS aliases of .git are refused")
    expect(windows_invalid(["auxiliary.go", "console/x.go", "com10.go", "./a/../b.go", "nulls.md", "git~2/x",
                            ".github/x.yml", ".gitignore"]) == [],
           "names that only start like a device name are fine")

    print("newest_patch (setup-go's reading of a bare minor):")
    rel = [{"version": "go1.27.3", "stable": True}, {"version": "go1.26.10", "stable": True},
           {"version": "go1.26.9", "stable": True}, {"version": "go1.26rc1", "stable": False},
           {"version": "go1.26.11", "stable": False}, {"version": "go1.2.6", "stable": True}]
    expect(newest_patch("1.26", rel) == "go1.26.10", "1.26 -> go1.26.10 (newest stable patch, not an rc)")
    expect(newest_patch("1.25", rel) is None, "a minor with no release -> None (caller falls back)")
    expect(setup_go_installs("1.26.9", rel) == "go1.26.9", "an exact go line installs exactly that patch")
    expect(setup_go_installs("1.26", rel) == "go1.26.10", "a bare go line installs its newest patch")
    expect(setup_go_installs("1.26", []) is None, "a bare go line with no release index is unknown (fail closed)")
    with tempfile.TemporaryDirectory() as td:
        gm = Path(td) / "go.mod"
        gm.write_text("module m\n\ngo 1.26\n\ntoolchain go1.26.9\n", encoding="utf-8")
        expect(ci_go(str(gm), rel) == "go1.26.10",
               "setup-go reads the go line, not the toolchain line (1.26 -> go1.26.10 despite toolchain go1.26.9)")
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
