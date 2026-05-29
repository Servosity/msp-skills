---
layout: default
title: "MSP Skills - HaloPSA and Servosity for the AI you already use"
description: "Free Claude Code skills + MCP servers for HaloPSA tickets and Servosity backups. Works with Claude, ChatGPT, Codex, Cursor, and any AI tool that speaks MCP. Built for MSP owners. No code required."
permalink: /
---

# MSP Skills - HaloPSA and Servosity, for the AI you already use

**MSP Skills** installs natural-language AI tools for **HaloPSA tickets** and **Servosity backups** directly into **Claude**, **ChatGPT**, **Codex**, **Cursor**, **Windsurf**, or any AI agent that speaks MCP. Free, open source, runs on your laptop. A local SQLite mirror lets your agent answer cross-client questions the live API can't return in one shot - no rate-limit hits, no per-tech SaaS fee, no data leaves your network. Built for MSP owners. No developer experience required.

> 🛠 **Built live with MSPs.** Join a free weekly **Build Session** at **[compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions)** - watch a Skill built against a real MSP system, or bring your own.

[Install in 60 seconds →](#install-in-60-seconds){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills){: .btn}

## Works with your agent

The four agents MSP owners actually use:

| Your AI agent | Why MSPs use it | Install |
| --- | --- | --- |
| **Claude Desktop** | The most common MSP-owner choice - no terminal, just a chat window | [Setup →](/integrations/claude-desktop/) |
| **ChatGPT** (Plus/Pro+) | The brand most MSPs already pay for; pair with Developer Mode | [Setup →](/integrations/chatgpt/) |
| **Claude Code** (CLI) | For technical-leaning MSP owners or senior techs | [Setup →](/integrations/claude-code/) |
| **Codex CLI** (OpenAI) | Same audience as Claude Code, OpenAI side | [Setup →](/integrations/codex/) |

> **Also works with** Cursor, Windsurf, Cline, Continue.dev, Zed, GitHub Copilot, and Gemini CLI. Full per-tool deep-dive in [which agent →](/which-agent/).

## What's in the box

| Skill | What it does | Install |
| --- | --- | --- |
| [HaloPSA →](/skills/halopsa/) | Ticket triage, SLA-breach pre-emption, per-client situational awareness, cross-client analytics. Local SQLite mirror so QBR-time questions don't hit rate limits. | [/skills/halopsa →](/skills/halopsa/) |
| [Servosity →](/skills/servosity/) | Fleet-wide backup triage, stale-backup detection, overnight drift, cross-engine analytics. Local fleet mirror - Friday-email-ready in seconds. | [/skills/servosity →](/skills/servosity/) |

## What makes this different

### Local mirror, not live calls

Most AI integrations for PSA and backup systems call the vendor's API on every question. That's fine for one ticket. It dies at QBR time, when you're asking *"how many backup-failure tickets across all 47 clients last quarter?"* - that's 47 paginated REST calls, rate-limit headaches, and a context window full of raw JSON the model has to re-read. MSP Skills syncs HaloPSA and Servosity into a local SQLite mirror with full-text search. Cross-client and cross-engine questions become one local query: instant, offline, and the AI sees the answer, not the raw data.

### Works with the AI you already use

You don't have to switch agents. Same package, two interfaces: a **Claude Code / Codex Skill** for shell-style agents, and an **MCP server** for Claude Desktop, ChatGPT, Cursor, Windsurf, Cline, Continue.dev, Gemini, and Copilot. One install drops both binaries on your machine. Use one or both - your call. No vendor lock-in, no proprietary plugin format, no SaaS subscription that ties you to a single AI.

### MSP owner first, not developer first

You don't have to know JSON, regex, or what "stdio transport" means. Paste one sentence into Claude Code or Codex and your agent reads the Skill, installs the binary, and walks you through authentication. The CLIs **plan by default** - mutations show you what they would do before they do it. Every command has a tier (read / write / destructive) and a recommended agent policy. The hard stuff is hidden; the safety bar is high.

## Install in 60 seconds

### Path A - let your AI install it (recommended)

Paste this into **Claude Code** or **Codex CLI**:

> Set up the **HaloPSA** skill from https://github.com/servosity/msp-skills - read `skills/halopsa/SKILL.md`, run its install steps, then run `halopsa-cli --version` to confirm. Walk me through authentication.

Swap `halopsa` for `servosity` to install the Servosity skill.

### Path B - run the installer yourself

**HaloPSA on macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
```

**HaloPSA on Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
```

Swap `halopsa` for `servosity` in either command. New to all this? See the [first-time setup guide →](/guides/first-time-setup/).

> **Now what?** Once `halopsa-cli --version` or `servosity-cli doctor` returns clean, **bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions)** - we'll work it live with the MSP cohort and the same Skills you just installed.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local binaries directly. MSP Skills ships local binaries, so to use them with ChatGPT you run them on your machine and expose them via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step: [ChatGPT integration →](/integrations/chatgpt/).

