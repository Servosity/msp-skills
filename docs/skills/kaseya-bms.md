---
layout: default
title: "Kaseya BMS MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "The first dedicated CLI and MCP server for Kaseya BMS - the full PSA surface plus offline sync, full-text search, and the queue, contract-burn, and unbilled-time analytics the web grid can't compute."
permalink: /skills/kaseya-bms/
skill_name: "Kaseya BMS MCP"
image: /assets/social/kaseya-bms/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Kaseya BMS MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Kaseya BMS data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Kaseya BMS API rate limits?"
    a: "Rarely. BMS allows 1,500 requests per hour per endpoint. Reads default to the local SQLite mirror (--data-source auto), so day-to-day questions cost zero API calls; you only spend the budget when you sync or query live."
  - q: "Do I need to be a Kaseya customer, and how do I authenticate?"
    a: "Yes - you need a Kaseya BMS tenant and an API user. The skill authenticates as that user with 'kaseya-bms-cli auth login' (your BMS username, password, and tenant name), and can only do what that user is permitted to do. You can also paste a pre-minted JWT via KASEYA_BMS_TOKEN."
  - q: "Will this replace my Kaseya BMS portal?"
    a: "No. It's a faster path for the questions you ask every day - queue health, unbilled time, pipeline - and for letting an AI agent drive the service desk. The BMS portal stays your system of record."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-bms/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-bms/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Kaseya BMS credentials once; kaseya-bms-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Kaseya BMS question in plain language; it runs kaseya-bms-cli for you."
---

# Kaseya BMS + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the Kaseya BMS
> API. Not affiliated with, endorsed by, or sponsored by Kaseya US LLC.

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

Run the Kaseya BMS service desk, contracts, and billing from your terminal - or let your AI agent do it. Ask in plain English which queues are underwater, which tickets are going stale, who's overloaded, how much of each contract you've burned, and what billable time is sitting unbilled - and get the answer instantly from a local mirror, without exporting a single report.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/kaseya-bms){: .btn}

## Instead of clicking through Kaseya BMS, just ask

**Instead of** Export the morning ticket list to Excel and pivot it by queue to find where the backlog is building.
**just ask:** *"Which queues are underwater this morning, and how many tickets are going stale?"*
<sub>Your agent runs: <code>kaseya-bms-cli queue-health --agent</code></sub>

**Instead of** Run the unbilled-time report, export it, and reconcile approved billable hours per client by hand before invoicing.
**just ask:** *"How many approved billable hours are sitting unbilled per client right now?"*
<sub>Your agent runs: <code>kaseya-bms-cli unbilled --agent</code></sub>

**Instead of** Click through each technician's board to work out who has bandwidth for the next escalation.
**just ask:** *"Who's overloaded and who can take the next ticket?"*
<sub>Your agent runs: <code>kaseya-bms-cli workload --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/kaseya-bms/wide-1200x630.png" src="/assets/video/kaseya-bms/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/kaseya-bms/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which queues are underwater and what's going stale before standup? | `kaseya-bms-cli queue-health --agent` |
| Which open tickets haven't been touched in a week, oldest first? | `kaseya-bms-cli stale-tickets --days 7 --agent` |
| Who's overloaded and who can take the next ticket? | `kaseya-bms-cli workload --agent` |
| How much of each contract have we burned this quarter? | `kaseya-bms-cli contract-burn --window-days 90 --agent` |
| What approved billable time is ready to invoice, by account? | `kaseya-bms-cli unbilled --agent` |
| What's the open sales pipeline by stage, and which deals have slipped? | `kaseya-bms-cli pipeline --agent` |
| Find every ticket mentioning a phrase across the whole tenant | `kaseya-bms-cli search "VPN outage"` |
| Sync the tenant into a local mirror for instant offline queries | `kaseya-bms-cli sync` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/kaseya-bms/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/kaseya-bms/guide.md).

## What makes this one different

Most Kaseya BMS integrations proxy each question into a live API call against the 1,500-per-hour-per-endpoint limit - fine for one record, but it falls over the moment you ask a fleet-wide or month-end question. This skill syncs the tenant into a local SQLite mirror with full-text search, so aggregate questions - queue health, contract burn, unbilled hours, weighted pipeline - are answered by one local join: instant, offline, and the agent sees the answer, not the raw data.

The BMS console and its reports are built for clicking through one screen at a time; this skill answers the cross-entity questions the grid won't compute - per-contract burn, unbilled hours by account, weighted pipeline with slipped-deal flags - from your terminal or your AI agent, and complements the portal rather than replacing it.

## The pain this closes

- MSPs on r/msp and MSPGeek repeatedly call BMS reporting the weak spot: the questions you ask every morning - queue backlog, aging tickets, who's overloaded - mean exporting grids to Excel and pivoting them by hand.
- Month-end billing is the other recurring complaint: pulling approved-but-unbilled time per client out of BMS for the invoice run is a manual export-and-reconcile chore, and the API's 1,500-requests-per-hour-per-endpoint limit punishes any tool that asks the question live every time.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/kaseya-bms/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/kaseya-bms/install.ps1 | iex
```

After install, authenticate once with your Kaseya BMS credentials, then verify with `kaseya-bms-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | All 243 read commands plus the analytics rollups: queue-health, stale-tickets, unbilled, pipeline, servicedesk get-ticket, search | Allow |
| Write (routine) | 124 create/update commands across the service desk, CRM, finance, and projects: servicedesk post-ticket, servicedesk assign-ticket, crm post-account, finance mark-invoices-as-sent, plus import | Preview with --dry-run, then a reviewed write |
| Credential / security | Commands that return or store secrets: integrations get-itgpassword-value, integrations get-vsa-access-info, security refresh-token, auth login | Human-in-the-loop only |
| Destructive | 21 delete commands: servicedesk delete-ticket, crm delete-contact, system delete-attachment | Human-in-the-loop only |
| Admin | The 75-command admin group: webhooks, workflows, services, K1 access control, Teams channels | Operator-only, not for agents |

The skill reads everything - tickets, CRM, contracts, finance, projects - and can create and update records across the service desk, CRM, finance, and projects, so keep an autonomous agent to reads plus previewed (--dry-run) writes with a human approving each write. Deletes, the back-office 'admin' group, and any command that returns stored credentials (ITGlue or VSA access info) are human-in-the-loop only. The strongest control is the scope of the BMS API user you authenticate as. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/kaseya-bms/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Kaseya BMS MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Kaseya BMS data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Kaseya BMS API rate limits?

Rarely. BMS allows 1,500 requests per hour per endpoint. Reads default to the local SQLite mirror (--data-source auto), so day-to-day questions cost zero API calls; you only spend the budget when you sync or query live.

### Do I need to be a Kaseya customer, and how do I authenticate?

Yes - you need a Kaseya BMS tenant and an API user. The skill authenticates as that user with 'kaseya-bms-cli auth login' (your BMS username, password, and tenant name), and can only do what that user is permitted to do. You can also paste a pre-minted JWT via KASEYA_BMS_TOKEN.

### Will this replace my Kaseya BMS portal?

No. It's a faster path for the questions you ask every day - queue health, unbilled time, pipeline - and for letting an AI agent drive the service desk. The BMS portal stays your system of record.


## Status

Beta. Validated against the Kaseya BMS API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
