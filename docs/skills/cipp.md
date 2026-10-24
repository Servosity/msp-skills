---
layout: default
title: "CIPP + Microsoft 365 (Entra ID, Intune, Exchange Online, Defender, GDAP) MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "MCP server and CLI for CIPP - Microsoft 365, Entra ID, Intune, Exchange Online, Defender and GDAP across every client tenant, with an offline store, fleet posture analytics, and cross-tenant fan-out."
permalink: /skills/cipp/
skill_name: "CIPP + Microsoft 365 MCP"
image: /assets/social/cipp/wide-1200x630.png
verification: live-verified
faqs:
  - q: "Is there an MCP server for CIPP + Microsoft 365?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for CIPP + Microsoft 365, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the CIPP + Microsoft 365 MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CIPP_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CIPP_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Is this an MCP server for Microsoft Defender, Intune, Exchange Online and Entra ID too?"
    a: "Yes, through your CIPP instance. CIPP manages Microsoft 365 for every client tenant, so this connector reaches what CIPP reaches: Entra ID users, MFA and Conditional Access (`cipp-cli list-mfausers`), Intune policies and Autopilot (`cipp-cli list-intune-policy`), Exchange Online mailboxes, rules and forwarding (`cipp-cli list-mailbox-forwarding`), Defender for Endpoint health, vulnerabilities and incidents (`cipp-cli list-defender-state`, `cipp-cli list-defender-tvm`, `cipp-cli exec-incidents-list`), Defender for Office 365 safe links and anti-phishing, SharePoint and Teams, and GDAP roles and relationships (`cipp-cli list-gdaproles`). If you don't run CIPP, the Microsoft Graph connector talks to Graph directly."
  - q: "Why not use Microsoft's own Defender MCP server?"
    a: "Because it only sees your own tenant. Microsoft's Defender tools in the Sentinel MCP server can't be used \"as a guest in another tenant or with delegated access\" (Microsoft Learn, Sentinel MCP triage tool), so an MSP can't point them at client tenants. This connector reads Defender across every client tenant through the GDAP access your CIPP instance already holds."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on Plus, Pro, Team, Business, Enterprise, and Education plans (Free tier does not yet expose Developer Mode). ChatGPT connects to remote MCP servers over HTTPS, not local stdio binaries. The CIPP MCP server runs locally over stdio by default. Its HTTP mode (cipp-mcp --transport http, in cipp 0.1.13 and later) serves http://127.0.0.1:7777/mcp, refuses to start until CIPP_MCP_HTTP_TOKEN is set, requires Authorization: Bearer <token> on every request, and needs --tls-cert/--tls-key for any other address. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my CIPP data safe?"
    a: "The CLI and MCP server run on your machine and talk only to your CIPP instance; MSP Skills receives nothing. But the connector keeps an unencrypted local copy of what it reads (the SQLite store and a five-minute response cache), a learn journal of the commands you ran, and, after auth login, the API client's secret in credentials.toml, so whoever can read your user profile can read them. cipp-cli doctor prints where each file lives, cipp-cli local-data purge --yes (cipp 0.1.13 and later) deletes the store, cache and journal, cipp-cli auth logout removes the stored credentials, and CIPP_NO_LOCAL_DATA=1 (also 0.1.13) keeps no local copy of CIPP data. Removing someone's CIPP access does not touch a copy already on their machine: when a technician leaves, run logout and purge there, delete the checkpoints folder in the CLI's state directory, and rotate the API client's secret in CIPP."
  - q: "Doesn't CIPP already show this across all tenants?"
    a: "Often, yes. CIPP's All Tenants views show the MFA report, Standards alignment and the licence report across your estate, and its own MCP server can query AllTenants. This connector does not replace those. It keeps a local, dated copy so an agent can join endpoints and compare your own snapshots offline, and it gives the agent CIPP's write endpoints behind a preview-then-confirm step (cipp 0.1.13 and later)."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need my own CIPP instance to use this?"
    a: "Yes - this drives your own self-hosted CIPP instance's API. In CIPP you create an API client (it issues a client ID, secret, tenant ID, and token URL); the CLI authenticates with OAuth2 client credentials via 'cipp-cli auth login', or you pipe a bearer token to 'cipp-cli auth set-token'. It reads and acts through your CIPP - it does not replace it."
  - q: "Will this hit CIPP's API rate limit?"
    a: "It can: fanout makes one call per tenant, and CIPP allows 100 requests per 10 seconds per API client. The client waits out HTTP 429 using Retry-After, --concurrency sets how many tenants run at once, and --resume skips tenants a halted run already finished. Once rows are saved, the rollups read the local store and make no API calls."
  - q: "Can this change my tenants, or only read?"
    a: "Both. CIPP is a full management API, so the CLI can create users, offboard, set forwarding, and more. In the MCP server (cipp 0.1.13 and later), any CIPP API call that changes something returns a preview first and runs only when the agent confirms it; set CIPP_MCP_READ_ONLY=1 and the server drops its write tools. On the command line, bulk plans by default and writes only with --execute, and other write commands preview with --dry-run. The strongest boundary is still the role you give the API client in CIPP."
  - q: "Does it replace the CIPP portal?"
    a: "No. CIPP stays your portal. This connector lets your AI agent work across a local copy of what CIPP returns and drive CIPP from natural language."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your CIPP + Microsoft 365 credentials once, then run cipp-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a CIPP + Microsoft 365 question in plain language; it runs cipp-cli for you."
