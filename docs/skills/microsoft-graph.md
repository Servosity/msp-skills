---
layout: default
title: "Microsoft Graph MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "The maintained single-binary successor to the retiring mgc \u2014 every MSP-relevant Microsoft Graph surface, plus an offline store that finds wasted licenses, privileged-access risks, and stale devices no single API call can."
permalink: /skills/microsoft-graph/
skill_name: "Microsoft Graph MCP"
image: /assets/social/microsoft-graph/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Microsoft Graph?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Microsoft Graph, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Microsoft Graph MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Microsoft Graph MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Microsoft 365 data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local SQLite mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills - the token is read from your environment and used only against the Microsoft Graph API."
  - q: "Will this hit my Microsoft Graph throttling limits?"
    a: "The local mirror exists so reads stop hitting Graph. After the first `pull`, the cross-entity views (licenses waste/orphans/map, admins audit, security triage, managed-devices drift, groups risk, tenant snapshot) run against local SQLite with zero API calls. Live calls follow @odata.nextLink and respect a `--rate-limit` throttle, and pull treats resources your token can't reach as warnings, not failures."
  - q: "Is this the replacement for the Microsoft Graph CLI (mgc) that's being retired?"
    a: "It is built as the lightweight successor for the MSP read-and-report core - directory, licensing, security, and device surfaces - as one cross-platform Go binary with no .NET or PowerShell runtime. Microsoft's own recommended path is the PowerShell SDK; this is the option for teams who want a scriptable single binary and their AI agent instead. It is not affiliated with or endorsed by Microsoft."
  - q: "Does it use a delegated or app-only token?"
    a: "Either. Run `auth login --tenant <id> --client-id <id> --client-secret <secret>` to mint and cache an app-only (client-credentials) token for unattended MSP use, or export a pre-minted token as MICROSOFT_GRAPH_TOKEN. Read scopes such as Directory.Read.All, RoleManagement.Read.Directory, SecurityAlert.Read.All, and DeviceManagementManagedDevices.Read.All must be granted and admin-consented. App-only tokens have no /me, so `users me` is delegated-only."
  - q: "How do I audit which third-party apps have been consented into a tenant?"
    a: "Run `microsoft-graph-cli apps consent --agent`. It inventories every non-Microsoft enterprise application (service principal) and the access it holds - joining service principals, delegated consent grants (oauth2PermissionGrants), and application/app-only permissions (appRoleAssignments) into one risk-ranked table. It flags over-privileged apps, admin-consented (tenant-wide) apps, privilege-escalation permissions, and user-consented shadow IT, all read-only. Microsoft's own first-party apps are counted but not listed so the report is just the consent you granted or inherited. Needs read scopes Application.Read.All, Directory.Read.All, and DelegatedPermissionGrant.Read.All."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Microsoft Graph credentials once; microsoft-graph-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Microsoft Graph question in plain language; it runs microsoft-graph-cli for you."
---

# The Microsoft Graph MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Microsoft Corporation.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Microsoft Graph. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Microsoft Graph to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Microsoft retires the Graph CLI (mgc) on August 28, 2026 and points admins at the heavier PowerShell SDK. This is the lightweight successor: one cross-platform binary, no .NET or PowerShell runtime. Ask your AI "which M365 licenses are we wasting," "who holds privileged admin right now," or "what's new in Defender since yesterday," and get cross-tenant answers computed offline from a local SQLite mirror - one query instead of CSV exports and portal tab-hopping.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/microsoft-graph){: .btn}

## Instead of clicking through Microsoft Graph, just ask

**Instead of** Exporting subscribedSku CSVs from the M365 admin center and reconciling assigned-versus-used seats in a spreadsheet to find license spend you can reclaim at renewal
**just ask:** *"Which M365 licenses are we paying for but not using?"*
<sub>Your agent runs: <code>microsoft-graph-cli licenses waste --agent</code></sub>

