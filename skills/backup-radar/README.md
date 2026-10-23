# Backup Radar + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Backup Radar
> API. Not affiliated with, endorsed by, or sponsored by ScalePad Software Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/backup-radar/">
    <img src="../../docs/assets/video/backup-radar/animated-og.gif" alt="Backup Radar demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/backup-radar/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

<!-- first-party-note:start -->
> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](../servosity) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.
<!-- first-party-note:end -->

Every Backup Radar v3 and v2 endpoint plus a local store that finds the backups nobody is watching. Works with the AI you already use - **ChatGPT** (Plus/Pro+, paused until this connector is reprinted; see [mcp-install.md](./mcp-install.md)), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

> **Security note (until this connector is reprinted).** Do not run HTTP mode
> (`--transport http`), not even on localhost. This build checks a token, but it
> predates the fix release's request size and time limits. The MCP tools can
> also make changes with your credentials. Use the default stdio transport. If
> you set up a tunnel for a remote agent, shut it down. A fix is on the way.

The six agents MSP owners actually use (ChatGPT is paused until this connector is reprinted, see below):

| Your AI agent | How to install the Backup Radar skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `backup-radar-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Paused until this connector is reprinted: ChatGPT needs its HTTP mode, and in this build that mode checks a token but has no request size or time limits yet. See the security note in [mcp-install.md](./mcp-install.md). |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `backup-radar-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

ChatGPT needs a remote HTTPS server, which means this connector's HTTP mode, and that is paused until the connector is reprinted: in this build that mode checks a token but has no request size or time limits yet. See the security note in [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | Paused until this connector is reprinted: Copilot Studio needs its HTTP mode, and in this build that mode checks a token but has no request size or time limits yet. See the security note in [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote and needs the same paused HTTP mode as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Backup Radar MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/backup-radar-v0.1.2/backup-radar-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Backup Radar release on the [releases page](https://github.com/servosity/msp-skills/releases?q=backup-radar).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install backup-radar@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Backup Radar Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.ps1 | iex`. Then authenticate per the README and run `backup-radar-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.sh)
```

The installer drops both `backup-radar-cli` (the CLI) and `backup-radar-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
backup-radar-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/backup-radar/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update backup-radar@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/backup-radar --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/backup-radar --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `backup-radar-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the backup-radar skill from https://github.com/servosity/msp-skills/tree/main/skills/backup-radar. The skill defines how its required CLI (`backup-radar-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (the v3 key from the ScalePad API tab, the v2 key from Backup Radar; either alone is enough for its own commands):

```bash
# macOS / Linux
BACKUP_RADAR_API_KEY=<value> BACKUP_RADAR_V2_API_KEY=<value> backup-radar-cli doctor
```

```powershell
# Windows PowerShell
$env:BACKUP_RADAR_API_KEY = "<value>"; $env:BACKUP_RADAR_V2_API_KEY = "<value>"; backup-radar-cli doctor
```

`doctor` checks config, paths and API reachability, then probes each configured key against the API (`GET /clients/health` for the ScalePad v3 key, `GET /backups/overview` for the Backup Radar v2 key) and reports `valid`, `rejected` or `not verified` per key (`credentials`, `credentials_v2`). The process exits 0 even when a row says ERROR; pass `--fail-on error` in scripts. Either key alone is enough for its own commands.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which backups are failing or silent today, new or ongoing, and how many tickets are open on each? | `backup-radar-cli check --agent` |
| Which backups is nobody watching: inactive, unmapped to ScalePad, failing with no ticket, no PSA company, or warnings counted as success? | `backup-radar-cli unwatched --agent` |
| What is one client's backup compliance rate over 90 days, with the weekly trend and the five worst devices? | `backup-radar-cli scorecard "Acme Dental" --days 90 --agent` |
| Rank every client by backup compliance, worst first | `backup-radar-cli scorecard --days 90 --agent` |
| What happened to one device's backups day by day this month, with tickets, tags and notes? | `backup-radar-cli timeline "DC01" --days 30 --agent` |
| Which backups have had no result in 45 days and could be retired, with a dry-run retire body for each one it can pin? | `backup-radar-cli stale --days 45 --agent` |
| Does the backup vendor protect devices that Backup Radar does not monitor, or the reverse? | `backup-radar-cli coverage diff --devices vendor-devices.json --agent` |
| Push results from a backup tool Backup Radar cannot parse, validated and batched under the 10,000-result limit | `backup-radar-cli push --from results.csv --dry-run` |

Every analysis command reads the local mirror that `backup-radar-cli sync` fills from whichever keys you set, and each one says which collections it skipped because they were never synced. The mirror is scoped to the credentials that filled it (the v3 key, and the v2 key when one was set), so run the analysis commands with the same `BACKUP_RADAR_API_KEY` (environment or `auth set-token`) and the same `BACKUP_RADAR_V2_API_KEY` setting that `sync` used.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Backup Radar integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of our 400 backups has been silent for a week with no ticket, and which client's compliance rate fell last quarter".

This skill syncs Backup Radar into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `check`, `unwatched` and `scorecard` join across the ScalePad v3 client-health and device history and the Backup Radar v2 backup status, inactive list, policy flags and ticket counts - work a stateless API wrapper can't do, because the two API generations never answer in one call.

