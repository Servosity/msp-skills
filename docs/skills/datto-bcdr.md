---
layout: default
title: "Datto BCDR MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Sync your whole Datto BCDR fleet into local SQLite and answer the questions the per-appliance Partner Portal can't: which backups failed screenshot verification, which are stale, and which clients are at risk."
permalink: /skills/datto-bcdr/
skill_name: "Datto BCDR MCP"
image: /assets/social/datto-bcdr/wide-1200x630.png
verification: live-verified
faqs:
  - q: "Is there an MCP server for Datto BCDR?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Datto BCDR, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Datto BCDR MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Datto BCDR MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Datto BCDR data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need to be a Datto partner to use this?"
    a: "Yes. It uses the Datto BCDR REST API, which needs a partner-generated public/secret key pair from the Partner Portal under Admin > Integrations. If you manage Datto BCDR appliances, you already qualify - the CLI base64-encodes the key pair into the Authorization header on every request."
  - q: "Will this hit my Datto BCDR API rate limits?"
    a: "It's gentle by design. The first sync pulls each resource with bounded pagination, and you can cap throughput with --rate-limit. After that, fleet questions run against the local mirror and make zero API calls - --data-source local never touches the API at all."
  - q: "Does this replace the Datto Partner Portal?"
    a: "No. It answers the fleet-wide, cross-client questions the per-appliance portal can't, and it's read-only for everyday use - you still use the portal for restores, virtualization, and device configuration."
  - q: "Can it change anything in Datto, or just read?"
    a: "Read-only for everyday use - every analysis, list, and report command only reads. The single exception is `import`, an explicit bulk data-load command you would never run by accident; preview it with --dry-run first."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-bcdr/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-bcdr/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Datto BCDR credentials once; datto-bcdr-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Datto BCDR question in plain language; it runs datto-bcdr-cli for you."
---

# The Datto BCDR MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Datto, Inc..

