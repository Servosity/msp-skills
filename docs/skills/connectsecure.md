---
layout: default
title: "ConnectSecure MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every ConnectSecure API operation, plus a local store that answers vulnerability x asset x client questions across your whole client base in one command."
permalink: /skills/connectsecure/
skill_name: "ConnectSecure MCP"
image: /assets/social/connectsecure/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for ConnectSecure?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for ConnectSecure, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the ConnectSecure MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CONNECTSECURE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CONNECTSECURE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS. Start the server with `CONNECTSECURE_MCP_HTTP_TOKEN=<token> connectsecure-mcp --transport http` (it serves http://127.0.0.1:7777/mcp and requires that bearer token on every call) and put it behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your pod URL, tenant name, client ID and client secret once."
  - q: "Is my ConnectSecure data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, the local mirror and a cache of API responses are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills. Credential records, integration secrets, scheduled-job passwords, attack-surface creds and hashes, custom-domain certificate details and the agent uninstall secret are never written to the local mirror or the response cache, even if you ask sync for them."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Where do I find my pod URL and API credentials?"
    a: "In the ConnectSecure portal: Settings > Users > (user) > API Key gives you the client ID and client secret, and Profile > API Documentation shows your pod URL (https://pod<N>.myconnectsecure.com). The tenant name is the one you type on the portal login screen. The CLI exchanges those for a short-lived token on first use."
  - q: "Will this hit my ConnectSecure API rate limits?"
    a: "Lightly. `sync` reads a curated set of reports once and stores them locally, and exposure, portfolio, blind-spots, suppression-audit and ad-hygiene then answer from that mirror without calling the API. qbr and cve-impact call the live API, with GET responses cached on disk for 5 minutes (--no-cache forces a fresh call); cve-impact reads at most --max-pages pages of 100 affected assets. By default the CLI paces itself to the server's rate-limit headers."
  - q: "Will this replace the ConnectSecure portal?"
    a: "No. Scanning, remediation, agent deployment and branded client reports stay in the portal. This skill answers the cross-client questions the portal makes you click through, from your AI."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your ConnectSecure credentials once, then run connectsecure-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a ConnectSecure question in plain language; it runs connectsecure-cli for you."
---

# The ConnectSecure MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by ConnectSecure.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for ConnectSecure. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects ConnectSecure to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI which open vulnerabilities across every client are most likely to be exploited, and get one ranked list naming the client and the machine behind every finding. It also tells you which clients a new CVE hits and what fixes exist, whose scan data you cannot trust, which suppressions are quietly hiding risk, and what goes in a client's QBR risk slide.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/connectsecure){: .btn}

## Instead of clicking through ConnectSecure, just ask

**Instead of** Export each client's vulnerability report from the ConnectSecure portal and merge them in a spreadsheet to find the worst exposures across the book
**just ask:** *"Which critical and high vulnerabilities across all our clients are most likely to be exploited?"*
<sub>Your agent runs: <code>connectsecure-cli exposure --min-epss 0.5 --severity Critical,High</code></sub>

**Instead of** Open a CVE in the portal, then click through every affected asset to work out which client it belongs to and whether a fix exists
**just ask:** *"A new CVE just dropped. Which of our clients does it hit, and is there a fix?"*
<sub>Your agent runs: <code>connectsecure-cli cve-impact "CVE-2024-3400"</code></sub>

**Instead of** Build each QBR risk slide by hand from the company dashboard, the remediation report and last quarter's export
**just ask:** *"Pull together Acme Dental's QBR risk slide, including what we fixed in the last 90 days."*
<sub>Your agent runs: <code>connectsecure-cli qbr "Acme Dental" --since 90d</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/connectsecure/wide-1200x630.png" src="/assets/video/connectsecure/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/connectsecure/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which open vulnerabilities across every client are most likely to be exploited? | `connectsecure-cli exposure --min-epss 0.5 --severity Critical,High` |
| What on the CISA Known Exploited list showed up this week? | `connectsecure-cli exposure --kev --new-since 7d` |
| Which clients carry the most risk, side by side? | `connectsecure-cli portfolio --sort max-epss` |
| A CVE just made the news. Which clients and assets does it hit, and is there a fix? | `connectsecure-cli cve-impact "CVE-2024-3400"` |
| Which assets have scan data we cannot trust? | `connectsecure-cli blind-spots --stale 14d --agent-stale 48h` |
| Which suppressions expired, never expire, or were approved by the person who asked for them? | `connectsecure-cli suppression-audit --within 14d` |
| Which clients have risky Active Directory accounts or weak password policies? | `connectsecure-cli ad-hygiene --stale-logon 90d --min-length 12` |
| What goes in this client's QBR risk slide? | `connectsecure-cli qbr "Acme Dental" --since 90d` |
| Is my auth and connectivity actually working? | `connectsecure-cli doctor` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/connectsecure/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/connectsecure/guide.md).

## What makes this one different

A stateless wrapper over the ConnectSecure API answers one endpoint at a time. The questions an MSP asks are cross-client: rank every open finding at asset level, flag untrustworthy scans, audit every suppression. This skill syncs a curated set of ConnectSecure reports into a local SQLite mirror and answers those as one local join, and it never writes credential records, integration secrets, scheduled-job passwords, attack-surface creds and hashes or the agent uninstall secret to that mirror.

