#!/usr/bin/env python3
"""Render docs/skills/<slug>.md (the Jekyll site page) from page.json + the
registry entry in tools/maintainer/skills.json.

This is the single, repo-side renderer for skill pages. It was ported verbatim
from msp-skills-publish's onboard.py (render_docs_skill_page) so that EVERY
regeneration path - the publish skill's scaffold, a live-verified badge flip,
the catalog CI job - produces the same bytes from the same inputs. Before this
module existed, a verify_live.py flip updated the catalog + README badge but
left the docs page badge stale, because only onboard.py knew how to render it.

Fully deterministic and re-renderable: ALL prose lives in page.json; this
module owns only structure. TODOs in page.json flow into the page and are
blocked by check_no_todos until the agent fills them.

Usage:
    python3 tools/maintainer/render_docs_page.py             # all connector skills
    python3 tools/maintainer/render_docs_page.py halopsa     # one slug

Pure stdlib. Invoked automatically by build-catalog.py.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT

# Owner/repo come from the registry so generated pages stay correct on a fork.
# OWNER_TITLE is the GitHub org's canonical display case (Servosity).
OWNER, REPO = registry.owner_repo()
OWNER_TITLE = OWNER[:1].upper() + OWNER[1:]


def banner(vendor: str, owner: str, first_party: bool) -> str:
    if first_party:
        return (
            f"> Published by {vendor} Inc. for MSP partners. {vendor} is a trademark of\n"
            f"> {owner}. Apache-2.0 licensed."
        )
    return (
        f"> Unofficial. Community-built Claude Code Skill and MCP server for the {vendor}\n"
        f"> API. Not affiliated with, endorsed by, or sponsored by {owner}."
    )


def render_page(slug: str) -> Path:
    """Render one skill's docs page and return the output path."""
    entry = registry.skills().get(slug)
    if entry is None:
        raise SystemExit(f"render_docs_page: unknown slug '{slug}'")
    if entry.get("markdown_only"):
        raise SystemExit(
            f"render_docs_page: '{slug}' is markdown-only and has no docs skill page"
        )
    skill_dir = registry.skill_path(slug)

    page_path = skill_dir / "page.json"
    if not page_path.exists():
        raise SystemExit(
            f"render_docs_page: {page_path} missing - every connector skill "
            "carries a page.json (the publish skill scaffolds a stub)"
        )
    page = json.loads(page_path.read_text())
    vendor = entry.get("vendor", slug)
    display = entry.get("display_name", vendor)
    owner = entry.get("vendor_trademark_owner", vendor)
    first_party = bool(entry.get("first_party"))
    cli_bin = entry.get("cli_binary", f"{slug}-cli")
    desc = ""
    mf = skill_dir / "manifest.json"
    if mf.exists():
        desc = json.loads(mf.read_text()).get("description", "")
    lv = entry.get("live_verified") or {}
    if lv.get("status") == "live-verified":
        badge = (f"**Live-verified** - confirmed by a real MSP against a live "
                 f"{display} tenant ({lv.get('date', '')}).")
    else:
        badge = ("**Awaiting live verification** - passes every mechanical gate "
                 "(build, command-surface, claims, install). Be the first to confirm "
                 "it against your tenant: "
                 f"[report it works](https://github.com/{OWNER_TITLE}/{REPO}/issues/new?template=it-works.yml).")

    fm_faqs = "".join(
        f"  - q: {json.dumps(f['q'])}\n    a: {json.dumps(f['a'])}\n"
        for f in page.get("faqs", [])
    )
    fm_howto = (
        f'  - name: "Run the one-line installer"\n'
        f'    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/{OWNER_TITLE}/{REPO}/main/skills/{slug}/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/{OWNER_TITLE}/{REPO}/main/skills/{slug}/install.ps1 | iex"\n'
        f'  - name: "Authenticate"\n'
        f'    text: "Enter your {display} credentials once; {cli_bin} doctor confirms they work."\n'
        f'  - name: "Ask your first question"\n'
        f'    text: "Ask your AI agent a {display} question in plain language; it runs {cli_bin} for you."\n'
    )

    insteads = "\n".join(
        f"**Instead of** {i['instead']}\n"
        f"**just ask:** *\"{i['say']}\"*\n"
        + (f"<sub>Your agent runs: <code>{i['cli']}</code></sub>\n" if i.get("cli") else "")
        for i in page.get("instead_of_say", [])
    )
    video_section = ""
    if entry.get("has_video"):
        video_section = f"""
## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/{slug}/wide-1200x630.png" src="/assets/video/{slug}/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/{slug}/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>
"""
    outcomes_rows = "\n".join(
        f"| {o['question']} | `{o['command']}` |" for o in page.get("outcomes", [])
    )
    pains = "\n".join(f"- {p}" for p in page.get("pain_points", []))
    faq_md = "\n".join(f"### {f['q']}\n\n{f['a']}\n" for f in page.get("faqs", []))
    tiers = "\n".join(
        f"| {t['tier']} | {t['examples']} | {t['policy']} |"
        for t in page.get("safety_tiers", [])
    )

    text = f"""---
layout: default
title: "{display} MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: {json.dumps(desc or f"Free Claude Code Skill and MCP server for {display}. Built for MSP owners.")}
permalink: /skills/{slug}/
skill_name: "{display} MCP"
image: /assets/social/{slug}/wide-1200x630.png
verification: {lv.get('status', 'awaiting')}
faqs:
{fm_faqs}howto:
{fm_howto}---

# {display} + AI in 60 seconds

{banner(vendor, owner, first_party)}

{badge}

{page.get('outcome_intro', '')}

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){{: .btn .btn-primary}} &nbsp; [View on GitHub →](https://github.com/{OWNER}/{REPO}/tree/main/skills/{slug}){{: .btn}}

## Instead of clicking through {display}, just ask

{insteads}
{video_section}
## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
{outcomes_rows}

Full command reference at [github.com/{OWNER}/{REPO}/blob/main/skills/{slug}/guide.md](https://github.com/{OWNER}/{REPO}/blob/main/skills/{slug}/guide.md).

## What makes this one different

{page.get('different_vs_wrappers', '')}

{page.get('different_vs_vendor', '')}

## The pain this closes

{pains}

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](/integrations/claude-desktop/) |
| **ChatGPT** (Plus/Pro+) | [Step-by-step →](/integrations/chatgpt/) |
| **Claude Code** | [Step-by-step →](/integrations/claude-code/) |
| **Codex CLI** | [Step-by-step →](/integrations/codex/) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](/which-agent/) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/{OWNER}/{REPO}/main/skills/{slug}/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/{OWNER}/{REPO}/main/skills/{slug}/install.ps1 | iex
```

{page.get('auth_note', f'After install, authenticate once with your {display} credentials, then verify with `{cli_bin} --version`.')}

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
{tiers}

{page.get('governance_summary', '')} Full details in [governance.md](https://github.com/{OWNER}/{REPO}/blob/main/skills/{slug}/governance.md).

## Frequently asked questions

{faq_md}

## Status

Beta. Validated against the {display} API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
"""
    out = ROOT / "docs" / "skills" / f"{slug}.md"
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(text)
    return out


def connector_slugs() -> list[str]:
    return [s for s, m in registry.skills().items() if not m.get("markdown_only")]


def main(argv: list[str]) -> int:
    slugs = argv or connector_slugs()
    for slug in slugs:
        out = render_page(slug)
        print(f"Rendered {out.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
