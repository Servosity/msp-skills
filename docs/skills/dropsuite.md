---
layout: default
title: "Dropsuite MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Dropsuite (NinjaOne SaaS Backup) read, plus fleet-wide backup health, coverage gaps and billing reconciliation from one reseller credential."
permalink: /skills/dropsuite/
skill_name: "Dropsuite MCP"
image: /assets/social/dropsuite/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Dropsuite?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Dropsuite, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Dropsuite MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `DROPSUITE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `DROPSUITE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Dropsuite (NinjaOne) MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Dropsuite data safe?"
    a: "Your data stays on your machine. The CLI, MCP server and the local store are local, and the AI sees query results, not raw bulk data. Requests go only to your configured Dropsuite API host, and the reseller tokens and every organization token the CLI reads are withheld from all output, files and the local store. Responses are never cached on disk."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Which Dropsuite credentials do I need?"
    a: "The two values from Settings > API Settings in the Dropsuite partner portal: the Reseller Token (DROPSUITE_RESELLER_TOKEN, sent as X-Reseller-Token) and your reseller Authentication Token (DROPSUITE_ACCESS_TOKEN, sent as X-Access-Token). You do not collect each client's token: --org, or DROPSUITE_ORG for the MCP server, resolves it from the organization list. If your partner API is not on dropsuite.us, set DROPSUITE_HOST."
  - q: "Can the AI change backups or restore data?"
    a: "No. Every command reads, and restore-test record only writes to a local ledger on your machine. The exception is a portal sign-in link: portal get-login-link, organizations get-support-sso-link, locate --sso-link and evidence --with-sso-link mint one, and a new organization link invalidates the previous one. Over MCP those calls return a preview and mint only when the agent repeats the call with confirm: true. For an agent that should only read, set DROPSUITE_MCP_READ_ONLY=1: the sign-in link tools are removed."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Dropsuite credentials once, then run dropsuite-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Dropsuite question in plain language; it runs dropsuite-cli for you."
---

# The Dropsuite MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by NinjaOne, LLC.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Dropsuite. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Dropsuite to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Selling Dropsuite backup to a book of clients? Ask your AI "what is failing or stale across my clients," "which users are not backed up yet," or "do my seat counts match what I invoice" and get one answer instead of opening each organization in the partner portal. One reseller credential resolves every client's token, and health snapshots plus a restore-test ledger build dated evidence the API does not keep.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/dropsuite){: .btn}

## Instead of clicking through Dropsuite, just ask

**Instead of** Opening each client organization in the Dropsuite partner portal every morning and scanning mailboxes, OneDrives, SharePoint sites and Teams for a failed or stuck backup
**just ask:** *"What is failing, stuck or stale across all my Dropsuite clients?"*
<sub>Your agent runs: <code>dropsuite-cli health --stale-after 48h --failing --agent</code></sub>

**Instead of** Comparing each client's Microsoft 365 user list with the mailboxes added to backup, one tenant at a time, to find the new hire nobody added
**just ask:** *"Which users exist in my clients' tenants but are not backed up?"*
<sub>Your agent runs: <code>dropsuite-cli coverage --agent</code></sub>

