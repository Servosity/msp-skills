---
layout: default
title: "Cisco Meraki MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Meraki Dashboard operation, plus cross-org fleet views, complete pagination, and a local mirror built for MSPs running many client orgs."
permalink: /skills/cisco-meraki/
skill_name: "Cisco Meraki MCP"
image: /assets/social/cisco-meraki/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Cisco Meraki?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Cisco Meraki, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Cisco Meraki MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CISCO_MERAKI_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CISCO_MERAKI_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and cisco-meraki-mcp speaks HTTP natively: set CISCO_MERAKI_MCP_HTTP_TOKEN, run `cisco-meraki-mcp --transport http`, and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your Meraki API key once."
  - q: "Is my Cisco Meraki data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the optional local mirror all run locally and talk only to the Meraki Dashboard API. The AI sees query results, not raw bulk data, and your API key is never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "What can the API key reach?"
    a: "A Meraki API key carries the Dashboard permissions of the admin account that generated it, across every organization that admin can see. Generate it from an admin with read-only access if you only want reporting. Each org must also have Dashboard API access enabled (Organization > Settings); an org without it shows up in `fleet health` as reachable: false rather than disappearing."
  - q: "Will this hit my Meraki API rate limit?"
    a: "Meraki allows about ten calls per second per organization. The CLI paces itself at 10 requests per second by default (change it with --rate-limit) and, when Meraki answers HTTP 429, waits for the Retry-After header before trying again. Fleet commands across many orgs take longer; they do not fail."
  - q: "Does it work with the Canada, China, India, or FedRAMP dashboards?"
    a: "Yes. Set CISCO_MERAKI_BASE_URL to your region's API host, for example https://api.meraki.ca/api/v1 (Canada), https://api.meraki.cn/api/v1 (China), https://api.meraki.in/api/v1 (India), or https://api.gov-meraki.com/api/v1 (FedRAMP). Leave it unset for the global dashboard."
  - q: "Can an agent change my clients' networks by accident?"
    a: "Writes send immediately when called, with no confirmation prompt, so keep autonomous agents to reads and --dry-run previews. The bulk tool, fleet push, previews a per-network diff by default and sends nothing unless you add --apply. See the safety model for which commands to keep behind a human. Set CISCO_MERAKI_MCP_READ_ONLY=1 on the MCP server and it refuses every write outright."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Cisco Meraki credentials once, then run cisco-meraki-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Cisco Meraki question in plain language; it runs cisco-meraki-cli for you."
---

# The Cisco Meraki MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Cisco Systems, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Cisco Meraki. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Cisco Meraki to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Cisco Meraki plus your AI answers the questions that span every client org at once: what is down right now and since when, which licenses and hardware hit their end date this quarter, who changed what across all orgs this week, and which change landed just before a site dropped. One API key, every org it can reach, every page read, and an org that fails to answer is named instead of silently counted as zero.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/cisco-meraki){: .btn}

## Instead of clicking through Cisco Meraki, just ask

**Instead of** Opening the MSP Portal, spotting an org with a red device count, then clicking into that org and its networks to find which device is offline and how long it has been down - once per client
**just ask:** *"What is down across all my Meraki clients right now?"*
<sub>Your agent runs: <code>cisco-meraki-cli fleet down --agent</code></sub>

**Instead of** Checking each org's license page by hand, because the portal shows only one co-term date per org and nothing for per-device, subscription, or end-of-support hardware, before the renewal quotes are due
**just ask:** *"Which Meraki licenses, subscriptions, and end-of-support dates hit in the next 90 days?"*
<sub>Your agent runs: <code>cisco-meraki-cli fleet renewals --within 90d --agent</code></sub>