**Instead of** Clicking through Entra > Roles and administrators, opening each privileged role, and reading its members one role at a time to see who can administer the tenant
**just ask:** *"Who holds global admin or other privileged roles right now?"*
<sub>Your agent runs: <code>microsoft-graph-cli admins audit --agent</code></sub>

**Instead of** Paging through the Defender portal every morning to work out which alerts are new and still open since yesterday
**just ask:** *"What security alerts are new and still open since yesterday?"*
<sub>Your agent runs: <code>microsoft-graph-cli security triage --since 24h --agent</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/microsoft-graph/wide-1200x630.png" src="/assets/video/microsoft-graph/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/microsoft-graph/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which SKUs are we paying for but not fully using, ranked by wasted seats? | `microsoft-graph-cli licenses waste --agent` |
| Which disabled or guest accounts still hold a paid license? | `microsoft-graph-cli licenses orphans --json` |
| Who exactly is consuming one specific SKU before I reclaim seats? | `microsoft-graph-cli licenses map "ENTERPRISEPACK" --agent` |
| Who holds a privileged directory role right now, and which holders are guest or disabled? | `microsoft-graph-cli admins audit --agent` |
| What open security alerts are new since yesterday, by severity and source? | `microsoft-graph-cli security triage --since 24h --agent` |
| Which Intune devices are non-compliant, unencrypted, or stale this month? | `microsoft-graph-cli managed-devices drift --days 30 --agent` |
| Which groups are ownerless, empty, or guest-heavy across the tenant? | `microsoft-graph-cli groups risk --agent` |
| Which third-party apps have been consented into the tenant, and which are over-privileged, admin-consented, or user-consented shadow IT? | `microsoft-graph-cli apps consent --agent` |
| Where does this tenant stand overall - users, license waste, admins, alerts, device drift? | `microsoft-graph-cli tenant snapshot --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/microsoft-graph/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/microsoft-graph/guide.md).

## What makes this one different

Most Microsoft Graph integrations and MCP servers proxy each question into a live Graph call - fine for one record, but a tenant-wide question becomes a paginate-and-join dance the AI burns context on, and Graph throttles the bulk reads those questions need. This skill pulls the MSP-relevant surface into a local SQLite mirror, so the cross-entity answers - license waste, orphaned SKUs, privileged-access audit, device drift, tenant snapshot - become one local join: instant, offline, and the AI sees the answer, not pages of raw Graph JSON.

It is the lightweight replacement for the retiring mgc rather than a competitor to the platform: the M365 admin center, Entra, Defender, and Intune portals stay best for in-console workflows and writes, while this skill brings the read-and-report core to whichever AI agent you already use - as one cross-platform binary with no .NET or PowerShell runtime - and answers the cross-entity questions no single portal screen composes.

## The pain this closes

- Microsoft is retiring the Microsoft Graph CLI (mgc) on August 28, 2026 - deprecated since September 2025, no new features, security fixes only - and steering everyone to the PowerShell SDK (Microsoft 365 Developer Blog, "Microsoft Graph CLI retirement"). MSPs who scripted tenant reporting on a lightweight cross-platform binary now face a heavier .NET/PowerShell dependency on every machine that runs it.
- The questions an MSP actually asks about a tenant - how much license spend is recoverable, who holds admin, which devices are drifting out of compliance - span multiple Graph entities, and no single Graph endpoint returns them. The admin center and Defender/Intune portals answer one object at a time, so each question becomes a CSV export plus a spreadsheet join or a click-path across modules.
- Microsoft Graph throttles bulk reads and paginates everything behind @odata.nextLink, so any script that wants a tenant-wide view - all users with their licenses, every privileged role with its members - has to fetch, page, cache, and join locally rather than ask the API for the answer directly.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/microsoft-graph/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/microsoft-graph/install.ps1 | iex
```