**Instead of** Copying seat counters out of the portal client by client into a spreadsheet next to the PSA invoice at month end
**just ask:** *"Do my Dropsuite seat counts match what I bill each client?"*
<sub>Your agent runs: <code>dropsuite-cli billing --invoice psa-seats.csv --flagged-only --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/dropsuite/wide-1200x630.png" src="/assets/video/dropsuite/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/dropsuite/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What is failing, stuck or stale across every client right now? | `dropsuite-cli health --stale-after 48h --failing --agent` |
| Which tenant mailboxes are not added to backup, with coverage percent per client? | `dropsuite-cli coverage --agent` |
| Which clients are over-allocated against my PSA invoice, or suspended but still holding seats? | `dropsuite-cli billing --invoice psa-seats.csv --flagged-only --agent` |
| Which deactivated or deleted mailboxes still hold storage, and which invitations were never accepted? | `dropsuite-cli hygiene --older-than 30d --agent` |
| Which client owns this email address, and are its mailbox and OneDrive backed up? | `dropsuite-cli locate jane@example.com --agent` |
| Give me dated proof of protection for one client for an insurer or QBR | `dropsuite-cli evidence --org 26561-14 --window 90d --markdown` |
| What changed since last week: objects that left backup or backups that stopped advancing? | `dropsuite-cli drift --since 7d --agent` |
| Which clients are due a restore test? | `dropsuite-cli restore-test due --every 90d --pick --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/dropsuite/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/dropsuite/guide.md).

## What makes this one different

A plain Dropsuite API wrapper answers one endpoint for one organization per call, so a fleet question turns into the AI listing organizations, fetching each one's token and walking every workload list on its own. health, coverage, billing, hygiene and locate do that walk in one command from your reseller credential and say which organizations they could not read. Health runs are saved locally, so drift and evidence can answer what changed and what was true on a date, which the API does not keep.

It complements the Dropsuite partner portal rather than replacing it: restores, backup settings and onboarding stay there, while this skill brings cross-client health, coverage, seat reconciliation and dated evidence to whichever AI agent you already use. The CLI only reads; the one exception is minting a portal sign-in link, which it does only when you ask.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](/skills/servosity/) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

> **Comparing backup vendors?** One of our MSP partners told us, "My guys now spend a tenth of the time on backups." Fixing is included in every Servosity co-managed backup plan, and our engineers tell you what they did. [See how co-managed backup works](https://servosity.com/?utm_source=msp-skills&utm_medium=connector-note&utm_campaign=dropsuite&utm_content=docs)

## The pain this closes

- A backup that silently stops is the one nobody notices. An r/msp thread, "Office365 backup alternative to Dropsuite" (December 2021), describes a client whose backups were disabled and went undiscovered for over three months. The portal shows one organization at a time, so a morning check across a whole book of clients is a click-through, and the gap shows up at restore time.
- Dropsuite's API is organized per organization: backup data, tenants and retention policies each take that organization's own token, while only the organization list takes the reseller token. Any cross-client question, such as coverage, seats or one user's protection, means collecting every client's token first.
- Proof of protection is asked for after the fact. A cyber-insurance form, an audit or a QBR wants dated evidence of coverage, recent backups and restore testing, and the API answers only the current state.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](/integrations/claude-desktop/) |
| **ChatGPT** (desktop app) | [Step-by-step →](/integrations/chatgpt/) |
| **Claude Code** | [Step-by-step →](/integrations/claude-code/) |
| **Codex CLI** | [Step-by-step →](/integrations/codex/) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](/which-agent/) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/dropsuite/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/dropsuite/install.ps1 | iex
```

After install, authenticate once with your Dropsuite credentials, then verify with `dropsuite-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | health, coverage, billing, hygiene, locate, drift, evidence, organizations list, accounts list, onedrives, sharepoint-sites, teams | Allow |
| Local records | restore-test record, sync, export, profile save, teach (all on your machine; nothing is sent to Dropsuite) | Allow; review what an agent records |
| Credential | portal get-login-link, organizations get-support-sso-link, locate --sso-link, evidence --with-sso-link, auth set-token | Human-in-the-loop only |
| Destructive (local) | local-data purge (needs --yes), learnings forget, auth logout | Human-in-the-loop only |

The skill reads backup status, coverage, seats and organization settings for every client your reseller credential can see, and keeps health snapshots and a restore-test ledger on your machine. It cannot change a backup or run a restore. The one sensitive action is minting a portal sign-in link, which is credential tier: let an agent read freely, and keep a human on sign-in links. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/dropsuite/governance.md).

## Frequently asked questions

### Is there an MCP server for Dropsuite?

Yes - this one. A free, open source MCP server and Claude Code Skill for Dropsuite, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Dropsuite MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `DROPSUITE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `DROPSUITE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Dropsuite (NinjaOne) MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Dropsuite data safe?

Your data stays on your machine. The CLI, MCP server and the local store are local, and the AI sees query results, not raw bulk data. Requests go only to your configured Dropsuite API host, and the reseller tokens and every organization token the CLI reads are withheld from all output, files and the local store. Responses are never cached on disk.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Which Dropsuite credentials do I need?

The two values from Settings > API Settings in the Dropsuite partner portal: the Reseller Token (DROPSUITE_RESELLER_TOKEN, sent as X-Reseller-Token) and your reseller Authentication Token (DROPSUITE_ACCESS_TOKEN, sent as X-Access-Token). You do not collect each client's token: --org, or DROPSUITE_ORG for the MCP server, resolves it from the organization list. If your partner API is not on dropsuite.us, set DROPSUITE_HOST.

### Can the AI change backups or restore data?

No. Every command reads, and restore-test record only writes to a local ledger on your machine. The exception is a portal sign-in link: portal get-login-link, organizations get-support-sso-link, locate --sso-link and evidence --with-sso-link mint one, and a new organization link invalidates the previous one. Over MCP those calls return a preview and mint only when the agent repeats the call with confirm: true. For an agent that should only read, set DROPSUITE_MCP_READ_ONLY=1: the sign-in link tools are removed.


## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](/skills/acronis/) · [Afi](/skills/afi/) · [Axcient x360Recover](/skills/axcient/) · [Backup Radar](/skills/backup-radar/) · [CloudAlly](/skills/cloudally/) · [Comet Backup](/skills/comet-backup/) · [Cove Data Protection](/skills/cove/) · [Datto BCDR](/skills/datto-bcdr/) · [Proxmox](/skills/proxmox/) · [Servosity](/skills/servosity/) · [SkyKick](/skills/skykick/) · [Synology](/skills/synology/) · [Veeam](/skills/veeam/)

## Status

Beta. Validated against the Dropsuite API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
