---
layout: default
title: "ThreatLocker MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Every ThreatLocker Portal API feature, plus the write operations the read-only tools lack and a cross-tenant offline store no other ThreatLocker tool has."
permalink: /skills/threatlocker/
skill_name: "ThreatLocker MCP"
image: /assets/social/threatlocker/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local ThreatLocker MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my ThreatLocker data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my ThreatLocker API rate limits?"
    a: "The local mirror exists so reads stop hitting the API. After the first sync, the cross-tenant views (approvals triage, audit drift, devices health, applications hunt) run against local SQLite with zero API calls. Live calls respect a --rate-limit throttle, and sync is incremental, fetching only what changed since the last checkpoint."
  - q: "How does it handle ThreatLocker's 31-day audit retention?"
    a: "ThreatLocker's Unified Audit log keeps about 31 days by default. audit export persists each tenant's log to JSONL or CSV locally so evidence outlives that window, and audit retention-check reports, per tenant, how close your archive is to the cliff and how stale your last sync is, so nothing ages off unnoticed."
  - q: "Do I need to be a ThreatLocker MSP or have child tenants?"
    a: "You need API access in your own ThreatLocker Portal. The cross-tenant features assume a managed (parent) organization with child tenants, which is the MSP setup; a single organization works too, you just get the one-tenant view. The credential you mint is the real permission boundary."
  - q: "Does it replace the ThreatLocker Portal?"
    a: "No. The Portal stays best for authoring policies and the interactive approve/deny workflow. This skill adds cross-tenant queries and scriptable writes to your AI agent so you stop logging into each tenant to answer book-wide questions."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatlocker/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your ThreatLocker credentials once; threatlocker-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a ThreatLocker question in plain language; it runs threatlocker-cli for you."
---

# ThreatLocker + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the ThreatLocker
> API. Not affiliated with, endorsed by, or sponsored by ThreatLocker, Inc..

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

Running ThreatLocker across a whole book of customer tenants? Ask your AI "what approvals are pending everywhere," "which agents went dark this week," or "which clients are about to lose audit evidence," and get one cross-tenant answer the Portal can't compose. Every tenant is mirrored into a local store, so one approval queue, one audit archive, and one health rollup replace dozens of one-tenant-at-a-time Portal logins.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/threatlocker){: .btn}

## Instead of clicking through ThreatLocker, just ask

**Instead of** Logging into each customer's ThreatLocker Portal in turn and clearing the application-approval queue tenant by tenant, re-reviewing the same blocked file every time it shows up in a different org
**just ask:** *"Show me every pending approval across all my clients, grouped so duplicate files collapse into one row"*
<sub>Your agent runs: <code>threatlocker-cli approvals triage --all-tenants</code></sub>

**Instead of** Approving the same known-good file (a Chrome update, a line-of-business installer) by hand in every tenant where it got blocked, one Portal session at a time
**just ask:** *"Approve this file everywhere it's pending, but show me the plan before you do"*
<sub>Your agent runs: <code>threatlocker-cli approvals approve-batch --hash <sha256> --all-tenants --dry-run</code></sub>

**Instead of** Remembering to export each tenant's Unified Audit log to CSV every month before it ages past the 31-day retention window, for the cyber-insurance and compliance evidence you can't regenerate later
**just ask:** *"Which clients are about to lose audit evidence, and pull it before they do"*
<sub>Your agent runs: <code>threatlocker-cli audit retention-check</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/threatlocker/wide-1200x630.png" src="/assets/video/threatlocker/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/threatlocker/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What application approvals are pending across all my clients right now? | `threatlocker-cli approvals triage --all-tenants` |
| Approve this file hash everywhere it's pending, but show me the plan first? | `threatlocker-cli approvals approve-batch --hash <sha256> --all-tenants --dry-run` |
| Which clients are about to lose audit evidence to the 31-day retention cliff? | `threatlocker-cli audit retention-check` |
| Export every client's audit log before it ages off? | `threatlocker-cli audit export --all-tenants --since 30d` |
| What security-relevant changes (protection off, policy edits, maintenance) happened across all tenants this week? | `threatlocker-cli audit drift --since 7d --all-tenants` |
| Which ThreatLocker agents are offline or stale across every client? | `threatlocker-cli devices health --all-tenants` |
| Where does this binary live across my whole book, approved or pending? | `threatlocker-cli applications hunt --hash <sha256>` |
| Pull every tenant's ThreatLocker data into a local mirror for offline queries? | `threatlocker-cli sync` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/threatlocker/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/threatlocker/guide.md).

## What makes this one different

