#!/usr/bin/env python3
"""verify_watch.py - say when the verify-msp-skills map is stale, and when the press moved.

Run weekly (and on demand) by .github/workflows/verify-watch.yml. Two outputs,
each ONE GitHub issue that is opened, updated in place, or closed:

  verify-map-stale  the feature map in .claude/skills/verify-msp-skills may be
                    behind the fleet. Reasons:
                    (a) the newest printing_press_version in skills/*/manifest.json
                        is above maintained_at_press in verify_maintained.json;
                    (b) a verify ERROR in the newest recorded result on main for
                        any connector (the verify-<slug> CI artifacts written by
                        tools/maintainer/verify_gate.py), or no recorded results
                        at all in the window, so (b) and (c) cannot be checked;
                    (c) a feature SKIPs on a connector whose press version should
                        have it (install and mcp-stdio always; every feature from
                        press 4.34.0 on);
                    (d) maintained_at is 30 or more days old.
  press-watch       mvanhorn/cli-printing-press has a release newer than the
                    fleet's newest printing_press_version. Input for the reprint
                    decision; nothing here ever reprints. Its body also lists the
                    untriaged press bug fixes (the `### Bug Fixes` bullets of every
                    release after the fleet's oldest print) whose PR changed
                    generated code, per connector printed before them, minus those
                    a fleet hand-fix cites by press PR number (for a connector whose
                    ledger has it) or press_fix_triage.json triages. Advisory: the
                    follow-up is one fleet hand-fix or a triage entry, never a reprint.

    python3 tools/maintainer/verify_watch.py --repo Servosity/msp-skills --collect --apply
    python3 tools/maintainer/verify_watch.py --repo Servosity/msp-skills --collect        # dry run
    python3 tools/maintainer/verify_watch.py --results DIR --press-release rel.json \\
        --press-releases rels.json --today 2026-10-23 --issues-fixture issues.json        # offline
    python3 tools/maintainer/verify_watch.py --self-test

Idempotent: an issue is found by its label plus a marker comment in its body;
an open one is edited (only when its text changed), never duplicated (extra
open copies are closed as duplicates), and closed once nothing is stale. When a
source cannot be read (CI artifacts, the press API) the matching issue is left
as it is, never closed on missing evidence, and the run exits 1.

Without --apply nothing is written: the planned issue actions are printed.
Stdlib only; GitHub calls go through the `gh` CLI (GH_TOKEN in CI).
"""

from __future__ import annotations

import argparse
import datetime as dt
import io
import json
import re
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
STAMP = ROOT / "tools" / "maintainer" / "verify_maintained.json"
FEATURE_IDS = ("install", "mcp-stdio", "mcp-http", "write-gating", "read-only", "local-data")
# Every press version has an installer and a stdio MCP server; from 4.34.0 on,
# every connector has all six surfaces (features 3 to 6 flip FAIL -> PASS on
# the reprint). A SKIP inside those bounds means the map lost track of a surface.
ALWAYS_PRESENT = ("install", "mcp-stdio")
ALL_FEATURES_SINCE = "4.34.0"
STALE_DAYS = 30
WINDOW_DAYS = 14
PRESS_REPO = "mvanhorn/cli-printing-press"
MAP_LABEL, PRESS_LABEL = "verify-map-stale", "press-watch"
MAP_MARKER, PRESS_MARKER = "<!-- verify-watch:verify-map-stale -->", "<!-- verify-watch:press-watch -->"
LABELS = {MAP_LABEL: ("d93f0b", "The verify-msp-skills feature map may be behind the fleet (verify-watch.yml)"),
          PRESS_LABEL: ("0e8a16", "A newer cli-printing-press release than the fleet uses (verify-watch.yml)")}
_VER = re.compile(r"^v?(\d+)\.(\d+)\.(\d+)$")


def parse_version(text) -> tuple[int, int, int]:
    m = _VER.match(str(text or "").strip())
    if not m:
        raise ValueError(f"not a release version: {text!r}")
    return int(m.group(1)), int(m.group(2)), int(m.group(3))


def vstr(v: tuple[int, int, int]) -> str:
    return ".".join(map(str, v))


# --------------------------------------------------------------------------- #
# Inputs.
# --------------------------------------------------------------------------- #

def fleet_versions(root: Path) -> dict[str, tuple[int, int, int]]:
    """{slug: printing_press_version} for every manifest that records one.
    An unparseable version raises: the newest version must never be a guess."""
    out = {}
    for mf in sorted((root / "skills").glob("*/manifest.json")):
        v = json.loads(mf.read_text(encoding="utf-8")).get("printing_press_version")
        if v:
            out[mf.parent.name] = parse_version(v)
    return out


WRITE_METHODS = ("POST", "PUT", "PATCH", "DELETE")
_EXECUTE_TOOL = re.compile(r'NewTool\(\s*"[^"]*_execute"')
_ANY_TOOL = re.compile(r'NewTool\(\s*"')


def expects_execute(manifest) -> bool | None:
    """Whether the press generated <slug>_execute, read from the connector's
    press/tools-manifest.json: only in code orchestration mode, and only when
    the API has a state-changing endpoint to execute. None when the manifest
    is not a tools manifest at all."""
    if not isinstance(manifest, dict):
        return None
    tools, mcp = manifest.get("tools"), manifest.get("mcp", {})
    # Missing or malformed evidence is unknown, never "no write endpoint".
    if not isinstance(tools, list) or not isinstance(mcp, dict):
        return None
    methods = []
    for t in tools:
        if not isinstance(t, dict) or not isinstance(t.get("method"), str) or not t["method"].strip():
            return None
        methods.append(t["method"].strip().upper())
    return mcp.get("orchestration") == "code" and any(m in WRITE_METHODS for m in methods)


def write_surfaces(root: Path) -> dict[str, bool]:
    """{slug: has <slug>_execute} for every connector whose answer is readable:
    its press/tools-manifest.json, or, for a connector printed without one,
    whether its MCP source (never a _test.go file) registers an *_execute
    tool. A slug left out is unknown (no readable manifest, no production
    source registering any tool), and a write-gating SKIP on it is flagged."""
    out: dict[str, bool] = {}
    for cli in sorted((root / "skills").glob("*/cli")):
        slug = cli.parent.name
        tm = cli.parent / "press" / "tools-manifest.json"
        if tm.is_file():
            try:
                got = expects_execute(json.loads(tm.read_text(encoding="utf-8")))
            except (OSError, ValueError):
                got = None
            if got is not None:
                out[slug] = got
            continue
        try:
            src = "\n".join(f.read_text(encoding="utf-8", errors="replace")
                            for f in sorted((cli / "internal" / "mcp").glob("*.go"))
                            if not f.name.endswith("_test.go"))
        except OSError:
            continue
        # Only source that registers MCP tools at all can say _execute is absent.
        if _ANY_TOOL.search(src):
            out[slug] = bool(_EXECUTE_TOOL.search(src))
    return out


def load_stamp(path: Path) -> tuple[tuple[int, int, int], dt.date]:
    d = json.loads(path.read_text(encoding="utf-8"))
    return parse_version(d["maintained_at_press"]), dt.date.fromisoformat(d["maintained_at"])


VERDICTS = ("PASS", "FAIL", "ERROR", "SKIP")


def valid_record(rec) -> bool:
    """A record is evidence only if it names its slug and either carries all six
    feature verdicts or says outright that the gate errored. Anything thinner
    (an empty features object, a missing feature) is not proof of health."""
    if not isinstance(rec, dict) or not rec.get("slug") or not isinstance(rec.get("features"), dict):
        return False
    if rec.get("gate_exit") == 2 or rec.get("errors"):
        return True  # an explicit error record: it flags (b) by itself
    # Evidence of health must carry every field a decision reads: all six
    # verdicts for (b) and (c), and a press version that is readable or says
    # outright that there is none (a connector not printed by the press:
    # verify.py writes "unknown" when the manifest records no version). A
    # missing version never hides a (c): map_reasons() then reads the fleet's.
    if not no_press(rec.get("press_version")):
        try:
            parse_version(rec.get("press_version"))
        except ValueError:
            return False
    return all(rec["features"].get(f) in VERDICTS for f in FEATURE_IDS)


def no_press(value) -> bool:
    """The record says it has no press version: JSON null (or absent), or the
    "unknown" verify.py writes when the manifest records none."""
    return value is None or value == "unknown"


def error_record(slug: str, why: str) -> dict:
    return {"slug": slug, "press_version": None, "features": {}, "reasons": {}, "gate_exit": 2, "errors": [why]}


def record_from(gate: dict | None, verdict: dict | None) -> dict | None:
    """One connector's newest verify result, from verify_gate.py's gate.json or,
    when that is missing or incomplete, verify.py's own verdict.json. None when
    neither is usable evidence."""
    if isinstance(gate, dict) and isinstance(gate.get("features"), dict):
        rec = {"slug": gate.get("slug"), "press_version": gate.get("press_version"),
               "features": dict(gate["features"]), "reasons": dict(gate.get("reasons") or {}),
               "gate_exit": gate.get("gate_exit"), "errors": list(gate.get("errors") or [])}
        if valid_record(rec):
            return rec
    if isinstance(verdict, dict) and isinstance(verdict.get("features"), dict):
        feats = verdict["features"]
        rec = {"slug": verdict.get("slug"), "press_version": verdict.get("press_version"),
               "features": {k: (v or {}).get("verdict") if isinstance(v, dict) else None for k, v in feats.items()},
               "reasons": {k: str((v or {}).get("reason", ""))[:300] if isinstance(v, dict) else ""
                           for k, v in feats.items()},
               "gate_exit": None, "errors": [str(p) for p in verdict.get("harness_problems") or []]}
        if not all(rec["features"].get(f) in VERDICTS for f in FEATURE_IDS):
            missing = [f for f in FEATURE_IDS if rec["features"].get(f) not in VERDICTS]
            rec["errors"].append(f"verdict.json lacks a verdict for {missing}")
        if valid_record(rec):
            return rec
    return None


def load_results(results: Path) -> dict[str, dict]:
    """{slug: record}. Each <dir>/<slug>.json is one record (written by
    collect(), or by hand for a fixture)."""
    out = {}
    for p in sorted(results.glob("*.json")):
        rec = json.loads(p.read_text(encoding="utf-8"))
        slug = rec.get("slug") if isinstance(rec, dict) and rec.get("slug") else p.stem
        out[slug] = rec if valid_record(rec) else error_record(slug, f"{p.name} is not a usable verify result")
    return out


# --------------------------------------------------------------------------- #
# Decisions (pure: the self-test drives each trigger both ways).
# --------------------------------------------------------------------------- #

def map_reasons(fleet: dict, stamp_press, stamp_date, results: dict[str, dict] | None,
                today: dt.date, window_days: int = WINDOW_DAYS,
                write_surface: dict[str, bool] | None = None) -> list[str]:
    """write_surface is write_surfaces(): {slug: has <slug>_execute}. The
    write-gating feature exists only where the press generated that tool, so a
    write-gating SKIP is stale only when it is there or the answer is unknown."""
    write_surface = write_surface or {}
    reasons = []
    if fleet:
        newest = max(fleet.values())
        if newest > stamp_press:
            at = sorted(s for s, v in fleet.items() if v == newest)
            reasons.append(f"(a) the fleet's newest printing_press_version is {vstr(newest)} "
                           f"({len(at)} connector(s): {', '.join(at[:5])}{', ...' if len(at) > 5 else ''}), "
                           f"above maintained_at_press {vstr(stamp_press)}")
    if results is not None:
        if not results:
            reasons.append(f"(b) no verify gate results were recorded on main in the last {window_days} days, "
                           f"so verify ERRORs and SKIPs cannot be checked")
        for slug in sorted(results):
            rec = results[slug]
            feats = rec.get("features") or {}
            errs = [f for f in FEATURE_IDS if feats.get(f) == "ERROR"]
            if errs or rec.get("gate_exit") == 2 or rec.get("errors"):
                why = "; ".join(f"{f}: {str((rec.get('reasons') or {}).get(f, ''))[:120]}" for f in errs)
                if not why:
                    why = "; ".join(str(e)[:120] for e in rec.get("errors") or []) or "gate exit 2"
                reasons.append(f"(b) verify ERROR on main for {slug}: {why}")
            try:
                press = parse_version(rec.get("press_version"))
            except ValueError:
                press = fleet.get(slug)  # the record names none: the manifest's version decides
            for f in FEATURE_IDS:
                if feats.get(f) != "SKIP":
                    continue
                if f in ALWAYS_PRESENT:
                    reasons.append(f"(c) {slug} SKIPs {f}, which every press version has")
                elif f == "write-gating" and write_surface.get(slug) is False:
                    continue  # no <slug>_execute: not code orchestration, or no write endpoint
                elif press is not None and press >= parse_version(ALL_FEATURES_SINCE):
                    reasons.append(f"(c) {slug} SKIPs {f} at press {vstr(press)}, which has it "
                                   f"(every feature exists from {ALL_FEATURES_SINCE})")
    if (today - stamp_date).days >= STALE_DAYS:
        reasons.append(f"(d) maintained_at {stamp_date.isoformat()} is {STALE_DAYS} or more days old")
    return reasons


