# Sherweb + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Sherweb
> API. Not affiliated with, endorsed by, or sponsored by Sherweb Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/sherweb/">
    <img src="../../docs/assets/video/sherweb/animated-og.gif" alt="Sherweb demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/sherweb/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Sherweb Partner API capability, plus a local SQLite store, offline analytics, and margin/drift/orphan joins no other Sherweb tool has. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Sherweb skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `sherweb-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `sherweb-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `sherweb-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Sherweb MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `sherweb-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Sherweb MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/sherweb-v4.22.0/sherweb-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Sherweb release on the [releases page](https://github.com/servosity/msp-skills/releases?q=sherweb).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install sherweb@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Sherweb Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sherweb/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sherweb/install.ps1 | iex`. Then authenticate per the README and run `sherweb-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sherweb/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sherweb/install.sh)
```

The installer drops both `sherweb-cli` (the CLI) and `sherweb-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
sherweb-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sherweb/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sherweb/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update sherweb@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/sherweb --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/sherweb --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `sherweb-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the sherweb skill from https://github.com/servosity/msp-skills/tree/main/skills/sherweb. The skill defines how its required CLI (`sherweb-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Sherweb portal):

```bash
SHERWEB_CLIENT_ID=<value> SHERWEB_CLIENT_SECRET=<value> SHERWEB_OAUTH_SCOPE=<value> SHERWEB_SUBSCRIPTION_KEY=<value> sherweb-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What's my net margin per customer this month - receivable minus payable? | `sherweb-cli margin --month 2026-04` |
| Whose margin is sliding month over month before an account goes negative? | `sherweb-cli margin-trend --last 6` |
| Which active subscriptions am I paying Sherweb for but not billing the customer? | `sherweb-cli orphans` |
| Where am I absorbing metered usage I never billed back? | `sherweb-cli usage-leak` |
| Which subscriptions have more seats paid than seats actually used? | `sherweb-cli right-size` |
| What changed on my payable charges since the last sync? | `sherweb-cli drift` |
| What was added, cancelled, or resized across my whole book this month? | `sherweb-cli sub-changes --since 30d` |
| How many total seats of each product do I carry across every customer? | `sherweb-cli fleet-subs --product "Microsoft 365"` |
| What will a seat change cost before I submit the amendment? | `sherweb-cli amend-preview --sub "SUB123" --qty 25` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Sherweb integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "what's my net margin across all 80 customers this month" and the answer means stitching the Distributor billing endpoints to per-customer receivable charges, subscriptions, and usage one client at a time.

This skill syncs Sherweb into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `margin`, `orphans`, and `usage-leak` join across payable charges, receivable charges, subscriptions, and metered usage - work a stateless API wrapper can't do.

## The pain this closes

The Sherweb Partner API splits billing across two surfaces: the **Distributor API** returns the *payable* charges (what you owe Sherweb) and the **Service Provider API** returns the *receivable* charges and subscriptions (what you bill clients). No portal screen joins them, so net margin per customer gets rebuilt by hand in a spreadsheet every close. The recurring r/msp version is "license sprawl" - a client downsizes, the subscription stays active on the Sherweb side, and you keep paying for seats you never bill back. This skill closes that gap from your synced mirror:

- `sherweb-cli margin` - net margin per customer (receivable minus payable), worst-margin-first.
- `sherweb-cli orphans` - active subscriptions with zero receivable charges: seats you pay for but never bill.
- `sherweb-cli usage-leak` - metered usage with no matching receivable charge: consumption you absorb.
- `sherweb-cli right-size` - subscriptions where paid seats differ from metered seats used.
- `sherweb-cli margin-trend --last 6` - each customer's margin across recent closes, steepest-decline-first.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Sherweb MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Sherweb credentials once.

### Is my Sherweb data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need to be a Sherweb partner, and what credentials does it use?

Yes - it talks to the Sherweb Partner API with your own partner credentials, using **composed authentication**. You need an OAuth2 client-credentials Client ID and Secret (with a scope) for the bearer token, **plus** an APIM gateway subscription key that rides on every call. Create the OAuth2 application and copy the subscription key from `cumulus.sherweb.com` under **Security > APIs**, then set `SHERWEB_CLIENT_ID`, `SHERWEB_CLIENT_SECRET`, `SHERWEB_OAUTH_SCOPE`, and `SHERWEB_SUBSCRIPTION_KEY`. The credential's own permissions are the real boundary - scope it to what you want the AI to reach.

### Will this hit my Sherweb API rate limits?

After `deep-sync`, the analytics commands (`margin`, `margin-trend`, `orphans`, `usage-leak`, `right-size`, `drift`, `sub-changes`, `fleet-subs`, `amend-preview`) run against your local SQLite mirror with **zero API calls**. Live calls respect a `--rate-limit` throttle, and `sync` is resumable and incremental - it only fetches what changed since the last checkpoint.

### Does this replace the Sherweb portal?

No. Provisioning, ordering, and subscription management stay in the portal. This skill answers the cross-entity margin and billing questions the portal cannot compose in one place - net margin, orphaned subscriptions, usage leakage, seat right-sizing - from your terminal or agent.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `margin`, `margin-trend`, `orphans`, `usage-leak`, `right-size`, `drift`, `sub-changes`, `fleet-subs`, `amend-preview`, `distributor`, and the `service-provider list-*` / `get-*` / `validate-order` queries | Allow |
| Write (routine) | `service-provider amend-subscriptions` (seat-quantity change), `service-provider place-order` (marketplace order), `import` (POSTs each record) - writes send immediately; `--dry-run` is an opt-in preview, not a default | Preview with `--dry-run`, then a reviewed write |
| Destructive / credential | `service-provider cancel-subscriptions` (cancels a customer's subscriptions); `auth login` / `auth logout` / `auth set-token` (credential changes) | Human-in-the-loop only |

The strongest control is the **scope you grant the Sherweb credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Sherweb API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
