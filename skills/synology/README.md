# Synology + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Synology
> API. Not affiliated with, endorsed by, or sponsored by Synology Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/synology/">
    <img src="../../docs/assets/video/synology/animated-og.gif" alt="Synology demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/synology/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

<!-- first-party-note:start -->
> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](../servosity) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.
<!-- first-party-note:end -->

Every client NAS's Active Backup for Business jobs and storage health, mirrored into one local SQLite database and checked with one command. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Synology skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `synology-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `synology-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `synology-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `synology-mcp --transport http` and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `synology-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Synology MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/synology-v0.1.0/synology-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Synology release on the [releases page](https://github.com/servosity/msp-skills/releases?q=synology).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install synology@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Synology Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.ps1 | iex`. Then authenticate per the README and run `synology-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.sh)
```

The installer drops both `synology-cli` (the CLI) and `synology-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
synology-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update synology@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/synology --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/synology --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `synology-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the synology skill from https://github.com/servosity/msp-skills/tree/main/skills/synology. The skill defines how its required CLI (`synology-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

DSM has no API keys: the CLI logs in with a DSM account in the administrators group (Active Backup for Business management and Storage Manager calls need administrator rights; Active Backup's own Operator and Manager delegation roles are not tested with this CLI). Use a dedicated account per client NAS. Passwords are read from environment variables and never written to disk.

**One NAS** - macOS / Linux:

```bash
export SYNOLOGY_BASE_URL=https://192.168.1.50:5001 SYNOLOGY_ACCOUNT=<account> SYNOLOGY_PASSWORD=<password>
synology-cli auth login
```

Windows (PowerShell):

```powershell
$env:SYNOLOGY_BASE_URL = "https://192.168.1.50:5001"; $env:SYNOLOGY_ACCOUNT = "<account>"; $env:SYNOLOGY_PASSWORD = "<password>"
synology-cli auth login
```

**Every client NAS** - register each one, then sync them all. The login for a NAS named `acme` comes from `SYNOLOGY_ACME_ACCOUNT` and `SYNOLOGY_ACME_PASSWORD` (or the prefix you pass with `--env-prefix`). macOS / Linux:

```bash
export SYNOLOGY_ACME_ACCOUNT=<account> SYNOLOGY_ACME_PASSWORD=<password>
```

Windows (PowerShell):

```powershell
$env:SYNOLOGY_ACME_ACCOUNT = "<account>"; $env:SYNOLOGY_ACME_PASSWORD = "<password>"
```

Then, in either shell:

```bash
synology-cli fleet add acme --url https://nas.acme.example:5001 --client "Acme Corp" --pin-cert --check
synology-cli fleet sync
synology-cli fleet status
```

The fleet reports (`fleet status`, `fleet health`, `abb failures`, `abb stale`, `abb device`, `abb trend`) read every registered NAS from the local mirror, and `abb triage` takes `--nas <name>`. The other live commands (`abb tasks`, `abb backup`, `hyperbackup`, `storage`, `system`, `user`, `group` and the rest) talk to the one NAS in `SYNOLOGY_BASE_URL`, so point it at a client's NAS (with that NAS's `SYNOLOGY_ACCOUNT` and `SYNOLOGY_PASSWORD`) before asking about it.

`--pin-cert` records the NAS certificate's SHA-256 fingerprint (most NAS units serve DSM's self-signed certificate) and trusts exactly that certificate afterwards; for a single NAS set `SYNOLOGY_TLS_FINGERPRINT` instead. `--check` logs in once and reports whether Active Backup for Business is installed.

**Two-factor accounts:** add a fresh code to the first login only - `SYNOLOGY_OTP_CODE` for one NAS, `SYNOLOGY_ACME_OTP_CODE` for a fleet NAS (`$env:SYNOLOGY_OTP_CODE = "123456"` in PowerShell). DSM returns a trusted-device token, which the CLI keeps with that NAS's cached session and sends on later logins.

