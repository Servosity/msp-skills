# Cove Data Protection + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Cove Data Protection
> API. Not affiliated with, endorsed by, or sponsored by N-able.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/cove/">
    <img src="../../docs/assets/video/cove/animated-og.gif" alt="Cove Data Protection demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/cove/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

<!-- first-party-note:start -->
> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](../servosity) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.
<!-- first-party-note:end -->

The first CLI and MCP server for Cove Data Protection: fleet-wide backup health, billing usage, and storage trends from a terminal, with the local history the vendor console doesn't keep. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Cove Data Protection skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `cove-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `cove-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `cove-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Cove Data Protection MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `cove-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Cove Data Protection MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/cove-v0.1.0/cove-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Cove Data Protection release on the [releases page](https://github.com/servosity/msp-skills/releases?q=cove).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install cove@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Cove Data Protection Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.ps1 | iex`. Then authenticate per the README and run `cove-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.sh)
```

The installer drops both `cove-cli` (the CLI) and `cove-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
cove-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cove/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update cove@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/cove --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/cove --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `cove-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the cove skill from https://github.com/servosity/msp-skills/tree/main/skills/cove. The skill defines how its required CLI (`cove-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

See [mcp-install.md](./mcp-install.md) for the credentials `cove-cli` needs.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which devices failed their last backup since yesterday? | `cove-cli devices failures --since 24h --json` |
| Which devices have no successful backup in 3 days? | `cove-cli devices stale --days 3 --json` |
| What's the fleet health rollup, broken down per customer? | `cove-cli fleet health --by partner --json` |
| Which devices and customers grew storage fastest this week? | `cove-cli storage growth --since 7d --json` |
| What's the month-end billing usage per device, codes decoded? | `cove-cli billing usage --csv` |
| Which device SKUs or seat counts changed since last month? | `cove-cli billing changes --json` |
| What backup statuses flipped since my last snapshot? | `cove-cli devices changes --since 7d --json` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Cove Data Protection integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking how many devices failed or went stale across all 40 customers last night, with the F00 status codes already decoded to names.

This skill sweeps the whole partner tree in one command and snapshots Cove into a **local SQLite mirror** with full-text search. Aggregate questions become one local sweep: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `fleet health --by partner` and `storage growth --since 7d` join across devices, customers, and timestamped snapshots - trend work a stateless API wrapper can't do because the console keeps no history at all.

## The pain this closes

MSPs running Cove Data Protection (the N-able backup platform, formerly SolarWinds Backup) keep hitting the same wall: the backup.management console scopes to one customer at a time and keeps no history. Threads on **r/msp** and the **MSPGeek** community ask the same questions over and over - "how do I get one cross-customer list of every failed backup?" and "how do I trend storage when the dashboard only shows today's number?" In the console the answer is always: click into each customer, eyeball the dashboard, repeat tomorrow. A silently stale device - one whose last status reads fine but hasn't actually succeeded in days - stays invisible until a restore fails.

What this skill does about it:

- `cove-cli devices failures --since 24h --agent` - the morning ticket queue: every failed, aborted, or never-started device across the whole fleet, status codes decoded.
- `cove-cli devices stale --days 3 --json` - the silently stale devices the last-status view hides, ranked worst-first.
- `cove-cli fleet health --by partner --json` - the single-pane rollup (healthy, failed, stale, never-run) with a per-customer breakdown.
- `cove-cli storage growth --since 7d --agent` - which devices and customers grow storage fastest, from local snapshot history.
- `cove-cli billing usage --csv` - per-device SKU, used storage, and M365 seats as the month-end invoice export.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Cove Data Protection MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Cove Data Protection credentials once.

### Is my Cove Data Protection data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### What kind of Cove account does it need?

A dedicated **API User**, created in the Cove Management Console under **Users > API Users**. It issues a login name and an API token (shown only once); API Users can use the JSON-RPC API but cannot sign in to the console. Set `COVE_USERNAME` to the API user's login name, `COVE_PASSWORD` to the API token, and `COVE_PARTNER` to the customer/partner it was created for (required for API Users). The skill exchanges them for a cached session visa and refreshes it automatically. N-able retired the older per-user "API access" checkbox. See [mcp-install.md](./mcp-install.md).

### Can it restore files or browse backed-up data?

No. This skill speaks the Cove **management** API: fleet health, billing, storage trends, and enumeration. Restores and per-session file browsing run through the Backup Manager client and the storage-node Reporting Service, which this CLI does not cover.

### Will it replace the Cove portal?

No - it complements it. The console stays the place for restores, agent deployment, and per-session detail. This skill adds the cross-tenant sweep and the local trend layer the console structurally withholds.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read (tenant) | `devices failures`, `devices stale`, `fleet health`, `billing usage`, `storage growth`, every enumerate command | Allow |
| Local-only writes | `sync`, `snapshot`, `auth login`/`logout` - write only to your machine's SQLite mirror and visa cache, never the tenant | Allow (no tenant change) |
| Generic API call | `call <method>` reaches any of the 251 JSON-RPC methods, including the few that mutate | Human-in-the-loop; preview with `--dry-run` |

The strongest control is the **scope you grant the Cove Data Protection credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Cove Data Protection API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-07._
