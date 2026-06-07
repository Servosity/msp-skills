---
layout: default
title: "Proofpoint TAP MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Every Proofpoint TAP Threat Insight endpoint plus a local threat store that answers cross-endpoint questions - who is both attacked and clicking, what touched a given user - without re-spending the limited daily API quota."
permalink: /skills/proofpoint/
skill_name: "Proofpoint TAP MCP"
image: /assets/social/proofpoint/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Proofpoint MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Proofpoint data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local, and the bundled MCP server exposes read-only threat-intelligence tools only. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this blow through my Proofpoint TAP API limits?"
    a: "No - avoiding that is the point. TAP caps you at 1,800 SIEM requests and 50 campaign-id lookups per rolling 24 hours. The skill backfills once into a local SQLite store, then answers repeat and cross-endpoint questions from that mirror, so re-querying a window or looping over users costs zero additional API calls. Live calls fire only when you ask for fresh data."
  - q: "Do I need a special Proofpoint partner API or Essentials admin access?"
    a: "No. It uses your standard TAP (Targeted Attack Protection) Service Principal and Secret, created under Settings then Connected Applications in the TAP dashboard. It reads the Threat Insight endpoints your account already exposes; it does not require Proofpoint Essentials administration or a separate partner program."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proofpoint/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/proofpoint/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Proofpoint TAP credentials once; proofpoint-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Proofpoint TAP question in plain language; it runs proofpoint-cli for you."
---

# Proofpoint TAP + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the Proofpoint
> API. Not affiliated with, endorsed by, or sponsored by Proofpoint, Inc.

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

Proofpoint TAP's dashboard answers one threat, one clicker, one campaign at a time, and every SIEM pull spends against a hard daily quota. This skill backfills clicks, messages, campaigns, Very Attacked People, and clickers into a local SQLite store, then answers the questions the console can't: who is both heavily attacked and clicking, every event that touched one user, and a full incident brief from a single threatId - offline, in seconds.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/proofpoint){: .btn}

## Instead of clicking through Proofpoint TAP, just ask

**Instead of** Pulling SIEM exports hour by hour to reconstruct what got through overnight, then re-querying the same window every time you need it again
**just ask:** *"What malicious clicks and messages got through in the last 12 hours?"*
<sub>Your agent runs: <code>proofpoint-cli backfill --since 12h --agent</code></sub>

**Instead of** Cross-referencing the Very Attacked People report against the top-clickers report in two separate dashboard exports to find your real problem people
**just ask:** *"Who is both Very Attacked and a top clicker right now?"*
<sub>Your agent runs: <code>proofpoint-cli risk-overlap --window 30 --agent</code></sub>

**Instead of** Opening three TAP screens - threat summary, forensics, and the message events - just to brief a single alert
**just ask:** *"Give me the full incident brief for this threatId"*
<sub>Your agent runs: <code>proofpoint-cli incident "threat-abc123" --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/proofpoint/wide-1200x630.png" src="/assets/video/proofpoint/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/proofpoint/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What malicious clicks and messages got through overnight? | `proofpoint-cli backfill --since 12h` |
| Who is both Very Attacked and a top clicker? | `proofpoint-cli risk-overlap --window 30` |
| Give me the full incident brief for a threatId | `proofpoint-cli incident "threat-abc123"` |
| What indicators should I block from this threat? | `proofpoint-cli iocs --threat-id "threat-abc123" --csv` |
| Show me every event that touched one user | `proofpoint-cli user "jane.doe@example.com"` |
| Who are my Very Attacked People this month? | `proofpoint-cli people list-vap --window 30` |
| Which permitted clicks and delivered threats still need a response? | `proofpoint-cli siem list-issues` |
| What threats are inside this campaign? | `proofpoint-cli campaign-threats "campaign-xyz789"` |
| Decode this urldefense-rewritten link to its real target | `proofpoint-cli url --urls "https://urldefense.com/v3/__https://example.com__;!!abc"` |
| Is my synced threat data fresh enough to trust an offline query? | `proofpoint-cli workflow status` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/proofpoint/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/proofpoint/guide.md).

## What makes this one different

Most Proofpoint TAP integrations and MCP servers proxy each question into a single live API call - fine for one lookup, but every call spends against TAP's hard daily quota and none of them join across endpoints. This skill backfills SIEM events, campaigns, Very Attacked People, and clickers into a local SQLite store, so repeat investigations, cross-endpoint joins like attacked-and-clicking, and per-user timelines become instant offline queries instead of a wall of rate-limited API pulls.

Proofpoint's TAP dashboard gives you per-threat and per-person screens; this skill adds the cross-endpoint rollups the portal never exposes - the people who are both Very Attacked and clicking, every local event that touched one user, and a one-shot incident brief from a single threatId - all from your own synced data and pointed at by your AI agent.

## The pain this closes

- MSPs running Proofpoint TAP hit the API's daily ceilings fast - 1,800 SIEM requests and only 50 campaign-id lookups per rolling 24 hours - so any investigation that re-queries the same window, or loops over yesterday's clicks, runs out of quota before it runs out of questions.
- The TAP dashboard reports one threat, one campaign, one Very Attacked Person at a time. The questions that matter at response time - who is both attacked and clicking, every event that touched this user, what is actually inside this campaign - cross endpoints the console never joins, so they become manual export-and-pivot work.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/proofpoint/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/proofpoint/install.ps1 | iex
```

After install, authenticate once with your Proofpoint TAP credentials, then verify with `proofpoint-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | siem list-issues, siem list-clicks-permitted, people list-vap, people list-top-clickers, campaign get, campaign-threats, threat, incident, iocs, forensics, risk-overlap, user, url, search, export, sync, workflow status | Allow |
| Write (routine) | import | Preview with --dry-run, then a reviewed write |
| Credential / security | auth set-token, auth logout | Human-in-the-loop only |

The skill reads your Proofpoint TAP threat data - SIEM click and message events, campaigns, Very Attacked People, top clickers, and forensic evidence - and can sync it into a local SQLite mirror; all of that is read-only and safe to let an agent run, and the bundled MCP server exposes only those read tools. The one API write path is CLI-only bulk import, which supports --dry-run - preview it and keep it human-reviewed. The local auth commands (auth set-token, auth logout) manage your stored TAP credentials and should stay operator-only. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/proofpoint/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Proofpoint MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Proofpoint data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local, and the bundled MCP server exposes read-only threat-intelligence tools only. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this blow through my Proofpoint TAP API limits?

No - avoiding that is the point. TAP caps you at 1,800 SIEM requests and 50 campaign-id lookups per rolling 24 hours. The skill backfills once into a local SQLite store, then answers repeat and cross-endpoint questions from that mirror, so re-querying a window or looping over users costs zero additional API calls. Live calls fire only when you ask for fresh data.

### Do I need a special Proofpoint partner API or Essentials admin access?

No. It uses your standard TAP (Targeted Attack Protection) Service Principal and Secret, created under Settings then Connected Applications in the TAP dashboard. It reads the Threat Insight endpoints your account already exposes; it does not require Proofpoint Essentials administration or a separate partner program.


## Status

Beta. Validated against the Proofpoint TAP API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
