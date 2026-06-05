#!/usr/bin/env python3
"""Gate: enforce the public-skill vocabulary contract.

Two rules, both sourced from tools/maintainer/vocab.json:

  banned          terms that must not appear in a skill's public prose. The
                  multiword phrases ("agent-native CLI", "capability pack") are
                  matched case-insensitively; the proper-noun competitor names
                  ("Smithery", "Composio") are matched case-sensitively so a
                  lowercased coincidence doesn't false-fire. Scanned files:
                  README.md, SKILL.md, pain-point.md, page.json.

  required_readme strings every skills/<slug>/README.md must contain verbatim
                  (case-sensitive). These are the terms MSPs actually search for
                  ("Claude Code Skill", "MCP server"); their absence hurts
                  discoverability.

Allowlist: any line containing "vocab:ignore" is skipped for the banned scan.

Pure stdlib. Run locally:  python3 tools/maintainer/check_vocabulary.py
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

SKILLS_DIR = registry.SKILLS_DIR
VOCAB = Path(__file__).resolve().parent / "vocab.json"

SCAN_FILES = ["README.md", "SKILL.md", "pain-point.md", "page.json"]
IGNORE_MARKER = "vocab:ignore"

# Proper-noun competitor names are matched case-sensitively; everything else
# (descriptive phrases) is matched case-insensitively.
CASE_SENSITIVE = {"Smithery", "Composio"}


def load_vocab() -> dict:
    return json.loads(VOCAB.read_text(encoding="utf-8"))


def banned_hit(line: str, term: str) -> bool:
    if term in CASE_SENSITIVE:
        return term in line
    return term.lower() in line.lower()


def main() -> int:
    vocab = load_vocab()
    banned = vocab.get("banned", [])
    required = vocab.get("required_readme", [])

    violations: list[str] = []

    for slug in sorted(registry.skills()):
        skill_dir = registry.skill_path(slug)

        # Banned-term scan.
        for fname in SCAN_FILES:
            fpath = skill_dir / fname
            if not fpath.exists():
                continue
            try:
                text = fpath.read_text(encoding="utf-8")
            except OSError:
                continue
            rel = fpath.relative_to(registry.ROOT)
            for n, line in enumerate(text.splitlines(), start=1):
                if IGNORE_MARKER in line:
                    continue
                for term in banned:
                    if banned_hit(line, term):
                        snippet = line.strip()
                        if len(snippet) > 120:
                            snippet = snippet[:117] + "..."
                        violations.append(f"{rel}:{n}: banned term '{term}': {snippet}")

        # Required-README scan.
        readme = skill_dir / "README.md"
        if readme.exists():
            try:
                rtext = readme.read_text(encoding="utf-8")
            except OSError:
                rtext = ""
            missing = [t for t in required if t not in rtext]
            if missing:
                rel = readme.relative_to(registry.ROOT)
                violations.append(
                    f"{rel}: missing required term(s): {', '.join(repr(m) for m in missing)}"
                )

    if violations:
        print("check_vocabulary FAILED:")
        for v in violations:
            print(f"  {v}")
        print(
            "\nBanned terms: rephrase, or add 'vocab:ignore' on the line ONLY if "
            "it is a legitimate registry-infrastructure reference (not a "
            "competitor comparison). Required terms: add the verbatim string to "
            "the README."
        )
        return 1

    print("PASS: vocabulary contract satisfied")
    return 0


if __name__ == "__main__":
    sys.exit(main())
