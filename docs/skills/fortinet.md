---
layout: default
title: "Fortinet MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Read every FortiGate's configuration and live state, then audit the whole client fleet for firmware, SSL-VPN exposure, admin accounts and license expiry in one command."
permalink: /skills/fortinet/
skill_name: "Fortinet MCP"
image: /assets/social/fortinet/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Fortinet?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Fortinet, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Fortinet MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `FORTINET_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `FORTINET_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and fortinet-mcp speaks HTTP natively: run `fortinet-mcp --transport http` with FORTINET_MCP_HTTP_TOKEN set and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You create a read-only REST API admin on each FortiGate and enter its token once."
  - q: "Can it change my FortiGates?"
    a: "No. Every FortiOS call is a read of configuration tables or monitor state, the FortiManager commands send only the JSON-RPC get method, and the FortiGate Cloud commands only list devices. Stored secrets stay out of the answers too: every FortiOS-encrypted value (RADIUS secrets, IPsec keys, password hashes) comes back as ENC (redacted), and a request for plain-text passwords is refused. Give each token a read-only admin profile and the FortiGate itself enforces the rest."
  - q: "How do I check 40 client FortiGates, not just one?"
    a: "List them in a devices.toml file: one [[device]] entry per FortiGate with its URL, a token_file (a file holding that box's token; on macOS and Linux the CLI refuses it unless it is chmod 600) and an optional client name. `fortinet-cli fleet devices` shows which entries have a usable URL and token file, and every audit command then runs across the whole list. A devices file you name that is missing or lists no [[device]] entries is an error, never a silent fall-back to one box."
  - q: "My FortiGate still uses its factory certificate. Will it connect?"
    a: "Not by default: certificate checks stay on. The secure fix is a trusted certificate on the FortiGate (FortiOS can get one from Let's Encrypt). Otherwise opt in per device with insecure = true in devices.toml, pass --insecure, or add FORTINET_SKIP_TLS_VERIFY=true to the MCP server's env block in a JSON config (it also covers FortiManager). The one-click .mcpb has no field for it on purpose, because one ticked box would turn off verification for every device. A devices.toml entry with insecure = true covers the fleet, audit and policies hygiene tools there; the single-device tools and the fmg tools still need a trusted certificate (or the JSON config)."
  - q: "Is my Fortinet data safe?"
    a: "Your data stays on your machine. The CLI, MCP server and the local mirror are all local, and the MCP server runs over stdio unless you start it with --transport http. `fleet backup` writes full configuration files that contain encrypted secrets and private keys, so it creates them owner-only (chmod 600 on macOS and Linux) and you should keep that directory private. Everything else it returns has FortiOS-encrypted secrets redacted. Credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/fortinet/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/fortinet/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Fortinet credentials once, then run fortinet-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Fortinet question in plain language; it runs fortinet-cli for you."
---

# The Fortinet MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Fortinet, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Fortinet. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Fortinet to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI which client FortiGates are below the fixed firmware, which have SSL-VPN open to the internet, and which licenses lapse this quarter, and get one graded row per firewall. FortiOS answers one box at a time through its GUI or REST API. This reads every FortiGate in a devices file in one pass, read-only, and adds FortiManager and FortiGate Cloud inventory alongside.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/fortinet){: .btn}

## Instead of clicking through Fortinet, just ask

**Instead of** Logging into every client's FortiGate after an SSL-VPN advisory to see whether SSL-VPN is on, which interface it listens on, and who can reach it
**just ask:** *"Which client FortiGates have SSL-VPN open to any source on a WAN interface?"*
<sub>Your agent runs: <code>fortinet-cli audit sslvpn --fail-on fail</code></sub>

**Instead of** Checking each box's firmware against the fixed release for its branch, one GUI dashboard at a time
**just ask:** *"Which FortiGates are below the fixed firmware for their branch?"*
<sub>Your agent runs: <code>fortinet-cli audit firmware --min-version 7.2.10,7.4.5</code></sub>

