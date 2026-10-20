---
layout: default
title: "Bitdefender GravityZone MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "140 GravityZone API methods, plus fleet health, billing and incident views across child companies."
permalink: /skills/bitdefender/
skill_name: "Bitdefender GravityZone MCP"
image: /assets/social/bitdefender/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Bitdefender GravityZone?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Bitdefender GravityZone, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Bitdefender GravityZone MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `BITDEFENDER_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `BITDEFENDER_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Your API key stays in your environment unless you save it with auth set-token, which writes it to a permission-restricted credentials file on your machine, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and bitdefender-mcp speaks HTTP natively: run `bitdefender-mcp --transport http` with BITDEFENDER_MCP_HTTP_TOKEN set (it serves `http://127.0.0.1:7777/mcp` and requires that token as a bearer header) and put it behind an HTTPS gateway that ChatGPT can sign in to with OAuth and that adds that bearer header, since ChatGPT connectors do not send a fixed one. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my GravityZone data safe?"
    a: "Your data stays on your machine. The CLI, the MCP server and the local snapshot store are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What GravityZone API key do I need?"
    a: "A Control Center API key: click the user icon in Control Center, choose My Account and use the Control Center API section, which also shows the Access URL. Keys are scoped per API area when you create them, with no read/write split inside an area. The fleet commands need the Companies and Network APIs, plus Licensing for usage billing, Incidents for fleet incident-queue, and Quarantine and Incidents for fleet threats. Set BITDEFENDER_API_KEY, and set BITDEFENDER_BASE_URL to your Access URL if it is not https://cloud.gravityzone.bitdefender.com/api."
  - q: "Does it need a partner account?"
    a: "For the cross-customer views, yes: walking child companies needs a partner-level key, because getCompaniesList is a partner API. A customer-level key works, but every fleet view then covers only that one company."
  - q: "Will this hit GravityZone API rate limits?"
    a: "Control Center limits each API key to a number of requests per second and answers 429 with a Retry-After header when you go over. The CLI paces itself at 5 requests per second by default (--rate-limit) and waits out a 429. getIncidentsList allows about 3 calls a minute, so fleet incident-queue makes one partner-wide call and spaces any extra pages 21 seconds apart."
  - q: "Can the AI isolate a machine or run a command on an endpoint by itself?"
    a: "Only if you let it. Generated commands run as soon as they are invoked and do not prompt, so preview with --dry-run first: it prints the JSON-RPC method and parameters it would send, without sending. Over MCP, the generic bitdefender_execute tool can call any GravityZone method, including isolation, kill process, remote command execution and deletes, gated only by your AI app's approval prompt. For an agent that should only read, set BITDEFENDER_MCP_READ_ONLY=1 in the MCP server's environment: bitdefender_execute then runs only GravityZone read methods, refuses every write and the Credential-tier reads, and the import tool is hidden. Incident reads stay allowed, and XDR evidence can hold connection headers captured on the endpoint, so treat incident output as sensitive. Keep the approval prompt on."
  - q: "Why does fleet changes say it has nothing to compare?"
    a: "It diffs saved snapshots, and each fleet health run saves one (unless you pass --no-save). Run fleet health at least twice, for example daily, and fleet changes reports endpoints added or removed, managed/unmanaged flips, newly outdated agents, policies no longer applied and isolation changes between them."
  - q: "Does it see endpoints from GravityZone's Active Directory integration?"
    a: "Not in the fleet commands. They read getEndpointsList, which returns the Computers and Groups inventory; Bitdefender documents that endpoints living only under the Active Directory integration come from getNetworkInventoryItems instead. So fleet health counts, --fail-on and fleet rmm-gap cover Computers and Groups, and the fleet health, locate, rmm-gap and changes results carry an inventory_scope field that says so. Before deploying an agent to a device rmm-gap lists as missing, check it with inventory list (a Credential-tier read, because its company rows carry license keys)."
  - q: "Does it work with on-premises GravityZone?"
    a: "It speaks the same JSON-RPC Public API, so set BITDEFENDER_BASE_URL to your Control Center's Access URL. It has not yet been verified against a live on-premises console."
  - q: "Does it replace GravityZone Control Center?"
    a: "No. Control Center stays best for policy authoring, investigations and day-to-day console work. This skill answers the across-every-customer questions and runs scripted actions from your AI agent."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/bitdefender/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Bitdefender GravityZone credentials once, then run bitdefender-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Bitdefender GravityZone question in plain language; it runs bitdefender-cli for you."
