---
layout: default
title: "SonicWall MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Cyber-insurance posture for a whole SonicWall fleet: firmware, advisories, MFA, SSL-VPN exposure and licences, one row per firewall."
permalink: /skills/sonicwall/
skill_name: "SonicWall MCP"
image: /assets/social/sonicwall/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for SonicWall?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for SonicWall, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the SonicWall MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SONICWALL_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `SONICWALL_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and sonicwall-mcp speaks HTTP natively: set SONICWALL_MCP_HTTP_TOKEN, run `sonicwall-mcp --transport http`, and serve its /mcp endpoint (http://127.0.0.1:7777/mcp by default) over HTTPS behind an authenticating proxy that signs ChatGPT in with OAuth and adds the bearer header the server requires. ChatGPT cannot send a fixed bearer token, so a bare tunnel gets HTTP 401. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter a MySonicWall API key once and run `sonicwall-cli nsm-login`."
  - q: "What credentials does it need?"
    a: "A MySonicWall API key (SONICWALL_MSW_API_KEY). `sonicwall-cli nsm-login` trades it for an NSM session token and saves it; the token lasts 8 hours with a 15-minute idle timeout, so run nsm-login again when NSM commands exit 4. EU portals also set SONICWALL_REGION=eucentral. Use a MySonicWall account whose NSM role is read-only."
  - q: "Can it change my SonicWall firewalls?"
    a: "No. It reads NSM, the firewalls through NSM's SonicOS passthrough, and MySonicWall. The only POSTs are read-style MySonicWall queries and the token exchanges (nsm-login, nsm-session, msw-tenants access-code). Secret fields SonicWall's own API schemas document, such as local-user and device passwords, registration and authentication codes, SNMP community names and MySonicWall API keys, are blanked before anything is printed, cached or handed to an agent."
  - q: "Will it see firewalls that are not in NSM?"
    a: "Not with posture, exposure, mfa-audit or firmware drift: those only see firewalls onboarded to NSM SaaS. Run `sonicwall-cli blind-spots` to list firewalls registered in MySonicWall that NSM cannot see, with the advisories for their registered firmware."
  - q: "Is my SonicWall data safe?"
    a: "Your data stays on your machine. The CLI, MCP server and the local mirror are all local, and the MCP server runs over stdio unless you start it with --transport http. Credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your SonicWall credentials once, then run sonicwall-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a SonicWall question in plain language; it runs sonicwall-cli for you."
---

# The SonicWall MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by SonicWall Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for SonicWall. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects SonicWall to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI which client SonicWall firewalls are behind on firmware, hit by an advisory with SSL-VPN on WAN, or have SSL-VPN users without one-time passwords, and get one pass, warn, fail or unknown row per firewall. It reads every firewall in your NSM tenant plus your MySonicWall registrations, read-only, and keeps a local history so you can see what got worse since last week.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/sonicwall){: .btn}

## Instead of clicking through SonicWall, just ask

**Instead of** Logging into each client's firewall after SonicWall's SSL-VPN notice to check its firmware, whether SSL-VPN is on a WAN zone, and which local users still have no one-time password
**just ask:** *"Which client firewalls does the CVE-2024-40766 advisory hit, and which of them have SSL-VPN on a WAN zone?"*
<sub>Your agent runs: <code>sonicwall-cli exposure "SNWLID-2024-0015"</code></sub>

**Instead of** Opening the local user list on every firewall to find SSL-VPN accounts and admins without TOTP
**just ask:** *"Which SSL-VPN users and admin logins across my clients have no MFA?"*
<sub>Your agent runs: <code>sonicwall-cli mfa-audit</code></sub>

**Instead of** Filling in each client's cyber-insurance questionnaire from firewall screenshots and a renewals spreadsheet
**just ask:** *"Give me the insurance posture for every client firewall: firmware, support, advisories, licences, MFA and SSL-VPN."*
<sub>Your agent runs: <code>sonicwall-cli posture</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/sonicwall/wide-1200x630.png" src="/assets/video/sonicwall/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/sonicwall/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Give me one insurance-posture row per client firewall: firmware, end of support, advisories, licences, admin TOTP and SSL-VPN exposure. | `sonicwall-cli posture` |
| Which firewalls does the CVE-2024-40766 advisory hit, and which have SSL-VPN on a WAN zone? | `sonicwall-cli exposure "SNWLID-2024-0015"` |
| Which SonicWall advisories hit the most firewalls in my fleet right now? | `sonicwall-cli exposure` |
| Which admins and local SSL-VPN users have no one-time password? | `sonicwall-cli mfa-audit` |
| Which firewalls are behind the newest firmware, and which advisories does the upgrade clear? | `sonicwall-cli firmware drift --behind-only` |
| Which licences and security services expire in the next 90 days, and which boxes should be replaced instead of renewed? | `sonicwall-cli licenses renewals --days 90` |
| Which firewalls are registered in MySonicWall but invisible to NSM? | `sonicwall-cli blind-spots` |
| What got worse since last week: newly vulnerable, MFA turned off, SSL-VPN newly on WAN, a licence lapsed? | `sonicwall-cli posture changes --fleet --since 7d` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/sonicwall/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/sonicwall/guide.md).

## What makes this one different

[GTalksTech/sonicwall-mcp](https://github.com/GTalksTech/sonicwall-mcp) is a read-only Python MCP server for NSM SaaS with 46 tools for inventory, firmware, advisories, licence posture, per-site SSL-VPN configuration and config history. This one is a single binary that works as a CLI and an MCP server and is built around the insurance and advisory questions: one pass, warn, fail or unknown row per firewall, a per-account list of SSL-VPN users without a one-time password, MySonicWall registrations that NSM cannot see, and saved posture runs you can diff week to week. Anything it could not read is unknown, never pass.

SonicWall's [SAMI](https://www.sonicwall.com/blog/the-firewall-is-not-dead-how-ai-gives-it-a-co-pilot) is an assistant inside SonicWall's Unified Management console that answers fleet questions and can run tasks such as firmware upgrades. This works from the AI agent you already use, never changes a firewall, and grades the posture an insurer asks about per firewall.

## The pain this closes

- Akira ransomware operators went after SonicWall SSL-VPN in 2024 and again from July 2025; SonicWall linked the 2025 wave to CVE-2024-40766 and told admins to update firmware, reset local SSL-VPN passwords and enforce MFA.
- Firmware, SSL-VPN zones and per-user one-time-password settings live on each firewall. Checking them for every client means opening every box.
- NSM SaaS shows the firewalls onboarded to it; a firewall registered in MySonicWall but never added to NSM drops out of every report.
- Licence and security-service expiry dates sit per serial in MySonicWall, so the renewal list is a spreadsheet someone keeps by hand.
- Cyber-insurance questionnaires ask the same things per client every year: patched, MFA on remote access, supported hardware, licensed security services.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/sonicwall/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/sonicwall/install.ps1 | iex
```

