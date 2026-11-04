> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/backup-radar/

# The Backup Radar MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by ScalePad Software Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Backup Radar. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Backup Radar to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask which backups are failing, silent or unwatched across every client and get one ranked answer. Backup Radar already collects every backup result; this skill syncs its v3 and v2 APIs into a local SQLite mirror so your AI can rank today's failures, list the backups nobody acts on, score client compliance for a QBR and plan retirements without paging the portal.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/backup-radar/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/backup-radar)

## Instead of clicking through Backup Radar, just ask

**Instead of** Opening the Backup Radar dashboard every morning, filtering to failures and no-results, then cross-checking the ticket queue to see which ones already have a ticket
**just ask:** *"Which backups are failing or silent today, and which of those have no ticket yet?"*
Your agent runs: `backup-radar-cli check --include-at-risk --agent`

**Instead of** Exporting the inactive list, the unmapped-device report and each policy's warning setting, then reconciling them in a spreadsheet to find backups nobody owns
**just ask:** *"Which backups is nobody watching?"*
Your agent runs: `backup-radar-cli unwatched --reason inactive,unmapped,ticketless --agent`

**Instead of** Building the QBR backup slide by hand from three months of client health screenshots
**just ask:** *"What was Acme Dental's backup compliance rate last quarter, and which devices dragged it down?"*
Your agent runs: `backup-radar-cli scorecard "Acme Dental" --days 90 --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/backup-radar/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which backups are failing or silent today, new or ongoing, and how many tickets are open on each? | `backup-radar-cli check --agent` |
| Which backups is nobody watching: inactive, unmapped to ScalePad, failing with no ticket, no PSA company, or warnings counted as success? | `backup-radar-cli unwatched --agent` |
| What is one client's backup compliance rate over 90 days, with the weekly trend and the five worst devices? | `backup-radar-cli scorecard "Acme Dental" --days 90 --agent` |
| Rank every client by backup compliance, worst first | `backup-radar-cli scorecard --days 90 --agent` |
| What happened to one device's backups day by day this month, with tickets, tags and notes? | `backup-radar-cli timeline "DC01" --days 30 --agent` |
| Which backups have had no result in 45 days and could be retired, with a dry-run retire body for each one it can pin? | `backup-radar-cli stale --days 45 --agent` |
| Does the backup vendor protect devices that Backup Radar does not monitor, or the reverse? | `backup-radar-cli coverage diff --devices vendor-devices.json --agent` |
| Push results from a backup tool Backup Radar cannot parse, validated and batched under the 10,000-result limit | `backup-radar-cli push --from results.csv --dry-run` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/backup-radar/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/backup-radar/guide.md).

## What makes this one different

Most Backup Radar MCP servers proxy each question into one live API call, so "which backups is nobody watching" means paging the v2 backups list, the inactive list, the v3 device list and each policy's warning flag separately and joining them yourself. This skill syncs both API generations into one local SQLite mirror and answers across them in one command: check joins the latest v3 SLA day with v2 status and ticket counts, unwatched reports five blind-spot reasons in one pass, and scorecard scores up to a year of client health offline.

Backup Radar's portal and email digests already flag failures and no-results. This skill adds the cross-cutting answers the portal does not give in one view - new versus ongoing with days in status, blind spots by reason, compliance trends per client, retire candidates with a prepared dry-run body - and lets any MCP agent ask for them. It complements Backup Radar's monitoring and ticketing; it does not replace them.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](https://msp-skills.compoundingteams.com/skills/servosity.md) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

> **Comparing backup vendors?** One of our MSP partners told us, "My guys now spend a tenth of the time on backups." Fixing is included in every Servosity co-managed backup plan, and our engineers tell you what they did. [See how co-managed backup works](https://servosity.com/?utm_source=msp-skills&utm_medium=connector-note&utm_campaign=backup-radar&utm_content=docs)

## The pain this closes

- A backup that never reports sends no failure email. ScalePad's Gflex partner story describes backups that went unnoticed for weeks or months before no-result monitoring, and a technician spending about four hours a day sorting backup mail to find them.
- Backup Radar catches the no-result, but someone still opens the portal every morning to decide what is new versus ongoing and which failures already have a ticket. River Run only acts on failures that persist past a day, because a job that fails once usually self-corrects by the next run.
- Decommissioned devices keep generating no-result noise until someone retires them, and nobody wants to retire the wrong backup from a name-only filter.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/backup-radar/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/backup-radar/install.ps1 | iex
```

