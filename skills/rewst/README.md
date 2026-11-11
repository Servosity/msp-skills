# Rewst + AI - for Claude, the ChatGPT desktop app, GitHub Copilot, Gemini CLI, and any agent that speaks MCP

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

Operate and monitor Rewst RPA across every client org from the terminal or your AI agent: workflow execution health, failure triage, automation ROI, dormant-workflow cleanup, cross-org config drift, and integration-pack coverage - the multi-tenant answers the web console makes you assemble one org at a time. Works with the AI you already use - **ChatGPT** (desktop app), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Gemini CLI**. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Rewst skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `rewst-mcp` (no JSON editing). |
| **ChatGPT** (desktop app) | Run installer, then add `rewst-mcp` in **Settings > MCP servers** as a local (STDIO) server. ChatGPT on the web is not supported yet. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `rewst-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

The ChatGPT desktop app runs `rewst-mcp` locally and shares MCP configuration with Codex ([OpenAI](https://learn.chatgpt.com/docs/extend/mcp)). ChatGPT on the web needs a remote server, which msp-skills does not support yet. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: Gemini CLI runs the local binary you just installed, while Microsoft 365 Copilot and the Gemini app need a remote server, which msp-skills does not support yet.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | Not supported by msp-skills yet: it needs a remote server. A [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list. |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. See [mcp-install.md](./mcp-install.md). The **Gemini app** needs a remote server and is not supported by msp-skills yet. |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

> **Claude Desktop bundle:** the `.mcpb` launches on macOS (Intel and Apple Silicon), Windows x64 and Linux x64. Every tool shells out to the companion `rewst-cli`. Releases cut from 2026-09-18 on ship that CLI inside the bundle; older bundles contain only the MCP server, so if yours lacks the companion, run the Path A or Path B installer below first (or set `REWST_CLI_PATH` to an existing binary). Details: [#331](https://github.com/Servosity/msp-skills/issues/331).

[**Download Rewst MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/rewst-v0.1.11/rewst-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Rewst release on the [releases page](https://github.com/servosity/msp-skills/releases?q=rewst).)

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

### Add to Claude Desktop, the ChatGPT desktop app, GitHub Copilot, Gemini CLI, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the ChatGPT desktop app. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

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

`doctor` confirms the credentials work before you run anything that touches data. It exits 0 even when the credential is rejected, so scripts should add `--fail-on error`. The US endpoint (`https://api.rewst.io`) is the default; for EU/AU or another region, also set `REWST_BASE_URL=<your-region-url>`.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Is automation healthy for this client right now? | `rewst-cli health --org <orgId> --since 24h --agent` |
| Which workflows failed for this org overnight? | `rewst-cli failures --org <orgId> --since 12h --agent` |
| Which workflows have gone dormant (no runs in 30 days)? | `rewst-cli dormant --org <orgId> --days 30 --agent` |
| How much time did automation save this client? | `rewst-cli roi --org <orgId> --since 30d --agent` |
| What does one org have that another is missing? | `rewst-cli drift --org <orgId> --against <otherOrgId> --agent` |
| Which managed sub-orgs are missing an integration pack? | `rewst-cli coverage --parent <orgId> --pack microsoft --agent` |
| Which org ids can my token see? | `rewst-cli orgs --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Rewst's only surface is a **GraphQL gateway** - no first-party CLI, no terminal access, no cross-tenant view. Most integrations just forward a query and hand back raw GraphQL nodes. That's fine for one record. It dies when you're asking "is automation healthy across all 40 of my clients" or "which workflows failed overnight, in which tenant".

This skill adds the cross-org rollups Rewst's gateway has no single endpoint for. `orgs`, `health`, `failures`, `dormant`, `roi`, `drift` and `coverage` send real GraphQL queries and aggregate across organizations, workflows, executions, variables and packs live - and your AI sees the answer, not a page of GraphQL JSON. A Rewst answer that is refused, errored or missing a value is reported as a failure, never as empty or zero data.

**Known limitation of this build:** the typed entity commands (`organizations`, `workflows`, `org-variables` and the rest, plus `sync`) are generated as REST-shaped calls the Rewst GraphQL gateway refuses, so they fail with an HTTP 400 or a GraphQL error. Use the seven commands above and `doctor`, which are the ones that reach Rewst today.

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

Yes, in the **ChatGPT desktop app**. It runs MCP servers locally (STDIO), the way `rewst-mcp` runs, and shares MCP configuration with Codex ([OpenAI](https://learn.chatgpt.com/docs/extend/mcp)): add `rewst-mcp` under **Settings > MCP servers** as a STDIO server, or add it once to Codex's `~/.codex/config.toml`. **ChatGPT on the web** is not supported by msp-skills yet: it needs a remote server. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Rewst credentials once.

### Is my Rewst data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Is Rewst's API REST or GraphQL?

GraphQL-only. The rollups (`orgs`, `health`, `failures`, `dormant`, `roi`, `drift`, `coverage`) and `doctor` send real GraphQL queries, so you don't hand-write GraphQL for those questions. The typed entity commands in this build are generated as REST-shaped calls the gateway refuses (see the known limitation above). EU/AU regions set `REWST_BASE_URL`; the US default is `api.rewst.io`.

### Can it build or run workflows?

No. It reads workflow health, failures, time saved, drift and pack coverage; it does not author workflow graphs - use the Rewst designer for that. The typed create and update commands for triggers and workflows are refused by the gateway in this build (see the known limitation above), and the safety tiers below still treat them as human-in-the-loop, since those changes affect automation running on live customer tenants.

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

## Credential safety

- Your Rewst API token goes only to the base URL you configured (`https://api.rewst.io`, or `REWST_BASE_URL` for the EU and AU regions). A redirect to any other host is refused.
- Output, logs, files, the local store and MCP results never repeat the token or a credential Rewst answers with (an API client's secret, a third-party login token, an integration secret in a pack config): the shared credential kit replaces it with a fixed notice.
- Only `credentials.toml` (mode 0600) holds the token; `auth set-token` reads it from stdin.
- Not held yet: the value of an org variable whose category is `secret`. Treat `org-variables` output as sensitive.

## Status

Beta. Validated against the Rewst API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-13._
