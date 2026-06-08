# Pax8 + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Pax8
> API. Not affiliated with, endorsed by, or sponsored by Pax8, Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/pax8/">
    <img src="../../docs/assets/video/pax8/animated-og.gif" alt="Pax8 demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/pax8/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Pax8 Partner API endpoint, plus an offline store that reconciles billing, tracks MRR, and catches usage overages no Pax8 tool surfaces. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Pax8 skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `pax8-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `pax8-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `pax8-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Pax8 MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `pax8-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Pax8 MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/pax8-v0.1.0/pax8-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Pax8 release on the [releases page](https://github.com/servosity/msp-skills/releases?q=pax8).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install pax8@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Pax8 Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.ps1 | iex`. Then authenticate per the README and run `pax8-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.sh)
```

The installer drops both `pax8-cli` (the CLI) and `pax8-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
pax8-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update pax8@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/pax8 --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/pax8 --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `pax8-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the pax8 skill from https://github.com/servosity/msp-skills/tree/main/skills/pax8. The skill defines how its required CLI (`pax8-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Create an OAuth2 Client ID and Secret in the Pax8 portal under **Integrations**, then set the two credentials the CLI needs:

```bash
PAX8_CLIENT_ID=<value> PAX8_CLIENT_SECRET=<value> pax8-cli doctor
```

`PAX8_AUDIENCE` (default `api://p8p.client`) and `PAX8_OAUTH_SCOPE` are optional overrides for non-standard tenants. `doctor` confirms the credentials work before you run anything that touches data; `pax8-cli auth login` mints and caches the bearer token interactively.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Where is my billing leaking - billed for a cancelled product, or active but never invoiced? | `pax8-cli reconcile` |
| Can I catch that leakage before the next invoice finalizes? | `pax8-cli reconcile --draft` |
| What is my MRR and margin right now, broken down by product? | `pax8-cli mrr` |
| Which usage is about to overage before it hits the customer invoice? | `pax8-cli overage` |
| What changed in my book of business this week - new, cancelled, resized subscriptions? | `pax8-cli since 7d` |
| Which customers cost the most across every invoice? | `pax8-cli spend` |
| Everything about one customer - subscriptions, contacts, invoices, usage - in one view? | `pax8-cli company show <companyId>` |
| Which products can I resell that match a vendor or keyword? | `pax8-cli search "microsoft 365"` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Pax8 integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking where margin leaked across every customer last month, or which usage lines are about to overage before they invoice.

This skill syncs Pax8 into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `reconcile`, `mrr`, and `company show` join across invoices, subscriptions, products, and usage - work a stateless API wrapper can't do.

## The pain this closes

Pax8 is excellent at provisioning and ordering. It is not good at telling you, in one place, where the money is leaking. The marketplace hands you a monthly invoice file but never joins those lines back to your active subscriptions, so a line billing for a cancelled product - or a live subscription that never got invoiced - is yours to find by hand. Automation vendor Bumblebee profiled an MSP that spent **five days every month** reconciling Pax8 invoices against their PSA before automating it ([hirebumblebee.com](https://www.hirebumblebee.com/blogs/pax8-integration-guide-partnership)); Pax8's own developer docs are candid that mapping invoice lines back to subscriptions is real integration work ([devx.pax8.com](https://devx.pax8.com/docs/invoice-billing-integrations)).

This skill closes that gap from the local mirror:

- `pax8-cli reconcile` - invoice lines with no active subscription, and active subscriptions never billed (`--draft` pre-checks the next unposted invoice)
- `pax8-cli mrr` - recurring revenue and margin by product, the spreadsheet you stop rebuilding
- `pax8-cli overage` - usage running hot before it posts to the customer invoice
- `pax8-cli spend` - customers ranked by total spend across invoices
- `pax8-cli company show <companyId>` - one customer's subscriptions, contacts, invoices, and usage in a single view

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Pax8 MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Pax8 credentials once.

### Is my Pax8 data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need to be a Pax8 partner, and what credentials does it use?

Yes - it talks to the Pax8 Partner API with your own partner credentials. Create an OAuth2 Client ID and Client Secret in the Pax8 portal under **Integrations**, then set `PAX8_CLIENT_ID` and `PAX8_CLIENT_SECRET`. The CLI exchanges them for a bearer token at Pax8's token endpoint and caches it. `PAX8_AUDIENCE` (default `api://p8p.client`) and `PAX8_OAUTH_SCOPE` are optional overrides. The credential's own permissions are the real boundary - scope it to what you want the AI to reach.

### Will this hit my Pax8 API rate limits?

After the first sync, the analytics commands (`reconcile`, `mrr`, `overage`, `spend`, `since`, `company show`, `search`) run against your **local SQLite mirror** with zero API calls. Live calls respect a `--rate-limit` throttle, and sync is incremental - it only fetches what changed since the last checkpoint.

### Does this replace the Pax8 portal?

No. Provisioning, ordering, and support workflows stay in the portal. This skill answers the cross-entity billing and revenue questions the portal cannot compose in one place - reconciliation, MRR and margin, usage overages, customer-360 - from your terminal or agent.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `pax8-cli reconcile`, `pax8-cli mrr`, `pax8-cli overage`, `pax8-cli spend`, `pax8-cli since 7d`, `pax8-cli company show <companyId>`, `pax8-cli search "microsoft 365"`, `pax8-cli companies find`, `pax8-cli subscriptions find` | Allow |
| Write (routine) | `pax8-cli companies create-company`, `pax8-cli companies update-company`, `pax8-cli companies contacts create`, `pax8-cli subscriptions update`, `pax8-cli orders create` - writes send immediately; `--dry-run` is an opt-in preview, not a default | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | `pax8-cli subscriptions delete` (cancels a subscription), `pax8-cli companies contacts delete` | Human-in-the-loop only |

The strongest control is the **scope you grant the Pax8 credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Pax8 API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-05._
