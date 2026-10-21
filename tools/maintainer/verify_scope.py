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
    if changed or full_fleet:
        cs = release_matrix.change_set(base, worktree=True) if base_sha else None
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
    }


# --------------------------------------------------------------------------- #
# Attribution of a failed fleet-wide check
# --------------------------------------------------------------------------- #

_PATH = re.compile(r"(?<![\w/.-])((?:[\w.-]+/)*[\w-][\w.-]*\.(?:md|json|ya?ml|sh|ps1|py|txt|toml|go|mod|sum|"
                   r"html|png|jpe?g|gif|webp|mp4|svg|xml|csv))(?::\d+)?")
_BRACKET = re.compile(r"\[([a-z0-9_][a-z0-9-]*)\]")
_MARKERS = re.compile(r"^\s*(?:(?:[-*]|FAIL:?|WARN:?|ERROR:?|\[BLOCK\])\s+)*")
_LEAD = re.compile(r"([a-z0-9_][a-z0-9-]*):\s")


def _skill_of(path: str) -> str | None:
    parts = path.split("/")
    if len(parts) >= 3 and parts[0] == "skills":
        return parts[1]
    if len(parts) == 3 and parts[0] == "docs" and parts[1] == "skills":
        return parts[2].rsplit(".", 1)[0]
    return None


def attribute_line(line: str, known: set[str]) -> str | None:
    """The skill (dir or slug) a failure line is about, "<repo>" when the line
    is about a repo file outside any skill, or None when it names no location
    (a header, a summary, a hint).

    Precedence: an explicit `[slug]` tag (the new checks print one); else the
    earliest of a leading `slug: ` and a skills/<dir>/ or docs/skills/<slug>.md
    path anywhere in the line; else a repo path, but only as the line's
    SUBJECT (at its start: `README.md:12: ...`). A repo path mid-sentence is a
    reference ("See docs/reprint-survival.md"), not the place that failed."""
    m = _BRACKET.search(line)
    if m and m.group(1) in known:
        return m.group(1)
    body = line[_MARKERS.match(line).end():]
    start = len(line) - len(body)
    cands: list[tuple[int, str]] = []
    m = _LEAD.match(body)
    if m and m.group(1) in known:
        cands.append((start, m.group(1)))
    for pm in _PATH.finditer(line):
        who = _skill_of(pm.group(1))
        if who is not None:
            cands.append((pm.start(1), who))
            break
    if cands:
        return min(cands)[1]
    pm = _PATH.match(body)
    if pm:
        return "<repo>"
    return None


