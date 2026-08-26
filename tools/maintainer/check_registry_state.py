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
import subprocess
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
REGISTRY = REPO / "tools" / "maintainer" / "skills.json"
REGISTRY_REF = "origin/main:tools/maintainer/skills.json"


def load_current() -> dict:
    try:
        return json.loads(REGISTRY.read_text(encoding="utf-8"))["skills"]
    except (OSError, json.JSONDecodeError, KeyError) as e:
        print(f"check_registry_state: cannot read {REGISTRY}: {e}")
        sys.exit(2)


def load_base() -> tuple[dict | None, str | None]:
    """Return (baseline, error). A non-None error means the trusted baseline is
    UNAVAILABLE and the downgrade comparison cannot be made. Callers must not
    treat that as "no downgrades found"."""
    try:
        p = subprocess.run(
            ["git", "-C", str(REPO), "show", REGISTRY_REF],
            capture_output=True, text=True,
        )
    except (OSError, FileNotFoundError) as e:
        return None, f"could not run git to read {REGISTRY_REF}: {e}"
    if p.returncode != 0:
        detail = (p.stderr or "").strip().splitlines()
        why = detail[0] if detail else "git show returned non-zero"
        return None, f"{REGISTRY_REF} is unreadable ({why})"
    try:
        skills = json.loads(p.stdout)["skills"]
    except json.JSONDecodeError as e:
        return None, f"{REGISTRY_REF} is not valid JSON ({e})"
    except KeyError:
        return None, f"{REGISTRY_REF} has no top-level 'skills' object"
    if not isinstance(skills, dict):
        return None, f"{REGISTRY_REF}: 'skills' is {type(skills).__name__}, expected an object"
    return skills, None


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
    args = ap.parse_args()

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
                    f"(origin/main) to {lv_status(cur_entry)!r} - a stale-snapshot "
                    "merge is destroying a verification receipt. Restore the "
                    "origin/main value; only verify_live.py revoke may clear it."
                )

    if violations:
        print("check_registry_state: FAIL")
        for v in violations:
            print(f"  - {v}")
        return 1
    scope = ("shape only; downgrade check SKIPPED" if base is None
             else "shape + downgrade vs origin/main")
    print(f"check_registry_state: OK ({len(current)} entries; {scope})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
