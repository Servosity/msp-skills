---
layout: default
title: "Synology MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every client NAS's Active Backup for Business jobs and storage health, mirrored into one local SQLite database and checked with one command."
permalink: /skills/synology/
skill_name: "Synology MCP"
image: /assets/social/synology/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Synology?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Synology, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Synology MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SYNOLOGY_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `SYNOLOGY_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and synology-mcp speaks HTTP natively: set SYNOLOGY_MCP_HTTP_TOKEN to a long random value, run `synology-mcp --transport http`, and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You register each client NAS once with `synology-cli fleet add` and set its login in environment variables."
  - q: "How do I check 40 client NAS units, not just one?"
    a: "Register each one: `synology-cli fleet add acme --url https://nas.acme.example:5001 --client \"Acme Corp\" --pin-cert --check`. Its login comes from SYNOLOGY_ACME_ACCOUNT and SYNOLOGY_ACME_PASSWORD (or the prefix you pass with --env-prefix), so no password is stored in the registry. `fleet sync` then logs in to every NAS, four at a time by default, and `fleet status` gives one row per client. One unreachable NAS is recorded and does not stop the others."
  - q: "Our NAS accounts use two-factor sign-in. Does that work?"
    a: "Yes. Run the first login with a fresh code in SYNOLOGY_OTP_CODE (or SYNOLOGY_<NAME>_OTP_CODE for a fleet NAS). DSM returns a trusted-device token, which the CLI keeps with that NAS's cached session (owner-only on macOS and Linux) and sends on later logins, so the code is not needed again."
  - q: "My NAS uses DSM's self-signed certificate. Will it connect?"
    a: "Yes, without turning verification off. `fleet add --pin-cert` records the NAS certificate's SHA-256 fingerprint and trusts exactly that certificate afterwards; for a single NAS set SYNOLOGY_TLS_FINGERPRINT. SYNOLOGY_SKIP_TLS_VERIFY=1 trusts any certificate and is only for a network you control. When a NAS renews its certificate, check the new fingerprint in DSM and run `synology-cli fleet add <name> --pin-cert` again; it keeps the address and mirrored history."
  - q: "Can it change my clients' NAS units?"
    a: "Three commands act on a NAS: `abb backup` starts a backup, `abb cancel` cancels a running one, and `session logout` ends the CLI's own DSM session. Each sends immediately unless you add --dry-run. Everything else reads. Set SYNOLOGY_MCP_READ_ONLY=1 and the MCP server drops every command that is not read-only."
  - q: "Is my Synology data safe?"
    a: "Your data stays on your machine. The CLI, MCP server and the local mirror are all local. DSM passwords are read from environment variables and never written to disk; the fleet registry holds only each NAS's URL, client name, certificate setting and the names of its env vars. Credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Synology credentials once, then run synology-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Synology question in plain language; it runs synology-cli for you."
---

# The Synology MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Synology Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Synology. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Synology to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI which client PCs and servers missed last night's Active Backup, which have not had a good backup in three days, and which NAS volumes are degraded or nearly full, and get one answer across every client NAS. DSM answers one NAS at a time in its own web UI. This logs in to each NAS you register, mirrors its Active Backup jobs and storage health into one local database, and answers from there.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/synology){: .btn}

## Instead of clicking through Synology, just ask

**Instead of** Opening every client's DSM web UI each morning to check Active Backup for Business, or reading a pile of per-NAS notification emails
**just ask:** *"Which Active Backup jobs failed, partially failed or never ran last night, across every client NAS?"*
<sub>Your agent runs: <code>synology-cli abb failures --since 24h</code></sub>

**Instead of** Noticing a laptop has not backed up in a week only when the user asks for a restore, because no alert fires for a backup that simply stopped
**just ask:** *"Which protected PCs and servers have had no successful backup in 3 days?"*
<sub>Your agent runs: <code>synology-cli abb stale --older-than 72h</code></sub>

