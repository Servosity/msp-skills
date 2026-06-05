#!/usr/bin/env python3
"""Gate: every connector is represented on every skill-enumerating surface.

The repo now generates each homepage / docs / README surface that lists skills
from tools/skills.json (build-catalog.py). This gate is the receipt that the
generation actually reached every surface for every connector - so a new skill
can never again ship to the registry while a hand-or-Jekyll-rendered surface
still lists only the old set.

It exists because of the 2026-06-05 staleness bug: connectwise-manage + hubspot
shipped (registry + releases), but the homepage "What's in the box" box table
and the docs site still listed only halopsa + servosity, because those surfaces
were not regenerated. This gate would have caught it - it asserts, for every
non-markdown-only connector, that:

  a. docs/_data/catalog.json (the Jekyll homepage data file) has a connectors[]
     entry for the slug - this is what the site table renders from.
  b. The README <!-- agent-can-do --> block has at least one outcome row whose
     Skill column is exactly the slug.
  c. The README <!-- install-featured --> block names the skill (display_name)
     OR carries the generic "every connector in the table above" swap line - so
     a visitor can find an install path for every connector.
  d. The README <!-- hero-live --> block states the correct connector count
     (the number followed by " connectors").

It also asserts all five generated marker pairs exist in the README (start and
end, start before end), since every other check reads from inside them.

The markdown-only concierge is skipped (no binary surface, no catalog connector
entry, no outcomes table row).

Pure stdlib. Run locally:
    python3 tools/maintainer/check_surface_coverage.py
Optionally point it at an alternate README (used by the negative self-test):
    python3 tools/maintainer/check_surface_coverage.py --readme /tmp/README.bad
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
DOCS_CATALOG = ROOT / "docs" / "_data" / "catalog.json"

# The five generated marker pairs that build-catalog.py rewrites. Every surface
# this gate inspects lives between one of these pairs, so their presence (and
# ordering) is the precondition for the rest of the checks.
MARKERS = [
    "catalog",
    "hero-live",
    "install-featured",
    "agent-can-do",
    "footer-releases",
]


def block_between(text: str, marker: str) -> str | None:
    """Return the text between <!-- marker:start --> and <!-- marker:end -->,
    or None if the pair is missing or out of order."""
    start = text.find(f"<!-- {marker}:start -->")
    end = text.find(f"<!-- {marker}:end -->")
    if start == -1 or end == -1 or start >= end:
        return None
    return text[start:end]


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--readme",
        default=str(ROOT / "README.md"),
        help="path to the README to check (defaults to repo README.md)",
    )
    args = parser.parse_args()
    readme_path = Path(args.readme)

    errors: list[str] = []

    # --- README marker pairs exist and are ordered (precondition). ---
    if not readme_path.exists():
        print(f"check_surface_coverage FAILED:\n  - README not found: {readme_path}")
        return 1
    readme = readme_path.read_text(encoding="utf-8")

    blocks: dict[str, str] = {}
    for marker in MARKERS:
        block = block_between(readme, marker)
        if block is None:
            errors.append(
                f"README ({readme_path}): the <!-- {marker}:start --> / "
                f"<!-- {marker}:end --> marker pair is missing or out of order"
            )
        else:
            blocks[marker] = block

    # --- docs/_data/catalog.json exists and is the Jekyll connector source. ---
    docs_connector_slugs: set[str] = set()
    if not DOCS_CATALOG.exists():
        errors.append(
            f"docs/_data/catalog.json missing ({DOCS_CATALOG}) - the Jekyll "
            "homepage table renders from it; run build-catalog.py"
        )
    else:
        try:
            data = json.loads(DOCS_CATALOG.read_text(encoding="utf-8"))
            docs_connector_slugs = {
                c.get("slug") for c in data.get("connectors", []) if c.get("slug")
            }
        except (OSError, json.JSONDecodeError) as exc:
            errors.append(f"docs/_data/catalog.json is not valid JSON: {exc}")

    skills = registry.skills()
    connectors = [
        (slug, meta)
        for slug, meta in skills.items()
        if not meta.get("markdown_only")
    ]

    hero_block = blocks.get("hero-live")
    agent_block = blocks.get("agent-can-do")
    install_block = blocks.get("install-featured")

    # --- (d) hero-live states the correct connector count. ---
    if hero_block is not None:
        n = len(connectors)
        if not re.search(rf"\b{n}\s+connectors\b", hero_block):
            errors.append(
                f"hero-live block: expected the text \"{n} connectors\" (the live "
                "connector count) but did not find it - regenerate via "
                "build-catalog.py (surface: README <!-- hero-live -->)"
            )

    # --- Per-connector coverage on each surface. ---
    for slug, meta in connectors:
        display_name = meta.get("display_name") or meta.get("vendor", slug)

        # (a) docs/_data/catalog.json connector entry.
        if DOCS_CATALOG.exists() and slug not in docs_connector_slugs:
            errors.append(
                f"{slug}: no connectors[] entry in docs/_data/catalog.json "
                f"(file: {DOCS_CATALOG.relative_to(ROOT)}) - the Jekyll homepage "
                "table would not list it; run build-catalog.py"
            )

        # (b) agent-can-do has a row whose Skill column is exactly the slug.
        if agent_block is not None:
            row_re = re.compile(
                rf"^\|[^|]*\|\s*{re.escape(slug)}\s*\|", re.MULTILINE
            )
            if not row_re.search(agent_block):
                errors.append(
                    f"{slug}: no outcome row in the README <!-- agent-can-do --> "
                    "block (surface: README outcomes table) - every connector needs "
                    "at least one row; run build-catalog.py"
                )

        # (c) install-featured names the skill OR has the generic swap line.
        if install_block is not None:
            generic = "every connector in the table above" in install_block
            named = display_name in install_block
            if not (generic or named):
                errors.append(
                    f"{slug}: the README <!-- install-featured --> block neither "
                    f"names \"{display_name}\" nor carries the generic "
                    "\"every connector in the table above\" swap line "
                    "(surface: README install prompts) - a visitor has no install "
                    "path for this connector"
                )

    if errors:
        print("check_surface_coverage FAILED:")
        for e in errors:
            print(f"  - {e}")
        print(
            "\nFix: run 'python3 tools/maintainer/build-catalog.py' and commit the "
            "regenerated docs/_data/catalog.json + README generated blocks."
        )
        return 1

    print(
        f"PASS: surface coverage for {len(connectors)} connector(s) across "
        "docs/_data/catalog.json + README hero-live/install-featured/agent-can-do."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
