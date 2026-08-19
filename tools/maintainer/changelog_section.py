#!/usr/bin/env python3
"""changelog_section.py - the release body for one skill version, from its CHANGELOG.

    python3 tools/maintainer/changelog_section.py --slug zammad --version 0.1.1

Why this exists
---------------
`gh release create --generate-notes` computes its range against the previous tag
in the REPO. In a monorepo with per-skill tags, that is whatever unrelated
skill's tag happens to sort before this one, so the generated notes list every
PR merged in between regardless of what it touched.

`zammad-v0.1.1` shipped notes advertising the auvik onboarding, four CI fixes and
a fleet toolchain sweep, footed with `compare/unifi-network-v0.1.0...zammad-v0.1.1`.
Accurate about the commit range; useless and misleading about the connector. An
MSP reading it cannot tell what changed in the thing they installed.

The per-skill `CHANGELOG.md` section is the authoritative human-written answer,
and `release.py` already maintains it. This just extracts it.

It REFUSES a placeholder section (exit 2). A release whose notes say "Describe
the changes in this release." is not better than the generated ones - it is the
same failure with fewer words. Failing here forces the section to be written
before the tag goes out, which is the only point where anyone still remembers
what changed.

Exit codes: 0 section printed. 2 missing or placeholder section. 1 bad usage.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent.parent

# Text release.py stamps into a fresh section BODY. If it survives to release
# time, nobody filled the section in.
#
# Deliberately body-only. An "unreleased" HEADING is a dating problem, not a
# content problem: a section can carry real, hand-written notes under a heading
# nobody dated. Treating that as a placeholder is a false-RED - it refuses to
# build notes that are perfectly good, and (measured) it caused a backfill
# script to overwrite hand-written prose with generated bullets. release.py
# stamps the date at release time; that is where undated headings get fixed.
PLACEHOLDERS = (
    "describe the changes in this release",
)


def section(slug: str, version: str) -> tuple[str | None, str]:
    """Return (body, reason). body is None when unusable."""
    path = REPO / "skills" / slug / "CHANGELOG.md"
    if not path.is_file():
        return None, f"no CHANGELOG.md for {slug}"
    text = path.read_text()

    # Match "## [1.2.3]" through the next "## [" heading (or EOF).
    pattern = re.compile(
        r"^##\s*\[" + re.escape(version) + r"\][^\n]*\n(.*?)(?=^##\s*\[|\Z)",
        re.S | re.M,
    )
    m = pattern.search(text)
    if not m:
        return None, f"no section '## [{version}]' in skills/{slug}/CHANGELOG.md"

    heading = text[m.start(): m.start(1)].strip()
    body = m.group(1).strip()
    if not body:
        return None, f"section '## [{version}]' is empty"

    # Body only - see PLACEHOLDERS. The heading is not evidence about content.
    for ph in PLACEHOLDERS:
        if ph in body.lower():
            return None, (
                f"section '## [{version}]' for {slug} still carries the release.py "
                f"placeholder ({ph!r}). Write what actually changed, and date the "
                f"heading, before tagging."
            )
    return body, "ok"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--slug", required=True)
    ap.add_argument("--version", required=True)
    ap.add_argument("--check", action="store_true",
                    help="report usability without printing the body")
    args = ap.parse_args()

    body, reason = section(args.slug, args.version)
    if body is None:
        print(reason, file=sys.stderr)
        return 2
    print(f"{args.slug} {args.version}: ok" if args.check else body)
    return 0


if __name__ == "__main__":
    sys.exit(main())
