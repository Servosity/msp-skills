# Salesbuildr + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Salesbuildr
> API. Not affiliated with, endorsed by, or sponsored by Salesbuildr.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/salesbuildr/">
    <img src="../../docs/assets/video/salesbuildr/animated-og.gif" alt="Salesbuildr demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/salesbuildr/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Salesbuildr resource as a scriptable command, plus offline margin and pipeline analytics. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Salesbuildr skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `salesbuildr-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `salesbuildr-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `salesbuildr-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Salesbuildr MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `salesbuildr-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Salesbuildr MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/salesbuildr-v0.1.0/salesbuildr-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Salesbuildr release on the [releases page](https://github.com/servosity/msp-skills/releases?q=salesbuildr).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install salesbuildr@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Salesbuildr Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/salesbuildr/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/salesbuildr/install.ps1 | iex`. Then authenticate per the README and run `salesbuildr-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/salesbuildr/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/salesbuildr/install.sh)
```

The installer drops both `salesbuildr-cli` (the CLI) and `salesbuildr-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
salesbuildr-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/salesbuildr/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/salesbuildr/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update salesbuildr@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/salesbuildr --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/salesbuildr --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `salesbuildr-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the salesbuildr skill from https://github.com/servosity/msp-skills/tree/main/skills/salesbuildr. The skill defines how its required CLI (`salesbuildr-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Salesbuildr portal):

```bash
SALESBUILDR_API_KEY=<value> SALESBUILDR_TENANT=<value> salesbuildr-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which sent or approved quotes are aging, and how much is at risk? | `salesbuildr-cli quote stale --days 14` |
| Which quote line items are priced below my markup floor? | `salesbuildr-cli quote thin --floor 20` |
| How does my quote pipeline convert stage by stage, in count and dollars? | `salesbuildr-cli quote funnel` |
| Where have per-company pricing-book prices drifted from the master catalog? | `salesbuildr-cli pricing drift` |
| What's my win rate by owner, stage, or category? | `salesbuildr-cli opportunity winrate --by owner` |
| What's my probability-weighted recurring-revenue forecast? | `salesbuildr-cli opportunity mrr-forecast` |
| Which catalog products have I never quoted to a given company? | `salesbuildr-cli company whitespace "Acme Managed IT"` |
| Which records are missing the external ID my PSA sync depends on? | `salesbuildr-cli reconcile-psa` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Salesbuildr integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of the 80 quotes I sent this quarter are still open, aging, and worth following up before they expire?".

This skill syncs Salesbuildr into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `quote stale`, `quote thin`, and `company whitespace` join across quotes, line items, products, and pricing books - work a stateless API wrapper can't do.

## The pain this closes

Quotes go out and then go quiet. On a lean MSP nobody has time to babysit every sent proposal, so deals worth real money quietly age past their expiry - a recurring r/msp complaint about quote follow-up. Meanwhile margin leaks one line at a time: a tech swaps in a part at cost, forgets the markup, and you don't catch it until the QBR. And per-company pricing books drift from the master catalog until two clients pay different prices for the same SKU.

This skill turns those into one-line questions:

- **`salesbuildr-cli quote stale --days 14`** - every sent/approved quote aging past your cutoff, with the dollar value still at risk.
- **`salesbuildr-cli quote thin --floor 20`** - every open line priced below your markup floor, before it ships.
- **`salesbuildr-cli pricing drift`** - per-company prices that have diverged from the master catalog cost or price.
- **`salesbuildr-cli company whitespace "Acme Managed IT"`** - catalog products a client has never been quoted: the cross-sell you keep meaning to make.
- **`salesbuildr-cli reconcile-psa`** - records missing the external ID your Autotask/ConnectWise sync needs.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Salesbuildr MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Salesbuildr credentials once.

### Is my Salesbuildr data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hammer my Salesbuildr API rate limits?

No. The analytics read a **local SQLite mirror**, not the live API - you `sync` once, then ask as many questions as you want offline with zero further API calls. The sync itself honors a configurable `--rate-limit`, and you control how often it runs.

### Do I need to be a Salesbuildr customer, and will this replace my PSA sync?

You need a Salesbuildr account, a Public API key from your portal, and your tenant subdomain. It does **not** replace your Autotask/ConnectWise sync - it reads your Salesbuildr data and flags the records missing the external ID that sync depends on (`reconcile-psa`), so you can fix the gaps. It never writes to your PSA.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | every `public-get`/`public-get-list`, the analytics (`quote stale`, `quote thin`, `quote funnel`, `pricing drift`, `opportunity velocity`/`winrate`/`mrr-forecast`, `product velocity`, `company whitespace`), `reconcile-psa`, `search`, `sql`, `export`, and `sync` (local mirror only) | Allow |
| Write (routine) | company/contact/product/pricing-book/quote/quote-template create & update, the upserts, `opportunity public-win`/`public-lose`/`public-upsert`, `field public-update-values`, `product inventory`, the contact undeletes, and the bulk `import` - 30 routine-write commands in all | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | the 6 delete commands (`company`/`contact`/`product public-delete*`), plus the local credential commands (`auth set-token`, `auth setup`, `auth logout`) | Human-in-the-loop only |

The strongest control is the **scope you grant the Salesbuildr credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Salesbuildr API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-07._
