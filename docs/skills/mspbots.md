---
layout: default
title: "MSPbots MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "The first MSPbots tool anywhere \u2014 readable filters, alias-named resources, full exports, and the KPI history MSPbots itself doesn't keep."
permalink: /skills/mspbots/
skill_name: "MSPbots MCP"
image: /assets/social/mspbots/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local MSPbots MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my MSPbots data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local snapshot store are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "What credentials do I need?"
    a: "An MSPbots API key: an admin creates it at Settings > Public API in the MSPbots app and binds each dataset or widget to it. Set MSPBOTS_API_KEY in your environment or run mspbots-cli auth set-token. The binding is the permission boundary - the key can only read resources explicitly bound to it, and the global Enable Public API toggle gates everything."
  - q: "Can this write to MSPbots?"
    a: "No. The MSPbots Public API is read-only - datasets and widgets out, nothing in. The only things this skill writes are local: alias registrations, snapshots, and the sync cache in your own SQLite store. No command can change anything in your MSPbots tenant."
  - q: "Why do I register datasets before pulling them?"
    a: "The Public API has no list endpoint - it cannot tell you what is bound to your key. You copy each resource ID from Settings > Public API once, register it with mspbots-cli registry add open_tickets <resourceId>, and every other command accepts the alias from then on."
  - q: "Will this hit MSPbots rate limits?"
    a: "The API documents rate limits without publishing numbers. The CLI ships a --rate-limit throttle, export bounds its page-walking with --max-pages and reports when the cap was hit, and the history questions (trend, diff) run entirely against local snapshots - zero API calls after capture."
  - q: "Does it support widgets as well as datasets?"
    a: "Yes - pull, export, snapshot, describe, and registry add all accept --type widget. One documented exception inherited from the API: widgets with measure or calculate layers are not supported by the Public API itself."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mspbots/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mspbots/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your MSPbots credentials once; mspbots-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a MSPbots question in plain language; it runs mspbots-cli for you."
---

# MSPbots + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the MSPbots
> API. Not affiliated with, endorsed by, or sponsored by MSPbots, Inc..

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

MSPbots aggregates your PSA, RMM, and finance data into KPI dashboards - then keeps it there. Ask your AI "is our ticket backlog up or down this week" and get the answer the dashboard can't give: point-in-time snapshots, week-over-week deltas, row-level diffs, and full CSV exports - through readable aliases and filters instead of 19-digit resource IDs and a comma-encoded query DSL.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/mspbots){: .btn}

## Instead of clicking through MSPbots, just ask

**Instead of** Screenshotting the open_tickets widget every Monday and hand-maintaining a week-over-week column in a spreadsheet, because the dashboard only shows now
**just ask:** *"Is our ticket backlog up or down versus last week?"*
<sub>Your agent runs: <code>mspbots-cli trend open_tickets --agg count</code></sub>

**Instead of** Hand-encoding filters into URL query params - price=12.6,56.3 for a range, a trailing comma for on-or-after - one undocumented comma rule per operator
**just ask:** *"Pull the open tickets updated since June 1"*
<sub>Your agent runs: <code>mspbots-cli pull open_tickets --where "Update Date >= 2026-06-01"</code></sub>

**Instead of** Paging a dataset 50 rows at a time and stitching the pages together by hand to get the full table into a spreadsheet
**just ask:** *"Export the whole tickets dataset to CSV for the QBR deck"*
<sub>Your agent runs: <code>mspbots-cli export open_tickets --format csv</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/mspbots/wide-1200x630.png" src="/assets/video/mspbots/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/mspbots/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Is our open-ticket backlog up or down versus last week? | `mspbots-cli trend open_tickets --agg count` |
| What changed in open tickets since the last snapshot - rows added, removed, or edited? | `mspbots-cli diff open_tickets` |
| Pull the open tickets updated since June 1 | `mspbots-cli pull open_tickets --where "Update Date >= 2026-06-01"` |
| What columns does this dataset have, and what types? | `mspbots-cli describe open_tickets` |
| Export the entire dataset to CSV for the QBR deck | `mspbots-cli export open_tickets --format csv` |
| Capture today's KPI snapshot (schedule it and history accrues) | `mspbots-cli snapshot open_tickets` |
| Stop pasting 19-digit IDs - name the dataset once | `mspbots-cli registry add open_tickets 1534956341424005122` |
| Are my API key and resource bindings working? | `mspbots-cli doctor` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/mspbots/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/mspbots/guide.md).

