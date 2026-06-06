# Run your MSP tools by asking - MCP servers and Skills for the AI you already use

**MSP Skills** connects your PSA, RMM, backup, and more to the AI you already use - **Claude**, **ChatGPT**, **Codex**, **Cursor**, **Windsurf**, or any agent that speaks MCP. Ask a plain-English question about your stack and get a real answer back.
<!-- hero-live:start -->
14 connectors are live today - including Servosity, ConnectWise PSA, HubSpot, and HaloPSA - and more PSA, RMM, backup, and M365 connectors ship every week.
<!-- hero-live:end -->
Free, open source, runs on your laptop. A local SQLite mirror lets your agent answer cross-client questions the live API can't return in one shot - no rate-limit hits, no per-tech SaaS fee, no data leaves your network. Built for MSP owners. No developer experience required.

> **New to the term?** What this repo calls an **MCP server** is what ChatGPT calls an *app* or *connector*, Claude on the web calls a *connector*, Microsoft Copilot calls a *connector*, and Claude Code calls a *Skill*. Same standard underneath: the [Model Context Protocol](https://modelcontextprotocol.io). Full plain-language answer: **[What is an MCP server?](https://msp-skills.compoundingteams.com/what-is-an-mcp-server/)**.

[![License: Apache 2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](./LICENSE)
[![Skills](https://img.shields.io/badge/skills-14-green.svg)](./catalog.json)
[![MCP](https://img.shields.io/badge/MCP-compatible-1f6feb.svg)](https://modelcontextprotocol.io)
[![Agent Skills](https://img.shields.io/badge/Agent_Skills-spec-2E7D32.svg)](https://agentskills.io)
![Status](https://img.shields.io/badge/status-beta-yellow.svg)

[![Run your MSP tools by asking - 14-second demo](./docs/assets/video/hero-14s.gif)](https://msp-skills.compoundingteams.com/)

<p align="center"><a href="https://msp-skills.compoundingteams.com/">▶ Watch the full demo with sound</a> - every skill page has its own 30-second walkthrough (<a href="https://msp-skills.compoundingteams.com/skills/hubspot/">example</a>). Demo data is simulated; every command shown exists in the real CLI.</p>

## What's in the box

<!-- catalog:start -->
| Skill | System | Status | Install |
| --- | --- | --- | --- |
| [msp-skills-concierge](./skills/_meta) | Connector recommendations + guided install for the msp-skills catalog | ![Meta](https://img.shields.io/badge/Meta-skill-6B7280) | [Marketplace](./skills/_meta/README.md) |
| [autotask](./skills/autotask) | Autotask PSA | ![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308) | [Install](./skills/autotask/README.md) |
| [cipp](./skills/cipp) | the CyberDrain Improved Partner Portal (CIPP), the open-source Microsoft 365 multi-tenant management platform for MSPs | ![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308) | [Install](./skills/cipp/README.md) |
| [connectwise-manage](./skills/connectwise-manage) | ConnectWise PSA (Manage) | ![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308) | [Install](./skills/connectwise-manage/README.md) |
| [halopsa](./skills/halopsa) | HaloPSA, HaloITSM, HaloCRM | ![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308) | [Install](./skills/halopsa/README.md) |
| [hubspot](./skills/hubspot) | HubSpot CRM: contacts, companies, deals, tickets, engagements | ![Live-verified](https://img.shields.io/badge/Live--verified-by_a_real_MSP-2E7D32) | [Install](./skills/hubspot/README.md) |
| [microsoft-graph](./skills/microsoft-graph) | Microsoft 365 / Entra tenant administration via Microsoft Graph | ![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308) | [Install](./skills/microsoft-graph/README.md) |
| [mspbots](./skills/mspbots) | MSPbots BI: datasets, KPI snapshots, ticket analytics, exports | ![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308) | [Install](./skills/mspbots/README.md) |
| [n-central](./skills/n-central) | N-able N-central RMM: devices, org tree, issue triage, cross-tenant search | ![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308) | [Install](./skills/n-central/README.md) |
| [ninjaone](./skills/ninjaone) | NinjaOne RMM: devices, patch compliance, backup gaps, AV blast-radius, health, drift | ![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308) | [Install](./skills/ninjaone/README.md) |
| [pax8](./skills/pax8) | Pax8 cloud marketplace billing, subscriptions, reconciliation, MRR, and usage overages | ![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308) | [Install](./skills/pax8/README.md) |
| [sentinelone](./skills/sentinelone) | SentinelOne Singularity: threat triage, mitigation, agent fleet health, and protection-coverage gaps across customer sites | ![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308) | [Install](./skills/sentinelone/README.md) |
| [servosity](./skills/servosity) | Servosity backup and DR: fleet attention, stale backups, QBR reports, restores, billing | ![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308) | [Install](./skills/servosity/README.md) |
| [superops](./skills/superops) | SuperOps PSA+RMM: tickets, SLAs, assets, alerts, clients, contracts, invoices, worklogs | ![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308) | [Install](./skills/superops/README.md) |
| [threatlocker](./skills/threatlocker) | ThreatLocker Portal: approvals, audit log, device health, and policy across customer tenants | ![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308) | [Install](./skills/threatlocker/README.md) |
<!-- catalog:end -->

> **About the badges.** `Live-verified` (green) means a real MSP confirmed this skill against a live production tenant - the badge carries the date and source. `Awaiting live verification` (amber) means it passes every mechanical gate (build, command-surface-vs-docs claims check, install dry-run) but no one has reported a live run yet - **be the first**: run it against your tenant and [report that it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

> 🛠 **Built live with MSPs.** Join a free weekly **Build Session** at **[compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions)** to watch a Skill built against a real MSP system - or bring your own.

## Install in 60 seconds

### Path A - paste one prompt into your AI agent (recommended)

<!-- install-featured:start -->
**Not sure which connector to pick? Let your agent decide.** Paste this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Read https://github.com/Servosity/msp-skills and, using everything you know about me and how my MSP works, recommend which connectors I should install - then install the ones I approve.

Prefer a guided version? The [concierge](./skills/_meta) does the same from inside Claude Code: `/plugin marketplace add Servosity/msp-skills`, then `/plugin install msp-skills-concierge@msp-skills`.

**Or install a specific connector.** The **Servosity** prompt:

> Install the Servosity Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.ps1 | iex`. Then authenticate with `SERVOSITY_MSP_TOKEN=<your-partner-token> servosity-cli doctor` and run `servosity-cli --help` to explore.

And the **ConnectWise PSA (Manage)** prompt:

> Install the ConnectWise PSA (Manage) Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-manage/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-manage/install.ps1 | iex`. Then authenticate per the README and run `connectwise-manage-cli --help` to explore.

The same prompt works for **every connector in the table above** - swap the skill name and slug. If your agent can't run shell, use Path B below.
<!-- install-featured:end -->

### Path B - run the installer yourself

**On macOS / Linux** (swap the slug for any connector in the table above):

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

**On Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

Each installer drops both the CLI and the MCP server, so you can use the Skill (Claude Code / Codex) and the MCP server (Claude Desktop / ChatGPT / Cursor / etc.) from one install. For Claude Desktop, ChatGPT, Cursor, Windsurf, Cline, Continue, Gemini, or Copilot wire-up, see [docs/which-agent.md](./docs/which-agent.md) and each skill's `mcp-install.md`.

> **Now what?** Once your connector's `--version` (or `doctor`) returns clean, you're 60 seconds from your first real query. **Bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions)** - we'll work it live with the MSP cohort and the same Skills you just installed.

## What your agent can do

<!-- agent-can-do:start -->
Outcomes, not hype - drawn from each of the 14 connectors' skill pages:

| Outcome | Skill | Command |
| --- | --- | --- |
| Which approved time entries haven't been invoiced yet? | autotask | `autotask-cli unbilled` |
| Which tenants still have users without MFA registered? | cipp | `cipp-cli posture --dimension mfa` |
| Which tickets did we touch this week that have zero time logged against them? | connectwise-manage | `connectwise-manage-cli unbilled --since 7d` |
| What's about to breach SLA in the next 24 hours? | halopsa | `halopsa-cli sla breaching --within 24h --team Support --json` |
| Which open deals have gone cold with no engagement in the last three weeks? | hubspot | `hubspot-cli stale deals --days 21 --owner me` |
| Which SKUs are we paying for but not fully using, ranked by wasted seats? | microsoft-graph | `microsoft-graph-cli licenses waste --agent` |
| Is our open-ticket backlog up or down versus last week? | mspbots | `mspbots-cli trend open_tickets --agg count` |
| What's red right now, grouped by customer and ranked by severity? | n-central | `n-central-cli triage --by customer` |
| Which organizations are below 95% patch compliance? | ninjaone | `ninjaone-cli patch-compliance --min-pct 95` |
| Where is my billing leaking - invoiced for a cancelled product, or active but never billed? | pax8 | `pax8-cli reconcile` |
| What should I triage first across all my client sites right now? | sentinelone | `sentinelone-cli threats triage` |
| Where is my attention needed today, ranked worst-first? | servosity | `servosity-cli attention --top 5` |
| Who's about to breach SLA, grouped by technician? | superops | `superops-cli sla-watch --by tech --window 4h` |
| What application approvals are pending across all my clients right now? | threatlocker | `threatlocker-cli approvals triage --all-tenants` |
<!-- agent-can-do:end -->

Cross-skill questions compose too: find every ticket about backup failures across all clients by combining `halopsa-cli tickets search` with `servosity-cli stale-backups`.

## Works with your agent

The five agents MSP owners actually use:

| Your AI agent | Why MSPs use it | How to install MSP Skills |
| --- | --- | --- |
| **Claude Desktop** (Mac/Windows app) | The most common MSP-owner choice - no terminal, just a chat window | Run installer, then Settings > Extensions to register the MCP server |
| **ChatGPT** (paid plans) | The brand most MSPs already pay for; pair with Developer Mode | Run installer, expose MCP over HTTPS, register as a Developer Mode connector |
| **Claude Code** (CLI) | For the technical-leaning MSP owner or your senior tech | Paste the install prompt above |
| **Codex CLI** (OpenAI) | OpenAI's terminal agent for the same audience | Paste the install prompt above |
| **Claude Cowork** (Anthropic, GA Mar 2026) | Desktop agent that runs shell on your behalf | Paste the install prompt above |

> **Also works with** Cursor, Windsurf, Cline, Continue.dev, Zed, GitHub Copilot, and Gemini CLI. MSP Skills speaks the open MCP standard, so any current or future MCP-capable agent can use it. Full per-tool deep-dive: **[docs/which-agent.md](./docs/which-agent.md)**.

> **Run more than one agent?** Install MSP Skills across all 51+ supported agents at once: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). Details in [docs/which-agent.md](./docs/which-agent.md#install-across-all-your-agents-at-once).

## What makes this different

### Local mirror, not live calls

Most AI tools and MCP servers for PSAs and backup systems call the vendor's API on every question your agent asks. That works fine for "show me ticket #4421." It falls over at QBR time, when you're asking "how many backup-failure tickets across all 47 clients last quarter, grouped by engine?" - that's 47 paginated REST calls, rate-limit headaches, and a context window full of raw JSON the model has to re-read.

MSP Skills syncs each connected system into a local SQLite mirror with full-text search. Cross-client and cross-engine questions become one local query: instant, offline, and the AI sees the answer, not the raw data.

### Works with the AI you already use

You don't have to switch agents to use MSP Skills. Same package, two interfaces: a **Claude Code / Codex Skill** for shell-style agents, and an **MCP server** for Claude Desktop, ChatGPT, Cursor, Windsurf, Cline, Continue.dev, Gemini, and Copilot. One install drops both binaries on your machine. Use one or both - your call. No vendor lock-in, no proprietary plugin format, no SaaS subscription that ties you to a single AI.

### MSP owner first, not developer first

You don't have to know JSON, regex, or what "stdio transport" means. Paste one sentence into Claude Code or Codex and your agent reads the Skill, installs the binary, and walks you through authentication. Every command has a tier (read / write / destructive) and a recommended agent policy in each skill's `governance.md` - reads are always safe, and the recommended agent rule is to preview writes with `--dry-run` and require your approval before any mutation. The hard stuff is hidden; the safety bar is high.

## Safety model

These skills hold privileged, multi-tenant access to systems that run MSP businesses, so safety is a first-class concern, not a footnote:

- **You supply your own credentials at runtime.** Nothing is stored in this repo.
- **Every skill ships a permission matrix.** Each skill's `governance.md` tags commands read / write / destructive and tells you how to scope an agent. The safe default for an autonomous agent is **read plus previewed writes** (`--dry-run` first, human approval before the real thing); gate destructive and credential-touching operations behind a human.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not to local binaries directly. MSP Skills ships local binaries, so to use them with ChatGPT you run them on your machine and expose them via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step: each skill's `mcp-install.md`.

### Does this work with Claude?

Yes - **Claude Code**, **Claude Desktop**, and **Claude.ai** web (via Claude Desktop's MCP). Claude Code reads the Skill directly. For Claude Desktop, run the installer, then go to **Settings > Extensions** to register the MCP server - the panel walks you through it without editing JSON. (Power users can still hand-edit `claude_desktop_config.json` via Settings > Developer > Edit Config if you prefer.) Both paths are first-class.

### Is my PSA, CRM, or backup data safe? Does it leave my network?

Your data stays on **your machine**. MSP Skills runs locally: the CLI and the MCP server are binaries on your laptop. The SQLite mirror sits in a local directory under your user account. The AI agent (Claude, ChatGPT, Codex) only sees what the CLI or MCP server returns - typically the result of a query, not raw bulk data. Credentials are read from your environment or your agent's config; they're never bundled into this repo or transmitted to MSP Skills servers (there are none).

<details>
<summary><strong>More questions</strong> - Codex/Cursor/Copilot support, coding skill needed, rate limits, vendor AI overlap, Windows, cost, trying one client first, non-MCP agents</summary>

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them. Each speaks MCP (some natively, some via an extension or marketplace). The cross-tool table at the top of this README links each one's install path. The pattern is the same: install the MSP Skills binary, point your AI tool at it, authenticate. Tool-specific gotchas (Copilot uses `servers` in `mcp.json` not `mcpServers`, Cline needs an `npx` workaround on Windows, etc.) are documented in [docs/which-agent.md](./docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install path is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code, editing JSON in a text editor, or installing Node, Python, Docker, or a database. You will need to enter API credentials your PSA or backup tool gives you.

### Will this hit my vendor's API rate limits?

Almost never. Each connector syncs your data into a local SQLite mirror once, then incrementally; subsequent questions (triage, SLA breaches, client cards, cross-client analytics) run against the local mirror, not the live API. The big-batch QBR queries that get you 429'd with API-passthrough tools become a single SQL join here.

### How is this different from my vendor's built-in AI?

Vendor-native AI is great for ticket-by-ticket work - rewriting replies, summarizing one ticket, classifying sentiment. MSP Skills is the **MSP-owner-on-the-couch-with-Claude** layer: cross-client analytics, ad-hoc questions across thousands of tickets, multi-system queries that join your PSA with your backup tool. The two complement each other; you don't have to choose.

### Can I run this on Windows?

Yes. The PowerShell installer in "Install in 60 seconds" above is the same install path as macOS / Linux, just packaged for PowerShell. The CLI and MCP binaries are native Windows builds. Cline users on Windows may need a small `npx` workaround documented in [docs/which-agent.md](./docs/which-agent.md).

### What does it cost?

The Skills, the CLI binaries, and the MCP servers are **free**. Apache-2.0 licensed - free to use commercially, free to fork. Servosity does not charge for API access required to run the Servosity skill. Other PSA, RMM, and backup vendors set their own API-access terms. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and those are billed by your AI provider, not by us.

### Can I try it on one client first?

Yes. Every command takes a client / company filter (e.g. `halopsa-cli client card "Acme Corp"`, `servosity-cli company show 4421`). Read-tier commands change nothing; you can try them against your live system safely. For writes, each skill's `governance.md` tags every command with a recommended agent policy - preview with `--dry-run` and approve before the real thing. The safe starting move is a read-only triage against one client, see the output, then widen scope.

### What if my AI doesn't speak MCP?

If your AI doesn't yet support MCP and isn't Claude Code / Codex CLI (which read SKILL.md directly), MSP Skills won't fit yet. Every connector's CLI binary still works standalone in a shell - you can pipe its output into anything. As more AI tools add MCP (the protocol is moving fast in 2026), they'll work with MSP Skills automatically without anything changing on our side.

</details>

## Built and tested live with MSPs

These skills are built and tested with real MSPs running them against their own production systems, live, in our free weekly Build Sessions (Thursdays). An MSP brings a system we have not covered - your PSA, RMM, backup product, or security tool - and we co-build the Claude Code Skill and MCP server live. You watch, you ask, you walk away with a working integration, and every shipped Skill lands here the day it merges.

RSVP for the next one at **[compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions)**.

## Roadmap

We co-build the next skills live with MSPs in the weekly Build Session. The targets the MSP community asks for most, in demand order:

- **NinjaOne fleet hygiene** (the fastest-growing RMM; silent-failure detector)
- **Autotask PSA** (ConnectWise PSA and HaloPSA shipped - Autotask completes the big three)
- **IT Glue / Hudu** (RMM ticket-to-doc: resolution to documentation)
- **M365 governance / Copilot data-exposure pre-check**
- **Datto RMM, Kaseya VSA, Atera, Syncro**

Want one of these next? Bring the system to a Build Session (above) or open an issue.

## Don't see the system you need?

**Easiest path - tell your AI.** Paste this into Claude Code, Codex, Claude Desktop, or ChatGPT:

> Open an issue on https://github.com/servosity/msp-skills using the skill-request template. The system I want is **[YOUR SYSTEM NAME]** - my one question for the AI would be **"[YOUR QUESTION]"**, I'm on **[Mac or Windows]**, and my MSP has about **[N] techs**. Submit it.

Your AI fills the form and posts the issue. Done. (You'll need to be signed in to GitHub in your browser or have a GitHub CLI token where your AI can see it.)

**Don't have an AI set up yet?** Open the issue directly: **[Submit a skill request →](https://github.com/Servosity/msp-skills/issues/new?template=skill-request.yml)**. First time filing a GitHub issue? See **[docs/requesting-a-skill.md](./docs/requesting-a-skill.md)** for a 90-second walkthrough.

More votes (👍 reactions on an issue) move a system up the roadmap. Share with one MSP friend who'd want the same skill.

## Contribute a Skill or MCP

If your MSP uses a system we have not covered, send a PR. We co-build alongside contributors in Build Sessions when that is easier than going it alone.

A skill PR includes: `SKILL.md` (with `vendor` frontmatter), a `README.md` with the non-affiliation banner, `install.sh` + `install.ps1`, `mcp-install.md` if it ships an MCP server, and ideally `pain-point.md` + `governance.md`. CI enforces the contract (DCO sign-off, frontmatter schema, required files, no secrets, no personal paths). See [CONTRIBUTING.md](./CONTRIBUTING.md) for the full checklist and the non-affiliation banner template.

## About Compounding Teams

MSPs are the channel that brings AI to small business. The durable moat is the [Compounding Teams](https://compoundingteams.com) methodology for running an MSP where every interaction with a tool, a customer, or a system makes the next one better. Loops close, feedback returns to the source, work compounds instead of evaporating. `msp-skills` is the part of that methodology you can install: the executable layer that lets your AI agent operate alongside your team in the same systems, with the same context, every day.

<details>
<summary><strong>Glossary</strong> (for the non-developer)</summary>

- **Skill** - a markdown file (and a binary it drives) that tells a Claude Code / Codex agent how to operate a specific tool. Think of it as a recipe card the AI reads on the fly.
- **MCP** - Model Context Protocol. The open standard that lets AI apps (Claude Desktop, ChatGPT, Cursor, Windsurf, etc.) call tools on a separate server (yours).
- **MCP server** - the small program on your machine that exposes the tools. Every MSP Skills connector ships one (e.g. `halopsa-mcp`, `servosity-mcp`).
- **PSA** - Professional Services Automation. The system MSPs use for tickets, contracts, time, billing. HaloPSA, ConnectWise, Autotask are PSAs.
- **BCDR / Backup and DR** - Business Continuity and Disaster Recovery. Servosity is a BCDR vendor.
- **Cross-client analytics** - questions that span multiple clients ("how many backup failures across all 47 clients last quarter") rather than one ("show me ticket #4421").

</details>

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](https://docs.openclaw.ai), with `metadata.openclaw` frontmatter pre-wired so `openclaw skills install` works on every skill.

Built by [Servosity](https://www.servosity.com). Maintained by Damien Stevens. Apache-2.0 licensed. See [TRADEMARKS.md](./TRADEMARKS.md) for vendor non-affiliation and [SECURITY.md](./SECURITY.md) to report a vulnerability. Methodology: [Compounding Teams](https://compoundingteams.com). Generated CLIs and MCP servers built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).

<!-- footer-releases:start -->
_Last updated: 2026-06-06. Latest releases: [autotask-v0.1.0](https://github.com/servosity/msp-skills/releases/tag/autotask-v0.1.0) · [cipp-v0.1.0](https://github.com/servosity/msp-skills/releases/tag/cipp-v0.1.0) · [connectwise-manage-v0.1.1](https://github.com/servosity/msp-skills/releases/tag/connectwise-manage-v0.1.1) · [halopsa-v0.2.0](https://github.com/servosity/msp-skills/releases/tag/halopsa-v0.2.0) · [hubspot-v0.1.0](https://github.com/servosity/msp-skills/releases/tag/hubspot-v0.1.0) · [microsoft-graph-v0.1.0](https://github.com/servosity/msp-skills/releases/tag/microsoft-graph-v0.1.0) · [mspbots-v0.1.0](https://github.com/servosity/msp-skills/releases/tag/mspbots-v0.1.0) · [n-central-v0.1.0](https://github.com/servosity/msp-skills/releases/tag/n-central-v0.1.0) · [ninjaone-v0.1.0](https://github.com/servosity/msp-skills/releases/tag/ninjaone-v0.1.0) · [pax8-v0.1.0](https://github.com/servosity/msp-skills/releases/tag/pax8-v0.1.0) · [sentinelone-v0.1.0](https://github.com/servosity/msp-skills/releases/tag/sentinelone-v0.1.0) · [servosity-v0.2.0](https://github.com/servosity/msp-skills/releases/tag/servosity-v0.2.0) · [superops-v0.1.0](https://github.com/servosity/msp-skills/releases/tag/superops-v0.1.0) · [threatlocker-v0.1.0](https://github.com/servosity/msp-skills/releases/tag/threatlocker-v0.1.0)._
<!-- footer-releases:end -->