MAX_REASONS, MAX_REASON_CHARS = 40, 400  # keeps the body far under GitHub's 65536-character limit


def map_issue(reasons: list[str]) -> tuple[str, str] | None:
    if not reasons:
        return None
    shown = [f"- {r[:MAX_REASON_CHARS]}" for r in reasons[:MAX_REASONS]]
    if len(reasons) > MAX_REASONS:
        shown.append(f"- ...and {len(reasons) - MAX_REASONS} more (see the verify-watch run log)")
    counts = {k: sum(1 for r in reasons if r.startswith(f"({k})")) for k in "abcd"}
    body = "\n".join([
        MAP_MARKER,
        "The verify-msp-skills feature map (`.claude/skills/verify-msp-skills`) may be behind the fleet.",
        "",
        f"Why ({len(reasons)} reason(s): " + ", ".join(f"{k}={n}" for k, n in counts.items() if n) + "):",
        "",
        *shown,
        "",
        "What to do: run `/maintain-verification-skill` locally, on the subscription (not in CI). Then "
        "update `tools/maintainer/verify_maintained.json` (`maintained_at_press`, `maintained_at`) in the "
        "same PR.",
        "",
        "verify-watch.yml updates this issue in place and closes it on the first run where nothing is stale.",
    ]) + "\n"
    return "verify-map-stale: the verify-msp-skills map needs a maintenance pass", body


# --------------------------------------------------------------------------- #
# Press drift: the press fixes that connectors printed before them do not carry.
# --------------------------------------------------------------------------- #

FLEET_HANDFIXES = ROOT / "tools" / "maintainer" / "fleet_handfixes.json"
PRESS_FIX_TRIAGE = ROOT / "tools" / "maintainer" / "press_fix_triage.json"
# Bug-fix scopes that never reach a printed connector (the press's own CI and tests).
IGNORED_SCOPES = frozenset({"ci", "test", "tests", "docs", "chore"})
# The release-please section names. A body whose only headings are others (say
# "## What Changed") is not a shape this watch reads, and is named in a note.
RP_SECTIONS = frozenset({"bug fixes", "features", "code refactoring", "performance improvements", "reverts",
                         "documentation", "miscellaneous chores", "tests", "build system",
                         "continuous integration", "styles", "dependencies"})
_PRESS_REF = rf"https://github\.com/{re.escape(PRESS_REPO)}/(?:issues|pull)/(\d+)"
_RP_BULLET = re.compile(r"^\* (?:\*\*(?P<scope>[^*]+?):\*\* )?(?P<text>.+)$")
_CC_FIX = re.compile(r"^[*-] (?:\*\*)?fix(?:\((?P<scope>[^)]*)\))?!?:(?:\*\*)? (?P<text>.+)$", re.I)
_PR_GROUP = re.compile(rf"\(\[#(\d+)\]\({_PRESS_REF}\)\)")
_PLAIN_PR = re.compile(r"\(#(\d+)\)")
_COMMIT_REF = rf"https://github\.com/{re.escape(PRESS_REPO)}/commit/[0-9a-f]+"
# release-please's suffix: " ([#PR](url))", " ([sha](commit url))", ", closes [#N](url), [#M](url)".
_RP_META = re.compile(rf"(?:\s*\(\[#\d+\]\({_PRESS_REF}\)\))?(?:\s*\(\[[0-9a-f]{{7,40}}\]\({_COMMIT_REF}\)\))?"
                      rf"(?:,?\s*closes\s+(?:\[#\d+\]\({_PRESS_REF}\)(?:,\s*|\s+and\s+|\s*))+)?\s*$", re.I)
# A conventional-commit line's suffix: " (#N)" and GitHub's " by @x in url".
_CC_META = re.compile(r"(?:\s*\(#\d+\))?(?:\s+by\s+@\S+\s+in\s+\S+)?\s*$")
_CITE = re.compile(rf"cli-printing-press ?#(\d+)((?:\s*(?:,|and|,\s*and)\s*#\d+)*)|{_PRESS_REF}")
MAX_SHOWN_SLUGS, MAX_TITLE_CHARS, MAX_NOTES, MAX_PRINT_VERSIONS = 15, 140, 20, 40
# The section's share of the issue body: full release blocks up to DRIFT_BUDGET,
# one-line release summaries up to SUMMARY_ROOM more, then a single tail line.
# With the rest of the body this stays far under GitHub's 65536-character limit.
DRIFT_BUDGET, SUMMARY_ROOM = 50000, 6000


def _plain(text: str, cap: int = MAX_TITLE_CHARS) -> str:
    """Third-party text made safe for one Markdown code span: links reduced to
    their text, no backticks, no markup, no control characters, bounded."""
    t = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", text)
    t = re.sub(r"[`*\x00-\x1f\x7f]", "", t)
    t = re.sub(r"\s+", " ", t).strip()
    return t if len(t) <= cap else t[:cap - 3].rstrip() + "..."


def release_fixes(body: str) -> tuple[list[dict], bool]:
    """The bug-fix entries of one press release body, and whether the body had a
    shape this parser knows (a release-please section header or a conventional
    `fix:` line). Release-please writes `### Bug Fixes` with one
    `* **scope:** subject ([#PR](url)) ([sha](url))` bullet per fix; every other
    section (Features, Chores, Documentation, ...) is skipped. Each entry:
    {scope, title, key, pr, refs, sha, kind: fix|ignored}. refs are the fix's OWN
    press numbers: its PR and the issues release-please says it closes, never a
    number its subject merely mentions (a follow-up to #N is not #N)."""
    fixes: list[dict] = []
    section, known = None, False
    for ln in (body or "").splitlines():
        if re.match(r"^#{2,3} ", ln):
            section = ln.lstrip("#").strip().lower() if ln.startswith("### ") else None
            known = known or section in RP_SECTIONS
            continue
        m = _CC_FIX.match(ln) or (_RP_BULLET.match(ln) if section == "bug fixes" else None)
        if not m:
            continue
        known = True
        text = m.group("text")
        # The metadata suffix is read from the END of the line, so a link inside
        # the subject ("follow-up to ([#100](..))") is never taken for the PR.
        meta = _RP_META.search(text) if section == "bug fixes" and not _CC_FIX.match(ln) else None
        if meta is None or meta.start() == len(text):
            meta = _CC_META.search(text)
        title, tail = text[:meta.start()], text[meta.start():]
        refs = {int(n) for n in re.findall(_PRESS_REF, tail)} | {int(n) for n in _PLAIN_PR.findall(tail)}
        pr = _PR_GROUP.search(tail)
        pr_num = int(pr.group(1)) if pr else (min(refs) if refs else None)
        scope = _plain(m.group("scope") or "", 40).lower()
        kind = "ignored" if scope in IGNORED_SCOPES else "fix"
        sha = re.search(rf"\(\[([0-9a-f]{{7,40}})\]\(https://github\.com/{re.escape(PRESS_REPO)}/commit/", text)
        fixes.append({"scope": scope, "title": _plain(title), "key": _plain(title, 10 ** 6),
                      "pr": pr_num, "refs": refs, "kind": kind,
                      "sha": sha.group(1)[:7] if sha else None})
    return fixes, known


def cited_press_numbers(entry) -> set[int]:
    """The cli-printing-press PR/issue numbers a fleet hand-fix entry cites:
    `cli-printing-press #N` (and the `, #M` list that follows it), or a press
    URL. A bare `#N` is an msp-skills number and is never read as the press's."""
    out: set[int] = set()
    for first, rest, url in _CITE.findall(json.dumps(entry)):
        if url:
            out.add(int(url))
        else:
            out.add(int(first))
            out.update(int(n) for n in re.findall(r"#(\d+)", rest))
    return out


def ledger_ids(root: Path) -> dict[str, set[str]]:
    """{slug: the hand-fix ids its own skills/<slug>/handfixes.json records}."""
    out: dict[str, set[str]] = {}
    for led in sorted((root / "skills").glob("*/handfixes.json")):
        out[led.parent.name] = {e.get("id") for e in json.loads(led.read_text(encoding="utf-8")).get("handfixes")
                                or [] if isinstance(e, dict) and isinstance(e.get("id"), str)}
    return out


def carried_numbers(root: Path, fleet_handfixes: dict, ids: dict[str, set[str]] | None = None
                    ) -> dict[str, dict[int, str]]:
    """{slug: {press number: fleet hand-fix id}} for the fleet-wide hand-fixes
    each connector's own skills/<slug>/handfixes.json records. Only a connector
    whose ledger names the id carries the port."""
    cites = {hid: cited_press_numbers(e) for hid, e in (fleet_handfixes.get("handfixes") or {}).items()}
    out: dict[str, dict[int, str]] = {}
    for slug, have in sorted((ids if ids is not None else ledger_ids(root)).items()):
        nums = {n: hid for hid in sorted(i for i in have if i in cites) for n in sorted(cites[hid])}
        if nums:
            out[slug] = nums
    return out


def load_triage(data) -> dict[int, dict]:
    """tools/maintainer/press_fix_triage.json, validated: {"fixes": {"<press PR>":
    {"ported": "<hand-fix id>"} or {"not_affected": "<reason>"}, optional
    "slugs": [...]}}. Anything else raises: a ledger that cannot be read must
    never drop a fix by accident."""
    if not isinstance(data, dict) or not isinstance(data.get("fixes"), dict):
        raise ValueError("press_fix_triage.json: no fixes object")
    out: dict[int, dict] = {}
    for k, e in data["fixes"].items():
        if not re.fullmatch(r"[1-9]\d*", str(k)) or not isinstance(e, dict):
            raise ValueError(f"press_fix_triage.json: entry {k!r} is not a press PR number with an object")
        kinds = [x for x in ("ported", "not_affected") if x in e]
        if len(kinds) != 1 or not isinstance(e[kinds[0]], str) or not e[kinds[0]].strip():
            raise ValueError(f"press_fix_triage.json #{k}: exactly one of ported / not_affected, a non-empty string")
        extra = set(e) - {"ported", "not_affected", "slugs", "why"}
        if extra:
            raise ValueError(f"press_fix_triage.json #{k}: unknown keys {sorted(extra)}")
        if "slugs" in e and (not isinstance(e["slugs"], list) or not e["slugs"]
                             or not all(isinstance(s, str) and s for s in e["slugs"])):
            raise ValueError(f"press_fix_triage.json #{k}: slugs must be a non-empty list of connector slugs")
        out[int(k)] = e
    return out


def press_drift(releases: list, fleet: dict, carried: dict[str, dict[int, str]],
                triage: dict[int, dict] | None = None, ids: dict[str, set[str]] | None = None) -> dict:
    """Every press bug fix shipped after the fleet's oldest print, with the
    connectors printed before the release that shipped it, minus the
    connectors that already have it: a fleet hand-fix their ledger names cites
    the fix's press number, or press_fix_triage.json triaged the fix's PR
    (ported: for the connectors whose ledger names that hand-fix id, and any
    slugs listed; not_affected: for the slugs listed, or every connector). Pure."""
    triage, ids = triage or {}, ids or {}
    if not fleet:
        return {"releases": [], "odd": [], "oldest": None, "fleet": fleet}
    oldest = min(fleet.values())
    rels, odd, seen = [], [], {}
    for r in releases:
        if not isinstance(r, dict) or r.get("draft") or r.get("prerelease"):
            continue
        try:
            v = parse_version(r.get("tag_name"))
        except ValueError:
            odd.append(f"release tag `{_plain(str(r.get('tag_name')), 40)}` is not a version; read it by hand")
            continue
        if v > oldest:
            rels.append((v, r))
    out = []
    for v, r in sorted(rels, key=lambda x: x[0]):
        if v in seen:
            continue
        seen[v] = True
        fixes, known = release_fixes(r.get("body") or "")
        if not known:
            odd.append(f"v{vstr(v)}: the release notes have no section this watch can read; read them by hand")
        before = sorted((s for s, fv in fleet.items() if fv < v), key=lambda s: (fleet[s], s))
        out.append({"version": v, "published": str(r.get("published_at") or "")[:10],
                    "before": before, "fixes": fixes})
    for hid in sorted({e["ported"] for e in triage.values() if "ported" in e}):
        if not any(hid in have for have in ids.values()):
            odd.append(f"press_fix_triage.json names ported id `{_plain(hid, 60)}`, which no connector ledger records")
    # A fix shipped twice (a backport: same PR, same subject, a later release)
    # belongs to the first release that shipped it. Two bullets of one release
    # are never merged, and a shared PR with another subject is another fix.
    first: set[tuple] = set()
    for rel in out:
        kept, here = [], set()
        for f in rel["fixes"]:
            key = (f["pr"], f["key"])
            if f["pr"] is not None and key in first:
                continue
            here.add(key)
            nums = f["refs"] | ({f["pr"]} if f["pr"] is not None else set())
            drop = {s: f"fleet hand-fix `{carried[s][n]}`" for s in rel["before"] for n in sorted(nums)
                    if n in carried.get(s, {})}
            t = triage.get(f["pr"]) if f["pr"] is not None else None
            if t is not None:
                named = set(t.get("slugs") or [])
                if "ported" in t:
                    label = f"triage: ported as `{_plain(t['ported'], 60)}`"
                    who = {s for s in rel["before"] if t["ported"] in ids.get(s, ()) or s in named}
                else:
                    label = "triage: not affected"
                    who = {s for s in rel["before"] if s in named} if named else set(rel["before"])
                drop.update({s: label for s in who if s not in drop})
            kept.append({**f, "missing": [s for s in rel["before"] if s not in drop],
                         "carried_by": sorted(set(drop.values())), "carried": len(drop)})
        rel["fixes"] = kept
        first |= here
    return {"releases": list(reversed(out)), "odd": odd, "oldest": oldest, "fleet": fleet}


