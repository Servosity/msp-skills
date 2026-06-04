#!/usr/bin/env python3
"""Generate docs/llms.txt (compact index) and docs/llms-full.txt (expanded
corpus) from the catalog + skills registry + the rendered docs pages.

Why two files: llms.txt is the short, skimmable index AI search engines fetch
first (one line per connector); llms-full.txt is the full corpus they pull when
they want depth (per-connector outcomes, instead/say examples, FAQs, governance
summary, plus the WHY pillars and the glossary definition).

Source of truth, in order of preference per connector:
  1. skills/<slug>/page.json  - structured page data, when the pipeline emits it.
  2. docs/skills/<slug>.md     - the rendered page body, parsed as a fallback.

Output is DETERMINISTIC: stable ordering (skills sorted by slug), no timestamps,
no machine-specific paths. verify_all.sh regenerates both files and diffs them
against the committed copies (same drift gate as the catalog), so a stale
llms.txt fails CI.

Pure stdlib. Run locally:  python3 tools/maintainer/build-llms.py
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/maintainer module)

ROOT = registry.ROOT
DOCS = ROOT / "docs"
SKILLS_DIR = registry.SKILLS_DIR
CATALOG = ROOT / "catalog.json"
LLMS = DOCS / "llms.txt"
LLMS_FULL = DOCS / "llms-full.txt"

SITE = "https://msp-skills.compoundingteams.com"


# --------------------------------------------------------------------------- #
# Front-matter + markdown parsing helpers (pure stdlib, no yaml dep).
# --------------------------------------------------------------------------- #
def split_front_matter(text: str) -> tuple[dict, str]:
    """Return (front_matter_dict, body). Parses simple scalars plus the
    `faqs:` list (items shaped `- q: ...` / `a: ...`). Other nested structures
    are ignored. Good enough for the docs/skills/*.md pages we read."""
    fm: dict = {}
    body = text
    if text.startswith("---"):
        end = text.find("\n---", 3)
        if end != -1:
            block = text[3:end]
            body = text[end + 4 :]
            fm = _parse_fm_block(block)
    return fm, body


def _parse_fm_block(block: str) -> dict:
    fm: dict = {}
    faqs: list[dict] = []
    in_faqs = False
    cur: dict | None = None
    for line in block.splitlines():
        if re.match(r"^faqs:\s*$", line):
            in_faqs = True
            fm["faqs"] = faqs
            continue
        if in_faqs:
            mq = re.match(r"^\s*-\s*q:\s*(.*)$", line)
            ma = re.match(r"^\s*a:\s*(.*)$", line)
            if mq:
                cur = {"q": _unquote(mq.group(1)), "a": ""}
                faqs.append(cur)
                continue
            if ma and cur is not None:
                cur["a"] = _unquote(ma.group(1))
                continue
            # A non-list, non-indented line ends the faqs block.
            if line and not line[0].isspace() and not line.startswith("-"):
                in_faqs = False
            else:
                continue
        m = re.match(r"^([A-Za-z_][\w-]*):\s*(.*)$", line)
        if m and (line and not line[0].isspace()):
            fm[m.group(1)] = _unquote(m.group(2).strip())
    return fm


def _unquote(v: str) -> str:
    v = v.strip()
    if (v.startswith('"') and v.endswith('"')) or (v.startswith("'") and v.endswith("'")):
        return v[1:-1]
    return v


def strip_md(s: str) -> str:
    """Flatten inline markdown to plain text for the corpus."""
    s = re.sub(r"`([^`]*)`", r"\1", s)
    s = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", s)  # link -> text
    s = re.sub(r"\{:[^}]*\}", "", s)  # kramdown attrs
    s = re.sub(r"[*_]{1,3}", "", s)  # emphasis
    s = s.replace("&ldquo;", '"').replace("&rdquo;", '"').replace("&nbsp;", " ")
    return s.strip()


def section(body: str, heading: str) -> str:
    """Return the text under a `## heading` up to the next `## ` heading."""
    pat = re.compile(
        rf"^##\s+{re.escape(heading)}\s*$(.*?)(?=^##\s|\Z)", re.DOTALL | re.MULTILINE
    )
    m = pat.search(body)
    return m.group(1).strip() if m else ""


def parse_table(block: str) -> list[tuple[str, str]]:
    """Parse a 2-column markdown table -> [(col1, col2)], skipping header +
    separator rows."""
    rows: list[tuple[str, str]] = []
    for line in block.splitlines():
        line = line.strip()
        if not line.startswith("|"):
            continue
        cells = [c.strip() for c in line.strip("|").split("|")]
        if len(cells) < 2:
            continue
        if set("".join(cells).replace(" ", "")) <= set("-:"):  # separator
            continue
        c1, c2 = strip_md(cells[0]), strip_md(cells[1])
        if c1.lower() in ("question your msp keeps asking", "outcome") or not c1:
            continue
        rows.append((c1, c2))
    return rows


# --------------------------------------------------------------------------- #
# Per-connector data assembly.
# --------------------------------------------------------------------------- #
def load_page_data(slug: str, meta: dict, cat: dict) -> dict:
    """Return a dict with: name, tagline, category, badge, url, install,
    intro, outcomes [(q, cmd)], faqs [(q, a)], governance_summary."""
    page_json = SKILLS_DIR / slug / "page.json"
    data: dict = {
        "slug": slug,
        "name": meta.get("display_name", slug.title()),
        "tagline": meta.get("tagline", ""),
        "category": meta.get("category", ""),
        "badge": badge_state(meta),
        "url": f"{SITE}/skills/{slug}/",
        "install": cat.get("install_skill_one_liner", ""),
        "intro": "",
        "outcomes": [],
        "faqs": [],
        "governance_summary": "",
    }
    if page_json.exists():
        pj = json.loads(page_json.read_text())
        data["intro"] = pj.get("intro", data["intro"])
        data["outcomes"] = [(o.get("q", ""), o.get("cmd", "")) for o in pj.get("outcomes", [])]
        data["faqs"] = [(f.get("q", ""), f.get("a", "")) for f in pj.get("faqs", [])]
        data["governance_summary"] = pj.get("governance_summary", "")
        return data

    # Fallback: parse the rendered docs page.
    md = DOCS / "skills" / f"{slug}.md"
    if md.exists():
        fm, body = split_front_matter(md.read_text())
        # Intro = first prose paragraph after the H1 + banner blockquote + the
        # badge notice + the vocab one-liner.
        data["intro"] = first_intro_paragraph(body)
        data["outcomes"] = parse_table(section(body, "What it does"))
        data["faqs"] = [(f["q"], f["a"]) for f in fm.get("faqs", [])]
        gov = section(body, "Safety model")
        if gov:
            # Compress the safety table to a one-line summary.
            data["governance_summary"] = summarize_safety(gov)
    return data


def first_intro_paragraph(body: str) -> str:
    blocks = re.split(r"\n\s*\n", body)
    for b in blocks:
        s = b.strip()
        if not s:
            continue
        first = s.splitlines()[0].lstrip()
        if first.startswith(("#", ">", "<", "{%", "{{", "|", "---", "***")):
            continue
        # Skip the badge notice + the "New to the term?" vocab one-liner.
        if s.startswith("**Awaiting live verification**") or s.startswith(
            "New to the term?"
        ):
            continue
        # Skip pure button/link rows.
        stripped = re.sub(r"!?\[[^\]]*\]\([^)]*\)", "", s)
        stripped = re.sub(r"\{:[^}]*\}", "", stripped).replace("&nbsp;", " ").strip()
        if len(stripped) <= 3:
            continue
        return strip_md(s.replace("\n", " "))
    return ""


def summarize_safety(gov: str) -> str:
    tiers = []
    for c1, c2 in parse_table(gov):
        if c1.lower() in ("tier",):
            continue
        tiers.append(c1)
    if tiers:
        return "Permission tiers: " + ", ".join(tiers) + "."
    # No table? take the first sentence-ish line.
    for line in gov.splitlines():
        line = strip_md(line).strip()
        if line and not line.startswith("|"):
            return line
    return ""


def badge_state(meta: dict) -> str:
    lv = meta.get("live_verified", {})
    if isinstance(lv, dict) and lv.get("status") == "verified":
        return "Live-verified"
    return "Awaiting live verification"


# --------------------------------------------------------------------------- #
# WHY pillars + glossary definition (read from the rendered pages).
# --------------------------------------------------------------------------- #
def why_pillars() -> list[tuple[str, str]]:
    md = DOCS / "why-msp-skills.md"
    if not md.exists():
        return []
    _, body = split_front_matter(md.read_text())
    out: list[tuple[str, str]] = []
    for m in re.finditer(r"^##\s+(.+?)\s*$(.*?)(?=^##\s|\Z)", body, re.DOTALL | re.MULTILINE):
        head = strip_md(m.group(1))
        # First prose paragraph of the pillar.
        para = ""
        for b in re.split(r"\n\s*\n", m.group(2)):
            s = b.strip()
            if not s or s.startswith(("#", ">", "<", "{%", "|", "-")):
                continue
            para = strip_md(s.replace("\n", " "))
            break
        out.append((head, para))
    return out


def glossary_definition() -> str:
    md = DOCS / "what-is-an-mcp-server.md"
    if not md.exists():
        return ""
    _, body = split_front_matter(md.read_text())
    for b in re.split(r"\n\s*\n", body):
        s = b.strip()
        if not s or s.startswith(("#", ">", "<", "{%", "|")):
            continue
        return strip_md(s.replace("\n", " "))
    return ""


# --------------------------------------------------------------------------- #
# Rendering.
# --------------------------------------------------------------------------- #
def render_index(pages: list[dict]) -> str:
    lines = [
        "# MSP Skills",
        "",
        "> Free MCP servers and Skills that connect your MSP tools (PSA, RMM, "
        "backup, M365, and more) to the AI you already use - Claude, ChatGPT, "
        "Copilot, Codex, Cursor, and any agent that speaks the Model Context "
        "Protocol. Local SQLite mirror so cross-client questions don't hit API "
        "rate limits. Built for MSP business owners. No code required. "
        "Apache-2.0 licensed.",
        "",
        "What this site calls an MCP server is what ChatGPT calls an app or "
        "connector, Claude on the web and Microsoft Copilot call a connector, "
        "and Claude Code calls a Skill - all the same MCP standard. Full "
        f"answer: {SITE}/what-is-an-mcp-server/",
        "",
        "## Connectors",
        "",
    ]
    for p in pages:
        lines.append(f"- {p['name']} ({p['category']}) - {p['tagline']}")
        lines.append(f"  Badge: {p['badge']}. Page: {p['url']}")
        if p["install"]:
            lines.append(f"  Install: {p['install']}")
    lines += [
        "",
        "## Key pages",
        "",
        f"- Why msp-skills: {SITE}/why/",
        f"- What is an MCP server: {SITE}/what-is-an-mcp-server/",
        f"- Which agent: {SITE}/which-agent/",
        f"- Full corpus for AI search: {SITE}/llms-full.txt",
        "",
    ]
    return "\n".join(lines) + "\n"


def render_full(pages: list[dict]) -> str:
    lines = [
        "# MSP Skills - full corpus",
        "",
        "> Free MCP servers and Skills connecting MSP tools to the AI you "
        "already use. Built for MSP business owners. Apache-2.0, runs locally, "
        "no data leaves your network. This file is the expanded corpus; the "
        f"short index is at {SITE}/llms.txt",
        "",
        "## What an MCP server is (for MSPs)",
        "",
        glossary_definition(),
        "",
        "One thing, many names: what this site calls an MCP server is what "
        "ChatGPT calls an app or connector, Claude on the web calls a "
        "connector, Microsoft Copilot calls a connector, and Claude Code calls "
        "a Skill. Same standard underneath: the Model Context Protocol.",
        "",
        "## Why MSP owners use msp-skills",
        "",
    ]
    for head, para in why_pillars():
        lines.append(f"### {head}")
        lines.append("")
        if para:
            lines.append(para)
            lines.append("")

    lines += ["## Connectors", ""]
    for p in pages:
        lines.append(f"### {p['name']} ({p['category']})")
        lines.append("")
        lines.append(f"Page: {p['url']}")
        lines.append(f"Badge: {p['badge']}")
        if p["install"]:
            lines.append(f"Install: {p['install']}")
        lines.append("")
        if p["intro"]:
            lines.append(p["intro"])
            lines.append("")
        if p["outcomes"]:
            lines.append("Outcomes (question -> command your AI agent runs):")
            for q, cmd in p["outcomes"]:
                if cmd:
                    lines.append(f"- {q} -> {cmd}")
                else:
                    lines.append(f"- {q}")
            lines.append("")
        if p["faqs"]:
            lines.append("FAQ:")
            for q, a in p["faqs"]:
                lines.append(f"Q: {q}")
                lines.append(f"A: {a}")
            lines.append("")
        if p["governance_summary"]:
            lines.append(f"Governance: {p['governance_summary']}")
            lines.append("")

    return "\n".join(lines).rstrip() + "\n"


def main() -> int:
    catalog = json.loads(CATALOG.read_text())
    cat_by_slug = {c["name"]: c for c in catalog.get("skills", [])}
    meta = registry.skills()  # sorted by slug

    pages = [load_page_data(slug, meta[slug], cat_by_slug.get(slug, {})) for slug in meta]

    LLMS.write_text(render_index(pages), encoding="utf-8")
    LLMS_FULL.write_text(render_full(pages), encoding="utf-8")
    print(f"Wrote {LLMS.relative_to(ROOT)} and {LLMS_FULL.relative_to(ROOT)} "
          f"({len(pages)} connectors).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
