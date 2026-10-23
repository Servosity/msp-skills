---
layout: default
title: "Jira Service Management MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Jira Service Management Cloud request-API (servicedeskapi) and Assets endpoint in one binary, plus SLA breach radar, internal notes, ticket timelines, and CMDB-to-ticket joins."
permalink: /skills/jira-service-management/
skill_name: "Jira Service Management MCP"
image: /assets/social/jira-service-management/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Jira Service Management?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Jira Service Management, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Jira Service Management MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `JIRA_SERVICE_MANAGEMENT_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `JIRA_SERVICE_MANAGEMENT_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "How do I install the Jira Service Management MCP server?"
    a: "This is a free, open source MCP server for Jira Service Management Cloud. It runs on your own machine and connects to Claude Desktop, Claude Code, and other MCP-capable agents.\n\n1. Create an Atlassian API token at id.atlassian.com/manage-profile/security/api-tokens, signed in as an agent on your JSM site.\n2. **Claude Desktop:** download the Jira Service Management `.mcpb` from the [releases page](https://github.com/servosity/msp-skills/releases?q=jira-service-management), open **Claude Desktop > Settings > Extensions**, select the file, and enter your site subdomain, email and API token.\n3. **Claude Code:** run `/plugin marketplace add Servosity/msp-skills`, then `/plugin install jira-service-management@msp-skills`.\n4. **Any other agent, or a terminal:** run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.sh)` on macOS or Linux, or `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.ps1 | iex` in Windows PowerShell. Set `JIRA_SERVICE_MANAGEMENT_SITE` (acme for acme.atlassian.net), `JIRA_SERVICE_MANAGEMENT_EMAIL` and `JIRA_SERVICE_MANAGEMENT_API_TOKEN`, then run `jira-service-management-cli doctor`. To wire it into your agent, follow the [agent setup guide](https://github.com/Servosity/msp-skills/blob/main/skills/jira-service-management/mcp-install.md).\n5. Ask your agent: \"Which tickets breach SLA in the next two hours?\" That reads live from your queues, so no sync is needed first.\n\nAtlassian also runs an official, hosted Rovo MCP Server (its JSM request tools are listed in the [developer docs](https://developer.atlassian.com/cloud/rovo-mcp/guides/supported-tools/), its Assets tools in the [support docs](https://support.atlassian.com/atlassian-rovo-mcp-server/docs/supported-tools/)), and the community [mcp-atlassian](https://github.com/sooperset/mcp-atlassian) server covers Jira and Confluence broadly, including service desk queues and internal comments. This one runs locally, ranks open tickets across every queue by JSM's own SLA timers, keeps a local copy of your requests for period SLA reports and per-client views, and joins Assets (CMDB) objects to their tickets."
  - q: "Does this work with ChatGPT?"
    a: "Not with this build. ChatGPT connects only to remote MCP servers over HTTPS, which means running jira-service-management-mcp in HTTP mode, and in this build HTTP mode checks a token but has no request size or time limits yet. Do not run it or put it behind a tunnel until the connector is reprinted; the security note at the top of the install guide explains why. Until then, use Claude Desktop, Claude Code or Codex."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Jira Service Management data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Does this work with Jira Service Management Data Center or Server?"
    a: "No. It targets Jira Service Management Cloud (your-site.atlassian.net). Data Center and Server use different hosts and authentication, and this connector does not call them."
  - q: "Do I need JSM Premium for Assets (CMDB)?"
    a: "Only for the Assets commands. Assets is part of JSM Premium and Enterprise. Queues, SLAs, requests, comments, customers and organizations work on any JSM Cloud plan. For Assets, run `jira-service-management-cli cmdb discover --save` once per client site: it finds that site's cloudId and Assets workspaceId and stores them for that site only."
  - q: "What can the AI see and change?"
    a: "Exactly what the Atlassian account behind the API token can. An agent account sees its service desks, queues and internal notes; a customer-only account sees only its own requests. Create the token as an agent with the access you want the AI to have, and keep writes behind your agent's approval policy (see the safety model below)."
  - q: "Will an internal note ever reach the customer?"
    a: "Use `request note` for anything internal: it always posts with public=false, and if JSM reports the stored comment as public it exits with an error so the agent knows. `request comment add` is the customer-reply command: pass `--public` for a customer-visible reply. It sends no public field when you omit the flag, so never use it for internal notes. Read `request timeline` before replying: it labels every comment PUBLIC or INTERNAL."
  - q: "Will this hit my Atlassian API rate limits?"
    a: "Live commands make normal API calls and pace themselves to the rate-limit headers Atlassian returns (`--rate-limit` overrides it). `breaches` reads each queue and then each ticket's SLA, so scope it with `--desk` and `--queue` on large sites. `sla-report`, `client` and `search` read the local copy after a `sync` and make no API calls."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Jira Service Management credentials once, then run jira-service-management-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Jira Service Management question in plain language; it runs jira-service-management-cli for you."
