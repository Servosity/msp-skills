#!/usr/bin/env python3
"""Verify every skill (and the pack) has its marketplace social assets.

The publish pipeline mints these with social-preview-generator (msp-skills-publish
Step 6.5). They are not optional polish: the 400x400 is what the MCP Market upload
form and the forthcoming Claude-plugin icon field consume, the 512x512 is the
`server.json icons[]` target, and the 1200x630 is each docs page's og:image. Before
this gate existed, generation was an agent-driven step enforced by nothing, so a
publish could report "all gates green" having produced no images at all. This gate
is the receipt that the assets actually got made.

For every skill dir under skills/, require (under docs/assets/social/<slug>/):
  - <slug>-400x400.png   exactly 400x400   (MCP Market square)
  - <slug>-512x512.png   exactly 512x512   (server.json icons[])
  - wide-1200x630.png    exactly 1200x630  (docs-page og:image)
Plus, at the docs/assets/social/ root, the pack/main set (<mp> = marketplace name):
  - <mp>-400x400.png     exactly 400x400   (pack square icon)
  - <mp>-512x512.png     exactly 512x512   (pack square icon)
  - og-1280x640.png      exactly 1280x640  (repo-level GitHub social preview)

Pure stdlib: PNG dimensions are read straight from the IHDR header (no Pillow), so
this runs identically locally and in CI. Exits non-zero listing every violation.
Run locally:  python3 tools/maintainer/check_social_assets.py
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent.parent
SKILLS_DIR = ROOT / "skills"
SOCIAL_DIR = ROOT / "docs" / "assets" / "social"
MARKETPLACE = ROOT / ".claude-plugin" / "marketplace.json"

PNG_SIG = b"\x89PNG\r\n\x1a\n"


def png_size(p: Path) -> tuple[int, int] | None:
    """Return (width, height) from a PNG's IHDR chunk, or None if not a valid PNG."""
    try:
        with open(p, "rb") as f:
            head = f.read(24)
    except OSError:
        return None
    if len(head) < 24 or head[:8] != PNG_SIG or head[12:16] != b"IHDR":
        return None
    return int.from_bytes(head[16:20], "big"), int.from_bytes(head[20:24], "big")


def check(errors: list[str], path: Path, want: tuple[int, int]) -> None:
    """Append an error unless `path` exists and is a PNG of exactly `want` dimensions."""
    rel = path.relative_to(ROOT)
    if not path.exists():
        errors.append(f"missing {rel} (expected {want[0]}x{want[1]} PNG)")
        return
    size = png_size(path)
    if size is None:
        errors.append(f"{rel} is not a valid PNG")
    elif size != want:
        errors.append(f"{rel} is {size[0]}x{size[1]}, expected {want[0]}x{want[1]}")


def marketplace_name() -> str:
    try:
        return (json.loads(MARKETPLACE.read_text(encoding="utf-8")).get("name") or "").strip() or "msp-skills"
    except (OSError, json.JSONDecodeError):
        return "msp-skills"


def main() -> int:
    errors: list[str] = []

    skill_dirs = sorted(p for p in SKILLS_DIR.iterdir() if p.is_dir()) if SKILLS_DIR.is_dir() else []
    if not skill_dirs:
        print("No skills found under skills/.")
        return 1

    # Per-skill assets.
    for d in skill_dirs:
        slug = d.name
        skill_social = SOCIAL_DIR / slug
        check(errors, skill_social / f"{slug}-400x400.png", (400, 400))
        check(errors, skill_social / f"{slug}-512x512.png", (512, 512))
        check(errors, skill_social / "wide-1200x630.png", (1200, 630))

    # Pack / repo-level assets (the "main" set).
    mp = marketplace_name()
    check(errors, SOCIAL_DIR / f"{mp}-400x400.png", (400, 400))
    check(errors, SOCIAL_DIR / f"{mp}-512x512.png", (512, 512))
    check(errors, SOCIAL_DIR / "og-1280x640.png", (1280, 640))

    if errors:
        print("Social assets check FAILED:")
        for e in errors:
            print(f"  - {e}")
        print(
            "\nGenerate them (msp-skills-publish Step 6.5):\n"
            "  ~/.claude/skills/social-preview-generator/scripts/generate.py "
            "--repo . --mode both --out docs/assets/social\n"
            "  ~/.claude/skills/social-preview-generator/scripts/generate.py "
            "--repo . --mode og --out docs/assets/social"
        )
        return 1

    print(f"Social assets check passed for {len(skill_dirs)} skill(s) + pack ({mp}).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
