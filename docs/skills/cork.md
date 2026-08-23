---
layout: default
title: "Cork MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Cork API operation, plus cross-client risk attribution, exploitability-first vulnerability triage, overdue-compliance detection, and stale-connector health checks that a stateless API mirror cannot answer in a single call."
permalink: /skills/cork/
skill_name: "Cork MCP"
image: /assets/social/cork/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Cork?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Cork, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Cork MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Cork MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your Cork API key once."
  - q: "Is my Cork data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local, and the MCP server speaks stdio only so it opens no network listener. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Cork API rate limits?"
    a: "`sync` fills a local SQLite mirror, and search and score regressions answer from it without touching the API. export is NOT one of them - it paginates the live API and is one of the heaviest readers here, so pass --limit unless you mean to pull everything. The other seven analysis commands fetch live on every run, and two of them fan out (coverage gaps per connector, compliance overdue per client), so those are the ones to watch. Each caps its own scan and tells you when it truncated."
  - q: "Why does my Cork key get a 403 on some commands?"
    a: "A Cork API key inherits the permissions of the user who created it. A 403 on the distributor or integration endpoints usually means the key was minted by an operator without that scope, not that the key is wrong. That property is also your best control: mint a read-only key for read-only work."
  - q: "Will this replace the Cork portal?"
    a: "No, it complements it. The portal is still where you configure integrations and watch a single client. This skill answers the cross-client questions the portal makes you click through - score attribution, regression ranking, exploitability triage, stale connectors, coverage gaps - from your AI."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cork/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cork/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Cork credentials once; cork-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Cork question in plain language; it runs cork-cli for you."
---

# The Cork MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Cork Protection, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Cork. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Cork to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI why a client's Cork risk score moved and get the component that actually drove it, not just the number. It reads your whole book of business at once: which clients slipped this week, what to patch first by what is genuinely being exploited, which compliance events have blown their remediation window, and which connector is reporting healthy while its data quietly went stale.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/cork){: .btn}

## Instead of clicking through Cork, just ask

**Instead of** Export two score snapshots from Cork and subtract the four components in a spreadsheet to work out what moved before a QBR
**just ask:** *"Why did this client's Cork score drop this month?"*
<sub>Your agent runs: <code>cork-cli score attribute <client-uuid> --since 30d</code></sub>

**Instead of** Pull the vulnerability inventory and re-sort it by hand because the API can only sort by vendor and product name
**just ask:** *"What should we patch first across all our clients?"*
<sub>Your agent runs: <code>cork-cli vulnerabilities triage</code></sub>

**Instead of** Open each integration in the Cork portal and eyeball its last sync time against the clock
**just ask:** *"Which of our Cork connectors have quietly stopped syncing?"*
<sub>Your agent runs: <code>cork-cli integrations health</code></sub>


## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Why did this client's risk score move, and which component drove it? | `cork-cli score attribute <client-uuid> --since 30d` |
| Which clients across the whole book slipped the most this week? | `cork-cli score regressions --since 7d` |
| What should we patch first, ranked by what is actually being exploited? | `cork-cli vulnerabilities triage` |
| An advisory just named a CVE. Are we exposed, and where? | `cork-cli vulnerabilities exposure CVE-2023-21608` |
| Which compliance events have blown their remediation window? | `cork-cli compliance overdue` |
| Which connectors report healthy but stopped syncing? | `cork-cli integrations health` |
| Did this client get fully monitored after onboarding, or did an endpoint fall through? | `cork-cli coverage gaps --client <client-uuid>` |
| Which unwarranted clients are carrying the most risk right now? | `cork-cli warranties exposure` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/cork/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/cork/guide.md).

## What makes this one different

A stateless API mirror can answer one client at a time, right now. Every question an MSP actually asks of Cork is cross-client, cross-time, or both, and the API exposes no cross-client aggregate endpoint to build them from. This skill syncs Cork into a local SQLite mirror and answers those questions as one offline join: score attribution across a window, regression ranking over the whole book, exploitability-first triage, and a compliance join the API leaves in two separate endpoints.

