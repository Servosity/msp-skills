---
layout: default
title: "PagerDuty MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Every PagerDuty incident, on-call and service operation from the terminal, plus a local SQLite mirror that answers cross-entity questions \u2014 MTTA/MTTR, on-call coverage gaps, responder load \u2014 that neither the API nor the web UI can."
permalink: /skills/pagerduty/
skill_name: "PagerDuty MCP"
image: /assets/social/pagerduty/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local PagerDuty MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my PagerDuty data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my PagerDuty API rate limits?"
    a: "Rarely. Read questions run against the local SQLite mirror, not the API - you sync once, then analytics, audits, and search are offline. Only sync and live writes call PagerDuty, and the CLI honors a configurable --rate-limit."
  - q: "Do I need PagerDuty's paid Analytics add-on for MTTR reporting?"
    a: "No. The skill computes MTTA, MTTR, responder workload, and noisy-service rankings locally from the incidents and log entries any REST API key can read, so you get the post-incident numbers without the paid Analytics tier."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pagerduty/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pagerduty/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your PagerDuty credentials once; pagerduty-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a PagerDuty question in plain language; it runs pagerduty-cli for you."
---

# PagerDuty + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the PagerDuty
> API. Not affiliated with, endorsed by, or sponsored by PagerDuty, Inc..

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

Ask "who's on call for the payments service right now, and when do they hand off?" or "what's our MTTR by service this month?" and get the answer in one command. PagerDuty syncs to a local SQLite mirror, so post-incident analytics, on-call coverage audits, and escalation-gap checks run instantly and offline: no Analytics add-on, no portal clicking, no per-question API call.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/pagerduty){: .btn}

## Instead of clicking through PagerDuty, just ask

**Instead of** Clicking through the PagerDuty Analytics dashboard service by service the night before a QBR to stitch together an MTTR report
**just ask:** *"What's our mean time to acknowledge and resolve by service over the last 30 days?"*
<sub>Your agent runs: <code>pagerduty-cli insights mttr --by service --since 30d</code></sub>

**Instead of** Opening every escalation policy and schedule by hand to find which services have a coverage gap
**just ask:** *"Which services have a broken escalation chain or a single point of failure?"*
<sub>Your agent runs: <code>pagerduty-cli audit coverage --severity high</code></sub>

**Instead of** Pinging the team in Slack to find out who is actually on call for a service right now
**just ask:** *"Who's on call for the Payments service right now, and when's the handoff?"*
<sub>Your agent runs: <code>pagerduty-cli oncall who --service "Payments"</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/pagerduty/wide-1200x630.png" src="/assets/video/pagerduty/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/pagerduty/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What's on fire right now, ranked by SLA risk? | `pagerduty-cli pulse` |
| Who's on call for a service right now, and when's the handoff? | `pagerduty-cli oncall who --service "Payments"` |
| What's our MTTA and MTTR by service this month? | `pagerduty-cli insights mttr --by service --since 30d` |
| Which services have a broken escalation chain or single point of failure? | `pagerduty-cli audit coverage --severity high` |
| Where does a schedule have nobody on call over the next two weeks? | `pagerduty-cli audit schedule-gaps --days 14` |
| Which responders carry the most pages and off-hours load? | `pagerduty-cli insights responders --since 30d` |
| Which services are the noisiest? | `pagerduty-cli insights noisy --top 10 --since 7d` |
| What changed right before this incident broke? | `pagerduty-cli incidents changes <incident-id> --window 4h` |
| What's the full timeline of this incident? | `pagerduty-cli incidents timeline <incident-id>` |
| Which open incidents are quietly rotting with no recent activity? | `pagerduty-cli insights stale --hours 24` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/pagerduty/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/pagerduty/guide.md).

## What makes this one different

Most PagerDuty integrations and MCP servers proxy each question into a live API call - fine for reading one incident, costly for "MTTR by service across the quarter," which otherwise means PagerDuty's paid Analytics tier or a call per service. This skill syncs PagerDuty into a local SQLite mirror, so cross-object analytics (insights mttr, insights responders, audit coverage) become one offline join: instant, rate-limit-free, computed without the Analytics add-on, and the AI sees the answer, not a raw data dump.

PagerDuty's own AI and Analytics live in the web app and largely on paid tiers; this skill puts the same post-incident math - MTTA/MTTR, responder workload, noisy-service ranking, escalation-coverage and schedule-gap audits - in your terminal and your AI agent, computed offline from data any REST API key can read. It complements the portal; it does not replace your account.

## The pain this closes

- On-call responders drown in pages - the average engineer gets roughly 50 alerts a week and only a handful matter, so the real incident hides in the noise and burnout climbs (Catchpoint 2025 SRE report).
- The numbers leadership asks for at the QBR - MTTA, MTTR, who's carrying the on-call load - sit behind PagerDuty's paid Analytics tier or a CSV you assemble by hand.
- Nobody notices the hole in next week's on-call schedule until an incident finds it first.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/pagerduty/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/pagerduty/install.ps1 | iex
```

After install, authenticate once with your PagerDuty credentials, then verify with `pagerduty-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | pulse, insights mttr, insights responders, audit coverage, oncall who, search, incidents list / get / timeline | Allow |
| Write (routine) | incidents update (acknowledge / resolve / reassign), incidents snooze, incidents notes create-incident, incidents create | Preview with --dry-run, then a reviewed write |
| Destructive / config | escalation-policies delete-escalation-policy, automation-actions delete, business-services delete, addons delete | Human-in-the-loop only |

The skill reads everything through your PagerDuty REST API key and writes only when you tell it to - acknowledging, resolving, snoozing, or noting incidents, and creating or editing services, schedules, and policies. Reads (pulse, insights, audit, oncall, search) are always safe to run. Routine writes should be previewed with --dry-run and approved; deletes and config changes are human-in-the-loop only. The strongest control is scoping the API key to what your workflow actually needs. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/pagerduty/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local PagerDuty MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my PagerDuty data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my PagerDuty API rate limits?

Rarely. Read questions run against the local SQLite mirror, not the API - you sync once, then analytics, audits, and search are offline. Only sync and live writes call PagerDuty, and the CLI honors a configurable --rate-limit.

### Do I need PagerDuty's paid Analytics add-on for MTTR reporting?

No. The skill computes MTTA, MTTR, responder workload, and noisy-service rankings locally from the incidents and log entries any REST API key can read, so you get the post-incident numbers without the paid Analytics tier.


## Status

Beta. Validated against the PagerDuty API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
