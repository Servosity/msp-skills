---
layout: default
title: "Acronis Cyber Protect Cloud MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "The first real CLI for the Acronis Cyber Protect Cloud platform - every tenant, agent, and usage metric mirrored locally, with cross-tenant rollups no single API call returns."
permalink: /skills/acronis/
skill_name: "Acronis Cyber Protect Cloud MCP"
image: /assets/social/acronis/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Acronis MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Acronis data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Acronis API rate limits?"
    a: "The local mirror exists so reads stop hitting the API. After the first sync, the cross-tenant views (health, coverage, freshness, agents stale, reconcile usages) run against local SQLite with zero API calls. Live calls respect a --rate-limit throttle, and sync is incremental - it only fetches what changed since the last checkpoint."
  - q: "Do I need to be an Acronis partner?"
    a: "Yes. You authenticate with an API client created in your own Acronis Management Portal (an OAuth2 client, or a bearer token), so you need partner or admin access to the tenants you want to reach. The CLI can only see what that API client is scoped to."
  - q: "Which datacenter does it point at?"
    a: "Whichever hosts your Acronis account. Set ACRONIS_DATACENTER (for example us-cloud or eu2-cloud), or pass --datacenter on auth login. The CLI builds the correct regional API host from it, so you don't hand-assemble the datacenter URL in every call."
  - q: "Will this replace the Acronis console?"
    a: "No. The console stays best for configuring protection plans and running restores. This skill adds the cross-tenant reporting layer the partner dashboards don't - one place to ask whose backups failed, which agents are offline, and where billing and protection diverge."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/acronis/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/acronis/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Acronis Cyber Protect Cloud credentials once; acronis-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Acronis Cyber Protect Cloud question in plain language; it runs acronis-cli for you."
---

# Acronis Cyber Protect Cloud + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the Acronis
> API. Not affiliated with, endorsed by, or sponsored by Acronis International GmbH.

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

MSPs run Acronis Cyber Protect Cloud across dozens of customer tenants, but its partner dashboards report one tenant at a time. Ask your AI "whose backups failed last night," "which agents went offline," or "where am I billing for protection that isn't running," and get the cross-tenant answer in one table - computed offline from a local mirror, not six console drill-downs or a month-end CSV export.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/acronis){: .btn}

## Instead of clicking through Acronis Cyber Protect Cloud, just ask

**Instead of** Logging into the Acronis console and opening each customer tenant one at a time to see whose backups failed overnight, because partner-level dashboards only show one tenant at a time
**just ask:** *"Whose backups failed last night?"*
<sub>Your agent runs: <code>acronis-cli failures --since 24h</code></sub>

**Instead of** Keeping a side spreadsheet of which backup agents last checked in, because a silently offline agent is the most common way a backup quietly stops and the console won't page you about it
**just ask:** *"Which backup agents have gone offline across all my customers?"*
<sub>Your agent runs: <code>acronis-cli agents stale --older-than 7d</code></sub>

**Instead of** Exporting per-tenant usage reports at month-end and reconciling them by hand against what each customer is actually billed for
**just ask:** *"Where am I billing for protection that isn't running?"*
<sub>Your agent runs: <code>acronis-cli coverage --unprotected</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/acronis/wide-1200x630.png" src="/assets/video/acronis/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/acronis/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Whose backups succeeded, failed, or went stale across every customer last night? | `acronis-cli health` |
| Show me each failed or missed backup in the last 24 hours, newest first | `acronis-cli failures --since 24h` |
| Which customers have gone too long without a good backup (SLA breach)? | `acronis-cli freshness --sla 48h --breached` |
| Which backup agents have silently gone offline across all tenants? | `acronis-cli agents stale --older-than 7d` |
| Did every customer's agents update after the rollout? | `acronis-cli agents compliance` |
| Where am I billing for protection that isn't actually running? | `acronis-cli coverage --unprotected` |
| Which usage has no matching SKU, and which paid SKUs have zero usage? | `acronis-cli reconcile usages` |
| What grew or shrank in usage between two months? | `acronis-cli usages drift --from 2026-04-01 --to 2026-05-01` |
| Give me the full picture on one customer before the call | `acronis-cli customer "<TENANT_ID>"` |
| Which enabled customers are missing users, agents, or offering items? | `acronis-cli tenants audit` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/acronis/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/acronis/guide.md).

## What makes this one different

