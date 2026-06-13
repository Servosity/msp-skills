# Microsoft Graph + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Microsoft Graph
> API. Not affiliated with, endorsed by, or sponsored by Microsoft Corporation.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/microsoft-graph/">
    <img src="../../docs/assets/video/microsoft-graph/animated-og.gif" alt="Microsoft Graph demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/microsoft-graph/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

The maintained single-binary successor to the retiring mgc  -  every MSP-relevant Microsoft Graph surface, plus an offline store that finds wasted licenses, privileged-access risks, and stale devices no single API call can. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Microsoft Graph skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `microsoft-graph-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `microsoft-graph-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `microsoft-graph-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Microsoft Graph MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `microsoft-graph-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Microsoft Graph MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/microsoft-graph-v0.1.0/microsoft-graph-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Microsoft Graph release on the [releases page](https://github.com/servosity/msp-skills/releases?q=microsoft-graph).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install microsoft-graph@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Microsoft Graph Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.ps1 | iex`. Then authenticate per the README and run `microsoft-graph-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.sh)
```

The installer drops both `microsoft-graph-cli` (the CLI) and `microsoft-graph-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
microsoft-graph-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update microsoft-graph@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/microsoft-graph --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/microsoft-graph --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `microsoft-graph-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the microsoft-graph skill from https://github.com/servosity/msp-skills/tree/main/skills/microsoft-graph. The skill defines how its required CLI (`microsoft-graph-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Microsoft Graph portal):

```bash
MICROSOFT_GRAPH_TOKEN=<value> microsoft-graph-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which SKUs are we paying for but not fully using, ranked by wasted seats? | `microsoft-graph-cli licenses waste --agent` |
| Which disabled or guest accounts still hold a paid license? | `microsoft-graph-cli licenses orphans --json` |
| Who exactly is consuming one specific SKU before I reclaim seats? | `microsoft-graph-cli licenses map "ENTERPRISEPACK" --agent` |
| Who holds a privileged directory role right now, and which holders are guest or disabled? | `microsoft-graph-cli admins audit --agent` |
| What open security alerts are new since yesterday, by severity and source? | `microsoft-graph-cli security triage --since 24h --agent` |
| Which Intune devices are non-compliant, unencrypted, or stale this month? | `microsoft-graph-cli managed-devices drift --days 30 --agent` |
| Which groups are ownerless, empty, or guest-heavy across the tenant? | `microsoft-graph-cli groups risk --agent` |
| Where does this tenant stand overall - users, license waste, admins, alerts, device drift? | `microsoft-graph-cli tenant snapshot --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Microsoft Graph integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "how much license spend can we reclaim across the tenant" or "who can administer this tenant right now" - and Graph throttles the bulk reads those questions need.

This skill syncs the MSP-relevant Microsoft Graph surface into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `licenses waste`, `admins audit`, and `tenant snapshot` join across users, subscribed SKUs, directory roles, security alerts, and Intune devices - work a stateless API wrapper can't do. And it does it as one cross-platform binary with no .NET or PowerShell runtime - the lightweight successor to the mgc CLI Microsoft retires on August 28, 2026.

## The pain this closes

Microsoft retires the Microsoft Graph CLI (mgc) on **August 28, 2026** - deprecated since September 2025, security fixes only - and steers everyone to the heavier PowerShell SDK ([Microsoft 365 Developer Blog, "Microsoft Graph CLI retirement"](https://devblogs.microsoft.com/microsoft365dev/microsoft-graph-cli-retirement/)). MSPs who scripted tenant reporting on a lightweight cross-platform binary now face a .NET/PowerShell dependency on every machine that runs it. And mgc never answered the cross-entity questions an MSP actually asks - recoverable license spend, who holds admin, which devices are drifting - because no single Graph endpoint returns them; each one becomes a CSV export and a spreadsheet join, per client, every month.

This skill is the lightweight successor that also closes that gap:

| The pain | The command |
| --- | --- |
| "Which licenses are we wasting?" means exporting SKU CSVs and reconciling seats by hand | `microsoft-graph-cli licenses waste --agent` |
| "Who can administer this tenant?" means opening every privileged role in Entra one at a time | `microsoft-graph-cli admins audit --agent` |
| "What's new in Defender since yesterday?" means paging the portal each morning | `microsoft-graph-cli security triage --since 24h --agent` |
| "Which devices are out of compliance?" means a portal-to-spreadsheet ETL every week | `microsoft-graph-cli managed-devices drift --days 30 --agent` |
| "Where does this tenant stand?" has no single screen at all | `microsoft-graph-cli tenant snapshot --agent` |

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Microsoft Graph MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Microsoft Graph credentials once.

### Is my Microsoft Graph data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Is this the replacement for the Microsoft Graph CLI (mgc) that's being retired?

It's built as the lightweight successor for the MSP read-and-report core - directory, licensing, security, and device surfaces - as one cross-platform Go binary with no .NET or PowerShell runtime. Microsoft's own recommended path is the PowerShell SDK; this is the option for teams who want a scriptable single binary and their AI agent instead. It is not affiliated with or endorsed by Microsoft.

### Will this hit my Microsoft Graph throttling limits?

The local SQLite mirror exists so reads stop hitting Graph. After the first `pull`, the cross-entity views (`licenses waste`/`orphans`/`map`, `admins audit`, `security triage`, `managed-devices drift`, `groups risk`, `tenant snapshot`) run against local SQLite with **zero API calls**. Live calls follow `@odata.nextLink` and respect a `--rate-limit` throttle, and `pull` treats resources your token can't reach as warnings, not failures.

### Does it use a delegated or app-only token?

Either. Run `microsoft-graph-cli auth login --tenant <id> --client-id <id> --client-secret <secret>` to mint and cache an app-only (client-credentials) token for unattended MSP use, or export a pre-minted token as `MICROSOFT_GRAPH_TOKEN`. Read scopes such as `Directory.Read.All`, `RoleManagement.Read.Directory`, `SecurityAlert.Read.All`, and `DeviceManagementManagedDevices.Read.All` must be granted and admin-consented. App-only tokens have no `/me`, so `users me` is delegated-only.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `licenses waste`, `admins audit`, `security triage`, `managed-devices drift`, `groups risk`, `tenant snapshot`, `users list`, `pull`, `search`, `export` | Allow |
| Write (import only) | `import <resource> --input data.jsonl` - the sole write path; one POST per JSONL record | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | None - the CLI exposes no delete or update path | Human-in-the-loop only |

Every typed command is read-only; the one create path is `import`, which previews with `--dry-run`. The strongest control is the **scope you grant the Microsoft Graph credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Microsoft Graph API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-05._
