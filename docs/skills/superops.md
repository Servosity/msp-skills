---
layout: default
title: "SuperOps MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every SuperOps PSA+RMM entity in your terminal, plus a local SQLite mirror that answers cross-entity questions the web UI can't."
permalink: /skills/superops/
skill_name: "SuperOps MCP"
image: /assets/social/superops/wide-1200x630.png
verification: live-verified
faqs:
  - q: "Is there an MCP server for SuperOps?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for SuperOps, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the SuperOps MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SUPEROPS_FEEDBACK_AUTO_SEND=true` mails feedback you wrote to the maintainers; `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but superops-mcp speaks HTTP natively: run `superops-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my SuperOps data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "Will this hit my SuperOps API rate limits?"
    a: "The local mirror exists so reads stop hitting the API. After the first `sync`, the cross-entity views (sla-watch, client-360, at-risk-assets, alert-coverage, unbilled, stale-tickets) run against local SQLite with zero API calls. Live calls respect a `--rate-limit` throttle, and sync is incremental and resumable - it only fetches what changed, and it treats resources your token can't reach as warnings, not failures."
  - q: "Does it work with the US and EU SuperOps regions?"
    a: "Yes. The US host is the default; set SUPEROPS_REGION=eu to target the EU host (euapi.superops.ai). Your tenant subdomain goes in SUPEROPS_SUBDOMAIN, which the CLI sends as the CustomerSubDomain header on every request."
  - q: "Can it create or update tickets?"
    a: "The typed commands are read-only by design - inspection, export, sync, and analysis. The one write path is `raw mutation`, the supported escape hatch for operations the typed commands don't wrap (for example createTicket, updateTicket, resolveAlerts). Pair it with --dry-run to preview the exact GraphQL request, and keep a human in the loop; `raw query` is the read-only counterpart."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/superops/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/superops/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your SuperOps credentials once, then run superops-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a SuperOps question in plain language; it runs superops-cli for you."
---

# The SuperOps MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by SuperOps Inc..

**✓ Live-verified by @AvlCompCo (MSP)** against a production tenant · 2026-06-18 · [receipt →](https://github.com/Servosity/msp-skills/issues/140).

Yes - there is an MCP server for SuperOps. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects SuperOps to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

SuperOps unifies PSA and RMM on one database, but the console still answers one entity at a time. Ask your AI "who's about to breach SLA, and on whose queue," "what's the full picture on Acme before the QBR," or "which endpoints are unpatched and actively alerting," and get cross-entity answers computed offline from a local SQLite mirror of your tenant - one query instead of five console screens or a scheduled report.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/superops){: .btn}

## Instead of clicking through SuperOps, just ask

**Instead of** Running a scheduled SLA report, exporting it, and re-sorting by technician in a spreadsheet to spot who is about to miss
**just ask:** *"Who's about to breach SLA, and on whose queue?"*
<sub>Your agent runs: <code>superops-cli sla-watch --by tech --window 4h</code></sub>

**Instead of** Opening the client, then its sites, users, contracts, tickets, assets, and invoices across six console tabs to prep one QBR
**just ask:** *"Give me the full picture on Acme Corp before my 2 o'clock"*
<sub>Your agent runs: <code>superops-cli client-360 "Acme Corp"</code></sub>

**Instead of** Cross-checking the patch report against the alert console by hand to find endpoints that are both unpatched and actively alerting
**just ask:** *"Which endpoints are missing a critical patch and actively alerting?"*
<sub>Your agent runs: <code>superops-cli at-risk-assets --client Acme</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/superops/wide-1200x630.png" src="/assets/video/superops/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/superops/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Who's about to breach SLA, grouped by technician? | `superops-cli sla-watch --by tech --window 4h` |
| Which clients have alerts still sitting unresolved? | `superops-cli alert-coverage --client Acme` |
| Which endpoints are missing a critical patch and actively alerting? | `superops-cli at-risk-assets --client Acme` |
| Which open tickets has nobody touched in a week? | `superops-cli stale-tickets --days 7` |
| Everything about one client - sites, users, contracts, tickets, assets, open invoices? | `superops-cli client-360 "Acme Corp"` |
| Where is billable time concentrated before this month's invoicing? | `superops-cli unbilled --since 2026-05-01` |
| Give my triage agent one ticket with its worklogs, client, and SLA in a single read | `superops-cli context-ticket 12345 --agent --select ticket.subject,client.name,sla.name` |
| Search every synced ticket, asset, and client for "disk full" | `superops-cli search "disk full"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/superops/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/superops/guide.md).

## What makes this one different

Most SuperOps integrations and MCP servers proxy each question into a live GraphQL call - fine for one record, but an aggregate question becomes a multi-call dance the AI burns context on, and the API rate-limits reads and omits the cross-entity links those questions need. This skill syncs your tenant into a local SQLite mirror with full-text search, so cross-entity questions - SLA breach by tech, client 360, at-risk assets, billable-time reconciliation - become one local join: instant, offline, and the AI sees the answer, not pages of raw JSON.

It complements SuperOps's own roadmap rather than replacing it: the console stays best for in-app workflows - dispatch, time entry, invoicing - while this skill brings your tenant to whichever AI agent you already use and answers the cross-entity questions, joined across PSA and RMM, that no single console screen composes today.

## The pain this closes

- SuperOps unifies PSA and RMM on one database, but the console answers one entity at a time and its own AI features are, in third-party reviews, "more roadmap than reality" (Flamingo, SuperOps Review for MSPs, 2026) - so cross-entity questions like SLA-breach-by-technician or a client's full footprint still mean clicking across modules or waiting on a scheduled report.
- Reconciling billable time at month-end is manual: the SuperOps list API does not expose a per-entry "already billed" flag, so seeing where billable worklog is concentrated per client - the number you sanity-check before invoicing - means exporting worklogs and totaling them by hand.
- The GraphQL API rate-limits reads and omits some cross-entity links (asset-to-ticket, aggregated child-activity timestamps) from its list payloads, so any script answering "which tickets have gone stale" or "which assets are at risk" has to fetch, cache, and join locally rather than ask the API directly.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/superops/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/superops/install.ps1 | iex
```