### Does this work with Claude?

Yes - **Claude Code**, **Claude Desktop**, and the **Claude.ai** web (via Claude Desktop's MCP). Claude Code reads the Skill directly; Claude Desktop loads MSP Skills as an MCP server you add to `claude_desktop_config.json`. Both paths are first-class. Most MSP owners start with Claude Desktop because it's a Mac/Windows app - no terminal.

### Do I need to know how to code?

No. The recommended install path is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code, editing JSON in a text editor, or installing Node, Python, Docker, or a database. You will need to enter API credentials your PSA or backup tool gives you.

### Is my HaloPSA or Servosity data safe? Does it leave my network?

Your data stays on **your machine**. MSP Skills runs locally: the CLI and the MCP server are binaries on your laptop. The SQLite mirror sits in a local directory under your user account. The AI agent (Claude, ChatGPT, Codex) only sees what the CLI or MCP server returns - typically the result of a query, not raw bulk data. Credentials are read from your environment or your agent's config; they're never bundled into the repo or transmitted to MSP Skills servers (there are none).

### Will this hit my HaloPSA API rate limits?

Almost never. HaloPSA's rate limits aren't publicly documented and vary between cloud-hosted and self-hosted instances. MSP Skills syncs your HaloPSA data into a local SQLite mirror once, then incrementally; subsequent questions (triage, SLA breaches, client cards, cross-client analytics) run against the local mirror, not the live API. The big-batch QBR queries that get you 429'd with API-passthrough tools become a single SQL join here.

### How is this different from HaloPSA's built-in ChatGPT integration?

HaloPSA's built-in ChatGPT integration is great for ticket-by-ticket work - rewriting replies, summarizing one ticket, classifying sentiment. MSP Skills is the **MSP-owner-on-the-couch-with-Claude** layer: cross-client analytics, ad-hoc questions across thousands of tickets, multi-system queries that join HaloPSA with Servosity. The two complement each other; you don't have to choose.

### Can I run this on Windows?

Yes. The PowerShell installer in "Install in 60 seconds" above is the same install path as macOS / Linux, just packaged for PowerShell. The CLI and MCP binaries are native Windows builds.

### What does it cost?

The Skills, the CLI binaries, and the MCP servers are **free**. Apache-2.0 licensed - free to use commercially, free to fork. Servosity does not charge for API access required to run the Servosity skill. Other PSA, RMM, and backup vendors set their own API-access terms. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and those are billed by your AI provider, not by us.

### Can I try it on one client first?

Yes. Every command takes a client / company filter (e.g. `halopsa-cli client card "Acme Corp"`, `servosity-cli company show 4421`). Read-tier commands plan nothing and change nothing; you can try them against your live system safely. Write-tier commands run `--dry-run` until you pass `--confirm`. The safe starting move is a read-only triage against one client, see the output, then widen scope.

### What if my AI doesn't speak MCP?

If your AI doesn't yet support MCP and isn't Claude Code / Codex CLI (which read SKILL.md directly), MSP Skills won't fit yet. The CLI binaries (`halopsa-cli`, `servosity-cli`) still work standalone in a shell - you can pipe their output into anything. As more AI tools add MCP (the protocol is moving fast in 2026), they'll work with MSP Skills automatically without anything changing on our side.

## Tested by MSPs in Build Sessions

These skills are built and tested with real MSPs running them against their own production systems, live, in our free weekly Build Sessions. They are currently beta and being validated now. **[RSVP for the next one →](https://compoundingteams.com/build-sessions)**.

## Don't see the system you need?

Tell us what to build next. **[Open an issue →](https://github.com/servosity/msp-skills/issues/new?template=skill-request.yml)** and name the PSA, RMM, backup, security, or M365 tool. First time filing a GitHub issue? See [the 90-second walkthrough →](/requesting-a-skill/).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Built by [Servosity](https://www.servosity.com). Maintained by Damien Stevens. Apache-2.0 licensed. Methodology: [Compounding Teams](https://compoundingteams.com).