# A press PR changed what a connector is printed with when it touched a
# generator template or the golden expected output (rendered CLIs the press
# checks every template against). Test files are left out on both sides.
GENERATED_PATHS = ("internal/generator/templates/", "testdata/golden/expected/")
GQL_BATCH, GQL_RESERVE, MAX_FILE_PAGES = 40, 100, 30


def generated_change(paths) -> bool:
    return any(isinstance(p, str) and p.startswith(GENERATED_PATHS)
               and not re.search(r"_test\.go(?:\.tmpl)?$", p) for p in paths)


def gql(query: str) -> dict:
    """One GraphQL call. A partial answer (some aliases NOT_FOUND) is still an
    answer: its data is returned, the missing aliases come back as None."""
    try:
        r = subprocess.run(["gh", "api", "graphql", "-f", f"query={query}"],
                           capture_output=True, text=True, timeout=120)
    except (OSError, subprocess.SubprocessError) as e:
        raise RuntimeError(f"gh api graphql failed: {e}") from e
    try:
        got = json.loads(r.stdout)
    except ValueError:
        got = None
    if not isinstance(got, dict) or not isinstance(got.get("data"), dict):
        raise RuntimeError(f"gh api graphql failed: {(r.stderr or r.stdout).strip()[:300]}")
    return got["data"]


def pr_generated_changes(prs: list[int], call=None) -> tuple[dict[int, bool | None], dict]:
    """{pr: True (changed generated code) / False / None (unreadable)} for the
    press PRs, read from each PR's file list through GraphQL, GQL_BATCH PRs a
    call (the REST budget of the Actions token, 1000 an hour, is mostly spent
    by the verify-map collection). Stops asking when the GraphQL budget falls
    to GQL_RESERVE; what is not asked stays None, never False."""
    call = call or gql
    owner, name = PRESS_REPO.split("/")
    res: dict[int, bool | None] = {}
    stats = {"prs": len(prs), "asked": 0, "calls": 0, "unreadable": 0, "stopped": ""}
    files = "files(first: 100{after}) {{ nodes {{ path }} pageInfo {{ hasNextPage endCursor }} }}"
    pending = [(p, None, 0) for p in prs]  # (pr, cursor, page)
    partial: set[int] = set()  # PRs with an unreadable file node on some page
    while pending:
        batch, pending = pending[:GQL_BATCH], pending[GQL_BATCH:]
        body = " ".join(f'p{p}: pullRequest(number: {p}) {{ '
                        + files.format(after=f', after: "{c}"' if c else "") + " }" for p, c, _ in batch)
        try:
            data = call(f'query {{ rateLimit {{ remaining }} repository(owner: "{owner}", name: "{name}") '
                        f'{{ {body} }} }}')
        except RuntimeError as e:
            stats["stopped"] = _plain(str(e), 160)
            break
        stats["calls"] += 1
        repo = data.get("repository") if isinstance(data.get("repository"), dict) else {}
        for p, c, page in batch:
            if c is None:
                stats["asked"] += 1
            node = repo.get(f"p{p}")
            fl = node.get("files") if isinstance(node, dict) else None
            nodes = fl.get("nodes") if isinstance(fl, dict) else None
            if not isinstance(nodes, list):
                res[p] = None
                continue
            paths = [n.get("path") if isinstance(n, dict) else None for n in nodes]
            if generated_change(paths):
                res[p] = True  # one generated path is proof, whatever else the page holds
                continue
            if not all(isinstance(x, str) for x in paths):
                partial.add(p)  # a null file node: "no generated change" is not proven
            info = fl.get("pageInfo") or {}
            if info.get("hasNextPage") and page + 1 < MAX_FILE_PAGES and isinstance(info.get("endCursor"), str) \
                    and re.fullmatch(r"[A-Za-z0-9+/=_-]+", info["endCursor"]):
                pending.append((p, info["endCursor"], page + 1))
            else:
                res[p] = None if info.get("hasNextPage") or p in partial else False
        left = (data.get("rateLimit") or {}).get("remaining")
        if pending and isinstance(left, int) and left <= GQL_RESERVE:
            stats["stopped"] = f"GraphQL budget down to {left}"
            break
    stats["unreadable"] = sum(1 for p in prs if res.get(p) is None)
    return {p: res.get(p) for p in prs}, stats


def classify_drift(drift: dict, lookup) -> dict:
    """Marks every listed fix (not ignored, still missing somewhere) with
    gen = True / False / None from its press PR's files; a fix with no PR
    number is None (unclassified). Newest release first, each PR once."""
    prs: list[int] = []
    for rel in drift.get("releases", []):
        for f in rel["fixes"]:
            if f["kind"] != "ignored" and f["missing"] and f["pr"] is not None and f["pr"] not in prs:
                prs.append(f["pr"])
    got, stats = lookup(prs) if prs else ({}, {"prs": 0, "asked": 0, "calls": 0, "unreadable": 0, "stopped": ""})
    for rel in drift.get("releases", []):
        for f in rel["fixes"]:
            f["gen"] = got.get(f["pr"]) if f["pr"] is not None else None
    drift["query"] = stats
    return drift


def _slugs(slugs: list[str]) -> str:
    shown = ", ".join(f"`{s}`" for s in slugs[:MAX_SHOWN_SLUGS])
    return shown + (f" and {len(slugs) - MAX_SHOWN_SLUGS} more" if len(slugs) > MAX_SHOWN_SLUGS else "")


def _ref(f: dict) -> str:
    return f" (#{f['pr']})" if f["pr"] is not None else f" (commit {f['sha']})" if f.get("sha") else ""


def drift_section(drift: dict | None, error: str | None = None, budget: int = DRIFT_BUDGET,
                  full_log: bool = False) -> str:
    """The press-watch issue's section: untriaged press fixes that changed
    generated code, per connector printed before them, newest release first.
    A fix whose PR files could not be read is listed as unclassified, never
    dropped. A release that does not fit the budget is one summary line (the
    full list is in the verify-watch run log, rendered with full_log=True,
    which also names the fixes that changed no generated code)."""
    head = ["", "## Press fixes that older connectors do not carry", ""]
    if error is not None:
        return "\n".join(head + [f"The press drift inputs (the press release list, fleet_handfixes.json, "
                                 f"press_fix_triage.json) could not be read this run (`{_plain(error, 200)}`), "
                                 "so which fixes older connectors lack is unknown. Nothing below is a clean "
                                 "list; the next run retries."])
    drift = drift or {}
    lines = list(head) + [
        "Each fix below changed generated code (a press template or its golden output) after the listed "
        "connectors were printed, and has not been triaged. Triaging a fix means porting it as one fleet "
        "hand-fix (its `tools/maintainer/fleet_handfixes.json` entry cites `cli-printing-press #N`) or "
        "recording it in `tools/maintainer/press_fix_triage.json` (`ported` or `not_affected`). The list "
        "only shrinks by triage. Never a reprint by default; see `tools/maintainer/README.md`, press-watch.",
        ""]
    live = lambda f: f["kind"] != "ignored" and f["missing"]  # noqa: E731
    # A fix every older connector carries is still NAMED (as carried), never
    # silently gone: a citation the parser misread can then only mislabel a fix.
    gone = lambda f: f["kind"] != "ignored" and not f["missing"] and f["carried"]  # noqa: E731
    rels = [r for r in drift.get("releases", []) if any(live(f) or gone(f) for f in r["fixes"])]
    fleet = drift.get("fleet") or {}
    allf = [f for r in rels for f in r["fixes"] if live(f)]
    listed = [f for f in allf if f.get("gen") is not False]
    gen = [f for f in allf if f.get("gen") is True]
    uncl = [f for f in allf if f.get("gen") is None]
    q = drift.get("query") or {}
    if not rels:
        lines.append("Every connector carries every press bug fix released since the fleet's oldest print.")
    else:
        hit = sorted({s for f in listed for s in f["missing"]})
        lines += [f"{len(gen)} untriaged fix(es) that changed generated code and {len(uncl)} unclassified "
                  f"(its PR's files could not be read, or it has no PR), from {len(rels)} release(s), missing "
                  f"from {len(hit)} connector(s). {len(allf) - len(listed)} other fix(es) changed no generated "
                  f"code and are not listed. PR file lists queried: {q.get('asked', 0)} of {q.get('prs', 0)} "
                  f"in {q.get('calls', 0)} GraphQL call(s)"
                  + (f" (stopped: {q['stopped']})" if q.get("stopped") else "") + ".", ""]
        if listed:
            lines += ["By print version (connectors oldest first; untriaged fixes missing, unclassified included):",
                      ""]
            for pv in sorted(set(fleet.values()))[:MAX_PRINT_VERSIONS]:
                at = sorted(s for s, v in fleet.items() if v == pv)
                n = max(sum(1 for f in listed if s in f["missing"]) for s in at)
                if n:
                    lines.append(f"- {vstr(pv)}, {len(at)} connector(s), {n} fix(es): {_slugs(at)}")
            lines.append("")
        full, rest = True, []
        for rel in rels:
            shown = [f for f in rel["fixes"] if live(f) and f.get("gen") is True]
            unk = [f for f in rel["fixes"] if live(f) and f.get("gen") is None]
            other = [f for f in rel["fixes"] if live(f) and f.get("gen") is False]
            dropped = [f for f in rel["fixes"] if gone(f)]
            if not (shown or unk or dropped or (full_log and other)):
                continue
            url = f"https://github.com/{PRESS_REPO}/releases/tag/v{vstr(rel['version'])}"
            pub = f", published {rel['published']}" if rel["published"] else ""
            newest_before = vstr(max(fleet[s] for s in rel["before"]))
            block = [f"### v{vstr(rel['version'])}{pub} ([release notes]({url}))", "",
                     f"Missing from the {len(rel['before'])} connector(s) printed on {newest_before} or older "
                     f"(named above, by print version), except where a line says otherwise.", ""]
            for f in shown + unk:
                note = " Unclassified." if f.get("gen") is None else ""
                if f["carried"]:
                    note += (f" Missing from {len(f['missing'])} ({_slugs(f['missing'])}); {f['carried']} have it: "
                             + ", ".join(f["carried_by"]) + ".")
                block.append(f"- `{f['scope'] + ': ' if f['scope'] else ''}{f['title']}{_ref(f)}`{note}")
            if dropped:
                block.append("- Carried or triaged for every older connector (dropped; check the citation if it "
                             "looks wrong): " + ", ".join(f"`{f['title'][:60]}{_ref(f)}` via "
                                                          + ", ".join(f["carried_by"]) for f in dropped))
            if full_log and other:
                block.append("- Changed no generated code (not in the issue): "
                             + ", ".join(f"`{f['title'][:60]}{_ref(f)}`" for f in other))
            block.append("")
            text = "\n".join(block)
            used = sum(len(x) + 1 for x in lines)
            if full and used + len(text) > budget:
                full = False
            if full:
                lines.append(text)
            elif used < budget + SUMMARY_ROOM:
                lines.append(f"- v{vstr(rel['version'])}: {len(shown)} fix(es), {len(unk)} unclassified, "
                             f"{len(rel['before'])} connector(s) printed before it ([release notes]({url}); "
                             "the full list is in the verify-watch run log)")
            else:
                rest.append((rel, len(shown) + len(unk)))
        if rest:
            lines.append(f"- ...and {len(rest)} older release(s) (v{vstr(rest[-1][0]['version'])} to "
                         f"v{vstr(rest[0][0]['version'])}), {sum(n for _, n in rest)} fix(es): the full list is "
                         "in the verify-watch run log")
    odd = drift.get("odd", [])
    lines += [f"- Note: {o}" for o in odd[:MAX_NOTES]]
    if len(odd) > MAX_NOTES:
        lines.append(f"- Note: ...and {len(odd) - MAX_NOTES} more (see the verify-watch run log)")
    # The hard cap covers every part (a fleet of many print versions with long
    # names, many notes): whatever is cut is named, and the run log has it all.
    cap = budget + SUMMARY_ROOM + 1500
    out, used = [], 0
    for ln in lines:
        if used + len(ln) + 1 > cap:
            out.append("- ...the section is cut here to fit the issue; the full list is in the verify-watch run log")
            break
        out.append(ln)
        used += len(ln) + 1
    return "\n".join(out)


