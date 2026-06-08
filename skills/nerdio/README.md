# Nerdio Manager + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Nerdio Manager
> API. Not affiliated with, endorsed by, or sponsored by Nerdio, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/nerdio/">
    <img src="../../docs/assets/video/nerdio/animated-og.gif" alt="Nerdio Manager demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/nerdio/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

The first non-PowerShell client for the Nerdio Manager for MSP API - cross-account AVD fleet audits, async-job plumbing, and offline search no other Nerdio tool has. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Nerdio Manager skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `nerdio-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `nerdio-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `nerdio-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Nerdio Manager MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `nerdio-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Nerdio Manager MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/nerdio-v0.1.0/nerdio-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Nerdio Manager release on the [releases page](https://github.com/servosity/msp-skills/releases?q=nerdio).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install nerdio@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Nerdio Manager Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.ps1 | iex`. Then authenticate per the README and run `nerdio-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.sh)
```

The installer drops both `nerdio-cli` (the CLI) and `nerdio-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
nerdio-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update nerdio@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/nerdio --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/nerdio --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `nerdio-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the nerdio skill from https://github.com/servosity/msp-skills/tree/main/skills/nerdio. The skill defines how its required CLI (`nerdio-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Nerdio Manager portal):

```bash
NERDIO_BASE_URL=<value> NERDIO_TOKEN_URL=<value> NERDIO_CLIENT_ID=<value> NERDIO_CLIENT_SECRET=<value> NERDIO_OAUTH_SCOPE=<value> nerdio-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which host pools have autoscale off or drifting across every customer? | `nerdio-cli fleet autoscale-audit` |
| What is running right now across all accounts, and where? | `nerdio-cli fleet host-estate` |
| What did each customer get billed this period, and who is unpaid? | `nerdio-cli fleet billing-rollup --period 2026-05-01:2026-05-31 --unpaid-only` |
| Which customers' Azure usage spiked month-over-month? | `nerdio-cli usages drift --from 2026-04-01:2026-04-30 --to 2026-05-01:2026-05-31` |
| List every customer account I manage | `nerdio-cli accounts` |
| Show the host pools for one account | `nerdio-cli host-pools list <account_id>` |
| Which Intune devices does this account have? | `nerdio-cli devices list <account_id>` |
| Did that backup or provisioning job actually finish? | `nerdio-cli job wait <job_id>` |
| Run one scripted action across many accounts and wait for all of them | `nerdio-cli scripted-actions fan-run <script_id> --accounts 101,102,103 --wait` |
| Search everything I have synced, offline | `nerdio-cli search <query>` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Nerdio Manager integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which host pools across all 40 of my customers have autoscale turned off this quarter" - because the NMM API only returns one account, or one billing period, per call.

This skill syncs Nerdio Manager into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `fleet autoscale-audit`, `fleet billing-rollup`, and `usages drift` join across customer accounts, host pools, invoices, and usage windows - work a stateless API wrapper can't do. And because every NMM change is async, `job wait` turns "did it finish?" into a scriptable primitive that exits non-zero when the job actually failed.

## The pain this closes

On r/msp, the recurring Nerdio refrain is that autoscale is the whole reason to buy it - and the hardest thing to keep honest across a fleet. There's no single view of which host pools, in which customer tenants, actually have scaling enabled, so idle session hosts quietly bleed Azure spend until the invoice lands. Every NMM install is per-tenant, so even "how many hosts are running right now across all clients" means logging into each portal one at a time. And because every change is async, the portal makes you babysit the Jobs page to know whether anything finished.

This skill answers those at the fleet level:

- **`nerdio-cli fleet autoscale-audit`** - every pool across every account whose autoscale is off or drifting from your baseline.
- **`nerdio-cli fleet host-estate`** - one table of every session host, its pool, account, and power state.
- **`nerdio-cli fleet billing-rollup --period <start>:<end>`** - per-customer billed, unpaid, and usage totals, PSA-reconciliation-ready.
- **`nerdio-cli usages drift --from <start>:<end> --to <start>:<end>`** - which customers' consumption spiked month-over-month.
- **`nerdio-cli job wait <job_id>`** - poll any NMM job to a terminal state and exit non-zero if it failed.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Nerdio Manager MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Nerdio Manager credentials once.

### Is my Nerdio Manager data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need to be a Nerdio partner, or run Nerdio Manager for MSP (NMM)?

Yes - this targets the NMM Partner REST API, which is the MSP edition (not Nerdio Manager for Enterprise). You create an API client in your own NMM portal under **Settings > Integrations > REST API**. There is no vendor-global endpoint; the CLI talks to your own instance URL, which you set as `NERDIO_BASE_URL`.

### Will this replace the Nerdio Manager portal?

No. It's a read-first, cross-account companion. Day-to-day operating still happens in NMM; this is for the fleet-wide questions and scripted automation the portal makes tedious.

### Why does a change only return a job ID?

Every NMM mutation (provisioning, scripted actions, backup, host power) is async and returns a job ID. Run `nerdio-cli job wait <job_id>` to poll it to a terminal state and exit non-zero if it failed - so your agent never reports "done" on a job that actually errored.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** | `accounts`, `fleet autoscale-audit`, `fleet host-estate`, `fleet billing-rollup`, `usages drift`, `host-pools list`, `devices list`, `job wait`, `search`, `sync`, and the other `list`/`get` commands | Allow |
| **Write (routine)** | `host-pools create`, `host-pools set-autoscale`, `reservations create`/`update`, `recovery-vaults create`/`link`/`unlink`, `resource-groups link`/`set-default`, `networks link`, `provisioning link-network`/`link-tenant`, `workspaces create`, `backup enable`/`disable`, `job retry`, `import` | Preview with `--dry-run`, then a reviewed write |
| **Endpoint / infrastructure control** | `hosts start`/`stop`/`restart`, `desktop-images start`/`stop`, `devices sync`, `scripted-actions run`/`run-account`/`fan-run`, `backup run`/`restore` | Human-in-the-loop - these power, restart, or execute code on live VMs and devices |
| **Credential / security** | `secure-variables list`/`create`/`update`/`delete` (and `account-*` variants), `devices bitlocker-keys`, `devices laps` | Human-in-the-loop only - these read or write stored secrets, BitLocker keys, and LAPS passwords |
| **Destructive** | `host-pools delete`, `reservations delete`, `recovery-vaults delete-policy`, `resource-groups unlink`/`account-unlink`, `scripted-actions unschedule` | Human-in-the-loop only, explicit confirmation |

The strongest control is the **scope you grant the Nerdio Manager credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Nerdio Manager API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-07._
