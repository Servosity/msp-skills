# Gradient MSP + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Gradient MSP
> API. Not affiliated with, endorsed by, or sponsored by Gradient MSP, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/gradient/">
    <img src="../../docs/assets/video/gradient/animated-og.gif" alt="Gradient MSP demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/gradient/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

The Gradient MSP Synthesize vendor API from your terminal: bulk usage pushes with a single billing rebuild, a local push-drift ledger, and alert-to-ticket tracing the portal and SDK don't offer. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Gradient MSP skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `gradient-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `gradient-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `gradient-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Gradient MSP MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `gradient-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Gradient MSP MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/gradient-v0.1.0/gradient-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Gradient MSP release on the [releases page](https://github.com/servosity/msp-skills/releases?q=gradient).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install gradient@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Gradient MSP Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.ps1 | iex`. Then authenticate per the README and run `gradient-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.sh)
```

The installer drops both `gradient-cli` (the CLI) and `gradient-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
gradient-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update gradient@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/gradient --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/gradient --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `gradient-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the gradient skill from https://github.com/servosity/msp-skills/tree/main/skills/gradient. The skill defines how its required CLI (`gradient-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Gradient MSP portal):

```bash
GRADIENT_TOKEN=<value> gradient-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Push a whole file of usage counts and rebuild billing exactly once? | `gradient-cli usage push --file ./counts.csv --agent` |
| Which accounts' usage changed between my last two pushes? | `gradient-cli usage drift --agent` |
| Send an alert and confirm the PSA ticket was actually created? | `gradient-cli alert send --account "123456789" --title "Backup failure" --wait --agent` |
| Which of my dispatched alerts never became tickets? | `gradient-cli alert trace --stuck --agent` |
| Which accounts are unmapped or missing a vendor SKU? | `gradient-cli hygiene unmapped --agent` |
| Is my integration ready to flip to active? | `gradient-cli status ready --agent` |
| Add a single ad-hoc unit count for one account and service? | `gradient-cli billing <serviceId> --account-id "123456789" --unit-count 42` |
| Are my credentials valid and what's my integration status? | `gradient-cli integration get --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Gradient MSP integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking which accounts drifted in billable units since your last push, or how many of 200 dispatched alerts never became PSA tickets.

This skill syncs Gradient MSP into a **local SQLite mirror** with full-text search, and records every count you push in a local ledger. Aggregate questions become one local lookup: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `usage drift`, `hygiene unmapped`, and `alert trace --stuck` join across accounts, service/SKU mappings, and the push ledger - work a stateless API wrapper can't do.

## The pain this closes

Billing reconciliation is the recurring tax on MSP margin: every cycle someone pulls a CSV from each vendor, hand-maps the fields to the PSA, and hopes they caught every seat that drifted. It's a standing topic on r/msp, and Gradient MSP's own research frames the manual version as costing up to 90% more time than automating it - which is why 1,000+ MSPs run Synthesize. But Synthesize's only programmatic surface is a PowerShell SDK that wants a script project per integration, so the pushes that drive reconciliation live in brittle one-off scripts with no record of what was sent.

This skill closes that gap:

- **`usage push`** - push a whole CSV or JSON file of counts in one shot, with exactly one billing rebuild.
- **`usage drift`** - the pre-invoice pre-flight: which accounts' counts changed between your last two pushes.
- **`alert send --wait`** / **`alert trace --stuck`** - dispatch an alert and block until the PSA ticket exists; list the ones that never landed.
- **`hygiene unmapped`** - one work-queue rollup of every unmapped account and SKU gap.
- **`status ready`** - go/no-go before you flip the integration to active.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Gradient MSP MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Gradient MSP credentials once.

### Is my Gradient MSP data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Which credentials do I need, and how do I set them?

A Synthesize vendor token. Set `GRADIENT_TOKEN` to the base64 of `<vendorApiKey>:<partnerApiKey>`, or set `GRADIENT_VENDOR_API_KEY` and `GRADIENT_PARTNER_API_KEY` and the CLI derives the token for you. `GRADIENT_BASE_URL` is optional. Run `gradient-cli integration get` to confirm the credential works.

### Does this replace the Synthesize portal or Managed Billing Reconciliation?

No. Mapping approval and reconciliation review still happen in the Synthesize portal (or are handled by Gradient's MBR service). This CLI is the vendor and integration side: it pushes accounts, services, and usage counts in, audits what was pushed, and traces alerts to PSA tickets. It complements the portal, it doesn't log into it.

### Will this hit my Gradient MSP API rate limits?

The CLI talks to the live API only when you ask it to, and the bulk `usage push` defers the billing rebuild to a single call at the end instead of one per row. Read-heavy work runs against the local SQLite mirror after a `sync`. A global `--rate-limit` flag caps requests per second when you need it.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `integration get`, `vendor get`, `services get`, `accounts list`, `usage drift`, `alert trace`, `hygiene unmapped`, `status ready`, `analytics` | Allow |
| Write (routine) | `usage push`, `billing`, `alert send`, `alerting`, `import`, `accounts create`, `accounts update`, `accounts update-one`, `services create`, `mappings create`, `mappings update`, `mappings update-bulk`, `integration update-status`, `vendor update` | Preview with `--dry-run`, then a reviewed write |
| Credential / config | `auth set-token`, `auth logout` (local credential file only) | Human-in-the-loop only |
| Destructive | none - the Synthesize vendor API exposes no delete commands | Human-in-the-loop only |

The strongest control is the **scope you grant the Gradient MSP credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Gradient MSP API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-07._
