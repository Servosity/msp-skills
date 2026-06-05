#!/usr/bin/env python3
"""Gate: .claude-plugin/marketplace.json is consistent with skills.json.

The marketplace manifest lists one plugin per skill. The registry
(tools/maintainer/skills.json) is the single source of truth for which skills
exist. These two MUST agree on the set of skill slugs, or a skill ships in the
registry/catalog but is invisible in the marketplace (or vice versa).

This gate compares the two slug sets and fails when they diverge:

  - a slug registered in skills.json but missing from marketplace.json
  - a plugin in marketplace.json with no matching slug in skills.json

Each marketplace plugin's "name" is its slug; its "source" must be
"./skills/<slug>" so the manifest points at the skill directory the registry
describes. A name/source mismatch is also a failure.

Pure stdlib. Run locally:  python3 tools/maintainer/check_marketplace_sync.py
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
MARKETPLACE = ROOT / ".claude-plugin" / "marketplace.json"


def main() -> int:
    errors: list[str] = []

    if not MARKETPLACE.exists():
        print(f"check_marketplace_sync FAILED: missing {MARKETPLACE.relative_to(ROOT)}")
        return 1

    try:
        mp = json.loads(MARKETPLACE.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as e:
        print(f"check_marketplace_sync FAILED: cannot read marketplace.json: {e}")
        return 1

    registry_slugs = set(registry.skills())

    plugins = mp.get("plugins", [])
    market_slugs: set[str] = set()
    for plugin in plugins:
        name = plugin.get("name")
        if not name:
            errors.append("a marketplace plugin entry has no 'name'")
            continue
        market_slugs.add(name)
        # The source points at the on-disk directory, which equals the slug for a
        # binary skill but may differ for a markdown-only skill that declares a
        # registry "source_dir" (msp-skills-concierge -> ./skills/_meta).
        expected_source = f"./skills/{registry.source_dir(name)}"
        source = plugin.get("source")
        if source != expected_source:
            errors.append(
                f"plugin '{name}' source is {source!r}, expected {expected_source!r}"
            )

    missing_from_market = sorted(registry_slugs - market_slugs)
    for slug in missing_from_market:
        errors.append(
            f"skill '{slug}' is in skills.json but missing from marketplace.json"
        )

    extra_in_market = sorted(market_slugs - registry_slugs)
    for slug in extra_in_market:
        errors.append(
            f"plugin '{slug}' is in marketplace.json but not in skills.json"
        )

    if errors:
        print("check_marketplace_sync FAILED:")
        for e in errors:
            print(f"  {e}")
        print(
            "\nKeep .claude-plugin/marketplace.json and tools/maintainer/skills.json "
            "in sync: every registered skill needs a matching plugin entry "
            "(name = slug, source = ./skills/<slug>), and no plugin may reference "
            "an unregistered slug."
        )
        return 1

    print(f"PASS: marketplace.json in sync with skills.json ({len(registry_slugs)} skills)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
