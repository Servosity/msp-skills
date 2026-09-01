---
layout: default
title: "Level MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Level RMM endpoint, plus a local SQLite fleet store and offline cross-entity rollups no Level tool has: at-risk ranking, patch posture, alert triage, and stale-device detection in one command."
permalink: /skills/levelio/
skill_name: "Level MCP"
image: /assets/social/levelio/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Level?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Level, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Level MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Two paths can move data off the machine, both opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `LEVELIO_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `LEVELIO_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file). Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and levelio-mcp speaks stdio only, so you publish it over HTTPS with the supergateway bridge (`npx -y supergateway --stdio levelio-mcp --port 7777`) behind a tunnel or reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Level data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Level API rate limits?"
    a: "Only 'sync' calls the Level API, and it paginates politely. Every report, rollup, and search after that runs against the local SQLite mirror - zero API calls, no rate-limit pressure. Re-sync when you want fresh data."
  - q: "Do I need a paid Level plan or to be a Level partner?"
    a: "You need a Level account and an API key (Settings > API keys). A read-only key is enough for every report and rollup here, and you can scope it tighter than your portal login. The skill itself is free and open source."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/levelio/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/levelio/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Level credentials once, then run levelio-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Level question in plain language; it runs levelio-cli for you."
---

# The Level MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Level.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Level. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Level to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask "which Level endpoints are most at risk right now?" and get one ranked list across active alerts, missing patches, low security scores, and dark devices - not four portal tabs. levelio-cli syncs your whole Level fleet into a local mirror, then answers portfolio-wide questions the portal shows one device at a time: patch exposure, stale agents, alert clusters, and per-client QBR scorecards. Offline, instant, and read-only-safe.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/levelio){: .btn}

## Instead of clicking through Level, just ask

**Instead of** Clicking through Level device by device to work out which endpoints need attention first across alerts, patches, and security scores
**just ask:** *"Which Level devices are most at risk right now?"*
<sub>Your agent runs: <code>levelio-cli at-risk --top 20 --agent</code></sub>

**Instead of** Filtering the device list by last-seen date to catch the machines that quietly stopped checking in
**just ask:** *"Which Level devices have gone dark in the last 30 days?"*
<sub>Your agent runs: <code>levelio-cli stale --days 30 --agent</code></sub>

**Instead of** Opening each client's group and tallying patch exposure and open criticals by hand for the QBR deck
**just ask:** *"Give me a per-client posture scorecard for my Level fleet"*
<sub>Your agent runs: <code>levelio-cli client-scorecard --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/levelio/wide-1200x630.png" src="/assets/video/levelio/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/levelio/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which devices are most at risk across alerts, patches, score, and staleness? | `levelio-cli at-risk --top 20` |
| Which devices have gone dark and stopped checking in? | `levelio-cli stale --days 30` |
| What is my fleet-wide patch exposure, by category? | `levelio-cli patch-posture --category security` |
| How is my fleet broken down by OS, platform, or group? | `levelio-cli fleet --by os` |
| Where are my active critical fires, clustered by group? | `levelio-cli alert-triage --severity critical` |
| Give me a per-client posture scorecard for QBRs. | `levelio-cli client-scorecard` |
| Which devices are below my security-score threshold? | `levelio-cli security-posture --below 70` |
| Which devices are waiting on a reboot to finish patching? | `levelio-cli reboot-due` |
| Which monitors fire most often across the fleet? | `levelio-cli alert-recurrence --top 15` |
| What changed since yesterday - new alerts, updates, device activity? | `levelio-cli since --days 1` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/levelio/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/levelio/guide.md).

## What makes this one different

Most Level integrations and MCP servers proxy each question into a single live API call - fine for one device, useless for 'across the whole fleet.' This skill syncs Level into a local SQLite mirror with full-text search, so a portfolio-wide question becomes one offline join: instant, rate-limit-free, and the AI sees the answer, not raw bulk data.

Level's portal surfaces devices one at a time and does not roll exposure up across every client at once; this skill answers the cross-entity, time-windowed questions - at-risk ranking, fleet-wide patch posture, dark-device sweeps, and per-client QBR scorecards - from one local mirror you can read every line of.

## The pain this closes

- The Level portal answers one device at a time - there is no single screen for 'which endpoints across every client need attention first.'
- Patch exposure and reboot debt live per-device; rolling them up across the whole fleet for a QBR means tallying by hand.
- Dark devices and alert noise hide in the queue, so the systemic fires never separate themselves from one-off blips.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/levelio/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/levelio/install.ps1 | iex
```

After install, authenticate once with your Level credentials, then verify with `levelio-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | at-risk, patch-posture, fleet, alert-triage, stale, client-scorecard, security-posture, every list/show, search, sync | Allow |
| Write (routine) | devices update, groups/tags/custom-fields create and update, custom-field-values update, alerts resolve, import | Preview with --dry-run, then a reviewed write |
| Destructive / device actions | devices/groups/tags/custom-fields delete, automations (runs scripts on endpoints), auth set-token | Human-in-the-loop only |

The skill reads everything - reports, rollups, search, and a sync to a local mirror that never writes back to Level. It can also change Level records when you let it: update devices, manage groups, tags, and custom fields, resolve alerts, import data, and trigger automations that run on real endpoints. Recommended policy: give autonomous agents a read-only API key, preview every write with --dry-run, and keep a human in the loop for automations, deletes, and credential changes. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/levelio/governance.md).

## Frequently asked questions

### Is there an MCP server for Level?

Yes - this one. A free, open source MCP server and Claude Code Skill for Level, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Level MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Two paths can move data off the machine, both opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `LEVELIO_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `LEVELIO_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file). Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and levelio-mcp speaks stdio only, so you publish it over HTTPS with the supergateway bridge (`npx -y supergateway --stdio levelio-mcp --port 7777`) behind a tunnel or reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Level data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Level API rate limits?

Only 'sync' calls the Level API, and it paginates politely. Every report, rollup, and search after that runs against the local SQLite mirror - zero API calls, no rate-limit pressure. Re-sync when you want fresh data.

### Do I need a paid Level plan or to be a Level partner?

You need a Level account and an API key (Settings > API keys). A read-only key is enough for every report and rollup here, and you can scope it tighter than your portal login. The skill itself is free and open source.


## More RMM connectors

Run more than one RMM tool, or comparing options? These connectors work the same way: [Action1](/skills/action1/) · [Atera](/skills/atera/) · [Auvik](/skills/auvik/) · [ConnectWise Automate](/skills/connectwise-automate/) · [Datto RMM](/skills/datto-rmm/) · [ImmyBot](/skills/immybot/) · [N-able N-central](/skills/n-central/) · [Nerdio Manager](/skills/nerdio/) · [NinjaOne](/skills/ninjaone/) · [Tactical RMM](/skills/tactical-rmm/)

## Status

Beta. Validated against the Level API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
