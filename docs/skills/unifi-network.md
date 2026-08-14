---
layout: default
title: "UniFi Network MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every UniFi Network API operation, plus drift detection, topology, and rule prediction no other UniFi tool has."
permalink: /skills/unifi-network/
skill_name: "UniFi Network MCP"
image: /assets/social/unifi-network/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for UniFi Network?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for UniFi Network, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the UniFi Network MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local UniFi MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my UniFi data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local, and the gateway itself is on your own network - nothing routes through a vendor cloud. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Does this use the UniFi Site Manager cloud API?"
    a: "No. This skill talks to the local Network integration API on a self-hosted UniFi OS gateway, reached at https://<gateway>/proxy/network with an API key you mint in the gateway's own UI. It is a single-gateway tool: one CLI instance sees one controller, not a multi-tenant view across every deployment. UniFi Protect (cameras) and UniFi Access (doors) are separate APIs and are not covered."
  - q: "Why do the drift and newcomer commands report nothing on the first run?"
    a: "Both maintain their own baseline because the API offers no history to read. The first run for a site captures the current state as the baseline and reports no changes - that is expected, not an error. From the second run on, they report what moved since the previous run. Run `unifi-network-cli sync` before each check so the mirror is current."
  - q: "Can I trust rule-predict before making a firewall change?"
    a: "Treat it as a local simulation, not a live trace. It walks the last synced firewall policies in the same ascending-index, first-match-wins order the gateway uses, and matches on source and destination IP only - `--port` is echoed for reference and is not used for matching. Pass host IPs rather than CIDRs: a CIDR is collapsed to the network's first address, so `--src 10.0.3.0/24` predicts only for `10.0.3.0` and would miss a policy matching `10.0.3.50`. Zone-wide policies and traffic-matching-list references it cannot resolve are flagged as uncertain rather than silently assumed. Sync first, and confirm the real change in the console."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/unifi-network/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/unifi-network/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your UniFi Network credentials once; unifi-network-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a UniFi Network question in plain language; it runs unifi-network-cli for you."
---

# The UniFi Network MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Ubiquiti Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for UniFi Network. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects UniFi Network to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

UniFi Network plus your AI answers the gateway questions the console makes you reconstruct by hand: what actually changed in this site's config since you last looked, what hardware joined the network this week, and whether there is PoE and port headroom before you hang another AP. It syncs the gateway to a local SQLite mirror, so change-over-time questions the integration API cannot answer become one offline command.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/unifi-network){: .btn}

## Instead of clicking through UniFi Network, just ask

**Instead of** Clicking through Settings screen by screen trying to remember what the firewall and VLAN config looked like last week, because the integration API exposes no config history
**just ask:** *"What changed on this UniFi site since my last check?"*
<sub>Your agent runs: <code>unifi-network-cli drift --site default --json</code></sub>

**Instead of** Scrolling the client list looking for hardware you do not recognise, with no way to tell what is new versus what has always been there
**just ask:** *"What devices and clients joined this network in the last week?"*
<sub>Your agent runs: <code>unifi-network-cli newcomer --since 7d --json</code></sub>

**Instead of** Opening each switch in turn and counting free ports and checking which ones already energize PoE across a stack before adding a camera or an AP
**just ask:** *"Do I have port and PoE headroom on this site?"*
<sub>Your agent runs: <code>unifi-network-cli port-audit --site default --json</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/unifi-network/wide-1200x630.png" src="/assets/video/unifi-network/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/unifi-network/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What changed in this site's config since my last check? | `unifi-network-cli drift --site default --json` |
| What devices and clients joined the network this week? | `unifi-network-cli newcomer --since 7d --json` |
| Do I have switch port and PoE headroom before adding hardware? | `unifi-network-cli port-audit --site default --json` |
| Which firewall policy would match traffic from this host? | `unifi-network-cli rule-predict --src 10.0.3.50 --dst 10.0.0.1` |
| Which clients are sitting behind which device? | `unifi-network-cli topology --site default` |
| What firewall policies are configured on this site? | `unifi-network-cli sites firewall get-policies <siteId>` |
| Who is on the guest network, and which vouchers are live? | `unifi-network-cli guest report --site default` |
| List every adopted device on the site | `unifi-network-cli sites devices get-adopted-overview-page <siteId>` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/unifi-network/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/unifi-network/guide.md).

## What makes this one different

Most UniFi integrations proxy each question straight into a live API call. That works for reading one record and falls apart for anything historical, because the integration API has no config-versioning or audit-trail endpoint to proxy to - the data simply is not there to ask for. This skill syncs the gateway into a local SQLite mirror and keeps its own snapshots, so `drift` can diff this site's config against the last time you looked and `newcomer` can hold a first-seen record per device and client. Those answers are computed locally from state the API never returns twice.