Most Acronis integrations and MCP servers proxy each question into a live API call - fine for one tenant, but a fleet-wide question becomes a token-burning loop of paginated calls, one per customer, behind token exchange and the datacenter-host dance. This skill follows Acronis's own monitoring guidance: it syncs every tenant, agent, and usage metric into a local SQLite mirror, so cross-tenant questions (health, coverage, freshness, reconcile usages) become one offline join - instant, and the AI sees the answer table, not pages of raw JSON.

It complements the Acronis console rather than replacing it: the portal stays best for configuring protection plans and running restores, while this skill brings every customer tenant into one place for the cross-tenant questions - whose backups failed, which agents are offline, where billing and protection diverge - that partner-level dashboards, scoped to a single tenant, can't answer in one view.

## The pain this closes

- Acronis's own Management Portal Administrator Guide states that partner-level dashboards and reports work only inside one partner tenant - so the question every MSP asks each morning, "whose backups failed," has no single cross-customer view. You drill into each tenant, or you export and stitch in Excel.
- A silently offline backup agent is the most common way a backup quietly stops succeeding - the agent just stops checking in, and nobody notices until a restore isn't there. Catching it means knowing which agents across every tenant have gone quiet, which the per-tenant console makes a manual sweep.
- Acronis's developer monitoring guide tells MSPs that custom multi-tenant monitoring requires fetching each customer tenant's data via the API at intervals and storing it locally - real engineering most MSPs never build, so usage-to-billing reconciliation and backup-SLA reporting fall back to month-end spreadsheets.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/acronis/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/acronis/install.ps1 | iex
```

After install, authenticate once with your Acronis Cyber Protect Cloud credentials, then verify with `acronis-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | acronis-cli health; acronis-cli failures --since 24h; acronis-cli freshness --sla 48h --breached; acronis-cli coverage --unprotected; acronis-cli agents stale --older-than 7d; acronis-cli reconcile usages; acronis-cli customer "<TENANT_ID>"; acronis-cli search "acme" | Allow |
| Write (routine) | acronis-cli tenants create (provision a customer); acronis-cli tenants update (rename or enable/disable); acronis-cli tenants offering-items update (change SKUs); acronis-cli clients create (issue an API client); acronis-cli agent-manager force-agent-update; acronis-cli agent-manager update-agent-update-settings - writes send immediately; --dry-run is an opt-in preview, not a default | Preview with --dry-run, then a reviewed write |
| Destructive / config | acronis-cli tenants delete; acronis-cli clients delete; acronis-cli agent-manager delete-agent; acronis-cli agent-manager delete-agents; acronis-cli idp revoke-token; acronis-cli idp request-token | Human-in-the-loop only |

The skill drives the acronis-cli and acronis-mcp binaries, authenticating with an Acronis API credential read from the environment (ACRONIS_CYBER_PROTECT_BEARER_AUTH, or an OAuth2 client via auth login) and scoped to your datacenter (ACRONIS_DATACENTER) - never logged, never sent anywhere except the Acronis API. Read commands (the cross-tenant rollups, search, reports) cannot change anything. Writes are not gated by default: --dry-run is an opt-in preview flag, so the recommended policy is an agent-level rule - preview with --dry-run, show the exact command, get approval, then run the write. Keep tenant, agent, and OAuth-client deletion plus the token (idp) commands human-only. The strongest control is the scope of the API client you create in the Acronis Management Portal. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/acronis/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Acronis MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Acronis data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Acronis API rate limits?

The local mirror exists so reads stop hitting the API. After the first sync, the cross-tenant views (health, coverage, freshness, agents stale, reconcile usages) run against local SQLite with zero API calls. Live calls respect a --rate-limit throttle, and sync is incremental - it only fetches what changed since the last checkpoint.

### Do I need to be an Acronis partner?

Yes. You authenticate with an API client created in your own Acronis Management Portal (an OAuth2 client, or a bearer token), so you need partner or admin access to the tenants you want to reach. The CLI can only see what that API client is scoped to.

### Which datacenter does it point at?

Whichever hosts your Acronis account. Set ACRONIS_DATACENTER (for example us-cloud or eu2-cloud), or pass --datacenter on auth login. The CLI builds the correct regional API host from it, so you don't hand-assemble the datacenter URL in every call.

### Will this replace the Acronis console?

No. The console stays best for configuring protection plans and running restores. This skill adds the cross-tenant reporting layer the partner dashboards don't - one place to ask whose backups failed, which agents are offline, and where billing and protection diverge.


## Status

Beta. Validated against the Acronis Cyber Protect Cloud API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
