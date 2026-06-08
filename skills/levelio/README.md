# Level + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Level
> API. Not affiliated with, endorsed by, or sponsored by Level.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/levelio/">
    <img src="../../docs/assets/video/levelio/animated-og.gif" alt="Level demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/levelio/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Level RMM endpoint, plus a local SQLite fleet store and offline cross-entity rollups no Level tool has: at-risk ranking, patch posture, alert triage, and stale-device detection in one command. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Level skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `levelio-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `levelio-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `levelio-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Level MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `levelio-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Level MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/levelio-v0.1.0/levelio-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Level release on the [releases page](https://github.com/servosity/msp-skills/releases?q=levelio).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install levelio@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Level Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/levelio/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/levelio/install.ps1 | iex`. Then authenticate per the README and run `levelio-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/levelio/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/levelio/install.sh)
```

The installer drops both `levelio-cli` (the CLI) and `levelio-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
levelio-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/levelio/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/levelio/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update levelio@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/levelio --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/levelio --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `levelio-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the levelio skill from https://github.com/servosity/msp-skills/tree/main/skills/levelio. The skill defines how its required CLI (`levelio-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Level portal):

```bash
LEVEL_API_TOKEN=<value> levelio-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which devices are most at risk across alerts, patches, score, and staleness? | `levelio-cli at-risk --top 20` |
| Which devices have gone dark and stopped checking in? | `levelio-cli stale --days 30` |
| What is my fleet-wide patch exposure, by category? | `levelio-cli patch-posture --category security` |
| How is my fleet broken down by OS, platform, or group? | `levelio-cli fleet --by os` |
| Where are my active critical fires, clustered by group? | `levelio-cli alert-triage --severity critical` |
| Give me a per-client posture scorecard for QBRs. | `levelio-cli client-scorecard` |
| Which devices are below my security-score threshold? | `levelio-cli security-posture --below 70` |
| Which devices are waiting on a reboot to finish patching? | `levelio-cli reboot-due` |
| Which monitors fire most often across the fleet? | `levelio-cli alert-recurrence --top 15` |
| What changed since yesterday - new alerts, updates, activity? | `levelio-cli since --days 1` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Level integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 600 endpoints across every client need attention first" or "what's each client's patch exposure for the QBR deck."

This skill syncs Level into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `at-risk`, `client-scorecard`, and `cf-coverage` join across devices, alerts, OS updates, groups, and custom fields - work a stateless API wrapper can't do.

## The pain this closes

Level is a fast, modern RMM, but like every RMM its console answers questions one device at a time. The recurring complaint on [r/msp](https://www.reddit.com/r/msp/) isn't monitoring - it's *reporting across the whole fleet at once*: which machines quietly went dark, which clients are furthest behind on patches, where the alert noise is systemic, and what each client's posture is when a QBR is due. There's no single screen for that, so the numbers that matter get assembled by hand, group tab by group tab.

This skill closes that gap with cross-entity rollups:

- **`levelio-cli at-risk --top 20`** - the worst endpoints across alerts, patches, score, and staleness as one ranked "fix these first" list.
- **`levelio-cli stale --days 30`** - the machines that quietly stopped checking in, before the client calls.
- **`levelio-cli patch-posture --category security`** - fleet-wide update exposure (available vs installed vs errored) at a glance.
- **`levelio-cli client-scorecard`** - one QBR-ready row per client: devices, online %, open criticals, average score, stale count, patch exposure.
- **`levelio-cli alert-triage --severity critical`** - unresolved alerts clustered by group and severity, so systemic fires surface above one-off noise.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Level MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Level credentials once.

### Is my Level data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my Level API rate limits?

Only `sync` calls the Level API, and it paginates politely. Every report, rollup, and search after that runs against the local SQLite mirror - zero API calls, no rate-limit pressure. Re-sync when you want fresh data.

### Do I need a paid Level plan or to be a Level partner?

You need a Level account and an API key (**Settings > API keys**). A **read-only** key is enough for every report and rollup here, and you can scope it tighter than your portal login. The skill itself is free and open source.

### Will this replace my Level portal?

No - it complements it. Use the portal for live remote control, scripting, and day-to-day device work; use this skill for the cross-client, fleet-wide answers the portal shows one device at a time.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `at-risk`, `patch-posture`, `fleet`, `alert-triage`, `stale`, `client-scorecard`, every `list`/`show`, `search`, `sync` | Allow |
| Write (routine) | `devices update`, `groups`/`tags`/`custom-fields create` and `update`, `custom-field-values update`, `alerts resolve`, `import` | Preview with `--dry-run`, then a reviewed write |
| Destructive / device actions | `devices`/`groups`/`tags`/`custom-fields delete`, `automations` (runs scripts on endpoints), `auth set-token` | Human-in-the-loop only |

The strongest control is the **scope you grant the Level credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Level API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
