#!/usr/bin/env python3
"""Verify every skill under skills/ meets the contributor contract.

Checks, per CONTRIBUTING.md "What a Skill PR must include":
  - SKILL.md exists with frontmatter containing the required keys.
  - README.md exists and has a blockquote banner right after the H1
    (non-affiliation banner for third-party skills, trademark note for
    first-party).
  - install.sh, install.ps1, mcp-install.md, manifest.json exist.

Pure stdlib (no PyYAML): frontmatter is parsed as simple `key:` lines, which is
all the contract needs. Exits non-zero with a list of every violation so CI can
gate on it. Run locally:  python3 tools/check_skill_contract.py
"""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = Path(__file__).resolve().parent.parent.parent
SKILLS_DIR = ROOT / "skills"

REQUIRED_FRONTMATTER = ["name", "description", "allowed-tools", "author", "license", "vendor"]
REQUIRED_FILES = ["SKILL.md", "README.md", "manifest.json", "install.sh", "install.ps1", "mcp-install.md"]
# A markdown-only skill (no vendored Go cli/) ships no installers, no manifest,
# and no MCP wire-up doc - it is markdown-thin. Only SKILL.md + README.md are
# required, with the same frontmatter keys and README banner.
REQUIRED_FILES_MARKDOWN_ONLY = ["SKILL.md", "README.md"]


def frontmatter_keys(skill_md: Path) -> set[str]:
    text = skill_md.read_text(encoding="utf-8")
    if not text.startswith("---"):
        return set()
    end = text.find("\n---", 3)
    if end == -1:
        return set()
    block = text[3:end]
    keys = set()
    for line in block.splitlines():
        # Top-level keys only (no leading whitespace), of the form `key:`.
        if line and not line[0].isspace() and ":" in line:
            keys.add(line.split(":", 1)[0].strip())
    return keys


def has_banner_after_h1(readme: Path) -> bool:
    seen_h1 = False
    count = 0
    for line in readme.read_text(encoding="utf-8").splitlines():
        s = line.strip()
        if not seen_h1:
            if s.startswith("# "):
                seen_h1 = True
            continue
        if not s:
            continue
        count += 1
        if s.startswith(">"):
            return True
        if count >= 4:  # banner must be near the top, not buried
            return False
    return False


def main() -> int:
    errors: list[str] = []
    skill_dirs = sorted(p for p in SKILLS_DIR.iterdir() if p.is_dir())
    if not skill_dirs:
        print("No skills found under skills/.")
        return 1

    for d in skill_dirs:
        name = d.name
        slug = registry.slug_for_dir(name)
        required = (
            REQUIRED_FILES_MARKDOWN_ONLY
            if registry.is_markdown_only(slug)
            else REQUIRED_FILES
        )
        for f in required:
            if not (d / f).exists():
                errors.append(f"{name}: missing required file {f}")

        skill_md = d / "SKILL.md"
        if skill_md.exists():
            keys = frontmatter_keys(skill_md)
            for k in REQUIRED_FRONTMATTER:
                if k not in keys:
                    errors.append(f"{name}: SKILL.md frontmatter missing key '{k}'")

        readme = d / "README.md"
        if readme.exists() and not has_banner_after_h1(readme):
            errors.append(f"{name}: README.md needs a '>' banner (non-affiliation or trademark) right after the H1")

    if errors:
        print("Skill contract check FAILED:")
        for e in errors:
            print(f"  - {e}")
        return 1

    print(f"Skill contract check passed for {len(skill_dirs)} skill(s).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
