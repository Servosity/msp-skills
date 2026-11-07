# Veeam Service Provider Console + AI - for Claude, the ChatGPT desktop app, GitHub Copilot, Gemini CLI, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Veeam
> API. Not affiliated with, endorsed by, or sponsored by Veeam Software.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/veeam/">
    <img src="../../docs/assets/video/veeam/animated-og.gif" alt="Veeam demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/veeam/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

<!-- first-party-note:start -->
> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](../servosity) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.
<!-- first-party-note:end -->

<!-- backup-vendor-note:start -->
> **Comparing backup vendors?** One of our MSP partners told us, "My guys now spend a tenth of the time on backups." Fixing is included in every Servosity co-managed backup plan, and our engineers tell you what they did. [See how co-managed backup works](https://servosity.com/?utm_source=msp-skills&utm_medium=connector-note&utm_campaign=veeam&utm_content=readme)
<!-- backup-vendor-note:end -->

Every Veeam Service Provider Console endpoint as a typed command, plus a local multi-tenant mirror, cross-company rollups, and drift detection no per-instance Veeam tool offers. Works with the AI you already use - **ChatGPT** (desktop app), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Gemini CLI**. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Veeam skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `veeam-mcp` (no JSON editing). |
| **ChatGPT** (desktop app) | Run installer, then add `veeam-mcp` in **Settings > MCP servers** as a local (STDIO) server. ChatGPT on the web is not supported yet. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `veeam-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

The ChatGPT desktop app runs `veeam-mcp` locally and shares MCP configuration with Codex ([OpenAI](https://learn.chatgpt.com/docs/extend/mcp)). ChatGPT on the web needs a remote (HTTP) server, which msp-skills does not support yet. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: Gemini CLI runs the local binary you just installed, while Microsoft 365 Copilot and the Gemini app need a remote server, which msp-skills does not support yet.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | Not supported by msp-skills yet: it needs a remote server. A [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list. |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. See [mcp-install.md](./mcp-install.md). The **Gemini app** needs a remote server and is not supported by msp-skills yet. |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

> **Claude Desktop bundle:** the `.mcpb` launches on macOS (Intel and Apple Silicon), Windows x64 and Linux x64. Every tool shells out to the companion `veeam-cli`. Releases cut from 2026-09-18 on ship that CLI inside the bundle; older bundles contain only the MCP server, so if yours lacks the companion, run the Path A or Path B installer below first (or set `VEEAM_CLI_PATH` to an existing binary). Details: [#331](https://github.com/Servosity/msp-skills/issues/331).

[**Download Veeam MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/veeam-v0.1.8/veeam-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Veeam release on the [releases page](https://github.com/servosity/msp-skills/releases?q=veeam).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install veeam@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Veeam Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/veeam/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/veeam/install.ps1 | iex`. Then authenticate per the README and run `veeam-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/veeam/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/veeam/install.sh)
```

The installer drops both `veeam-cli` (the CLI) and `veeam-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
veeam-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/veeam/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/veeam/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update veeam@msp-skills`.

### Add to Claude Desktop, the ChatGPT desktop app, GitHub Copilot, Gemini CLI, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the ChatGPT desktop app. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/veeam --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/veeam --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `veeam-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the veeam skill from https://github.com/servosity/msp-skills/tree/main/skills/veeam. The skill defines how its required CLI (`veeam-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Veeam portal):

```bash
VEEAM_BASE_URL=<value> VEEAM_TOKEN=<value> veeam-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data. It exits 0 even when the credential is rejected, so scripts should add `--fail-on error`.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which backup jobs are failing across all my customers? | `veeam-cli fleet-health --agent` |
| Which jobs and agents haven't succeeded in 3+ days? | `veeam-cli stale-backups --days 3 --agent` |
| Which protected workloads are past their RPO? | `veeam-cli at-risk --rpo 24h --agent` |
| What active alarms need a human, deduped across tenants? | `veeam-cli alarms-triage --severity Error --agent` |
| How is one customer doing across backups, agents, and alarms? | `veeam-cli company-overview --company "Contoso Ltd" --agent` |
| What's each customer's license usage and the change? | `veeam-cli license-usage --agent` |
| What changed across the fleet in the last 24h? | `veeam-cli since 24h --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Veeam integrations and MCP servers proxy each question into a live API call against one console. That's fine for one job. It dies at scale, when you're asking "which of my 60 customers has a failed backup right now" or "who's past their RPO before this QBR".

This skill syncs Veeam Service Provider Console into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `fleet-health`, `stale-backups`, and `at-risk` join across companies, backup servers, jobs, agents, alarms, and protected workloads - work a stateless API wrapper can't do.

## The pain this closes

Backups that fail silently are the recurring nightmare on r/msp and the Veeam community forums: the job stops succeeding, nobody notices, and you find out the week a customer needs a restore. VSPC gives MSPs a real multi-tenant console, but it's organized per-tenant - the questions that matter most across a book of business ("which customers have a failed backup right now?", "who's past their RPO?", "which agents went stale?") mean clicking into each company one at a time. The data is all in VSPC; getting a fleet-wide answer out of it is the work, and it lands at the worst moments: the morning standup, a restore request, the monthly invoice.

This skill answers those questions directly:

- `fleet-health` - jobs by last status, agents online/offline, and active alarms per tenant
- `stale-backups --days 3` - every job and agent with no successful run in N days, across all tenants
- `at-risk --rpo 24h` - protected workloads past their RPO or missing a recent restore point
- `alarms-triage --severity Error` - active alarms deduped and grouped by company and severity
- `license-usage` - per-organization consumption with the delta since last run, for billing

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, in the **ChatGPT desktop app**. It runs MCP servers locally (STDIO), the way `veeam-mcp` runs, and shares MCP configuration with Codex ([OpenAI](https://learn.chatgpt.com/docs/extend/mcp)): add `veeam-mcp` under **Settings > MCP servers** as a STDIO server, or add it once to Codex's `~/.codex/config.toml`. **ChatGPT on the web** is not supported by msp-skills yet: it needs a remote (HTTP) server. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Veeam credentials once.

### Is my Veeam data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries, and the MCP server runs over stdio unless you deliberately start it in HTTP mode (`veeam-mcp --transport http`): that mode refuses to start until `VEEAM_MCP_HTTP_TOKEN` is set (32 or more characters), every request must send it as `Authorization: Bearer <token>`, and any address other than loopback also needs `--tls-cert` and `--tls-key`. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Does this work with standalone Veeam Backup & Replication (VBR)?

No. This skill targets the **Veeam Service Provider Console (VSPC) v3 REST API** for multi-tenant MSP estates. A single VBR server with no VSPC in front of it exposes a separate API.

### Will this hammer my VSPC appliance or hit rate limits?

The local mirror is the point. You `sync` once, then rollups, triage, and search run against local SQLite, so day-to-day questions never touch the appliance. Only `sync` and the live `get` commands call VSPC.

### Do I need a special Veeam license or partner status?

You need a VSPC appliance you administer and a REST API bearer token (`POST /token`), plus its base URL. The skill authenticates as you and adds nothing to your Veeam account.

### Can it change backup jobs or run restores?

The full VSPC v3 surface is exposed, including create/delete jobs and agent deployment, but those are gated human-in-the-loop in [governance.md](./governance.md). Day-to-day this is read-first monitoring; destructive and infrastructure commands need explicit approval.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

VSPC exposes ~1000 commands. The safe default is read-only; everything that writes, installs, deletes, or touches credentials is gated.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read (~610) | `fleet-health`, `stale-backups`, `at-risk`, `alarms-triage`, `company-overview`, `license-usage`, `since`, all `get`/`list`/`search` | Allow |
| Write (config) | `configuration create/patch backup-policy`, `infrastructure create/assign jobs`, `discovery create/patch rules`, `alarms acknowledge/resolve` | Preview with `--dry-run`, then a reviewed write |
| Infrastructure and agent execution | `discovery install-backup-agent-on-computer` / `reboot-computer` / `start-rule`, `infrastructure activate` / `force-collect` | Human-in-the-loop, explicit confirmation |
| Destructive | `infrastructure delete-tenant` / `delete-backup-agent`, `configuration delete-backup-policy`, `alarms delete-active` | Human-in-the-loop only |
| Credential / security | `authentication generate-*` / `decrypt-pkcs12-container`, `infrastructure create/get *-credentials` and `*-encryption-passwords`, `users tokens revoke-authentication` | Human-in-the-loop only |

The strongest control is the **scope you grant the VSPC token** - the CLI can only do what the token is permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Veeam API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-13._
