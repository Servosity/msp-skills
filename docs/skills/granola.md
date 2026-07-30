---
layout: default
title: "Granola MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Search, export, and analyze every Granola meeting across all your calls - attendee timelines, talk-time roll-ups, and full-text transcript search from an offline local mirror."
permalink: /skills/granola/
skill_name: "Granola MCP"
image: /assets/social/granola/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Granola?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Granola, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Granola MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Granola MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Granola data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need a Granola API key?"
    a: "You need a Granola account. Cross-meeting sync pulls from the Granola public REST API using a personal or enterprise API key (GRANOLA_API_KEY); recent Granola desktop versions seal the local cache, so the API key is the supported path. Run granola-cli doctor to confirm auth is working."
  - q: "Will this replace the Granola app?"
    a: "No. Keep recording and reviewing meetings in Granola as usual. This skill sits alongside it for the cross-meeting questions the app answers one meeting at a time - attendee timelines, bulk export, talk-time, and topic search across everything you've recorded."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/granola/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/granola/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Granola credentials once; granola-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Granola question in plain language; it runs granola-cli for you."
---

# The Granola MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Granola.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Granola. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Granola to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "every meeting we've had with Northwind IT" or "talk-time across last month's QBRs" and get the answer across all your calls at once - not one meeting at a time. Granola records and summarizes each meeting; this skill syncs them into a local, searchable mirror so your agent queries transcripts, summaries, and attendee history across every client conversation.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/granola){: .btn}

## Instead of clicking through Granola, just ask

**Instead of** Scrolling back through weeks of recorded calls to reconstruct what you told a client before a QBR
**just ask:** *"Show me every meeting we've had with sarah@northwind.com"*
<sub>Your agent runs: <code>granola-cli attendee timeline sarah@northwind.com</code></sub>

**Instead of** Opening each recorded call one by one to copy notes and transcript into your PSA or a follow-up doc
**just ask:** *"Export last week's client calls to markdown files"*
<sub>Your agent runs: <code>granola-cli export-all --out ./client-calls --last 7d</code></sub>

**Instead of** Guessing which scheduled client calls this week never actually got recorded
**just ask:** *"Which calendar events this week didn't get recorded?"*
<sub>Your agent runs: <code>granola-cli calendar overlay --missed-only</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/granola/wide-1200x630.png" src="/assets/video/granola/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/granola/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Every meeting with a client contact, oldest to newest? | `granola-cli attendee timeline sarah@northwind.com` |
| The last few calls with a contact, notes and AI summary inlined? | `granola-cli attendee brief sarah@northwind.com --last 5` |
| Every mention of a topic across all synced meetings? | `granola-cli search "backup migration"` |
| One meeting as notes + summary + transcript in a single file? | `granola-cli export not_06Yq6JtogRihEr --out ./meeting.md` |
| Bulk-export the last 7 days of calls to a folder? | `granola-cli export-all --out ./calls --last 7d` |
| Who talked the most across last month's meetings? | `granola-cli talktime --by participant --since 30d` |
| Which scheduled calls this week never got recorded? | `granola-cli calendar overlay --missed-only` |
| Top attendees by meeting count? | `granola-cli stats attendees --top 10` |
| Is auth working and is my local mirror fresh? | `granola-cli doctor` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/granola/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/granola/guide.md).

## What makes this one different

Most meeting-notes integrations answer one question by making one live API call - fine for a single meeting, useless when you need to reason across a quarter of calls. This skill syncs your meetings into a local SQLite mirror with full-text search, so cross-meeting questions - an attendee's whole timeline, talk-time rolled up across dozens of calls, every mention of a topic - resolve as one instant local query your agent can actually read.

Granola's own AI answers questions inside a single meeting; this complements it by making every meeting queryable at once - attendee history, cross-call search, talk-time aggregation, and missed-recording detection the per-meeting view can't give you.

## The pain this closes

- Client context lives in dozens of separate meeting recordings - before a QBR you're scrubbing back through weeks of calls to remember what you promised whom.
- The notes all exist, but nobody can answer "what did we tell this client last quarter" without opening every meeting by hand.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/granola/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/granola/install.ps1 | iex
```

After install, authenticate once with your Granola credentials, then verify with `granola-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | search, meetings list/get, transcript get, attendee timeline/brief, talktime, stats, calendar overlay, export, doctor | Allow |
| Write (routine) | meetings delete (reversible soft-delete), meetings restore | Preview with --dry-run, then a reviewed write |
| Destructive / config | auth set-token / logout, mcp-auth login (credential handling) | Human-in-the-loop only |

The skill reads your Granola meetings - notes, AI summaries, transcripts, folders, and attendees - into a local mirror and answers questions against it; the commands you use day to day are all read-only. The only account-changing commands are an explicit meetings delete (a reversible soft-delete) and meetings restore, both of which support --dry-run. Keep autonomous agents to read plus previewed writes, and require a human for delete/restore and any credential command. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/granola/governance.md).

## Frequently asked questions

### Is there an MCP server for Granola?

Yes - this one. A free, open source MCP server and Claude Code Skill for Granola, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Granola MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Granola MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Granola data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need a Granola API key?

You need a Granola account. Cross-meeting sync pulls from the Granola public REST API using a personal or enterprise API key (GRANOLA_API_KEY); recent Granola desktop versions seal the local cache, so the API key is the supported path. Run granola-cli doctor to confirm auth is working.

### Will this replace the Granola app?

No. Keep recording and reviewing meetings in Granola as usual. This skill sits alongside it for the cross-meeting questions the app answers one meeting at a time - attendee timelines, bulk export, talk-time, and topic search across everything you've recorded.


## More Documentation connectors

Run more than one Documentation tool, or comparing options? These connectors work the same way: [Hudu](/skills/hudu/) · [IT Glue](/skills/itglue/) · [Liongard](/skills/liongard/) · [PandaDoc](/skills/pandadoc/)

## Status

Beta. Validated against the Granola API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
