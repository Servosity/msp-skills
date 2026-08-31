#!/usr/bin/env python3
"""check_engine_freshness.py - is this connector's ENGINE and its SHIPPED BINARY current?

Run it when a skill flips to live-verified (live-verified.yml does this
automatically), or by hand at any time:

    python3 tools/maintainer/check_engine_freshness.py --slug hudu
    python3 tools/maintainer/check_engine_freshness.py --all

Why this exists
---------------
A live-verified badge means a real MSP confirmed the connector against a real
tenant. That is the moment the connector becomes worth keeping current, and the
moment someone is positioned to notice if it breaks - so it is the right trigger
for asking two questions we otherwise only ask by accident:

  1. Is the vendored engine behind the fleet?  A connector minted on an older
     printing-press carries whatever the press has fixed since. Reprinting is
     the fix, but a reprint is expensive and can clobber hand-fixes, so this
     REPORTS, it never decides. Only a MINOR-version gap is reported: a patch
     behind is not worth a reprint, and an advisory that always fires is an
     advisory nobody reads.

  2. Is the shipped binary behind the tree?  This is the one that actually
     bites. `main` can carry changes for months while every user runs the binary
     from the last tag. Measured 2026-08-16: 41 of 61 connectors were shipping
     binaries built before the 4.24 engine upgrade that had been sitting in main
     since June. A tree-only upgrade reaches nobody.

     This question is answered by `release_state.classify()`, which compares a
     freshly computed CLI hash against `cli_hash_at_release` - NOT by comparing
     engine version stamps. Comparing stamps is a false-GREEN: a security fix
     like the GO-2026-6218 toolchain sweep changes every connector's binary
     without touching a single `printing_press_version`, and a stamp comparison
     would report all of them as fine.

There is no hardcoded "current press version" to keep updated. The bar is the
newest press version anywhere in the fleet, so it rises on its own every time a
connector is minted or reprinted.

Exit codes: 0 = nothing to report. 3 = something is stale (advisory; nothing in
CI gates on this - it is a prompt for a human decision, not a verdict). 1 = the
check itself could not run. 2 is left to argparse for usage errors, so the
caller can tell a real finding from a bad invocation.
"""

from __future__ import annotations

import argparse
import glob
import json
import os
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)
import release_state  # noqa: E402  (local tools/ module)

REPO = str(Path(__file__).resolve().parent.parent.parent)

STALE = 3
ERROR = 1

# A slug reaches this script from an ISSUE BODY (live-verified.yml parses the
# "Which skill" dropdown). The workflow already constrains it to a registry key,
# but this script is also run by hand and must not depend on its caller for
# safety: a slug like "--format=%(refname)" would be read by `git tag --list` as
# an option, and one containing "../" would escape the skills tree.
#
# One definition, from registry.py, so this cannot drift from the grammar the
# build matrix is validated against - and so it carries the same `\Z` anchor
# (Python's `$` also matches before a trailing newline).
SLUG_RE = registry.SLUG_RE


def _press_version(manifest: dict):
    """Engine version from a manifest, top-level or nested `_meta` provenance.

    Mirrors `printing_press_version()` in build-catalog.py, which cannot be
    imported (its filename has a hyphen). `connectwise-manage` carries its
    version only in the nested shape, and reading just the top-level key reports
    it as unstamped.
    """
    version = manifest.get("printing_press_version")
    if version:
        return version
    meta = manifest.get("_meta")
    if not isinstance(meta, dict):
        return None
    press = meta.get("io.github.mvanhorn.cli-printing-press")
    if not isinstance(press, dict):
        return None
    return press.get("printing_press_version")


def _git(*args):
    """git stdout, or '' when git failed.

    Distinguishing "git said no" from "git could not run" matters here: this
    tool posts its conclusions to a contributor's issue as fact, and a failed
    `git tag` must not be rendered as "never released".
    """
    try:
        out = subprocess.run(
            ["git", "-C", REPO, *args], capture_output=True, text=True, timeout=15
        )
    except (OSError, subprocess.SubprocessError):
        return ""
    return out.stdout.strip() if out.returncode == 0 else ""


