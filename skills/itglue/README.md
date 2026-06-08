# IT Glue + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the IT Glue
> API. Not affiliated with, endorsed by, or sponsored by Kaseya US LLC.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/itglue/">
    <img src="../../docs/assets/video/itglue/animated-og.gif" alt="IT Glue demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/itglue/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every IT Glue resource, plus an offline SQLite mirror, fleet-wide cross-resource search, and documentation-hygiene analytics no other IT Glue tool offers. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the IT Glue skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `itglue-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `itglue-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `itglue-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the IT Glue MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `itglue-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download IT Glue MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/itglue-v0.1.0/itglue-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every IT Glue release on the [releases page](https://github.com/servosity/msp-skills/releases?q=itglue).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install itglue@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the IT Glue Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/itglue/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/itglue/install.ps1 | iex`. Then authenticate per the README and run `itglue-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/itglue/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/itglue/install.sh)
```

The installer drops both `itglue-cli` (the CLI) and `itglue-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
itglue-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/itglue/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/itglue/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update itglue@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/itglue --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/itglue --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `itglue-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the itglue skill from https://github.com/servosity/msp-skills/tree/main/skills/itglue. The skill defines how its required CLI (`itglue-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your IT Glue portal):

```bash
ITGLUE_API_KEY=<value> itglue-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which client owns this device, serial number, or contact? | `itglue-cli search "Fortinet"` |
| Which clients are under-documented, thinnest first? | `itglue-cli coverage --below 1` |
| Which credentials haven't been rotated in a year, by client? | `itglue-cli passwords stale --days 365` |
| What changed across every client since a given date? | `itglue-cli changes --since 2026-05-01` |
| Which contacts are duplicated across or within clients? | `itglue-cli contacts dupes` |
| Everything we know about one client, in one offline read? | `itglue-cli org show "12345"` |
| Which records are orphaned after a client offboarding? | `itglue-cli orphans` |
| List every organization in the account. | `itglue-cli organizations list` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most IT Glue integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 200 clients are missing a documented firewall" or "which credentials across the whole fleet haven't rotated in a year" - questions that would mean thousands of calls against IT Glue's 3000-requests / 5-minute rate ceiling, one org and one resource at a time.

This skill syncs IT Glue into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `coverage`, `passwords stale`, and `orphans` join across organizations, contacts, passwords, configurations, and documents - work a stateless API wrapper can't do.

## The pain this closes

IT Glue is the system of record for your whole client base, but it shows you one organization at a time. There's no built-in scorecard for which clients are under-documented, so completeness gaps surface during an incident or a QBR instead of before - keeping IT Glue current is a recurring discipline complaint on r/msp. Credential hygiene is worse: SOC2 and cyber-insurance audits ask when privileged credentials last rotated, but IT Glue has no single screen that lists stale passwords across every client, and scanning live runs into the 3000-requests / 5-minute API ceiling. Underneath, the API has no endpoint for "which contacts are duplicated" or "which records point at an organization that no longer exists" - the messes overlapping PSA syncs and client offboarding leave behind.

This skill answers each of those from the local mirror in one command:

- **`itglue-cli coverage --below 1`** - clients missing a whole documentation category, before a QBR finds them.
- **`itglue-cli passwords stale --days 365`** - every credential past its rotation threshold, grouped by client, metadata only - the audit answer.
- **`itglue-cli search "Fortinet"`** - which client owns a device, contact, or serial number, across every synced org at once.
- **`itglue-cli contacts dupes`** - duplicate contacts overlapping PSA syncs leave behind.
- **`itglue-cli orphans`** - configurations, contacts, passwords, and documents whose owning organization is gone.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The IT Glue MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your IT Glue credentials once.

### Is my IT Glue data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### What credentials do I need?

An IT Glue API key, generated in your IT Glue account settings (**Account > Settings > API Keys**). Set `ITGLUE_API_KEY` in your environment, or run `itglue-cli auth set-token`. The key inherits the permissions of the IT Glue account it belongs to, so scope that account to exactly what you want the AI to reach - note that a key with password access lets `passwords get`/`list` return stored secrets.

### Will this hit my IT Glue API rate limits?

IT Glue meters the API at roughly **3000 requests per 5 minutes**. The local mirror exists so reads stop hitting the API: after the first `sync`, `search`, `coverage`, `passwords stale`, `changes`, `contacts dupes`, `org show`, and `orphans` all run against local SQLite with **zero API calls**, and sync is incremental - it only fetches what changed since the last checkpoint.

### Does it work with IT Glue's EU or AU data regions?

Yes. IT Glue hosts data in regional API endpoints; set `ITGLUE_BASE_URL` to your region's API URL and the CLI uses it. The default targets the US endpoint.

### Can it delete my documentation?

No. The CLI reads, and creates or updates contacts, passwords, configurations, and documents - it exposes **no delete** for any IT Glue resource. The `coverage` and `passwords stale` audits read metadata only; password secret values are never read or printed.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `search`, `coverage`, `passwords stale` (metadata only), `changes`, `contacts dupes`, `org show`, `orphans`, `organizations list`/`get`, `configurations`/`contacts`/`documents` `list`/`get` | Allow |
| Write (routine) | `configurations create`/`update`, `contacts create`/`update`, `documents create`/`update`, `organizations relationships create-organization-contact` | Preview with `--dry-run`, then a reviewed write |
| Credential / sensitive | `passwords get` and `passwords list` (can return the stored secret when the API key has password access), `passwords create`, `passwords update`; the CLI exposes no delete for any IT Glue resource | Human-in-the-loop only |

The strongest control is the **scope you grant the IT Glue credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the IT Glue API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