def _fixture_lookup(path: str | None):
    """A lookup for classify_drift from a {"<pr>": [paths]} file; a PR it does
    not name is unclassified. With no file every PR is unclassified."""
    table = json.loads(Path(path).read_text(encoding="utf-8")) if path else {}

    def lookup(prs):
        got = {p: (generated_change(table[str(p)]) if isinstance(table.get(str(p)), list) else None) for p in prs}
        return got, {"prs": len(prs), "asked": sum(1 for p in prs if str(p) in table), "calls": 0,
                     "unreadable": sum(1 for v in got.values() if v is None),
                     "stopped": "" if path else "no --press-pr-files fixture"}
    return lookup


class _NoDriftFixture(Exception):
    """A fixture run named --press-release but no --press-releases: stay offline."""


def press_releases() -> list[dict]:
    """Every published cli-printing-press release (a list, newest first). A page
    that is not a list, or more than MAX_PAGES pages, raises: a partial list
    could hide a fix."""
    out: dict = {}
    for page in range(1, MAX_PAGES + 1):
        got = gh_json("-X", "GET", f"repos/{PRESS_REPO}/releases", "-f", "per_page=100", "-f", f"page={page}")
        if not isinstance(got, list):
            raise RuntimeError(f"releases page {page} is not a list")
        if not got:
            return list(out.values())
        for r in got:
            if not isinstance(r, dict) or r.get("id") is None:
                raise RuntimeError(f"releases page {page}: an item without an id")
            out[r["id"]] = r
    raise RuntimeError(f"releases: more than {MAX_PAGES} pages; list incomplete")


def press_issue(release: dict, fleet: dict, drift: str | None = None) -> tuple[str, str] | None:
    latest = parse_version(release.get("tag_name"))
    newest = max(fleet.values()) if fleet else (0, 0, 0)
    if latest <= newest:
        return None
    at = sum(1 for v in fleet.values() if v == newest)
    # Third-party text lands in an issue body: only a link under the press's own
    # releases path is used verbatim; anything else is rebuilt from the tag.
    url = str(release.get("html_url") or "")
    if not re.fullmatch(rf"https://github\.com/{re.escape(PRESS_REPO)}/releases/tag/[A-Za-z0-9._-]+", url):
        url = f"https://github.com/{PRESS_REPO}/releases/tag/v{vstr(latest)}"
    published = str(release.get("published_at") or "")[:10]
    body = "\n".join([
        PRESS_MARKER,
        f"cli-printing-press {release.get('tag_name')} is out"
        f"{f' (published {published})' if published else ''}: {url}",
        "",
        f"The fleet's newest `printing_press_version` is {vstr(newest)} ({at} connector(s) at it, "
        f"{len(fleet)} with a recorded version).",
        "",
        "This is input for the reprint decision. Nothing reprints automatically. verify-watch.yml updates "
        "this issue when a newer release appears and closes it once the fleet has caught up.",
        *([drift] if drift is not None else []),
    ]) + "\n"
    return f"press-watch: cli-printing-press {release.get('tag_name')} is newer than the fleet ({vstr(newest)})", body


# --------------------------------------------------------------------------- #
# Issues: one per label, found by label + marker, never duplicated.
# --------------------------------------------------------------------------- #

class FakeIssues:
    """In-memory (optionally file-backed) stand-in for the GitHub issues API."""

    def __init__(self, path: Path | None = None):
        self.path = path
        self.issues = json.loads(path.read_text(encoding="utf-8")) if path and path.is_file() else []

    def save(self) -> None:
        if self.path:
            self.path.write_text(json.dumps(self.issues, indent=1) + "\n", encoding="utf-8")

    def ensure_label(self, name: str) -> None:
        pass

    def list_open(self, label: str) -> list[dict]:
        return [i for i in self.issues if i["state"] == "open" and label in i["labels"]]

    def create(self, title: str, body: str, label: str) -> int:
        n = max([i["number"] for i in self.issues] or [0]) + 1
        self.issues.append({"number": n, "title": title, "body": body, "labels": [label], "state": "open"})
        return n

    def edit(self, number: int, title: str, body: str) -> None:
        for i in self.issues:
            if i["number"] == number:
                i.update(title=title, body=body)

    def close(self, number: int, comment: str) -> None:
        for i in self.issues:
            if i["number"] == number:
                i["state"] = "closed"
                i.setdefault("comments", []).append(comment)


class GhIssues:
    def __init__(self, repo: str):
        self.repo = repo

    def _api(self, *args: str, inp: dict | None = None) -> str:
        argv = ["gh", "api", *args]
        if inp is not None:
            argv += ["--input", "-"]
        r = subprocess.run(argv, input=json.dumps(inp) if inp is not None else None,
                           capture_output=True, text=True, timeout=120)
        if r.returncode != 0:
            raise RuntimeError(f"gh api {' '.join(args[:3])} failed: {r.stderr.strip()[:300]}")
        return r.stdout

    def ensure_label(self, name: str) -> None:
        color, desc = LABELS[name]
        try:
            self._api(f"repos/{self.repo}/labels/{name}")
        except RuntimeError:
            self._api("-X", "POST", f"repos/{self.repo}/labels", inp={"name": name, "color": color,
                                                                       "description": desc})

    def list_open(self, label: str) -> list[dict]:
        # One compact JSON object per line (@json), so a body containing "][" or
        # a newline can never be mistaken for a page boundary.
        out = self._api("-X", "GET", "--paginate", f"repos/{self.repo}/issues",
                        "-f", f"labels={label}", "-f", "state=open", "-f", "per_page=100",
                        "-q", ".[] | select(.pull_request == null) | {number, title, body, state, "
                              "labels: [.labels[].name]} | @json")
        items = [json.loads(ln) for ln in out.splitlines() if ln.strip()]
        return [{**i, "body": i.get("body") or ""} for i in items]

    def create(self, title: str, body: str, label: str) -> int:
        out = self._api("-X", "POST", f"repos/{self.repo}/issues",
                        inp={"title": title, "body": body, "labels": [label]})
        return json.loads(out)["number"]

    def edit(self, number: int, title: str, body: str) -> None:
        self._api("-X", "PATCH", f"repos/{self.repo}/issues/{number}", inp={"title": title, "body": body})

    def close(self, number: int, comment: str) -> None:
        self._api("-X", "POST", f"repos/{self.repo}/issues/{number}/comments", inp={"body": comment})
        self._api("-X", "PATCH", f"repos/{self.repo}/issues/{number}",
                  inp={"state": "closed", "state_reason": "completed"})


CLOSE_NOTE = {MAP_MARKER: "Nothing is stale on this verify-watch run; closing. A new issue opens if that changes.",
              PRESS_MARKER: "The fleet has caught up with the latest cli-printing-press release; closing."}


def plan(open_issues: list[dict], marker: str, want: tuple[str, str] | None) -> list[tuple]:
    """The actions that make the open issues match `want`. Pure."""
    mine = sorted((i for i in open_issues if marker in (i.get("body") or "")), key=lambda i: i["number"])
    acts: list[tuple] = []
    if want is None:
        return [("close", i["number"], CLOSE_NOTE[marker]) for i in mine]
    title, body = want
    if not mine:
        return [("create", title, body)]
    keep = mine[0]
    if keep.get("title") != title or keep.get("body") != body:
        acts.append(("edit", keep["number"], title, body))
    for dup in mine[1:]:
        acts.append(("close", dup["number"], f"Duplicate of #{keep['number']}; closing."))
    return acts


def sync(backend, label: str, marker: str, want: tuple[str, str] | None, apply: bool) -> list[tuple]:
    acts = plan(backend.list_open(label), marker, want)
    for a in acts:
        print(f"  {label}: {a[0]} {a[1] if a[0] != 'create' else repr(a[1])}" + ("" if apply else "  (dry run)"))
        if not apply:
            continue
        if a[0] == "create":
            backend.ensure_label(label)
            n = backend.create(a[1], a[2], label)
            print(f"  {label}: opened #{n}")
        elif a[0] == "edit":
            backend.edit(a[1], a[2], a[3])
        else:
            backend.close(a[1], a[2])
    if not acts:
        print(f"  {label}: no change")
    return acts


# --------------------------------------------------------------------------- #
# Collection from main's CI runs (the verify-<slug> artifacts).
# --------------------------------------------------------------------------- #

def gh_json(*args: str):
    try:
        r = subprocess.run(["gh", "api", *args], capture_output=True, text=True, timeout=120)
    except (OSError, subprocess.SubprocessError) as e:  # a hung or missing gh is unreadable, never a crash
        raise RuntimeError(f"gh api {' '.join(args[:3])} failed: {e}") from e
    if r.returncode != 0:
        raise RuntimeError(f"gh api {' '.join(args[:3])} failed: {r.stderr.strip()[:300]}")
    return json.loads(r.stdout)


TRUSTED_EVENTS = ("push", "schedule", "workflow_dispatch")
MAX_PAGES = 20  # 2000 items; a listing longer than that is read as incomplete, never truncated


def paged(path: str, key: str, *args: str, attempts: int = 3) -> list[dict]:
    """Every item of a paginated GitHub listing, as one consistent snapshot.
    Completeness is counted in unique ids against the reported total; if the
    total changes between pages (a run landed mid-listing and shifted the
    pages) the listing starts over, up to `attempts` times. A missing or
    malformed total, a page that is not a list, an item without an id, an
    empty page before the total, too many pages, or a listing that never holds
    still each raise: a partial listing could hide a connector's newest result."""
    for _ in range(attempts):
        seen: dict = {}
        total = None
        for page in range(1, MAX_PAGES + 1):
            got = gh_json("-X", "GET", path, *args, "-f", "per_page=100", "-f", f"page={page}")
            t = got.get("total_count") if isinstance(got, dict) else None
            batch = got.get(key) if isinstance(got, dict) else None
            if not isinstance(t, int) or isinstance(t, bool) or t < 0 or not isinstance(batch, list):
                raise RuntimeError(f"{path} page {page}: no total_count or no {key} list")
            if total is not None and t != total:
                break  # the listing moved under us: start over
            total = t
            for item in batch:
                if not isinstance(item, dict) or item.get("id") is None:
                    raise RuntimeError(f"{path} page {page}: an item without an id")
                seen[item["id"]] = item
            if len(seen) >= total:
                return list(seen.values())
            if not batch:
                raise RuntimeError(f"{path} ended at {len(seen)} of {total} {key}; listing incomplete")
        else:
            raise RuntimeError(f"{path}: more than {MAX_PAGES} pages; listing incomplete")
    raise RuntimeError(f"{path} kept changing across {attempts} listings; listing incomplete")


def list_runs(repo: str, branch: str, since: dt.date) -> list[dict]:
    """Every completed ci.yml run on `branch` created since `since`."""
    return paged(f"repos/{repo}/actions/workflows/ci.yml/runs", "workflow_runs", "-f", f"branch={branch}",
                 "-f", "status=completed", "-f", f"created=>={since.isoformat()}")


def trusted_run(run: dict, repo: str, branch: str) -> bool:
    """Only main's own runs count: a pull_request run (a fork's branch may also
    be named main) never speaks for main."""
    return (run.get("event") in TRUSTED_EVENTS and run.get("head_branch") == branch
            and (run.get("head_repository") or {}).get("full_name", "").lower() == repo.lower())


def run_artifacts(repo: str, run_id) -> list[dict]:
    return paged(f"repos/{repo}/actions/runs/{run_id}/artifacts", "artifacts")


