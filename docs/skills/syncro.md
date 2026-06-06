---
layout: default
title: "Syncro MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Every Syncro PSA and RMM workflow in your terminal, plus a local database, offline search, and cross-entity reports no other Syncro tool has."
permalink: /skills/syncro/
skill_name: "Syncro MCP"
image: /assets/social/syncro/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Syncro MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Syncro data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Syncro API rate limits?"
    a: "Day-to-day questions read from the local SQLite mirror, not the live API, so they don't touch your rate limit at all. Only `sync` and live reads call Syncro, and the CLI has a built-in `--rate-limit` flag plus response caching to stay polite."
  - q: "Do I need to be a Syncro partner or on a specific plan?"
    a: "You just need a Syncro account and an API token from your own portal (Admin area). It authenticates as you, against your subdomain, with whatever permissions that token is scoped to - no partner program or special tier required."
  - q: "Will this replace my Syncro portal?"
    a: "No. It reads from your Syncro account and is for the cross-customer questions and bulk analysis the portal makes tedious. You still run tickets, billing, and RMM day-to-day in Syncro; this is the fast lane for the questions an owner keeps re-asking."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/syncro/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/syncro/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Syncro credentials once; syncro-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Syncro question in plain language; it runs syncro-cli for you."
---

# Syncro + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the Syncro
> API. Not affiliated with, endorsed by, or sponsored by Servably, Inc..

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

Ask your AI "which customers have logged time we never invoiced?" and get the unbilled hours ranked by customer in seconds - no report exports, no portal clicking. Syncro plus your agent answers billing-leakage, AR-aging, SLA-breach, and patch-gap questions across every customer at once, from one local mirror of your Syncro PSA and RMM data. Free, open source, runs on your laptop.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/syncro){: .btn}

## Instead of clicking through Syncro, just ask

**Instead of** Export the time-entries report, pivot it against invoices in a spreadsheet, and hunt for hours nobody ever billed
**just ask:** *"Which customers have logged labor we never invoiced?"*
<sub>Your agent runs: <code>syncro-cli billing uninvoiced</code></sub>

**Instead of** Click through every overdue invoice to work out how bad AR really is this month
**just ask:** *"Bucket our unpaid invoices into aging tiers"*
<sub>Your agent runs: <code>syncro-cli billing ar-aging</code></sub>

**Instead of** Pull a customer's open tickets, assets, AR balance, and RMM alerts from four different screens before a QBR
**just ask:** *"Give me a full snapshot for customer 12345"*
<sub>Your agent runs: <code>syncro-cli customers profile 12345</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/syncro/wide-1200x630.png" src="/assets/video/syncro/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/syncro/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which customers have logged time we never invoiced? | `syncro-cli billing uninvoiced` |
| Which closed tickets had billable time that was never invoiced? | `syncro-cli billing drift` |
| How is our unpaid AR aging (0-30/30-60/60-90/90+)? | `syncro-cli billing ar-aging` |
| What is our revenue per labor hour by customer this quarter? | `syncro-cli customers margin` |
| Which open tickets are going stale with no recent activity? | `syncro-cli tickets aging` |
| Which assets are missing the most critical patches? | `syncro-cli assets patch-gaps` |
| Which customers generate the most RMM alert noise? | `syncro-cli alerts noise` |
| Which RMM alerts never became a ticket? | `syncro-cli alerts orphans` |
| Give me one cross-entity card for a single customer. | `syncro-cli customers profile 12345` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/syncro/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/syncro/guide.md).

## What makes this one different

Most Syncro integrations and MCP servers proxy each question into a single live API call - fine for one record, useless for 'across every customer, last quarter.' This skill syncs Syncro into a local SQLite mirror with full-text search, so cross-customer billing, SLA, and patch questions become one offline SQL join: instant, and the AI sees the answer, not your raw bulk data.

It complements the Syncro portal rather than replacing it. The portal shows you one record or one canned report at a time; this skill answers the aggregate questions the portal makes you export and pivot - logged-but-unbilled labor ranked by customer, AR aging, revenue-per-hour - and hands the result straight to whichever AI agent you already use.

## The pain this closes

- Time logged on tickets quietly never makes it onto an invoice - billing leakage that the time-entries screen never surfaces because nothing ranks logged-but-unbilled hours by customer.
- Cross-customer questions ('which tickets are going stale?', 'which assets are missing patches?', 'how noisy is each client's RMM?') mean exporting reports and pivoting spreadsheets, so they get asked at QBR time instead of every week.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/syncro/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/syncro/install.ps1 | iex
```

After install, authenticate once with your Syncro credentials, then verify with `syncro-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | billing uninvoiced, billing ar-aging, customers margin, tickets aging, assets patch-gaps, alerts noise, customers profile, and every list/get/search/export command | Allow |
| Write (routine) | tickets create, tickets comment create, tickets update, invoices create, customers update, appointments create | Preview with --dry-run, then a reviewed write |
| Destructive / config | tickets delete, invoices delete, customers delete, contracts delete, and the other delete commands | Human-in-the-loop only |

The skill is read-first: reporting, rollups, and cross-entity views can't change anything in Syncro. Mutating commands (create/update/delete tickets, invoices, customers, and the like) send immediately unless you pass `--dry-run` to preview first, so the safe agent policy is read plus previewed writes, with a human approving anything that creates, updates, or deletes. The CLI can only ever do what the API token you supply is scoped to do. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/syncro/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Syncro MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Syncro data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Syncro API rate limits?

Day-to-day questions read from the local SQLite mirror, not the live API, so they don't touch your rate limit at all. Only `sync` and live reads call Syncro, and the CLI has a built-in `--rate-limit` flag plus response caching to stay polite.

### Do I need to be a Syncro partner or on a specific plan?

You just need a Syncro account and an API token from your own portal (Admin area). It authenticates as you, against your subdomain, with whatever permissions that token is scoped to - no partner program or special tier required.

### Will this replace my Syncro portal?

No. It reads from your Syncro account and is for the cross-customer questions and bulk analysis the portal makes tedious. You still run tickets, billing, and RMM day-to-day in Syncro; this is the fast lane for the questions an owner keeps re-asking.


## Status

Beta. Validated against the Syncro API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
