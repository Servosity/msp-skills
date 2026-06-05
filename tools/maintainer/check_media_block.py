#!/usr/bin/env python3
"""Gate: every connector README carries a working media block with real assets.

The publish skill injects a <!-- media:start --> ... <!-- media:end --> block
into each connector's skills/<slug>/README.md - an <img> pointing at the demo
GIF (video skills) or the wide social card (non-video). This gate is the receipt
that the block is present AND that every image it references is a real, non-empty
file on disk - so a README can never ship a broken-image placeholder on GitHub.

It is the README-asset twin of the surface-coverage gate. The same class of bug
that orphaned the homepage table on 2026-06-05 - connectwise-manage + hubspot
shipped while a surface still pointed at the old set - is exactly what an absent
or dangling media block is: a per-skill surface that silently went stale. This
gate makes that loud.

For every non-markdown-only connector it asserts:
  a. skills/<dir>/README.md has both media markers, start before end.
  b. Every <img src="..."> inside the block resolves (relative to the skill dir)
     to a file that exists and is non-empty.
  c. If the registry says has_video, docs/assets/video/<slug>/animated-og.gif
     exists and is under MAX_GIF_BYTES (a repo-size guard for ~50 connectors).

The markdown-only concierge is skipped (no demo, no media block).

Pure stdlib. Run locally:
    python3 tools/maintainer/check_media_block.py
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT

# Per-skill animated-og.gif budget. At ~500 KB each today, 2 MB leaves generous
# headroom while guarding against a multi-MB GIF blowing up the repo when the
# fleet reaches ~50 connectors.
MAX_GIF_BYTES = 2 * 1024 * 1024

IMG_SRC_RE = re.compile(r"""<img\b[^>]*?\bsrc\s*=\s*["']([^"']+)["']""", re.IGNORECASE)


def media_block(text: str) -> str | None:
    """Return the text between <!-- media:start --> and <!-- media:end -->,
    or None if the pair is missing or out of order."""
    start = text.find("<!-- media:start -->")
    end = text.find("<!-- media:end -->")
    if start == -1 or end == -1 or start >= end:
        return None
    return text[start:end]


def main() -> int:
    errors: list[str] = []

    connectors = [
        (slug, meta)
        for slug, meta in registry.skills().items()
        if not meta.get("markdown_only")
    ]

    for slug, meta in connectors:
        skill_dir = registry.skill_path(slug)
        readme = skill_dir / "README.md"
        rel_readme = readme.relative_to(ROOT)

        if not readme.exists():
            errors.append(f"{slug}: missing {rel_readme}")
            continue

        text = readme.read_text(encoding="utf-8")

        # (a) media markers present and ordered.
        block = media_block(text)
        if block is None:
            errors.append(
                f"{slug}: {rel_readme} has no <!-- media:start --> / "
                "<!-- media:end --> block (or they are out of order) - the publish "
                "skill injects it"
            )
            continue

        # (b) every <img src> inside the block resolves to a non-empty file.
        srcs = IMG_SRC_RE.findall(block)
        if not srcs:
            errors.append(
                f"{slug}: the media block in {rel_readme} contains no "
                "<img src=...> - expected the demo GIF or social card image"
            )
        for src in srcs:
            asset = (skill_dir / src).resolve()
            if not asset.exists():
                errors.append(
                    f"{slug}: media image {src!r} in {rel_readme} does not exist "
                    f"(resolved to {asset})"
                )
            elif asset.stat().st_size == 0:
                errors.append(
                    f"{slug}: media image {src!r} in {rel_readme} is empty "
                    f"(0 bytes at {asset})"
                )

        # (c) video skills must ship a sized-bounded animated-og.gif.
        if meta.get("has_video"):
            gif = ROOT / "docs" / "assets" / "video" / slug / "animated-og.gif"
            rel_gif = gif.relative_to(ROOT)
            if not gif.exists():
                errors.append(
                    f"{slug}: has_video is set but {rel_gif} is missing"
                )
            else:
                size = gif.stat().st_size
                if size == 0:
                    errors.append(f"{slug}: {rel_gif} is empty (0 bytes)")
                elif size > MAX_GIF_BYTES:
                    errors.append(
                        f"{slug}: {rel_gif} is {size} bytes (over the "
                        f"{MAX_GIF_BYTES}-byte / 2 MB repo-size guard)"
                    )

    if errors:
        print("check_media_block FAILED:")
        for e in errors:
            print(f"  - {e}")
        print(
            "\nFix: re-run the publish skill's media injection for the named skill "
            "so its README media block + demo assets are present and in budget."
        )
        return 1

    print(f"PASS: README media block + assets for {len(connectors)} connector(s).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
