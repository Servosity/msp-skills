#!/usr/bin/env python3
"""Generate docs/question-book.md - "The MSP AI Question Book".

Aggregates every plain-English question the connectors can answer - the
`instead_of_say[].say` prompts and `outcomes[].question` rows from every
skills/<slug>/page.json - into one Jekyll page, grouped by tool category and
connector. One page that shows an MSP owner, in their own vocabulary, what
agentic AI can answer from the tools they already run.

Output is DETERMINISTIC: categories sorted alphabetically, connectors sorted
by slug, questions kept in page.json order with exact-string dedupe per
connector. No timestamps. verify_all.sh regenerates the page and diffs it
against the committed copy (same drift gate as the catalog), so a stale
question book fails CI.

The title promises "200+" questions; the script fails the build if the
aggregate ever drops below MIN_QUESTIONS so the page can never overclaim.

Pure stdlib. Run locally:  python3 tools/maintainer/build-question-book.py
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
OUT = ROOT / "docs" / "question-book.md"

MIN_QUESTIONS = 200  # the title says "200+" - never ship fewer


def connector_questions() -> dict[str, list[tuple[str, dict, list[str]]]]:
    """{category: [(slug, meta, questions)]}, deterministic ordering."""
    by_cat: dict[str, list[tuple[str, dict, list[str]]]] = {}
    for slug, meta in registry.skills().items():  # slug-sorted
        if meta.get("markdown_only"):
            continue
        page_path = registry.skill_path(slug) / "page.json"
        if not page_path.exists():
            raise SystemExit(
                f"build-question-book: skills/{meta.get('source_dir', slug)}/"
                "page.json missing - every connector needs one."
            )
        pj = json.loads(page_path.read_text(encoding="utf-8"))
        questions: list[str] = []
        seen: set[str] = set()
        for item in pj.get("instead_of_say", []):
            q = (item.get("say") or "").strip()
            if q and q not in seen:
                seen.add(q)
                questions.append(q)
        for o in pj.get("outcomes", []):
            q = (o.get("question") or "").strip()
            if q and q not in seen:
                seen.add(q)
                questions.append(q)
        if not questions:
            raise SystemExit(f"build-question-book: '{slug}' has no questions.")
        cat = meta.get("category") or "Other"
        by_cat.setdefault(cat, []).append((slug, meta, questions))
    return by_cat


def render(by_cat: dict[str, list[tuple[str, dict, list[str]]]]) -> str:
    total = sum(len(qs) for conns in by_cat.values() for _, _, qs in conns)
    n_conn = sum(len(conns) for conns in by_cat.values())
    if total < MIN_QUESTIONS:
        raise SystemExit(
            f"build-question-book: only {total} questions aggregated; the page "
            f"promises {MIN_QUESTIONS}+. Add connectors or fix page.json data."
        )

    lines = [
        "---",
        "layout: default",
        'title: "The MSP AI Question Book - 200+ plain-English questions your '
        'AI can answer"',
        f'description: "{total} plain-English questions your AI can answer '
        "from the PSA, RMM, backup, security, and billing tools you already "
        f'run - aggregated from all {n_conn} free MSP Skills connectors. No '
        'code required, runs locally."',
        "permalink: /question-book/",
        "---",
        "",
        "# The MSP AI Question Book",
        "",
        f"{total} plain-English questions MSP owners ask their AI every day - "
        f"aggregated from the {n_conn} free, open source connectors on this "
        "site and grouped by tool category. Every question maps to a real, "
        "gate-verified command your AI agent runs against your PSA, RMM, "
        "backup, security, or billing stack. It runs on your own machine, "
        "and your data stays local unless you route it somewhere "
        "yourself. Find your tools, steal the questions.",
        "",
        "Each connector installs in about 60 seconds and works with Claude, "
        "ChatGPT, Copilot, and any MCP-capable agent. New to the term? "
        "[What an MCP server is →](/what-is-an-mcp-server/)",
        "",
    ]

    for cat in sorted(by_cat):
        lines.append(f"## {cat}")
        lines.append("")
        for slug, meta, questions in by_cat[cat]:
            display = meta.get("display_name") or meta.get("vendor", slug)
            lines.append(f"### {display}")
            lines.append("")
            lines.append(
                f"Free {display} MCP server: [install and ask →](/skills/{slug}/)"
            )
            lines.append("")
            for q in questions:
                lines.append(f"- {q}")
            lines.append("")

    lines += [
        "---",
        "",
        "Want a question answered live? **[Build Sessions]"
        "(https://compoundingteams.com/build-sessions)** are free every "
        "Thursday - bring one of these questions and your own tenant.",
        "",
    ]
    return "\n".join(lines)


def main() -> int:
    by_cat = connector_questions()
    OUT.write_text(render(by_cat), encoding="utf-8")
    total = sum(len(qs) for conns in by_cat.values() for _, _, qs in conns)
    print(f"Wrote {OUT.relative_to(ROOT)} ({total} questions, "
          f"{sum(len(c) for c in by_cat.values())} connectors, "
          f"{len(by_cat)} categories).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
