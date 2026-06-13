---
layout: default
title: "PandaDoc MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "Every PandaDoc endpoint, plus an offline document pipeline no other PandaDoc tool has \u2014 stalled deals, aging, recipient engagement, and open quote value from a local store."
permalink: /skills/pandadoc/
skill_name: "PandaDoc MCP"
image: /assets/social/pandadoc/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local PandaDoc MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my PandaDoc data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my PandaDoc API rate limits?"
    a: "Day-to-day questions read the local mirror, so they never touch the API. Only `sync`, `tail`, and a few live commands (such as `reminder-gaps`) call PandaDoc directly, and the CLI honors a configurable `--rate-limit` so you stay inside your plan's limits."
  - q: "Do I need to be a PandaDoc partner or customer?"
    a: "You need your own PandaDoc account with API access (included on PandaDoc's paid plans). The skill authenticates with your own `PANDADOC_API_KEY` - there is no Servosity or PandaDoc partner requirement."
  - q: "Will this replace my PandaDoc portal?"
    a: "No. You still create, send, and sign documents in PandaDoc. This adds the cross-document reporting and follow-up rollups the portal doesn't surface, so you can ask your AI instead of exporting spreadsheets."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pandadoc/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pandadoc/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your PandaDoc credentials once; pandadoc-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a PandaDoc question in plain language; it runs pandadoc-cli for you."
---

# PandaDoc + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the PandaDoc
> API. Not affiliated with, endorsed by, or sponsored by PandaDoc, Inc..

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

Ask your AI "which proposals are stalled?" or "what's our open quote value?" and get the answer in seconds. PandaDoc's portal shows one document at a time and has no rollup for these. This skill syncs your documents, templates, and contacts into a local mirror, so cross-document questions - stalled deals, aging quotes, recipient engagement, dollars in-flight - become one instant query instead of a manual export-and-pivot.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/pandadoc){: .btn}

## Instead of clicking through PandaDoc, just ask

**Instead of** Export the documents report and pivot it in a spreadsheet to spot which proposals went quiet
**just ask:** *"Which proposals are stalled?"*
<sub>Your agent runs: <code>pandadoc-cli stalled --days 14</code></sub>

**Instead of** Click through every open quote in the portal adding up the dollar amounts by hand
**just ask:** *"How much money is tied up in open quotes right now?"*
<sub>Your agent runs: <code>pandadoc-cli value</code></sub>

**Instead of** Scroll the documents list trying to remember which clients haven't signed anything lately
**just ask:** *"Which clients have gone cold?"*
<sub>Your agent runs: <code>pandadoc-cli cold-clients --days 30</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/pandadoc/wide-1200x630.png" src="/assets/video/pandadoc/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/pandadoc/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which documents were sent but never completed? | `pandadoc-cli stalled --days 14` |
| How much money is tied up in open quotes? | `pandadoc-cli value` |
| What does my whole document funnel look like right now? | `pandadoc-cli pipeline` |
| How long has each document sat in its current status? | `pandadoc-cli aging` |
| Which clients haven't signed anything in a month? | `pandadoc-cli cold-clients --days 30` |
| Who should I follow up with today? | `pandadoc-cli followup --days 7` |
| Which recipients actually open and sign vs. let documents sit? | `pandadoc-cli engagement` |
| Which templates actually close? | `pandadoc-cli template-stats` |
| Which sent documents have no auto-reminder set? | `pandadoc-cli reminder-gaps` |
| What changed in the last day? | `pandadoc-cli since 24h` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/pandadoc/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/pandadoc/guide.md).

## What makes this one different

Most PandaDoc integrations proxy each question into a live API call - fine for one record, useless when you ask "across every open document, how much is unsigned and how old is it?" This skill syncs PandaDoc into a local SQLite mirror with full-text search, so aggregate questions become one instant offline join. Compound commands like `followup` and `forecast` join stalled documents to recipient emails and bucket open quote dollars by deal age - work a stateless API wrapper can't do, and your AI sees the answer, not a dump of raw documents.

PandaDoc's portal shows you one document at a time and has no rollup for "every stalled deal" or "total open quote value." This skill adds the cross-document reporting and follow-up worklists the portal lacks, from the terminal or your AI agent - it complements PandaDoc, it doesn't replace it.

## The pain this closes

- Proposals, SOWs, and MSAs go out, then go quiet - and nobody notices a deal is dying until the renewal slips. The r/msp community trades follow-up scripts for exactly this, because the proposal tool itself never tells you a deal stalled.
- There is no single view of how much revenue is sitting in unsigned quotes, so the forecast is a guess and the QBR slide is built by hand.
- Chasing unsigned documents means clicking through the PandaDoc portal one record at a time - there is no 'who do I nudge today' list.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/pandadoc/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/pandadoc/install.ps1 | iex
```

After install, authenticate once with your PandaDoc credentials, then verify with `pandadoc-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `pipeline`, `stalled`, `aging`, `value`, `engagement`, `search`, `documents list` | Allow |
| Write (routine) | `contacts create`, `contacts update`, `documents create`, `documents send document`, `documents recipients add-document`, `templates create` | Preview with --dry-run, then a reviewed write |
| Credential / destructive | `workspaces api-keys create`, `members token create-member`, `documents delete`, `documents bulk-delete`, `documents recipients delete-document` | Human-in-the-loop only |

Reads (pipeline, stalled, aging, value, search, list commands) are always safe and cannot change anything, so an agent can run them freely. Routine writes (create or update contacts, documents, and templates; send a document; add recipients) should be previewed with `--dry-run` and approved before they run. Credential-issuing commands (issue a workspace API key, create a member token, set the webhook shared key) and destructive deletes (delete or bulk-delete documents, remove recipients) are human-in-the-loop only. The strongest control is scoping the API key you grant the CLI. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/pandadoc/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local PandaDoc MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my PandaDoc data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my PandaDoc API rate limits?

Day-to-day questions read the local mirror, so they never touch the API. Only `sync`, `tail`, and a few live commands (such as `reminder-gaps`) call PandaDoc directly, and the CLI honors a configurable `--rate-limit` so you stay inside your plan's limits.

### Do I need to be a PandaDoc partner or customer?

You need your own PandaDoc account with API access (included on PandaDoc's paid plans). The skill authenticates with your own `PANDADOC_API_KEY` - there is no Servosity or PandaDoc partner requirement.

### Will this replace my PandaDoc portal?

No. You still create, send, and sign documents in PandaDoc. This adds the cross-document reporting and follow-up rollups the portal doesn't surface, so you can ask your AI instead of exporting spreadsheets.


## Status

Beta. Validated against the PandaDoc API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
