#!/usr/bin/env python3
"""Check that relative Markdown links point at files that exist.

Only local links are checked: http(s):// and mailto: links are skipped (external
link liveness is flaky and not this gate's job), as are pure anchors (#section).
A relative link may carry its own #anchor, which is stripped before resolving.

Pure stdlib. Run locally:  python3 tools/check_md_links.py
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent.parent
LINK_RE = re.compile(r"\[[^\]]*\]\(([^)]+)\)")
SKIP_PREFIXES = ("http://", "https://", "mailto:", "#", "tel:")


def md_files() -> list[Path]:
    out = []
    for p in ROOT.rglob("*.md"):
        parts = p.relative_to(ROOT).parts
        if ".git" in parts:
            continue
        # Skip vendored, generated module trees.
        if "cli" in parts and parts[0] == "skills":
            continue
        out.append(p)
    return out


def main() -> int:
    errors: list[str] = []
    for md in md_files():
        text = md.read_text(encoding="utf-8")
        for m in LINK_RE.finditer(text):
            target = m.group(1).strip()
            if target.startswith(SKIP_PREFIXES):
                continue
            # Strip an optional anchor and any surrounding angle brackets/quotes.
            path_part = target.split("#", 1)[0].strip().strip("<>").split(" ", 1)[0]
            if not path_part:
                continue
            resolved = (md.parent / path_part).resolve()
            if not resolved.exists():
                rel = md.relative_to(ROOT)
                errors.append(f"{rel}: broken local link -> {target}")

    if errors:
        print("Markdown local-link check FAILED:")
        for e in errors:
            print(f"  - {e}")
        return 1

    print(f"Markdown local-link check passed across {len(md_files())} file(s).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
