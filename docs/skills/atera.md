---
layout: default
title: "Atera MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Atera RMM + PSA endpoint, plus a local SQLite mirror that answers fleet-health, SLA, and book-of-business questions no single API call can."
permalink: /skills/atera/
skill_name: "Atera MCP"
image: /assets/social/atera/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Atera?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Atera, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Atera MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `ATERA_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `ATERA_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but atera-mcp speaks HTTP natively: run `atera-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Atera data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Atera API rate limits?"
    a: "Rarely. Most questions run against the local SQLite mirror after a one-time `sync`, so they make zero API calls. The few commands that fetch live (like `agents patch-status`) are paced under Atera's 700-requests-per-minute limit."
  - q: "Do I need to be an Atera partner?"
    a: "No. You need an Atera account and an API key created under Admin \u2192 API. Any plan that exposes the API works; nothing here requires a special partner tier."
  - q: "Will this replace my Atera portal?"
    a: "No - it complements it. The portal stays your system of record and remote-access console; this skill adds the cross-client, terminal-and-AI query layer the portal doesn't offer."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/atera/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/atera/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Atera credentials once, then run atera-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Atera question in plain language; it runs atera-cli for you."
---

# The Atera MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Atera Networks Ltd.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Atera. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Atera to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask plain-English questions about your whole Atera estate and get answers the portal can't assemble in one view: which agents went dark, which tickets are about to breach SLA, which customers are under-contracted, and what contracts expire next quarter. `atera-cli` syncs Atera into a local SQLite mirror, then answers cross-client rollups instantly and offline - from the terminal or any AI agent.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/atera){: .btn}

## Instead of clicking through Atera, just ask

**Instead of** Exporting the agents list and filtering by last-seen date to find the machines that quietly stopped reporting
**just ask:** *"Which Atera agents have gone dark in the last 30 days?"*
<sub>Your agent runs: <code>atera-cli agents stale --days 30 --agent</code></sub>

**Instead of** Opening every open ticket to eyeball which ones are closest to breaching first-response or resolution SLA
**just ask:** *"Which open Atera tickets are about to breach SLA?"*
<sub>Your agent runs: <code>atera-cli tickets sla --agent</code></sub>

**Instead of** Cross-referencing the customer list against contracts by hand to spot accounts you manage but don't bill
**just ask:** *"Which customers have managed agents but no active contract?"*
<sub>Your agent runs: <code>atera-cli customers coverage --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/atera/wide-1200x630.png" src="/assets/video/atera/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/atera/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which agents have gone offline or stopped checking in? | `atera-cli agents stale --days 30` |
| Which open tickets are closest to breaching SLA? | `atera-cli tickets sla` |
| Who is overloaded on the service desk right now? | `atera-cli tickets workload` |
| Which customers have managed agents but no active contract? | `atera-cli customers coverage` |
| What contracts expire in the next 60 days? | `atera-cli contracts expiring --days 60` |
| What's my full book of business by customer and contract mix? | `atera-cli customers book` |
| Which machines generate the most alerts over a week? | `atera-cli agents noisy --days 7` |
| What's the patch-compliance picture across the fleet? | `atera-cli agents patch-status` |
| Which machines are running an end-of-life OS? | `atera-cli agents inventory --eol` |
| What changed across agents, tickets, and alerts in the last 24 hours? | `atera-cli since 24h` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/atera/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/atera/guide.md).

## What makes this one different

Most Atera integrations proxy each question into a live API call - fine for one record, useless for a fleet-wide rollup that would page through thousands of objects against the rate limit. This skill syncs Atera into a local SQLite mirror, so cross-entity questions become one offline join: instant, rate-limit-friendly, and the AI sees the answer, not the raw dump.

Atera's portal and its add-on AI surface single records and canned reports; this skill answers the cross-client, time-windowed questions the portal leaves you to assemble by hand - dark-agent sweeps, SLA-breach queues, under-contracted accounts, and renewal calendars - from one local mirror you can read every line of.

## The pain this closes

- Atera's reporting is the consistent gripe in G2 and Capterra reviews: custom reports need workarounds, filtering is rigid, exports are clunky, and the deeper cross-client analytics sit behind higher-tier plans.
- There's no single screen that answers 'which machines went dark,' 'which tickets breach SLA next,' or 'which customers are under-contracted' across every client at once - you assemble it by hand, portal tab by portal tab.
- Pulling fleet-wide numbers through the live API means paging thousands of objects against a rate limit, so the questions that matter at a QBR are the ones nobody has time to answer.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/atera/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/atera/install.ps1 | iex
```

After install, authenticate once with your Atera credentials, then verify with `atera-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | agents stale, tickets sla, customers coverage, contracts expiring, since, search, sync, and every get-/rollup command | Allow |
| Write (routine) | tickets post/put, contacts post/put, customers post/put, contracts post/update, alerts post/resolve, devices create-*, customvalues set-*, import | Preview with --dry-run, then a reviewed write |
| Destructive / config | agents delete, tickets delete, customers delete, devices delete-*, and credential changes (auth set-token, auth setup, auth logout) | Human-in-the-loop only |

The skill reads everything - agents, tickets, customers, contracts, alerts, devices, rates, and custom fields - and can also create, update, and delete those records through the Atera API. Reads, including every cross-client rollup, are always safe to run; routine writes should be previewed with `--dry-run` and approved before they fire; deletes and credential changes are human-in-the-loop only. The CLI can only do what your Atera API key is permitted to do, so scope the key to the workflow. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/atera/governance.md).

## Frequently asked questions

### Is there an MCP server for Atera?

Yes - this one. A free, open source MCP server and Claude Code Skill for Atera, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Atera MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `ATERA_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `ATERA_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but atera-mcp speaks HTTP natively: run `atera-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Atera data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Atera API rate limits?

Rarely. Most questions run against the local SQLite mirror after a one-time `sync`, so they make zero API calls. The few commands that fetch live (like `agents patch-status`) are paced under Atera's 700-requests-per-minute limit.

### Do I need to be an Atera partner?

No. You need an Atera account and an API key created under Admin → API. Any plan that exposes the API works; nothing here requires a special partner tier.

### Will this replace my Atera portal?

No - it complements it. The portal stays your system of record and remote-access console; this skill adds the cross-client, terminal-and-AI query layer the portal doesn't offer.


## More RMM connectors

Run more than one RMM tool, or comparing options? These connectors work the same way: [Action1](/skills/action1/) · [Auvik](/skills/auvik/) · [ConnectWise Automate](/skills/connectwise-automate/) · [Datto RMM](/skills/datto-rmm/) · [ImmyBot](/skills/immybot/) · [Level](/skills/levelio/) · [N-able N-central](/skills/n-central/) · [Nerdio Manager](/skills/nerdio/) · [NinjaOne](/skills/ninjaone/) · [Tactical RMM](/skills/tactical-rmm/)

## Status

Beta. Validated against the Atera API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
