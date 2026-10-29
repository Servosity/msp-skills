---
layout: default
title: "Jamf Pro MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every client's Jamf Pro in one CLI: register each instance once, then find devices, check posture and patch status, and audit policies across all of them."
permalink: /skills/jamf-pro/
skill_name: "Jamf Pro MCP"
image: /assets/social/jamf-pro/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Jamf Pro?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Jamf Pro, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Jamf Pro MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `JAMF_PRO_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `JAMF_PRO_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Jamf Pro MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Jamf Pro data safe?"
    a: "Your data stays on your machine. The CLI, the MCP server and the local mirror all run locally. The AI sees query results, and credentials are never bundled or transmitted by MSP Skills: they go to your own Jamf Pro server and nowhere else, and any output that repeats one is withheld."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need a separate API client for each client's Jamf Pro?"
    a: "Yes. Create one in each instance under Settings > System > API roles and clients, with only the read privileges the reports need, then register it once with `jamf-pro-cli instance add`. Each secret stays in the JAMF_PRO_FLEET_SECRETS environment variable (name=secret entries separated by commas) and is never written to the registry."
  - q: "Will this replace Jamf Account or the Jamf Pro console?"
    a: "No. Jamf Account is where a service provider manages customers and their products, and each console is where you build policies and enroll devices. This skill answers questions across the data inside those instances."
  - q: "Can it read LAPS passwords, FileVault keys or lock PINs?"
    a: "No. Those endpoints are not included, `fleet exec` refuses them by path, and any password, key, token or PIN field in another response prints as [redacted]. Read them in the Jamf Pro console."
  - q: "We run one Jamf Pro with sites. Does it help?"
    a: "Partly. Register that server once and every read works, but fleet reports count per instance, not per site. Fleet reports earn their keep when each client has its own instance."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Jamf Pro credentials once, then run jamf-pro-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Jamf Pro question in plain language; it runs jamf-pro-cli for you."
---

# The Jamf Pro MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by JAMF Software, LLC.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Jamf Pro. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Jamf Pro to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask which client has this Mac, or which clients have FileVault off and patches behind, and get one answer across every client's Jamf Pro. Jamf Pro plus your AI agent reads each client's own server in parallel, with that client's own API client, so a question that means logging in to every console one at a time becomes one reply.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/jamf-pro){: .btn}

## Instead of clicking through Jamf Pro, just ask

**Instead of** Log in to each client's Jamf Pro console and search for a serial number until one of them has it
**just ask:** *"Which client has the Mac with serial C02XK1ABJG5H?"*
<sub>Your agent runs: <code>jamf-pro-cli fleet find C02XK1ABJG5H --agent</code></sub>

**Instead of** Export inventory from every client's Jamf Pro, then count stale check-ins, old macOS and FileVault status in a spreadsheet
**just ask:** *"Which clients have Macs with FileVault off, or below macOS 14.6?"*
<sub>Your agent runs: <code>jamf-pro-cli fleet posture --stale-days 14 --min-os 14.6 --agent</code></sub>

**Instead of** Keep a calendar reminder per client for every ADE and VPP token, and find out about the one you missed when enrollment stops
**just ask:** *"Which enrollment or VPP tokens expire in the next 30 days, across every client?"*
<sub>Your agent runs: <code>jamf-pro-cli fleet health --within 30d --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/jamf-pro/wide-1200x630.png" src="/assets/video/jamf-pro/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/jamf-pro/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which client has this Mac, iPad or iPhone, by serial, name, username or email? | `jamf-pro-cli fleet find C02XK1ABJG5H --agent` |
| Which clients have stale check-ins, Macs below a macOS floor, FileVault off or the firewall off? | `jamf-pro-cli fleet posture --stale-days 14 --min-os 14.6 --agent` |
| Which patch titles are behind latest, and which clients have failed managed OS updates? | `jamf-pro-cli fleet patch --behind-only --agent` |
| A vendor advisory landed: which clients and computers have this app below the fixed version? | `jamf-pro-cli fleet app "Google Chrome" --below 129.0.6668.100 --agent` |
| Which policies are disabled, can never run, have no scope, or point at a missing script or package? | `jamf-pro-cli fleet policy-hygiene --agent` |
| Which ADE or VPP tokens expire soon, which enrollment syncs failed, and which Jamf Pro versions are clients on? | `jamf-pro-cli fleet health --within 30d --agent` |
| Which clients are missing objects from our template instance, or run a different copy of its scripts? | `jamf-pro-cli fleet baseline --golden msp-template --agent` |
| Does every client's API client still work? | `jamf-pro-cli instance test --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/jamf-pro/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/jamf-pro/guide.md).

## What makes this one different

