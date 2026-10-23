#!/usr/bin/env python3
"""verify_gate.py - the CI gate over .claude/skills/verify-msp-skills.

The verify skill drives one BUILT connector the way a user does (install.sh,
MCP stdio, MCP HTTP token and loopback, write gating, the read-only switch,
local data) and writes one verdict per feature. This wrapper runs it for one
connector, reads its verdict.json, and compares it with verify_baseline.json:
the features each connector is KNOWN to fail today (features 3 to 6 fail by
design on connectors printed before press 4.34.0 and pass after the reprint).

    python3 tools/maintainer/verify_gate.py --slug cipp
    python3 tools/maintainer/verify_gate.py --slug cipp --policy-base origin/main
    python3 tools/maintainer/verify_gate.py --slug cipp --report-only --record out/cipp.json
    python3 tools/maintainer/verify_gate.py --assert-baseline-shrinks <base-sha>
    python3 tools/maintainer/verify_gate.py --seed-from <dir of verdicts> [--write]
    python3 tools/maintainer/verify_gate.py --self-test

Rules (the same shrink-only model as class_baseline.json):
  * a FAIL that the baseline does not list fails the job (a regression);
  * a listed feature that now PASSes (or SKIPs) fails the job until it is
    removed from the baseline, so the list only shrinks and a later
    regression cannot hide behind a stale entry;
  * an ERROR fails the job, labelled "harness error, not a connector defect",
    unless the pair is in known_errors: the harness ERRORs the fleet already
    had when the gate was seeded. Those are reported with the same label, never
    excused as FAILs, kept visible on main by verify-watch.yml, and only
    shrink (a known error that stops recurring must be removed). A run-level
    harness problem (isolation, no verdict) is never excusable;
  * the baseline only shrinks: --assert-baseline-shrinks (the guards job)
    fails a change that adds a pair. Moving a pair from known_errors to
    connectors (the harness was fixed and the feature now FAILs by design) is
    allowed; moving one the other way is not.

With --policy-base REF (a PR row), a pair is excused only when the baseline
lists it BOTH at REF (the trusted base) and in the working tree, so a PR can
neither widen its own baseline nor drop an entry that still fails. A baseline
file that is absent at a valid REF is the change that introduces it, and the
working file is used (said so in the output); an unreadable REF fails closed.

Exit: 0 green, 1 a connector check failed or the baseline must shrink,
2 harness or gate error (says nothing about the connector), 64 usage.
--report-only prints the same verdict and always exits 0 (baseline seeding).
Stdlib only.
"""

from __future__ import annotations

import argparse
import json
import os
import platform
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
BASELINE = ROOT / "tools" / "maintainer" / "verify_baseline.json"
VERIFY = ROOT / ".claude" / "skills" / "verify-msp-skills" / "scripts" / "verify.py"
FEATURE_IDS = ("install", "mcp-stdio", "mcp-http", "write-gating", "read-only", "local-data")
PASS, FAIL, ERROR, SKIP = "PASS", "FAIL", "ERROR", "SKIP"
EXIT_OK, EXIT_FAIL, EXIT_HARNESS, EXIT_USAGE = 0, 1, 2, 64
HARNESS_LABEL = "harness error, not a connector defect"
BASELINE_COMMENT = (
    "Read by tools/maintainer/verify_gate.py in every CI build row. 'connectors': the "
    ".claude/skills/verify-msp-skills features each connector FAILS on main today; features 3 to 6 "
    "(mcp-http, write-gating, read-only, local-data) fail by design on connectors printed before press "
    "4.34.0 and pass after the reprint. 'known_errors': harness ERRORs (not connector defects) the fleet "
    "had when this was seeded; reported, kept visible by verify-watch.yml, fixed in the skill. Both only "
    "SHRINK: an unlisted FAIL or ERROR fails CI (fix it, never baseline it); a listed pair that no "
    "longer happens fails CI until its entry is removed (in the PR that fixed it); the guards job's "
    "--assert-baseline-shrinks refuses any added pair; PR rows read it from the base commit via "
    "--policy-base. Seeded from Linux CI verdicts (PR #453)."
)
KINDS = ("connectors", "known_errors")


class GateError(Exception):
    """The gate could not decide (bad baseline, unreadable base, no verdict)."""


