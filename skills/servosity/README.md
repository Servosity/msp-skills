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

Add **fleet-wide backup triage, stale-backup-set detection, overnight drift, per-client situational awareness, and cross-client analytics** to the AI you already use - **Claude Code**, **Claude Desktop**, **ChatGPT** (Plus/Pro+), **Codex**, **Cursor**, **Windsurf**, **Cline**, **Continue**, **Gemini**, or **GitHub Copilot**. Free, open source, runs on your laptop. A local fleet mirror means your AI can answer cross-client questions the partner portal can't show on one screen - Friday-email-ready in seconds. Built for MSP partners. No code required.

## Works with your agent

The five agents MSP owners actually use:

| Your AI agent | How to install the Servosity skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `servosity-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `servosity-mcp` over HTTPS, register as a Developer Mode connector. |
| **Claude Code** | Paste the install prompt below. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |

For ChatGPT, the Servosity MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

> **Also works with** Cursor, Windsurf, Cline, Continue.dev, Zed, GitHub Copilot, and Gemini CLI - plus [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw), both via MCP and the pre-wired skill frontmatter. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Servosity MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/servosity-v0.2.0/servosity-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Servosity release on the [releases page](https://github.com/servosity/msp-skills/releases?q=servosity).)

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

`doctor` confirms the token works and the local mirror is reachable before you run anything that touches client data.

## What this skill does

Outcome-first, with the single command that answers each question:

| Question your MSP keeps asking | Command |
| --- | --- |
| Where is my attention needed today, ranked worst-first? | `servosity-cli attention --top 5` |
| What got worse since yesterday, and what recovered? | `servosity-cli drift` |
| Which clients have backups stale for 7+ days? | `servosity-cli stale-backups --days 7` |
| Draft the follow-up email for every stale-backup client | `servosity-cli email-draft --stale --days 7` |
| Build the backup section of Acme's QBR as a PDF | `servosity-cli qbr "Acme Co" --out acme-q1.pdf` |
| Quarter-end: every client's QBR report in one pass | `servosity-cli qbr-all --quarter 2026-Q1 --out ./qbrs/` |
| Watch every client's restore queue during a DR event | `servosity-cli restore-queue watch` |
| Does my Servosity bill match what I invoice clients? | `servosity-cli bill --reconcile invoices.csv` |
| Which installed agents still aren't pulling backups? | `servosity-cli unprovisioned` |
| First-time hydration of the local fleet mirror | `servosity-cli sync --full` |

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

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Servosity MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md). Tool-specific gotchas (Copilot uses `servers` not `mcpServers`, Cline's Windows `npx` issue, ChatGPT's tier + bridge) are documented there.

### Do I need to be a Servosity MSP partner?

Yes. The Servosity API surface this skill wraps is the one available to **authenticated MSP partners**. You'll need a partner API token from the Servosity partner portal. The skill itself is free; the token is part of your Servosity partner agreement. Not a Servosity partner yet? Reach out via [servosity.com](https://www.servosity.com).

### Will this replace my Servosity partner portal?

No. The portal is still where you do administrative work, configure backups, and onboard clients. This skill makes the portal's **data** answerable by AI - so morning triage, Friday stale-backup reports, drift checks, and ad-hoc cross-client questions don't require portal archaeology. The portal is unchanged; you're adding a new way to ask the data questions.

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Servosity partner token once.

### Is my Servosity / client data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The fleet mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. The partner token is read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills. The token is scoped to **your reseller account only** (no cross-reseller access).

### Can I run this on Windows?

Yes. The PowerShell installer above is the Windows path. CLI and MCP binaries are native Windows builds.

### What does it cost?

Free. Apache-2.0 licensed. Servosity does not charge for the API access required to run this skill - the partner-portal API is part of your existing partner agreement. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), billed by your AI provider.

## Safety model

The skill authenticates with one partner token, scoped to **your reseller account only** (no cross-reseller access). Know how writes are gated - the binary does NOT gate them for you: `--dry-run` is an **opt-in** preview, and where a command prompts interactively, `--yes` (and `--agent`, which implies `--yes`) skips the prompt. The gate lives in your agent's policy: preview with `--dry-run`, show the exact command, get approval, then run.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `attention`, `drift`, `stale-backups`, `backup-facts`, `qbr`, `fleet-health`, `unprovisioned`, `bill`, `email-draft` (drafts only - nothing is sent), `restore-queue watch`, `search` | Allow |
| Write (routine) | `triage` (`--ignore`/`--archive`/`--reactivate`/`--comment`), `import`, raw create/update subcommands under the resource groups | Preview with `--dry-run`, then a reviewed write - never blanket `--yes` |
| Credential / security | `credentials rotate/delete`, `current-user *-mfa-*`, `resellers agent-install-token`, `*-backups encryption-key update`, `auth set-token`/`logout` | Human-in-the-loop only |
| Destructive | `companies delete` (deletes all backup accounts and data), `backups delete`, `users delete` | Human-in-the-loop only |

Keep autonomous agents to **Read plus previewed writes**; gate everything below that behind a human. Full matrix and lock-down guidance in [governance.md](./governance.md).

## Status

Already used inside Servosity's own backup / DR operations; published here for MSP partners. The public surface is in beta and being validated with MSPs in live Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](https://docs.openclaw.ai), with `metadata.openclaw` frontmatter pre-wired so `openclaw skills install` works directly.

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-05-28._
