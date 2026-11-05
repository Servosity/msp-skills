---
layout: default
title: "Blackpoint Cyber MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Blackpoint Cyber CompassOne from the terminal: tenants, detections, assets, vulnerabilities, posture, reports and Cloud Response admin, plus a cross-tenant view of open detections, stale agents and coverage gaps."
permalink: /skills/blackpoint/
skill_name: "Blackpoint Cyber MCP"
image: /assets/social/blackpoint/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Blackpoint Cyber?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Blackpoint Cyber, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Blackpoint Cyber MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `BLACKPOINT_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `BLACKPOINT_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Blackpoint Cyber MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Blackpoint Cyber data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "What Blackpoint access do I need?"
    a: "A CompassOne API token, created in the CompassOne portal under API tokens. A partner token reaches every customer tenant; a tenant token sees one customer. A read-only partner token is enough for every read and rollup command. Some routes need entitlements: a 403 on vulnerabilities or users means the account is not entitled, and fleet-sync records that tenant's slice as failed instead of empty."
  - q: "Will this hit my Blackpoint API rate limits?"
    a: "The key quota is 2000 requests per 15 minutes. fleet-sync makes one or more calls per tenant and data kind, so on a large fleet limit it with --tenant and --kinds. After a sync, triage, gaps, stale-agents, cve-spread, leaderboard, geo-audit and overview read the local mirror and make no API calls; client-report and the endpoint commands call the API live."
  - q: "Will this replace the CompassOne portal?"
    a: "No. The portal stays your console for investigations, Cloud Response configuration and reports. This connector answers the questions that span customers, and gives your AI agent a safe, previewable path to the few changes the API allows."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Blackpoint Cyber credentials once, then run blackpoint-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Blackpoint Cyber question in plain language; it runs blackpoint-cli for you."
---

# The Blackpoint Cyber MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Blackpoint Cyber.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Blackpoint Cyber. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Blackpoint Cyber to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask "which open Blackpoint detections are oldest across all my clients?" and get one queue spanning every CompassOne tenant. fleet-sync pulls each tenant's detections, devices, vulnerabilities, scans, posture and Cloud Response settings into a local mirror, so your AI can rank stale agents, coverage gaps, CVE spread and posture across the whole book of customers in plain English.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/blackpoint){: .btn}

## Instead of clicking through Blackpoint Cyber, just ask

**Instead of** Switch into each customer tenant in the CompassOne portal to see which detections are still open and how long they have waited.
**just ask:** *"Show me every open Blackpoint detection across all my clients, oldest first."*
<sub>Your agent runs: <code>blackpoint-cli triage --older-than 24h</code></sub>

**Instead of** Open each tenant's asset list and sort by last check-in to find endpoints whose agent went quiet.
**just ask:** *"Which Blackpoint agents have not reported in two weeks?"*
<sub>Your agent runs: <code>blackpoint-cli stale-agents --days 14</code></sub>

**Instead of** Check every customer by hand for a recent scan, an escalation contact group and a posture rating.
**just ask:** *"Which customers have no recent scan or nobody to escalate to?"*
<sub>Your agent runs: <code>blackpoint-cli gaps --scan-days 30</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/blackpoint/wide-1200x630.png" src="/assets/video/blackpoint/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/blackpoint/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which open detections are oldest across every client? | `blackpoint-cli triage --older-than 24h` |
| Which devices' agents stopped reporting? | `blackpoint-cli stale-agents --days 14` |
| Which customers have no recent scan, no escalation contacts or no posture rating? | `blackpoint-cli gaps --scan-days 30` |
| Which exploited CVE affects the most customers? | `blackpoint-cli cve-spread --exploited` |
| Which tenants have the weakest posture score, and in which NIST function? | `blackpoint-cli leaderboard` |
| Which travel exceptions have expired or end this week? | `blackpoint-cli geo-audit --expiring 7d` |
| What does one client look like right now? | `blackpoint-cli overview "Acme Dental"` |
| What are this client's numbers for the monthly report? | `blackpoint-cli client-report "Acme Dental" --since 30d` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/blackpoint/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/blackpoint/guide.md).

## What makes this one different

This connector stores every customer tenant side by side in a local SQLite mirror, so cross-tenant questions (one detection queue, stale agents, coverage gaps, CVE spread, posture ranking) are one local query. Other Blackpoint MCP servers, such as WYRE-AI/blackpoint-mcp and MSPbotsAI/compassone-mcp, call the live API per question; that works for one tenant, while a fleet-wide view needs one call per tenant.

