---
layout: default
title: "IT Glue MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every IT Glue resource, plus an offline SQLite mirror, fleet-wide cross-resource search, and documentation-hygiene analytics no other IT Glue tool offers."
permalink: /skills/itglue/
skill_name: "IT Glue MCP"
image: /assets/social/itglue/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for IT Glue?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for IT Glue, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the IT Glue MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local IT Glue MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my IT Glue data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "What credentials do I need?"
    a: "An IT Glue API key, which you generate in your IT Glue account settings (Account > Settings > API Keys). Set ITGLUE_API_KEY in your environment, or run itglue-cli auth set-token. The key inherits the permissions of the IT Glue account it belongs to, so scope that account to exactly what you want the AI to reach - a key with password access lets passwords get/list return stored secrets, so scope it accordingly."
  - q: "Will this blow through my IT Glue API limits?"
    a: "IT Glue meters the API at roughly 3000 requests per 5 minutes. The local mirror exists so reads stop hitting the API: after the first sync, search, coverage, passwords stale, changes, contacts dupes, org show, and orphans all run against local SQLite with zero API calls, and sync is incremental - it only fetches what changed since the last checkpoint."
  - q: "Does it work with IT Glue's EU or AU data regions?"
    a: "Yes. IT Glue hosts data in regional API endpoints; set ITGLUE_BASE_URL to your region's API URL and the CLI uses it. The default targets the US endpoint."
  - q: "Can it delete my documentation?"
    a: "No. The CLI reads, and creates or updates contacts, passwords, configurations, and documents - it exposes no delete for any IT Glue resource. The coverage and stale-password audits read metadata only; password secret values are never read or printed."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/itglue/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/itglue/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your IT Glue credentials once; itglue-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a IT Glue question in plain language; it runs itglue-cli for you."
---

# The IT Glue MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Kaseya US LLC.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for IT Glue. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects IT Glue to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

MSPs keep their whole client world in IT Glue - organizations, contacts, passwords, configurations, documents - but the API answers one record at a time under a 3000-request rate ceiling. Ask your AI "which clients are under-documented," "which credentials haven't rotated in a year," or "which client owns this serial number," and get fleet-wide answers from a local mirror in one query - no portal clicking, no rate-limit math.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/itglue){: .btn}

## Instead of clicking through IT Glue, just ask

**Instead of** Clicking into each client in the IT Glue portal one at a time to find which one owns a device when a ticket only gives you a serial number or a hostname
**just ask:** *"Which client does this Fortinet firewall belong to?"*
<sub>Your agent runs: <code>itglue-cli search "Fortinet"</code></sub>

**Instead of** Pulling every client's passwords in the portal and eyeballing last-changed dates to build a credential-rotation list for the cyber-insurance audit
**just ask:** *"Which credentials haven't been rotated in over a year, by client?"*
<sub>Your agent runs: <code>itglue-cli passwords stale --days 365</code></sub>

**Instead of** Opening each organization in turn to judge which clients are missing runbooks, contacts, or documented configurations before a quarterly review
**just ask:** *"Which clients are under-documented right now?"*
<sub>Your agent runs: <code>itglue-cli coverage --below 1</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/itglue/wide-1200x630.png" src="/assets/video/itglue/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/itglue/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which client owns this device, serial number, or contact? | `itglue-cli search "Fortinet"` |
| Which clients are under-documented, thinnest first? | `itglue-cli coverage --below 1` |
| Which credentials haven't been rotated in a year, grouped by client? | `itglue-cli passwords stale --days 365` |
| What changed across every client since a given date? | `itglue-cli changes --since 2026-05-01` |
| Which contacts are duplicated across or within clients? | `itglue-cli contacts dupes` |
| Everything we know about one client, in a single offline read? | `itglue-cli org show "12345"` |
| Which records are orphaned after a client was offboarded? | `itglue-cli orphans` |
| List every organization in the account. | `itglue-cli organizations list` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/itglue/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/itglue/guide.md).

## What makes this one different

Most IT Glue integrations and MCP servers proxy each question into a live API call - fine for one record, but every fleet-wide question becomes a multi-call dance against IT Glue's 3000-requests / 5-minute rate ceiling, one resource and one organization at a time. This skill syncs IT Glue into a local SQLite mirror with full-text search, so cross-client questions - search, coverage, stale-password audits, duplicate and orphan detection - become one local join: instant, offline, and the AI sees the answer, not pages of raw JSON. Reads run against the mirror, so they stop spending your API rate budget.

