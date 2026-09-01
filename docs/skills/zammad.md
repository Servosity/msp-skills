---
layout: default
title: "Zammad MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Zammad ticket, article, and Knowledge Base operation as one CLI and MCP server  -  plus a team-management layer (agent load, customer health, aging backlog, escalation triage, churn risk, feedback mining) the Zammad API can't answer in a single call."
permalink: /skills/zammad/
skill_name: "Zammad MCP"
image: /assets/social/zammad/wide-1200x630.png
verification: live-verified
faqs:
  - q: "Is there an MCP server for Zammad?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Zammad, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Zammad MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `ZAMMAD_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `ZAMMAD_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but zammad-mcp speaks HTTP natively: run `zammad-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Zammad data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Does it work with my Zammad instance, self-hosted or hosted?"
    a: "Both. Point it at any Zammad instance with ZAMMAD_URL (for example https://support.yourcompany.com) and a personal access token in ZAMMAD_API_TOKEN, created under Profile then Token Access. Nothing is hardcoded to a specific instance."
  - q: "Are the escalation, churn, and feedback signals AI sentiment analysis?"
    a: "No. They are transparent keyword-and-timing heuristics that surface the tickets and matched text for your AI or a human to judge. They flag candidates and show the evidence - they never claim a verdict on their own."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zammad/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zammad/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Zammad credentials once, then run zammad-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Zammad question in plain language; it runs zammad-cli for you."
---

# The Zammad MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Zammad GmbH.

**✓ Live-verified by Servosity (maintainer)** against a production tenant · 2026-08-16.

Yes - there is an MCP server for Zammad. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Zammad to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI who on the support team is overloaded, which customers are aging out, who sounds ready to churn, and what customers keep asking for - and get an answer built from every ticket at once. Zammad's portal shows one ticket at a time; this reads, searches, and changes tickets and the Knowledge Base, then answers the team-management questions the API can't return in a single call.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/zammad){: .btn}

## Instead of clicking through Zammad, just ask

**Instead of** Export the ticket list, pivot it in a spreadsheet, and eyeball who has the biggest queue
**just ask:** *"Who on support is overloaded right now?"*
<sub>Your agent runs: <code>zammad-cli agent-load --json</code></sub>

**Instead of** Click through every open ticket looking for the ones that are aging or where the customer sounds angry
**just ask:** *"Which tickets are open too long, and which customers sound ready to escalate?"*
<sub>Your agent runs: <code>zammad-cli escalate --json</code></sub>

**Instead of** Guess which accounts are at risk from memory and a gut feel
**just ask:** *"Which customers are trending toward churn?"*
<sub>Your agent runs: <code>zammad-cli churn-risk --json</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/zammad/wide-1200x630.png" src="/assets/video/zammad/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/zammad/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Who on the team is overloaded, and who is idle? | `zammad-cli agent-load --json` |
| Is each agent's queue growing or shrinking week over week? | `zammad-cli agent-trend --weeks 4 --json` |
| Which customers are struggling and should get attention first? | `zammad-cli customer-health --at-risk --json` |
| What tickets have been open too long, worst first? | `zammad-cli overdue --days 3 --json` |
| Which customers sound upset and should be escalated? | `zammad-cli escalate --json` |
| Which accounts are trending toward churn, and why? | `zammad-cli churn-risk --json` |
| What are customers asking for around features, pricing, and compliance? | `zammad-cli feedback-scan --bucket pricing --json` |
| Find open tickets (scope to a customer with organization_id:N) | `zammad-cli tickets search --query "state:open organization_id:123" --json` |
| Read a ticket's full conversation | `zammad-cli articles by-ticket 12345 --json` |
| Log an internal note on a ticket without opening the browser | `zammad-cli ticket note 12345 --body "Investigated, awaiting logs" --internal` |
| Search the Knowledge Base before answering a customer | `zammad-cli kb search "restore" --json` |
| See the whole Knowledge Base as a tree | `zammad-cli kb browse` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/zammad/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/zammad/guide.md).

## What makes this one different

Most Zammad integrations proxy each question straight into a live API call, so 'who is overloaded' or 'which customers are at risk' is impossible - the API has no endpoint that aggregates across every ticket. This syncs your tickets, articles, organizations, and users into a local mirror, then answers those cross-ticket questions with local joins that are instant, offline, and cost nothing per query.

Zammad's own AI features (ticket summary, writing assistant) work inside a single ticket; this works across the whole desk - team load, aging backlog, customer health, churn signals, and feedback themes - and pipes structured JSON straight to the AI agent you already use.

## The pain this closes

- The Zammad dashboard and overviews answer 'what is in this queue' but never 'who is overloaded, which customer is aging out, and who is about to churn' - a support lead ends up exporting tickets and rebuilding those answers by hand every week.
- Sentiment and 'what are customers asking for' live buried in thousands of ticket articles; there is no report that surfaces the upset threads or buckets feature/pricing/compliance requests, so those signals only surface after a customer has already left.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/zammad/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/zammad/install.ps1 | iex
```

After install, authenticate once with your Zammad credentials, then verify with `zammad-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | agent-load, agent-trend, customer-health, overdue, escalate, churn-risk, feedback-scan, tickets search/get, articles by-ticket, kb browse/search/get, search, sync | Allow |
| Write (routine) | ticket note, tickets create/update, articles create, tags add/remove, organizations create/update, users create/update, kb answer-create/publish/internal, kb category-create | Preview with --dry-run, then a reviewed write |
| Destructive / config | tickets delete, kb answer-delete | Human-in-the-loop only |

Read commands (agent-load, customer-health, escalate, churn-risk, feedback-scan, overdue, ticket and Knowledge Base reads, search, sync) only pull data into a local mirror and are safe to allow. Routine writes (adding a ticket note, creating or updating a ticket, tagging, publishing a Knowledge Base answer) change the desk and should be previewed with --dry-run before a reviewed run. Deleting a ticket or Knowledge Base answer is destructive and should stay human-in-the-loop. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/zammad/governance.md).

## Frequently asked questions

### Is there an MCP server for Zammad?

Yes - this one. A free, open source MCP server and Claude Code Skill for Zammad, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Zammad MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `ZAMMAD_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `ZAMMAD_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but zammad-mcp speaks HTTP natively: run `zammad-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Zammad data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Does it work with my Zammad instance, self-hosted or hosted?

Both. Point it at any Zammad instance with ZAMMAD_URL (for example https://support.yourcompany.com) and a personal access token in ZAMMAD_API_TOKEN, created under Profile then Token Access. Nothing is hardcoded to a specific instance.

### Are the escalation, churn, and feedback signals AI sentiment analysis?

No. They are transparent keyword-and-timing heuristics that surface the tickets and matched text for your AI or a human to judge. They flag candidates and show the evidence - they never claim a verdict on their own.


## More PSA connectors

Run more than one PSA tool, or comparing options? These connectors work the same way: [Autotask PSA](/skills/autotask/) · [ConnectWise PSA (Manage)](/skills/connectwise-manage/) · [HaloPSA](/skills/halopsa/) · [Kaseya BMS](/skills/kaseya-bms/) · [SuperOps](/skills/superops/) · [Syncro](/skills/syncro/)

## Status

Beta. Validated against the Zammad API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
