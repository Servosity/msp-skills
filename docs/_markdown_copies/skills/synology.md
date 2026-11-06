> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/synology/

# The Synology MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Synology Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Synology. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Synology to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI which client PCs and servers missed last night's Active Backup, which have not had a good backup in three days, and which NAS volumes are degraded or nearly full, and get one answer across every client NAS. DSM answers one NAS at a time in its own web UI. This logs in to each NAS you register, mirrors its Active Backup jobs and storage health into one local database, and answers from there.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/synology/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/synology)

## Instead of clicking through Synology, just ask

**Instead of** Opening every client's DSM web UI each morning to check Active Backup for Business, or reading a pile of per-NAS notification emails
**just ask:** *"Which Active Backup jobs failed, partially failed or never ran last night, across every client NAS?"*
Your agent runs: `synology-cli abb failures --since 24h`

**Instead of** Noticing a laptop has not backed up in a week only when the user asks for a restore, because no alert fires for a backup that simply stopped
**just ask:** *"Which protected PCs and servers have had no successful backup in 3 days?"*
Your agent runs: `synology-cli abb stale --older-than 72h`

**Instead of** Logging in to each NAS's Storage Manager to look for a degraded pool, a failing disk or a volume running out of room
**just ask:** *"Which client NAS units have a degraded volume, a bad disk, or a volume over 85 percent full?"*
Your agent runs: `synology-cli fleet health --full-pct 85`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/synology/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Is every client NAS OK? One row per NAS with backup outcomes, stale devices, worst storage state and sync age. | `synology-cli fleet status` |
| Which Active Backup jobs failed, partially failed or never ran in the last 24 hours? | `synology-cli abb failures --since 24h` |
| Which machines dropped out of protection in the last 30 days: removed from a task, task deleted, or schedule turned off? | `synology-cli abb unprotected --since 30d` |
| Which PCs, servers and VMs have had no successful backup in 36 hours, or never had one? | `synology-cli abb stale --older-than 36h` |
| Which volumes, pools or disks are degraded, failing, hot or nearly full, and how many days until full? | `synology-cli fleet health --full-pct 85 --disk-temp 55` |
| Which volumes fill within the next 30 days at their current growth rate? | `synology-cli fleet capacity --horizon 30d` |
| Which task protects BOB-LAPTOP, when did it last back up, and what happened on its recent runs? | `synology-cli abb device "BOB-LAPTOP"` |
| Why did the Office PCs task fail? Show the failed run's error lines and the log from that window. | `synology-cli abb triage "Office PCs" --nas acme` |
| Which tasks have the lowest success rate, or ran far longer than usual last time? | `synology-cli abb trend --since 30d` |
| Which machines' newest backup moved far more data than usual, or nothing at all for three runs? | `synology-cli abb change-rate --since 30d` |
| Which Hyper Backup tasks does a client's NAS run (live, the NAS in SYNOLOGY_BASE_URL), and to what kind of destination? | `synology-cli hyperbackup tasks` |
| Who is in the administrators group on one client's NAS (live, the NAS in SYNOLOGY_BASE_URL)? | `synology-cli group members --group administrators` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/synology/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/synology/guide.md).

## What makes this one different

