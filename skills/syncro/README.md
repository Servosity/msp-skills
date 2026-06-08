# Syncro + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Syncro
> API. Not affiliated with, endorsed by, or sponsored by Servably, Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/syncro/">
    <img src="../../docs/assets/video/syncro/animated-og.gif" alt="Syncro demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/syncro/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Syncro PSA and RMM workflow in your terminal, plus a local database, offline search, and cross-entity reports no other Syncro tool has. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Syncro skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `syncro-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `syncro-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `syncro-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Syncro MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `syncro-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Syncro MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/syncro-v0.1.0/syncro-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Syncro release on the [releases page](https://github.com/servosity/msp-skills/releases?q=syncro).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install syncro@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Syncro Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/syncro/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/syncro/install.ps1 | iex`. Then authenticate per the README and run `syncro-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/syncro/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/syncro/install.sh)
```

The installer drops both `syncro-cli` (the CLI) and `syncro-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
syncro-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/syncro/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/syncro/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update syncro@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/syncro --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/syncro --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `syncro-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the syncro skill from https://github.com/servosity/msp-skills/tree/main/skills/syncro. The skill defines how its required CLI (`syncro-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Syncro portal):

```bash
SYNCRO_API_KEY=<value> SYNCRO_SUBDOMAIN=<value> syncro-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

Ask in plain language; your agent runs the command. Every row below reads from a local mirror of your Syncro data, so the answer spans **every customer at once** and comes back in seconds.

| Question your MSP keeps asking | Command |
| --- | --- |
| Which customers have logged time we never invoiced? | `syncro-cli billing uninvoiced` |
| Which closed tickets had billable time that was never invoiced? | `syncro-cli billing drift` |
| How is our unpaid AR aging (0-30/30-60/60-90/90+)? | `syncro-cli billing ar-aging` |
| What is our revenue per labor hour by customer? | `syncro-cli customers margin` |
| Which open tickets are going stale with no recent activity? | `syncro-cli tickets aging` |
| Which assets are missing the most critical patches? | `syncro-cli assets patch-gaps` |
| Which customers generate the most RMM alert noise? | `syncro-cli alerts noise` |
| Which RMM alerts never became a ticket? | `syncro-cli alerts orphans` |
| Give me one cross-entity card for a single customer. | `syncro-cli customers profile 12345` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Syncro integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "how many billable hours did we log but never invoice across all 47 clients last quarter".

This skill syncs Syncro into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `billing uninvoiced`, `customers margin`, and `customers profile` join timer entries, tickets, invoices, assets, and RMM alerts across every customer - work a stateless API wrapper can't do.

## The pain this closes

MSPs leak roughly **10% of revenue** to billing errors, and industry analyses name time-tracking inaccuracy the single biggest source - labor logged on a ticket that quietly never makes it onto an invoice (DeskDay, *Revenue Leakage in Mid-Market MSPs*; rev.io, *Where MSPs Are Losing Money on Billing*). Syncro has the time entries and the invoices, but the portal never *ranks* logged-but-unbilled hours by customer, so the leak stays invisible until a quarter later.

This skill turns that leak - and the other questions an owner keeps re-asking - into one command each:

- `syncro-cli billing uninvoiced` - logged-but-unbilled labor ranked by customer
- `syncro-cli billing drift` - closed tickets that had billable time and never got invoiced
- `syncro-cli billing ar-aging` - unpaid invoices bucketed by age, worst first
- `syncro-cli tickets aging` - open tickets with no recent activity, going stale
- `syncro-cli assets patch-gaps` - assets missing critical patches, ranked across every customer

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Syncro MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Syncro credentials once.

### Is my Syncro data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my Syncro API rate limits?

Day-to-day questions read from the local SQLite mirror, not the live API, so they don't touch your rate limit at all. Only `sync` and explicit live reads call Syncro, and the CLI ships a `--rate-limit` flag plus response caching to stay polite.

### Do I need to be a Syncro partner or on a specific plan?

No. You need a Syncro account and an API token from your own portal (Admin area). It authenticates as you, against your subdomain, with whatever permissions that token is scoped to - no partner program or special tier required.

### Will this replace my Syncro portal?

No. It reads from your Syncro account for the cross-customer questions and bulk analysis the portal makes tedious. You still run tickets, billing, and RMM day-to-day in Syncro; this is the fast lane for the questions an owner keeps re-asking.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** | `billing uninvoiced`, `billing ar-aging`, `customers margin`, `tickets aging`, `assets patch-gaps`, `alerts noise`, `customers profile`, and every `list` / `get` / `search` / `export` | Allow |
| **Write (routine)** | `tickets create`, `tickets comment create`, `tickets update`, `invoices create`, `customers update`, `appointments create` | Preview with `--dry-run`, then a reviewed write |
| **Destructive / config** | `tickets delete`, `invoices delete`, `customers delete`, `contracts delete`, and the other `delete` commands | Human-in-the-loop only |

The strongest control is the **scope you grant the Syncro credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Syncro API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
