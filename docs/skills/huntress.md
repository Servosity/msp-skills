---
layout: default
title: "Huntress MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Huntress endpoint, plus fleet-wide incident, coverage, and billing rollups the API can't."
permalink: /skills/huntress/
skill_name: "Huntress MCP"
image: /assets/social/huntress/wide-1200x630.png
verification: live-verified
faqs:
  - q: "Is there an MCP server for Huntress?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Huntress, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Huntress MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Huntress MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Huntress data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need to be a Huntress partner?"
    a: "You need a Huntress account with API credentials (a key and secret) generated from your portal. Reseller credentials unlock the cross-account reseller-rollup; a single-account credential drives everything else."
  - q: "Will this hit my Huntress API rate limits?"
    a: "Sync pulls your account into a local mirror once, then every rollup and search runs against that local copy - so repeated questions cost zero API calls. The CLI also honors a configurable --rate-limit on the requests it does make."
  - q: "Will this replace my Huntress portal?"
    a: "No - it complements it. The portal stays your console for configuration and deep investigation; this skill answers the cross-tenant and historical questions the portal can only show one organization at a time."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/huntress/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/huntress/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Huntress credentials once; huntress-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Huntress question in plain language; it runs huntress-cli for you."
---

# The Huntress MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Huntress Labs, Inc.

**✓ Live-verified by @Xenith-B (MSP)** against a production tenant · 2026-06-26 · [receipt →](https://github.com/Servosity/msp-skills/issues/144).

Yes - there is an MCP server for Huntress. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Huntress to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask "which Huntress incidents are oldest across all my clients?" and get one age-sorted queue spanning every organization - the cross-tenant view the per-org portal never shows. Triage incidents fleet-wide, find coverage gaps and dark agents, reconcile invoiced seats against deployed agents, and trace an indicator's blast radius across your whole account, in plain English, from one local mirror.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/huntress){: .btn}

## Instead of clicking through Huntress, just ask

**Instead of** Log into the Huntress portal and open each client organization one at a time to eyeball which incidents are oldest.
**just ask:** *"Show me every open Huntress incident across all my clients, oldest first."*
<sub>Your agent runs: <code>huntress-cli fleet-incidents --sort age</code></sub>

**Instead of** Export the agent list and the invoice, then reconcile seats against deployed agents in a spreadsheet.
**just ask:** *"Am I paying for more Huntress seats than I have agents deployed?"*
<sub>Your agent runs: <code>huntress-cli billing-reconcile</code></sub>

**Instead of** Search each org's agents by hand to find the machines that stopped checking in.
**just ask:** *"Which Huntress agents haven't called home in a week?"*
<sub>Your agent runs: <code>huntress-cli stale-agents --days 7</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/huntress/wide-1200x630.png" src="/assets/video/huntress/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/huntress/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which incidents are oldest across every client org? | `huntress-cli fleet-incidents --sort age` |
| Where are my posture gaps - stale callbacks, disabled Defender or firewall? | `huntress-cli coverage-gaps` |
| Has this IP or file hash touched any of my clients? | `huntress-cli blast-radius --indicator 203.0.113.10` |
| Am I billed for more seats than I have agents deployed? | `huntress-cli billing-reconcile` |
| Which agents went dark in the last week? | `huntress-cli stale-agents --days 7` |
| What is my mean time-to-resolve per client? | `huntress-cli mttr --group-by org` |
| What changed across the fleet since my last shift? | `huntress-cli handoff --since 12h` |
| Give me a QBR scorecard for one client. | `huntress-cli org-scorecard --org 12345` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/huntress/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/huntress/guide.md).

## What makes this one different

Most Huntress integrations proxy each question straight to the live API - one record, one organization at a time. This skill syncs your whole account into a local SQLite mirror, so cross-tenant questions (one incident queue for every client, fleet-wide blast radius, seat-vs-agent reconciliation) resolve as a single local join: instant, offline, and the AI sees the answer rather than raw bulk data.

Huntress exposes no public AI assistant over its API, and the portal scopes to one organization at a time. This skill adds the rollups the portal never pre-computes - a unified cross-org incident queue, posture gaps, billing reconciliation, and history (drift, MTTR, handoff) the point-in-time API throws away - without replacing the portal you already use.

## The pain this closes

- The Huntress portal scopes one organization at a time. Run dozens of client tenants and there is no single queue that tells you which incident - anywhere - is the oldest and most urgent right now.
- Invoiced seats and deployed agents drift apart silently; MSPs discover they are over- or under-billed only when someone reconciles by hand at month-end.
- During an incident you need to know whether an IP or file hash touched any other client, but the API answers one org and one entity at a time - never the fleet-wide correlation.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/huntress/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/huntress/install.ps1 | iex
```

After install, authenticate once with your Huntress credentials, then verify with `huntress-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet-incidents, coverage-gaps, blast-radius, billing-reconcile, mttr, org-scorecard, stale-agents, search | Allow |
| Write (routine) | organizations update-parameters, accounts memberships update-parameters, unwanted-access-rules update-parameters | Preview with --dry-run, then a reviewed write |
| Destructive / config | organizations delete-v1-id, accounts delete-v1-id, unwanted-access-rules delete-v1-id | Human-in-the-loop only |

The skill authenticates with your Huntress API key and secret and is read-first: every rollup, search, and report is non-mutating and safe to let an agent run. The handful of write commands (update an organization, membership, account, or unwanted-access rule) and the destructive deletes are opt-in and should sit behind an agent policy of preview-then-approve. Keep autonomous agents to reads plus dry-run previews; require a human for any write or delete. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/huntress/governance.md).

## Frequently asked questions

### Is there an MCP server for Huntress?

Yes - this one. A free, open source MCP server and Claude Code Skill for Huntress, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Huntress MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Huntress MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Huntress data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need to be a Huntress partner?

You need a Huntress account with API credentials (a key and secret) generated from your portal. Reseller credentials unlock the cross-account reseller-rollup; a single-account credential drives everything else.

### Will this hit my Huntress API rate limits?

Sync pulls your account into a local mirror once, then every rollup and search runs against that local copy - so repeated questions cost zero API calls. The CLI also honors a configurable --rate-limit on the requests it does make.

### Will this replace my Huntress portal?

No - it complements it. The portal stays your console for configuration and deep investigation; this skill answers the cross-tenant and historical questions the portal can only show one organization at a time.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Blumira](/skills/blumira/) · [CIPP](/skills/cipp/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [ThreatLocker](/skills/threatlocker/)

## Status

Beta. Validated against the Huntress API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