## What makes this one different

A thin wrapper on the MSPbots Public API inherits everything painful about it: two endpoints, 19-digit IDs, comma-encoded filters, no enumeration, no history. This skill adds the missing layers locally - an alias registry so resources have names, readable --where predicates compiled into the wire DSL, auto-paginated exports, and timestamped snapshots in a local SQLite store that diff and trend turn into the KPI history the API cannot return.

MSPbots' own AI and bots act inside its dashboards and workflows. This skill complements them: it brings the data those dashboards hold out to whichever agent you already use, and keeps the local snapshot history that lets your agent answer versus-last-week questions the Public API has no endpoint for.

## The pain this closes

- The MSPbots Public API is two endpoints with no enumeration: per the official API documentation (the wiki.mspbots.ai Public API article, archived April 2024), resource IDs are 19-digit identifiers you copy out of Settings > Public API one at a time - no list endpoint, no metadata endpoint, and as of June 2026 no published CLI, SDK, or MCP server for it that we could find.
- The API and the dashboards both show now - there is no history endpoint. The week-over-week KPI column every ops manager wants (backlog trend, SLA drift) is hand-maintained in a spreadsheet from screenshots, and it dies the week someone forgets to update it.
- Filters are a comma-encoded DSL keyed by column name - price=12.6,56.3 is a range, a trailing comma means on-or-after - and the same official docs list rate limits with unspecified numbers and intermittent HTTP 502 responses on heavy widget fetches.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/mspbots/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/mspbots/install.ps1 | iex
```

After install, authenticate once with your MSPbots credentials, then verify with `mspbots-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read (live API) | mspbots-cli pull open_tickets --where "Status = Open"; mspbots-cli export open_tickets --format csv; mspbots-cli describe open_tickets; mspbots-cli dataset <resourceId>; mspbots-cli widget <resourceId>; mspbots-cli doctor | Allow |
| Local-only writes (never touch the tenant) | mspbots-cli registry add open_tickets <resourceId>; mspbots-cli snapshot open_tickets; mspbots-cli sync - all writes land in your local SQLite store; the API has no write endpoint | Allow - safe to schedule |
| Credentials / local config | mspbots-cli auth set-token; mspbots-cli auth logout; mspbots-cli registry rm (removes a local alias) | Human-in-the-loop only |

The skill drives the mspbots-cli and mspbots-mcp binaries, authenticating with an API key read from MSPBOTS_API_KEY or saved locally via auth set-token - never logged and never sent anywhere except the MSPbots API. The Public API is read-only, so no command can change anything in your MSPbots tenant; the only writes are local (alias registry, snapshot store, sync cache in your own SQLite file). The permission boundary is which datasets and widgets an admin binds to the key. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/mspbots/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local MSPbots MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my MSPbots data safe?

Your data stays on your machine. The CLI, MCP server, and the local snapshot store are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What credentials do I need?

An MSPbots API key: an admin creates it at Settings > Public API in the MSPbots app and binds each dataset or widget to it. Set MSPBOTS_API_KEY in your environment or run mspbots-cli auth set-token. The binding is the permission boundary - the key can only read resources explicitly bound to it, and the global Enable Public API toggle gates everything.

### Can this write to MSPbots?

No. The MSPbots Public API is read-only - datasets and widgets out, nothing in. The only things this skill writes are local: alias registrations, snapshots, and the sync cache in your own SQLite store. No command can change anything in your MSPbots tenant.

### Why do I register datasets before pulling them?

The Public API has no list endpoint - it cannot tell you what is bound to your key. You copy each resource ID from Settings > Public API once, register it with mspbots-cli registry add open_tickets <resourceId>, and every other command accepts the alias from then on.

### Will this hit MSPbots rate limits?

The API documents rate limits without publishing numbers. The CLI ships a --rate-limit throttle, export bounds its page-walking with --max-pages and reports when the cap was hit, and the history questions (trend, diff) run entirely against local snapshots - zero API calls after capture.

### Does it support widgets as well as datasets?

Yes - pull, export, snapshot, describe, and registry add all accept --type widget. One documented exception inherited from the API: widgets with measure or calculate layers are not supported by the Public API itself.


## Status

Beta. Validated against the MSPbots API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
