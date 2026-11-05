> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/cloudally/

# The CloudAlly MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by OpenText Corporation.

**✓ Live-verified by @sonofskywalker3 (Servosity)** against a production tenant · 2026-11-05 · [receipt →](https://github.com/Servosity/msp-skills/pull/400).

Yes - there is an MCP server for CloudAlly. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects CloudAlly to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI about CloudAlly SaaS backup for Microsoft 365 and Google Workspace: the latest backup status of every customer, one customer's backup tasks and their entities, running or pausing backups, users, billing and restore jobs, all through CloudAlly's Management API v2 from one CLI and MCP server.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/cloudally/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/cloudally)

## Instead of clicking through CloudAlly, just ask

**Instead of** Clicking into every customer account in the partner portal to look for red backup statuses
**just ask:** *"Show me the latest backup status of every mailbox and site across all my CloudAlly customers."*
Your agent runs: `cloudally-cli distributor-accounts backup-statuses --all`

**Instead of** Opening one customer in the portal to see which backup tasks they have and whether any are paused
**just ask:** *"Switch to this customer and list their backup tasks."*
Your agent runs: `cloudally-cli tasks list --type BACKUP --all`

**Instead of** Paging through a task's entities in the portal to find the one that stopped backing up
**just ask:** *"What state is every mailbox on this backup task in, and when did each last back up?"*
Your agent runs: `cloudally-cli tasks get 64b0e9 --select entities.entity,entities.status`

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What is the latest backup status of every mailbox and site across all my CloudAlly customers? | `cloudally-cli distributor-accounts backup-statuses --all` |
| Which customer accounts do I manage? | `cloudally-cli distributor-accounts list --all` |
| Switch into this customer's account. | `cloudally-cli auth use-account 5f1c2a` |
| Which backup tasks does this customer have? | `cloudally-cli tasks list --type BACKUP --all` |
| What state is every entity of this backup task in? | `cloudally-cli tasks get 64b0e9 --select entities.entity,entities.status` |
| How are all of this account's tasks doing? | `cloudally-cli user status --all` |
| Which restore jobs are running or finished? | `cloudally-cli tasks list --type RESTORE` |
| What are this month's bills across my resellers? | `cloudally-cli distributor-resellers all-bills` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/cloudally/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/cloudally/guide.md).

## What makes this one different

