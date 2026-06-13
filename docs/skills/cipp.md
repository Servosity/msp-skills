---
layout: default
title: "CIPP MCP Server - for Claude, ChatGPT, Copilot, and any MCP agent"
description: "First single-binary CLI for CIPP \u2014 offline SQLite store, fleet posture analytics, and cross-tenant fan-out no other CIPP tool has."
permalink: /skills/cipp/
skill_name: "CIPP MCP"
image: /assets/social/cipp/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local CIPP MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my CIPP data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local store are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Do I need my own CIPP instance to use this?"
    a: "Yes - this drives your own self-hosted CIPP instance's API. In CIPP you create an API client (it issues a client ID, secret, tenant ID, and token URL); the CLI authenticates with OAuth2 client credentials via 'cipp-cli auth login', or you save a bearer token with 'cipp-cli auth set-token'. It reads and acts through your CIPP - it does not replace it."
  - q: "Will this hit Microsoft Graph or CIPP rate limits?"
    a: "The cross-tenant rollups (posture, licenses waste, users stale, standards drift) read the local store after one fan-out, so repeat questions cost zero API calls. fanout throttles with --concurrency and the client retries 429s with Retry-After; bulk checkpoints completed rows with --resume so a throttled batch continues instead of restarting."
  - q: "Can this change my tenants, or only read?"
    a: "Both. CIPP is a full management API, so the CLI can create users, offboard, set forwarding, and more. But the fleet rollups are read-only, and bulk prints its plan by default and only writes when you pass --execute. If you want reporting only, scope the API client to read-only in CIPP - the credential is the boundary."
  - q: "Does it replace the CIPP portal?"
    a: "No. CIPP stays your portal for deep single-tenant work. This skill adds the cross-tenant reporting layer and lets your AI agent drive CIPP from natural language."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cipp/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your CIPP credentials once; cipp-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a CIPP question in plain language; it runs cipp-cli for you."
---

# CIPP + AI in 60 seconds

> Unofficial. Community-built Claude Code Skill and MCP server for the CIPP
> API. Not affiliated with, endorsed by, or sponsored by CyberDrain.

**Awaiting live verification** - passes every mechanical gate (build, command-surface, claims, install). Be the first to confirm it against your tenant: [report it works](https://github.com/Servosity/msp-skills/issues/new?template=it-works.yml).

CIPP manages Microsoft 365 across all your client tenants, but its portal and API work one tenant at a time. Ask your AI "which tenants are off our security baseline" or "where are we paying for unused licenses" and get the cross-tenant answer the UI never renders: one fan-out pulls every tenant into a local store, then posture, license-waste, stale-account, and standards-drift rollups run instantly - offline, across the whole fleet.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/cipp){: .btn}

## Instead of clicking through CIPP, just ask

**Instead of** Logging into each client's tenant one at a time to count who still hasn't registered MFA, then pasting the numbers into a QBR spreadsheet
**just ask:** *"Show me MFA registration across every tenant"*
<sub>Your agent runs: <code>cipp-cli posture --dimension mfa</code></sub>

**Instead of** Clicking through every tenant's license page to find seats you pay for but nobody uses, then reconciling against the CSP bill by hand
**just ask:** *"Which assigned M365 licenses are going unused across all my clients?"*
<sub>Your agent runs: <code>cipp-cli licenses waste</code></sub>

**Instead of** Running the same offboarding steps - block sign-in, convert mailbox, set forwarding - separately in each tenant for a batch of departures, and starting over when the API throttles you
**just ask:** *"Offboard this CSV of departures across their tenants without tripping rate limits"*
<sub>Your agent runs: <code>cipp-cli bulk --from offboards.csv --execute</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/cipp/wide-1200x630.png" src="/assets/video/cipp/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/cipp/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which tenants still have users without MFA registered? | `cipp-cli posture --dimension mfa` |
| How does Conditional Access coverage compare across all tenants? | `cipp-cli posture --dimension ca` |
| Where am I paying for M365 licenses nobody uses? | `cipp-cli licenses waste` |
| Which licensed accounts haven't signed in for 90 days, across every client? | `cipp-cli users stale --days 90` |
| Which tenants drifted off our security baseline since the last check? | `cipp-cli standards drift` |
| Pull one read across every client tenant at once and keep it locally | `cipp-cli fanout --endpoint /ListUsers --all-tenants --save` |
| Offboard a batch of departures from a CSV with 429 backoff and resume | `cipp-cli bulk --from offboards.csv --execute` |
| Are my CIPP credentials and connectivity healthy? | `cipp-cli doctor` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/cipp/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/cipp/guide.md).

## What makes this one different

