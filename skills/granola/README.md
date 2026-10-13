# Granola + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Granola
> API. Not affiliated with, endorsed by, or sponsored by Granola.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/granola/">
    <img src="../../docs/assets/video/granola/animated-og.gif" alt="Granola demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/granola/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Search, export, and analyze every Granola meeting across all your calls - attendee timelines, talk-time roll-ups, and transcript search from an offline local mirror. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Granola skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `granola-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `granola-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `granola-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `granola-mcp --transport http` and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `granola-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Granola MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/granola-v0.1.0/granola-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Granola release on the [releases page](https://github.com/servosity/msp-skills/releases?q=granola).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install granola@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Granola Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/granola/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/granola/install.ps1 | iex`. Then authenticate per the README and run `granola-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/granola/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/granola/install.sh)
```

The installer drops both `granola-cli` (the CLI) and `granola-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
granola-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/granola/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/granola/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update granola@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/granola --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/granola --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `granola-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the granola skill from https://github.com/servosity/msp-skills/tree/main/skills/granola. The skill defines how its required CLI (`granola-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Granola portal):

```bash
GRANOLA_API_KEY=<value> granola-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Every meeting with a client contact, oldest to newest? | `granola-cli attendee timeline sarah@northwind.com` |
| The last few calls with a contact, notes + AI summary inlined? | `granola-cli attendee brief sarah@northwind.com --last 5` |
| Every meeting where a topic came up - in the title, your notes, or the transcript? | `granola-cli meetings list --query "backup migration"` |
| Bulk-export the last 7 days of calls to a folder? | `granola-cli export-all --out ./calls --last 7d` |
| How much of last month's calls was you talking versus everyone else? | `granola-cli talktime --by participant --since 30d` |
| Which recent meetings have a transcript but no write-up in your notes folder yet? | `granola-cli memo queue --since 7d --root ~/Documents/meeting-transcripts` |
| Top attendees by meeting count? | `granola-cli stats attendees --top 10` |
| Is auth working and is my local mirror fresh? | `granola-cli doctor` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Granola integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "what did we tell this client across every call last quarter" and the answer is spread over thirty separate recordings.

This skill syncs Granola into a **local SQLite mirror** you can search by title, notes, and transcript text. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `attendee timeline`, `talktime --by participant`, and `stats attendees` join across meetings, transcripts, and attendees - work a stateless API wrapper can't do.

## The pain this closes

MSP owners on r/msp keep hitting the same wall around client meetings: the recordings and AI summaries all exist, but the context is trapped one meeting at a time. Before a QBR you're scrubbing back through weeks of calls to remember what you promised; nobody can answer "what did we tell this client last quarter" without opening every meeting by hand.

This skill closes that gap by making every meeting queryable at once:

- **`attendee timeline <contact>`** - every call with a client contact, oldest to newest, before you walk into their QBR.
- **`attendee brief <contact> --last 5`** - the last few calls with a contact, real notes + AI summary inlined, as pre-call prep.
- **`meetings list --query "<topic>"`** - every meeting where a topic came up, matched against titles, your notes, and transcript text.
- **`export-all --out ./calls --last 7d`** - bulk-export a week of calls to markdown for your PSA or a follow-up doc.
- **`talktime --by participant --since 30d`** - how much of a month of calls was you talking (your microphone) versus everyone else, without re-listening to any of them.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Granola MCP server runs locally, so for ChatGPT you start it with `granola-mcp --transport http` and put it behind your own HTTPS endpoint (a secure tunnel or reverse proxy). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Granola credentials once.

### Is my Granola data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need a Granola API key?

You need a Granola account. Cross-meeting sync pulls from the Granola public REST API using a Personal or Workspace API key (`GRANOLA_API_KEY`) from Granola Settings > API. Recent Granola desktop versions seal the local cache app-private, so the API key is the supported path. Run `granola-cli doctor` to confirm auth is working before anything touches data.

### Will this replace the Granola app?

No. Keep recording and reviewing meetings in Granola as usual. This skill sits alongside it for the cross-meeting questions the app answers one meeting at a time - attendee timelines, bulk export, talk-time, and topic search across everything you've recorded.

### How is this different from Granola's own AI?

Granola's AI answers questions inside a single meeting. This complements it by making every meeting queryable at once - an attendee's whole history, talk-time rolled up across dozens of calls, and search over the transcripts you have synced - the cross-call view the per-meeting summary can't give you. `sync` pulls transcripts for your 50 most recent meetings; `sync --full` pulls the whole library.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `meetings list/get`, `search`, `transcript get`, `attendee timeline/brief`, `talktime`, `stats`, `doctor` | Allow |
| Local writes | `sync`, `enrich`, `sync-api`, `export`, `export-all`, `extract`, `memo run`, `collect` (local mirror and files you choose; never your Granola account) | Allow, with output folders you choose |
| Credential | `auth set-token` / `logout`, `mcp-auth login` / `logout` | Human-in-the-loop only |

No command changes or deletes anything in your Granola account. The strongest control is the **scope of the API key you create** - a Personal key sees only your notes, a Workspace key sees workspace-visible notes. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Granola API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-07-20._