# --------------------------------------------------------------------------- #
# Baseline: {"connectors": {slug: [FAIL features]}, "known_errors": {slug: [ERROR features]}}.
# --------------------------------------------------------------------------- #

class Baseline:
    def __init__(self, fail: dict[str, set[str]] | None = None, error: dict[str, set[str]] | None = None):
        self.fail = {s: set(v) for s, v in (fail or {}).items() if v}
        self.error = {s: set(v) for s, v in (error or {}).items() if v}

    def f(self, slug: str) -> set[str]:
        return self.fail.get(slug, set())

    def e(self, slug: str) -> set[str]:
        return self.error.get(slug, set())

    def __eq__(self, other) -> bool:
        return isinstance(other, Baseline) and self.fail == other.fail and self.error == other.error

    def pairs(self) -> int:
        return sum(len(v) for v in self.fail.values()) + sum(len(v) for v in self.error.values())


def _parse_kind(data: dict, kind: str) -> dict[str, set[str]]:
    conns = data.get(kind, {})
    if not isinstance(conns, dict):
        raise GateError(f"verify_baseline.json: {kind!r} is not an object")
    out: dict[str, set[str]] = {}
    for slug, feats in conns.items():
        if not isinstance(feats, list) or not all(isinstance(f, str) for f in feats):
            raise GateError(f"verify_baseline.json: {kind}.{slug} is not a list of feature ids")
        bad = sorted(set(feats) - set(FEATURE_IDS))
        if bad:
            raise GateError(f"verify_baseline.json: {kind}.{slug} names unknown feature(s) {bad}")
        if len(set(feats)) != len(feats):
            raise GateError(f"verify_baseline.json: {kind}.{slug} lists a feature twice")
        out[slug] = set(feats)
    return out


def parse_baseline(text: str) -> Baseline:
    """Fails closed on any malformation: a broken baseline must never silently
    excuse a failure."""
    try:
        data = json.loads(text)
    except json.JSONDecodeError as e:
        raise GateError(f"verify_baseline.json is malformed ({e!r})")
    if not isinstance(data, dict) or "connectors" not in data:
        raise GateError("verify_baseline.json is malformed (no 'connectors' object)")
    extra = sorted(set(data) - set(KINDS) - {"_comment"})
    if extra:
        raise GateError(f"verify_baseline.json: unknown key(s) {extra}")
    b = Baseline(_parse_kind(data, "connectors"), _parse_kind(data, "known_errors"))
    both = sorted((s, f) for s in b.fail for f in b.fail[s] & b.e(s))
    if both:
        raise GateError(f"verify_baseline.json: pair(s) listed as both a FAIL and a known error: {both}")
    return b


def dump_baseline(b: Baseline) -> str:
    order = {f: i for i, f in enumerate(FEATURE_IDS)}
    data = {"_comment": BASELINE_COMMENT,
            "connectors": {s: sorted(b.fail[s], key=order.__getitem__) for s in sorted(b.fail)},
            "known_errors": {s: sorted(b.error[s], key=order.__getitem__) for s in sorted(b.error)}}
    return json.dumps(data, indent=1, ensure_ascii=True) + "\n"


def git(*args: str, repo: Path = ROOT) -> subprocess.CompletedProcess:
    return subprocess.run(["git", *args], capture_output=True, text=True, cwd=str(repo), timeout=60)


def resolve_commit(ref: str, repo: Path = ROOT) -> str:
    rp = git("rev-parse", "--verify", "--quiet", f"{ref}^{{commit}}", repo=repo)
    if rp.returncode != 0 or not rp.stdout.strip():
        raise GateError(f"cannot resolve base ref {ref!r} (fails closed: a bad base never passes)")
    return rp.stdout.strip()


def baseline_at(ref: str, repo: Path = ROOT) -> Baseline | None:
    """The baseline at a git ref; None when the ref is a valid commit without
    the file (the change that introduces it). An unreadable ref raises."""
    sha = resolve_commit(ref, repo)
    rel = BASELINE.relative_to(ROOT).as_posix()
    ls = git("ls-tree", "--name-only", sha, "--", rel, repo=repo)
    if ls.returncode != 0:
        raise GateError(f"cannot list {rel} at {ref!r}: {ls.stderr.strip()[:200]}")
    if not ls.stdout.strip():
        return None
    show = git("show", f"{sha}:{rel}", repo=repo)
    if show.returncode != 0:
        raise GateError(f"cannot read {rel} at {ref!r}: {show.stderr.strip()[:200]}")
    return parse_baseline(show.stdout)


