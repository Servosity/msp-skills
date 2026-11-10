# Dropsuite (NinjaOne) + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Dropsuite (NinjaOne)
> API. Not affiliated with, endorsed by, or sponsored by NinjaOne, LLC.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/dropsuite/">
    <img src="../../docs/assets/video/dropsuite/animated-og.gif" alt="Dropsuite demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/dropsuite/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

<!-- first-party-note:start -->
> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](../servosity) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.
<!-- first-party-note:end -->

<!-- backup-vendor-note:start -->
> **Comparing backup vendors?** One of our MSP partners told us, "My guys now spend a tenth of the time on backups." Fixing is included in every Servosity co-managed backup plan, and our engineers tell you what they did. [See how co-managed backup works](https://servosity.com/?utm_source=msp-skills&utm_medium=connector-note&utm_campaign=dropsuite&utm_content=readme)
<!-- backup-vendor-note:end -->

Every Dropsuite (NinjaOne SaaS Backup) read, plus fleet-wide backup health, coverage gaps and billing reconciliation from one reseller credential. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Dropsuite (NinjaOne) skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `dropsuite-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `dropsuite-mcp` over HTTPS with `DROPSUITE_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `dropsuite-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `dropsuite-mcp --transport http` with `DROPSUITE_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `dropsuite-mcp` over HTTPS with `DROPSUITE_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Dropsuite (NinjaOne) MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/dropsuite-v0.1.2/dropsuite-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Dropsuite (NinjaOne) release on the [releases page](https://github.com/servosity/msp-skills/releases?q=dropsuite).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install dropsuite@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Dropsuite (NinjaOne) Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.ps1 | iex`. Then authenticate per the README and run `dropsuite-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.sh)
```

The installer drops both `dropsuite-cli` (the CLI) and `dropsuite-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
dropsuite-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update dropsuite@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/dropsuite --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/dropsuite --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `dropsuite-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the dropsuite skill from https://github.com/servosity/msp-skills/tree/main/skills/dropsuite. The skill defines how its required CLI (`dropsuite-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the two tokens from **Settings > API Settings** in your Dropsuite partner portal (add `DROPSUITE_HOST=<host>` if your partner API is not on dropsuite.us):

```bash
DROPSUITE_RESELLER_TOKEN=<reseller token> DROPSUITE_ACCESS_TOKEN=<authentication token> dropsuite-cli doctor
```

Windows (PowerShell):

```powershell
$env:DROPSUITE_RESELLER_TOKEN = "<reseller token>"; $env:DROPSUITE_ACCESS_TOKEN = "<authentication token>"; dropsuite-cli doctor
```

`doctor` checks config, paths, and API reachability, and reports whether credentials are loaded - it does not validate them. Run `dropsuite-cli organizations list --agent` to confirm the reseller tokens work end-to-end, then add `--org <organization id or owner email>` to any per-client read.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What is failing, stuck or stale across every client? | `dropsuite-cli health --stale-after 48h --failing --agent` |
| Which tenant mailboxes are not backed up yet? | `dropsuite-cli coverage --agent` |
| Do my seat counts match what I invoice? | `dropsuite-cli billing --invoice psa-seats.csv --flagged-only --agent` |
| Which deactivated mailboxes still hold storage? | `dropsuite-cli hygiene --older-than 30d --agent` |
| Which client owns this address, and is it protected? | `dropsuite-cli locate jane@example.com --agent` |
| What stopped backing up since last week? | `dropsuite-cli drift --since 7d --agent` |
| Dated proof of protection for one client | `dropsuite-cli evidence --org 26561-14 --window 90d --markdown` |
| Which clients are due a restore test? | `dropsuite-cli restore-test due --every 90d --pick --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Dropsuite (NinjaOne) integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 40 clients has a mailbox that stopped backing up this week": Dropsuite's API answers that one organization at a time, each with its own token.

This skill resolves every organization's token from your reseller credential and walks the whole fleet in one command: `health`, `coverage`, `billing`, `hygiene` and `locate` each answer across every client and say which organizations they could not read. Each health run is saved locally, so `drift` and `evidence` can say what changed and what was true on a date - work a stateless API wrapper can't do, because the API keeps only the current state.

## The pain this closes

A backup that silently stops is the one nobody notices. An r/msp thread, "Office365 backup alternative to Dropsuite" (December 2021), describes a client whose backups had been disabled and went undiscovered for over three months. The partner portal shows one organization at a time, and the API needs each organization's own token, so a daily check across a whole book of clients rarely happens. `health` grades every object across every active client in one run, `drift` shows what stopped advancing since the last run, and `evidence` turns the saved runs and your recorded restore tests into dated proof for an insurer or a QBR.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Dropsuite (NinjaOne) MCP server runs locally over stdio by default. Its HTTP mode (`dropsuite-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `DROPSUITE_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Dropsuite (NinjaOne) credentials once.

### Is my Dropsuite (NinjaOne) data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Which Dropsuite credentials do I need?

The two values from **Settings > API Settings** in the Dropsuite partner portal: the Reseller Token (`DROPSUITE_RESELLER_TOKEN`, sent as X-Reseller-Token) and your reseller Authentication Token (`DROPSUITE_ACCESS_TOKEN`, sent as X-Access-Token). You do not collect each client's token: `--org <organization id or owner email>`, or `DROPSUITE_ORG` for the MCP server, resolves it from the organization list. If your partner API is not on dropsuite.us, set `DROPSUITE_HOST`.

### Can the AI change backups or restore data?

No. Every command reads; `restore-test record` writes only to a local ledger. The one exception is a portal sign-in link (`portal get-login-link`, `organizations get-support-sso-link`, `locate --sso-link`, `evidence --with-sso-link`), and a new organization link invalidates the previous one. Over MCP those calls return a preview until the agent repeats them with `confirm: true`, and `DROPSUITE_MCP_READ_ONLY=1` removes them.

### Does it keep a copy of my clients' data?

Reads in the default `--data-source auto` mode are saved to a local SQLite store, and `health` saves a snapshot per run for `drift` and `evidence`. Tokens are never written to the store, and API responses are never cached on disk. `local-data purge` previews what it would erase and deletes only with `--yes`; `DROPSUITE_NO_LOCAL_DATA=1` turns local saving off.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `health`, `coverage`, `billing`, `hygiene`, `locate`, `drift`, `evidence`, `organizations list`, `accounts list` | Allow |
| Local records | `restore-test record`, `sync`, `export`, `profile save`, `teach` (on your machine only) | Allow; review what an agent records |
| Credential | `portal get-login-link`, `organizations get-support-sso-link`, `locate --sso-link`, `evidence --with-sso-link`, `auth set-token` | Human-in-the-loop only |
| Destructive (local) | `local-data purge` (needs `--yes`), `learnings forget`, `auth logout` | Human-in-the-loop only |

The strongest control is the **scope you grant the Dropsuite (NinjaOne) credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Dropsuite (NinjaOne) API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-11-04._
