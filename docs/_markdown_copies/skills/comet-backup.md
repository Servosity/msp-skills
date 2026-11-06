> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/comet-backup/

# The Comet Backup MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Comet Licensing Ltd.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Comet Backup. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Comet Backup to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask which Comet backups failed or went quiet last night and get every protected item across every tenant on your Comet Server in one list, with the error lines that explain each failure. Comet's console and emailed reports answer one customer at a time; this skill reads the whole server, reports per-tenant storage against quota, and keeps a local snapshot history so you can see what changed since yesterday.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/comet-backup/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/comet-backup)

## Instead of clicking through Comet Backup, just ask

**Instead of** Open each customer in the Comet console, or read a pile of per-job report emails, to find which backups failed, are overdue or never ran.
**just ask:** *"Which Comet backups failed, are overdue or never ran across all my customers?"*
Your agent runs: `comet-backup-cli health --state failed,overdue,never --agent`

**Instead of** Open each failed job's log one at a time to find the actual error before you can decide what to fix.
**just ask:** *"Why did last night's Comet backups fail? Group them by error."*
Your agent runs: `comet-backup-cli triage --window 24h --group-by error --agent`

**Instead of** Add up storage, device and Microsoft 365 counts tenant by tenant before invoicing, and find out a customer hit quota when their backup stops.
**just ask:** *"Which tenants are at or above 80 percent of a quota?"*
Your agent runs: `comet-backup-cli usage --over 80 --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/comet-backup/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which protected items failed, are overdue or never backed up, across every tenant? | `comet-backup-cli health --state failed,overdue,never --json` |
| Why did backups fail in the last 24 hours, with the first error lines from each job log? | `comet-backup-cli triage --window 24h --json` |
| Which tenants are at or above 80 percent of a storage, device or Microsoft 365 quota? | `comet-backup-cli usage --over 80 --json` |
| Which offline devices have not finished a job in a week and still own protected items? | `comet-backup-cli devices stale --older-than 7d --billable-only --json` |
| Which devices run a Comet client version that differs from the server? | `comet-backup-cli devices outdated --json` |
| Which backup problems opened or resolved since the previous snapshot? | `comet-backup-cli changes --only health --json` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/comet-backup/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/comet-backup/guide.md).

## What makes this one different

Most Comet Backup integrations proxy one admin API call per question: fine for one user account, not for "every failed or overdue item across 60 tenants". This skill reads every user profile, device and recent job on the server in one command, works out overdue and never-ran items from each schedule, pulls the error lines from the job logs, and keeps local snapshots so what-changed questions have an answer.

Comet has no AI assistant; its console and emailed reports show one tenant or one job at a time. This skill adds the server-wide view, the error lines and the local history on top of the same admin API, and leaves restores, policies and per-device detail where they already live in the console.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](https://msp-skills.compoundingteams.com/skills/servosity.md) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

> **Comparing backup vendors?** One of our MSP partners told us, "My guys now spend a tenth of the time on backups." Fixing is included in every Servosity co-managed backup plan, and our engineers tell you what they did. [See how co-managed backup works](https://servosity.com/?utm_source=msp-skills&utm_medium=connector-note&utm_campaign=comet-backup&utm_content=docs)

## The pain this closes

- Comet's console and its emailed job reports work one customer and one job at a time. To answer "what failed last night across every tenant" you click through each user account or read every report email, every morning.
- A backup that stops running raises no failure: a device that went offline or a schedule that never fired just goes quiet. Magnus Box, a Comet-based backup provider, tells of an MSP who lost a major client because an Exchange backup had been failing silently for two months.
- The console shows today's state only, so "what changed since yesterday" and "which tenants are creeping toward quota" have no history behind them.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/comet-backup/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/comet-backup/install.ps1 | iex
```

After install, authenticate once with your Comet Backup credentials, then verify with `comet-backup-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | health, triage, usage, devices stale, devices outdated, devices online, backup-jobs recent, server version, users list | Allow |
| Credential (reads that return secrets) | users get, users list-full, user-groups get, organizations, storage buckets, storage bucket, constellation last-report | Human-in-the-loop only |
| Write (routine) | rerun-failed --yes, devices run-backup, backup-jobs cancel, users add, users set-profile, storage request-vault, import | Preview with --dry-run, then a reviewed write |
| Customer endpoint actions | devices run-restore, devices update-software, devices uninstall, devices unlock, devices kill | Human-in-the-loop only |
| Destructive / security | users delete, users reset-password, users disable-totp, users revoke-device, backup-jobs abandon | Human-in-the-loop only, explicit confirmation |

Most commands only read: fleet health, failure triage, usage, device lists, job history and server status. Seven reads return credential material (password hashes, TOTP keys, storage keys); the CLI redacts those fields and the MCP server runs them only with confirm: true. Writes reach your tenants and their devices (run a backup or restore, uninstall, delete a user), so preview them with --dry-run and keep a human in the loop. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/comet-backup/governance.md).

## Frequently asked questions

### Is there an MCP server for Comet Backup?

Yes - this one. A free, open source MCP server and Claude Code Skill for Comet Backup, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Comet Backup MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `COMET_BACKUP_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `COMET_BACKUP_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Comet Backup MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Comet Backup data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What Comet account does it need?

A Comet Server admin account. Set COMET_BACKUP_SERVER to your server's host and port (or COMET_BACKUP_BASE_URL to its full URL), then COMET_BACKUP_USERNAME and COMET_BACKUP_PASSWORD. Comet recommends a dedicated API admin with a long random password and no 2FA; for an admin that requires 2FA, start a session with Comet's session-start API and set COMET_BACKUP_SESSION_KEY instead. Personal API tokens are not supported yet. On Comet-Hosted servers, create an Additional User with a local password.

### Does it work with self-hosted and Comet-Hosted servers?

Yes. Each MSP runs one Comet Server, self-hosted or Comet-Hosted, holding all of its tenants; the skill talks only to the server you name and refuses a redirect to any other host.

### Can it run backups or restores?

It can ask an online device to run a backup or a restore through Comet's live-connection API, but those are writes on a customer endpoint: preview them with --dry-run and keep them human-approved. The MCP server sends a write only when the call carries confirm: true. rerun-failed only prints its plan unless you add --yes.

## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](https://msp-skills.compoundingteams.com/skills/acronis.md) · [Afi](https://msp-skills.compoundingteams.com/skills/afi.md) · [Axcient x360Recover](https://msp-skills.compoundingteams.com/skills/axcient.md) · [Backup Radar](https://msp-skills.compoundingteams.com/skills/backup-radar.md) · [CloudAlly](https://msp-skills.compoundingteams.com/skills/cloudally.md) · [Cove Data Protection](https://msp-skills.compoundingteams.com/skills/cove.md) · [Datto BCDR](https://msp-skills.compoundingteams.com/skills/datto-bcdr.md) · [Dropsuite](https://msp-skills.compoundingteams.com/skills/dropsuite.md) · [Proxmox](https://msp-skills.compoundingteams.com/skills/proxmox.md) · [Servosity](https://msp-skills.compoundingteams.com/skills/servosity.md) · [SkyKick](https://msp-skills.compoundingteams.com/skills/skykick.md) · [Synology](https://msp-skills.compoundingteams.com/skills/synology.md) · [Veeam](https://msp-skills.compoundingteams.com/skills/veeam.md)

## Status

Beta. Validated against the Comet Backup API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
