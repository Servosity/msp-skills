---
layout: default
title: "Keeper Admin and MSP Console MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Read-only Keeper MSP posture across every client, with history, a change feed and no path to a vault secret."
permalink: /skills/keeper/
skill_name: "Keeper Admin and MSP Console MCP"
image: /assets/social/keeper/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Keeper Admin and MSP Console?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Keeper Admin and MSP Console, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Keeper Admin and MSP Console MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `KEEPER_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `KEEPER_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Keeper Security MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Keeper Security data safe?"
    a: "Your data stays on your machine and your own Service Mode host. Typed reads are printed and not saved; only posture report saves metrics and per-user facts to a local store. The skill cannot read a vault record, and a response shaped like record content is refused whole."
  - q: "Can this read passwords from a client's vault?"
    a: "No. It can send only the admin and MSP reports listed by keeper-cli allowlist, each with fixed read-only flags. Set the same list on your Service Mode key with service-create -c, so Service Mode refuses anything else too."
  - q: "What do I need from Keeper?"
    a: "Keeper Commander running in Service Mode on a host you control, over TLS (service-create with -crtf and -crtp), and its API key. The CLI refuses plain http on every host, localhost included. A private certificate is trusted with KEEPER_CA_CERT."
  - q: "Do I need a Keeper MSP account?"
    a: "The cross-client commands need one. An account that is not an MSP is reported as its own enterprise, one company in the same format."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/keeper/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Keeper Admin and MSP Console credentials once, then run keeper-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Keeper Admin and MSP Console question in plain language; it runs keeper-cli for you."
---

# The Keeper Admin and MSP Console MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Keeper Security, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Keeper Admin and MSP Console. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Keeper Admin and MSP Console to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

See every client's Keeper 2FA coverage, weak and reused passwords, BreachWatch exposure, inactive users and seats in one run, then ask what changed since last quarter. Keeper Security + your AI reads through your own Commander Service Mode host and stores each run locally. It is read-only, and it has no path to a vault record.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/keeper){: .btn}

## Instead of clicking through Keeper Admin and MSP Console, just ask

**Instead of** Open the Keeper console for each managed company and note who still has 2FA off
**just ask:** *"Which clients have users without 2FA, admins first?"*
<sub>Your agent runs: <code>keeper-cli users risk --kind no-2fa --agent</code></sub>

**Instead of** Rebuild last quarter's password-health spreadsheet by hand and compare it to this one
**just ask:** *"What changed in Keeper posture since the last run?"*
<sub>Your agent runs: <code>keeper-cli posture diff --agent</code></sub>

**Instead of** Search client by client to find which company a departing user belongs to
**just ask:** *"Which client is jane@example.com in, and when did they last log in?"*
<sub>Your agent runs: <code>keeper-cli users find jane@example.com --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/keeper/wide-1200x630.png" src="/assets/video/keeper/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/keeper/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| How is every client doing on 2FA, weak passwords and BreachWatch? | `keeper-cli posture report --days 90 --agent` |
| Which clients have users without 2FA, admins first? | `keeper-cli users risk --kind no-2fa --agent` |
| What changed since the last posture run, by client and by user? | `keeper-cli posture diff --agent` |
| Which invites went stale, and who is inactive or locked? | `keeper-cli users hygiene --kind invited,inactive --agent` |
| Which managed company does this user belong to? | `keeper-cli users find jane@example.com --agent` |
| How many seats does each client have allocated and active? | `keeper-cli companies list --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/keeper/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/keeper/guide.md).

## What makes this one different

Most password-manager integrations, and an MCP server that only forwards each tool call, answer about one tenant now. This skill stores each cross-client posture run locally, so "what changed" and "who has no 2FA anywhere" are one offline query, and every command it can send is on a short fixed read-only allowlist it prints with keeper-cli allowlist.

The Keeper Admin Console and MSP Console stay your place to change users, roles and seats; this skill adds cross-client posture history in the AI agent you already use, and complements the console rather than replacing it.

## The pain this closes

- Password health and 2FA are read per tenant, so a QBR across a book of clients means repeating the same look client by client. A 2026 review for MSPs describes the weak and reused password view as "per tenant".
- Keeper's own reports do not answer "what changed since last quarter" across clients without someone keeping the earlier numbers; Keeper's MSP best practices recommend change and activity reports plus alerts, and advanced reporting is an add-on on some plans.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](/integrations/claude-desktop/) |
| **ChatGPT** (desktop app) | [Step-by-step →](/integrations/chatgpt/) |
| **Claude Code** | [Step-by-step →](/integrations/claude-code/) |
| **Codex CLI** | [Step-by-step →](/integrations/codex/) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](/which-agent/) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/keeper/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/keeper/install.ps1 | iex
```

