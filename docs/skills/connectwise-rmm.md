---
layout: default
title: "ConnectWise RMM MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "66 ConnectWise RMM (Asio) partner API operations, plus a local fleet database, patch-compliance rollups, and script runs you can preview, schedule and watch in one command."
permalink: /skills/connectwise-rmm/
skill_name: "ConnectWise RMM MCP"
image: /assets/social/connectwise-rmm/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for ConnectWise RMM?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for ConnectWise RMM, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the ConnectWise RMM MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CONNECTWISE_RMM_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CONNECTWISE_RMM_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Is this for ConnectWise RMM, ConnectWise Automate, or ConnectWise PSA?"
    a: "ConnectWise RMM, the cloud RMM on the ConnectWise Asio platform (formerly Command and Continuum). ConnectWise Automate (LabTech), ConnectWise PSA (Manage) and ConnectWise ScreenConnect (Control) each have their own skill. This one talks to the Asio partner API for companies, sites, endpoints, patching, scripts and tasks, and Asio's own service tickets."
  - q: "What credentials do I need?"
    a: "An API client ID and secret created under Integrations > API Access in the ConnectWise portal. A client ID starting 0e30, 0e31 or 0e32 picks North America, Europe or Australia for you; if yours starts differently, set `CONNECTWISE_RMM_BASE_URL` to your region's API host. If your key was not granted every scope, run `connectwise-rmm-cli scopes --agent` and set the `CONNECTWISE_RMM_OAUTH_SCOPE` line it prints, because Asio rejects a whole token request that asks for one scope it lacks."
  - q: "Will this hit my ConnectWise RMM API rate limits?"
    a: "The local mirror is the point. You sync companies, sites, endpoints and tickets, and roll-ups run against that copy. Availability is checked 400 endpoints per call, patch summaries are cached for 12 hours by default, and the CLI slows down automatically when Asio answers HTTP 429."
  - q: "Can my AI run scripts on client machines?"
    a: "Only through commands you allow. From the CLI, `run` previews by default and schedules nothing until you add `--confirm`. Three other paths schedule immediately: `tasks schedule`, `import` of task records, and the MCP server's generic `connectwise-rmm_execute` tool. For an agent that must never run scripts, set `CONNECTWISE_RMM_MCP_READ_ONLY=1` (the MCP server then drops its write tools and refuses every non-read API call) and do not grant the key the `platform.automation.create` scope. A cached API client wins over the environment variables, so after switching to a narrower key run `connectwise-rmm-cli auth logout` first."
  - q: "Will this replace the ConnectWise RMM portal?"
    a: "No. The portal stays where you configure policies, monitoring and remote access. This skill answers cross-client reporting and triage questions from your AI agent or terminal."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-rmm/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-rmm/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your ConnectWise RMM credentials once, then run connectwise-rmm-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a ConnectWise RMM question in plain language; it runs connectwise-rmm-cli for you."
---

# The ConnectWise RMM MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by ConnectWise, LLC.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for ConnectWise RMM. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects ConnectWise RMM to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "which agents went offline overnight?" or "which clients are behind on patches?" and get one answer across every company. The ConnectWise RMM (Asio) partner API returns per-endpoint rows keyed by IDs and never rolls them up per client. This skill syncs companies, sites and endpoints into a local SQLite mirror, joins live availability and cached patch summaries to it, and previews script runs before anything is scheduled.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/connectwise-rmm){: .btn}

## Instead of clicking through ConnectWise RMM, just ask

**Instead of** Opening each client in the ConnectWise RMM portal and scrolling the device list for agents that stopped checking in
**just ask:** *"Which endpoints have been offline or silent for more than a day, across every client?"*
<sub>Your agent runs: <code>connectwise-rmm-cli fleet stale --older-than 24h --agent</code></sub>

**Instead of** Pulling patch compliance one client at a time and pasting the numbers into a spreadsheet before a client review
**just ask:** *"Which clients are behind on patches, worst first?"*
<sub>Your agent runs: <code>connectwise-rmm-cli fleet patch-posture --min-compliance 90 --agent</code></sub>

**Instead of** Searching device by device to find which machine a user who just called in is logged on to
**just ask:** *"Which machine is jsmith logged on to right now?"*
<sub>Your agent runs: <code>connectwise-rmm-cli fleet whois "jsmith" --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/connectwise-rmm/wide-1200x630.png" src="/assets/video/connectwise-rmm/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/connectwise-rmm/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which endpoints are offline or silent for more than a day, by client? | `connectwise-rmm-cli fleet stale --older-than 24h --agent` |
| Which clients are behind on patches, worst first? | `connectwise-rmm-cli fleet patch-posture --min-compliance 90 --agent` |
| Which machines need a reboot this patch week, and which have someone logged on? | `connectwise-rmm-cli fleet reboot-due --agent` |
| Which machine is this user logged on to? | `connectwise-rmm-cli fleet whois "jsmith" --agent` |
| Give me one client's health card for the review. | `connectwise-rmm-cli fleet scorecard "Acme Corp" --agent` |
| Why did last night's script fail, and on which machines? | `connectwise-rmm-cli fleet task-failures "<task-id>" --agent` |
| Which OAuth scopes did my API key actually get? | `connectwise-rmm-cli scopes --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/connectwise-rmm/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/connectwise-rmm/guide.md).

## What makes this one different

Most ConnectWise RMM integrations, such as the open-source [connectwise-asio-mcp](https://github.com/MSPbotsAI/connectwise-asio-mcp) and [asio-cli](https://github.com/tfournet/asio-cli), work against the live API with no persistent local copy - fine for one device, slow for a cross-client question. This skill syncs your fleet into a local SQLite mirror, batches availability checks 400 endpoints at a time, and caches patch summaries for 12 hours, so a cross-client roll-up is one local join and your AI sees the answer, not thousands of raw device records.

ConnectWise RMM's portal and its built-in AI, ConnectWise Sidekick, work inside ConnectWise. This skill complements them by bringing your fleet into the AI agent you already use, answering cross-client questions in one command without replacing your RMM.

## The pain this closes

- The ConnectWise RMM partner API returns patch compliance as per-endpoint rows keyed by endpoint IDs, with no per-client roll-up, so a cross-client patch report for a client review means joining hundreds of rows to company names by hand, or an afternoon in the portal.
- Offline and silent agents hide across clients, and the availability API caps each call at 400 endpoints and 40 calls per 5 minutes, so nobody sweeps the whole fleet by hand.
- After a fleet script push, every endpoint's result carries its own raw error text, and working out which ones failed for the same reason - and re-running just those - is manual.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectwise-rmm/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectwise-rmm/install.ps1 | iex
```

