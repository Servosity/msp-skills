# Action1 + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Action1
> API. Not affiliated with, endorsed by, or sponsored by Action1 Corporation.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/action1/">
    <img src="../../docs/assets/video/action1/animated-og.gif" alt="Action1 demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/action1/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Action1 endpoint, plus fleet-wide patch and vulnerability views across all your organizations. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Action1 skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `action1-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `action1-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `action1-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Action1 MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `action1-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Action1 MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/action1-v0.1.0/action1-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Action1 release on the [releases page](https://github.com/servosity/msp-skills/releases?q=action1).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install action1@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Action1 Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.ps1 | iex`. Then authenticate per the README and run `action1-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.sh)
```

The installer drops both `action1-cli` (the CLI) and `action1-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
action1-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update action1@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/action1 --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/action1 --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `action1-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the action1 skill from https://github.com/servosity/msp-skills/tree/main/skills/action1. The skill defines how its required CLI (`action1-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Action1 portal):

```bash
ACTION1_CLIENT_ID=<value> ACTION1_CLIENT_SECRET=<value> ACTION1_REGION=<value> action1-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which endpoints across all clients are missing the most patches? | `action1-cli fleet patch-posture` |
| Which CVEs hit the most machines, severity- and KEV-weighted? | `action1-cli fleet vuln-triage --kev-only` |
| Which agents have stopped checking in across the fleet? | `action1-cli fleet stale --days 14` |
| What is the patch-and-vulnerability posture per client organization? | `action1-cli fleet org-scorecard` |
| Which endpoints are waiting on a reboot to finish a patch cycle? | `action1-cli fleet reboot-pending` |
| Rank every endpoint by overall risk (updates, CVEs, reboot, staleness) | `action1-cli fleet health-score` |
| What software is installed across the whole fleet, deduped by version? | `action1-cli fleet software-rollup` |
| List the managed endpoints in one client organization | `action1-cli endpoints managed <orgId>` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Action1 integrations and MCP servers proxy each question into a live API call, one organization at a time. That's fine for one record. It dies at scale, when you're asking "which endpoint across all 40 of my clients is most behind on patches, and which CVE is on the most machines this quarter" - questions the org-siloed API cannot answer in a single call.

This skill syncs every organization into a **local SQLite mirror** with full-text search. Aggregate questions become one local query: instant, offline, and the AI sees the answer, not the raw data. Cross-org rollups like `fleet patch-posture`, `fleet vuln-triage`, and `fleet org-scorecard` join across every organization's endpoints, updates, and vulnerabilities at once - work a stateless API wrapper can't do.

## The pain this closes

Patch-management threads on r/msp keep surfacing the same gap with multi-tenant patch tools: the console is organized one client organization at a time, but the questions an MSP owner has to answer are fleet-wide - "which endpoints are most behind?", "which CVE is everywhere?", "what posture number goes in this client's QBR?" In Action1 that means switching organizations one by one, or exporting per-org and merging spreadsheets. The data is there per client; it just doesn't roll up across every organization in one view.

This skill closes that gap:

- `action1-cli fleet patch-posture` - every endpoint, all orgs, ranked by missing updates.
- `action1-cli fleet vuln-triage --kev-only` - CVEs ranked by blast radius (CVSS + CISA KEV).
- `action1-cli fleet org-scorecard` - one posture row per client organization, the QBR number in a line.
- `action1-cli fleet stale --days 14` - dark agents across every org, before they fall out of coverage.
- `action1-cli fleet reboot-pending` - the fleet-wide action queue that closes out a patch cycle.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Action1 MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Action1 credentials once.

### Is my Action1 data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Which Action1 region and credentials do I need?

An API client (Client ID + Client Secret) from your Action1 console's API Credentials page, plus `ACTION1_REGION` set to `us`, `eu`, or `au` to match your console URL. The CLI POSTs them to `/oauth2/token` and mints and refreshes the bearer token for you. Scope the client to read-only permission templates if you only need reporting - see [governance.md](./governance.md).

### Does this replace the Action1 console?

No. The console stays your place to configure automations and approve patches interactively. This skill answers the cross-organization questions the console shows one org at a time - fleet patch posture, CVE blast-radius, per-client scorecards - and lets your AI agent run them from the terminal.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet patch-posture`, `fleet vuln-triage`, `endpoints managed <orgId>`, `search`, `export`, `doctor` | Allow |
| Write (routine) | `endpoints groups`, `endpoints managed-id-patch`, `settings org-id-post`, `automations policies-schedules-org-id-post`, `import` | Preview with `--dry-run`, then a reviewed write |
| Endpoint / patch execution | `automations policies-instances-org-id-post` (run an automation), `updates approvals updates-org-id`, `endpoints managed-id-remote-sessions-post`, `scripts org-id-post` | Human-in-the-loop; never unattended |
| Credential / security | `oauth2` (mints a token), `users post`, `roles post`, `roles id-patch` | Human-in-the-loop only |
| Destructive / account | `endpoints managed-id-delete`, `organizations org-id-delete`, `users id-delete`, `enterprise request-closure` | Human-in-the-loop only, explicit confirmation |

The strongest control is the **scope you grant the Action1 credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Action1 API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
