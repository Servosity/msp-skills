---
layout: default
title: "NinjaOne MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every NinjaOne report, plus a local store that answers fleet-wide questions no single API call can: patch compliance, backup gaps, AV blast-radius, health, drift."
permalink: /skills/ninjaone/
skill_name: "NinjaOne MCP"
image: /assets/social/ninjaone/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for NinjaOne?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for NinjaOne, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the NinjaOne MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local NinjaOne MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my NinjaOne data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "Does it work with the US, EU, and other NinjaOne regions?"
    a: "Yes. The US host (https://app.ninjarmm.com) is the default. For another region set NINJAONE_BASE_URL (for example https://eu.ninjarmm.com) and NINJAONE_TOKEN_URL (for example https://eu.ninjarmm.com/ws/oauth/token); run 'ninjaone-cli doctor' to confirm the credentials reach your instance."
  - q: "Will this hit my NinjaOne API rate limits?"
    a: "The local mirror exists so reads stop hitting the API. After the first 'sync', the fleet views (patch-compliance, backup-coverage, av-sweep, fleet-health, stale-devices, os-eol, software-audit, drift) run against local SQLite with zero API calls. Live calls respect a --rate-limit throttle, and sync is incremental and resumable - it only fetches what changed and treats resources your token cannot reach as warnings, not failures."
  - q: "Do I need to be a NinjaOne partner or buy the reporting add-on?"
    a: "No. You need an API app you create yourself under Administration > Apps > API, with an OAuth2 client_id and client_secret. The fleet rollups are computed locally from whatever your token can already read - no reporting add-on, data warehouse, or partner tier required."
  - q: "Can it change things in NinjaOne, or is it read-only?"
    a: "The headline fleet views are read-only. The CLI also wraps NinjaOne's write surface (for example updating an organization, creating a ticket, running a script, or rebooting a device) and a smaller destructive tier (deletes). Preview any write with --dry-run, keep a human in the loop, and scope the API token to only what your workflow needs."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/ninjaone/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/ninjaone/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your NinjaOne credentials once; ninjaone-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a NinjaOne question in plain language; it runs ninjaone-cli for you."
---

# The NinjaOne MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by NinjaOne, LLC.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for NinjaOne. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects NinjaOne to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

NinjaOne is built for real-time, per-device RMM, but its reporting answers one organization at a time. Ask your AI "which clients are below 95% patched," "which endpoints across the fleet have no backup," or "how far did this threat spread" and get fleet-wide rollups computed offline from a local SQLite mirror of your estate - one query instead of a per-org report re-totaled in a spreadsheet or a third-party BI overlay.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/ninjaone){: .btn}

## Instead of clicking through NinjaOne, just ask

**Instead of** Running NinjaOne's per-organization patch report for each client and re-totaling the results in a spreadsheet to find who is behind
**just ask:** *"Which clients are below 95% patch compliance right now?"*
<sub>Your agent runs: <code>ninjaone-cli patch-compliance --min-pct 95</code></sub>

**Instead of** Opening each device's backup tab, or exporting the backup-usage report, to find the endpoints that have no protection at all
**just ask:** *"Which endpoints across the whole fleet have no backup?"*
<sub>Your agent runs: <code>ninjaone-cli backup-coverage</code></sub>

**Instead of** Cross-checking a single threat detection against the device list by hand to see how far across your clients it spread
**just ask:** *"Every device carrying this threat, fleet-wide"*
<sub>Your agent runs: <code>ninjaone-cli av-sweep --threat "Trojan.Generic"</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/ninjaone/wide-1200x630.png" src="/assets/video/ninjaone/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/ninjaone/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which organizations are below 95% patch compliance? | `ninjaone-cli patch-compliance --min-pct 95` |
| Which endpoints across the fleet have no backup at all? | `ninjaone-cli backup-coverage` |
| Which devices is a given threat on, fleet-wide? | `ninjaone-cli av-sweep --threat "Trojan.Generic"` |
| Which devices have antivirus definitions older than a week? | `ninjaone-cli av-sweep --definition-stale-days 7` |
| What is each organization's overall health score, and why? | `ninjaone-cli fleet-health` |
| Which devices have not checked in for two weeks? | `ninjaone-cli stale-devices --days 14` |
| Which devices are running an end-of-life operating system? | `ninjaone-cli os-eol` |
| Where is a software title sprawling across too many versions? | `ninjaone-cli software-audit --min-versions 3` |
| Did patch compliance get better or worse since last week? | `ninjaone-cli drift --metric patch` |
| Search every synced device, organization, and alert for a string? | `ninjaone-cli search "disk full"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/ninjaone/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/ninjaone/guide.md).

## What makes this one different

Most NinjaOne integrations and MCP servers proxy each question into a live API call - fine for one device, but a fleet question becomes a multi-call dance the AI burns context on, and the API only ever returns per-device rows. This skill syncs your whole estate into a local SQLite mirror with full-text search, so cross-fleet questions - patch compliance by organization, unprotected-device detection, AV blast-radius, OS end-of-life, software sprawl, week-over-week drift - become one local join: instant, offline, and the AI sees the answer, not pages of raw JSON.

It complements NinjaOne rather than replacing it: the console stays best for real-time monitoring, remote control, and remediation, while this skill brings your estate to whichever AI agent you already use and answers the cross-fleet rollups - joined across every organization - that NinjaOne's per-org reporting does not compose in a single view today.

## The pain this closes

- NinjaOne is strong at real-time per-device monitoring, but its built-in reporting is not consistently executive-ready and client-facing rollups often need effort or a third-party BI overlay (Flamingo, NinjaOne Review, 2026) - so "which clients are behind on patches" still means running a per-org report and re-totaling it by hand.
- There is no single unified multi-tenant view: managing many client organizations means a fleet-wide answer is assembled across screens or pushed onto the separately-handled reporting path (Flamingo, NinjaOne vs Intune, 2026), which is exactly the cross-client question MSP owners ask most.
- The API returns per-device rows; any fleet answer - unprotected endpoints, AV blast-radius, OS end-of-life exposure, software version sprawl, or week-over-week trend - has to be fetched, cached, and joined locally rather than asked of the API in one call.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/ninjaone/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/ninjaone/install.ps1 | iex
```

