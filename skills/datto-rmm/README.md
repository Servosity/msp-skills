# Datto RMM + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Datto RMM
> API. Not affiliated with, endorsed by, or sponsored by Datto, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/datto-rmm/">
    <img src="../../docs/assets/video/datto-rmm/animated-og.gif" alt="Datto RMM demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/datto-rmm/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Datto RMM API operation, plus a local SQLite fleet store and fleet-wide analytics no other Datto tool has. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Datto RMM skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `datto-rmm-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `datto-rmm-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `datto-rmm-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Datto RMM MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `datto-rmm-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Datto RMM MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/datto-rmm-v0.1.0/datto-rmm-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Datto RMM release on the [releases page](https://github.com/servosity/msp-skills/releases?q=datto-rmm).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install datto-rmm@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Datto RMM Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-rmm/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-rmm/install.ps1 | iex`. Then authenticate per the README and run `datto-rmm-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-rmm/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-rmm/install.sh)
```

The installer drops both `datto-rmm-cli` (the CLI) and `datto-rmm-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
datto-rmm-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-rmm/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-rmm/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update datto-rmm@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/datto-rmm --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/datto-rmm --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `datto-rmm-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the datto-rmm skill from https://github.com/servosity/msp-skills/tree/main/skills/datto-rmm. The skill defines how its required CLI (`datto-rmm-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Datto RMM portal):

```bash
DATTO_RMM_API_KEY=<value> DATTO_RMM_API_SECRET_KEY=<value> datto-rmm-cli doctor
```

The API key and secret key come from **Setup > Users** in your Datto RMM portal (the OAuth API user). A pre-minted bearer token is optional - set `DATTO_RMM_TOKEN` only if you want to skip the auto-mint. `doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which devices haven't checked in for 30 days, across every client? | `datto-rmm-cli fleet stale --days 30 --agent` |
| Where is antivirus missing, disabled, or not running? | `datto-rmm-cli fleet av-gaps --status not-running --agent` |
| Which endpoints are most behind on patches right now? | `datto-rmm-cli fleet patch-gaps --min-missing 5 --agent` |
| Which devices have warranties expiring in the next 60 days? | `datto-rmm-cli fleet warranty --within 60 --agent` |
| Which devices and sites generate the most alert noise this week? | `datto-rmm-cli fleet storms --days 7 --top 20 --agent` |
| Give me a one-page health scorecard for a client before the QBR. | `datto-rmm-cli fleet scorecard "Acme Corporation" --agent` |
| How many copies of an app are installed fleet-wide, and which versions? | `datto-rmm-cli fleet sprawl --name "Google Chrome" --agent` |
| Which devices are running an out-of-date RMM agent? | `datto-rmm-cli fleet agent-drift --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Datto RMM integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking how many endpoints across all 40 clients are missing antivirus or about to drop out of warranty this quarter.

This skill syncs Datto RMM into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `fleet scorecard`, `fleet storms`, and `fleet sprawl` join across sites, devices, alerts, audit, and warranty data - work a stateless API wrapper can't do.

## The pain this closes

Datto RMM is built around a per-customer view. That's fine until you have to answer a question that spans the whole fleet - and those are the questions that matter to an MSP owner: which endpoints stopped reporting, where antivirus is off, who's behind on patches, whose hardware warranty is about to lapse. On r/msp the recurring Datto RMM complaints are exactly this shape - alert noise that buries real failures, and reporting that makes you click through site by site because there's no clean cross-client rollup. So dead agents, unprotected endpoints, and expiring warranties get discovered late, often when the customer calls.

This skill turns those into a 10-second sweep:

- **`datto-rmm-cli fleet stale --days 30`** - every device fleet-wide that stopped checking in.
- **`datto-rmm-cli fleet av-gaps --status not-running`** - every endpoint with antivirus off.
- **`datto-rmm-cli fleet patch-gaps --min-missing 5`** - the most patch-exposed endpoints first.
- **`datto-rmm-cli fleet warranty --within 60`** - hardware about to fall out of warranty, ready for a QBR.
- **`datto-rmm-cli fleet scorecard "Acme Corporation"`** - a one-page client health card for the QBR.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Datto RMM MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Datto RMM credentials once.

### Is my Datto RMM data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my Datto RMM API rate limits?

It's gentle by design. The skill syncs your fleet into a local mirror once, then answers fleet-wide questions from that mirror instead of calling the API per question. You re-sync on your own cadence; everyday analytics run offline against the local store.

### Do I need to be a Datto partner or have special permissions?

You need an API key and secret key from **Setup > Users** in your Datto RMM portal (the OAuth API user). The skill can only do what that API user is allowed to do, so scope the user to the access your workflow actually needs.

### Will this replace my Datto RMM portal?

No - it complements it. You still use the console for remote control, policy, and component authoring. This skill answers the cross-client read questions and scripts the bulk reads/writes the web UI makes tedious.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** | `fleet stale`, `fleet av-gaps`, `fleet scorecard`, `account get-sites`, `device get-by-uid`, `search` | Allow |
| **Write (routine)** | `account create-variable`, `account update-variable`, `device warranty set-data`, `device quickjob create-quick-job`, `site create`, `site update` | Preview with `--dry-run`, then a reviewed write |
| **Destructive / credential** | `account delete-variable`, `site variable delete-site`, `site settings delete-proxy`, `fleet resolve-storm` (`--confirm`-gated), `user` (resets API keys) | Human-in-the-loop only |

The strongest control is the **scope you grant the Datto RMM credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Datto RMM API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
