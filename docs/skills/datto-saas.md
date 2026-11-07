---
layout: default
title: "Datto SaaS Protection MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Datto SaaS Protection (Backupify) client in one local database: seat audits, protection gaps, backup health ranking, exit inventories and plan-first bulk seat changes."
permalink: /skills/datto-saas/
skill_name: "Datto SaaS Protection MCP"
image: /assets/social/datto-saas/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Datto SaaS Protection?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Datto SaaS Protection, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Datto SaaS Protection MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `DATTO_SAAS_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `DATTO_SAAS_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Datto SaaS Protection MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Datto SaaS Protection data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "Which Datto API key does it need?"
    a: "The Datto REST API key pair from the Partner Portal (Admin, Integrations, API Keys): DATTO_SAAS_PUBLIC_KEY and DATTO_SAAS_SECRET_KEY, sent as HTTP Basic auth to api.datto.com. One partner key sees every client you manage."
  - q: "Can it change seats or billing on its own?"
    a: "Only one command writes: seats bulk-change, which licenses, pauses or unlicenses seats. Over MCP it returns a preview and sends only when confirm is true, and the read-only MCP mode refuses it. Plan the batches first with seats bulk-plan, which sends nothing."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-saas/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-saas/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Datto SaaS Protection credentials once, then run datto-saas-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Datto SaaS Protection question in plain language; it runs datto-saas-cli for you."
---

# The Datto SaaS Protection MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Datto, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Datto SaaS Protection. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Datto SaaS Protection to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask which users across every client are not backed up, which clients' backups are slipping, and where Datto's seat count disagrees with what you bill, and get one answer for the whole fleet. Datto SaaS Protection reports client by client; this skill mirrors every client's domains and seats locally, so the fleet-wide seat audit becomes one command, and seat changes are planned before anything is sent.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/datto-saas){: .btn}

## Instead of clicking through Datto SaaS Protection, just ask

**Instead of** Open each client in the SaaS Protection portal and scan the seat list for users nobody turned protection on for
**just ask:** *"Which users across all my clients joined in the last month and are not backed up?"*
<sub>Your agent runs: <code>datto-saas-cli seats gaps --since 30d --agent</code></sub>

**Instead of** Export every client's seat list to a spreadsheet and count billable and Active seats against seatsUsed by hand before billing
**just ask:** *"Before I invoice, where does Datto's seatsUsed disagree with its billable and Active seats, per client?"*
<sub>Your agent runs: <code>datto-saas-cli seats reconcile --only-flagged --agent</code></sub>

**Instead of** Click through each client's backup report to find the ones whose backups are slipping
**just ask:** *"Which clients have the worst backup coverage right now?"*
<sub>Your agent runs: <code>datto-saas-cli backup-health --below 95 --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/datto-saas/wide-1200x630.png" src="/assets/video/datto-saas/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/datto-saas/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which seats across every client are unprotected, newest first? | `datto-saas-cli seats gaps --since 30d --agent` |
| Where does Datto's seatsUsed disagree with billable and Active seats, per client? | `datto-saas-cli seats reconcile --only-flagged --agent` |
| Which clients rank worst on backup coverage and short backup windows? | `datto-saas-cli backup-health --below 95 --agent` |
| How many seats does each client have, by seat type and state? | `datto-saas-cli seats summary --agent` |
| Give me one seat table for the whole fleet with each client's domains and subscription ids. | `datto-saas-cli seats roster --state Unprotected,Paused --agent` |
| What do I need to know before a client leaves SaaS Protection? | `datto-saas-cli exit-inventory 123456 --data-source live --agent` |
| Which batches would license this client's unprotected seats, without sending anything? | `datto-saas-cli seats bulk-plan --action License --state Unprotected --customer 123456 --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/datto-saas/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/datto-saas/guide.md).

## What makes this one different

Most Datto SaaS Protection integrations proxy one question into one live API call, and the API answers one client at a time. This skill syncs every client's domains, seats and applications into a local SQLite mirror, so a fleet question such as which seats are unprotected everywhere becomes one offline query, and the only write, a bulk seat change, is planned and previewed before it is sent.