---

# The Bitdefender GravityZone MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Bitdefender SRL.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Bitdefender GravityZone. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Bitdefender GravityZone to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Running Bitdefender GravityZone for a book of customers? Ask your AI "which customers have unmanaged or outdated machines," "what open incidents are high or critical across every client," or "which company is LAPTOP-7 in" and get one answer instead of opening each company in Control Center. The fleet commands walk your whole partner tree, so health ranking, incident triage, threat spread and monthly seat usage cover every child company.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/bitdefender){: .btn}

## Instead of clicking through Bitdefender GravityZone, just ask

**Instead of** Opening each customer company in GravityZone Control Center to count unmanaged machines, outdated agents and endpoints that have not scanned in a week
**just ask:** *"Which of my customers have unmanaged, outdated or unscanned endpoints, worst first?"*
<sub>Your agent runs: <code>bitdefender-cli fleet health --scan-days 7</code></sub>

**Instead of** Checking the Incidents page company by company every morning to find the open EDR incidents that matter most and the ones nobody has picked up
**just ask:** *"What open incidents across all my clients are high or critical, and which are unassigned?"*
<sub>Your agent runs: <code>bitdefender-cli fleet incident-queue --priority high,critical --sla 24h</code></sub>

**Instead of** Exporting monthly license usage per company at month end and reconciling it against PSA agreements in a spreadsheet
**just ask:** *"What did each customer use in GravityZone last month, and does it match what I bill them?"*
<sub>Your agent runs: <code>bitdefender-cli usage billing --month 09/2026 --agreements psa-agreements.csv</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/bitdefender/wide-1200x630.png" src="/assets/video/bitdefender/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/bitdefender/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which customers have unmanaged, outdated or unscanned endpoints? | `bitdefender-cli fleet health` |
| What changed in endpoint protection since last week? | `bitdefender-cli fleet changes --since 7d` |
| What open incidents across every client are high or critical? | `bitdefender-cli fleet incident-queue --priority high,critical` |
| Is the same malware quarantined at several customers, and is it blocklisted? | `bitdefender-cli fleet threats --window 7d` |
| Which customer is this machine in, and what is its endpoint id? | `bitdefender-cli fleet locate "LAPTOP-7"` |
| Which machines in my RMM have no Bitdefender agent? | `bitdefender-cli fleet rmm-gap --devices ninja-devices.csv` |
| What did each customer use last month, after the partner rollup? | `bitdefender-cli usage billing --month 09/2026` |
| List every child company and sub-partner with its id and parent | `bitdefender-cli companies tree` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/bitdefender/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/bitdefender/guide.md).

## What makes this one different