def working_baseline(path: Path = BASELINE) -> Baseline:
    if not path.is_file():
        raise GateError(f"baseline file missing: {path}")
    return parse_baseline(path.read_text(encoding="utf-8"))


# --------------------------------------------------------------------------- #
# Decision (pure: the self-test drives it on both shapes).
# --------------------------------------------------------------------------- #

def evaluate(slug: str, record: dict | None, verify_rc: int | None,
             trusted: Baseline, working: Baseline) -> dict:
    """Decide one connector. `record` is verify.py's verdict.json (None when it
    wrote none). Returns {"exit", "lines", "fails", "stale", "errors", "known_errors"}."""
    out = {"fails": [], "stale": [], "errors": [], "known_errors": [], "retired": [], "lines": []}
    lines = out["lines"]
    if record is None:
        out.update(exit=EXIT_HARNESS, errors=["(no verdict.json)"])
        lines.append(f"ERROR ({HARNESS_LABEL}): verify.py wrote no verdict.json (exit {verify_rc})")
        return out
    feats = record.get("features")
    if not isinstance(feats, dict):
        out.update(exit=EXIT_HARNESS, errors=["(malformed verdict.json)"])
        lines.append(f"ERROR ({HARNESS_LABEL}): verdict.json has no features object")
        return out
    t_fail, t_err, w_fail, w_err = trusted.f(slug), trusted.e(slug), working.f(slug), working.e(slug)
    for fid in FEATURE_IDS:
        res = feats.get(fid)
        if not isinstance(res, dict) or res.get("verdict") not in (PASS, FAIL, ERROR, SKIP):
            out["errors"].append(fid)
            lines.append(f"  ERROR {fid:<13} ({HARNESS_LABEL}): feature missing from verdict.json")
            continue
        v, why = res["verdict"], str(res.get("reason", ""))[:220]
        if v == ERROR:
            if fid in t_err and fid in w_err:
                out["known_errors"].append(fid)
                lines.append(f"  known {fid:<13} ERROR ({HARNESS_LABEL}), in known_errors: not gating; "
                             f"verify-watch keeps it open until the skill is fixed: {why}")
            else:
                out["errors"].append(fid)
                lines.append(f"  ERROR {fid:<13} ({HARNESS_LABEL}): {why}")
        elif v == FAIL:
            if fid in w_err:
                out["stale"].append(fid)
                lines.append(f"  FAIL  {fid:<13} now FAILs, no longer ERRORs: move \"{fid}\" of \"{slug}\" from "
                             f"known_errors to connectors in tools/maintainer/verify_baseline.json: {why}")
            elif fid in w_fail and fid in (t_fail | t_err):
                lines.append(f"  ok    {fid:<13} FAIL, baselined (known; fixed by the reprint)")
            elif fid in (t_fail | t_err):
                out["fails"].append(fid)
                lines.append(f"  FAIL  {fid:<13} still FAILs, but this change removed it from "
                             f"verify_baseline.json: {why}")
            else:
                out["fails"].append(fid)
                lines.append(f"  FAIL  {fid:<13} not in verify_baseline.json (a regression; fix the "
                             f"connector, never baseline it): {why}")
        else:
            if fid in w_fail or fid in w_err:
                kind = "connectors" if fid in w_fail else "known_errors"
                out["stale"].append(fid)
                lines.append(f"  FAIL  {fid:<13} now {'passes' if v == PASS else 'skips'} but verify_baseline.json "
                             f"still lists it: remove "
                             f"\"{fid}\" from {kind}.\"{slug}\" in tools/maintainer/verify_baseline.json")
            elif fid in t_fail or fid in t_err:
                out["retired"].append(fid)
                lines.append(f"  ok    {fid:<13} {v}, and this change retired its baseline entry")
            else:
                lines.append(f"  ok    {fid:<13} {v}")
    problems = [str(p) for p in record.get("harness_problems") or []]
    for p in problems:
        lines.append(f"  ERROR harness       ({HARNESS_LABEL}): {p[:220]}")
    # verify.py's exit must be the one its own verdict implies (its exit_code()):
    # a kill, a crash after writing, or any other mismatch means the verdict on
    # disk is not the run's final word, so it is never applied to the baseline.
    vs = [(r or {}).get("verdict") if isinstance(r, dict) else None for r in feats.values()]
    implied = EXIT_HARNESS if problems or ERROR in vs else EXIT_FAIL if FAIL in vs else EXIT_OK
    if verify_rc != implied:
        msg = f"verify.py exited {verify_rc}, but its verdict.json implies exit {implied}"
        problems.append(msg)
        lines.append(f"  ERROR harness       ({HARNESS_LABEL}): {msg}")
    out["errors"] += problems
    if out["errors"]:
        out["exit"] = EXIT_HARNESS
    elif out["fails"] or out["stale"]:
        out["exit"] = EXIT_FAIL
    else:
        out["exit"] = EXIT_OK
    return out


