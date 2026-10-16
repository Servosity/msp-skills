---
layout: default
title: "WatchGuard Cloud MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "WatchGuard Cloud's Firebox, FireCloud, Endpoint Security, ThreatSync, NDR, AuthPoint and account platform APIs in one CLI and MCP server, with cross-customer reports for Service Providers."
permalink: /skills/watchguard/
skill_name: "WatchGuard Cloud MCP"
image: /assets/social/watchguard/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for WatchGuard Cloud?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for WatchGuard Cloud, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the WatchGuard Cloud MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `WATCHGUARD_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `WATCHGUARD_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay on your machine, but not only in your environment: the token the CLI mints for your own account is cached in its credentials.toml (mode 0600), and `auth login` given `--client-id` or `--client-secret` also saves that Access ID and password there, so removing the environment variables does not revoke access - run `watchguard-cli auth logout` or click Revoke Access under Administration > Managed Access in WatchGuard Cloud. Per-customer tokens stay in memory and are never written to disk. Apart from those paths, whatever your agent reads goes to your AI provider as part of the conversation, including the secrets that credential reads return (see the safety tiers), and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you run the WatchGuard MCP server in its built-in HTTP mode with a bearer token (WATCHGUARD_MCP_HTTP_TOKEN) and put it behind an HTTPS tunnel or reverse proxy. No bridge package is needed. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my WatchGuard data safe?"
    a: "The CLI, MCP server and local mirror run on your machine, and MSP Skills never bundles or transmits your WatchGuard credentials. What your agent reads does go to your AI provider: query results, and the secrets that credential reads return (BOVPN pre-shared keys, private keys, Firebox passwords, activation and license keys). Keep those commands human-in-the-loop as the safety tiers describe."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "What credentials do I need?"
    a: "WatchGuard Cloud API access: as an Owner or Administrator, open Administration > Managed Access and click Enable API Access (a Service Provider does this from Subscriber view of its own account). You get a read-write Access ID, a read-only Access ID, the passwords you set for each, and an API key. Set WATCHGUARD_CLIENT_ID to an Access ID, WATCHGUARD_CLIENT_SECRET to its password, WATCHGUARD_API_KEY to the API key and WATCHGUARD_ACCOUNT_ID to your account ID (ACC-... or WGC-...), then run watchguard-cli doctor. For a reporting-only agent, use the read-only Access ID."
  - q: "Does it work across all my customers, or one account at a time?"
    a: "Across all of them, with a Service Provider account. accounts health, endpoint-devices posture and endpoint-patches exposure list the accounts you manage, mint a separate token for each one and report every customer in one result, naming any account they could not reach. threatsync-incidents queue reads incidents for all your managed accounts in one ThreatSync call. If any read fails, including a product a customer does not have, these commands exit 6 (5 when nothing could be read) and say which account and product; accounts health lists that account first with complete: false instead of showing zeros, and an account it could not reach at all appears in fetch_failures. To work on one customer, set WATCHGUARD_AUDIENCE to that customer's audience value (watchguard-cli audiences --account-id <customer-account-id> returns it) and WATCHGUARD_ACCOUNT_ID to that customer's account ID. That is a convenience, not a security boundary: WatchGuard's audience token also works on your own account."
  - q: "My WatchGuard Cloud account is in Europe or Asia Pacific. Does it work?"
    a: "Yes. Set WATCHGUARD_BASE_URL and WATCHGUARD_TOKEN_URL to your region's gateway: https://api.deu.cloud.watchguard.com for EMEA or https://api.jpn.cloud.watchguard.com for APAC, with /oauth/token on the end for the token URL. The default is the North America gateway, api.usa.cloud.watchguard.com. Managed Access in WatchGuard Cloud shows your account's API URL as https://api.<region>.cloud.watchguard.com/rest/; drop the /rest/ when you set WATCHGUARD_BASE_URL (the CLI adds it to every path)."
  - q: "Will this hit my WatchGuard API rate limits?"
    a: "WatchGuard allows 500 requests per second and 200,000 requests per day per API key; a 429 means the day's quota is spent until 00:00 UTC. accounts health, endpoint-devices posture and endpoint-patches exposure fetch at most four accounts in parallel by default (--concurrency), the cross-customer commands stop at --max-pages, --rate-limit caps requests per second, GET responses are cached for 5 minutes, and sync keeps a local copy that search and analytics read without calling the API."
  - q: "Can my AI isolate devices or change firewall policy?"
    a: "Only if you let it. Write commands such as endpoint-devices isolate, endpoint-devices uninstall, firebox-deployments and the delete commands send as soon as they run; there is no confirmation prompt. Preview any of them with --dry-run first; through the MCP server there is no preview, watchguard_execute sends a write as soon as it is called. For a reporting agent, use the read-only Access ID, or set WATCHGUARD_MCP_READ_ONLY=1 so the MCP server drops its write tools and refuses every non-GET call through watchguard_execute."
  - q: "Has it been tested against a live WatchGuard Cloud account?"
    a: "Not yet. Every command was built from WatchGuard's published OpenAPI specifications and tested against a local mock of those APIs, not a real account. If you run it against your own WatchGuard Cloud account, tell us what worked or broke on the GitHub issue tracker or at a Build Session; a real MSP run is what earns the live-verified badge."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/watchguard/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your WatchGuard Cloud credentials once, then run watchguard-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a WatchGuard Cloud question in plain language; it runs watchguard-cli for you."
