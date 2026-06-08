# CIPP + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the CIPP
> API. Not affiliated with, endorsed by, or sponsored by CyberDrain.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/cipp/">
    <img src="../../docs/assets/video/cipp/animated-og.gif" alt="CIPP demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/cipp/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

First single-binary CLI for CIPP  -  offline SQLite store, fleet posture analytics, and cross-tenant fan-out no other CIPP tool has. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the CIPP skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `cipp-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `cipp-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `cipp-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the CIPP MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `cipp-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download CIPP MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/cipp-v0.1.0/cipp-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every CIPP release on the [releases page](https://github.com/servosity/msp-skills/releases?q=cipp).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install cipp@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the CIPP Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.ps1 | iex`. Then authenticate per the README and run `cipp-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.sh)
```

The installer drops both `cipp-cli` (the CLI) and `cipp-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
cipp-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update cipp@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/cipp --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/cipp --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `cipp-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the cipp skill from https://github.com/servosity/msp-skills/tree/main/skills/cipp. The skill defines how its required CLI (`cipp-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your CIPP portal):

```bash
CIPP_API_KEY=<value> cipp-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which tenants still have users without MFA registered? | `cipp-cli posture --dimension mfa` |
| How does Conditional Access coverage compare across all tenants? | `cipp-cli posture --dimension ca` |
| Where am I paying for M365 licenses nobody uses? | `cipp-cli licenses waste` |
| Which licensed accounts haven't signed in for 90 days, across every client? | `cipp-cli users stale --days 90` |
| Which tenants drifted off our security baseline since the last check? | `cipp-cli standards drift` |
| Pull one read across every client tenant at once and keep it locally | `cipp-cli fanout --endpoint /ListUsers --all-tenants --save` |
| Offboard a batch of departures from a CSV with 429 backoff and resume | `cipp-cli bulk --from offboards.csv --execute` |
| Are my CIPP credentials and connectivity healthy? | `cipp-cli doctor` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most CIPP integrations and MCP servers proxy each question into a live, single-tenant API call. That's fine for one record. It dies at QBR time, when you're asking *how many users still lack MFA across all 40 clients* or *which tenants drifted off baseline this month* - every tenant is a separate round trip, and Microsoft Graph throttles you at fleet scale.

This skill fans one read out across every tenant and persists it into a **local SQLite store**. The fleet questions then become instant local rollups: `posture` joins MFA, Conditional Access, Standards, and BPA per tenant; `licenses waste` joins synced licenses against users to surface unused seats; and `standards drift` compares two synced snapshots over time. Offline, resumable after a 429, and work a stateless API wrapper can't do.

## The pain this closes

CIPP scopes nearly every endpoint by a single `tenantFilter`, so the portal walks you through one client tenant at a time. The fleet-wide questions an MSP owner actually asks at QBR time - *how many users still lack MFA across all clients, which tenants drifted off baseline, where licenses sit unused* - have no single screen. Per the official [CIPP API documentation](https://docs.cipp.app/user-documentation/cipp/integrations/cipp-api), the API authenticates with an Azure AD app registration over OAuth2 client credentials and runs through Microsoft Graph, which throttles at scale; a bulk change across dozens of tenants hits HTTP 429 / Retry-After, and a halted batch means starting over. As of June 2026 there's no published single-binary CLI or MCP server for CIPP that we could find.

This skill closes that gap:

- **`cipp-cli fanout --endpoint /ListUsers --all-tenants --save`** - run one read across every tenant at once, with throttle-aware backoff and resume-after-halt, into a local store.
- **`cipp-cli posture --dimension mfa`** (also `ca`, `standards`, `bpa`) - the cross-tenant posture matrix the UI never renders.
- **`cipp-cli licenses waste`** - assigned-but-unused seats across all tenants.
- **`cipp-cli users stale --days 90`** - licensed accounts with no recent sign-in, fleet-wide.
- **`cipp-cli bulk --from offboards.csv --execute`** - drive add-user / offboard / remove-user / set-forwarding from a CSV with 429 backoff and resume-after-429 checkpointing.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The CIPP MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your CIPP credentials once.

### Is my CIPP data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need my own CIPP instance to use this?

Yes - this drives your own self-hosted CIPP instance's API. In CIPP you create an API client (it issues a client ID, secret, tenant ID, and token URL); the CLI authenticates with OAuth2 client credentials via `cipp-cli auth login`, or you save a bearer token with `cipp-cli auth set-token`. It reads and acts through your CIPP - it does not replace it.

### Will this hit Microsoft Graph or CIPP rate limits?

The cross-tenant rollups (`posture`, `licenses waste`, `users stale`, `standards drift`) read the local store after one fan-out, so repeat questions cost zero API calls. `fanout` throttles with `--concurrency` and the client retries 429s with Retry-After; `bulk` checkpoints completed rows with `--resume` so a throttled batch continues instead of restarting.

### Can this change my tenants, or only read?

Both. CIPP is a full management API, so the CLI can create users, offboard, set forwarding, and more. But the fleet rollups are read-only, and `bulk` prints its plan by default and only writes when you pass `--execute`. If you want reporting only, scope the API client to read-only in CIPP - the credential is the boundary.

### Does it replace the CIPP portal?

No. CIPP stays your portal for deep single-tenant work. This skill adds the cross-tenant reporting layer and lets your AI agent drive CIPP from natural language.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read (rollups and lists)** | `cipp-cli posture --dimension mfa`, `cipp-cli licenses waste`, `cipp-cli users stale --days 90`, `cipp-cli standards drift`, `cipp-cli fanout --endpoint /ListUsers --all-tenants`, `cipp-cli list-tenants`, `cipp-cli doctor` | Allow |
| **Write (routine)** | `cipp-cli add-user`, `cipp-cli edit-user`, `cipp-cli bulk --from changes.csv --execute` (plans by default; writes only with `--execute`) | Preview with `--dry-run`, then a reviewed write |
| **Credential / security** | `cipp-cli exec-reset-mfa`, `cipp-cli exec-per-user-mfa`, `cipp-cli exec-get-local-admin-password`, `cipp-cli exec-token-exchange` | Human-in-the-loop only |
| **Destructive** | `cipp-cli exec-offboard-user`, `cipp-cli remove-user`, `cipp-cli exec-device-delete`, `cipp-cli delete-sharepoint-site` | Human-in-the-loop only, explicit confirmation |

The strongest control is the **scope you grant the CIPP credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the CIPP API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-05._
