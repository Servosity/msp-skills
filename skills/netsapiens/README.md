# NetSapiens + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the NetSapiens
> API. Not affiliated with, endorsed by, or sponsored by Crexendo, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/netsapiens/">
    <img src="../../docs/assets/video/netsapiens/animated-og.gif" alt="NetSapiens demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/netsapiens/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every NetSapiens v2 endpoint plus a reseller-wide fleet view: seat limits, offline phones, unrouted numbers and E911 gaps across every client at once. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the NetSapiens skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `netsapiens-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `netsapiens-mcp` over HTTPS with `NETSAPIENS_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `netsapiens-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `netsapiens-mcp --transport http` with `NETSAPIENS_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `netsapiens-mcp` over HTTPS with `NETSAPIENS_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download NetSapiens MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/netsapiens-v0.1.1/netsapiens-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every NetSapiens release on the [releases page](https://github.com/servosity/msp-skills/releases?q=netsapiens).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install netsapiens@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the NetSapiens Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.ps1 | iex`. Then authenticate per the README and run `netsapiens-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.sh)
```

The installer drops both `netsapiens-cli` (the CLI) and `netsapiens-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
netsapiens-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update netsapiens@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/netsapiens --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/netsapiens --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `netsapiens-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the netsapiens skill from https://github.com/servosity/msp-skills/tree/main/skills/netsapiens. The skill defines how its required CLI (`netsapiens-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your NetSapiens portal):

```bash
NETSAPIENS_API_KEY=<value> NETSAPIENS_SERVER=<value> netsapiens-cli doctor
```

`doctor` checks config, paths and API reachability, and makes one authenticated call to report whether the server accepted the credential. A scope-limited key shows as a warning; run a read command to confirm the access you need.

Credentials the API hands back, such as the key from `apikeys create` or the tokens from `jwt create`, print as `<redacted>`: read a new key's value in the NetSapiens portal.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which clients are over or near a user, queue, auto attendant or conference limit? | `netsapiens-cli fleet seats --flagged --agent` |
| Which phones are unregistered, high-latency or never provisioned? | `netsapiens-cli fleet offline --agent` |
| Which phone numbers route to a missing or unreachable user, queue or attendant? | `netsapiens-cli fleet dids --problems-only --agent` |
| Which domains, users and devices are missing E911 caller ID or address data? | `netsapiens-cli fleet e911 --agent` |
| Which users had unanswered inbound calls this week? | `netsapiens-cli fleet missed --window 7d --agent` |
| Which call queues are dropping callers, and are their agents reachable? | `netsapiens-cli fleet queues --window 7d --agent` |
| What was added, removed or re-pointed since my last check? | `netsapiens-cli fleet changes --agent` |
| Whose number is this, and which phones ring for it? | `netsapiens-cli trace 6025550142 --agent` |

The fleet audits read a local mirror: run `netsapiens-cli sync` first (each audit prints the exact `sync --resources ...` line it needs when the mirror is missing or stale). `fleet missed` and `fleet queues` read CDRs and queue statistics live.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most NetSapiens integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 40 client domains are over their seat limit, and which of their numbers route nowhere?"

This skill syncs NetSapiens into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `fleet dids`, `fleet e911` and `trace` join across domains, users, devices, phone numbers, call queues, auto attendants and E911 addresses - work a stateless API wrapper can't do.

## The pain this closes

A NetSapiens reseller runs dozens of client domains on one platform, and the reseller portal answers one domain at a time. Seat true-ups, offline phones and numbers that still point at a departed user are all found by opening every client, or by the client calling to complain.

E911 is where that gap costs money. [IPComms' E911 terms](https://www.ipcomms.net/legal/e911/), a SIP trunking carrier's, bill an unregistered-911 fee of $250 per call placed from a number with no registered address, and Kari's Law and RAY BAUM's Act put the dispatchable-location duty on whoever runs the phone system. MSPs are asking AI agents to reach NetSapiens: [phoneware/netsapiens-mcp](https://github.com/phoneware/netsapiens-mcp) and [MSPbots' oitvoip-mcp](https://github.com/MSPbotsAI/oitvoip-mcp) both exist, and both are under a year old.

- `fleet seats --flagged`: every client over or near a limit, ready for the monthly true-up.
- `fleet offline`: unregistered, stale or high-latency phones before the client notices.
- `fleet dids --problems-only`: numbers that ring nowhere.
- `fleet e911`: caller-ID and address gaps, as input to a Kari's Law / RAY BAUM's Act review (it checks the data, not 911 dialing).
- `fleet changes`: what was added, removed or re-pointed since last week.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The NetSapiens MCP server runs locally over stdio by default. Its HTTP mode (`netsapiens-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `NETSAPIENS_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your NetSapiens credentials once.

### Is my NetSapiens data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Does it work with Crexendo, OITVOIP and other white-label NetSapiens platforms?

Yes. Set `NETSAPIENS_SERVER` to your provider's platform host and use an API key or OAuth2 access token from that platform. Without `NETSAPIENS_SERVER` the CLI talks to ns-api.com, which is only the NetSapiens public sandbox.

### Do I need a reseller-level API key?

For the fleet audits, yes: a reseller-scoped key sees every client domain. An office-manager key sees one domain, so the fleet commands then cover only that client. A read-only key is enough for every audit.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet seats`, `fleet offline`, `fleet dids`, `fleet e911`, `fleet missed`, `fleet queues`, `fleet changes`, `trace`, `search`, and the list, get and count commands | Allow |
| Local-only writes | `sync`, `fleet changes --rebaseline` (CLI only), `auth set-token`, `auth logout` (your machine only) | Allow (no tenant change) |
| Write (routine) | `domains users create`, `domains users update`, `domains phonenumbers update`, `domains callqueues agents-create`, `phones create` | Preview with `--dry-run`, then a reviewed write |
| Live calls and voicemail | `domains users calls-create`, `calls-transfer`, `calls-delete`, `voicemails-delete` | Human-in-the-loop only |
| Credentials and security | `apikeys`, `jwt`, `tokens`, `auth-code`, `sso-enroll`, `certificates`, `firebase`, `subscriptions` | Human-in-the-loop only |
| Destructive / config | every `delete` command, `domains delete`, `restore update`, `backup`, `domains backup create` | Human-in-the-loop only, explicit confirmation |

Over MCP, every write previews first and sends only with `confirm: true`; set `NETSAPIENS_MCP_READ_ONLY=1` to refuse every write.

The strongest control is the **scope you grant the NetSapiens credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the NetSapiens API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-28._
