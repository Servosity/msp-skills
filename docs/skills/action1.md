---
layout: default
title: "Action1 MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Action1 endpoint, plus fleet-wide patch and vulnerability views across all your organizations."
permalink: /skills/action1/
skill_name: "Action1 MCP"
image: /assets/social/action1/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Action1?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Action1, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Action1 MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Action1 MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Action1 data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Which Action1 region and credentials do I need?"
    a: "An API client (Client ID + Client Secret) from your Action1 console's API Credentials page, plus ACTION1_REGION set to us, eu, or au to match your console URL. The CLI mints and refreshes the bearer token for you. Scope the client to read-only permissions if you only need reporting."
  - q: "Does this replace the Action1 console?"
    a: "No. The console stays your place to configure automations and approve patches interactively. This skill answers the cross-organization questions the console shows one org at a time - fleet patch posture, CVE blast-radius, per-client scorecards - and lets your AI agent run them."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Action1 credentials once; action1-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Action1 question in plain language; it runs action1-cli for you."
---

# The Action1 MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Action1 Corporation.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Action1. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Action1 to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask "which endpoints across all my clients are missing the most patches?" and get one ranked list - no clicking org by org. Action1's API and console are siloed per client organization; this skill syncs every org into a local mirror so patch posture, CVE blast-radius, stale agents, and a per-client scorecard become single fleet-wide commands your AI agent runs from the terminal.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/action1){: .btn}

## Instead of clicking through Action1, just ask

**Instead of** Logging into Action1 and switching to each client organization one at a time to read its missing-updates count off the dashboard
**just ask:** *"Which endpoints across all my clients are missing the most patches?"*
<sub>Your agent runs: <code>action1-cli fleet patch-posture</code></sub>

**Instead of** Exporting each org's vulnerability list and merging spreadsheets to find which CVE hits the most machines
**just ask:** *"Rank our CVEs by how many endpoints they hit, known-exploited ones first"*
<sub>Your agent runs: <code>action1-cli fleet vuln-triage --kev-only</code></sub>

**Instead of** Hunting through every organization for agents that quietly stopped checking in
**just ask:** *"Which agents have gone dark across the whole fleet in the last two weeks?"*
<sub>Your agent runs: <code>action1-cli fleet stale --days 14 --include-offline</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/action1/wide-1200x630.png" src="/assets/video/action1/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/action1/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which endpoints across all clients are missing the most patches? | `action1-cli fleet patch-posture` |
| Which CVEs hit the most machines, weighted by severity and known-exploited status? | `action1-cli fleet vuln-triage --kev-only` |
| Which agents have stopped checking in across the fleet? | `action1-cli fleet stale --days 14` |
| What is the patch-and-vulnerability posture for each client organization? | `action1-cli fleet org-scorecard` |
| Which endpoints are waiting on a reboot to finish a patch cycle? | `action1-cli fleet reboot-pending` |
| Rank every endpoint by overall risk (missing updates, open CVEs, reboot, staleness) | `action1-cli fleet health-score` |
| What software is installed across the whole fleet, deduped by version? | `action1-cli fleet software-rollup` |
| What changed since the last sync - what got remediated, what is newly missing? | `action1-cli fleet patch-drift` |
| List the managed endpoints in one client organization | `action1-cli endpoints managed <orgId>` |
| What updates are available or missing for one organization? | `action1-cli updates org-id-get <orgId>` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/action1/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/action1/guide.md).

## What makes this one different

Most Action1 integrations proxy each question into a live API call, one organization at a time - fine for a single record, useless when you need a number that spans every client. This skill syncs all your organizations into a local SQLite mirror, so cross-org rollups (patch-posture, vuln-triage, org-scorecard) are one offline query the org-siloed API cannot answer in a single call.

Action1's console shows you one organization at a time; this skill adds the fleet-wide view across every client organization at once - in your terminal or your AI agent, as a number you can pipe into a report instead of a dashboard you click through. It complements the console, it does not replace it.

## The pain this closes

- Action1 organizes everything one client organization at a time, but the questions that matter at patch-review time - who is most behind, which CVE is everywhere - are fleet-wide, so MSPs end up clicking through every org or exporting and merging spreadsheets to answer them.
- At QBR time there is no single per-client posture number to hand an account manager; you assemble it by hand from each organization's dashboard.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/action1/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/action1/install.ps1 | iex
```

After install, authenticate once with your Action1 credentials, then verify with `action1-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet patch-posture, fleet vuln-triage, fleet org-scorecard, endpoints managed <orgId>, search, analytics, export, doctor | Allow |
| Write (routine) | endpoints groups, endpoints managed-id-patch, reports org-id-custom-post, settings org-id-post, automations policies-schedules-org-id-post, import | Preview with --dry-run, then a reviewed write |
| Endpoint / patch execution | automations policies-instances-org-id-post (run an automation), updates approvals updates-org-id, endpoints managed-id-remote-sessions-post, scripts org-id-post | Human-in-the-loop; never unattended |
| Credential / security | oauth2, users post, roles post, roles id-patch | Human-in-the-loop only |
| Destructive / account | endpoints managed-id-delete, organizations org-id-delete, users id-delete, scripts org-id-id-delete, enterprise request-closure | Human-in-the-loop only, explicit confirmation |

The skill reads freely - fleet rollups, lists, reports, search, export - and those reads change nothing. Routine writes (config edits, including the read-named import command) should be previewed with --dry-run, then approved. Endpoint-level actions like running an automation, approving updates for deployment, or opening a remote session, plus token-minting, user/role management, and deletes, are endpoint-execution, credential, or destructive tier - keep them human-in-the-loop and scope your API client to only the permissions your workflow needs. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/action1/governance.md).

## Frequently asked questions

### Is there an MCP server for Action1?

Yes - this one. A free, open source MCP server and Claude Code Skill for Action1, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Action1 MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Action1 MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Action1 data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Which Action1 region and credentials do I need?

An API client (Client ID + Client Secret) from your Action1 console's API Credentials page, plus ACTION1_REGION set to us, eu, or au to match your console URL. The CLI mints and refreshes the bearer token for you. Scope the client to read-only permissions if you only need reporting.

### Does this replace the Action1 console?

No. The console stays your place to configure automations and approve patches interactively. This skill answers the cross-organization questions the console shows one org at a time - fleet patch posture, CVE blast-radius, per-client scorecards - and lets your AI agent run them.


## More RMM connectors

Run more than one RMM tool, or comparing options? These connectors work the same way: [Atera](/skills/atera/) · [Auvik](/skills/auvik/) · [ConnectWise Automate](/skills/connectwise-automate/) · [Datto RMM](/skills/datto-rmm/) · [ImmyBot](/skills/immybot/) · [Level](/skills/levelio/) · [N-able N-central](/skills/n-central/) · [Nerdio Manager](/skills/nerdio/) · [NinjaOne](/skills/ninjaone/) · [Tactical RMM](/skills/tactical-rmm/)

## Status

Beta. Validated against the Action1 API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