Every login first checks that the address answers as DSM's WebAPI, so an address that is not a DSM (a router, a web page) never receives the password; another Synology would, so check each address you register. `synology-cli doctor` checks config and paths, confirms `SYNOLOGY_BASE_URL` answers as a DSM WebAPI, then logs in and makes one authenticated read, so it reports the login as valid, rejected (wrong account or password) or needing a two-factor code; `fleet add --check` does the same for a fleet NAS.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Is every client NAS OK? One row per NAS: backups, stale devices, storage, sync age | `synology-cli fleet status` |
| Which Active Backup jobs failed, partially failed or never ran in the last 24 hours? | `synology-cli abb failures --since 24h` |
| Which PCs, servers and VMs have had no successful backup in 36 hours, or never had one? | `synology-cli abb stale --older-than 36h` |
| Which volumes, pools or disks are degraded, failing, hot or nearly full? | `synology-cli fleet health --full-pct 85 --disk-temp 55` |
| Which task protects BOB-LAPTOP, and when did it last back up? | `synology-cli abb device "BOB-LAPTOP"` |
| Why did the Office PCs task fail? | `synology-cli abb triage "Office PCs" --nas acme` |
| Which tasks have the lowest success rate, or ran far longer than usual last time? | `synology-cli abb trend --since 30d` |
| Which Hyper Backup tasks does this NAS run, and to what kind of destination? | `synology-cli hyperbackup tasks` |
| Who is in the administrators group on this client's NAS? | `synology-cli group members --group administrators` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

