# Kaseya VSA 10 + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Kaseya VSA
> API. Not affiliated with, endorsed by, or sponsored by Kaseya US LLC.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/kaseya-vsa/">
    <img src="../../docs/assets/video/kaseya-vsa/animated-og.gif" alt="Kaseya VSA 10 demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/kaseya-vsa/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Kaseya VSA 10 API endpoint (the platform formerly called VSA X), plus a local fleet database that answers patch, health, and software questions across all your clients offline. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Kaseya VSA skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `kaseya-vsa-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `kaseya-vsa-mcp` over HTTPS with `KASEYA_VSA_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `kaseya-vsa-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `kaseya-vsa-mcp --transport http` with `KASEYA_VSA_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `kaseya-vsa-mcp` over HTTPS with `KASEYA_VSA_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Kaseya VSA MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/kaseya-vsa-v0.1.0/kaseya-vsa-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Kaseya VSA release on the [releases page](https://github.com/servosity/msp-skills/releases?q=kaseya-vsa).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install kaseya-vsa@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Kaseya VSA Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.ps1 | iex`. Then authenticate per the README and run `kaseya-vsa-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.sh)
```

The installer drops both `kaseya-vsa-cli` (the CLI) and `kaseya-vsa-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
kaseya-vsa-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update kaseya-vsa@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/kaseya-vsa --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/kaseya-vsa --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `kaseya-vsa-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the kaseya-vsa skill from https://github.com/servosity/msp-skills/tree/main/skills/kaseya-vsa. The skill defines how its required CLI (`kaseya-vsa-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Create an API token in the VSA 10 console under **Administration > Configuration > API Access**, then set your instance host and the token ID and secret:

**macOS / Linux:**

```bash
export KASEYA_VSA_SERVER=yourcompany.vsax.net
export KASEYA_VSA_TOKEN_ID=<token-id>
export KASEYA_VSA_TOKEN_SECRET=<token-secret>
kaseya-vsa-cli doctor
```

**Windows (PowerShell):**

```powershell
$env:KASEYA_VSA_SERVER = "yourcompany.vsax.net"
$env:KASEYA_VSA_TOKEN_ID = "<token-id>"
$env:KASEYA_VSA_TOKEN_SECRET = "<token-secret>"
kaseya-vsa-cli doctor
```

`KASEYA_VSA_SERVER` is the host name only, without `https://`. The CLI sends the token only to that host and refuses redirects to any other.

`doctor` checks config and paths, then sends one read request (`GET /assets`) with your token to the configured server and reports whether the credential was accepted, rejected, or pointed at the wrong base URL. While the server still resolves to the shipped placeholder (`yourcompany.vsax.net`), it sends nothing and names the variable to set.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which pending OS updates are exposed on the most devices, across every client? | `kaseya-vsa-cli patch-exposure --by kb --severity critical,important --agent` |
| Which clients carry the most critical patch exposure? | `kaseya-vsa-cli patch-exposure --by org --severity critical --agent` |
| Which devices are offline, without working antivirus, or with the firewall off? | `kaseya-vsa-cli health --check offline,av,firewall --offline-for 72h --agent` |
| Which versions of Google Chrome are installed, and on how many devices? | `kaseya-vsa-cli software "Google Chrome" --versions --agent` |
| Which devices with software inventory are missing our security agent? | `kaseya-vsa-cli software "Huntress Agent" --absent --agent` |
| Who did what in the tenant this week? | `kaseya-vsa-cli audit-logs digest --since 7d --agent` |
| What is going on with this one device right now? | `kaseya-vsa-cli devices brief FRONTDESK-01 --agent` |
| Run a script on a device and give me the output | `kaseya-vsa-cli scripts run-wait <script-id> --device FRONTDESK-01 --agent` |

`patch-exposure`, `health`, `software` and `audit-logs digest` read the local mirror: run `kaseya-vsa-cli sync` first (an empty mirror answers with `scanned_devices: 0`). `devices brief` and `scripts run-wait` call the live API.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Kaseya VSA integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking which of 1,200 devices across 40 clients are still missing this month's cumulative update.

