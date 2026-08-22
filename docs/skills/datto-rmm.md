---
layout: default
title: "Datto RMM MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Datto RMM API operation, plus a local SQLite fleet store and fleet-wide analytics no other Datto tool has."
permalink: /skills/datto-rmm/
skill_name: "Datto RMM MCP"
image: /assets/social/datto-rmm/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Datto RMM?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Datto RMM, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Datto RMM MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Datto RMM MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Datto RMM data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Datto RMM API rate limits?"
    a: "It's gentle by design. The skill syncs your fleet into a local mirror once, then answers fleet-wide questions from that mirror instead of calling the API per question. You re-sync on your own cadence; everyday analytics run offline against the local store."
  - q: "Do I need to be a Datto partner or have special permissions?"
    a: "You need an API key and secret key from Setup > Users in your Datto RMM portal (the OAuth API user). The skill can only do what that API user is allowed to do, so scope the user to the access your workflow actually needs."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-rmm/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-rmm/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Datto RMM credentials once; datto-rmm-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Datto RMM question in plain language; it runs datto-rmm-cli for you."
---

# The Datto RMM MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Datto, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Datto RMM. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Datto RMM to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask in plain English which endpoints across every client have gone dark, lost antivirus, fallen behind on patches, or are about to drop out of warranty - and get the list in seconds. Datto RMM plus your AI agent reads a local mirror of your whole multi-site fleet, so the cross-customer questions the portal makes you click through site by site become one instant, reproducible answer.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/datto-rmm){: .btn}

## Instead of clicking through Datto RMM, just ask

**Instead of** Open every customer site in the Datto RMM console and read down the last-seen column to spot dead agents
**just ask:** *"Which endpoints across all my clients have stopped checking in?"*
<sub>Your agent runs: <code>datto-rmm-cli fleet stale --days 30 --agent</code></sub>

**Instead of** Click into each device's antivirus panel, one site at a time, to find unprotected machines
**just ask:** *"Show me every endpoint where antivirus is missing or not running"*
<sub>Your agent runs: <code>datto-rmm-cli fleet av-gaps --status not-running --agent</code></sub>

**Instead of** Export device lists and reconcile warranty dates in a spreadsheet before a QBR
**just ask:** *"Which devices have hardware warranties expiring in the next 60 days?"*
<sub>Your agent runs: <code>datto-rmm-cli fleet warranty --within 60 --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/datto-rmm/wide-1200x630.png" src="/assets/video/datto-rmm/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/datto-rmm/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which devices haven't checked in for 30 days, across every client? | `datto-rmm-cli fleet stale --days 30 --agent` |
| Where is antivirus missing, disabled, or not running? | `datto-rmm-cli fleet av-gaps --status not-running --agent` |
| Which endpoints are most behind on patches right now? | `datto-rmm-cli fleet patch-gaps --min-missing 5 --agent` |
| Which devices have warranties expiring in the next 60 days? | `datto-rmm-cli fleet warranty --within 60 --agent` |
| Which devices are generating the most alert noise this week? | `datto-rmm-cli fleet storms --days 7 --top 20 --agent` |
| Give me a one-page health scorecard for a client before the QBR. | `datto-rmm-cli fleet scorecard "Acme Corporation" --agent` |
| How many copies of an app are installed fleet-wide, and which versions? | `datto-rmm-cli fleet sprawl --name "Google Chrome" --agent` |
| Which devices are running an out-of-date RMM agent? | `datto-rmm-cli fleet agent-drift --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/datto-rmm/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/datto-rmm/guide.md).

## What makes this one different

Most Datto RMM integrations and MCP servers proxy each question into a live API call - fine for looking up one device, useless when you ask about all of them. This skill syncs your entire multi-site fleet into a local SQLite mirror, so fleet-wide questions become one offline query: instant, fully paginated, and reproducible later as a saved snapshot you can diff against.

The Datto RMM console answers questions one customer site at a time; this skill answers them across every client at once and hands the result to the AI agent you already work in - it complements the portal you still use for remote control and policy, it doesn't replace it.

## The pain this closes

- Dead agents and offline endpoints hide across dozens of customer sites - you find out a machine stopped reporting when the client calls, not before.
- Cross-client questions ('how many endpoints are missing antivirus everywhere?') mean opening the console one customer at a time, because Datto RMM scopes most views to a single site.
- Alert volume buries the real failures - the noisiest monitors drown the NOC, and tuning them means first finding which devices and sites generate the noise.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/datto-rmm/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/datto-rmm/install.ps1 | iex
```

After install, authenticate once with your Datto RMM credentials, then verify with `datto-rmm-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet stale, fleet av-gaps, fleet scorecard, account get-sites, device get-by-uid, search | Allow |
| Write (routine) | account create-variable, account update-variable, device warranty set-data, device quickjob create-quick-job, site create, site update | Preview with --dry-run, then a reviewed write |
| Destructive / credential | account delete-variable, site variable delete-site, site settings delete-proxy, fleet resolve-storm (--confirm gated), user (resets API keys) | Human-in-the-loop only |

The skill reads your Datto RMM fleet and writes only when you ask it to - creating or updating variables, warranty and UDF data, sites, and quick jobs. Every fleet rollup, search, and lookup is read-only and always safe to run. Deleting variables or site proxy settings, bulk-resolving alert storms, and resetting your API keys are gated as destructive or credential operations and should stay human-in-the-loop. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/datto-rmm/governance.md).

## Frequently asked questions

### Is there an MCP server for Datto RMM?

Yes - this one. A free, open source MCP server and Claude Code Skill for Datto RMM, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Datto RMM MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Datto RMM MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Datto RMM data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Datto RMM API rate limits?

It's gentle by design. The skill syncs your fleet into a local mirror once, then answers fleet-wide questions from that mirror instead of calling the API per question. You re-sync on your own cadence; everyday analytics run offline against the local store.

### Do I need to be a Datto partner or have special permissions?

You need an API key and secret key from Setup > Users in your Datto RMM portal (the OAuth API user). The skill can only do what that API user is allowed to do, so scope the user to the access your workflow actually needs.


## More RMM connectors

Run more than one RMM tool, or comparing options? These connectors work the same way: [Action1](/skills/action1/) · [Atera](/skills/atera/) · [Auvik](/skills/auvik/) · [ConnectWise Automate](/skills/connectwise-automate/) · [ImmyBot](/skills/immybot/) · [Level](/skills/levelio/) · [N-able N-central](/skills/n-central/) · [Nerdio Manager](/skills/nerdio/) · [NinjaOne](/skills/ninjaone/) · [Tactical RMM](/skills/tactical-rmm/)

## Status

Beta. Validated against the Datto RMM API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
