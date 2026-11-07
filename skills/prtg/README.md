# PRTG + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the PRTG
> API. Not affiliated with, endorsed by, or sponsored by Paessler GmbH.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/prtg/">
    <img src="../../docs/assets/video/prtg/animated-og.gif" alt="PRTG Network Monitor demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/prtg/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every PRTG API v2 endpoint, plus a fleet view across every client's PRTG core: alarms, status, handoffs, hygiene and safe bulk actions in one command. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the PRTG skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `prtg-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `prtg-mcp` over HTTPS with `PRTG_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `prtg-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `prtg-mcp --transport http` with `PRTG_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `prtg-mcp` over HTTPS with `PRTG_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download PRTG MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/prtg-v0.1.1/prtg-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every PRTG release on the [releases page](https://github.com/servosity/msp-skills/releases?q=prtg).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install prtg@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the PRTG Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/prtg/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/prtg/install.ps1 | iex`. Then authenticate per the README and run `prtg-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/prtg/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/prtg/install.sh)
```

The installer drops both `prtg-cli` (the CLI) and `prtg-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
prtg-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/prtg/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/prtg/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update prtg@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/prtg --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/prtg --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `prtg-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the prtg skill from https://github.com/servosity/msp-skills/tree/main/skills/prtg. The skill defines how its required CLI (`prtg-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Create an API key in each PRTG core under **Account Settings > My Account > API Keys**. The PRTG application server answers API v2 on port 1616 by default.

**One core** - macOS / Linux:

```bash
export PRTG_SERVER=prtg.acme.example:1616
export PRTG_API_KEY=<your-api-key>
prtg-cli doctor
```

**One core** - Windows (PowerShell):

```powershell
$env:PRTG_SERVER = "prtg.acme.example:1616"
$env:PRTG_API_KEY = "<your-api-key>"
prtg-cli doctor
```

**Every client core** (for the `fleet` commands) - register each core once, and put every core's key in one variable, `PRTG_FLEET_KEYS`, as `name=key` pairs separated by commas:

```bash
prtg-cli core add acme --server prtg.acme.example:1616
prtg-cli core add globex --server 10.20.0.5:1616
export PRTG_FLEET_KEYS='acme=<acme-api-key>,globex=<globex-api-key>'
prtg-cli core list
```

```powershell
prtg-cli core add acme --server prtg.acme.example:1616
prtg-cli core add globex --server 10.20.0.5:1616
$env:PRTG_FLEET_KEYS = "acme=<acme-api-key>,globex=<globex-api-key>"
prtg-cli core list
```

`core list` shows `key_set: true` for each core whose key variable is set. A core with a self-signed certificate takes `core add <name> --insecure-tls` (that core only); for a single core use `PRTG_SKIP_TLS_VERIFY=true`. `doctor` checks config, paths, and API reachability, and reports whether credentials are loaded - it does not validate them. Run a read command to confirm the credential actually works end-to-end.

## What this skill does

Register each client's PRTG core once (`prtg-cli core add <name> --server <host[:port]>`, keys in `PRTG_FLEET_KEYS`), then ask across all of them:

| Question your MSP keeps asking | Command |
| --- | --- |
| Which client core is in the worst shape right now? | `prtg-cli fleet status --agent` |
| What is down and unacknowledged across every core, oldest first? | `prtg-cli fleet alarms --status down --unacked --agent` |
| What changed overnight: new alarms and recoveries? | `prtg-cli fleet handoff --since 12h --agent` |
| What has been paused or acknowledged with no end date for over a week? | `prtg-cli fleet silenced --indefinite --older-than 7d --agent` |
| Which cores are behind on version, have a license or maintenance ending, or a disconnected probe? | `prtg-cli fleet drift --within 30d --agent` |
| Acknowledge one outage on every client with the ticket number (preview first) | `prtg-cli fleet ack --filter 'name = "Ping"' --message 'TKT-4411 ISP outage' --duration 2h` |
| Pause a device on every core for patch night (preview first) | `prtg-cli fleet pause --kind device --filter 'name = "fw01"' --duration 2h --message 'Patch night'` |

The fleet commands ask every registered core live, in one run. A core that fails to answer stays in the result as an error row rather than being counted as healthy; the command exits 0 when some cores fail and non-zero only when all of them do. `fleet ack`, `fleet pause` and `fleet resume` only preview until you add `--yes`. Every PRTG API v2 operation is also a typed command you can point at one core with `--core <name>`.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most PRTG integrations talk to one PRTG server with one key. That is fine when you run one core. It stops working when every client has its own core and the question is "what is down for everyone".