**Instead of** Scrolling one org's change log against the outage time after a site goes down, trying to work out whether a tech's change caused it
**just ask:** *"Did someone change something right before this switch went offline?"*
<sub>Your agent runs: <code>cisco-meraki-cli fleet blame "Q2QN-9J8L-SLPD" --window 2h --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/cisco-meraki/wide-1200x630.png" src="/assets/video/cisco-meraki/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/cisco-meraki/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which client org is in the worst shape right now? | `cisco-meraki-cli fleet health --agent` |
| What is offline or alerting across every org, and for how long? | `cisco-meraki-cli fleet down --agent` |
| Which MX WAN uplinks have failed? | `cisco-meraki-cli fleet down --kind uplink --json` |
| Which licenses, subscriptions, and end-of-support dates are due in 90 days? | `cisco-meraki-cli fleet renewals --within 90d --agent` |
| Who changed what across all orgs this week? | `cisco-meraki-cli fleet changes --since 7d --group-by admin --agent` |
| Which org and network owns this MAC, IP, or serial? | `cisco-meraki-cli fleet find "00:18:0a:12:34:56" --agent` |
| Which admins have two-factor off or have not signed in for 90 days? | `cisco-meraki-cli fleet admins --stale 90d --flagged --agent` |
| A tech left - which orgs still grant them access? | `cisco-meraki-cli fleet admins --email "departed.tech@msp.example" --json` |
| Which devices flapped or were down longest this week? | `cisco-meraki-cli fleet flaps --since 7d --agent` |
| Point every managed network at the new syslog server - show me the diff first | `cisco-meraki-cli fleet push --path /networks/{networkId}/syslogServers --body syslog.json --network-tag managed` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/cisco-meraki/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/cisco-meraki/guide.md).

## What makes this one different

Most Meraki integrations, including the many single-org Meraki MCP servers, proxy one question into one live API call against one organization, and often read only the first page of a list. This connector is built for the MSP case: the fleet commands run across every org the key can see under one rate limiter, follow every Link-header page, and keep an org that fails to answer in the result as unreachable instead of reporting zero devices. It also wraps all 1,035 Dashboard operations as typed commands, so the fleet views are a starting point, not the ceiling.

The Meraki Dashboard and its MSP Portal stay the place you configure networks; the portal, as Meraki documents it, shows per-org device counts and one co-term date. This adds the cross-org answers on top: a single list of what is down with network names and downtime, renewals across all three licensing models plus end-of-support hardware, one change log for every org, admin two-factor and offboarding checks, and the config changes made just before an outage.

## The pain this closes

- Meraki's MSP Portal lists each organization with device counts, license status, and one co-terminated license expiration date (Meraki documentation, 'Monitoring and Managing Multiple Organizations'). What it documents is per-org counts: no cross-org list of which devices are down, no cross-org change log, no admin management across orgs. Answering 'what is down for every client' still means clicking into each org.
- Meraki's paginated list endpoints return a bare JSON array and page only through an RFC 5988 Link header (ten items per page by default on many of them), so a script that reads one response silently stops at page one. The Meraki Community Developers & APIs board has threads such as 'Using Powershell how to paginate thru pages' and 'Meraki API - Get Organization Clients Search - Only return 3 networks' working through exactly that.
- Renewals follow three licensing models - co-termination, per-device, and subscription - plus end-of-sale and end-of-support dates on hardware, so one date-sorted list of what is due across all clients does not exist in the Dashboard.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cisco-meraki/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cisco-meraki/install.ps1 | iex
```

After install, authenticate once with your Cisco Meraki credentials, then verify with `cisco-meraki-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet health, fleet down, fleet renewals, fleet changes, fleet find, fleet fanout (GET-only), fleet admins, fleet flaps, fleet blame, fleet push without --apply (preview; see the credential tier), sync, search, and every GET command except the secret-bearing ones below | Allow |
| Credential (incl. secret-returning reads) | auth set-token, auth logout, administered generate-identities-me-api-keys, administered revoke-identities-me-api-keys, every organizations admins, organizations saml and organizations saml-roles write, and the reads whose output carries a live secret (the CLI redacts nothing): networks wireless get-network-ssid / get-network-ssids (Wi-Fi psk), networks wireless get-network-ssid-identity-psk / get-network-ssid-identity-psks (passphrase), networks camera get-network-wireless-profile / get-network-wireless-profiles, networks snmp get-network, organizations snmp get-organization (community strings), organizations appliance get-organization-vpn-third-party-vpnpeers (IPsec secret), networks appliance get-network-vpn-bgp, networks switch get-network-routing-ospf, devices cellular get-device-sims, plus fleet fanout pointed at an org-scoped one of those paths and a fleet push preview whose --body names a secret key | Human-in-the-loop only - never in a blanket allow-all-reads policy, and do not hand the raw output to a model |
| Write (routine) | networks update, networks wireless update-network-ssid, networks appliance update-network-vlan, devices update, networks snmp update-network, networks webhooks create-network-http-server, import | Preview with --dry-run, then a reviewed write |
| Bulk and device-disrupting | fleet push --apply (one change to many networks across orgs), devices reboot device, devices switch cycle-device-ports, devices live-tools create-device-ports-cycle, devices live-tools create-device-cable-test, networks sm reboot-network-devices, networks sm shutdown-network-devices, networks appliance swap-network-warm-spare, networks firmware-upgrades update-network | Human-in-the-loop only - these take effect on live client networks at once |
| Destructive | every delete command (organizations delete, networks delete, networks appliance delete-network-vlan, ...), networks devices remove-network, organizations inventory release-from-organization, networks unbind network, networks split network, organizations networks combine-organization, networks sm wipe-network-devices, networks sm unenroll-network-device, organizations licenses move-organization | Human-in-the-loop only, explicit confirmation |

