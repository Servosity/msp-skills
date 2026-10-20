---
layout: default
title: "Sophos Central MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "The Sophos Central partner, organization and tenant APIs, routed to the right tenant and region automatically, plus a cross-tenant triage queue, fleet tamper remediation, a multi-tenant SIEM collector and hostname-based containment."
permalink: /skills/sophos-central/
skill_name: "Sophos Central MCP"
image: /assets/social/sophos-central/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Sophos Central?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Sophos Central, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Sophos Central MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SOPHOS_CENTRAL_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `SOPHOS_CENTRAL_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. The client secret stays in your environment (it is written to disk only if you pass auth login --client-secret), and the short-lived access token minted from it is cached in a permission-restricted credentials file on your machine until it expires or you run auth logout, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and sophos-central-mcp speaks HTTP natively: run `sophos-central-mcp --transport http` with SOPHOS_CENTRAL_MCP_HTTP_TOKEN set (it serves `http://127.0.0.1:7777/mcp` and requires that token as a bearer header) and put that endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Sophos Central data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "What Sophos Central API credential do I need?"
    a: "An API credential (a service principal) created in Sophos Central: in the Partner Dashboard under Settings & Policies > API Credentials for a partner, or under Global Settings > API Credentials Management for an organization or a single customer. Set SOPHOS_CENTRAL_CLIENT_ID and SOPHOS_CENTRAL_CLIENT_SECRET. The role you give it is the real permission boundary: Service Principal Read-Only covers every report here but cannot run Live Discover queries; writes such as isolation, scans and tamper protection need a role that allows them."
  - q: "Does it work with a partner, organization and single-tenant credential?"
    a: "Yes. whoami tells the CLI which kind of credential it holds. A partner or organization credential walks every customer tenant in the fleet commands, and picks one customer with --tenant (or SOPHOS_CENTRAL_TENANT_ID) for tenant-scoped commands such as endpoints or alerts. A single-tenant credential works on its own tenant."
  - q: "Will this hit Sophos Central API rate limits?"
    a: "Calls go through a shared throttle of 1.5 requests per second by default (--rate-limit), under Sophos's own limits. fleet refresh --plan prints the tenant count and estimated calls and time before you run it. After a refresh, fleet triage, fleet health and fleet gaps read the local store and make no API calls."
  - q: "Why do some customers show as forbidden?"
    a: "Sophos returns 403 to a partner for trial tenants and for term customers without Partner Assistance. fleet refresh records those in its per-tenant ledger and carries on, and fleet triage lists them as blind tenants so you can see what was not read. Trial tenants are skipped unless you pass --include-trial. A full refresh (no --tenant) also removes what earlier fleet refreshes stored for tenants it no longer covers (offboarded tenants, and trial tenants when --include-trial is not set), so fleet reports always match your last full refresh."
  - q: "Can the AI isolate a machine or turn tamper protection back on by itself?"
    a: "Only if you let it. contain and fleet tamper print a plan and change nothing without --yes, and the MCP server never passes --yes, so those two only preview over MCP. But the MCP server's generic sophos-central_execute tool can call any Sophos Central endpoint except the six that return a password or token, including isolation, scans and policy changes, gated only by your AI app's approval prompt. If an agent should only read, set SOPHOS_CENTRAL_MCP_READ_ONLY=1 in the MCP server's environment: it refuses every non-GET call and hides the sync and export tools. Keep the approval prompt on, and give the API credential a read-only role."
  - q: "Does it cover Sophos XDR or MDR Cases and Detections?"
    a: "Not in this version. It covers the REST APIs: alerts, endpoints, policies, exclusions, Live Discover, XDR Data Lake queries, SIEM events and alerts, Account Health Check, licences and partner billing. Cases and Detections are not included."
  - q: "Does it replace the Sophos Central Partner Dashboard?"
    a: "No. The dashboard stays best for policy authoring, investigations and day-to-day console work. This skill answers the across-every-customer questions and runs scripted actions from your AI agent."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Sophos Central credentials once, then run sophos-central-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Sophos Central question in plain language; it runs sophos-central-cli for you."
---

# The Sophos Central MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Sophos Limited.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Sophos Central. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Sophos Central to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Running Sophos Central for a book of customers? Ask your AI "what needs attention across every customer today," "which machines have tamper protection off," or "isolate WS-FIN-07" and get one answer instead of opening each tenant from the Partner Dashboard. One command refreshes every customer tenant into a local store, so triage, health ranking, gap checks and billing reconciliation run across the whole book at once.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/sophos-central){: .btn}

## Instead of clicking through Sophos Central, just ask

**Instead of** Opening each customer from the Sophos Central Partner Dashboard every morning to work out which alerts, unhealthy machines and isolated or stale devices actually matter most
**just ask:** *"What needs attention across all my Sophos customers today, worst first?"*
<sub>Your agent runs: <code>sophos-central-cli fleet triage --severity high,medium</code></sub>

**Instead of** Clicking through each tenant's endpoint list to find the machines where tamper protection was switched off for a fix and never turned back on
**just ask:** *"Which machines across my customers have tamper protection off, and can you turn it back on?"*
<sub>Your agent runs: <code>sophos-central-cli fleet tamper --yes</code></sub>

