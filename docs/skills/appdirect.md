---
layout: default
title: "AppDirect MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every documented AppDirect marketplace operation in one binary, plus offline sync and billing-reconciliation joins."
permalink: /skills/appdirect/
skill_name: "AppDirect MCP"
image: /assets/social/appdirect/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for AppDirect?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for AppDirect, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the AppDirect MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local AppDirect MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my AppDirect data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need to be an AppDirect partner?"
    a: "Yes. The skill authenticates with partner API credentials (OAuth2 client_credentials) for a marketplace you operate or resell on - it is not for end-user purchases on a marketplace you don't control. Point it at your own marketplace with APPDIRECT_BASE_URL if you run a white-label domain."
  - q: "Will this hit AppDirect's API rate limits?"
    a: "It is built to avoid them. The marketplace REST API uses leaky-bucket rate limits (for example, 20-request buckets that refill a few per second); because this skill syncs to a local mirror and answers most questions offline, your day-to-day queries make almost no live calls. Sync itself respects the limits and you can cap request rate with --rate-limit."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/appdirect/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/appdirect/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your AppDirect credentials once; appdirect-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a AppDirect question in plain language; it runs appdirect-cli for you."
---

# The AppDirect MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by AppDirect, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for AppDirect. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects AppDirect to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask 'which AppDirect payments failed this week?' or 'reconcile billing before month-close' and get an answer across every reseller company in one call. The skill syncs your whole AppDirect marketplace - subscriptions, invoices, payments, companies, pipeline - into a local mirror, so cross-company billing questions that take hundreds of console clicks return instantly, offline, from your terminal or your AI agent.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/appdirect){: .btn}

## Instead of clicking through AppDirect, just ask

**Instead of** Open the AppDirect console and filter each company's billing screen one by one, copying failed payments into a spreadsheet for the weekly chase.
**just ask:** *"Which AppDirect payments failed in the last 7 days, across every company?"*
<sub>Your agent runs: <code>appdirect-cli payments unpaid --since 7d --json</code></sub>

**Instead of** Export subscriptions and invoices separately, then VLOOKUP to find active subscriptions that were never invoiced before month-close.
**just ask:** *"Reconcile AppDirect billing for the last 30 days - what's active but unbilled, overdue, or failed?"*
<sub>Your agent runs: <code>appdirect-cli reconcile --since 30d --agent</code></sub>

**Instead of** Click through four console screens - users, subscriptions, invoices, opportunities - to brief one customer before a renewal call.
**just ask:** *"Show me everything on AppDirect company <companyId>."*
<sub>Your agent runs: <code>appdirect-cli company show <companyId></code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/appdirect/wide-1200x630.png" src="/assets/video/appdirect/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/appdirect/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which payments failed or stalled in the last week, across every company? | `appdirect-cli payments unpaid --since 7d --json` |
| What's active-but-unbilled, overdue, or failed before month-close? | `appdirect-cli reconcile --since 30d --agent` |
| What changed in subscriptions this week - new, ended, or suspended? | `appdirect-cli subs changed --since 7d --json` |
| Show one customer's full picture - users, subscriptions, invoices, opportunities. | `appdirect-cli company show <companyId>` |
| What does my assisted-sales pipeline look like by status? | `appdirect-cli pipeline --group-by status --agent` |
| Which open opportunities have gone stale? | `appdirect-cli pipeline stale --days 14 --json` |
| Find any company, subscription, invoice, or opportunity by keyword. | `appdirect-cli search "<text>"` |
| Pull the whole marketplace into a local mirror for offline analysis. | `appdirect-cli sync` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/appdirect/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/appdirect/guide.md).

## What makes this one different

Most AppDirect integrations proxy each question into a live REST call and re-mint an hourly OAuth token every time - fine for one record, but it falls over the moment you ask something that spans every company. This skill syncs the marketplace into a local SQLite mirror, so cross-entity questions become one offline join: reconcile, subs changed, and pipeline answer in a single call what a stateless wrapper would need hundreds of paginated requests to attempt.

The AppDirect console shows one company, one screen at a time, with no cross-company billing-reconciliation view. This skill complements the portal by answering the aggregate, before-month-close questions the UI can't - from your terminal or your AI agent.

## The pain this closes

- Active subscriptions slip through un-invoiced and failed payments sit unnoticed for weeks, leaking margin every month-close - and the console has no cross-company view to catch them.
- Briefing one customer means clicking through users, subscriptions, invoices, and open opportunities on separate screens; multiply that by hundreds of companies and the monthly reconciliation never gets done.
- Every API call re-mints an hourly OAuth token and answers for one record at a time, so aggregate questions across the whole marketplace are effectively impossible from the portal.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/appdirect/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/appdirect/install.ps1 | iex
```

After install, authenticate once with your AppDirect credentials, then verify with `appdirect-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | payments unpaid, reconcile, subs changed, company show, pipeline, search, and every GET (companies, users, subscriptions, invoices, opportunities) | Allow |
| Write (routine) | create/update companies, users, memberships, groups, and opportunities; invite users; request a purchase; finalize or clone an opportunity; apply a discount | Preview with --dry-run, then a reviewed write |
| Credential / financial | set a temporary password; create, update, or set a default payment method or payment instrument | Human-in-the-loop only |
| Destructive | delete a company membership, user group, subscription assignment, or shopping cart; cancel a subscription or add-on; expire a developer account | Human-in-the-loop only |

The skill reads marketplace data - companies, users, subscriptions, invoices, payments, and the assisted-sales pipeline - and those reads are always safe. It can also write: create and update companies, users, memberships, and opportunities; touch money-moving operations like purchases and payment instruments; and delete or expire records. Keep an autonomous agent to reads plus previewed (--dry-run) writes, and require a human for payment/credential and destructive (delete, cancel, expire) operations. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/appdirect/governance.md).

## Frequently asked questions

### Is there an MCP server for AppDirect?

Yes - this one. A free, open source MCP server and Claude Code Skill for AppDirect, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the AppDirect MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local AppDirect MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my AppDirect data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need to be an AppDirect partner?

Yes. The skill authenticates with partner API credentials (OAuth2 client_credentials) for a marketplace you operate or resell on - it is not for end-user purchases on a marketplace you don't control. Point it at your own marketplace with APPDIRECT_BASE_URL if you run a white-label domain.

### Will this hit AppDirect's API rate limits?

It is built to avoid them. The marketplace REST API uses leaky-bucket rate limits (for example, 20-request buckets that refill a few per second); because this skill syncs to a local mirror and answers most questions offline, your day-to-day queries make almost no live calls. Sync itself respects the limits and you can cap request rate with --rate-limit.


## More Billing connectors

Run more than one Billing tool, or comparing options? These connectors work the same way: [Gradient MSP](/skills/gradient/) · [Maxio](/skills/maxio/) · [Pax8](/skills/pax8/) · [QuickBooks Online](/skills/quickbooks/) · [Sherweb](/skills/sherweb/) · [Xero](/skills/xero/)

## Status

Beta. Validated against the AppDirect API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