def shrink_violations(head: Baseline, base: Baseline) -> list[tuple[str, str, str]]:
    """Pairs the working baseline excuses that the base did not. A FAIL entry
    may come from either base list (a fixed harness error now FAILs by design);
    a known error may only come from the base's known errors."""
    bad = []
    for s in sorted(head.fail):
        for f in sorted(head.fail[s] - base.f(s) - base.e(s)):
            bad.append(("connectors", s, f))
    for s in sorted(head.error):
        for f in sorted(head.error[s] - base.e(s)):
            bad.append(("known_errors", s, f))
    return bad


# --------------------------------------------------------------------------- #
# Running verify.py.
# --------------------------------------------------------------------------- #

def find_verdict(root: Path, slug: str) -> Path | None:
    """The one verdict.json of a run whose evidence root was empty when it
    started. Zero or several is no verdict: never pick across runs."""
    runs = sorted((root / slug).glob("*/verdict.json")) if (root / slug).is_dir() else []
    return runs[0] if len(runs) == 1 else None


def run_verify(slug: str, evidence_root: Path) -> tuple[int, dict | None, Path | None]:
    argv = [sys.executable, str(VERIFY), "--evidence-root", str(evidence_root), "run", "--slug", slug]
    if platform.system() == "Linux":
        argv += ["--egress-guard", "proxy-env"]
    print("$ " + " ".join(argv), flush=True)
    rc = subprocess.run(argv, cwd=str(ROOT), timeout=3600).returncode
    vpath = find_verdict(evidence_root, slug)
    if vpath is None:
        return rc, None, None
    try:
        return rc, json.loads(vpath.read_text(encoding="utf-8")), vpath
    except (OSError, json.JSONDecodeError):
        return rc, None, vpath


def cmd_slug(args) -> int:
    try:
        working = working_baseline()
        if args.policy_base:
            trusted = baseline_at(args.policy_base)
            if trusted is None:
                print(f"note: verify_baseline.json is new at {args.policy_base}; this change introduces "
                      f"it, so the working file is the baseline (reviewed in this change)")
                trusted = working
        else:
            trusted = working
    except GateError as e:
        print(f"verify-gate: ERROR (gate error, not a connector defect): {e}")
        return EXIT_HARNESS
    base = Path(args.evidence_root) if args.evidence_root else Path(tempfile.gettempdir())
    base.mkdir(parents=True, exist_ok=True)
    root = Path(tempfile.mkdtemp(prefix=f"verify-gate-{args.slug}-", dir=str(base)))  # fresh, empty, this run only
    rc, record, vpath = run_verify(args.slug, root)
    if rc == EXIT_USAGE:
        print(f"verify-gate: ERROR (gate error): verify.py refused {args.slug!r} as a usage error (exit 64)")
        return EXIT_HARNESS
    res = evaluate(args.slug, record, rc, trusted, working)
    press = (record or {}).get("press_version")
    listed = sorted(working.f(args.slug)) + [f"{f} (known error)" for f in sorted(working.e(args.slug))]
    print(f"\n== verify gate: {args.slug} (press {press}, verify.py exit {rc}, "
          f"baseline {listed or 'none'}{' from ' + args.policy_base if args.policy_base else ''})")
    for ln in res["lines"]:
        print(ln)
    word = {EXIT_OK: "PASS", EXIT_FAIL: "FAIL", EXIT_HARNESS: f"ERROR ({HARNESS_LABEL})"}[res["exit"]]
    if res["exit"] == EXIT_OK and res["known_errors"]:
        word = f"PASS with known harness error(s) {res['known_errors']} (tracked, not gating)"
    print(f"verify-gate: {word}" + ("  [report-only: not gating]" if args.report_only else ""))
    if args.record:
        out = Path(args.record)
        out.parent.mkdir(parents=True, exist_ok=True)
        feats = (record or {}).get("features") or {}
        rec = {"slug": args.slug, "press_version": press, "verify_exit": rc, "gate_exit": res["exit"],
               "gate_verdict": word, "report_only": bool(args.report_only),
               "head": (record or {}).get("head"), "host": (record or {}).get("host"),
               "baseline": sorted(working.f(args.slug)), "known_errors_baseline": sorted(working.e(args.slug)),
               "fails": res["fails"], "stale": res["stale"], "errors": res["errors"],
               "known_errors": res["known_errors"], "verdict_json": str(vpath) if vpath else None,
               "features": {k: (v or {}).get("verdict") for k, v in feats.items()},
               "reasons": {k: str((v or {}).get("reason", ""))[:300] for k, v in feats.items()}}
        out.write_text(json.dumps(rec, indent=1, ensure_ascii=True) + "\n", encoding="utf-8")
    return EXIT_OK if args.report_only else res["exit"]


