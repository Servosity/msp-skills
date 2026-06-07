---
layout: default
title: "Tactical RMM MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Every Tactical RMM endpoint as a typed command, plus an offline SQLite mirror and cross-entity fleet queries the web UI can't express."
permalink: /skills/tactical-rmm/
skill_name: "Tactical RMM MCP"
image: /assets/social/tactical-rmm/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Tactical RMM MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Tactical RMM data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "It's self-hosted - how does it find my server?"
    a: "You point it at your own instance: set TACTICAL_RMM_BASE_URL to your Tactical RMM API URL (for example https://api.yourdomain.com) and TRMM_API_KEY to a key from Settings > Global Settings > API Keys. Both are set once; nothing is hard-coded to a vendor cloud."
  - q: "Do I need to be a Tactical RMM customer or partner?"
    a: "No. Tactical RMM is free and open source and you self-host it. You only need an API key on your own instance; any Tactical RMM server with API access works."
  - q: "Will this hit my server's rate limits?"
    a: "Rarely. Most questions run against the local SQLite mirror after a one-time sync, so they make zero API calls. The few commands that fan out live (like services down or actions pending) are paced and capped with --max-scan-agents."
  - q: "Will this replace my Tactical RMM web UI?"
    a: "No - it complements it. The UI stays your system of record and remote-access console; this skill adds the cross-client query-and-automation layer it doesn't offer."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/tactical-rmm/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/tactical-rmm/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Tactical RMM credentials once; tactical-rmm-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Tactical RMM question in plain language; it runs tactical-rmm-cli for you."
---

# Tactical RMM + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the Tactical RMM
> API. Not affiliated with, endorsed by, or sponsored by AmidaWare LLC.

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

Ask plain-English questions about your whole self-hosted Tactical RMM fleet and get answers the web UI can't assemble in one view: which agents went dark, where patches and reboots are pending across every client, what changed overnight, and which endpoints are unmonitored. `tactical-rmm-cli` mirrors your fleet into local SQLite, then answers cross-client rollups instantly and offline - and can fan a command across a filtered cohort, preview-first.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/tactical-rmm){: .btn}

## Instead of clicking through Tactical RMM, just ask

**Instead of** Clicking through each client's agent list to find the machines that quietly stopped checking in
**just ask:** *"Which agents across all my clients have gone dark in the last week?"*
<sub>Your agent runs: <code>tactical-rmm-cli agents stale --days 7 --agent</code></sub>

**Instead of** Opening every client and site before patch night to tally pending Windows updates and the reboots they'll need
**just ask:** *"Where do I stand on pending patches and reboots across every client?"*
<sub>Your agent runs: <code>tactical-rmm-cli patch posture --by client --agent</code></sub>

**Instead of** Remoting into each machine one by one to run the same quick command across a group
**just ask:** *"Run whoami on every online Windows agent - show me the cohort before it executes"*
<sub>Your agent runs: <code>tactical-rmm-cli agents bulk-run --command whoami --filter "os=windows,online=true"</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/tactical-rmm/wide-1200x630.png" src="/assets/video/tactical-rmm/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/tactical-rmm/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What's the overall health of my fleet right now? | `tactical-rmm-cli fleet health` |
| Which agents need attention first? | `tactical-rmm-cli triage --limit 20` |
| Which agents have gone dark or stopped checking in? | `tactical-rmm-cli agents stale --days 7` |
| Where are patches and reboots pending across every client? | `tactical-rmm-cli patch posture --by client` |
| What changed across the fleet in the last few hours? | `tactical-rmm-cli since "2h"` |
| Which endpoints have no checks configured (monitoring gaps)? | `tactical-rmm-cli coverage` |
| What's each client's posture in a single row? | `tactical-rmm-cli clients scorecard` |
| Which checks are failing on the most agents? | `tactical-rmm-cli checks worst` |
| Which agents have a given software package installed? | `tactical-rmm-cli software find --name openssl` |
| Summarize alerts by severity over the last day | `tactical-rmm-cli alerts digest --since 24h --by severity` |
| Which agents have a named Windows service stopped? | `tactical-rmm-cli services down --name Spooler` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/tactical-rmm/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/tactical-rmm/guide.md).

## What makes this one different

Most Tactical RMM integrations and MCP servers proxy each question into a live API call - fine for one agent, useless for a fleet-wide rollup that would page through every endpoint against your own server. This skill syncs Tactical RMM into a local SQLite mirror, so cross-entity questions - fleet health, triage, patch posture, what-changed-since - become one offline join: instant, and the AI sees the answer, not the raw dump.

