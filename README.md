# MSP Skills — HaloPSA and Servosity, for the AI you already use

**MSP Skills** installs natural-language AI tools for **HaloPSA tickets** and **Servosity backups** directly into **Claude**, **ChatGPT**, **Codex**, **Cursor**, **Windsurf**, or any AI agent that speaks MCP. Free, open source, runs on your laptop. A local SQLite mirror lets your agent answer cross-client questions the live API can't return in one shot — no rate-limit hits, no per-tech SaaS fee, no data leaves your network. Built for MSP owners. No developer experience required.

> 🛠 **Built live with MSPs.** Join a free weekly **Build Session** at **[compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions)** to watch a Skill built against a real MSP system — or bring your own.

[![License: Apache 2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)
[![Skills](https://img.shields.io/badge/skills-2-green.svg)](./catalog.json)
[![MCP](https://img.shields.io/badge/MCP-compatible-7C3AED.svg)](https://modelcontextprotocol.io)
[![Agent Skills](https://img.shields.io/badge/Agent_Skills-spec-2E7D32.svg)](https://agentskills.io)
![Status](https://img.shields.io/badge/status-beta-yellow.svg)

## Works with your agent

The four agents MSP owners actually use:

| Your AI agent | Why MSPs use it | How to install MSP Skills |
| --- | --- | --- |
| **Claude Desktop** (Mac/Windows app) | The most common MSP-owner choice — no terminal, just a chat window | Run installer, add MSP block to `claude_desktop_config.json` |
| **ChatGPT** (Plus, Pro, Team, Business, Enterprise, Education)¹ | The brand most MSPs already pay for; pair with Developer Mode | Run installer, expose MCP over HTTPS, register as a Developer Mode connector |
| **Claude Code** (CLI) | For the technical-leaning MSP owner or your senior tech | Paste the install prompt into the chat, or run the installer yourself |
| **Codex CLI** (OpenAI) | Same audience as Claude Code, OpenAI side | Paste the install prompt, or run the installer |

¹ ChatGPT requires a paid plan (Free tier does not yet expose Developer Mode). MSP Skills' binaries are local (stdio) — for ChatGPT you expose them over HTTPS via the `mcp-remote` bridge. Step-by-step in [docs/which-agent.md](./docs/which-agent.md).

> **Also works with** Cursor, Windsurf, Cline, Continue.dev, Zed, GitHub Copilot, and Gemini CLI. MSP Skills speaks the open MCP standard, so any current or future MCP-capable agent can use it. Full per-tool deep-dive: **[docs/which-agent.md](./docs/which-agent.md)**.

## What's in the box

<!-- catalog:start -->
| Skill | System | Install (Skill) | Install (MCP) |
| --- | --- | --- | --- |
| [halopsa](./skills/halopsa) | HaloPSA, HaloITSM, HaloCRM | `bash skills/halopsa/install.sh` | [mcp-install](./skills/halopsa/mcp-install.md) |
| [servosity](./skills/servosity) | Servosity backup and DR | `bash skills/servosity/install.sh` | [mcp-install](./skills/servosity/mcp-install.md) |
<!-- catalog:end -->

The table is regenerated from each skill's `manifest.json` whenever a PR touches `skills/`. The machine-readable form is [`catalog.json`](./catalog.json). Both skills are **beta** and being validated live against real MSPs in weekly Build Sessions (see below).

## What makes this different

### Local mirror, not live calls

Most AI tools and MCP servers for PSAs and backup systems call the vendor's API on every question your agent asks. That works fine for "show me ticket #4421." It falls over at QBR time, when you're asking "how many backup-failure tickets across all 47 clients last quarter, grouped by engine?" — that's 47 paginated REST calls, rate-limit headaches, and a context window full of raw JSON the model has to re-read.

MSP Skills syncs HaloPSA and Servosity into a local SQLite mirror with full-text search. Cross-client and cross-engine questions become one local query: instant, offline, and the AI sees the answer, not the raw data.

### Works with the AI you already use

You don't have to switch agents to use MSP Skills. Same package, two interfaces: a **Claude Code / Codex Skill** for shell-style agents, and an **MCP server** for Claude Desktop, ChatGPT, Cursor, Windsurf, Cline, Continue.dev, Gemini, and Copilot. One install drops both binaries on your machine. Use one or both — your call. No vendor lock-in, no proprietary plugin format, no SaaS subscription that ties you to a single AI.

### MSP owner first, not developer first

You don't have to know JSON, regex, or what "stdio transport" means. Paste one sentence into Claude Code or Codex and your agent reads the Skill, installs the binary, and walks you through authentication. The CLIs **plan by default** — mutations show you what they would do before they do it. Every command has a tier (read / write / destructive) and a recommended agent policy. The hard stuff is hidden; the safety bar is high.

## Install in 60 seconds

### Path A — let your AI install it (recommended)

Paste this into **Claude Code** or **Codex CLI**:

> Set up the **HaloPSA** skill from https://github.com/servosity/msp-skills — read `skills/halopsa/SKILL.md`, run its install steps, then run `halopsa-cli --version` to confirm. Walk me through authentication.

Swap `halopsa` for `servosity` (and `halopsa-cli --version` for `servosity-cli doctor`) to install the Servosity skill. Your agent does the rest.

### Path B — run the installer yourself

**HaloPSA on macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
```

**HaloPSA on Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
```

**Servosity on macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

**Servosity on Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

Each installer drops both the CLI and the MCP server, so you can use the Skill (Claude Code / Codex) and the MCP server (Claude Desktop / ChatGPT / Cursor / etc.) from one install. For Claude Desktop, ChatGPT, Cursor, Windsurf, Cline, Continue, Gemini, or Copilot wire-up, see [docs/which-agent.md](./docs/which-agent.md) and each skill's `mcp-install.md`.

> **Now what?** Once `halopsa-cli --version` or `servosity-cli doctor` returns clean, you're 60 seconds from your first real query. **Bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions)** — we'll work it live with the MSP cohort and the same Skills you just installed.

## What your agent can do

Outcomes, not endpoints. Today, with the two skills available:

| Outcome | Skill | Command |
| --- | --- | --- |
| Pre-empt SLA breaches before a hand-off | halopsa | `halopsa-cli sla breaching --within 24h` |
| Find stale backups across every client | servosity | `servosity-cli stale-backups --days 7` |
| Build a per-client situational-awareness card | halopsa | `halopsa-cli client card "Acme Corp"` |
| Triage what needs attention across every client | servosity | `servosity-cli attention` |
| Pull a client's full backup picture for a ticket | servosity | `servosity-cli company show 4421` |
| See what changed across your fleet overnight | servosity | `servosity-cli drift --from yesterday --to now` |
| Find every ticket about backup failures across all clients | halopsa + servosity | combine `halopsa-cli tickets search` with `servosity-cli stale-backups` |

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not to local binaries directly. MSP Skills ships local binaries, so to use them with ChatGPT you run them on your machine and expose them via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step: each skill's `mcp-install.md`.

### Does this work with Claude?

Yes — **Claude Code**, **Claude Desktop**, and the **Claude.ai** web (via Claude Desktop's MCP). Claude Code reads the Skill directly; Claude Desktop loads MSP Skills as an MCP server you add to `claude_desktop_config.json`. Both paths are first-class. Most MSP owners start with Claude Desktop because it's a Mac/Windows app — no terminal.

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes — all of them. Each speaks MCP (some natively, some via an extension or marketplace). The cross-tool table at the top of this README links each one's install path. The pattern is the same: install the MSP Skills binary, point your AI tool at it, authenticate. Tool-specific gotchas (Copilot uses `servers` in `mcp.json` not `mcpServers`, Cline needs an `npx` workaround on Windows, etc.) are documented in [docs/which-agent.md](./docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install path is to paste one sentence into Claude Code or Codex — your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code, editing JSON in a text editor, or installing Node, Python, Docker, or a database. You will need to enter API credentials your PSA or backup tool gives you.

### Is my HaloPSA or Servosity data safe? Does it leave my network?

Your data stays on **your machine**. MSP Skills runs locally: the CLI and the MCP server are binaries on your laptop. The SQLite mirror sits in a local directory under your user account. The AI agent (Claude, ChatGPT, Codex) only sees what the CLI or MCP server returns — typically the result of a query, not raw bulk data. Credentials are read from your environment or your agent's config; they're never bundled into this repo or transmitted to MSP Skills servers (there are none).

### Will this hit my HaloPSA API rate limits?

Almost never. HaloPSA's rate limits aren't publicly documented and vary between cloud-hosted and self-hosted instances. MSP Skills syncs your HaloPSA data into a local SQLite mirror once, then incrementally; subsequent questions (triage, SLA breaches, client cards, cross-client analytics) run against the local mirror, not the live API. The big-batch QBR queries that get you 429'd with API-passthrough tools become a single SQL join here.

### How is this different from HaloPSA's built-in ChatGPT integration?

HaloPSA's built-in ChatGPT integration is great for ticket-by-ticket work — rewriting replies, summarizing one ticket, classifying sentiment. MSP Skills is the **MSP-owner-on-the-couch-with-Claude** layer: cross-client analytics, ad-hoc questions across thousands of tickets, multi-system queries that join HaloPSA with Servosity. The two complement each other; you don't have to choose.

### Can I run this on Windows?

Yes. The PowerShell installer in "Install in 60 seconds" above is the same install path as macOS / Linux, just packaged for PowerShell. The CLI and MCP binaries are native Windows builds. Cline users on Windows may need a small `npx` workaround documented in [docs/which-agent.md](./docs/which-agent.md).

### What does it cost?

The Skills, the CLI binaries, and the MCP servers are **free**. Apache-2.0 licensed — free to use commercially, free to fork. Servosity does not charge for API access required to run the Servosity skill. Other PSA, RMM, and backup vendors set their own API-access terms. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and those are billed by your AI provider, not by us.

### Can I try it on one client first?

Yes. Every command takes a client / company filter (e.g. `halopsa-cli client card "Acme Corp"`, `servosity-cli company show 4421`). Read-tier commands plan nothing and change nothing; you can try them against your live system safely. Write-tier commands run `--dry-run` until you pass `--confirm`. The safe starting move is a read-only triage against one client, see the output, then widen scope.

### What if my AI doesn't speak MCP?

If your AI doesn't yet support MCP and isn't Claude Code / Codex CLI (which read SKILL.md directly), MSP Skills won't fit yet. The CLI binaries (`halopsa-cli`, `servosity-cli`) still work standalone in a shell — you can pipe their output into anything. As more AI tools add MCP (the protocol is moving fast in 2026), they'll work with MSP Skills automatically without anything changing on our side.

## Safety model

These skills hold privileged, multi-tenant access to systems that run MSP businesses, so safety is a first-class concern, not a footnote:

- **You supply your own credentials at runtime.** Nothing is stored in this repo.
- **Mutations plan by default.** Each skill's CLI runs in dry-run / discovery mode and makes no change until you pass `--confirm`.
- **Every skill ships a permission matrix.** Each skill's `governance.md` tags commands read / write / destructive and tells you how to scope an agent.

The safe default for an autonomous agent is **read plus planned (dry-run) writes**; gate destructive and credential-touching operations behind a human. See [skills/halopsa/governance.md](./skills/halopsa/governance.md) and [skills/servosity/governance.md](./skills/servosity/governance.md).

## Tested by MSPs in Build Sessions

These skills are built and tested with real MSPs running them against their own production systems, live, in our free weekly Build Sessions. They are currently beta and being validated now. Join a session ([RSVP](https://compoundingteams.com/build-sessions)) to watch one run against a real system, or bring your own to co-build.

## Roadmap

We co-build the next skills live with MSPs in the weekly Build Session. The targets the MSP community asks for most:

- **M365 governance / Copilot data-exposure pre-check**
- **PSA**: Autotask, ConnectWise PSA (HaloPSA shipped)
- **NinjaOne fleet hygiene** (silent-failure detector)
- **RMM ticket-to-doc** (resolution to IT Glue / Hudu)
- **Datto RMM, Kaseya, Atera, Syncro**

Want one of these next? Bring the system to a Build Session (below) or open an issue.

## Don't see the system you need?

Tell us what to build next. **[Open an issue →](https://github.com/servosity/msp-skills/issues/new?template=skill-request.yml)** and name the PSA, RMM, backup, security, or M365 tool — the more votes a system gets, the faster it moves up the roadmap.

First time filing a GitHub issue? See **[docs/requesting-a-skill.md](./docs/requesting-a-skill.md)** — a 90-second walkthrough for MSP business owners. No terminal required, no developer experience needed.

## Co-build a new Skill or MCP with us live

Every Thursday we host a free Build Session where an MSP brings a system we have not covered (your PSA, RMM, backup product, or security tool) and we co-build the Claude Code Skill and MCP server live. You watch, you ask, you walk away with a working integration.

- Free weekly Build Sessions (Thursdays), co-built with a volunteer MSP.
- Access to every shipped Skill and MCP the day it merges.
- Conversations with other MSP owners about running an MSP as a Compounding Team.

RSVP for the next one at **[compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions)**.

## Contribute a Skill or MCP

If your MSP uses a system we have not covered, send a PR. We co-build alongside contributors in Build Sessions when that is easier than going it alone.

A skill PR includes: `SKILL.md` (with `vendor` frontmatter), a `README.md` with the non-affiliation banner, `install.sh` + `install.ps1`, `mcp-install.md` if it ships an MCP server, and ideally `pain-point.md` + `governance.md`. CI enforces the contract (DCO sign-off, frontmatter schema, required files, no secrets, no personal paths). See [CONTRIBUTING.md](./CONTRIBUTING.md) for the full checklist and the non-affiliation banner template.

## About Compounding Teams

MSPs are the channel that brings AI to small business. The durable moat is the [Compounding Teams](https://compoundingteams.com) methodology for running an MSP where every interaction with a tool, a customer, or a system makes the next one better. Loops close, feedback returns to the source, work compounds instead of evaporating. `msp-skills` is the part of that methodology you can install: the executable layer that lets your AI agent operate alongside your team in the same systems, with the same context, every day.

## Glossary (for the non-developer)

- **Skill** — a markdown file (and a binary it drives) that tells a Claude Code / Codex agent how to operate a specific tool. Think of it as a recipe card the AI reads on the fly.
- **MCP** — Model Context Protocol. The open standard that lets AI apps (Claude Desktop, ChatGPT, Cursor, Windsurf, etc.) call tools on a separate server (yours).
- **MCP server** — the small program on your machine that exposes the tools. MSP Skills includes one for HaloPSA (`halopsa-mcp`) and one for Servosity (`servosity-mcp`).
- **PSA** — Professional Services Automation. The system MSPs use for tickets, contracts, time, billing. HaloPSA, ConnectWise, Autotask are PSAs.
- **BCDR / Backup and DR** — Business Continuity and Disaster Recovery. Servosity is a BCDR vendor.
- **Cross-client analytics** — questions that span multiple clients ("how many backup failures across all 47 clients last quarter") rather than one ("show me ticket #4421").

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible — works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Built by [Servosity](https://www.servosity.com). Maintained by Damien Stevens. Apache-2.0 licensed. See [TRADEMARKS.md](./TRADEMARKS.md) for vendor non-affiliation and [SECURITY.md](./SECURITY.md) to report a vulnerability. Methodology: [Compounding Teams](https://compoundingteams.com). Generated CLIs and MCP servers built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).

_Last updated: 2026-05-28. Latest releases: [halopsa-v0.1.0](https://github.com/servosity/msp-skills/releases/tag/halopsa-v0.1.0) · [servosity-v0.1.0](https://github.com/servosity/msp-skills/releases/tag/servosity-v0.1.0)._
