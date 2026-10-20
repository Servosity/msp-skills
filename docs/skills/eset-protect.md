---
layout: default
title: "ESET PROTECT MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every ESET Connect endpoint in one binary, plus per-customer health, exposure and QBR reports built from a local mirror of your MSP tree."
permalink: /skills/eset-protect/
skill_name: "ESET PROTECT MCP"
image: /assets/social/eset-protect/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for ESET PROTECT?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for ESET PROTECT, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the ESET PROTECT MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `ESET_PROTECT_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `ESET_PROTECT_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. The API user's password stays in your environment and is never written to disk, but the access and refresh tokens the CLI mints are cached in its credentials.toml (owner-only permissions), so removing the environment variables does not revoke access - run `eset-protect-cli auth logout` on every machine that cached them, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you run eset-protect-mcp --transport http with ESET_PROTECT_MCP_HTTP_TOKEN set (it refuses to start without one) and put it behind an HTTPS endpoint you control; no bridge package is needed. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my ESET data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "What ESET account do I need?"
    a: "A dedicated ESET Connect API user. A Superuser or Root creates it in ESET PROTECT Hub, ESET Business Account or ESET MSP Administrator 2 and turns on the Integrations toggle; the Superuser or Root account itself cannot be given API access. The new user accepts the invitation email and finishes account setup, and a Hub user must also sign in to ESET PROTECT Hub once. Set ESET_PROTECT_USERNAME and ESET_PROTECT_PASSWORD; the CLI exchanges them for a one-hour token."
  - q: "Which region should I set?"
    a: "Set ESET_PROTECT_REGION to the region of your ESET PROTECT console: eu (the default), de, us, ca or jpn. The jpn region has no quarantine or Cloud Office user API, and no v2 detection list."
  - q: "Will this hit ESET Connect rate limits?"
    a: "ESET Connect allows 10 calls per second for each set of credentials, account and source IP, and answers 429 above that; under its Fair Use Policy a very large burst is slowed to 2 requests per second and the rest is rejected with 50x. The CLI slows down and retries when it sees a 429; set --rate-limit 8 to stay under the limit from the start and keep sync --concurrency low. Slow requests answered with 202 and a response id are polled until --timeout runs out (60 seconds by default). ESET answers 504 to a query that is not ready within 120 seconds, so pass --timeout 3m for large queries and narrow the query if ESET still answers 504."
  - q: "Can the AI isolate a machine or change a policy by itself?"
    a: "The respond commands only print the task unless you add --yes, and over MCP they only preview. Other write commands run when invoked unless you pass --dry-run, and the generic eset-protect_execute MCP tool can call any ESET Connect endpoint. For a read-only agent, set ESET_PROTECT_MCP_READ_ONLY=1 in the MCP server's environment: no tool that changes ESET is offered and installer and enrollment links are refused. Keep your AI app's approval prompt on."
  - q: "Does it work with on-premises ESET PROTECT?"
    a: "No. It signs in through the ESET Connect cloud API, so it works with ESET PROTECT (cloud). An on-premises ESET PROTECT server uses a different API and sign-in, which this connector does not support."
  - q: "Does it replace the ESET PROTECT console?"
    a: "No. The console stays the place for policy editing, investigations in ESET Inspect and day-to-day administration. This skill answers the across-every-customer questions and runs previewed actions from your AI agent."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/eset-protect/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your ESET PROTECT credentials once, then run eset-protect-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent an ESET PROTECT question in plain language; it runs eset-protect-cli for you."
---

# The ESET PROTECT MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by ESET, spol. s r.o.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for ESET PROTECT. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects ESET PROTECT to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI which customers have ESET devices that need attention, which machines stopped checking in, who is exposed to a new CVE, and what the morning detection queue looks like, across every customer at once. The CLI syncs your ESET PROTECT tree into a local SQLite mirror, rolls it up per customer, and shows the exact scan, isolate or release task before it is sent.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/eset-protect){: .btn}

## Instead of clicking through ESET PROTECT, just ask

**Instead of** Reading the Managed Customers page, then drilling into each customer to find the devices that need attention, are muted or stopped checking in
**just ask:** *"Which of my customers have ESET devices that need attention or stopped checking in?"*
<sub>Your agent runs: <code>eset-protect-cli fleet health</code></sub>

