# Huntress + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Huntress
> API. Not affiliated with, endorsed by, or sponsored by Huntress Labs, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/huntress/">
    <img src="../../docs/assets/video/huntress/animated-og.gif" alt="Huntress demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/huntress/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Huntress endpoint, plus a local SQLite mirror that delivers fleet-wide incident, coverage, and billing rollups the API and official MCP can't. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Huntress skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `huntress-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `huntress-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `huntress-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Huntress MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `huntress-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Huntress MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/huntress-v0.1.0/huntress-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Huntress release on the [releases page](https://github.com/servosity/msp-skills/releases?q=huntress).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install huntress@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Huntress Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/huntress/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/huntress/install.ps1 | iex`. Then authenticate per the README and run `huntress-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/huntress/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/huntress/install.sh)
```

The installer drops both `huntress-cli` (the CLI) and `huntress-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
huntress-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/huntress/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/huntress/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update huntress@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/huntress --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/huntress --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `huntress-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the huntress skill from https://github.com/servosity/msp-skills/tree/main/skills/huntress. The skill defines how its required CLI (`huntress-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Huntress portal):

```bash
HUNTRESS_API_KEY=<value> HUNTRESS_API_SECRET=<value> huntress-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which incidents are oldest across every client org? | `huntress-cli fleet-incidents --sort age` |
| Where are my posture gaps - stale callbacks, disabled Defender or firewall? | `huntress-cli coverage-gaps` |
| Has this IP or file hash touched any of my clients? | `huntress-cli blast-radius --indicator 203.0.113.10` |
| Am I billed for more seats than I have agents deployed? | `huntress-cli billing-reconcile` |
| Which agents went dark in the last week? | `huntress-cli stale-agents --days 7` |
| What is my mean time-to-resolve per client? | `huntress-cli mttr --group-by org` |
| What changed across the fleet since my last shift? | `huntress-cli handoff --since 12h` |
| Give me a QBR scorecard for one client. | `huntress-cli org-scorecard --org 12345` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Huntress integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which incident is oldest across all 40 of my client organizations right now?" or "did this attacker IP touch any other tenant?" - the portal answers one org at a time and the API offers no single query for it.

This skill syncs Huntress into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `fleet-incidents`, `blast-radius`, and `billing-reconcile` join across incident reports, agents, external ports, and invoices from every organization at once - work a stateless API wrapper can't do.

## The pain this closes

On r/msp, the recurring Huntress complaint from multi-tenant shops is that visibility is scoped one organization at a time: there is no native cross-client incident queue, no fleet-wide posture rollup, and no built-in reconciliation of invoiced seats against deployed agents. At 20, 40, 80 client tenants, "which fire do I put out first?" becomes a tab-switching exercise, and billing drift is caught by hand at month-end.

This skill closes that gap:

- **One incident queue for every client** - `huntress-cli fleet-incidents --sort age` age-sorts open incidents across all organizations so the oldest, most urgent one is at the top.
- **Posture gaps, worst-first** - `huntress-cli coverage-gaps` rolls up stale callbacks, disabled Defender, and disabled firewall across the fleet.
- **Fleet-wide blast radius** - `huntress-cli blast-radius --indicator 203.0.113.10` correlates an IP, hash, or hostname across incidents, agents, and external ports in every org.
- **Seat-vs-agent reconciliation** - `huntress-cli billing-reconcile` flags the delta between invoiced seats and agents actually deployed.
- **Shift handoff and QBR** - `huntress-cli handoff --since 12h` and `huntress-cli org-scorecard --org 12345` turn repeated syncs into the history the live API throws away.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Huntress MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Huntress credentials once.

### Is my Huntress data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need to be a Huntress partner?

You need a Huntress account with API credentials - a key and secret generated from your portal. Reseller credentials unlock the cross-account `reseller-rollup`; a single-account credential drives everything else.

### Will this hit my Huntress API rate limits?

`sync` pulls your account into a local mirror once, then every rollup and search runs against that local copy - so repeated questions cost zero API calls. The CLI also honors a configurable `--rate-limit` on the requests it does make.

### Will this replace my Huntress portal?

No - it complements it. The portal stays your console for configuration and deep investigation; this skill answers the cross-tenant and historical questions the portal can only show one organization at a time.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet-incidents`, `coverage-gaps`, `blast-radius`, `billing-reconcile`, `mttr`, `org-scorecard`, `stale-agents`, `search` | Allow |
| Write (routine) | `organizations update-parameters`, `accounts memberships update-parameters`, `unwanted-access-rules update-parameters` | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | `organizations delete-v1-id`, `accounts delete-v1-id`, `unwanted-access-rules delete-v1-id` | Human-in-the-loop only |

The strongest control is the **scope you grant the Huntress credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Huntress API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
