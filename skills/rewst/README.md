# Rewst + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Rewst
> API. Not affiliated with, endorsed by, or sponsored by Rewst, Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/rewst/">
    <img src="../../docs/assets/video/rewst/animated-og.gif" alt="Rewst demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/rewst/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Operate and monitor Rewst RPA across every client org from the terminal or your AI agent: workflow execution health, failure triage, automation ROI, dormant-workflow cleanup, cross-org config drift, and integration-pack coverage - the multi-tenant answers the web console makes you assemble one org at a time. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Rewst skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `rewst-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `rewst-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `rewst-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Rewst MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `rewst-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Rewst MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/rewst-v0.1.0/rewst-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Rewst release on the [releases page](https://github.com/servosity/msp-skills/releases?q=rewst).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install rewst@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Rewst Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.ps1 | iex`. Then authenticate per the README and run `rewst-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.sh)
```

The installer drops both `rewst-cli` (the CLI) and `rewst-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
rewst-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rewst/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update rewst@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/rewst --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/rewst --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `rewst-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the rewst skill from https://github.com/servosity/msp-skills/tree/main/skills/rewst. The skill defines how its required CLI (`rewst-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the API token the CLI needs (from an API client you create in the Rewst console):

```bash
REWST_API_TOKEN=<value> rewst-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data. The US endpoint (`https://api.rewst.io`) is the default; for EU/AU or another region, also set `REWST_BASE_URL=<your-region-url>`.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Is automation healthy for this client right now? | `rewst-cli health --org <orgId> --since 24h --agent` |
| Which workflows failed for this org overnight? | `rewst-cli failures --org <orgId> --since 12h --agent` |
| Which workflows have gone dormant (no runs in 30 days)? | `rewst-cli dormant --org <orgId> --days 30 --agent` |
| How much time did automation save this client? | `rewst-cli roi --org <orgId> --since 30d --agent` |
| What does one org have that another is missing? | `rewst-cli drift --org <orgId> --against <otherOrgId> --agent` |
| Which managed sub-orgs are missing an integration pack? | `rewst-cli coverage --parent <orgId> --pack microsoft --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Rewst's only surface is a **GraphQL gateway** - no first-party CLI, no terminal access, no cross-tenant view. Most integrations just forward a query and hand back raw GraphQL nodes. That's fine for one record. It dies when you're asking "is automation healthy across all 40 of my clients" or "which workflows failed overnight, in which tenant".

This skill turns the whole schema into typed commands, syncs the syncable entities into a **local SQLite mirror** for offline search, and adds the cross-org rollups Rewst's gateway has no single endpoint for. Compound commands like `health`, `failures`, `roi`, `drift`, and `coverage` aggregate across organizations, workflows, executions, variables, and packs live - and your AI sees the answer, not a page of GraphQL JSON.

## The pain this closes

Rewst is the automation layer a lot of MSPs now run service delivery on - and its only surface is a GraphQL gateway and a web app. On r/msp and in the Rewst community, the recurring theme is operational blindness at scale: an automation that quietly stopped firing, a workflow that's been failing for a week in one client and nobody noticed, a pack that reached 18 of 20 tenants. The data exists, but answering "is automation healthy for this client?" means opening the web app and reading execution history org by org. For an MSP, Rewst automations ARE the service - a failed workflow is a ticket that didn't get created.

This skill answers those questions directly:

- `health --org <id>` - succeeded/failed/running counts plus time saved, with an unhealthy verdict when failures are present
- `failures --org <id> --since 12h` - the recent failed runs, newest first - the triage queue, not every execution
- `dormant --org <id> --days 30` - workflows that stopped running after a trigger or integration broke
- `roi --org <id> --since 30d` - humanSecondsSaved aggregated into hours saved, for a QBR
- `drift --org <id> --against <id>` - what one tenant has that another is missing (variables, packs)

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Rewst MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Rewst credentials once.

### Is my Rewst data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Is Rewst's API REST or GraphQL?

GraphQL-only. This CLI maps the schema to typed commands and includes an `api` command that exposes every endpoint by interface name for full schema coverage, so you don't hand-write GraphQL for common operations. EU/AU regions set `REWST_BASE_URL`; the US default is `api.rewst.io`.

### Can it build or run workflows?

It reads and manages Rewst entities (workflows, triggers, variables, packs); it does not visually author workflow graphs - use the Rewst designer for that. Creating or updating triggers and workflows is gated human-in-the-loop, since those changes affect automation running on live customer tenants.

### Does it work across all my client organizations?

Yes. The rollups (`health`, `failures`, `drift`, `coverage`) are multi-org by design and take an `--org` or `--parent` id; `coverage` scans a parent org's managed sub-orgs.

### Do I need a special Rewst plan?

You need a Rewst API token from an API client you create in the console. The skill authenticates as that client and adds nothing to your Rewst account.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

Rewst's API is GraphQL: reads are queries, writes are mutations. The nuance for an automation platform - some writes don't just change a record, they arm or alter automation that runs on live customer tenants.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `health`, `failures`, `dormant`, `roi`, `drift`, `coverage`, all `get`/`list`/`search`, local `sync` | Allow |
| Write (config) | `crates create/update`, `component-instances create/update`, `actions create` | Preview with `--dry-run`, then a reviewed write |
| Automation and trigger changes | `workflows create/update`, `triggers create/update`, `packs`/`pack-configs create/update`, `org-variables create/update` | Human-in-the-loop, explicit confirmation |
| Destructive | `delete-org-interpreter-responses`, `pack-delete-responses` | Human-in-the-loop only |
| Admin and identity | `organizations create/update`, `users create/update`, `user-invites create` | Operator-only |
| Credential | `auth set-token` | Human-in-the-loop only |

The strongest control is the **scope you grant the Rewst API client** - the CLI can only do what the token is permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Rewst API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-13._
