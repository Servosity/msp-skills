---
layout: default
title: "Nerdio Manager MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "The first non-PowerShell client for the Nerdio Manager for MSP API - cross-account AVD fleet audits, async-job plumbing, and offline search no other Nerdio tool has."
permalink: /skills/nerdio/
skill_name: "Nerdio Manager MCP"
image: /assets/social/nerdio/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Nerdio Manager?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Nerdio Manager, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Nerdio Manager MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `NERDIO_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `NERDIO_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but nerdio-mcp speaks HTTP natively: run `nerdio-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Nerdio Manager data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need to be a Nerdio partner, or run Nerdio Manager for MSP (NMM)?"
    a: "Yes - this targets the NMM Partner REST API, which is the MSP edition (not Nerdio Manager for Enterprise). You create an API client in your own NMM portal under Settings > Integrations > REST API. There is no vendor-global endpoint; the CLI talks to your own instance URL, which you set as NERDIO_BASE_URL."
  - q: "Will this replace the Nerdio Manager portal?"
    a: "No. It is a read-first, cross-account companion. Day-to-day operating still happens in NMM; this is for the fleet-wide questions and scripted automation the portal makes tedious."
  - q: "Why does a change only return a job ID?"
    a: "Every NMM mutation (provisioning, scripted actions, backup, host power) is async and returns a job ID. Run nerdio-cli job wait <job_id> to poll it to a terminal state and exit non-zero if it failed - so your agent never reports \"done\" on a job that actually errored."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Nerdio Manager credentials once, then run nerdio-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Nerdio Manager question in plain language; it runs nerdio-cli for you."
---

# The Nerdio Manager MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Nerdio, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Nerdio Manager. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Nerdio Manager to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask "which host pools have autoscale off across all my customers?" and get one table - not 30 portal logins. Every MSP runs its own Nerdio Manager (NMM) install, so each answer normally means clicking through one tenant at a time. This skill pulls your whole NMM fleet into a local mirror and answers cross-account autoscale, power-state, billing, and Intune questions in a single command.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/nerdio){: .btn}

## Instead of clicking through Nerdio Manager, just ask

**Instead of** Log into each customer's Nerdio Manager portal and click through Autoscale settings to find the host pools you never turned scaling on for
**just ask:** *"Which host pools have autoscale disabled or drifting across all my customers?"*
<sub>Your agent runs: <code>nerdio-cli fleet autoscale-audit</code></sub>

**Instead of** Export each account's usage and invoices and reconcile them in a spreadsheet before the QBR
**just ask:** *"Give me a per-customer billing and unpaid-balance rollup for May"*
<sub>Your agent runs: <code>nerdio-cli fleet billing-rollup --period 2026-05-01:2026-05-31 --unpaid-only</code></sub>

**Instead of** Kick off a scripted action in one tenant, then refresh the Jobs page over and over to see if it finished
**just ask:** *"Run script 42 on these three accounts and tell me when all of them are done"*
<sub>Your agent runs: <code>nerdio-cli scripted-actions fan-run <script_id> --accounts 101,102,103 --wait</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/nerdio/wide-1200x630.png" src="/assets/video/nerdio/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/nerdio/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which host pools have autoscale off or drifting across every customer? | `nerdio-cli fleet autoscale-audit` |
| What is running right now across all accounts, and where? | `nerdio-cli fleet host-estate` |
| What did each customer get billed this period, and who is unpaid? | `nerdio-cli fleet billing-rollup --period 2026-05-01:2026-05-31 --unpaid-only` |
| Which customers' Azure usage spiked month-over-month? | `nerdio-cli usages drift --from 2026-04-01:2026-04-30 --to 2026-05-01:2026-05-31` |
| List every customer account I manage | `nerdio-cli accounts` |
| Show the host pools for one account | `nerdio-cli host-pools list <account_id>` |
| Which Intune devices does this account have? | `nerdio-cli devices list <account_id>` |
| Did that backup or provisioning job actually finish? | `nerdio-cli job wait <job_id>` |
| Run one scripted action across many accounts and wait for all of them | `nerdio-cli scripted-actions fan-run <script_id> --accounts 101,102,103 --wait` |
| Search everything I have synced, offline | `nerdio-cli search <query>` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/nerdio/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/nerdio/guide.md).

## What makes this one different

Most Nerdio Manager MCP servers proxy each question into a single live API call - fine for one record, useless for "across all my customers," because the NMM API only returns one account or one period per call. This skill syncs your whole fleet into a local SQLite mirror with full-text search, so cross-account audits (autoscale, host estate, billing, usage drift) are one offline join the AI reads as a finished answer.

Nerdio Manager's portal is built for operating one tenant at a time. This skill adds the cross-customer, terminal-and-agent surface the portal lacks - fleet-wide autoscale audits, a scriptable "wait until the job finishes" primitive, and offline search - without replacing NMM or changing how it runs your AVD.

## The pain this closes

- AVD autoscale is the main reason to buy Nerdio, but there is no single view of which host pools across which customers actually have it enabled - so idle session hosts quietly bleed Azure spend until someone notices the bill.
- Every NMM install is per-tenant, so a basic question like "how many session hosts are running right now across all clients" means logging into each portal one at a time.
- NMM mutations return a job ID and the portal makes you babysit the Jobs page - there is no scriptable "wait until done" for backup, provisioning, or scripted actions.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/nerdio/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/nerdio/install.ps1 | iex
```