[rafalr100/synology-mcp](https://github.com/rafalr100/synology-mcp) is a general-purpose DSM server with 71 tools for one NAS, covering files, storage, users, packages and Hyper Backup, with power controls off unless you enable them. [cmeans/mcp-synology](https://github.com/cmeans/mcp-synology) covers File Station, system health, Download Station and Synology Photos, with one config and login per NAS. Neither covers Active Backup for Business.

This one is built for the question an MSP asks every morning: did last night's backups work, at every client? `fleet sync` logs in to every client NAS you register, each with its own login (two-factor accounts included), and mirrors Active Backup tasks, job results, per-device runs and a Storage Manager snapshot into one local SQLite database. `fleet status`, `abb failures`, `abb stale` and `fleet health` then answer across every client in one local query, and `abb device` finds a machine by host name without knowing which client's NAS protects it. A NAS that did not sync, or whose mirror is older than `--max-sync-age`, reads as not synced, never as OK, and the report commands exit 6 when anything needs attention, so an RMM script can alert on them.

Synology's AI Console brings AI models into Synology's own apps (drafting and summarising in MailPlus and Office, AI search in Drive), and DSM 7.4's DSM Agent is a built-in advisor for one NAS (Synology has announced backup validation for a later DSM Agent 2.0); neither answers Active Backup questions across client NAS units. Active Insight, Synology's cloud monitoring service, offers MSP delegation, storage pool status and capacity forecasts, Hyper Backup monitoring and alerts on abnormal events per host; ActiveProtect Manager centrally manages NAS units running Active Backup for Business but needs an ActiveProtect appliance as its management server. This adds the per-device Active Backup answers (failed runs, devices with no recent success, which task protects a machine) across every client NAS, in the AI agent you already use.

## The pain this closes

Active Backup for Business is free with the NAS, so it backs up a lot of small-business PCs and servers. At scale, every NAS is its own island. An MSP with almost a hundred client NAS units reported (in French) that per-NAS notification emails had become unmanageable, that nothing alerted on a backup that had not run for days, and that neither Active Insight nor CMS monitored Active Backup ([nas-forum.com, Nov 2021](https://www.nas-forum.com/forum/topic/73029-monitoring-multiple-active-backup-for-business)); the replies were SNMP (DSM's MIBs do not expose backup state) and SSH scripts reading Active Backup's internal SQLite files. Tom Lawrence's 2023 review said the same from the MSP side: "mostly because of their lack of central management" ([Lawrence Systems forum](https://forums.lawrencesystems.com/t/synology-active-backup-for-business-review-2023/16543)).

- `synology-cli fleet status` - one row per client NAS, unknown (never OK) when a NAS did not sync, exit 6 for an RMM alert.
- `synology-cli abb failures --since 24h` - every failed, partial or missed task, with the first error line.
- `synology-cli abb stale --older-than 72h` - every machine with no good backup in three days, including ones that never had one.
- `synology-cli fleet health` - degraded pools, bad SMART disks and nearly full volumes, with days to full.
- `synology-cli abb triage "Office PCs" --nas acme` - why one task failed: the failed run's detail lines and the log from that window.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Synology MCP server runs locally, but it speaks HTTP natively: set `SYNOLOGY_MCP_HTTP_TOKEN` to a long random value, start it with `synology-mcp --transport http` (it serves `http://127.0.0.1:7777/mcp` by default and refuses to start without that token), and put that endpoint behind an HTTPS tunnel or your own reverse proxy. No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Synology credentials once.

### Is my Synology data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### How do I check 40 client NAS units, not just one?

Register each one with `synology-cli fleet add <name> --url https://<nas>:5001 --client "<Client>" --pin-cert --check`. Its login comes from `SYNOLOGY_<NAME>_ACCOUNT` and `SYNOLOGY_<NAME>_PASSWORD` (or the prefix you pass with `--env-prefix`), so the registry holds no password. `synology-cli fleet sync` then logs in to every NAS, four at a time by default, and `synology-cli fleet list` shows which credential env vars are set for each. One unreachable NAS is recorded and does not stop the others; `fleet sync` exits 5 when any NAS failed.

### Our NAS accounts use two-factor sign-in. Does that work?

Yes. Run the first login with a fresh code in `SYNOLOGY_OTP_CODE` (one NAS, `synology-cli auth login`) or `SYNOLOGY_<NAME>_OTP_CODE` (a fleet NAS, `fleet add --check` or `fleet sync --nas <name>`). DSM returns a trusted-device token, which the CLI keeps with that NAS's cached session (owner-only on macOS and Linux) and sends on later logins, so the code is not needed again.

### My NAS uses DSM's self-signed certificate. Will it connect?

Yes, without turning verification off. `fleet add --pin-cert` records the NAS certificate's SHA-256 fingerprint and trusts exactly that certificate afterwards; for a single NAS set `SYNOLOGY_TLS_FINGERPRINT`. `SYNOLOGY_SKIP_TLS_VERIFY=1` trusts any certificate and is only for a network you control. When a NAS renews its certificate, calls to it fail with "the NAS certificate does not match the pinned SHA-256 fingerprint": check the new fingerprint in DSM (Control Panel > Security > Certificate), then re-pin with that value: `synology-cli fleet add <name> --tls-fingerprint <sha256>` (it keeps the address and history) or update `SYNOLOGY_TLS_FINGERPRINT`. A pin mismatch is also what an interception looks like, so do not re-pin without that check.

### Can it change my clients' NAS units?

Three commands act on a NAS: `abb backup` starts a backup, `abb cancel` cancels a running one, and `session logout` ends the CLI's own DSM session. Each sends immediately unless you add `--dry-run`. Everything else reads. Set `SYNOLOGY_MCP_READ_ONLY=1` and the MCP server drops every command that is not read-only.

### Does it cover Hyper Backup and Synology C2?

Hyper Backup, the NAS's own backup to another NAS, USB or cloud, is covered live for the NAS in `SYNOLOGY_BASE_URL`: `synology-cli hyperbackup tasks` and `synology-cli hyperbackup task-status`. The fleet mirror and reports cover Active Backup for Business and storage health only.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet status`, `fleet health`, `fleet list`, `abb failures`, `abb stale`, `abb device`, `abb trend`, `abb triage`, `abb tasks`, `abb results`, `abb logs`, `hyperbackup`, `storage`, `system`, `log`, `package`, `folder`, `user`, `group`, `sync`, `search`, `export` (`sync` fills the local mirror; `export --output` overwrites the file you name) | Allow |
| Write (routine) | `abb backup`, `abb cancel`, `session logout` (act on a NAS); `fleet sync` (sends each registered NAS's login to its registered URL, logs out when done) | Preview with `--dry-run`, then a reviewed write |
| Credential | `auth login` (sends the password to `SYNOLOGY_BASE_URL`), `auth logout` | Human-in-the-loop only |
| Admin (operator only) | `fleet add`, `fleet remove`: they decide which URL each NAS password goes to, so they are CLI only and not offered over MCP | Operator only |
| Data egress | `--deliver webhook:<url>`, `feedback --send` | Human-in-the-loop |

No command deletes, reconfigures, restores or reboots anything on a NAS.

The strongest controls are the **DSM account you give each NAS** - the CLI can only do what that account may do - and `SYNOLOGY_MCP_READ_ONLY=1` for an agent that should only read. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta, awaiting live verification. The command surface is built from Synology's public DSM WebAPI documentation and community-documented Active Backup for Business calls (Synology publishes no Active Backup API specification), and the fleet reports are covered by tests against simulated DSM responses, but no MSP has yet run it against production NAS units and reported back - so nothing here is proven against a real NAS. If you run it against your own, the it-works form is how that becomes a badge. MSPs put connectors like this through their paces in our weekly Build Sessions - RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-20._
