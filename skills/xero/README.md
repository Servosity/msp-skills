# Xero + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Xero
> API. Not affiliated with, endorsed by, or sponsored by Xero Limited.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/xero/">
    <img src="../../docs/assets/video/xero/animated-og.gif" alt="Xero demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/xero/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Xero Accounting read plus a local SQLite ledger  -  aging, reconciliation, and GL tie-out that no other Xero tool computes offline. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Xero skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `xero-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `xero-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `xero-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Xero MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `xero-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Xero MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/xero-v0.1.0/xero-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Xero release on the [releases page](https://github.com/servosity/msp-skills/releases?q=xero).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install xero@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Xero Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.ps1 | iex`. Then authenticate per the README and run `xero-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.sh)
```

The installer drops both `xero-cli` (the CLI) and `xero-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
xero-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update xero@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/xero --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/xero --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `xero-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the xero skill from https://github.com/servosity/msp-skills/tree/main/skills/xero. The skill defines how its required CLI (`xero-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the two credentials the CLI needs - an OAuth2 access token (mint a Custom Connection in the [Xero developer portal](https://developer.xero.com)) and your organisation's tenant id:

```bash
XERO_ACCESS_TOKEN=<value> XERO_TENANT_ID=<value> xero-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Who owes us, and how overdue is each invoice? | `xero-cli aging --agent` |
| What do we owe suppliers, bucketed by age? | `xero-cli aging --payable --agent` |
| Which contacts carry the most receivable risk? | `xero-cli exposure --agent` |
| Which authorised invoices are still owed with no applied payment? | `xero-cli reconcile --agent` |
| Which bank transactions are unreconciled, and what might they match? | `xero-cli bank-recon --agent` |
| Do the GL control accounts tie to outstanding invoices at close? | `xero-cli tie-out --agent` |
| What posted to a single account, as a running balance? | `xero-cli ledger 200 --agent` |
| What changed in the organisation since last week? | `xero-cli since 7d --agent` |
| Give me one state-of-the-org summary in a single call. | `xero-cli snapshot --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Xero integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "who owes us across every authorised invoice and how overdue is each one" - and it burns through Xero's 60-call-per-minute, 5,000-per-day rate limit doing it.

This skill syncs Xero into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `reconcile` (authorised invoices cross-joined to applied payments) and `tie-out` (the immutable journal control accounts compared against outstanding invoices) join across invoices, payments, journals, and accounts - work a stateless API wrapper can't do.

## The pain this closes

Cash flow is what kills small service businesses, and the slowest loop in it is collections. On r/msp and r/bookkeeping the same complaint recurs: the weekly AR chase and the month-end close are manual rituals. You export the Aged Receivables report, paste it into a spreadsheet, pivot by days overdue - a chase list already a day stale by the time it's built. Reconciling applied cash to authorised invoices, and unreconciled bank lines to the invoices they settle, is click-back-and-forth one row at a time. And scripting around it hits Xero's 60-calls-per-minute, 5,000-per-day API ceiling, so the cross-endpoint answers you actually want never come back in a single call.

This skill closes that loop by syncing the org into a local mirror, then answering offline:

- **`xero-cli aging --agent`** - this week's chase list, bucketed by days overdue (`--payable` for what you owe).
- **`xero-cli exposure --agent`** - receivable risk ranked by contact, with an overdue split.
- **`xero-cli reconcile --agent`** - the cash-application gap: authorised invoices still owed with no applied payment.
- **`xero-cli bank-recon --agent`** - unreconciled bank lines matched to their likely invoices and payments.
- **`xero-cli tie-out --agent`** - proof the GL control accounts tie to outstanding invoices at close; variance zero is the signal.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Xero MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Xero credentials once.

### Is my Xero data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my Xero API rate limits?

Rarely. After one `sync` into the local mirror, aging, reconciliation, tie-out, exposure, ledger, and search run entirely offline - zero API calls. Only `sync`, explicit live reads, and writes touch the API, which Xero caps at **60 calls per minute and 5,000 per day per organisation**. The local-first design exists precisely to stay under that ceiling.

### Which Xero credentials does it need, and does it create them?

You mint an OAuth2 token in the Xero developer portal - a **Custom Connection** is the simplest for machine-to-machine use - then pass it plus your organisation's tenant id via `XERO_ACCESS_TOKEN` and `XERO_TENANT_ID`. The CLI **never creates or rotates tokens**; it only uses the one you give it. Run `xero-cli doctor` to confirm both are set before syncing.

### Does it cover one organisation or several?

One organisation per `XERO_TENANT_ID`. The local mirror holds a single tenant; for a multi-entity portfolio, point the tenant id at each organisation in turn and loop.

### Will it replace the Xero web app?

No. It complements the portal you still use for editing and day-to-day bookkeeping. This skill is for the cross-report questions - aging, reconciliation, GL tie-out - answered in the AI agent you already work in, computed offline.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `aging`, `exposure`, `reconcile`, `bank-recon`, `tie-out`, `ledger`, `snapshot`, `since`, `search`, `sync`, `doctor`, and every `get` | Allow |
| Write (routine) | `invoices create`, `invoices update`, `contacts create`, `accounts create`, `payments create`, `bank-transactions create`, `items create`, and the bulk `import` | Preview with `--dry-run`, then a reviewed write |
| Destructive / credential | `accounts delete`, `items delete`, `payments delete`, `auth logout` | Human-in-the-loop only |

The strongest control is the **scope you grant the Xero credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Xero API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
