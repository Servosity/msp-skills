# Comet Backup + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Comet Backup
> API. Not affiliated with, endorsed by, or sponsored by Comet Licensing Ltd.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/comet-backup/">
    <img src="../../docs/assets/video/comet-backup/animated-og.gif" alt="Comet Backup demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/comet-backup/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

<!-- first-party-note:start -->
> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](../servosity) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.
<!-- first-party-note:end -->

<!-- backup-vendor-note:start -->
> **Comparing backup vendors?** One of our MSP partners told us, "My guys now spend a tenth of the time on backups." Fixing is included in every Servosity co-managed backup plan, and our engineers tell you what they did. [See how co-managed backup works](https://servosity.com/?utm_source=msp-skills&utm_medium=connector-note&utm_campaign=comet-backup&utm_content=readme)
<!-- backup-vendor-note:end -->

Fleet-wide backup health, failure triage and usage reports for every tenant on your Comet Server, plus the admin API calls an MSP uses day to day. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Comet Backup skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `comet-backup-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `comet-backup-mcp` over HTTPS with `COMET_BACKUP_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `comet-backup-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `comet-backup-mcp --transport http` with `COMET_BACKUP_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `comet-backup-mcp` over HTTPS with `COMET_BACKUP_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Comet Backup MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/comet-backup-v0.1.4/comet-backup-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Comet Backup release on the [releases page](https://github.com/servosity/msp-skills/releases?q=comet-backup).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install comet-backup@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Comet Backup Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/comet-backup/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/comet-backup/install.ps1 | iex`. Then authenticate per the README and run `comet-backup-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/comet-backup/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/comet-backup/install.sh)
```

The installer drops both `comet-backup-cli` (the CLI) and `comet-backup-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
comet-backup-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/comet-backup/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/comet-backup/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update comet-backup@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/comet-backup --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/comet-backup --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `comet-backup-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the comet-backup skill from https://github.com/servosity/msp-skills/tree/main/skills/comet-backup. The skill defines how its required CLI (`comet-backup-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Point the CLI at your Comet Server and a Comet admin account (a dedicated API admin with a long random password is best). For an admin that requires 2FA, set `COMET_BACKUP_SESSION_KEY` from Comet's session-start API instead of the password.

```bash
export COMET_BACKUP_SERVER=backup.example.com:8060
export COMET_BACKUP_USERNAME=<admin-username>
export COMET_BACKUP_PASSWORD=<admin-password>
comet-backup-cli doctor
comet-backup-cli server version --json
```

**Windows (PowerShell):**

```powershell
$env:COMET_BACKUP_SERVER = "backup.example.com:8060"
$env:COMET_BACKUP_USERNAME = "<admin-username>"
$env:COMET_BACKUP_PASSWORD = "<admin-password>"
comet-backup-cli doctor
comet-backup-cli server version --json
```

`doctor` checks config and paths and makes one authenticated call to your server, so its `credentials` row reads healthy only when your login worked. The CLI connects over https and never sends the admin login over plain http: a Comet Server that only listens on plain http needs a TLS listener first. For a private certificate authority set `COMET_BACKUP_CA_CERT` to its PEM file.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which backups failed, are overdue or never ran, across every tenant? | `comet-backup-cli health --state failed,overdue,never --agent` |
| Why did last night's backups fail? | `comet-backup-cli triage --window 24h --group-by error --agent` |
| Which tenants are near a storage, device or Microsoft 365 quota? | `comet-backup-cli usage --over 80 --agent` |
| Which offline devices stopped backing up but still cost money? | `comet-backup-cli devices stale --older-than 7d --billable-only --agent` |
| Which devices run an older Comet client than the server? | `comet-backup-cli devices outdated --agent` |
| What opened or resolved since yesterday? | `comet-backup-cli snapshot` then `comet-backup-cli changes --only health --agent` |
| Re-run the failed backups on devices that are online now | `comet-backup-cli rerun-failed --window 24h` (plan), then add `--yes` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Comet Backup integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which protected items across all 60 tenants failed or have not run since Friday, and why".

This skill reads the whole server in a few bulk Comet calls and joins the answer locally. Compound commands like `health`, `triage` and `usage` join across tenants, devices, protected items, schedules, jobs and job logs in one pass - work a stateless API wrapper can't do - and the AI sees the answer, not the raw data. `snapshot` keeps each run's fleet state in a local SQLite store, so `changes` can say what opened or resolved since the last one.

## The pain this closes

Comet keeps every customer on one server, but its console and emailed job reports work one tenant and one job at a time. The backup that hurts most is the one that never reports a failure because it never ran: Magnus Box, a Comet-based backup provider, describes an MSP that lost a major client to an Exchange backup that had been failing silently for two months ([source](https://www.magnusbox.com/post/are-you-making-these-common-msp-backup-mistakes-86-of-msps-miss-these-critical-steps)).

- `health --state failed,overdue,never` lists every protected item that failed, is overdue against its own schedule, or never ran.
- `triage --group-by error` adds the first error lines from each job log, bucketed by cause.
- `devices stale --billable-only` finds offline devices that stopped backing up but still own protected items.
- `usage --over 80` flags tenants near a quota before the backup stops.
- `snapshot` plus `changes` turns the server's current state into a history you can open and close tickets from.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Comet Backup MCP server runs locally over stdio by default. Its HTTP mode (`comet-backup-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `COMET_BACKUP_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Comet Backup credentials once.

### Is my Comet Backup data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The local SQLite store (snapshots and saved reads) sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### What Comet account does it need?

A Comet Server admin account. Set `COMET_BACKUP_SERVER` (host and port) or `COMET_BACKUP_BASE_URL` (full URL), then `COMET_BACKUP_USERNAME` and `COMET_BACKUP_PASSWORD`. Comet recommends a dedicated API admin with a long random password and no 2FA; for an admin that requires 2FA, set `COMET_BACKUP_SESSION_KEY` from Comet's session-start API instead. Personal API tokens are not supported yet. On Comet-Hosted servers, create an Additional User with a local password.

### Does it work with self-hosted and Comet-Hosted servers?

Yes. The skill talks only to the one server you name and refuses a redirect to any other host.

### Can it run backups or restores?

It can ask an online device to run a backup or a restore through Comet's live-connection API. Those are writes on a customer endpoint: preview them with `--dry-run` and keep them human-approved. The MCP server sends a write only when the call carries `confirm: true`, and `rerun-failed` only prints its plan unless you add `--yes`.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `health`, `triage`, `usage`, `devices stale`, `devices outdated`, `backup-jobs recent`, `server version` | Allow |
| Credential (reads that return secrets) | `users get`, `users list-full`, `user-groups get`, `organizations`, `storage buckets`, `storage bucket`, `constellation last-report`, `policies get`, `policies list-full`, `protected-items` | Human-in-the-loop only |
| Write (routine) | `rerun-failed --yes`, `devices run-backup`, `backup-jobs cancel`, `users add`, `users set-profile`, `storage request-vault`, `import` | Preview with `--dry-run`, then a reviewed write |
| Customer endpoint | `devices run-restore`, `devices update-software`, `devices uninstall`, `devices unlock`, `devices kill` | Human-in-the-loop only |
| Destructive / security | `users delete`, `users reset-password`, `users disable-totp`, `users revoke-device`, `backup-jobs abandon` | Human-in-the-loop only, explicit confirmation |

The strongest control is the **scope you grant the Comet Backup credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Comet Backup API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-28._
