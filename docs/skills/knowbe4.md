---
layout: default
title: "KnowBe4 MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every KnowBe4 KMSAT reporting feature plus a local SQLite store that answers the cross-client questions the console can't \u2014 repeat-clicker hunts and training-coverage anti-joins."
permalink: /skills/knowbe4/
skill_name: "KnowBe4 MCP"
image: /assets/social/knowbe4/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for KnowBe4?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for KnowBe4, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the KnowBe4 MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local KnowBe4 MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my KnowBe4 data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local, and the bundled MCP server exposes read-only reporting tools only. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Does this replace the KnowBe4 console or need a special partner API?"
    a: "Neither. It uses your standard KMSAT Reporting API key (Account Settings - API - enable Reporting API) and your region (us, eu, ca, uk, or de). It reads what your account already exposes and adds the cross-client rollups the console doesn't. The one write path that needs extra setup is pushing custom risk events, which uses a separate, opt-in User Event API key."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/knowbe4/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your KnowBe4 credentials once; knowbe4-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a KnowBe4 question in plain language; it runs knowbe4-cli for you."
---

# The KnowBe4 MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by KnowBe4, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for KnowBe4. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects KnowBe4 to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

KnowBe4's console reports one tenant, one phishing test, one chart at a time. This skill syncs your KMSAT data into a local SQLite mirror and answers the questions the portal can't: which users clicked the bait in multiple phishing tests, whose risk score is deteriorating this quarter, and who clicked a phish but finished zero training - across every client, in seconds, from your terminal.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/knowbe4){: .btn}

## Instead of clicking through KnowBe4, just ask

**Instead of** Exporting a phishing-test CSV from every client tenant and pivot-tabling to find who failed more than once
**just ask:** *"Which users clicked the bait in two or more phishing tests in the last 90 days?"*
<sub>Your agent runs: <code>knowbe4-cli repeat-clickers --min-clicks 2 --since 90d --top 25</code></sub>

**Instead of** Clicking through each user's risk-score chart in the console to guess who is getting worse
**just ask:** *"Rank the users whose risk score worsened the most this quarter"*
<sub>Your agent runs: <code>knowbe4-cli risk-drift --window 90d --worsened --top 20</code></sub>

**Instead of** Cross-referencing the phishing report against the training report by hand to find people who failed and never trained
**just ask:** *"Who clicked a phish but has no passed training to show for it?"*
<sub>Your agent runs: <code>knowbe4-cli untrained-clickers --since 180d</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/knowbe4/wide-1200x630.png" src="/assets/video/knowbe4/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/knowbe4/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Who clicked the bait in more than one phishing test? | `knowbe4-cli repeat-clickers --min-clicks 2 --since 90d` |
| Whose risk score is getting worse this quarter? | `knowbe4-cli risk-drift --window 90d --worsened --top 20` |
| Who clicked a phish but never passed training? | `knowbe4-cli untrained-clickers --since 180d` |
| Which active users have zero training or zero phishing coverage? | `knowbe4-cli coverage-gaps` |
| Is training actually working for the Finance group? | `knowbe4-cli phish-prone-trend --group "Finance" --since 12mo` |
| Who are my highest-risk users, with the why behind the score? | `knowbe4-cli risk-leaderboard --top 25` |
| Which departments are driving our risk up? | `knowbe4-cli group-risk-contribution --window 90d --top 10` |
| Assemble the full client quarterly review in one command | `knowbe4-cli qbr --since 90d` |
| Who never reports a simulated phish? | `knowbe4-cli report-rate --bottom 25` |
| Is my synced data fresh enough to trust a clicker hunt? | `knowbe4-cli freshness` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/knowbe4/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/knowbe4/guide.md).

## What makes this one different

Most KnowBe4 integrations and MCP servers proxy each question into a single live API call - fine for one user, one test. But repeat-clicker hunts, risk drift, and untrained-clicker anti-joins need data fused across many phishing tests and against the training records the console keeps in a separate report. This skill syncs KnowBe4 into a local SQLite mirror, so those cross-test, cross-report questions become one instant offline join instead of a wall of API pulls.

KnowBe4's console and Virtual Risk Officer give you per-tenant dashboards; this skill adds the cross-client, cross-test rollups and anti-joins the portal never exposes - repeat clickers across every phishing test, risk drift ranked across all users, and clicked-but-untrained remediation lists - all from your own synced data and pointed at by your AI agent.

## The pain this closes

- MSPs running KnowBe4 across dozens of client tenants live in CSV exports: the console answers one account and one phishing test at a time, so 'who are my repeat clickers across all clients' becomes an afternoon of spreadsheets.
- At QBR time a vCISO needs the short list - who is deteriorating, who never reports, who failed and never trained - but the portal shows risk numbers without the why, and nothing correlates phishing results against training completion.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/knowbe4/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/knowbe4/install.ps1 | iex
```

After install, authenticate once with your KnowBe4 credentials, then verify with `knowbe4-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | account info, users list, groups list, phishing-tests list, training-enrollments list, risk-leaderboard, repeat-clickers, sync, search, qbr | Allow |
| Write (routine) | events create, import | Preview with --dry-run, then a reviewed write |
| Destructive / config | events delete | Human-in-the-loop only |

The skill reads your KnowBe4 reporting data - accounts, users, groups, phishing tests, training, risk scores - and can sync it to a local SQLite mirror; all of that is read-only and safe to let an agent run, and the bundled MCP server exposes only those read tools. The only write paths are CLI-only: pushing or deleting custom risk events on a user's timeline (a separate, opt-in User Event API key) and bulk import. Keep those human-reviewed and preview them with --dry-run first. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/knowbe4/governance.md).

## Frequently asked questions

### Is there an MCP server for KnowBe4?

Yes - this one. A free, open source MCP server and Claude Code Skill for KnowBe4, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the KnowBe4 MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local KnowBe4 MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my KnowBe4 data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local, and the bundled MCP server exposes read-only reporting tools only. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Does this replace the KnowBe4 console or need a special partner API?

Neither. It uses your standard KMSAT Reporting API key (Account Settings - API - enable Reporting API) and your region (us, eu, ca, uk, or de). It reads what your account already exposes and adds the cross-client rollups the console doesn't. The one write path that needs extra setup is pushing custom risk events, which uses a separate, opt-in User Event API key.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Blumira](/skills/blumira/) · [CIPP](/skills/cipp/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Huntress](/skills/huntress/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [ThreatLocker](/skills/threatlocker/)

## Status

Beta. Validated against the KnowBe4 API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
