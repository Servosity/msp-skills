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
     REPORTS, it never decides.

  2. Is the SHIPPED BINARY behind the TREE?  This is the one that actually
     bites. `main` can carry an engine upgrade for months while every user runs
     the binary from the last tag. Measured 2026-08-16: 41 of 61 connectors were
     shipping binaries built before the 4.24 engine upgrade that had been
     sitting in main since June. A tree-only upgrade reaches nobody.

There is no hardcoded "current press version" to keep updated. The bar is the
newest press version anywhere in the fleet, so it rises on its own every time a
connector is minted or reprinted.

Exit codes: 0 = current, or reported-only. 2 = stale (advisory; nothing in CI
gates on this - it is a prompt for a human decision, not a verdict).
"""

import argparse
import glob
import json
import os
import re
import subprocess
import sys

REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))


def _git(*args):
    return subprocess.run(
        ["git", "-C", REPO, *args], capture_output=True, text=True
    ).stdout.strip()


def _press(slug, ref=None):
    """printing_press_version from a slug's manifest, at HEAD or at a git ref."""
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
        return json.loads(raw).get("printing_press_version")
    except json.JSONDecodeError:
        return None


def _ver(v):
    """'4.30.2' -> (4, 30, 2); unparseable/absent sorts lowest."""
    if not v:
        return (-1,)
    return tuple(int(p) for p in re.findall(r"\d+", v)) or (-1,)


def _latest_tag(slug):
    return _git("tag", "--list", f"{slug}-v*", "--sort=-v:refname").splitlines()[:1]


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
    for f in glob.glob(os.path.join(REPO, "skills", slug, "cli", "internal", "**", "*.go"), recursive=True):
        if "ResolveTemplateURL" in open(f, errors="ignore").read():
            return "templated token_url - press >=4.30 refuses to generate (upstream #4145); fix surgically"
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
    tags = _latest_tag(slug)
    tag = tags[0] if tags else None
    shipped = _press(slug, tag) if tag else None
    findings = []

    if tree is None:
        findings.append(
            "- **No `printing_press_version` in `manifest.json`.** Engine provenance is "
            "unknown, so staleness cannot be measured. Stamp it on the next touch."
        )
    elif _ver(tree) < _ver(bar):
        findings.append(
            f"- **Engine is behind the fleet.** This connector was printed on "
            f"**{tree}**; the newest engine in the repo is **{bar}** (`{holder}`). "
            f"Consider a reprint - see `docs/reprint-survival.md` first, and run "
            f"`python3 tools/maintainer/check_handfixes.py --brief --slug {slug}`."
        )

    if tag and shipped is not None and tree is not None and _ver(shipped) < _ver(tree):
        findings.append(
            f"- **The shipped binary is older than `main`.** The newest tag "
            f"(`{tag}`) was built on engine **{shipped}**, but `main` carries "
            f"**{tree}**. Every user is running the older engine until this is "
            f"released. A tree-only upgrade reaches nobody."
        )
    elif tag and shipped is None and tree is not None:
        findings.append(
            f"- **The shipped binary predates engine stamping.** The newest tag "
            f"(`{tag}`) records no engine version, so it is older than `main`'s "
            f"**{tree}**. Release to ship the current engine."
        )
    elif not tag:
        findings.append("- **Never released.** No tag exists, so there is no binary to install.")

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
    out = [
        f"### Engine freshness check for `{slug}`",
        "",
        "Now that a real MSP has confirmed this connector against a live tenant, it is "
        "worth keeping current - and someone is finally positioned to notice if it "
        "breaks. Two things look stale:",
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
    """The only non-obvious logic here is version ordering. Pin it."""
    assert _ver("4.30.2") > _ver("4.24.0"), "minor version must dominate patch"
    assert _ver("4.30.10") > _ver("4.30.2"), "numeric compare, not lexical"
    assert _ver("4.24.0") > _ver(None), "absent sorts lowest, so unstamped reads as stale"
    assert _ver(None) == _ver("no digits here"), "unparseable is treated as absent"
    assert not _ver("4.24.0") > _ver("4.24.0"), "equal is not stale"
    print("selftest ok")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--slug")
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--markdown", action="store_true", help="emit the issue-comment body")
    ap.add_argument("--selftest", action="store_true")
    args = ap.parse_args()

    if args.selftest:
        selftest()
        return 0

    bar, holder = fleet_bar()
    if not bar:
        print("no engine versions found in the fleet", file=sys.stderr)
        return 0

    slugs = (
        sorted(os.path.basename(os.path.dirname(m)) for m in glob.glob(os.path.join(REPO, "skills", "*", "manifest.json")))
        if args.all
        else [args.slug]
    )
    if not slugs or slugs == [None]:
        ap.error("pass --slug <name> or --all")

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
    return 2 if stale else 0


if __name__ == "__main__":
    sys.exit(main())
