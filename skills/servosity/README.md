# Servosity + AI - for Claude, ChatGPT, Codex, Cursor, and any agent that speaks MCP

> Published by Servosity Inc. for MSP partners. A first-party Claude Code Skill and MCP server for the Servosity backup and DR platform. Servosity is a trademark of Servosity Inc. Apache-2.0 licensed.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/servosity/">
    <img src="../../docs/assets/video/servosity/animated-og.gif" alt="Servosity demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/servosity/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Add **fleet-wide backup triage, stale-backup-set detection, overnight drift, per-client situational awareness, and cross-client analytics** to the AI you already use - **Claude Code**, **Claude Desktop**, **ChatGPT** (desktop app), **Codex**, **Cursor**, **Windsurf**, **Cline**, **Continue**, **Gemini**, or **GitHub Copilot**. Free, open source, runs on your laptop. A local fleet mirror means your AI can answer cross-client questions the partner portal can't show on one screen - Friday-email-ready in seconds. Built for MSP partners. No code required.

## Works with your agent

The five agents MSP owners actually use:

| Your AI agent | How to install the Servosity skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `servosity-mcp` (no JSON editing). |
| **ChatGPT** (desktop app) | Run installer, then add `servosity-mcp` in **Settings > MCP servers** as a local (STDIO) server. ChatGPT on the web is not supported yet. |
| **Claude Code** | Paste the install prompt below. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |

The ChatGPT desktop app runs `servosity-mcp` locally and shares MCP configuration with Codex ([OpenAI](https://learn.chatgpt.com/docs/extend/mcp)). ChatGPT on the web needs a remote server, which msp-skills does not support yet. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list. Setup steps: [mcp-install.md](./mcp-install.md).

> **Also works with** Cursor, Windsurf, Cline, Continue.dev, Zed, GitHub Copilot, and Gemini CLI - plus [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw), both via MCP and the pre-wired skill frontmatter. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

> **Claude Desktop bundle:** the `.mcpb` launches on macOS (Intel and Apple Silicon), Windows x64 and Linux x64. Every tool shells out to the companion `servosity-cli`. Releases cut from 2026-09-18 on ship that CLI inside the bundle; older bundles contain only the MCP server, so if yours lacks the companion, run the Path A or Path B installer below first (or set `SERVOSITY_MSP_CLI_PATH` to an existing binary). Details: [#331](https://github.com/Servosity/msp-skills/issues/331).

[**Download Servosity MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/servosity-v0.6.15/servosity-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Servosity release on the [releases page](https://github.com/servosity/msp-skills/releases?q=servosity).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install servosity@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Servosity Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.ps1 | iex`. Then authenticate with `SERVOSITY_MSP_TOKEN=<your-partner-token> servosity-cli doctor` and run `servosity-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.sh)
```

The installer drops both `servosity-cli` and `servosity-mcp` into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
servosity-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update servosity@msp-skills`.

### Add to Claude Desktop, Cursor, Windsurf, Cline, Continue, Gemini, or Copilot

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up. Claude Desktop's Settings > Extensions panel is the simplest path.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/servosity --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/servosity --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `servosity-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

[OpenClaw](https://docs.openclaw.ai) is GA. Install the skill directly from this repo:

```bash
openclaw skills install git:Servosity/msp-skills/skills/servosity@main
```

Or tell your OpenClaw agent (copy this):

> Install the servosity skill from https://github.com/Servosity/msp-skills/tree/main/skills/servosity. The `metadata.openclaw` frontmatter declares the required CLI (`servosity-cli`) and env vars; run `openclaw doctor` if anything is missing.

`openclaw` also speaks MCP natively (see `openclaw mcp set` in the [OpenClaw MCP docs](https://docs.openclaw.ai/cli/mcp)) so you can wire `servosity-mcp` as an MCP server alongside the skill.

### Authenticate

Get your partner API token from the Servosity partner portal, then:

```bash
SERVOSITY_MSP_TOKEN=<token> servosity-cli doctor
```

`doctor` confirms the token works and the local mirror is reachable before you run anything that touches client data. It exits 0 even when the credential is rejected, so scripts should add `--fail-on error`.

## What this skill does

Outcome-first, with the single command that answers each question:

| Question your MSP keeps asking | Command |
| --- | --- |
| Where is my attention needed today, ranked worst-first? | `servosity-cli attention --top 5` |
| What got worse since yesterday, and what recovered? | `servosity-cli drift` |
| Which clients have backups stale for 7+ days? | `servosity-cli stale-backups --days 7 --refresh` |
| Draft the follow-up email for every stale-backup client | `servosity-cli email-draft --stale --days 7` |
| Build the backup section of Acme's QBR as a PDF | `servosity-cli qbr "Acme Co" --out acme-q1.pdf` |
| Quarter-end: every client's QBR report in one pass | `servosity-cli qbr-all --quarter 2026-Q1 --out ./qbrs/` |
| Watch every client's restore queue during a DR event | `servosity-cli restore-queue watch` |
| Hear about new and changed issues without opening the portal | `servosity-cli issues watch` · `servosity-cli events list --since 0 --json` |
| Does my Servosity bill match what I invoice clients? | `servosity-cli bill --reconcile invoices.csv` |
| Which installed agents still aren't pulling backups? | `servosity-cli unprovisioned` |
| Which hosts are below 7-year retention or outside the recovery guarantee? | `servosity-cli retention-audit --min 7YEAR` |
| What one root cause is behind this month's backup failures? | `servosity-cli failure-clusters --top 10` |
| First-time hydration of the local fleet mirror | `servosity-cli sync --full` |

### Issue changes as events

Servosity has no issue webhooks yet, so `issues watch` polls your reseller's issue list (read-only, every 5 minutes by default) and records each change as an event your agent reads by cursor.

```bash
# Keep running; the first poll records a baseline and no events
servosity-cli issues watch
# Or one poll, for cron
servosity-cli issues watch --once --json
# Read what changed; pass the returned cursor as --since next time
servosity-cli events list --since 0 --json
```

- Events: `servosity.issue.created`, `servosity.issue.updated` (state, severity, priority, `updated_at` or `last_seen_at` changed), `servosity.issue.archived`, `servosity.issue.ignored`, `servosity.issue.reactivated`. `--type` narrows `events list` to one.
- Each event carries issue and company ids and triage fields only, never the description or comments. Run `issues read <id>` for the rest.
- The issue list holds active issues only, so an active issue that leaves it is re-read to learn whether it was archived or ignored, and an ignored issue is re-read at most hourly to catch it being archived or changed. An issue you can no longer read is dropped.
- Only GET requests reach Servosity. The first poll must read every page before it records the baseline. A failed poll waits twice as long each time, at most an hour; a refused token stops the watch.
- Over MCP, `events_list` reads the recorded events (read-only, no Servosity call). `issues watch` is CLI-only.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most AI integrations for backup and DR vendors proxy each question into a live API call against the partner portal. That's fine for one client. It fails when you're asking *"which of my 47 clients have a backup set that hasn't succeeded in 7 days?"* - the partner portal scatters that across per-client pages, per-type views (M365, DR Server, DR Desktop), and an alert queue full of noise.

This skill syncs Servosity into a **local fleet mirror** with snapshot history and FTS5 search. Cross-client questions become one local query: instant, offline, and the AI sees the answer, not a context window full of paginated JSON. Compound commands like `attention`, `drift`, `stale-backups`, and `backup-facts` join across companies, backups, and issues - and the snapshot history they persist powers `drift`, `fleet-health` deltas, and `storage-trend` forecasts no stateless API wrapper can offer.

## The pain this closes

Backup and DR is where "silent failure" hurts most. The pains MSP owners name:

- **Silent backup failures discovered too late.** A backup that quietly stopped succeeding is invisible until a client needs a restore - the worst possible moment to find out.
- **No fleet-wide view.** Each client's backup state lives in its own portal view; there is no single screen that says "across my whole book, here's what's stale, failing, or in-flight right now."
- **Alert-queue noise buries the real failure.** Dozens of repeat and known-safe issues pile up per client; the one that matters hides in the pile.
- **Per-client questions mean portal archaeology.** Answering "is this client OK?" means clicking through metadata, every backup type, contracts, and issues by hand.

This skill turns per-client portal views into fleet-wide, offline-fast intelligence: `attention` is one screen across every client; `stale-backups` finds the Friday-email list and `email-draft` writes the emails; `drift` opens Monday with situational awareness instead of a blank slate; `triage` batch-suppresses known-safe noise (ignore/archive/comment) so the queue shows only what's new; `qbr` and `qbr-all` turn quarter-end report-building into one command.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, in the **ChatGPT desktop app**. It runs MCP servers locally (STDIO), the way `servosity-mcp` runs, and shares MCP configuration with Codex ([OpenAI](https://learn.chatgpt.com/docs/extend/mcp)): add `servosity-mcp` under **Settings > MCP servers** as a STDIO server, or add it once to Codex's `~/.codex/config.toml`. **ChatGPT on the web** is not supported by msp-skills yet: it needs a remote server. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list. Setup steps: [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md). Tool-specific gotchas (Copilot uses `servers` not `mcpServers`, Cline's Windows `npx` issue) are documented there.

### Do I need to be a Servosity MSP partner?

Yes. The Servosity API surface this skill wraps is the one available to **authenticated MSP partners**. You'll need a partner API token from the Servosity partner portal. The skill itself is free; the token is part of your Servosity partner agreement. Not a Servosity partner yet? Reach out via [servosity.com](https://www.servosity.com).

### Will this replace my Servosity partner portal?

No. The portal is still where you do administrative work, configure backups, and onboard clients. This skill makes the portal's **data** answerable by AI - so morning triage, Friday stale-backup reports, drift checks, and ad-hoc cross-client questions don't require portal archaeology. The portal is unchanged; you're adding a new way to ask the data questions.

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Servosity partner token once.

### Is my Servosity / client data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The fleet mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. The partner token is read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills. The token is scoped to **your reseller account only** (no cross-reseller access).

### Can my agent hear about new Servosity issues?

Yes. Servosity does not send webhooks yet, so `servosity-cli issues watch` polls your issue list read-only (every 5 minutes by default) and records each new, changed, archived, ignored or reactivated issue as an event. Your agent reads them with `servosity-cli events list --since <cursor>` or the read-only MCP tool `events_list`, which costs no Servosity call. Events carry ids and triage fields, never issue text.

### Can I run this on Windows?

Yes. The PowerShell installer above is the Windows path. CLI and MCP binaries are native Windows builds.

### What does it cost?

Free. Apache-2.0 licensed. Servosity does not charge for the API access required to run this skill - the partner-portal API is part of your existing partner agreement. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), billed by your AI provider.

## Safety model

The skill authenticates with one partner token, scoped to **your reseller account only** (no cross-reseller access). Know how writes are gated - the binary does NOT gate them for you: `--dry-run` is an **opt-in** preview, and where a command prompts interactively, only an explicit `--yes` skips the prompt - `--agent` does not imply `--yes`. The gate lives in your agent's policy: preview with `--dry-run`, show the exact command, get approval, then run.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `attention`, `drift`, `stale-backups`, `backup-facts`, `qbr`, `fleet-health`, `unprovisioned`, `retention-audit`, `failure-clusters`, `bill`, `email-draft` (drafts only - nothing is sent), `restore-queue watch`, `issues watch`, `events list`, `search` | Allow |
| Write (routine) | `triage` (`--ignore`/`--archive`/`--reactivate`/`--comment`), `import`, raw create/update subcommands under the resource groups | Preview with `--dry-run`, then a reviewed write - never blanket `--yes` |
| Credential / security | `credentials rotate/delete`, `current-user api-token-list --reveal-token`, `current-user *-mfa-*`, `resellers agent-install-token`, `*-backups encryption-key update`, `auth set-token`/`logout` | Human-in-the-loop only; the API-token read is excluded from MCP and sync |
| Destructive | `companies delete` (deletes all backup accounts and data), `backups delete`, `users delete` | Human-in-the-loop only |

Keep autonomous agents to **Read plus previewed writes**; gate everything below that behind a human. Full matrix and lock-down guidance in [governance.md](./governance.md).

## Status

Live-verified. Confirmed by Servosity (maintainer) against their own production tenant. More MSPs run it live in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](https://docs.openclaw.ai), with `metadata.openclaw` frontmatter pre-wired so `openclaw skills install` works directly.

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-23._
