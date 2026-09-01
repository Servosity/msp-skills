---
layout: default
title: "ConnectWise Control MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Manage ConnectWise Control (ScreenConnect) remote support and access sessions from the terminal or your AI agent: list and inspect sessions across groups, run commands on guest machines, rename and tag sessions, manage instance users, and query the audit log - the whole instance from one CLI."
permalink: /skills/connectwise-control/
skill_name: "ConnectWise Control MCP"
image: /assets/social/connectwise-control/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for ConnectWise Control?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for ConnectWise Control, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the ConnectWise Control MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CONNECTWISE_CONTROL_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CONNECTWISE_CONTROL_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but connectwise-control-mcp speaks HTTP natively: run `connectwise-control-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my ConnectWise Control data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Is this ConnectWise Control, or ConnectWise Manage / Automate?"
    a: "ConnectWise Control - the remote support and access tool, formerly ScreenConnect. ConnectWise Manage (the PSA) and ConnectWise Automate (the RMM) are separate skills. This one talks to your Control instance's session, user, and audit surface."
  - q: "How does it authenticate, and does it need the RESTful API Manager extension?"
    a: "HTTP Basic auth with a ConnectWise Control instance user (CONNECTWISE_CONTROL_USERNAME / CONNECTWISE_CONTROL_PASSWORD) against your instance base URL (CONNECTWISE_CONTROL_BASE_URL) - the same login you use for the web console. No extension is required; these are the built-in instance services the console itself uses."
  - q: "Can it actually run commands on machines?"
    a: "Yes, via sessions run-command, but that is gated human-in-the-loop in governance.md - it runs a real command on a guest endpoint. Day-to-day use is read-first: listing, searching, and inspecting sessions."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-control/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-control/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your ConnectWise Control credentials once, then run connectwise-control-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a ConnectWise Control question in plain language; it runs connectwise-control-cli for you."
---

# The ConnectWise Control MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by ConnectWise, LLC.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for ConnectWise Control. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects ConnectWise Control to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Drive ConnectWise Control (ScreenConnect) from the terminal or your AI agent instead of clicking the web console: list and search remote-support and access sessions, inspect a machine's session detail, run an approved command on a guest, rename and tag sessions, manage users, and read the audit log - scriptable, with an offline SQLite mirror so session lookups are fast and local.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/connectwise-control){: .btn}

## Instead of clicking through ConnectWise Control, just ask

**Instead of** Scrolling the ConnectWise Control web console to find which machines have an active session
**just ask:** *"List the access sessions across the instance"*
<sub>Your agent runs: <code>connectwise-control-cli sessions list --session-type Access --agent</code></sub>

**Instead of** Clicking into the console's audit view to see what happened on a machine
**just ask:** *"What's in the audit log for this session?"*
<sub>Your agent runs: <code>connectwise-control-cli audit query-log --session-name "ACME-WS01" --agent</code></sub>

**Instead of** Opening a session and typing the same command into the console command box on each machine
**just ask:** *"Run this command on the guest (gated for my approval)"*
<sub>Your agent runs: <code>connectwise-control-cli sessions run-command --session-id <sessionId> --command "..." --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/connectwise-control/wide-1200x630.png" src="/assets/video/connectwise-control/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/connectwise-control/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which access sessions are in this group? | `connectwise-control-cli sessions list --session-type Access --group "All Machines" --agent` |
| Show the full detail (connections, events) for one session | `connectwise-control-cli sessions get-detail --session-id <sessionId> --agent` |
| What session groups exist on the instance? | `connectwise-control-cli session-groups --agent` |
| Run a command on a guest machine (approval-gated) | `connectwise-control-cli sessions run-command --session-id <sessionId> --command "Get-Service" --powershell --agent` |
| Who are the instance users and roles? | `connectwise-control-cli security get-configuration --agent` |
| What's in the audit log for a session or time window? | `connectwise-control-cli audit query-log --session-name "ACME-WS01" --agent` |
| Rename a session | `connectwise-control-cli sessions update-name --session-id <sessionId> --new-name "ACME - Reception PC" --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/connectwise-control/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/connectwise-control/guide.md).

## What makes this one different

Most ConnectWise Control integrations script one .ashx call at a time or wrap the console's session list. This skill turns the whole instance surface - sessions, groups, users, and the audit log - into typed commands with JSON output and an offline SQLite mirror, so an AI agent can list, inspect, and act on sessions across your instance without clicking through the web console one machine at a time.

ConnectWise Control's web console is built for a technician driving one remote session interactively. This complements it by making the same instance scriptable and agent-friendly - bulk session queries, audit lookups, and approval-gated command execution - from the terminal, without replacing the live remote-control experience.

## The pain this closes

- ConnectWise Control is a web console built for a technician driving one remote session at a time - there's no scriptable, terminal-native way to list, search, or act on sessions in bulk.
- Answering 'which machines have an open session?' or 'what happened on this endpoint?' means scrolling the console and clicking into sessions and the audit view by hand.
- Running the same diagnostic or remediation command across several machines means opening each session and typing it into the console's command box.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectwise-control/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectwise-control/install.ps1 | iex
```

After install, authenticate once with your ConnectWise Control credentials, then verify with `connectwise-control-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | sessions list / get-detail, session-groups, security get-configuration, audit get-info / query-log, and search | Allow |
| Write (session metadata) | sessions update-name, sessions update-custom-property | Preview with --dry-run, then a reviewed write |
| Session and host control | sessions run-command (runs a real command on a guest machine), sessions add-event-to (queues control events like wake) | Human-in-the-loop, explicit confirmation |
| Access grant | sessions get-access-token (issues a one-time token/URL that grants remote access to a session) | Human-in-the-loop only |
| Admin and identity | security save-user (create or update an instance user and roles), security delete-user | Operator-only, not for agents |

The skill is read-first: listing and inspecting sessions, session groups, instance users and roles, and the audit log are read-only and safe to let an agent run. Session and host control - running a command on a guest machine (sessions run-command) and queuing control events like wake (sessions add-event-to) - is gated human-in-the-loop, as are issuing a join access token, and creating, updating, or deleting instance users. The instance user's own ConnectWise Control permissions are the outer limit on anything the agent can do. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/connectwise-control/governance.md).

## Frequently asked questions

### Is there an MCP server for ConnectWise Control?

Yes - this one. A free, open source MCP server and Claude Code Skill for ConnectWise Control, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the ConnectWise Control MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CONNECTWISE_CONTROL_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CONNECTWISE_CONTROL_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, but connectwise-control-mcp speaks HTTP natively: run `connectwise-control-mcp --transport http --addr :7777` and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my ConnectWise Control data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Is this ConnectWise Control, or ConnectWise Manage / Automate?

ConnectWise Control - the remote support and access tool, formerly ScreenConnect. ConnectWise Manage (the PSA) and ConnectWise Automate (the RMM) are separate skills. This one talks to your Control instance's session, user, and audit surface.

### How does it authenticate, and does it need the RESTful API Manager extension?

HTTP Basic auth with a ConnectWise Control instance user (CONNECTWISE_CONTROL_USERNAME / CONNECTWISE_CONTROL_PASSWORD) against your instance base URL (CONNECTWISE_CONTROL_BASE_URL) - the same login you use for the web console. No extension is required; these are the built-in instance services the console itself uses.

### Can it actually run commands on machines?

Yes, via sessions run-command, but that is gated human-in-the-loop in governance.md - it runs a real command on a guest endpoint. Day-to-day use is read-first: listing, searching, and inspecting sessions.


## Status

Beta. Validated against the ConnectWise Control API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
