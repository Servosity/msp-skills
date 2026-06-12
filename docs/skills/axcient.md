---
layout: default
title: "Axcient x360Recover MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Every x360Recover endpoint plus the fleet-wide backup-health answers the API alone cannot give: offline, joined, and agent-ready."
permalink: /skills/axcient/
skill_name: "Axcient x360Recover MCP"
image: /assets/social/axcient/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Axcient x360Recover?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Axcient x360Recover, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Axcient x360Recover MCP server safe for client data?"
    a: "Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Axcient MCP server via a secure bridge. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Axcient data safe?"
    a: "Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
  - q: "Will this hit my Axcient API rate limits?"
    a: "The local mirror exists so reads stop hitting the API. After the first sync, the fleet views (health, client-rollup, rpo, compliance, billing, appliance-map) run against local SQLite with zero API calls. Live calls respect a --rate-limit throttle, and sync is incremental - it only fetches what changed since the last checkpoint."
  - q: "What kind of Axcient credential do I need?"
    a: "An organization-scoped API key created in the x360Portal (Settings > API Keys, admin role required). The CLI sends it as the X-Api-Key header and can only see what that key is scoped to. Set it as AXCIENT_API_KEY; nothing is written to disk."
  - q: "Can I try it without a real tenant?"
    a: "Yes. Axcient hosts a public mock server - set AXCIENT_BASE_URL=https://ax-pub-recover.wiremockapi.cloud/x360recover with any non-empty AXCIENT_API_KEY and the whole CLI runs against fixtures, no real credentials needed."
  - q: "Does this cover x360Sync or x360Cloud?"
    a: "No. This skill is the x360Recover (BCDR) public API only - vaults, appliances, devices, jobs, restore points, AutoVerify, and usage. x360Sync and x360Cloud are separate products with separate APIs."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/axcient/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/axcient/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Axcient x360Recover credentials once; axcient-cli doctor confirms they work."
  - name: "Ask your first question"
    text: "Ask your AI agent a Axcient x360Recover question in plain language; it runs axcient-cli for you."
---

# The Axcient x360Recover MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Axcient, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Axcient x360Recover. It's free, open source, and runs on your own machine, so your client data never leaves your network. It connects Axcient x360Recover to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

MSPs run Axcient x360Recover across dozens of clients, but the portal answers one entity at a time and the public API famously won't tell you which client a device belongs to. Ask your AI "whose backups failed last night," "who's breaching RPO," or "what do I bill each client this month," and get the fleet-wide answer in one table - computed offline from a local mirror that joins the device, job, restore-point, and client data the raw API leaves unconnected.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/axcient){: .btn}

## Instead of clicking through Axcient x360Recover, just ask

**Instead of** Opening the x360Recover console and checking each appliance and device one at a time every morning to find the backups that failed or silently went stale overnight
**just ask:** *"Whose backups failed or went stale last night?"*
<sub>Your agent runs: <code>axcient-cli health --agent</code></sub>

**Instead of** Screenshotting restore-point ages and AutoVerify boot results device by device to assemble backup-compliance evidence for a client's QBR or an audit
**just ask:** *"Give me backup-compliance evidence for this client - restore-point age, AutoVerify, and an RPO pass/fail"*
<sub>Your agent runs: <code>axcient-cli compliance --client 42 --hours 24 --csv</code></sub>

**Instead of** Visiting each client's usage page at month-end and reconciling protected-system counts and storage against the invoice by hand
**just ask:** *"What does each client consume this month for invoice reconciliation?"*
<sub>Your agent runs: <code>axcient-cli billing --csv</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/axcient/wide-1200x630.png" src="/assets/video/axcient/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/axcient/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Whose backups failed or went stale across every client last night? | `axcient-cli health` |
| Give me one row per client: devices total, failing, stale, RPO-breach, and AutoVerify-fail counts | `axcient-cli client-rollup` |
| Which devices are past their recovery-point objective, grouped by client? | `axcient-cli rpo --hours 24` |
| Only show the devices actually breaching RPO at the cloud tier | `axcient-cli rpo --hours 24 --target cloud` |
| Produce backup-compliance evidence for one client (restore-point age + AutoVerify + RPO verdict) | `axcient-cli compliance --client 42 --hours 24 --csv` |
| Only the compliance rows that fail RPO or AutoVerify | `axcient-cli compliance --failing-only` |
| What does each client consume for invoice reconciliation this month? | `axcient-cli billing --csv` |
| Which devices does each appliance protect, and what state are those backups in? | `axcient-cli appliance-map` |
| Find everything matching a client or device name across synced data | `axcient-cli search "Acme Corp"` |
| Refresh the local mirror, then run the morning sweep | `axcient-cli sync && axcient-cli health --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/axcient/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/axcient/guide.md).

## What makes this one different

Most Axcient integrations and MCP wrappers proxy each question into a live API call - fine for one device, but a fleet-wide question becomes a token-burning loop of per-entity calls (appliance, then device, then each device's jobs) because the API has no rollup endpoint and won't join client to device for you. This skill syncs appliances, devices, jobs, restore points, AutoVerify results, and usage into a local SQLite mirror, then answers health, client-rollup, rpo, compliance, and billing as one offline join - instant, and the AI sees the answer table, not pages of nested JSON.

It complements the x360Portal rather than replacing it: the portal and the appliance/vault UI stay best for configuring protection and running actual restores, while this skill brings every client into one place for the cross-fleet questions - whose backups failed, who's breaching RPO, where billing and protection diverge - that the per-entity API and one-client-at-a-time portal can't answer in a single view.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](/skills/servosity/) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

## The pain this closes

- Axcient's x360Recover Public API is built around per-entity endpoints - one call for appliances, another for a device, another for that device's jobs - and it does not hand you the client-to-device mapping directly. So the question every MSP asks each morning, "whose backups failed," has no single fleet-wide endpoint; you walk each appliance and device by hand or click through the portal.
- A backup that "succeeded" can still be out of compliance: the job ran, but the newest restore point is hours stale, or AutoVerify never booted the image. MSPs on r/msp keep describing the same trap - finding out a recovery point wasn't there only when a restore is needed - because job-success and restore-point-age are different questions the console shows on different screens.
- Month-end reconciliation - matching protected-system counts and storage to what each client is invoiced - means opening each client's usage view and stitching it into a spreadsheet, since the portal reports one client at a time and the API exposes usage per client, not as a fleet rollup.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/axcient/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/axcient/install.ps1 | iex
```