Most CIPP integrations proxy each question into a live, single-tenant API call - fine for one record, but every tenant is a separate round trip and Microsoft Graph throttles you at fleet scale. This skill fans one read out across every tenant, persists the results into a local SQLite store, and then answers posture, license-waste, stale-account, and standards-drift questions from that store - instant, offline, and resumable after a 429. The drift report even compares two synced snapshots over time, history a stateless wrapper has nowhere to keep.

CIPP's portal is where you act deeply on a single tenant; this skill is where you see across all of them. It complements CIPP - it reads and writes through the same CIPP API with your own scoped credentials - and adds the fleet-wide rollups plus the local snapshot history (drift over time) that the one-tenant-at-a-time UI never renders.

## The pain this closes

- CIPP scopes nearly every endpoint by a single tenantFilter, so the portal forces you through one client tenant at a time. The fleet-wide questions an MSP owner actually asks at QBR time - how many users still lack MFA across all clients, which tenants drifted off baseline, where licenses sit unused - have no single screen; you click each tenant or script the API by hand.
- The CIPP API (per the official CIPP API documentation at docs.cipp.app) authenticates with an Azure AD app registration via OAuth2 client credentials and runs through Microsoft Graph, which throttles at scale. A bulk change across dozens of tenants hits HTTP 429 / Retry-After limits, and a halted batch means re-running everything - there is no resumable, checkpointing bulk runner.
- As of June 2026 there is no published single-binary CLI or MCP server for CIPP that we could find - automating it means hand-rolled PowerShell against the API or clicking through the portal, neither of which an AI agent can drive in one natural-language step.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cipp/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cipp/install.ps1 | iex
```

After install, authenticate once with your CIPP credentials, then verify with `cipp-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read (rollups and lists) | cipp-cli posture --dimension mfa; cipp-cli licenses waste; cipp-cli users stale --days 90; cipp-cli standards drift; cipp-cli fanout --endpoint /ListUsers --all-tenants; cipp-cli list-tenants; cipp-cli doctor | Allow |
| Write (routine) | cipp-cli add-user; cipp-cli edit-user; cipp-cli bulk --from changes.csv --execute (plans by default; writes only with --execute) | Preview with --dry-run, then a reviewed write |
| Credential / security | cipp-cli exec-reset-mfa; cipp-cli exec-per-user-mfa; cipp-cli exec-get-local-admin-password; cipp-cli exec-token-exchange | Human-in-the-loop only |
| Destructive | cipp-cli exec-offboard-user; cipp-cli remove-user; cipp-cli exec-device-delete; cipp-cli delete-sharepoint-site | Human-in-the-loop only, explicit confirmation |

The skill drives the cipp-cli and cipp-mcp binaries, authenticating to your self-hosted CIPP instance with a bearer token from CIPP_API_KEY (obtained via OAuth2 client credentials - 'auth login' performs and caches the exchange) read from the environment, never logged or sent anywhere except the CIPP API. CIPP is a read-write management API: read and rollup commands change nothing; bulk prints its plan and only writes with --execute; everything else (create/edit/offboard/delete, MFA and token actions) sends on run unless you pass --dry-run first. The real permission boundary is the scope you grant the API client in CIPP, so keep autonomous agents to reads plus previewed writes. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/cipp/governance.md).

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local CIPP MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my CIPP data safe?

Your data stays on your machine. The CLI, MCP server, and the local store are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need my own CIPP instance to use this?

Yes - this drives your own self-hosted CIPP instance's API. In CIPP you create an API client (it issues a client ID, secret, tenant ID, and token URL); the CLI authenticates with OAuth2 client credentials via 'cipp-cli auth login', or you save a bearer token with 'cipp-cli auth set-token'. It reads and acts through your CIPP - it does not replace it.

### Will this hit Microsoft Graph or CIPP rate limits?

The cross-tenant rollups (posture, licenses waste, users stale, standards drift) read the local store after one fan-out, so repeat questions cost zero API calls. fanout throttles with --concurrency and the client retries 429s with Retry-After; bulk checkpoints completed rows with --resume so a throttled batch continues instead of restarting.

### Can this change my tenants, or only read?

Both. CIPP is a full management API, so the CLI can create users, offboard, set forwarding, and more. But the fleet rollups are read-only, and bulk prints its plan by default and only writes when you pass --execute. If you want reporting only, scope the API client to read-only in CIPP - the credential is the boundary.

### Does it replace the CIPP portal?

No. CIPP stays your portal for deep single-tenant work. This skill adds the cross-tenant reporting layer and lets your AI agent drive CIPP from natural language.


## Status

Beta. Validated against the CIPP API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
