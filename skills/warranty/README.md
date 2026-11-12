# Dell, Lenovo and Apple + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Dell, Lenovo and Apple
> API. Not affiliated with, endorsed by, or sponsored by Dell Inc., Lenovo and Apple Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/warranty/">
    <img src="../../docs/assets/video/warranty/animated-og.gif" alt="Device Warranty demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/warranty/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Dell, Lenovo and Apple warranty end dates for every client's devices, in one local mirror. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Dell, Lenovo and Apple skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `warranty-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `warranty-mcp` over HTTPS with `WARRANTY_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `warranty-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `warranty-mcp --transport http` with `WARRANTY_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `warranty-mcp` over HTTPS with `WARRANTY_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Dell, Lenovo and Apple MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/warranty-v0.1.5/warranty-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Dell, Lenovo and Apple release on the [releases page](https://github.com/servosity/msp-skills/releases?q=warranty).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install warranty@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Dell, Lenovo and Apple Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/warranty/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/warranty/install.ps1 | iex`. Then authenticate per the README and run `warranty-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/warranty/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/warranty/install.sh)
```

The installer drops both `warranty-cli` (the CLI) and `warranty-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
warranty-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/warranty/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/warranty/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update warranty@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/warranty --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/warranty --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `warranty-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the warranty skill from https://github.com/servosity/msp-skills/tree/main/skills/warranty. The skill defines how its required CLI (`warranty-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Every vendor is optional: set the ones you have, then run `doctor`.

**macOS / Linux:**

```bash
export WARRANTY_DELL_CLIENT_ID=<client id> WARRANTY_DELL_CLIENT_SECRET=<client secret>   # Dell TechDirect
export WARRANTY_LENOVO_CLIENT_KEY=<ClientID token>                                    # Lenovo eSupport
export WARRANTY_APPLE_CLIENT_ID=<BUSINESSAPI.uuid> WARRANTY_APPLE_KEY_ID=<key id> WARRANTY_APPLE_PRIVATE_KEY=~/keys/apple-business.pem   # Apple Business
warranty-cli doctor --json
```

**Windows (PowerShell):**

```powershell
$env:WARRANTY_DELL_CLIENT_ID = "<client id>"; $env:WARRANTY_DELL_CLIENT_SECRET = "<client secret>"
$env:WARRANTY_LENOVO_CLIENT_KEY = "<ClientID token>"
$env:WARRANTY_APPLE_CLIENT_ID = "<BUSINESSAPI.uuid>"; $env:WARRANTY_APPLE_KEY_ID = "<key id>"; $env:WARRANTY_APPLE_PRIVATE_KEY = "$HOME\keys\apple-business.pem"
warranty-cli doctor --json
```

`doctor --json` lists each vendor under `vendors` with what it still needs, without printing any value. With a Dell credential it spends one read of Dell's entitlement endpoint (service tag `0000000`) and reports whether Dell accepted it. It does not prove a Lenovo or Apple key works: run a `lookup` for that. Dell credentials can also be stored with `warranty-cli auth login`.

## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which client devices are out of warranty or expire in the next 90 days? | `warranty-cli report --within 90d --agent` |
| Which of one client's devices have already expired? | `warranty-cli report --customer "Acme Dental" --status expired --agent` |
| What is the warranty on these serials right now? | `warranty-cli lookup 1ABC234 PF1ABCDE --agent` |
| Look up every device in this RMM export for a client | `warranty-cli lookup --file devices.csv --customer "Acme Dental" --agent` |
| How many devices per client lose coverage each quarter next year? | `warranty-cli lifecycle --quarters 4 --age 4y --agent` |
| Which devices could the report not speak for, and why? | `warranty-cli unresolved --agent` |
| Which coverage expired or changed since I last checked? | `warranty-cli events list --since 0 --agent` |
| Refresh only the stale and soon-expiring devices | `warranty-cli recheck --older-than 30d --expiring 120d --max-calls 50` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Warranty lookups are usually one serial in one vendor portal, or an API wrapper that forwards each question as one live call. That's fine for one device. It dies at scale, when you're asking "which devices across all 40 clients lose coverage before the end of next quarter, from Dell, Lenovo and Apple at once?"

This skill keeps every answer in a **local warranty mirror** per client. Fleet questions become one local query: instant, offline, and the AI sees the answer, not the raw data. `report`, `lifecycle` and `unresolved` join devices, clients and coverage lines from all three vendors - work a stateless API wrapper can't do.

## The pain this closes

Dell's own community has asked for years for a bulk answer: "Is there an easy way to enter multiple service tags to check the warranty status on all of my hardware?" ([Dell Community](https://www.dell.com/community/Latitude/Checking-Multiple-Service-Tags/td-p/8194966), May 2022; one reply had over 1,000 computers to check). For an MSP the question repeats per client and per vendor, in three portals with three credentials.

- `warranty-cli import devices.csv --customer "Acme Dental"` - load an RMM export under the client's name.
- `warranty-cli lookup --file devices.csv --customer "Acme Dental"` - every serial, with whichever vendor credentials you have.
- `warranty-cli report --within 90d` - expired and expiring devices per client, offline.
- `warranty-cli lifecycle --quarters 4` - coverage endings by quarter for refresh planning.
- `warranty-cli unresolved` - what could not be verified, each with one reason.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Dell, Lenovo and Apple MCP server runs locally over stdio by default. Its HTTP mode (`warranty-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `WARRANTY_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Dell, Lenovo and Apple credentials once.

### Is my Dell, Lenovo and Apple data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need credentials for all three vendors?

No. Every vendor is optional. Dell needs a TechDirect Warranty API client ID and secret, Lenovo a ClientID token from your Lenovo account manager, Apple an Apple Business API account in the client's organization. A vendor without credentials is reported as `vendor_unconfigured` and never called; the others keep working. `warranty-cli doctor --json` lists what each vendor still needs.

### Where do the serials come from?

Any CSV or JSON device export with a serial column (manufacturer, model and hostname are optional), for example from your RMM: `warranty-cli import devices.csv --customer "Acme Dental"`. `apple-pull` loads a client's Apple devices straight from Apple Business.

### Will this hit the vendors' rate limits?

It is built to stay inside them: Dell lookups go 100 service tags per call, Lenovo 25 serials per call spaced 250 ms apart, Apple one serial per call. `recheck` refreshes only stale or soon-expiring devices, and `--max-calls` caps a run.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `report`, `lifecycle`, `unresolved`, `events list`, `doctor`, `dell entitlements`, `lenovo warranty`, `apple coverage` | Allow |
| Write (local mirror only) | `import`, `lookup`, `recheck`, `apple-pull`, `sync` | Preview with `--dry-run`, then a reviewed write; over MCP a save needs `confirm` |
| Credential | `auth login`, `auth set-token`, `auth logout`, `auth status` | Human-in-the-loop only |
| Destructive | `local-data purge` | Human-in-the-loop only, explicit confirmation |

Nothing in this skill changes anything at Dell, Lenovo or Apple.

The strongest control is **which vendor credentials you configure** - a vendor without credentials is never called, and each credential is sent only to its own vendor's host. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Built and tested offline from each vendor's documented request and response formats (no live vendor account has run it yet), and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-29._
