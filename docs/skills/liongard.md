---
layout: default
title: "Liongard MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Liongard endpoint, plus an offline copy of your whole MSP estate you can join, search, and drift-check from one command."
permalink: /skills/liongard/
skill_name: "Liongard MCP"
image: /assets/social/liongard/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Liongard?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Liongard, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Liongard MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Liongard MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Liongard data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need to be a Liongard customer, and will this hit my API limits?"
    a: "Yes - you use your own Liongard instance and access keys, so the skill only reaches data your credentials already permit. Because it syncs once into a local mirror and answers most questions from there, it makes far fewer API calls than a per-question live wrapper, which keeps you well clear of rate limits during reporting and QBR prep."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/liongard/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/liongard/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Liongard credentials once; liongard-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Liongard question in plain language; it runs liongard-cli for you."
---

# The Liongard MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Liongard, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Liongard. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Liongard to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask "what changed across all my clients this week," "which Liongard collectors went stale," or "which agents are offline," and get the answer in one command instead of clicking through the portal environment by environment. The skill syncs your whole Liongard estate into a local mirror, then drift-checks, searches, and rolls it up across every environment - so the visibility you already pay for finally reaches you.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/liongard){: .btn}

## Instead of clicking through Liongard, just ask

**Instead of** Log into the Liongard portal and click environment by environment to see what changed this week.
**just ask:** *"What changed across all my clients in the last 7 days?"*
<sub>Your agent runs: <code>liongard-cli drift --since 7d</code></sub>

**Instead of** Export each client's launchpoints to hunt for collectors that quietly stopped running.
**just ask:** *"Which Liongard collectors have gone stale?"*
<sub>Your agent runs: <code>liongard-cli launchpoints stale --older-than 7d</code></sub>

**Instead of** Write an API script to pull one metric across every system for the QBR deck.
**just ask:** *"Give me the MFA-enabled count for every system as a CSV."*
<sub>Your agent runs: <code>liongard-cli metrics pivot "MFA Enabled Count" --csv</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/liongard/wide-1200x630.png" src="/assets/video/liongard/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/liongard/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What changed across all my clients in the last 24 hours? | `liongard-cli drift --since 24h` |
| Which collectors (launchpoints) have gone stale? | `liongard-cli launchpoints stale --older-than 7d` |
| Which agents are offline right now, and whose environment do they serve? | `liongard-cli agents offline` |
| Give me one health scorecard for the whole estate. | `liongard-cli health --agent` |
| Show me one client's complete picture in a single command. | `liongard-cli environments overview 42` |
| Which inspections failed or errored across the estate? | `liongard-cli detections failures --since 7d` |
| Pull one metric across every system, CSV-ready for a report. | `liongard-cli metrics pivot "MFA Enabled Count" --csv` |
| Which systems breach a threshold, like patch age over 30 days? | `liongard-cli metrics breach "Patch Age Days" --op gt --value 30` |
| Where are my monitoring gaps (systems with no launchpoint, environments with no systems)? | `liongard-cli coverage` |
| Which environments are still missing a given inspector? | `liongard-cli inspectors coverage --inspector "Microsoft 365"` |
| What is the full change history for one system? | `liongard-cli systems history 4821` |
| Search everything I've synced for a term. | `liongard-cli search "domain admin"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/liongard/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/liongard/guide.md).

## What makes this one different

Most Liongard integrations and MCP servers proxy each question into a live API call. That is fine for one record and falls apart the moment you ask an estate-wide question - 'what changed across all 40 clients this week' becomes 40+ paginated calls. This skill syncs Liongard into a local SQLite mirror with full-text search, so cross-client questions become one local join: instant, offline, and the agent sees the answer, not the raw bulk data.

It complements the Liongard portal rather than replacing it. The portal is built for clicking into one environment at a time; this skill answers the cross-estate questions - drift, stale collectors, offline agents, coverage gaps, one metric across every system - in a single command you can pipe into a report or hand to your AI agent.

## The pain this closes

- Liongard is data-rich but hard to operationalize: the answers live in the portal, but seeing 'what changed across every client' means opening each environment one at a time.
- Collectors and agents go quiet silently - a stale launchpoint or an offline agent stops collecting, and the documentation rots until a QBR or an audit exposes the gap.
- Cross-client reporting - one metric across every system, estate-wide failed inspections, who is missing an inspector - means manual portal work or a custom API script every time.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/liongard/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/liongard/install.ps1 | iex
```

After install, authenticate once with your Liongard credentials, then verify with `liongard-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | drift, health, coverage, launchpoints stale, agents offline, detections failures, inspectors coverage, metrics pivot, metrics breach, systems history, environments overview, search, and every get/list/count | Allow |
| Write (routine) | environments create / update, launchpoints launchpoint / update / update-bulk, metrics create / update, users create / update, agents update, import, and inspection triggers (launchpoints run / bulk-run / run-stale, agents flush job-queue) | Preview with --dry-run, then a reviewed write |
| Destructive / credential | environments delete, launchpoints delete / bulk-delete, metrics delete, users delete, agents delete, access-keys create / delete, and the authentication / auth token commands | Human-in-the-loop only |

Read commands (drift, health, coverage, the stale/offline/failure rollups, search, and every get/list) cannot change anything and are safe to allow. Writes are explicit and named - creating or updating environments, launchpoints, metrics, users, and agents, plus importing data and triggering inspection runs - and should be previewed before they send. Deletes, minting access keys, and the auth/token commands are human-in-the-loop only. The strongest control is the scope of the Liongard access keys you supply: the CLI can only do what those keys are permitted to do. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/liongard/governance.md).

## Frequently asked questions

### Is there an MCP server for Liongard?

Yes - this one. A free, open source MCP server and Claude Code Skill for Liongard, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Liongard MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Liongard MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Liongard data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need to be a Liongard customer, and will this hit my API limits?

Yes - you use your own Liongard instance and access keys, so the skill only reaches data your credentials already permit. Because it syncs once into a local mirror and answers most questions from there, it makes far fewer API calls than a per-question live wrapper, which keeps you well clear of rate limits during reporting and QBR prep.


## More Documentation connectors

Run more than one Documentation tool, or comparing options? These connectors work the same way: [Hudu](/skills/hudu/) · [IT Glue](/skills/itglue/) · [PandaDoc](/skills/pandadoc/)

## Status

Beta. Validated against the Liongard API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
