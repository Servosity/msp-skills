#!/usr/bin/env python3
"""is_registered_slug.py - exit 0 if the slug is a registered skill, else 1.

    python3 tools/maintainer/is_registered_slug.py --slug halopsa

Why this exists as a script rather than a `python3 -c` one-liner
----------------------------------------------------------------
Both release.yml and mcp-publish.yml used to interpolate a TAG-DERIVED shell
variable straight into a Python program text:

    python3 -c "... sys.exit(0 if '$slug' in reg.get('skills',{}) else 1)"

`$slug` comes from `${TAG%-v*}` where TAG is `github.ref_name`, so anyone able
to push a tag controlled the contents of a Python string literal. A single quote
in the slug closes the literal and the rest is executed:

    git tag "'or(__import__(\\"os\\").popen(\\"id\\").read())or'-v1.0.0"

That runs arbitrary code as the workflow token, which holds `contents: write`.
Confirmed exploitable before this change.

Passing the value as **argv** removes the class entirely: the slug is data to a
fixed program, never program text. The `*-v*` tag ruleset restricted this to
admins, so it was gated rather than open - but a ruleset is configuration, and
configuration changes. This does not depend on it.

The charset check is defence in depth, not the fix: the fix is argv.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent.parent
REGISTRY = REPO / "tools" / "maintainer" / "skills.json"
SLUG_RE = re.compile(r"^[a-z0-9][a-z0-9-]*$")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--slug", required=True)
    args = ap.parse_args()

    if not SLUG_RE.match(args.slug):
        print(f"slug {args.slug!r} is not a valid skill slug", file=sys.stderr)
        return 1
    try:
        reg = json.loads(REGISTRY.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        print(f"cannot read {REGISTRY}: {exc}", file=sys.stderr)
        return 1
    return 0 if args.slug in (reg.get("skills") or {}) else 1


if __name__ == "__main__":
    sys.exit(main())
