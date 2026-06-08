# Rootly + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Rootly
> API. Not affiliated with, endorsed by, or sponsored by Rootly, Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/rootly/">
    <img src="../../docs/assets/video/rootly/animated-og.gif" alt="Rootly demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/rootly/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Rootly incident, alert, and on-call object as a typed command  -  plus a local SQLite mirror that answers related-incident, MTTR, coverage-gap, and on-call questions offline and instantly. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Rootly skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `rootly-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `rootly-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `rootly-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Rootly MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `rootly-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Rootly MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/rootly-v0.1.0/rootly-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Rootly release on the [releases page](https://github.com/servosity/msp-skills/releases?q=rootly).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install rootly@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Rootly Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rootly/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rootly/install.ps1 | iex`. Then authenticate per the README and run `rootly-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rootly/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rootly/install.sh)
```

The installer drops both `rootly-cli` (the CLI) and `rootly-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
rootly-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rootly/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rootly/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update rootly@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/rootly --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/rootly --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `rootly-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the rootly skill from https://github.com/servosity/msp-skills/tree/main/skills/rootly. The skill defines how its required CLI (`rootly-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Rootly portal):

```bash
ROOTLY_API_KEY=<value> ROOTLY_API_TOKEN=<value> rootly-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Who's on call right now across every service and schedule? | `rootly-cli oncall-now` |
| What past incidents are most similar to this one? | `rootly-cli related <incident-id>` |
| What actually fixed this service the last time it broke? | `rootly-cli fixed-last-time <service>` |
| What's our MTTA and MTTR by service this quarter? | `rootly-cli mttr --by service --since 90d` |
| Where does an on-call schedule have an unstaffed gap? | `rootly-cli coverage-gaps --days 14` |
| Is it safe to deploy this service right now? | `rootly-cli deploy-guard <service>` |
| Which incidents are breaching or about to breach SLA? | `rootly-cli sla-breach --within 2h` |
| Which open action items are overdue, grouped by owner? | `rootly-cli action-items-overdue --group-by owner` |
| Draft a paste-ready post-mortem skeleton for this incident. | `rootly-cli postmortem-skeleton <incident-id>` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Rootly integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "what's our MTTR by service across the quarter?" or "which past incidents are most similar to this one?" - questions that otherwise mean a call per object or Rootly's own analytics surface.

This skill syncs Rootly into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `related`, `mttr`, and `coverage-gaps` join across incidents, services, schedules, and on-call shifts - work a stateless API wrapper can't do.

## The pain this closes

On-call is where good teams quietly bleed out. Recurring on-call burnout and alert-fatigue threads on r/sysadmin and r/sre - and the industry's annual Catchpoint SRE Report - tell the same story: responders field a flood of pages, only a handful matter, and the real incident hides in the noise. Then at the reliability review, the numbers leadership wants - MTTA, MTTR, who's carrying the on-call load - get hand-stitched from exports. And nobody notices the gap in next week's schedule until an incident finds it first.

For an MSP running incident response across many client services, that's three open loops at once: too much noise to triage, no cheap way to report on it, and no early warning when coverage lapses. This skill closes them from the terminal:

- **`rootly-cli oncall-now`** - who's on call right now across every schedule and service, escalation tier included.
- **`rootly-cli mttr --by service --since 90d`** - MTTA and MTTR per service, computed offline from synced incidents. Review numbers without the portal expedition.
- **`rootly-cli coverage-gaps --days 14`** - future windows where a schedule has nobody on call, before a page is missed.
- **`rootly-cli related <incident-id>`** and **`rootly-cli fixed-last-time <service>`** - surface how this class of problem played out before and what actually resolved it.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Rootly MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Rootly credentials once.

### Is my Rootly data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my Rootly API rate limits?

Rarely. Read questions run against the local SQLite mirror, not the API - you `sync` once, then analytics, search, and the on-call views are offline. Only `sync` and live writes call Rootly.

### How is this different from Rootly's own AI and analytics?

Rootly's AI and analytics live in the web app. This skill puts the post-incident math - MTTA/MTTR, incident similarity, resolution mining, on-call coverage and escalation-gap checks - in your terminal and your AI agent, computed offline from data any API key can read. It complements the Rootly portal; it does not replace your account.

### Do I need to be a Rootly customer?

Yes - you authenticate with your own Rootly API key against your own account. The skill is an unofficial, community-built connector; it is not affiliated with or endorsed by Rootly.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `oncall-now`, `mttr`, `related`, `coverage-gaps`, `sla-breach`, `service-health`, `search`, and any non-secret `list`/`get` (not `secrets`/`api-keys`, which return stored credentials - see below) | Allow |
| Write (routine) | `incidents create`, `incidents update`, `incidents resolve`, `schedules update` | Preview with `--dry-run`, then a reviewed write |
| Credential / security | `secrets`, `api-keys rotate` | Human-in-the-loop only |
| Destructive / config | `incidents delete`, `schedules delete` | Human-in-the-loop only |

The strongest control is the **scope you grant the Rootly credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Rootly API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
