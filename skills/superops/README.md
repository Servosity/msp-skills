# SuperOps + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the SuperOps
> API. Not affiliated with, endorsed by, or sponsored by SuperOps Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/superops/">
    <img src="../../docs/assets/video/superops/animated-og.gif" alt="SuperOps demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/superops/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every SuperOps PSA+RMM entity in your terminal, plus a local SQLite mirror that answers cross-entity questions the web UI can't. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the SuperOps skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `superops-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `superops-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `superops-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the SuperOps MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `superops-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download SuperOps MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/superops-v0.1.0/superops-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every SuperOps release on the [releases page](https://github.com/servosity/msp-skills/releases?q=superops).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install superops@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the SuperOps Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/superops/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/superops/install.ps1 | iex`. Then authenticate per the README and run `superops-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/superops/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/superops/install.sh)
```

The installer drops both `superops-cli` (the CLI) and `superops-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
superops-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/superops/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/superops/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update superops@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/superops --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/superops --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `superops-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the superops skill from https://github.com/servosity/msp-skills/tree/main/skills/superops. The skill defines how its required CLI (`superops-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs - a SuperOps **API token** (Settings > My Profile >
API token) and your **tenant subdomain** (Settings > MSP Information):

```bash
export SUPEROPS_API_TOKEN=<your api token>
export SUPEROPS_SUBDOMAIN=<your tenant subdomain>   # sent as the CustomerSubDomain header
export SUPEROPS_REGION=us                            # or eu for euapi.superops.ai (us is the default)
superops-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data. The
**scope of the token you mint** is the permission boundary - the CLI can only do what
that token is allowed to do.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Who's about to breach SLA, grouped by technician? | `superops-cli sla-watch --by tech --window 4h` |
| Which clients have alerts still sitting unresolved? | `superops-cli alert-coverage --client Acme` |
| Which endpoints are missing a critical patch and actively alerting? | `superops-cli at-risk-assets --client Acme` |
| Which open tickets has nobody touched in a week? | `superops-cli stale-tickets --days 7` |
| Everything about one client - sites, users, contracts, tickets, assets, open invoices? | `superops-cli client-360 "Acme Corp"` |
| Where is billable time concentrated before this month's invoicing? | `superops-cli unbilled --since 2026-05-01` |
| One ticket with its worklogs, client, and SLA for a triage agent? | `superops-cli context-ticket 12345 --agent` |
| Search every synced ticket, asset, and client | `superops-cli search "disk full"` |

Beyond the views: every SuperOps PSA+RMM entity - tickets, assets, alerts, clients,
sites, users, contracts, invoices, worklogs, technicians, service items, IT docs, KB -
is a typed `list`/`get` subcommand, and `raw query` runs any GraphQL read the typed
commands don't wrap. Every command is also available as an MCP tool.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most SuperOps integrations and MCP servers proxy each question into a live GraphQL call. That's fine for one record. It dies at scale, when you're asking "which open tickets across every client are about to breach SLA, and on whose queue" - the API rate-limits reads, and its list payloads don't even carry some of the cross-entity links that question needs.

This skill syncs SuperOps into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `sla-watch`, `client-360`, and `at-risk-assets` join across tickets, SLAs, technicians, clients, contracts, assets, alerts, and invoices - work a stateless API wrapper can't do.

## The pain this closes

SuperOps' pitch is one database for PSA and RMM - and that part is real. But the console still answers one entity at a time, and the platform's own AI is, as third-party reviewers put it, "more roadmap than reality" (Flamingo, "SuperOps Review for MSPs," 2026). So the questions you actually ask at month-end or before a QBR are still cross-entity questions no single console screen composes: who's about to miss SLA and on whose queue, how much billable time is sitting unreconciled for a client, which endpoints are both unpatched and actively alerting. And the GraphQL API makes the do-it-yourself version painful - it rate-limits reads and omits some of the very links those questions need (asset-to-ticket, aggregated child-activity timestamps), so any script has to fetch, cache, and join locally.

What this skill does about it:

- `superops-cli sla-watch --by tech --window 4h` - every open ticket breaching or about to breach its resolution SLA, grouped by technician. The dispatcher's morning triage.
- `superops-cli client-360 "Acme Corp"` - the client plus its sites, users, contracts, open tickets, assets, and open invoices in one bundle. Six console tabs in one command.
- `superops-cli at-risk-assets --client Acme` - endpoints whose patch status signals a missing/critical patch that also carry an unresolved alert. Remediation, prioritized.
- `superops-cli unbilled --since 2026-05-01` - billable logged worklog totaled per client (the reconciliation target), so you see where billable time is concentrated before the invoice run.

The first run, a `sync` pulls your tenant into local SQLite; after that these views run offline, instant, and free of rate limits.

See [pain-point.md](./pain-point.md) for the longer narrative.

### Known gaps (honest limits)

These views are computed from what the SuperOps list API actually exposes, and the CLI is candid about its proxies:

- `unbilled` surfaces **billable** logged time per client, not a strict worklog-minus-invoice diff - the API carries no per-entry "already billed" flag.
- `at-risk-assets` and `alert-coverage` use an **unresolved alert** as the proxy for "currently causing pain," because the asset-to-ticket link is absent from the list payloads.
- `context-ticket` bundles the synced ticket, its worklogs, client, and SLA; conversation and note threads are fetched live with `superops-cli tickets <id>`.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The SuperOps MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your SuperOps credentials once.

### Is my SuperOps data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my SuperOps API rate limits?

The local mirror exists so reads stop hitting the API. After the first `sync`, the cross-entity views (`sla-watch`, `client-360`, `at-risk-assets`, `alert-coverage`, `unbilled`, `stale-tickets`) run against local SQLite with **zero API calls**. Live calls respect a `--rate-limit` throttle, and `sync` is incremental and resumable - it only fetches what changed, and it treats resources your token can't reach as warnings, not failures.

### Does it work with the US and EU SuperOps regions?

Yes. The US host is the default; set `SUPEROPS_REGION=eu` to target the EU host (`euapi.superops.ai`). Your tenant subdomain goes in `SUPEROPS_SUBDOMAIN`, which the CLI sends as the `CustomerSubDomain` header on every request.

### Can it create or update tickets?

The typed commands are **read-only** by design - inspection, export, sync, and analysis. The one write path is `raw mutation`, the supported escape hatch for operations the typed commands don't wrap (for example `createTicket`, `updateTicket`, `resolveAlerts`). Pair it with `--dry-run` to preview the exact GraphQL request, and keep a human in the loop. `raw query` is the read-only counterpart.

### Will this replace my SuperOps console?

No - and it isn't trying to. The console stays best for in-app workflows like dispatch, time entry, and invoicing. This skill brings your tenant's data to whichever AI agent you already use, and answers the cross-entity questions - joined across PSA and RMM - that no single console screen composes today.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | every typed command + `raw query` - `sla-watch`, `client-360`, `at-risk-assets`, `alert-coverage`, `unbilled`, `stale-tickets`, `tickets list`, `assets list`, `search`, `sync` | Allow |
| Write (mutation escape hatch) | `raw mutation` - the only write path; wraps what the read-only typed commands don't cover (`createTicket`, `updateTicket`, `resolveAlerts`) | Preview with `--dry-run`, then a reviewed write |
| Destructive | no typed destructive command exists; a delete would be an explicit destructive GraphQL operation run through `raw mutation` | Human-in-the-loop only |

Every typed command is **read-only**; the binary's one path to remote change is `raw mutation`, and `--dry-run` prints the exact GraphQL request without sending it. Put the gate in your agent's policy: preview, show the request, get approval, then run.

The strongest control is the **scope you grant the SuperOps credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the SuperOps API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-05._