---

# The WatchGuard Cloud MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by WatchGuard Technologies, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for WatchGuard Cloud. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects WatchGuard Cloud to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "which customer needs attention first?" and get one answer across Firebox, Endpoint Security, ThreatSync and license allocation for every account you manage in WatchGuard Cloud. Each product is a separate WatchGuard API, and Endpoint Security answers for a customer only with a token minted for that customer. This skill mints those tokens for you, fans out across your customers and ranks what needs you.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/watchguard){: .btn}

## Instead of clicking through WatchGuard Cloud, just ask

**Instead of** Opening each customer in WatchGuard Cloud to check offline Fireboxes, unprotected endpoints, open ThreatSync incidents and expiring licenses, one product view at a time
**just ask:** *"Which customer needs my attention first?"*
<sub>Your agent runs: <code>watchguard-cli accounts health --agent</code></sub>

**Instead of** Switching into every subscriber account to find computers that are unprotected, out of date, isolated or waiting on a reboot
**just ask:** *"Which endpoints across my customers are unprotected or out of date?"*
<sub>Your agent runs: <code>watchguard-cli endpoint-devices posture --agent</code></sub>

**Instead of** Refreshing the deployment status in WatchGuard Cloud after pushing a Firebox configuration change
**just ask:** *"Tell me when the deployment to FB-12345 finishes, and whether it failed."*
<sub>Your agent runs: <code>watchguard-cli firebox-transactions wait --device FB-12345 --max-wait 10m</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/watchguard/wide-1200x630.png" src="/assets/video/watchguard/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/watchguard/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which customer needs attention first across Firebox, endpoints, ThreatSync and licenses? | `watchguard-cli accounts health --agent` |
| Which endpoints are unprotected, out of date, isolated or waiting on a reboot, across every customer? | `watchguard-cli endpoint-devices posture --agent` |
| Which new ThreatSync incidents from the last 30 days need triage first, and which has nobody touched? | `watchguard-cli threatsync-incidents queue --status new --duration 30D --agent` |
| Which customers and computers are exposed to this CVE? | `watchguard-cli endpoint-patches exposure --cve CVE-2024-38063 --agent` |
| Which Fireboxes and access points went offline, came back or changed firmware since the last sync? | `watchguard-cli firebox-devices changes --agent` |
| Which Fireboxes are running older firmware than the rest of the same model in my fleet? | `watchguard-cli firebox-devices firmware-lag --agent` |
| Which allocations don't match the Fireboxes that actually report in, and what expires this month? | `watchguard-cli allocations reconcile --expiring-within 30d --agent` |
| Did the Firebox deployment finish, or fail? | `watchguard-cli firebox-transactions wait --device FB-12345 --max-wait 10m` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/watchguard/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/watchguard/guide.md).

## What makes this one different

