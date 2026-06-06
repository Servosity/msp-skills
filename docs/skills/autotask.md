---
layout: default
title: "Autotask PSA MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Every Autotask entity at the command line, plus a local SQLite mirror that answers ticket-aging, workload, unbilled-time, and account-360 questions no other Autotask tool can."
permalink: /skills/autotask/
skill_name: "Autotask PSA MCP"
image: /assets/social/autotask/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Autotask MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Autotask data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "What credentials do I need?"
    a: "An API-only user created in your own Autotask instance, which gives you a UserName and Secret, plus the tracking Integration Code from your integration vendor record. Set AUTOTASK_PSA_USER_NAME, AUTOTASK_PSA_SECRET, and AUTOTASK_PSA_API_INTEGRATION_CODE in your environment. The API user's security level is the real permission boundary - scope it to exactly what you want the AI to reach."
  - q: "Do I have to figure out my Autotask zone URL?"
    a: "No. Autotask hosts each tenant in a numbered zone (webservicesN.autotask.net). Run autotask-cli zone once and the CLI discovers and caches your tenant's base URL, the same way Autotask's own docs tell integrations to make a zone-information request first."
  - q: "Will this blow through my Autotask API limits?"
    a: "Autotask meters API calls per database in a rolling 60-minute window and adds latency as you approach the threshold. The local mirror exists so reads stop hitting the API: after the first sync, the cross-entity views (unbilled, reconcile, retainer, company-360, ticket-aging, triage, workload) run against local SQLite with zero API calls, and sync is incremental - it only fetches what changed since the last checkpoint."
  - q: "How do I deal with status and priority being numbers, not labels?"
    a: "Run autotask-cli picklist \"Tickets\" \"status\" (or any entity and field) and it prints the label-to-ID map from cached field metadata, so you can read reports and build filters without memorizing the integer IDs Autotask stores categorical fields as."
  - q: "Is this Datto Autotask PSA or just Autotask?"
    a: "Same product. Autotask PSA is now branded Datto Autotask PSA under Kaseya; this skill targets the Autotask REST API, which both names refer to. It does not cover Datto RMM - that is a separate product with a separate API."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/autotask/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/autotask/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Autotask PSA credentials once; autotask-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Autotask PSA question in plain language; it runs autotask-cli for you."
---

# Autotask PSA + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the Autotask
> API. Not affiliated with, endorsed by, or sponsored by Datto, Inc.

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

MSPs run Autotask PSA as the system of record - tickets, time, contracts, billing. Ask your AI "what's gone stale on the service desk," "which approved time hasn't been invoiced," or "how burned is that block-hours contract," and get answers the portal can't compose: cross-entity rollups across tickets, time, contracts, and resources, computed offline from a local mirror in one query instead of a LiveReport and an Excel export.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/autotask){: .btn}

## Instead of clicking through Autotask PSA, just ask

**Instead of** Building a LiveReport or exporting closed tickets and time entries to Excel before the invoice run to find which approved hours never made it onto an invoice
**just ask:** *"Which approved time haven't we invoiced yet?"*
<sub>Your agent runs: <code>autotask-cli unbilled</code></sub>

**Instead of** Sorting the ticket grid by last activity and eyeballing which tickets have sat untouched while the SLA clock runs
**just ask:** *"What's gone stale on the service desk in the last week?"*
<sub>Your agent runs: <code>autotask-cli stale --days 7</code></sub>

**Instead of** Clicking through tickets, contacts, contracts, configuration items, and opportunities across separate Autotask screens to prep one client review
**just ask:** *"Give me the full picture on company 1234 before my 2 o'clock"*
<sub>Your agent runs: <code>autotask-cli company-360 "1234"</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/autotask/wide-1200x630.png" src="/assets/video/autotask/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/autotask/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which approved time entries haven't been invoiced yet? | `autotask-cli unbilled` |
| How stale is the service desk right now, bucketed by age? | `autotask-cli ticket-aging` |
| Which open tickets has nobody touched in a week? | `autotask-cli stale --days 7` |
| Which unassigned tickets should the dispatcher pick up first? | `autotask-cli triage` |
| Who's overloaded before I assign the next ticket? | `autotask-cli workload` |
| How burned are our block-hour contracts, and when do they run out? | `autotask-cli retainer` |
| Everything we know about one company - tickets, contacts, contracts, config items, opportunities? | `autotask-cli company-360 "1234"` |
| What's the month-end billing picture - unbilled time, contract burn, money on the table? | `autotask-cli reconcile` |
| What's the label-to-ID map for the ticket status picklist? | `autotask-cli picklist "Tickets" "status"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/autotask/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/autotask/guide.md).

## What makes this one different