CompassOne is where the SOC works and where you configure Cloud Response. This connector does not replace it: it adds the rollups across customers that the per-tenant views do not pre-compute, readable by any AI agent that speaks MCP.

## The pain this closes

- CompassOne scopes detections, assets, vulnerabilities and scans to one customer tenant at a time: every one of those API calls needs that tenant's id. Across 30 or 80 customers there is no single queue that says which open detection anywhere has waited longest.
- Coverage decays quietly: an agent stops reporting, a scan stops completing, an escalation contact group is emptied. Nothing in the per-tenant views rolls those gaps up across customers before an incident finds them.
- Cloud Response travel exceptions carry end dates, and connection allow-lists grow country by country. Auditing them means opening every Microsoft 365, Cisco Duo and Google connection one by one.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/blackpoint/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/blackpoint/install.ps1 | iex
```

After install, authenticate once with your Blackpoint Cyber credentials, then verify with `blackpoint-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | triage, overview, client-report, gaps, stale-agents, cve-spread, leaderboard, geo-audit, fleet-sync, tenants list, detections list, vulnerabilities, posture, scans, search | Allow |
| Credential | darkweb-exposures (lists breached accounts; each password is replaced by [withheld]), cisco-onboardings create (takes a Duo secret key), auth set-token, auth logout | Human-in-the-loop only |
| Write (routine) | contact-groups create, contact-groups update, contact-groups add-member, contact-groups update-member, contact-groups assign-tenants, cisco-onboardings sync-users, cisco-onboardings complete, import | Preview with --dry-run, then a reviewed write |
| Access policy (Cloud Response) | ms365 approve-country, ms365 approve-user-country, ms365 remove-country, ms365 remove-user-country, cloud-connections approve-country, cloud-connections approve-user-country, cloud-connections remove-approved-country, cloud-connections remove-user-country | Human-in-the-loop only |
| Destructive | contact-groups delete, contact-groups delete-many, contact-groups delete-member, cisco-onboardings delete | Human-in-the-loop only, explicit confirmation |

The connector authenticates with your CompassOne API token and is read-first: every rollup, report and list command changes nothing. Writes (contact groups, Cloud Response approved countries, Cisco Duo onboardings) send as soon as you run them from the CLI unless you pass --dry-run to preview; over MCP every write only previews until the call sets confirm to true, and read-only mode (preset on in the Claude Desktop bundle) hides them. Keep autonomous agents to reads and previews, and require a human for any write, any approved-country change and any delete. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/blackpoint/governance.md).

## Frequently asked questions

### Is there an MCP server for Blackpoint Cyber?

Yes - this one. A free, open source MCP server and Claude Code Skill for Blackpoint Cyber, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Blackpoint Cyber MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `BLACKPOINT_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `BLACKPOINT_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Blackpoint Cyber MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Blackpoint Cyber data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What Blackpoint access do I need?

A CompassOne API token, created in the CompassOne portal under API tokens. A partner token reaches every customer tenant; a tenant token sees one customer. A read-only partner token is enough for every read and rollup command. Some routes need entitlements: a 403 on vulnerabilities or users means the account is not entitled, and fleet-sync records that tenant's slice as failed instead of empty.

### Will this hit my Blackpoint API rate limits?

The key quota is 2000 requests per 15 minutes. fleet-sync makes one or more calls per tenant and data kind, so on a large fleet limit it with --tenant and --kinds. After a sync, triage, gaps, stale-agents, cve-spread, leaderboard, geo-audit and overview read the local mirror and make no API calls; client-report and the endpoint commands call the API live.

### Will this replace the CompassOne portal?

No. The portal stays your console for investigations, Cloud Response configuration and reports. This connector answers the questions that span customers, and gives your AI agent a safe, previewable path to the few changes the API allows.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Bitdefender GravityZone](/skills/bitdefender/) · [Blumira](/skills/blumira/) · [CIPP + Microsoft 365](/skills/cipp/) · [Cisco Duo](/skills/cisco-duo/) · [ConnectSecure](/skills/connectsecure/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [DNSFilter](/skills/dnsfilter/) · [Exchange Online](/skills/exchange-online/) · [Fortinet](/skills/fortinet/) · [Google Workspace](/skills/google-workspace/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Mimecast](/skills/mimecast/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SaaS Alerts](/skills/saas-alerts/) · [SentinelOne](/skills/sentinelone/) · [SonicWall](/skills/sonicwall/) · [Sophos Central](/skills/sophos-central/) · [ThreatDown](/skills/threatdown/) · [ThreatLocker](/skills/threatlocker/) · [WatchGuard Cloud](/skills/watchguard/)

## Status

Beta. Validated against the Blackpoint Cyber API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