**Instead of** Logging in to each NAS's Storage Manager to look for a degraded pool, a failing disk or a volume running out of room
**just ask:** *"Which client NAS units have a degraded volume, a bad disk, or a volume over 85 percent full?"*
<sub>Your agent runs: <code>synology-cli fleet health --full-pct 85</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/synology/wide-1200x630.png" src="/assets/video/synology/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/synology/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Is every client NAS OK? One row per NAS with backup outcomes, stale devices, worst storage state and sync age. | `synology-cli fleet status` |
| Which Active Backup jobs failed, partially failed or never ran in the last 24 hours? | `synology-cli abb failures --since 24h` |
| Which PCs, servers and VMs have had no successful backup in 36 hours, or never had one? | `synology-cli abb stale --older-than 36h` |
| Which volumes, pools or disks are degraded, failing, hot or nearly full, and how many days until full? | `synology-cli fleet health --full-pct 85 --disk-temp 55` |
| Which task protects BOB-LAPTOP, when did it last back up, and what happened on its recent runs? | `synology-cli abb device "BOB-LAPTOP"` |
| Why did the Office PCs task fail? Show the failed run's error lines and the log from that window. | `synology-cli abb triage "Office PCs" --nas acme` |
| Which tasks have the lowest success rate, or ran far longer than usual last time? | `synology-cli abb trend --since 30d` |
| Which Hyper Backup tasks does a client's NAS run (live, the NAS in SYNOLOGY_BASE_URL), and to what kind of destination? | `synology-cli hyperbackup tasks` |
| Who is in the administrators group on one client's NAS (live, the NAS in SYNOLOGY_BASE_URL)? | `synology-cli group members --group administrators` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/synology/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/synology/guide.md).

## What makes this one different

