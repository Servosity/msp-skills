#!/usr/bin/env python3
"""Gate: skills.json stateful fields must not regress vs origin/main.

The failure mode this blocks: an update/fleet branch cut from a stale local
main carries an old skills.json; its whole-file rewrite then downgrades
live_verified from 'live-verified' back to 'awaiting', destroying the
verification receipt. Only verify_live.py revoke - which REMOVES the key -
may take a skill out of live-verified.

Checks per slug, current file vs origin/main:
  1. DOWNGRADE - base is live-verified, current still has the key but with any
     other status. FAIL. (Key removed entirely = explicit revoke = allowed.)
  2. SHAPE - live_verified must be a dict or absent. null / strings FAIL
     (a null entry silently reads as awaiting in every consumer).
  3. BINDING - a live-verified badge records the code it verified
     (verify_live.py writes verified_version, verified_cli_hash and
     verified_basis; --bind sets them on an existing badge). basis "unknown"
     carries null for both; any other basis names X.Y.Z and a hash that must
     EQUAL the cli_hash.py hash of skills/<dir>/cli at tag <slug>-vX.Y.Z,
     computed here from git objects (so a hand-edited binding cannot claim
     code it does not name). A tag missing from the clone fails: fetch tags.
     An awaiting entry may not carry binding fields.

Exit 0 = clean, 1 = violations (printed), 2 = environment error.

The DOWNGRADE check needs a TRUSTED BASELINE (origin/main's skills.json). If
that baseline cannot be read or parsed, the check cannot run - and a check that
could not run is an environment error (exit 2), NOT a pass. Reporting success
there is how a branch that destroyed a live_verified receipt sails through
verify_all.sh: the one comparison that would have caught it never happened.

`--allow-missing-baseline` is the deliberate opt-out for a genuinely offline
checkout with no origin/main. It prints SKIP (never OK), runs the SHAPE check
only, and CI never passes it. Offline runs remain covered by onboard.py
register()'s in-process guard.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
REGISTRY = REPO / "tools" / "maintainer" / "skills.json"
REGISTRY_REF = "origin/main:tools/maintainer/skills.json"
REGISTRY_PATH = "tools/maintainer/skills.json"


def load_current() -> dict:
    """Read skills.json RAW, deliberately not through registry.load().

    Every other reader goes through the loader so the identifier grammar is
    enforced on the way in. This one must not: it exists to report on what the
    working file says versus its origin/main baseline, and a file the loader
    would refuse is precisely a state this comparison has to be able to see and
    describe rather than die on."""
    try:
        return json.loads(REGISTRY.read_text(encoding="utf-8"))["skills"]
    except (OSError, json.JSONDecodeError, KeyError) as e:
        print(f"check_registry_state: cannot read {REGISTRY}: {e}")
        sys.exit(2)


def load_base(ref: str = REGISTRY_REF) -> tuple[dict | None, str | None]:
    """Return (baseline, error). A non-None error means the trusted baseline is
    UNAVAILABLE and the downgrade comparison cannot be made. Callers must not
    treat that as "no downgrades found"."""
    try:
        p = subprocess.run(
            ["git", "-C", str(REPO), "show", ref],
            capture_output=True, text=True,
        )
    except (OSError, FileNotFoundError) as e:
        return None, f"could not run git to read {ref}: {e}"
    if p.returncode != 0:
        detail = (p.stderr or "").strip().splitlines()
        why = detail[0] if detail else "git show returned non-zero"
        return None, f"{ref} is unreadable ({why})"
    try:
        skills = json.loads(p.stdout)["skills"]
    except json.JSONDecodeError as e:
        return None, f"{ref} is not valid JSON ({e})"
    except KeyError:
        return None, f"{ref} has no top-level 'skills' object"
    if not isinstance(skills, dict):
        return None, f"{ref}: 'skills' is {type(skills).__name__}, expected an object"
    return skills, None


BINDING_KEYS = ("verified_version", "verified_cli_hash", "verified_basis")
BASES = ("reported", "latest-release-at-flip", "unknown")
_HASH_RE = re.compile(r"^sha256:[0-9a-f]{64}\Z")
_VERSION_RE = re.compile(r"^\d+\.\d+\.\d+\Z")


def tag_cli_hash(slug: str, entry: dict, version: str) -> str | None:
    """cli_hash.py's hash of the connector's cli/ at tag <slug>-v<version>, from
    git objects. The directory comes from the TAGGED commit's own registry (never
    today's source_dir, which a branch controls), and that commit must record
    the same version in skills.json and manifest.json. None when the tag is not
    in this clone, does not release this connector at that version, or cannot
    be hashed reproducibly. `entry` is unused: kept so callers pass what they
    have and the rule cannot drift back to it."""
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    import stamp_release  # noqa: E402  (local tools/ module; imported lazily: git work)
    tag = f"{slug}-v{version}"
    if subprocess.run(["git", "-C", str(REPO), "rev-parse", "-q", "--verify", f"refs/tags/{tag}"],
                      capture_output=True).returncode != 0:
        return None
    facts = stamp_release._tag_entry(REPO, tag, slug)
    if facts is None or facts[1] != version or facts[2] != version:
        return None
    return stamp_release.tree_hash(REPO, tag, f"skills/{facts[0]}/cli")


def binding_violations(slug: str, entry: dict, tag_hash=tag_cli_hash) -> list[str]:
    lv = entry.get("live_verified")
    if not isinstance(lv, dict):
        return []
    present = [k for k in BINDING_KEYS if k in lv]
    if lv.get("status") != "live-verified":
        return ([f"{slug}: {', '.join(present)} set on a badge that is not live-verified"]
                if present else [])
    basis = lv.get("verified_basis")
    if basis not in BASES:
        return [f"{slug}: live_verified.verified_basis is {basis!r}: a live-verified badge must "
                f"record the code it verified ({', '.join(BASES)}). verify_live.py writes it at a "
                f"flip; for an existing badge run `verify_live.py --bind --slug {slug} "
                "--verified-version X.Y.Z` (or --unknown)."]
    version, h = lv.get("verified_version"), lv.get("verified_cli_hash")
    if basis == "unknown":
        return ([] if version is None and h is None else
                [f"{slug}: verified_basis 'unknown' must carry null verified_version and verified_cli_hash"])
    if not (isinstance(version, str) and _VERSION_RE.match(version)
            and isinstance(h, str) and _HASH_RE.match(h)):
        return [f"{slug}: verified_version {version!r} / verified_cli_hash {h!r} are not X.Y.Z / sha256:<64 hex>"]
    actual = tag_hash(slug, entry, version)
    if actual is None:
        return [f"{slug}: cannot check the badge binding: tag {slug}-v{version} is not in this "
                "clone (git fetch --tags), does not release this connector at that version, or "
                "its cli/ cannot be hashed reproducibly"]
    if actual != h:
        return [f"{slug}: verified_cli_hash {h[:19]}... is not the code at {slug}-v{version} "
                f"({actual[:19]}...): the badge would claim code nobody verified"]
    return []


def _self_test() -> int:
    good = "sha256:" + "a" * 64
    other = "sha256:" + "b" * 64

    def entry(**lv) -> dict:
        return {"live_verified": {"status": "live-verified", "date": "2026-10-01", **lv}}

    def fake(slug, e, version):
        return {"1.0.0": good}.get(version)

    cases = [
        ("bound to the tag's code", entry(verified_version="1.0.0", verified_cli_hash=good,
                                          verified_basis="reported"), True),
        ("bound at a flip", entry(verified_version="1.0.0", verified_cli_hash=good,
                                  verified_basis="latest-release-at-flip"), True),
        ("recorded unknown", entry(verified_version=None, verified_cli_hash=None,
                                   verified_basis="unknown"), True),
        ("awaiting, no binding", {"live_verified": {"status": "awaiting"}}, True),
        ("no live_verified at all", {}, True),
        ("a badge with no binding (the pre-4.4 shape)", entry(), False),
        ("a hash that is not the tag's code", entry(verified_version="1.0.0", verified_cli_hash=other,
                                                    verified_basis="reported"), False),
        ("a version with no tag in the clone", entry(verified_version="2.0.0", verified_cli_hash=good,
                                                     verified_basis="reported"), False),
        ("unknown but carrying a hash", entry(verified_version=None, verified_cli_hash=good,
                                              verified_basis="unknown"), False),
        ("a made-up basis", entry(verified_version="1.0.0", verified_cli_hash=good,
                                  verified_basis="trust-me"), False),
        ("a malformed hash", entry(verified_version="1.0.0", verified_cli_hash="abc",
                                   verified_basis="reported"), False),
        ("binding on an awaiting badge", {"live_verified": {"status": "awaiting",
                                                            "verified_basis": "unknown"}}, False),
    ]
    bad = 0
    for label, e, want_clean in cases:
        clean = not binding_violations("x", e, fake)
        ok = clean is want_clean
        bad += 0 if ok else 1
        print(f"  {'ok  ' if ok else 'FAIL'} {label} -> {'clean' if clean else 'violation'}")
    print("self-test:", "PASS" if not bad else f"FAIL ({bad})")
    return 0 if not bad else 1


def lv_status(entry: dict):
    lv = entry.get("live_verified")
    return lv.get("status") if isinstance(lv, dict) else None


def main() -> int:
    ap = argparse.ArgumentParser(
        description="Gate: skills.json stateful fields must not regress vs origin/main.")
    ap.add_argument(
        "--allow-missing-baseline", action="store_true",
        help="if origin/main's skills.json cannot be read or parsed, print SKIP and run the "
             "SHAPE check only instead of exiting 2. The DOWNGRADE check is NOT performed. "
             "For genuinely offline checkouts; CI must never pass this.")
    ap.add_argument(
        "--base",
        help="the revision to compare against (default origin/main). CI passes the PR base, or "
             "on a push to main github.event.before: by then the runner's origin/main already "
             "CONTAINS the push, so comparing against it would compare the push with itself. A "
             "--base that cannot be read is an environment error (exit 2): substituting "
             "origin/main there would compare a push with itself.")
    ap.add_argument("--self-test", action="store_true", help="prove the BINDING rule both ways")
    args = ap.parse_args()
    if args.self_test:
        return _self_test()

    current = load_current()
    violations: list[str] = []

    for slug, entry in current.items():
        # Key-presence check, not .get(): JSON null is an invalid SHAPE (it
        # silently reads as awaiting in every consumer), absence is the valid
        # post-revoke state.
        if "live_verified" in entry and not isinstance(entry["live_verified"], dict):
            violations.append(
                f"{slug}: live_verified has invalid shape "
                f"{entry['live_verified']!r} - must be a dict or absent "
                "(absence = awaiting)"
            )

    for slug, entry in current.items():
        if isinstance(entry, dict):
            violations += binding_violations(slug, entry)

    base_label = "origin/main"
    if args.base:
        base, base_err = load_base(f"{args.base}:{REGISTRY_PATH}")
        base_label = args.base[:12]
    else:
        base, base_err = load_base()
    if base_err is not None:
        if not args.allow_missing_baseline:
            print("check_registry_state: ENVIRONMENT ERROR - the trusted baseline could not "
                  "be loaded, so the DOWNGRADE check did not run.")
            print(f"  reason: {base_err}")
            print("  This is NOT a pass: without the baseline this gate cannot see a branch "
                  "that downgraded a live_verified receipt.")
            print("  Fix: `git fetch origin main` (CI checks out with the remote present). "
                  "For a deliberate offline run, pass --allow-missing-baseline, which SKIPS "
                  "the downgrade check and verifies only the shape.")
            return 2
        print(f"check_registry_state: SKIP (not a pass) - {base_err}; DOWNGRADE check did NOT "
              "run (--allow-missing-baseline). Shape check only; onboard.py register() guard "
              "still applies.")
    if base is not None:
        for slug, base_entry in base.items():
            if lv_status(base_entry) != "live-verified":
                continue
            cur_entry = current.get(slug)
            if cur_entry is None:
                continue  # slug removal is its own review concern, not this gate's
            if "live_verified" not in cur_entry:
                continue  # key removed = explicit verify_live.py revoke
            if lv_status(cur_entry) != "live-verified":
                violations.append(
                    f"{slug}: live_verified DOWNGRADED from 'live-verified' "
                    f"({base_label}) to {lv_status(cur_entry)!r} - a stale-snapshot "
                    "merge is destroying a verification receipt. Restore the "
                    "origin/main value; only verify_live.py revoke may clear it."
                )

    if violations:
        print("check_registry_state: FAIL")
        for v in violations:
            print(f"  - {v}")
        return 1
    scope = ("shape + binding only; downgrade check SKIPPED" if base is None
             else f"shape + binding + downgrade vs {base_label}")
    print(f"check_registry_state: OK ({len(current)} entries; {scope})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
