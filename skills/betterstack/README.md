# Better Stack + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Better Stack
> API. Not affiliated with, endorsed by, or sponsored by Better Stack.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/betterstack/">
    <img src="../../docs/assets/video/betterstack/animated-og.gif" alt="Better Stack demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/betterstack/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Better Stack Uptime feature, plus an offline SQLite mirror and cross-resource fleet analytics  -  what's down and who's paged, coverage gaps, MTTA/MTTR, flapping, on-call gaps, and status-page drift  -  that the API alone can't answer. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Better Stack skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `betterstack-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `betterstack-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `betterstack-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Better Stack MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `betterstack-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Better Stack MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/betterstack-v0.1.0/betterstack-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Better Stack release on the [releases page](https://github.com/servosity/msp-skills/releases?q=betterstack).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install betterstack@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Better Stack Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.ps1 | iex`. Then authenticate per the README and run `betterstack-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.sh)
```

The installer drops both `betterstack-cli` (the CLI) and `betterstack-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
betterstack-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update betterstack@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/betterstack --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/betterstack --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `betterstack-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the betterstack skill from https://github.com/servosity/msp-skills/tree/main/skills/betterstack. The skill defines how its required CLI (`betterstack-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Better Stack portal):

```bash
BETTERSTACK_API_TOKEN=<value> betterstack-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What's down right now and is anyone actually paged? | `betterstack-cli down` |
| Which monitors would page nobody if they failed? | `betterstack-cli coverage` |
| What's our MTTA and MTTR over the last 30 days, by monitor? | `betterstack-cli mttr --days 30 --by-monitor --top 10` |
| Which monitors are the noisiest over the last week? | `betterstack-cli flapping --days 7 --top 10` |
| Is anyone actually on call right now, or is there a gap? | `betterstack-cli oncall-gaps` |
| Which heartbeats are most at risk of a silent miss? | `betterstack-cli heartbeat-risk --top 10` |
| Are any status pages green while a monitor has an open incident? | `betterstack-cli statuspage-audit` |
| How healthy is each client group right now? | `betterstack-cli group-health` |
| Give me one health board for the whole account. | `betterstack-cli fleet` |
| Which open incidents are oldest and still unacknowledged? | `betterstack-cli triage` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Better Stack integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 200 monitors across every client would page nobody if they failed?" or "what was our real MTTA/MTTR last quarter, by monitor?".

This skill syncs Better Stack into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `coverage`, `mttr`, and `statuspage-audit` join across monitors, heartbeats, incidents, escalation policies, on-call calendars, and status pages - work a stateless API wrapper can't do.

## The pain this closes

Monitoring sprawl and alert fatigue are a standing complaint in MSP communities - r/msp and MSPGeek threads on "monitoring noise," "alert fatigue," and "who's actually getting paged" recur constantly. Across dozens of client accounts:

- A monitor with no escalation policy goes down at 2am and pages nobody. You find out when the client calls. `betterstack-cli coverage` lists every monitor that would page nobody if it failed.
- Flapping monitors wake the on-call tech night after night until alerts get ignored. `betterstack-cli flapping --days 7 --top 10` ranks the noisiest sources.
- A rotation has nobody on call right now. `betterstack-cli oncall-gaps` flags it.
- Your status page reads "operational" while a backing monitor has an open incident. `betterstack-cli statuspage-audit` catches the drift.
- QBR time, and you need real numbers. `betterstack-cli mttr --days 30 --by-monitor` computes MTTA/MTTR from the local mirror.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Better Stack MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Better Stack credentials once.

### Is my Better Stack data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my Better Stack API rate limits?

No. The skill syncs once into a local SQLite mirror, then answers from local data, so repeated questions never touch the API. Only `sync`, live writes, and the status-page resource fan-out (used by `statuspage-audit`) call Better Stack.

### Do I need a paid Better Stack plan?

You need a Better Stack account with an API token. The analytics run against whatever monitors, heartbeats, incidents, on-call calendars, and status pages your plan includes - the skill reads what your token can see.

### Will this replace the Better Stack portal?

No, it complements it. The portal is still where you configure monitors and watch live. This skill answers the cross-account questions the portal makes you click through - coverage gaps, MTTA/MTTR, on-call gaps, status-page drift - from your AI.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet`, `down`, `coverage`, `mttr`, `flapping`, `oncall-gaps`, `heartbeat-risk`, `statuspage-audit`, `group-health`, `triage`, `search`, every `list` / `get` | Allow |
| Write (routine) | `monitors create/update`, `heartbeats create/update`, `monitor-groups create`, `heartbeat-groups create`, `policies create`, `status-pages create`, `status-page-sections create`, `incidents acknowledge/resolve`, `import` | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | `monitors delete`, `heartbeats delete`, `incidents delete`, `policies delete`, `status-pages delete`, `status-page-sections delete`, `status-page-resources delete`, `monitor-groups delete`, `heartbeat-groups delete` | Human-in-the-loop only |

The strongest control is the **scope you grant the Better Stack credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Better Stack API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-07._