It complements IT Glue's portal and its built-in AI features rather than replacing them: the portal stays best for editing and day-to-day documentation, while this skill brings IT Glue to whichever AI agent you already use and answers the cross-client questions - completeness ranking, fleet-wide credential rotation, orphan and duplicate detection - that no single IT Glue screen composes without exporting and joining the data yourself.

## The pain this closes

- Documentation completeness is invisible until it bites: IT Glue shows one organization at a time, with no fleet-wide view of which clients are missing configurations, contacts, passwords, or documents - so gaps surface during an incident or a QBR, not before. MSPs on r/msp repeatedly describe keeping IT Glue current as a discipline problem with no built-in scorecard.
- Credential rotation is an audit scramble: SOC2 and cyber-insurance questionnaires ask when privileged credentials were last rotated, but IT Glue has no single screen that lists stale passwords across every client - and a live fleet-wide scan runs straight into IT Glue's documented 3000-requests / 5-minute API rate ceiling.
- Overlapping syncs leave a mess: when IT Glue ingests contacts from a PSA, the same person often lands twice, and after a client is offboarded their configurations and documents linger as orphans pointing at an organization that no longer exists. The API exposes no endpoint that surfaces either one.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/itglue/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/itglue/install.ps1 | iex
```

After install, authenticate once with your IT Glue credentials, then verify with `itglue-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | itglue-cli search "Fortinet"; itglue-cli coverage --below 1; itglue-cli passwords stale --days 365; itglue-cli changes --since 7d; itglue-cli contacts dupes; itglue-cli org show "12345"; itglue-cli orphans; itglue-cli organizations list | Allow |
| Write (routine) | itglue-cli configurations create; itglue-cli configurations update; itglue-cli contacts create; itglue-cli contacts update; itglue-cli documents create; itglue-cli documents update; itglue-cli organizations relationships create-organization-contact - writes send immediately; --dry-run is an opt-in preview, not a default | Preview with --dry-run, then a reviewed write |
| Destructive / config | itglue-cli passwords get and itglue-cli passwords list (can return the stored secret when the API key has password access); itglue-cli passwords create; itglue-cli passwords update - credential reads and writes, the most sensitive operations; the CLI exposes no delete for any IT Glue resource | Human-in-the-loop only |

The skill drives the itglue-cli and itglue-mcp binaries, authenticating with an IT Glue API key read from the environment (ITGLUE_API_KEY) - never logged and never sent anywhere except the IT Glue API. Most read commands (search, coverage, changes, contacts dupes, org show, orphans, and the organization/configuration/contact/document list and get) change nothing and run against the local mirror. The exceptions are passwords get and passwords list: when the API key has password access they can return the stored secret, so treat them as credential-tier, not routine reads (passwords stale is metadata-only and stays safe). Writes are not gated by default: --dry-run is an opt-in preview flag, so the recommended policy is an agent-level rule - preview with --dry-run, show the exact command, get approval, then run the write. The CLI exposes no delete for any IT Glue resource; the most sensitive operations are credential reads and writes (passwords get/list/create/update), which should stay human-approved. The real permission boundary is the scope of the IT Glue account whose key you use. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/itglue/governance.md).

## Frequently asked questions

### Is there an MCP server for IT Glue?

Yes - this one. A free, open source MCP server and Claude Code Skill for IT Glue, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the IT Glue MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local IT Glue MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my IT Glue data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What credentials do I need?

An IT Glue API key, which you generate in your IT Glue account settings (Account > Settings > API Keys). Set ITGLUE_API_KEY in your environment, or run itglue-cli auth set-token. The key inherits the permissions of the IT Glue account it belongs to, so scope that account to exactly what you want the AI to reach - a key with password access lets passwords get/list return stored secrets, so scope it accordingly.

### Will this blow through my IT Glue API limits?

IT Glue meters the API at roughly 3000 requests per 5 minutes. The local mirror exists so reads stop hitting the API: after the first sync, search, coverage, passwords stale, changes, contacts dupes, org show, and orphans all run against local SQLite with zero API calls, and sync is incremental - it only fetches what changed since the last checkpoint.

### Does it work with IT Glue's EU or AU data regions?

Yes. IT Glue hosts data in regional API endpoints; set ITGLUE_BASE_URL to your region's API URL and the CLI uses it. The default targets the US endpoint.

### Can it delete my documentation?

No. The CLI reads, and creates or updates contacts, passwords, configurations, and documents - it exposes no delete for any IT Glue resource. The coverage and stale-password audits read metadata only; password secret values are never read or printed.


## More Documentation connectors

Run more than one Documentation tool, or comparing options? These connectors work the same way: [Hudu](/skills/hudu/) · [Liongard](/skills/liongard/) · [PandaDoc](/skills/pandadoc/)

## Status

Beta. Validated against the IT Glue API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
