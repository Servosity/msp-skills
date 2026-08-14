---
layout: default
title: "runZero MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every runZero query, plus a local SQLite copy of your whole attack surface that diffs over time, joins assets to vulnerabilities offline, and costs zero API quota to re-slice."
permalink: /skills/runzero/
skill_name: "runZero MCP"
image: /assets/social/runzero/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for runZero?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for runZero, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the runZero MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local runZero MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your runZero API key once."
  - q: "Is my runZero data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local SQLite copy are all local. The AI sees query results, not raw bulk data, and your API key is never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Does this burn my runZero API quota?"
    a: "Only 'inventory sync' and live queries call the API. The cross-entity analysis - triage, diff, affected, exposure-map, exposure-delta, certs-expiring, software rollup - runs entirely against the local SQLite copy, so re-slicing your attack surface a hundred ways costs zero additional API calls."
  - q: "Does it work with self-hosted runZero?"
    a: "Yes. It defaults to the hosted console at console.runzero.com; point it at your own console with RUNZERO_BASE_URL. The same API-token scopes apply."
  - q: "What token scope do I need?"
    a: "A read/Export token (Export ET, Organization OT, or Account CT key) covers sync and every analysis command. Launching a scan with scan-watch or org create-scan needs a token with scan permission. Scope the credential to only what your workflow uses."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/runzero/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/runzero/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your runZero credentials once; runzero-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a runZero question in plain language; it runs runzero-cli for you."
---

# The runZero MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by runZero, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for runZero. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects runZero to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

runZero's API answers one entity at a time; it cannot join assets to services to vulnerabilities in a single call. This skill syncs your whole attack surface into a local SQLite copy, then ranks exposure, diffs what changed since the last sync, and traces any CVE to the exact assets it hits - offline, at zero API quota. Ask in plain language; your agent runs the command and reads back the answer.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/runzero){: .btn}

## Instead of clicking through runZero, just ask

**Instead of** Export the asset inventory to CSV, pivot it against the services export, then cross-reference a third export for vulnerabilities just to see what is actually exposed
**just ask:** *"Which of our assets are most exposed right now?"*
<sub>Your agent runs: <code>runzero-cli triage --agent</code></sub>

**Instead of** Kick off a fresh discovery scan and eyeball two reports side by side trying to spot what changed on the network
**just ask:** *"What appeared or disappeared on our attack surface since last week?"*
<sub>Your agent runs: <code>runzero-cli diff --since 7d</code></sub>

**Instead of** Open the CVE bulletin, then hand-search the asset inventory hostname by hostname to find what it actually hits
**just ask:** *"Which of our assets are affected by CVE-2024-3094?"*
<sub>Your agent runs: <code>runzero-cli affected "CVE-2024-3094" --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/runzero/wide-1200x630.png" src="/assets/video/runzero/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/runzero/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which of our assets are most exposed right now? | `runzero-cli triage --agent` |
| Only the internet-facing ones? | `runzero-cli triage --internet-facing --agent` |
| What changed on our attack surface since last week? | `runzero-cli diff --since 7d` |
| What newly became exposed or vulnerable since the last sync? | `runzero-cli exposure-delta --agent` |
| Which assets are affected by a given CVE? | `runzero-cli affected "CVE-2024-3094" --agent` |
| Where are risky services concentrated in a subnet? | `runzero-cli exposure-map "10.0.0.0/8" --agent` |
| Which TLS certificates are expiring soon or using weak crypto? | `runzero-cli certs-expiring --days 90 --weak` |
| Which assets are stale, end-of-life, or unowned? | `runzero-cli stale --days 90 --json` |
| How many assets run a given software product, by version? | `runzero-cli software rollup "openssl" --agent` |
| Scan a subnet on a site and wait for the result? | `runzero-cli scan-watch "<site_id>" --targets "10.0.0.0/24"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/runzero/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/runzero/guide.md).

## What makes this one different

Most runZero integrations proxy each question straight into a live API call. That works for one lookup and falls apart at scale: the API is scoped per entity, so 'which critical assets run a vulnerable service' is three calls and a manual join. This skill syncs the whole surface into a local SQLite copy once, then answers cross-entity questions with a single offline join - instant, repeatable, and free of API quota. Because every sync is a snapshot, it can also diff two points in time, which a stateless wrapper cannot do at all.

runZero's console and API stay the source of truth for discovery; this skill does not replace them. It adds the offline cross-entity layer the API cannot return in one call - point-in-time diffs, CVE-to-asset blast radius, and exposure ranking - so an AI agent answers a security question in one step instead of stitching three exports together.

## The pain this closes

- Asset inventory is always stale: r/msp threads return again and again to unknown devices, shadow IT, and 'you cannot secure what you cannot see' - the portal shows assets, but answering a real security question means exporting reports and joining them in a spreadsheet.
- The questions that matter are cross-entity - which critical assets run a vulnerable service, what newly became exposed, which machines a CVE lands on - and runZero's API is scoped per entity (/org, /account, /export), so each one is several calls plus a manual join.
- Point-in-time is invisible: the console shows the surface now, not what changed since last week, so drift, newly-opened ports, and expiring certificates slip by until something breaks.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/runzero/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/runzero/install.ps1 | iex
```