The ConnectSecure portal is where you scan, remediate and produce branded client reports, and this skill does not replace it. It puts the cross-client questions - exposure ranking, CVE blast radius, blind spots, suppression hygiene, AD hygiene - one sentence away in the AI you already use, and returns the rows behind each answer.

## The pain this closes

- Vulnerability scans rarely come back clean, so an MSP is staring at thousands of findings across dozens of clients (ChannelPro, "Too many vulnerabilities, not enough time"). ConnectSecure's per-client vulnerability report lists affected assets only by ID, so ranking findings across every client by machine means matching those IDs to the asset list yourself.
- "Which of our clients does this CVE hit?" takes two reports - the CVE report for fixes and the assets-by-problem report for affected machines - and a manual grouping of assets by client, on the morning an advisory lands.
- A clean result on an asset that has not been discovered in weeks, was scanned without credentials, or whose agent went silent is not clean, it is unknown. So is a vulnerability suppression that expired but was never lifted, or was approved by the same person who asked for it.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectsecure/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectsecure/install.ps1 | iex
```

After install, authenticate once with your ConnectSecure credentials, then verify with `connectsecure-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | exposure, portfolio, blind-spots, suppression-audit, ad-hygiene, qbr, cve-impact, sync, search, every list / get except the credential reads | Allow |
| Write (routine) | tags create, tag-rules update, companies update, custom-profile create, report-builder create-job, report-builder update-cover-page / update-standard-report-settings / upload-template, every other create / update not named below | Preview with --dry-run, then a reviewed write |
| Credential / security | credentials list / get / create / update, integration-credentials list / get / create / update, integration-rules list / get / create / update, attack-surface-results list / get, agents uninstall-secret, scheduler list / get / create / update, custom-domains list / get / create, agent-credentials-mapping create / update, company-mappings create / update, export credentials / integration_credentials / integration_rules / scheduler / attack_surface_results / custom_domains, tail on any of these, session, auth set-token | Human-in-the-loop only |
| Endpoint-affecting / posture | patching patch-now (alias patching create), scheduler create / update / update-schedule, companies scan, companies external-scan, agents reset (alias agents create), suppress-vulnerability create / update, discovery-settings / application-baseline-rules / pii-scan-settings / agent-discoverysettings-mapping / attack-surface-domain create / update, assets create / update (scan credentials, discovery settings, deprecation) | Human-in-the-loop only, explicit confirmation |
| Destructive | every delete (companies, assets, credentials and 19 more), assets bulk-deprecate, scheduler remove-schedule, import | Human-in-the-loop only, explicit confirmation |

Read commands - the seven analysis commands, sync, search, and every list or get except credential reads - change nothing and are safe for an agent. Credential reads, patching and patch schedules, scans, agent moves, suppressions and deletes are human-in-the-loop only. Set CONNECTSECURE_MCP_READ_ONLY=1 to make the MCP server refuse every write to ConnectSecure; it still allows credential reads, so scope the API user too. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/connectsecure/governance.md).

## Frequently asked questions

### Is there an MCP server for ConnectSecure?

Yes - this one. A free, open source MCP server and Claude Code Skill for ConnectSecure, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the ConnectSecure MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CONNECTSECURE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CONNECTSECURE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS. Start the server with `CONNECTSECURE_MCP_HTTP_TOKEN=<token> connectsecure-mcp --transport http` (it serves http://127.0.0.1:7777/mcp and requires that bearer token on every call) and put it behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your pod URL, tenant name, client ID and client secret once.

### Is my ConnectSecure data safe?

Your data stays on your machine. The CLI, MCP server, the local mirror and a cache of API responses are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills. Credential records, integration secrets, scheduled-job passwords, attack-surface creds and hashes, custom-domain certificate details and the agent uninstall secret are never written to the local mirror or the response cache, even if you ask sync for them.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Where do I find my pod URL and API credentials?

In the ConnectSecure portal: Settings > Users > (user) > API Key gives you the client ID and client secret, and Profile > API Documentation shows your pod URL (https://pod<N>.myconnectsecure.com). The tenant name is the one you type on the portal login screen. The CLI exchanges those for a short-lived token on first use.

### Will this hit my ConnectSecure API rate limits?

Lightly. `sync` reads a curated set of reports once and stores them locally, and exposure, portfolio, blind-spots, suppression-audit and ad-hygiene then answer from that mirror without calling the API. qbr and cve-impact call the live API, with GET responses cached on disk for 5 minutes (--no-cache forces a fresh call); cve-impact reads at most --max-pages pages of 100 affected assets. By default the CLI paces itself to the server's rate-limit headers.

### Will this replace the ConnectSecure portal?

No. Scanning, remediation, agent deployment and branded client reports stay in the portal. This skill answers the cross-client questions the portal makes you click through, from your AI.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Blumira](/skills/blumira/) · [CIPP + Microsoft 365](/skills/cipp/) · [Cisco Duo](/skills/cisco-duo/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Fortinet](/skills/fortinet/) · [Google Workspace](/skills/google-workspace/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [SonicWall](/skills/sonicwall/) · [Sophos Central](/skills/sophos-central/) · [ThreatDown](/skills/threatdown/) · [ThreatLocker](/skills/threatlocker/)

## Status

Beta. Validated against the ConnectSecure API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
