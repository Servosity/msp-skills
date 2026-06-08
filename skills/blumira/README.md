# Blumira + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Blumira
> API. Not affiliated with, endorsed by, or sponsored by Blumira, Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/blumira/">
    <img src="../../docs/assets/video/blumira/animated-og.gif" alt="Blumira demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/blumira/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Blumira finding, detection, and agent across your direct org and every MSP sub-account - in one offline-searchable store with cross-account triage and over-time trends no single API call can answer. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Blumira skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `blumira-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `blumira-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `blumira-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Blumira MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `blumira-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Blumira MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/blumira-v0.1.0/blumira-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Blumira release on the [releases page](https://github.com/servosity/msp-skills/releases?q=blumira).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install blumira@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Blumira Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.ps1 | iex`. Then authenticate per the README and run `blumira-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.sh)
```

The installer drops both `blumira-cli` (the CLI) and `blumira-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
blumira-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update blumira@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/blumira --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/blumira --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `blumira-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the blumira skill from https://github.com/servosity/msp-skills/tree/main/skills/blumira. The skill defines how its required CLI (`blumira-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Blumira portal):

```bash
BLUMIRA_API_TOKEN=<value> BLUMIRA_CLIENT_ID=<value> BLUMIRA_CLIENT_SECRET=<value> blumira-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What are the worst open findings across all my client accounts right now? | `blumira-cli triage --status open` |
| What changed since my last sync, new, resolved, or status-changed findings? | `blumira-cli drift` |
| What's my mean-time-to-resolve per account this month? | `blumira-cli velocity --by account --window 30d` |
| Which open findings are about to breach my age-based SLA? | `blumira-cli sla --breach-in 4h` |
| Which detection rules fell out of coverage versus our basis ruleset? | `blumira-cli coverage --against basis` |
| Which domain controllers are stale or unprotected across every account? | `blumira-cli exposure --flag-dc-stale` |
| Which detections keep firing over and over across accounts? | `blumira-cli recurring --window 90d` |
| Which findings mention this IOC, hostname, or user in their evidence? | `blumira-cli evidence-search "<ioc>"` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Blumira integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "how many open critical findings sit across all 30 client accounts right now, and which three accounts own most of them."

This skill syncs Blumira into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `triage`, `coverage`, and `velocity` join findings against detection rules and agent check-ins across every account at once - work a stateless API wrapper can't do.

## The pain this closes

Blumira is sold as detection-and-response that doesn't need a full SOC, and r/msp threads weighing it against the other MDR/SIEM options keep landing on the same operational catch: the product is easy to deploy, but the day-two work is triage, and the partner portal makes you do that triage one account at a time. Switch the active organization, sort the open findings, note the worst, switch again. Across a book of clients there's no single screen that joins findings, detection coverage, and agent health, so "which accounts are behind on coverage" or "which domain controllers went dark this week" becomes a manual sweep you hold in your head.

This skill closes that with cross-account reads off a local mirror:

- **`blumira-cli triage --status open`** - one globally-ranked open-findings queue across every client account.
- **`blumira-cli coverage --against basis`** - detection rules missing or disabled versus your basis ruleset, per account.
- **`blumira-cli exposure --flag-dc-stale`** - domain controllers that went stale or unprotected, surfaced first.
- **`blumira-cli velocity --by account --window 30d`** - mean-time-to-resolve and open-rate per account, so you can see who's drowning.
- **`blumira-cli drift`** - what's new, resolved, or status-changed since your last sync.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Blumira MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Blumira credentials once.

### Is my Blumira data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need a Blumira partner account for the cross-account views?

The cross-account commands (`triage`, `overview`, and coverage across every client) read Blumira's **MSP sub-account API**, so they need partner API credentials with sub-account access. A single-org account still gets every direct-org command, findings, evidence search, and agent and detection rollups, plus offline `sync`. Generate credentials under **Settings > Organization > Generate API Credentials**, then run `blumira-cli auth login`.

### Does this replace the Blumira portal or its automated response?

No. Blumira's portal and its automated detection-and-response playbooks stay exactly where they are. This skill reads the same findings, detections, and agents through Blumira's public API and adds the cross-account views the portal doesn't compose. It only writes when you ask it to (a comment, a resolution, an owner assignment), and those writes are best previewed with `--dry-run` first.

### Will this hit my Blumira API rate limits?

`sync` mirrors each account into local SQLite, so your day-to-day questions run against the mirror, not the live API. You control request pace with `--rate-limit` and `--concurrency` on `sync`, and most analytical commands (`triage`, `coverage`, `velocity`, `drift`) read the local store by default.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** | `triage`, `overview`, `drift`, `velocity`, `sla`, `coverage`, `exposure`, `recurring`, `audit`, `search`, `evidence-search`, `sync` | Allow |
| **Write (routine)** | `msp resolve-finding`, `msp set-finding-owners`, `msp add-account-finding-comment`, `org controller-direct-resolve-finding`, `org controller-direct-set-owners`, `org controller-direct-add-comment` | Preview with `--dry-run`, then a reviewed write |
| **Credential / config** | Credentials live in `auth login` / `auth set-token`; the API exposes no delete or bulk-config command | Human-in-the-loop only |

The strongest control is the **scope you grant the Blumira credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Blumira API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
