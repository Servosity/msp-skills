# Jamf Pro + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Jamf
> API. Not affiliated with, endorsed by, or sponsored by JAMF Software, LLC.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/jamf-pro/">
    <img src="../../docs/assets/video/jamf-pro/animated-og.gif" alt="Jamf Pro demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/jamf-pro/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every client's Jamf Pro in one CLI: register each instance once, then find devices, check posture and patch status, and audit policies across all of them. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Jamf Pro skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `jamf-pro-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `jamf-pro-mcp` over HTTPS with `JAMF_PRO_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `jamf-pro-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `jamf-pro-mcp --transport http` with `JAMF_PRO_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `jamf-pro-mcp` over HTTPS with `JAMF_PRO_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Jamf Pro MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/jamf-pro-v0.1.3/jamf-pro-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Jamf Pro release on the [releases page](https://github.com/servosity/msp-skills/releases?q=jamf-pro).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install jamf-pro@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Jamf Pro Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.ps1 | iex`. Then authenticate per the README and run `jamf-pro-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.sh)
```

The installer drops both `jamf-pro-cli` (the CLI) and `jamf-pro-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
jamf-pro-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update jamf-pro@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/jamf-pro --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/jamf-pro --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `jamf-pro-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the jamf-pro skill from https://github.com/servosity/msp-skills/tree/main/skills/jamf-pro. The skill defines how its required CLI (`jamf-pro-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Create an API client in each client's Jamf Pro (**Settings > System > API roles and clients**), with only the privileges your reports need. Then register each instance once. The secret stays in an environment variable you name, and is never written to the registry.

**macOS / Linux:**

```bash
export JAMF_PRO_FLEET_SECRETS="acme=<client-secret>"
jamf-pro-cli instance add acme --server acme.jamfcloud.com --client-id <client-id>
jamf-pro-cli instance test
```

**Windows (PowerShell):**

```powershell
$env:JAMF_PRO_FLEET_SECRETS = "acme=<client-secret>"
jamf-pro-cli instance add acme --server acme.jamfcloud.com --client-id <client-id>
jamf-pro-cli instance test
```

`instance test` requests a new token for each instance and reads its Jamf Pro version, so it proves each API client works. With one server, set `JAMF_PRO_SERVER`, `JAMF_PRO_CLIENT_ID` and `JAMF_PRO_CLIENT_SECRET` instead and run `jamf-pro-cli doctor`: it requests a token and reads the Jamf Pro version with it.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which client has this Mac, iPad or iPhone, by serial, name, username or email? | `jamf-pro-cli fleet find C02XK1ABJG5H` |
| Which clients have stale check-ins, Macs below a macOS floor, FileVault off or the firewall off? | `jamf-pro-cli fleet posture --stale-days 14 --min-os 14.6` |
| Which patch titles are behind latest, and which clients have failed managed OS updates? | `jamf-pro-cli fleet patch --behind-only` |
| A vendor advisory landed: which clients and computers have this app below the fixed version? | `jamf-pro-cli fleet app "Google Chrome" --below 129.0.6668.100` |
| Which policies are disabled, can never run, have no scope, or point at a missing script or package? | `jamf-pro-cli fleet policy-hygiene` |
| Which ADE or VPP tokens expire soon, which enrollment syncs failed, and which Jamf Pro versions are clients on? | `jamf-pro-cli fleet health --within 30d` |
| Which clients are missing objects from our template instance, or run a different copy of its scripts? | `jamf-pro-cli fleet baseline --golden msp-template` |
| Does every client's API client still work? | `jamf-pro-cli instance test` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

A connector that forwards each tool call to one Jamf Pro server answers for one client at a time. That's fine for one record. It stops working when you're asking "which of my 40 clients has FileVault off, and on how many Macs?"

This skill keeps a registry of every client's server and API client and reads them in parallel, each with its own credential, so that question is one command and one table with an instance column: `fleet posture`, `fleet patch`, `fleet baseline`. Those commands read live. `sync` also builds a **local SQLite mirror** per instance for offline search. An unsynced mirror answers empty, so sync first when you use `--data-source local`.