---

# The Jira Service Management MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Atlassian Pty Ltd.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Jira Service Management. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Jira Service Management to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

If you run your service desk on Jira Service Management, ask your AI "which tickets breach SLA in the next two hours across every queue," "what happened on SD-123, internal notes included," or "which laptops in our RMM are missing from Assets," and get the answer in one step. It reads JSM's own SLA timers, keeps internal notes internal, and joins Assets (CMDB) objects to the tickets that touch them.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/jira-service-management){: .btn}

## Instead of clicking through Jira Service Management, just ask

**Instead of** Opening each queue every morning, sorting by SLA, and copying the red and amber tickets into a spreadsheet
**just ask:** *"Which tickets breach SLA in the next two hours, across all our queues?"*
<sub>Your agent runs: <code>jira-service-management-cli breaches --within 2h</code></sub>

**Instead of** Scrolling a long ticket to work out which comments the customer can see before you write a reply
**just ask:** *"Show me SD-123's full history, internal notes marked, before I answer the customer"*
<sub>Your agent runs: <code>jira-service-management-cli request timeline SD-123</code></sub>

**Instead of** Exporting your RMM device list and VLOOKUP-ing it against Assets to find machines nobody recorded
**just ask:** *"Which laptops are in our RMM export but missing from Assets?"*
<sub>Your agent runs: <code>jira-service-management-cli cmdb reconcile --type Laptop --file rmm.csv --key Name --compare Serial,OS</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/jira-service-management/wide-1200x630.png" src="/assets/video/jira-service-management/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/jira-service-management/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which tickets breach SLA in the next two hours, across every queue? | `jira-service-management-cli breaches --within 2h` |
| What happened on this ticket, internal notes included, before I reply? | `jira-service-management-cli request timeline SD-123` |
| Add a note for the other techs that the customer will never see | `jira-service-management-cli request note SD-123 "Escalated to network team"` |
| How are Acme Corp's open tickets doing before my client call? | `jira-service-management-cli client "Acme Corp"` |
| What was our SLA breach rate by request type over the last 30 days? | `jira-service-management-cli sla-report --since 30d --group-by requesttype` |
| Which client staff are in no organization, or in two? | `jira-service-management-cli roster 3` |
| Which tickets have touched this server? | `jira-service-management-cli cmdb tickets srv-01` |
| Which laptops are in our RMM export but missing from Assets? | `jira-service-management-cli cmdb reconcile --type Laptop --file rmm.csv --key Name --compare Serial,OS` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/jira-service-management/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/jira-service-management/guide.md).

## What makes this one different

Most Jira Service Management MCP servers expose the API one call at a time, so "what breaches next" becomes a queue walk plus one SLA call per ticket that the AI has to orchestrate and pay for in context. This connector does the walk itself ('breaches'), labels every comment public or internal in one timeline, refuses to post an internal note as public, and keeps a local SQLite copy of your requests so period SLA reports and per-client views are one local query. It also reaches Assets (CMDB): objects joined to their tickets, and an object type diffed against an RMM or PSA export.

Atlassian's Rovo MCP Server is the official option: hosted by Atlassian, it covers Jira and Confluence plus a growing set of JSM request, operations and Assets tools. This connector complements it rather than replacing it: it runs on your own machine with an API token, adds a local copy for period SLA reports and per-client views, and adds MSP-shaped commands (cross-queue SLA ranking, organization hygiene, Assets-to-ticket joins, Assets-vs-RMM reconcile) that are not single API calls.

