---
layout: default
title: "Riverside.fm MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Riverside API surface as MCP tools."
permalink: /skills/riverside-fm/
skill_name: "Riverside.fm MCP"
image: /assets/social/riverside-fm/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Riverside.fm?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Riverside.fm, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Riverside.fm MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Two paths can move data off the machine, both opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `RIVERSIDE_FM_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `RIVERSIDE_FM_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file). Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Riverside MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Riverside data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need a Riverside Business plan or an API key?"
    a: "No. The official Riverside API requires a custom-priced Business plan. This CLI reuses the session cookies from your already-logged-in browser to reach the same internal API the Riverside web app uses, so it works on Pro, Live, and Webinar accounts that can't issue an API key."
  - q: "Can this download other people's recordings?"
    a: "No. It authenticates as you, with your own browser session, and only reaches the studios and takes your account already has access to. It is scoped to exporting your own content, not scraping anyone else's."
  - q: "Will this change anything in my Riverside account?"
    a: "The commands you use to export, search, and convert all read - they only download and transform your data. The CLI does include the generic import command, which can POST records from a local file via the API's create/upsert path; it is not part of any workflow in this skill, and an agent should treat it as a reviewed write. Nothing in this CLI deletes or publishes."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/riverside-fm/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/riverside-fm/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Riverside.fm credentials once, then run riverside-fm-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Riverside.fm question in plain language; it runs riverside-fm-cli for you."
---

# The Riverside.fm MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by RiversideFM, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Riverside.fm. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Riverside.fm to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Pull every transcript, audio track, and video out of your own Riverside.com account in one command - no Business plan, no API key, no clicking Studio then Project then Take for each file. Ask your agent to archive a whole studio, find a quote across every episode, or convert a transcript to WebVTT, and it runs the real command against the same internal API the web app already uses.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/riverside-fm){: .btn}

## Instead of clicking through Riverside.fm, just ask

**Instead of** Open each episode in Riverside, click into every take, and download the transcript plus each participant's audio one file at a time.
**just ask:** *"Archive my whole studio since April 1st to a folder."*
<sub>Your agent runs: <code>riverside-fm-cli bulk export --studio my-studio --since 2026-04-01 --out ./archive</code></sub>

**Instead of** Scrub through hours of old recordings trying to remember which episode you said something in.
**just ask:** *"Find every episode where we talked about compounding loops."*
<sub>Your agent runs: <code>riverside-fm-cli search "compounding loops" --json</code></sub>

**Instead of** Re-type or screen-cap a transcript because Riverside only shows it inside the editor, never as a caption file.
**just ask:** *"Give me a WebVTT caption file for this session."*
<sub>Your agent runs: <code>riverside-fm-cli transcripts convert bf487406-af40-4bb4-b7f9-a6b49047b55d --format vtt --out ep.vtt</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/riverside-fm/wide-1200x630.png" src="/assets/video/riverside-fm/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/riverside-fm/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Back up everything in a studio to disk? | `riverside-fm-cli bulk export --studio my-studio --out ./archive` |
| Get the most useful asset for one recording in a single shot? | `riverside-fm-cli grab bf487406-af40-4bb4-b7f9-a6b49047b55d --out ./dl` |
| Find a quote across my whole transcript archive? | `riverside-fm-cli search "network effects" --json` |
| Turn a transcript into captions for a web player? | `riverside-fm-cli transcripts convert bf487406-af40-4bb4-b7f9-a6b49047b55d --format srt --out ep.srt` |
| Who talked the most in this episode? | `riverside-fm-cli transcripts talktime bf487406-af40-4bb4-b7f9-a6b49047b55d --json` |
| Which takes in a studio are fully ready to edit? | `riverside-fm-cli ready --studio my-studio` |
| Pull every Magic Clip for a project with fresh download URLs? | `riverside-fm-cli clips harvest --project 69fcda9fba030a19ae93a526 --download --out ./clips` |
| Refresh expiring CloudFront media links before they die? | `riverside-fm-cli media refresh --project 69fcda9fba030a19ae93a526 --prefetch --out ./media` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/riverside-fm/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/riverside-fm/guide.md).

## What makes this one different

Most Riverside integrations proxy one question into one live API call - fine for a single record, useless when you want to archive years of episodes or search across them. This skill syncs your account into a local SQLite mirror with full-text search and walks projects then takes then assets with a resume cursor, so a single command exports a whole studio and an aggregate query runs offline and instantly.

Riverside's own UI exports one take at a time and shows transcripts only inside its editor. This adds what the portal never has: one-command bulk studio export with resume, full-text search across every cached transcript, and conversion to WebVTT, SRT, plain text, JSON, or speaker-grouped Markdown - without re-hitting the API.

## The pain this closes

- Riverside has no bulk export: every transcript, audio track, and video comes out one click at a time, per take, down the Studio then Project then Take path. Podcasters in community groups openly ask how to batch-download Riverside recordings just to archive a show.
- The official Riverside API is gated behind a custom-priced Business plan, so Pro, Live, and Webinar accounts have no supported programmatic way to back up or search their own content.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/riverside-fm/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/riverside-fm/install.ps1 | iex
```

After install, authenticate once with your Riverside.fm credentials, then verify with `riverside-fm-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | bulk export, grab, search, transcripts convert, transcripts talktime, clips harvest, media refresh, ready, wait, sync - every download, search, and conversion command | Allow |
| Write (routine) | import (POSTs records from a local JSONL via the API's create/upsert path); the endpoint-mirror capability checks the safety scanner flagged by verb name (ai can-create-event, takes get-assets, clips get-patches) are themselves read-only GETs | Preview with --dry-run, then a reviewed write |
| Destructive / config | none - this CLI exposes no delete, publish, or account-config command | Human-in-the-loop only |

The Riverside data commands in this skill read: they download, search, and convert data from your own account. The one mutating command is the generic import, which POSTs records from a local JSONL file via the API's create/upsert path and is not part of any export workflow here. The CLI authenticates with your own browser session cookies, read from your environment only, and can do only what your Riverside account already permits. The safe agent policy is to allow reads freely and keep a human on import and anything else the safety scanner flags as a write. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/riverside-fm/governance.md).

## Frequently asked questions

### Is there an MCP server for Riverside.fm?

Yes - this one. A free, open source MCP server and Claude Code Skill for Riverside.fm, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Riverside.fm MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Two paths can move data off the machine, both opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `RIVERSIDE_FM_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `RIVERSIDE_FM_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file). Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Riverside MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Riverside data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need a Riverside Business plan or an API key?

No. The official Riverside API requires a custom-priced Business plan. This CLI reuses the session cookies from your already-logged-in browser to reach the same internal API the Riverside web app uses, so it works on Pro, Live, and Webinar accounts that can't issue an API key.

### Can this download other people's recordings?

No. It authenticates as you, with your own browser session, and only reaches the studios and takes your account already has access to. It is scoped to exporting your own content, not scraping anyone else's.

### Will this change anything in my Riverside account?

The commands you use to export, search, and convert all read - they only download and transform your data. The CLI does include the generic import command, which can POST records from a local file via the API's create/upsert path; it is not part of any workflow in this skill, and an agent should treat it as a reviewed write. Nothing in this CLI deletes or publishes.


## More Marketing connectors

Run more than one Marketing tool, or comparing options? These connectors work the same way: [WordPress](/skills/wordpress/)

## Status

Beta. Validated against the Riverside.fm API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