## The pain this closes

A backup that never reports sends no failure email. ScalePad's [Gflex partner story](https://www.scalepad.com/partner-stories/gflex-reclaimed-hours-with-automated-backup-monitoring) describes backups that went unnoticed for weeks or months until no-result monitoring caught them, and a technician who spent about four hours a day sorting backup mail; [MicroAge Swift Current](https://www.scalepad.com/partner-stories/microage-swift-current-saves-hours-every-week-with-backup-radar) put its manual review at two hours a day. Backup Radar closes the collection gap, but the morning review is still a person in a portal: which failures are new, which have run for a week, which already have a ticket, which backups were never activated, and which decommissioned devices are still generating no-result noise. [River Run](https://www.scalepad.com/partner-stories/river-run-securing-data-and-creating-revenue) only acts on failures that persist past a day because a single failed run usually self-corrects, which is exactly the new-versus-ongoing judgment the portal leaves to the technician.

What this skill does about it:

- `backup-radar-cli check --include-at-risk --agent` - today's failing, silent and at-risk backups ranked, each marked new or ongoing with days in status and open ticket count (without `--include-at-risk`, only failing and no-result backups).
- `backup-radar-cli unwatched --agent` - every backup nobody acts on, with a reason code: inactive, unmapped, ticketless, no-psa, masked.
- `backup-radar-cli stale --days 45 --agent` - silent backups grouped by client, each with a `dry_run: true` retire body (pinned by ScalePad device id for mapped devices, by exact company, device and job name for unmapped ones; v2-only rows get a `retire_note` instead), ready to preview with `backups retire --stdin`.
- `backup-radar-cli scorecard --days 90 --agent` - every client ranked by in-compliance rate, or one client's weekly trend and worst devices for the QBR.
- `backup-radar-cli coverage diff --devices vendor-devices.json --agent` - the devices your backup vendor protects that Backup Radar never sees, and the reverse.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Not with this build. ChatGPT (Plus, Pro, Team, Business, Enterprise, and Education plans; Free does not yet expose Developer Mode) connects only to **remote** MCP servers over HTTPS, which means running the Backup Radar MCP server in HTTP mode. In this build HTTP mode checks a token but has no request size or time limits yet, so do not run it or put it behind a tunnel until the connector is reprinted; the security note in [mcp-install.md](./mcp-install.md) explains why. Until then, use Claude Desktop, Claude Code or Codex.

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Backup Radar credentials once.

### Is my Backup Radar data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need both a ScalePad API key and a Backup Radar API v2 key?

No. The ScalePad API key (`BACKUP_RADAR_API_KEY`, v3) powers `health`, `devices`, `backups ingest`, `backups retire` and `push`. The Backup Radar API v2 key (`BACKUP_RADAR_V2_API_KEY`) powers `backups list`, `get`, `results`, `filters`, `inactive`, `retired` and `overview`. Either key alone runs its own commands; `check`, `unwatched`, `stale` and `timeline` use whichever data you synced and list what they skipped.

### Will this hit my Backup Radar or ScalePad API rate limits?

The analysis commands read the local mirror, so the daily API cost is one `sync`: v3 client health with a year of history, v3 devices with 90 days, and the v2 backups lists with 31 days. The client sends at most 2 requests per second by default (`--rate-limit`).

### Can an AI agent retire backups by mistake?

`backups retire` is the only command that removes backups from monitoring, and it is a v3 write that supports the API's own `--dry-run` preview. `stale` prepares retire bodies with `dry_run: true`, pinned by ScalePad device id for mapped devices and by exact company, device and job name for unmapped ones; a backup it cannot pin gets a `retire_note` instead of a body. Over MCP, `BACKUP_RADAR_MCP_READ_ONLY=1` removes the `backups_retire` and `backups_ingest` tools entirely, and `backups_retire` is annotated destructive so hosts ask first.

### Does it work for EU ScalePad tenants?

Yes. Set `BACKUP_RADAR_BASE_URL` to `https://eu.api.scalepad.com/backup-radar/v3` (the default is the US host). The Backup Radar API v2 has a single host, `https://api.backupradar.com`.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `check`, `unwatched`, `scorecard`, `timeline`, `stale`, `coverage diff`, `health list`, `devices`, `backups list`, `backups overview`, `sync`, `search`, `analytics` | Allow |
| Write (routine) | `backups ingest`, `push`, `import` (post backup results into Backup Radar) | Preview with `--dry-run`, then a reviewed write |
| Destructive / credential | `backups retire` (removes backups from monitoring; `--close-psa-tickets` closes their tickets), `auth set-token`, `auth logout` | Human-in-the-loop only |

Over MCP, `BACKUP_RADAR_MCP_READ_ONLY=1` removes `backups_ingest`, `backups_retire`, `sync`, `export`, `workflow_archive` and the learning-store write tools (`learnings_forget`, `learnings_reject`); the `push` and `import` commands are never exposed over MCP, and flags that read or write local files (`--devices`, `--playbook-file`, `--notes-file`, `--output`, `--db`) are refused from MCP callers.

The strongest control is the **scope you grant the Backup Radar credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Backup Radar API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-19._
