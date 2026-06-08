# ThreatLocker + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the ThreatLocker
> API. Not affiliated with, endorsed by, or sponsored by ThreatLocker, Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/threatlocker/">
    <img src="../../docs/assets/video/threatlocker/animated-og.gif" alt="ThreatLocker demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/threatlocker/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every ThreatLocker Portal API feature, plus the write operations the read-only tools lack and a cross-tenant offline store no other ThreatLocker tool has. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the ThreatLocker skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `threatlocker-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `threatlocker-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `threatlocker-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the ThreatLocker MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `threatlocker-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download ThreatLocker MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/threatlocker-v4.22.0/threatlocker-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every ThreatLocker release on the [releases page](https://github.com/servosity/msp-skills/releases?q=threatlocker).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install threatlocker@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the ThreatLocker Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.ps1 | iex`. Then authenticate per the README and run `threatlocker-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.sh)
```

The installer drops both `threatlocker-cli` (the CLI) and `threatlocker-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
threatlocker-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update threatlocker@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/threatlocker --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/threatlocker --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `threatlocker-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the threatlocker skill from https://github.com/servosity/msp-skills/tree/main/skills/threatlocker. The skill defines how its required CLI (`threatlocker-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your ThreatLocker portal):

```bash
THREATLOCKER_API_KEY=<value> threatlocker-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What application approvals are pending across all my clients right now? | `threatlocker-cli approvals triage --all-tenants` |
| Approve this file hash everywhere it's pending, plan first? | `threatlocker-cli approvals approve-batch --hash <sha256> --all-tenants --dry-run` |
| Which clients are about to lose audit evidence to the 31-day retention cliff? | `threatlocker-cli audit retention-check` |
| Export every client's audit log before it ages off? | `threatlocker-cli audit export --all-tenants --since 30d` |
| What changed across all tenants this week, protection off, policy edits, maintenance? | `threatlocker-cli audit drift --since 7d --all-tenants` |
| Which ThreatLocker agents are offline or stale across every client? | `threatlocker-cli devices health --all-tenants` |
| Where does this binary live across my whole book, approved or pending? | `threatlocker-cli applications hunt --hash <sha256>` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most ThreatLocker integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, because the Portal API is scoped one tenant at a time, so asking "show me every pending approval across all 40 clients, deduped by file hash" or "which clients are about to lose audit evidence this month" turns into a header-swap-and-re-query dance the AI burns context on, tenant by tenant.

This skill syncs ThreatLocker into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `approvals triage`, `applications hunt`, and `devices health` join approval requests, applications, computers, and audit events across every tenant at once - work a stateless API wrapper can't do.

## The pain this closes

ThreatLocker is default-deny, so every new or updated application becomes an approval request an admin has to clear. ThreatLocker's own MSP guide admits the volume: _"While you are onboarding your first few clients, your devices are learning and can be quite noisy"_ ([Allowlisting for MSPs](https://www.threatlocker.com/blog/allowlisting-for-msps)). Across a book of tenants the queue never goes quiet, and the Portal makes you clear it one tenant at a time. Then there's evidence: ThreatLocker's Help Center is explicit that _"by default, the Unified Audit retains data for 31 days. After 31 days, the information is permanently deleted and cannot be recovered"_ ([Log Retention](https://threatlocker.kb.help/log-retention/)) - shorter than most compliance and cyber-insurance asks.

This skill closes both:

- **`approvals triage --all-tenants`** - one ranked queue of every pending approval across all tenants, deduped by file hash.
- **`approvals approve-batch --hash <sha256> --all-tenants --dry-run`** - permit a known-good file everywhere it's pending, plan first.
- **`audit retention-check`** + **`audit export --all-tenants --since 30d`** - flag tenants near the 31-day cliff and persist their logs locally before evidence ages off.
- **`devices health --all-tenants`** - surface dark and stale agents across the whole book without a tenant-by-tenant sweep.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The ThreatLocker MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your ThreatLocker credentials once.

### Is my ThreatLocker data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my ThreatLocker API rate limits?

The local mirror exists so reads stop hitting the API. After the first `sync`, the cross-tenant views (`approvals triage`, `audit drift`, `devices health`, `applications hunt`) run against local SQLite with **zero API calls**. Live calls respect a `--rate-limit` throttle, and sync is incremental, fetching only what changed since the last checkpoint.

### How does it handle ThreatLocker's 31-day audit retention?

ThreatLocker's Unified Audit log keeps about 31 days by default. `audit export` persists each tenant's log to JSONL or CSV locally so the evidence outlives that window, and `audit retention-check` reports, per tenant, how close your archive is to the cliff and how stale your last sync is, so nothing ages off unnoticed.

### Do I need to be a ThreatLocker MSP or have child tenants?

You need API access in your own ThreatLocker Portal. The cross-tenant features assume a managed (parent) organization with child tenants, the MSP setup, but a single organization works too, you just get the one-tenant view. The credential you mint is the real permission boundary.

### Does it replace the ThreatLocker Portal?

No. The Portal stays best for authoring policies and the interactive approve/deny workflow. This skill adds cross-tenant queries and scriptable writes to your AI agent so you stop logging into each tenant to answer book-wide questions.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `approvals triage`, `audit drift`, `audit retention-check`, `audit export`, `devices health`, `applications hunt`, `search` | Allow |
| Write (routine) | `approvals approve` / `approve-batch`, `applications create` / `update`, `policies create` / `copy` / `deploy`, `computers maintenance` / `enable-protection` / `restart-service` | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | `computers delete`, `policies delete` | Human-in-the-loop only |

The strongest control is the **scope you grant the ThreatLocker credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the ThreatLocker API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-05._
