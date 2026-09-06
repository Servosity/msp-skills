---
layout: default
title: "DataGate MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every DataGate telecom billing API resource, plus a local SQLite store and offline search the API can't return in one call."
permalink: /skills/datagate/
skill_name: "DataGate MCP"
image: /assets/social/datagate/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for DataGate?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for DataGate, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the DataGate MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `DATAGATE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `DATAGATE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datagate/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datagate/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your DataGate credentials once, then run datagate-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a DataGate question in plain language; it runs datagate-cli for you."
---

# The DataGate MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by DataGate.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for DataGate. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects DataGate to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

MSPs and telecom resellers who bill through DataGate normally work through its web portal one customer at a time. Ask your AI "pull this month's invoices" or "look up this customer's agreement," and get an answer without clicking through the portal - a local SQLite mirror means repeated lookups don't re-spend DataGate's per-account rate limit either.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/datagate){: .btn}

## Instead of clicking through DataGate, just ask

**Instead of** Opening the DataGate portal, filtering to a billing period, and exporting or eyeballing each invoice
**just ask:** *"Pull this month's DataGate invoices as JSON."*
<sub>Your agent runs: <code>datagate-cli invoices --period-start 2026-08-01T00:00:00Z --period-end 2026-08-31T23:59:59Z --json</code></sub>

**Instead of** Clicking into a customer's record in the portal to check their agreement
**just ask:** *"What's this customer's agreement in DataGate?"*
<sub>Your agent runs: <code>datagate-cli agreements list --customer-id <customer-id> --json</code></sub>

**Instead of** Re-fetching the same customer list from the live API every time you need to look something up
**just ask:** *"Search DataGate for this customer by name."*
<sub>Your agent runs: <code>datagate-cli search "<customer name>" --data-source local</code></sub>


## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Pull this month's invoices | `datagate-cli invoices --period-start 2026-08-01T00:00:00Z --period-end 2026-08-31T23:59:59Z --json` |
| Look up a customer | `datagate-cli customers get <customer-id> --json` |
| What's this customer's agreement? | `datagate-cli agreements list --customer-id <customer-id> --json` |
| Search for a customer or invoice by name/number | `datagate-cli search "<term>" --json` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/datagate/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/datagate/guide.md).

## What makes this one different

A thin DataGate wrapper proxies every question into a live API call, which works for one lookup but adds up fast against DataGate's per-account rate limit once you're checking a large customer list or pulling invoices repeatedly. This skill syncs customers, agreements, and invoices into a local SQLite mirror with full-text search, so repeated lookups become one local query: instant, offline, and no extra API calls spent.



## The pain this closes

- DataGate's own web portal has no cross-customer view - answering "which invoices are unpaid this month across every customer" means clicking into each one by hand (skills/datagate/pain-point.md).
- DataGate's API enforces a real rate limit (60 requests/minute, 5,000/day, per account) that a naive script or wrapper burns through fast if it re-fetches everything on every question.
- Monthly billing reconciliation tends to be a recurring manual chore: open the portal, filter to the period, export or eyeball each invoice, repeat next month.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/datagate/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/datagate/install.ps1 | iex
```

After install, authenticate once with your DataGate credentials, then verify with `datagate-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |


 Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/datagate/governance.md).

## Frequently asked questions

### Is there an MCP server for DataGate?

Yes - this one. A free, open source MCP server and Claude Code Skill for DataGate, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the DataGate MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `DATAGATE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `DATAGATE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.


## More Billing connectors

Run more than one Billing tool, or comparing options? These connectors work the same way: [AppDirect](/skills/appdirect/) · [AWS](/skills/aws-billing/) · [Gradient MSP](/skills/gradient/) · [Maxio](/skills/maxio/) · [Pax8](/skills/pax8/) · [QuickBooks Online](/skills/quickbooks/) · [Sherweb](/skills/sherweb/) · [Xero](/skills/xero/)

## Status

Beta. Validated against the DataGate API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