After install, authenticate once with your Axcient x360Recover credentials, then verify with `axcient-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | axcient-cli health; axcient-cli client-rollup; axcient-cli rpo --hours 24; axcient-cli compliance --client 42 --hours 24 --csv; axcient-cli billing; axcient-cli appliance-map; axcient-cli clients get; axcient-cli vault get; axcient-cli device get-by-org-id-org-level; axcient-cli organization; axcient-cli search "Acme Corp" | Allow |
| Write (routine) | axcient-cli vault threshold set-by-vault-id <vault_id> --threshold 60 (change a vault connectivity threshold); axcient-cli import <resource> --input data.jsonl (bulk - one create/upsert POST per JSONL record) - writes send immediately; --dry-run is an opt-in preview, not a default | Preview with --dry-run, then a reviewed write |
| Destructive / config | axcient-cli client vault get-d2c-agent-token-by-client-and-ids <client_id> <vault_id> (mints a direct-to-cloud agent install token - a deployment credential) | Human-in-the-loop only |

The skill drives the axcient-cli and axcient-mcp binaries, authenticating with an organization-scoped API key (AXCIENT_API_KEY) read from the environment and sent as the X-Api-Key header - never written to disk, never logged, never sent anywhere except the Axcient API. The surface is overwhelmingly read-only: every fleet view (health, client-rollup, rpo, compliance, billing, appliance-map) and every resource read cannot change anything. Only three commands write - vault threshold set-by-vault-id and the bulk import <resource> mutate or create data, and client vault get-d2c-agent-token-by-client-and-ids mints a direct-to-cloud agent install token - so the recommended policy is read plus previewed (--dry-run) writes, with a human in the loop for the token mint. The strongest control is the scope of the API key you create in the x360Portal. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/axcient/governance.md).

## Frequently asked questions

### Is there an MCP server for Axcient x360Recover?

Yes - this one. A free, open source MCP server and Claude Code Skill for Axcient x360Recover, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Axcient x360Recover MCP server safe for client data?

Yes, by design. The CLI, the MCP server, and any local data mirror run on your own machine - nothing is sent to MSP Skills or any third party. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so you expose the local Axcient MCP server via a secure bridge. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Axcient data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Axcient API rate limits?

The local mirror exists so reads stop hitting the API. After the first sync, the fleet views (health, client-rollup, rpo, compliance, billing, appliance-map) run against local SQLite with zero API calls. Live calls respect a --rate-limit throttle, and sync is incremental - it only fetches what changed since the last checkpoint.

### What kind of Axcient credential do I need?

An organization-scoped API key created in the x360Portal (Settings > API Keys, admin role required). The CLI sends it as the X-Api-Key header and can only see what that key is scoped to. Set it as AXCIENT_API_KEY; nothing is written to disk.

### Can I try it without a real tenant?

Yes. Axcient hosts a public mock server - set AXCIENT_BASE_URL=https://ax-pub-recover.wiremockapi.cloud/x360recover with any non-empty AXCIENT_API_KEY and the whole CLI runs against fixtures, no real credentials needed.

### Does this cover x360Sync or x360Cloud?

No. This skill is the x360Recover (BCDR) public API only - vaults, appliances, devices, jobs, restore points, AutoVerify, and usage. x360Sync and x360Cloud are separate products with separate APIs.


## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](/skills/acronis/) · [Afi](/skills/afi/) · [Cove Data Protection](/skills/cove/) · [Datto BCDR](/skills/datto-bcdr/) · [Servosity](/skills/servosity/) · [SkyKick](/skills/skykick/)

## Status

Beta. Validated against the Axcient x360Recover API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
