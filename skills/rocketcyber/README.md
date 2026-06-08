# RocketCyber + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the RocketCyber
> API. Not affiliated with, endorsed by, or sponsored by Kaseya.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/rocketcyber/">
    <img src="../../docs/assets/video/rocketcyber/animated-og.gif" alt="RocketCyber demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/rocketcyber/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

The first CLI and MCP server for RocketCyber Managed SOC: triage, incident MTTR, device risk ranking, and posture analytics the console doesn't compute. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the RocketCyber skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `rocketcyber-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `rocketcyber-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `rocketcyber-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the RocketCyber MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `rocketcyber-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download RocketCyber MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/rocketcyber-v0.1.0/rocketcyber-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every RocketCyber release on the [releases page](https://github.com/servosity/msp-skills/releases?q=rocketcyber).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install rocketcyber@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the RocketCyber Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.ps1 | iex`. Then authenticate per the README and run `rocketcyber-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.sh)
```

The installer drops both `rocketcyber-cli` (the CLI) and `rocketcyber-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
rocketcyber-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update rocketcyber@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/rocketcyber --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/rocketcyber --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `rocketcyber-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the rocketcyber skill from https://github.com/servosity/msp-skills/tree/main/skills/rocketcyber. The skill defines how its required CLI (`rocketcyber-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your RocketCyber portal):

```bash
ROCKETCYBER_API_TOKEN=<value> rocketcyber-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What broke across all my clients overnight? | `rocketcyber-cli triage --since 24h` |
| Which devices went dark this week? | `rocketcyber-cli agents stale --since 7d` |
| How fast is my SOC actually resolving incidents? | `rocketcyber-cli incidents mttr --since 90d` |
| Which machines are riskiest in Defender right now? | `rocketcyber-cli defender riskiest --top 10` |
| Is this client's Microsoft 365 posture improving? | `rocketcyber-cli office trend --account-id 2` |
| Which suppression rules are stale and may hide detections? | `rocketcyber-cli suppression audit --stale-after 90d` |
| What detection events fired, by verdict? | `rocketcyber-cli events summary --account-id 2` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most RocketCyber integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "how many malicious detections fired across all 47 clients last quarter" or "what's our median incident MTTR this year."

This skill syncs RocketCyber into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `triage`, `incidents mttr`, and `defender riskiest` join across incidents, detection events, agents, and Defender telemetry - work a stateless API wrapper can't do.

## The pain this closes

Technicians on r/msp describe SOC work in one phrase: **alert fatigue**. RocketCyber gives you a managed SOC, but answering "what actually broke across my clients overnight?" means logging into the console, switching to each client account, and reading the incidents, events, and agents tabs one at a time. Stale suppression rules quietly pile up and can hide real detections, and at QBR time you're screenshotting secure-score charts and computing MTTR by hand.

The highest-leverage commands map straight to that pain:

| The pain | The command |
| --- | --- |
| "What broke overnight?" across every client | `rocketcyber-cli triage --since 24h` |
| Devices that went dark, grouped by client | `rocketcyber-cli agents stale --since 7d` |
| MTTR + open-incident aging for the QBR | `rocketcyber-cli incidents mttr --since 90d` |
| Riskiest machines, ranked | `rocketcyber-cli defender riskiest --top 10` |
| Stale suppression rules masking detections | `rocketcyber-cli suppression audit --stale-after 90d` |

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The RocketCyber MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your RocketCyber credentials once.

### Is my RocketCyber data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### What RocketCyber access do I need?

A RocketCyber provider account and an API token you generate in the RocketCyber app. The skill talks to the RocketCyber Customer API v3 (US region by default; set `ROCKETCYBER_BASE_URL` for the EU endpoint) and reads your own SOC data - incidents, agents, detection events, Defender, Microsoft 365 posture, and suppression rules - scoped to the accounts your token can see.

### Will this replace the RocketCyber console?

No. The console stays your live SOC dashboard. This skill gives your AI agent a terminal-native, multi-account read of the same data plus analytics the console won't compute for you: incident MTTR, device risk ranking, secure-score trend, and suppression-rule hygiene.

### Will it hit my API rate limits?

It syncs once into a local SQLite mirror, then answers aggregate questions offline from that mirror - so a QBR rollup across every client is one local query, not 47 live API calls. Use `sync --since` for incremental refreshes to keep API traffic low.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** | `triage`, `incidents`, `agents` (+ `stale`), `defender` (+ `riskiest`), `events list`/`summary`, `office trend`, `suppression rules`/`audit`, `reports`, `search`, `sync` | Allow |
| **Write (routine)** | `import <resource>` (create/upsert from a JSONL file) - the only command that writes to the API | Preview with `--dry-run`, then a reviewed write |
| **Credential / config** | `auth set-token`, `auth logout` | Human-in-the-loop only |

The strongest control is the **scope you grant the RocketCyber credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the RocketCyber API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-07._