**Instead of** Keeping a spreadsheet of FortiGuard and FortiCare renewal dates per serial number
**just ask:** *"What Fortinet licenses expire in the next 60 days across all my clients?"*
<sub>Your agent runs: <code>fortinet-cli audit licenses --within 60d</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/fortinet/wide-1200x630.png" src="/assets/video/fortinet/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/fortinet/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Give me one posture verdict across every client FortiGate, showing only findings that are new since the last run. | `fortinet-cli fleet audit --new` |
| Which FortiGates are below my firmware floor, and how many patches behind their branch is each one? | `fortinet-cli audit firmware --min-version 7.2.10,7.4.5` |
| Where is SSL-VPN enabled on a WAN interface and open to any source address? | `fortinet-cli audit sslvpn` |
| Which admins have no trusted hosts or no two-factor, and which WAN interfaces allow HTTP or Telnet? | `fortinet-cli audit admins` |
| Which FortiGuard and FortiCare licenses expire in the next 60 days? | `fortinet-cli audit licenses --within 60d` |
| Which firewalls have HA out of sync, IPsec tunnels down, SD-WAN members down, or certificates expiring? | `fortinet-cli fleet health --cert-within 14d` |
| Which firewall policies are any-any accept, unused for 90 days, or not logging? | `fortinet-cli policies hygiene --unused-days 90` |
| What changed in each client's FortiGate configuration this week? | `fortinet-cli fleet changes --since 7d` |
| Which FortiManager-managed FortiGates are out of sync or have the management tunnel down? | `fortinet-cli fmg devices` |
| Which FortiGate Cloud devices have the management tunnel down or a contract ending? | `fortinet-cli cloud devices` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/fortinet/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/fortinet/guide.md).

## What makes this one different

