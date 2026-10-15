# QuoteWerks + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the QuoteWerks
> API. Not affiliated with, endorsed by, or sponsored by Aspire Technologies, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/quotewerks/">
    <img src="../../docs/assets/video/quotewerks/animated-og.gif" alt="QuoteWerks demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/quotewerks/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Typed quote tools for QuoteWerks Web that use the real column names, preview typed writes before sending them, and add margin, win-rate and pipeline answers from a local mirror. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the QuoteWerks skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `quotewerks-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `quotewerks-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `quotewerks-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, set `QUOTEWERKS_MCP_HTTP_TOKEN`, run `quotewerks-mcp --transport http`, and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `quotewerks-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download QuoteWerks MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/quotewerks-v0.1.0/quotewerks-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every QuoteWerks release on the [releases page](https://github.com/servosity/msp-skills/releases?q=quotewerks).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install quotewerks@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the QuoteWerks Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.ps1 | iex`. Then authenticate per the README and run `quotewerks-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.sh)
```

The installer drops both `quotewerks-cli` (the CLI) and `quotewerks-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
quotewerks-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update quotewerks@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/quotewerks --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/quotewerks --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `quotewerks-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the quotewerks skill from https://github.com/servosity/msp-skills/tree/main/skills/quotewerks. The skill defines how its required CLI (`quotewerks-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Create an API key in QuoteWerks Web under **Setup > Options > Zapier and API** (the API requires a user with MasterRights). Tick **Allow write access** on the key only if you want the agent to create or change documents; a read-only key is enough for every lookup and report. Then:

```bash
export QUOTEWERKS_API_KEY=<your-key>
quotewerks-cli doctor
```

`doctor` checks config and paths and verifies the key with one read (`GET /api/v1/qw/tables`): a rejected key shows as `ERROR rejected`. It exits 0 by default either way; add `--fail-on error` to make a failed check exit non-zero in a script.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Where does the Acme quote stand? | `quotewerks-cli docs search --customer "Acme" --type QUOTE` |
| What is on quote 1042, line by line, with margin per line? | `quotewerks-cli docs get "1042"` |
| Is anything on this quote below cost or under 10% margin before it goes out? | `quotewerks-cli docs lint "1042" --min-margin 10` |
| What does an 18% margin on these laptops do to the quote? (preview, nothing written) | `quotewerks-cli items reprice "1042" --part 20UES0X100 --margin 18` |
| Copy last March's order into a new quote with 12 laptops at today's prices (preview) | `quotewerks-cli requote "1042" --qty 20UES0X100=12` |
| Which open quotes are 30+ days old, and who do I chase? | `quotewerks-cli pipeline --older-than 30d` |
| What is our win rate by rep over the last 90 days? | `quotewerks-cli winrate --by rep --since 90d` |
| Which open quotes carry a cost that no longer matches our product database? | `quotewerks-cli cost-drift --threshold 2` |

The pipeline, win-rate, margin, cost-drift and changes reports read a local copy: run `quotewerks-cli mirror` first, then again whenever you want fresh numbers. Writes (`docs create`, `items add`, `items reprice`, `requote`) only preview until you add `--confirm`.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most QuoteWerks integrations, the Zapier app included, make one live API call per record. That's fine for one record. It dies at scale, when you're asking "what was margin by rep across every order last quarter, and which open quotes went thin when the distributor raised cost?"

This skill copies your QuoteWerks documents, line items and product databases into a **local SQLite mirror** (`quotewerks-cli mirror`). Aggregate questions become one local query: instant, offline, and the AI sees the answer, not the raw data. `cost-drift` joins every open-quote line with your product databases, and `margin --by manufacturer` and `winrate` roll up headers and lines across every mirrored document - work a stateless API wrapper can't do. Compared with the raw REST API and the [Zapier QuoteWerks app](https://zapier.com/apps/quotewerks/integrations), which work one record at a time, it also adds typed quote commands that use the real QuoteWerks column names and preview every typed write before it is sent.

## The pain this closes