[rafalr100/synology-mcp](https://github.com/rafalr100/synology-mcp) is a general-purpose DSM server with 71 tools for one NAS, covering files, storage, users, packages and Hyper Backup, with power controls off unless you enable them. [cmeans/mcp-synology](https://github.com/cmeans/mcp-synology) covers File Station, system health, Download Station and Synology Photos, with one config and login per NAS. Neither covers Active Backup for Business. This one is built for it: `fleet sync` logs in to every client NAS you register and mirrors Active Backup jobs, per-device runs and storage health into one local database, so `fleet status`, `abb failures` and `abb stale` answer across every client in one query. A NAS that did not sync reads as not synced, never as OK.

Synology's AI Console brings AI models into Synology's own apps (drafting and summarising in MailPlus and Office, AI search in Drive); it does not answer questions about backups. Active Insight, Synology's cloud monitoring service, offers MSP delegation and alerts on abnormal events per host. This adds the per-device Active Backup answers an MSP asks every morning (failed runs, devices with no recent success, which task protects a machine), across every client NAS, in the AI agent you already use.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](/skills/servosity/) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

## The pain this closes

- Active Backup for Business has no multi-client console: an MSP with almost a hundred client NAS units reported that per-NAS email notifications had become unmanageable and that nothing alerts on a backup that has not run for days.
- SNMP does not help: Synology's MIBs do not expose backup state, so an RMM cannot see whether last night's Active Backup jobs succeeded.
- The workarounds are SSH scripts that read Active Backup's internal SQLite files on every NAS, or a custom dashboard someone has to build and keep running.
- Storage problems hide on the same boxes: a degraded pool, a disk with bad SMART status or a volume about to fill is one Storage Manager screen per client.
- Finding which task protects one machine, and when it last backed up, means knowing which client's NAS to log in to first.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/synology/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/synology/install.ps1 | iex
```

DSM has no API keys: the CLI logs in with a DSM account. For one NAS set SYNOLOGY_BASE_URL, SYNOLOGY_ACCOUNT and SYNOLOGY_PASSWORD and run `synology-cli auth login`. For many, register each NAS with `synology-cli fleet add <name> --url https://<nas>:5001 --pin-cert --check` and set SYNOLOGY_<NAME>_ACCOUNT and SYNOLOGY_<NAME>_PASSWORD. Two-factor accounts add a one-time code on the first login.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet status, fleet health, fleet list, abb failures, abb stale, abb device, abb trend, abb triage, abb tasks, abb results, abb logs, abb transfers, hyperbackup tasks, storage, system, log, package, folder, user, group, sync, search, export. Local writes only: sync fills the local mirror, export --output overwrites the file you name | Allow |
| Write (routine) | abb backup (start a backup now), abb cancel (cancel a running backup), session logout (end the CLI's DSM session); fleet sync (sends each registered NAS's env-var login to that NAS's registered URL, caches its session and rewrites the mirror) | Preview with --dry-run, then a reviewed write |
| Credential | auth login (sends the password to SYNOLOGY_BASE_URL and caches the DSM session and two-factor trusted-device token), auth logout | Human-in-the-loop only |
| Admin (operator only) | fleet add (registers a NAS URL and the env vars whose login is sent there; --check logs in once), fleet remove. CLI only: neither is offered as an MCP tool | Operator only, never an agent |
| Data egress | --deliver webhook:<url> on any command (POSTs that command's output to a URL you name), and feedback --send, which POSTs a feedback note to the configured feedback endpoint | Human-in-the-loop - a webhook sink moves client data off your machine |

Almost everything here reads: Active Backup for Business tasks, results and logs, Hyper Backup tasks, storage and SMART state, system health, users and groups, and the local mirror built from them. Three commands act on a NAS (abb backup, abb cancel, session logout) and send immediately unless you add --dry-run, so preview them and have a human approve. Registering a NAS (fleet add) decides where its password is sent, so it is CLI only and never offered to an agent; SYNOLOGY_MCP_READ_ONLY=1 drops every non-read-only tool from the MCP server. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/synology/governance.md).

## Frequently asked questions

### Is there an MCP server for Synology?

Yes - this one. A free, open source MCP server and Claude Code Skill for Synology, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Synology MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SYNOLOGY_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `SYNOLOGY_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and synology-mcp speaks HTTP natively: set SYNOLOGY_MCP_HTTP_TOKEN to a long random value, run `synology-mcp --transport http`, and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You register each client NAS once with `synology-cli fleet add` and set its login in environment variables.

### How do I check 40 client NAS units, not just one?

Register each one: `synology-cli fleet add acme --url https://nas.acme.example:5001 --client "Acme Corp" --pin-cert --check`. Its login comes from SYNOLOGY_ACME_ACCOUNT and SYNOLOGY_ACME_PASSWORD (or the prefix you pass with --env-prefix), so no password is stored in the registry. `fleet sync` then logs in to every NAS, four at a time by default, and `fleet status` gives one row per client. One unreachable NAS is recorded and does not stop the others.

### Our NAS accounts use two-factor sign-in. Does that work?

Yes. Run the first login with a fresh code in SYNOLOGY_OTP_CODE (or SYNOLOGY_<NAME>_OTP_CODE for a fleet NAS). DSM returns a trusted-device token, which the CLI keeps with that NAS's cached session (owner-only on macOS and Linux) and sends on later logins, so the code is not needed again.

### My NAS uses DSM's self-signed certificate. Will it connect?

Yes, without turning verification off. `fleet add --pin-cert` records the NAS certificate's SHA-256 fingerprint and trusts exactly that certificate afterwards; for a single NAS set SYNOLOGY_TLS_FINGERPRINT. SYNOLOGY_SKIP_TLS_VERIFY=1 trusts any certificate and is only for a network you control. When a NAS renews its certificate, check the new fingerprint in DSM and run `synology-cli fleet add <name> --pin-cert` again; it keeps the address and mirrored history.

### Can it change my clients' NAS units?

Three commands act on a NAS: `abb backup` starts a backup, `abb cancel` cancels a running one, and `session logout` ends the CLI's own DSM session. Each sends immediately unless you add --dry-run. Everything else reads. Set SYNOLOGY_MCP_READ_ONLY=1 and the MCP server drops every command that is not read-only.

### Is my Synology data safe?

Your data stays on your machine. The CLI, MCP server and the local mirror are all local. DSM passwords are read from environment variables and never written to disk; the fleet registry holds only each NAS's URL, client name, certificate setting and the names of its env vars. Credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](/skills/acronis/) · [Afi](/skills/afi/) · [Axcient x360Recover](/skills/axcient/) · [Backup Radar](/skills/backup-radar/) · [Cove Data Protection](/skills/cove/) · [Datto BCDR](/skills/datto-bcdr/) · [Proxmox](/skills/proxmox/) · [Servosity](/skills/servosity/) · [SkyKick](/skills/skykick/) · [Veeam](/skills/veeam/)

## Status

Beta. Validated against the Synology API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
