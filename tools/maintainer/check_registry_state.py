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
When origin/main is unreachable the gate SKIPs (exit 0 with a notice) -
offline runs are still covered by onboard.py register()'s in-process guard.
"""

from __future__ import annotations

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


def load_base() -> dict | None:
    p = subprocess.run(
        ["git", "-C", str(REPO), "show", REGISTRY_REF],
        capture_output=True, text=True,
    )
    if p.returncode != 0:
        return None
    try:
        return json.loads(p.stdout)["skills"]
    except (json.JSONDecodeError, KeyError):
        return None


def lv_status(entry: dict):
    lv = entry.get("live_verified")
    return lv.get("status") if isinstance(lv, dict) else None


def main() -> int:
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

    base = load_base()
    if base is None:
        print("check_registry_state: origin/main unreachable - downgrade check "
              "SKIPPED (onboard.py register() guard still applies)")
    else:
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
    print(f"check_registry_state: OK ({len(current)} entries; no stateful regressions)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
