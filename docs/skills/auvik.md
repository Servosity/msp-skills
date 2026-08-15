---
layout: default
title: "Auvik MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every Auvik endpoint as a command, plus the cross-client answers the Auvik UI and API cannot give you."
permalink: /skills/auvik/
skill_name: "Auvik MCP"
image: /assets/social/auvik/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Auvik?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Auvik, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Auvik MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Auvik MCP server through a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your Auvik user email and API key once."
  - q: "Why does it need both a username and an API key?"
    a: "Auvik authenticates with HTTP Basic: your Auvik user email is the username and the API key is the password. There is no single-token form. You also need the right regional host - AUVIK_BASE_URL selects us1, us2, eu1 and so on, and pointing at the wrong region returns a 401 that looks exactly like a bad key."
  - q: "Is my Auvik data safe?"
    a: "Your data stays on your machine by default. The CLI, MCP server, and the local mirror are all local, and the MCP server runs over stdio unless you deliberately start it in HTTP mode (`--transport http`, or `PP_MCP_TRANSPORT=http`), which the remote-agent path for ChatGPT and Copilot asks you to do - that opens a listener, so tunnel it behind SSO and treat the URL as a secret. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "Will this hit my Auvik API rate limits?"
    a: "Mostly no, and that is the point of the mirror. sync does the reading; the cross-client commands then answer from local SQLite and touch no API at all. Re-running the same question costs nothing upstream."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/auvik/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/auvik/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Auvik credentials once; auvik-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Auvik question in plain language; it runs auvik-cli for you."
---

# The Auvik MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Auvik Networks Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Auvik. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Auvik to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI what is going end-of-life, what is unbacked-up, or why a client's billable device count moved, and get the device rows behind the answer across every client at once. Auvik holds the richest network truth an MSP has, but its API answers one client, right now, and reports no deletions at all. This mirrors Auvik into local SQLite so the cross-client and change-over-time questions become one query.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/auvik){: .btn}

## Instead of clicking through Auvik, just ask

**Instead of** Exporting each client's inventory to a spreadsheet and sorting by support date to find what is aging out
**just ask:** *"What is past end-of-support across all my clients?"*
<sub>Your agent runs: <code>auvik-cli eol --bucket expired</code></sub>

**Instead of** Opening every client's config-backup screen to check which devices actually have a backup
**just ask:** *"Which devices have no configuration backup?"*
<sub>Your agent runs: <code>auvik-cli configuration audit --finding no_backup</code></sub>

**Instead of** Arguing about an Auvik invoice with no way to see which devices moved the billable count
**just ask:** *"Which clients' billed device counts do not match their inventory?"*
<sub>Your agent runs: <code>auvik-cli usage reconcile --mismatch-only</code></sub>


## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What hardware is past or approaching end-of-support across every client? | `auvik-cli eol --bucket expired` |
| What ages out in the next 90 days, so I can put it in a QBR? | `auvik-cli eol --within 90` |
| Which devices have no configuration backup, or only a stale one? | `auvik-cli configuration audit --finding no_backup` |
| What was added, changed, or removed across the fleet since the last sync? | `auvik-cli inventory diff --since 7d` |
| Which clients' billed device counts disagree with their actual inventory? | `auvik-cli usage reconcile --mismatch-only` |
| Which devices can Auvik not fully poll, and what credential is failing? | `auvik-cli device discovery-gaps --method snmp` |
| Which devices and clients generate the most alert noise? | `auvik-cli alert noise --since 30d --group-by client` |
| Which SaaS apps have users but no licence, and which licences is nobody using? | `auvik-cli asm shadow --finding unused_licenses` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/auvik/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/auvik/guide.md).

## What makes this one different

Most Auvik integrations are a language binding: they hand a question straight to the REST API and hand back typed structs. That works for one device, right now. It cannot answer what changed, what left, or anything that spans clients, because the API has no cross-client aggregate and emits no deletion event. This syncs Auvik into a local SQLite mirror first, so those become one local join instead of a fan-out that does not exist upstream.