def collect(repo: str, branch: str, out: Path, today: dt.date, window_days: int = WINDOW_DAYS) -> int:
    """Write the newest verify record per connector from main's own ci.yml runs
    in the window to out/<slug>.json. Artifact zips are read in memory, member
    by basename; nothing is extracted to disk. Returns the number of records."""
    out.mkdir(parents=True, exist_ok=True)
    since = today - dt.timedelta(days=window_days)
    seen: set[str] = set()
    runs = [r for r in list_runs(repo, branch, since) if trusted_run(r, repo, branch)]
    for run in sorted(runs, key=lambda r: (r["created_at"], r["id"]), reverse=True):
        if dt.date.fromisoformat(run["created_at"][:10]) < since:
            continue
        for art in run_artifacts(repo, run["id"]):
            name = art.get("name", "")
            slug = name[len("verify-"):]
            if not name.startswith("verify-") or art.get("expired") or slug in seen:
                continue
            z = subprocess.run(["gh", "api", f"repos/{repo}/actions/artifacts/{art['id']}/zip"],
                               capture_output=True, timeout=120)
            if z.returncode != 0:
                raise RuntimeError(f"cannot download artifact {name} of run {run['id']}")
            gate = verdict = None
            try:
                with zipfile.ZipFile(io.BytesIO(z.stdout)) as zf:
                    for member in zf.namelist():
                        base = member.rsplit("/", 1)[-1]
                        if base == "gate.json" and gate is None:
                            gate = json.loads(zf.read(member))
                        elif base == "verdict.json" and verdict is None:
                            verdict = json.loads(zf.read(member))
            except (zipfile.BadZipFile, ValueError):
                gate = verdict = None
            rec = record_from(gate, verdict)
            if rec is None or rec.get("slug") != slug:
                rec = error_record(slug, f"artifact {name} of run {run['id']} holds no usable verify result")
            rec["source"] = {"run_id": run["id"], "run_url": run.get("html_url"), "event": run.get("event"),
                             "created_at": run["created_at"], "head_sha": run.get("head_sha")}
            (out / f"{slug}.json").write_text(json.dumps(rec, indent=1) + "\n", encoding="utf-8")
            seen.add(slug)
    return len(seen)


# --------------------------------------------------------------------------- #
# Main.
# --------------------------------------------------------------------------- #

def run(args) -> int:
    today = dt.date.fromisoformat(args.today) if args.today else dt.datetime.now(dt.timezone.utc).date()
    root = Path(args.skills_root) if args.skills_root else ROOT
    problems: list[str] = []
    fleet = fleet_versions(root)
    try:
        stamp_press, stamp_date = load_stamp(Path(args.stamp))
        stamp_ok = True
    except (OSError, ValueError, KeyError, TypeError) as e:
        problems.append(f"stamp {args.stamp} unreadable: {e!r}")
        stamp_ok = False

    results: dict[str, dict] | None = None
    try:
        if args.collect:
            rdir = Path(args.results) if args.results else Path(tempfile.mkdtemp(prefix="verify-watch-"))
            n = collect(args.repo, args.branch, rdir, today)
            print(f"collected {n} verify result(s) from {args.branch} CI runs in the last {WINDOW_DAYS} days")
            results = load_results(rdir)
        elif args.results:
            results = load_results(Path(args.results))
    except (RuntimeError, OSError, ValueError, zipfile.BadZipFile) as e:
        problems.append(f"verify results unreadable: {e}")
        results = None
    results_known = results is not None or not (args.collect or args.results)

    if args.issues_fixture:
        backend = FakeIssues(Path(args.issues_fixture))
    else:
        backend = GhIssues(args.repo) if args.repo else None

    print(f"fleet: {len(fleet)} connectors with a press version; newest "
          f"{vstr(max(fleet.values())) if fleet else 'none'}")
    if stamp_ok:
        reasons = map_reasons(fleet, stamp_press, stamp_date, results, today, write_surface=write_surfaces(root))
    else:
        reasons = [f"(d) {args.stamp} is unreadable, so when the map was maintained is unknown"]
    want = map_issue(reasons)
    print(f"verify map: {'STALE' if reasons else 'current'}")
    for r in reasons:
        print(f"  - {r}")
    if want is None and not results_known:
        print("  verify-map-stale: left as it is (verify results unreadable; never closed on missing evidence)")
    elif backend is not None:
        sync(backend, MAP_LABEL, MAP_MARKER, want, args.apply)

    try:
        release = (json.loads(Path(args.press_release).read_text(encoding="utf-8")) if args.press_release
                   else gh_json(f"repos/{PRESS_REPO}/releases/latest"))
        try:
            if args.press_release and not args.press_releases:
                # A fixture run stays offline: the issue body then has no drift section.
                raise _NoDriftFixture("no --press-releases fixture beside --press-release")
            rel_list = (json.loads(Path(args.press_releases).read_text(encoding="utf-8")) if args.press_releases
                        else press_releases())
            if not isinstance(rel_list, list):
                raise ValueError("the press release list is not a JSON list")
            fhf = json.loads(Path(args.fleet_handfixes).read_text(encoding="utf-8"))
            triage = load_triage(json.loads(Path(args.press_fix_triage).read_text(encoding="utf-8")))
            ids = ledger_ids(root)
            d = press_drift(rel_list, fleet, carried_numbers(root, fhf, ids), triage, ids)
            classify_drift(d, _fixture_lookup(args.press_pr_files) if args.press_releases else pr_generated_changes)
            print("press drift (the full list; the issue shows what fits):"
                  + drift_section(d, budget=10 ** 9, full_log=True).replace("\n", "\n  "))
            drift = drift_section(d)
        except _NoDriftFixture as e:
            print(f"press drift: not computed ({e})")
            drift = None
        except (RuntimeError, OSError, ValueError, KeyError, TypeError, AttributeError) as e:
            problems.append(f"press drift inputs unreadable: {e}")
            drift = drift_section(None, error=str(e))
        pwant = press_issue(release, fleet, drift)
        print(f"press: latest {release.get('tag_name')} ({release.get('html_url')}); "
              f"{'NEWER than the fleet' if pwant else 'the fleet is current'}")
        if backend is not None:
            sync(backend, PRESS_LABEL, PRESS_MARKER, pwant, args.apply)
    except (RuntimeError, OSError, ValueError, KeyError) as e:
        problems.append(f"press release unreadable: {e}")

    if isinstance(backend, FakeIssues) and args.apply:
        backend.save()
    for p in problems:
        print(f"ERROR: {p}")
    return 1 if problems else 0