[rafalr100/synology-mcp](https://github.com/rafalr100/synology-mcp) is a general-purpose DSM server with 71 tools for one NAS, covering files, storage, users, packages and Hyper Backup, with power controls off unless you enable them. [cmeans/mcp-synology](https://github.com/cmeans/mcp-synology) covers File Station, system health, Download Station and Synology Photos, with one config and login per NAS. Neither covers Active Backup for Business. This one is built for it: `fleet sync` logs in to every client NAS you register and mirrors Active Backup jobs, per-device runs and storage health into one local database, so `fleet status`, `abb failures` and `abb stale` answer across every client in one query. A NAS that did not sync reads as not synced, never as OK.

Synology's AI Console brings AI models into Synology's own apps (drafting and summarising in MailPlus and Office, AI search in Drive), and DSM 7.4's DSM Agent is a built-in advisor for one NAS (Synology has announced backup validation for a later DSM Agent 2.0); neither answers Active Backup questions across client NAS units. Active Insight, Synology's cloud monitoring service, offers MSP delegation, storage pool status and capacity forecasts, Hyper Backup monitoring and alerts on abnormal events per host; ActiveProtect Manager centrally manages NAS units running Active Backup for Business but needs an ActiveProtect appliance. This adds the per-device Active Backup answers an MSP asks every morning (failed runs, devices with no recent success, which task protects a machine), across every client NAS, in the AI agent you already use.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](https://msp-skills.compoundingteams.com/skills/servosity.md) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

> **Comparing backup vendors?** One of our MSP partners told us, "My guys now spend a tenth of the time on backups." Fixing is included in every Servosity co-managed backup plan, and our engineers tell you what they did. [See how co-managed backup works](https://servosity.com/?utm_source=msp-skills&utm_medium=connector-note&utm_campaign=synology&utm_content=docs)

## The pain this closes

- Without an ActiveProtect appliance, Active Backup for Business has no multi-client console: an MSP with almost a hundred client NAS units reported that per-NAS email notifications had become unmanageable and that nothing alerts on a backup that has not run for days.
- SNMP does not help: DSM's MIBs do not expose backup state, so an RMM cannot see whether last night's Active Backup jobs succeeded.
- The workarounds are SSH scripts that read Active Backup's internal SQLite files on every NAS, or a custom dashboard someone has to build and keep running.
- Storage problems hide on the same boxes: without Active Insight, a degraded pool, a disk with bad SMART status or a volume about to fill is one Storage Manager screen per client.
- Finding which task protects one machine, and when it last backed up, means knowing which client's NAS to log in to first.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/synology/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/synology/install.ps1 | iex
```

DSM has no API keys: the CLI logs in with a DSM account. For one NAS set SYNOLOGY_BASE_URL, SYNOLOGY_ACCOUNT and SYNOLOGY_PASSWORD and run `synology-cli auth login`. For many, register each NAS with `synology-cli fleet add <name> --url https://<nas>:5001 --tls-fingerprint <sha256> --check` (the fingerprint checked in DSM; `--pin-cert` only displays the presented one) and set SYNOLOGY_<NAME>_ACCOUNT and SYNOLOGY_<NAME>_PASSWORD. Two-factor accounts add a one-time code on the first login only, then unset it. The address must be https://; plain http:// to another host is refused.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet status, fleet health, fleet capacity, fleet list, auth status, auth setup, abb failures, abb stale, abb unprotected, abb device, abb trend, abb change-rate, abb triage, abb tasks, abb results, abb logs, abb transfers, hyperbackup tasks, storage, system, log, package, folder, user, group, sync, search, export. Local writes only: sync fills the local mirror, export --output overwrites the file you name | Allow |
| Write (routine) | abb backup (start a backup now), abb cancel (cancel a running backup), session logout (end the CLI's DSM session), each previewing over MCP until confirm is true; fleet sync (sends each registered NAS's env-var login to that NAS's registered URL, logs out when done and rewrites the mirror) | Preview with --dry-run, then a reviewed write |
| Credential | auth login (sends the password to SYNOLOGY_BASE_URL and caches the DSM session and two-factor trusted-device token), auth logout | Human-in-the-loop only |
| Admin (operator only) | fleet add (registers a NAS URL and the env vars whose login is sent there; --check logs in once), fleet remove. CLI only: neither is offered as an MCP tool | Operator only, never an agent |
| Data egress | --deliver webhook:<url> on any command (POSTs that command's output to a URL you name), and feedback --send, which POSTs a feedback note to the configured feedback endpoint | Human-in-the-loop - a webhook sink moves client data off your machine |

Almost everything here reads: Active Backup for Business tasks, results and logs, Hyper Backup tasks, storage and SMART state, system health, users and groups, and the local mirror built from them. Three commands act on a NAS (abb backup, abb cancel, session logout): in a terminal they send immediately unless you add --dry-run, and over MCP they return a preview until called with confirm true, so have a human approve them. Registering a NAS (fleet add) decides where its password is sent, so it is CLI only and never offered to an agent; SYNOLOGY_MCP_READ_ONLY=1 drops every non-read-only tool from the MCP server. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/synology/governance.md).

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

Register each one: `synology-cli fleet add acme --url https://nas.acme.example:5001 --client "Acme Corp" --tls-fingerprint <sha256> --check`, with the certificate fingerprint checked in DSM (`--pin-cert` only displays the one the NAS presents, to compare). Its login comes from SYNOLOGY_ACME_ACCOUNT and SYNOLOGY_ACME_PASSWORD (or the prefix you pass with --env-prefix), so no password is stored in the registry. `fleet sync` then logs in to every NAS, four at a time by default, and `fleet status` gives one row per client. One unreachable NAS is recorded and does not stop the others.

### Our NAS accounts use two-factor sign-in. Does that work?

Yes. Run the first login with a fresh code in SYNOLOGY_OTP_CODE (or SYNOLOGY_<NAME>_OTP_CODE for a fleet NAS). DSM returns a trusted-device token, which the CLI keeps with that NAS's cached session (owner-only on macOS and Linux) and sends on later logins, so the code is not needed again.

### My NAS uses DSM's self-signed certificate. Will it connect?

Yes, without turning verification off. `fleet add --tls-fingerprint <sha256>` pins the NAS certificate's SHA-256 fingerprint and trusts exactly that certificate afterwards (`fleet add --pin-cert` only displays the fingerprint the NAS presents, to compare with DSM's Control Panel > Security > Certificate first); for a single NAS set SYNOLOGY_TLS_FINGERPRINT. SYNOLOGY_SKIP_TLS_VERIFY=1 trusts any certificate and is only for a network you control. When a NAS renews its certificate, check the new fingerprint in DSM, then re-pin with that value: `synology-cli fleet add <name> --tls-fingerprint <sha256>` keeps the address and mirrored history.

### Can it change my clients' NAS units?

Three commands act on a NAS: `abb backup` starts a backup, `abb cancel` cancels a running one, and `session logout` ends the CLI's own DSM session. Each sends immediately unless you add --dry-run. Everything else reads. Set SYNOLOGY_MCP_READ_ONLY=1 and the MCP server drops every command that is not read-only.

### Is my Synology data safe?

Your data stays on your machine. The CLI, MCP server and the local mirror are all local. DSM passwords are read from environment variables and never written to disk; the fleet registry holds only each NAS's URL, client name, certificate setting and the names of its env vars. Credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](https://msp-skills.compoundingteams.com/skills/acronis.md) · [Afi](https://msp-skills.compoundingteams.com/skills/afi.md) · [Axcient x360Recover](https://msp-skills.compoundingteams.com/skills/axcient.md) · [Backup Radar](https://msp-skills.compoundingteams.com/skills/backup-radar.md) · [CloudAlly](https://msp-skills.compoundingteams.com/skills/cloudally.md) · [Comet Backup](https://msp-skills.compoundingteams.com/skills/comet-backup.md) · [Cove Data Protection](https://msp-skills.compoundingteams.com/skills/cove.md) · [Datto BCDR](https://msp-skills.compoundingteams.com/skills/datto-bcdr.md) · [Datto SaaS Protection](https://msp-skills.compoundingteams.com/skills/datto-saas.md) · [Dropsuite](https://msp-skills.compoundingteams.com/skills/dropsuite.md) · [Proxmox](https://msp-skills.compoundingteams.com/skills/proxmox.md) · [Servosity](https://msp-skills.compoundingteams.com/skills/servosity.md) · [SkyKick](https://msp-skills.compoundingteams.com/skills/skykick.md) · [Veeam](https://msp-skills.compoundingteams.com/skills/veeam.md)

## Status

Beta. Validated against the Synology API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