---

# The CIPP + Microsoft 365 (Entra ID, Intune, Exchange Online, Defender, GDAP) MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by CyberDrain.

**✓ Live-verified by Abhi Saini, Bearium Networks (MSP)** against a production tenant · 2026-08-13 · [receipt →](https://compoundingteams.com/). The version it verified was not recorded.

Yes - there is an MCP server for CIPP + Microsoft 365. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects CIPP + Microsoft 365 to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

This is the MCP server for CIPP, and through CIPP for the Microsoft 365 it manages: Entra ID, Intune, Exchange Online, Defender and GDAP, across every client tenant. It is local-first: one fan-out saves what CIPP returns for each tenant into a SQLite store on your machine, so your AI can search, join and compare that copy offline - per-tenant MFA and Conditional Access tables, the policy one tenant lacks, what changed since your last snapshot - and, from cipp 0.1.13, drive CIPP's writes behind a preview it must confirm.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/cipp){: .btn}

## Instead of clicking through CIPP + Microsoft 365, just ask

**Instead of** Exporting CIPP's MFA report and totalling it per tenant in a spreadsheet before every QBR
**just ask:** *"Show me MFA registration across every tenant"*
<sub>Your agent runs: <code>cipp-cli posture --dimension mfa</code></sub>

**Instead of** Exporting every tenant's Conditional Access policies and comparing them by hand to find the client that is missing one the rest of the fleet has
**just ask:** *"Which tenants are missing a Conditional Access policy most of my clients have?"*
<sub>Your agent runs: <code>cipp-cli ca diff</code></sub>

**Instead of** Running the same offboarding steps - block sign-in, convert mailbox, set forwarding - separately in each tenant for a batch of departures, and checking each response by hand to see whether it really worked
**just ask:** *"Plan the offboarding for this CSV of departures across their tenants"*
<sub>Your agent runs: <code>cipp-cli bulk --from offboards.csv</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/cipp/wide-1200x630.png" src="/assets/video/cipp/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/cipp/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which tenants have users with no MFA registered, as one row per tenant? | `cipp-cli posture --dimension mfa` |
| How does Conditional Access compare across all tenants? | `cipp-cli posture --dimension ca` |
| Which tenants lack a Conditional Access policy most of the fleet has? | `cipp-cli ca diff` |
| Which delegated-admin (GDAP) relationships expire in the next 30 days? | `cipp-cli gdap expiring --days 30` |
| What changed in admin roles since last week? | `cipp-cli changes --resource roles --since 7d` |
| Everything stored about one client tenant, in one record | `cipp-cli scorecard --tenant contoso.onmicrosoft.com` |
| Pull one read across every client tenant and keep it locally | `cipp-cli fanout --endpoint /ListMFAUsers --all-tenants --save` |
| Plan a batch of offboards from a CSV before anything runs | `cipp-cli bulk --from offboards.csv` |
| Delete the local store, response cache and learn journal (cipp 0.1.13 and later) | `cipp-cli local-data purge --yes` |
| Are my CIPP credentials and connectivity healthy? | `cipp-cli doctor` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/cipp/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/cipp/guide.md).

## What makes this one different

Most CIPP integrations answer each question with a live call and keep nothing. This connector is local-first: fanout runs a CIPP read once per tenant and, with --save, keeps the rows in a SQLite file on your machine, each save a dated snapshot. The agent can then work across that copy (a per-tenant MFA or Conditional Access table in posture, every stored signal for one tenant in scorecard, the policy one tenant lacks in ca diff) and compare your own snapshots over time (changes, standards drift) without another API call. The cost is one call per tenant and a local copy of tenant data, which you can delete with local-data purge or switch off with CIPP_NO_LOCAL_DATA=1 (both in cipp 0.1.13 and later).

