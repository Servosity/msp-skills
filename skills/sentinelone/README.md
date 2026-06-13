# SentinelOne + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the SentinelOne
> API. Not affiliated with, endorsed by, or sponsored by SentinelOne, Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/sentinelone/">
    <img src="../../docs/assets/video/sentinelone/animated-og.gif" alt="SentinelOne demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/sentinelone/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every SentinelOne v2.1 management endpoint, plus an offline SQLite store and cross-entity analytics  -  fleet health, threat triage, blast radius, drift  -  that no console view offers. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the SentinelOne skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `sentinelone-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `sentinelone-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `sentinelone-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the SentinelOne MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `sentinelone-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download SentinelOne MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/sentinelone-v0.1.0/sentinelone-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every SentinelOne release on the [releases page](https://github.com/servosity/msp-skills/releases?q=sentinelone).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install sentinelone@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the SentinelOne Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sentinelone/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sentinelone/install.ps1 | iex`. Then authenticate per the README and run `sentinelone-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sentinelone/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sentinelone/install.sh)
```

The installer drops both `sentinelone-cli` (the CLI) and `sentinelone-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
sentinelone-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sentinelone/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sentinelone/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update sentinelone@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/sentinelone --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/sentinelone --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `sentinelone-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the sentinelone skill from https://github.com/servosity/msp-skills/tree/main/skills/sentinelone. The skill defines how its required CLI (`sentinelone-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your SentinelOne management console):

```bash
SENTINELONE_API_TOKEN=<value> sentinelone-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What should I triage first across all my client sites right now? | `sentinelone-cli threats triage` |
| Where did this malicious file spread, and which endpoints are still active? | `sentinelone-cli threats blast-radius "<hash>"` |
| Which endpoints are decaying - offline, out-of-date, infected, or under-protected? | `sentinelone-cli fleet-health stale` |
| Which clients have protection gaps (detect-only, Ranger off, firewall off)? | `sentinelone-cli coverage gaps` |
| What changed across the whole fleet since yesterday? | `sentinelone-cli whatchanged --since 24h` |
| Which threats keep coming back after we mitigated them? | `sentinelone-cli threats recurrence` |
| Are we hitting our mitigation SLA, and where are the breaches? | `sentinelone-cli threats mttr --sla 4` |
| Give me one posture scorecard per client for the QBR deck? | `sentinelone-cli posture` |
| Rank my clients by risk so I know which tenant to call first? | `sentinelone-cli sites risk` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most SentinelOne integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "rank every open threat across all 40 client sites by confidence, severity, and age" or "which endpoints went dark or dropped to detect-only since yesterday" - questions the console only answers one scope at a time, if at all.

This skill syncs SentinelOne into a **local SQLite mirror** with full-text search, snapshotting history on every sync. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `threats triage`, `threats blast-radius`, `fleet-health stale`, and `whatchanged` join across threats, agents, sites, and stored snapshots - cross-site rollups and time-aware diffs a stateless API wrapper can't do.

## The pain this closes

The SentinelOne management console scopes to one Account or Site at a time. Run it across a book of customer sites and every cross-client question - who has the worst open threats, whose agents went dark, who is stuck on an old build, who quietly dropped to detect-only - means flipping the scope selector and re-reading the same screens tenant by tenant. MSPs raise this single-pane gap on r/msp repeatedly: there is no one view that ranks every client's threats or fleet health side by side, and protection erodes silently between the per-site filters nobody has time to check.

This skill closes that gap by mirroring every site locally, then answering the cross-site questions in one call:

| The pain | The command |
| --- | --- |
| No ranked, all-clients triage worklist | `sentinelone-cli threats triage` |
| Can't see one threat's full blast radius | `sentinelone-cli threats blast-radius "<hash>"` |
| Dark / stale / under-protected agents hide per-site | `sentinelone-cli fleet-health stale` |
| Protection gaps (detect-only, Ranger/firewall off) go unnoticed | `sentinelone-cli coverage gaps` |
| No "what changed overnight" across the fleet | `sentinelone-cli whatchanged --since 24h` |
| QBR posture scorecard assembled by hand | `sentinelone-cli posture` |

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The SentinelOne MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your SentinelOne credentials once.

### Is my SentinelOne data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my SentinelOne API rate limits?

The local mirror exists so reads stop hitting the API. After the first `sync`, the cross-site views (`threats triage`, `blast-radius`, `fleet-health`, `coverage gaps`, `posture`, `sites risk`, `whatchanged`) run against local SQLite with **zero API calls**, and live calls respect a `--rate-limit` throttle. The history-aware analytics (`whatchanged`, `threats mttr`, `versions rollout`, `threats verdicts --changed`) need at least two syncs to have something to diff.

### What API token do I need, and how do I scope it?

A SentinelOne API token from your management console (a Service User token is the durable choice; a personal user token works but expires). The token inherits the role of the user that mints it, so **that role is the real permission boundary** - mint a read-scoped token for reporting workflows and reserve write or admin scope for the rare case you actually need it.

### How is this different from Purple AI?

It complements it. Purple AI and the console are best for deep hunting, policy authoring, and the response workflow inside one scope. This skill brings the whole console's **cross-site rollups** - one triage worklist, one fleet-health and posture view, one blast-radius trace across every client at once - to whichever AI agent you already use, offline against the local mirror.

### Does it replace the SentinelOne console?

No. The console stays best for hunting, policy authoring, and the interactive response workflow. This skill adds cross-site queries and scriptable actions to your AI agent so you stop scoping into each site to answer book-wide questions.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** | `threats triage`, `threats blast-radius`, `fleet-health stale`, `coverage gaps`, `posture`, `sites risk`, `whatchanged`, `exclusions audit`, `search` | Allow |
| **Write (routine)** | `agents initiate-scan`, `threats mitigate`, `agents disconnect-from-network`, `agents update-software`, `exclusions create` - writes send immediately; `--dry-run` is an opt-in preview, not a default | Preview with `--dry-run`, then a reviewed write |
| **Destructive / config** | `agents uninstall`, `agents decommission`, `exclusions delete`, `sites delete`, `config-override delete`, `users delete` | Human-in-the-loop only |

The strongest control is the **scope you grant the SentinelOne credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the SentinelOne API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