The ThreatLocker Portal API is scoped one tenant at a time: a managed-organization header selects which customer you're acting on, so a live API wrapper answering a book-wide question has to swap that header and re-query tenant by tenant, burning agent context on each round trip. This skill syncs every tenant into a local SQLite mirror, so cross-tenant questions (one approval queue deduped by file hash, a file hunt across all endpoints, a drift table, a health rollup) become one offline query the agent reads as an answer, not pages of raw JSON.

It complements the ThreatLocker Portal rather than replacing it: the Portal stays best for authoring policies and the deny/permit workflow inside one tenant, while this skill brings the whole book to whichever AI agent you already use and answers the cross-tenant questions, one approval queue and one audit archive across every customer, that no single Portal screen composes.

## The pain this closes

- ThreatLocker is default-deny: every new or updated application a user runs creates an approval request an admin has to clear. Run it across a book of customer tenants and the requests pile up faster than anyone can triage them, and the Portal makes you work one tenant at a time, switching the managed-organization context for every single one.
- The Unified Audit log keeps roughly 31 days by default. Cyber-insurance questionnaires and compliance audits routinely ask for longer, so the evidence you need is the evidence that just aged off, unless someone remembered to export each tenant's log before the cliff.
- Answering "where does this binary live across all my clients" or "which agents went dark this week" means opening each tenant's Portal in turn. There is no single view that joins approvals, device health, and audit events across the whole book at once.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](/integrations/claude-desktop/) |
| **ChatGPT** (Plus/Pro+) | [Step-by-step →](/integrations/chatgpt/) |
| **Claude Code** | [Step-by-step →](/integrations/claude-code/) |
| **Codex CLI** | [Step-by-step →](/integrations/codex/) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](/which-agent/) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/threatlocker/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/threatlocker/install.ps1 | iex
```

After install, authenticate once with your ThreatLocker credentials, then verify with `threatlocker-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | threatlocker-cli approvals triage --all-tenants; threatlocker-cli audit drift --since 7d --all-tenants; threatlocker-cli audit retention-check; threatlocker-cli audit export --all-tenants --since 30d; threatlocker-cli devices health --all-tenants; threatlocker-cli applications hunt --hash <sha256>; threatlocker-cli search | Allow |
| Write (routine) | threatlocker-cli approvals approve (permit a file); threatlocker-cli approvals approve-batch (permit a file across tenants); threatlocker-cli applications create / applications update; threatlocker-cli policies create / policies copy / policies deploy; threatlocker-cli computers maintenance / computers enable-protection / computers restart-service - writes send immediately; --dry-run is an opt-in preview, not a default | Preview with --dry-run, then a reviewed write |
| Destructive / config | threatlocker-cli computers delete; threatlocker-cli policies delete | Human-in-the-loop only |

The skill drives the threatlocker-cli and threatlocker-mcp binaries, authenticating with a THREATLOCKER_API_KEY read from the environment, never logged and never sent anywhere except the ThreatLocker API. Read commands (approvals triage, audit drift, audit export, audit retention-check, devices health, applications hunt, search) change nothing. Writes are not gated by default: --dry-run is an opt-in preview flag, so the recommended policy is an agent-level rule, preview with --dry-run, show the exact command, get approval, then run the write. Keep computers delete and policies delete human-only. The strongest control is the scope of the API key you mint in the Portal. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/threatlocker/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local ThreatLocker MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my ThreatLocker data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my ThreatLocker API rate limits?

The local mirror exists so reads stop hitting the API. After the first sync, the cross-tenant views (approvals triage, audit drift, devices health, applications hunt) run against local SQLite with zero API calls. Live calls respect a --rate-limit throttle, and sync is incremental, fetching only what changed since the last checkpoint.

### How does it handle ThreatLocker's 31-day audit retention?

ThreatLocker's Unified Audit log keeps about 31 days by default. audit export persists each tenant's log to JSONL or CSV locally so evidence outlives that window, and audit retention-check reports, per tenant, how close your archive is to the cliff and how stale your last sync is, so nothing ages off unnoticed.

### Do I need to be a ThreatLocker MSP or have child tenants?

You need API access in your own ThreatLocker Portal. The cross-tenant features assume a managed (parent) organization with child tenants, which is the MSP setup; a single organization works too, you just get the one-tenant view. The credential you mint is the real permission boundary.

### Does it replace the ThreatLocker Portal?

No. The Portal stays best for authoring policies and the interactive approve/deny workflow. This skill adds cross-tenant queries and scriptable writes to your AI agent so you stop logging into each tenant to answer book-wide questions.


## Status

Beta. Validated against the ThreatLocker API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
