# N-able N-central + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the N-able N-central
> API. Not affiliated with, endorsed by, or sponsored by N-able, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/n-central/">
    <img src="../../docs/assets/video/n-central/animated-og.gif" alt="N-able N-central demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/n-central/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every N-central REST endpoint, plus an offline SQLite mirror of your whole org tree, cross-tenant search, issue-triage rollups, and a JWT-expiry guardian no other N-central tool has. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the N-able N-central skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `n-central-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `n-central-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `n-central-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the N-able N-central MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `n-central-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download N-able N-central MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/n-central-v0.1.0/n-central-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every N-able N-central release on the [releases page](https://github.com/servosity/msp-skills/releases?q=n-central).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install n-central@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the N-able N-central Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/n-central/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/n-central/install.ps1 | iex`. Then authenticate per the README and run `n-central-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/n-central/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/n-central/install.sh)
```

The installer drops both `n-central-cli` (the CLI) and `n-central-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
n-central-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/n-central/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/n-central/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update n-central@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/n-central --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/n-central --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `n-central-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the n-central skill from https://github.com/servosity/msp-skills/tree/main/skills/n-central. The skill defines how its required CLI (`n-central-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your N-able N-central portal):

```bash
NCENTRAL_JWT=<value> n-central-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What's red right now, grouped by customer and ranked by severity? | `n-central-cli triage --by customer` |
| Where is EXCHANGE01 - server, service org, customer, site? | `n-central-cli whereis EXCHANGE01` |
| Find anything named acme across every server we run | `n-central-cli fanout "acme"` |
| Which devices are missing the Backup Plan custom property? | `n-central-cli props audit --required "Backup Plan"` |
| Which devices have no maintenance window before the patch wave? | `n-central-cli maint coverage --before 2026-06-15` |
| Is the JWT healthy, and when does the API user's password kill it? | `n-central-cli guardian --password-set 2026-03-01` |
| Hardware and software inventory for one device | `n-central-cli devices assets 987654321` |
| Every device, exported for the QBR or your documentation tool | `n-central-cli export "devices" --format jsonl` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most N-able N-central integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "where is this device across our three N-central servers" or "what's red right now across every customer" - org-tree pagination, per-endpoint concurrency caps (N-able documents 429s beyond 3-50 concurrent calls), and a JWT that silently dies on password rotation.

This skill syncs the org tree into a **local SQLite mirror** with full-text search. Compound commands like `whereis` (device name fragment to full server > service org > customer > site path), `fanout` (one query unioned across every server's mirror), and `triage` (active issues grouped by customer, device, or monitor and ranked by severity) join across the org tree - work a stateless API wrapper can't do. And `guardian` turns the silent JWT failure mode into a CI-wireable warning.

## The pain this closes

N-able's own REST API documentation admits the two traps every N-central integrator hits: per-endpoint concurrency caps (429 beyond 3-50 concurrent calls, per the official known-issues page) and error messages that arrive **inside a 200 OK response**. The community-maintained NC-API-Documentation project on GitHub documents the third: the API user's password expiry - 90 days by default - silently invalidates the JWT, and the integration just stops.

Meanwhile the daily questions stay manual: the morning NOC sweep is a per-customer console walk, "where is that machine" means knowing its customer and site already, and a multi-server MSP repeats all of it in each console.

- `n-central-cli triage --by customer` - the NOC sweep as one command, ranked by severity.
- `n-central-cli whereis EXCHANGE01` - full org path from the local mirror, offline.
- `n-central-cli fanout "acme"` - one query across every server's mirror, each match tagged with its server.
- `n-central-cli guardian --password-set 2026-03-01` - validates the token, warns 14 days before password expiry kills the JWT, and catches the 200-OK-with-error-body case. Exits non-zero so CI can gate on it.
- `n-central-cli maint coverage --before 2026-06-15` - devices with no maintenance window before the patch wave, so nothing reboots in business hours.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The N-able N-central MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your N-able N-central credentials once.

### Is my N-able N-central data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### What credentials do I need?

A JSON Web Token for an API-only user: in N-central, go to **Administration > User Management > Users** > select the API user > **API Access** > **Generate JSON Web Token**. MFA must be off on that user, and the user's password expiry (90 days by default) silently invalidates the JWT - `guardian` tracks that countdown. Set `NCENTRAL_JWT` and `N_CENTRAL_BASE_URL` (e.g. `https://yourmsp.ncod.n-able.com/api`).

### Can this make changes in N-central?

Almost everything is read-only. Two commands can change things: `scheduled-tasks run` executes an API-enabled Automation Policy or Script on a device, and `import` POSTs records from a JSONL file. `--dry-run` is an opt-in preview, not a default - the recommended agent policy is preview first, a human approves, then run. Keep `scheduled-tasks run` human-in-the-loop.

### We run more than one N-central server - does this handle that?

Yes. Each server syncs to its own local mirror, and `fanout` unions them: one query returns matches across every server's mirror, each row tagged with the server it came from.

### Will this hit N-central API rate limits?

N-able documents per-endpoint concurrency caps - 429 responses beyond roughly 3 to 50 concurrent calls depending on the endpoint. The CLI ships a `--rate-limit` throttle, and the heaviest questions (`whereis`, `fanout`, `search`) run against the local mirror with zero API calls.

### Does this work with N-able N-sight?

No. This skill targets the N-central REST API specifically. N-sight is a separate N-able product with a separate API.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `n-central-cli triage`, `whereis`, `fanout`, `devices list`, `props audit`, `maint coverage`, `guardian`, `export` | Allow |
| Write (routine) | `n-central-cli import <resource> --input data.jsonl` - POSTs each record; `--dry-run` is an opt-in preview, not a default | Preview with `--dry-run`, then a reviewed write |
| Remote execution | `n-central-cli scheduled-tasks run --task-type Script --device-id <id> --item-id <id>` - executes on a live endpoint | Human-in-the-loop only, explicit confirmation |
| Credential / security | `n-central-cli customers registration-token`, `org-units registration-token` (device-enrollment tokens), `auth set-token`, `auth logout` | Human-in-the-loop only |

The strongest control is the **scope you grant the N-able N-central credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the N-able N-central API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-05._
