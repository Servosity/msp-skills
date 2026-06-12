---
layout: default
title: "Rootly MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Rootly incident, alert, and on-call object as a typed command - plus a local SQLite mirror that answers related-incident, MTTR, coverage-gap, and on-call questions offline and instantly."
permalink: /skills/rootly/
skill_name: "Rootly MCP"
image: /assets/social/rootly/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Rootly?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Rootly, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Rootly MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Rootly MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Rootly data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Rootly API rate limits?"
    a: "Rarely. Read questions run against the local SQLite mirror, not the API - you sync once, then analytics, search, and the on-call views are offline. Only sync and live writes call Rootly."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rootly/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rootly/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Rootly credentials once; rootly-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Rootly question in plain language; it runs rootly-cli for you."
---

# The Rootly MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Rootly, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Rootly. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Rootly to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask "who's on call for this service right now?" or "what's our MTTR by service this quarter?" and get the answer in one command. Rootly syncs to a local SQLite mirror, so incident-similarity, resolution mining, on-call coverage audits, and MTTA/MTTR analytics run instantly and offline - no portal clicking, no per-question API call.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/rootly){: .btn}

## Instead of clicking through Rootly, just ask

**Instead of** Clicking through Rootly service by service the night before a reliability review to stitch together an MTTR report
**just ask:** *"What's our mean time to acknowledge and resolve by service over the last quarter?"*
<sub>Your agent runs: <code>rootly-cli mttr --by service --since 90d</code></sub>

**Instead of** Opening every on-call schedule by hand to find which weekends and holidays have nobody covering
**just ask:** *"Where does an on-call schedule have an unstaffed gap in the next two weeks?"*
<sub>Your agent runs: <code>rootly-cli coverage-gaps --days 14</code></sub>

**Instead of** Pinging the team in Slack to find out who is actually on call for a service mid-incident
**just ask:** *"Who's on call right now across every service, escalation tier included?"*
<sub>Your agent runs: <code>rootly-cli oncall-now</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/rootly/wide-1200x630.png" src="/assets/video/rootly/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/rootly/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Who's on call right now across every service and schedule? | `rootly-cli oncall-now` |
| What past incidents are most similar to this one? | `rootly-cli related <incident-id>` |
| What actually fixed this service the last time it broke? | `rootly-cli fixed-last-time <service>` |
| What's our MTTA and MTTR by service this quarter? | `rootly-cli mttr --by service --since 90d` |
| Where does an on-call schedule have an unstaffed gap? | `rootly-cli coverage-gaps --days 14` |
| Is it safe to deploy this service right now? | `rootly-cli deploy-guard <service>` |
| Give me one screen for this active incident. | `rootly-cli war-room <incident-id>` |
| Which incidents are breaching or about to breach SLA? | `rootly-cli sla-breach --within 2h` |
| Which open action items are overdue, grouped by owner? | `rootly-cli action-items-overdue --group-by owner` |
| Draft a paste-ready post-mortem skeleton for this incident. | `rootly-cli postmortem-skeleton <incident-id>` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/rootly/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/rootly/guide.md).

## What makes this one different

Most Rootly integrations and MCP servers proxy each question into a live API call - fine for reading one incident, costly for "MTTR by service across the quarter" or "find the incidents most similar to this one," which otherwise means a call per object. This skill syncs Rootly into a local SQLite mirror, so cross-incident analytics (mttr, related, fixed-last-time, coverage-gaps, service-health) become one offline join: instant, rate-limit-free, and the AI sees the computed answer, not a raw data dump.

Rootly's own AI and analytics live in the web app; this skill puts the post-incident math - MTTA/MTTR, incident similarity, resolution mining, on-call coverage and escalation-gap checks - in your terminal and your AI agent, computed offline from data any API key can read. It complements the Rootly portal; it does not replace your account.

## The pain this closes

- On-call responders drown in alerts and burn out - the predictable result when most pages don't matter and the real incident hides in the noise (Catchpoint SRE Report).
- The reliability numbers leadership asks for at the review - MTTA, MTTR, who is carrying the on-call load - get hand-assembled from exports the night before.
- Nobody notices the hole in next week's on-call schedule until an incident finds it first.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/rootly/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/rootly/install.ps1 | iex
```

After install, authenticate once with your Rootly credentials, then verify with `rootly-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | oncall-now, mttr, related, coverage-gaps, sla-breach, service-health, search, and any non-secret list/get (not secrets/api-keys, which return stored credentials) | Allow |
| Write (routine) | incidents create, incidents update, incidents resolve, schedules update, import (bulk create/upsert from JSONL) | Preview with --dry-run, then a reviewed write |
| Credential / security | secrets, api-keys rotate | Human-in-the-loop only |
| Destructive / config | incidents delete, schedules delete | Human-in-the-loop only |

The skill reads incidents, alerts, schedules, and on-call data, and can create or update incidents and related objects - but never changes anything unless you ask. Reads are always safe to run; routine writes should be previewed with --dry-run and then approved; credential, destructive, and config commands (secrets, deletes, key rotation) are human-in-the-loop only. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/rootly/governance.md).

## Frequently asked questions

### Is there an MCP server for Rootly?

Yes - this one. A free, open source MCP server and Claude Code Skill for Rootly, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Rootly MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Rootly MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Rootly data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Rootly API rate limits?

Rarely. Read questions run against the local SQLite mirror, not the API - you sync once, then analytics, search, and the on-call views are offline. Only sync and live writes call Rootly.


## More Incident Response connectors

Run more than one Incident Response tool, or comparing options? These connectors work the same way: [PagerDuty](/skills/pagerduty/)

## Status

Beta. Validated against the Rootly API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
