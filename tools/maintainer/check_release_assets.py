#!/usr/bin/env python3
"""Assert a release carries the COMPLETE, fully-uploaded asset set its tag promises.

This is the gate that stands between a draft release and the irreversible act
of publishing it. The repository has immutable releases enabled: a PUBLISHED
release is sealed the instant it exists, and no asset can ever be added to or
replaced in it. So the release pipeline assembles into a DRAFT, proves the set
is complete here, and only then flips the draft to published. Nothing unverified
becomes public, and nothing public is ever incomplete.

What "complete" means
---------------------
Three things, because a name alone does not mean a usable file:

  1. Every expected name is attached.
  2. Every one of them has `state == "uploaded"`. GitHub creates the asset
     record BEFORE the bytes finish transferring; an upload that dies partway
     leaves it in `starter` state. Sealing over one of those would publish a
     permanently broken download that a name-only check waves through.
  3. Every one of them has a `size` that is a real byte count: an int (never a
     bool - `isinstance(True, int)` is True in Python), strictly greater than
     zero. Truthiness is not the question. `"0"`, `"1024"`, `true` and `NaN` are
     all truthy and none of them is a size, so each is refused by TYPE.

Where the expected names come from
----------------------------------
The SAME shared source the build matrix is driven from:
release_matrix.skill_entries() -> registry.asset_map() for the per-target
binaries, and release_matrix's `mcpb_asset` for the bundle. Every literal
filename below is read out of that structure. This script does NOT rebuild the
"-<goos>-<goarch>" suffix, the windows ".exe" rule, or the "<mcp-binary>.mcpb"
bundle name - re-deriving those in a second place is exactly how a workflow and
its gate drift apart while both stay green (tools/maintainer/check_release_contract.py
applies the same rule to the install scripts).

Per skill, per target, release.yml uploads four files:
    <cli-asset>  <mcp-asset>  <cli-asset>.sha256  <mcp-asset>.sha256
Across the six targets in registry.TARGETS that is 24 files. With --with-mcpb
the single cross-platform bundle is required too, for 25.

Usage
-----
    gh release view "$TAG" --json assets \\
      | python3 tools/maintainer/check_release_assets.py --tag "$TAG" --with-mcpb

The release's own asset list arrives on stdin as the JSON `gh release view
--json assets` prints, so this script needs no network and no `gh` of its own:
it is a pure comparison you can run locally against a saved listing. Exit 0 when
every expected asset is present and fully uploaded, 1 otherwise (with the
offending names printed). Extra assets are reported but never fatal - a re-run
that uploads the same files again must converge, not fail.

    python3 tools/maintainer/check_release_assets.py --self-test

proves the gate both fires and stays silent; CI runs it alongside the other
maintainer gate self-tests.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/maintainer module)
import release_matrix  # noqa: E402  (local tools/maintainer module)

# GitHub's asset lifecycle: the record exists from the moment an upload starts.
# Only this state means the bytes are all there.
UPLOADED = "uploaded"


def real_size(value: object) -> bool:
    """True only for a genuine byte count: an int, not a bool, strictly > 0.

    `if not size` was the whole check here once, and truthiness is the wrong
    question to ask of a field that arrives from JSON. Every one of these is
    truthy and none of them describes a usable asset:

        "0"      a string, so truthy no matter what it says
        "1024"   ditto: a size-shaped string is not a size
        -1       negative
        True     `isinstance(True, int)` is True in Python, so a bool sails
                 straight through an int check unless it is excluded FIRST
        NaN      a float, and `not float("nan")` is False

    A half-uploaded asset is exactly what this field was added to catch, so it
    is checked as a TYPE and a value, not as a truth value. A float is refused
    even when it is positive: GitHub sends an integer, and anything else means
    the listing is not the one this gate was told to read.
    """
    return isinstance(value, int) and not isinstance(value, bool) and value > 0


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
            # release_matrix owns this name, so the workflow that attaches the
            # bundle and the gate that requires it read one definition.
            names.add(entry["mcpb_asset"])
    return slug, sorted(names)


def parse_assets(payload: str) -> dict[str, dict]:
    """{name: asset} from `gh release view --json assets` output.

    Accepts either the object gh prints ({"assets": [...]}) or a bare list, so
    a saved `-q .assets` listing works too.
    """
    data = json.loads(payload) if payload.strip() else {"assets": []}
    rows = data.get("assets", []) if isinstance(data, dict) else data
    return {row["name"]: row for row in rows if isinstance(row, dict) and row.get("name")}


def compare(expected: list[str], actual: dict[str, dict]) -> tuple[list[str], list[str], list[str]]:
    """(missing, incomplete, extra).

    `missing` and `incomplete` are both fatal - an asset whose bytes never
    finished transferring is a broken download, not a present one. `extra` is
    informational so a re-run that leaves something behind still converges.
    """
    missing, incomplete = [], []
    for name in expected:
        row = actual.get(name)
        if row is None:
            missing.append(name)
            continue
        # Absent fields are NOT treated as healthy. Defaulting them to
        # "uploaded"/non-zero would silently degrade this back to a name-only
        # check the moment the input is not what we asked for, which is the
        # exact false-GREEN this function exists to remove.
        if "state" not in row or "size" not in row:
            incomplete.append(
                f"{name} (listing carries no state/size; expected the JSON from "
                f"`gh release view <tag> --json assets`)")
            continue
        state, size = row["state"], row["size"]
        if state != UPLOADED:
            incomplete.append(f"{name} (state={state!r}, not {UPLOADED!r})")
        elif not real_size(size):
            incomplete.append(
                f"{name} (size={size!r} is not a positive integer byte count)")
    return missing, incomplete, sorted(set(actual) - set(expected))


# --------------------------------------------------------------------------
# Self-test: prove the gate fires on the defects it was written for AND stays
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

    def healthy(names) -> dict[str, dict]:
        """The gh shape for a set of fully-uploaded assets."""
        return {n: {"name": n, "state": UPLOADED, "size": 1024} for n in names}

    def case(label, actual, want_missing=(), want_incomplete=(), with_mcpb=True):
        missing, incomplete, _extra = compare(full if with_mcpb else no_bundle, actual)
        ok = (sorted(missing) == sorted(want_missing)
              and len(incomplete) == len(want_incomplete)
              and all(any(w in got for got in incomplete) for w in want_incomplete))
        print(f"  [{'ok' if ok else 'FAIL'}] {label}")
        if not ok:
            failures.append(f"{label}: missing={sorted(missing)} incomplete={incomplete}")

    want_count = len(registry.TARGETS) * 4 + 1
    if len(full) != want_count:
        failures.append(f"expected {want_count} names for {tag}, got {len(full)}")

    # SILENT on a healthy release.
    case("a complete release passes", healthy(full))
    # SILENT when a re-run leaves an extra asset behind: re-runs must converge.
    case("an extra asset is tolerated", healthy(list(full) + ["SBOM.spdx.json"]))
    # FIRES on the observed production failure: immutable releases rejected
    # every upload and the release published with zero assets.
    case("an empty release fires on every expected name", {}, want_missing=full)
    # FIRES when one of the six build targets failed, naming exactly its four
    # files and nothing else.
    first = entry["assets"][registry.target_key(registry.TARGETS[0]["goos"],
                                                registry.TARGETS[0]["goarch"])]
    dropped = {first["cli"], first["mcp"],
               first["cli"] + ".sha256", first["mcp"] + ".sha256"}
    case("one failed build target fires on its four files",
         healthy(set(full) - dropped), want_missing=dropped)
    # FIRES when the bundle never attached - and the SAME input passes without
    # --with-mcpb, which proves the flag discriminates rather than adds noise.
    bundle = entry["mcpb_asset"]
    case("a missing .mcpb fires under --with-mcpb",
         healthy(set(full) - {bundle}), want_missing=[bundle])
    case("the same input passes without --with-mcpb",
         healthy(set(full) - {bundle}), with_mcpb=False)
    # FIRES when an upload died partway and left the record behind. A name-only
    # check would seal this and publish a permanently broken download.
    half = healthy(full)
    half[bundle] = {"name": bundle, "state": "starter", "size": 0}
    case("an asset stuck in 'starter' state fires", half, want_incomplete=["starter"])
    # FIRES on a zero-byte asset that GitHub nonetheless calls uploaded.
    empty = healthy(full)
    empty[bundle] = {"name": bundle, "state": UPLOADED, "size": 0}
    case("a zero-byte asset fires", empty, want_incomplete=["size=0"])

    # FIRES on every shape a truthiness check waved through. "0" and "1024" are
    # strings, so truthy whatever they say; True is an int subclass; NaN is a
    # float that is not falsy; a float is not a byte count; null is not a size.
    # Each of these PASSED while `if not size` was the check.
    def sized(value) -> dict[str, dict]:
        row = healthy(full)
        row[bundle] = {"name": bundle, "state": UPLOADED, "size": value}
        return row

    for label, bad in [('a string "0"', "0"),
                       ('a size-shaped string "1024"', "1024"),
                       ("a negative -1", -1),
                       ("a boolean true", True),
                       ("a NaN", float("nan")),
                       ("a float 1024.0", 1024.0),
                       ("a null", None),
                       ("a list", [1024])]:
        case(f"{label} fires as not a byte count", sized(bad),
             want_incomplete=["is not a positive integer"])
    # SILENT on the smallest legitimate size, so the type check discriminates
    # rather than simply rejecting everything unusual.
    case("a one-byte asset still passes", sized(1))

    # The bad shapes above are REACHABLE from real input: `gh` emits JSON, and
    # json.loads turns `true` into a bool and `NaN` into a float by default. Walk
    # them through parse_assets so the proof covers the actual entry point.
    bad_json = json.dumps({"assets": [
        {"name": n, "state": UPLOADED, "size": (True if n == bundle else 1024)}
        for n in full]})
    assert '"size": true' in bad_json, bad_json[:120]
    case("a boolean size survives JSON parsing and still fires",
         parse_assets(bad_json), want_incomplete=["is not a positive integer"])
    # FIRES rather than silently degrading to a name-only check when the input
    # is a bare name listing with no state/size to inspect.
    bare = {n: {"name": n} for n in full}
    case("a listing with no state/size fires on every asset", bare,
         want_incomplete=["carries no state/size"] * len(full))
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
                    help="also require the MCPB bundle "
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

    try:
        actual = parse_assets(sys.stdin.read())
    except (ValueError, TypeError, KeyError) as exc:
        # Fail CLOSED and say why. An unreadable listing must never be mistaken
        # for a complete one - that is the whole point of this gate.
        print(f"::error::could not read the asset listing for {args.tag} ({exc}).",
              file=sys.stderr)
        print("::error::Expected the JSON from `gh release view <tag> --json assets`.",
              file=sys.stderr)
        return 1

    missing, incomplete, extra = compare(expected, actual)

    if missing or incomplete:
        print(f"::error::release {args.tag} is not publishable: "
              f"{len(missing)} missing and {len(incomplete)} incomplete "
              f"of {len(expected)} expected assets:", file=sys.stderr)
        for name in missing:
            print(f"::error::  missing: {name}", file=sys.stderr)
        for name in incomplete:
            print(f"::error::  incomplete: {name}", file=sys.stderr)
        return 1

    print(f"{args.tag}: all {len(expected)} expected assets are attached and "
          f"fully uploaded ({len(registry.TARGETS)} targets x 4 files"
          f"{' + the .mcpb bundle' if args.with_mcpb else ''}).")
    if extra:
        print(f"note: {len(extra)} additional asset(s) attached: {', '.join(extra)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
