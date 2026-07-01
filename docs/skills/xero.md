---
layout: default
title: "Xero MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Xero Accounting read plus a local SQLite ledger \u2014 aging, reconciliation, and GL tie-out that no other Xero tool computes offline."
permalink: /skills/xero/
skill_name: "Xero MCP"
image: /assets/social/xero/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Xero?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Xero, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Xero MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Xero MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Xero data safe?"
    a: "Your data stays on your machine - the CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Xero API rate limits?"
    a: "Rarely. After one sync into the local mirror, aging, reconciliation, tie-out, exposure, ledger, and search run entirely offline - zero API calls. Only sync, explicit live reads, and writes touch the API, which Xero caps at 60 calls per minute and 5,000 per day per organisation; the local-first design is built around that limit."
  - q: "Which Xero credentials does it need, and does it create them?"
    a: "You mint an OAuth2 token in the Xero developer portal - a Custom Connection is the simplest for machine-to-machine use - then pass it plus your organisation's tenant id via XERO_ACCESS_TOKEN and XERO_TENANT_ID. The CLI never creates or rotates tokens; it only uses the one you give it. Run xero-cli doctor to confirm both are set before syncing."
  - q: "Does it cover one organisation or several?"
    a: "One organisation per XERO_TENANT_ID. The local mirror holds a single tenant; for a multi-entity portfolio, point the tenant id at each organisation in turn and loop."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Xero credentials once; xero-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Xero question in plain language; it runs xero-cli for you."
---

# The Xero MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Xero Limited.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Xero. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Xero to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask in plain English who owes you and how overdue, which authorised invoices still have no payment applied, and whether the general ledger ties to outstanding invoices at close - for a Xero organisation, in one call. Xero plus your AI agent reads a local mirror of the org, so the aging, reconciliation, and tie-out questions the web reports make you export and pivot become one instant, offline answer.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/xero){: .btn}

## Instead of clicking through Xero, just ask

**Instead of** Export the Aged Receivables report from Xero, drop it into a spreadsheet, and sort by days overdue to build this week's collections list
**just ask:** *"Who owes us money, and how overdue is each one?"*
<sub>Your agent runs: <code>xero-cli aging --agent</code></sub>

**Instead of** Open each authorised invoice and cross-check it against the payments list to find which ones still are not paid
**just ask:** *"Which authorised invoices are still owed with no payment applied?"*
<sub>Your agent runs: <code>xero-cli reconcile --agent</code></sub>

**Instead of** Pull the General Ledger and the receivables report and reconcile the control-account totals by hand at month-end close
**just ask:** *"Do the books tie out before I sign off the close?"*
<sub>Your agent runs: <code>xero-cli tie-out --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/xero/wide-1200x630.png" src="/assets/video/xero/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/xero/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Who owes us, and how overdue is each invoice? | `xero-cli aging --agent` |
| What do we owe suppliers, bucketed by age? | `xero-cli aging --payable --agent` |
| Which contacts carry the most receivable risk? | `xero-cli exposure --agent` |
| Which authorised invoices are still owed with no applied payment? | `xero-cli reconcile --agent` |
| Which bank transactions are unreconciled, and what might they match? | `xero-cli bank-recon --agent` |
| Do the GL control accounts tie to outstanding invoices at close? | `xero-cli tie-out --agent` |
| What posted to a single account, as a running balance? | `xero-cli ledger 200 --agent` |
| What changed in the organisation since last week? | `xero-cli since 7d --agent` |
| Give me one state-of-the-org summary in a single call. | `xero-cli snapshot --agent` |
| Find every synced record matching a keyword. | `xero-cli search "overdue" --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/xero/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/xero/guide.md).

## What makes this one different

Most Xero integrations and MCP servers proxy each question into a live API call - fine for one record, but it dies at scale and burns through Xero's 60-call-per-minute rate limit. This skill syncs the organisation into a local SQLite mirror, so aging, reconciliation, exposure, and GL tie-out become one offline join: instant, reproducible, and computed from your own data instead of re-paging the API.

Xero's web reports answer one report at a time in the browser; this skill answers the cross-report questions - aging plus reconciliation plus GL tie-out - in the AI agent you already work in, computed offline from a local mirror. It complements the Xero web app you still use for editing and day-to-day bookkeeping; it does not replace it.

## The pain this closes

- AR collections run on a stale spreadsheet: pulling Aged Receivables, exporting it, and pivoting by days overdue is a weekly manual chore, so the chase list is always a day behind the actual ledger.
- Reconciling cash to invoices is one row at a time in the browser - finding which authorised invoices still have no applied payment, or which bank lines are unreconciled, means clicking back and forth between two screens.
- At month-end close nobody can answer 'do the books tie?' fast: matching the GL control accounts to outstanding invoices is a manual reconciliation, and Xero's 60-calls-per-minute / 5,000-per-day API limit makes roll-your-own scripts crawl.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/xero/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/xero/install.ps1 | iex
```

After install, authenticate once with your Xero credentials, then verify with `xero-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | aging, exposure, reconcile, bank-recon, tie-out, ledger, snapshot, since, search, analytics, sync, doctor, accounts get, contacts get, invoices get, payments get, journals get | Allow |
| Write (routine) | invoices create, invoices update, contacts create, contacts update, accounts create, accounts update, payments create, bank-transactions create, items create, items update, import | Preview with --dry-run, then a reviewed write |
| Destructive / credential | accounts delete, items delete, payments delete, auth logout | Human-in-the-loop only |

Everything analytical reads from a local mirror - aging, exposure, reconcile, bank-recon, tie-out, ledger, snapshot, since, and search are always safe to run and cannot change anything. Writes (creating or updating invoices, contacts, accounts, payments, bank transactions, and items, plus the bulk import) send to your live Xero organisation, so the recommended agent policy is preview-then-approve. Deletes of accounts, items, and payments are destructive and removing stored auth tokens is credential-tier - keep both human-in-the-loop. The strongest control is the scope of the OAuth2 token you grant. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/xero/governance.md).

## Frequently asked questions

### Is there an MCP server for Xero?

Yes - this one. A free, open source MCP server and Claude Code Skill for Xero, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Xero MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Xero MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Xero data safe?

Your data stays on your machine - the CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Xero API rate limits?

Rarely. After one sync into the local mirror, aging, reconciliation, tie-out, exposure, ledger, and search run entirely offline - zero API calls. Only sync, explicit live reads, and writes touch the API, which Xero caps at 60 calls per minute and 5,000 per day per organisation; the local-first design is built around that limit.

### Which Xero credentials does it need, and does it create them?

You mint an OAuth2 token in the Xero developer portal - a Custom Connection is the simplest for machine-to-machine use - then pass it plus your organisation's tenant id via XERO_ACCESS_TOKEN and XERO_TENANT_ID. The CLI never creates or rotates tokens; it only uses the one you give it. Run xero-cli doctor to confirm both are set before syncing.

### Does it cover one organisation or several?

One organisation per XERO_TENANT_ID. The local mirror holds a single tenant; for a multi-entity portfolio, point the tenant id at each organisation in turn and loop.


## More Billing connectors

Run more than one Billing tool, or comparing options? These connectors work the same way: [AppDirect](/skills/appdirect/) · [Gradient MSP](/skills/gradient/) · [Maxio](/skills/maxio/) · [Pax8](/skills/pax8/) · [QuickBooks Online](/skills/quickbooks/) · [Sherweb](/skills/sherweb/)

## Status

Beta. Validated against the Xero API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
