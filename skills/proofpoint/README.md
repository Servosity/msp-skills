# Proofpoint + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Proofpoint
> API. Not affiliated with, endorsed by, or sponsored by Proofpoint, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/proofpoint/">
    <img src="../../docs/assets/video/proofpoint/animated-og.gif" alt="Proofpoint TAP demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/proofpoint/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Proofpoint TAP Threat Insight endpoint plus a local threat store that answers cross-endpoint questions - who is both attacked and clicking, what touched a given user - without re-spending the limited daily API quota. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Proofpoint skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `proofpoint-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `proofpoint-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `proofpoint-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Proofpoint MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `proofpoint-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Proofpoint MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/proofpoint-v0.1.0/proofpoint-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Proofpoint release on the [releases page](https://github.com/servosity/msp-skills/releases?q=proofpoint).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install proofpoint@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Proofpoint Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proofpoint/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proofpoint/install.ps1 | iex`. Then authenticate per the README and run `proofpoint-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proofpoint/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proofpoint/install.sh)
```

The installer drops both `proofpoint-cli` (the CLI) and `proofpoint-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
proofpoint-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proofpoint/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proofpoint/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update proofpoint@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/proofpoint --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/proofpoint --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `proofpoint-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the proofpoint skill from https://github.com/servosity/msp-skills/tree/main/skills/proofpoint. The skill defines how its required CLI (`proofpoint-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Proofpoint portal):

```bash
PROOFPOINT_API_SECRET=<value> PROOFPOINT_SERVICE_PRINCIPAL=<value> proofpoint-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What malicious clicks and messages got through overnight? | `proofpoint-cli backfill --since 12h` |
| Who is both Very Attacked and a top clicker? | `proofpoint-cli risk-overlap --window 30` |
| Give me the full incident brief for a threatId | `proofpoint-cli incident "threat-abc123"` |
| What indicators should I block from this threat? | `proofpoint-cli iocs --threat-id "threat-abc123" --csv` |
| Show me every event that touched one user | `proofpoint-cli user "jane.doe@example.com"` |
| Who are my Very Attacked People this month? | `proofpoint-cli people list-vap --window 30` |
| Which permitted clicks and delivered threats still need a response? | `proofpoint-cli siem list-issues` |
| What threats are inside this campaign? | `proofpoint-cli campaign-threats "campaign-xyz789"` |
| Decode this urldefense-rewritten link | `proofpoint-cli url --urls "https://urldefense.com/v3/..."` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Proofpoint integrations and MCP servers proxy each question into a live API call. That's fine for one lookup. It dies at scale, when you're asking "who is both Very Attacked and clicking" or "every event that touched this user" - and every one of those calls spends against TAP's hard daily quota of 1,800 SIEM requests and 50 campaign-id lookups.

This skill backfills Proofpoint TAP into a **local SQLite mirror** with full-text search. Cross-endpoint questions become one local join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `risk-overlap`, `incident`, and `user` join across SIEM clicks, threat messages, VAP status, clicker status, and forensic evidence - work a stateless API wrapper can't do, and re-queries cost zero additional API calls.

## The pain this closes

On r/msp and the Proofpoint community forums the recurring TAP complaint isn't detection quality - it's the API and the dashboard around it. The Threat Insight SIEM endpoints cap you at 1,800 requests per rolling 24 hours and a single call can't span more than a one-hour window, so reconstructing even a day of clicks and messages means looping the API by hand; the campaign-ids endpoint is throttled to just 50 requests a day. And the dashboard answers one screen at a time, so the questions you actually ask during response - who is both attacked and clicking, every event that touched this user, what's inside this campaign - cross endpoints the console never joins. The pattern is the same one teams describe everywhere: export the feeds, re-pull the same windows, rebuild the correlation in a spreadsheet, burn quota every time.

This skill backfills once, then answers offline:

- `proofpoint-cli backfill --since 12h` - reconstruct overnight clicks and messages in one command, auto-looping the API's 1-hour windows.
- `proofpoint-cli risk-overlap --window 30` - the people who are both Very Attacked and top clickers, attack index beside click count.
- `proofpoint-cli incident "threat-abc123"` - one incident brief from a threatId: severity, attribution, evidence, and every local event that touched it.
- `proofpoint-cli iocs --threat-id "threat-abc123" --csv` - the nested forensic tree flattened into a paste-ready indicator table for an EDR or blocklist.
- `proofpoint-cli user "jane.doe@example.com"` - every click, threat message, VAP status, and clicker status for one person, without re-spending SIEM quota.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Proofpoint MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Proofpoint credentials once.

### Is my Proofpoint data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this blow through my Proofpoint TAP API limits?

No - avoiding that is the point. TAP caps you at 1,800 SIEM requests and 50 campaign-id lookups per rolling 24 hours. The skill backfills once into a local SQLite store, then answers repeat and cross-endpoint questions from that mirror, so re-querying a window or looping over users costs zero additional API calls. Live calls fire only when you ask for fresh data.

### Do I need a special Proofpoint partner API or Essentials admin access?

No. It uses your standard TAP (Targeted Attack Protection) Service Principal and Secret, created under **Settings > Connected Applications** in the TAP dashboard. It reads the Threat Insight endpoints your account already exposes; it does not require Proofpoint Essentials administration or a separate partner program.

### Will this replace my Proofpoint TAP dashboard?

No - it complements it. The dashboard stays your place for configuration and per-threat drill-down. This skill adds the cross-endpoint rollups the portal never exposes - attacked-and-clicking overlap, per-user timelines, one-shot incident briefs - pointed at by your AI agent and answered from your own synced data.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `siem list-issues`, `people list-vap`, `campaign-threats`, `threat`, `incident`, `iocs`, `forensics`, `risk-overlap`, `user`, `url`, `search`, `export`, `sync`, `workflow status` | Allow |
| Write (routine) | `import` (bulk data import via API create/upsert) | Preview with `--dry-run`, then a reviewed write |
| Credential / security | `auth set-token`, `auth logout` (manage the locally stored TAP credential) | Human-in-the-loop only |

The strongest control is the **scope you grant the Proofpoint credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Proofpoint API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-07._