The only other MCP server for WatchGuard Cloud we found on GitHub as of October 2026, [WYRE-AI/watchguard-cloud-mcp](https://github.com/WYRE-AI/watchguard-cloud-mcp), covers the platform's account, activation and operator APIs and lists Firebox and Endpoint Security as out of scope for now. This skill covers 208 operations from 13 of WatchGuard's published API specifications (Firebox Management and Reports, FireCloud, Endpoint Security, ThreatSync, NDR assets and Smart Alerts, AuthPoint authentication, and the platform's accounts, operators, activations, license allocation and authorization APIs), mints a separate token for each managed account where WatchGuard requires one, so one Service Provider credential reaches every customer, and keeps a local SQLite mirror so `firebox-devices changes` and `firmware-lag` can compare your fleet over time.

WatchGuard Cloud stays where you configure Fireboxes, policies and endpoint security, and ThreatSync keeps doing its own correlation. This skill complements them by answering cross-customer, cross-product questions from the AI agent you already use, through WatchGuard's public API, in one command.

## The pain this closes

- WatchGuard Endpoint Security answers for a managed customer only with an access token minted for that customer's audience value, which you look up per account first (WatchGuard API docs, Authentication). A device-level question about every customer is a token request and at least one call per customer.
- Firebox Management, Endpoint Security, ThreatSync, NDR, AuthPoint and license allocation are separate APIs with their own paths, paging and status codes, so a customer review spans many screens or many scripts even where an API can list every customer at once.
- Allocations and the Fireboxes that actually report in drift apart. A WatchGuard Community thread (May 2023, still open in September 2024) shows retired, unallocated Fireboxes staying in the inventory and inflating the device count, with a support case as the only way out.
- A Firebox deployment or certificate install is asynchronous: the API hands back a transaction ID and you poll it to learn whether the change landed or failed.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/watchguard/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/watchguard/install.ps1 | iex
```

After install, authenticate once with your WatchGuard Cloud credentials, then verify with `watchguard-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | accounts health, endpoint-devices posture, threatsync-incidents queue, endpoint-patches exposure, allocations reconcile, firebox-devices changes and firmware-lag, firebox-transactions wait, sync, search, analytics, and every list / get command except the credential reads under Credential and admin | Allow |
| Endpoint and incident actions | endpoint-devices isolate / stop-isolation / scan / action / uninstall, endpoint-configurations assign, threatsync-actions perform, threatsync-incident-actions perform, threatsync-transactions, threatsync-incidents set-status, ndr-smart-alerts close, authpoint push | Human-in-the-loop, explicit confirmation |
| Firewall config and deployment | firebox-deployments, firebox-policies create / update / set-enabled, the Firebox and FireCloud exception creates and updates, firebox-exceptions enable-global / disable-global, firebox-templates subscribe, firebox-bovpn-p1-settings create / update, firebox-transactions update, endpoint-risks update-config, ndr-assets create / update, threatsync-comments create / update, allocations allocate / update-hardware / update-software, accounts create / update | Preview with --dry-run, then a human-approved write |
| Destructive | accounts delete (it targets --account-id, which defaults to your own WATCHGUARD_ACCOUNT_ID), operators delete, allocations deallocate, firebox-transactions cancel, ndr-assets delete, firebox-policies delete, threatsync-comments delete, and every other delete command | Human-in-the-loop only, explicit confirmation |
| Credential and admin | firebox-bovpn-tunnels list / get and their writes (pre-shared keys), firebox-certificates list / get / create / install (private keys and passwords), firebox-networks, firebox-users and firebox-content-filtering list / get (PPPoE, Dynamic DNS, Wi-Fi and override passwords), activations list-recent / status and activate (activation keys), allocations list-owned / list-assigned / summary (serial numbers and license keys), export activations (activation keys), import, endpoint-installers (enrollment download links), operators create / update, audiences, authpoint check-policy / password / otp / qrcode / forgot-token / transaction-status, auth login, auth set-token | Human-in-the-loop only |

The skill is read-first: the cross-customer reports, sync, search and nearly every list and get command only read. The exceptions are the reads whose output carries a secret (BOVPN tunnels, Firebox certificates, networks, users, content filtering and the activation keys from activations), which belong with the credential commands; the response cache never stores them and the local mirror drops them (a software license key in an allocation row is replaced by a one-way fingerprint; Firebox serial numbers are kept). Every write sends immediately with no confirmation prompt, so preview with --dry-run and keep endpoint actions, deployments and deletes human-in-the-loop. The read-only Access ID is the outer limit for a reporting agent. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/watchguard/governance.md).

## Frequently asked questions

### Is there an MCP server for WatchGuard Cloud?

Yes - this one. A free, open source MCP server and Claude Code Skill for WatchGuard Cloud, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the WatchGuard Cloud MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `WATCHGUARD_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `WATCHGUARD_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay on your machine, but not only in your environment: the token the CLI mints for your own account is cached in its credentials.toml (mode 0600), and `auth login` given `--client-id` or `--client-secret` also saves that Access ID and password there, so removing the environment variables does not revoke access - run `watchguard-cli auth logout` or click Revoke Access under Administration > Managed Access in WatchGuard Cloud. Per-customer tokens stay in memory and are never written to disk. Apart from those paths, whatever your agent reads goes to your AI provider as part of the conversation, including the secrets that credential reads return (see the safety tiers), and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you run the WatchGuard MCP server in its built-in HTTP mode with a bearer token (WATCHGUARD_MCP_HTTP_TOKEN) and put it behind an HTTPS tunnel or reverse proxy. No bridge package is needed. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my WatchGuard data safe?