After install, authenticate once with your Microsoft Graph credentials, then verify with `microsoft-graph-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | microsoft-graph-cli licenses waste --agent; microsoft-graph-cli admins audit --agent; microsoft-graph-cli apps consent --agent; microsoft-graph-cli security triage --since 24h --agent; microsoft-graph-cli managed-devices drift --days 30 --agent; microsoft-graph-cli groups risk --agent; microsoft-graph-cli tenant snapshot --agent; microsoft-graph-cli users list --top 50 --agent; microsoft-graph-cli pull; microsoft-graph-cli search "disk full" | Allow |
| Write (import escape hatch) | microsoft-graph-cli import <resource> --input data.jsonl - the only write path; issues a POST per JSONL record. Pass --dry-run to preview the requests without sending | Preview with --dry-run, then a reviewed write |
| Destructive / config | No typed destructive command exists; the CLI exposes no delete or update path. Any irreversible change would require a write the typed commands do not provide | Human-in-the-loop only |

The skill drives the microsoft-graph-cli and microsoft-graph-mcp binaries, authenticating with a MICROSOFT_GRAPH_TOKEN read from the environment - never logged, never written to disk, never sent anywhere except the Microsoft Graph API. Every typed command is read-only: users, groups, directory roles, licenses, devices, managed devices, security alerts and incidents, third-party app consent (`apps consent`), and the cross-entity analytics change nothing. The single write path is the explicit `import` command (a JSONL-to-POST create path), which previews with `--dry-run`. The strongest control is the scope of the token you mint - grant read-only Graph scopes and the CLI can only read. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/microsoft-graph/governance.md).

## Frequently asked questions

### Is there an MCP server for Microsoft Graph?

Yes - this one. A free, open source MCP server and Claude Code Skill for Microsoft Graph, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Microsoft Graph MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Microsoft Graph MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Microsoft 365 data safe?

Your data stays on your machine. The CLI, MCP server, and the local SQLite mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills - the token is read from your environment and used only against the Microsoft Graph API.

### Will this hit my Microsoft Graph throttling limits?

The local mirror exists so reads stop hitting Graph. After the first `pull`, the cross-entity views (licenses waste/orphans/map, admins audit, security triage, managed-devices drift, groups risk, tenant snapshot) run against local SQLite with zero API calls. Live calls follow @odata.nextLink and respect a `--rate-limit` throttle, and pull treats resources your token can't reach as warnings, not failures.

### Is this the replacement for the Microsoft Graph CLI (mgc) that's being retired?

It is built as the lightweight successor for the MSP read-and-report core - directory, licensing, security, and device surfaces - as one cross-platform Go binary with no .NET or PowerShell runtime. Microsoft's own recommended path is the PowerShell SDK; this is the option for teams who want a scriptable single binary and their AI agent instead. It is not affiliated with or endorsed by Microsoft.

### Does it use a delegated or app-only token?

Either. Run `auth login --tenant <id> --client-id <id> --client-secret <secret>` to mint and cache an app-only (client-credentials) token for unattended MSP use, or export a pre-minted token as MICROSOFT_GRAPH_TOKEN. Read scopes such as Directory.Read.All, RoleManagement.Read.Directory, SecurityAlert.Read.All, and DeviceManagementManagedDevices.Read.All must be granted and admin-consented. App-only tokens have no /me, so `users me` is delegated-only.

### How do I audit which third-party apps have been consented into a tenant?

Run `microsoft-graph-cli apps consent --agent`. It inventories every non-Microsoft enterprise application (service principal) and the access it holds - joining service principals, delegated consent grants (oauth2PermissionGrants), and application/app-only permissions (appRoleAssignments) into one risk-ranked table. It flags over-privileged apps, admin-consented (tenant-wide) apps, privilege-escalation permissions, and user-consented shadow IT, all read-only. Microsoft's own first-party apps are counted but not listed so the report is just the consent you granted or inherited. Needs read scopes Application.Read.All, Directory.Read.All, and DelegatedPermissionGrant.Read.All.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Blumira](/skills/blumira/) · [CIPP](/skills/cipp/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [ThreatLocker](/skills/threatlocker/)

## Status

Beta. Validated against the Microsoft Graph API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
