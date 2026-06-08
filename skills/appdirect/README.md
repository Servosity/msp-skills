# AppDirect + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the AppDirect
> API. Not affiliated with, endorsed by, or sponsored by AppDirect, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/appdirect/">
    <img src="../../docs/assets/video/appdirect/animated-og.gif" alt="AppDirect demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/appdirect/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every documented AppDirect marketplace operation in one binary, plus offline sync and billing-reconciliation joins. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the AppDirect skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `appdirect-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `appdirect-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `appdirect-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the AppDirect MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `appdirect-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download AppDirect MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/appdirect-v0.1.0/appdirect-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every AppDirect release on the [releases page](https://github.com/servosity/msp-skills/releases?q=appdirect).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install appdirect@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the AppDirect Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/appdirect/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/appdirect/install.ps1 | iex`. Then authenticate per the README and run `appdirect-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/appdirect/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/appdirect/install.sh)
```

The installer drops both `appdirect-cli` (the CLI) and `appdirect-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
appdirect-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/appdirect/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/appdirect/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update appdirect@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/appdirect --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/appdirect --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `appdirect-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the appdirect skill from https://github.com/servosity/msp-skills/tree/main/skills/appdirect. The skill defines how its required CLI (`appdirect-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your AppDirect portal):

```bash
APPDIRECT_CLIENT_ID=<value> APPDIRECT_CLIENT_SECRET=<value> APPDIRECT_OAUTH_SCOPE=<value> appdirect-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which payments failed or stalled in the last week, across every company? | `appdirect-cli payments unpaid --since 7d --json` |
| What's active-but-unbilled, overdue, or failed before month-close? | `appdirect-cli reconcile --since 30d --agent` |
| What changed in subscriptions this week - new, ended, or suspended? | `appdirect-cli subs changed --since 7d --json` |
| Show one customer's full picture - users, subscriptions, invoices, opportunities. | `appdirect-cli company show <companyId>` |
| What does my assisted-sales pipeline look like by status? | `appdirect-cli pipeline --group-by status --agent` |
| Which open opportunities have gone stale? | `appdirect-cli pipeline stale --days 14 --json` |
| Find any company, subscription, invoice, or opportunity by keyword. | `appdirect-cli search "<text>"` |
| Pull the whole marketplace into a local mirror for offline analysis. | `appdirect-cli sync` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most AppDirect integrations and MCP servers proxy each question into a live API call - and re-mint an hourly OAuth token to do it. That's fine for one record. It dies at scale, when you're asking which subscriptions across all your reseller companies are active but never got invoiced this quarter.

This skill syncs AppDirect into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `reconcile`, `subs changed`, and `company show` join across subscriptions, invoices, payments, companies, and opportunities - work a stateless API wrapper can't do.

## The pain this closes

On r/msp, billing reconciliation for resold SaaS is a recurring complaint - "billing reconciliation nightmare," monthly true-ups, and **margin leak from subscriptions that are active in the marketplace but never made it onto an invoice**. The console shows one company on one screen, so catching an un-invoiced subscription, a failed payment, or a stalled deal means clicking through hundreds of company billing pages by hand. The reconciliation slips, and the leak compounds every month-close.

This skill turns those questions into one command each:

- `appdirect-cli reconcile --since 30d --agent` - active subscriptions with no invoice, overdue invoices, and failed payments across every company.
- `appdirect-cli payments unpaid --since 7d --json` - the weekly failed-payment chase as one sorted list.
- `appdirect-cli subs changed --since 7d --json` - new, ended, and suspended subscriptions for churn review.
- `appdirect-cli company show <companyId>` - one customer's whole picture for renewal prep.
- `appdirect-cli pipeline stale --days 14 --json` - open opportunities that have gone quiet.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The AppDirect MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your AppDirect credentials once.

### Is my AppDirect data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need to be an AppDirect partner?

Yes. The skill authenticates with **partner API credentials** (OAuth2 client_credentials) for a marketplace you operate or resell on. It is not for end-user purchases on a marketplace you don't control. If you run a white-label marketplace on your own domain, point the CLI at it with `APPDIRECT_BASE_URL`.

### Will this hit AppDirect's API rate limits?

It is built to avoid them. The marketplace REST API uses leaky-bucket rate limits (for example, 20-request buckets that refill a few requests per second). Because this skill syncs to a **local mirror** and answers most questions offline, your day-to-day queries make almost no live calls. `sync` respects the limits, and you can cap request rate with `--rate-limit`.

### Will this replace my AppDirect portal?

No - it complements it. The portal stays the place you click through one company at a time; this skill answers the cross-company, before-month-close billing and pipeline questions the portal has no screen for.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

Tiers are derived from each command's real HTTP method, so a write named `finalize-opportunity` or `upload-and-link-image` is gated as a write, not waved through as a read.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `payments unpaid`, `reconcile`, `subs changed`, `company show`, `pipeline`, `search`, and all 160+ GET endpoints | Allow |
| Write (routine) | create/update companies, users, memberships, and opportunities; invite users; request a purchase; apply a discount; finalize an opportunity (118 total) | Preview with `--dry-run`, then a reviewed write |
| Credential / financial | set a temporary password; create/update/default a payment method or payment instrument (7 total) | Human-in-the-loop only |
| Destructive | delete a membership, group, subscription assignment, or shopping cart; cancel a subscription; expire a developer account (34 total) | Human-in-the-loop only |

The strongest control is the **scope you grant the AppDirect credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the AppDirect API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-07._
