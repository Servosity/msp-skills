---
layout: default
title: "Afi MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "The first CLI for Afi SaaS backup - full public-API coverage plus the fleet-wide coverage, staleness, and offboarding answers the rate-limited API can't serve live."
permalink: /skills/afi/
skill_name: "Afi MCP"
image: /assets/social/afi/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Afi?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Afi, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Afi MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Afi MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Afi data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this trip Afi's API rate limits?"
    a: "Not if you use it as designed. Afi throttles - and may suspend - applications that poll continuously, so this skill walks the fleet into a local store in one respectful, rate-limited pass with fleet-sync, then answers every question offline against that store. You sync on a schedule, not on every question."
  - q: "Do I need to be an Afi customer, and what access does the key need?"
    a: "Yes - you need an Afi account and an Application API key (created in the Afi portal: org level under Configuration to Apps, or tenant level under Service to Settings to Apps). The key inherits the Application's installation scope, so the CLI sees exactly the orgs and tenants that Application is installed on. Each Application supports two keys for rotation."
  - q: "Will this replace the Afi portal?"
    a: "No. Restores, exports, and policy editing still happen in the Afi portal - the public API does not expose them. This skill is the read, report, and guarded-offboard layer: it answers fleet questions and runs the verified archive-then-release, then hands you back to the portal for the actions only it can do."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Afi credentials once; afi-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Afi question in plain language; it runs afi-cli for you."
---

# The Afi MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Afi.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Afi. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Afi to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI 'which mailboxes aren't backed up in Afi?' and get the answer across every tenant at once. This skill walks your whole Afi fleet into a local store, then answers coverage gaps, stale backups, license drift, and per-tenant posture offline - no per-tenant portal clicking, no tripping Afi's rate limits - and runs a verified archive-then-release when someone leaves.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/afi){: .btn}

## Instead of clicking through Afi, just ask

**Instead of** Open the Afi portal, click into each client tenant, and eyeball which users actually have a backup policy attached.
**just ask:** *"Which resources across the whole fleet have no backup protection?"*
<sub>Your agent runs: <code>afi-cli coverage-gaps --agent</code></sub>

**Instead of** Tab through every tenant's task history hunting for the nightly backups that quietly stopped landing.
**just ask:** *"Show me protected resources whose newest backup is older than 48 hours."*
<sub>Your agent runs: <code>afi-cli backup-stale --max-age 48h --agent</code></sub>

**Instead of** Stitch two portal screens into a spreadsheet to compare licenses purchased against seats actually protected.
**just ask:** *"Where am I over- or under-provisioned on Afi licenses?"*
<sub>Your agent runs: <code>afi-cli reconcile-licenses --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/afi/wide-1200x630.png" src="/assets/video/afi/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/afi/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which resources have no backup protection at all? | `afi-cli coverage-gaps --agent` |
| Which protected resources have a stale backup (silent failures)? | `afi-cli backup-stale --max-age 48h --agent` |
| Is the whole fleet green this morning, or who failed? | `afi-cli fleet-health --failed-only --agent` |
| What is one tenant's full backup posture for a QBR or ticket? | `afi-cli tenant-scorecard <tenant-id> --agent` |
| Am I over- or under-licensed on Afi seats? | `afi-cli reconcile-licenses --agent` |
| Who is jane.doe@example.com in Afi, across Multi-Geo tenants? | `afi-cli resolve <email-or-id> --agent` |
| Safely back up then release a departing employee's mailbox? | `afi-cli offboard <resource-id> --tenant <tenant-id> --reason "employee departure"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/afi/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/afi/guide.md).

## What makes this one different

Most Afi integrations proxy each question into a live API call - fine for one lookup, but it falls over when you ask a fleet-wide question across dozens of tenants, and Afi's rate limits punish the polling. This skill syncs the whole hierarchy into a local SQLite mirror once, then answers coverage, staleness, and licensing as offline SQL joins: instant, and gentle on the API.

The Afi portal answers one tenant at a time and has no cross-tenant rollup or coverage-gap report. This skill adds the fleet-wide views - coverage gaps, stale backups, license reconciliation, and a verified archive-then-release offboard - that neither the portal nor the rate-limited public API serves directly.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](/skills/servosity/) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

## The pain this closes

- A new mailbox or site gets created in Microsoft 365 or Google Workspace but never gets a backup policy attached - and nobody notices until a restore request arrives and the data was never being protected.
- A backup quietly stops landing on a protected resource; the portal still shows the policy attached, so the silent failure stays invisible until the day you actually need that archive.
- Verifying backup coverage across dozens of client tenants means walking the Afi portal one tenant tab at a time, and the public API throttles you for polling.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/afi/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/afi/install.ps1 | iex
```

After install, authenticate once with your Afi credentials, then verify with `afi-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | coverage-gaps, backup-stale, fleet-health, tenant-scorecard, reconcile-licenses, resolve, fleet-sync, and every list/get command | Allow |
| Write (routine) | orgs create, import, tenants resources protections-protect, tenants jobs tasks-trigger | Preview with --dry-run, then a reviewed write |
| Destructive / config | offboard (releases protection), tenants resources protections-unprotect, tenants archives delete | Human-in-the-loop only |

The skill reads your Afi fleet (installations, orgs, tenants, resources, protections, policies, archives, quotas, and task stats) and can run a small set of writes: create a child org, import records, trigger a backup, and add a protection. Three commands are genuinely destructive - offboard, protections-unprotect, and archives delete - because they release backup coverage or delete an archive. Let an agent run reads freely; require a human to approve every write, and especially the destructive tier. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/afi/governance.md).

## Frequently asked questions

### Is there an MCP server for Afi?

Yes - this one. A free, open source MCP server and Claude Code Skill for Afi, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Afi MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Afi MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Afi data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this trip Afi's API rate limits?

Not if you use it as designed. Afi throttles - and may suspend - applications that poll continuously, so this skill walks the fleet into a local store in one respectful, rate-limited pass with fleet-sync, then answers every question offline against that store. You sync on a schedule, not on every question.

### Do I need to be an Afi customer, and what access does the key need?

Yes - you need an Afi account and an Application API key (created in the Afi portal: org level under Configuration to Apps, or tenant level under Service to Settings to Apps). The key inherits the Application's installation scope, so the CLI sees exactly the orgs and tenants that Application is installed on. Each Application supports two keys for rotation.

### Will this replace the Afi portal?

No. Restores, exports, and policy editing still happen in the Afi portal - the public API does not expose them. This skill is the read, report, and guarded-offboard layer: it answers fleet questions and runs the verified archive-then-release, then hands you back to the portal for the actions only it can do.


## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](/skills/acronis/) · [Axcient x360Recover](/skills/axcient/) · [Cove Data Protection](/skills/cove/) · [Datto BCDR](/skills/datto-bcdr/) · [Servosity](/skills/servosity/) · [SkyKick](/skills/skykick/)

## Status

Beta. Validated against the Afi API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