Tactical RMM's web UI is your system of record and remote-control console, and it shows one agent or one client at a time. This skill adds the cross-client, time-windowed layer the UI leaves you to assemble by hand - dark-agent sweeps, patch-posture rollups, blast-radius check rankings, and preview-first cohort runs - from one local mirror whose every line you can read.

## The pain this closes

- Tactical RMM is the budget-friendly, self-hosted favorite on r/msp and in the MSPGeek community - but the recurring gripe is reporting: the web UI shows you one agent or one client at a time, with no built-in fleet-wide view of health, patch posture, or 'what changed overnight' across every client at once.
- Everything is in the API, but pulling a cross-client rollup means scripting against it yourself - so the questions that matter at a Monday stand-up or a QBR are the ones nobody has time to assemble by hand, portal tab by portal tab.
- Self-hosting means you own all the data and pay nothing per endpoint - and also that there's no built-in analytics or BI layer to fall back on. The data is right there; the cross-fleet views aren't.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/tactical-rmm/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/tactical-rmm/install.ps1 | iex
```

After install, authenticate once with your Tactical RMM credentials, then verify with `tactical-rmm-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | Fleet rollups (fleet health, triage, patch posture, clients scorecard, coverage, since, checks worst/flapping, alerts digest, services down, software find), agent/client/site/check/alert get & list, search, sync, analytics. (Listing API keys or reading the keystore, codesign token, or core settings is the exception - those return stored secrets and are credential-tier.) | Allow |
| Write (routine config) | Create/update clients, sites, checks, alerts & templates, scripts & snippets, automation policies & patch policies, autotasks, custom fields, schedules, agent notes, deployments, and software/service records; import (bulk create/upsert from JSONL). | Preview with --dry-run, then a reviewed write |
| Endpoint & script execution | Run scripts/commands on endpoints (agents bulk-run, scripts test, autotasks/automation run), reboot/shutdown/wake/recover agents, install or scan Windows updates, uninstall software, reset checks, put cohorts into maintenance, open remote sessions (meshcentral, webvnc), and server actions (test SMS/email, generate via OpenAI). These execute code on managed machines. | Human-in-the-loop only - never unattended |
| Credential & identity | API keys (create/list/delete/update - listing returns key values), keystore and codesign tokens, core settings, users, roles, sessions, and password/2FA/TOTP resets. Listing or reading several of these returns stored secrets. | Human-in-the-loop only |
| Destructive | Delete agents, clients, sites, checks, scripts & snippets, alerts & templates, automation policies & patch policies, autotasks, custom fields, schedules, deployments, agent processes, and pending actions. | Human-in-the-loop only, explicit confirmation |

The skill reads everything across your fleet - agents, clients, sites, checks, alerts, patches, software, services, and the cross-client rollups - and through the API it can also create and update config, delete records, run scripts and commands on endpoints, reboot or shut machines down, install Windows updates, and manage users, roles, and API keys. Reads are safe to run unattended, with one exception: listing API keys or reading the keystore, codesign token, or core settings can return stored secrets, so treat those as credential-tier. Keep an autonomous agent to reads plus previewed writes; require a human for anything that runs on an endpoint, deletes, or touches credentials. The CLI can only do what your API key is permitted to do, so scope the key to the workflow. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/tactical-rmm/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Tactical RMM MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Tactical RMM data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### It's self-hosted - how does it find my server?

You point it at your own instance: set TACTICAL_RMM_BASE_URL to your Tactical RMM API URL (for example https://api.yourdomain.com) and TRMM_API_KEY to a key from Settings > Global Settings > API Keys. Both are set once; nothing is hard-coded to a vendor cloud.

### Do I need to be a Tactical RMM customer or partner?

No. Tactical RMM is free and open source and you self-host it. You only need an API key on your own instance; any Tactical RMM server with API access works.

### Will this hit my server's rate limits?

Rarely. Most questions run against the local SQLite mirror after a one-time sync, so they make zero API calls. The few commands that fan out live (like services down or actions pending) are paced and capped with --max-scan-agents.

### Will this replace my Tactical RMM web UI?

No - it complements it. The UI stays your system of record and remote-access console; this skill adds the cross-client query-and-automation layer it doesn't offer.


## Status

Beta. Validated against the Tactical RMM API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