After install, authenticate once with your ConnectWise RMM credentials, then verify with `connectwise-rmm-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet stale, fleet patch-posture, fleet reboot-due, fleet whois, fleet brief, fleet scorecard, fleet task-failures, scopes, sync, search, run (preview without --confirm), and every list / get command | Allow |
| Endpoint actions | run --confirm, tasks schedule (alias tasks create), import of task records (each line schedules a script run), and the MCP connectwise-rmm_execute tool calling tasks.schedule | Human-in-the-loop, explicit confirmation |
| Destructive | endpoints uninstall (alias endpoints delete; removes the agent, the endpoint stops being managed) | Human-in-the-loop only, explicit confirmation |
| Write (records) | tickets create / update, ticket-notes create, sites create / update, custom-fields set-company / set-site / set-endpoint, import of ticket records | Preview with --dry-run, then a reviewed write |
| Credential | endpoints create-agent-install-token (alias endpoints create), import of endpoint records (each line mints an install token), auth login, auth set-token | Human-in-the-loop only |

The skill is read-first: fleet roll-ups, patch posture, user lookup, task-failure grouping, search and every list or get command are read-only and safe to let an agent run. A small set of commands act on real machines or issue credentials - `run --confirm`, `tasks schedule` and `import` of task records run scripts on endpoints, `endpoints uninstall` removes the agent, and `endpoints create-agent-install-token` issues enrollment tokens - and the MCP server's generic `connectwise-rmm_execute` tool can reach all of them, so keep them human-in-the-loop or set `CONNECTWISE_RMM_MCP_READ_ONLY=1`. The API key's granted scopes are the outer limit on anything an agent can do. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/connectwise-rmm/governance.md).

## Frequently asked questions

### Is there an MCP server for ConnectWise RMM?

Yes - this one. A free, open source MCP server and Claude Code Skill for ConnectWise RMM, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the ConnectWise RMM MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CONNECTWISE_RMM_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CONNECTWISE_RMM_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Is this for ConnectWise RMM, ConnectWise Automate, or ConnectWise PSA?

ConnectWise RMM, the cloud RMM on the ConnectWise Asio platform (formerly Command and Continuum). ConnectWise Automate (LabTech), ConnectWise PSA (Manage) and ConnectWise ScreenConnect (Control) each have their own skill. This one talks to the Asio partner API for companies, sites, endpoints, patching, scripts and tasks, and Asio's own service tickets.

### What credentials do I need?

An API client ID and secret created under Integrations > API Access in the ConnectWise portal. A client ID starting 0e30, 0e31 or 0e32 picks North America, Europe or Australia for you; if yours starts differently, set `CONNECTWISE_RMM_BASE_URL` to your region's API host. If your key was not granted every scope, run `connectwise-rmm-cli scopes --agent` and set the `CONNECTWISE_RMM_OAUTH_SCOPE` line it prints, because Asio rejects a whole token request that asks for one scope it lacks.

### Will this hit my ConnectWise RMM API rate limits?

The local mirror is the point. You sync companies, sites, endpoints and tickets, and roll-ups run against that copy. Availability is checked 400 endpoints per call, patch summaries are cached for 12 hours by default, and the CLI slows down automatically when Asio answers HTTP 429.

### Can my AI run scripts on client machines?

Only through commands you allow. From the CLI, `run` previews by default and schedules nothing until you add `--confirm`. Three other paths schedule immediately: `tasks schedule`, `import` of task records, and the MCP server's generic `connectwise-rmm_execute` tool. For an agent that must never run scripts, set `CONNECTWISE_RMM_MCP_READ_ONLY=1` (the MCP server then drops its write tools and refuses every non-read API call) and do not grant the key the `platform.automation.create` scope. A cached API client wins over the environment variables, so after switching to a narrower key run `connectwise-rmm-cli auth logout` first.

### Will this replace the ConnectWise RMM portal?

No. The portal stays where you configure policies, monitoring and remote access. This skill answers cross-client reporting and triage questions from your AI agent or terminal.


## More RMM connectors

Run more than one RMM tool, or comparing options? These connectors work the same way: [Action1](/skills/action1/) · [Atera](/skills/atera/) · [Auvik](/skills/auvik/) · [ConnectWise Automate](/skills/connectwise-automate/) · [Datto RMM](/skills/datto-rmm/) · [ImmyBot](/skills/immybot/) · [Level](/skills/levelio/) · [N-able N-central](/skills/n-central/) · [Nerdio Manager](/skills/nerdio/) · [NinjaOne](/skills/ninjaone/) · [Tactical RMM](/skills/tactical-rmm/)

## Status

Beta. Validated against the ConnectWise RMM API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
