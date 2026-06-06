---
layout: default
title: "ConnectWise PSA (Manage) MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Every ConnectWise PSA workflow from the terminal \u2014 with a typed conditions query builder, offline SQLite sync, and cross-entity views (unbilled work, account 360, board triage) the PSA web UI can't give you."
permalink: /skills/connectwise-manage/
skill_name: "ConnectWise PSA (Manage) MCP"
image: /assets/social/connectwise-manage/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local ConnectWise Manage MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my ConnectWise Manage data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my ConnectWise API rate limits?"
    a: "The local mirror exists so reads stop hitting the API. After the first sync, the cross-entity views (unbilled, account, agreement-burn, board, stale, workload) run against local SQLite with zero API calls. Live calls respect a --rate-limit throttle, and sync is incremental - it only fetches what changed since the last checkpoint."
  - q: "Is this for ConnectWise PSA or ConnectWise Manage?"
    a: "Same product - ConnectWise renamed Manage to ConnectWise PSA. This skill targets the Manage REST API (v4_6_release/apis/3.0), which is the API both names refer to. It does not cover ConnectWise Automate - that is a separate product with a separate API."
  - q: "What credentials do I need?"
    a: "An API Member with public/private keys from your own Manage instance, plus a clientId from the ConnectWise developer portal. Set CW_COMPANY_ID, CW_PUBLIC_KEY, CW_PRIVATE_KEY, and CW_CLIENT_ID in your environment; CW_SITE selects your region or on-prem host. The API Member's security role is the real permission boundary - scope it to what you want the AI to reach."
  - q: "Does it work with on-premises ConnectWise Manage?"
    a: "Yes. CW_SITE accepts your own server's hostname as well as the cloud region hosts; the CLI builds the standard v4_6_release/apis/3.0 base URL either way."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-manage/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-manage/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your ConnectWise PSA (Manage) credentials once; connectwise-manage-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a ConnectWise PSA (Manage) question in plain language; it runs connectwise-manage-cli for you."
---

# ConnectWise PSA (Manage) + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the ConnectWise Manage
> API. Not affiliated with, endorsed by, or sponsored by ConnectWise, LLC.

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

MSPs run ConnectWise PSA (Manage) as the system of record - tickets, time, agreements, billing. Ask your AI "what's rotting on the board," "which closed tickets have no time logged," or "how burned is that block-hours agreement," and get answers the portal can't compose: cross-entity joins across tickets, time, agreements, and configurations, computed offline from a local mirror in one query instead of five portal screens and an Excel export.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/connectwise-manage){: .btn}

## Instead of clicking through ConnectWise PSA (Manage), just ask

**Instead of** Exporting closed tickets and time entries to Excel before the invoice run and VLOOKUP-ing which ones have no hours logged against them
**just ask:** *"Which tickets did we close this week with no time logged?"*
<sub>Your agent runs: <code>connectwise-manage-cli unbilled --since 7d</code></sub>

**Instead of** Opening the board, sorting by last-update, and eyeballing which tickets have sat untouched while the SLA clock runs
**just ask:** *"What's gone stale on the Help Desk board in the last three days?"*
<sub>Your agent runs: <code>connectwise-manage-cli stale --days 3 --board "Help Desk"</code></sub>

**Instead of** Clicking through contacts, agreements, configurations, and open tickets across five portal screens to prep one client call
**just ask:** *"Give me the full picture on Acme Corp before my 2 o'clock"*
<sub>Your agent runs: <code>connectwise-manage-cli account AcmeCorp</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/connectwise-manage/wide-1200x630.png" src="/assets/video/connectwise-manage/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/connectwise-manage/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which tickets did we touch this week that have zero time logged against them? | `connectwise-manage-cli unbilled --since 7d` |
| Which clients are about to blow through their block-hours agreement? | `connectwise-manage-cli agreement-burn --period 30d` |
| What does the Help Desk board look like right now - age, owner, priority? | `connectwise-manage-cli board "Help Desk"` |
| Which open tickets has nobody touched in five days? | `connectwise-manage-cli stale --days 5` |
| Who has bandwidth for the next ticket? | `connectwise-manage-cli workload` |
| Which tickets are sitting unassigned on the board? | `connectwise-manage-cli board "Help Desk" --unassigned` |
| Everything we know about one client - contacts, agreements, configurations, open tickets? | `connectwise-manage-cli account AcmeCorp` |
| Write me a valid conditions filter for open Help Desk tickets | `connectwise-manage-cli condition build --field board/name --op = --value "Help Desk" --field closedFlag --op = --value false` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/connectwise-manage/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/connectwise-manage/guide.md).

