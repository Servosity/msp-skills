---
layout: default
title: "QuickBooks Online MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every QuickBooks Online Accounting entity, plus an offline SQLite mirror, cross-entity search, and AR/AP aging no SDK or read-only MCP ships."
permalink: /skills/quickbooks/
skill_name: "QuickBooks Online MCP"
image: /assets/social/quickbooks/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for QuickBooks Online?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for QuickBooks Online, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the QuickBooks Online MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local QuickBooks Online MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my QuickBooks Online data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my QuickBooks API rate limits?"
    a: "Rarely. The only API-heavy step is the one-time sync that mirrors your company into local SQLite; after that, ar-aging, dso, cash-forecast, reconcile and the rest run entirely offline against the mirror. Intuit throttles per company (realm), and the CLI paginates and rate-limits sync for you, so day-to-day questions never touch the API."
  - q: "Do I need to be an Intuit partner to use it?"
    a: "No. You need a QuickBooks Online company and an OAuth access token scoped to com.intuit.quickbooks.accounting, plus your company realm ID. You mint the token from the Intuit Developer portal or the OAuth 2.0 Playground, and `quickbooks-cli auth refresh` turns a refresh token into a fresh access token - no partner status required."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your QuickBooks Online credentials once; quickbooks-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a QuickBooks Online question in plain language; it runs quickbooks-cli for you."
---

# The QuickBooks Online MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Intuit Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for QuickBooks Online. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects QuickBooks Online to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your books a question and get the answer, not a CSV export. The skill syncs your QuickBooks Online company into a local SQLite mirror, then answers who owes you (ar-aging), what you owe (ap-aging), where the cash is (balances), which invoices to chase first (invoices stale), and whether the books are clean to close (reconcile) - instantly, offline, across the whole book. No portal clicking, no per-question API call.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/quickbooks){: .btn}

## Instead of clicking through QuickBooks Online, just ask

**Instead of** Exporting the A/R Aging Summary to CSV and pivoting it in Excel to see who is overdue
**just ask:** *"Who owes us money, and how overdue are they?"*
<sub>Your agent runs: <code>quickbooks-cli ar-aging --agent</code></sub>

**Instead of** Clicking through every open invoice to build this week's collections call list by hand
**just ask:** *"Which overdue invoices should I chase first?"*
<sub>Your agent runs: <code>quickbooks-cli invoices stale --days 30 --agent</code></sub>

**Instead of** Eyeballing the bank balance and guessing whether next month's bills will clear
**just ask:** *"What does our cash position look like over the next month?"*
<sub>Your agent runs: <code>quickbooks-cli cash-forecast --weeks 4 --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/quickbooks/wide-1200x630.png" src="/assets/video/quickbooks/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/quickbooks/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Who owes us money, bucketed 0-30 / 31-60 / 61-90 / 90+? | `quickbooks-cli ar-aging --agent` |
| What do we owe vendors, and when is it due? | `quickbooks-cli ap-aging --agent` |
| Which overdue invoices should I chase first? | `quickbooks-cli invoices stale --days 30 --agent` |
| Where does our cash stand across accounts, AR, and AP? | `quickbooks-cli balances --agent` |
| What net cash movement is scheduled over the next 4 weeks? | `quickbooks-cli cash-forecast --weeks 4 --agent` |
| What is our DSO, and who are the slowest payers? | `quickbooks-cli dso --agent` |
| Are the books clean enough to close this month? | `quickbooks-cli reconcile --agent` |
| Which payments came in but were never applied to an invoice? | `quickbooks-cli payments unapplied --agent` |
| Which customers are duplicated in our list? | `quickbooks-cli dupes customers --agent` |
| Who slipped an aging bucket since our last check? | `quickbooks-cli aging-delta --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/quickbooks/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/quickbooks/guide.md).

## What makes this one different

Most QuickBooks integrations and MCP servers proxy each question into a live API call - fine for fetching one invoice, useless for "who is overdue across the whole book" because QuickBooks has no single endpoint that returns aging, DSO, or a reconciliation verdict. This skill syncs your company into a local SQLite mirror once, then computes those answers offline with real date math and cross-entity joins (invoices to payments to customers) - instant, no rate-limit pressure, and the agent sees the answer, not raw JSON pages.

QuickBooks Online's own reports are point-in-time and live in the portal - you click, export, and pivot. This skill answers the same questions from your terminal or AI agent, adds cross-run memory (aging-delta) the portal does not keep, and composes a one-shot month-end reconcile sweep the UI makes you assemble by hand. It complements your books of record; it does not replace them.

## The pain this closes

- Month-end close eats a day: chasing unapplied payments, deduping near-identical customer records, and proving the books actually balance before they go to the accountant - the kind of bookkeeping overhead MSP owners vent about on r/msp and r/QuickBooks.
- Receivables age silently. By the time you pull the A/R aging report the 90+ bucket already holds money you may never collect, and QuickBooks reports are point-in-time - so you cannot see who slipped a bucket since last month without keeping your own spreadsheet history.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/quickbooks/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/quickbooks/install.ps1 | iex
```

After install, authenticate once with your QuickBooks Online credentials, then verify with `quickbooks-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | ar-aging, ap-aging, balances, dso, cash-forecast, reconcile, dupes, search, plus every list and get | Allow |
| Write (routine) | invoices / bills / payments / customers / vendors / accounts / items / journal-entries create and update (16 commands), plus bulk import | Preview with --dry-run, then a reviewed write |
| Destructive / config | invoices delete, bills delete, payments delete, journal-entries delete (hard deletes) | Human-in-the-loop only |

The skill reads everything - aging, balances, DSO, cash forecast, reconciliation, search - and read commands cannot change anything. Writes are explicit create/update/delete commands on invoices, bills, payments, customers, vendors, accounts, items, and journal entries, and `--dry-run` is opt-in, so the recommended agent policy is: read freely, preview every write, and keep a human on deletes. Credentials are read from the environment only and are never written to disk or logged. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/quickbooks/governance.md).

## Frequently asked questions

### Is there an MCP server for QuickBooks Online?

Yes - this one. A free, open source MCP server and Claude Code Skill for QuickBooks Online, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the QuickBooks Online MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local QuickBooks Online MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my QuickBooks Online data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my QuickBooks API rate limits?

Rarely. The only API-heavy step is the one-time sync that mirrors your company into local SQLite; after that, ar-aging, dso, cash-forecast, reconcile and the rest run entirely offline against the mirror. Intuit throttles per company (realm), and the CLI paginates and rate-limits sync for you, so day-to-day questions never touch the API.

### Do I need to be an Intuit partner to use it?

No. You need a QuickBooks Online company and an OAuth access token scoped to com.intuit.quickbooks.accounting, plus your company realm ID. You mint the token from the Intuit Developer portal or the OAuth 2.0 Playground, and `quickbooks-cli auth refresh` turns a refresh token into a fresh access token - no partner status required.


## More Billing connectors

Run more than one Billing tool, or comparing options? These connectors work the same way: [AppDirect](/skills/appdirect/) · [Gradient MSP](/skills/gradient/) · [Pax8](/skills/pax8/) · [Sherweb](/skills/sherweb/) · [Xero](/skills/xero/)

## Status

Beta. Validated against the QuickBooks Online API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