After install, authenticate once with your Backup Radar credentials, then verify with `backup-radar-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | check, unwatched, scorecard, timeline, stale, coverage diff, health list, devices, backups list, backups overview, sync, search, analytics | Allow |
| Write (routine) | backups ingest, push, import (post backup results into Backup Radar) | Preview with --dry-run, then a reviewed write |
| Destructive / credential | backups retire (removes backups from monitoring; --close-psa-tickets closes their tickets), auth set-token, auth logout | Human-in-the-loop only |

The skill reads client health, devices and backup lists with whichever keys you give it and mirrors them locally. Four commands write to Backup Radar: backups ingest, push and import post backup results, and backups retire removes backups from monitoring and can close PSA tickets. Keep agents on read commands plus --dry-run previews, and let a human run the retire. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/backup-radar/governance.md).

## Frequently asked questions

### Is there an MCP server for Backup Radar?

Yes - this one. A free, open source MCP server and Claude Code Skill for Backup Radar, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Backup Radar MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `BACKUP_RADAR_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `BACKUP_RADAR_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way backup-radar-mcp runs, and shares MCP configuration with Codex: add backup-radar-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Backup Radar data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need both a ScalePad API key and a Backup Radar API v2 key?

No. The ScalePad API key (BACKUP_RADAR_API_KEY, v3) powers health, devices, backups ingest, backups retire and push. The Backup Radar API v2 key (BACKUP_RADAR_V2_API_KEY) powers backups list, get, results, filters, inactive, retired and overview. Either key alone runs its own commands; check, unwatched, stale and timeline use whichever data you synced and list what they skipped.

### Will this hit my Backup Radar or ScalePad API rate limits?

The analysis commands read the local mirror, so the daily API cost is one sync: v3 client health with a year of history, v3 devices with 90 days, and the v2 backups lists with 31 days. The client sends at most 2 requests per second by default (--rate-limit).

### Can an AI agent retire backups by mistake?

backups retire is the only command that removes backups from monitoring, and it is a v3 write that supports the API's own preview, --server-dry-run (the request is sent and nothing is retired; the global --dry-run sends nothing). stale prepares retire bodies with dry_run true, pinned by ScalePad device id for mapped devices and by exact company, device and job name for unmapped ones; a backup it cannot pin gets a retire_note instead of a body. Over MCP, BACKUP_RADAR_MCP_READ_ONLY=1 removes the backups_retire and backups_ingest tools entirely, and backups_retire is annotated destructive so hosts ask first.

### Does it work for EU ScalePad tenants?

Yes. Set BACKUP_RADAR_BASE_URL to https://eu.api.scalepad.com/backup-radar/v3 (the default is the US host). The Backup Radar API v2 has a single host, https://api.backupradar.com.

## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](https://msp-skills.compoundingteams.com/skills/acronis.md) · [Afi](https://msp-skills.compoundingteams.com/skills/afi.md) · [Axcient x360Recover](https://msp-skills.compoundingteams.com/skills/axcient.md) · [Cove Data Protection](https://msp-skills.compoundingteams.com/skills/cove.md) · [Datto BCDR](https://msp-skills.compoundingteams.com/skills/datto-bcdr.md) · [Dropsuite](https://msp-skills.compoundingteams.com/skills/dropsuite.md) · [Proxmox](https://msp-skills.compoundingteams.com/skills/proxmox.md) · [Servosity](https://msp-skills.compoundingteams.com/skills/servosity.md) · [SkyKick](https://msp-skills.compoundingteams.com/skills/skykick.md) · [Synology](https://msp-skills.compoundingteams.com/skills/synology.md) · [Veeam](https://msp-skills.compoundingteams.com/skills/veeam.md)

## Status

Beta. Validated against the Backup Radar API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