Most Autotask integrations and MCP servers proxy every question into a live API call - fine for one ticket, but an aggregate question becomes a multi-call dance against a per-tenant zone URL, each categorical filter gated by integer picklist IDs that vary per instance. This skill syncs the PSA into a local SQLite mirror with full-text search, so cross-entity questions - unbilled time, contract burn, account 360 - become one local join: instant, offline, and the AI sees the answer, not pages of raw JSON. Reads run against the mirror, so they stop spending your hourly API request budget.

It complements Autotask's own reporting rather than replacing it: LiveReports and the portal stay best for in-app workflows and scheduled documents, while this skill brings the PSA to whichever agent you already use and answers the cross-object questions - tickets joined to time joined to contracts - that no single portal screen or LiveReport composes without an export.

## The pain this closes

- Unlogged and uninvoiced time is direct revenue leakage: techs close tickets without entering time, or approved hours never make it onto an invoice, and nobody notices until the billing run - because Autotask has no single screen that joins approved time entries to the invoices they belong on. Finding the gap means a LiveReport plus an Excel pass.
- Cross-object reporting means an export. Autotask's native LiveReports is a copy-and-edit report designer that outputs to Excel, PDF, RTF, or CSV (per Kaseya's own Autotask PSA reporting help), so answering "how burned is this contract" or "what is this client's full footprint" across tickets, time, contracts, and opportunities is commonly pushed into Power BI or another BI tool - an entire third-party connector market exists just to pull Autotask data into a unified model.
- The REST API is friction at the edges: it authenticates with three static headers (UserName, Secret, and an ApiIntegrationCode) against a per-tenant zone URL you must discover first, and categorical fields like status, priority, and queue are integer picklist IDs that vary per instance - so even a simple filter means resolving a label to its numeric ID before you can ask the question (per Autotask's REST API developer docs).

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/autotask/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/autotask/install.ps1 | iex
```

After install, authenticate once with your Autotask PSA credentials, then verify with `autotask-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | autotask-cli triage; autotask-cli ticket-aging; autotask-cli unbilled; autotask-cli stale --days 7; autotask-cli company-360 "1234"; autotask-cli retainer; autotask-cli workload; autotask-cli search "vpn outage"; autotask-cli tickets query | Allow |
| Write (routine) | autotask-cli tickets create-entity (open a ticket); autotask-cli tickets patch-entity (update status or owner); autotask-cli time-entries create-entity (log time); autotask-cli companies update-entity - writes send immediately; --dry-run is an opt-in preview, not a default | Preview with --dry-run, then a reviewed write |
| Destructive / config | autotask-cli time-entries delete-entity; autotask-cli service-calls delete-entity "1234"; autotask-cli contact-groups delete-entity "1234" | Human-in-the-loop only |

The skill drives the autotask-cli and autotask-mcp binaries, authenticating with an Autotask API user's credentials read from the environment (AUTOTASK_PSA_USER_NAME, AUTOTASK_PSA_SECRET, AUTOTASK_PSA_API_INTEGRATION_CODE) - never logged and never sent anywhere except your own Autotask zone. Read commands (the cross-entity views, search, picklist) can change nothing. Writes are not gated by default: --dry-run is an opt-in preview flag, so the recommended policy is an agent-level rule - preview with --dry-run, show the exact command, get approval, then run the write. Keep the delete commands and any security-level administration human-only. The strongest control is the security level on the API user you create. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/autotask/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Autotask MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Autotask data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What credentials do I need?

An API-only user created in your own Autotask instance, which gives you a UserName and Secret, plus the tracking Integration Code from your integration vendor record. Set AUTOTASK_PSA_USER_NAME, AUTOTASK_PSA_SECRET, and AUTOTASK_PSA_API_INTEGRATION_CODE in your environment. The API user's security level is the real permission boundary - scope it to exactly what you want the AI to reach.

### Do I have to figure out my Autotask zone URL?

No. Autotask hosts each tenant in a numbered zone (webservicesN.autotask.net). Run autotask-cli zone once and the CLI discovers and caches your tenant's base URL, the same way Autotask's own docs tell integrations to make a zone-information request first.

### Will this blow through my Autotask API limits?

Autotask meters API calls per database in a rolling 60-minute window and adds latency as you approach the threshold. The local mirror exists so reads stop hitting the API: after the first sync, the cross-entity views (unbilled, reconcile, retainer, company-360, ticket-aging, triage, workload) run against local SQLite with zero API calls, and sync is incremental - it only fetches what changed since the last checkpoint.

### How do I deal with status and priority being numbers, not labels?

Run autotask-cli picklist "Tickets" "status" (or any entity and field) and it prints the label-to-ID map from cached field metadata, so you can read reports and build filters without memorizing the integer IDs Autotask stores categorical fields as.

### Is this Datto Autotask PSA or just Autotask?

Same product. Autotask PSA is now branded Datto Autotask PSA under Kaseya; this skill targets the Autotask REST API, which both names refer to. It does not cover Datto RMM - that is a separate product with a separate API.


## Status

Beta. Validated against the Autotask PSA API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
