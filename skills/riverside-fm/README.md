# Riverside + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Riverside
> API. Not affiliated with, endorsed by, or sponsored by RiversideFM, Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/riverside-fm/">
    <img src="../../docs/assets/social/riverside-fm/wide-1200x630.png" alt="Riverside.fm - MCP server and Claude Code Skill" width="600">
  </a>
</p>
<p align="center"><sub><a href="https://msp-skills.compoundingteams.com/skills/riverside-fm/">Full skill page</a> - install, outcomes, safety model.</sub></p>
<!-- media:end -->

Riverside API surface as MCP tools. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Riverside skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `riverside-fm-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `riverside-fm-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `riverside-fm-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Riverside MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `supergateway` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `riverside-fm-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Riverside MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/riverside-fm-v0.1.0/riverside-fm-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Riverside release on the [releases page](https://github.com/servosity/msp-skills/releases?q=riverside-fm).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install riverside-fm@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Riverside Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/riverside-fm/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/riverside-fm/install.ps1 | iex`. Then authenticate per the README and run `riverside-fm-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/riverside-fm/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/riverside-fm/install.sh)
```

The installer drops both `riverside-fm-cli` (the CLI) and `riverside-fm-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
riverside-fm-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/riverside-fm/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/riverside-fm/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update riverside-fm@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/riverside-fm --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/riverside-fm --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `riverside-fm-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the riverside-fm skill from https://github.com/servosity/msp-skills/tree/main/skills/riverside-fm. The skill defines how its required CLI (`riverside-fm-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

See [mcp-install.md](./mcp-install.md) for the credentials `riverside-fm-cli` needs.


## What this skill does

| Question you keep asking | Command |
| --- | --- |
| Back up everything in a studio to disk? | `riverside-fm-cli bulk export --studio my-studio --out ./archive` |
| Get the most useful asset for one recording in a single shot? | `riverside-fm-cli grab <session-id> --out ./dl` |
| Find a quote across my whole transcript archive? | `riverside-fm-cli search "network effects" --json` |
| Turn a transcript into captions for a web player? | `riverside-fm-cli transcripts convert <session-id> --format srt --out ep.srt` |
| Who talked the most in this episode? | `riverside-fm-cli transcripts talktime <session-id> --json` |
| Which takes in a studio are fully ready to edit? | `riverside-fm-cli ready --studio my-studio` |
| Pull every Magic Clip for a project with fresh URLs? | `riverside-fm-cli clips harvest --project 69fcda9fba030a19ae93a526 --download --out ./clips` |
| Refresh expiring CloudFront media links before they die? | `riverside-fm-cli media refresh --project 69fcda9fba030a19ae93a526 --prefetch --out ./media` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Riverside integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "find the episode where we mentioned that customer story across two years of a weekly show" or "archive every take in this studio before we cancel the plan."

This skill syncs Riverside into a **local SQLite mirror** with full-text search. Aggregate questions become one local query: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `bulk export`, `clips harvest`, and `search` join across studios, projects, takes, transcripts, and clip-assets - work a stateless API wrapper can't do.

## The pain this closes

Riverside records studio-quality podcast and video, but getting your own content back out is a manual slog. There is no bulk export: every transcript, audio track, and video file comes out one click at a time, per take, down the Studio then Project then Take path. Back up a fifty-episode show and you face hundreds of clicks. The friction is real enough that podcasters in community groups - such as the [Podcasting](https://www.reddit.com/r/podcasting/) subreddit and the podcast-host Facebook groups - keep asking how to "batch download Riverside recordings" to archive a series or free up storage. And the official API that would fix it is gated behind a custom-priced Business plan, so Pro, Live, and Webinar accounts get no supported way to back up, search, or re-format their own recordings.

This skill closes that gap:

- **`bulk export`** - archive a whole studio (or a date range) to disk, resumable.
- **`grab`** - one command that downloads the most useful asset for a recording (transcript, then audio, then video).
- **`search`** - full-text search across every transcript in your local mirror.
- **`transcripts convert`** - turn a transcript into WebVTT / SRT / Markdown captions the Riverside UI never exposes.
- **`clips harvest`** - pull every Magic Clip for a project with freshly refreshed download URLs.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Riverside MCP server is local, so for ChatGPT you expose it via the `supergateway` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Riverside credentials once.

### Is my Riverside data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need a Riverside Business plan or an API key?

No. The official Riverside API requires a custom-priced Business plan. This CLI reuses the session cookies from your already-logged-in browser to reach the same internal API the Riverside web app uses, so it works on **Pro, Live, and Webinar** accounts that can't issue an API key. Run `riverside-fm-cli auth login --chrome` once.

### Can this download other people's recordings?

No. It authenticates as **you**, with your own browser session, and only reaches the studios and takes your account already has access to. It is scoped to exporting your own content, not scraping anyone else's.

### Will this change anything in my Riverside account?

The commands you use to export, search, and convert all **read** - they only download and transform your data. The CLI does include the generic `import` command, which can POST records from a local file via the API's create/upsert path; it is not part of any workflow in this skill, and an agent should treat it as a reviewed write. Nothing in this CLI deletes or publishes.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

The Riverside data commands in this skill **read**: the export, search, download, and transcript commands only GET from your account. The one mutating command is the generic `import`, which POSTs records from a local JSONL file - it is not part of any export workflow here.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `bulk export`, `grab`, `search`, `transcripts convert`, `transcripts talktime`, `clips harvest`, `media refresh`, `ready`, `wait`, `sync` | Allow |
| Write (routine) | `import` (POSTs records from a local JSONL via the API's create/upsert path); the endpoint-mirror capability checks the safety scanner flagged by verb name (`ai can-create-event`, `takes get-assets`, `clips get-patches`) are themselves read-only GETs | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | none - this CLI exposes no delete, publish, or account-config command | Human-in-the-loop only |

The strongest control is the **scope you grant the Riverside credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Riverside API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-30._