After install, authenticate once with your SuperOps credentials, then verify with `superops-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | superops-cli sla-watch --by tech --window 4h; superops-cli client-360 "Acme Corp"; superops-cli at-risk-assets --client Acme; superops-cli alert-coverage; superops-cli unbilled --since 2026-05-01; superops-cli stale-tickets --days 7; superops-cli tickets list; superops-cli search "disk full"; superops-cli raw query | Allow |
| Write (mutation escape hatch) | superops-cli raw mutation - the only write path; wraps operations the read-only typed commands don't cover (createTicket, updateTicket, resolveAlerts). --dry-run prints the exact GraphQL request without sending it | Preview with --dry-run, then a reviewed write |
| Destructive / config | No typed destructive command exists; any irreversible change would have to be an explicit destructive GraphQL operation run through raw mutation (e.g. a delete mutation) | Human-in-the-loop only |

The skill drives the superops-cli and superops-mcp binaries, authenticating with a SUPEROPS_API_TOKEN (plus SUPEROPS_SUBDOMAIN for your tenant) read from the environment - never logged, never sent anywhere except the SuperOps API. Every typed command is read-only: tickets, assets, alerts, clients, contracts, invoices, worklogs, and the cross-entity views change nothing. The single write path is `raw mutation`; the recommended policy is to preview it with --dry-run, show the exact GraphQL request, get approval, then run. The strongest control is the scope of the API token you mint. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/superops/governance.md).

## Frequently asked questions

### Is there an MCP server for SuperOps?

Yes - this one. A free, open source MCP server and Claude Code Skill for SuperOps, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the SuperOps MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SUPEROPS_FEEDBACK_AUTO_SEND=true` mails feedback you wrote to the maintainers; `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but superops-mcp speaks HTTP natively: run `superops-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my SuperOps data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### Will this hit my SuperOps API rate limits?

The local mirror exists so reads stop hitting the API. After the first `sync`, the cross-entity views (sla-watch, client-360, at-risk-assets, alert-coverage, unbilled, stale-tickets) run against local SQLite with zero API calls. Live calls respect a `--rate-limit` throttle, and sync is incremental and resumable - it only fetches what changed, and it treats resources your token can't reach as warnings, not failures.

### Does it work with the US and EU SuperOps regions?

Yes. The US host is the default; set SUPEROPS_REGION=eu to target the EU host (euapi.superops.ai). Your tenant subdomain goes in SUPEROPS_SUBDOMAIN, which the CLI sends as the CustomerSubDomain header on every request.

### Can it create or update tickets?

The typed commands are read-only by design - inspection, export, sync, and analysis. The one write path is `raw mutation`, the supported escape hatch for operations the typed commands don't wrap (for example createTicket, updateTicket, resolveAlerts). Pair it with --dry-run to preview the exact GraphQL request, and keep a human in the loop; `raw query` is the read-only counterpart.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## More PSA connectors

Run more than one PSA tool, or comparing options? These connectors work the same way: [Autotask PSA](/skills/autotask/) · [ConnectWise PSA (Manage)](/skills/connectwise-manage/) · [HaloPSA](/skills/halopsa/) · [Kaseya BMS](/skills/kaseya-bms/) · [Syncro](/skills/syncro/) · [Zammad](/skills/zammad/)

## Status

Beta. Validated against the SuperOps API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
