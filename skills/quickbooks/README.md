# QuickBooks Online + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the QuickBooks Online
> API. Not affiliated with, endorsed by, or sponsored by Intuit Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/quickbooks/">
    <img src="../../docs/assets/video/quickbooks/animated-og.gif" alt="QuickBooks Online demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/quickbooks/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every QuickBooks Online Accounting entity, plus an offline SQLite mirror, cross-entity search, and AR/AP aging no SDK or read-only MCP ships. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the QuickBooks Online skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `quickbooks-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `quickbooks-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `quickbooks-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the QuickBooks Online MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `quickbooks-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download QuickBooks Online MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/quickbooks-v0.1.0/quickbooks-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every QuickBooks Online release on the [releases page](https://github.com/servosity/msp-skills/releases?q=quickbooks).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install quickbooks@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the QuickBooks Online Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.ps1 | iex`. Then authenticate per the README and run `quickbooks-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.sh)
```

The installer drops both `quickbooks-cli` (the CLI) and `quickbooks-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
quickbooks-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quickbooks/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update quickbooks@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/quickbooks --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/quickbooks --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `quickbooks-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the quickbooks skill from https://github.com/servosity/msp-skills/tree/main/skills/quickbooks. The skill defines how its required CLI (`quickbooks-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs. You need two things: an OAuth access token
(scope `com.intuit.quickbooks.accounting`) and your company **realm ID** - the
token says who you are, the realm ID says which company to read.

```bash
export QUICKBOOKS_ACCESS_TOKEN=<token>     # from the Intuit OAuth 2.0 Playground, or: quickbooks-cli auth refresh
export QUICKBOOKS_REALM_ID=<company-id>    # your QuickBooks Online company (realm) ID
# optional: export QUICKBOOKS_ENVIRONMENT=sandbox   # default is production
quickbooks-cli doctor
```

`doctor` confirms the credentials and company scope work before you run anything that
touches data. Have a refresh token instead of a live access token? `quickbooks-cli
auth refresh` mints a fresh one (set `QUICKBOOKS_CLIENT_ID`, `QUICKBOOKS_CLIENT_SECRET`,
and `QUICKBOOKS_REFRESH_TOKEN`).


## What this skill does

Sync your company once, then ask:

| Question your MSP keeps asking | Command |
| --- | --- |
| Who owes us money, bucketed 0-30 / 31-60 / 61-90 / 90+? | `quickbooks-cli ar-aging --agent` |
| What do we owe vendors, and when is it due? | `quickbooks-cli ap-aging --agent` |
| Which overdue invoices should I chase first? | `quickbooks-cli invoices stale --days 30 --agent` |
| Where does our cash stand across accounts, AR, and AP? | `quickbooks-cli balances --agent` |
| What net cash movement is scheduled over the next 4 weeks? | `quickbooks-cli cash-forecast --weeks 4 --agent` |
| What is our DSO, and who are the slowest payers? | `quickbooks-cli dso --agent` |
| Are the books clean enough to close this month? | `quickbooks-cli reconcile --agent` |
| Who slipped an aging bucket since our last check? | `quickbooks-cli aging-delta --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most QuickBooks Online integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at close time, when you're asking "who's overdue across the whole book and how much is in the 90+ bucket" - because QuickBooks has no single endpoint that returns aging, DSO, or a reconciliation verdict.

This skill syncs QuickBooks Online into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `ar-aging`, `dso`, and `reconcile` join across invoices, payments, customers, and vendors with real date math - work a stateless API wrapper can't do.

## The pain this closes

Ask any MSP owner what eats the last day of the month and the answer is the books. On r/msp and r/QuickBooks the same complaints recur: close drags because you're chasing unapplied payments, deduping "Acme Inc" against "Acme, Inc.", and proving the ledger balances before it goes to the accountant. Receivables age silently - by the time you export the A/R Aging Summary, the 90+ bucket holds money you'll fight to collect - and because QuickBooks reports are point-in-time, you can't see who slipped a bucket since last month without keeping your own spreadsheet history.

This skill turns each chore into one offline question:

| The chore | The command |
| --- | --- |
| See who's overdue without an export-and-pivot | `quickbooks-cli ar-aging --agent` |
| Build this week's collections call list | `quickbooks-cli invoices stale --days 30 --agent` |
| Run the whole month-end close-hygiene sweep | `quickbooks-cli reconcile --agent` |
| Find duplicate customer/vendor records | `quickbooks-cli dupes customers --agent` |
| See what changed in aging since last check | `quickbooks-cli aging-delta --agent` |

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The QuickBooks Online MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your QuickBooks Online credentials once.

### Is my QuickBooks Online data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my QuickBooks API rate limits?

Rarely. The only API-heavy step is the one-time `sync` that mirrors your company into local SQLite; after that, `ar-aging`, `dso`, `cash-forecast`, `reconcile` and the rest run entirely offline against the mirror. Intuit throttles per company (realm), and the CLI paginates and rate-limits sync for you, so day-to-day questions never touch the API.

### Do I need to be an Intuit partner to use it?

No. You need a QuickBooks Online company and an OAuth access token scoped to `com.intuit.quickbooks.accounting`, plus your company realm ID. Mint the token from the Intuit Developer portal or the OAuth 2.0 Playground; `quickbooks-cli auth refresh` turns a refresh token into a fresh access token. No partner status required.

### Will this replace my QuickBooks Online portal?

No - it complements it. QuickBooks stays your book of record; this skill answers questions from your terminal or AI agent, keeps cross-run aging memory the portal doesn't, and composes a one-shot month-end `reconcile` the UI makes you assemble by hand. Writes go through the same Accounting API the portal uses.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `ar-aging`, `ap-aging`, `balances`, `dso`, `cash-forecast`, `reconcile`, `dupes`, `search`, plus every `list` / `get` | Allow |
| Write (routine) | `invoices` / `bills` / `payments` / `customers` / `vendors` / `accounts` / `items` / `journal-entries` `create` and `update` (16 commands), plus bulk `import` | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | `invoices delete`, `bills delete`, `payments delete`, `journal-entries delete` (hard deletes) | Human-in-the-loop only |

The strongest control is the **scope you grant the QuickBooks Online credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the QuickBooks Online API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
