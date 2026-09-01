#!/usr/bin/env python3
"""Assert a release carries the COMPLETE asset set its tag promises.

This is the gate that stands between a draft release and the irreversible act
of publishing it. The repository has immutable releases enabled: a PUBLISHED
release is sealed the instant it exists, and no asset can ever be added to it.
So the release pipeline assembles into a DRAFT, proves the set is complete
here, and only then flips the draft to published. Nothing unverified becomes
public, and nothing public is ever incomplete.

Where the expected names come from
----------------------------------
The SAME shared source the build matrix is driven from:
release_matrix.skill_entries() -> registry.asset_map(). Every literal filename
below is read out of that structure. This script does NOT rebuild the
"-<goos>-<goarch>" suffix or the windows ".exe" rule - re-deriving those in a
second place is exactly how a workflow and its gate drift apart while both stay
green (tools/maintainer/check_release_contract.py applies the same rule to the
install scripts).

Per skill, per target, release.yml uploads four files:
    <cli-asset>  <mcp-asset>  <cli-asset>.sha256  <mcp-asset>.sha256
Across the six targets in registry.TARGETS that is 24 files. With --with-mcpb
the single cross-platform bundle "<mcp-binary>.mcpb" is required too, for 25.

Usage
-----
    gh release view "$TAG" --json assets -q '.assets[].name' \\
      | python3 tools/maintainer/check_release_assets.py --tag "$TAG" --with-mcpb

Actual asset names arrive on stdin, one per line, so this script needs no
network and no `gh`: it is a pure set comparison you can run locally against a
saved listing. Exit 0 when every expected name is present, 1 otherwise (with
the missing names printed). Extra assets are reported but never fatal - a
re-run that uploads the same files again must converge, not fail.

    python3 tools/maintainer/check_release_assets.py --self-test

proves the gate both fires and stays silent; CI runs it alongside the other
maintainer gate self-tests.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/maintainer module)
import release_matrix  # noqa: E402  (local tools/maintainer module)


def expected_assets(tag: str, with_mcpb: bool) -> tuple[str, list[str]]:
    """(slug, sorted expected asset names) for a per-skill release tag.

    Returns an empty list when the tag names no releasable skill - the same
    condition release_matrix.py turns into an empty build matrix, which
    release.yml's `any` guard turns into "no release is created at all".
    """
    slug = tag.rsplit("-v", 1)[0]
    entries = [e for e in release_matrix.skill_entries() if e["name"] == slug]
    names: set[str] = set()
    for entry in entries:
        for per_target in entry["assets"].values():
            for asset in (per_target["cli"], per_target["mcp"]):
                names.add(asset)
                # release.yml writes a sidecar next to every binary it uploads.
                names.add(f"{asset}.sha256")
        if with_mcpb:
            # Named from the mcp binary registry.py already holds, so this
            # cannot drift from what tools/maintainer/mcpb_bundle.py writes.
            names.add(f"{entry['mcp_bin']}.mcpb")
    return slug, sorted(names)


def compare(expected: list[str], actual: set[str]) -> tuple[list[str], list[str]]:
    """(missing, extra). Missing is fatal; extra is informational."""
    return [n for n in expected if n not in actual], sorted(actual - set(expected))


# --------------------------------------------------------------------------
# Self-test: prove the gate fires on the defect it was written for AND stays
# silent on a healthy release. A gate that cannot fail is worthless; a gate
# that always fires is worse, because it teaches people to ignore it.
#
# Every case is built from the repo's OWN registry, so the proof tracks the
# real asset-naming rule instead of a copy of it, and needs no network.
# --------------------------------------------------------------------------
def _self_test() -> int:
    entries = release_matrix.skill_entries()
    if not entries:
        print("self-test: no releasable skills in the registry", file=sys.stderr)
        return 1
    entry = entries[0]
    slug = entry["name"]
    tag = f"{slug}-v9.9.9"
    _, full = expected_assets(tag, with_mcpb=True)
    _, no_bundle = expected_assets(tag, with_mcpb=False)
    failures: list[str] = []

    def case(label: str, actual: set[str], expect_missing, with_mcpb: bool = True) -> None:
        missing, _extra = compare(full if with_mcpb else no_bundle, actual)
        got, want = sorted(missing), sorted(expect_missing)
        ok = got == want
        print(f"  [{'ok' if ok else 'FAIL'}] {label}")
        if not ok:
            failures.append(f"{label}: expected missing {want}, got {got}")

    want_count = len(registry.TARGETS) * 4 + 1
    if len(full) != want_count:
        failures.append(f"expected {want_count} names for {tag}, got {len(full)}")

    # SILENT on a healthy release.
    case("a complete release passes", set(full), [])
    # SILENT when a re-run leaves an extra asset behind: re-runs must converge.
    case("an extra asset is tolerated", set(full) | {"SBOM.spdx.json"}, [])
    # FIRES on the observed production failure: immutable releases rejected
    # every upload and the release published with zero assets.
    case("an empty release fires on every expected name", set(), full)
    # FIRES when one of the six build targets failed, naming exactly its four
    # files and nothing else.
    first = entry["assets"][registry.target_key(registry.TARGETS[0]["goos"],
                                                registry.TARGETS[0]["goarch"])]
    dropped = {first["cli"], first["mcp"],
               first["cli"] + ".sha256", first["mcp"] + ".sha256"}
    case("one failed build target fires on its four files",
         set(full) - dropped, dropped)
    # FIRES when the bundle never attached - and the SAME input passes without
    # --with-mcpb, which proves the flag discriminates rather than adds noise.
    bundle = f"{entry['mcp_bin']}.mcpb"
    case("a missing .mcpb fires under --with-mcpb", set(full) - {bundle}, [bundle])
    case("the same input passes without --with-mcpb", set(full) - {bundle}, [],
         with_mcpb=False)
    # SILENT for a tag that names no releasable skill: release.yml creates no
    # release for one, so this must never false-RED it.
    _, none = expected_assets("not-a-real-skill-v1.2.3", with_mcpb=True)
    ok = none == []
    print(f"  [{'ok' if ok else 'FAIL'}] a tag naming no releasable skill expects nothing")
    if not ok:
        failures.append(f"unreleasable tag expected [], got {none}")

    if failures:
        print("check_release_assets self-test FAILED:", file=sys.stderr)
        for f in failures:
            print(f"  - {f}", file=sys.stderr)
        return 1
    print(f"check_release_assets self-test passed "
          f"(slug={slug}, {len(full)} expected assets).")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    ap.add_argument("--tag", help="the release tag, e.g. hudu-v0.1.8")
    ap.add_argument("--with-mcpb", action="store_true",
                    help="also require the <mcp-binary>.mcpb bundle "
                         "(required before a release may be published)")
    ap.add_argument("--print-expected", action="store_true",
                    help="print the expected names and exit, reading no stdin")
    ap.add_argument("--self-test", action="store_true",
                    help="prove this gate fires and stays silent; reads no stdin")
    args = ap.parse_args()

    if args.self_test:
        return _self_test()
    if not args.tag:
        ap.error("--tag is required (or use --self-test)")

    slug, expected = expected_assets(args.tag, args.with_mcpb)

    if not expected:
        # Not a releasable skill. release.yml creates no release for such a
        # tag, so there is nothing to verify and nothing to seal.
        print(f"tag {args.tag!r} names no releasable skill "
              f"(derived slug {slug!r}); nothing to verify.")
        return 0

    if args.print_expected:
        print("\n".join(expected))
        return 0

    missing, extra = compare(expected, {ln.strip() for ln in sys.stdin if ln.strip()})

    if missing:
        print(f"::error::release {args.tag} is missing "
              f"{len(missing)} of {len(expected)} expected assets:",
              file=sys.stderr)
        for name in missing:
            print(f"::error::  missing: {name}", file=sys.stderr)
        return 1

    print(f"{args.tag}: all {len(expected)} expected assets are attached "
          f"({len(registry.TARGETS)} targets x 4 files"
          f"{' + the .mcpb bundle' if args.with_mcpb else ''}).")
    if extra:
        print(f"note: {len(extra)} additional asset(s) attached: {', '.join(extra)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
