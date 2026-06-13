# Acronis + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Acronis
> API. Not affiliated with, endorsed by, or sponsored by Acronis International GmbH.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/acronis/">
    <img src="../../docs/assets/social/acronis/wide-1200x630.png" alt="Acronis Cyber Protect Cloud - MCP server and Claude Code Skill" width="600">
  </a>
</p>
<p align="center"><sub><a href="https://msp-skills.compoundingteams.com/skills/acronis/">Full skill page</a> - install, outcomes, safety model.</sub></p>
<!-- media:end -->

<!-- first-party-note:start -->
> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](../servosity) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.
<!-- first-party-note:end -->

The first real CLI for the Acronis Cyber Protect Cloud platform  -  every tenant, agent, and usage metric mirrored locally, with cross-tenant rollups no single API call returns. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Acronis skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `acronis-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `acronis-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `acronis-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Acronis MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `acronis-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Acronis MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/acronis-v0.1.0/acronis-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Acronis release on the [releases page](https://github.com/servosity/msp-skills/releases?q=acronis).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install acronis@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Acronis Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/acronis/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/acronis/install.ps1 | iex`. Then authenticate per the README and run `acronis-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/acronis/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/acronis/install.sh)
```

The installer drops both `acronis-cli` (the CLI) and `acronis-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
acronis-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/acronis/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/acronis/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update acronis@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/acronis --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/acronis --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `acronis-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the acronis skill from https://github.com/servosity/msp-skills/tree/main/skills/acronis. The skill defines how its required CLI (`acronis-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Acronis portal):

```bash
ACRONIS_CYBER_PROTECT_BEARER_AUTH=<value> ACRONIS_DATACENTER=<value> acronis-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Whose backups failed, succeeded, or went stale across every customer last night? | `acronis-cli health` |
| Show me each failed or missed backup in the last 24 hours | `acronis-cli failures --since 24h` |
| Which customers have gone too long without a good backup (SLA breach)? | `acronis-cli freshness --sla 48h --breached` |
| Which backup agents have silently gone offline across all tenants? | `acronis-cli agents stale --older-than 7d` |
| Did every customer's agents update after the rollout? | `acronis-cli agents compliance` |
| Where am I billing for protection that isn't actually running? | `acronis-cli coverage --unprotected` |
| Which usage has no matching SKU, and which paid SKUs have zero usage? | `acronis-cli reconcile usages` |
| Everything about one customer before the call? | `acronis-cli customer "<TENANT_ID>"` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Acronis integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking which of your 40-plus customer tenants had a failed backup last night and which agents have silently gone offline - one paginated API call per tenant, behind token exchange and the datacenter-host dance.

This skill syncs Acronis into a **local SQLite mirror** with full-text search - the exact local-store approach Acronis's own developer monitoring guide tells MSPs to build. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `health`, `coverage --unprotected`, and `customer` join across tenants, agents, backup tasks, usage, and offering items - work a stateless API wrapper can't do.

## The pain this closes

Backup verification is the one job an MSP can never let slide, and Acronis Cyber Protect Cloud makes it a per-tenant chore. Acronis's own Management Portal Administrator Guide is explicit that partner-level dashboards and reports work only inside one partner tenant - so *whose backups failed last night* has no single cross-customer view. You drill into each tenant, or you export and stitch in Excel. The failure mode that hurts most is the quiet one: a backup agent that silently stops checking in is the most common way protection lapses without anyone noticing, and it only surfaces when a customer asks for a restore that isn't there. It's the daily slog MSPs describe on r/msp whenever backup monitoring comes up.

After one `sync`, the cross-tenant questions become one offline query:

- `acronis-cli health` - backup success / failure / stale across your whole book of customers in one table.
- `acronis-cli agents stale --older-than 7d` - every silently offline agent, across all tenants, sorted by customer.
- `acronis-cli freshness --sla 48h --breached` - customers who have gone too long without a good backup.
- `acronis-cli coverage --unprotected` - tenants billed for protection that isn't actually running.
- `acronis-cli reconcile usages` - usage with no SKU and paid SKUs with zero usage, so invoices match reality.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Acronis MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Acronis credentials once.

### Is my Acronis data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my Acronis API rate limits?

The local mirror exists so reads stop hitting the API. After the first `sync`, the cross-tenant views (`health`, `coverage`, `freshness`, `agents stale`, `reconcile usages`) run against local SQLite with zero API calls. Live calls respect a `--rate-limit` throttle, and sync is incremental - it only fetches what changed since the last checkpoint.

### Do I need to be an Acronis partner?

Yes. You authenticate with an API client created in your own Acronis Management Portal (an OAuth2 client, or a bearer token), so you need partner or admin access to the tenants you want to reach. The CLI can only see what that API client is scoped to.

### Which datacenter does it point at?

Whichever hosts your Acronis account. Set `ACRONIS_DATACENTER` (for example `us-cloud` or `eu2-cloud`), or pass `--datacenter` on `acronis-cli auth login`. The CLI builds the correct regional API host from it, so you never hand-assemble the datacenter URL.

### Will this replace the Acronis console?

No. The console stays best for configuring protection plans and running restores. This skill adds the cross-tenant reporting layer the partner dashboards don't - one place to ask whose backups failed, which agents are offline, and where billing and protection diverge.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `acronis-cli health`, `acronis-cli coverage --unprotected`, `acronis-cli agents stale`, `acronis-cli reconcile usages`, `acronis-cli customer "<TENANT_ID>"` | Allow |
| Write (routine) | `acronis-cli tenants create`, `acronis-cli tenants update`, `acronis-cli tenants offering-items update`, `acronis-cli clients create`, `acronis-cli agent-manager force-agent-update` | Preview with `--dry-run`, then a reviewed write |
| Credential / security | `acronis-cli idp request-token`, `acronis-cli idp revoke-token` | Human-in-the-loop only |
| Destructive | `acronis-cli tenants delete`, `acronis-cli clients delete`, `acronis-cli agent-manager delete-agent`, `acronis-cli agent-manager delete-agents` | Human-in-the-loop only |

The strongest control is the **scope you grant the Acronis credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Acronis API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
