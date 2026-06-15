---
layout: default
title: "Rewst MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Operate and monitor Rewst RPA across every client org from the terminal or your AI agent: workflow execution health, failure triage, automation ROI, dormant-workflow cleanup, cross-org config drift, and integration-pack coverage - the multi-tenant answers the web console makes you assemble one org at a time."
permalink: /skills/rewst/
skill_name: "Rewst MCP"
image: /assets/social/rewst/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Rewst?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Rewst, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Rewst MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Rewst MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Rewst data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Is Rewst's API REST or GraphQL?"
    a: "GraphQL-only. This CLI maps the schema to typed commands and includes an api command that exposes every endpoint by interface name for full schema coverage, so you don't hand-write GraphQL for common operations. EU/AU regions set REWST_BASE_URL; the US default is api.rewst.io."
  - q: "Can it build or run workflows?"
    a: "It reads and manages Rewst entities (workflows, triggers, variables, packs); it does not visually author workflow graphs - use the Rewst designer for that. Creating or updating triggers and workflows is gated human-in-the-loop, since those changes affect automation running on live customer tenants."
  - q: "Does it work across all my client organizations?"
    a: "Yes. The rollups (health, failures, drift, coverage) are multi-org by design and take an --org or --parent id; coverage scans a parent org's managed sub-orgs."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Rewst credentials once; rewst-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Rewst question in plain language; it runs rewst-cli for you."
---

# The Rewst MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Rewst, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Rewst. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Rewst to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "is automation healthy for this client?" or "which workflows failed overnight?" and get an answer across every tenant. Rewst's only surface is a GraphQL gateway with no first-party CLI. This skill turns every entity into a typed command and adds cross-org rollups - execution health, failure triage, ROI, dormant workflows, config drift, and pack coverage - the web app makes you assemble one client at a time.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/rewst){: .btn}

## Instead of clicking through Rewst, just ask

**Instead of** Opening the Rewst web app per client to check whether last night's automations succeeded or failed
**just ask:** *"Is automation healthy for this client right now?"*
<sub>Your agent runs: <code>rewst-cli health --org <orgId> --since 24h --agent</code></sub>

**Instead of** Hunting through execution history in the console to find which workflows broke overnight
**just ask:** *"What workflows failed for this org in the last 12 hours?"*
<sub>Your agent runs: <code>rewst-cli failures --org <orgId> --since 12h --agent</code></sub>

**Instead of** Manually comparing two tenants' variables and installed packs when an automation works in one but not the other
**just ask:** *"What does this org have that the other is missing?"*
<sub>Your agent runs: <code>rewst-cli drift --org <orgId> --against <otherOrgId> --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/rewst/wide-1200x630.png" src="/assets/video/rewst/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/rewst/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Is automation healthy for this client right now? | `rewst-cli health --org <orgId> --since 24h --agent` |
| Which workflows failed for this org overnight? | `rewst-cli failures --org <orgId> --since 12h --agent` |
| Which workflows have gone dormant (no runs in 30 days)? | `rewst-cli dormant --org <orgId> --days 30 --agent` |
| How much time did automation save this client? | `rewst-cli roi --org <orgId> --since 30d --agent` |
| What does one org have that another is missing (variables, packs)? | `rewst-cli drift --org <orgId> --against <otherOrgId> --agent` |
| Which managed sub-orgs are missing an integration pack? | `rewst-cli coverage --parent <orgId> --pack microsoft --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/rewst/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/rewst/guide.md).

## What makes this one different

Most Rewst integrations just forward a GraphQL query and hand back raw nodes. This skill turns the whole schema into typed commands, syncs the syncable entities into a local SQLite mirror for offline search, and adds cross-org rollups - health, failures, ROI, drift, coverage - queried live across organizations, that Rewst's gateway has no single endpoint for. Your AI sees the answer, not a page of GraphQL JSON.

Rewst's web app is built to design and run automation one tenant at a time. This complements it by answering the cross-tenant operational questions - which clients are failing, where automation went dormant, which orgs drift - from the terminal or your agent, without replacing the workflow designer.

## The pain this closes

- Rewst is a GraphQL-only product with no CLI and no terminal access - every operational question means clicking through the web app, one client at a time.
- Automation fails quietly. Finding which workflows broke for which tenant overnight means paging execution history org by org.
- Proving automation ROI for a QBR, or finding automation that silently went dormant, means assembling numbers across tenants by hand.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/rewst/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/rewst/install.ps1 | iex
```

After install, authenticate once with your Rewst credentials, then verify with `rewst-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | health, failures, dormant, roi, drift, coverage, and every get / list / search command (plus local sync and analytics) | Allow |
| Write (config) | create / update of non-automation entities such as crates, components, actions, action-options, and forms | Preview with --dry-run, then a reviewed write |
| Automation and trigger changes | workflows create / update, triggers create / update, org-trigger-instances update, packs / pack-configs create / update, org-variables create / update (arm or alter automation on live tenants) | Human-in-the-loop, explicit confirmation |
| Destructive | delete-org-interpreter-responses, pack-delete-responses (real delete mutations) | Human-in-the-loop only, explicit confirmation |
| Admin and identity | organizations create / update, users create / update, user-invites create, org-support-accesses | Operator-only, not for agents |
| Credential | auth set-token (saves your API token to the local config) | Human-in-the-loop only |

The skill is read-first: the six cross-org rollups (health, failures, dormant, roi, drift, coverage) plus get/list/search are read-only and safe to let an agent run. Creating or updating workflows, triggers, packs, and org-variables arms or alters automation that runs on live customer tenants - those are gated human-in-the-loop, as are organization/user admin and saving an API token. The Rewst API token's own permissions are the outer limit on anything the agent can do. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/rewst/governance.md).

## Frequently asked questions

### Is there an MCP server for Rewst?

Yes - this one. A free, open source MCP server and Claude Code Skill for Rewst, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Rewst MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Rewst MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Rewst data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Is Rewst's API REST or GraphQL?

GraphQL-only. This CLI maps the schema to typed commands and includes an api command that exposes every endpoint by interface name for full schema coverage, so you don't hand-write GraphQL for common operations. EU/AU regions set REWST_BASE_URL; the US default is api.rewst.io.

### Can it build or run workflows?

It reads and manages Rewst entities (workflows, triggers, variables, packs); it does not visually author workflow graphs - use the Rewst designer for that. Creating or updating triggers and workflows is gated human-in-the-loop, since those changes affect automation running on live customer tenants.

### Does it work across all my client organizations?

Yes. The rollups (health, failures, drift, coverage) are multi-org by design and take an --org or --parent id; coverage scans a parent org's managed sub-orgs.


## Status

Beta. Validated against the Rewst API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
