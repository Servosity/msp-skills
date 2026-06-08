# Hudu + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Hudu
> API. Not affiliated with, endorsed by, or sponsored by Hudu Technologies, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/hudu/">
    <img src="../../docs/assets/video/hudu/animated-og.gif" alt="Hudu demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/hudu/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Hudu cmdlet, plus an offline SQLite mirror, cross-entity audits, and agent-native output no PowerShell module or read-only MCP ships. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Hudu skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `hudu-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `hudu-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `hudu-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Hudu MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `hudu-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Hudu MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/hudu-v0.1.0/hudu-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Hudu release on the [releases page](https://github.com/servosity/msp-skills/releases?q=hudu).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install hudu@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Hudu Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.ps1 | iex`. Then authenticate per the README and run `hudu-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.sh)
```

The installer drops both `hudu-cli` (the CLI) and `hudu-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
hudu-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update hudu@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/hudu --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/hudu --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `hudu-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the hudu skill from https://github.com/servosity/msp-skills/tree/main/skills/hudu. The skill defines how its required CLI (`hudu-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Hudu portal). Create the key under
**Admin -> API Keys**, and point `HUDU_BASE_URL` at your instance's `/api/v1` URL:

```bash
HUDU_BASE_URL=https://<your-subdomain>.huducloud.com/api/v1 HUDU_API_KEY=<value> hudu-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

Run `hudu-cli sync` once to mirror your instance locally, then ask:

| Question your MSP keeps asking | Command |
| --- | --- |
| Which clients have the worst documentation completeness? | `hudu-cli audit completeness --agent` |
| What's expiring in the next 30 days across every client? | `hudu-cli audit expirations --within 30d --agent` |
| Which vault passwords are overdue for rotation? | `hudu-cli audit stale-passwords --older-than 180d --agent` |
| Which knowledge-base articles are stale and probably out of date? | `hudu-cli audit stale-articles --older-than 365d --agent` |
| Which assets have drifted from their layout's current schema? | `hudu-cli audit layout-drift --agent` |
| Give me one worst-first hygiene scorecard across every company. | `hudu-cli audit summary --agent` |
| Find everything matching a keyword across all synced docs. | `hudu-cli search "vpn gateway" --agent` |
| Which PSA/RMM records don't map to a live Hudu asset? | `hudu-cli reconcile --agent` |
| Scaffold a new client's docs from our house template. | `hudu-cli onboard --company 42 --template msp-standard` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Hudu integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 50 clients have stale passwords, incomplete asset docs, and certs expiring this month" right before a round of QBRs.

This skill syncs Hudu into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `audit summary` and `audit completeness --cross-tenant` join assets across their layouts, companies, and password vaults to rank every tenant worst-first - work a stateless API wrapper can't do.

## The pain this closes

"Our documentation is always out of date" is one of the most repeated complaints on r/msp - assets get created with half their required fields blank, passwords go years without rotation, and certs lapse because nobody was watching the renewal date across every tenant at once. Hudu stores all of it, but answering "where is the rot?" means clicking company by company, and Hudu has no native password-expiration tracking at all.

This skill turns those recurring questions into one command each:

- `hudu-cli audit completeness --agent` - rank clients by how much required documentation is actually filled in.
- `hudu-cli audit stale-passwords --older-than 180d --agent` - surface vault credentials nobody has rotated, grouped by company (computed locally; secret values are never read).
- `hudu-cli audit expirations --within 30d --agent` - one typed list of SSL, domain, warranty, and password expirations due soon, across every tenant.
- `hudu-cli audit summary --agent` - a single worst-first hygiene scorecard so you know which client to fix first.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Hudu MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Hudu credentials once.

### Is my Hudu data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will a company-scoped Hudu API key work?

Mostly. Audits and search run over whatever you've synced, so they work with any key. The one limit: Hudu's global asset-list endpoint requires a global (not company-scoped) key - with a scoped key, use `hudu-cli assets list-by-company <company_id>` instead. The `onboard --apply` write path also needs a global key.

### Will this replace the Hudu portal?

No. It complements the web app you already use for editing and day-to-day documentation. The skill answers cross-tenant hygiene questions and hands the result to your AI agent; you still use Hudu's UI for authoring and browsing.

### Will this hit my Hudu API rate limits?

It's designed not to. `hudu-cli sync` pulls your instance into a local SQLite mirror once (with resumable incremental sync), and every audit, search, and resolve then runs offline against that mirror - so repeated questions cost zero API calls.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `audit completeness`, `audit stale-passwords`, `audit expirations`, `audit summary`, `search`, `resolve`, `reconcile`, `companies list`, `assets list-by-company`, `sync`, `doctor` | Allow |
| Write (routine) | `assets create`, `assets update`, `articles create`, `articles update`, `companies create`, `companies update`, `asset-layouts create`, `onboard --apply` | Preview with `--dry-run`, then a reviewed write |
| Destructive / credential | `assets delete`, `articles delete`, `companies delete`, `asset-passwords delete`, `asset-passwords get`, `asset-passwords list`, `auth set-token`, `auth logout` | Human-in-the-loop only |

The strongest control is the **scope you grant the Hudu credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Hudu API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
