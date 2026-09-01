---
layout: default
title: "Servosity MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "The first MSP-fleet CLI for backup. Every Servosity API endpoint as a typed command, plus a local mirror that lets you ask questions the dashboard can't \u2014 across your whole book of clients."
permalink: /skills/servosity/
skill_name: "Servosity MCP"
image: /assets/social/servosity/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Servosity?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Servosity, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Servosity MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SERVOSITY_MSP_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `SERVOSITY_MSP_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but servosity-mcp speaks HTTP natively: run `servosity-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Servosity and client data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local fleet mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "What credentials do I need?"
    a: "A Servosity partner API token from the partner portal. Set SERVOSITY_MSP_TOKEN in your environment or run servosity-cli auth set-token. The token carries your reseller scope - the CLI sees exactly the clients your Servosity account sees, nothing more."
  - q: "Can this change anything in my Servosity account?"
    a: "The day-to-day surface is read-only. The write surface is issue triage (triage --ignore / --archive / --reactivate / --comment) and import, plus raw create/update/delete subcommands under the API resource groups. --dry-run is an opt-in preview, not a default - the recommended agent policy is preview, approve, then run. Restores and backup configuration stay in the dashboard."
  - q: "Is it fast enough for the morning sweep?"
    a: "After a sync, the fleet views (attention, drift, stale-backups, backup-facts, qbr) read the local store - instant and offline. Pass --refresh when you want a live pull; restore-queue watch polls live by design."
  - q: "Do I need to be a Servosity partner?"
    a: "Yes - the CLI authenticates with an MSP partner token. If you're not a partner yet, start at servosity.com; the skill itself is Apache-2.0 and free either way."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Servosity credentials once, then run servosity-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Servosity question in plain language; it runs servosity-cli for you."
---

# The Servosity MCP Server - free, local, built for MSPs

> Published by Servosity Inc. for MSP partners. Servosity is a trademark of
> Servosity Inc.. Apache-2.0 licensed.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Servosity. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Servosity to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

MSPs run Servosity as the backup and DR platform - M365, DR Server, and DR Desktop protection across a whole book of clients. Ask your AI "where is my attention needed today", "which backups went stale this week", or "build Acme's QBR backup section" and get fleet-wide answers the per-client dashboard can't compose: ranked attention sweeps, day-over-day drift, ready-to-send stale-backup follow-ups, and the whole book's QBR reports in one pass.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/servosity){: .btn}

## Instead of clicking through Servosity, just ask

**Instead of** Opening every client's dashboard each morning to figure out who needs attention first
**just ask:** *"Where is my attention needed today, worst first?"*
<sub>Your agent runs: <code>servosity-cli attention --top 5</code></sub>

**Instead of** Assembling each client's QBR backup slide by hand - success rates, restore tests, coverage, storage growth
**just ask:** *"Build the backup section of Acme's QBR as a PDF"*
<sub>Your agent runs: <code>servosity-cli qbr "Acme Co" --out acme-q1.pdf</code></sub>

**Instead of** Finding out at renewal time that agents were installed at three clients but never started backing up
**just ask:** *"Which installed agents still aren't pulling backups?"*
<sub>Your agent runs: <code>servosity-cli unprovisioned</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/servosity/wide-1200x630.png" src="/assets/video/servosity/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/servosity/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Where is my attention needed today, ranked worst-first? | `servosity-cli attention --top 5` |
| What got worse since yesterday, and what recovered? | `servosity-cli drift` |
| Which clients have backups stale for 7+ days? | `servosity-cli stale-backups --days 7` |
| Draft the follow-up email for every client with a stale backup | `servosity-cli email-draft --stale --days 7` |
| Build the backup section of Acme's QBR as a PDF | `servosity-cli qbr "Acme Co" --out acme-q1.pdf` |
| Quarter-end: every client's QBR backup report in one pass | `servosity-cli qbr-all --quarter 2026-Q1 --out ./qbrs/` |
| Watch every client's restore queue during a DR event | `servosity-cli restore-queue watch` |
| Does my Servosity bill match what I invoice my clients? | `servosity-cli bill --reconcile invoices.csv` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/servosity/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/servosity/guide.md).

## What makes this one different

