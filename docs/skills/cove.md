---
layout: default
title: "Cove Data Protection MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "The first CLI and MCP server for Cove Data Protection \u2014 fleet-wide backup health, billing usage, and storage trends from a terminal, with the local history the vendor console doesn't keep."
permalink: /skills/cove/
skill_name: "Cove Data Protection MCP"
image: /assets/social/cove/wide-1200x630.png
verification: live-verified
faqs:
  - q: "Is there an MCP server for Cove Data Protection?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Cove Data Protection, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Cove Data Protection MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `COVE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `COVE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but cove-mcp speaks HTTP natively: run `cove-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Cove Data Protection data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "What kind of Cove account does it need?"
    a: "A dedicated API User, created in the Cove Management Console under Users > API Users. It issues a login name and an API token (shown only once); API Users can use the JSON-RPC API but cannot sign in to the console. Set COVE_USERNAME to the API user's login name, COVE_PASSWORD to the API token, and COVE_PARTNER to the customer/partner it was created for (required for API Users). COVE_PARTNER must be the full customer/partner string exactly as shown in the Cove console customer dropdown - usually including the account email in parentheses, e.g. Acme Corp (admin@acme.com), not just the company name; Cove returns the same Unknown partner/username or bad password error for a wrong format as for a bad password. The skill exchanges them for a cached session visa and refreshes it automatically. N-able retired the older per-user API access checkbox."
  - q: "Can it restore files or browse backed-up data?"
    a: "No. This skill speaks the Cove management API: fleet health, billing, storage trends, and enumeration. Restores and per-session file browsing run through the Backup Manager client and the storage-node Reporting Service, which this CLI does not cover."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Cove Data Protection credentials once, then run cove-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Cove Data Protection question in plain language; it runs cove-cli for you."
---

# The Cove Data Protection MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by N-able.

**✓ Live-verified by @AvlCompCo (MSP)** against a production tenant · 2026-06-18 · [receipt →](https://github.com/Servosity/msp-skills/issues/139).

Yes - there is an MCP server for Cove Data Protection. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Cove Data Protection to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask which Cove backups failed last night and get every failed, aborted, or never-started device across all your customers in one sweep, the status codes decoded to plain names. Cove's console scopes to one partner at a time and forgets yesterday; this skill speaks the whole JSON-RPC API and keeps a local snapshot history, so storage-growth and what-changed-since-Friday trends exist at all.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/cove){: .btn}

## Instead of clicking through Cove Data Protection, just ask

**Instead of** Log into backup.management, switch into each customer one at a time, and eyeball the dashboard for red devices every single morning.
**just ask:** *"Which devices failed their last backup across all my customers since yesterday?"*
<sub>Your agent runs: <code>cove-cli devices failures --since 24h --agent</code></sub>

**Instead of** Export the monthly usage report, then decode SKU and M365 seat counts by hand with the column-code legend open in another tab.
**just ask:** *"Give me the month-end billing usage for every device as a CSV."*
<sub>Your agent runs: <code>cove-cli billing usage --csv</code></sub>

**Instead of** Wonder whether a customer's backup storage is creeping up, with no way to tell because the console only ever shows today's number.
**just ask:** *"Which devices and customers grew their backup storage the most this week?"*
<sub>Your agent runs: <code>cove-cli storage growth --since 7d --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/cove/wide-1200x630.png" src="/assets/video/cove/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/cove/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which devices failed their last backup since yesterday? | `cove-cli devices failures --since 24h --json` |
| Which devices have had no successful backup in 3 days? | `cove-cli devices stale --days 3 --json` |
| What is the fleet-wide health rollup, broken down per customer? | `cove-cli fleet health --by partner --json` |
| Which devices and customers grew their storage fastest this week? | `cove-cli storage growth --since 7d --json` |
| What is the month-end billing usage per device, with codes decoded? | `cove-cli billing usage --csv` |
| Which device SKUs or seat counts changed since last month? | `cove-cli billing changes --json` |
| Which backup statuses flipped since my last snapshot? | `cove-cli devices changes --since 7d --json` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/cove/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/cove/guide.md).