A connector that forwards each tool call to one Jamf Pro server answers for one client at a time. This one keeps a registry of every client's server and API client, reads them in parallel with each client's own credential, and answers in one table with an instance column. The fleet commands read live; `sync` also builds a local SQLite mirror per instance for offline search.

Jamf Pro and Jamf Account stay your place to manage each customer, enroll devices and build policies; this skill answers the questions that cross customers, in the AI agent you already use, and complements them rather than replacing them.

## The pain this closes

- Jamf Pro gives each customer its own server, so an MSP works many consoles. On Jamf Nation one MSP engineer put it as "about 100 instances", because Apple requires each organization to have its own push certificate. Any question across clients means one login per client.
- A ticket names a Mac or a user but not the client, and someone logs in to console after console until a serial number turns up.
- ADE and VPP tokens expire per client on their own dates, so a lapsed token shows up as an onboarding that stopped working.
- A policy changed in your template drifts out of step across clients, and nothing tells you which client lags or which policy can never run.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/jamf-pro/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/jamf-pro/install.ps1 | iex
```

After install, authenticate once with your Jamf Pro credentials, then verify with `jamf-pro-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet find`, `fleet posture`, `fleet patch`, `fleet app`, `fleet policy-hygiene`, `fleet health`, `fleet baseline`, `fleet exec`, `computer-inventory list`, `scripts get`, `policies list`, `sync`, `search` | Allow |
| Write (routine) | `buildings create`, `categories update`, `departments update`, `computer-groups create-static-groups`, `packages update`, `computer-extension-attributes update`, every other POST, PUT or PATCH not named below | Preview with --dry-run, then a reviewed write |
| Credential and identity | `auth login`, `auth set-token`, every `api-integrations` and `api-roles` command, `accounts`, `account-groups`, every `local-admin-password` command, `volume-purchasing-locations create` and `update`, and any prestage write that sets an admin password | Human-in-the-loop only |
| Acts on customer devices | `computer-inventory erase`, `computer-inventory remove-mdm-profile`, `mobile-devices erase`, `mobile-devices unmanage`, `mdm send-command`, `mdm deploy-package`, `mdm blank-push`, `mdm renew-profile`, `managed-software-updates create-plans`, `patch-policy-logs retry`, `policies create` and `update`, `scripts create` and `update`, `device-enrollments disown`, `volume-purchasing-locations revoke-licenses` and `reclaim` | Human-in-the-loop only, one instance at a time |
| Destructive / config | Every `delete` and `delete-multiple` command, for example `scripts delete`, `policies delete`, `computer-inventory delete`, `api-roles delete`, `device-enrollments delete` | Human-in-the-loop only, explicit confirmation |

It reads inventory, policies, patch status and enrollment health from every Jamf Pro instance you register, and can change one instance at a time with the writes that instance's API client allows. Passwords, keys, tokens and PINs in responses are redacted, and the endpoints that return LAPS passwords, FileVault keys, recovery-lock passwords and lock PINs are not included. Let an agent read freely, preview routine writes, and keep a human on anything that touches credentials or acts on a device. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/jamf-pro/governance.md).

## Frequently asked questions

### Is there an MCP server for Jamf Pro?

Yes - this one. A free, open source MCP server and Claude Code Skill for Jamf Pro, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Jamf Pro MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `JAMF_PRO_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `JAMF_PRO_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Jamf Pro MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Jamf Pro data safe?

Your data stays on your machine. The CLI, the MCP server and the local mirror all run locally. The AI sees query results, and credentials are never bundled or transmitted by MSP Skills: they go to your own Jamf Pro server and nowhere else, and any output that repeats one is withheld.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need a separate API client for each client's Jamf Pro?

Yes. Create one in each instance under Settings > System > API roles and clients, with only the read privileges the reports need, then register it once with `jamf-pro-cli instance add`. Each secret stays in the JAMF_PRO_FLEET_SECRETS environment variable (name=secret entries separated by commas) and is never written to the registry.

### Will this replace Jamf Account or the Jamf Pro console?

No. Jamf Account is where a service provider manages customers and their products, and each console is where you build policies and enroll devices. This skill answers questions across the data inside those instances.

### Can it read LAPS passwords, FileVault keys or lock PINs?

No. Those endpoints are not included, `fleet exec` refuses them by path, and any password, key, token or PIN field in another response prints as [redacted]. Read them in the Jamf Pro console.

### We run one Jamf Pro with sites. Does it help?

Partly. Register that server once and every read works, but fleet reports count per instance, not per site. Fleet reports earn their keep when each client has its own instance.


## More MDM connectors

Run more than one MDM tool, or comparing options? These connectors work the same way: [Apple Business](/skills/apple-business/)

## Status

Beta. Validated against the Jamf Pro API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