def _press(slug, ref=None):
    """printing_press_version from a slug's manifest, at HEAD or at a git ref.

    Reads the nested `_meta` provenance shape too - `connectwise-manage` stores
    its version only there, and reading just the top-level key reports it as
    unstamped.
    """
    path = f"skills/{slug}/manifest.json"
    if ref:
        raw = _git("show", f"{ref}:{path}")
        if not raw:
            return None
    else:
        full = os.path.join(REPO, path)
        if not os.path.exists(full):
            return None
        raw = open(full).read()
    try:
        return _press_version(json.loads(raw))
    except json.JSONDecodeError:
        return None


def _ver(v):
    """Comparable version tuple. Delegates to release_state so the repo has ONE
    version parser, not two that disagree on prereleases."""
    if not v:
        return (-1,)
    return release_state._version_tuple(v)


def _minor(v):
    """(major, minor) - the granularity the engine-gap report acts on."""
    t = _ver(v)
    return t[:2] if len(t) >= 2 else t


def _handfix_count(slug):
    path = os.path.join(REPO, "skills", slug, "handfixes.json")
    if not os.path.exists(path):
        return 0
    try:
        d = json.load(open(path))
    except json.JSONDecodeError:
        return 0
    return len(d.get("handfixes", d) if isinstance(d, dict) else d)


def _reprint_blocked(slug):
    """Known hard blockers that make a reprint impossible today.

    printing-press >=4.30 refuses any spec whose auth token_url carries an
    unresolved {placeholder} (upstream mvanhorn/cli-printing-press#4145), which
    is exactly the legitimate shape of a per-tenant connector. Such a connector
    must be fixed surgically until that lands.
    """
    pattern = os.path.join(REPO, "skills", slug, "cli", "internal", "**", "*.go")
    for f in glob.glob(pattern, recursive=True):
        if "ResolveTemplateURL" in open(f, errors="ignore").read():
            return (
                "templated token_url - press >=4.30 refuses to generate "
                "(upstream #4145); fix surgically"
            )
    return None


def fleet_bar():
    """Newest press version anywhere in the fleet - the self-maintaining bar."""
    best, holder = None, None
    for m in sorted(glob.glob(os.path.join(REPO, "skills", "*", "manifest.json"))):
        slug = os.path.basename(os.path.dirname(m))
        p = _press(slug)
        if p and _ver(p) > _ver(best):
            best, holder = p, slug
    return best, holder


def check(slug, bar, holder):
    tree = _press(slug)
    findings = []

    # 1. Engine vs the fleet. Minor-version granularity on purpose.
    if tree is None:
        findings.append(
            "- **No engine version in `manifest.json`.** Provenance is unknown, so "
            "engine staleness cannot be measured. Stamp it on the next touch."
        )
    elif _minor(tree) < _minor(bar):
        findings.append(
            f"- **Engine is behind the fleet.** This connector was printed on "
            f"**{tree}**; the newest engine in the repo is **{bar}** (`{holder}`). "
            f"Consider a reprint - read [`docs/reprint-survival.md`](docs/reprint-survival.md) "
            f"first, and run `python3 tools/maintainer/check_handfixes.py --brief --slug {slug}`."
        )

    # 2. Shipped binary vs the tree. Content hash, not version stamp: a security
    # fix can change every binary in the fleet without moving any stamp.
    try:
        state = release_state.classify(slug, remote=False)
    except Exception as exc:  # noqa: BLE001 - report, never guess
        findings.append(
            f"- **Could not determine release state** (`release_state.classify` "
            f"raised `{type(exc).__name__}`). Run "
            f"`python3 tools/maintainer/release_state.py` by hand."
        )
        state = {}

    st = state.get("state")
    tag = state.get("latest_tag")
    if st == "binary-pending":
        shipped = _press(slug, tag) if tag else None
        extra = (
            f" The last tag (`{tag}`) was built on engine **{shipped}**; `main` "
            f"carries **{tree}**."
            if shipped and tree and _ver(shipped) < _ver(tree)
            else ""
        )
        findings.append(
            f"- **The shipped binary is older than `main`.** `main` carries CLI "
            f"changes that no released binary contains, so every user is running "
            f"older code until this is released.{extra} A tree-only fix reaches nobody."
        )
    elif st == "never-released":
        findings.append("- **Never released.** No binary exists to install.")
    elif st == "version-pending":
        findings.append(
            f"- **Released but not tagged.** The version bump for `{slug}` landed on "
            f"`main`, but no matching tag exists, so no binaries were ever built or "
            f"published. Push the tag."
        )

    n = _handfix_count(slug)
    blocked = _reprint_blocked(slug)
    notes = []
    if n:
        notes.append(
            f"{n} recorded hand-fix{'es' if n != 1 else ''} must survive any reprint "
            f"(`check_handfixes.py --slug {slug}`)"
        )
    if blocked:
        notes.append(f"reprint currently BLOCKED: {blocked}")

    return findings, notes


