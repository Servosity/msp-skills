---
layout: default
title: "Domotz MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Domotz endpoint, plus a local SQLite fleet mirror that answers cross-site questions."
permalink: /skills/domotz/
skill_name: "Domotz MCP"
image: /assets/social/domotz/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Domotz?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Domotz, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Domotz MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Domotz MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Domotz data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need a special Domotz plan to use the API?"
    a: "No add-on is required - you generate an API Key from the Domotz Portal under Settings > Services > API Keys on your existing account. A handful of endpoints (company areas, team moves, some RBAC) are Enterprise-plan only; everything else works on standard accounts, and the CLI returns a clear error when a plan gates an endpoint."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Domotz credentials once; domotz-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Domotz question in plain language; it runs domotz-cli for you."
---

# The Domotz MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Domotz Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Domotz. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Domotz to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "which client sites have devices down right now?" and get one answer instead of clicking through the Domotz portal Collector by Collector. domotz-cli syncs every Collector into a local mirror, so cross-site rollups - fleet health, every offline device, overnight new-device sweeps, one unified asset inventory - come back as single offline queries your agent can run in seconds.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/domotz){: .btn}

## Instead of clicking through Domotz, just ask

**Instead of** Open the Domotz portal and click into each Collector one at a time to tally which sites have devices offline
**just ask:** *"Which client sites have devices down right now?"*
<sub>Your agent runs: <code>domotz-cli fleet health --agent</code></sub>

**Instead of** Export a device list from every site and merge the spreadsheets into one asset report for the client QBR
**just ask:** *"Give me one asset inventory across every site"*
<sub>Your agent runs: <code>domotz-cli fleet inventory --csv</code></sub>

**Instead of** Walk each network by hand looking for devices that appeared overnight
**just ask:** *"Show me new devices that appeared anywhere in the last 24 hours"*
<sub>Your agent runs: <code>domotz-cli fleet new --since 24h --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/domotz/wide-1200x630.png" src="/assets/video/domotz/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/domotz/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Is anything on fire across all my sites? | `domotz-cli fleet health --agent` |
| Which Collectors (sites) are offline or degraded right now? | `domotz-cli fleet agents --agent` |
| What devices are offline across every client? | `domotz-cli fleet offline --agent` |
| What new devices appeared on any network in the last day? | `domotz-cli fleet new --since 24h --agent` |
| Give me one asset inventory across every site | `domotz-cli fleet inventory --csv` |
| Where are IP conflicts across the fleet? | `domotz-cli fleet ip-conflicts --agent` |
| Which devices can't be fully monitored (auth or SNMP gaps)? | `domotz-cli fleet unmonitored --agent` |
| How many devices of each vendor do we manage? | `domotz-cli fleet breakdown --by vendor --agent` |
| Which Collectors have gone quiet (stale sync)? | `domotz-cli fleet stale --agent` |
| Find a hostname or IP anywhere in the synced fleet | `domotz-cli search "<query>"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/domotz/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/domotz/guide.md).

## What makes this one different

Most Domotz integrations proxy each question into a live, agent-scoped API call - fine for one device, but "across all my sites" means looping every Collector and paginating each one. This skill syncs the whole fleet into a local SQLite mirror, so cross-site rollups run as a single offline query and your AI sees the answer, not raw pages of JSON.

The Domotz portal and mobile app are built for working one site at a time. This skill adds what no single portal view does: fleet-wide rollups across every Collector from the terminal, scriptable and agent-ready. It complements the portal rather than replacing it.

## The pain this closes

- Domotz is organized per-site: the portal shows one Collector at a time, so "is anything down across all my clients?" means clicking through every agent by hand.
- Building a fleet-wide asset inventory for a QBR or a client report means exporting devices from each site and stitching the spreadsheets together.
- Rogue-device and IP-conflict checks happen network by network - there's no single view that surfaces what changed across the whole fleet overnight.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/domotz/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/domotz/install.ps1 | iex
```

After install, authenticate once with your Domotz credentials, then verify with `domotz-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | Fleet rollups (health, offline, inventory, breakdown), device/agent get & list, search, status & history, counts. (Reading SNMP authentication is the exception - it returns secrets and is treated as credential-tier.) | Allow |
| Write (routine) | Create/edit monitoring objects (SNMP & TCP sensors, tags, alert-profile bindings), set inventory fields, import, control power outlets, apply device profiles | Preview with --dry-run, then a reviewed write |
| Destructive / credential | Delete agents, devices, sensors, tags; clear inventory; set or read SNMP/device credentials; RBAC user and role changes | Human-in-the-loop only |

The skill reads your Domotz fleet - Collectors, devices, variables, alerts, and topology - and can also create and delete monitoring objects, control device power outlets, and change SNMP credentials. Reads are safe to run unattended with one exception: reading SNMP authentication returns community strings and keys, so treat it like a secret. Keep an autonomous agent to reads plus previewed writes, and require a human for anything that deletes, controls hardware, or touches credentials. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/domotz/governance.md).

## Frequently asked questions

### Is there an MCP server for Domotz?

Yes - this one. A free, open source MCP server and Claude Code Skill for Domotz, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Domotz MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Domotz MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Domotz data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need a special Domotz plan to use the API?

No add-on is required - you generate an API Key from the Domotz Portal under Settings > Services > API Keys on your existing account. A handful of endpoints (company areas, team moves, some RBAC) are Enterprise-plan only; everything else works on standard accounts, and the CLI returns a clear error when a plan gates an endpoint.


## Status

Beta. Validated against the Domotz API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
