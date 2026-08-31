---
layout: default
title: "Resource Guru MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Resource Guru endpoint as a typed command, plus a local database and per-resource per-day utilization no Resource Guru report gives you."
permalink: /skills/resourceguru/
skill_name: "Resource Guru MCP"
image: /assets/social/resourceguru/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Resource Guru?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Resource Guru, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Resource Guru MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `RESOURCEGURU_FEEDBACK_AUTO_SEND=true` mails feedback you wrote to the maintainers; `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but resourceguru-mcp speaks HTTP natively: run `resourceguru-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Resource Guru data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Resource Guru API rate limits?"
    a: "No. After the first sync, utilization, overbooked, bench, capacity, and search all read the local SQLite mirror with zero API calls. You only touch the API when you sync or run a live read, and the CLI honors a configurable --rate-limit."
  - q: "Do I need to be a Resource Guru admin?"
    a: "No. You need an account that can read the schedule, authenticated with your account email and password over HTTP Basic. Read-only analytics never need write or admin scope - scope the credential to what you actually use."
  - q: "Will this replace the Resource Guru web app?"
    a: "No. It reports and analyzes; it is not where you edit the schedule. Writes go through the same API the web app uses, so confirm any mutation in Resource Guru afterward. The unique value is the per-day, fleet-wide utilization the portal does not surface."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/resourceguru/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/resourceguru/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Resource Guru credentials once, then run resourceguru-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Resource Guru question in plain language; it runs resourceguru-cli for you."
---

# The Resource Guru MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Resource Guru Limited.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Resource Guru. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Resource Guru to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI 'who is overbooked next week?' or 'who has capacity for this project?' and get an answer Resource Guru's reports never show: booked-vs-available utilization for every resource on every day. The skill syncs your whole schedule into a local mirror, then computes per-day utilization, fleet-wide overbooking, who is on the bench, and remaining capacity offline - no report export, no pivot table.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/resourceguru){: .btn}

## Instead of clicking through Resource Guru, just ask

**Instead of** Download a booking report and pivot it in a spreadsheet to eyeball who is overcommitted day by day
**just ask:** *"Who is overbooked between June 1 and June 30?"*
<sub>Your agent runs: <code>resourceguru-cli overbooked --start 2026-06-01 --end 2026-06-30 --agent</code></sub>

**Instead of** Click through each person's schedule to guess who has room for new work
**just ask:** *"Who has slack next week to take on a new project?"*
<sub>Your agent runs: <code>resourceguru-cli bench --start 2026-06-08 --end 2026-06-14 --threshold 50 --agent</code></sub>

**Instead of** Open the calendar and add up free hours by hand before promising a deadline
**just ask:** *"How much bookable capacity is left in July?"*
<sub>Your agent runs: <code>resourceguru-cli capacity --start 2026-07-01 --end 2026-07-31 --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/resourceguru/wide-1200x630.png" src="/assets/video/resourceguru/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/resourceguru/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Who is overbooked across the whole fleet this month? | `resourceguru-cli overbooked --start 2026-06-01 --end 2026-06-30 --agent` |
| What is each person's utilization, day by day? | `resourceguru-cli utilization --start 2026-06-01 --end 2026-06-30 --heatmap --agent` |
| Who has slack to take on new work next week? | `resourceguru-cli bench --start 2026-06-08 --end 2026-06-14 --threshold 50 --agent` |
| How much bookable capacity is left next month? | `resourceguru-cli capacity --start 2026-07-01 --end 2026-07-31 --agent` |
| What changed on the schedule in the last week? | `resourceguru-cli since 7d --agent` |
| How is workload distributed across the team? | `resourceguru-cli load --json` |
| Find a booking, client, or project anywhere in the synced schedule | `resourceguru-cli search "website redesign"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/resourceguru/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/resourceguru/guide.md).

## What makes this one different

Most Resource Guru integrations proxy each question into a single live API call - fine for one record, useless for 'every resource, every day, this quarter.' This skill syncs your whole schedule into a local SQLite mirror first, so utilization, overbooked, bench, and capacity are computed offline across the entire fleet in one pass, and your agent sees the answer instead of raw bulk data.

Resource Guru's portal answers 'is this person free right now' and its reports give range-level rollups; this skill adds the per-day, fleet-wide utilization math the reports never expose and hands it to your AI agent as a typed command instead of a spreadsheet export.

## The pain this closes

- Resource Guru's reporting gives range-level rollups and has to be downloaded and pivoted in a spreadsheet to answer day-level questions about who is over- or under-booked.
- The booking-clash warning flags one over-allocation at a time on write, but there is no single view of every overcommitted resource-day across the whole fleet for a window.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/resourceguru/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/resourceguru/install.ps1 | iex
```

After install, authenticate once with your Resource Guru credentials, then verify with `resourceguru-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | utilization, overbooked, bench, capacity, since, load, search, accounts list, and any get | Allow |
| Write (routine) | bookings create, bookings update, clients create, projects update, activity-types create | Preview with --dry-run, then a reviewed write |
| Destructive / config | bookings delete, clients delete, resources delete, calendars delete, webhooks delete | Human-in-the-loop only |

The skill reads your schedule and computes analytics locally, and it can also create, update, and delete bookings, clients, projects, and other objects through the same API the web app uses. Reads are always safe. Keep autonomous agents to reads plus previewed (--dry-run) writes, and require a human for any delete or configuration change. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/resourceguru/governance.md).

## Frequently asked questions

### Is there an MCP server for Resource Guru?

Yes - this one. A free, open source MCP server and Claude Code Skill for Resource Guru, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Resource Guru MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `RESOURCEGURU_FEEDBACK_AUTO_SEND=true` mails feedback you wrote to the maintainers; `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but resourceguru-mcp speaks HTTP natively: run `resourceguru-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Resource Guru data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Resource Guru API rate limits?

No. After the first sync, utilization, overbooked, bench, capacity, and search all read the local SQLite mirror with zero API calls. You only touch the API when you sync or run a live read, and the CLI honors a configurable --rate-limit.

### Do I need to be a Resource Guru admin?

No. You need an account that can read the schedule, authenticated with your account email and password over HTTP Basic. Read-only analytics never need write or admin scope - scope the credential to what you actually use.

### Will this replace the Resource Guru web app?

No. It reports and analyzes; it is not where you edit the schedule. Writes go through the same API the web app uses, so confirm any mutation in Resource Guru afterward. The unique value is the per-day, fleet-wide utilization the portal does not surface.


## Status

Beta. Validated against the Resource Guru API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