After install, authenticate once with your Keeper Admin and MSP Console credentials, then verify with `keeper-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | companies list, users list, users risk, users hygiene, posture diff, posture runs, events list, billing report, security-audit report, audit-trail events | Allow |
| Local write (this machine only) | posture report (reads Keeper, saves the run locally), teach, playbook amend, learnings forget, profile save, local-data purge | Allow in the CLI; through MCP only posture_report is a tool, and it needs confirm set to true |
| Credential | auth set-token, auth logout | Human-in-the-loop only |

The skill reads Keeper admin and MSP reports: companies, seats, users, roles, teams, 2FA, password-health scores, BreachWatch summaries and audit events. It cannot change anything in Keeper and cannot read a vault record. The only things it writes are local: posture runs, learnings and profiles. Let an agent read freely, and keep a human on the credential commands. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/keeper/governance.md).

## Frequently asked questions

### Is there an MCP server for Keeper Admin and MSP Console?

Yes - this one. A free, open source MCP server and Claude Code Skill for Keeper Admin and MSP Console, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Keeper Admin and MSP Console MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `KEEPER_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `KEEPER_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Keeper Security MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Keeper Security data safe?

Your data stays on your machine and your own Service Mode host. Typed reads are printed and not saved; only posture report saves metrics and per-user facts to a local store. The skill cannot read a vault record, and a response shaped like record content is refused whole.

### Can this read passwords from a client's vault?

No. It can send only the admin and MSP reports listed by keeper-cli allowlist, each with fixed read-only flags. Set the same list on your Service Mode key with service-create -c, so Service Mode refuses anything else too.

### What do I need from Keeper?

Keeper Commander running in Service Mode on a host you control, over TLS (service-create with -crtf and -crtp), and its API key. The CLI refuses plain http on every host, localhost included. A private certificate is trusted with KEEPER_CA_CERT.

### Do I need a Keeper MSP account?

The cross-client commands need one. An account that is not an MSP is reported as its own enterprise, one company in the same format.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Bitdefender GravityZone](/skills/bitdefender/) · [Blackpoint Cyber](/skills/blackpoint/) · [Blumira](/skills/blumira/) · [CIPP + Microsoft 365](/skills/cipp/) · [Cisco Duo](/skills/cisco-duo/) · [Cisco Umbrella](/skills/cisco-umbrella/) · [ConnectSecure](/skills/connectsecure/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [DNSFilter](/skills/dnsfilter/) · [Exchange Online](/skills/exchange-online/) · [Fortinet](/skills/fortinet/) · [Google Workspace](/skills/google-workspace/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Mimecast](/skills/mimecast/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SaaS Alerts](/skills/saas-alerts/) · [SentinelOne](/skills/sentinelone/) · [SonicWall](/skills/sonicwall/) · [Sophos Central](/skills/sophos-central/) · [ThreatDown](/skills/threatdown/) · [ThreatLocker](/skills/threatlocker/) · [WatchGuard Cloud](/skills/watchguard/)

## Status

Beta. Validated against the Keeper Admin and MSP Console API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
