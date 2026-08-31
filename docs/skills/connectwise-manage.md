---
layout: default
title: "ConnectWise PSA (Manage) MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every ConnectWise PSA workflow from the terminal \u2014 with a typed conditions query builder, offline SQLite sync, and cross-entity views (unbilled work, account 360, board triage) the PSA web UI can't give you."
permalink: /skills/connectwise-manage/
skill_name: "ConnectWise PSA (Manage) MCP"
image: /assets/social/connectwise-manage/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for ConnectWise PSA (Manage)?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for ConnectWise PSA (Manage), built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the ConnectWise PSA (Manage) MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CONNECTWISE_MANAGE_FEEDBACK_AUTO_SEND=true` mails feedback you wrote to the maintainers; `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but connectwise-manage-mcp speaks HTTP natively: run `connectwise-manage-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my ConnectWise Manage data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my ConnectWise API rate limits?"
    a: "The local mirror exists so reads stop hitting the API. After the first sync, the cross-entity views (unbilled, account, agreement-burn, board, stale, workload) run against local SQLite with zero API calls. Live calls respect a --rate-limit throttle. Note: ConnectWise list endpoints expose no 'changed since' parameter, so a plain sync re-fetches each resource in full rather than only what changed (you will see resource_not_incremental warnings) - on a tenant with years of history, scope the sync with a conditions filter."
  - q: "I have years of history - how do I keep sync and reads fast?"
    a: "ConnectWise list endpoints declare no temporal-filter parameter, so a plain sync re-fetches every resource in full and --since has no effect on the sync itself. Scope the sync with a conditions filter: build one with 'condition build', then name the resource with --resources and attach the condition with --resource-param, e.g. connectwise-manage-cli sync --resources service-tickets --resource-param \"service-tickets:conditions=lastUpdated > [2025-01-01T00:00:00Z]\" (sync takes no resource positional; repeat the pair per resource). For reads, stay local: the cross-entity views read the mirror, and search defaults to auto mode (live API first) so add --data-source local and/or --type <resource> to stay offline; give a slow command more room with --timeout 2m. The MCP search tool is already local-only."
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
    text: "Enter your ConnectWise PSA (Manage) credentials once, then run connectwise-manage-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a ConnectWise PSA (Manage) question in plain language; it runs connectwise-manage-cli for you."
---

# The ConnectWise PSA (Manage) MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by ConnectWise, LLC.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for ConnectWise PSA (Manage). It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects ConnectWise PSA (Manage) to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

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
| Destructive / config | connectwise-manage-cli service delete-tickets-by-id; connectwise-manage-cli company delete-companies-by-id; connectwise-manage-cli company delete-configurations-by-id | Human-in-the-loop only |

The skill drives the connectwise-manage-cli and connectwise-manage-mcp binaries, authenticating with API Member keys read from the environment (CW_COMPANY_ID, CW_PUBLIC_KEY, CW_PRIVATE_KEY, CW_CLIENT_ID) - never logged and never sent anywhere except your ConnectWise Manage instance. Read commands (boards, cross-entity views, search, reports) can change nothing. Writes are not gated by default: --dry-run is an opt-in preview flag, so the recommended policy is an agent-level rule - preview with --dry-run, show the exact command, get approval, then run the write. Keep delete and member-administration commands human-only. The strongest control is the security role on the API Member you create. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/connectwise-manage/governance.md).

## Frequently asked questions

### Is there an MCP server for ConnectWise PSA (Manage)?

Yes - this one. A free, open source MCP server and Claude Code Skill for ConnectWise PSA (Manage), built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the ConnectWise PSA (Manage) MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CONNECTWISE_MANAGE_FEEDBACK_AUTO_SEND=true` mails feedback you wrote to the maintainers; `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but connectwise-manage-mcp speaks HTTP natively: run `connectwise-manage-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my ConnectWise Manage data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my ConnectWise API rate limits?

The local mirror exists so reads stop hitting the API. After the first sync, the cross-entity views (unbilled, account, agreement-burn, board, stale, workload) run against local SQLite with zero API calls. Live calls respect a --rate-limit throttle. Note: ConnectWise list endpoints expose no 'changed since' parameter, so a plain sync re-fetches each resource in full rather than only what changed (you will see resource_not_incremental warnings) - on a tenant with years of history, scope the sync with a conditions filter.

### I have years of history - how do I keep sync and reads fast?

ConnectWise list endpoints declare no temporal-filter parameter, so a plain sync re-fetches every resource in full and --since has no effect on the sync itself. Scope the sync with a conditions filter: build one with 'condition build', then name the resource with --resources and attach the condition with --resource-param, e.g. connectwise-manage-cli sync --resources service-tickets --resource-param "service-tickets:conditions=lastUpdated > [2025-01-01T00:00:00Z]" (sync takes no resource positional; repeat the pair per resource). For reads, stay local: the cross-entity views read the mirror, and search defaults to auto mode (live API first) so add --data-source local and/or --type <resource> to stay offline; give a slow command more room with --timeout 2m. The MCP search tool is already local-only.

### Is this for ConnectWise PSA or ConnectWise Manage?

Same product - ConnectWise renamed Manage to ConnectWise PSA. This skill targets the Manage REST API (v4_6_release/apis/3.0), which is the API both names refer to. It does not cover ConnectWise Automate - that is a separate product with a separate API.

### What credentials do I need?

An API Member with public/private keys from your own Manage instance, plus a clientId from the ConnectWise developer portal. Set CW_COMPANY_ID, CW_PUBLIC_KEY, CW_PRIVATE_KEY, and CW_CLIENT_ID in your environment; CW_SITE selects your region or on-prem host. The API Member's security role is the real permission boundary - scope it to what you want the AI to reach.

### Does it work with on-premises ConnectWise Manage?

Yes. CW_SITE accepts your own server's hostname as well as the cloud region hosts; the CLI builds the standard v4_6_release/apis/3.0 base URL either way.


## More PSA connectors

Run more than one PSA tool, or comparing options? These connectors work the same way: [Autotask PSA](/skills/autotask/) · [HaloPSA](/skills/halopsa/) · [Kaseya BMS](/skills/kaseya-bms/) · [SuperOps](/skills/superops/) · [Syncro](/skills/syncro/) · [Zammad](/skills/zammad/)

## Status

Beta. Validated against the ConnectWise PSA (Manage) API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
