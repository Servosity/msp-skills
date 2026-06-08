# Datto BCDR + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Datto BCDR
> API. Not affiliated with, endorsed by, or sponsored by Datto, Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/datto-bcdr/">
    <img src="../../docs/assets/video/datto-bcdr/animated-og.gif" alt="Datto BCDR demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/datto-bcdr/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

<!-- first-party-note:start -->
> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](../servosity) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.
<!-- first-party-note:end -->

Sync your whole Datto BCDR fleet into local SQLite and answer the questions the per-appliance Partner Portal can't: which backups failed screenshot verification, which are stale, and which clients are at risk. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Datto BCDR skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `datto-bcdr-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `datto-bcdr-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `datto-bcdr-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Datto BCDR MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `datto-bcdr-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Datto BCDR MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/datto-bcdr-v4.22.0/datto-bcdr-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Datto BCDR release on the [releases page](https://github.com/servosity/msp-skills/releases?q=datto-bcdr).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install datto-bcdr@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Datto BCDR Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-bcdr/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-bcdr/install.ps1 | iex`. Then authenticate per the README and run `datto-bcdr-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-bcdr/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-bcdr/install.sh)
```

The installer drops both `datto-bcdr-cli` (the CLI) and `datto-bcdr-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
datto-bcdr-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-bcdr/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-bcdr/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update datto-bcdr@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/datto-bcdr --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/datto-bcdr --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `datto-bcdr-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the datto-bcdr skill from https://github.com/servosity/msp-skills/tree/main/skills/datto-bcdr. The skill defines how its required CLI (`datto-bcdr-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Datto BCDR portal):

```bash
DATTO_BCDR_PUBLIC_KEY=<value> DATTO_BCDR_SECRET_KEY=<value> datto-bcdr-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

The Datto BCDR API answers one appliance at a time. This skill mirrors every device, agent, share, and alert into local SQLite, then answers the fleet-wide questions the per-appliance Partner Portal can't:

| Question your MSP keeps asking | Command |
| --- | --- |
| Which protected machines failed their last backup screenshot verification? | `datto-bcdr-cli screenshots --failed --stale-days 7 --agent` |
| Which agents are behind on local snapshots or offsite sync? | `datto-bcdr-cli stale-backups --local-days 1 --offsite-days 3 --agent` |
| What percentage of my fleet is actually recoverable right now? | `datto-bcdr-cli recoverability --agent` |
| Which clients are most at risk across backups, alerts, and storage? | `datto-bcdr-cli client-risk --top 10 --agent` |
| Show me every open alert across the whole fleet, grouped by client | `datto-bcdr-cli alert-triage --group-by client --agent` |
| Which appliance runs out of local or offsite storage first? | `datto-bcdr-cli storage-runway --threshold-pct 85 --agent` |
| Which machines are paused, archived, or on an appliance that went dark? | `datto-bcdr-cli forgotten-assets --offline-days 2 --agent` |
| One backup-health report for a single client before the QBR | `datto-bcdr-cli client-report "Acme Corp" --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Datto BCDR integrations and MCP servers proxy each question into a live API call - and because the Datto API is strictly per-appliance, that means one call per device just to answer one fleet question like "which protected machines across all 40 clients failed their last screenshot verification, oldest failures first".

This skill syncs Datto BCDR into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `client-risk`, `recoverability`, and `client-report` join across devices, agents, alerts, screenshots, and storage in a single pass - work a stateless per-device API wrapper can't do.

## The pain this closes

On r/msp, the recurring Datto BCDR complaint isn't that backups fail loudly - it's that they fail *quietly*. A protected machine's screenshot/boot verification starts failing, or an agent stops taking local snapshots, and nobody notices because the Partner Portal shows backup health one appliance at a time. The gap surfaces at the worst possible moment: restore time, with the client already down. Owners describe walking every SIRIS and ALTO by hand before a QBR or an audit, because the one question they actually need answered - "are all my backups across all my clients recoverable right now?" - has no fleet-wide view.

This skill closes that gap by mirroring the whole estate locally and answering across it:

- `datto-bcdr-cli screenshots --failed --stale-days 7 --agent` - every silently-unbootable backup, fleet-wide, oldest failures first.
- `datto-bcdr-cli recoverability --agent` - the single defensible number: what percent of the fleet is fresh *and* screenshot-verified bootable.
- `datto-bcdr-cli stale-backups --local-days 1 --offsite-days 3 --agent` - agents that quietly stopped taking points, before a client needs them.
- `datto-bcdr-cli client-risk --top 10 --agent` - which clients to call first, ranked across backups, alerts, and storage.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Datto BCDR MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Datto BCDR credentials once.

### Is my Datto BCDR data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need to be a Datto partner to use this?

Yes. It uses the Datto BCDR REST API, which needs a partner-generated public/secret key pair from the Partner Portal under **Admin > Integrations**. If you manage Datto BCDR appliances you already qualify - the CLI base64-encodes the key pair into the `Authorization` header on every request.

### Will this hit my Datto BCDR API rate limits?

It's gentle by design. The first sync pulls each resource with bounded pagination, and you can cap throughput with `--rate-limit`. After that, fleet questions run against the local mirror and make zero API calls - `--data-source local` never touches the API at all.

### Does this replace the Datto Partner Portal?

No. It answers the fleet-wide, cross-client questions the per-appliance portal can't, and it's read-only for everyday use. You still use the portal for restores, virtualization, and device configuration.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `screenshots`, `stale-backups`, `recoverability`, `client-risk`, `alert-triage`, `storage-runway`, `forgotten-assets`, `agent-versions`, `client-report`, `device`, `agent`, `asset`, `shares`, `alert`, `vm-restore`, `sync`, `search`, `analytics` | Allow |
| Write (routine) | `import` (POST each record to the Datto BCDR API) | Preview with `--dry-run`, then a reviewed write |
| Credential / config | `auth set-token`, `auth logout` (replace or clear stored credentials) | Human-in-the-loop only |

The strongest control is the **scope you grant the Datto BCDR credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Datto BCDR API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
