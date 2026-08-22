---
layout: default
title: "ImmyBot MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every ImmyBot endpoint typed, plus a local SQLite mirror that answers the cross-tenant questions the web UI cannot."
permalink: /skills/immybot/
skill_name: "ImmyBot MCP"
image: /assets/social/immybot/wide-1200x630.png
verification: live-verified
faqs:
  - q: "Is there an MCP server for ImmyBot?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for ImmyBot, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the ImmyBot MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local ImmyBot MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "What credentials do I need?"
    a: "An app registration in Microsoft Entra ID. ImmyBot issues no API key of its own: you register an app, create a client secret, then add that Enterprise Application's object ID as an admin person inside ImmyBot. The CLI needs four values, IMMYBOT_SUBDOMAIN, IMMYBOT_TENANT_ID, IMMYBOT_CLIENT_ID and IMMYBOT_CLIENT_SECRET. Run `immybot-cli doctor` to confirm."
  - q: "Is my ImmyBot data safe?"
    a: "The mirror is a local SQLite file on your own machine and your credentials stay in your environment. Nothing is sent anywhere except to your own ImmyBot instance and to Microsoft Entra ID to mint the token."
  - q: "Can it change things, or only read?"
    a: "Both, but reads are the default and every cross-tenant command is read-only. Writes exist because the API exposes them, and the ones that act on endpoints should sit behind preview-then-approve. Nothing touches a machine unless you ask it to."
  - q: "Will it replace the ImmyBot portal?"
    a: "No. Onboarding, script authoring, and remote control stay in the portal. This is for the questions that span tenants or need history, which is where the portal runs out."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/immybot/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/immybot/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your ImmyBot credentials once; immybot-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a ImmyBot question in plain language; it runs immybot-cli for you."
---

# The ImmyBot MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by ImmyBot, LLC..

**✓ Live-verified by @geekbrownbear (MSP)** against a production tenant · 2026-08-22 · [receipt →](https://github.com/Servosity/msp-skills/pull/276).

Yes - there is an MCP server for ImmyBot. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects ImmyBot to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask "what failed in last night's maintenance window?" and get back a handful of distinct root causes instead of the same error read off forty machines. ImmyBot's API is entirely per-tenant, so the questions that matter most span calls the web UI never joins. This skill types every endpoint, mirrors the fleet into local SQLite with full-text search, and adds the joins on top: which deployment rule actually won on a given machine, how one software title is spread across every tenant, what changed since last night, and which computers silently never finished onboarding.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/immybot){: .btn}

## Instead of clicking through ImmyBot, just ask

**Instead of** Open last night's maintenance session and scroll failed actions machine by machine to work out how many real problems you have.
**just ask:** *"What failed in last night's maintenance window, grouped by root cause?"*
<sub>Your agent runs: <code>immybot-cli session-triage --since 24h</code></sub>

**Instead of** Check each tenant's software inventory one at a time to find who is still below the version you are trying to standardise on.
**just ask:** *"Which tenants are still running Chrome below 140?"*
<sub>Your agent runs: <code>immybot-cli version-spread "Google Chrome" --min-version 140</code></sub>

**Instead of** Read through target assignments, filters, and exceptions by hand to guess why one machine did not get a deployment.
**just ask:** *"Why didn't this computer get the deployment?"*
<sub>Your agent runs: <code>immybot-cli assignment-explain 4821</code></sub>


## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What failed in last night's maintenance window, by root cause? | `immybot-cli session-triage --since 24h` |
| Which tenants are still below a patched version of a title? | `immybot-cli version-spread "Google Chrome" --min-version 140` |
| Why did this computer not get the deployment? | `immybot-cli assignment-explain 4821` |
| What changed across the fleet since yesterday? | `immybot-cli fleet-diff --snapshot   # once, then: fleet-diff --since 24h` |
| Which computers are stuck part way through onboarding? | `immybot-cli onboarding-stalled --older-than 3d` |
| Which deployments have never actually worked? | `immybot-cli deployment-health --only-failing` |
| What does this script reach before I edit it? | `immybot-cli script-blast-radius "Install Chrome"` |
| Where does ImmyBot disagree with my linked PSA or RMM roster? | `immybot-cli psa-reconcile` |
| Everything known about one computer, in one view? | `immybot-cli computer-dossier 4821` |
| Which tenants are drifting on software compliance? | `immybot-cli drift` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/immybot/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/immybot/guide.md).

## What makes this one different

