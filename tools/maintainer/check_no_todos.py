#!/usr/bin/env python3
"""Gate: no TODO / placeholder markers leak into a skill's shipped prose.

Public skill docs are the storefront. A stray "TODO", "lorem ipsum", or
"PLACEHOLDER" that slips into README.md or guide.md reads as half-finished to an
MSP evaluating the skill. This gate scans every prose file under each skill and
fails the build if a leftover marker is found.

Scanned per slug (only those that exist): SKILL.md, README.md, guide.md,
AGENTS.md, mcp-install.md, governance.md, pain-point.md, CHANGELOG.md,
page.json. NEVER scans cli/, server.json, mcp-descriptions.json, manifest.json
(those carry generated/structural strings that are not storefront prose).

Patterns (failures): the words TODO, FIXME, TBD, XXX as whole tokens; "lorem
ipsum" (case-insensitive); an HTML comment opening a TODO ("<!--" then optional
space then "TODO"); the literal "PLACEHOLDER".

Allowlist: any line containing "no-todos:ignore" is skipped.

Pure stdlib. Run locally:  python3 tools/maintainer/check_no_todos.py
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

SKILLS_DIR = registry.SKILLS_DIR

SCAN_FILES = [
    "SKILL.md",
    "README.md",
    "guide.md",
    "AGENTS.md",
    "mcp-install.md",
    "governance.md",
    "pain-point.md",
    "CHANGELOG.md",
    "page.json",
]

IGNORE_MARKER = "no-todos:ignore"

# (label, compiled pattern). \b word boundaries keep TODO/FIXME/TBD/XXX from
# matching inside longer words; lorem ipsum + the HTML-comment TODO are loose.
PATTERNS = [
    ("TODO", re.compile(r"\bTODO\b")),
    ("FIXME", re.compile(r"\bFIXME\b")),
    ("TBD", re.compile(r"\bTBD\b")),
    ("XXX", re.compile(r"\bXXX\b")),
    ("lorem ipsum", re.compile(r"lorem ipsum", re.IGNORECASE)),
    ("HTML-comment TODO", re.compile(r"<!--\s?TODO")),
    ("PLACEHOLDER", re.compile(r"PLACEHOLDER")),
]


def scan_file(path: Path, violations: list[str]) -> None:
    try:
        text = path.read_text(encoding="utf-8")
    except OSError:
        return
    rel = path.relative_to(registry.ROOT)
    for n, line in enumerate(text.splitlines(), start=1):
        if IGNORE_MARKER in line:
            continue
        for label, pat in PATTERNS:
            m = pat.search(line)
            if m:
                snippet = line.strip()
                if len(snippet) > 120:
                    snippet = snippet[:117] + "..."
                violations.append(f"{rel}:{n}: [{label}] {snippet}")
                break  # one finding per line is enough


def main() -> int:
    violations: list[str] = []
    for slug in sorted(registry.skills()):
        skill_dir = SKILLS_DIR / slug
        for fname in SCAN_FILES:
            fpath = skill_dir / fname
            if fpath.exists():
                scan_file(fpath, violations)

    if violations:
        print("check_no_todos FAILED:")
        for v in violations:
            print(f"  {v}")
        print(
            "\nRemove the marker, or add 'no-todos:ignore' on the line if it is "
            "a legitimate structural reference."
        )
        return 1

    print("PASS: no TODO/placeholder markers")
    return 0


if __name__ == "__main__":
    sys.exit(main())