## The pain this closes

- Internal notes leak to customers. Atlassian's own tracker carries the request (JSDCLOUD-17210): agents "don't know if they are making an internal or external facing comment and may accidentally have selected the wrong one." An AI agent posting through a generic comment call has the same problem, so 'request note' always posts public=false and errors if JSM stores the comment as public.
- No single API call answers "what breaches next" across queues. You list each queue, drop the tickets that sit in several, and read each ticket's SLA metrics. 'breaches' does that walk once and ranks open tickets by SLA time remaining, breached first.
- MSPs model each client as a JSM organization (Atlassian's guide to supporting multiple clients on one site), and keeping client staff in the right organization is manual. 'roster' flags customers in no organization or in two, empty organizations, and organizations holding a desk's customers that are not attached to that desk.
- Assets (CMDB) only knows what someone imported. Nothing in JSM compares it with your RMM or PSA inventory, so drift is found during an outage. 'cmdb reconcile' diffs an Assets object type against a CSV export, and 'cmdb tickets' lists every ticket connected to a device.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/jira-service-management/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/jira-service-management/install.ps1 | iex
```

After install, authenticate once with your Jira Service Management credentials, then verify with `jira-service-management-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | jira-service-management-cli breaches --within 2h; jira-service-management-cli request timeline SD-123; jira-service-management-cli servicedesk queue list-issues 3 10; jira-service-management-cli sla-report --since 30d; jira-service-management-cli roster 3; jira-service-management-cli cmdb tickets srv-01; jira-service-management-cli object find-by-aql | Allow |
| Write (routine) | jira-service-management-cli request note (internal note); jira-service-management-cli request comment add (reply, --public for customer-visible); jira-service-management-cli request transition perform; jira-service-management-cli request create; jira-service-management-cli request approval answer; jira-service-management-cli object create / update - writes send immediately; --dry-run is an opt-in preview, not a default | Preview with --dry-run, then a reviewed write |
| Customers and organizations | jira-service-management-cli customer create; jira-service-management-cli servicedesk customer add / invite / remove; jira-service-management-cli organization create; jira-service-management-cli organization user add / remove; jira-service-management-cli customer revoke-portal-only-access-for-user | Human-approved: these change who can raise tickets for a client |
| Destructive / config | jira-service-management-cli organization delete; jira-service-management-cli object delete; jira-service-management-cli objectschema delete; jira-service-management-cli objecttype delete; jira-service-management-cli servicedesk requesttype delete; jira-service-management-cli importsource token generate; jira-service-management-cli api-import | Human-in-the-loop only |

The skill drives jira-service-management-cli and jira-service-management-mcp with your Atlassian email and API token, sent only to your atlassian.net site and, for Assets, api.atlassian.com. Read commands (queues, SLA ranking, timelines, reports, Assets lookups) change nothing. Writes send immediately: --dry-run is an opt-in preview, so the recommended policy is preview, show the exact command, get approval, then run it. Keep deletes, customer and organization removals, and Assets schema changes human-only. The strongest control is the permissions of the Atlassian account that owns the token; setting JIRA_SERVICE_MANAGEMENT_MCP_READ_ONLY=1 also drops the MCP server's tools that write to your site or bulk-write local data, and makes its generic execute tool refuse non-GET API calls. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/jira-service-management/governance.md).

## Frequently asked questions

### Is there an MCP server for Jira Service Management?

Yes - this one. A free, open source MCP server and Claude Code Skill for Jira Service Management, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Jira Service Management MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `JIRA_SERVICE_MANAGEMENT_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `JIRA_SERVICE_MANAGEMENT_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### How do I install the Jira Service Management MCP server?

This is a free, open source MCP server for Jira Service Management Cloud. It runs on your own machine and connects to Claude Desktop, Claude Code, and other MCP-capable agents.

1. Create an Atlassian API token at id.atlassian.com/manage-profile/security/api-tokens, signed in as an agent on your JSM site.
2. **Claude Desktop:** download the Jira Service Management `.mcpb` from the [releases page](https://github.com/servosity/msp-skills/releases?q=jira-service-management), open **Claude Desktop > Settings > Extensions**, select the file, and enter your site subdomain, email and API token.
3. **Claude Code:** run `/plugin marketplace add Servosity/msp-skills`, then `/plugin install jira-service-management@msp-skills`.
4. **Any other agent, or a terminal:** run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.sh)` on macOS or Linux, or `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.ps1 | iex` in Windows PowerShell. Set `JIRA_SERVICE_MANAGEMENT_SITE` (acme for acme.atlassian.net), `JIRA_SERVICE_MANAGEMENT_EMAIL` and `JIRA_SERVICE_MANAGEMENT_API_TOKEN`, then run `jira-service-management-cli doctor`. To wire it into your agent, follow the [agent setup guide](https://github.com/Servosity/msp-skills/blob/main/skills/jira-service-management/mcp-install.md).
5. Ask your agent: "Which tickets breach SLA in the next two hours?" That reads live from your queues, so no sync is needed first.

Atlassian also runs an official, hosted Rovo MCP Server (its JSM request tools are listed in the [developer docs](https://developer.atlassian.com/cloud/rovo-mcp/guides/supported-tools/), its Assets tools in the [support docs](https://support.atlassian.com/atlassian-rovo-mcp-server/docs/supported-tools/)), and the community [mcp-atlassian](https://github.com/sooperset/mcp-atlassian) server covers Jira and Confluence broadly, including service desk queues and internal comments. This one runs locally, ranks open tickets across every queue by JSM's own SLA timers, keeps a local copy of your requests for period SLA reports and per-client views, and joins Assets (CMDB) objects to their tickets.

### Does this work with ChatGPT?

Not with this build. ChatGPT connects only to remote MCP servers over HTTPS, which means running jira-service-management-mcp in HTTP mode, and in this build HTTP mode checks a token but has no request size or time limits yet. Do not run it or put it behind a tunnel until the connector is reprinted; the security note at the top of the install guide explains why. Until then, use Claude Desktop, Claude Code or Codex.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Jira Service Management data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Does this work with Jira Service Management Data Center or Server?

No. It targets Jira Service Management Cloud (your-site.atlassian.net). Data Center and Server use different hosts and authentication, and this connector does not call them.

### Do I need JSM Premium for Assets (CMDB)?

Only for the Assets commands. Assets is part of JSM Premium and Enterprise. Queues, SLAs, requests, comments, customers and organizations work on any JSM Cloud plan. For Assets, run `jira-service-management-cli cmdb discover --save` once per client site: it finds that site's cloudId and Assets workspaceId and stores them for that site only.

### What can the AI see and change?

Exactly what the Atlassian account behind the API token can. An agent account sees its service desks, queues and internal notes; a customer-only account sees only its own requests. Create the token as an agent with the access you want the AI to have, and keep writes behind your agent's approval policy (see the safety model below).

### Will an internal note ever reach the customer?

Use `request note` for anything internal: it always posts with public=false, and if JSM reports the stored comment as public it exits with an error so the agent knows. `request comment add` is the customer-reply command: pass `--public` for a customer-visible reply. It sends no public field when you omit the flag, so never use it for internal notes. Read `request timeline` before replying: it labels every comment PUBLIC or INTERNAL.

### Will this hit my Atlassian API rate limits?

Live commands make normal API calls and pace themselves to the rate-limit headers Atlassian returns (`--rate-limit` overrides it). `breaches` reads each queue and then each ticket's SLA, so scope it with `--desk` and `--queue` on large sites. `sla-report`, `client` and `search` read the local copy after a `sync` and make no API calls.


## More PSA connectors

Run more than one PSA tool, or comparing options? These connectors work the same way: [Autotask PSA](/skills/autotask/) · [ConnectWise PSA (Manage)](/skills/connectwise-manage/) · [HaloPSA](/skills/halopsa/) · [Kaseya BMS](/skills/kaseya-bms/) · [SuperOps](/skills/superops/) · [Syncro](/skills/syncro/) · [Zammad](/skills/zammad/)

## Status

Beta. Validated against the Jira Service Management API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
