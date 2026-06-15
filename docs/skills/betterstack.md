---
layout: default
title: "Better Stack MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Better Stack Uptime feature, plus an offline SQLite mirror and cross-resource fleet analytics \u2014 what's down and who's paged, coverage gaps, MTTA/MTTR, flapping, on-call gaps, and status-page drift \u2014 that the API alone can't answer."
permalink: /skills/betterstack/
skill_name: "Better Stack MCP"
image: /assets/social/betterstack/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Better Stack?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Better Stack, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Better Stack MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Better Stack MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Better Stack data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Better Stack API rate limits?"
    a: "No. The skill syncs once into a local SQLite mirror, then answers from local data, so repeated questions never touch the API. Only sync, live writes, and the status-page resource fan-out (used by statuspage-audit) call Better Stack."
  - q: "Do I need a paid Better Stack plan?"
    a: "You need a Better Stack account with an API token. The analytics run against whatever monitors, heartbeats, incidents, on-call calendars, and status pages your plan includes - the skill reads what your token can see."
  - q: "Will this replace the Better Stack portal?"
    a: "No, it complements it. The portal is still where you configure monitors and watch live. This skill answers the cross-account questions the portal makes you click through - coverage gaps, MTTA/MTTR, on-call gaps, status-page drift - from your AI."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Better Stack credentials once; betterstack-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Better Stack question in plain language; it runs betterstack-cli for you."
---

# The Better Stack MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Better Stack.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Better Stack. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Better Stack to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "what's down and is anyone actually paged?" and get a straight answer across your whole Better Stack account: every client's monitors, heartbeats, open incidents, and on-call rotation in one view. It surfaces the silent monitors that page nobody, the noisy ones waking techs at 3am, your real MTTA/MTTR, and status pages showing green while a monitor is down.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/betterstack){: .btn}

## Instead of clicking through Better Stack, just ask

**Instead of** Open the Better Stack dashboard and click into each monitor by hand to check which ones actually have an escalation policy attached
**just ask:** *"Which monitors would page nobody if they went down right now?"*
<sub>Your agent runs: <code>betterstack-cli coverage</code></sub>

**Instead of** Export the last month of incidents to a spreadsheet to work out your average acknowledge and resolve times for the QBR
**just ask:** *"What was our MTTA and MTTR over the last 30 days, by monitor?"*
<sub>Your agent runs: <code>betterstack-cli mttr --days 30 --by-monitor --top 10</code></sub>

**Instead of** Keep the incident list open in one tab and the on-call calendar in another to see what's down and whether anyone is on for it
**just ask:** *"What's down right now and is anyone actually paged?"*
<sub>Your agent runs: <code>betterstack-cli down</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/betterstack/wide-1200x630.png" src="/assets/video/betterstack/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/betterstack/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What's down right now and is anyone actually paged? | `betterstack-cli down` |
| Which monitors would page nobody if they failed? | `betterstack-cli coverage` |
| What's our MTTA and MTTR over the last 30 days, by monitor? | `betterstack-cli mttr --days 30 --by-monitor --top 10` |
| Which monitors are the noisiest over the last week? | `betterstack-cli flapping --days 7 --top 10` |
| Is anyone actually on call right now, or is there a gap? | `betterstack-cli oncall-gaps` |
| Which heartbeats are most at risk of a silent miss? | `betterstack-cli heartbeat-risk --top 10` |
| Are any status pages green while a monitor has an open incident? | `betterstack-cli statuspage-audit` |
| How healthy is each client group right now? | `betterstack-cli group-health` |
| Give me one health board for the whole account. | `betterstack-cli fleet` |
| Which open incidents are oldest and still unacknowledged? | `betterstack-cli triage` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/betterstack/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/betterstack/guide.md).

## What makes this one different

Most Better Stack integrations proxy each question into a live API call - fine for one monitor, useless when you ask about the whole fleet. This skill syncs Better Stack into a local SQLite mirror and answers cross-resource questions - coverage gaps, MTTA/MTTR, flapping, on-call gaps, status-page drift - as one offline join the live API can't express in a single call.

The Better Stack dashboard shows one monitor, one incident, one status page at a time. This skill answers the questions that span all of them at once - what's down and who's paged, where coverage is missing, which monitors are noisy - from a local mirror your AI can query without clicking through the portal.

## The pain this closes

- A monitor with no escalation policy goes down at 2am and pages nobody. You find out when the client calls. Across dozens of client accounts, nobody can hand-check which monitors are actually wired to alert someone.
- Flapping monitors wake the on-call tech for nothing, night after night, until alert fatigue sets in and a real outage gets ignored in the noise.
- Your public status page still reads "all systems operational" while a backing monitor has an open incident, so the client sees green and then notices the outage themselves.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/betterstack/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/betterstack/install.ps1 | iex
```

After install, authenticate once with your Better Stack credentials, then verify with `betterstack-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet, down, coverage, mttr, flapping, oncall-gaps, heartbeat-risk, statuspage-audit, group-health, triage, search, and every list/get command | Allow |
| Write (routine) | monitors create/update, heartbeats create/update, monitor-groups create, heartbeat-groups create, policies create, status-pages create, status-page-sections create, incidents acknowledge/resolve, import | Preview with --dry-run, then a reviewed write |
| Destructive / config | monitors delete, heartbeats delete, incidents delete, policies delete, status-pages delete, status-page-sections delete, status-page-resources delete, monitor-groups delete, heartbeat-groups delete | Human-in-the-loop only |

The skill reads your Better Stack monitors, heartbeats, incidents, on-call calendars, and status pages, and it can create or update monitors, heartbeats, groups, policies, and status-page sections, acknowledge or resolve incidents, bulk-import records, and delete resources. Reads are always safe to allow. Routine writes - creates, updates, incident acknowledge/resolve, and import - should be previewed with --dry-run and approved. Deletes are human-in-the-loop only. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/betterstack/governance.md).

## Frequently asked questions

### Is there an MCP server for Better Stack?

Yes - this one. A free, open source MCP server and Claude Code Skill for Better Stack, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Better Stack MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Better Stack MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Better Stack data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Better Stack API rate limits?

No. The skill syncs once into a local SQLite mirror, then answers from local data, so repeated questions never touch the API. Only sync, live writes, and the status-page resource fan-out (used by statuspage-audit) call Better Stack.

### Do I need a paid Better Stack plan?

You need a Better Stack account with an API token. The analytics run against whatever monitors, heartbeats, incidents, on-call calendars, and status pages your plan includes - the skill reads what your token can see.

### Will this replace the Better Stack portal?

No, it complements it. The portal is still where you configure monitors and watch live. This skill answers the cross-account questions the portal makes you click through - coverage gaps, MTTA/MTTR, on-call gaps, status-page drift - from your AI.


## Status

Beta. Validated against the Better Stack API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