The CLI, MCP server and local mirror run on your machine, and MSP Skills never bundles or transmits your WatchGuard credentials. What your agent reads does go to your AI provider: query results, and the secrets that credential reads return (BOVPN pre-shared keys, private keys, Firebox passwords, activation and license keys). Keep those commands human-in-the-loop as the safety tiers describe.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What credentials do I need?

WatchGuard Cloud API access: as an Owner or Administrator, open Administration > Managed Access and click Enable API Access (a Service Provider does this from Subscriber view of its own account). You get a read-write Access ID, a read-only Access ID, the passwords you set for each, and an API key. Set WATCHGUARD_CLIENT_ID to an Access ID, WATCHGUARD_CLIENT_SECRET to its password, WATCHGUARD_API_KEY to the API key and WATCHGUARD_ACCOUNT_ID to your account ID (ACC-... or WGC-...), then run watchguard-cli doctor. For a reporting-only agent, use the read-only Access ID.

### Does it work across all my customers, or one account at a time?

Across all of them, with a Service Provider account. accounts health, endpoint-devices posture and endpoint-patches exposure list the accounts you manage, mint a separate token for each one and report every customer in one result, naming any account they could not reach. threatsync-incidents queue reads incidents for all your managed accounts in one ThreatSync call. If any read fails, including a product a customer does not have, these commands exit 6 (5 when nothing could be read) and say which account and product; accounts health lists that account first with complete: false instead of showing zeros, and an account it could not reach at all appears in fetch_failures. To work on one customer, set WATCHGUARD_AUDIENCE to that customer's audience value (watchguard-cli audiences --account-id <customer-account-id> returns it) and WATCHGUARD_ACCOUNT_ID to that customer's account ID. That is a convenience, not a security boundary: WatchGuard's audience token also works on your own account.

### My WatchGuard Cloud account is in Europe or Asia Pacific. Does it work?

Yes. Set WATCHGUARD_BASE_URL and WATCHGUARD_TOKEN_URL to your region's gateway: https://api.deu.cloud.watchguard.com for EMEA or https://api.jpn.cloud.watchguard.com for APAC, with /oauth/token on the end for the token URL. The default is the North America gateway, api.usa.cloud.watchguard.com. Managed Access in WatchGuard Cloud shows your account's API URL as https://api.<region>.cloud.watchguard.com/rest/; drop the /rest/ when you set WATCHGUARD_BASE_URL (the CLI adds it to every path).

### Will this hit my WatchGuard API rate limits?

WatchGuard allows 500 requests per second and 200,000 requests per day per API key; a 429 means the day's quota is spent until 00:00 UTC. accounts health, endpoint-devices posture and endpoint-patches exposure fetch at most four accounts in parallel by default (--concurrency), the cross-customer commands stop at --max-pages, --rate-limit caps requests per second, GET responses are cached for 5 minutes, and sync keeps a local copy that search and analytics read without calling the API.

### Can my AI isolate devices or change firewall policy?

Only if you let it. Write commands such as endpoint-devices isolate, endpoint-devices uninstall, firebox-deployments and the delete commands send as soon as they run; there is no confirmation prompt. Preview any of them with --dry-run first; through the MCP server there is no preview, watchguard_execute sends a write as soon as it is called. For a reporting agent, use the read-only Access ID, or set WATCHGUARD_MCP_READ_ONLY=1 so the MCP server drops its write tools and refuses every non-GET call through watchguard_execute.

### Has it been tested against a live WatchGuard Cloud account?

Not yet. Every command was built from WatchGuard's published OpenAPI specifications and tested against a local mock of those APIs, not a real account. If you run it against your own WatchGuard Cloud account, tell us what worked or broke on the GitHub issue tracker or at a Build Session; a real MSP run is what earns the live-verified badge.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Blumira](/skills/blumira/) · [CIPP + Microsoft 365](/skills/cipp/) · [Cisco Duo](/skills/cisco-duo/) · [ConnectSecure](/skills/connectsecure/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Fortinet](/skills/fortinet/) · [Google Workspace](/skills/google-workspace/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [Sophos Central](/skills/sophos-central/) · [ThreatDown](/skills/threatdown/) · [ThreatLocker](/skills/threatlocker/)

## Status

Beta. Validated against the WatchGuard Cloud API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
