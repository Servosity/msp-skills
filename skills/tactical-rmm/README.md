# Tactical RMM + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Tactical RMM
> API. Not affiliated with, endorsed by, or sponsored by AmidaWare LLC.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/tactical-rmm/">
    <img src="../../docs/assets/video/tactical-rmm/animated-og.gif" alt="Tactical RMM demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/tactical-rmm/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Tactical RMM endpoint as a typed command, plus an offline SQLite mirror and cross-entity fleet queries the web UI can't express. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Tactical RMM skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `tactical-rmm-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `tactical-rmm-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `tactical-rmm-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Tactical RMM MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `tactical-rmm-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Tactical RMM MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/tactical-rmm-v0.1.0/tactical-rmm-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Tactical RMM release on the [releases page](https://github.com/servosity/msp-skills/releases?q=tactical-rmm).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install tactical-rmm@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Tactical RMM Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/tactical-rmm/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/tactical-rmm/install.ps1 | iex`. Then authenticate per the README and run `tactical-rmm-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/tactical-rmm/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/tactical-rmm/install.sh)
```

The installer drops both `tactical-rmm-cli` (the CLI) and `tactical-rmm-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
tactical-rmm-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/tactical-rmm/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/tactical-rmm/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update tactical-rmm@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/tactical-rmm --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/tactical-rmm --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `tactical-rmm-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the tactical-rmm skill from https://github.com/servosity/msp-skills/tree/main/skills/tactical-rmm. The skill defines how its required CLI (`tactical-rmm-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Tactical RMM is self-hosted, so point the CLI at **your** instance and give it an API key from **Settings > Global Settings > API Keys**:

```bash
TACTICAL_RMM_BASE_URL=https://api.yourdomain.com TRMM_API_KEY=<value> tactical-rmm-cli doctor
```

`doctor` confirms the base URL and credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What's the overall health of my fleet right now? | `tactical-rmm-cli fleet health` |
| Which agents need attention first? | `tactical-rmm-cli triage --limit 20` |
| Which agents have gone dark or stopped checking in? | `tactical-rmm-cli agents stale --days 7` |
| Where are patches and reboots pending across every client? | `tactical-rmm-cli patch posture --by client` |
| What changed across the fleet in the last few hours? | `tactical-rmm-cli since "2h"` |
| Which endpoints have no checks configured (monitoring gaps)? | `tactical-rmm-cli coverage` |
| Which checks are failing on the most agents? | `tactical-rmm-cli checks worst` |
| Which agents have a given software package installed? | `tactical-rmm-cli software find --name openssl` |
| Run a command across a filtered cohort (preview first) | `tactical-rmm-cli agents bulk-run --command whoami --filter "os=windows,online=true"` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Tactical RMM integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "how many of my 40 clients have machines pending a reboot tonight, across every site" and the answer would page through every endpoint on your server.

This skill syncs Tactical RMM into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `fleet health`, `triage`, and `clients scorecard` join across agents, checks, alerts, and Windows-update state per client and site - work a stateless API wrapper can't do.

## The pain this closes

Tactical RMM is the budget-friendly, self-hosted favorite on r/msp and in the MSPGeek community - you own the data and pay nothing per endpoint. The recurring gripe is reporting: the web UI shows you one agent or one client at a time, with no built-in fleet-wide view of health, patch posture, or "what changed overnight" across every client. Everything is in the API, but a cross-client rollup means scripting against it yourself - so the questions that matter at a Monday stand-up or a QBR go unanswered.

This skill closes that gap:

- **`fleet health`** - whole-fleet posture (online/offline/overdue, failing checks, pending reboots, outstanding patches, active alerts) in one command.
- **`triage --limit 20`** - the agents that need attention first, ranked across offline state, failing checks, reboots, and patches.
- **`patch posture --by client`** - pending Windows updates and reboots rolled up per client or site, before patch night.
- **`since "2h"`** - what moved across the fleet since you last looked: new alerts, newly-offline agents.
- **`agents bulk-run --command whoami --filter "os=windows,online=true"`** - fan a command across a filtered cohort, previewed by default and run only with `--execute`.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Tactical RMM MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Tactical RMM credentials once.

### Is my Tactical RMM data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### It's self-hosted - how does it find my server?

You point it at your own instance: set `TACTICAL_RMM_BASE_URL` to your Tactical RMM API URL (for example `https://api.yourdomain.com`) and `TRMM_API_KEY` to a key from **Settings > Global Settings > API Keys**. Both are set once; nothing is hard-coded to a vendor cloud.

### Do I need to be a Tactical RMM customer or partner?

No. Tactical RMM is free and open source, and you self-host it. You only need an API key on your own instance; any Tactical RMM server with API access works.

### Will this hit my server's rate limits?

Rarely. Most questions run against the local SQLite mirror after a one-time `sync`, so they make zero API calls. The few commands that fan out live (like `services down` or `actions pending`) are paced and capped with `--max-scan-agents`.

### Will this replace my Tactical RMM web UI?

No - it complements it. The UI stays your system of record and remote-access console; this skill adds the cross-client query-and-automation layer it doesn't offer.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

Tactical RMM is a remote-control plane: beyond reading and editing config, it can run scripts on endpoints, reboot machines, install Windows updates, and manage credentials. The tiers below reflect that. The safe default for an autonomous agent is **reads plus previewed writes**; require a human for anything that runs on an endpoint, deletes, or touches credentials.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** | Fleet rollups (`fleet health`, `triage`, `patch posture`, `clients scorecard`, `coverage`, `since`, `checks worst`/`flapping`, `alerts digest`, `services down`, `software find`), get/list, `search`, `sync`, `analytics`. *Exception: listing API keys or reading the keystore, codesign token, or core settings returns stored secrets - those are credential-tier.* | Allow |
| **Write (routine config)** | Create/update clients, sites, checks, alerts & templates, scripts & snippets, automation & patch policies, autotasks, custom fields, schedules, agent notes, deployments, software/service records; `import`. | Preview with `--dry-run`, then a reviewed write |
| **Endpoint & script execution** | `agents bulk-run`, `scripts test`, autotasks/automation run, reboot/shutdown/wake/recover agents, install/scan Windows updates, uninstall software, reset checks, `maintenance set`, remote sessions (meshcentral, webvnc), server test actions. *Runs code on managed machines.* | Human-in-the-loop only - never unattended |
| **Credential & identity** | API keys (create/list/delete/update), keystore & codesign tokens, core settings, users, roles, sessions, password/2FA/TOTP resets. *Several of these return stored secrets.* | Human-in-the-loop only |
| **Destructive** | Delete agents, clients, sites, checks, scripts & snippets, alerts & templates, automation & patch policies, autotasks, custom fields, schedules, deployments, agent processes, pending actions. | Human-in-the-loop only, explicit confirmation |

The strongest control is the **scope you grant the Tactical RMM credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Tactical RMM API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