**✓ Live-verified by Abhi Saini, Bearium Networks (MSP)** against a production tenant · 2026-08-13 · [receipt →](https://compoundingteams.com/).

Yes - there is an MCP server for Datto BCDR. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Datto BCDR to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask in plain English which Datto backups failed their last screenshot verification, which clients are most at risk, and which appliance fills up first - and get the answer across your whole fleet in seconds. The Datto BCDR API answers one appliance at a time; this skill mirrors every device, agent, and alert locally, so the fleet-wide question the Partner Portal can't answer becomes one instant command.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/datto-bcdr){: .btn}

## Instead of clicking through Datto BCDR, just ask

**Instead of** Open every SIRIS and ALTO in the Datto Partner Portal one at a time and read down the screenshot column to find backups that won't boot
**just ask:** *"Which protected machines failed their last backup screenshot verification, across every client?"*
<sub>Your agent runs: <code>datto-bcdr-cli screenshots --failed --stale-days 7 --agent</code></sub>

**Instead of** Click into each appliance to check recovery-point dates, hoping you catch a backup that quietly stopped taking snapshots
**just ask:** *"Which agents haven't taken a local snapshot or synced offsite recently, fleet-wide?"*
<sub>Your agent runs: <code>datto-bcdr-cli stale-backups --local-days 1 --offsite-days 3 --agent</code></sub>

**Instead of** Build a per-client risk picture by hand in a spreadsheet before a QBR or a renewal conversation
**just ask:** *"Which of my clients are most at risk right now in Datto?"*
<sub>Your agent runs: <code>datto-bcdr-cli client-risk --top 10 --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/datto-bcdr/wide-1200x630.png" src="/assets/video/datto-bcdr/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/datto-bcdr/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which protected machines failed their last backup screenshot verification? | `datto-bcdr-cli screenshots --failed --stale-days 7 --agent` |
| Which agents are behind on local snapshots or offsite sync? | `datto-bcdr-cli stale-backups --local-days 1 --offsite-days 3 --agent` |
| What percentage of my fleet is actually recoverable right now? | `datto-bcdr-cli recoverability --agent` |
| Which clients are most at risk across backups, alerts, and storage? | `datto-bcdr-cli client-risk --top 10 --agent` |
| Show me every open alert across the whole fleet, grouped by client. | `datto-bcdr-cli alert-triage --group-by client --agent` |
| Which appliance runs out of local or offsite storage first? | `datto-bcdr-cli storage-runway --threshold-pct 85 --agent` |
| Which protected machines are paused, archived, or on an appliance that went dark? | `datto-bcdr-cli forgotten-assets --offline-days 2 --agent` |
| Which machines are running an outdated backup agent? | `datto-bcdr-cli agent-versions --outdated --agent` |
| Give me a one-page backup health report for a single client before the QBR. | `datto-bcdr-cli client-report "Acme Corp" --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/datto-bcdr/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/datto-bcdr/guide.md).

## What makes this one different

Most Datto BCDR integrations and MCP servers proxy each question into a single live API call - and because the Datto API is strictly per-appliance, that means one call per device just to answer one fleet question. This skill syncs your entire estate - every device, agent, share, and alert - into a local SQLite mirror, so a question like 'which backups failed verification everywhere' becomes one offline join: instant, fully paginated, and reproducible later as a snapshot you can diff.

The Datto Partner Portal answers backup-health questions one appliance at a time and has no cross-client recoverability rollup; this skill answers them across every client at once and hands the result to the AI agent you already work in - it complements the portal you still use for restores and device config, it doesn't replace it.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](/skills/servosity/) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

## The pain this closes

- A backup that fails screenshot verification is silently unbootable - you find out at restore time, when the client is already down, because the Partner Portal makes you check bootability one appliance at a time.
- Datto's API and portal are scoped per-device, so the cross-client question every owner actually asks - 'are all my backups recoverable?' - means walking every SIRIS and ALTO by hand.
- An agent that quietly stops taking recovery points, or one paused 'temporarily' months ago, never fires an alert - the gap only surfaces when someone needs that machine restored.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/datto-bcdr/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/datto-bcdr/install.ps1 | iex
```

After install, authenticate once with your Datto BCDR credentials, then verify with `datto-bcdr-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | screenshots, stale-backups, recoverability, client-risk, alert-triage, storage-runway, forgotten-assets, agent-versions, client-report, device/agent/asset/shares/alert/vm-restore, sync, search, analytics | Allow |
| Write (routine) | import (POST each record to the Datto BCDR API) | Preview with --dry-run, then a reviewed write |
| Credential / config | auth set-token, auth logout (replace or clear stored credentials) | Human-in-the-loop only |

The datto-bcdr skill is read-only for everyday use: it reads your Datto BCDR fleet (devices, agents, shares, alerts, screenshots) and writes only to a local SQLite mirror on your machine. The single API-mutating command is `import`, a bulk data load you preview with --dry-run; nothing else can change remote state. Scope the partner key pair to what your workflow needs, and keep autonomous agents to read plus previewed imports. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/datto-bcdr/governance.md).

## Frequently asked questions

### Is there an MCP server for Datto BCDR?

Yes - this one. A free, open source MCP server and Claude Code Skill for Datto BCDR, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Datto BCDR MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Datto BCDR MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Datto BCDR data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need to be a Datto partner to use this?

Yes. It uses the Datto BCDR REST API, which needs a partner-generated public/secret key pair from the Partner Portal under Admin > Integrations. If you manage Datto BCDR appliances, you already qualify - the CLI base64-encodes the key pair into the Authorization header on every request.

### Will this hit my Datto BCDR API rate limits?

It's gentle by design. The first sync pulls each resource with bounded pagination, and you can cap throughput with --rate-limit. After that, fleet questions run against the local mirror and make zero API calls - --data-source local never touches the API at all.

### Does this replace the Datto Partner Portal?

No. It answers the fleet-wide, cross-client questions the per-appliance portal can't, and it's read-only for everyday use - you still use the portal for restores, virtualization, and device configuration.

### Can it change anything in Datto, or just read?

Read-only for everyday use - every analysis, list, and report command only reads. The single exception is `import`, an explicit bulk data-load command you would never run by accident; preview it with --dry-run first.


## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](/skills/acronis/) · [Afi](/skills/afi/) · [Axcient x360Recover](/skills/axcient/) · [Cove Data Protection](/skills/cove/) · [Servosity](/skills/servosity/) · [SkyKick](/skills/skykick/) · [Veeam](/skills/veeam/)

## Status

Beta. Validated against the Datto BCDR API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