The Auvik UI is per-client and per-screen by design, and its API mirrors that shape. Neither differences two points in time, so a removed device is invisible and a moved billable count has no explanation attached. The mirror keeps your own prior view of the fleet, which is what makes removals and deltas detectable at all.

## The pain this closes

- Auvik's API emits no deletion event. A decommissioned device just stops appearing, so nothing tells you it left.
- Hardware lifecycle lives per device, per client. 'What is aging out across the book' is a spreadsheet export every time.
- The billable-device count is a number in the UI. When it moves, nothing shows you which devices caused it.
- A device Auvik cannot fully poll looks similar to one that is simply quiet, and the credential state behind it is three screens away.
- Config backups are a per-device screen, so 'which devices have no backup at all' is not a question you can ask.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/auvik/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/auvik/install.ps1 | iex
```

After install, authenticate once with your Auvik credentials, then verify with `auvik-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | eol, configuration audit, inventory diff, usage reconcile, device discovery-gaps, alert noise, asm shadow, changes, sync, search, export, every list/get command, and all of the settings and stat SNMP-poller commands (they are GET-only despite the name) | Allow |
| Write (routine) | alert dismiss-single and its friendly twin alert dismiss - both call POST /v1/alert/dismiss/{id}, the only write the Auvik API supports; allowlist both names | Preview with --dry-run, then a reviewed write |
| Credential / security | auth set-credentials (writes the credential to the CLI's credentials file), auth logout | Human-in-the-loop only |
| Data egress | --deliver webhook:<url> on any command (POSTs that command's output to a URL you name), feedback --send, and bare feedback when AUVIK_FEEDBACK_AUTO_SEND=true - that one needs no flag at all | Human-in-the-loop - a webhook sink moves client data off-box |

Auvik's API is read-only in practice and this CLI reflects that: the eight cross-client analysis commands read the local mirror and cannot change anything upstream, and dismissing an alert is the only write the API supports at all. The strongest control is the scope of the API key, which inherits the permissions of the user who created it - mint it as a read-only user for a reporting workflow. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/auvik/governance.md).

## Frequently asked questions

### Is there an MCP server for Auvik?

Yes - this one. A free, open source MCP server and Claude Code Skill for Auvik, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Auvik MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Auvik MCP server through a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your Auvik user email and API key once.

### Why does it need both a username and an API key?

Auvik authenticates with HTTP Basic: your Auvik user email is the username and the API key is the password. There is no single-token form. You also need the right regional host - AUVIK_BASE_URL selects us1, us2, eu1 and so on, and pointing at the wrong region returns a 401 that looks exactly like a bad key.

### Is my Auvik data safe?

Your data stays on your machine by default. The CLI, MCP server, and the local mirror are all local, and the MCP server runs over stdio unless you deliberately start it in HTTP mode (`--transport http`, or `PP_MCP_TRANSPORT=http`), which the remote-agent path for ChatGPT and Copilot asks you to do - that opens a listener, so tunnel it behind SSO and treat the URL as a secret. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### Will this hit my Auvik API rate limits?

Mostly no, and that is the point of the mirror. sync does the reading; the cross-client commands then answer from local SQLite and touch no API at all. Re-running the same question costs nothing upstream.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## More RMM connectors

Run more than one RMM tool, or comparing options? These connectors work the same way: [Action1](/skills/action1/) · [Atera](/skills/atera/) · [ConnectWise Automate](/skills/connectwise-automate/) · [Datto RMM](/skills/datto-rmm/) · [Level](/skills/levelio/) · [N-able N-central](/skills/n-central/) · [Nerdio Manager](/skills/nerdio/) · [NinjaOne](/skills/ninjaone/) · [Tactical RMM](/skills/tactical-rmm/)

## Status

Beta. Validated against the Auvik API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
