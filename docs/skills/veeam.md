---
layout: default
title: "Veeam MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Veeam Service Provider Console endpoint as a typed command, plus a local multi-tenant mirror, cross-company rollups, and drift detection no per-instance Veeam tool offers."
permalink: /skills/veeam/
skill_name: "Veeam MCP"
image: /assets/social/veeam/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Veeam?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Veeam, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Veeam MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `VEEAM_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `VEEAM_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but veeam-mcp speaks HTTP natively: run `veeam-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Veeam data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Does this work with standalone Veeam Backup & Replication (VBR)?"
    a: "No. This skill targets the Veeam Service Provider Console (VSPC) v3 REST API for multi-tenant MSP estates. A single VBR server with no VSPC in front of it exposes a separate API."
  - q: "Will this hammer my VSPC appliance or hit rate limits?"
    a: "The local mirror is the point. You sync once, then rollups, triage, and search run against local SQLite, so day-to-day questions never touch the appliance. Only sync and the live get commands call VSPC."
  - q: "Can it change backup jobs or run restores?"
    a: "The full VSPC v3 surface is exposed, including create/delete jobs and agent deployment, but those are gated human-in-the-loop in governance.md. Day-to-day this is read-first monitoring; destructive and infrastructure commands need explicit approval."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/veeam/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/veeam/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Veeam credentials once, then run veeam-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Veeam question in plain language; it runs veeam-cli for you."
---

# The Veeam MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Veeam Software.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Veeam. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Veeam to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "which customers have failing backups?" or "who's past their RPO?" and get a cross-tenant answer in one shot. Veeam Service Provider Console is built per-tenant. This skill syncs every company's jobs, agents, alarms, and license usage into a local mirror, then answers the multi-tenant backup questions the console makes you click through company by company.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/veeam){: .btn}

## Instead of clicking through Veeam, just ask

**Instead of** Logging into VSPC and clicking each company to see which backup jobs failed overnight
**just ask:** *"Which backup jobs are failing across all my customers?"*
<sub>Your agent runs: <code>veeam-cli fleet-health --agent</code></sub>

**Instead of** Finding out a customer's backups silently failed for a week only when they ask for a restore
**just ask:** *"Which protected workloads are past their RPO or have no recent restore point?"*
<sub>Your agent runs: <code>veeam-cli at-risk --rpo 24h --agent</code></sub>

**Instead of** Pulling license consumption per tenant from the console before the monthly invoice
**just ask:** *"What's each customer's Veeam license usage and how much did it change?"*
<sub>Your agent runs: <code>veeam-cli license-usage --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/veeam/wide-1200x630.png" src="/assets/video/veeam/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/veeam/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which backup jobs are failing across all my customers? | `veeam-cli fleet-health --agent` |
| Which jobs and agents haven't had a successful run in 3+ days? | `veeam-cli stale-backups --days 3 --agent` |
| Which protected workloads are past their RPO? | `veeam-cli at-risk --rpo 24h --agent` |
| What active alarms actually need a human, deduped across tenants? | `veeam-cli alarms-triage --severity Error --agent` |
| How is one customer doing across backups, agents, alarms, and license? | `veeam-cli company-overview --company "Contoso Ltd" --agent` |
| What's each customer's license usage and the change since last run? | `veeam-cli license-usage --agent` |
| What changed across the fleet in the last 24h - failures, alarms, new agents? | `veeam-cli since 24h --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/veeam/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/veeam/guide.md).

## What makes this one different

Most Veeam integrations proxy each question into a live API call against one console - fine for one job, useless when you ask "across all 60 customers." This skill syncs every tenant into a local SQLite mirror, so a cross-company rollup is one local join: instant, offline, and your AI sees the answer, not thousands of raw job records.

VSPC's console and reports are built for per-tenant administration. This complements them by answering the whole-portfolio backup questions - failing jobs, stale protection, RPO breaches, license drift - in a single command, without touching your console workflows.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](/skills/servosity/) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

## The pain this closes

- VSPC is organized per-tenant, so "which customers have a failed backup right now?" means clicking into each company - exactly the question that matters most at 8am.
- Backups fail silently. The worst time to discover a week of failures is when a customer asks for a restore, and the console doesn't surface stale protection across the whole fleet in one view.
- License reconciliation and RPO compliance for a QBR or invoice mean assembling numbers per tenant by hand.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/veeam/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/veeam/install.ps1 | iex
```

After install, authenticate once with your Veeam credentials, then verify with `veeam-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet-health, stale-backups, at-risk, alarms-triage, company-overview, license-usage, since, and every get / list / search command | Allow |
| Write (config) | configuration create/patch backup-policy, infrastructure create/assign jobs, discovery create/patch rules, alarms acknowledge/resolve | Preview with --dry-run, then a reviewed write |
| Infrastructure and agent execution | discovery install-backup-agent-on-computer / reboot-computer / start-rule, infrastructure activate / force-collect, deployment wait-task | Human-in-the-loop, explicit confirmation (runs/installs software on real customer machines) |
| Destructive | infrastructure delete-tenant / delete-backup-agent / delete-vb365-server, configuration delete-backup-policy, alarms delete-active | Human-in-the-loop only, explicit confirmation |
| Credential / security | authentication generate-totp-secret / decrypt-pkcs12-container, infrastructure create/get backup-server-credentials and encryption-passwords, users tokens revoke-authentication | Human-in-the-loop only |

The skill is read-first: cross-tenant health, stale-backup and at-risk sweeps, alarm triage, license usage, and search are read-only and safe to let an agent run. The write surface is large - creating and deleting backup jobs, policies and rules, installing agents, rebooting and deploying to real customer infrastructure, and credential/certificate operations - and is gated human-in-the-loop. The VSPC token's own permissions are the outer limit on anything the agent can do. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/veeam/governance.md).

## Frequently asked questions

### Is there an MCP server for Veeam?

Yes - this one. A free, open source MCP server and Claude Code Skill for Veeam, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Veeam MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `VEEAM_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `VEEAM_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but veeam-mcp speaks HTTP natively: run `veeam-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Veeam data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Does this work with standalone Veeam Backup & Replication (VBR)?

No. This skill targets the Veeam Service Provider Console (VSPC) v3 REST API for multi-tenant MSP estates. A single VBR server with no VSPC in front of it exposes a separate API.

### Will this hammer my VSPC appliance or hit rate limits?

The local mirror is the point. You sync once, then rollups, triage, and search run against local SQLite, so day-to-day questions never touch the appliance. Only sync and the live get commands call VSPC.

### Can it change backup jobs or run restores?

The full VSPC v3 surface is exposed, including create/delete jobs and agent deployment, but those are gated human-in-the-loop in governance.md. Day-to-day this is read-first monitoring; destructive and infrastructure commands need explicit approval.


## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](/skills/acronis/) · [Afi](/skills/afi/) · [Axcient x360Recover](/skills/axcient/) · [Cove Data Protection](/skills/cove/) · [Datto BCDR](/skills/datto-bcdr/) · [Servosity](/skills/servosity/) · [SkyKick](/skills/skykick/)

## Status

Beta. Validated against the Veeam API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