**Instead of** Searching the vulnerability view for a new CVE, then working out which customer each affected device belongs to
**just ask:** *"Which customers and devices are exposed to CVE-2024-3400, and is there a patch?"*
<sub>Your agent runs: <code>eset-protect-cli exposure --cve CVE-2024-3400</code></sub>

**Instead of** Pulling one customer's device, detection, incident and patch numbers into a QBR deck by hand
**just ask:** *"Build Acme Dental's quarterly security summary from ESET."*
<sub>Your agent runs: <code>eset-protect-cli report customer "Acme Dental" --since 90d</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/eset-protect/wide-1200x630.png" src="/assets/video/eset-protect/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/eset-protect/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which customers have devices that need attention, are muted or stopped checking in? | `eset-protect-cli fleet health` |
| What should my techs work first this morning, detections and open incidents together? | `eset-protect-cli triage --since 24h --min-severity medium` |
| Which customers and devices are exposed to this CVE, and is a patch available? | `eset-protect-cli exposure --cve CVE-2024-3400` |
| Give me one customer's quarter: device health, detections, incidents, top CVEs and patches | `eset-protect-cli report customer "Acme Dental" --since 90d` |
| Which devices have not checked in for two weeks, most stale first? | `eset-protect-cli fleet stale --days 14` |
| Which security patches are still missing, ranked by the devices and CVE risk they fix? | `eset-protect-cli patches backlog --security-only` |
| Is the same file hash showing up at more than one customer this week? | `eset-protect-cli detections spread --since 7d --min-customers 2` |
| Isolate this machine from the network (show me the exact task first) | `eset-protect-cli respond isolate "FRONTDESK-01"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/eset-protect/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/eset-protect/guide.md).

## What makes this one different

MCP servers such as [maciekaz/ESET-MCP](https://github.com/maciekaz/ESET-MCP) and [Fenrindale/eset-protect-mcp](https://github.com/Fenrindale/eset-protect-mcp) expose the ESET Connect endpoints as tools and answer each question with live calls. This connector also covers the endpoints, and adds a local SQLite mirror of your ESET PROTECT tree, so per-customer rollups such as fleet health, the triage queue and the QBR one-pager are joins over synced devices, groups, detections and incidents, attributed to the customer each device belongs to.

ESET's own [ESET LiveAI](https://www.eset.com/us/business/solutions/liveai/) (formerly AI Advisor) is a generative AI assistant inside ESET PROTECT and ESET Inspect, in the ESET PROTECT Premium and Ultimate tiers. This skill complements it: it answers across-every-customer questions in whichever AI agent you already use, through the ESET Connect API, and it does not replace the ESET PROTECT console.

## The pain this closes

- The ESET Connect API returns devices, detections and incidents by device group, with no field that names the customer, so every per-customer answer an AI agent gives means walking the group tree first.
- ESET PROTECT's Managed Customers page shows per-customer device and detection counts; turning that into a worst-first list of stale and muted devices, a CVE exposure list or a QBR page is still manual work.
- Detections are split across API versions (v1 for ESET PROTECT and ESET Inspect, the v2 list for ESET Cloud Office Security) and incidents live in another service, so the API has no single morning queue.
- The API mints one-hour tokens with a password grant and answers slow requests with 202 and a response id to poll, which breaks simple scripts and some SIEM integrations.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/eset-protect/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/eset-protect/install.ps1 | iex
```

