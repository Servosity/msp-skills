# Autotask + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Autotask
> API. Not affiliated with, endorsed by, or sponsored by Datto, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/autotask/">
    <img src="../../docs/assets/video/autotask/animated-og.gif" alt="Autotask PSA demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/autotask/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Autotask entity at the command line, plus a local SQLite mirror that answers ticket-aging, workload, unbilled-time, and account-360 questions no other Autotask tool can. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Autotask skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `autotask-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `autotask-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `autotask-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Autotask MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `autotask-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Autotask MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/autotask-v0.1.0/autotask-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Autotask release on the [releases page](https://github.com/servosity/msp-skills/releases?q=autotask).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install autotask@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Autotask Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/autotask/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/autotask/install.ps1 | iex`. Then authenticate per the README and run `autotask-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/autotask/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/autotask/install.sh)
```

The installer drops both `autotask-cli` (the CLI) and `autotask-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
autotask-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/autotask/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/autotask/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update autotask@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/autotask --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/autotask --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `autotask-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the autotask skill from https://github.com/servosity/msp-skills/tree/main/skills/autotask. The skill defines how its required CLI (`autotask-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Autotask portal):

```bash
AUTOTASK_PSA_API_INTEGRATION_CODE=<value> AUTOTASK_PSA_SECRET=<value> AUTOTASK_PSA_USER_NAME=<value> autotask-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which approved time entries haven't been invoiced yet? | `autotask-cli unbilled` |
| How stale is the service desk right now, bucketed by age? | `autotask-cli ticket-aging` |
| Which open tickets has nobody touched in a week? | `autotask-cli stale --days 7` |
| Which unassigned tickets should the dispatcher pick up first? | `autotask-cli triage` |
| Who's overloaded before I assign the next ticket? | `autotask-cli workload` |
| How burned are our block-hour contracts, and when do they run out? | `autotask-cli retainer` |
| Everything we know about one company at once? | `autotask-cli company-360 "1234"` |
| The month-end billing picture in one table? | `autotask-cli reconcile` |
| What's the label-to-ID map for a picklist field? | `autotask-cli picklist "Tickets" "status"` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Autotask integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "how much approved time is unbilled across every client this month" or "which retainers run out before the next renewal" - each one a multi-call dance against a per-tenant zone URL, every categorical filter gated by integer picklist IDs that vary per instance, and all of it spending your hourly API request budget.

This skill syncs Autotask into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `reconcile`, `retainer`, and `company-360` join across tickets, time entries, contracts, invoices, and resources - work a stateless API wrapper can't do.

## The pain this closes

Ask any MSP owner where Autotask hurts and you get two answers: **time that never gets billed**, and **reporting that means an export**. Techs close tickets without entering time, or approved hours never make it onto an invoice, and nobody catches it until the billing run - because Autotask has no single screen joining approved time to the invoices it belongs on. And when you want the cross-object picture, Autotask's native **LiveReports** is a copy-and-edit report designer that outputs to Excel, PDF, RTF, or CSV (per Kaseya's own Autotask PSA reporting help) - so anything joining tickets to time to contracts gets pushed into Power BI or another BI tool. An entire third-party connector market exists for exactly that.

This skill answers those questions where the data already lives - one offline query, no report build, no spreadsheet:

| The pain | The command |
| --- | --- |
| Approved hours quietly falling off invoices | `autotask-cli unbilled` |
| Month-end close across time, contracts, and invoices | `autotask-cli reconcile` |
| Retainers silently going negative | `autotask-cli retainer` |
| One client's full footprint for a review | `autotask-cli company-360 "1234"` |
| Status and priority being opaque integer IDs | `autotask-cli picklist "Tickets" "status"` |

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Autotask MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Autotask credentials once.

### Is my Autotask data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### What credentials do I need?

An **API-only user** created in your own Autotask instance, which gives you a `UserName` and `Secret`, plus the tracking **Integration Code** from your integration vendor record. Set `AUTOTASK_PSA_USER_NAME`, `AUTOTASK_PSA_SECRET`, and `AUTOTASK_PSA_API_INTEGRATION_CODE` in your environment. The API user's security level is the real permission boundary - scope it to exactly what you want the AI to reach.

### Do I have to figure out my Autotask zone URL?

No. Autotask hosts each tenant in a numbered zone (`webservicesN.autotask.net`). Run `autotask-cli zone` once and the CLI discovers and caches your tenant's base URL - the same zone-information request Autotask's own docs tell integrations to make first.

### Will this hit my Autotask API rate limits?

Autotask meters API calls per database in a rolling 60-minute window and adds latency as you approach the threshold. The local mirror exists so reads stop hitting the API: after the first sync, the cross-entity views (`unbilled`, `reconcile`, `retainer`, `company-360`, `ticket-aging`, `triage`, `workload`) run against local SQLite with **zero** API calls, and sync is incremental - it only fetches what changed since the last checkpoint.

### Is this Datto Autotask PSA or just Autotask?

Same product. Autotask PSA is now branded **Datto Autotask PSA** under Kaseya; this skill targets the Autotask REST API, which both names refer to. It does **not** cover Datto RMM - that's a separate product with a separate API.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `autotask-cli triage`, `autotask-cli unbilled`, `autotask-cli company-360 "1234"`, `autotask-cli search "vpn outage"`, `autotask-cli tickets query` | Allow |
| Write (routine) | `autotask-cli tickets create-entity`, `autotask-cli tickets patch-entity`, `autotask-cli time-entries create-entity`, `autotask-cli companies update-entity` - writes send immediately; `--dry-run` is an opt-in preview, not a default | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | `autotask-cli time-entries delete-entity`, `autotask-cli service-calls delete-entity "1234"`, `autotask-cli contact-groups delete-entity "1234"` | Human-in-the-loop only |

The strongest control is the **scope you grant the Autotask credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Autotask API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
