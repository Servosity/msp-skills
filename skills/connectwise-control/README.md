# ConnectWise Control + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the ConnectWise
> Control (ScreenConnect) API. Not affiliated with, endorsed by, or sponsored by ConnectWise, LLC.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/connectwise-control/">
    <img src="../../docs/assets/video/connectwise-control/animated-og.gif" alt="ConnectWise Control demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/connectwise-control/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Manage ConnectWise Control (ScreenConnect) remote support and access sessions from the terminal or your AI agent: list and inspect sessions across groups, run commands on guest machines, rename and tag sessions, manage instance users, and query the audit log - the whole instance from one CLI. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the ConnectWise skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `connectwise-control-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `connectwise-control-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `connectwise-control-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the ConnectWise Control MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `connectwise-control-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download ConnectWise MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/connectwise-control-v0.1.0/connectwise-control-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every ConnectWise release on the [releases page](https://github.com/servosity/msp-skills/releases?q=connectwise-control).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install connectwise-control@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the ConnectWise Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-control/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-control/install.ps1 | iex`. Then authenticate per the README and run `connectwise-control-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-control/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-control/install.sh)
```

The installer drops both `connectwise-control-cli` (the CLI) and `connectwise-control-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
connectwise-control-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-control/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-control/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update connectwise-control@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/connectwise-control --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/connectwise-control --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `connectwise-control-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the connectwise-control skill from https://github.com/servosity/msp-skills/tree/main/skills/connectwise-control. The skill defines how its required CLI (`connectwise-control-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your ConnectWise portal):

```bash
CONNECTWISE_CONTROL_BASE_URL=<value> CONNECTWISE_CONTROL_USERNAME=<value> CONNECTWISE_CONTROL_PASSWORD=<value> connectwise-control-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which access sessions are in this group? | `connectwise-control-cli sessions list --session-type Access --agent` |
| Show the full detail for one session | `connectwise-control-cli sessions get-detail --session-id <id> --agent` |
| What session groups exist on the instance? | `connectwise-control-cli session-groups --agent` |
| Run a command on a guest machine (approval-gated) | `connectwise-control-cli sessions run-command --session-id <id> --command "..." --agent` |
| Who are the instance users and roles? | `connectwise-control-cli security get-configuration --agent` |
| What's in the audit log for a session? | `connectwise-control-cli audit query-log --session-name "ACME-WS01" --agent` |
| Rename a session | `connectwise-control-cli sessions update-name --session-id <id> --new-name "..." --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

ConnectWise Control (ScreenConnect) is a **web console with no first-party CLI** - built for a technician driving one remote session interactively. There's no terminal-native way to list, search, or act on sessions in bulk, or to hand the work to an AI agent.

This skill turns the instance's session, group, user, and audit surface into **typed commands with JSON output** and an **offline SQLite mirror** for fast session lookups. An agent can list and inspect sessions, read the audit log, and - with your approval - run a command on a guest machine, all from the terminal instead of clicking through the console one session at a time. It complements the live console; it doesn't replace the eyes-on remote-control experience.

## The pain this closes

ConnectWise Control (formerly ScreenConnect) is where a huge number of MSPs run remote support - and it's a web console built for a tech driving one session at a time. On r/msp and in the ScreenConnect community, the recurring ask is the same: there's no first-party CLI, no terminal-native way to work with sessions in bulk. "Which machines have an open session?", "what happened on this endpoint?", or "run this one command across these five machines" means scrolling the console and typing into each command box by hand.

This skill answers those directly:

- `sessions list --session-type Access` - every access session in a group, as JSON
- `sessions get-detail --session-id <id>` - one session's connections and recent events
- `audit query-log --session-name <name>` - what happened on a machine, from the audit log
- `sessions run-command --session-id <id> --command <cmd>` - run a command on a guest (gated human-in-the-loop - it executes on a real machine)
- `security get-configuration` - the instance's users and roles in one read

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The ConnectWise Control MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your ConnectWise credentials once.

### Is my ConnectWise data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Is this ConnectWise Control, or ConnectWise Manage / Automate?

ConnectWise Control - the remote support and access tool, formerly **ScreenConnect**. ConnectWise Manage (the PSA) and ConnectWise Automate (the RMM) are separate skills. This one talks to your Control instance's session, user, and audit surface.

### How does it authenticate, and does it need the RESTful API Manager extension?

HTTP Basic auth with a ConnectWise Control instance user (`CONNECTWISE_CONTROL_USERNAME` / `CONNECTWISE_CONTROL_PASSWORD`) against your instance base URL (`CONNECTWISE_CONTROL_BASE_URL`) - the same login you use for the web console. No extension is required; these are the built-in instance services the console itself uses.

### Can it actually run commands on machines?

Yes, via `sessions run-command`, but that's gated **human-in-the-loop** in [governance.md](./governance.md) - it runs a real command on a guest endpoint. Day-to-day use is read-first: listing, searching, and inspecting sessions.

### Will this replace the ConnectWise Control console?

No. It's for scripting and agent-driven session queries, audit lookups, and approval-gated actions from the terminal. The console stays where you drive a live, eyes-on remote session.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

The key fact: some commands **act on a real remote machine**. Running a command on a guest is remote code execution on a customer's computer. The safe default is read-only.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `sessions list`/`get-detail`, `session-groups`, `security get-configuration`, `audit get-info`/`query-log`, `search` | Allow |
| Write (session metadata) | `sessions update-name`, `sessions update-custom-property` | Preview with `--dry-run`, then a reviewed write |
| Session and host control | `sessions run-command` (runs a real command on a guest), `sessions add-event-to` (control events like wake) | Human-in-the-loop, explicit confirmation |
| Access grant | `sessions get-access-token` (one-time token/URL granting remote access) | Human-in-the-loop only |
| Admin and identity | `security save-user`, `security delete-user` | Operator-only |

The strongest control is the **scope you grant the ConnectWise Control instance login** - the CLI can only do what that user is permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the ConnectWise Control API surface and being validated with MSPs running it live against their own instance in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-13._
