# runZero + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the runZero
> API. Not affiliated with, endorsed by, or sponsored by runZero, Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/runzero/">
    <img src="../../docs/assets/video/runzero/animated-og.gif" alt="runZero demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/runzero/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every runZero query, plus a local SQLite copy of your whole attack surface that diffs over time, joins assets to vulnerabilities offline, and costs zero API quota to re-slice. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the runZero skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `runzero-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `runzero-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `runzero-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the runZero MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `runzero-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download runZero MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/runzero-v0.1.0/runzero-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every runZero release on the [releases page](https://github.com/servosity/msp-skills/releases?q=runzero).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install runzero@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the runZero Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/runzero/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/runzero/install.ps1 | iex`. Then authenticate per the README and run `runzero-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/runzero/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/runzero/install.sh)
```

The installer drops both `runzero-cli` (the CLI) and `runzero-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
runzero-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/runzero/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/runzero/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update runzero@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/runzero --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/runzero --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `runzero-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the runzero skill from https://github.com/servosity/msp-skills/tree/main/skills/runzero. The skill defines how its required CLI (`runzero-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your runZero portal):

```bash
RUNZERO_API_KEY=<value> runzero-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which of our assets are most exposed right now? | `runzero-cli triage --agent` |
| Only the internet-facing ones? | `runzero-cli triage --internet-facing --agent` |
| What changed on our attack surface since last week? | `runzero-cli diff --since 7d` |
| What newly became exposed or vulnerable since the last sync? | `runzero-cli exposure-delta --agent` |
| Which of our assets are affected by a given CVE? | `runzero-cli affected "CVE-2024-3094" --agent` |
| Where are risky services concentrated in a subnet? | `runzero-cli exposure-map "10.0.0.0/8" --agent` |
| Which TLS certificates are expiring soon or using weak crypto? | `runzero-cli certs-expiring --days 90 --weak` |
| Which assets are stale, end-of-life, or unowned? | `runzero-cli stale --days 90 --json` |
| How many assets run a given software product, by version? | `runzero-cli software rollup "openssl" --agent` |

Everything above the line reads the local SQLite copy - run `runzero-cli inventory sync` once first, twice if you want `diff` and `exposure-delta` to have two snapshots to compare.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most runZero integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which critical assets are running a vulnerable service" - because runZero's API is scoped per entity (`/org`, `/account`, `/export`), so that one question is three calls and a manual join.

This skill syncs runZero into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `triage`, `affected`, and `exposure-delta` join across assets, services, and vulnerabilities in a single pass - work a stateless API wrapper can't do. And because every `inventory sync` is a snapshot, `diff` and `exposure-delta` compare two points in time, which a live wrapper cannot do at all.

## The pain this closes

"You can't secure what you can't see" is a refrain on r/msp for a reason: asset inventory drifts, shadow IT shows up unannounced, and the questions that matter are never about one asset. They're cross-entity - *which* critical machines run *this* vulnerable service, *what* newly opened a port since last week, *which* hosts a fresh CVE actually lands on. runZero discovers all of it, but its API hands you one entity at a time, so the answer lives in a spreadsheet where three exports get joined by hand.

This skill closes that gap by syncing the whole surface locally and answering the cross-entity question directly:

- **Rank what's exposed** - `runzero-cli triage --agent` joins criticality, services, and vulnerabilities in one pass.
- **See what changed** - `runzero-cli diff --since 7d` and `runzero-cli exposure-delta --agent` show drift and newly-exposed services between syncs.
- **Trace a CVE's blast radius** - `runzero-cli affected "CVE-2024-3094" --agent` lists every affected asset with its services and criticality.
- **Clean up the hygiene tail** - `runzero-cli stale --days 90 --json` buckets unseen, EOL, and unowned assets, emitting IDs ready to pipe into the bulk commands.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The runZero MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your runZero credentials once.

### Is my runZero data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Does this burn my runZero API quota?

Only `inventory sync` and live queries call the API. The cross-entity analysis - `triage`, `diff`, `affected`, `exposure-map`, `exposure-delta`, `certs-expiring`, `software rollup` - runs entirely against the local SQLite copy, so re-slicing your attack surface a hundred ways costs zero additional API calls.

### Does it work with self-hosted runZero?

Yes. It defaults to the hosted console at `console.runzero.com`; point it at your own console with `RUNZERO_BASE_URL`. The same API-token scopes apply.

### What token scope do I need?

A read/Export token (Export ET, Organization OT, or Account CT key) covers `inventory sync` and every analysis command. Launching a scan with `scan-watch` or `org create-scan` needs a token with scan permission. Scope the credential to only what your workflow uses.

### Will this replace the runZero console?

No. The console and API stay the source of truth for discovery. This skill adds the offline cross-entity layer the API can't return in one call - point-in-time diffs, CVE-to-asset blast radius, and exposure ranking - so your AI answers a security question in one step.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** | `triage`, `diff`, `affected`, `exposure-delta`, `exposure-map`, `stale`, `certs-expiring`, `software rollup`, `search`, `inventory list` / `status`, and the non-secret `account` / `org` `get-*` reports (assets, sites, services, tasks, organizations, users) - not the credential/token/key reads below | Allow |
| **Write (routine)** | `inventory sync` (writes the local copy), `org create-site`, `org create-scan` and `scan-watch` (these launch a real network scan against your targets), `import` / `runzero-import`, and asset tag/owner updates | Preview with `--dry-run`, then a reviewed write |
| **Credential / destructive / config** | the secret-returning reads (`account get-apitoken` which mints a token, `get-credential` / `get-credentials`, `get-key` / `get-keys`, `get-organization-export-token` / `-tokens`), all credential and key writes (`create-credential`, `create-key`, `rotate-key`, `reset-user-password` / `reset-user-mfa`), and every `delete-*` / `remove-*` plus the `org` bulk-clear operations | Human-in-the-loop only |

The strongest control is the **scope you grant the runZero credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the runZero API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
