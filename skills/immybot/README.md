# ImmyBot + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the ImmyBot
> API. Not affiliated with, endorsed by, or sponsored by ImmyBot, LLC..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/immybot/">
    <img src="../../docs/assets/social/immybot/wide-1200x630.png" alt="ImmyBot - MCP server and Claude Code Skill" width="600">
  </a>
</p>
<p align="center"><sub><a href="https://msp-skills.compoundingteams.com/skills/immybot/">Full skill page</a> - install, outcomes, safety model.</sub></p>
<!-- media:end -->

Every ImmyBot endpoint typed, plus a local SQLite mirror that answers the cross-tenant questions the web UI cannot. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the ImmyBot skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `immybot-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `immybot-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `immybot-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the ImmyBot MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `immybot-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download ImmyBot MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/immybot-v0.1.1/immybot-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every ImmyBot release on the [releases page](https://github.com/servosity/msp-skills/releases?q=immybot).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install immybot@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the ImmyBot Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/immybot/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/immybot/install.ps1 | iex`. Then authenticate per the README and run `immybot-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/immybot/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/immybot/install.sh)
```

The installer drops both `immybot-cli` (the CLI) and `immybot-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
immybot-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/immybot/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/immybot/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update immybot@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/immybot --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/immybot --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `immybot-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the immybot skill from https://github.com/servosity/msp-skills/tree/main/skills/immybot. The skill defines how its required CLI (`immybot-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your ImmyBot portal):

```bash
export IMMYBOT_SUBDOMAIN=acme          # the "acme" in acme.immy.bot
export IMMYBOT_TENANT_ID=<entra-tenant-guid>
export IMMYBOT_CLIENT_ID=<entra-app-client-id>
export IMMYBOT_CLIENT_SECRET=<entra-app-client-secret>
immybot-cli doctor
```

ImmyBot issues no API key of its own. You register an app in Microsoft Entra ID, create a
client secret, then add that Enterprise Application's object ID as an admin person inside
ImmyBot (Show More > People > New > AD External ID). `IMMYBOT_SUBDOMAIN` is required: it
derives the OAuth scope, and without it the token request falls back to a scope naming your
own app registration and Entra rejects it.

`doctor` checks config, paths, and API reachability, and reports whether credentials are loaded - it does not validate them. Run a read command to confirm the credential actually works end-to-end.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What failed in last night's maintenance window, and why? | `immybot-cli session-triage --since 24h` |
| Which tenants are still behind on Chrome? | `immybot-cli version-spread "Google Chrome" --min-version 140` |
| Why did this machine never get the deployment? | `immybot-cli assignment-explain 4821` |
| What actually changed in the fleet since last night? | `immybot-cli fleet-diff --snapshot` once, then `immybot-cli fleet-diff --since 24h` |
| Which computers are stuck part-way through onboarding? | `immybot-cli onboarding-stalled --older-than 3d` |
| What does this shared script reach before I edit it? | `immybot-cli script-blast-radius "Install Chrome"` |
| Which deployments have never once succeeded? | `immybot-cli deployment-health --only-failing` |
| Which computers are in ImmyBot but missing from my PSA? | `immybot-cli psa-reconcile` |
| What is this one machine's full history in one view? | `immybot-cli computer-dossier WS-01` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most ImmyBot integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking which of your 40 clients are still running a Chrome build below 140, because that is one API call per tenant per title and ImmyBot keeps no history to compare against.

This skill syncs ImmyBot into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `session-triage`, `version-spread` and `assignment-explain` join across maintenance sessions, actions, computers, tenants, software inventory and target assignments - work a stateless API wrapper can't do.

## The pain this closes

ImmyBot's own community threads and the r/msp deployment discussions circle the same
shape: the platform is excellent at putting software on machines and unhelpful the morning
after. A maintenance window runs across hundreds of endpoints, a slice of it fails, and the
console shows you the same error text once per machine with no grouping, no root cause, and
no way to ask "is this one broken package or forty broken agents?". The neighbouring
questions are worse, because they are cross-tenant: which clients are still below a version
floor, why one machine did not match a deployment when its neighbour did, and what changed
overnight. ImmyBot keeps no history to diff against, so the honest answer today is a
spreadsheet.

- **`session-triage --since 24h`** - collapses a night of failed actions into distinct root
  causes, so you read the failure once instead of forty times.
- **`version-spread "Google Chrome" --min-version 140`** - one title ranked across every
  tenant with real semver comparison, not string sorting.
- **`assignment-explain 4821`** - every target assignment that resolves onto one computer,
  which scope matched, and which rules are shadowed: the answer to "why not this machine?".
- **`fleet-diff --since 24h`** - what actually changed between two syncs, from history the
  API does not keep. ImmyBot exposes no updated-since cursor, so this compares against local
  snapshots: run `immybot-cli fleet-diff --snapshot` after a sync to record the first
  baseline, and the comparison works from the next sync onward.
- **`deployment-health --only-failing`** - deployments that never once succeeded, which is
  the failure nobody notices because it never generated an alert.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The ImmyBot MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your ImmyBot credentials once.

### Is my ImmyBot data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need to be an ImmyBot partner or MSP customer?

You need an ImmyBot instance and the ability to register an app in the Microsoft Entra
tenant it trusts. Everything the skill does, it does as an admin person you create inside
your own instance.

### Will this hit my ImmyBot API rate limits?

`sync` is the only command that talks to the API in bulk, and it is the one you schedule.
Every cross-tenant view above reads the local SQLite mirror, so asking the same question ten
times costs zero API calls.

### Will this replace the ImmyBot console?

No. Deployments, approvals and desired-state configuration stay in the console. This is the
read layer the console does not have: cross-tenant joins, root-cause grouping, and history
to diff against.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `session-triage`, `drift`, `version-spread`, `fleet-diff`, `assignment-explain`, and every other `list` / `get` except the credential read below | Allow |
| Endpoint and fleet execution | `scripts create-run-adhoc-metascript` (runs a script body you supply on a named machine), `run-immy-service`, `maintenance-actions create-latest-action-for-tenants`, `schedules create-bulk-run-now`, `computers registry create-keys`, `target-assignments create`, `software create-global-upload` | Human-in-the-loop, explicit confirmation |
| Write (data) | `tenants create`, `persons create`, `tags create`, `preferences update-application`, `import` | Preview with `--dry-run`, then a reviewed write |
| Credential / security | `auth login`, `oauth get-access-tokens`, `access get-get-azure-tenant-auth-details-by-azure-tenant-principal-id` (a read that returns a secret), `roles create` | Human-in-the-loop only |
| Destructive | `computers create-bulk-delete`, `scripts delete-global-by-id`, `software delete-global-by-identifier`, `brandings delete-by-id` | Human-in-the-loop only, explicit confirmation |
| Local config | `profile save`, `auth logout`, `sync`, `export`, `feedback` | Allow (no server effect) |

ImmyBot deploys software and runs maintenance on real client machines, so the
**Endpoint and fleet execution** tier is the one that matters most here: a single
`create-latest-action-for-tenants` reaches every computer in a tenant.

The strongest control is the **scope you grant the ImmyBot credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the ImmyBot API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-08-22._