The SaaS Protection portal and Datto's own exports stay where you restore data and manage settings; this skill adds fleet-wide seat audits, billing reconciliation and offboarding manifests inside the AI agent you already use, and complements the portal rather than replacing it.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](/skills/servosity/) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

> **Comparing backup vendors?** One of our MSP partners told us, "My guys now spend a tenth of the time on backups." Fixing is included in every Servosity co-managed backup plan, and our engineers tell you what they did. [See how co-managed backup works](https://servosity.com/?utm_source=msp-skills&utm_medium=connector-note&utm_campaign=datto-saas&utm_content=docs)

## The pain this closes

- A new hire who lands outside Auto-Add, or after the seat cap is reached, is not backed up, and nothing in a single client's view flags it across the fleet.
- Seat counts drive the Datto bill, so a seat that is Active but not billable, or billed but archived, is a margin leak that only shows up when someone reconciles by hand.
- Offboarding a client means collecting domains, subscription ids, retention and every seat from several screens before anyone can cancel safely.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](/integrations/claude-desktop/) |
| **ChatGPT** (desktop app) | [Step-by-step →](/integrations/chatgpt/) |
| **Claude Code** | [Step-by-step →](/integrations/claude-code/) |
| **Codex CLI** | [Step-by-step →](/integrations/codex/) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](/which-agent/) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/datto-saas/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/datto-saas/install.ps1 | iex
```

After install, authenticate once with your Datto SaaS Protection credentials, then verify with `datto-saas-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | seats gaps, seats reconcile, seats summary, seats roster, backup-health, exit-inventory, seats bulk-plan (plans only, sends nothing), domains, seats list, applications, backup-stats, activity-log, search, analytics | Allow |
| Local state | sync, export, workflow archive, local-data purge, learnings, teach*, playbook amend, profile, feedback (write the local mirror, a local file or the learning store; never change anything at Datto) | Allow |
| Seat and billing change | seats bulk-change (License, Pause or Unlicense up to 100 seats per call; changes what is backed up and billed) | Human-in-the-loop only: plan with seats bulk-plan, preview with --dry-run, then a reviewed send |
| Credential / config | auth set-credentials, auth logout (replace or clear the stored key pair) | Human-in-the-loop only |

The datto-saas skill reads your Datto SaaS Protection domains, seats, applications, backup stats and activity log, and keeps a local copy on your machine. It changes one thing at Datto: seats bulk-change licenses, pauses or unlicenses seats, which changes what is backed up and what you are billed, so keep it human-in-the-loop. Reads and seat plans are safe to allow. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/datto-saas/governance.md).

## Frequently asked questions

### Is there an MCP server for Datto SaaS Protection?

Yes - this one. A free, open source MCP server and Claude Code Skill for Datto SaaS Protection, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Datto SaaS Protection MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `DATTO_SAAS_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `DATTO_SAAS_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Datto SaaS Protection MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Datto SaaS Protection data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### Which Datto API key does it need?

The Datto REST API key pair from the Partner Portal (Admin, Integrations, API Keys): DATTO_SAAS_PUBLIC_KEY and DATTO_SAAS_SECRET_KEY, sent as HTTP Basic auth to api.datto.com. One partner key sees every client you manage.

### Can it change seats or billing on its own?

Only one command writes: seats bulk-change, which licenses, pauses or unlicenses seats. Over MCP it returns a preview and sends only when confirm is true, and the read-only MCP mode refuses it. Plan the batches first with seats bulk-plan, which sends nothing.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](/skills/acronis/) · [Afi](/skills/afi/) · [Axcient x360Recover](/skills/axcient/) · [Backup Radar](/skills/backup-radar/) · [CloudAlly](/skills/cloudally/) · [Comet Backup](/skills/comet-backup/) · [Cove Data Protection](/skills/cove/) · [Datto BCDR](/skills/datto-bcdr/) · [Dropsuite](/skills/dropsuite/) · [Proxmox](/skills/proxmox/) · [Servosity](/skills/servosity/) · [SkyKick](/skills/skykick/) · [Synology](/skills/synology/) · [Veeam](/skills/veeam/)

## Status

Beta. Validated against the Datto SaaS Protection API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