## What makes this one different

Most ConnectWise Manage integrations and MCP servers proxy every question into a live API call - fine for one ticket, but an aggregate question becomes a multi-call dance the AI burns context on, each call gated by the strict conditions syntax where one wrong quote returns silently empty results. This skill syncs the PSA into a local SQLite mirror with full-text search, so cross-entity questions (unbilled work, agreement burn, account 360) become one local join: instant, offline, and the AI sees the answer, not pages of raw JSON.

It complements ConnectWise's own AI features rather than replacing them: the portal stays best for in-app workflows, while this skill brings the PSA to whichever agent you already use and answers the cross-entity questions - tickets joined to time joined to agreements - that no single portal screen or API call composes.

## The pain this closes

- Unlogged time is direct revenue leakage - MSP consultancy Bering McKinley prices it as "lost billable hours, lower profitability, bad decision-making caused by incomplete data, and unhappy clients": techs close tickets without entering time, and nobody notices until the invoice run, because Manage has no single view joining closed tickets to logged hours.
- Cross-entity reporting means exports: native reports are widely panned (Support Adventure's ConnectWise Manage overview steers MSPs to paid BI add-ons for anything client-facing), so answering "how burned is this agreement" or "what is this client's full footprint" falls back to portal screens and Excel.
- The REST API's conditions syntax is strict and unforgiving - integration vendors maintain dedicated KB articles for "ConnectWise API Error 400: Condition Syntax Error" - and getting one quote wrong yields a 400 or, worse, silently empty results.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectwise-manage/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectwise-manage/install.ps1 | iex
```

After install, authenticate once with your ConnectWise PSA (Manage) credentials, then verify with `connectwise-manage-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | connectwise-manage-cli stale --days 5; connectwise-manage-cli board "Help Desk"; connectwise-manage-cli unbilled --since 7d; connectwise-manage-cli account AcmeCorp; connectwise-manage-cli agreement-burn; connectwise-manage-cli workload; connectwise-manage-cli search "vpn outage" | Allow |
| Write (routine) | connectwise-manage-cli time post-entries (log time); connectwise-manage-cli service post-tickets (create a ticket); connectwise-manage-cli service patch-tickets-by-id (update status or owner); connectwise-manage-cli company patch-companies-by-id - writes send immediately; --dry-run is an opt-in preview, not a default | Preview with --dry-run, then a reviewed write |
| Destructive / config | connectwise-manage-cli service delete-tickets-by-id; connectwise-manage-cli company delete-companies-by-id; connectwise-manage-cli system post-members-by-member-identifier-tokens | Human-in-the-loop only |

The skill drives the connectwise-manage-cli and connectwise-manage-mcp binaries, authenticating with API Member keys read from the environment (CW_COMPANY_ID, CW_PUBLIC_KEY, CW_PRIVATE_KEY, CW_CLIENT_ID) - never logged and never sent anywhere except your ConnectWise Manage instance. Read commands (boards, cross-entity views, search, reports) can change nothing. Writes are not gated by default: --dry-run is an opt-in preview flag, so the recommended policy is an agent-level rule - preview with --dry-run, show the exact command, get approval, then run the write. Keep delete and member-administration commands human-only. The strongest control is the security role on the API Member you create. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/connectwise-manage/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local ConnectWise Manage MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my ConnectWise Manage data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my ConnectWise API rate limits?

The local mirror exists so reads stop hitting the API. After the first sync, the cross-entity views (unbilled, account, agreement-burn, board, stale, workload) run against local SQLite with zero API calls. Live calls respect a --rate-limit throttle, and sync is incremental - it only fetches what changed since the last checkpoint.

### Is this for ConnectWise PSA or ConnectWise Manage?

Same product - ConnectWise renamed Manage to ConnectWise PSA. This skill targets the Manage REST API (v4_6_release/apis/3.0), which is the API both names refer to. It does not cover ConnectWise Automate - that is a separate product with a separate API.

### What credentials do I need?

An API Member with public/private keys from your own Manage instance, plus a clientId from the ConnectWise developer portal. Set CW_COMPANY_ID, CW_PUBLIC_KEY, CW_PRIVATE_KEY, and CW_CLIENT_ID in your environment; CW_SITE selects your region or on-prem host. The API Member's security role is the real permission boundary - scope it to what you want the AI to reach.

### Does it work with on-premises ConnectWise Manage?

Yes. CW_SITE accepts your own server's hostname as well as the cloud region hosts; the CLI builds the standard v4_6_release/apis/3.0 base URL either way.


## Status

Beta. Validated against the ConnectWise PSA (Manage) API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
