# Keeper Security + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Keeper Security
> API. Not affiliated with, endorsed by, or sponsored by Keeper Security, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/keeper/">
    <img src="../../docs/assets/video/keeper/animated-og.gif" alt="Keeper Security demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/keeper/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Read-only Keeper MSP posture across every client, with history, a change feed and no path to a vault secret. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Keeper Security skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `keeper-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `keeper-mcp` over HTTPS with `KEEPER_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `keeper-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `keeper-mcp --transport http` with `KEEPER_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `keeper-mcp` over HTTPS with `KEEPER_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Keeper Security MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/keeper-v0.1.3/keeper-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Keeper Security release on the [releases page](https://github.com/servosity/msp-skills/releases?q=keeper).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install keeper@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Keeper Security Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.ps1 | iex`. Then authenticate per the README and run `keeper-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.sh)
```

The installer drops both `keeper-cli` (the CLI) and `keeper-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
keeper-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update keeper@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/keeper --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/keeper --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `keeper-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the keeper skill from https://github.com/servosity/msp-skills/tree/main/skills/keeper. The skill defines how its required CLI (`keeper-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Keeper Security portal):

```bash
KEEPER_API_KEY=<value> KEEPER_BASE_URL=<value> keeper-cli doctor
```

`doctor` checks config and paths, then tests the API key with one read of Service Mode's queue status, so it tells a working key from a rejected one and a wrong base URL. A Service Mode in legacy mode has no queue status, so there doctor says it could not test the key. That read runs no Commander command, so run a real read such as `keeper-cli companies list` to confirm the key's `-c` allowlist.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| How is every client doing on 2FA, weak passwords and BreachWatch? | `keeper-cli posture report --days 90 --agent` |
| Which clients have users without 2FA, admins first? | `keeper-cli users risk --kind no-2fa --agent` |
| What changed since the last posture run, by client and by user? | `keeper-cli posture diff --agent` |
| Which invites went stale, and who is inactive or locked? | `keeper-cli users hygiene --kind invited,inactive --agent` |
| Which managed company does this user belong to? | `keeper-cli users find jane@example.com --agent` |
| How many seats does each client have allocated and active? | `keeper-cli companies list --agent` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most password-manager integrations, and an MCP server that only forwards each tool call, answer about one tenant now. That's fine for one user. It stops working when a QBR asks which of 40 clients have admins without 2FA and what moved since last quarter.

`posture report` reads every managed company in one run through your own Commander Service Mode host and **stores the run in a local SQLite file**. `posture diff`, `users risk`, `users hygiene`, `users find` and `events list` then answer from that file: instant, offline, and the AI sees the answer. Typed reads such as `users list` go live and are not saved.

Every command it can send is on a short fixed allowlist (`keeper-cli allowlist`), each with fixed read-only flags, and arguments cannot add a flag or a second command. There is no path to a vault record.

## The pain this closes

Keeper's password-health and 2FA views are read per tenant ([one 2026 review for MSPs](https://www.flamingo.run/blog/keeper-security-review-for-msps) describes them as "per tenant"), and comparing against last quarter means keeping the earlier numbers yourself.

- `posture report`: every client's 2FA, password health, BreachWatch, inactivity and seats, in one run
- `posture diff`: per-company deltas and per-user changes, such as 2FA turned off
- `users risk --kind no-2fa`: users with 2FA off across every client, admins first
- `users hygiene`: stale invites, locked, inactive and pending-transfer users
- `events list`: change events once each, for automation

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Keeper Security MCP server runs locally over stdio by default. Its HTTP mode (`keeper-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `KEEPER_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Keeper Security credentials once.

### Is my Keeper Security data safe?

Your data stays on **your machine** and your own Service Mode host. Typed reads print and are not saved; only `posture report` saves metrics and per-user facts to a local SQLite file under your user account. The AI agent only sees what the CLI returns. The API key is sent only to `KEEPER_BASE_URL`, and is withheld from every output, log, file and MCP result. It is never bundled into this repo or sent anywhere by MSP Skills.

### Can this read passwords from a client's vault?

No. It can send only the admin and MSP reports `keeper-cli allowlist` prints, each with fixed read-only flags, and a response shaped like record content is refused whole. Set the same list on your Service Mode key with `service-create -c`, so Service Mode refuses anything else too.

### What do I need from Keeper?

Keeper Commander running in [Service Mode](https://docs.keeper.io/keeperpam/commander-cli/service-mode-rest-api) on a host you control, over TLS (`service-create` with `-crtf` and `-crtp`), and its API key. Service Mode defaults to plain http; this CLI refuses plain http on every host, localhost included, so the key is never sent in clear. Trust a private certificate with `KEEPER_CA_CERT`.

### Do I need a Keeper MSP account?

The cross-client commands need one. An account that is not an MSP is reported as its own enterprise, one company in the same format.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | companies list, users list, users risk, users hygiene, posture diff, posture runs, events list, billing report, security-audit report, audit-trail events | Allow |
| Local write (this machine only) | posture report (reads Keeper, saves the run locally), teach, playbook amend, learnings forget, profile save, local-data purge | Allow in the CLI; through MCP only posture_report is a tool, and it needs confirm set to true |
| Credential | auth set-token, auth logout | Human-in-the-loop only |

This connector has no write path to Keeper. Scope the Service Mode key to the same command list with `service-create -c`, so the key itself cannot do more. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Keeper Security API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-11-04._