After install, authenticate once with your Nerdio Manager credentials, then verify with `nerdio-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | accounts, fleet autoscale-audit, fleet host-estate, fleet billing-rollup, usages drift, host-pools list, devices list, job wait, search, sync | Allow |
| Write (routine) | host-pools create, host-pools set-autoscale, reservations create / update, recovery-vaults create / link, resource-groups link / set-default, networks link, workspaces create, backup enable / disable, job retry, import | Preview with --dry-run, then a reviewed write |
| Endpoint / infrastructure control | hosts start / stop / restart, desktop-images start / stop, devices sync, scripted-actions run / run-account / fan-run, backup run / restore | Human-in-the-loop - these power, restart, or execute code on live VMs and devices |
| Credential / security | secure-variables list / create / update / delete (and account-* variants), devices bitlocker-keys, devices laps | Human-in-the-loop only - these read or write stored secrets, BitLocker keys, and LAPS passwords |
| Destructive | host-pools delete, reservations delete, recovery-vaults delete-policy, resource-groups unlink / account-unlink, scripted-actions unschedule | Human-in-the-loop only, explicit confirmation |

The skill reads your whole NMM fleet - accounts, host pools, session hosts, Intune devices, billing - and can also make changes: create or delete host pools and reservations, power and restart session hosts, run scripted actions across accounts, and manage secure variables. Reads are safe to automate. Anything that powers, executes, deletes, or touches a stored secret should be previewed with --dry-run and approved by a human. The credential's NMM role is the real ceiling - scope it to only what your workflow needs. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/nerdio/governance.md).

## Frequently asked questions

### Is there an MCP server for Nerdio Manager?

Yes - this one. A free, open source MCP server and Claude Code Skill for Nerdio Manager, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Nerdio Manager MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `NERDIO_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `NERDIO_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but nerdio-mcp speaks HTTP natively: run `nerdio-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Nerdio Manager data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need to be a Nerdio partner, or run Nerdio Manager for MSP (NMM)?

Yes - this targets the NMM Partner REST API, which is the MSP edition (not Nerdio Manager for Enterprise). You create an API client in your own NMM portal under Settings > Integrations > REST API. There is no vendor-global endpoint; the CLI talks to your own instance URL, which you set as NERDIO_BASE_URL.

### Will this replace the Nerdio Manager portal?

No. It is a read-first, cross-account companion. Day-to-day operating still happens in NMM; this is for the fleet-wide questions and scripted automation the portal makes tedious.

### Why does a change only return a job ID?

Every NMM mutation (provisioning, scripted actions, backup, host power) is async and returns a job ID. Run nerdio-cli job wait <job_id> to poll it to a terminal state and exit non-zero if it failed - so your agent never reports "done" on a job that actually errored.


## More RMM connectors

Run more than one RMM tool, or comparing options? These connectors work the same way: [Action1](/skills/action1/) · [Atera](/skills/atera/) · [Auvik](/skills/auvik/) · [ConnectWise Automate](/skills/connectwise-automate/) · [Datto RMM](/skills/datto-rmm/) · [ImmyBot](/skills/immybot/) · [Level](/skills/levelio/) · [N-able N-central](/skills/n-central/) · [NinjaOne](/skills/ninjaone/) · [Tactical RMM](/skills/tactical-rmm/)

## Status

Beta. Validated against the Nerdio Manager API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
