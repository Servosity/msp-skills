---
layout: default
title: "RocketCyber MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "The first CLI and MCP server for RocketCyber Managed SOC, with triage and posture analytics no console page or API call computes."
permalink: /skills/rocketcyber/
skill_name: "RocketCyber MCP"
image: /assets/social/rocketcyber/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for RocketCyber?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for RocketCyber, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the RocketCyber MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local RocketCyber MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your RocketCyber API token once."
  - q: "Is my RocketCyber data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local SQLite mirror are all local. The AI sees query results, not raw bulk data, and your API token is never bundled or transmitted by MSP Skills - only sent to the RocketCyber API you point it at."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "What RocketCyber access do I need?"
    a: "A RocketCyber provider account and an API token you generate in the RocketCyber app. The skill talks to the RocketCyber Customer API v3 (US region by default; set ROCKETCYBER_BASE_URL for the EU endpoint) and reads your own SOC data - incidents, agents, detection events, Defender, Microsoft 365 posture, and suppression rules - scoped to the accounts your token can see."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your RocketCyber credentials once; rocketcyber-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a RocketCyber question in plain language; it runs rocketcyber-cli for you."
---

# The RocketCyber MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Kaseya.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for RocketCyber. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects RocketCyber to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "what broke across all my RocketCyber clients overnight?" and get one ranked board - open incidents, malicious event counts, and offline agents - instead of clicking through a per-client console. The same skill ranks devices at risk, computes incident MTTR for QBRs, trends Microsoft 365 secure scores, and flags stale suppression rules that quietly hide real detections. All from the terminal.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/rocketcyber){: .btn}

## Instead of clicking through RocketCyber, just ask

**Instead of** Logging into the RocketCyber console for each client and eyeballing the incidents tab to see what fired overnight
**just ask:** *"What broke across all my RocketCyber clients in the last 24 hours?"*
<sub>Your agent runs: <code>rocketcyber-cli triage --since 24h --agent</code></sub>

**Instead of** Exporting Defender detections to a spreadsheet and sorting by count to find the worst machines
**just ask:** *"Which devices are most at risk right now?"*
<sub>Your agent runs: <code>rocketcyber-cli defender riskiest --account-id 2 --top 10 --json</code></sub>

**Instead of** Scrolling the suppression-rules list by hand to find old rules that might be hiding live detections
**just ask:** *"Which suppression rules are stale and could be masking alerts?"*
<sub>Your agent runs: <code>rocketcyber-cli suppression audit --stale-after 90d --json</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/rocketcyber/wide-1200x630.png" src="/assets/video/rocketcyber/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/rocketcyber/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What broke across all my clients overnight? | `rocketcyber-cli triage --since 24h` |
| Which devices went dark this week? | `rocketcyber-cli agents stale --since 7d` |
| How fast is my SOC actually resolving incidents? | `rocketcyber-cli incidents mttr --since 90d` |
| Which machines are riskiest in Defender right now? | `rocketcyber-cli defender riskiest --top 10` |
| Is this client's Microsoft 365 posture improving? | `rocketcyber-cli office trend --account-id 2` |
| Which suppression rules are stale and may hide detections? | `rocketcyber-cli suppression audit --stale-after 90d` |
| What detection events fired, by verdict? | `rocketcyber-cli events summary --account-id 2` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/rocketcyber/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/rocketcyber/guide.md).

## What makes this one different

Most RocketCyber integrations proxy each question into a single live API call - fine for one record, useless for "across all 47 clients last quarter." This skill syncs RocketCyber into a local SQLite mirror with full-text search, so cross-account rollups like triage, MTTR, and secure-score trends become one offline query. Your agent sees the computed answer, not pages of raw JSON.

The RocketCyber console shows you live dashboards one client at a time. This skill doesn't replace the SOC - it gives your AI agent a terminal-native, multi-account read of the same data plus analytics the console won't compute for you: incident MTTR, device risk ranking, secure-score trend, and suppression-rule hygiene.

## The pain this closes

- SOC alert fatigue: open incidents, detection events, and offline agents each live on a different console tab, and you switch tabs per client account to answer one question.
- QBR and SLA prep means hand-computing MTTR and screenshotting secure-score charts, because the console shows the dashboard but won't hand you the trend or the number.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/rocketcyber/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/rocketcyber/install.ps1 | iex
```

After install, authenticate once with your RocketCyber credentials, then verify with `rocketcyber-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | triage, incidents, agents (+ stale), defender (+ riskiest), events list/summary, office trend, suppression rules/audit, reports, search, sync | Allow |
| Write (routine) | import <resource> (create/upsert from a JSONL file) | Preview with --dry-run, then a reviewed write |
| Credential / config | auth set-token, auth logout | Human-in-the-loop only |

The skill reads your RocketCyber SOC data - incidents, agents, detection events, Defender and Microsoft 365 posture, and suppression rules - and computes the analytics locally. The only command that writes to the API is `import` (create/upsert from a JSONL file), and it supports `--dry-run` to preview every request before sending. `auth set-token` and `auth logout` manage your stored credential. Keep autonomous agents on read plus previewed imports, and require a human for credential changes. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/rocketcyber/governance.md).

## Frequently asked questions

### Is there an MCP server for RocketCyber?

Yes - this one. A free, open source MCP server and Claude Code Skill for RocketCyber, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the RocketCyber MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local RocketCyber MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your RocketCyber API token once.

### Is my RocketCyber data safe?

Your data stays on your machine. The CLI, MCP server, and the local SQLite mirror are all local. The AI sees query results, not raw bulk data, and your API token is never bundled or transmitted by MSP Skills - only sent to the RocketCyber API you point it at.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What RocketCyber access do I need?

A RocketCyber provider account and an API token you generate in the RocketCyber app. The skill talks to the RocketCyber Customer API v3 (US region by default; set ROCKETCYBER_BASE_URL for the EU endpoint) and reads your own SOC data - incidents, agents, detection events, Defender, Microsoft 365 posture, and suppression rules - scoped to the accounts your token can see.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Blumira](/skills/blumira/) · [CIPP](/skills/cipp/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [ThreatLocker](/skills/threatlocker/)

## Status

Beta. Validated against the RocketCyber API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