def attribute(text: str, affected: set[str], known: set[str]) -> dict:
    blocking, other, unknown = [], [], []
    for line in text.splitlines():
        who = attribute_line(line, known)
        if who is None:
            continue
        if who == "<repo>" or who in affected:
            blocking.append(line)
        elif who in known:
            other.append((who, line))
        else:
            unknown.append(line)
    if blocking or unknown or not other:
        why = ("names an in-scope connector or a repo-level file" if blocking or unknown
               else "names no file or connector, so it cannot be attributed (fail closed)")
        return {"classification": "blocking", "why": why, "slugs": [], "lines": (blocking + unknown)[:40]}
    return {"classification": "out_of_scope", "why": "every failure names a connector outside the scope",
            "slugs": sorted({w for w, _ in other}), "lines": [ln for _, ln in other][:40]}


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
        if r["status"] == "fail":
            if r["kind"] == "fleet" and not scope["full_fleet"]:
                if c["slug"]:
                    a = ({"classification": "blocking"} if c["slug"] in affected else
                         {"classification": "out_of_scope", "slugs": [c["slug"]], "lines": _tail(r["log"])})
                else:
                    try:
                        text = Path(r["log"]).read_text(encoding="utf-8", errors="replace")
                    except OSError:
                        text = ""
                    a = attribute(text, affected, known)
                if a["classification"] == "out_of_scope":
                    c["status"] = "out_of_scope"
                    out_of_scope.append({"check": r["name"], "slugs": a["slugs"], "lines": a["lines"]})
                else:
                    c["blocking"] = True
            else:
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
        "scope": {k: scope[k] for k in ("mode", "full_fleet", "reason", "notes", "connectors", "touched",
                                        "security_gate_slugs", "changed_files")},
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
    a = ap.parse_args(argv)
    if a.cmd == "gate-go":
        print(gate_go() or "")
        return 0
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
        ("  - acme: missing required file guide.md", "acme"),
        ("  - skills/zeta/README.md: broken local link -> ../../CONTRIBUTING.md", "zeta"),
        ("  - missing skills/zeta/assets/social.png (expected 1200x630 PNG)", "zeta"),
        ("  - docs/skills/zeta.md: missing `title` front matter", "zeta"),
        ("README.md:3: [acme] acme-pp-cli -> | acme |", "acme"),
        ("CONTRIBUTING.md:40:an em-dash here", "<repo>"),
        ("  - tools/maintainer/skills.json: not valid JSON", "<repo>"),
        ("See docs/reprint-survival.md and issue #252.", None),
        ("Skill contract check FAILED:", None),
        ("FAIL: 2 finding(s)", None),
        ("  - unknownthing: something", None),
    ):
        got = attribute_line(line, known)
        expect(got == want, f"{line.strip()[:60]!r} -> {want} (got {got})")

    print("attribute (a failed fleet-wide check):")
    a = attribute("Skill contract check FAILED:\n  - zeta: missing required file guide.md\n"
                  "See CONTRIBUTING.md.\n", {"acme"}, known)
    expect(a["classification"] == "out_of_scope" and a["slugs"] == ["zeta"],
           "only another connector failed -> out_of_scope, naming it")
    a = attribute("FAILED:\n  - zeta: x\n  - acme: y\n", {"acme"}, known)
    expect(a["classification"] == "blocking", "an in-scope connector failed too -> blocking")
    a = attribute("skills/zeta/README.md:1:x\nREADME.md:9:y\n", {"acme"}, known)
    expect(a["classification"] == "blocking", "a repo-level file failed too -> blocking")
    a = attribute("check FAILED: something is wrong\n", {"acme"}, known)
    expect(a["classification"] == "blocking", "a failure naming nothing -> blocking (fail closed)")
    a = attribute("  - stranger: y\n  - zeta: x\n", {"acme"}, known)
    expect(a["classification"] == "out_of_scope", "an unknown leading word is not mistaken for a connector")
    a = attribute("skills/newskill/README.md:1:x\n", {"acme"}, known | {"newskill"})
    expect(a["classification"] == "out_of_scope", "a skill dir outside the scope -> out_of_scope")

    print("build_verdict:")
    with tempfile.TemporaryDirectory() as tmp:
        log_other = Path(tmp) / "other.log"
        log_other.write_text("FAILED:\n  - zeta: missing\n", encoding="utf-8")
        log_mine = Path(tmp) / "mine.log"
        log_mine.write_text("FAILED:\n  - acme: missing\n", encoding="utf-8")
        scope = {"affected": ["acme"], "full_fleet": False, "head_sha": "h", "dirty": False, "base_ref": "b",
                 "base_sha": "b", "merge_base": "m", "mode": "slug", "reason": "", "notes": [],
                 "connectors": ["acme"], "touched": ["acme"], "security_gate_slugs": ["acme"], "changed_files": 0}

        def rec(name: str, kind: str, slug: str, status: str, log: Path) -> dict:
            return {"name": name, "kind": kind, "slug": slug, "status": status, "seconds": 1.0, "log": str(log)}

        v = build_verdict(scope, [rec("build+vet", "connector", "acme", "pass", log_mine),
                                  rec("skill contract", "fleet", "", "fail", log_other)], 2, "t", known)
        expect(v["verdict"] == "pass" and v["out_of_scope_failures"][0]["slugs"] == ["zeta"],
               "another connector's fleet failure -> PASS, listed under out_of_scope_failures")
        v = build_verdict(scope, [rec("skill contract", "fleet", "", "fail", log_mine)], 2, "t", known)
        expect(v["verdict"] == "fail" and v["blocking_failures"] == ["skill contract"],
               "this connector's fleet failure -> FAIL")
        v = build_verdict(scope, [rec("go test", "connector", "acme", "fail", log_mine)], 2, "t", known)
        expect(v["verdict"] == "fail", "a per-connector check failure -> FAIL")
        v = build_verdict(scope, [rec("plugin validate", "fleet", "zeta", "fail", log_other)], 2, "t", known)
        expect(v["verdict"] == "pass" and v["out_of_scope_failures"][0]["slugs"] == ["zeta"],
               "a per-skill fleet check for an out-of-scope skill -> listed, not blocking")
        v = build_verdict(scope, [rec("catalog idempotency", "repo", "", "fail", log_other)], 2, "t", known)
        expect(v["verdict"] == "fail", "a repo-level check failure -> FAIL")
        v = build_verdict(dict(scope, full_fleet=True), [rec("skill contract", "fleet", "", "fail", log_other)],
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

    if failures:
        print(f"\nFAIL: verify_scope self-test - {len(failures)} case(s)")
        return 1
    print("\nPASS: verify_scope - scope, attribution and verdict each discriminate")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
