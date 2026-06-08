# SkyKick + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the SkyKick
> API. Not affiliated with, endorsed by, or sponsored by ConnectWise, LLC.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/skykick/">
    <img src="../../docs/assets/video/skykick/animated-og.gif" alt="SkyKick demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/skykick/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

<!-- first-party-note:start -->
> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](../servosity) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.
<!-- first-party-note:end -->

Fleet-wide M365 backup assurance for SkyKick Cloud Backup - posture, stale snapshots, and coverage gaps no portal or wrapper can show. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the SkyKick skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `skykick-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `skykick-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `skykick-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the SkyKick MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `skykick-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download SkyKick MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/skykick-v0.1.0/skykick-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every SkyKick release on the [releases page](https://github.com/servosity/msp-skills/releases?q=skykick).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install skykick@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the SkyKick Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/skykick/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/skykick/install.ps1 | iex`. Then authenticate per the README and run `skykick-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/skykick/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/skykick/install.sh)
```

The installer drops both `skykick-cli` (the CLI) and `skykick-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
skykick-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/skykick/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/skykick/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update skykick@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/skykick --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/skykick --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `skykick-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the skykick skill from https://github.com/servosity/msp-skills/tree/main/skills/skykick. The skill defines how its required CLI (`skykick-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your SkyKick portal):

```bash
SKYKICK_CLIENT_ID=<value> SKYKICK_CLIENT_SECRET=<value> SKYKICK_OAUTH_SCOPE=<value> skykick-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

Run `skykick-cli fleet-sync` once to build the local copy, then ask:

| Question your MSP keeps asking | Command |
| --- | --- |
| Which customers have a protection gap right now? | `skykick-cli fleet-health --flag-gaps --agent` |
| Whose mailboxes haven't been snapshotted in 48 hours? | `skykick-cli stale-snapshots --hours 48 --agent` |
| What's discovered but not actually being backed up? | `skykick-cli coverage-gaps --type all --agent` |
| Which tenants fall below our retention floor? | `skykick-cli retention-audit --floor-days 365 --agent` |
| Where is autodiscover off, so new mailboxes silently never enroll? | `skykick-cli autodiscover-audit --only-off --agent` |
| What protection changed since my last review? | `skykick-cli drift --agent` |
| What open alerts exist across the whole fleet, worst first? | `skykick-cli alert-sweep --agent` |
| How does backup posture roll up by partner? | `skykick-cli partner-rollup --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most SkyKick integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 50 tenants has a backup gap right now" - because the SkyKick API only serves data one subscription at a time, with no fleet endpoint and no skip paging on alerts, so that one question becomes 50+ sequential calls every time you ask it.

This skill syncs SkyKick into a **local SQLite mirror** with full-text search. Aggregate questions become one local query: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `fleet-health`, `stale-snapshots`, and `drift` join across every subscription's settings, retention, snapshot stats, mailboxes, and sites - work a stateless API wrapper can't do.

## The pain this closes

On r/msp, the recurring refrain about Microsoft 365 backup is "trust but verify" - operators warn each other that the worst time to learn a mailbox stopped backing up, a new hire's mailbox never enrolled, or retention quietly dropped below contract is during a restore request. SkyKick Cloud Backup is set-and-forget by design, and the partner portal shows one customer at a time, so across 30-50 tenants the daily "is everyone actually protected?" check is the thing that silently falls off the routine.

This skill turns that check into a few seconds:

- `skykick-cli fleet-health --flag-gaps --agent` - every tenant with at least one protection gap, in one table.
- `skykick-cli stale-snapshots --hours 48 --agent` - mailboxes that silently stopped snapshotting, fleet-wide.
- `skykick-cli coverage-gaps --type all --agent` - discovered-but-unprotected mailboxes and sites, the post-onboarding reconciliation gap.
- `skykick-cli retention-audit --floor-days 365 --agent` - tenants below your compliance retention floor.
- `skykick-cli drift --agent` - what protection changed since your last review.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The SkyKick MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your SkyKick credentials once.

### Is my SkyKick data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need to be a SkyKick partner?

Yes. The CLI authenticates with SkyKick Partner API client credentials - your API user ID and partner subscription key from your SkyKick / ConnectWise Cloud Services partner account (Partner Portal > Settings > User Profile > Developer API Access). It reads only what those credentials are already permitted to see. Set `SKYKICK_OAUTH_SCOPE=Distributor` for distributor accounts; the default scope is `Partner`.

### Will this hit my SkyKick API rate limits?

`fleet-sync` fans out per subscription with bounded concurrency (you set `--workers` and `--rate-limit`) and caches everything in the local SQLite store, so your day-to-day posture, staleness, and coverage questions run offline and never re-hit the API. SkyKick rate-limits the OAuth token endpoint aggressively, so the CLI mints and reuses cached tokens rather than re-authenticating per call.

### Will this replace the SkyKick portal?

No. It's a read-first fleet overlay for posture, staleness, coverage, retention compliance, and alert triage. Restores, subscription changes, and configuration still happen in the SkyKick portal.

### Does this still work after the ConnectWise migration?

Yes. SkyKick Cloud Backup moved to the `apis.cloudservices.connectwise.com` host (the old `apis.skykick.com` host was retired). This CLI targets the current host by default - no `SKYKICK_BASE_URL` override needed.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** | `fleet-health`, `stale-snapshots`, `coverage-gaps`, `retention-audit`, `autodiscover-audit`, `drift`, `partner-rollup`, `alert-sweep` (without `--apply`), `backup list` / `mailboxes` / `sites` / `storage-settings` / `subscription-settings`, `alerts list`, `identity` | Allow |
| **Write (routine)** | `alerts complete`, `alert-sweep --complete <ids> --apply`, `backup discover-mailboxes`, `backup discover-sites`, `import` | Preview with `--dry-run`, then a reviewed write |
| **Destructive / config** | None - the wrapped SkyKick API surface has no delete, credential-rotation, or admin commands | Human-in-the-loop only |

The strongest control is the **scope you grant the SkyKick credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the SkyKick API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-07._