The UniFi console stays the system of record and the place you make changes. This skill adds what the console does not: a terminal-and-agent surface where config drift, first-seen hardware, port and PoE headroom, and firewall-match prediction are single commands an AI can run, and where the answer arrives as JSON instead of a screen you have to read.

## The pain this closes

- The UniFi Network integration API exposes no config-versioning or audit-trail endpoint, so 'what changed on this site, and when?' has no answer you can pull - a gap the Ubiquiti Community has open feature requests for under titles like 'UniFi Change Logs or Change Control options?' and 'Audit log of recent changes'.
- Spotting new hardware on a network means eyeballing a client list you have no baseline for - there is no first-seen record, so nothing distinguishes a device that appeared this morning from one that has been there for a year.
- Port and PoE status lives one device-detail screen at a time; the list endpoints do not return per-port interface data at all, so checking free ports before planning a new AP or camera means opening every switch by hand.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/unifi-network/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/unifi-network/install.ps1 | iex
```

After install, authenticate once with your UniFi Network credentials, then verify with `unifi-network-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | drift, newcomer, topology, port-audit, rule-predict, analytics, sites devices get-adopted-overview-page, sites firewall get-policies (non-mutating reads except the secret-returning ones below; drift and newcomer advance their own local baseline each run) | Allow |
| Credential (incl. secret-returning reads) | auth set-token, auth logout, sites wifi get-broadcast-details (that SSID's cleartext passphrase), sites hotspot get-voucher, sites hotspot get-vouchers, guest report, search (usable guest voucher codes) | Human-in-the-loop only - never in a blanket allow-all-reads policy |
| Write (routine) | sites firewall create-policy, sites firewall update-policy, sites acl-rules create, sites networks create, sites wifi update-broadcast, sites dns create-policy, sites hotspot create-vouchers | Preview with --dry-run, then a reviewed write |
| Device / port control | sites devices adopt, sites devices execute-adopted-action, sites devices execute-port-action, sites clients execute-connected-action | Human-in-the-loop only - these take physical effect on the network |
| Destructive / config | sites devices remove (factory-resets an online device), sites firewall delete-zone, sites networks delete, sites acl-rules delete, sites dns delete-policy, sites wifi delete-broadcast | Human-in-the-loop only, explicit confirmation |

Most read commands - the local-mirror views, reports, and the non-mutating site endpoints - change nothing on the gateway and are safe to let an agent run. Two exceptions matter. Several reads return live secrets (the WiFi detail read includes that SSID's cleartext passphrase; guest report, search, and the hotspot reads return usable voucher codes) and the CLI does not redact output, so those belong in the credential tier rather than a blanket allow-all-reads policy. Writes are grouped by blast radius: routine config writes should be previewed with --dry-run and approved. Two tiers should never run unattended: device and port control, where one command can power-cycle a PoE port or kick a client off the network, and destructive commands, where `sites devices remove` factory-resets an online device. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/unifi-network/governance.md).

## Frequently asked questions

### Is there an MCP server for UniFi Network?

Yes - this one. A free, open source MCP server and Claude Code Skill for UniFi Network, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the UniFi Network MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local UniFi MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my UniFi data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local, and the gateway itself is on your own network - nothing routes through a vendor cloud. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Does this use the UniFi Site Manager cloud API?

No. This skill talks to the local Network integration API on a self-hosted UniFi OS gateway, reached at https://<gateway>/proxy/network with an API key you mint in the gateway's own UI. It is a single-gateway tool: one CLI instance sees one controller, not a multi-tenant view across every deployment. UniFi Protect (cameras) and UniFi Access (doors) are separate APIs and are not covered.

### Why do the drift and newcomer commands report nothing on the first run?

Both maintain their own baseline because the API offers no history to read. The first run for a site captures the current state as the baseline and reports no changes - that is expected, not an error. From the second run on, they report what moved since the previous run. Run `unifi-network-cli sync` before each check so the mirror is current.

### Can I trust rule-predict before making a firewall change?

Treat it as a local simulation, not a live trace. It walks the last synced firewall policies in the same ascending-index, first-match-wins order the gateway uses, and matches on source and destination IP only - `--port` is echoed for reference and is not used for matching. Pass host IPs rather than CIDRs: a CIDR is collapsed to the network's first address, so `--src 10.0.3.0/24` predicts only for `10.0.3.0` and would miss a policy matching `10.0.3.50`. Zone-wide policies and traffic-matching-list references it cannot resolve are flagged as uncertain rather than silently assumed. Sync first, and confirm the real change in the console.


## More Network Monitoring connectors

Run more than one Network Monitoring tool, or comparing options? These connectors work the same way: [Domotz](/skills/domotz/)

## Status

Beta. Validated against the UniFi Network API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
