---
layout: default
title: "HaloPSA MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every HaloPSA, HaloITSM and HaloCRM feature, plus a local SQLite store and cross-entity views the API can't return."
permalink: /skills/halopsa/
skill_name: "HaloPSA MCP"
image: /assets/social/halopsa/wide-1200x630.png
verification: live-verified
faqs:
  - q: "Is there an MCP server for HaloPSA?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for HaloPSA, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the HaloPSA MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on Plus, Pro, Team, Business, Enterprise, and Education plans (the Free tier does not yet expose Developer Mode). ChatGPT connects to remote MCP servers over HTTPS, not local stdio binaries, so you expose the local HaloPSA MCP server via the mcp-remote bridge or your own HTTPS endpoint. Step-by-step in mcp-install.md."
  - q: "Do I need to know how to code?"
    a: "No. The recommended install is to paste one sentence into Claude Code or Codex and your agent reads SKILL.md and does the install. The fallback is a one-line installer per OS (bash or PowerShell). You enter your HaloPSA API credentials once."
  - q: "Is my HaloPSA data safe?"
    a: "Your data stays on your machine. The CLI and MCP server are local binaries and the SQLite mirror sits under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment, never bundled into this repo or transmitted anywhere by MSP Skills."
  - q: "Will this hit my HaloPSA API rate limits?"
    a: "Almost never. HaloPSA's rate limits aren't publicly documented and vary between cloud-hosted and self-hosted instances. The skill syncs once with sync --full, then incrementally; subsequent triage, SLA, client-card, and cross-client queries run against the local mirror, not the live API. The big-batch queries that get you 429'd with API-passthrough tools become one SQL join here."
  - q: "How is this different from HaloPSA's built-in ChatGPT integration?"
    a: "HaloPSA's built-in integration is great for single-ticket work - rewriting replies, summarizing one ticket, sentiment-flagging. This skill is the cross-client analytics and MSP-owner-on-the-couch layer: questions across thousands of tickets, multi-system queries that join HaloPSA with other tools, and ad-hoc reports HaloPSA's UI doesn't surface. The two complement; you don't pick one."
  - q: "Can I run this on Windows?"
    a: "Yes. The PowerShell installer is the Windows path, and the CLI and MCP binaries are native Windows builds. Cline users on Windows may need a small npx workaround documented in docs/which-agent.md."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use (Claude, ChatGPT, Codex, and so on), billed by your AI provider, not by us."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your HaloPSA credentials once; halopsa-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a HaloPSA question in plain language; it runs halopsa-cli for you."
---

# The HaloPSA MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Halo Service Solutions Ltd..

**✓ Live-verified by Servosity (maintainer)** against a production tenant · 2026-06-05.

Yes - there is an MCP server for HaloPSA. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects HaloPSA to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

MSPs run HaloPSA as the service desk - tickets, SLAs, contracts, and the queue that never stops. Ask your AI "what's about to breach SLA," "who's overloaded," or "what's the whole story for this client," and get an answer the portal can't compose in one shot: cross-entity rollups across tickets, clients, contracts, time, and assets, joined offline in one query instead of clicking through five tabs. A local SQLite mirror means QBR-time questions don't hit rate limits.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/halopsa){: .btn}

## Instead of clicking through HaloPSA, just ask

**Instead of** Clicking through queues, filters, and tabs to find which tickets are about to blow their SLA before the Friday hand-off
**just ask:** *"What's about to breach SLA in the next 24 hours, sorted by time-to-breach?"*
<sub>Your agent runs: <code>halopsa-cli sla breaching --within 24h --team Support --json</code></sub>

**Instead of** Pulling each client's tickets, contracts, assets, and KB links from five different Halo tabs before a client call
**just ask:** *"Give me the whole story for this client on one screen - tickets, contracts, hours left, assets."*
<sub>Your agent runs: <code>halopsa-cli client card "Acme Corp" --json</code></sub>

**Instead of** Eyeballing the board to guess who's drowning before rebalancing the queue at standup
**just ask:** *"Who's overloaded right now - open tickets, hours logged, oldest open ticket per agent?"*
<sub>Your agent runs: <code>halopsa-cli agent workload --team Support --json</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/halopsa/wide-1200x630.png" src="/assets/video/halopsa/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/halopsa/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What's about to breach SLA in the next 24 hours? | `halopsa-cli sla breaching --within 24h --team Support --json` |
| What's the dispatcher view across all agents and teams? | `halopsa-cli triage --team Support --json` |
| Who's overloaded right now? | `halopsa-cli agent workload --team Support --json` |
| What's the whole story for this client, on one screen? | `halopsa-cli client card "Acme Corp" --json` |
| How much contract time is left, and who's tracking over their bank? | `halopsa-cli contracts burn --client "Acme Corp" --month current --json` |
| Which clients have stale tickets aging out that I should close? | `halopsa-cli tickets age-out --status "Awaiting Customer" --stale-days 14 --action-note "Auto-closing per policy" --apply` |
| What changed in Halo since this morning while I was in a meeting? | `halopsa-cli tickets changed-since 09:00 --mine --json` |
| Which KB article should the tech link to for this ticket? | `halopsa-cli kbarticle suggest --ticket 12345 --limit 5` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/halopsa/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/halopsa/guide.md).

## What makes this one different

Most HaloPSA MCP servers and AI integrations proxy each question into a live API call. That's fine for one ticket, but it dies at QBR time, when one cross-client question becomes dozens of paginated REST calls, rate-limit headaches, and a context window full of raw JSON the model has to re-read. This skill syncs HaloPSA into a local SQLite mirror with full-text search, so aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like triage, client card, and contracts burn join across tickets, clients, contracts, time, and assets - work a stateless API wrapper can't do.