Create a MySonicWall API key (ideally for an account whose NSM role is read-only), set SONICWALL_MSW_API_KEY (and SONICWALL_REGION=eucentral for EU portals), then run `sonicwall-cli nsm-login` to mint and save the NSM token and `sonicwall-cli doctor`: it makes one authenticated NSM read to confirm the token works.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | posture, posture changes, exposure, mfa-audit, firmware drift, licenses renewals, blind-spots; every list, get and count under devices, device-groups, firmware, licenses, vulnerabilities, tenants, alerts and nsm-system; config-audit count; the firewall reads other than the four under Sensitive read; msw-products, msw-licenses, msw-system and the msw-tenants reads other than access-code; search over the local store, and export of any resource other than config_audit and msw_users. Local writes only: posture saves each run, responses are cached, and export --output overwrites the file you name | Allow |
| Raw read (escape hatch) | nsm-get (vendor-documented NSM and SonicOS GET routes only; credential and state-changing routes refused by a keyword list; credential-shaped values redacted) | Allow, and review unfamiliar paths |
| Sensitive read | config-audit list and diff (configuration text as NSM returns it, not redacted field by field); firewall local-users, firewall local-groups, firewall user-auth, firewall sslvpn-sessions, msw-users and msw-activity (account inventory, who is connected and account activity; schema-documented secrets blanked); export config_audit and export msw_users, which write the same data to a file; sync and workflow archive, which copy config-audit configuration text and firewall local users and groups into the local store, where search and the MCP sql and search tools read them; tail of config_audit or msw_users | Human-in-the-loop - keep the output and the local store private |
| Credential | nsm-login (mints and saves the NSM token), nsm-session refresh and sso, msw-tenants access-code, import (its only targets are those token calls, and a refresh can invalidate the session token you are using), auth set-token, auth logout | Human-in-the-loop only |
| Data egress | --deliver webhook:<url> on any command (POSTs that command's output to the URL you name), feedback --send, and feedback when SONICWALL_FEEDBACK_ENDPOINT is set with SONICWALL_FEEDBACK_AUTO_SEND=true | Human-in-the-loop - a webhook sink moves client data off your machine |
| MCP execute | sonicwall_execute calls any curated endpoint by ID: each call follows the tiers above; its POSTs are the Credential-tier token exchanges plus four read-style MySonicWall queries, and every POST is refused while SONICWALL_MCP_READ_ONLY=1 | Human-in-the-loop unless SONICWALL_MCP_READ_ONLY=1 |

This connector is read-only against SonicWall: no command changes a firewall, NSM or MySonicWall configuration, and secret fields the vendor's schema documents are blanked from every read. The strongest control is a MySonicWall account whose NSM role is read-only. The tiers that need a human handle tokens or move data: nsm-login and the other token exchanges, config-audit and sync (configuration text as NSM returns it, not redacted field by field, in output or the local store), and a webhook delivery that sends output off your machine. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/sonicwall/governance.md).

## Frequently asked questions

### Is there an MCP server for SonicWall?

Yes - this one. A free, open source MCP server and Claude Code Skill for SonicWall, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the SonicWall MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SONICWALL_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `SONICWALL_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, and sonicwall-mcp speaks HTTP natively: set SONICWALL_MCP_HTTP_TOKEN, run `sonicwall-mcp --transport http`, and serve its /mcp endpoint (http://127.0.0.1:7777/mcp by default) over HTTPS behind an authenticating proxy that signs ChatGPT in with OAuth and adds the bearer header the server requires. ChatGPT cannot send a fixed bearer token, so a bare tunnel gets HTTP 401. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter a MySonicWall API key once and run `sonicwall-cli nsm-login`.

### What credentials does it need?

A MySonicWall API key (SONICWALL_MSW_API_KEY). `sonicwall-cli nsm-login` trades it for an NSM session token and saves it; the token lasts 8 hours with a 15-minute idle timeout, so run nsm-login again when NSM commands exit 4. EU portals also set SONICWALL_REGION=eucentral. Use a MySonicWall account whose NSM role is read-only.

### Can it change my SonicWall firewalls?

No. It reads NSM, the firewalls through NSM's SonicOS passthrough, and MySonicWall. The only POSTs are read-style MySonicWall queries and the token exchanges (nsm-login, nsm-session, msw-tenants access-code). Secret fields SonicWall's own API schemas document, such as local-user and device passwords, registration and authentication codes, SNMP community names and MySonicWall API keys, are blanked before anything is printed, cached or handed to an agent.

### Will it see firewalls that are not in NSM?

Not with posture, exposure, mfa-audit or firmware drift: those only see firewalls onboarded to NSM SaaS. Run `sonicwall-cli blind-spots` to list firewalls registered in MySonicWall that NSM cannot see, with the advisories for their registered firmware.

### Is my SonicWall data safe?

Your data stays on your machine. The CLI, MCP server and the local mirror are all local, and the MCP server runs over stdio unless you start it with --transport http. Credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Blumira](/skills/blumira/) · [CIPP + Microsoft 365](/skills/cipp/) · [Cisco Duo](/skills/cisco-duo/) · [ConnectSecure](/skills/connectsecure/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Fortinet](/skills/fortinet/) · [Google Workspace](/skills/google-workspace/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [Sophos Central](/skills/sophos-central/) · [ThreatDown](/skills/threatdown/) · [ThreatLocker](/skills/threatlocker/)

## Status

Beta. Validated against the SonicWall API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
