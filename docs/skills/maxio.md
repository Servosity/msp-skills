---
layout: default
title: "Maxio MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "The first open, local revenue-intelligence CLI for Maxio Advanced Billing  -  MRR waterfalls, retention, and per-client history computed offline from a SQLite mirror that outlives Maxio's deprecated Insights API."
permalink: /skills/maxio/
skill_name: "Maxio MCP"
image: /assets/social/maxio/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Maxio?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Maxio, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Maxio MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Maxio MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Maxio data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Maxio API rate limits?"
    a: "Rarely. After the first 'maxio-cli sync', the revenue commands read from the local SQLite mirror, not the API, so day-to-day questions make zero API calls. Only sync and tail talk to Maxio, and sync is incremental - it fetches only what changed since the last run."
  - q: "Do I need to be a Maxio partner or customer to use this?"
    a: "You need your own Maxio Advanced Billing API credentials (a username and password) - that's it. This is an unofficial, community-built skill, not a Maxio product, so there is no partner program or separate signup. It reads only the data your credentials already grant."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/maxio/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/maxio/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Maxio credentials once; maxio-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Maxio question in plain language; it runs maxio-cli for you."
---

# The Maxio MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Maxio LLC.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Maxio. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Maxio to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask 'what's our MRR, and how did it move last quarter?' and get the New / Expansion / Contraction / Churn / Reactivation waterfall, net and gross revenue retention, per-customer revenue history, and a ranked list of accounts that need attention - in one command, computed offline from a local mirror of your Maxio Advanced Billing data. It is the revenue math the dashboard buries and Maxio's deprecated Insights API no longer reconstructs.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/maxio){: .btn}

## Instead of clicking through Maxio, just ask

**Instead of** Exporting subscription CSVs into a spreadsheet every month to rebuild the MRR movement waterfall by hand
**just ask:** *"Show me the MRR waterfall by month since January"*
<sub>Your agent runs: <code>maxio-cli mrr waterfall --since 2026-01-01 --group-by month</code></sub>

**Instead of** Clicking through the portal account by account to spot who is contracting, churning, or up for a big renewal
**just ask:** *"Which accounts need my attention this week?"*
<sub>Your agent runs: <code>maxio-cli triage --limit 20</code></sub>

**Instead of** Pulling Maxio data into a separate BI tool just to compute net revenue retention
**just ask:** *"What's our net and gross revenue retention this year?"*
<sub>Your agent runs: <code>maxio-cli retention --since 2026-01-01 --group-by month</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/maxio/wide-1200x630.png" src="/assets/video/maxio/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/maxio/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What's our current MRR and ARR right now? | `maxio-cli mrr now` |
| How did MRR move month over month (new, expansion, contraction, churn, reactivation)? | `maxio-cli mrr waterfall --since 2026-01-01 --group-by month` |
| What's our net and gross revenue retention, and quick ratio? | `maxio-cli retention --since 2026-01-01 --group-by month` |
| What's recurring revenue for this customer, and how has it trended? | `maxio-cli mrr client --customer "Acme"` |
| Which accounts need attention this week? | `maxio-cli triage --limit 20` |
| How much MRR is up for renewal in the next 30 days? | `maxio-cli renewals --within 30` |
| Which usage components drove expansion versus contraction? | `maxio-cli usage-drivers --since 2026-01-01` |
| Where does normalized MRR diverge from what we actually invoiced? | `maxio-cli reconcile --since 2026-01-01` |
| How many new logos did we land this quarter, and how much MRR? | `maxio-cli new-customers --since 3m` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/maxio/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/maxio/guide.md).

## What makes this one different

Most Maxio integrations proxy each question into a live API call - fine for reading one record, useless for trending, because the API has no endpoint for historic MRR movement or retention. This skill syncs Maxio into a local SQLite mirror and snapshots each sync into a time series, so the waterfall, retention curves, and cohort history are computed locally and keep growing even as Maxio retires its Insights endpoints.

The Maxio dashboard shows point-in-time numbers behind clicks and exports; this skill answers the trended, cross-object revenue questions in one command from the terminal or your AI agent, and keeps the history on your own machine rather than depending on a reporting API Maxio is deprecating. It complements the portal, which still owns billing operations and configuration.

## The pain this closes

- Maxio's recurring-revenue reporting - MRR, churn, ARPA, retention - is thin enough that operators routinely export the data into a separate BI tool to answer board questions (a recurring theme in Maxio's G2 reviews).
- Simple asks like 'list the customers who signed up this quarter and the revenue they brought' have no one-click answer in the portal, and the legacy Insights/Analytics endpoints are being sunset.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/maxio/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/maxio/install.ps1 | iex
```

After install, authenticate once with your Maxio credentials, then verify with `maxio-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | mrr now, mrr waterfall, retention, cohort, triage, reconcile, renewals, churn, new-customers, search | Allow |
| Write (routine) | subscriptions-json create-subscription, customers update, components-json | Preview with --dry-run, then a reviewed write |
| Destructive / config | customers delete, subscription-groups delete, payment-profiles delete-unused, reason-codes delete | Human-in-the-loop only |

Read commands - every MRR, retention, cohort, triage, and reconcile rollup plus search - cannot change anything and are safe to run unattended. Routine writes (creating or updating subscriptions, components, or customers) send immediately unless you pass --dry-run first, so the recommended agent policy is preview-then-approve. Credential, destructive (deletes and cancellations), and admin commands should always be human-in-the-loop. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/maxio/governance.md).

## Frequently asked questions

### Is there an MCP server for Maxio?

Yes - this one. A free, open source MCP server and Claude Code Skill for Maxio, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Maxio MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Maxio MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Maxio data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Maxio API rate limits?

Rarely. After the first 'maxio-cli sync', the revenue commands read from the local SQLite mirror, not the API, so day-to-day questions make zero API calls. Only sync and tail talk to Maxio, and sync is incremental - it fetches only what changed since the last run.

### Do I need to be a Maxio partner or customer to use this?

You need your own Maxio Advanced Billing API credentials (a username and password) - that's it. This is an unofficial, community-built skill, not a Maxio product, so there is no partner program or separate signup. It reads only the data your credentials already grant.


## More Billing connectors

Run more than one Billing tool, or comparing options? These connectors work the same way: [AppDirect](/skills/appdirect/) · [Gradient MSP](/skills/gradient/) · [Pax8](/skills/pax8/) · [QuickBooks Online](/skills/quickbooks/) · [Sherweb](/skills/sherweb/) · [Xero](/skills/xero/)

## Status

Beta. Validated against the Maxio API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