Most backup integrations proxy one client's status at a time into live API calls. This skill syncs the whole book - companies, backups, issues, restore activity - into a local SQLite mirror, then answers fleet questions as one local join: attention ranks every client, drift compares any two days from the snapshots it keeps, stale-backups slices by client and age, and qbr-all renders the entire book's reports offline. The snapshot history is the part no stateless wrapper can offer: day-over-day memory the dashboard doesn't keep.

This is Servosity's own connector, published for MSP partners. It complements the dashboard rather than replacing it: the dashboard stays best for per-client deep dives and restore operations, while the skill brings the fleet view to whichever agent you already use - with the day-over-day memory (attention snapshots, drift, storage trends) the dashboard doesn't keep.

## The pain this closes

- Fleet questions are per-client portal walks: the dashboard shows one client at a time, so "who needs attention across my whole book" means opening every client every morning - and during a DR event, tab-switching between every restore queue.
- Stale-backup follow-up is manual: find the stale sets, look up the hosts and last-success dates, then write the same email to each client - the Friday sweep that slips the week things get loud.
- QBR season means rebuilding the same backup-health story per client by hand: job success rate, restore tests run, coverage, storage growth - multiplied by every client in the book.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

After install, authenticate once with your Servosity credentials, then verify with `servosity-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | servosity-cli attention --top 5; servosity-cli drift; servosity-cli stale-backups --days 7; servosity-cli backup-facts; servosity-cli qbr "Acme Co"; servosity-cli fleet-health; servosity-cli unprovisioned; servosity-cli bill; servosity-cli email-draft --stale --days 7 (drafts only - nothing is sent); servosity-cli restore-queue watch | Allow |
| Write (routine) | servosity-cli triage --ignore 18,22 --comment "scheduled outage" (issue mutations); servosity-cli import <resource> --input data.jsonl; raw create/update subcommands under the resource groups - --dry-run is an opt-in preview, not a default | Preview with --dry-run, then a reviewed write |
| Credentials / destructive | servosity-cli auth set-token; servosity-cli auth logout; resource-group delete subcommands (e.g. companies delete) | Human-in-the-loop only |

First-party: published by Servosity for MSP partners. The skill drives the servosity-cli and servosity-mcp binaries, authenticating with your partner token (SERVOSITY_MSP_TOKEN) - never logged and never sent anywhere except the Servosity API. The fleet views are reads; the write surface is issue triage (ignore/archive/reactivate/comment), import, and raw CRUD under the resource groups, all with the opt-in --dry-run preview. The token's reseller scope is the permission boundary. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/servosity/governance.md).

## Frequently asked questions

### Is there an MCP server for Servosity?

Yes - this one. A free, open source MCP server and Claude Code Skill for Servosity, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Servosity MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SERVOSITY_MSP_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `SERVOSITY_MSP_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but servosity-mcp speaks HTTP natively: run `servosity-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Servosity and client data safe?

Your data stays on your machine. The CLI, MCP server, and the local fleet mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What credentials do I need?

A Servosity partner API token from the partner portal. Set SERVOSITY_MSP_TOKEN in your environment or run servosity-cli auth set-token. The token carries your reseller scope - the CLI sees exactly the clients your Servosity account sees, nothing more.

### Can this change anything in my Servosity account?

The day-to-day surface is read-only. The write surface is issue triage (triage --ignore / --archive / --reactivate / --comment) and import, plus raw create/update/delete subcommands under the API resource groups. --dry-run is an opt-in preview, not a default - the recommended agent policy is preview, approve, then run. Restores and backup configuration stay in the dashboard.

### Is it fast enough for the morning sweep?

After a sync, the fleet views (attention, drift, stale-backups, backup-facts, qbr) read the local store - instant and offline. Pass --refresh when you want a live pull; restore-queue watch polls live by design.

### Do I need to be a Servosity partner?

Yes - the CLI authenticates with an MSP partner token. If you're not a partner yet, start at servosity.com; the skill itself is Apache-2.0 and free either way.


## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](/skills/acronis/) · [Afi](/skills/afi/) · [Axcient x360Recover](/skills/axcient/) · [Cove Data Protection](/skills/cove/) · [Datto BCDR](/skills/datto-bcdr/) · [SkyKick](/skills/skykick/) · [Veeam](/skills/veeam/)

## Status

Beta. Validated against the Servosity API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