Reads - the fleet views, every GET, sync and search - change nothing in the Dashboard and are safe for an agent, with one exception: a dozen reads return live secrets (SSID pre-shared keys, identity PSK passphrases, SNMP community strings, site-to-site VPN peer secrets, BGP and OSPF passwords) and the CLI does not redact them, so those belong behind a human. Routine config writes should be previewed with --dry-run and approved. Bulk pushes, reboots and port cycles, admin and SAML changes, API key generation, and deletes or unclaims should never run unattended. For an MCP agent that should never write, CISCO_MERAKI_MCP_READ_ONLY=1 makes the server refuse every non-GET call. The real ceiling is the API key: it can do whatever the Dashboard admin who generated it can do. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/cisco-meraki/governance.md).

## Frequently asked questions

### Is there an MCP server for Cisco Meraki?

Yes - this one. A free, open source MCP server and Claude Code Skill for Cisco Meraki, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Cisco Meraki MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CISCO_MERAKI_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CISCO_MERAKI_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and cisco-meraki-mcp speaks HTTP natively: set CISCO_MERAKI_MCP_HTTP_TOKEN, run `cisco-meraki-mcp --transport http`, and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your Meraki API key once.

### Is my Cisco Meraki data safe?

Your data stays on your machine. The CLI, MCP server, and the optional local mirror all run locally and talk only to the Meraki Dashboard API. The AI sees query results, not raw bulk data, and your API key is never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What can the API key reach?

A Meraki API key carries the Dashboard permissions of the admin account that generated it, across every organization that admin can see. Generate it from an admin with read-only access if you only want reporting. Each org must also have Dashboard API access enabled (Organization > Settings); an org without it shows up in `fleet health` as reachable: false rather than disappearing.

### Will this hit my Meraki API rate limit?

Meraki allows about ten calls per second per organization. The CLI paces itself at 10 requests per second by default (change it with --rate-limit) and, when Meraki answers HTTP 429, waits for the Retry-After header before trying again. Fleet commands across many orgs take longer; they do not fail.

### Does it work with the Canada, China, India, or FedRAMP dashboards?

Yes. Set CISCO_MERAKI_BASE_URL to your region's API host, for example https://api.meraki.ca/api/v1 (Canada), https://api.meraki.cn/api/v1 (China), https://api.meraki.in/api/v1 (India), or https://api.gov-meraki.com/api/v1 (FedRAMP). Leave it unset for the global dashboard.

### Can an agent change my clients' networks by accident?

Writes send immediately when called, with no confirmation prompt, so keep autonomous agents to reads and --dry-run previews. The bulk tool, fleet push, previews a per-network diff by default and sends nothing unless you add --apply. See the safety model for which commands to keep behind a human. Set CISCO_MERAKI_MCP_READ_ONLY=1 on the MCP server and it refuses every write outright.


## More Network Monitoring connectors

Run more than one Network Monitoring tool, or comparing options? These connectors work the same way: [Domotz](/skills/domotz/) · [UniFi Network](/skills/unifi-network/)

## Status

Beta. Validated against the Cisco Meraki API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