Integrations such as [Liongard's GravityZone inspector](https://docs.liongard.com/docs/bitdefender-gravityzone-inspector) and [Sumo Logic's GravityZone automation integration](https://www.sumologic.com/help/docs/platform-services/automation-service/app-central/integrations/bitdefender-gravityzone/) bring GravityZone data into their own platforms, and a plain API wrapper answers one JSON-RPC method for one company per call. This connector walks the whole partner tree for you, fans each question out across every child company, and saves each fleet health run as a dated local snapshot, so fleet changes, fleet locate and fleet rmm-gap can answer from it. A company it could not read completely is listed in fetch_failures, never counted as clean.

It complements GravityZone Control Center rather than replacing it: the console stays the place for policy authoring, investigations and day-to-day response, while this skill brings cross-customer health ranking, incident triage, threat spread, machine lookup and billing reconciliation to whichever AI agent you already use.

## The pain this closes

- The GravityZone Public API is JSON-RPC, one method per call, and getCompaniesList answers one level of the company tree at a time. Every MSP script walks sub-partners and folders itself, then pages getEndpointsList per company. A Bitdefender Expert Community thread ("Powershell API - Get List of Computers") shows a hand-built paging loop that still returned the wrong group of the hierarchy.
- The morning questions live on different pages: endpoint status under Network, open incidents under Incidents, quarantined files under Quarantine, seats in the monthly usage reports. Asking any of them across every customer means filtering company by company or stitching exports.
- Month-end reconciliation is manual. GravityZone rolls each child company's usage up into its parent partner, so matching billed seats per customer to PSA agreements means exporting usage and working out each partner's own share by hand.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/bitdefender/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/bitdefender/install.ps1 | iex
```

After install, authenticate once with your Bitdefender GravityZone credentials, then verify with `bitdefender-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | bitdefender-cli fleet health; fleet incident-queue; fleet threats; fleet locate; fleet rmm-gap; fleet changes; companies tree; usage billing; usage monthly; companies list; endpoints list; endpoints get; incidents list; quarantine list; policies list; patches missing; tasks status; reports list; api-key | Allow |
| Endpoint response | isolation isolate; isolation restore; endpoints kill-process; investigation kill-process; investigation run-command; investigation collect-package; investigation retrieve-file; tasks scan; tasks scan-by-mac; tasks reconfigure; tasks gather-logs; tasks sandbox-submit; quarantine add-file; quarantine remove; quarantine restore; quarantine empty; quarantine release-exchange; quarantine restore-exchange; live-search; yara; response-actions create | Human-in-the-loop, explicit confirmation. These reach real customer machines. |
| Protection and policy config | policies set-modules; endpoints assign-policy; blocklist add; blocklist remove; custom-rules create; custom-rules update; custom-rules delete; phasr apply; phasr edit-rules-access; phasr request-access-action; maintenance-windows create; maintenance-windows assign; push set; integrations create; integrations manage; integrators add; aws-ec2 configure; endpoints move; groups move | Preview with --dry-run, then a human-approved write. An exclusion rule, a weaker policy or a move into another group's policy can open a hole. |
| Write (routine) | incidents note; incidents set-status; incidents set-priority; incidents assign; endpoints set-label; groups create; company-folders create; custom-fields create; reports create; packages create; tasks delete | Preview with --dry-run, then a reviewed write |
| Credential and access | push get (returns the webhook Authorization value); license get (returns the license key); companies get-by-user (takes a user's password); packages get; packages install-links; integrations get; policies get (policy password settings); inventory list (can return license keys); accounts create; accounts update; accounts delete; email-verification; auth set-token | Human-in-the-loop only |
| Destructive and commercial | import (one create per JSONL line, so it can reach any tier above); endpoints delete; endpoints move-to-company; company-folders move; companies create; companies update; companies suspend; companies delete; company-folders delete; groups delete; custom-fields delete; packages delete; license set-key; license add-product-key; license remove-product-key; license set-monthly-subscription | Human-in-the-loop only, explicit confirmation |

The skill authenticates with a GravityZone Control Center API key you create, and its reach is exactly the API areas you enable on that key. Every fleet report (health, incident queue, threats, locate, RMM gap, changes, billing) and every list or get, except the credential reads named below, is read-only and safe to let an agent run. GravityZone is also an endpoint security console, so isolating machines, killing processes, running remote commands, scans and quarantine tasks reach real customer machines; those, plus policy, blocklist and exclusion changes, need a human to approve the exact command. Reads that return or take a credential (push settings, license keys, policy, package and integration settings, install links, user password checks) are human-only. Generated commands run immediately, and the MCP server's generic execute tool can call any method, so set BITDEFENDER_MCP_READ_ONLY=1 for an agent that should only read. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/bitdefender/governance.md).

## Frequently asked questions

### Is there an MCP server for Bitdefender GravityZone?

Yes - this one. A free, open source MCP server and Claude Code Skill for Bitdefender GravityZone, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Bitdefender GravityZone MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `BITDEFENDER_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `BITDEFENDER_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Your API key stays in your environment unless you save it with auth set-token, which writes it to a permission-restricted credentials file on your machine, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and bitdefender-mcp speaks HTTP natively: run `bitdefender-mcp --transport http` with BITDEFENDER_MCP_HTTP_TOKEN set (it serves `http://127.0.0.1:7777/mcp` and requires that token as a bearer header) and put it behind an HTTPS gateway that ChatGPT can sign in to with OAuth and that adds that bearer header, since ChatGPT connectors do not send a fixed one. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my GravityZone data safe?

Your data stays on your machine. The CLI, the MCP server and the local snapshot store are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What GravityZone API key do I need?

A Control Center API key: click the user icon in Control Center, choose My Account and use the Control Center API section, which also shows the Access URL. Keys are scoped per API area when you create them, with no read/write split inside an area. The fleet commands need the Companies and Network APIs, plus Licensing for usage billing, Incidents for fleet incident-queue, and Quarantine and Incidents for fleet threats. Set BITDEFENDER_API_KEY, and set BITDEFENDER_BASE_URL to your Access URL if it is not https://cloud.gravityzone.bitdefender.com/api.

### Does it need a partner account?

For the cross-customer views, yes: walking child companies needs a partner-level key, because getCompaniesList is a partner API. A customer-level key works, but every fleet view then covers only that one company.

### Will this hit GravityZone API rate limits?

Control Center limits each API key to a number of requests per second and answers 429 with a Retry-After header when you go over. The CLI paces itself at 5 requests per second by default (--rate-limit) and waits out a 429. getIncidentsList allows about 3 calls a minute, so fleet incident-queue makes one partner-wide call and spaces any extra pages 21 seconds apart.

### Can the AI isolate a machine or run a command on an endpoint by itself?

Only if you let it. Generated commands run as soon as they are invoked and do not prompt, so preview with --dry-run first: it prints the JSON-RPC method and parameters it would send, without sending. Over MCP, the generic bitdefender_execute tool can call any GravityZone method, including isolation, kill process, remote command execution and deletes, gated only by your AI app's approval prompt. For an agent that should only read, set BITDEFENDER_MCP_READ_ONLY=1 in the MCP server's environment: bitdefender_execute then runs only GravityZone read methods, refuses every write and the Credential-tier reads, and the import tool is hidden. Incident reads stay allowed, and XDR evidence can hold connection headers captured on the endpoint, so treat incident output as sensitive. Keep the approval prompt on.

### Why does fleet changes say it has nothing to compare?

It diffs saved snapshots, and each fleet health run saves one (unless you pass --no-save). Run fleet health at least twice, for example daily, and fleet changes reports endpoints added or removed, managed/unmanaged flips, newly outdated agents, policies no longer applied and isolation changes between them.

### Does it see endpoints from GravityZone's Active Directory integration?

Not in the fleet commands. They read getEndpointsList, which returns the Computers and Groups inventory; Bitdefender documents that endpoints living only under the Active Directory integration come from getNetworkInventoryItems instead. So fleet health counts, --fail-on and fleet rmm-gap cover Computers and Groups, and the fleet health, locate, rmm-gap and changes results carry an inventory_scope field that says so. Before deploying an agent to a device rmm-gap lists as missing, check it with inventory list (a Credential-tier read, because its company rows carry license keys).

### Does it work with on-premises GravityZone?

It speaks the same JSON-RPC Public API, so set BITDEFENDER_BASE_URL to your Control Center's Access URL. It has not yet been verified against a live on-premises console.

### Does it replace GravityZone Control Center?

No. Control Center stays best for policy authoring, investigations and day-to-day console work. This skill answers the across-every-customer questions and runs scripted actions from your AI agent.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Blumira](/skills/blumira/) · [CIPP + Microsoft 365](/skills/cipp/) · [Cisco Duo](/skills/cisco-duo/) · [ConnectSecure](/skills/connectsecure/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [DNSFilter](/skills/dnsfilter/) · [Fortinet](/skills/fortinet/) · [Google Workspace](/skills/google-workspace/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [SonicWall](/skills/sonicwall/) · [Sophos Central](/skills/sophos-central/) · [ThreatDown](/skills/threatdown/) · [ThreatLocker](/skills/threatlocker/) · [WatchGuard Cloud](/skills/watchguard/)

## Status

Beta. Validated against the Bitdefender GravityZone API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