def render(slug, findings, notes):
    if not findings:
        return f"`{slug}` is on the current engine and its shipped binary matches `main`. Nothing to do."
    count = "One thing looks" if len(findings) == 1 else f"{len(findings)} things look"
    out = [
        f"### Engine freshness check for `{slug}`",
        "",
        "Now that a real MSP has confirmed this connector against a live tenant, it is "
        "worth keeping current - and someone is finally positioned to notice if it "
        f"breaks. {count} stale:",
        "",
        *findings,
    ]
    if notes:
        out += ["", "Before acting:", "", *[f"- {n}" for n in notes]]
    out += [
        "",
        "This is advisory - nothing is gated on it. Decide reprint vs. release vs. "
        "leave-it, and say which in a reply so the next agent does not re-ask.",
    ]
    return "\n".join(out)


def selftest():
    """Pin the two behaviours that are easy to get subtly wrong."""
    # Version ordering, delegated to release_state - assert the delegation holds.
    assert _ver("4.30.2") > _ver("4.24.0"), "minor dominates patch"
    assert _ver("4.30.10") > _ver("4.30.2"), "numeric compare, not lexical"
    assert _ver("4.24.0") > _ver(None), "absent sorts lowest, so unstamped reads as stale"
    assert not _ver("4.24.0") > _ver("4.24.0"), "equal is not stale"
    # Minor-gap granularity: a patch behind must NOT be reported.
    assert _minor("4.30.1") == _minor("4.30.2"), "patch gap is not an engine gap"
    assert _minor("4.24.0") < _minor("4.30.2"), "minor gap is an engine gap"
    # Slug validation must reject the shapes that reach git and glob as options.
    assert SLUG_RE.match("connectwise-manage")
    assert not SLUG_RE.match("--format=%(refname)"), "git option must be rejected"
    assert not SLUG_RE.match("../etc"), "path traversal must be rejected"
    assert not SLUG_RE.match("-hudu"), "leading dash must be rejected"
    print("selftest ok")


def main():
    ap = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    ap.add_argument("--slug")
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--markdown", action="store_true", help="emit the issue-comment body")
    ap.add_argument("--selftest", action="store_true")
    args = ap.parse_args()

    if args.selftest:
        selftest()
        return 0

    if args.all:
        slugs = sorted(
            os.path.basename(os.path.dirname(m))
            for m in glob.glob(os.path.join(REPO, "skills", "*", "manifest.json"))
        )
    elif args.slug:
        slugs = [args.slug]
    else:
        ap.error("pass --slug <name> or --all")

    for s in slugs:
        if not SLUG_RE.match(s):
            print(f"refusing unsafe slug: {s!r}", file=sys.stderr)
            return ERROR

    bar, holder = fleet_bar()
    if not bar:
        # Every manifest unstamped means the repo state is broken, not clean.
        print("no engine versions found in the fleet - cannot measure", file=sys.stderr)
        return ERROR

    stale = 0
    for s in slugs:
        findings, notes = check(s, bar, holder)
        if findings:
            stale += 1
        if args.markdown:
            print(render(s, findings, notes))
        else:
            print(f"{s}: {'STALE' if findings else 'current'}")
            for f in findings + [f"(note) {n}" for n in notes]:
                print(f"    {f.lstrip('- ')}")
    return STALE if stale else 0


if __name__ == "__main__":
    sys.exit(main())