CIPP already answers estate-wide questions itself: its All Tenants views cover the MFA report, Standards alignment and the licence report, its API returns one estate-wide response for tenantFilter=AllTenants, and it ships its own MCP server. If one of those answers your question, use it. This connector complements CIPP rather than replacing it: it keeps a local, dated copy an agent can join and compare offline, and it carries CIPP's full command surface, writes included, behind a preview the agent must confirm (cipp 0.1.13 and later). Microsoft's own Defender tools in its Sentinel MCP server stop at your home tenant: "You can't use this collection as a guest in another tenant or with delegated access" (Microsoft Learn, Sentinel MCP triage tool). This connector reaches every client tenant through the GDAP access CIPP already holds.

## The pain this closes

- CIPP's own views answer questions about the current state. An AI agent working from live answers alone starts over on every question, and comparing this quarter with last needs dated copies you keep yourself.
- Scripting CIPP means living with its API limit of 100 requests per 10 seconds per API client (per the CIPP API documentation at docs.cipp.app), and with write endpoints that can answer HTTP 200 while reporting the failure inside the response's Results, so a batch has to read every response to know what happened.
- Joining what CIPP returns - which tenant lacks a Conditional Access policy most of the fleet has, what changed in admin roles since last month - is the kind of work an agent can do in seconds from a local copy.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cipp/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cipp/install.ps1 | iex
```

After install, authenticate once with your CIPP + Microsoft 365 credentials, then verify with `cipp-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read (rollups and lists) | cipp-cli posture --dimension mfa; cipp-cli ca diff; cipp-cli gdap expiring --days 30; cipp-cli fanout --endpoint /ListUsers --all-tenants; cipp-cli list-tenants; cipp-cli doctor | Allow |
| Write (routine) | cipp-cli add-user; cipp-cli edit-user; cipp-cli bulk --from changes.csv --execute (plans by default; writes only with --execute) | Preview with --dry-run, then a reviewed write |
| Credential / security | cipp-cli exec-reset-mfa; cipp-cli exec-per-user-mfa; cipp-cli exec-get-local-admin-password; cipp-cli exec-token-exchange | Human-in-the-loop only |
| Destructive | cipp-cli exec-offboard-user; cipp-cli remove-user; cipp-cli exec-device-delete; cipp-cli delete-sharepoint-site | Human-in-the-loop only, explicit confirmation |

