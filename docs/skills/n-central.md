---
layout: default
title: "N-able N-central MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every N-central REST endpoint, plus an offline SQLite mirror of your whole org tree, cross-tenant search, issue-triage rollups, and a JWT-expiry guardian no other N-central tool has."
permalink: /skills/n-central/
skill_name: "N-able N-central MCP"
image: /assets/social/n-central/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for N-able N-central?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for N-able N-central, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the N-able N-central MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local N-central MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my N-central data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "What credentials do I need?"
    a: "A JSON Web Token for an API-only user: in N-central, go to Administration > User Management > Users > select the API user > API Access > Generate JSON Web Token. MFA must be off on that user, and the user's password expiry (90 days by default) silently invalidates the JWT - guardian tracks that countdown. Set NCENTRAL_JWT and N_CENTRAL_BASE_URL (e.g. https://yourmsp.ncod.n-able.com/api)."
  - q: "Can this make changes in N-central?"
    a: "Almost everything is read-only. Two commands can change things: scheduled-tasks run executes an API-enabled Automation Policy or Script on a device, and import POSTs records from a JSONL file. --dry-run is an opt-in preview, not a default - the recommended agent policy is preview first, a human approves, then run. Keep scheduled-tasks run human-in-the-loop."
  - q: "We run more than one N-central server - does this handle that?"
    a: "Yes. Each server syncs to its own local mirror, and fanout unions them: one query returns matches across every server's mirror, each row tagged with the server it came from."
  - q: "Will this hit N-central API rate limits?"
    a: "N-able documents per-endpoint concurrency caps - 429 responses beyond roughly 3 to 50 concurrent calls depending on the endpoint. The CLI ships a --rate-limit throttle, and the heaviest questions (whereis, fanout, search) run against the local mirror with zero API calls."
  - q: "Does this work with N-able N-sight?"
    a: "No. This skill targets the N-central REST API specifically. N-sight is a separate N-able product with a separate API."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/n-central/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/n-central/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your N-able N-central credentials once; n-central-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a N-able N-central question in plain language; it runs n-central-cli for you."
---

# The N-able N-central MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by N-able, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for N-able N-central. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects N-able N-central to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

N-central knows every device you manage - and answering "where is that machine" still means walking the console's org tree. Ask your AI "what's red right now", "where is EXCHANGE01", or "which devices have no maintenance window before Saturday's patch wave" and get rollups the console can't compose: cross-tenant search from an offline mirror, severity-ranked triage, coverage audits, and a guardian that catches the JWT before it silently dies.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/n-central){: .btn}

## Instead of clicking through N-able N-central, just ask

**Instead of** Walking the console org tree customer by customer to find which site a machine lives under
**just ask:** *"Where is EXCHANGE01?"*
<sub>Your agent runs: <code>n-central-cli whereis EXCHANGE01</code></sub>

**Instead of** Opening every customer's active-issues view one at a time for the morning NOC sweep
**just ask:** *"What's red right now, worst first, by customer?"*
<sub>Your agent runs: <code>n-central-cli triage --by customer</code></sub>

**Instead of** Finding out the integration died because the API user's 90-day password expiry silently invalidated the JWT
**just ask:** *"Is our N-central API access healthy, and when does it expire?"*
<sub>Your agent runs: <code>n-central-cli guardian --password-set 2026-03-01</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/n-central/wide-1200x630.png" src="/assets/video/n-central/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/n-central/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What's red right now, grouped by customer and ranked by severity? | `n-central-cli triage --by customer` |
| Where is EXCHANGE01 - server, service org, customer, site? | `n-central-cli whereis EXCHANGE01` |
| Find anything named acme across every server we run | `n-central-cli fanout "acme"` |
| Which devices are missing the Backup Plan custom property, by customer? | `n-central-cli props audit --required "Backup Plan"` |
| Which devices have no maintenance window before the June 15 patch wave? | `n-central-cli maint coverage --before 2026-06-15` |
| Is the JWT healthy, and when does the API user's password kill it? | `n-central-cli guardian --password-set 2026-03-01` |
| Hardware and software inventory for one device | `n-central-cli devices assets 987654321` |
| Every device, exported for the QBR or your documentation tool | `n-central-cli export "devices" --format jsonl` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/n-central/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/n-central/guide.md).

## What makes this one different

Most N-central integrations proxy each question into live REST calls - which means org-tree pagination, per-endpoint concurrency caps, and a JWT that dies silently on password rotation. This skill syncs the org tree (service orgs, customers, sites, devices) into a local SQLite mirror: whereis and fanout answer from it instantly and offline, across as many N-central servers as you run; triage, props audit, and maint coverage sweep live data customer by customer without you scripting the pagination; and guardian turns the silent auth failure mode into a CI-wireable warning.

