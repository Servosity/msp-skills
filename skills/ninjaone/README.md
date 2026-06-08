# NinjaOne + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the NinjaOne
> API. Not affiliated with, endorsed by, or sponsored by NinjaOne, LLC.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/ninjaone/">
    <img src="../../docs/assets/video/ninjaone/animated-og.gif" alt="NinjaOne demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/ninjaone/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every NinjaOne report, plus a local store that answers fleet-wide questions no single API call can: patch compliance, backup gaps, AV blast-radius, health, drift. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the NinjaOne skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `ninjaone-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `ninjaone-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `ninjaone-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the NinjaOne MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `ninjaone-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download NinjaOne MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/ninjaone-v0.1.0/ninjaone-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every NinjaOne release on the [releases page](https://github.com/servosity/msp-skills/releases?q=ninjaone).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install ninjaone@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the NinjaOne Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/ninjaone/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/ninjaone/install.ps1 | iex`. Then authenticate per the README and run `ninjaone-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/ninjaone/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/ninjaone/install.sh)
```

The installer drops both `ninjaone-cli` (the CLI) and `ninjaone-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
ninjaone-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/ninjaone/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/ninjaone/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update ninjaone@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/ninjaone --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/ninjaone --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `ninjaone-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the ninjaone skill from https://github.com/servosity/msp-skills/tree/main/skills/ninjaone. The skill defines how its required CLI (`ninjaone-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your NinjaOne portal):

```bash
NINJAONE_CLIENT_ID=<value> NINJAONE_CLIENT_SECRET=<value> NINJAONE_OAUTH_SCOPE=<value> ninjaone-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which organizations are below 95% patch compliance? | `ninjaone-cli patch-compliance --min-pct 95` |
| Which endpoints across the fleet have no backup at all? | `ninjaone-cli backup-coverage` |
| Which devices is a given threat on, fleet-wide? | `ninjaone-cli av-sweep --threat "Trojan.Generic"` |
| Which devices have AV definitions older than a week? | `ninjaone-cli av-sweep --definition-stale-days 7` |
| What is each organization's overall health score, and why? | `ninjaone-cli fleet-health` |
| Which devices have not checked in for two weeks? | `ninjaone-cli stale-devices --days 14` |
| Which devices are running an end-of-life OS? | `ninjaone-cli os-eol` |
| Where is a software title sprawling across too many versions? | `ninjaone-cli software-audit --min-versions 3` |
| Did patch compliance get better or worse since last week? | `ninjaone-cli drift --metric patch` |
| Search every synced device, org, and alert for a string? | `ninjaone-cli search "disk full"` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most NinjaOne integrations and MCP servers proxy each question into a live API call. That's fine for one device. It dies at scale, when you're asking "which of my 40 client organizations are below 95% patched, and which endpoints across all of them have no backup" - a question the API only answers as per-device rows you have to fetch, cache, and total yourself.

This skill syncs NinjaOne into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `patch-compliance`, `backup-coverage`, `fleet-health`, and `drift` join devices against the organization map, the patch and backup reports, and the previous snapshot - cross-fleet rollups a stateless API wrapper can't do.

## The pain this closes

NinjaOne is one of the most-loved RMMs for real-time, per-device work. The recurring owner complaint is the layer above one device: reporting and the cross-client rollup. Independent reviews note NinjaOne's built-in reports are "not consistently executive-ready" and that client-facing reporting often needs a third-party BI overlay - a gap still open in 2026 - alongside the lack of a single unified multi-tenant view (Flamingo, *NinjaOne Review* and *NinjaOne vs Intune*, 2026).

So the questions you ask every week aren't one click - they're a per-org report run N times and re-totaled by hand. This skill makes them one command:

| The weekly question | One command |
| --- | --- |
| Who's behind on patches? | `ninjaone-cli patch-compliance --min-pct 95` |
| Who has no backup? | `ninjaone-cli backup-coverage` |
| How far did that threat spread? | `ninjaone-cli av-sweep --threat "Trojan.Generic"` |
| How healthy is each client? | `ninjaone-cli fleet-health` |
| Better or worse than last week? | `ninjaone-cli drift --metric patch` |

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The NinjaOne MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your NinjaOne credentials once.

### Is my NinjaOne data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Does it work with the US, EU, and other NinjaOne regions?

Yes. The US host (`https://app.ninjarmm.com`) is the default. For another region, set `NINJAONE_BASE_URL` (for example `https://eu.ninjarmm.com`) and `NINJAONE_TOKEN_URL` (for example `https://eu.ninjarmm.com/ws/oauth/token`), then run `ninjaone-cli doctor` to confirm the credentials reach your instance.

### Will this hit my NinjaOne API rate limits?

The local mirror exists so reads stop hitting the API. After the first `sync`, the fleet views (`patch-compliance`, `backup-coverage`, `av-sweep`, `fleet-health`, `stale-devices`, `os-eol`, `software-audit`, `drift`) run against local SQLite with **zero API calls**. Live calls respect a `--rate-limit` throttle, and `sync` is incremental and resumable - it only fetches what changed and treats resources your token can't reach as warnings, not failures.

### Do I need to be a NinjaOne partner or buy the reporting add-on?

No. You create an API app yourself under **Administration > Apps > API** with an OAuth2 `client_id` and `client_secret`. The fleet rollups are computed locally from whatever your token can already read - no reporting add-on, data warehouse, or partner tier required.

### Can it change things in NinjaOne, or is it read-only?

The headline fleet views are read-only. The CLI also wraps NinjaOne's write surface (for example updating an organization, creating a ticket, running a script, rebooting a device) and a smaller destructive tier (deletes). Preview any write with `--dry-run`, keep a human in the loop, and scope the API token to only what your workflow needs. See [governance.md](./governance.md).

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** | `patch-compliance`, `backup-coverage`, `av-sweep`, `fleet-health`, `stale-devices`, `os-eol`, `software-audit`, `drift`, `search` - reports, rollups, search; change nothing | Allow |
| **Write (routine)** | `organization update`, `ticketing create`, `contact update`, `device reboot`, `device script run-on-device`, `tag set-for-asset` (84 write commands in total) | Preview with `--dry-run`, then a reviewed write |
| **Destructive / config** | `contact delete`, `organization delete-client-document`, `knowledgebase delete-knowledge-base-articles`, `itam delete-unmanaged-device-public-api` (18 destructive commands in total) | Human-in-the-loop only |

The strongest control is the **scope you grant the NinjaOne credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the NinjaOne API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-05._
