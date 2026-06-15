---
layout: default
title: "MCP Servers for MSPs - Connect ConnectWise, HaloPSA, HubSpot, Backup to Claude, ChatGPT, Copilot"
description: "Free MCP servers and Skills that connect ConnectWise PSA, HaloPSA, HubSpot, and Servosity backup to the AI you already use - Claude, ChatGPT, Copilot, Codex. Local SQLite mirror, no data leaves your network. Built for MSP owners. No code required."
permalink: /
body_class: wide
---

# Run your MSP tools by asking

MCP servers and Skills that connect your PSA, RMM, backup, and more to the AI you already use.

Ask your AI a plain-English question about your stack and get a real answer back - no exports, no dashboards, no developer experience required. Every connector is **free, open source, and runs on your own machine**: it keeps a local SQLite copy of your tool's data so cross-client questions the live API can't answer in one shot come back instantly, and **no data leaves your network**. {{ site.data.catalog.count }} connectors are live today - {% for c in site.data.catalog.connectors %}{% if forloop.last and forloop.length > 1 %}and {% endif %}{{ c.display_name }}{% unless forloop.last %}, {% endunless %}{% endfor %} - and more PSA, RMM, backup, and M365 connectors ship every week.

{% include instead-of.html
   instead="exporting three reports and pivoting them in Excel"
   say="Which clients had backup-failure tickets last quarter?"
   cli="halopsa-cli tickets search --tag backup-failure --since last-quarter" %}

> 🛠 **Built live with MSPs.** Join a free weekly **Build Session** at **[compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions)** - watch a connector built against a real MSP system, or bring your own.