It complements the N-central console rather than replacing it: the console stays best for remote control and policy configuration, while this skill brings the org tree to whichever agent you already use and answers the cross-tenant rollups - where is this device, what's red everywhere, who has no maintenance window - that no single console screen composes.

## The pain this closes

- The morning NOC sweep is a per-customer console walk: active issues live under each org unit, so "what's red right now across everyone" means opening each customer's view - and at multi-server MSPs, repeating it in every console. N-able's own REST API documentation caps concurrency per endpoint (429 responses beyond 3-50 concurrent calls), so naive scripted sweeps throttle.
- The API user's password expiry - 90 days by default - silently invalidates the JWT, and the integration just stops. The community-maintained NC-API-Documentation project on GitHub documents the trap: an expired password for the originating account blocks JWT access, and regenerating the token invalidates the previous one.
- N-central sometimes returns an error message inside an HTTP 200 response - listed in N-able's official REST API known issues and limitations - so a script that only checks status codes happily processes an error body as data.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/n-central/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/n-central/install.ps1 | iex
```

After install, authenticate once with your N-able N-central credentials, then verify with `n-central-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | n-central-cli triage --by customer; n-central-cli whereis EXCHANGE01; n-central-cli fanout "acme"; n-central-cli devices list; n-central-cli props audit --required "Backup Plan"; n-central-cli maint coverage; n-central-cli guardian; n-central-cli export devices | Allow |
| Write (routine) | n-central-cli import <resource> --input data.jsonl - POSTs each record to the API; --dry-run is an opt-in preview, not a default | Preview with --dry-run, then a reviewed write |
| Destructive / high impact | n-central-cli scheduled-tasks run --task-type Script --device-id <id> --item-id <id> (executes on a live endpoint); n-central-cli customers registration-token / org-units registration-token (device-enrollment credentials); n-central-cli auth set-token / auth logout | Human-in-the-loop only |

The skill drives the n-central-cli and n-central-mcp binaries, authenticating with an API-only user's JWT (NCENTRAL_JWT) exchanged for short-lived access tokens - never logged and never sent anywhere except your N-central server. Nearly all commands are read-only; the exceptions are scheduled-tasks run (executes an API-enabled script or automation policy on a live device) and import (POSTs records), both previewable with the opt-in --dry-run flag. Registration tokens enroll new devices - treat them like credentials. The API user's role in N-central is the real permission boundary. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/n-central/governance.md).

## Frequently asked questions

### Is there an MCP server for N-able N-central?

Yes - this one. A free, open source MCP server and Claude Code Skill for N-able N-central, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the N-able N-central MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local N-central MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my N-central data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What credentials do I need?

A JSON Web Token for an API-only user: in N-central, go to Administration > User Management > Users > select the API user > API Access > Generate JSON Web Token. MFA must be off on that user, and the user's password expiry (90 days by default) silently invalidates the JWT - guardian tracks that countdown. Set NCENTRAL_JWT and N_CENTRAL_BASE_URL (e.g. https://yourmsp.ncod.n-able.com/api).

### Can this make changes in N-central?

Almost everything is read-only. Two commands can change things: scheduled-tasks run executes an API-enabled Automation Policy or Script on a device, and import POSTs records from a JSONL file. --dry-run is an opt-in preview, not a default - the recommended agent policy is preview first, a human approves, then run. Keep scheduled-tasks run human-in-the-loop.

### We run more than one N-central server - does this handle that?

Yes. Each server syncs to its own local mirror, and fanout unions them: one query returns matches across every server's mirror, each row tagged with the server it came from.

### Will this hit N-central API rate limits?

N-able documents per-endpoint concurrency caps - 429 responses beyond roughly 3 to 50 concurrent calls depending on the endpoint. The CLI ships a --rate-limit throttle, and the heaviest questions (whereis, fanout, search) run against the local mirror with zero API calls.

### Does this work with N-able N-sight?

No. This skill targets the N-central REST API specifically. N-sight is a separate N-able product with a separate API.


## More RMM connectors

Run more than one RMM tool, or comparing options? These connectors work the same way: [Action1](/skills/action1/) · [Atera](/skills/atera/) · [ConnectWise Automate](/skills/connectwise-automate/) · [Datto RMM](/skills/datto-rmm/) · [Level](/skills/levelio/) · [Nerdio Manager](/skills/nerdio/) · [NinjaOne](/skills/ninjaone/) · [Tactical RMM](/skills/tactical-rmm/)

## Status

Beta. Validated against the N-able N-central API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