**Instead of** Mid-incident, working out which customer tenant a hostname belongs to before you can even reach the isolate button
**just ask:** *"Isolate WS-FIN-07, start a scan and pull a forensic log"*
<sub>Your agent runs: <code>sophos-central-cli contain "WS-FIN-07" --scan --forensic-log --yes</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/sophos-central/wide-1200x630.png" src="/assets/video/sophos-central/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/sophos-central/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What needs attention across all my customers today? | `sophos-central-cli fleet triage` |
| Which customers have the worst Account Health Check scores, and who slipped since the last refresh? | `sophos-central-cli fleet health --limit 10` |
| Where are the security and commercial gaps, customer by customer? | `sophos-central-cli fleet gaps --kind all` |
| Which machines have tamper protection off across every customer? | `sophos-central-cli fleet tamper` |
| Isolate this machine without looking up which tenant it is in | `sophos-central-cli contain "WS-FIN-07" --scan --yes` |
| What did each customer bill last month, and does it match what is deployed? | `sophos-central-cli fleet revenue --month 2026-09` |
| Feed every customer's SIEM events and alerts into my log pipeline | `sophos-central-cli fleet siem-collect --stream both` |
| Refresh every customer tenant into the local store | `sophos-central-cli fleet refresh` |
| Which credential am I using, and is it a partner, organization or tenant? | `sophos-central-cli whoami` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/sophos-central/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/sophos-central/guide.md).

## What makes this one different

Live MCP wrappers for Sophos Central, such as [rijul170/sophos-central-mcp](https://github.com/rijul170/sophos-central-mcp), take a tenant ID on each call and answer one tenant at a time. This connector also resolves the caller with whoami and routes every tenant call to that tenant's regional host for you, but its fleet commands first refresh every customer tenant into a local SQLite store with a per-tenant ok/forbidden/error ledger, so a book-wide question (triage, health ranking, gaps) is one local query the agent reads as a finished answer, billing is joined to the deployed endpoints already in that store, and the next refresh shows what changed.

It complements the Sophos Central Partner Dashboard rather than replacing it: the dashboard stays the place for policy authoring, investigations and Sophos's own consolidated alert view, while this skill brings cross-customer triage, health ranking, gap checks, tamper remediation and billing reconciliation to whichever AI agent you already use.

## The pain this closes

- Sophos Central is tenant-by-tenant underneath. A partner credential has to list its tenants, then call each tenant's own regional API host with an X-Tenant-ID header (Sophos's own partner getting-started guide walks through that loop). The Partner Dashboard rolls up alerts, but the questions behind them - which machines are unhealthy, whose tamper protection is off, whose health score slipped - still mean drilling into one customer at a time.
- Home-grown scripts stop at one tenant. Community tooling is usually written for a single customer: the SophosCentral-Powershell module says outright "There is currently no support for multiple tenants", and multi-tenant support sits on its to-do list. Every MSP ends up rewriting the same tenant loop, region routing and paging.
- Monthly billing and coverage reconciliation is manual. Matching the partner usage report to what is actually deployed per customer, spotting licences used beyond their quantity or about to expire, and finding devices that are unlicensed or unencrypted means exporting and stitching data by hand.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/sophos-central/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/sophos-central/install.ps1 | iex
```

After install, authenticate once with your Sophos Central credentials, then verify with `sophos-central-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | sophos-central-cli fleet triage; fleet health; fleet gaps; fleet revenue; fleet refresh; whoami; tenants list; alerts list; endpoints list; health-check get; licenses list; siem events; search; sync; contain and fleet tamper without --yes (preview only) | Allow |
| Endpoint response | contain --yes; endpoints set-isolation; endpoints bulk-isolate; endpoints scan; endpoints update-check; endpoints collect-forensic-log; endpoints collect-memory-dump; live-discover run-query; fleet tamper --yes | Human-in-the-loop, explicit confirmation. These reach real customer machines. |
| Protection and policy config | exclusions add-scanning; exclusions delete-scanning-by-id; allowed-items post-by-id; blocked-items post-by-id; blocked-addresses post-blocked-address-sets; policies update-policy; policies update-settings-of-policy; web-control update-web-decryption-excluded-sites; exploit-mitigation add-application; event-journal update-settings; endpoint-groups add-multiple-endpoints-to-group | Preview with --dry-run, then a human-approved write. An exclusion or allowed item can open a hole. |
| Write (routine) | alerts act-on; endpoint-tags; users create-tenant; health-check post-snooze; xdr start-query-run; enrichment-pivots create | Preview with --dry-run, then a reviewed write |
| Credential and access | endpoints get-tamper-protection and endpoints set-tamper-protection (return the tamper password); migrations get-job, get-jobs, create-receiving-job (return the migration token); auth login; partner-admins create; admins create-role-assignment; roles create-tenant | Human-in-the-loop only |
| Destructive and commercial | endpoints delete; endpoints bulk-delete; policies delete-policy; users delete-tenant-by-id; migrations create-sending-job; tenants create; tenants post-account-amendment; provisioning reset-default-products | Human-in-the-loop only, explicit confirmation |