## What makes this one different

Most Cove integrations proxy each question into a single live JSON-RPC call: fine for one device, useless for "show me every failure across 40 customers" or any trend. This skill sweeps the whole partner tree in one command, decodes the status and SKU column codes to plain names, and keeps a timestamped local snapshot history in SQLite. So cross-customer rollups and storage-growth trends are answerable at all, not just today's single number.

Cove's backup.management console has no AI and answers one customer at a time with no history. This skill adds the cross-tenant sweep and the local trend layer the console structurally withholds; it complements the console for restores and per-session detail, which still live in the Backup Manager client and the storage-node Reporting Service.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](/skills/servosity/) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

## The pain this closes

- Cove's dashboard scopes to one customer at a time. To answer "what failed last night across all 40 clients" you open each customer separately, every morning. The N-able/Cove community on r/msp and MSPGeek repeatedly asks for a single cross-tenant failure list the console does not provide.
- The console shows today's status and storage but keeps no history, so "is this customer's backup growing?" and "did Friday's failures clear?" have no data behind them.
- Month-end billing means exporting usage and decoding cryptic column codes (SKU, used storage, seat counts) by hand against a legend in another tab.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cove/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cove/install.ps1 | iex
```

After install, authenticate once with your Cove Data Protection credentials, then verify with `cove-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read (tenant) | devices failures, devices stale, fleet health, billing usage, storage growth, and every enumerate command | Allow |
| Local-only writes | sync, snapshot, auth login/logout - write to your machine's SQLite mirror and visa cache, never the tenant | Allow (no tenant change) |
| Generic API call | call <method> reaches any of the 251 JSON-RPC methods, including the few that mutate | Human-in-the-loop; preview with --dry-run first |

The skill is read-first: every command that touches Cove reads (failure sweeps, health rollups, billing, storage, enumeration) and changes nothing in your tenant. The local commands (sync, snapshot, auth) write only to your machine's SQLite mirror and session cache. The one escape hatch, call, can invoke any JSON-RPC method including the few that mutate, so keep it human-reviewed. Recommended policy: allow reads freely; gate call and anything that writes behind a human. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/cove/governance.md).

## Frequently asked questions

### Is there an MCP server for Cove Data Protection?

Yes - this one. A free, open source MCP server and Claude Code Skill for Cove Data Protection, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Cove Data Protection MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `COVE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `COVE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but cove-mcp speaks HTTP natively: run `cove-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Cove Data Protection data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What kind of Cove account does it need?

A dedicated API User, created in the Cove Management Console under Users > API Users. It issues a login name and an API token (shown only once); API Users can use the JSON-RPC API but cannot sign in to the console. Set COVE_USERNAME to the API user's login name, COVE_PASSWORD to the API token, and COVE_PARTNER to the customer/partner it was created for (required for API Users). COVE_PARTNER must be the full customer/partner string exactly as shown in the Cove console customer dropdown - usually including the account email in parentheses, e.g. Acme Corp (admin@acme.com), not just the company name; Cove returns the same Unknown partner/username or bad password error for a wrong format as for a bad password. The skill exchanges them for a cached session visa and refreshes it automatically. N-able retired the older per-user API access checkbox.

### Can it restore files or browse backed-up data?

No. This skill speaks the Cove management API: fleet health, billing, storage trends, and enumeration. Restores and per-session file browsing run through the Backup Manager client and the storage-node Reporting Service, which this CLI does not cover.


## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](/skills/acronis/) · [Afi](/skills/afi/) · [Axcient x360Recover](/skills/axcient/) · [Datto BCDR](/skills/datto-bcdr/) · [Servosity](/skills/servosity/) · [SkyKick](/skills/skykick/) · [Veeam](/skills/veeam/)

## Status

Beta. Validated against the Cove Data Protection API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
