---
layout: default
title: "SkyKick MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Fleet-wide M365 backup assurance for SkyKick Cloud Backup - posture, stale snapshots, and coverage gaps no portal or wrapper can show."
permalink: /skills/skykick/
skill_name: "SkyKick MCP"
image: /assets/social/skykick/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for SkyKick?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for SkyKick, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the SkyKick MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local SkyKick MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my SkyKick data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need to be a SkyKick partner, and will this hit my API rate limits?"
    a: "Yes - it authenticates with SkyKick Partner API client credentials (your API user ID and partner subscription key) from your SkyKick / ConnectWise Cloud Services partner account, and reads only what those credentials already permit. On rate limits: fleet-sync fans out per subscription with bounded concurrency and caches results in local SQLite, and SkyKick rate-limits the token endpoint aggressively, so the CLI mints and reuses cached tokens. Day-to-day questions run against the local store and never re-hit the API; you control --workers and --rate-limit."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/skykick/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/skykick/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your SkyKick credentials once; skykick-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a SkyKick question in plain language; it runs skykick-cli for you."
---

# The SkyKick MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by ConnectWise, LLC.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for SkyKick. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects SkyKick to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Run one fleet-sync, then ask your AI which SkyKick customers have a backup gap right now. It reads every subscription's Exchange/SharePoint posture, snapshot recency, coverage, and retention from a local copy and answers across all your tenants at once - the cross-customer view the per-tenant SkyKick portal never rolls up.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/skykick){: .btn}

## Instead of clicking through SkyKick, just ask

**Instead of** Log into the SkyKick partner portal and open each customer one at a time to eyeball whether Exchange and SharePoint backup are on and recent.
**just ask:** *"Which of my SkyKick customers has a backup gap right now?"*
<sub>Your agent runs: <code>skykick-cli fleet-health --flag-gaps --agent</code></sub>

**Instead of** Export per-tenant snapshot reports and scan them by hand for mailboxes that quietly stopped backing up.
**just ask:** *"Show me every mailbox that hasn't been snapshotted in the last two days."*
<sub>Your agent runs: <code>skykick-cli stale-snapshots --hours 48 --agent</code></sub>

**Instead of** Manually reconcile each tenant's discovered mailboxes and sites against what's actually enabled after onboarding a new client.
**just ask:** *"What's discovered but not protected across all my tenants?"*
<sub>Your agent runs: <code>skykick-cli coverage-gaps --type all --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/skykick/wide-1200x630.png" src="/assets/video/skykick/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/skykick/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which customers have a protection gap right now? | `skykick-cli fleet-health --flag-gaps --agent` |
| Whose mailboxes haven't been snapshotted in 48 hours? | `skykick-cli stale-snapshots --hours 48 --agent` |
| What's discovered but not actually being backed up? | `skykick-cli coverage-gaps --type all --agent` |
| Which tenants fall below our retention floor? | `skykick-cli retention-audit --floor-days 365 --agent` |
| Where is autodiscover off, so new mailboxes silently never enroll? | `skykick-cli autodiscover-audit --only-off --agent` |
| What protection changed since my last review? | `skykick-cli drift --agent` |
| What open alerts exist across the whole fleet, worst first? | `skykick-cli alert-sweep --agent` |
| How does backup posture roll up by partner? | `skykick-cli partner-rollup --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/skykick/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/skykick/guide.md).

## What makes this one different

Most SkyKick MCP wrappers proxy each question into a live API call, and the SkyKick API only serves data one subscription at a time - no fleet endpoint, no skip paging on alerts. Asking 'which of my 50 tenants has a gap' becomes 50+ sequential calls every single time. This skill runs fleet-sync once into a local SQLite store, then fleet-health, stale-snapshots, coverage-gaps, and drift are instant local queries the API cannot answer directly.

The SkyKick portal is per-customer and has no AI surface; this skill adds the cross-tenant posture, staleness, coverage, retention-compliance, and partner roll-up views the portal never aggregates, and hands them to whatever agent you already use. It complements the portal - restores and configuration still happen there.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](/skills/servosity/) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

## The pain this closes

- Microsoft 365 backup is set-and-forget until it silently fails - a mailbox stops snapshotting, a new hire never enrolls, retention drifts below your compliance floor - and the worst time to find out is when a customer asks for a restore.
- The SkyKick portal shows one customer at a time. With 30-50 tenants there is no single screen that answers 'who is not fully protected today', so backup verification quietly falls off the weekly routine.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/skykick/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/skykick/install.ps1 | iex
```

After install, authenticate once with your SkyKick credentials, then verify with `skykick-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet-health, stale-snapshots, coverage-gaps, retention-audit, autodiscover-audit, drift, partner-rollup, alert-sweep (without --apply), backup list / mailboxes / sites / storage-settings / subscription-settings, alerts list, identity | Allow |
| Write (routine) | alerts complete, alert-sweep --complete <ids> --apply, backup discover-mailboxes, backup discover-sites, import | Preview with --dry-run, then a reviewed write |
| Destructive / config | none - the wrapped SkyKick API surface has no delete, credential-rotation, or admin commands | Human-in-the-loop only |

The skill is read-first: every posture, staleness, coverage, retention, autodiscover, drift, alert, and partner view only reads. It can change a small, explicit set of things, all of which POST to the live SkyKick API: marking alerts complete (one at a time, or in bulk only with --apply), triggering Exchange mailbox and SharePoint site discovery, and bulk import from JSONL. There are no delete, credential, or admin commands. Keep an autonomous agent to read plus previewed writes, and require a human to approve the completion, discovery, and import commands. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/skykick/governance.md).

## Frequently asked questions

### Is there an MCP server for SkyKick?

Yes - this one. A free, open source MCP server and Claude Code Skill for SkyKick, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the SkyKick MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local SkyKick MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my SkyKick data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need to be a SkyKick partner, and will this hit my API rate limits?

Yes - it authenticates with SkyKick Partner API client credentials (your API user ID and partner subscription key) from your SkyKick / ConnectWise Cloud Services partner account, and reads only what those credentials already permit. On rate limits: fleet-sync fans out per subscription with bounded concurrency and caches results in local SQLite, and SkyKick rate-limits the token endpoint aggressively, so the CLI mints and reuses cached tokens. Day-to-day questions run against the local store and never re-hit the API; you control --workers and --rate-limit.


## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](/skills/acronis/) · [Afi](/skills/afi/) · [Axcient x360Recover](/skills/axcient/) · [Cove Data Protection](/skills/cove/) · [Datto BCDR](/skills/datto-bcdr/) · [Servosity](/skills/servosity/) · [Veeam](/skills/veeam/)

## Status

Beta. Validated against the SkyKick API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