The skill authenticates with a Sophos Central API credential you create, and its reach is exactly the role you give that credential. Every fleet report (triage, health, gaps, revenue) and every list or get, except the credential reads named below, is read-only and safe to let an agent run. Sophos Central is also an endpoint security console, so isolating machines, starting scans, collecting forensic logs and memory dumps, running Live Discover queries and changing tamper protection reach real customer machines; those, plus policy, exclusion and allow-list changes that can weaken protection, need a human to approve the exact command. Commands that return a tamper protection password or a migration token are credential reads, kept out of the MCP server and human-only. contain and fleet tamper preview by default and act only with --yes, which the MCP server never passes; the MCP server's generic execute tool can still call any write endpoint except the six that return a password or token, so set SOPHOS_CENTRAL_MCP_READ_ONLY=1 for an agent that should only read. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/sophos-central/governance.md).

## Frequently asked questions

### Is there an MCP server for Sophos Central?

Yes - this one. A free, open source MCP server and Claude Code Skill for Sophos Central, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Sophos Central MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SOPHOS_CENTRAL_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `SOPHOS_CENTRAL_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. The client secret stays in your environment (it is written to disk only if you pass auth login --client-secret), and the short-lived access token minted from it is cached in a permission-restricted credentials file on your machine until it expires or you run auth logout, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and sophos-central-mcp speaks HTTP natively: run `sophos-central-mcp --transport http` with SOPHOS_CENTRAL_MCP_HTTP_TOKEN set (it serves `http://127.0.0.1:7777/mcp` and requires that token as a bearer header) and put that endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Sophos Central data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What Sophos Central API credential do I need?

An API credential (a service principal) created in Sophos Central: in the Partner Dashboard under Settings & Policies > API Credentials for a partner, or under Global Settings > API Credentials Management for an organization or a single customer. Set SOPHOS_CENTRAL_CLIENT_ID and SOPHOS_CENTRAL_CLIENT_SECRET. The role you give it is the real permission boundary: Service Principal Read-Only covers every report here but cannot run Live Discover queries; writes such as isolation, scans and tamper protection need a role that allows them.

### Does it work with a partner, organization and single-tenant credential?

Yes. whoami tells the CLI which kind of credential it holds. A partner or organization credential walks every customer tenant in the fleet commands, and picks one customer with --tenant (or SOPHOS_CENTRAL_TENANT_ID) for tenant-scoped commands such as endpoints or alerts. A single-tenant credential works on its own tenant.

### Will this hit Sophos Central API rate limits?

Calls go through a shared throttle of 1.5 requests per second by default (--rate-limit), under Sophos's own limits. fleet refresh --plan prints the tenant count and estimated calls and time before you run it. After a refresh, fleet triage, fleet health and fleet gaps read the local store and make no API calls.

### Why do some customers show as forbidden?

Sophos returns 403 to a partner for trial tenants and for term customers without Partner Assistance. fleet refresh records those in its per-tenant ledger and carries on, and fleet triage lists them as blind tenants so you can see what was not read. Trial tenants are skipped unless you pass --include-trial. A full refresh (no --tenant) also removes what earlier fleet refreshes stored for tenants it no longer covers (offboarded tenants, and trial tenants when --include-trial is not set), so fleet reports always match your last full refresh.

### Can the AI isolate a machine or turn tamper protection back on by itself?

Only if you let it. contain and fleet tamper print a plan and change nothing without --yes, and the MCP server never passes --yes, so those two only preview over MCP. But the MCP server's generic sophos-central_execute tool can call any Sophos Central endpoint except the six that return a password or token, including isolation, scans and policy changes, gated only by your AI app's approval prompt. If an agent should only read, set SOPHOS_CENTRAL_MCP_READ_ONLY=1 in the MCP server's environment: it refuses every non-GET call and hides the sync and export tools. Keep the approval prompt on, and give the API credential a read-only role.

### Does it cover Sophos XDR or MDR Cases and Detections?

Not in this version. It covers the REST APIs: alerts, endpoints, policies, exclusions, Live Discover, XDR Data Lake queries, SIEM events and alerts, Account Health Check, licences and partner billing. Cases and Detections are not included.

### Does it replace the Sophos Central Partner Dashboard?

No. The dashboard stays best for policy authoring, investigations and day-to-day console work. This skill answers the across-every-customer questions and runs scripted actions from your AI agent.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Bitdefender GravityZone](/skills/bitdefender/) · [Blumira](/skills/blumira/) · [CIPP + Microsoft 365](/skills/cipp/) · [Cisco Duo](/skills/cisco-duo/) · [ConnectSecure](/skills/connectsecure/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Fortinet](/skills/fortinet/) · [Google Workspace](/skills/google-workspace/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SaaS Alerts](/skills/saas-alerts/) · [SentinelOne](/skills/sentinelone/) · [SonicWall](/skills/sonicwall/) · [ThreatDown](/skills/threatdown/) · [ThreatLocker](/skills/threatlocker/) · [WatchGuard Cloud](/skills/watchguard/)

## Status

Beta. Validated against the Sophos Central API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