[alpadalar/fortigate-mcp-server](https://github.com/alpadalar/fortigate-mcp-server) is a general-purpose FortiGate MCP server: a device registry, read tools, and create, update and delete tools for policies, VIPs and routes that stay off unless you enable writes. It answers about the device you ask about. This one is read-only by construction and built to grade the whole fleet in one pass: every FortiGate in a devices file is marked PASS, WARN, FAIL or UNKNOWN for firmware floors per branch, SSL-VPN exposure, admin accounts and license expiry, and a box it could not read is UNKNOWN, never PASS.

Fortinet's FortiAI-Assist runs inside FortiManager and FortiAnalyzer, and since FortiOS 8.0 on FortiGates with more than 2 GB of RAM and FortiCare Premium; it answers inside the console it runs in. This needs only a read-only REST API token per FortiGate, so it also covers the small boxes FortiAI-Assist does not, and it answers across every client's firewall at once.

## The pain this closes

- FortiOS SSL-VPN flaws are exploited fast: CVE-2024-21762 was being exploited before most MSPs had read the advisory, and the advice was to patch or disable SSL-VPN on every box.
- Firmware state lives on each FortiGate's dashboard. Knowing which clients are below the fixed build for their branch means logging into every one.
- FortiManager can push and report across the fleet, but many MSPs manage small-business FortiGates without one.
- FortiGuard and FortiCare expiry dates sit per device, so the renewal queue is a spreadsheet someone keeps by hand.
- Admin accounts without trusted hosts, super_admin without two-factor, and HTTP or Telnet on a WAN interface are each a separate screen to check.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/fortinet/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/fortinet/install.ps1 | iex
```

Create a REST API admin on each FortiGate (System > Administrators > Create New > REST API Admin, read-only profile, your IP as a trusted host). Set FORTINET_BASE_URL and FORTINET_API_TOKEN for one box, or list every box in a devices.toml file, then run `fortinet-cli doctor`: it makes one authenticated read to confirm the token works.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet audit, fleet health, fleet devices, fleet changes, audit firmware, audit sslvpn, audit admins, audit licenses, policies hygiene, fmg adoms/devices/status, cloud devices/device, every list/get, monitor and settings command (GET-only despite the name), sync, search, export. Every FortiOS-encrypted value comes back as ENC (redacted), and plain-text-password requests are refused. Local writes only: sync fills the local store, fleet audit records each run in its local history, and export --output overwrites the file you name | Allow |
| Sensitive read | fleet backup (each FortiGate's full configuration, including encrypted secrets and private keys, kept unredacted on purpose and written to owner-only files on macOS and Linux); administrators, api-users, local-users and monitor fortitokens (account and token inventory, secret values redacted) | Human-in-the-loop - keep the output private |
| Credential | auth set-token (writes the token to the CLI's credentials file), auth logout | Human-in-the-loop only |
| Data egress | --deliver webhook:<url> on any command (POSTs that command's output to a URL you name), and feedback --send or FORTINET_FEEDBACK_AUTO_SEND=true, which POST only when FORTINET_FEEDBACK_ENDPOINT names a URL you set | Human-in-the-loop - a webhook sink moves client data off your machine |

This connector is read-only against FortiOS, FortiManager and FortiGate Cloud: no command changes a firewall, and FortiOS-encrypted secrets are redacted from every read before an agent sees them. The strongest control is a REST API admin with a read-only admin profile and your IP as its only trusted host. The tiers that need a human move secrets or data: fleet backup writes full configurations including encrypted secrets, auth set-token stores a token locally, and a webhook delivery sends output off your machine. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/fortinet/governance.md).

## Frequently asked questions

### Is there an MCP server for Fortinet?

Yes - this one. A free, open source MCP server and Claude Code Skill for Fortinet, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Fortinet MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `FORTINET_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `FORTINET_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and fortinet-mcp speaks HTTP natively: run `fortinet-mcp --transport http` with FORTINET_MCP_HTTP_TOKEN set and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You create a read-only REST API admin on each FortiGate and enter its token once.

### Can it change my FortiGates?

No. Every FortiOS call is a read of configuration tables or monitor state, the FortiManager commands send only the JSON-RPC get method, and the FortiGate Cloud commands only list devices. Stored secrets stay out of the answers too: every FortiOS-encrypted value (RADIUS secrets, IPsec keys, password hashes) comes back as ENC (redacted), and a request for plain-text passwords is refused. Give each token a read-only admin profile and the FortiGate itself enforces the rest.

### How do I check 40 client FortiGates, not just one?

List them in a devices.toml file: one [[device]] entry per FortiGate with its URL, a token_file (a file holding that box's token; on macOS and Linux the CLI refuses it unless it is chmod 600) and an optional client name. `fortinet-cli fleet devices` shows which entries have a usable URL and token file, and every audit command then runs across the whole list. A devices file you name that is missing or lists no [[device]] entries is an error, never a silent fall-back to one box.

### My FortiGate still uses its factory certificate. Will it connect?

Not by default: certificate checks stay on. The secure fix is a trusted certificate on the FortiGate (FortiOS can get one from Let's Encrypt). Otherwise opt in per device with insecure = true in devices.toml, pass --insecure, or add FORTINET_SKIP_TLS_VERIFY=true to the MCP server's env block in a JSON config (it also covers FortiManager). The one-click .mcpb has no field for it on purpose, because one ticked box would turn off verification for every device. A devices.toml entry with insecure = true covers the fleet, audit and policies hygiene tools there; the single-device tools and the fmg tools still need a trusted certificate (or the JSON config).

### Is my Fortinet data safe?

Your data stays on your machine. The CLI, MCP server and the local mirror are all local, and the MCP server runs over stdio unless you start it with --transport http. `fleet backup` writes full configuration files that contain encrypted secrets and private keys, so it creates them owner-only (chmod 600 on macOS and Linux) and you should keep that directory private. Everything else it returns has FortiOS-encrypted secrets redacted. Credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Bitdefender GravityZone](/skills/bitdefender/) · [Blumira](/skills/blumira/) · [CIPP + Microsoft 365](/skills/cipp/) · [Cisco Duo](/skills/cisco-duo/) · [ConnectSecure](/skills/connectsecure/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [DNSFilter](/skills/dnsfilter/) · [Google Workspace](/skills/google-workspace/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [SonicWall](/skills/sonicwall/) · [Sophos Central](/skills/sophos-central/) · [ThreatDown](/skills/threatdown/) · [ThreatLocker](/skills/threatlocker/) · [WatchGuard Cloud](/skills/watchguard/)

## Status

Beta. Validated against the Fortinet API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
