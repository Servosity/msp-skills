---
layout: default
title: "Kaseya VSA 10 MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every VSA 10 API endpoint, plus a local fleet database that answers patch, health, and software questions across all your clients offline."
permalink: /skills/kaseya-vsa/
skill_name: "Kaseya VSA 10 MCP"
image: /assets/social/kaseya-vsa/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Kaseya VSA 10?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Kaseya VSA 10, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Kaseya VSA 10 MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `KASEYA_VSA_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `KASEYA_VSA_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Kaseya VSA MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Kaseya VSA data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Is this for Kaseya VSA 10 or classic VSA 9?"
    a: "VSA 10, the platform formerly called VSA X, with an instance host such as yourcompany.vsax.net. It uses the VSA 10 REST API (/api/v3) with an API token ID and secret created under Administration > Configuration > API Access. Classic on-premises VSA 9 has a different API and is not supported."
  - q: "Will this hit my VSA 10 API rate limits?"
    a: "It is built to stay inside them. VSA 10 limits each API token per hour, so the skill syncs once and then answers fleet questions from the local mirror without new API calls. The CLI paces itself to the server's rate-limit headers, and kaseya-vsa-cli rate-limits shows what is left in the current window."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Kaseya VSA 10 credentials once, then run kaseya-vsa-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Kaseya VSA 10 question in plain language; it runs kaseya-vsa-cli for you."
---

# The Kaseya VSA 10 MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Kaseya US LLC.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Kaseya VSA 10. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Kaseya VSA 10 to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask which devices across every client are offline, unprotected or missing a critical patch, and get the list. Kaseya VSA 10 plus your AI agent reads a local mirror of your whole fleet, so the cross-client questions the portal answers one policy or one device at a time become one reproducible answer. It also runs a VSA script on a device and hands back the output in one call.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/kaseya-vsa){: .btn}

## Instead of clicking through Kaseya VSA 10, just ask

**Instead of** Build a patch status report in Advanced Reporting one policy at a time, then merge the exports in a spreadsheet
**just ask:** *"Which critical updates are missing across all my clients, and on how many devices?"*
<sub>Your agent runs: <code>kaseya-vsa-cli patch-exposure --by kb --severity critical,important --agent</code></sub>

**Instead of** Open each organization in the VSA 10 console and read down the device list for offline machines and disabled antivirus
**just ask:** *"Which devices are offline, unprotected or low on disk, across every client?"*
<sub>Your agent runs: <code>kaseya-vsa-cli health --check offline,av,firewall,disk --agent</code></sub>

**Instead of** Run a script from the console, wait, then open the execution log to copy the output
**just ask:** *"Run the disk cleanup script on FRONTDESK-01 and tell me what it printed"*
<sub>Your agent runs: <code>kaseya-vsa-cli scripts run-wait <script-id> --device FRONTDESK-01 --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/kaseya-vsa/wide-1200x630.png" src="/assets/video/kaseya-vsa/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/kaseya-vsa/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which pending OS updates are exposed on the most devices, across every client? | `kaseya-vsa-cli patch-exposure --by kb --severity critical,important --agent` |
| Which clients carry the most critical patch exposure? | `kaseya-vsa-cli patch-exposure --by org --severity critical --agent` |
| Which devices are offline, without working antivirus, or with the firewall off? | `kaseya-vsa-cli health --check offline,av,firewall --offline-for 72h --agent` |
| Which devices are low on disk space? | `kaseya-vsa-cli health --check disk --disk-below 15 --agent` |
| Which versions of Google Chrome are installed, and on how many devices? | `kaseya-vsa-cli software "Google Chrome" --versions --agent` |
| Which devices with software inventory are missing our security agent? | `kaseya-vsa-cli software "Huntress Agent" --absent --agent` |
| Who did what in the tenant this week, including remote control and API token changes? | `kaseya-vsa-cli audit-logs digest --since 7d --agent` |
| What is the current state of one device: notifications, security posture, disks and pending updates? | `kaseya-vsa-cli devices brief FRONTDESK-01 --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/kaseya-vsa/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/kaseya-vsa/guide.md).

## What makes this one different

This skill syncs your VSA 10 organizations, sites and device inventory into a local SQLite mirror, so fleet-wide questions (patch exposure by KB, health failures, software version spread) are one offline query instead of one live API call per device. Most VSA integrations, and an MCP server that only forwards each tool call to the API, answer one record at a time and spend your hourly API budget doing it.

The VSA 10 console and Advanced Reporting stay your place for remote control, policy and scheduled reports; this skill answers the cross-client questions in the AI agent you already use, from a local mirror, and complements the portal rather than replacing it.

## The pain this closes

- VSA 10's own patch status reporting is limited to one policy at a time (Kaseya's Advanced Reporting release note, February 2026), so a fleet-wide view of what is missing means one report per policy and a spreadsheet to join them.
- Offline agents, disabled antivirus and full disks hide across dozens of client organizations: you find out a machine stopped reporting when the client calls.
- Running a script and getting its output back is several clicks per device, and the output lives in an execution log the technician has to go and read.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/kaseya-vsa/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/kaseya-vsa/install.ps1 | iex
```

