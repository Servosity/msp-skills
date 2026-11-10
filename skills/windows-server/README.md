# Windows Server + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for Windows Server. Not affiliated with, endorsed by, or sponsored by Microsoft Corporation.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/windows-server/">
    <img src="../../docs/assets/video/windows-server/animated-og.gif" alt="Windows Server demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/windows-server/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Agentless Windows Server health checks over WinRM: one verdict per server, a diffable inventory, and no arbitrary PowerShell. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Windows Server skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `windows-server-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `windows-server-mcp` over HTTPS with `WINDOWS_SERVER_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `windows-server-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `windows-server-mcp --transport http` with `WINDOWS_SERVER_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `windows-server-mcp` over HTTPS with `WINDOWS_SERVER_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Windows Server MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/windows-server-v0.1.1/windows-server-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Windows Server release on the [releases page](https://github.com/servosity/msp-skills/releases?q=windows-server).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install windows-server@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Windows Server Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.ps1 | iex`. Then authenticate per the README and run `windows-server-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.sh)
```

The installer drops both `windows-server-cli` (the CLI) and `windows-server-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
windows-server-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/windows-server/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update windows-server@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/windows-server --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/windows-server --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `windows-server-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the windows-server skill from https://github.com/servosity/msp-skills/tree/main/skills/windows-server. The skill defines how its required CLI (`windows-server-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Windows Server has no API key: the CLI signs in to each server's WinRM HTTPS listener
(port 5986) with a local or domain account in that server's Administrators group. Add
each server to the config file (default `~/.config/windows-server-cli/config.toml` on
macOS and Linux, `%USERPROFILE%\.config\windows-server-cli\config.toml` on Windows;
override with `--config` or `WINDOWS_SERVER_CONFIG`):

```toml
[[servers]]
name = "fs01"
host = "fs01.corp.example"
auth = "ntlm"                  # or "basic" (HTTPS only)
username = "CORP\\svc-monitor"
cert_sha256 = "4B:EF:..."      # or ca_file = "/path/to/ca.pem" for a private CA
default = true
```

Then store its password (read from stdin, kept only in the CLI's 0600 credential store)
and run the real connectivity test:

**macOS / Linux:**

```bash
windows-server-cli servers login fs01 --username 'CORP\svc-monitor' < password.txt
windows-server-cli servers list --agent
windows-server-cli health --agent
```

**Windows (PowerShell):**

```powershell
Get-Content password.txt | windows-server-cli servers login fs01 --username 'CORP\svc-monitor'
windows-server-cli servers list --agent
windows-server-cli health --agent
```

Or share one account across servers with `WINDOWS_SERVER_USERNAME` and
`WINDOWS_SERVER_PASSWORD`. Passwords shorter than 8 characters are refused. With NTLM, a
local account other than the built-in Administrator gets HTTP 401 unless
`LocalAccountTokenFilterPolicy` is 1 on the server: use a domain account, or
`auth = "basic"` over HTTPS. `doctor` checks config and paths and reports whether
credentials are present; it does not contact a server. `health` is the connectivity
test.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which of my servers need attention today, and why? | `windows-server-cli health --agent` |
| Which automatic services are stopped, and what stopped them? | `windows-server-cli services down --agent` |
| Which errors are new on this server compared with its last 7 days? | `windows-server-cli eventlog new --window 24h --baseline 7d --agent` |
| Who added or removed a local admin this week? | `windows-server-cli local-admins changes --hours 168 --agent` |
| What changed since the last inventory snapshot? | `windows-server-cli inventory diff --agent` |
| Which certificates expire in the next 30 days? | `windows-server-cli certificates --expiring-days 30 --agent` |
| Which scheduled tasks failed their last run? | `windows-server-cli scheduled-tasks --failed --agent` |
| Restart the Print Spooler on fs01 (after a preview) | `windows-server-cli services restart Spooler --server fs01` |

`health`, `services down`, `eventlog new`, `local-admins changes` and `inventory
snapshot` read every configured server; the single reads and the service actions act on
one server (`--server <name>`, or the entry marked `default = true`).

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most ways to put an AI on a Windows server hand it a PowerShell prompt. This one cannot
run arbitrary script: every command is one of a fixed set of read scripts that ship
inside the binary, checked by SHA-256 on the server before they run, with arguments
validated and passed as typed data. The server comes from your config by name (never
from the agent), certificate verification is always on, and nothing is installed on the
server.

On top of the reads, compound commands do work a single PowerShell call cannot: `health`
rolls every check into one verdict per server and remembers how long each has failed,
`services down` pairs each stop with its Service Control Manager event, `eventlog new`
compares against the server's own history, and `inventory snapshot` plus
`inventory diff` keep a local SQLite history so you see what changed between runs.

## The pain this closes

Daily server checks are still a manual round on many MSP desks. The
[Active Directory Pro Windows Server maintenance checklist](https://activedirectorypro.com/windows-server-maintenance-checklist/)
has you "monitor critical server services" every day, review disk space weekly, and check
Event Viewer's application, security and system logs "for any critical errors": an RDP
session per server, per client. And when a server breaks, the first hour goes to working
out what changed.

- `health` answers "which servers need attention" in one call, with reasons.
- `services down` starts a down ticket with the cause, not a restart.
- `eventlog new` separates a new failure from the error that fires every night.
- `local-admins changes` and `inventory diff` name what changed, who did it, and when.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Windows Server MCP server runs locally over stdio by default. Its HTTP mode (`windows-server-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `WINDOWS_SERVER_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Windows Server credentials once.

### Is my Windows Server data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need to install anything on the server?

No. It uses WinRM over HTTPS (port 5986), which is built into Windows Server. Enable an
HTTPS listener (Microsoft's guide:
[Installation and configuration for Windows Remote Management](https://learn.microsoft.com/en-us/windows/win32/winrm/installation-and-configuration-for-windows-remote-management))
and give the CLI an account in the server's Administrators group.

### Can the AI run its own PowerShell on my client's server?

No. Every command runs one fixed script that ships inside the binary. A service name or
log name is checked against a strict pattern and passed as typed data; input that would
change the script is refused.

### What can it change on a server?

Two things, each after a preview: restart a service (`services restart`) or start a
stopped automatic service (`services start`). The CLI needs `--yes`; the MCP tools need
`confirm: true`. `WINDOWS_SERVER_MCP_READ_ONLY=1` removes both from the MCP server.

### Does it replace my RMM?

No. It complements it: an RMM agent alerts on thresholds; this answers "why" and "what
changed" questions on demand, across every server in your config.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read (server) | `health`, `system`, `volumes`, `services list`, `services down`, `eventlog`, `eventlog new`, `certificates`, `local-admins changes`, `scheduled-tasks`, and the other reads | Allow |
| Read (local) | `events list`, `inventory diff`, `servers list` | Allow |
| Write (local only) | `inventory snapshot`, `health` (records history), `sync`, `local-data purge` (preview unless `--yes`) | Allow; review `local-data purge` |
| Credential | `servers login`, `servers logout`, `auth set-credentials`, `auth logout` | Human-in-the-loop only |
| Server action | `services restart`, `services start` | Human-in-the-loop only: read the preview, then `--yes` or `confirm: true` |

The strongest control is the **account you give it**: the CLI can only do what that account may do on the server, and only through its fixed scripts. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Verified live against a Windows Server 2022 test server we run, and being validated with MSPs running it live against their own client servers in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-29._