Most ImmyBot integrations proxy each question straight to the live API, one tenant and one record at a time. This skill syncs the whole instance into a local SQLite mirror, so cross-tenant questions resolve as a single local join: instant, offline, and the agent sees an answer rather than pages of raw inventory. It also retains history the API discards, which is what makes fleet-diff and deployment-health possible at all.

ImmyBot's portal is built around one tenant at a time, and its API returns current state with no history. This skill adds the rollups the portal never pre-computes: a cross-tenant software version spread, root-cause clustering for a maintenance window, deployment resolution for a single machine, and reconciliation against a linked PSA or RMM. It does not replace the portal you already work in.

## The pain this closes

- ImmyBot scopes to one tenant at a time. Run dozens of clients and nothing answers which of them is still exposed on a software title you are trying to retire.
- A failed maintenance session shows you N red machines, not the handful of distinct causes behind them. Turning forty failures into three real problems is manual every single time.
- Deployment resolution is the hardest thing to answer by hand. Target assignments, filters, and exceptions interact, and the portal shows the result without showing which rule produced it.
- The API reports current state and keeps no history, so "what changed since yesterday" cannot be asked at all unless you keep your own copy.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/immybot/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/immybot/install.ps1 | iex
```

After install, authenticate once with your ImmyBot credentials, then verify with `immybot-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | session-triage, version-spread, fleet-diff, onboarding-stalled, deployment-health, assignment-explain, computer-dossier, drift, script-blast-radius, psa-reconcile, search, analytics, sync | Allow |
| Endpoint and fleet execution | scripts create-run-adhoc-metascript, maintenance-actions create-latest-action-for-tenants, schedules create-bulk-run-now, maintenance-sessions rerun create, computers registry create-keys, target-assignments create, software create-global-upload | Human-in-the-loop, explicit confirmation. These run code or deploy software on real client machines. |
| Write (routine) | tenants create, persons create, tags create, groups create, preferences update, import | Preview with --dry-run, then a reviewed write |
| Credential / security | auth login, oauth get-access-tokens, access get-get-azure-tenant-auth-details-by-azure-tenant-principal-id, roles create | Human-in-the-loop only |
| Destructive | computers create-bulk-delete, scripts delete-global-by-id, software delete-global-by-identifier, brandings delete-by-id | Require explicit human approval; never autonomous |

The skill authenticates with a Microsoft Entra ID app registration you control, using the client-credentials flow, and its permissions inside ImmyBot are exactly those you granted the person record you created for it. It is read-first: every cross-tenant rollup, dossier, and reconciliation is non-mutating and safe to let an agent run unattended. Writes and deletes exist because the API exposes them, and the commands that reach an endpoint, in particular run-immy-service and the maintenance session and script paths, should sit behind an agent policy of preview with --dry-run then a reviewed write. Keep autonomous agents to reads. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/immybot/governance.md).

## Frequently asked questions

### Is there an MCP server for ImmyBot?

Yes - this one. A free, open source MCP server and Claude Code Skill for ImmyBot, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the ImmyBot MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local ImmyBot MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What credentials do I need?

An app registration in Microsoft Entra ID. ImmyBot issues no API key of its own: you register an app, create a client secret, then add that Enterprise Application's object ID as an admin person inside ImmyBot. The CLI needs four values, IMMYBOT_SUBDOMAIN, IMMYBOT_TENANT_ID, IMMYBOT_CLIENT_ID and IMMYBOT_CLIENT_SECRET. Run `immybot-cli doctor` to confirm.

### Is my ImmyBot data safe?

The mirror is a local SQLite file on your own machine and your credentials stay in your environment. Nothing is sent anywhere except to your own ImmyBot instance and to Microsoft Entra ID to mint the token.

### Can it change things, or only read?

Both, but reads are the default and every cross-tenant command is read-only. Writes exist because the API exposes them, and the ones that act on endpoints should sit behind preview-then-approve. Nothing touches a machine unless you ask it to.

### Will it replace the ImmyBot portal?

No. Onboarding, script authoring, and remote control stay in the portal. This is for the questions that span tenants or need history, which is where the portal runs out.


## More RMM connectors

Run more than one RMM tool, or comparing options? These connectors work the same way: [Action1](/skills/action1/) · [Atera](/skills/atera/) · [Auvik](/skills/auvik/) · [ConnectWise Automate](/skills/connectwise-automate/) · [Datto RMM](/skills/datto-rmm/) · [Level](/skills/levelio/) · [N-able N-central](/skills/n-central/) · [Nerdio Manager](/skills/nerdio/) · [NinjaOne](/skills/ninjaone/) · [Tactical RMM](/skills/tactical-rmm/)

## Status

Beta. Validated against the ImmyBot API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