After install, authenticate once with your NinjaOne credentials, then verify with `ninjaone-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | ninjaone-cli patch-compliance --min-pct 95; ninjaone-cli backup-coverage; ninjaone-cli av-sweep --threat "Trojan.Generic"; ninjaone-cli fleet-health; ninjaone-cli stale-devices --days 14; ninjaone-cli os-eol; ninjaone-cli software-audit --min-versions 3; ninjaone-cli drift --metric patch; ninjaone-cli search "disk full" | Allow |
| Write (routine) | ninjaone-cli organization update; ninjaone-cli ticketing create; ninjaone-cli contact update; ninjaone-cli device reboot; ninjaone-cli device script run-on-device; ninjaone-cli tag set-for-asset (84 write commands in total) | Preview with --dry-run, then a reviewed write |
| Destructive / config | ninjaone-cli contact delete; ninjaone-cli organization delete-client-document; ninjaone-cli knowledgebase delete-knowledge-base-articles; ninjaone-cli itam delete-unmanaged-device-public-api (18 destructive commands in total) | Human-in-the-loop only |

The skill drives the ninjaone-cli and ninjaone-mcp binaries, authenticating with NINJAONE_CLIENT_ID and NINJAONE_CLIENT_SECRET (NINJAONE_OAUTH_SCOPE optionally narrows the token) read from the environment - never logged, never sent anywhere except the NinjaOne API. Every fleet view, report, and search is read-only. The CLI also exposes routine write commands and a smaller destructive tier; the recommended policy is to preview a write with --dry-run, show the request, get approval, then run. The strongest control is the scope of the API app you mint. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/ninjaone/governance.md).

## Frequently asked questions

### Is there an MCP server for NinjaOne?

Yes - this one. A free, open source MCP server and Claude Code Skill for NinjaOne, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the NinjaOne MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local NinjaOne MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my NinjaOne data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### Does it work with the US, EU, and other NinjaOne regions?

Yes. The US host (https://app.ninjarmm.com) is the default. For another region set NINJAONE_BASE_URL (for example https://eu.ninjarmm.com) and NINJAONE_TOKEN_URL (for example https://eu.ninjarmm.com/ws/oauth/token); run 'ninjaone-cli doctor' to confirm the credentials reach your instance.

### Will this hit my NinjaOne API rate limits?

The local mirror exists so reads stop hitting the API. After the first 'sync', the fleet views (patch-compliance, backup-coverage, av-sweep, fleet-health, stale-devices, os-eol, software-audit, drift) run against local SQLite with zero API calls. Live calls respect a --rate-limit throttle, and sync is incremental and resumable - it only fetches what changed and treats resources your token cannot reach as warnings, not failures.

### Do I need to be a NinjaOne partner or buy the reporting add-on?

No. You need an API app you create yourself under Administration > Apps > API, with an OAuth2 client_id and client_secret. The fleet rollups are computed locally from whatever your token can already read - no reporting add-on, data warehouse, or partner tier required.

### Can it change things in NinjaOne, or is it read-only?

The headline fleet views are read-only. The CLI also wraps NinjaOne's write surface (for example updating an organization, creating a ticket, running a script, or rebooting a device) and a smaller destructive tier (deletes). Preview any write with --dry-run, keep a human in the loop, and scope the API token to only what your workflow needs.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## More RMM connectors

Run more than one RMM tool, or comparing options? These connectors work the same way: [Action1](/skills/action1/) · [Atera](/skills/atera/) · [Datto RMM](/skills/datto-rmm/) · [Level](/skills/levelio/) · [N-able N-central](/skills/n-central/) · [Nerdio Manager](/skills/nerdio/) · [Tactical RMM](/skills/tactical-rmm/)

## Status

Beta. Validated against the NinjaOne API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