This connector is built for one core per client. Each core is registered once with its server and the name of the environment variable that holds its key (the key itself is never written to disk). `fleet alarms`, `fleet status`, `fleet handoff`, `fleet silenced` and `fleet drift` ask every core in one run and return one merged, worst-first result with a `core` column, and `fleet exec` does the same for any read-only API v2 path. `fleet sync` refreshes a separate offline mirror per core, for `search` and `analytics` without a live call per question.

## The pain this closes

Many MSPs give each client its own PRTG core: Paessler's [MSP offering](https://www.paessler.com/msp-offering) says PRTG MSP Edition supports running clients on fully separate instances. Each core then has its own web interface, alarm list and API key, so the start-of-shift question "what is down for every client" means opening each one. Paessler retired its multi-server desktop client, the Enterprise Console, from PRTG 19.4.53 and points users to [PRTG Desktop](https://blog.paessler.com/prtg-enterprise-console-eol-switch-to-prtg-desktop-now) and PRTG MultiBoard, which are viewers for a person at a desk, not something an AI agent can ask. And a pause or acknowledgement with no end date stays silent until someone remembers it.

- `prtg-cli fleet alarms --status down --unacked --agent` - every unacknowledged down sensor on every core, with core, device, priority and how long it has been down.
- `prtg-cli fleet handoff --since 12h --agent` - new alarms and recoveries since the last shift, including sensors that came back after a pause or acknowledgement (`via: after_gap`).
- `prtg-cli fleet silenced --indefinite --older-than 7d --agent` - everything muted with no end date for over a week, with who muted it and why.
- `prtg-cli fleet drift --within 30d --agent` - cores behind your newest PRTG version, licenses and maintenance ending soon, and disconnected probes.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The PRTG MCP server runs locally over stdio by default. Its HTTP mode (`prtg-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `PRTG_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your PRTG credentials once.

### Is my PRTG data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Which PRTG versions and editions does it work with?

It uses PRTG API v2, which the PRTG application server serves on port 1616 by default. Paessler documents API v2 as not available on PRTG clusters, so use it with single-node PRTG Network Monitor cores. The fleet views read the typed list endpoints Paessler marks deprecated in favour of the experimental ones; PRTG still serves them, and they also work on cores older than 24.3. The fleet commands were verified against mock cores, not a live PRTG installation.

### Can an agent pause or acknowledge things on my clients' cores by accident?

`fleet ack`, `fleet pause` and `fleet resume` only preview what they would do on each core until you add `--yes`. Over MCP, every tool that changes PRTG returns that preview until it is called again with `confirm: true`, and `PRTG_MCP_READ_ONLY=1` makes the MCP server refuse every write. Single-core write commands such as `sensors pause sensor` send when you run them, so keep agents to reads and `--dry-run` previews.

### Where do the API keys live?

In environment variables. For one core, set `PRTG_SERVER` and `PRTG_API_KEY`. For many, `core add` records each core's server, and `PRTG_FLEET_KEYS` holds every core's key as `name=key` pairs (`acme=KEY1,globex=KEY2`); the keys are never written to disk, and `core list` shows `key_set` for each core. In Claude Desktop, PRTG Fleet keys is one field in the extension's settings. Create keys in PRTG under **Account Settings > My Account > API Keys** with the lowest access level your workflow needs.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet alarms`, `fleet status`, `fleet handoff`, `fleet silenced`, `fleet drift`, `fleet exec`, `sensors get`, `sync`, `search` | Allow |
| Write (routine) | `fleet ack`, `fleet pause`, `fleet resume`, `sensors acknowledge sensor`, `sensors pause sensor`, `devices multi-scan-now`, `sensors update`, `objects move object`, `import` | Preview first (the fleet commands preview by default; `--dry-run` elsewhere; MCP `confirm`), then a reviewed write |
| Acts on client networks | `devices trigger-autodiscovery`, `groups trigger-autodiscovery`, `autodiscoverytasks start-auto-discovery-task`, `devices metascan` | Human-in-the-loop only |
| Credential / identity | any command that can emit a secret (`session login`, `users api-keys create`, `system license-info`), the `users` API key and password reset commands, `users pause user` / `users resume user` | Human-in-the-loop only |
| Destructive | `sensors delete`, `devices delete`, `groups delete`, `probes delete`, `objects multi-delete`, `autodiscoverytasks delete-auto-discovery-task`, `users delete-apikey` | Human-in-the-loop only, explicit confirmation |

The strongest control is the **scope you grant the PRTG credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the PRTG API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-28._
