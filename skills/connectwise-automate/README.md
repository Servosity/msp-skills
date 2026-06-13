# ConnectWise Automate + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the ConnectWise
> API. Not affiliated with, endorsed by, or sponsored by ConnectWise, LLC.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/connectwise-automate/">
    <img src="../../docs/assets/video/connectwise-automate/animated-og.gif" alt="ConnectWise Automate demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/connectwise-automate/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Sync your entire ConnectWise Automate fleet into local SQLite and answer cross-client questions the per-server web UI can't: fleet-wide health roll-ups, stale-agent sweeps, patch-compliance-by-client, and overnight drift. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the ConnectWise skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `connectwise-automate-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `connectwise-automate-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `connectwise-automate-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the ConnectWise MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `connectwise-automate-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download ConnectWise MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/connectwise-automate-v0.1.0/connectwise-automate-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every ConnectWise release on the [releases page](https://github.com/servosity/msp-skills/releases?q=connectwise-automate).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install connectwise-automate@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the ConnectWise Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-automate/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-automate/install.ps1 | iex`. Then authenticate per the README and run `connectwise-automate-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-automate/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-automate/install.sh)
```

The installer drops both `connectwise-automate-cli` (the CLI) and `connectwise-automate-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
connectwise-automate-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-automate/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-automate/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update connectwise-automate@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/connectwise-automate --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/connectwise-automate --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `connectwise-automate-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the connectwise-automate skill from https://github.com/servosity/msp-skills/tree/main/skills/connectwise-automate. The skill defines how its required CLI (`connectwise-automate-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your ConnectWise portal):

```bash
CONNECTWISE_AUTOMATE_SERVER=<value> CONNECTWISE_AUTOMATE_CLIENT_ID=<value> CONNECTWISE_AUTOMATE_TOKEN=<value> connectwise-automate-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which agents haven't checked in for 30+ days, by client? | `connectwise-automate-cli stale-agents --days 30 --agent` |
| Which clients are behind on patches, worst first? | `connectwise-automate-cli patch-compliance --agent` |
| What open alerts need a human across every client? | `connectwise-automate-cli alert-triage --min-priority 3 --agent` |
| What's my whole-fleet health right now? | `connectwise-automate-cli fleet-health --agent` |
| Give me a one-line snapshot per client for the review. | `connectwise-automate-cli client-rollup --agent` |
| What operating systems are end-of-life across the fleet? | `connectwise-automate-cli os-inventory --eol-only --agent` |
| What changed overnight - alerts, check-ins, patches? | `connectwise-automate-cli since --hours 24 --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most ConnectWise Automate integrations and MCP servers proxy each question into a live API call. That's fine for one computer. It dies at scale, when you're asking "how many agents are offline across all 40 clients" or "who's behind on patches before this morning's QBR".

This skill syncs ConnectWise Automate into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `fleet-health`, `patch-compliance`, and `alert-triage` join across computers, clients, locations, alerts, and patch history - work a stateless API wrapper can't do.

## The pain this closes

Automate (formerly LabTech) is built around a per-server console that excels at drilling into one endpoint and falls apart at "show me this across every client." On MSPGeek - the long-running ConnectWise Automate community forum - and on r/msp, the recurring complaints are the same: no fast cross-client roll-up, offline and stale agents quietly inflating license counts, and patch-compliance for a QBR that means assembling a report per client by hand. The data is all in Automate; getting a fleet-wide answer out of it is the work - and it lands at the worst times, before a license true-up or the morning of a review.

This skill answers those questions directly:

- `stale-agents --days 30` - offline agents grouped by client, ready for a true-up
- `patch-compliance` - per-client patch posture, worst first, ready for the QBR
- `alert-triage --min-priority 3` - what needs a human across every client this morning
- `fleet-health` - online/offline, last-contact age, and open alerts in one roll-up
- `os-inventory --eol-only` - end-of-life OSes flagged for the refresh conversation

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The ConnectWise MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your ConnectWise credentials once.

### Is my ConnectWise data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Is this for ConnectWise Automate or ConnectWise Manage (PSA)?

ConnectWise Automate - the RMM, formerly LabTech. ConnectWise Manage (the PSA) is a separate skill. This one talks to your Automate server's API for computers, clients, alerts, patching, scripts, and monitors.

### Will this hit my Automate API rate limits?

The local mirror is the point. You `sync` once, then every roll-up, triage, and search runs against local SQLite, so day-to-day questions never touch the API. Only `sync` and the live `list`/`get` commands call Automate.

### Do I need to be a ConnectWise partner?

You need an Automate server you administer plus an API token, and a registered integration `clientId` GUID (required for v2020.11+). The skill authenticates as you and adds nothing to your ConnectWise account.

### Will this replace the Automate console?

No. It's for cross-client reporting and triage from the terminal or your AI agent. The console stays where you configure monitors, scripts, and policies.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet-health`, `stale-agents`, `patch-compliance`, `alert-triage`, `client-rollup`, `os-inventory`, `since`, all `list`/`get`/`search` | Allow |
| Endpoint and fleet actions | `computers command-execute` (runs a real command on an agent), `patching deploy-approved` / `deploy-security` / `reattempt-failed` (fleet-wide patch deployment) | Human-in-the-loop, explicit confirmation |
| Write (data) | `import <resource>` (creates/upserts records via the API) | Preview with `--dry-run`, then a reviewed write |
| Credential | `apitoken mint` / `refresh`, `auth set-token` | Human-in-the-loop only |

The strongest control is the **scope you grant the ConnectWise Automate credentials** - the CLI can only do what the token is permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the ConnectWise API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-12._