It complements HaloPSA's built-in ChatGPT integration rather than replacing it. The built-in integration is great for single-ticket work - rewriting replies, summarizing one ticket, sentiment-flagging - while this skill is the cross-client analytics and situational-awareness layer: questions across thousands of tickets, multi-system joins, and ad-hoc reports the UI doesn't surface. The two are non-overlapping; you don't pick one.

## The pain this closes

- Cost-to-serve is rising faster than contract value, and the squeeze is operational, not sales: queue overload, SLA fires, and unpriced work eating margin on every ticket - a pain MSP owners name repeatedly in r/msp and MSP communities (skills/halopsa/pain-point.md).
- Key-person dependency: "if your best technician left tomorrow, would your clients notice within 30 days?" For most MSPs the answer is yes, because situational awareness lives in one person's head and in a PSA nobody else triages quickly (skills/halopsa/pain-point.md).
- The PSA holds the answer to "what is on fire and for whom," but getting it out means clicking through queues, filters, and tabs - exactly the work that does not scale and that a departing technician takes with them.
- QBR-time questions like "how many backup-failure tickets across all 47 clients last quarter, grouped by contract type?" become 47 paginated REST calls and HaloPSA rate-limit headaches (limits aren't publicly documented; they vary by hosting), with a context window full of raw JSON the model has to re-read.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
```

After install, authenticate once with your HaloPSA credentials, then verify with `halopsa-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | halopsa-cli triage --team Support --json; halopsa-cli sla breaching --within 24h --team Support --json; halopsa-cli agent workload --team Support --json; halopsa-cli client card "Acme Corp" --json; halopsa-cli contracts burn --client "Acme Corp" --month current --json; halopsa-cli tickets changed-since 09:00 --mine --json | Allow - these read and aggregate, they cannot change Halo state |
| Write (routine) | halopsa-cli tickets create; halopsa-cli actions create (ticket updates, notes, assignment); halopsa-cli tickets age-out --status "Awaiting Customer" --stale-days 14 --action-note "Auto-closing per policy" --apply | Writes are NOT gated by default - pass --dry-run first to preview the request, then do a reviewed write; require agent-side approval before sending |
| Destructive / config | halopsa-cli tickets delete; halopsa-cli client delete; halopsa-cli actions delete | Human-in-the-loop only; not for an autonomous agent |

The skill drives the halopsa-cli and halopsa-mcp binaries, authenticating to your own HaloPSA tenant with an OAuth2 client-credentials application you create at Configuration > Integrations > Halo PSA API. Credentials (HALOPSA_CLIENT_ID, HALOPSA_CLIENT_SECRET, HALOPSA_TENANT) are read from the environment only - never written to disk, never logged, never sent anywhere except your Halo endpoint. Read commands (triage, sla breaching, client card, contracts burn, and the other cross-entity views) read and aggregate; they cannot change Halo state. Writes are not gated by a built-in confirmation: --dry-run is an opt-in flag (default off) that previews a request without sending, and --yes is reserved for explicit confirmation of destructive actions, so a raw create or delete sends on first run unless your agent passes --dry-run first. The recommended policy is an agent-level rule: keep autonomous agents to read plus previewed writes, and require a human for anything destructive or configuration-touching. The strongest control is the scope you grant the OAuth application in your Halo tenant - because Halo enforces that scope server-side, the CLI can only do what the application is permitted to do, so a read/report workflow simply should not be granted write or delete permissions. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/halopsa/governance.md).

## Frequently asked questions

### Is there an MCP server for HaloPSA?

Yes - this one. A free, open source MCP server and Claude Code Skill for HaloPSA, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the HaloPSA MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on Plus, Pro, Team, Business, Enterprise, and Education plans (the Free tier does not yet expose Developer Mode). ChatGPT connects to remote MCP servers over HTTPS, not local stdio binaries, so you expose the local HaloPSA MCP server via the mcp-remote bridge or your own HTTPS endpoint. Step-by-step in mcp-install.md.

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex and your agent reads SKILL.md and does the install. The fallback is a one-line installer per OS (bash or PowerShell). You enter your HaloPSA API credentials once.

### Is my HaloPSA data safe?

Your data stays on your machine. The CLI and MCP server are local binaries and the SQLite mirror sits under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment, never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my HaloPSA API rate limits?

Almost never. HaloPSA's rate limits aren't publicly documented and vary between cloud-hosted and self-hosted instances. The skill syncs once with sync --full, then incrementally; subsequent triage, SLA, client-card, and cross-client queries run against the local mirror, not the live API. The big-batch queries that get you 429'd with API-passthrough tools become one SQL join here.

### How is this different from HaloPSA's built-in ChatGPT integration?

HaloPSA's built-in integration is great for single-ticket work - rewriting replies, summarizing one ticket, sentiment-flagging. This skill is the cross-client analytics and MSP-owner-on-the-couch layer: questions across thousands of tickets, multi-system queries that join HaloPSA with other tools, and ad-hoc reports HaloPSA's UI doesn't surface. The two complement; you don't pick one.

### Can I run this on Windows?

Yes. The PowerShell installer is the Windows path, and the CLI and MCP binaries are native Windows builds. Cline users on Windows may need a small npx workaround documented in docs/which-agent.md.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use (Claude, ChatGPT, Codex, and so on), billed by your AI provider, not by us.


## More PSA connectors

Run more than one PSA tool, or comparing options? These connectors work the same way: [Autotask PSA](/skills/autotask/) · [ConnectWise PSA (Manage)](/skills/connectwise-manage/) · [Kaseya BMS](/skills/kaseya-bms/) · [SuperOps](/skills/superops/) · [Syncro](/skills/syncro/)

## Status

Beta. Validated against the HaloPSA API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