After install, authenticate once with your Kaseya VSA 10 credentials, then verify with `kaseya-vsa-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | devices list, health, patch-exposure, software, audit-logs digest, devices brief, sync, search | Allow |
| Write (routine) | organizations create/update, sites create/update, groups create/update, devices move, devices publish, notifications create, custom-fields assign/unassign/update-assigned, webhooks create/update, import | Preview with --dry-run, then a reviewed write |
| Endpoint execution | scripts run, scripts run-wait, tasks run, tasks run-wait, workflows run, workflows cancel-executions | Human-in-the-loop only: these run code on customer devices |
| Credential / secret-bearing | any command that can return a stored secret: custom-fields get/list, devices list-custom-fields, organization-, site- and group-custom-fields, assets list and get-device (AssetInfo holds the Windows product key), groups get-package (agent install link), webhooks get/list and regenerate-secret-key, auth | Human-in-the-loop only |
| Destructive | organizations delete, sites delete, groups delete, notifications delete, webhooks delete, local-data purge | Human-in-the-loop only, explicit confirmation |

The skill reads devices, assets, organizations, policies, scripts, tasks and audit logs, and keeps a local mirror for offline questions; custom-field values, product keys and webhook signing keys are never stored in it. It can also create, change and delete organizations, sites, groups, notifications and webhooks, and it can run scripts, tasks and workflows on customer endpoints. Let an agent read freely, preview routine writes with --dry-run, and keep a human approving deletes, credential reads and anything that runs on an endpoint. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/kaseya-vsa/governance.md).

## Frequently asked questions

### Is there an MCP server for Kaseya VSA 10?

Yes - this one. A free, open source MCP server and Claude Code Skill for Kaseya VSA 10, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Kaseya VSA 10 MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `KASEYA_VSA_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `KASEYA_VSA_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Kaseya VSA MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Kaseya VSA data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Is this for Kaseya VSA 10 or classic VSA 9?

VSA 10, the platform formerly called VSA X, with an instance host such as yourcompany.vsax.net. It uses the VSA 10 REST API (/api/v3) with an API token ID and secret created under Administration > Configuration > API Access. Classic on-premises VSA 9 has a different API and is not supported.

### Will this hit my VSA 10 API rate limits?

It is built to stay inside them. VSA 10 limits each API token per hour, so the skill syncs once and then answers fleet questions from the local mirror without new API calls. The CLI paces itself to the server's rate-limit headers, and kaseya-vsa-cli rate-limits shows what is left in the current window.


## More RMM connectors

Run more than one RMM tool, or comparing options? These connectors work the same way: [Action1](/skills/action1/) · [Atera](/skills/atera/) · [Auvik](/skills/auvik/) · [ConnectWise Automate](/skills/connectwise-automate/) · [ConnectWise RMM](/skills/connectwise-rmm/) · [Datto RMM](/skills/datto-rmm/) · [ImmyBot](/skills/immybot/) · [Level](/skills/levelio/) · [N-able N-central](/skills/n-central/) · [Nerdio Manager](/skills/nerdio/) · [NinjaOne](/skills/ninjaone/) · [Tactical RMM](/skills/tactical-rmm/)

## Status

Beta. Validated against the Kaseya VSA 10 API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
