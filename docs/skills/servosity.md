---
layout: default
title: "Servosity MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Add fleet-wide backup triage, stale-backup detection, and cross-engine analytics to Claude, ChatGPT, Codex, or any AI that speaks MCP. Local fleet mirror with snapshot history so the partner portal's per-page views become one query."
permalink: /skills/servosity/
skill_name: "Servosity MCP"
image: /assets/social/servosity/wide-1200x630.png
verification: live-verified
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on Plus, Pro, Team, Business, Enterprise, and Education plans (the Free tier does not yet expose Developer Mode). ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Servosity MCP server via the mcp-remote bridge or your own HTTPS endpoint. Step-by-step in the install guide."
  - q: "Do I need to be a Servosity MSP partner?"
    a: "Yes. The Servosity API surface this skill wraps is the one available to authenticated MSP partners, so you'll need a partner API token from the Servosity partner portal. The skill itself is free; the token is part of your Servosity partner agreement. Not a partner yet? Reach out via servosity.com."
  - q: "Will this replace my Servosity partner portal?"
    a: "No. The portal is still where you do administrative work, configure backups, and onboard clients. This skill makes the portal's data answerable by AI, so morning triage, Friday stale-backup reports, drift checks, and ad-hoc cross-client questions don't require portal archaeology. The portal is unchanged; you're adding a new way to ask the data questions."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent reads SKILL.md and does the install, or run a one-line installer per OS. You enter your Servosity partner token once."
  - q: "Is my Servosity and client data safe?"
    a: "Your data stays on your machine. The CLI and MCP server are local binaries, and the fleet mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. The partner token is read from your environment or your agent's config, never bundled into this repo or transmitted anywhere by MSP Skills, and it is scoped to your reseller account only."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. Servosity does not charge for the API access required to run this skill - the partner-portal API is part of your existing partner agreement. You pay only for whichever AI agent you use, billed by your AI provider."
  - q: "Can I run this on Windows?"
    a: "Yes. There's a PowerShell installer for the Windows path, and the CLI and MCP binaries are native Windows builds."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Servosity credentials once; servosity-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Servosity question in plain language; it runs servosity-cli for you."
---

# Servosity + AI in 60 seconds

> Published by Servosity Inc. for MSP partners. Servosity is a trademark of
> Servosity Inc.. Apache-2.0 licensed.

**Live-verified** - confirmed by a real MSP against a live Servosity tenant (2026-06-05).

MSPs run Servosity as the backup and DR platform - M365, DR Server, and DR Desktop protection across a whole book of clients. Ask your AI "what needs my attention this morning," "which backups went stale," or "what changed overnight," and get an answer the partner portal can't show on one screen: cross-engine, cross-client rollups computed against a local fleet mirror in one query instead of clicking through per-client pages, three backup engine views, and an alert queue full of noise.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/servosity){: .btn}

## Instead of clicking through Servosity, just ask

**Instead of** Clicking through every client's portal view to find the backup sets that quietly stopped succeeding before a client needs a restore
**just ask:** *"Which backups went stale across all my clients in the last week?"*
<sub>Your agent runs: <code>servosity-cli stale-backups --days 7</code></sub>

**Instead of** Opening Monday with a blank slate and no idea which clients got worse or recovered over the weekend
**just ask:** *"What changed across my fleet overnight - what got worse, what recovered?"*
<sub>Your agent runs: <code>servosity-cli drift --from yesterday --to now</code></sub>

**Instead of** Doing portal archaeology across metadata, three backup engines, contracts, and issues to answer "is this client OK?"
**just ask:** *"Give me the whole story for this client on one screen."*
<sub>Your agent runs: <code>servosity-cli company show 4421</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/servosity/wide-1200x630.png" src="/assets/video/servosity/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/servosity/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What needs my attention across every client this morning? | `servosity-cli attention` |
| Which backups went stale across all clients in the last week? | `servosity-cli stale-backups --days 7` |
| What changed across my fleet overnight - what got worse and what recovered? | `servosity-cli drift --from yesterday --to now` |
| What's the whole story for this client, on one screen? | `servosity-cli company show 4421` |
| Where do I find that client, issue, or backup when I remember a phrase but not which one? | `servosity-cli find "image manager"` |
| Which backup engine is failing where for a client running multiple engines? | `servosity-cli backup-facts --company 4421 --status fail` |
| What's worth keeping in the alert queue, and what known-safe noise can I clear? | `servosity-cli triage --company 4421` |
| Which DR restores are in flight across my whole book right now? | `servosity-cli restore-queue list` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/servosity/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/servosity/guide.md).

## What makes this one different

Most AI integrations for backup and DR vendors proxy each question into a live API call against the partner portal. That's fine for one client. It fails when you're asking "which of my 47 clients have a backup set that hasn't succeeded in 7 days, sliced by engine?" - the portal scatters that across per-client pages, three engine views, and an alert queue full of noise. This skill syncs Servosity into a local fleet mirror with snapshot history and full-text search, so cross-engine, cross-client questions become one local query: instant, offline, and the AI sees the answer, not a context window full of paginated JSON. Compound commands like attention, drift, stale-backups, and company show join across companies, backup engines, issues, contracts, and DR events - work a stateless API wrapper can't do.