After install, authenticate once with your runZero credentials, then verify with `runzero-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | triage, diff, affected, exposure-delta, exposure-map, stale, certs-expiring, software rollup, search, inventory list / status, and the non-secret account / org get-* reports (assets, sites, services, tasks, organizations, users) - NOT the credential/token/key reads below | Allow |
| Write (routine) | inventory sync (writes the local copy), org create-site, org create-scan and scan-watch (these launch a real network scan against your targets), import / runzero-import, and asset tag/owner updates | Preview with --dry-run, then a reviewed write |
| Credential / destructive / config | the secret-returning reads (account get-apitoken which mints a token, get-credential / get-credentials, get-key / get-keys, get-organization-export-token / -tokens), all credential and key writes (create-credential, create-key, rotate-key, reset-user-password / reset-user-mfa), and every delete-* / remove-* plus the org bulk-clear operations | Human-in-the-loop only |

The skill reads your runZero attack surface - assets, services, software, vulnerabilities, and certificates - and keeps a local copy you can query offline. It can also write: launch network scans, create sites, manage account users and keys, and import data, all opt-in. Most read commands are safe, but the credential, token, and key reads (e.g. account get-apitoken, get-credentials, get-keys) return or mint secrets, so treat them like writes. Keep an autonomous agent to non-secret reads plus previewed (--dry-run) writes, and require a human for scan launches, any credential / token / key operation, and any delete. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/runzero/governance.md).

## Frequently asked questions

### Is there an MCP server for runZero?

Yes - this one. A free, open source MCP server and Claude Code Skill for runZero, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the runZero MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local runZero MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your runZero API key once.

### Is my runZero data safe?

Your data stays on your machine. The CLI, MCP server, and the local SQLite copy are all local. The AI sees query results, not raw bulk data, and your API key is never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Does this burn my runZero API quota?

Only 'inventory sync' and live queries call the API. The cross-entity analysis - triage, diff, affected, exposure-map, exposure-delta, certs-expiring, software rollup - runs entirely against the local SQLite copy, so re-slicing your attack surface a hundred ways costs zero additional API calls.

### Does it work with self-hosted runZero?

Yes. It defaults to the hosted console at console.runzero.com; point it at your own console with RUNZERO_BASE_URL. The same API-token scopes apply.

### What token scope do I need?

A read/Export token (Export ET, Organization OT, or Account CT key) covers sync and every analysis command. Launching a scan with scan-watch or org create-scan needs a token with scan permission. Scope the credential to only what your workflow uses.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Blumira](/skills/blumira/) · [CIPP](/skills/cipp/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [SentinelOne](/skills/sentinelone/) · [ThreatLocker](/skills/threatlocker/)

## Status

Beta. Validated against the runZero API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