def self_test() -> int:
    bad: list[str] = []

    def expect(cond: bool, label: str) -> None:
        print(("  ok   " if cond else "  FAIL ") + label)
        if not cond:
            bad.append(label)

    v = parse_version
    today = dt.date(2026, 10, 23)
    fleet = {"cipp": v("4.24.0"), "servosity": v("4.24.0"), "zammad": v("4.33.5")}
    healthy = {"cipp": {"slug": "cipp", "press_version": "4.24.0", "gate_exit": 0, "errors": [],
                        "features": {"install": "PASS", "mcp-stdio": "PASS", "mcp-http": "FAIL",
                                     "write-gating": "FAIL", "read-only": "FAIL", "local-data": "FAIL"}}}

    def reasons(**kw):
        a = dict(fleet=fleet, stamp_press=v("4.34.0"), stamp_date=today, results=healthy, today=today)
        a.update(kw)
        return map_reasons(**a)

    print("verify-map-stale triggers (each fires on its shape, silent on the healthy fleet):")
    expect(reasons() == [], "healthy fleet, fresh stamp, no ERROR or SKIP -> nothing stale")
    r = reasons(fleet={**fleet, "cipp": v("4.35.0")})
    expect(len(r) == 1 and r[0].startswith("(a)") and "4.35.0" in r[0], "(a) newest press above the stamp -> stale")
    expect(reasons(fleet={**fleet, "cipp": v("4.34.0")}) == [], "(a) newest press EQUAL to the stamp -> silent")
    err = {"cipp": {**healthy["cipp"], "gate_exit": 2,
                    "features": {**healthy["cipp"]["features"], "mcp-http": "ERROR"},
                    "reasons": {"mcp-http": "default port 7777 is already in use"}}}
    r = reasons(results=err)
    expect(len(r) == 1 and r[0].startswith("(b)") and "7777" in r[0], "(b) a verify ERROR on main -> stale")
    r = reasons(results={})
    expect(len(r) == 1 and r[0].startswith("(b) no verify gate results"),
           "(b) no results recorded in the window -> stale (fails closed, never silently current)")
    expect(reasons(results=None) == [], "(b)/(c) not requested (results=None) -> not judged")
    old_skip = {"cipp": {**healthy["cipp"], "features": {**healthy["cipp"]["features"], "write-gating": "SKIP"}}}
    expect(reasons(results=old_skip) == [], "(c) write-gating SKIP at press 4.24.0 -> silent (may lack _execute)")
    new_skip = {"x": {"slug": "x", "press_version": "4.34.0", "gate_exit": 0, "errors": [],
                      "features": {f: "PASS" for f in FEATURE_IDS} | {"read-only": "SKIP"}}}
    r = reasons(results=new_skip)
    expect(len(r) == 1 and r[0].startswith("(c)") and "read-only" in r[0],
           "(c) read-only SKIP at press 4.34.0 -> stale")
    always = {"cipp": {**healthy["cipp"], "features": {**healthy["cipp"]["features"], "mcp-stdio": "SKIP"}}}
    r = reasons(results=always)
    expect(len(r) == 1 and r[0].startswith("(c)"), "(c) mcp-stdio SKIP at any press -> stale")
    print("(c) write-gating is expected only where the connector has <slug>_execute:")
    wskip = {"x": {"slug": "x", "press_version": "4.34.0", "gate_exit": 0, "errors": [],
                   "features": {f: "PASS" for f in FEATURE_IDS} | {"write-gating": "SKIP"}}}
    expect(reasons(results=wskip, write_surface={"x": False}) == [],
           "(c) write-gating SKIP at press 4.34.0, no code-orchestration write endpoint -> silent")
    r = reasons(results=wskip, write_surface={"x": True})
    expect(len(r) == 1 and r[0].startswith("(c)") and "write-gating" in r[0],
           "(c) write-gating SKIP at press 4.34.0 with <slug>_execute expected -> stale")
    r = reasons(results=wskip, write_surface={})
    expect(len(r) == 1 and r[0].startswith("(c)") and "write-gating" in r[0],
           "(c) write-gating SKIP where the write surface is unknown -> stale (fails closed)")
    old_w = {"cipp": {**healthy["cipp"], "features": {**healthy["cipp"]["features"], "write-gating": "SKIP"}}}
    expect(reasons(results=old_w, write_surface={"cipp": True}) == [],
           "(c) write-gating SKIP below press 4.34.0 -> silent even when _execute exists")
    code_w = {"mcp": {"orchestration": "code"}, "tools": [{"name": "a", "method": "GET"},
                                                          {"name": "b", "method": "post"}]}
    expect(expects_execute(code_w) is True, "tools manifest: code orchestration + a POST tool -> _execute")
    expect(expects_execute({**code_w, "tools": [{"name": "a", "method": "GET"}]}) is False,
           "tools manifest: code orchestration, GET tools only -> no _execute")
    expect(expects_execute({"tools": code_w["tools"]}) is False,
           "tools manifest: per-endpoint tools (no code orchestration) with a POST -> no _execute")
    expect(expects_execute({"mcp": {"orchestration": "endpoint"}, "tools": code_w["tools"]}) is False,
           "tools manifest: another orchestration mode with a POST -> no _execute")
    expect(expects_execute(["not", "a", "manifest"]) is None, "tools manifest that is not an object -> unknown")
    for bad_tools in (None, "x", {"a": 1}):
        expect(expects_execute({"mcp": {"orchestration": "code"}, "tools": bad_tools}) is None,
               f"tools manifest whose tools is {bad_tools!r} -> unknown (never read as no write endpoint)")
    expect(expects_execute({"mcp": {"orchestration": "code"}}) is None, "tools manifest with no tools -> unknown")
    expect(expects_execute({"mcp": {"orchestration": "code"}, "tools": [{"name": "a", "method": "GET"}, "x"]})
           is None, "a tool entry that is not an object -> unknown")
    expect(expects_execute({"mcp": {"orchestration": "code"}, "tools": [{"name": "a"}]}) is None,
           "a tool entry with no method -> unknown")
    expect(expects_execute({"mcp": "code", "tools": code_w["tools"]}) is None, "an mcp entry that is not an object -> unknown")
    with tempfile.TemporaryDirectory(prefix="verify-watch-ws-") as td:
        sk = Path(td) / "skills"
        for slug, man, src in (("m-yes", code_w, None), ("m-no", {"tools": code_w["tools"]}, None),
                               ("s-yes", None, 'mcplib.NewTool("s-yes_execute",'),
                               ("s-no", None, 'mcplib.NewTool("s-no_search",'),
                               ("s-test-only", None, None), ("bad", "{not json", None),
                               ("s-no-tools", None, "package mcp\n")):
            (sk / slug / "cli" / "internal" / "mcp").mkdir(parents=True)
            if man is not None:
                (sk / slug / "press").mkdir()
                (sk / slug / "press" / "tools-manifest.json").write_text(
                    man if isinstance(man, str) else json.dumps(man), encoding="utf-8")
            if src is not None:
                (sk / slug / "cli" / "internal" / "mcp" / "code_orch.go").write_text(src, encoding="utf-8")
        (sk / "s-test-only" / "cli" / "internal" / "mcp" / "x_test.go").write_text(
            'mcplib.NewTool("s-test-only_execute",', encoding="utf-8")
        (sk / "no-mcp-dir" / "cli").mkdir(parents=True)
        ws = write_surfaces(Path(td))
        expect(ws.get("m-yes") is True and ws.get("m-no") is False,
               "write_surfaces reads skills/<slug>/press/tools-manifest.json both ways")
        expect(ws.get("s-yes") is True and ws.get("s-no") is False,
               "no tools manifest -> the MCP source decides both ways")
        expect("s-test-only" not in ws and "no-mcp-dir" not in ws and "s-no-tools" not in ws,
               "no production MCP source registering a tool (only a _test.go, no directory, no NewTool) -> unknown")
        expect("bad" not in ws, "an unreadable tools manifest -> unknown (left out, so a SKIP fails closed)")
    r = reasons(stamp_date=today - dt.timedelta(days=30))
    expect(len(r) == 1 and r[0].startswith("(d)"), "(d) stamp 30 days old -> stale")
    expect(reasons(stamp_date=today - dt.timedelta(days=29)) == [], "(d) stamp 29 days old -> silent")

    print("press-watch:")
    rel = {"tag_name": "v4.34.0", "html_url": f"https://github.com/{PRESS_REPO}/releases/tag/v4.34.0",
           "published_at": "2026-10-23T14:51:39Z"}
    pw = press_issue(rel, fleet)
    expect(pw is not None and "v4.34.0" in pw[0] and rel["html_url"] in pw[1] and "4.33.5" in pw[1],
           "release newer than the fleet -> issue names the version, the link and the fleet's newest")
    odd = press_issue({**rel, "html_url": "https://evil.example/x) [click](https://evil.example"}, fleet)
    expect(odd is not None and "evil" not in odd[1] and f"{PRESS_REPO}/releases/tag/v4.34.0" in odd[1],
           "a release link outside the press's releases path -> rebuilt from the tag")
    expect(press_issue({**rel, "tag_name": "v4.33.5"}, fleet) is None, "release equal to the fleet -> no issue")
    expect(press_issue({**rel, "tag_name": "v4.30.0"}, fleet) is None, "release older than the fleet -> no issue")
    try:
        press_issue({**rel, "tag_name": "nightly"}, fleet)
        expect(False, "unparseable release tag -> raises (never read as current)")
    except ValueError:
        expect(True, "unparseable release tag -> raises (never read as current)")

    _drift_self_test(expect)

    print("issue sync (idempotent, no duplicates, closes when clear):")
    fake = FakeIssues()
    want = map_issue(["(d) old"])
    sync(fake, MAP_LABEL, MAP_MARKER, want, True)
    expect(len(fake.list_open(MAP_LABEL)) == 1, "stale -> one issue opened")
    expect(sync(fake, MAP_LABEL, MAP_MARKER, want, True) == [], "same reasons again -> no action, no duplicate")
    want2 = map_issue(["(a) newer", "(d) old"])
    acts = sync(fake, MAP_LABEL, MAP_MARKER, want2, True)
    expect([a[0] for a in acts] == ["edit"] and len(fake.list_open(MAP_LABEL)) == 1
           and fake.list_open(MAP_LABEL)[0]["body"] == want2[1], "reasons changed -> edited in place")
    fake.issues.append({"number": 99, "title": "copy", "body": want2[1], "labels": [MAP_LABEL], "state": "open"})
    acts = sync(fake, MAP_LABEL, MAP_MARKER, want2, True)
    expect(acts == [("close", 99, "Duplicate of #1; closing.")] and len(fake.list_open(MAP_LABEL)) == 1,
           "a duplicate open copy -> closed, the oldest kept")
    fake.issues.append({"number": 100, "title": "unrelated", "body": "hand-written", "labels": [MAP_LABEL],
                        "state": "open"})
    acts = sync(fake, MAP_LABEL, MAP_MARKER, None, True)
    expect([a[0] for a in acts] == ["close"] and [i["number"] for i in fake.list_open(MAP_LABEL)] == [100],
           "nothing stale -> our issue closed; a hand-written issue with the label is left alone")
    expect(sync(fake, MAP_LABEL, MAP_MARKER, None, True) == [], "nothing stale again -> no action")
    dry = FakeIssues()
    sync(dry, PRESS_LABEL, PRESS_MARKER, pw, False)
    expect(dry.issues == [], "without --apply -> nothing written")

    print("records:")
    expect(record_from(None, {"slug": "a", "press_version": "4.24.0", "harness_problems": ["x"],
                              "features": {"install": {"verdict": "PASS", "reason": "r"}}})["errors"][0] == "x",
           "verdict.json fallback keeps harness problems (and names missing features)")
    expect(record_from(None, None) is None, "no gate.json and no verdict.json -> no record")

    print("collection from main's CI runs (offline, fake gh):")
    for label, want, runs, arts, zips in _collect_cases():
        got = _drive_collect(runs, arts, zips, today)
        expect(any(r.startswith("(b) verify ERROR") for r in got) is want, label)
    real_gh = globals()["gh_json"]
    for label, pages in (
            ("an empty page before the reported total -> listing raises (unreadable, never partial)",
             {1: {"total_count": 101, "workflow_runs": [_run(i) for i in range(1, 101)]},
              2: {"total_count": 101, "workflow_runs": []}}),
            ("a listing without total_count -> raises", {1: {"workflow_runs": []}}),
            ("a total_count that is not an integer -> raises", {1: {"total_count": "5", "workflow_runs": []}})):
        globals()["gh_json"] = lambda *a, _p=pages: _p[int(next(x for x in a if x.startswith("page="))[5:])]
        try:
            list_runs("Servosity/msp-skills", "main", today)
            expect(False, label)
        except RuntimeError:
            expect(True, label)
        finally:
            globals()["gh_json"] = real_gh
    calls = {"n": 0}

    def shifting(*a):
        page = int(next(x for x in a if x.startswith("page="))[5:])
        calls["n"] += 1
        if calls["n"] <= 2:  # a run completes between page 1 and page 2 of the first listing
            return ({"total_count": 200, "workflow_runs": [{"id": i} for i in range(200, 100, -1)]} if page == 1
                    else {"total_count": 201, "workflow_runs": [{"id": i} for i in range(101, 1, -1)]})
        ids = list(range(201, 0, -1))
        return {"total_count": 201, "workflow_runs": [{"id": i} for i in ids[(page - 1) * 100: page * 100]]}

    globals()["gh_json"] = shifting
    try:
        got = {r["id"] for r in list_runs("Servosity/msp-skills", "main", today)}
        expect(1 in got and len(got) == 201, "pages shifted by a run landing mid-listing -> relisted, no run lost")
    finally:
        globals()["gh_json"] = real_gh
    nover = {"slug": "cipp", "press_version": None, "gate_exit": 0, "errors": [],
             "features": {f: "PASS" for f in FEATURE_IDS} | {"read-only": "SKIP"}}
    expect(valid_record(nover), "a record with no press version (a connector not printed by the press) is valid")
    expect(valid_record({**nover, "press_version": "unknown"}),
           "verify.py's 'unknown' press version (manifest records none) is valid")
    expect(not valid_record({**nover, "press_version": "4.x"}),
           "a press version that is present but unreadable is not evidence of health")
    expect(not valid_record({**nover, "features": {"install": "PASS"}}),
           "a no-press record missing feature verdicts is still not evidence")
    r = map_reasons({**fleet, "cipp": v("4.34.0")}, v("4.34.0"), today, {"cipp": nover}, today)
    expect(len(r) == 1 and r[0].startswith("(c)") and "read-only" in r[0] and "4.34.0" in r[0],
           "a record without a press version for a connector the fleet has at 4.34.0 -> the fleet's version "
           "decides (c) (a missing version never hides a SKIP)")
    expect(map_reasons(fleet, v("4.34.0"), today, {"handmade": {**nover, "slug": "handmade"}}, today) == [],
           "a connector with no press version anywhere -> only install and mcp-stdio SKIPs count")
    expect(valid_record({"slug": "a", "press_version": None, "features": {}, "gate_exit": 2, "errors": ["x"]}),
           "an explicit error record is valid (it flags (b) by itself)")
    many = {f"s{i:02d}": {"slug": f"s{i:02d}", "press_version": "4.24.0", "gate_exit": 2, "errors": [],
                          "features": {f: "ERROR" for f in FEATURE_IDS}, "reasons": {f: "A" * 120 for f in FEATURE_IDS}}
            for i in range(84)}
    body = map_issue(map_reasons(fleet, v("4.34.0"), today, many, today))[1]
    expect(len(body) < 60000 and "more (see the verify-watch run log)" in body,
           f"every connector ERRORing everywhere -> body bounded ({len(body)} chars < 65536)")

    print(f"\n{'PASS' if not bad else 'FAIL'}: verify_watch self-test ({len(bad)} failure(s))")
    return 0 if not bad else 1


_U = f"https://github.com/{PRESS_REPO}"


def _rp_line(scope: str, subject: str, pr: int, sha: str = "36b5c64", tail: str = "") -> str:
    """One release-please bullet, in the exact shape the press's release notes use."""
    s = f"**{scope}:** " if scope else ""
    return f"* {s}{subject} ([#{pr}]({_U}/issues/{pr})) ([{sha}]({_U}/commit/{sha}abcdef)){tail}"


def _triage_self_test(expect, rels, fleet) -> None:
    v = parse_version
    print("press drift: triage ledger and the generated-code filter:")
    ids = {"old": {"fts-caps"}, "zz-old": set(), "mid": set(), "new": set()}
    tri = load_triage({"fixes": {"3185": {"ported": "fts-caps"},
                                 "5398": {"not_affected": "no export command", "slugs": ["zz-old"]},
                                 "4223": {"not_affected": "the clamp is press-side only"}}})
    d = press_drift(rels, fleet, {}, tri, ids)
    fx = {(vstr(r["version"]), f["pr"]): f for r in d["releases"] for f in r["fixes"]}
    expect(fx[("4.25.1", 3185)]["missing"] == ["zz-old"]
           and fx[("4.25.1", 3185)]["carried_by"] == ["triage: ported as `fts-caps`"],
           "triage ported -> dropped only for connectors whose ledger records that hand-fix id")
    expect(fx[("4.35.6", 5398)]["missing"] == ["old", "mid"],
           "triage not_affected with slugs -> dropped for the slugs named, kept for the rest")
    expect(fx[("4.35.6", 4223)]["missing"] == [] and fx[("4.35.6", 4223)]["carried"] == 3,
           "triage not_affected without slugs -> dropped for every connector")
    expect("Carried or triaged for every older connector" in drift_section(d)
           and "`clamp search limit. Closes #3742. (#4223)` via triage: not affected" in drift_section(d),
           "a fix triaged for everyone -> still named on the dropped line, never silently gone")
    expect(fx[("4.35.6", 5221)]["missing"] == ["old", "zz-old", "mid"], "a fix the ledger does not name -> untouched")
    d2 = press_drift(rels, fleet, {}, load_triage({"fixes": {"3185": {"ported": "nobody-has-it"}}}), ids)
    expect(any("nobody-has-it" in o for o in d2["odd"]),
           "a ported id no connector ledger records -> named in a note (it drops nothing)")
    for label, bad in (("no fixes object", {}), ("a key that is not a PR number", {"fixes": {"abc": {"ported": "x"}}}),
                       ("both kinds", {"fixes": {"1": {"ported": "x", "not_affected": "y"}}}),
                       ("neither kind", {"fixes": {"1": {"why": "x"}}}),
                       ("an empty reason", {"fixes": {"1": {"not_affected": " "}}}),
                       ("an unknown key", {"fixes": {"1": {"ported": "x", "slug": ["a"]}}}),
                       ("an empty slugs list", {"fixes": {"1": {"not_affected": "x", "slugs": []}}})):
        try:
            load_triage(bad)
            expect(False, f"triage ledger with {label} -> refused (never drops by accident)")
        except ValueError:
            expect(True, f"triage ledger with {label} -> refused (never drops by accident)")
    try:
        real = load_triage(json.loads(PRESS_FIX_TRIAGE.read_text(encoding="utf-8")))
        expect(bool(real), f"the repository's {PRESS_FIX_TRIAGE.name} is valid ({len(real)} entries)")
    except (OSError, ValueError) as e:
        expect(False, f"the repository's {PRESS_FIX_TRIAGE.name} is valid ({e})")

    expect(generated_change(["internal/generator/templates/store.go.tmpl"])
           and generated_change(["testdata/golden/expected/x/internal/cli/sync.go"]),
           "a template or golden-output change -> changed generated code")
    expect(not generated_change(["internal/pipeline/dogfood.go", "internal/generator/x_test.go",
                                 "internal/generator/templates/x_test.go.tmpl",
                                 "testdata/golden/expected/x/internal/cli/sync_test.go", "README.md"]),
           "press-only code, tests and docs -> no generated-code change")

    def fake(q):
        repo = {}
        for m in re.finditer(r'p(\d+): pullRequest\(number: \d+\) \{ files\(first: 100(, after: "([^"]+)")?\)', q):
            pr, cur = int(m.group(1)), m.group(3)
            if pr == 404:
                repo[f"p{pr}"] = None  # an issue number, not a PR: NOT_FOUND
            elif pr == 7 and cur is None:
                repo[f"p{pr}"] = {"files": {"nodes": [{"path": "README.md"}] * 100,
                                            "pageInfo": {"hasNextPage": True, "endCursor": "Y3Vy"}}}
            elif pr == 7:
                repo[f"p{pr}"] = {"files": {"nodes": [{"path": "internal/generator/templates/a.go.tmpl"}],
                                            "pageInfo": {"hasNextPage": False, "endCursor": "eg"}}}
            else:
                path = "internal/generator/templates/b.go.tmpl" if pr % 2 else "internal/cli/root.go"
                repo[f"p{pr}"] = {"files": {"nodes": [{"path": path}], "pageInfo": {"hasNextPage": False}}}
        return {"rateLimit": {"remaining": 4000}, "repository": repo}

    prs = [1, 2, 404, 7] + list(range(1001, 1101))
    got, st = pr_generated_changes(prs, call=fake)
    expect(got[1] is True and got[2] is False and got[404] is None and got[7] is True,
           "PR files: a template -> True, none -> False, NOT_FOUND -> unclassified, page 2 is read")
    expect(st["calls"] == 3 and st["asked"] == len(prs) and st["unreadable"] == 1,
           f"104 PRs -> {GQL_BATCH} a call, a page-2 follow-up riding in the last batch ({st['calls']} calls)")

    def low(q):
        out = fake(q)
        out["rateLimit"]["remaining"] = GQL_RESERVE
        return out

    got, st = pr_generated_changes(prs, call=low)
    expect(st["calls"] == 1 and "budget" in st["stopped"] and got[1050] is None
           and any(got[p] is False for p in prs),
           "the GraphQL budget at the reserve -> stops asking; the rest stay unclassified, never False")

    def boom(q):
        raise RuntimeError("HTTP 502")

    def holes(q):
        out = fake(q)
        for k in ("p2", "p7"):
            node = out["repository"].get(k)
            if node and not node["files"]["pageInfo"].get("endCursor") == "eg":
                node["files"]["nodes"] = node["files"]["nodes"][:1] + [None]
        return out

    got, _ = pr_generated_changes([2, 7, 1], call=holes)
    expect(got[2] is None and got[7] is True and got[1] is True,
           "a null file node -> unclassified (no-change unproven), on page 1 of a later-matching PR too")
    got, st = pr_generated_changes(prs, call=boom)
    expect(all(x is None for x in got.values()) and "502" in st["stopped"],
           "the GraphQL API failing -> every PR unclassified (listed), never dropped")
    nopr = press_drift([{"id": 1, "tag_name": "v4.25.1", "body": "### Bug Fixes\n\n* **cli:** pushed straight "
                         f"([5a462c3]({_U}/commit/5a462c30752bbf2c))"}], {"old": v("4.24.0")}, {})
    classify_drift(nopr, lambda prs: ({p: False for p in prs}, {"prs": len(prs), "asked": len(prs), "calls": 0,
                                                                 "unreadable": 0, "stopped": ""}))
    expect("`cli: pushed straight (commit 5a462c3)` Unclassified." in drift_section(nopr),
           "a fix with no PR number -> listed as unclassified")