Servosity publishes this skill itself, for MSP partners - it complements the partner portal rather than replacing it. The portal stays where you do administrative work, configure backups, and onboard clients; this skill makes the portal's data answerable by AI, so morning triage, Friday stale-backup reports, drift checks, and ad-hoc cross-client questions don't require portal archaeology. The token is scoped to your reseller account only - no cross-reseller access.

## The pain this closes

- Silent backup failures discovered too late: a backup that quietly stopped succeeding is invisible until a client needs a restore - the worst possible moment to find out.
- No fleet-wide view: each client's backup state lives in its own portal view; there is no single screen that says "across my whole book, here is what is stale, failing, or in-flight right now."
- Alert-queue noise buries the real failure: dozens of repeat and known-safe issues pile up per client, and the one that matters hides in the pile.
- Per-client questions mean portal archaeology: answering "is this client OK?" means clicking through metadata, three backup engines, contracts, and issues by hand.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

After install, authenticate once with your Servosity credentials, then verify with `servosity-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | servosity-cli attention; servosity-cli drift --from yesterday --to now; servosity-cli stale-backups --days 7; servosity-cli backup-facts --company 4421 --status fail; servosity-cli find "image manager"; servosity-cli company show 4421; servosity-cli restore-queue list | Allow - these only read and cannot modify anything. |
| Write (routine) | servosity-cli triage --company 4421 (plans by default; pass --confirm to mutate); servosity-cli clear "ACME Corp" --until "6am tomorrow" (defaults to --dry-run); servosity-cli stale-issues --mine (defaults to --dry-run); company-notes and issue-comments (interactive prompt by default, skippable with --yes/--agent) | Compound commands plan first - review the PLAN, then pass --confirm. Raw CRUD: preview with --dry-run, get agent-side approval, never blanket --yes. |
| Destructive / config | servosity-cli companies delete; servosity-cli backups delete; servosity-cli backup-sets delete; servosity-cli dr-backups delete; servosity-cli users delete; credentials rotate/delete, MFA, agent-install-token, and encryption-key update (Credential / security) | Human-in-the-loop only, with an explicit out-of-band confirmation step. Never allow an autonomous agent to run these unattended. |

The skill drives the servosity-cli and servosity-mcp binaries, authenticating with a single SERVOSITY_MSP_TOKEN you generate in the partner portal - read only from the environment, never written to disk, logged, or sent anywhere except the Servosity API. Every call is scoped to your reseller account; there is no cross-reseller access. Read commands (attention, drift, stale-backups, backup-facts, find, company show, restore-queue list) are always safe and change nothing. Writes split into two classes with different gating. The compound write commands plan by default: triage runs in PLAN mode unless you pass --confirm, and clear and stale-issues default to --dry-run - a plan prints what would change and exits without mutating, and the global --dry-run flag keeps PLAN mode even if --confirm is passed. Raw CRUD writes (notes, comments) use an interactive prompt by default, but --yes (and --agent, which implies --yes) skips that prompt and --dry-run is opt-in, so gate those at your agent's policy: preview with --dry-run, show the exact command, get approval, then run. The recommended policy is to keep autonomous agents to Read plus planned writes and require a human for any Credential, Destructive, or Admin command. The strongest control is scoping the partner token to only the access your workflow needs. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/servosity/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on Plus, Pro, Team, Business, Enterprise, and Education plans (the Free tier does not yet expose Developer Mode). ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Servosity MCP server via the mcp-remote bridge or your own HTTPS endpoint. Step-by-step in the install guide.

### Do I need to be a Servosity MSP partner?

Yes. The Servosity API surface this skill wraps is the one available to authenticated MSP partners, so you'll need a partner API token from the Servosity partner portal. The skill itself is free; the token is part of your Servosity partner agreement. Not a partner yet? Reach out via servosity.com.

### Will this replace my Servosity partner portal?

No. The portal is still where you do administrative work, configure backups, and onboard clients. This skill makes the portal's data answerable by AI, so morning triage, Friday stale-backup reports, drift checks, and ad-hoc cross-client questions don't require portal archaeology. The portal is unchanged; you're adding a new way to ask the data questions.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent reads SKILL.md and does the install, or run a one-line installer per OS. You enter your Servosity partner token once.

### Is my Servosity and client data safe?

Your data stays on your machine. The CLI and MCP server are local binaries, and the fleet mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. The partner token is read from your environment or your agent's config, never bundled into this repo or transmitted anywhere by MSP Skills, and it is scoped to your reseller account only.

### What does it cost?

Free. Apache-2.0 licensed. Servosity does not charge for the API access required to run this skill - the partner-portal API is part of your existing partner agreement. You pay only for whichever AI agent you use, billed by your AI provider.

### Can I run this on Windows?

Yes. There's a PowerShell installer for the Windows path, and the CLI and MCP binaries are native Windows builds.


## Status

Beta. Validated against the Servosity API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
