# Apple Business + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Apple Business
> API. Not affiliated with, endorsed by, or sponsored by Apple Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/apple-business/">
    <img src="../../docs/assets/video/apple-business/animated-og.gif" alt="Apple Business demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/apple-business/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Apple Business API call, plus offline fleet answers: unassigned devices, AppleCare expiry and Activation Lock. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Apple Business skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `apple-business-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `apple-business-mcp` over HTTPS with `APPLE_BUSINESS_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `apple-business-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `apple-business-mcp --transport http` with `APPLE_BUSINESS_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `apple-business-mcp` over HTTPS with `APPLE_BUSINESS_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Apple Business MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/apple-business-v0.1.4/apple-business-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Apple Business release on the [releases page](https://github.com/servosity/msp-skills/releases?q=apple-business).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install apple-business@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Apple Business Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.ps1 | iex`. Then authenticate per the README and run `apple-business-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.sh)
```

The installer drops both `apple-business-cli` (the CLI) and `apple-business-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
apple-business-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update apple-business@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/apple-business --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/apple-business --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `apple-business-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the apple-business skill from https://github.com/servosity/msp-skills/tree/main/skills/apple-business. The skill defines how its required CLI (`apple-business-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Apple Business portal):

```bash
APPLE_BUSINESS_CLIENT_ID=<value> APPLE_BUSINESS_KEY_ID=<value> APPLE_BUSINESS_PRIVATE_KEY=<value> apple-business-cli doctor
```

`doctor` checks config, paths, and API reachability, and reports whether credentials are loaded - it does not validate them. Run a read command to confirm the credential actually works end-to-end.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which devices are not assigned to any MDM server, and is a missing default server the reason? | `apple-business-cli devices unassigned --added-within 30d --agent` |
| Which devices lose AppleCare in the next 90 days, grouped by purchase order? | `apple-business-cli applecare expiring --within 90d --group-by order --agent` |
| Which devices have no active AppleCare coverage at all? | `apple-business-cli applecare expiring --uncovered --agent` |
| Which devices are Activation Locked to a user, and who is the owner when known? | `apple-business-cli devices locked --type user --agent` |
| Per client org: how many devices, how many unassigned, expiring, uncovered and user-locked, and how many stale MDM servers? | `apple-business-cli fleet summary --within 90d --agent` |
| Which MDM servers stopped checking in with Apple but still own devices? | `apple-business-cli mdm-servers health --issues-only --agent` |
| Everything about one device: org record, MDM server, AppleCare and Activation Lock | `apple-business-cli devices lookup C02XL0ABJGH5 --agent` |
| Who changed what in the organization this week? | `apple-business-cli audit-events digest --since 7d --agent` |

The fleet reports read the local store: run `apple-business-cli sync` and `apple-business-cli devices enrich` first (an unsynced store answers empty).

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Apple Business integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking which devices across 40 client organizations lose AppleCare this quarter: AppleCare and Activation Lock are per-device endpoints.

This skill syncs Apple Business into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `fleet summary`, `applecare expiring --group-by order` and `devices unassigned` join devices, MDM servers, AppleCare coverage and Activation Lock across every customer org's store - work a stateless API wrapper can't do.

## The pain this closes

A device never assigned to the right MDM server in Apple Business Manager does not enroll, and the technician finds out at the client's desk ([Jamf Community, July 2024](https://community.jamf.com/t5/jamf-pro/can-t-assign-a-laptop-from-apple-business-manager-to-mdm/m-p/321792)). AppleCare coverage and Activation Lock are per-device lookups, and every client is its own organization, so nothing rolls them up.

- `apple-business-cli devices unassigned --all-orgs` - unassigned devices across every client, with the missing default server flagged.
- `apple-business-cli applecare expiring --within 90d --group-by order --all-orgs` - coverage ending inside the window, grouped by order for the renewal quote.
- `apple-business-cli devices locked --type user --all-orgs` - devices still Activation Locked to a user.
- `apple-business-cli fleet summary` - one row of counts per client org.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Apple Business MCP server runs locally over stdio by default. Its HTTP mode (`apple-business-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `APPLE_BUSINESS_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Apple Business credentials once.

### Is my Apple Business data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### What do I need from each client's Apple Business Manager?

An API account. An Administrator opens Settings > API in Apple Business, adds an API account and downloads its private key (.pem, offered once). The account's Client ID and Key ID plus that key are the three values the skill needs. API accounts belong to one organization, so each client gets its own.

### Will this hit Apple's API rate limits?

It paces itself when Apple Business answers 429. Sync once, then the fleet reports read the local store and make no API calls; devices enrich is the only per-device fan-out, bounded by --concurrency, and --skip-fresh lets a rerun resume.

### Does it replace my MDM (Jamf, Intune, Kandji, Mosyle)?

No. Apple Business Manager decides which MDM server a device enrolls with; this skill reads and changes that assignment and reports on the devices. Your MDM still manages them after enrollment.

### What do I need from each client's Apple Business Manager?

An API account. An Administrator opens Settings > API in Apple Business, adds an API account and downloads its private key (.pem, offered once). The account's Client ID and Key ID plus that key are the three values the skill needs. API accounts belong to one organization, so each client gets its own.

### Will this hit Apple's API rate limits?

It paces itself when Apple answers 429. Sync once, then the fleet reports read the local store and make no API calls; devices enrich is the only per-device fan-out, bounded by --concurrency, and --skip-fresh lets a rerun resume.

### Does it replace my MDM (Jamf, Intune, Kandji, Mosyle)?

No. Apple Business Manager decides which MDM server a device enrolls with; this skill reads and changes that assignment and reports on the devices. Your MDM still manages them after enrollment.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | devices list, devices lookup, devices unassigned, devices locked, applecare expiring, fleet summary, mdm-servers health, audit-events digest, sync, devices enrich | Allow |
| Write (routine) | blueprints create, blueprints update, blueprints add-apps, configurations create, configurations update, mdm-servers create, mdm-servers update, import | Preview with --dry-run, then a reviewed write |
| Device assignment | devices assign, devices unassign, device-activities create (decide which MDM server a customer device enrolls with, and can set an MDM migration deadline) | Human-in-the-loop only |
| Destructive | blueprints delete, blueprints remove-devices, configurations delete, mdm-servers delete, local-data purge | Human-in-the-loop only, explicit confirmation |
| Credential | auth login, auth set-token, auth logout | Human-in-the-loop only |

Over MCP, every write tool returns a preview and runs only when called with `confirm: true`. On the command line, `--dry-run` previews any write.

The strongest control is the **scope you grant the Apple Business credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated offline against the Apple Business API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-29._