[cloudally](https://github.com/jancotanis/cloudally) (a Ruby gem) and [n8n-nodes-cloudally](https://github.com/davejlong/n8n-nodes-cloudally) (n8n workflow nodes) wrap CloudAlly's documented API. This one is a single binary that works as a CLI and an MCP server over the whole Management API v2. It handles the company and partner logins, refreshes tokens on its own, switches into a customer account with `auth use-account`, and can mirror the read endpoints into a local SQLite store for offline search.

The CloudAlly partner portal shows one customer at a time. This works from the AI agent you already use, answers backup status across every customer in one call, and previews any change with --dry-run before it is sent.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](https://msp-skills.compoundingteams.com/skills/servosity.md) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

> **Comparing backup vendors?** One of our MSP partners told us, "My guys now spend a tenth of the time on backups." Fixing is included in every Servosity co-managed backup plan, and our engineers tell you what they did. [See how co-managed backup works](https://servosity.com/?utm_source=msp-skills&utm_medium=connector-note&utm_campaign=cloudally&utm_content=docs)

## The pain this closes

- Spotting a failing backup means opening every customer account in the CloudAlly partner portal.
- The Management API v2 needs a client-id and client-secret header plus a bearer token on every request, and separate logins for partner and company scope.
- Acting inside one customer from a partner login needs a token minted for that account, and tokens expire.
- The documented export call answers 200 with an empty body and creates no job, so a script that trusts it reports downloads that never happen.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md) |
| **ChatGPT** (desktop app) | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/chatgpt.md) |
| **Claude Code** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-code.md) |
| **Codex CLI** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/codex.md) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](https://msp-skills.compoundingteams.com/which-agent.md) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cloudally/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cloudally/install.ps1 | iex
```

Create an API application in the CloudAlly partner portal (Integrations > REST API), set CLOUDALLY_CLIENT_ID and CLOUDALLY_CLIENT_SECRET, set CLOUDALLY_PARTNER_EMAIL and CLOUDALLY_PARTNER_PASSWORD, then run `cloudally-cli auth login --partner` and `cloudally-cli doctor`: it makes one authenticated read to confirm the token works. For a single company, set CLOUDALLY_EMAIL and CLOUDALLY_PASSWORD and run `cloudally-cli auth login` instead.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | every list, get, details, status, bills, tasks and backup-statuses read; sync, search, export. Local writes only: the response cache, the local store and files you name | Allow |
| Write | backup-now, activate, pause, add-entity and cancel on backup tasks; creating accounts, resellers and users; updating settings and permissions | Human-in-the-loop |
| Restore | the recovery restore commands, which write backed-up items back into live mailboxes, sites and drives | Human-in-the-loop - run --dry-run first |
| Destructive | tasks delete and its distributor and reseller variants (delete a backup task and its data; use cancel to stop a job), remove-entity, unsubscribe and user delete (delete a CloudAlly account), delete-user | Human only |
| Credential | auth login, auth use-account, auth set-token, auth logout, session login and refresh, access-token and sign-in-token, user update-password, import | Human-in-the-loop only |
| Data egress | --deliver webhook:<url> on any command, feedback --send, and feedback when CLOUDALLY_FEEDBACK_ENDPOINT is set with CLOUDALLY_FEEDBACK_AUTO_SEND=true | Human-in-the-loop - a webhook sink moves client data off your machine |
| MCP execute | cloudally_execute calls any endpoint of the CloudAlly spec by ID: each call follows the tiers above, and the tool is not listed while CLOUDALLY_MCP_READ_ONLY=1 (cloudally_read stays, and refuses every state-changing endpoint) | Human-in-the-loop unless CLOUDALLY_MCP_READ_ONLY=1 |

This connector can start restores into live customer mailboxes and, through CloudAlly's own API, delete backup tasks and their data, so it is not read-only. Every command previews with --dry-run, and CLOUDALLY_MCP_READ_ONLY=1 limits an MCP agent to reads. The company login (auth login) reaches one customer; the partner login reaches all of them. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/cloudally/governance.md).

## Frequently asked questions

### Is there an MCP server for CloudAlly?

Yes - this one. A free, open source MCP server and Claude Code Skill for CloudAlly, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the CloudAlly MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CLOUDALLY_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CLOUDALLY_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and cloudally-mcp speaks HTTP natively: set CLOUDALLY_MCP_HTTP_TOKEN, run `cloudally-mcp --transport http`, and serve its /mcp endpoint (http://127.0.0.1:7777/mcp by default) over HTTPS behind an authenticating proxy that signs ChatGPT in with OAuth and adds the bearer header the server requires. ChatGPT cannot send a fixed bearer token, so a bare tunnel gets HTTP 401. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter the CloudAlly API client id and secret once and run `cloudally-cli auth login --partner`.

### What credentials does it need?

A CloudAlly API application key pair (CLOUDALLY_CLIENT_ID and CLOUDALLY_CLIENT_SECRET, from the partner portal under Integrations > REST API) plus one login: `auth login` for a single company account, or `auth login --partner` for every customer. The distributor and reseller commands and `auth use-account` need the partner login.

### Can it change or delete my customers' backups?

It can, because CloudAlly's API can: restores write back into live mailboxes, and deleting a backup task deletes its data. Every command takes --dry-run to show the request first, and governance.md lists the destructive commands an agent should never run on its own. Set CLOUDALLY_MCP_READ_ONLY=1 to give an MCP agent read-only tools.

### Is my CloudAlly data safe?

Your data stays on your machine. The CLI, MCP server and local store are all local, and the MCP server runs over stdio unless you start it with --transport http. Credentials are never bundled or transmitted by MSP Skills.

### Can it download mailbox archives?

Not reliably. The documented export call (recovery export and its distributor and reseller variants) answers 200 with an empty body and creates no job on the live API (seen 2026-10-15). Use the CloudAlly portal for downloads.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](https://msp-skills.compoundingteams.com/skills/acronis.md) · [Afi](https://msp-skills.compoundingteams.com/skills/afi.md) · [Axcient x360Recover](https://msp-skills.compoundingteams.com/skills/axcient.md) · [Backup Radar](https://msp-skills.compoundingteams.com/skills/backup-radar.md) · [Cove Data Protection](https://msp-skills.compoundingteams.com/skills/cove.md) · [Datto BCDR](https://msp-skills.compoundingteams.com/skills/datto-bcdr.md) · [Dropsuite](https://msp-skills.compoundingteams.com/skills/dropsuite.md) · [Proxmox](https://msp-skills.compoundingteams.com/skills/proxmox.md) · [Servosity](https://msp-skills.compoundingteams.com/skills/servosity.md) · [SkyKick](https://msp-skills.compoundingteams.com/skills/skykick.md) · [Synology](https://msp-skills.compoundingteams.com/skills/synology.md) · [Veeam](https://msp-skills.compoundingteams.com/skills/veeam.md)

## Status

Live-verified. Confirmed by @sonofskywalker3 (Servosity) against their own production tenant ([receipt](https://github.com/Servosity/msp-skills/pull/400)). More MSPs run it live in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
