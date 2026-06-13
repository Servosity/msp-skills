---
layout: default
title: "Abnormal Security MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "The full Abnormal Security REST API as an agent-ready CLI \u2014 with a local threat store, ranked SOC triage, and one-shot reporting no SOAR pack offers."
permalink: /skills/abnormal/
skill_name: "Abnormal Security MCP"
image: /assets/social/abnormal/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Abnormal Security MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Abnormal Security data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Abnormal API rate limits?"
    a: "It does not have to. A sync pulls your tenant into a local SQLite mirror once, then triage, search, and reporting answer from disk - so repeat questions never touch the API. For live calls you can cap throughput with --rate-limit and page large pulls."
  - q: "Do I need to be an Abnormal partner or customer?"
    a: "You need API access to an Abnormal Security tenant and a token generated from the portal's integration settings. The REST API does not require a separate partner tier - just a credential scoped to what you want the skill to do."
  - q: "Can it actually remediate, or only read?"
    a: "It can remediate threats and delete or move malicious messages through the remediation commands, and remediate-watch blocks until Abnormal reports the action reached a terminal state. Those actions are gated behind a human-in-the-loop policy - see the safety model below."
  - q: "Will it replace the Abnormal portal?"
    a: "No. Detection tuning, policy, and configuration stay in the portal. This is a read-first, action-on-approval surface for your terminal and your agents, not a replacement UI."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Abnormal Security credentials once; abnormal-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Abnormal Security question in plain language; it runs abnormal-cli for you."
---

# Abnormal Security + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the Abnormal Security
> API. Not affiliated with, endorsed by, or sponsored by Abnormal Security Corporation.

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

Abnormal Security plus your AI answers the SOC questions the portal makes you click for: what new email threats are still unremediated right now, what numbers go in this quarter's client security report, and an employee's full account-takeover risk picture - each in one command. It syncs your tenant to a local store, ranks the threat queue, and confirms remediations actually completed.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/abnormal){: .btn}

## Instead of clicking through Abnormal Security, just ask

**Instead of** Logging into the Abnormal portal every morning and scrolling the Threat Log to find what is new and not yet remediated
**just ask:** *"What new email threats hit us in the last 24 hours that nobody has remediated yet?"*
<sub>Your agent runs: <code>abnormal-cli triage --since 24h --top 20 --agent</code></sub>

**Instead of** Screenshotting dashboard tiles into a client QBR deck by hand every quarter
**just ask:** *"Pull last quarter's attacks-stopped and impersonation numbers for the client report"*
<sub>Your agent runs: <code>abnormal-cli report-snapshot --since 90d --csv</code></sub>

**Instead of** Clicking through the account-takeover case, then the employee profile, then their logins across three portal screens
**just ask:** *"Give me the full account-takeover risk picture for jane@acme.com"*
<sub>Your agent runs: <code>abnormal-cli employee-risk "jane@acme.com"</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/abnormal/wide-1200x630.png" src="/assets/video/abnormal/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/abnormal/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What new, unremediated email threats need attention right now? | `abnormal-cli triage --since 24h --top 20` |
| Pull a client-ready security report for the quarter | `abnormal-cli report-snapshot --since 90d --csv` |
| What is the account-takeover risk picture for this employee? | `abnormal-cli employee-risk "vip@acme.com"` |
| Is this vendor showing email-compromise signs? | `abnormal-cli vendor-risk "acme-supplies.com"` |
| Remediate a threat and block until it actually completes | `abnormal-cli remediate-watch <threat|case> <id>` |
| List the latest Abnormal cases | `abnormal-cli cases retrieve --all` |
| How many attacks did we stop this week? | `abnormal-cli aggregations attack-stopped-retrieve` |
| Find threats from a spoofed sender | `abnormal-cli threats retrieve --sender "ceo@spoofed.com"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/abnormal/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/abnormal/guide.md).

## What makes this one different

Most Abnormal Security integrations proxy each question into a live API call - fine for one record, but it falls over the moment you ask across a quarter of threats or every employee at once. This skill syncs your tenant into a local SQLite mirror with full-text search, so cross-entity questions - a threat joined to its messages, an employee joined to their open cases and 30-day logins - resolve as one offline query, and your AI sees the answer instead of raw bulk data.

Abnormal's own portal and AI detection stay the system of record and the place detection policy lives. This skill adds a terminal-and-agent surface the portal does not: a ranked triage queue, blocking remediation receipts, and one-shot client reporting your AI can drive without a human clicking through screens.

## The pain this closes

- Email threat triage means living in the portal - every shift starts by manually scrolling the Threat Log to spot what is new and still unremediated, with no ranked queue.
- Client security reporting is a copy-paste chore - screenshotting dashboard tiles into a QBR deck instead of pulling attacks-seen, attacks-stopped, and impersonation numbers once.
- Confirming a remediation actually finished means refreshing the portal and hoping - there is no terminal receipt that the action reached a terminal state.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/abnormal/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/abnormal/install.ps1 | iex
```

After install, authenticate once with your Abnormal Security credentials, then verify with `abnormal-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | triage, report-snapshot, threats/cases/vendors/employee retrieve, employee-risk, vendor-risk, aggregations, soar (API-token metadata only), search | Allow |
| Write (routine) | cases create (update case status), detection360 reports-create, api-resources resources-create-create / resources-update-partial-update / resources-actions-create | Preview with --dry-run, then a reviewed write |
| Remediation / destructive | threats create (remediate/unremediate), email-search search-remediate-create (delete/move mail), remediate-watch, import | Human-in-the-loop only |

The skill authenticates with a single ABNORMAL_API_TOKEN scoped to your tenant. Read commands - triage, reporting, threat/case/vendor/employee lookups, and API-token metadata - are always safe and can run unattended. Routine writes such as updating a case status, submitting a misclassification report, or managing API resources should be previewed with --dry-run and approved. Remediation actions that delete or move mail, remediate threats, or bulk-import records are human-in-the-loop only. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/abnormal/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Abnormal Security MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Abnormal Security data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Abnormal API rate limits?

It does not have to. A sync pulls your tenant into a local SQLite mirror once, then triage, search, and reporting answer from disk - so repeat questions never touch the API. For live calls you can cap throughput with --rate-limit and page large pulls.

### Do I need to be an Abnormal partner or customer?

You need API access to an Abnormal Security tenant and a token generated from the portal's integration settings. The REST API does not require a separate partner tier - just a credential scoped to what you want the skill to do.

### Can it actually remediate, or only read?

It can remediate threats and delete or move malicious messages through the remediation commands, and remediate-watch blocks until Abnormal reports the action reached a terminal state. Those actions are gated behind a human-in-the-loop policy - see the safety model below.

### Will it replace the Abnormal portal?

No. Detection tuning, policy, and configuration stay in the portal. This is a read-first, action-on-approval surface for your terminal and your agents, not a replacement UI.


## Status

Beta. Validated against the Abnormal Security API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
