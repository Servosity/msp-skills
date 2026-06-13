# Afi + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Afi
> API. Not affiliated with, endorsed by, or sponsored by Afi.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/afi/">
    <img src="../../docs/assets/social/afi/wide-1200x630.png" alt="Afi - MCP server and Claude Code Skill" width="600">
  </a>
</p>
<p align="center"><sub><a href="https://msp-skills.compoundingteams.com/skills/afi/">Full skill page</a> - install, outcomes, safety model.</sub></p>
<!-- media:end -->

<!-- first-party-note:start -->
> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](../servosity) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.
<!-- first-party-note:end -->

The first CLI for Afi SaaS backup  -  full public-API coverage plus the fleet-wide coverage, staleness, and offboarding answers the rate-limited API can't serve live. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Afi skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `afi-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `afi-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `afi-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Afi MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `afi-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Afi MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/afi-v0.1.0/afi-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Afi release on the [releases page](https://github.com/servosity/msp-skills/releases?q=afi).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install afi@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Afi Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.ps1 | iex`. Then authenticate per the README and run `afi-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.sh)
```

The installer drops both `afi-cli` (the CLI) and `afi-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
afi-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/afi/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update afi@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/afi --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/afi --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `afi-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the afi skill from https://github.com/servosity/msp-skills/tree/main/skills/afi. The skill defines how its required CLI (`afi-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Afi portal):

```bash
AFI_API_KEY=<value> afi-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which resources have no backup protection at all? | `afi-cli coverage-gaps --agent` |
| Which protected resources have a stale backup (silent failures)? | `afi-cli backup-stale --max-age 48h --agent` |
| Is the whole fleet green this morning, or who failed? | `afi-cli fleet-health --failed-only --agent` |
| What is one tenant's full backup posture for a QBR or ticket? | `afi-cli tenant-scorecard <tenant-id> --agent` |
| Am I over- or under-licensed on Afi seats? | `afi-cli reconcile-licenses --agent` |
| Who is jane.doe@example.com in Afi, across Multi-Geo tenants? | `afi-cli resolve <email-or-id> --agent` |
| Safely back up then release a departing employee's mailbox? | `afi-cli offboard <resource-id> --tenant <tenant-id> --reason "employee departure"` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Afi integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which mailboxes across all 47 client tenants have no backup policy attached?" - and Afi's public API throttles you for polling.

This skill syncs Afi into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `coverage-gaps`, `backup-stale`, and `reconcile-licenses` join resources against protections, archives, and purchased subscriptions across every tenant at once - work a stateless API wrapper can't do.

## The pain this closes

SaaS backup has a quiet failure mode MSP owners raise again and again on r/msp: the backup you *think* is running isn't. A mailbox or site gets created in Microsoft 365 or Google Workspace, the onboarding ticket closes, and nobody confirms a backup policy was ever attached - or a policy *is* attached but the nightly archive quietly stops landing, and the portal still shows it "protected." The gap stays invisible until a client asks for a restore and the data was never there. The recurring refrain: *"how do you actually verify your M365 backups are running across every client?"* Afi has the data, but only one tenant at a time, through a portal that forces a per-client walk, behind an API the vendor asks you not to poll.

This skill closes that gap from the local mirror:

- **`afi-cli coverage-gaps --agent`** - every resource with no backup protection, fleet-wide. The blind spots, named.
- **`afi-cli backup-stale --max-age 48h --agent`** - protected resources whose newest archive has gone stale: the silent-failure class.
- **`afi-cli fleet-health --failed-only --agent`** - the Monday "is the fleet green" sweep in one table.
- **`afi-cli reconcile-licenses --agent`** - seats purchased vs seats actually protected.
- **`afi-cli offboard <resource-id> --tenant <tenant-id> --reason "employee departure"`** - a guarded archive-then-release that refuses to drop protection until a fresh backup is verified.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Afi MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Afi credentials once.

### Is my Afi data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this trip Afi's API rate limits?

Not if you use it as designed. Afi throttles - and may suspend - applications that poll continuously, so this skill walks the fleet into a local store in one respectful, rate-limited pass (`afi-cli fleet-sync`), then answers every question offline against that store. You sync on a schedule, not on every question.

### Do I need to be an Afi customer, and what access does the key need?

Yes - you need an Afi account and an Application API key, created in the Afi portal (org level under **Configuration > Apps**, or tenant level under **Service > Settings > Apps**). The key inherits the Application's installation scope, so the CLI sees exactly the orgs and tenants that Application is installed on. Each Application supports two keys for rotation.

### Will this replace the Afi portal?

No. Restores, exports, and policy editing still happen in the Afi portal - the public API doesn't expose them. This skill is the read, report, and guarded-offboard layer: it answers fleet-wide questions and runs the verified archive-then-release, then hands you back to the portal for the actions only it can do.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `coverage-gaps`, `backup-stale`, `fleet-health`, `tenant-scorecard`, `reconcile-licenses`, `resolve`, `fleet-sync`, and every `list`/`get` | Allow |
| Write (routine) | `orgs create`, `import`, `tenants resources protections-protect`, `tenants jobs tasks-trigger` | Preview with `--dry-run`, then a reviewed write |
| Credential / security | `auth set-token`, `auth logout` | Human-in-the-loop only |
| Destructive / config | `offboard` (releases protection), `tenants resources protections-unprotect`, `tenants archives delete` | Human-in-the-loop only |

The strongest control is the **scope you grant the Afi credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Afi API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-07._
