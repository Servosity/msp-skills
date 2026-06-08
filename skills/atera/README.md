# Atera + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Atera
> API. Not affiliated with, endorsed by, or sponsored by Atera Networks Ltd.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/atera/">
    <img src="../../docs/assets/video/atera/animated-og.gif" alt="Atera demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/atera/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Atera RMM + PSA endpoint, plus a local SQLite mirror that answers fleet-health, SLA, and book-of-business questions no single API call can. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Atera skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `atera-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `atera-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `atera-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Atera MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `atera-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Atera MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/atera-v0.1.0/atera-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Atera release on the [releases page](https://github.com/servosity/msp-skills/releases?q=atera).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install atera@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Atera Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/atera/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/atera/install.ps1 | iex`. Then authenticate per the README and run `atera-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/atera/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/atera/install.sh)
```

The installer drops both `atera-cli` (the CLI) and `atera-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
atera-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/atera/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/atera/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update atera@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/atera --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/atera --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `atera-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the atera skill from https://github.com/servosity/msp-skills/tree/main/skills/atera. The skill defines how its required CLI (`atera-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Create an API key in your Atera portal under **Admin → API**, then set it and confirm it works:

```bash
ATERA_API_KEY=<your-key> atera-cli doctor
```

`doctor` confirms the credential works and the API is reachable before you run anything that touches data. You can also persist it with `atera-cli auth set-token`.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which agents have gone offline or stopped checking in? | `atera-cli agents stale --days 30` |
| Which open tickets are closest to breaching SLA? | `atera-cli tickets sla` |
| Who is overloaded on the service desk right now? | `atera-cli tickets workload` |
| Which customers have managed agents but no active contract? | `atera-cli customers coverage` |
| What contracts expire in the next 60 days? | `atera-cli contracts expiring --days 60` |
| What's my full book of business by customer and contract mix? | `atera-cli customers book` |
| Where are the chronic machines generating the most alerts? | `atera-cli agents noisy --days 7` |
| What's the patch-compliance picture across the fleet? | `atera-cli agents patch-status` |
| Which machines are running an end-of-life OS? | `atera-cli agents inventory --eol` |
| What changed across agents, tickets, and alerts since yesterday? | `atera-cli since 24h` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Atera integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which customers across all 40 clients have managed agents but no active contract" or "what's about to breach SLA right now."

This skill syncs Atera into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `customers coverage`, `customers book`, and `agents noisy` join across customers, contracts, agents, and alerts - work a stateless API wrapper can't do.

## The pain this closes

Atera's reporting is the consistent gripe in [G2](https://www.g2.com/products/atera/reviews) and [Capterra](https://www.capterra.com/p/144309/Atera/reviews/) reviews: custom reports need workarounds, filtering is rigid, exports are clunky, and the deeper cross-client analytics sit behind higher-tier plans. There's no single screen that answers "which machines went dark," "which tickets breach SLA next," or "which customers are under-contracted" across every client at once - you assemble it by hand, portal tab by portal tab.

This skill closes that gap with cross-client rollups that run offline against the local mirror:

- **`atera-cli agents stale --days 30`** - the machines that quietly stopped reporting, before the client calls.
- **`atera-cli tickets sla`** - open tickets ranked by minutes-to-breach, soonest first.
- **`atera-cli customers coverage`** - accounts you manage but don't bill: managed agents, no active contract.
- **`atera-cli contracts expiring --days 60`** - the renewal calendar Atera never shows as one view.
- **`atera-cli agents patch-status`** - fleet-wide missing-patch rollup the API has no single endpoint for.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Atera MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Atera credentials once.

### Is my Atera data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my Atera API rate limits?

Rarely. Most questions run against the local SQLite mirror after a one-time `sync`, so they make zero API calls. The few commands that fetch live (like `agents patch-status`) are paced under Atera's 700-requests-per-minute limit.

### Do I need to be an Atera partner?

No. You need an Atera account and an API key created under **Admin → API**. Any plan that exposes the API works; nothing here requires a special partner tier.

### Will this replace my Atera portal?

No - it complements it. The portal stays your system of record and remote-access console; this skill adds the cross-client, terminal-and-AI query layer the portal doesn't offer.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `agents stale`, `tickets sla`, `customers coverage`, `contracts expiring`, `since`, `search`, `sync`, every `get-`/rollup command | Allow |
| Write (routine) | `tickets post`/`put`, `contacts post`/`put`, `customers post`/`put`, `contracts post`/`update`, `alerts post`/`resolve`, `devices create-*`, `customvalues set-*`, `import` | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | `agents delete`, `tickets delete`, `customers delete`, `devices delete-*`, and credential changes (`auth set-token`, `auth setup`, `auth logout`) | Human-in-the-loop only |

The strongest control is the **scope you grant the Atera credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Atera API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
