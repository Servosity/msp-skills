#!/usr/bin/env python3
"""Answer-first AEO gate for the Jekyll site under docs/.

Two assertions, both aimed at making the site answer its own title immediately -
the behavior AI search engines (and impatient MSP owners) reward:

  1. ANSWER-FIRST. For every docs/*.md page that carries a `title` front matter,
     the FIRST paragraph of body text - after the front matter, after any
     leading H1, blockquote banner, badge line, or button row - must be
     non-empty and <= MAX_WORDS words. The page must answer its title before any
     wall of navigation.

  2. UNIQUE TITLE + DESCRIPTION. Every docs/skills/*.md page plus the WHY page
     (/why/) and the glossary page (/what-is-an-mcp-server/) must declare both a
     unique `title` and a unique `description` in front matter.

Scope decision (documented per task brief): docs/integrations/*.md is SKIPPED by
the answer-first scan. Those pages already follow an answer-first structure, but
there are nine of them, they are pre-existing, and tightening them is out of
scope for this change. The glossary, why, skills, guides, and homepage pages -
the ones this positioning work owns - are all gated.

A per-file ALLOWLIST (with reasons) lets pre-existing pages pass today and
tighten later. Add an entry only when the fix would be more than a trivial
reorder / add-description.

Pure stdlib. Run locally:  python3 tools/maintainer/check_aeo.py
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent.parent
DOCS = ROOT / "docs"

MAX_WORDS = 90

# Permalinks (or repo-relative doc paths) that must have unique title +
# description. Resolved by permalink front matter where present.
REQUIRE_TITLE_DESC_PERMALINKS = {"/why/", "/what-is-an-mcp-server/"}

# Every docs/skills/*.md page must follow the generator's AEO formulas
# (render_docs_page.py): the title carries the search keywords MSPs type
# ("MCP Server", free/open-source/local), and the first answer paragraph is a
# literal direct answer to "is there an MCP server for X?". These assertions
# keep a hand edit or a generator regression from silently dropping either.
SKILL_TITLE_REQUIRED = "MCP Server - Free, Open Source, Runs Locally | MSP Skills"
SKILL_ANSWER_PREFIX = "Yes - there is an MCP server for"

# Per-file answer-first allowlist. key = path relative to ROOT, value = reason.
# Keep this list short and justified; each entry is a deferred-closure marker.
# Currently EMPTY: every in-scope page (homepage, why, glossary, skills, guides)
# answers its title in <= MAX_WORDS today. Add an entry here only if a future
# pre-existing page fails and the fix is more than a trivial reorder.
ANSWER_FIRST_ALLOWLIST: dict[str, str] = {}


def split_front_matter(text: str) -> tuple[dict, str]:
    """Return (front_matter_dict, body). Front matter is the leading --- block.
    Only the simple `key: value` lines we need are parsed (title, description,
    permalink); nested structures (faqs/howto) are ignored."""
    if not text.startswith("---"):
        return {}, text
    end = text.find("\n---", 3)
    if end == -1:
        return {}, text
    fm_block = text[3:end]
    body = text[end + 4 :]
    fm: dict[str, str] = {}
    for line in fm_block.splitlines():
        # Top-level keys only (no leading whitespace) so list items under
        # faqs:/howto: don't get mistaken for keys.
        if not line or line[0] in (" ", "\t", "-"):
            continue
        m = re.match(r"^([A-Za-z_][\w-]*):\s*(.*)$", line)
        if not m:
            continue
        key, val = m.group(1), m.group(2).strip()
        if (val.startswith('"') and val.endswith('"')) or (
            val.startswith("'") and val.endswith("'")
        ):
            val = val[1:-1]
        fm[key] = val
    return fm, body


def is_banner_block(block: str) -> bool:
    """True for blocks that are leading chrome, not the answer paragraph:
    an H1/H2 heading, a blockquote banner, a badge/button-only line, an HTML
    include div, a horizontal rule, or a Liquid include line."""
    s = block.strip()
    if not s:
        return True
    # The verification-badge paragraph render_docs_page.py emits above the
    # direct answer (both states, current and legacy spellings) is chrome,
    # not the answer.
    if s.startswith(
        ("**✓ Live-verified", "**Passes all 4 mechanical gates**",
         "**Live-verified**", "**Awaiting live verification**")
    ):
        return True
    first = s.splitlines()[0].lstrip()
    if first.startswith("#"):  # heading
        return True
    if first.startswith(">"):  # blockquote banner
        return True
    if first.startswith("<"):  # raw HTML (e.g. why-cards, include output)
        return True
    if first.startswith("{%") or first.startswith("{{"):  # Liquid include/tag
        return True
    if s.startswith("---") or s == "***":  # hr
        return True
    # A line that is ONLY links/badges/buttons (no prose sentence). Heuristic:
    # strip markdown links and images; if almost nothing prose-like remains,
    # treat as a button/badge row.
    stripped = re.sub(r"!?\[[^\]]*\]\([^)]*\)", "", s)
    stripped = re.sub(r"\{:[^}]*\}", "", stripped)  # kramdown {: .btn} attrs
    stripped = stripped.replace("&nbsp;", " ").replace("·", " ").strip()
    if len(stripped) <= 3:  # nothing but link chrome
        return True
    return False


def first_answer_paragraph(body: str) -> str | None:
    """Return the first prose paragraph after skipping leading banner blocks."""
    # Normalize: paragraphs are separated by blank lines.
    blocks = re.split(r"\n\s*\n", body)
    for block in blocks:
        if is_banner_block(block):
            continue
        return block.strip()
    return None


def word_count(text: str) -> int:
    # Strip inline markdown emphasis/code/link syntax for a fair word count.
    t = re.sub(r"`[^`]*`", " ", text)
    t = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", t)  # keep link text
    t = re.sub(r"[*_>#]", " ", t)
    return len([w for w in t.split() if any(c.isalnum() for c in w)])


def main() -> int:
    errors: list[str] = []

    title_seen: dict[str, str] = {}
    desc_seen: dict[str, str] = {}

    md_files = sorted(DOCS.glob("*.md")) + sorted(DOCS.glob("skills/*.md")) + sorted(
        DOCS.glob("guides/*.md")
    )

    for md in md_files:
        rel = str(md.relative_to(ROOT))
        text = md.read_text(encoding="utf-8")
        fm, body = split_front_matter(text)

        # Only pages that declare a title are in scope for the answer-first rule.
        if "title" not in fm:
            continue

        permalink = fm.get("permalink", "")

        # --- Assertion 2: unique title + description for the required pages. ---
        is_skill = md.parent.name == "skills"
        require_unique = is_skill or permalink in REQUIRE_TITLE_DESC_PERMALINKS
        if require_unique:
            title = fm.get("title", "").strip()
            desc = fm.get("description", "").strip()
            if not title:
                errors.append(f"{rel}: missing `title` front matter")
            if not desc:
                errors.append(f"{rel}: missing `description` front matter")
            if title:
                if title in title_seen:
                    errors.append(
                        f"{rel}: duplicate title (also in {title_seen[title]}): {title!r}"
                    )
                else:
                    title_seen[title] = rel
            if desc:
                if desc in desc_seen:
                    errors.append(
                        f"{rel}: duplicate description (also in {desc_seen[desc]})"
                    )
                else:
                    desc_seen[desc] = rel
            # --- Assertion 3: skill pages follow the AEO title formula. ---
            if is_skill and title and SKILL_TITLE_REQUIRED not in title:
                errors.append(
                    f"{rel}: skill-page title missing the AEO formula "
                    f"({SKILL_TITLE_REQUIRED!r}): {title!r}"
                )

        # --- Assertion 1: answer-first paragraph <= MAX_WORDS. ---
        if rel in ANSWER_FIRST_ALLOWLIST:
            continue
        para = first_answer_paragraph(body)
        if not para:
            errors.append(f"{rel}: no answer paragraph found after the title/banner")
            continue
        # --- Assertion 4: skill pages open with the literal direct answer. ---
        if is_skill and not para.startswith(SKILL_ANSWER_PREFIX):
            preview = " ".join(para.split()[:12])
            errors.append(
                f"{rel}: skill-page first paragraph must start with "
                f"{SKILL_ANSWER_PREFIX!r}: \"{preview} ...\""
            )
        wc = word_count(para)
        if wc > MAX_WORDS:
            preview = " ".join(para.split()[:12])
            errors.append(
                f"{rel}: first answer paragraph is {wc} words (limit {MAX_WORDS}): "
                f"\"{preview} ...\""
            )

    if errors:
        print("check_aeo FAILED:")
        for e in errors:
            print(f"  {e}")
        print(
            "\nFix: make the first body paragraph answer the title in <= "
            f"{MAX_WORDS} words, or add the file to ANSWER_FIRST_ALLOWLIST with a "
            "reason. Skill/why/glossary pages also need a unique title + description."
        )
        return 1

    print("PASS: AEO answer-first + title/description contract satisfied")
    return 0


if __name__ == "__main__":
    sys.exit(main())