An MSP account manager [asked QuoteWerks for an official MCP server](https://tickets.quotewerks.com/support/discussions/topics/9000064305) and listed the four jobs that eat the day:

- **Quoting from an email.** "12 more laptops like the ones you sent in March, plus docks" means hunting down the March document and hand-keying every line into a new quote.
- **Quote status.** "Where does the Acme quote stand?" means leaving what you are doing to run a Document Lookup.
- **Repricing.** When distributor cost moves, someone opens each document and touches every line.
- **QBR numbers.** Margin and win rate mean a script or a report per question.

The same post names why the beta REST API alone does not fix it: it is generic table access, so an AI has to guess the schema, and a wrong guess quietly produces a bad quote. This skill answers each job with one command:

- **`quotewerks-cli requote "1042" --qty 20UES0X100=12`** - the March order copied into a new quote with new quantities and today's product-database prices, previewed before anything is created.
- **`quotewerks-cli docs search --customer "Acme" --type QUOTE`** - quote status from the window you are already in.
- **`quotewerks-cli cost-drift --threshold 2`**, then **`items reprice`** - every open-quote line whose cost moved, with margin at the old and new cost, and an old-versus-new diff before any price changes.
- **`quotewerks-cli margin --by rep --since 90d`** and **`winrate --by rep`** - QBR answers from the local mirror.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The QuoteWerks MCP server runs locally, but it speaks HTTP natively: set `QUOTEWERKS_MCP_HTTP_TOKEN` to a bearer token of your choosing, start it with `quotewerks-mcp --transport http` (it serves `http://127.0.0.1:7777/mcp` by default and refuses requests without that token), and put that endpoint behind an HTTPS tunnel or your own reverse proxy. No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your QuoteWerks credentials once.

### Is my QuoteWerks data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Your API key is read from your environment or your agent's config, or from the CLI's credentials file (mode 0600) if you saved it with `quotewerks-cli auth set-token`; it is never bundled into this repo or transmitted anywhere by MSP Skills.

### Does it work with QuoteWerks Desktop?

It works with QuoteWerks Web through the QuoteWerks Web REST API at `qwwapi.quotewerks.com`, which QuoteWerks labels beta. It does not open a QuoteWerks Desktop database or use the desktop COM API.

### What API key do I need, and can the AI change my quotes?

Create a key in QuoteWerks Web under **Setup > Options > Zapier and API**; the API requires a user with MasterRights. A key without **Allow write access** can read but cannot change anything (QuoteWerks answers writes with 403). Even with a write-enabled key, `docs create`, `items add`, `items reprice` and `requote` only preview until you add `--confirm`. The raw `tables insert`, `tables update` and `tables delete` commands send immediately, so keep the key read-only unless you need writes.

### Will this hammer the QuoteWerks API?

No. `margin`, `winrate`, `pipeline`, `cost-drift` and `changes` read the local copy that `quotewerks-cli mirror` makes, so you can ask as many questions as you like without another API call. `mirror` reads 1,000 rows per request, and requests are paced to the server's rate-limit headers by default.

### How is this different from QuoteWerks Web's own AI agent?

QuoteWerks Web has its own AI agent, and this skill can ask it questions too (`quotewerks-cli agent ask`). What the skill adds is reach from the AI you already work in: previewed quote builds and repricing, a per-line audit before a quote goes out, and analysis across every mirrored document. It complements QuoteWerks Web; it does not replace it.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `docs search`, `docs get`, `docs lint`, `items list`, `products search`, the `tables` reads (`list`, `rows`, `row`, `query`, `schema`, `values`), the `cloud-settings` reads, `reports run`, `identity userinfo`, `agent ask` (and `import agent`, CLI only, which sends a JSONL file of questions to the same AI agent), and the mirror reports (`margin`, `pipeline`, `cost-drift`, `winrate`, `changes`); `mirror` and `sync` write only the local copy | Allow |
| Previewed writes | `docs create`, `items add`, `items reprice`, `requote`: each previews by default and writes only with `--confirm` and a write-enabled key | Let the agent preview; a human approves before it re-runs with `--confirm` |
| Raw writes | `tables insert` and `tables update` (CLI only, never an MCP tool), which send immediately to any table your key can write | Preview with `--dry-run`, then a reviewed write |
| Destructive / credential | `tables delete` (CLI only; deletes a row in any table, documents included), `auth set-token`, `auth logout` | Human-in-the-loop only |

The strongest control is the **key you create**: leave **Allow write access** off and nothing the agent runs can change a quote. The CLI can only do what the key is permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta, and so is the QuoteWerks Web REST API it calls. Built from the vendor's published API spec and documentation and tested against a local stand-in for the API; no live QuoteWerks Web tenant has run it yet. It is being validated with MSPs running it against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-14._