[Why msp-skills? →](/why/){: .btn .btn-primary} &nbsp; [Install in 60 seconds →](#install-in-60-seconds){: .btn} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills){: .btn}

{% include vocab-bridge.html %}

<video autoplay muted loop playsinline preload="metadata" style="width:100%; max-width:960px; border-radius:12px; display:block; margin: 18px 0;" poster="/assets/social/og-1280x640.png" src="/assets/video/hero-14s.mp4"></video>


## Why MSP owners use this

<div class="why-cards">
  <div class="why-card">
    <h3>Answers your vendor's API can't</h3>
    <p>A private, searchable copy of your data lives on your machine, so a 90-day, 10-client question is one instant local lookup - not hundreds of rate-limited API calls.</p>
  </div>
  <div class="why-card">
    <h3>One quality bar, every connector</h3>
    <p>Each one passes mechanical verification before it ships. The badge tells you whether a real MSP has confirmed it against a live tenant yet.</p>
  </div>
  <div class="why-card">
    <h3>Free, local, yours</h3>
    <p>Open source, Apache-2.0, runs on your own hardware. No per-tech fee, no SaaS lock-in, nothing to rip out.</p>
  </div>
</div>

[Read the full why → ](/why/)

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | Why MSPs use it | Install |
| --- | --- | --- |
| **Claude Desktop** | The most common MSP-owner choice - no terminal, just a chat window | [Setup →](/integrations/claude-desktop/) |
| **ChatGPT** (Plus/Pro+) | The brand most MSPs already pay for; pair with Developer Mode | [Setup →](/integrations/chatgpt/) |
| **Codex CLI** (OpenAI) | Same audience as Claude Code, OpenAI side | [Setup →](/integrations/codex/) |
| **Claude Code** (CLI) | For technical-leaning MSP owners or senior techs | [Setup →](/integrations/claude-code/) |
| **Claude Cowork** | Anthropic's desktop agent - paste a prompt, it installs itself | [Setup →](/integrations/cowork/) |
| **GitHub Copilot** (VS Code) | The Microsoft surface that runs the local binary today, in Agent mode | [Setup →](/integrations/github-copilot/) |

**Microsoft & Google paths** - big install base, but the honest remote / enterprise route:

| Your AI agent | What it takes | Install |
| --- | --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | Remote only: host over HTTPS + Copilot Studio license + tenant admin | [Setup →](/integrations/microsoft-365-copilot/) |
| **Google Gemini** | Gemini CLI: local. Gemini app: remote (HTTPS). | [Setup →](/integrations/gemini/) |

> **Skill-native agents (secondary):** [Hermes →](/integrations/hermes/) and [OpenClaw →](/integrations/openclaw/) read the skill directly and speak MCP. **Also works with** Cursor, Windsurf, Cline, Continue.dev, and Zed. Full per-tool deep-dive in [which agent →](/which-agent/).

## What's in the box

{% include skill-table.html %}

New to the term? [What is an MCP server? →](/what-is-an-mcp-server/)

## Let your AI pick for you

Already have msp-skills installed? Let the concierge recommend the right connectors for your stack and install the ones you approve. Paste this into Claude Code or Codex:

> You have msp-skills installed. Using everything you know about me and how I work, recommend which connectors I should install - and install the ones I approve.

More in the [concierge README →](https://github.com/servosity/msp-skills/tree/main/skills/_meta).

## What makes this different

### Local mirror, not live calls

Most MCP servers for PSA and backup systems proxy each question into a live API call. That's fine for one ticket. It dies at QBR time, when you're asking *"how many backup-failure tickets across all 47 clients last quarter?"* - that's 47 paginated REST calls, rate-limit headaches, and a context window full of raw JSON the model has to re-read. These connectors sync your tool into a local SQLite mirror with full-text search. Cross-client and cross-engine questions become one local query: instant, offline, and the AI sees the answer, not the raw data.

### Works with the AI you already use

You don't have to switch agents. Same package, two interfaces: a **Skill** for Claude Code / Codex, and an **MCP server** for Claude Desktop, ChatGPT, Cursor, Windsurf, Cline, Continue.dev, Gemini, and Copilot. One install drops both binaries on your machine. Use one or both - your call. No vendor lock-in, no proprietary plugin format, no SaaS subscription that ties you to a single AI.

### MSP owner first, not developer first

You don't have to know JSON, regex, or what "stdio transport" means. Paste one sentence into Claude Code or Codex and your agent reads the Skill, installs the binary, and walks you through authentication. Every command has a tier (read / write / destructive) and a recommended agent policy in each skill's `governance.md` - reads are always safe, and the recommended agent rule is to preview writes with `--dry-run` and require your approval before any mutation. The hard stuff is hidden; the safety bar is high.

## Install in 60 seconds

{% include install-examples.html %}

> **Now what?** Once your connector's `--version` (or `doctor`) returns clean, **bring your tenant + your hardest cross-client question to a free [Build Session](https://compoundingteams.com/build-sessions)** - we'll work it live with the MSP cohort and the same connectors you just installed.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local binaries directly. These connectors ship local binaries, so to use them with ChatGPT you run them on your machine and expose them via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step: [ChatGPT integration →](/integrations/chatgpt/).

### Does this work with Claude?

Yes - **Claude Code**, **Claude Desktop**, and the **Claude.ai** web (via Claude Desktop's MCP). Claude Code reads the Skill directly; Claude Desktop loads each connector as an MCP server you add to `claude_desktop_config.json`. Both paths are first-class. Most MSP owners start with Claude Desktop because it's a Mac/Windows app - no terminal.

### Do I need to know how to code?

No. The recommended install path is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code, editing JSON in a text editor, or installing Node, Python, Docker, or a database. You will need to enter API credentials your PSA or backup tool gives you.

### Is my PSA, CRM, or backup data safe? Does it leave my network?

Your data stays on **your machine**. Each connector runs locally: the CLI and the MCP server are binaries on your laptop. The SQLite mirror sits in a local directory under your user account. The AI agent (Claude, ChatGPT, Codex) only sees what the CLI or MCP server returns - typically the result of a query, not raw bulk data. Credentials are read from your environment or your agent's config; they're never bundled into the repo or transmitted to our servers (there are none).

### Will this hit my vendor's API rate limits?

Almost never. Each connector syncs your data into a local SQLite mirror once, then incrementally; subsequent questions (triage, SLA breaches, client cards, cross-client analytics) run against the local mirror, not the live API. The big-batch QBR queries that get you 429'd with API-passthrough tools become a single SQL join here.

### How is this different from my vendor's built-in AI?

Vendor-native AI is great for ticket-by-ticket work - rewriting one reply, summarizing one ticket, classifying sentiment. These connectors are the **cross-client, cross-system layer**: analytics across thousands of tickets, ad-hoc questions, multi-system queries that join your PSA with your backup tool. The two complement each other; nothing gets ripped out. More in [why msp-skills →](/why/).

### Can I run this on Windows?

Yes. The PowerShell installer in "Install in 60 seconds" above is the same install path as macOS / Linux, just packaged for PowerShell. The CLI and MCP binaries are native Windows builds.

### What does it cost?

The Skills, the CLI binaries, and the MCP servers are **free**. Apache-2.0 licensed - free to use commercially, free to fork. Servosity does not charge for API access required to run the Servosity connector. Other PSA, RMM, and backup vendors set their own API-access terms. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and those are billed by your AI provider, not by us.

### Can I try it on one client first?

Yes. Every command takes a client / company filter (e.g. `halopsa-cli client card "Acme Corp"`, `servosity-cli company show 4421`). Read-tier commands change nothing; you can try them against your live system safely. For writes, each skill's `governance.md` tags every command read / write / destructive with a recommended agent policy - the safe agent rule is to preview with `--dry-run` and require your approval before any mutation. The safe starting move is a read-only triage against one client, see the output, then widen scope.

### What if my AI doesn't speak MCP?

If your AI doesn't yet support MCP and isn't Claude Code / Codex CLI (which read SKILL.md directly), these connectors won't fit yet. Every connector's CLI binary still works standalone in a shell - you can pipe its output into anything. As more AI tools add MCP (the protocol is moving fast in 2026), they'll work automatically without anything changing on our side.

## Built and verified in Build Sessions

Every connector passes mechanical verification before it ships, and we build the next ones live with real MSPs running them against their own production systems in our free weekly Build Sessions. **[RSVP for the next one →](https://compoundingteams.com/build-sessions)**.

## Don't see the system you need?

Tell us what to build next. **[Open an issue →](https://github.com/servosity/msp-skills/issues/new?template=skill-request.yml)** and name the PSA, RMM, backup, security, or M365 tool. First time filing a GitHub issue? See [the 90-second walkthrough →](/requesting-a-skill/).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Built by [Servosity](https://www.servosity.com). Maintained by Damien Stevens. Apache-2.0 licensed. Methodology: [Compounding Teams](https://compoundingteams.com).
