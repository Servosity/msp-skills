#!/usr/bin/env python3
"""check_copyright.py - one project copyright line, everywhere.

Why this gate exists: before 2026-08-23 the fleet carried TEN different
copyright strings across 19,307 files - `Damien Stevens and contributors`,
`Abhi Saini and contributors`, and bare usernames like `dstevens`,
`damienstevens`, `servosity`, `phoenix-server`. Nine connectors disagreed with
THEMSELVES (servosity carried three). Whatever a header says, it goes stale the
moment someone else edits the file, and it tells an adopter the wrong thing
about who owns the subtree.

The contract:
  - Every generated Go file, every cli/NOTICE, every cli/LICENSE attribution
    line carries exactly `Copyright 2026 Servosity Inc. and msp-skills
    contributors`, matching the root LICENSE.
  - Contributor credit lives where Apache-2.0 puts it: the NOTICE file's
    "contributed by" line, plus SKILL.md `author` frontmatter and git history.
    Those are checked by check_skill_contract and check_dco, not here.

The press emits the header from whoever ran it, so a reprint reintroduces an
individual's name; this gate is what catches that. Upstream fix tracked as
cli-printing-press#4326.
"""
import pathlib
import re
import sys

CANON = "Copyright 2026 Servosity Inc. and msp-skills contributors"
ROOT = pathlib.Path(__file__).resolve().parents[2]
GO_LINE = re.compile(r'^// Copyright 2026 [^\n]*$', re.M)

def main() -> int:
    failures = []

    for p in sorted(ROOT.glob('skills/*/cli/**/*.go')):
        try:
            text = p.read_text(encoding='utf-8')
        except (OSError, UnicodeDecodeError):
            continue
        for m in GO_LINE.finditer(text):
            line = m.group(0)
            if line != f"// {CANON}. Licensed under Apache-2.0. See LICENSE.":
                rel = p.relative_to(ROOT)
                failures.append(f"{rel}: {line.strip()}")
                break

    for name in ('NOTICE', 'LICENSE'):
        for p in sorted(ROOT.glob(f'skills/*/cli/{name}')):
            try:
                text = p.read_text(encoding='utf-8')
            except (OSError, UnicodeDecodeError):
                continue
            for raw in text.split('\n'):
                stripped = raw.strip()
                # The Apache body says "Grant of Copyright License" etc; only the
                # bare attribution line starts with "Copyright <year>".
                if stripped.startswith('Copyright 2026 ') and stripped != CANON:
                    failures.append(f"{p.relative_to(ROOT)}: {stripped}")

    if failures:
        print(f"FAIL: {len(failures)} file(s) carry a non-canonical copyright line.")
        print(f"  Expected: {CANON}")
        for f in failures[:25]:
            print(f"  {f}")
        if len(failures) > 25:
            print(f"  ... and {len(failures) - 25} more")
        print("\n  Contributor credit belongs in cli/NOTICE ('contributed by ...')")
        print("  and SKILL.md `author`, not in per-file headers. See CONTRIBUTING.md.")
        return 1

    print("PASS: copyright line is uniform across every connector.")
    return 0

if __name__ == '__main__':
    sys.exit(main())