# --------------------------------------------------------------------------- #
# Build-free guards and seeding.
# --------------------------------------------------------------------------- #

def cmd_assert_shrinks(ref: str) -> int:
    try:
        head = working_baseline()
        base = baseline_at(ref)
    except GateError as e:
        print(f"FAIL: {e}")
        return EXIT_FAIL
    if base is None:
        print(f"PASS: verify_baseline.json is new at {ref}; nothing to shrink against "
              f"({head.pairs()} pair(s) introduced, reviewed in this change).")
        return EXIT_OK
    added = shrink_violations(head, base)
    if added:
        print("FAIL: verify_baseline.json added pairs vs the trusted base; the baseline only shrinks. "
              "Fix the connector (or the harness) instead of baselining it:")
        for kind, s, f in added:
            print(f"  - {kind}.{s}: {f}")
        return EXIT_FAIL
    print(f"PASS: verify baseline only shrank vs {ref} ({base.pairs() - head.pairs()} pair(s) retired, none added).")
    return EXIT_OK


def cmd_seed(src: Path, write: bool) -> int:
    """Build the first baseline from collected Linux verdicts (CI artifacts).
    Seeds only: refuses when the working baseline already lists anything, and
    refuses any run-level harness problem (isolation, a crashed run)."""
    recs: dict[str, dict] = {}
    for p in sorted(src.rglob("verdict.json")):
        try:
            d = json.loads(p.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as e:
            print(f"ERROR: unreadable {p}: {e}")
            return EXIT_HARNESS
        slug = d.get("slug")
        if slug in recs:
            print(f"ERROR: two verdicts for {slug!r} under {src}; seed from one run")
            return EXIT_HARNESS
        recs[slug] = d
    if not recs:
        print(f"ERROR: no verdict.json under {src}")
        return EXIT_HARNESS
    if BASELINE.is_file() and working_baseline().pairs():
        print("ERROR: verify_baseline.json already lists pairs; --seed-from only seeds an empty baseline")
        return EXIT_USAGE
    fails, errs, broken = {}, {}, []
    for slug, d in sorted(recs.items()):
        feats = d.get("features") or {}
        if d.get("harness_problems") or any(f not in feats for f in FEATURE_IDS):
            broken.append(slug)
        fails[slug] = {f for f in FEATURE_IDS if (feats.get(f) or {}).get("verdict") == FAIL}
        errs[slug] = {f for f in FEATURE_IDS if (feats.get(f) or {}).get("verdict") == ERROR}
    b = Baseline(fails, errs)
    by_feat = {f: sum(1 for v in b.fail.values() if f in v) for f in FEATURE_IDS}
    print(f"{len(recs)} connector verdicts; {sum(len(v) for v in b.fail.values())} FAIL pair(s) across "
          f"{len(b.fail)} connector(s); per feature: {by_feat}")
    print(f"known harness errors: {sum(len(v) for v in b.error.values())} pair(s): "
          f"{ {s: sorted(v) for s, v in b.error.items()} }")
    if broken:
        print(f"ERROR: run-level harness problems in {broken}; those runs cannot be seeded")
        return EXIT_HARNESS
    if write:
        BASELINE.write_text(dump_baseline(b), encoding="utf-8")
        print(f"wrote {BASELINE.relative_to(ROOT)}")
    return EXIT_OK


# --------------------------------------------------------------------------- #
# Self-test: every rule fires on its broken shape and is silent on the healthy one.
# --------------------------------------------------------------------------- #

def _rec(**verdicts: str) -> dict:
    feats = {f: {"verdict": PASS, "reason": "held"} for f in FEATURE_IDS}
    for k, v in verdicts.items():
        feats[k.replace("_", "-")] = {"verdict": v, "reason": f"{v.lower()} by fixture"}
    return {"slug": "x", "press_version": "4.24.0", "harness_problems": [], "features": feats}


def self_test() -> int:
    bad: list[str] = []

    def expect(cond: bool, label: str) -> None:
        print(("  ok   " if cond else "  FAIL ") + label)
        if not cond:
            bad.append(label)

    old = {"mcp-http", "write-gating", "read-only", "local-data"}
    B = lambda fail=None, err=None: Baseline({"x": set(fail or ())}, {"x": set(err or ())})  # noqa: E731
    pre434 = _rec(mcp_http=FAIL, write_gating=FAIL, read_only=FAIL, local_data=FAIL)
    print("evaluate:")
    expect(evaluate("x", pre434, 1, B(old), B(old))["exit"] == EXIT_OK,
           "pre-4.34 connector failing exactly its baselined features -> green")
    r = evaluate("x", _rec(mcp_http=FAIL, write_gating=FAIL, read_only=FAIL, local_data=FAIL, mcp_stdio=FAIL),
                 1, B(old), B(old))
    expect(r["exit"] == EXIT_FAIL and r["fails"] == ["mcp-stdio"],
           "planted regression (mcp-stdio FAIL, not baselined) -> blocks, names the feature")
    r = evaluate("x", _rec(), 0, B(old), B(old))
    expect(r["exit"] == EXIT_FAIL and r["stale"] == sorted(old, key=FEATURE_IDS.index),
           "reprinted connector passing its baselined features -> told to shrink the baseline")
    expect(evaluate("x", _rec(), 0, B(old), B())["exit"] == EXIT_OK,
           "reprint PR that retired its entries (trusted lists them, working does not) -> green")
    r = evaluate("x", pre434, 1, B(), B(old))
    expect(r["exit"] == EXIT_FAIL and len(r["fails"]) == 4, "PR that ADDS its own baseline entries -> still blocks")
    r = evaluate("x", pre434, 1, B(old), B({"mcp-http"}))
    expect(r["exit"] == EXIT_FAIL and r["fails"] == ["write-gating", "read-only", "local-data"],
           "PR that drops entries which still FAIL -> blocks")
    r = evaluate("x", _rec(read_only=SKIP), 0, B({"read-only"}), B({"read-only"}))
    expect(r["exit"] == EXIT_FAIL and r["stale"] == ["read-only"], "baselined feature now SKIP -> shrink")
    r = evaluate("x", _rec(mcp_http=ERROR), 2, B(), B())
    expect(r["exit"] == EXIT_HARNESS and any(HARNESS_LABEL in ln for ln in r["lines"]),
           "new ERROR -> fails the job, labelled harness error")
    r = evaluate("x", _rec(mcp_http=ERROR), 2, B({"mcp-http"}), B({"mcp-http"}))
    expect(r["exit"] == EXIT_HARNESS, "ERROR on a pair baselined as a FAIL -> still an ERROR (never excused)")
    r = evaluate("x", _rec(write_gating=ERROR), 2, B(err={"write-gating"}), B(err={"write-gating"}))
    expect(r["exit"] == EXIT_OK and r["known_errors"] == ["write-gating"]
           and any(HARNESS_LABEL in ln for ln in r["lines"]),
           "known harness ERROR (in known_errors at base and head) -> reported with the label, not gating")
    r = evaluate("x", _rec(write_gating=ERROR), 2, B(), B(err={"write-gating"}))
    expect(r["exit"] == EXIT_HARNESS, "a PR that adds its own known error -> still blocks")
    r = evaluate("x", _rec(write_gating=FAIL), 1, B(err={"write-gating"}), B(err={"write-gating"}))
    expect(r["exit"] == EXIT_FAIL and r["stale"] == ["write-gating"],
           "known error now FAILs -> told to move it to connectors")
    r = evaluate("x", _rec(write_gating=FAIL), 1, B(err={"write-gating"}), B(fail={"write-gating"}))
    expect(r["exit"] == EXIT_OK, "the harness-fix PR that moved it to connectors -> green")
    r = evaluate("x", _rec(), 0, B(err={"write-gating"}), B(err={"write-gating"}))
    expect(r["exit"] == EXIT_FAIL and r["stale"] == ["write-gating"], "known error now PASSes -> shrink")
    expect(evaluate("x", None, 2, B(old), B(old))["exit"] == EXIT_HARNESS, "no verdict.json -> harness error")
    hp = _rec(write_gating=ERROR)
    hp["harness_problems"] = ["isolation: 1 real path changed"]
    expect(evaluate("x", hp, 2, B(err={"write-gating"}), B(err={"write-gating"}))["exit"] == EXIT_HARNESS,
           "run-level harness problem -> harness error, even beside a known error")
    expect(evaluate("x", _rec(), 2, B(), B())["exit"] == EXIT_HARNESS,
           "verify exit 2 with no ERROR feature -> harness error (never a pass)")
    expect(evaluate("x", pre434, -9, B(old), B(old))["exit"] == EXIT_HARNESS,
           "verify.py killed (-9) after writing a plausible verdict -> harness error")
    expect(evaluate("x", pre434, 0, B(old), B(old))["exit"] == EXIT_HARNESS,
           "verify exit 0 while its verdict holds FAILs -> harness error")
    expect(evaluate("x", _rec(write_gating=ERROR), 1, B(err={"write-gating"}), B(err={"write-gating"}))["exit"]
           == EXIT_HARNESS, "verify exit 1 beside a (known) ERROR -> harness error")
    with tempfile.TemporaryDirectory(prefix="verify-gate-selftest-") as tmp:
        for name in ("20990101T000000Z-1", "20261023T160000Z-2"):
            (Path(tmp) / "x" / name).mkdir(parents=True)
            (Path(tmp) / "x" / name / "verdict.json").write_text("{}", encoding="utf-8")
        expect(find_verdict(Path(tmp), "x") is None, "two verdicts in one evidence root -> none chosen")
        (Path(tmp) / "x" / "20990101T000000Z-1" / "verdict.json").unlink()
        expect(find_verdict(Path(tmp), "x") is not None, "exactly one verdict -> that one")
    missing = _rec()
    del missing["features"]["local-data"]
    expect(evaluate("x", missing, 0, B(), B())["exit"] == EXIT_HARNESS, "feature missing -> harness error")
    expect(evaluate("x", _rec(), 0, B(), B())["exit"] == EXIT_OK, "healthy 4.34 connector -> green")

    print("baseline parsing (fails closed):")
    base = Baseline({"cipp": set(old), "servosity": set(old)}, {"zammad": {"write-gating"}})
    good = dump_baseline(Baseline({"cipp": set(old), "empty": set()}, {"zammad": {"write-gating"}}))
    expect(parse_baseline(good) == Baseline({"cipp": set(old)}, {"zammad": {"write-gating"}}),
           "round trip; empty entries dropped")
    expect(parse_baseline('{"connectors": {}}') == Baseline(), "known_errors is optional")
    for text, label in (("{", "not JSON"), ("{}", "no connectors key"), ('{"connectors": []}', "not an object"),
                        ('{"connectors": {"a": "mcp-http"}}', "entry not a list"),
                        ('{"connectors": {"a": ["mcp-htp"]}}', "unknown feature id"),
                        ('{"connectors": {"a": ["read-only", "read-only"]}}', "duplicate feature"),
                        ('{"connectors": {}, "known_error": {}}', "unknown top-level key"),
                        ('{"connectors": {"a": ["install"]}, "known_errors": {"a": ["install"]}}',
                         "a pair in both lists")):
        try:
            parse_baseline(text)
            expect(False, f"malformed baseline ({label}) -> raises")
        except GateError:
            expect(True, f"malformed baseline ({label}) -> raises")
    expect(good.isascii() and good.endswith("\n"), "dump: ASCII, trailing newline")

    print("shrink guard:")
    expect(not shrink_violations(Baseline({"cipp": {"mcp-http"}}), base), "removing pairs -> pass")
    expect(shrink_violations(Baseline({"cipp": set(old) | {"mcp-stdio"}}), base) == [("connectors", "cipp", "mcp-stdio")],
           "adding a FAIL to a listed connector -> fail")
    expect(shrink_violations(Baseline({"newone": {"install"}}), base) == [("connectors", "newone", "install")],
           "adding a new connector -> fail")
    expect(shrink_violations(Baseline(error={"cipp": {"mcp-stdio"}}), base) == [("known_errors", "cipp", "mcp-stdio")],
           "adding a known error -> fail")
    expect(not shrink_violations(Baseline({"zammad": {"write-gating"}}), base),
           "moving a known error to connectors (harness fixed, FAILs by design) -> pass")
    expect(shrink_violations(Baseline(error={"cipp": {"mcp-http"}}), base) == [("known_errors", "cipp", "mcp-http")],
           "moving a FAIL to known_errors -> fail")

    print("baseline at a git ref (throwaway repo):")
    try:
        resolve_commit("refs/heads/does-not-exist-verify-gate")
        expect(False, "unresolvable base ref -> raises (fails closed)")
    except GateError:
        expect(True, "unresolvable base ref -> raises (fails closed)")
    with tempfile.TemporaryDirectory(prefix="verify-gate-selftest-") as tmp:
        repo = Path(tmp)
        env = {**os.environ, "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.invalid",
               "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.invalid"}

        def commit(msg: str) -> str:
            for argv in (["git", "add", "-A"], ["git", "commit", "-q", "--allow-empty", "-m", msg]):
                subprocess.run(argv, cwd=tmp, env=env, check=True, capture_output=True, timeout=30)
            return git("rev-parse", "HEAD", repo=repo).stdout.strip()

        subprocess.run(["git", "init", "-q"], cwd=tmp, check=True, capture_output=True, timeout=30)
        c0 = commit("no baseline yet")
        path = repo / BASELINE.relative_to(ROOT)
        path.parent.mkdir(parents=True)
        path.write_text(dump_baseline(base), encoding="utf-8")
        c1 = commit("baseline")
        expect(baseline_at(c0, repo) is None,
               "valid commit without the file -> None (the introducing change), never a parse of nothing")
        expect(baseline_at(c1, repo) == base, "commit with the file -> that commit's baseline")
        path.write_text("{not json", encoding="utf-8")
        c2 = commit("broken")
        try:
            baseline_at(c2, repo)
            expect(False, "malformed baseline at the base ref -> raises")
        except GateError:
            expect(True, "malformed baseline at the base ref -> raises")

    print(f"\n{'PASS' if not bad else 'FAIL'}: verify_gate self-test ({len(bad)} failure(s))")
    return EXIT_OK if not bad else EXIT_FAIL


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    mode = ap.add_mutually_exclusive_group(required=True)
    mode.add_argument("--slug", help="run verify.py for this connector and gate it")
    mode.add_argument("--assert-baseline-shrinks", metavar="REF", dest="shrink_base",
                      help="build-free: fail if the working baseline adds a pair the baseline at REF lacks")
    mode.add_argument("--seed-from", metavar="DIR", help="seed an EMPTY baseline from verdict.json files under DIR")
    mode.add_argument("--self-test", action="store_true")
    ap.add_argument("--policy-base", metavar="REF", help="read the trusted baseline from this git ref (PR rows)")
    ap.add_argument("--report-only", action="store_true", help="print the verdict, always exit 0 (seeding)")
    ap.add_argument("--record", metavar="FILE", help="write the gate's verdict as JSON here (CI artifact)")
    ap.add_argument("--evidence-root", metavar="DIR", help="verify.py evidence root (default: a fresh temp dir)")
    ap.add_argument("--write", action="store_true", help="with --seed-from: write verify_baseline.json")
    args = ap.parse_args(argv)
    if args.self_test:
        return self_test()
    if args.shrink_base is not None:
        return cmd_assert_shrinks(args.shrink_base)
    if args.seed_from:
        return cmd_seed(Path(args.seed_from), args.write)
    return cmd_slug(args)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