The Cork portal shows one client's score, one client's compliance feed, one connector's status. This skill answers the questions that span all of them at once - who regressed, what to patch first, which connector went stale, who has no warranty - and it explains a score move by differencing its components instead of just reporting the new number.

## The pain this closes

- Cork reports a per-client risk score but never differences the four components behind it, so working out why a score moved means exporting two snapshots and subtracting by hand before every QBR.
- There is no cross-client aggregate endpoint in the Cork API at all, so 'which clients got worse this week' needs a fan-out across every client with nothing stored to compare yesterday against - which means in practice nobody asks it, and regressions surface when a client notices.
- A Cork integration can report connection_status ok while its last sync is days old. Both fields ship in the same payload and nothing compares the timestamp to now, so the dashboard stays green while every risk number behind it silently drifts.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cork/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cork/install.ps1 | iex
```

After install, authenticate once with your Cork credentials, then verify with `cork-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | score attribute, score regressions, vulnerabilities triage, vulnerabilities exposure, compliance overdue, integrations health, coverage gaps, warranties exposure, sync, search, export, and every list/get command except the secret-returning reads in the Credential row | Allow |
| Write (routine) | integrations connect, integrations update, integrations resync integration | Preview with --dry-run, then a reviewed write |
| Credential / security | integrations credentials, integrations credentials get-integration (printed verbatim, not redacted), integrations raw-data get-integration (returns a presigned URL that downloads the connector's full raw data with no further auth), the credential fields of integrations update | Human-in-the-loop only, never in a blanket allow-all-reads policy |
| Destructive / endpoint-affecting | integrations delete, software install (installs a package on a real customer device through its RMM integration) | Human-in-the-loop only, explicit confirmation |
| Admin | distributor provision-partner | Operator-only, not for agents |
| Bulk write | import <resource> --input file.jsonl - one POST per line into the write endpoints above, continuing past failures | Human-in-the-loop only, explicit confirmation. Never unattended |

The skill reads your Cork clients, devices, risk scores, compliance events, vulnerabilities, integrations, warranties, and invoices, and it can connect, update, resync, or delete integrations, install a software package on a mapped device through that device's RMM integration, and provision a partner account. Reads and sync are always safe to allow. Routine integration writes should be previewed with --dry-run, then approved. Deleting an integration, reading or replacing connector credentials, installing software on a customer endpoint, and provisioning a partner are human-in-the-loop only. The strongest control is the credential itself: a Cork API key inherits the permissions of the user who created it, so a key minted by a read-only user makes the destructive tiers unreachable. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/cork/governance.md).

## Frequently asked questions

### Is there an MCP server for Cork?

Yes - this one. A free, open source MCP server and Claude Code Skill for Cork, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Cork MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Cork MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your Cork API key once.

### Is my Cork data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local, and the MCP server speaks stdio only so it opens no network listener. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Cork API rate limits?

`sync` fills a local SQLite mirror, and search and score regressions answer from it without touching the API. export is NOT one of them - it paginates the live API and is one of the heaviest readers here, so pass --limit unless you mean to pull everything. The other seven analysis commands fetch live on every run, and two of them fan out (coverage gaps per connector, compliance overdue per client), so those are the ones to watch. Each caps its own scan and tells you when it truncated.

### Why does my Cork key get a 403 on some commands?

A Cork API key inherits the permissions of the user who created it. A 403 on the distributor or integration endpoints usually means the key was minted by an operator without that scope, not that the key is wrong. That property is also your best control: mint a read-only key for read-only work.

### Will this replace the Cork portal?

No, it complements it. The portal is still where you configure integrations and watch a single client. This skill answers the cross-client questions the portal makes you click through - score attribution, regression ranking, exploitability triage, stale connectors, coverage gaps - from your AI.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Blumira](/skills/blumira/) · [CIPP](/skills/cipp/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [ThreatLocker](/skills/threatlocker/)

## Status

Beta. Validated against the Cork API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