def _drift_self_test(expect) -> None:
    v = parse_version
    print("press drift (fixes older connectors lack):")
    body = "\n".join([
        f"## [4.35.6]({_U}/compare/v4.35.5...v4.35.6) (2026-11-15)", "", "",
        "### Features", "",
        _rp_line("cli", "add a fix-everything command", 5390),
        "", "### Bug Fixes", "",
        _rp_line("cli", "keep dry-run off files, stores, and `profiles`", 5398),
        _rp_line("cli", f"clamp search limit. Closes [#3742]({_U}/issues/3742).", 4223),
        _rp_line("ci", "honor latest codeowner review state", 3068),
        _rp_line("cli", "count novel hooks in the dogfood scorecard", 5208),
        _rp_line("cli", "pin the share remote", 5221, tail=f", closes [#5152]({_U}/issues/5152)"),
        "", "### Miscellaneous Chores", "",
        _rp_line("deps", "fix: bump a module", 5399),
        "Some prose that mentions a fix but is not a bullet.",
    ])
    shared = "### Bug Fixes\n\n" + "\n".join([
        _rp_line("cli", "correct pagination", 123), _rp_line("mcp", "stop leaking tokens", 123),
        _rp_line("deps", "upgrade generated go.mod to patched x/crypto", 124),
        _rp_line("cli", f"stop leaking tokens; follow-up to [#100]({_U}/pull/100)", 125)])
    fx, known = release_fixes(body)
    by = {f["pr"]: f for f in fx}
    expect(known and len(fx) == 5
           and sorted(by, key=lambda k: (k is None, k or 0)) == [3068, 4223, 5208, 5221, 5398],
           "release notes: only the Bug Fixes bullets are read (Features, Chores and prose ignored)")
    expect(by[5398]["kind"] == "fix" and by[5398]["title"] == "keep dry-run off files, stores, and profiles",
           "a fix bullet -> its subject (markup removed) and its PR number")
    expect(by[4223]["refs"] == {4223} and by[4223]["pr"] == 4223
           and by[4223]["title"] == "clamp search limit. Closes #3742.",
           "a subject that mentions a number -> the PR is the bracketed one; a mention is not the fix's own ref")
    expect(by[5221]["refs"] == {5221, 5152}, "a trailing 'closes #N' is a ref of the fix")
    expect(by[3068]["kind"] == "ignored", "a ci-scoped fix -> ignored (never reaches a connector)")
    expect(by[5208]["kind"] == "fix", "a fix that names press tooling is still a fix: its PR's files decide")
    cc, known_cc = release_fixes(f"* fix(cli): stop leaking the token (#77) by @someone in {_U}/pull/77\n"
                                 "* feat: a new thing (#78)\n* chore: tidy (#79)")
    expect(known_cc and [(f["pr"], f["title"]) for f in cc] == [(77, "stop leaking the token")],
           "conventional-commit notes -> only fix: lines, PR from (#N), no @mention kept")
    expect(release_fixes("Just a sentence.")[1] is False, "release notes with no known shape -> flagged")
    expect(release_fixes("## What Changed\n\n- Fix auth token leak (#123)")[1] is False,
           "release notes whose only heading is not a release-please section -> flagged")
    nopr = release_fixes("### Bug Fixes\n\n* **cli:** build from a nested dir "
                         f"([5a462c3]({_U}/commit/5a462c30752bbf2c))")[0]
    expect([(f["pr"], f["sha"], f["title"]) for f in nopr] == [(None, "5a462c3", "build from a nested dir")],
           "a fix pushed without a PR -> listed, with its commit")

    fleet = {"old": v("4.24.0"), "mid": v("4.30.0"), "new": v("4.35.6"), "zz-old": v("4.24.0")}
    rels = [{"id": 1, "tag_name": "v4.35.6", "body": body, "published_at": "2026-11-15T00:00:00Z"},
            {"id": 2, "tag_name": "v4.25.1", "body": "### Bug Fixes\n\n"
             + _rp_line("cli", "index 3-letter all-caps acronyms in generated FTS", 3185)},
            {"id": 3, "tag_name": "v4.24.0", "body": "### Bug Fixes\n\n" + _rp_line("cli", "an old fix", 2000)},
            {"id": 4, "tag_name": "v4.36.0-rc1", "body": "### Bug Fixes\n\n" + _rp_line("cli", "rc", 6000)},
            {"id": 5, "tag_name": "v4.36.0", "draft": True, "body": "### Bug Fixes\n\n" + _rp_line("cli", "d", 6001)},
            {"id": 6, "tag_name": "v4.31.0",
             "body": _rp_line("cli", "index 3-letter all-caps acronyms in generated FTS", 3185)
             .join(["### Bug Fixes\n\n", ""])},
            {"id": 7, "tag_name": "v4.32.0", "body": "### Bug Fixes\n\n" + _rp_line("cli", "another fix", 3185)}]
    d = press_drift(rels, fleet, {})
    fixes = {(vstr(r["version"]), f["pr"]): f for r in d["releases"] for f in r["fixes"]}
    expect(fixes[("4.25.1", 3185)]["missing"] == ["old", "zz-old"],
           "a fix -> listed for the connectors printed before its release (4.24.0), not for 4.30.0 or 4.35.6")
    expect(fixes[("4.35.6", 5398)]["missing"] == ["old", "zz-old", "mid"],
           "a 4.35.6 fix -> listed for 4.24.0 and 4.30.0, NOT for the connector already at 4.35.6")
    expect(("4.24.0", 2000) not in fixes, "a release at the fleet's oldest print -> not read (everyone has it)")
    expect(not any(r["version"] == v("4.36.0") for r in d["releases"]) and d["odd"] == [
        "release tag `v4.36.0-rc1` is not a version; read it by hand"],
        "a draft is skipped; an unparseable tag is named, never silently dropped")
    expect(("4.31.0", 3185) not in fixes, "a fix shipped twice belongs to the first release that shipped it")
    expect(fixes.get(("4.32.0", 3185), {}).get("title") == "another fix",
           "a later bullet citing the same PR with another subject -> another fix, listed")
    sd = press_drift([{"id": 1, "tag_name": "v4.25.1", "body": shared}], fleet, {"old": {100: "follow-up"}})
    got = [(f["pr"], f["title"], f["kind"], f["missing"]) for r in sd["releases"] for f in r["fixes"]]
    expect([g[:2] for g in got] == [(123, "correct pagination"), (123, "stop leaking tokens"),
                                    (124, "upgrade generated go.mod to patched x/crypto"),
                                    (125, "stop leaking tokens; follow-up to #100")],
           "two bullets of one release sharing a PR -> both listed (never merged)")
    expect(got[2][2] == "fix", "a deps-scoped fix -> listed (a go.mod floor reaches connectors), never ignored")
    expect(got[3][3] == ["old", "zz-old"],
           "a hand-fix citing #100 -> NOT a drop for a fix whose subject only mentions #100")
    amb = press_drift([{"id": 1, "tag_name": "v4.25.1", "body": "### Bug Fixes\n\n* **cli:** stop leaking tokens; "
                        f"follow-up to ([#100]({_U}/pull/100)) ([abcdef0]({_U}/commit/abcdef0123456789))"}],
                      {"old": v("4.24.0")}, {"old": {100: "unrelated-fix"}})
    expect("`stop leaking tokens; follow-up to (#100)` via fleet hand-fix `unrelated-fix`" in drift_section(amb)
           and "Every connector carries" not in drift_section(amb),
           "a fix every connector is said to carry -> still named as carried, never silently gone")
    paren = release_fixes("### Bug Fixes\n\n" + _rp_line(
        "cli", f"stop leaking tokens; follow-up to ([#100]({_U}/pull/100))", 123))[0][0]
    expect(paren["pr"] == 123 and paren["refs"] == {123},
           "a parenthesized link inside the subject -> not the PR; the suffix is read from the end")
    long_ = press_drift([{"id": 1, "tag_name": "v4.25.1", "body": "### Bug Fixes\n\n" + _rp_line(
        "cli", "x" * 140 + " first bug", 123)}, {"id": 2, "tag_name": "v4.26.0", "body": "### Bug Fixes\n\n"
        + _rp_line("cli", "x" * 140 + " second bug", 123)}], {"old": v("4.24.0"), "mid": v("4.25.1")}, {})
    expect([f["missing"] for r in long_["releases"] for f in r["fixes"]] == [["old", "mid"], ["old"]],
           "two long subjects that differ only past the display cut -> two fixes (dedupe reads the whole subject)")

    fhf = {"handfixes": {"fts-caps": {"verdict": "replay", "upstream": "cli-printing-press #3185 (4.25.1)"},
                         "dry-run": {"verdict": "replay", "reason": "cli-printing-press#5000, #5398 and #9"},
                         "bare": {"verdict": "replay", "reason": "see #4223 and msp-skills #5221"},
                         "url": {"verdict": "replay", "proof": f"{_U}/pull/5152"}}}
    expect(cited_press_numbers(fhf["handfixes"]["dry-run"]) == {5000, 5398, 9},
           "a 'cli-printing-press#N, #M and #K' citation -> every listed press number")
    expect(cited_press_numbers(fhf["handfixes"]["bare"]) == set(),
           "a bare #N (an msp-skills number) is never read as a press number")
    with tempfile.TemporaryDirectory(prefix="verify-watch-drift-") as td:
        for slug, ids in (("old", ["fts-caps", "local-only"]), ("mid", ["dry-run", "bare", "url"]), ("new", [])):
            (Path(td) / "skills" / slug).mkdir(parents=True)
            (Path(td) / "skills" / slug / "handfixes.json").write_text(
                json.dumps({"slug": slug, "handfixes": [{"id": i} for i in ids]}), encoding="utf-8")
        carried = carried_numbers(Path(td), fhf)
    expect(carried.get("old") == {3185: "fts-caps"} and "new" not in carried,
           "carried press numbers come only from fleet ids the connector's own ledger names")
    d = press_drift(rels, fleet, carried)
    fixes = {(vstr(r["version"]), f["pr"]): f for r in d["releases"] for f in r["fixes"]}
    f = fixes[("4.25.1", 3185)]
    expect(f["missing"] == ["zz-old"] and f["carried_by"] == ["fleet hand-fix `fts-caps`"] and f["carried"] == 1,
           "a fleet hand-fix citing the fix's press PR -> dropped for the connector that carries it, kept for others")
    expect(fixes[("4.35.6", 5398)]["missing"] == ["old", "zz-old"],
           "dropped for 'mid' (its ledger's fleet id cites cli-printing-press #5398)")
    expect(fixes[("4.35.6", 4223)]["missing"] == ["old", "zz-old", "mid"],
           "a hand-fix citing only a bare #4223 -> NOT dropped (false-listed is fine, false-dropped is not)")
    expect(fixes[("4.35.6", 5221)]["missing"] == ["old", "zz-old"],
           "a hand-fix citing the fix's 'closes' issue by press URL -> dropped")

    table = {3185: True, 5398: True, 4223: False, 5208: False, 5221: None}
    classify_drift(d, lambda prs: ({p: table.get(p) for p in prs},
                                   {"prs": len(prs), "asked": len(prs), "calls": 1, "unreadable": 1, "stopped": ""}))
    sec = drift_section(d)
    expect("`cli: index 3-letter all-caps acronyms in generated FTS (#3185)` Missing from 1 (`zz-old`)" in sec
           and "`cli: keep dry-run off files, stores, and profiles (#5398)`" in sec,
           "the section names each fix, its press PR and, where a hand-fix drops it, who still lacks it")
    expect("honor latest codeowner" not in sec and "fix-everything" not in sec and "clamp search limit" not in sec
           and "count novel hooks" not in sec,
           "the section leaves out ci fixes, features and fixes whose PR changed no generated code")
    expect("`cli: pin the share remote (#5221)` Unclassified." in sec,
           "a fix whose PR files could not be read -> listed as unclassified, never dropped")
    expect("clamp search limit" in drift_section(d, budget=10 ** 9, full_log=True)
           and "2 other fix(es) changed no generated code" in sec,
           "a fix that changed no generated code -> counted in the issue, named in the run log")
    expect("Triaging a fix means porting it as one fleet hand-fix" in sec and "only shrinks by triage" in sec,
           "the section says up front what triage is and that only triage shrinks the list")
    expect(sec.index("### v4.35.6") < sec.index("### v4.25.1"), "newest release first")
    expect("- 4.24.0, 2 connector(s)" in sec and "`old`, `zz-old`" in sec, "connectors named by print version")
    _triage_self_test(expect, rels, fleet)
    evil = press_drift([{"id": 9, "tag_name": "v4.35.6", "body": "### Bug Fixes\n\n"
                         + _rp_line("cli", "@mvanhorn see [x](https://evil.example) `rm -rf` #12", 5500)}],
                       fleet, {})
    es = drift_section(evil)
    expect("`cli: @mvanhorn see x rm -rf #12 (#5500)`" in es and "evil.example" not in es,
           "third-party text lands inside one code span: no link, no mention, no autolinked #N")
    many = [{"id": 100 + i, "tag_name": f"v4.{25 + i // 10}.{i % 10}", "body": "### Bug Fixes\n\n" + "\n".join(
        _rp_line("cli", "x" * 120, 7000 + i * 50 + k) for k in range(40))} for i in range(110)]
    big = drift_section(press_drift(many, fleet, {}))
    expect(len(big) <= DRIFT_BUDGET + SUMMARY_ROOM + 1000 and "older release(s) (v4.25.0 to" in big
           and len(press_issue({"tag_name": "v9.0.0"}, fleet, big)[1]) < 65536,
           f"4400 fixes -> section bounded ({len(big)} chars), the rest summarized, the body under 65536")
    wide = {f"{i:02d}-{j:02d}-" + "x" * 100: (4, i, 0) for i in range(1, 41) for j in range(15)}
    wb = press_issue({"tag_name": "v9.0.0"}, wide, drift_section(press_drift(
        [{"id": 1, "tag_name": "v5.0.0", "body": "### Bug Fixes\n\n" + _rp_line("cli", "fix", 123)}], wide, {})))[1]
    expect(len(wb) < 65536 and "the section is cut here" in wb,
           f"600 long-named connectors at 40 print versions -> the body is still bounded ({len(wb)} chars)")
    expect(len(drift_section(press_drift(many, fleet, {}), budget=10 ** 9)) > len(big),
           "the run log gets the full, unbounded list")
    expect("Every connector carries" in drift_section(press_drift(rels, {"a": v("4.35.6")}, {})),
           "a fleet at the newest release -> says nothing is missing (never an empty list)")
    err = drift_section(None, error="gh api failed: HTTP 502 `x`")
    expect("could not be read this run" in err and "HTTP 502 x" in err,
           "the release list unreadable -> the section says so (never a silent empty list)")
    rel = {"tag_name": "v4.36.0", "html_url": f"{_U}/releases/tag/v4.36.0"}
    pw = press_issue(rel, fleet, err)
    expect(pw is not None and "is out" in pw[1] and "could not be read" in pw[1] and len(pw[1]) < 65536,
           "the press-watch body keeps its 'newer than the fleet' lines and carries the section")
    real_gh = globals()["gh_json"]
    pages = {1: [{"id": i, "tag_name": "v4.25.1"} for i in range(100)], 2: [{"id": 100}], 3: []}
    globals()["gh_json"] = lambda *a: pages[int(next(x for x in a if x.startswith("page="))[5:])]
    try:
        expect(len(press_releases()) == 101, "the release list is read page by page until an empty page")
        globals()["gh_json"] = lambda *a: {"message": "Not Found"}
        try:
            press_releases()
            expect(False, "a release page that is not a list -> raises (unreadable, never empty)")
        except RuntimeError:
            expect(True, "a release page that is not a list -> raises (unreadable, never empty)")
    finally:
        globals()["gh_json"] = real_gh


