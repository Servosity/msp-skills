# MSPbots + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the MSPbots
> API. Not affiliated with, endorsed by, or sponsored by MSPbots, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/mspbots/">
    <img src="../../docs/assets/video/mspbots/animated-og.gif" alt="MSPbots demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/mspbots/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

The first MSPbots tool we could find anywhere (June 2026)  -  readable filters, alias-named resources, full exports, and the KPI history MSPbots itself doesn't keep. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the MSPbots skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `mspbots-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `mspbots-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `mspbots-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the MSPbots MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `mspbots-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download MSPbots MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/mspbots-v0.1.0/mspbots-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every MSPbots release on the [releases page](https://github.com/servosity/msp-skills/releases?q=mspbots).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install mspbots@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the MSPbots Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mspbots/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mspbots/install.ps1 | iex`. Then authenticate per the README and run `mspbots-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mspbots/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mspbots/install.sh)
```

The installer drops both `mspbots-cli` (the CLI) and `mspbots-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
mspbots-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mspbots/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mspbots/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update mspbots@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/mspbots --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/mspbots --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `mspbots-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the mspbots skill from https://github.com/servosity/msp-skills/tree/main/skills/mspbots. The skill defines how its required CLI (`mspbots-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your MSPbots portal):

```bash
MSPBOTS_API_KEY=<value> mspbots-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Is our open-ticket backlog up or down versus last week? | `mspbots-cli trend open_tickets --agg count` |
| What changed in open tickets since the last snapshot? | `mspbots-cli diff open_tickets` |
| Pull the open tickets updated since June 1 | `mspbots-cli pull open_tickets --where "Update Date >= 2026-06-01"` |
| What columns does this dataset have, and what types? | `mspbots-cli describe open_tickets` |
| Export the entire dataset to CSV for the QBR deck | `mspbots-cli export open_tickets --format csv` |
| Capture today's KPI snapshot (schedule it - history accrues) | `mspbots-cli snapshot open_tickets` |
| Stop pasting 19-digit IDs - name the dataset once | `mspbots-cli registry add open_tickets 1534956341424005122` |
| Are my API key and resource bindings working? | `mspbots-cli doctor` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

There was no MSPbots tool to be different *from* - as of June 2026 we could find no published CLI, SDK, or MCP server for the MSPbots Public API. And a thin wrapper would inherit everything painful about that API: two endpoints, 19-digit resource IDs, comma-encoded filters, no list endpoint, no metadata, no history. It dies the moment you ask "is our backlog up or down versus last week" - the API only knows *now*.

This skill adds the missing layers locally. `registry add` gives resources names instead of snowflake IDs. `pull --where "Update Date >= 2026-06-01"` compiles readable predicates into the wire DSL. `export` walks every page automatically. And `snapshot` + `diff` + `trend` keep timestamped copies in a **local SQLite store** - the KPI history MSPbots itself doesn't keep, computed offline with zero API calls.

## The pain this closes

MSPbots' own Public API documentation (the wiki.mspbots.ai Public API article, archived April 2024) describes the entire programmatic surface: two GET endpoints, resource IDs copied one at a time out of Settings > Public API, filters encoded as comma-rules in query params, rate limits with unspecified numbers, and intermittent HTTP 502s on heavy widget fetches. No history, no enumeration, no tooling - not even a curl gist.

So the ops manager who lives in MSPbots dashboards keeps the week-over-week column by hand: screenshot the widget on Monday, paste the count into a spreadsheet, repeat. The trend dies the week someone forgets.

- `mspbots-cli snapshot open_tickets` - capture a timestamped copy of any dataset or widget into local SQLite. Schedule it and history accrues.
- `mspbots-cli trend open_tickets --agg count` - the week-over-week answer, computed from your snapshots, offline.
- `mspbots-cli diff open_tickets` - row-level added/removed/changed between any two snapshots.
- `mspbots-cli pull open_tickets --where "Update Date >= 2026-06-01"` - readable filters compiled into the comma-encoded DSL, correct the first time.
- `mspbots-cli export open_tickets --format csv` - the full table, auto-paginated, with an honest partial-dump flag when a page cap is hit.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The MSPbots MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your MSPbots credentials once.

### Is my MSPbots data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Can this write to MSPbots?

No. The MSPbots Public API is read-only - datasets and widgets out, nothing in. The only things this skill writes are local: alias registrations, snapshots, and the sync cache in your own SQLite store. No command can change anything in your MSPbots tenant.

### What credentials do I need?

An MSPbots API key: an admin creates it at **Settings > Public API** in the MSPbots app and binds each dataset or widget to it. Set `MSPBOTS_API_KEY` in your environment or run `mspbots-cli auth set-token`. The binding is the permission boundary - the key can only read resources explicitly bound to it, and the global Enable Public API toggle gates everything.

### Why do I register datasets before pulling them?

The Public API has no list endpoint - it cannot tell you what is bound to your key. You copy each resource ID from Settings > Public API once, register it with `mspbots-cli registry add open_tickets <resourceId>`, and every other command accepts the alias from then on.

### Will this hit MSPbots rate limits?

The API documents rate limits without publishing numbers. The CLI ships a `--rate-limit` throttle, `export` bounds its page-walking with `--max-pages` and reports when the cap was hit, and the history questions (`trend`, `diff`) run entirely against local snapshots - zero API calls after capture.

### Does it support widgets as well as datasets?

Yes - `pull`, `export`, `snapshot`, `describe`, and `registry add` all accept `--type widget`. One documented exception inherited from the API: widgets with measure or calculate layers are not supported by the Public API itself.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read (live API) | `mspbots-cli pull`, `export`, `describe`, `dataset`, `widget`, `doctor` | Allow |
| Local-only writes (never touch the tenant) | `mspbots-cli registry add`, `snapshot`, `sync` - all writes land in your local SQLite store; the API has no write endpoint | Allow - safe to schedule |
| Credentials / local config | `mspbots-cli auth set-token`, `auth logout`, `registry rm` | Human-in-the-loop only |

The strongest control is the **scope you grant the MSPbots credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the MSPbots API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-05._
