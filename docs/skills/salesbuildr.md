---
layout: default
title: "Salesbuildr MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Every Salesbuildr resource as a scriptable command, plus offline margin and pipeline analytics."
permalink: /skills/salesbuildr/
skill_name: "Salesbuildr MCP"
image: /assets/social/salesbuildr/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Salesbuildr MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Salesbuildr data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hammer my Salesbuildr API rate limits?"
    a: "No. The analytics read a local SQLite mirror, not the live API - you sync once, then ask as many questions as you want offline with zero further API calls. The sync itself honors a configurable --rate-limit, and you control how often it runs."
  - q: "Do I need to be a Salesbuildr customer, and will this replace my PSA sync?"
    a: "You need a Salesbuildr account, a Public API key from your portal, and your tenant subdomain. It does not replace your Autotask/ConnectWise sync - it reads your Salesbuildr data and flags the records missing the external ID that sync depends on, so you can fix the gaps. It never writes to your PSA."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/salesbuildr/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/salesbuildr/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Salesbuildr credentials once; salesbuildr-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Salesbuildr question in plain language; it runs salesbuildr-cli for you."
---

# Salesbuildr + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the Salesbuildr
> API. Not affiliated with, endorsed by, or sponsored by Salesbuildr.

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

Ask your AI which quotes are aging, which lines are bleeding margin, and where your pipeline is stalling - and get the answer from your own Salesbuildr data in seconds. salesbuildr-cli syncs your quotes, opportunities, products, and pricing books into a local mirror, so the cross-quote questions the portal can't answer in one click become a single command.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/salesbuildr){: .btn}

## Instead of clicking through Salesbuildr, just ask

**Instead of** Export the quotes report to a spreadsheet and eyeball which sent proposals have gone cold.
**just ask:** *"Which quotes have been sitting sent or approved for two weeks, and how much revenue is at risk?"*
<sub>Your agent runs: <code>salesbuildr-cli quote stale --days 14</code></sub>

**Instead of** Open every open quote and check each line's markup by hand to find the ones below floor.
**just ask:** *"Show me every quote line priced under 20% markup."*
<sub>Your agent runs: <code>salesbuildr-cli quote thin --floor 20</code></sub>

**Instead of** Pull a CSV from Salesbuildr and a CSV from the PSA, then VLOOKUP to find what never synced.
**just ask:** *"Which companies, contacts, and products are missing the external ID my PSA sync needs?"*
<sub>Your agent runs: <code>salesbuildr-cli reconcile-psa</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/salesbuildr/wide-1200x630.png" src="/assets/video/salesbuildr/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/salesbuildr/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which sent or approved quotes are aging, and how much is at risk? | `salesbuildr-cli quote stale --days 14` |
| Which quote line items are priced below my markup floor? | `salesbuildr-cli quote thin --floor 20` |
| How does my quote pipeline convert stage by stage, in count and dollars? | `salesbuildr-cli quote funnel` |
| Where have per-company pricing-book prices drifted from the master catalog? | `salesbuildr-cli pricing drift` |
| What's my win rate by owner, stage, or category? | `salesbuildr-cli opportunity winrate --by owner` |
| What's my probability-weighted recurring-revenue forecast on the open pipeline? | `salesbuildr-cli opportunity mrr-forecast` |
| Which catalog products have I never quoted to a given company? | `salesbuildr-cli company whitespace "Acme Managed IT"` |
| Which records are missing the external ID my PSA sync depends on? | `salesbuildr-cli reconcile-psa` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/salesbuildr/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/salesbuildr/guide.md).

## What makes this one different

Most Salesbuildr integrations proxy each question into a live API call - fine for one record, useless for "across every open quote." This skill syncs Salesbuildr into a local SQLite mirror with full-text search, so margin, pipeline, and pricing-drift questions resolve as one local join: instant, offline, and the AI sees the answer, not your raw catalog.

Salesbuildr's portal shows you one quote, one opportunity, one pricing book at a time. This skill answers the cross-entity questions - every stale quote, every thin-margin line, every company you've never cross-sold - that the portal makes you assemble by hand. It complements the portal; it doesn't replace it.

## The pain this closes

- Quotes go out and then go quiet. On a lean MSP nobody has time to babysit every sent proposal, so deals worth real money quietly age past their expiry - a recurring r/msp complaint about quote follow-up.
- Margin leaks one line at a time. A tech swaps in a part at cost and forgets the markup; you don't catch it until the QBR, and by then it has already shipped.
- Per-company pricing books drift away from the master catalog, so two clients pay different prices for the same SKU and nobody decided that on purpose.
- Records that never got an external ID silently fall out of the Autotask/ConnectWise sync, so your PSA and your quoting tool quietly disagree about who your customers are.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/salesbuildr/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/salesbuildr/install.ps1 | iex
```

After install, authenticate once with your Salesbuildr credentials, then verify with `salesbuildr-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | every `public-get` / `public-get-list`, the analytics (`quote stale`, `quote thin`, `quote funnel`, `pricing drift`, `opportunity velocity` / `winrate` / `mrr-forecast`, `product velocity`, `company whitespace`), `reconcile-psa`, `search`, `sql`, `export`, and `sync` (local mirror only) | Allow |
| Write (routine) | company / contact / product / pricing-book / quote / quote-template create & update, the upserts, `opportunity public-win` / `public-lose` / `public-upsert`, `field public-update-values`, `product inventory`, the contact undeletes, and the bulk `import` - 30 routine-write commands in all | Preview with --dry-run, then a reviewed write |
| Destructive / config | the 6 delete commands (`company` / `contact` / `product public-delete*`), plus the local credential commands (`auth set-token`, `auth setup`, `auth logout`) | Human-in-the-loop only |

The skill reads your Salesbuildr quotes, opportunities, products, companies, and pricing books, and can create, update, or delete those records through the Public API. Reads - every get/list, the analytics rollups, search, sql, export, and sync - are safe to run unattended. Routine writes (create/update/upsert, the bulk import, winning or losing an opportunity) should be previewed with --dry-run and approved. The delete commands and the local auth/credential commands are human-in-the-loop only. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/salesbuildr/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Salesbuildr MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Salesbuildr data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hammer my Salesbuildr API rate limits?

No. The analytics read a local SQLite mirror, not the live API - you sync once, then ask as many questions as you want offline with zero further API calls. The sync itself honors a configurable --rate-limit, and you control how often it runs.

### Do I need to be a Salesbuildr customer, and will this replace my PSA sync?

You need a Salesbuildr account, a Public API key from your portal, and your tenant subdomain. It does not replace your Autotask/ConnectWise sync - it reads your Salesbuildr data and flags the records missing the external ID that sync depends on, so you can fix the gaps. It never writes to your PSA.


## Status

Beta. Validated against the Salesbuildr API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