## The pain this closes

Jamf Pro gives each customer its own server, so an MSP works many consoles. On [Jamf Nation's "Multi-tenant Jamf Pro?" thread](https://community.jamf.com/general-discussions-2/multi-tenant-jamf-pro-4059) one MSP engineer put it as "about 100 instances", because Apple requires each organization to have its own push certificate. Any question across clients then means one login per client.

- `jamf-pro-cli fleet find <serial>` - the ticket names a Mac but not the client: find which client has it.
- `jamf-pro-cli fleet health --within 30d` - catch a lapsing ADE or VPP token before onboarding stops.
- `jamf-pro-cli fleet posture --min-os 14.6` - per-client counts of stale check-ins, old macOS, FileVault off and firewall off.
- `jamf-pro-cli fleet policy-hygiene` - policies that can never run, before a client notices.
- `jamf-pro-cli fleet baseline --golden msp-template` - which clients are missing your template's policies, profiles and smart groups by name, or run a different copy of its scripts.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Jamf Pro MCP server runs locally over stdio by default. Its HTTP mode (`jamf-pro-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `JAMF_PRO_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Jamf Pro credentials once.

### Is my Jamf Pro data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need a separate API client for each client's Jamf Pro?

Yes. Create one in each instance under Settings > System > API roles and clients, with only the read privileges the reports need, then register it once with `jamf-pro-cli instance add`. Each secret stays in the JAMF_PRO_FLEET_SECRETS environment variable (name=secret entries separated by commas) and is never written to the registry.

### Will this replace Jamf Account or the Jamf Pro console?

No. Jamf Account is where a service provider manages customers and their products, and each console is where you build policies and enroll devices. This skill answers questions across the data inside those instances.

### Can it read LAPS passwords, FileVault keys or lock PINs?

No. Those endpoints are not included, `fleet exec` refuses them by path, and any password, key, token or PIN field in another response prints as [redacted]. Read them in the Jamf Pro console.

### We run one Jamf Pro with sites. Does it help?

Partly. Register that server once and every read works, but fleet reports count per instance, not per site. Fleet reports earn their keep when each client has its own instance.


### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet find`, `fleet posture`, `fleet patch`, `fleet app`, `fleet policy-hygiene`, `fleet health`, `fleet baseline`, `fleet exec`, `computer-inventory list`, `scripts get`, `policies list`, `sync`, `search` | Allow |
| Write (routine) | `buildings create`, `categories update`, `departments update`, `computer-groups create-static-groups`, `packages update`, `computer-extension-attributes update`, every other POST, PUT or PATCH not named below | Preview with `--dry-run`, then a reviewed write |
| Credential and identity | `auth login`, `auth set-token`, every `api-integrations` and `api-roles` command, `accounts`, `account-groups`, every `local-admin-password` command, `volume-purchasing-locations create` and `update`, and any prestage write that sets an admin password | Human-in-the-loop only |
| Acts on customer devices | `computer-inventory erase`, `computer-inventory remove-mdm-profile`, `mobile-devices erase`, `mobile-devices unmanage`, `mdm send-command`, `mdm deploy-package`, `mdm blank-push`, `mdm renew-profile`, `managed-software-updates create-plans`, `patch-policy-logs retry`, `policies create` and `update`, `scripts create` and `update`, `device-enrollments disown`, `volume-purchasing-locations revoke-licenses` and `reclaim` | Human-in-the-loop only, one instance at a time |
| Destructive / config | Every `delete` and `delete-multiple` command, for example `scripts delete`, `policies delete`, `computer-inventory delete`, `api-roles delete`, `device-enrollments delete` | Human-in-the-loop only, explicit confirmation |

The strongest control is the **scope you grant the Jamf Pro credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against mocks built from Jamf's public API reference; no live Jamf Pro tenant has run it yet. We validate with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-11-04._
