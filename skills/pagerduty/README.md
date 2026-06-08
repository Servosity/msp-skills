# PagerDuty + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the PagerDuty
> API. Not affiliated with, endorsed by, or sponsored by PagerDuty, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/pagerduty/">
    <img src="../../docs/assets/video/pagerduty/animated-og.gif" alt="PagerDuty demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/pagerduty/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Operate PagerDuty from your AI agent. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the PagerDuty skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `pagerduty-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `pagerduty-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `pagerduty-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the PagerDuty MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `pagerduty-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download PagerDuty MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/pagerduty-v0.1.0/pagerduty-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every PagerDuty release on the [releases page](https://github.com/servosity/msp-skills/releases?q=pagerduty).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install pagerduty@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the PagerDuty Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pagerduty/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pagerduty/install.ps1 | iex`. Then authenticate per the README and run `pagerduty-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pagerduty/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pagerduty/install.sh)
```

The installer drops both `pagerduty-cli` (the CLI) and `pagerduty-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
pagerduty-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pagerduty/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pagerduty/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update pagerduty@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/pagerduty --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/pagerduty --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `pagerduty-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the pagerduty skill from https://github.com/servosity/msp-skills/tree/main/skills/pagerduty. The skill defines how its required CLI (`pagerduty-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

See [mcp-install.md](./mcp-install.md) for the credentials `pagerduty-cli` needs.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What's on fire right now, ranked by SLA risk? | `pagerduty-cli pulse` |
| Who's on call for a service right now, and when's the handoff? | `pagerduty-cli oncall who --service "Payments"` |
| What's our MTTA and MTTR by service this month? | `pagerduty-cli insights mttr --by service --since 30d` |
| Which services have a broken escalation chain or single point of failure? | `pagerduty-cli audit coverage --severity high` |
| Where does a schedule have nobody on call over the next two weeks? | `pagerduty-cli audit schedule-gaps --days 14` |
| Which responders carry the most pages and off-hours load? | `pagerduty-cli insights responders --since 30d` |
| Which services are the noisiest? | `pagerduty-cli insights noisy --top 10 --since 7d` |
| What changed right before this incident broke? | `pagerduty-cli incidents changes <incident-id> --window 4h` |
| Which open incidents are quietly rotting? | `pagerduty-cli insights stale --hours 24` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most PagerDuty integrations and MCP servers proxy each question into a live API call. That's fine for one record. It gets slow and costly at scale, when you're asking "what was our MTTR by service across the whole quarter" or "who carried the most off-hours pages last month" - the cross-incident analytics PagerDuty otherwise gates behind its paid Analytics tier.

This skill syncs PagerDuty into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Commands like `insights mttr`, `insights responders`, and `audit coverage` compute that math locally from the incidents, log entries, services, escalation policies, and schedules any REST API key can read - no Analytics add-on, no live call per question.

## The pain this closes

On-call is where good teams quietly bleed out. Catchpoint's 2025 SRE survey found nearly **70% of responders say on-call stress drives burnout and attrition**, and a 2025 Splunk study tied **73% of outages to alerts that got ignored** - because the average engineer fields ~50 pages a week and only a handful matter. Meanwhile the numbers leadership wants at the QBR (MTTA, MTTR, who's carrying the load) sit behind PagerDuty's paid Analytics tier or a hand-stitched CSV, and the gap in next week's on-call schedule stays invisible until an incident finds it.

This skill turns those into one-line answers:

- `pagerduty-cli pulse` - what's open right now, bucketed by service and sorted by SLA risk.
- `pagerduty-cli insights mttr --by service --since 30d` - MTTA/MTTR per service, computed offline, no Analytics add-on.
- `pagerduty-cli insights responders --since 30d` - who's carrying the pages and the off-hours share, the burnout signal.
- `pagerduty-cli audit coverage --severity high` - services whose escalation chain is broken or a single point of failure.
- `pagerduty-cli audit schedule-gaps --days 14` - future windows where a schedule has nobody on call, before the incident finds the hole.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The PagerDuty MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your PagerDuty credentials once.

### Is my PagerDuty data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my PagerDuty API rate limits?

Rarely. Read questions run against the local SQLite mirror, not the API - you `pagerduty-cli sync` once, then analytics, audits, and search are offline. Only `sync` and live writes call PagerDuty, and the CLI honors a configurable `--rate-limit`.

### Do I need PagerDuty's paid Analytics add-on for MTTR reporting?

No. The skill computes MTTA, MTTR, responder workload, and noisy-service rankings locally from the incidents and log entries any REST API key can read, so you get the post-incident numbers without the paid Analytics tier.

### How is this different from PagerDuty's own AI features?

It complements them. PagerDuty Advance and Analytics live in the web app and largely on paid tiers; this skill puts the same post-incident math in your terminal and your AI agent, computed offline from your synced data. It doesn't replace your PagerDuty account - it reads and acts through your own API key.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `pulse`, `insights mttr`, `audit coverage`, `oncall who`, `search`, `incidents timeline` | Allow |
| Write (routine) | `incidents update` (acknowledge / resolve / reassign), `incidents snooze`, `incidents notes create-incident`, `incidents create` | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | `escalation-policies delete-escalation-policy`, `automation-actions delete`, `business-services delete`, `addons delete` | Human-in-the-loop only |

The strongest control is the **scope you grant the PagerDuty credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the PagerDuty API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
