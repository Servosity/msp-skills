# Domotz + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Domotz
> API. Not affiliated with, endorsed by, or sponsored by Domotz Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/domotz/">
    <img src="../../docs/assets/video/domotz/animated-og.gif" alt="Domotz demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/domotz/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Domotz endpoint, plus a local SQLite fleet mirror that answers cross-site questions. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Domotz skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `domotz-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `domotz-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `domotz-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Domotz MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `domotz-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Domotz MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/domotz-v0.1.0/domotz-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Domotz release on the [releases page](https://github.com/servosity/msp-skills/releases?q=domotz).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install domotz@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Domotz Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.ps1 | iex`. Then authenticate per the README and run `domotz-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.sh)
```

The installer drops both `domotz-cli` (the CLI) and `domotz-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
domotz-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update domotz@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/domotz --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/domotz --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `domotz-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the domotz skill from https://github.com/servosity/msp-skills/tree/main/skills/domotz. The skill defines how its required CLI (`domotz-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Domotz portal):

```bash
DOMOTZ_API_KEY=<value> DOMOTZ_PUBLIC_API_KEY=<value> DOMOTZ_REGION=<value> domotz-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Is anything on fire across all my sites? | `domotz-cli fleet health --agent` |
| Which Collectors (sites) are offline or degraded right now? | `domotz-cli fleet agents --agent` |
| What devices are offline across every client? | `domotz-cli fleet offline --agent` |
| What new devices appeared on any network in the last day? | `domotz-cli fleet new --since 24h --agent` |
| Give me one asset inventory across every site | `domotz-cli fleet inventory --csv` |
| Where are IP conflicts across the fleet? | `domotz-cli fleet ip-conflicts --agent` |
| Which devices can't be fully monitored (auth/SNMP gaps)? | `domotz-cli fleet unmonitored --agent` |
| How many devices of each vendor do we manage? | `domotz-cli fleet breakdown --by vendor --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Domotz integrations and MCP servers proxy each question into a live, agent-scoped API call. That's fine for one device. It dies at scale, when you're asking "which of my 40 client sites has a device offline right now?" or "give me one asset inventory across every Collector for the QBR."

This skill syncs Domotz into a **local SQLite mirror** with full-text search. Aggregate questions become one local query: instant, offline, and the AI sees the answer, not raw pages of JSON. Compound commands like `fleet health`, `fleet inventory`, and `fleet new` roll up across every Collector, device, and site at once - work a stateless API wrapper can't do in one call.

## The pain this closes

Network-monitoring threads on r/msp keep surfacing the same gap with per-site tools: visibility is organized one Collector at a time, but the questions you actually ask are fleet-wide. "Which client has something down?" "What appeared on a network overnight?" "Give me one asset list for the QBR." In the portal each of those means clicking through every site by hand or exporting per-site and merging spreadsheets - the data is all there, it just doesn't roll up across clients in a single view.

This skill closes that gap:

- **`fleet health`** - one status board across every site, so "is anything on fire?" is a single glance.
- **`fleet offline`** - every offline device across all sites, prioritized, instead of paginating each agent.
- **`fleet new --since 24h`** - a rogue-device sweep across all clients at once.
- **`fleet inventory --csv`** - the QBR/CMDB export for the whole fleet in one line.
- **`fleet unmonitored`** - auth/SNMP coverage gaps surfaced fleet-wide, so blind spots don't go silent.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Domotz MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Domotz credentials once.

### Is my Domotz data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need a special Domotz plan to use the API?

No add-on is required. You generate an API Key from the Domotz Portal under **Settings > Services > API Keys** on your existing account. A handful of endpoints (company areas, team moves, some RBAC) are Enterprise-plan only; everything else works on standard accounts, and the CLI returns a clear error when a plan gates an endpoint.

### Will this replace my Domotz portal?

No. The portal and mobile app are built for working one site at a time and remain the place for live device interaction. This skill adds the fleet-wide rollups no single portal view does - across every Collector, from the terminal, scriptable and agent-ready.

### Will this hit my Domotz API rate limits?

Cross-site questions read from the **local SQLite mirror**, not the live API, so a `fleet inventory` across 40 sites is one local query, not 40 API calls. The API is only touched on `sync`, on live commands, and where you explicitly pass `--data-source live`. A global `--rate-limit` flag caps request rate when you do go live.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet health`, `fleet offline`, `fleet inventory`, `agent device list`, `search`, `topology`, `export` | Allow |
| Write (routine) | `agent device create-eye-snmp`, `agent device edit`, `custom-tag create`, `alert-profile binding bind-alert-profile-to-device`, `device-profile apply`, `import` | Preview with `--dry-run`, then a reviewed write |
| Physical / device control | `agent device power-action-on`, `agent device trigger-outlet-action`, `agent device backup-configuration`, `agent connection create-agent-vpnconnection` | Human-in-the-loop; never unattended |
| Credential / security | `agent device set-credentials`, `agent device set-snmpauthentication`, `agent device get-snmpauthentication` (returns SNMP secrets), `rbac create-user` | Human-in-the-loop only |
| Destructive | `agent delete`, `agent device delete`, `inventory delete`, `rbac delete-user` | Human-in-the-loop only, explicit confirmation |

The strongest control is the **scope you grant the Domotz credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Domotz API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