The skill drives the cipp-cli and cipp-mcp binaries against your self-hosted CIPP instance, with a bearer token from CIPP_API_KEY or one minted by 'auth login' (kept with the client secret in the CLI's credentials file; with CIPP_NO_CONFIG_WRITE=1, auth login refuses and nothing is stored), sent nowhere except the CIPP API. CIPP is a read-write management API. Read and rollup commands change nothing. In the MCP server (cipp 0.1.13 and later), every CIPP API call that changes something, including every CIPP GET endpoint named as an action, returns a preview and runs only when the agent confirms it, and CIPP_MCP_READ_ONLY=1 drops the write tools. On the command line, bulk prints its plan and only writes with --execute, and other write commands send on run unless you pass --dry-run first. The real permission boundary is the role you grant the API client in CIPP. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/cipp/governance.md).

## Frequently asked questions

### Is there an MCP server for CIPP + Microsoft 365?

Yes - this one. A free, open source MCP server and Claude Code Skill for CIPP + Microsoft 365, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the CIPP + Microsoft 365 MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CIPP_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CIPP_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Is this an MCP server for Microsoft Defender, Intune, Exchange Online and Entra ID too?

Yes, through your CIPP instance. CIPP manages Microsoft 365 for every client tenant, so this connector reaches what CIPP reaches: Entra ID users, MFA and Conditional Access (`cipp-cli list-mfausers`), Intune policies and Autopilot (`cipp-cli list-intune-policy`), Exchange Online mailboxes, rules and forwarding (`cipp-cli list-mailbox-forwarding`), Defender for Endpoint health, vulnerabilities and incidents (`cipp-cli list-defender-state`, `cipp-cli list-defender-tvm`, `cipp-cli exec-incidents-list`), Defender for Office 365 safe links and anti-phishing, SharePoint and Teams, and GDAP roles and relationships (`cipp-cli list-gdaproles`). If you don't run CIPP, the Microsoft Graph connector talks to Graph directly.

### Why not use Microsoft's own Defender MCP server?

Because it only sees your own tenant. Microsoft's Defender tools in the Sentinel MCP server can't be used "as a guest in another tenant or with delegated access" (Microsoft Learn, Sentinel MCP triage tool), so an MSP can't point them at client tenants. This connector reads Defender across every client tenant through the GDAP access your CIPP instance already holds.

### Does this work with ChatGPT?

Yes, on Plus, Pro, Team, Business, Enterprise, and Education plans (Free tier does not yet expose Developer Mode). ChatGPT connects to remote MCP servers over HTTPS, not local stdio binaries. The CIPP MCP server runs locally over stdio by default. Its HTTP mode (cipp-mcp --transport http, in cipp 0.1.13 and later) serves http://127.0.0.1:7777/mcp, refuses to start until CIPP_MCP_HTTP_TOKEN is set, requires Authorization: Bearer <token> on every request, and needs --tls-cert/--tls-key for any other address. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my CIPP data safe?

The CLI and MCP server run on your machine and talk only to your CIPP instance; MSP Skills receives nothing. But the connector keeps an unencrypted local copy of what it reads (the SQLite store and a five-minute response cache), a learn journal of the commands you ran, and, after auth login, the API client's secret in credentials.toml, so whoever can read your user profile can read them. cipp-cli doctor prints where each file lives, cipp-cli local-data purge --yes (cipp 0.1.13 and later) deletes the store, cache and journal, cipp-cli auth logout removes the stored credentials, and CIPP_NO_LOCAL_DATA=1 (also 0.1.13) keeps no local copy of CIPP data. Removing someone's CIPP access does not touch a copy already on their machine: when a technician leaves, run logout and purge there, delete the checkpoints folder in the CLI's state directory, and rotate the API client's secret in CIPP.

### Doesn't CIPP already show this across all tenants?

Often, yes. CIPP's All Tenants views show the MFA report, Standards alignment and the licence report across your estate, and its own MCP server can query AllTenants. This connector does not replace those. It keeps a local, dated copy so an agent can join endpoints and compare your own snapshots offline, and it gives the agent CIPP's write endpoints behind a preview-then-confirm step (cipp 0.1.13 and later).

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need my own CIPP instance to use this?

Yes - this drives your own self-hosted CIPP instance's API. In CIPP you create an API client (it issues a client ID, secret, tenant ID, and token URL); the CLI authenticates with OAuth2 client credentials via 'cipp-cli auth login', or you pipe a bearer token to 'cipp-cli auth set-token'. It reads and acts through your CIPP - it does not replace it.

### Will this hit CIPP's API rate limit?

It can: fanout makes one call per tenant, and CIPP allows 100 requests per 10 seconds per API client. The client waits out HTTP 429 using Retry-After, --concurrency sets how many tenants run at once, and --resume skips tenants a halted run already finished. Once rows are saved, the rollups read the local store and make no API calls.

### Can this change my tenants, or only read?

Both. CIPP is a full management API, so the CLI can create users, offboard, set forwarding, and more. In the MCP server (cipp 0.1.13 and later), any CIPP API call that changes something returns a preview first and runs only when the agent confirms it; set CIPP_MCP_READ_ONLY=1 and the server drops its write tools. On the command line, bulk plans by default and writes only with --execute, and other write commands preview with --dry-run. The strongest boundary is still the role you give the API client in CIPP.

### Does it replace the CIPP portal?

No. CIPP stays your portal. This connector lets your AI agent work across a local copy of what CIPP returns and drive CIPP from natural language.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Bitdefender GravityZone](/skills/bitdefender/) · [Blumira](/skills/blumira/) · [Cisco Duo](/skills/cisco-duo/) · [ConnectSecure](/skills/connectsecure/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Fortinet](/skills/fortinet/) · [Google Workspace](/skills/google-workspace/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SaaS Alerts](/skills/saas-alerts/) · [SentinelOne](/skills/sentinelone/) · [SonicWall](/skills/sonicwall/) · [Sophos Central](/skills/sophos-central/) · [ThreatDown](/skills/threatdown/) · [ThreatLocker](/skills/threatlocker/) · [WatchGuard Cloud](/skills/watchguard/)

## Status

Live-verified. Confirmed by Abhi Saini, Bearium Networks (MSP) against their own production tenant ([receipt](https://compoundingteams.com/)). The version it verified was not recorded. More MSPs run it live in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
