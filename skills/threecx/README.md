# 3CX + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the 3CX
> API. Not affiliated with, endorsed by, or sponsored by 3CX Ltd.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/threecx/">
    <img src="../../docs/assets/video/threecx/animated-og.gif" alt="3CX demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/threecx/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Run every client's 3CX phone system from one CLI: trunk, version, licence and backup health across the whole fleet, plus the full Configuration API on any single PBX. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the 3CX skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `threecx-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `threecx-mcp` over HTTPS with `THREECX_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `threecx-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `threecx-mcp --transport http` with `THREECX_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `threecx-mcp` over HTTPS with `THREECX_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download 3CX MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/threecx-v0.1.1/threecx-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every 3CX release on the [releases page](https://github.com/servosity/msp-skills/releases?q=threecx).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install threecx@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the 3CX Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threecx/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threecx/install.ps1 | iex`. Then authenticate per the README and run `threecx-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threecx/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threecx/install.sh)
```

The installer drops both `threecx-cli` (the CLI) and `threecx-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
threecx-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threecx/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threecx/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update threecx@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/threecx --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/threecx --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `threecx-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the threecx skill from https://github.com/servosity/msp-skills/tree/main/skills/threecx. The skill defines how its required CLI (`threecx-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your 3CX portal):

```bash
THREECX_CLIENT_ID=<value> THREECX_CLIENT_SECRET=<value> THREECX_FQDN=<value> threecx-cli doctor
```

`doctor` checks config, paths, and API reachability, and reports whether credentials are loaded - it does not validate them. Run a read command to confirm the credential actually works end-to-end.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which client phone systems are failing or warning, and why? | `threecx-cli fleet health --agent` |
| Is any client's SIP trunk offline, misconfigured or in an unknown state? | `threecx-cli fleet trunks --offline --agent` |
| Whose licence or maintenance expires in the next 60 days? | `threecx-cli fleet licences --within 60d --agent` |
| Which PBXes are behind the newest build in our fleet, or have updates pending? | `threecx-cli fleet versions --behind --agent` |
| Which clients have no backup in the last two days? | `threecx-cli fleet backups --stale 48h --problems --agent` |
| What changed on our phone systems since yesterday? | `threecx-cli fleet changes --since 24h --agent` |
| Who are the users on every client's PBX, in one table? | `threecx-cli fleet exec /Users --param '$select=Number,FirstName,LastName' --agent` |

Register each client PBX once (`threecx-cli fleet add acme --fqdn acme.3cx.us --client-id 900 --env-prefix THREECX_ACME`; the API key stays in `THREECX_ACME_CLIENT_SECRET`, never in a file). The fleet commands query every registered PBX live and in parallel; a PBX that cannot be read gets its own error row instead of stopping the run. Any generated command targets one PBX with `--pbx <name>`.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Every other 3CX tool we know of, from the admin console to 3CX's own MCP preview, the [3cx-xapi CLI](https://github.com/mvanhorn/printing-press-library) and [SSIG-IT/3cx-mcp-server](https://github.com/SSIG-IT/3cx-mcp-server), talks to one phone system per connection. An MSP's question is about the fleet: which of 40 clients has a trunk down this morning.

This skill keeps a local registry of every client PBX, each with its own API credential, and runs the health questions against all of them at once: `fleet health` rates each PBX pass, warn, fail or unknown on version, licence, backup, trunks, disk, services and capacity, and names each missing input instead of reading it as healthy. `fleet changes` compares saved runs, so trunks that went down or came back, version bumps, renewals and stale backups show up as a diff. For one PBX, `sync` mirrors its configuration into local SQLite for offline `search`.

## The pain this closes

MSPs and VoIP resellers run one 3CX PBX per client, and 3CX gives them one admin console per PBX. A dead SIP trunk, a lapsed maintenance plan or a backup that stopped running is usually found when the client calls. The need is real enough that resellers build their own pollers: see the [3CX forum thread on centralized monitoring for 3CX instances](https://www.3cx.com/community/threads/centralized-monitoring-management-for-your-3cx-instances-and-more.135446/) (October 2025), where a partner describes checking every instance each minute and opening tickets.

- `fleet health`: the morning check, one row per client with the reason for every warn or fail.
- `fleet trunks --offline`: every trunk that is down, misconfigured or unknown, with certificate days left.
- `fleet licences --within 60d`: the renewals pipeline, soonest first, with seat headroom.
- `fleet backups --problems`: stale, unscheduled or shrinking backups.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The 3CX MCP server runs locally over stdio by default. Its HTTP mode (`threecx-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `THREECX_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your 3CX credentials once.

### Is my 3CX data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Which 3CX versions and licences does this need?

3CX V20 or later with the Configuration API, which 3CX documents as needing an 8SC or higher licence. On each PBX, create an API client under **Admin Console > Integrations > API** with Configuration API access and the department and role your workflow needs; the client ID is the DN number shown there and the API key is shown once. A PBX that refuses the token gets an error in its own row.

### Does it change anything on my clients' phone systems?

The fleet commands only read. The single-PBX commands cover the whole Configuration API, including writes such as restarting services or rebooting phones. In the MCP server every write returns a preview until the agent sends `confirm: true`; on the CLI, preview any write with `--dry-run` first, which sends nothing at all. `THREECX_MCP_READ_ONLY=1` removes every MCP tool that can write.

### Where do the API keys live?

In one environment variable per PBX, `<PREFIX>_CLIENT_SECRET` (the prefix is `fleet add --env-prefix`, default `THREECX_<NAME>`), or for a single PBX `THREECX_CLIENT_SECRET`. The fleet registry stores the prefix, never the key. Commands that mint a credential, such as `conference-settings generate-api-key`, print a notice in place of the new value, so a key never lands in an agent transcript.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet health`, `fleet trunks`, `fleet licences`, `fleet versions`, `fleet backups`, `fleet changes`, `fleet exec`, `users list`, `trunks list`, every GET command | Allow |
| Write (routine) | `users-id update-user`, `queues-id update-queue`, `holidays-id update-holiday`, `office-hours update`, `import` | Preview with `--dry-run`, then a reviewed write |
| Phone-system control | `services restart`, `services restart-operating-system`, `install-updates`, `users reboot-phone`, `users reprovision-all-phones`, `sip-devices bulk-update-firmware`, `backups-file-name restore`, `users make-call` | Human-in-the-loop only |
| Credential / identity | any command that can return or mint a credential: `conference-settings generate-api-key`, `system-status apitoken`, `service-principals-id generate-app-token`, `users-id generate-prov-link`, `users regenerate-passwords`, `service-principals create`, `oauth-clients` | Human-in-the-loop only |
| Destructive | every delete and purge: `users-id delete-user`, `users batch-delete`, `purge-calls`, `purge-all-logs`, `recordings purge-local`, `backups purge-local`, `fleet remove`, `local-data purge` | Human-in-the-loop only, explicit confirmation |

The strongest control is the **scope you grant the 3CX credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the 3CX API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-29._