def _zip(**members) -> bytes:
    b = io.BytesIO()
    with zipfile.ZipFile(b, "w") as z:
        for n, obj in members.items():
            z.writestr(n.replace("__", "/"), json.dumps(obj))
    return b.getvalue()


def _gate(slug: str, **verdicts: str) -> dict:
    f = {k: "PASS" for k in FEATURE_IDS}
    f.update({k.replace("_", "-"): x for k, x in verdicts.items()})
    return {"slug": slug, "press_version": "4.24.0", "features": f, "reasons": {k: k for k in f},
            "gate_exit": 2 if "ERROR" in f.values() else 0, "errors": []}


def _run(i: int, event: str = "push", repo: str = "Servosity/msp-skills",
         created: str = "2026-10-22T00:00:00Z") -> dict:
    return {"id": i, "event": event, "head_branch": "main", "head_repository": {"full_name": repo},
            "created_at": created, "html_url": f"run/{i}", "head_sha": f"sha{i}"}


def _collect_cases():
    """(label, want (b) flagged, runs newest first, {run: [artifact names]}, {(run, name): zip})."""
    err = _gate("cipp", mcp_stdio="ERROR")
    runs = [_run(i) for i in range(1, 102)]
    arts = {i: ["verify-zammad"] for i in range(1, 101)}
    arts[101] = ["verify-cipp"]
    zips = {(i, "verify-zammad"): _zip(**{"gate.json": _gate("zammad")}) for i in range(1, 101)}
    zips[(101, "verify-cipp")] = _zip(**{"gate.json": err})
    yield "an ERROR beyond the first page of runs is still read", True, runs, arts, zips
    yield ("a newer pull_request run (a fork's branch named main) cannot speak for main", True,
           [_run(3, "pull_request", "evil/fork", "2026-10-22T09:00:00Z"), _run(2)],
           {3: ["verify-cipp"], 2: ["verify-cipp"]},
           {(3, "verify-cipp"): _zip(**{"gate.json": _gate("cipp")}), (2, "verify-cipp"): _zip(**{"gate.json": err})})
    verdict = {"slug": "cipp", "press_version": "4.24.0", "harness_problems": [],
               "features": {"mcp-stdio": {"verdict": "ERROR", "reason": "harness failed"}}}
    yield ("an incomplete gate.json falls back to verdict.json's ERROR", True, [_run(1)], {1: ["verify-cipp"]},
           {(1, "verify-cipp"): _zip(**{"g__gate.json": {"slug": "cipp", "features": {}},
                                        "e__cipp__r__verdict.json": verdict})})
    yield ("an incomplete gate.json alone is not evidence of health", True, [_run(1)], {1: ["verify-cipp"]},
           {(1, "verify-cipp"): _zip(**{"gate.json": {"slug": "cipp", "features": {}}})})
    yield ("a corrupt artifact zip is not evidence of health", True, [_run(1)], {1: ["verify-cipp"]},
           {(1, "verify-cipp"): b"not a zip"})
    yield ("main's newest healthy result after an older ERROR -> not flagged", False, [_run(2), _run(1)],
           {2: ["verify-cipp"], 1: ["verify-cipp"]},
           {(2, "verify-cipp"): _zip(**{"gate.json": _gate("cipp")}), (1, "verify-cipp"): _zip(**{"gate.json": err})})


def _drive_collect(runs, arts, zips, today) -> list[str]:
    ids: dict[int, tuple] = {}

    def fake_gh(*args):
        path = next(a for a in args if a.startswith("repos/"))
        q = dict(a.split("=", 1) for a in args if "=" in a and not a.startswith("repos/"))
        per, page = int(q.get("per_page", 30)), int(q.get("page", 1))
        if path.endswith("/workflows/ci.yml/runs"):
            return {"total_count": len(runs), "workflow_runs": runs[(page - 1) * per: page * per]}
        rid = int(path.split("/runs/")[1].split("/")[0])
        listed = []
        for n in arts.get(rid, []):
            ids[len(ids) + 1] = (rid, n)
            listed.append({"id": len(ids), "name": n, "expired": False})
        return {"total_count": len(listed), "artifacts": listed[(page - 1) * per: page * per]}

    class _Done:
        def __init__(self, out: bytes):
            self.returncode, self.stdout, self.stderr = 0, out, b""

    real_run, real_gh = subprocess.run, globals()["gh_json"]

    def fake_run(argv, *a, **k):
        if argv[:2] == ["gh", "api"] and argv[2].endswith("/zip"):
            return _Done(zips[ids[int(argv[2].split("/artifacts/")[1].split("/")[0])]])
        return real_run(argv, *a, **k)

    globals()["gh_json"], subprocess.run = fake_gh, fake_run
    try:
        with tempfile.TemporaryDirectory(prefix="verify-watch-selftest-") as tmp:
            collect("Servosity/msp-skills", "main", Path(tmp), today)
            return map_reasons({"cipp": (4, 24, 0)}, (4, 34, 0), today, load_results(Path(tmp)), today)
    finally:
        globals()["gh_json"], subprocess.run = real_gh, real_run


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--self-test", action="store_true")
    ap.add_argument("--repo", help="OWNER/REPO for artifacts and issues (gh api)")
    ap.add_argument("--branch", default="main", help="whose ci.yml runs to read results from (default main)")
    ap.add_argument("--collect", action="store_true", help="download the newest verify-<slug> artifacts first")
    ap.add_argument("--results", metavar="DIR", help="one <slug>.json record per connector (collect writes here)")
    ap.add_argument("--press-release", metavar="FILE", help="release JSON instead of the GitHub API (fixtures)")
    ap.add_argument("--press-releases", metavar="FILE",
                    help="the press release list (a JSON list) instead of the GitHub API (fixtures)")
    ap.add_argument("--fleet-handfixes", default=str(FLEET_HANDFIXES), metavar="FILE")
    ap.add_argument("--press-fix-triage", default=str(PRESS_FIX_TRIAGE), metavar="FILE")
    ap.add_argument("--press-pr-files", metavar="FILE",
                    help='{"<press PR>": [paths]} instead of the GraphQL API (fixtures; with --press-releases)')
    ap.add_argument("--stamp", default=str(STAMP))
    ap.add_argument("--skills-root", metavar="DIR", help="repo root whose skills/*/manifest.json to read")
    ap.add_argument("--today", metavar="YYYY-MM-DD")
    ap.add_argument("--issues-fixture", metavar="FILE", help="a JSON file standing in for the issues API")
    ap.add_argument("--apply", action="store_true", help="write the issue changes (default: print them)")
    args = ap.parse_args(argv)
    if args.self_test:
        return self_test()
    if args.collect and not args.repo:
        ap.error("--collect needs --repo")
    return run(args)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