After install, authenticate once with your ESET PROTECT credentials, then verify with `eset-protect-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet health, triage, exposure, report customer, patches backlog, devices list, incidents list, detections list | Allow |
| Endpoint response | respond scan, respond isolate, respond release (preview unless --yes), device-tasks create, device-tasks update-targets | Human-in-the-loop, explicit confirmation |
| Protection and policy config | executables block, edr-rules create, edr-rule-exclusions create, policy-assignments assign-policy, devices move | Preview with --dry-run, then a human-approved write |
| Write (routine) | incidents close, incidents reopen, detection-groups resolve, devices rename, groups create | Preview with --dry-run, then a reviewed write |
| Credential and access | installers list, gpo-sccm-files-generate, mobile-devices-batch-get-enrollment-links, roles create, role-assignments-assign-role | Human-in-the-loop only |
| Destructive / high-risk | groups delete, policies delete, edr-rules delete, quarantined-objects-restore, quarantined-objects-purge, import | Human-in-the-loop only, explicit confirmation |

The skill signs in as the ESET Connect API user you create, so that user's access rights are the real boundary. Reads, reports and the local mirror are safe to allow. Response actions (scan, isolate, release), policy and EDR rule changes, quarantine restores and deletes reach real customer machines and need a human to approve the exact request. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/eset-protect/governance.md).

## Frequently asked questions

### Is there an MCP server for ESET PROTECT?

Yes - this one. A free, open source MCP server and Claude Code Skill for ESET PROTECT, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the ESET PROTECT MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `ESET_PROTECT_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `ESET_PROTECT_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. The API user's password stays in your environment and is never written to disk, but the access and refresh tokens the CLI mints are cached in its credentials.toml (owner-only permissions), so removing the environment variables does not revoke access - run `eset-protect-cli auth logout` on every machine that cached them, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you run eset-protect-mcp --transport http with ESET_PROTECT_MCP_HTTP_TOKEN set (it refuses to start without one) and put it behind an HTTPS endpoint you control; no bridge package is needed. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my ESET data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What ESET account do I need?

A dedicated ESET Connect API user. A Superuser or Root creates it in ESET PROTECT Hub, ESET Business Account or ESET MSP Administrator 2 and turns on the Integrations toggle; the Superuser or Root account itself cannot be given API access. The new user accepts the invitation email and finishes account setup, and a Hub user must also sign in to ESET PROTECT Hub once. Set ESET_PROTECT_USERNAME and ESET_PROTECT_PASSWORD; the CLI exchanges them for a one-hour token.

### Which region should I set?

Set ESET_PROTECT_REGION to the region of your ESET PROTECT console: eu (the default), de, us, ca or jpn. The jpn region has no quarantine or Cloud Office user API, and no v2 detection list.

### Will this hit ESET Connect rate limits?

ESET Connect allows 10 calls per second for each set of credentials, account and source IP, and answers 429 above that; under its Fair Use Policy a very large burst is slowed to 2 requests per second and the rest is rejected with 50x. The CLI slows down and retries when it sees a 429; set --rate-limit 8 to stay under the limit from the start and keep sync --concurrency low. Slow requests answered with 202 and a response id are polled until --timeout runs out (60 seconds by default). ESET answers 504 to a query that is not ready within 120 seconds, so pass --timeout 3m for large queries and narrow the query if ESET still answers 504.

### Can the AI isolate a machine or change a policy by itself?

The respond commands only print the task unless you add --yes, and over MCP they only preview. Other write commands run when invoked unless you pass --dry-run, and the generic eset-protect_execute MCP tool can call any ESET Connect endpoint. For a read-only agent, set ESET_PROTECT_MCP_READ_ONLY=1 in the MCP server's environment: no tool that changes ESET is offered and installer and enrollment links are refused. Keep your AI app's approval prompt on.

### Does it work with on-premises ESET PROTECT?

No. It signs in through the ESET Connect cloud API, so it works with ESET PROTECT (cloud). An on-premises ESET PROTECT server uses a different API and sign-in, which this connector does not support.

### Does it replace the ESET PROTECT console?

No. The console stays the place for policy editing, investigations in ESET Inspect and day-to-day administration. This skill answers the across-every-customer questions and runs previewed actions from your AI agent.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Bitdefender GravityZone](/skills/bitdefender/) · [Blumira](/skills/blumira/) · [CIPP + Microsoft 365](/skills/cipp/) · [Cisco Duo](/skills/cisco-duo/) · [ConnectSecure](/skills/connectsecure/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Fortinet](/skills/fortinet/) · [Google Workspace](/skills/google-workspace/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [SonicWall](/skills/sonicwall/) · [Sophos Central](/skills/sophos-central/) · [ThreatDown](/skills/threatdown/) · [ThreatLocker](/skills/threatlocker/) · [WatchGuard Cloud](/skills/watchguard/)

## Status

Beta. Validated against the ESET PROTECT API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
