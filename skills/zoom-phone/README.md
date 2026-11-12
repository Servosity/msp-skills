# Zoom Phone + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Zoom
> Phone API. Not affiliated with, endorsed by, or sponsored by Zoom Communications, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/zoom-phone/">
    <img src="../../docs/assets/video/zoom-phone/animated-og.gif" alt="Zoom Phone demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/zoom-phone/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Zoom Phone admin operation plus E911, routing and footprint audits, across every client account. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Zoom Phone skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `zoom-phone-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `zoom-phone-mcp` over HTTPS with `ZOOM_PHONE_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `zoom-phone-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `zoom-phone-mcp --transport http` with `ZOOM_PHONE_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `zoom-phone-mcp` over HTTPS with `ZOOM_PHONE_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Zoom Phone MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/zoom-phone-v0.1.5/zoom-phone-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Zoom Phone release on the [releases page](https://github.com/servosity/msp-skills/releases?q=zoom-phone).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install zoom-phone@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Zoom Phone Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zoom-phone/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zoom-phone/install.ps1 | iex`. Then authenticate per the README and run `zoom-phone-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zoom-phone/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zoom-phone/install.sh)
```

The installer drops both `zoom-phone-cli` (the CLI) and `zoom-phone-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
zoom-phone-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zoom-phone/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zoom-phone/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update zoom-phone@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/zoom-phone --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/zoom-phone --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `zoom-phone-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the zoom-phone skill from https://github.com/servosity/msp-skills/tree/main/skills/zoom-phone. The skill defines how its required CLI (`zoom-phone-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Create a Server-to-Server OAuth app in each client's Zoom App Marketplace (Develop > Build App > Server-to-Server OAuth), add the phone admin scopes you need, and copy its Account ID, Client ID and Client Secret.

**macOS / Linux:**

```bash
ZOOM_PHONE_ACCOUNT_ID=<account-id> ZOOM_PHONE_CLIENT_ID=<client-id> ZOOM_PHONE_CLIENT_SECRET=<client-secret> zoom-phone-cli doctor
```

**Windows (PowerShell):**

```powershell
$env:ZOOM_PHONE_ACCOUNT_ID="<account-id>"; $env:ZOOM_PHONE_CLIENT_ID="<client-id>"; $env:ZOOM_PHONE_CLIENT_SECRET="<client-secret>"; zoom-phone-cli doctor
```

Managing several clients? Save one profile per client Zoom account (the secret is read from stdin, never from the command line), then run any read across all of them:

```bash
printf '%s' "$ACME_SECRET" | zoom-phone-cli accounts connect acme --account-id <account-id> --client-id <client-id> --client-secret-stdin
zoom-phone-cli fleet -- e911 audit
```

`doctor` checks config, paths, and API reachability, and reports whether credentials are loaded - it does not validate them. Run a read command to confirm the credential actually works end-to-end.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which users, common areas and sites have a missing or unverified 911 address? | `zoom-phone-cli e911 audit --agent` |
| Where does a call to the main number ring, in business and closed hours? | `zoom-phone-cli routing trace +14155550123 --agent` |
| Which IVR keys point at nothing, and which queues have nobody to answer? | `zoom-phone-cli routing audit --agent` |
| Which numbers are unassigned, pending, or held by an inactive user? | `zoom-phone-cli numbers inventory --agent` |
| Which desk phones are offline, unassigned or on drifted firmware? | `zoom-phone-cli devices health --agent` |
| Which external callers missed us this week and are still waiting? | `zoom-phone-cli callbacks --since 7d --agent` |
| What does Jane hold, and what is the plan to remove it? | `zoom-phone-cli offboard jane@example.com --agent` |
| Run the E911 audit for every client account | `zoom-phone-cli fleet -- e911 audit` |

`e911 audit`, `numbers inventory` and `devices health` read the local mirror: run `zoom-phone-cli sync --full` first (each command names the exact resources it needs when they are missing). `routing`, `callbacks` and the group memberships in `footprint` and `offboard` read live data.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Zoom integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which users across all 30 client accounts have no verified 911 address".

This skill syncs Zoom into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `e911 audit`, `devices health` and `footprint` join users, sites, common areas, emergency addresses, numbers and desk phones - work a stateless API wrapper can't do. `fleet` runs the same read across every client account and returns one table with an account column.

## The pain this closes

Every client is a separate Zoom account with its own portal, and the questions that matter cross pages the portal keeps apart. Zoom's own [emergency services best practices](https://library.zoom.com/zoom-workplace/zoom-phone/expert-insights/emergency-services-best-practices/regulatory-considerations-in-the-united-states) note that in the United States the RAY BAUM's Act requires a dispatchable address with every emergency call, and that each user and device should have a default address. Checking that, finding dead IVR keys, and offboarding a user by hand means clicking through every tenant.

- `zoom-phone-cli e911 audit` - every missing or unverified emergency address, in one list.
- `zoom-phone-cli routing audit` - dead IVR targets and call queues with nobody to answer.
- `zoom-phone-cli offboard jane@example.com` - the ordered removal plan; `--apply` runs it after a live re-check.
- `zoom-phone-cli fleet -- e911 audit` - the same answer for every client account.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Zoom Phone MCP server runs locally over stdio by default. Its HTTP mode (`zoom-phone-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `ZOOM_PHONE_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Zoom credentials once.

### Is my Zoom data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config, then saved with the current token in `credentials.toml` in the CLI's data directory, readable only by your user; they are never bundled into this repo or transmitted anywhere by MSP Skills.

### What Zoom credentials does it need?

A Server-to-Server OAuth app created in each client's Zoom account, with the phone admin read scopes your workflow needs (add write scopes only if you want changes). The CLI uses its Account ID, Client ID and Client Secret and mints a one-hour token itself. Save one profile per client with `accounts connect`.

### Will it hit my Zoom rate limits?

Zoom Phone limits are per account and shared by every app. The E911, number and desk-phone audits read the local mirror, so after a sync they make no API calls. Live reads (routing, callbacks, footprint group membership) back off on HTTP 429 and honor Retry-After.

### Does Zoom have its own MCP server for Phone?

Zoom ships official MCP servers for Meetings, Team Chat and other products, but none for Zoom Phone. This one covers the whole Zoom Phone API and adds the cross-client audits above.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `e911 audit`, `routing trace`, `routing audit`, `numbers inventory`, `devices health`, `callbacks`, `footprint`, `offboard` (plan only), `sync`, every GET without a stored secret | Allow |
| Write (routine) | call forwarding, call queue members and settings, site and common-area updates | Preview with `--dry-run` (opt-in in the CLI), then a reviewed write |
| Destructive | every delete command, `offboard --apply`, `fleet --allow-writes` | Human-in-the-loop only |
| Endpoint, messaging and E911 | `devices reboot-phone`, firmware and provisioning changes, `sms post-message`, `whatsapp post-whats-app-message`, emergency address changes | Human-in-the-loop only |
| Credential / identity | any command that can return a stored secret field (`devices get-adevice`, `common-areas list-activation-codes`, `users phone-settings`), `roles` changes, `auth`, `accounts connect` | Human-in-the-loop only |

Two different gates. In the CLI, a write command sends its request as soon as it runs; `--dry-run` is opt-in and prints the request instead. Through the MCP server, a write runs only with `confirm: true` (otherwise the server returns the plan), and `ZOOM_PHONE_MCP_READ_ONLY=1` refuses every write and every secret-returning read.

The strongest control is the **scope you grant the Zoom credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Verified offline against the Zoom Phone API specification and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-28._
