---
layout: default
title: "NetSapiens MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every NetSapiens v2 endpoint plus a reseller-wide fleet view: seat limits, offline phones, unrouted numbers and E911 gaps across every client at once."
permalink: /skills/netsapiens/
skill_name: "NetSapiens MCP"
image: /assets/social/netsapiens/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for NetSapiens?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for NetSapiens, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the NetSapiens MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `NETSAPIENS_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `NETSAPIENS_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the NetSapiens MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my NetSapiens data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "Does it work with Crexendo, OITVOIP and other white-label NetSapiens platforms?"
    a: "Yes. Set NETSAPIENS_SERVER to your provider's platform host and use an API key or OAuth2 access token from that platform. Without NETSAPIENS_SERVER the CLI talks to ns-api.com, which is only the NetSapiens public sandbox."
  - q: "Do I need a reseller-level API key?"
    a: "For the fleet audits, yes: a reseller-scoped key sees every client domain. An office-manager key sees one domain, so the fleet commands then cover only that client. A read-only key is enough for every audit."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your NetSapiens credentials once, then run netsapiens-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a NetSapiens question in plain language; it runs netsapiens-cli for you."
---

# The NetSapiens MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Crexendo, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for NetSapiens. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects NetSapiens to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask which clients are over their seat limit, which phones went offline, which numbers route nowhere and where E911 data is missing, and get one answer across every client domain on your NetSapiens platform. The reseller portal shows one domain at a time; this skill syncs your whole reseller into a local mirror, so each fleet-wide audit is one command.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/netsapiens){: .btn}

## Instead of clicking through NetSapiens, just ask

**Instead of** Open each client domain in the reseller portal, count users, call queues and auto attendants, and compare them to the limits by hand before you invoice.
**just ask:** *"Which clients are over or near their seat limits this month?"*
<sub>Your agent runs: <code>netsapiens-cli fleet seats --flagged --agent</code></sub>

**Instead of** Find out a desk phone is offline when the client calls to say it stopped ringing.
**just ask:** *"Which phones across all my clients are unregistered or have not pulled their config in two weeks?"*
<sub>Your agent runs: <code>netsapiens-cli fleet offline --agent</code></sub>

**Instead of** Click through every domain's numbers and dial rules to find the DID still pointing at a user who left or a queue that was deleted.
**just ask:** *"Which of my phone numbers route nowhere?"*
<sub>Your agent runs: <code>netsapiens-cli fleet dids --problems-only --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/netsapiens/wide-1200x630.png" src="/assets/video/netsapiens/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/netsapiens/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which clients are over or near a user, call queue, auto attendant or conference limit? | `netsapiens-cli fleet seats --flagged --json` |
| Which phones are unregistered, high-latency or never provisioned? | `netsapiens-cli fleet offline --json` |
| Which phone numbers route to a missing or unreachable user, queue or attendant? | `netsapiens-cli fleet dids --problems-only --json` |
| Which domains, users and devices are missing an emergency caller ID or point at an unknown emergency address? | `netsapiens-cli fleet e911 --json` |
| Which users had unanswered inbound calls in the last 7 days, and is their phone registered now? | `netsapiens-cli fleet missed --window 7d --json` |
| Which call queues are dropping callers, and are their agents' phones registered? | `netsapiens-cli fleet queues --window 7d --json` |
| What users, devices, numbers and domains were added, removed or re-pointed since my last check? | `netsapiens-cli fleet changes --json` |
| Which client, user and phones does this number belong to? | `netsapiens-cli trace 6025550142 --json` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/netsapiens/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/netsapiens/guide.md).

## What makes this one different

The NetSapiens MCP servers that exist today answer one live API call at a time: [phoneware/netsapiens-mcp](https://github.com/phoneware/netsapiens-mcp) is a hosted OAuth proxy over the full v1 and v2 API, and [MSPbots' oitvoip-mcp](https://github.com/MSPbotsAI/oitvoip-mcp) exposes five domain, reseller, device, subscriber and CDR methods. This skill runs on your machine and keeps a local SQLite mirror of every client domain, so a cross-client audit such as seat overages, unrouted DIDs or E911 gaps is one local query instead of hundreds of calls, and it can tell you what changed since last week.

The NetSapiens reseller portal is where you configure domains and routing, one client at a time. This skill adds the reseller-wide audits and change history the portal does not show, on the same v2 API and the same API key; configuration changes still belong in the portal or a reviewed write.

## The pain this closes

- A NetSapiens reseller runs dozens of client domains, and the portal answers one domain at a time: seat true-ups, offline phones and dead-end numbers all mean opening every client.
- E911 gaps cost real money: IPComms, a SIP trunking carrier, bills a $250 unregistered-911 fee per call placed from a number with no registered address, and Kari's Law and RAY BAUM's Act put the dispatchable-location duty on whoever runs the phone system.
- Unanswered calls and abandoned queues only surface when the client complains, because nobody reads every domain's CDRs and queue stats each week.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](/integrations/claude-desktop/) |
| **ChatGPT** (desktop app) | [Step-by-step →](/integrations/chatgpt/) |
| **Claude Code** | [Step-by-step →](/integrations/claude-code/) |
| **Codex CLI** | [Step-by-step →](/integrations/codex/) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](/which-agent/) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/netsapiens/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/netsapiens/install.ps1 | iex
```

After install, authenticate once with your NetSapiens credentials, then verify with `netsapiens-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet seats, fleet offline, fleet dids, fleet e911, fleet missed, fleet queues, fleet changes, trace, search, and the list, get and count commands | Allow |
| Local-only writes | sync, fleet changes --rebaseline, auth set-token, auth logout - they write only to your machine's mirror and credential store | Allow (no tenant change) |
| Write (routine) | domains users create, domains users update, domains phonenumbers update, domains callqueues agents-create, phones create | Preview with --dry-run, then a reviewed write |
| Live calls and voicemail | domains users calls-create, calls-transfer, calls-delete, voicemails-delete - they act on a client's live calls and mailboxes | Human-in-the-loop only |
| Credentials and security | apikeys, jwt, tokens, auth-code, sso-enroll, certificates, firebase, subscriptions | Human-in-the-loop only |
| Destructive / config | every delete command, domains delete, restore update, backup, domains backup create | Human-in-the-loop only, explicit confirmation |

Most of what an MSP asks is read-only: the fleet audits, trace and search read the local mirror, and sync reads the API. The CLI also carries every NetSapiens v2 write, from user and number changes to live call control, API keys and domain restores. Over MCP every write previews first and sends only with confirm: true; on the CLI, preview with --dry-run. Allow reads; keep writes, credential commands and restores with a human. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/netsapiens/governance.md).

## Frequently asked questions

### Is there an MCP server for NetSapiens?

Yes - this one. A free, open source MCP server and Claude Code Skill for NetSapiens, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the NetSapiens MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `NETSAPIENS_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `NETSAPIENS_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the NetSapiens MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my NetSapiens data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### Does it work with Crexendo, OITVOIP and other white-label NetSapiens platforms?

Yes. Set NETSAPIENS_SERVER to your provider's platform host and use an API key or OAuth2 access token from that platform. Without NETSAPIENS_SERVER the CLI talks to ns-api.com, which is only the NetSapiens public sandbox.

### Do I need a reseller-level API key?

For the fleet audits, yes: a reseller-scoped key sees every client domain. An office-manager key sees one domain, so the fleet commands then cover only that client. A read-only key is enough for every audit.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## Status

Beta. Validated against the NetSapiens API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