This skill syncs Kaseya VSA into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `patch-exposure`, `health` and `software` join devices, their asset inventory, sites and organizations - work a stateless API wrapper can't do. `scripts run-wait` and `tasks run-wait` start an automation, wait for it to finish, and return each device's exit code and output in one call.

## The pain this closes

Kaseya's own release note for VSA 10 patch status reports says reporting "is currently limited to one policy at a time" ([Advanced reporting: Patch status reports](https://www.kaseya.com/platform/innovations/advanced-reporting-patch-status-reports/), February 2026). An MSP that wants one answer to "what is missing, where" builds a report per policy and joins the exports by hand. The same is true of offline agents, disabled antivirus and full disks: the console shows them one organization or one device at a time.

- `patch-exposure --by kb` ranks every missing update by how many devices lack it, with CVSS and days since release.
- `patch-exposure --by org` shows which clients carry the most exposure, for the QBR.
- `health` lists every device failing an offline, antivirus, firewall, UAC, disk, CPU or memory check, with counts.
- `software --versions` and `software --absent` show version drift and devices missing an agent you require.
- `scripts run-wait` runs a script on a device and returns its output, so the technician never opens the execution log.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Kaseya VSA MCP server runs locally over stdio by default. Its HTTP mode (`kaseya-vsa-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `KASEYA_VSA_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Kaseya VSA credentials once.

### Is my Kaseya VSA data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Is this for Kaseya VSA 10 or classic VSA 9?

VSA 10, the platform formerly called VSA X, with an instance host such as `yourcompany.vsax.net`. It uses the VSA 10 REST API (`/api/v3`) with an API token ID and secret created under **Administration > Configuration > API Access**. Classic on-premises VSA 9 has a different API and is not supported.

### Will this hit my VSA 10 API rate limits?

It is built to stay inside them. VSA 10 limits each API token per hour, so the skill syncs once and then answers fleet questions from the local mirror without new API calls. The CLI paces itself to the server's rate-limit headers, and `kaseya-vsa-cli rate-limits` shows what is left in the current window.

### Does it store my custom-field values or product keys?

No. MSPs often keep BitLocker keys and client passwords in VSA custom fields, so `sync` never copies custom-field values, webhook signing keys or the Windows product key into the local mirror. Live commands such as `devices list-custom-fields` still return them, which is why they sit in the credential tier below.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `devices list`, `health`, `patch-exposure`, `software`, `audit-logs digest`, `devices brief`, `sync`, `search` | Allow |
| Write (routine) | `organizations create`/`update`, `sites create`/`update`, `groups create`/`update`, `devices move`, `devices publish`, `notifications create`, `custom-fields assign`/`unassign`/`update-assigned`, `webhooks create`/`update`, `import` | Preview with `--dry-run`, then a reviewed write |
| Endpoint execution | `scripts run`, `scripts run-wait`, `tasks run`, `tasks run-wait`, `workflows run`, `workflows cancel-executions` | Human-in-the-loop only: these run code on customer devices |
| Credential / secret-bearing | any command that can return a stored secret: `custom-fields get`/`list`, `devices list-custom-fields`, `organization-custom-fields`, `site-custom-fields`, `group-custom-fields`, `assets list`/`get-device` (AssetInfo holds the Windows product key), `groups get-package` (agent install link), `webhooks get`/`list`/`regenerate-secret-key`, `auth` | Human-in-the-loop only |
| Destructive | `organizations delete`, `sites delete`, `groups delete`, `notifications delete`, `webhooks delete`, `local-data purge` | Human-in-the-loop only, explicit confirmation |

`--dry-run` is opt-in: the CLI sends a write as soon as it runs. In the MCP server, every tool that changes VSA 10 or runs automation (`kaseya-vsa_execute`, `import`, `scripts_run_wait`, `tasks_run_wait`) returns a preview and sends only when called again with `confirm: true`.

The strongest control is the **scope you grant the Kaseya VSA credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Kaseya VSA API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-28._
