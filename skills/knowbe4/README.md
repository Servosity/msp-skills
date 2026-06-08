# KnowBe4 + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the KnowBe4
> API. Not affiliated with, endorsed by, or sponsored by KnowBe4, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/knowbe4/">
    <img src="../../docs/assets/video/knowbe4/animated-og.gif" alt="KnowBe4 demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/knowbe4/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every KnowBe4 KMSAT reporting feature plus a local SQLite store that answers the cross-client questions the console can't  -  repeat-clicker hunts and training-coverage anti-joins. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the KnowBe4 skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `knowbe4-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `knowbe4-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `knowbe4-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the KnowBe4 MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `knowbe4-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download KnowBe4 MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/knowbe4-v0.1.0/knowbe4-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every KnowBe4 release on the [releases page](https://github.com/servosity/msp-skills/releases?q=knowbe4).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install knowbe4@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the KnowBe4 Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.ps1 | iex`. Then authenticate per the README and run `knowbe4-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.sh)
```

The installer drops both `knowbe4-cli` (the CLI) and `knowbe4-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
knowbe4-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update knowbe4@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/knowbe4 --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/knowbe4 --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `knowbe4-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the knowbe4 skill from https://github.com/servosity/msp-skills/tree/main/skills/knowbe4. The skill defines how its required CLI (`knowbe4-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your KnowBe4 portal):

```bash
KNOWBE4_API_KEY=<value> KNOWBE4_REGION=<value> knowbe4-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Who clicked the bait in more than one phishing test? | `knowbe4-cli repeat-clickers --min-clicks 2 --since 90d` |
| Who clicked a phish but never passed training? | `knowbe4-cli untrained-clickers --since 180d` |
| Whose risk score is getting worse this quarter? | `knowbe4-cli risk-drift --window 90d --worsened --top 20` |
| Which active users have zero training or zero phishing coverage? | `knowbe4-cli coverage-gaps` |
| Is training actually working for the Finance group? | `knowbe4-cli phish-prone-trend --group "Finance" --since 12mo` |
| Who are my highest-risk users, with the why behind the score? | `knowbe4-cli risk-leaderboard --top 25` |
| Which departments are driving our risk up? | `knowbe4-cli group-risk-contribution --window 90d --top 10` |
| Assemble the full client quarterly review in one command | `knowbe4-cli qbr --since 90d` |
| Who never reports a simulated phish? | `knowbe4-cli report-rate --bottom 25` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most KnowBe4 integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "who are my repeat clickers across all 40 client tenants this quarter" - a question no single KnowBe4 endpoint answers.

This skill syncs KnowBe4 into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `repeat-clickers`, `untrained-clickers`, and `qbr` join across phishing tests, training enrollments, and risk-score snapshots - work a stateless API wrapper can't do, because KnowBe4 keeps those records in separate reports.

## The pain this closes

On r/msp the recurring KnowBe4 complaint is reporting, not the product: the KMSAT console answers one tenant, one phishing test, and one risk chart at a time, so any cross-client or cross-test question turns into a CSV export and a pivot table. The hardest gap is correlation - phishing results and training completion live in separate reports, so the most useful list a vCISO owns (clicked a phish, never finished training) doesn't exist in the portal. This skill closes that with `untrained-clickers` (the anti-join), `repeat-clickers` (the riskiest humans across every test), `risk-drift` (who is deteriorating), `coverage-gaps` (who your program silently misses), and `qbr` (the whole quarterly review in one command).

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The KnowBe4 MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your KnowBe4 credentials once.

### Is my KnowBe4 data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Does this replace the KnowBe4 console or need a special partner API?

Neither. It uses your standard KMSAT **Reporting API** key (Account Settings - API - enable Reporting API) and your region (`us`, `eu`, `ca`, `uk`, or `de`). It reads what your account already exposes and adds the cross-client rollups the console doesn't. It complements the portal; it doesn't replace it.

### How is this different from KnowBe4's own dashboards and Virtual Risk Officer?

The console and VRO give you per-tenant dashboards. This skill adds the cross-client, cross-test rollups and anti-joins the portal never exposes - repeat clickers across every phishing test, risk drift ranked across all users, and clicked-but-untrained remediation lists - all from your own synced data and driven by your AI agent.

### Can the AI change anything in my KnowBe4 account?

The bundled MCP server is **read-only** - it exposes reporting tools only. The write paths are CLI-only and preview with `--dry-run`: `events create` / `events delete` push or remove a custom risk event (these use a separate, opt-in User Event API key), and `import` bulk-upserts records through your standard Reporting API key. See [governance.md](./governance.md).

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `account info`, `users list`, `groups list`, `phishing-tests list`, `training-enrollments list`, `risk-leaderboard`, `repeat-clickers`, `sync`, `search`, `qbr` | Allow |
| Write (routine) | `events create` (push a custom risk event), `import` (bulk create/upsert from JSONL) | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | `events delete` (remove a custom risk event from a user's timeline) | Human-in-the-loop only |

The MCP server exposes the Read tier only. The Write and Destructive tiers are CLI-only; the `events` commands use a separate, opt-in `KNOWBE4_USER_EVENT_API_KEY`, while `import` writes through your standard `KNOWBE4_API_KEY`.

The strongest control is the **scope you grant the KnowBe4 credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the KnowBe4 API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
