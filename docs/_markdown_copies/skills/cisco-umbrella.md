> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/cisco-umbrella/

# The Cisco Umbrella MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Cisco Systems, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Cisco Umbrella. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Cisco Umbrella to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask which customers have roaming computers off, uninstalled or not checking in, and get one row per customer. Cisco Umbrella plus your AI agent mints a token for each child org in your MSP console and rolls up deployment health, instead of switching into one customer at a time. It also gives one verdict for a suspicious domain and reconciles a block list against your own file.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/cisco-umbrella/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/cisco-umbrella)

## Instead of clicking through Cisco Umbrella, just ask

**Instead of** Switch into each customer's dashboard in the MSP console and read down its roaming computers, virtual appliances and tunnels
**just ask:** *"Which customers have roaming computers off, uninstalled or not checking in this week?"*
Your agent runs: `cisco-umbrella-cli fleet health --stale-days 7 --agent`

**Instead of** Look a domain up in Investigate, then search every destination list for it, then run an activity report to see who resolved it
**just ask:** *"Is login.example.com blocked, what does Umbrella say about it, and who hit it in the last day?"*
Your agent runs: `cisco-umbrella-cli verdict login.example.com --agent`

**Instead of** Compare a destination list with your blocklist file by hand and add or remove entries in the portal one at a time
**just ask:** *"What would change if I made destination list 1234567 match my blocklist file?"*
Your agent runs: `cisco-umbrella-cli destination-lists diff 1234567 --file blocklist.txt --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/cisco-umbrella/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which customers have roaming computers off, uninstalled, stale or without web protection, and how many seats each? | `cisco-umbrella-cli fleet health --stale-days 7 --agent` |
| In one customer, which roaming computers, virtual appliances, tunnels and networks have a deployment problem? | `cisco-umbrella-cli hygiene --stale-days 14 --agent` |
| For a domain from a ticket: its category, risk score, security status, which of our lists block or allow it, and who resolved it? | `cisco-umbrella-cli verdict login.example.com --agent` |
| Which allow-list entries are now categorized as malicious, conflict with a block-list entry, or were added this week? | `cisco-umbrella-cli allowlist audit --since 7d --agent` |
| What would I add and remove to make a destination list match my blocklist file? | `cisco-umbrella-cli destination-lists diff 1234567 --file blocklist.txt --agent` |
| For one customer's QBR: request summary, top threat types and blocked categories against the prior 30 days, and deployment status? | `cisco-umbrella-cli report-card --org 8675309 --from -30days --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/cisco-umbrella/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/cisco-umbrella/guide.md).

## What makes this one different

This skill covers the whole current Umbrella v2 API and is built for the MSP console: it mints a token per child org (X-Umbrella-OrgId) and keeps each customer's synced data in its own local store, so a fleet rollup or an offline search is one command. Existing integrations, such as MSPbots' Cisco Umbrella MCP server and the Splunk and XSOAR add-ons, cover a slice of the API (activity reports, or destination-list edits) one call at a time.

The Umbrella dashboard and MSP console stay your place for policy design and centralized reports; this skill answers the cross-customer and multi-step questions (fleet health, a domain verdict, a list diff) in the AI agent you already use, and complements the console rather than replacing it.

## The pain this closes

- In an Umbrella MSP or Multi-org console, deployment status lives inside each child org: Cisco's own description is that the console lists every child org so you can "switch in and out between customers" (Cisco partner blog, December 2022). A fleet-wide health check is one customer at a time.
- Answering "is this domain blocked for this client, and who hit it?" means three places: Investigate for the category and risk score, every destination list (including parent-domain entries) for membership, and an activity report for who resolved it.
- Keeping a destination list in step with a blocklist you maintain elsewhere is a manual diff, and an allow-list entry that has since been categorized as malicious stays allowed until someone notices.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md) |
| **ChatGPT** (desktop app) | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/chatgpt.md) |
| **Claude Code** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-code.md) |
| **Codex CLI** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/codex.md) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](https://msp-skills.compoundingteams.com/which-agent.md) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cisco-umbrella/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cisco-umbrella/install.ps1 | iex
```

After install, authenticate once with your Cisco Umbrella credentials, then verify with `cisco-umbrella-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet health, hygiene, verdict, allowlist audit, report-card, destination-lists diff (without --apply), activity, reports, networks list, roaming-computers list, sync, search | Allow |
| Write (routine) | destination-lists add-destinations/create/update, destination-lists diff --apply, networks/sites/internal-networks/internal-domains/network-devices create/update, tags create/add-to-devices, application-lists create/update, policies update-policy-identities, swg-device-settings set, roaming-computers update, virtual-appliances update, tunnels update, provider-customers and managed-customers create/update, console-config create/update, import | Preview with --dry-run, then a reviewed write |
| Credential / secret-bearing | api-keys update, tunnels add and tunnels update-credentials with a secret you chose, password-resets, users create, auth login, auth set-token | Human-in-the-loop only |
| Refused here | api-keys create, api-keys refresh, s3-bucket-keys, tunnels add with no secret of your own, tunnels update-credentials with auto-rotate or no secret of your own | Do it in the Umbrella dashboard: the new secret is shown only once and this program withholds it |
| Destructive | api-keys delete, destination-lists delete/remove-destinations, networks/sites/internal-networks/internal-domains/network-devices delete, roaming-computers delete, virtual-appliances delete, tunnels delete, tags remove-from-devices, users delete, managed-customers delete, provider-customers delete, application-lists delete, console-config delete-*, policies delete-policy-identities, swg-device-settings remove, local-data purge | Human-in-the-loop only, explicit confirmation |

The skill reads Umbrella deployments, policies, destination lists, reports, Investigate data and MSP customer records, and keeps a local store for offline questions; credential values are withheld from its output and never kept in the SQLite store; the key, secret and bearer token are saved to an owner-only, unencrypted credentials.toml until auth logout. It can also create, change and delete networks, sites, destination lists, tunnels, users and customer records, and it can change API keys and set tunnel credentials you chose. It refuses five actions, because Umbrella returns a new secret only once and this program withholds it: api-keys create, api-keys refresh, s3-bucket-keys, tunnels add with no secret of your own and tunnels update-credentials with auto-rotate or no secret of your own. Do those in the Umbrella dashboard. Let an agent read freely, preview routine writes with --dry-run, and keep a human approving deletes and anything that changes a credential. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/cisco-umbrella/governance.md).

## Frequently asked questions

### Is there an MCP server for Cisco Umbrella?

Yes - this one. A free, open source MCP server and Claude Code Skill for Cisco Umbrella, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Cisco Umbrella MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CISCO_UMBRELLA_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CISCO_UMBRELLA_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Cisco Umbrella MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Cisco Umbrella data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Does it work with an MSP or Multi-org console?

Yes. Create the API key in the parent console. Every command takes --org <child org ID> (or CISCO_UMBRELLA_ORG_ID) and mints a token for that customer with the X-Umbrella-OrgId header; fleet health walks every managed customer for you. The child token stays in memory and each customer's synced data goes to its own local store.

### Which API key scopes does it need?

Only the scopes for what you run: Reports for activity, report-card and verdict's resolver lookup, Deployments for networks, roaming computers and fleet health, which also needs the Admin customers scope, Policies for destination lists, Investigate for categorization and risk scores, Admin for keys and users. A section the key cannot reach returns 403 and is skipped with a note where the command says so; read-only keys cannot change anything.

### Will this hit the Umbrella API rate limits?

It is built to stay inside them. Token requests are limited to 20 a minute, so fleet health starts customers 3 seconds apart; narrow a large fleet with --customers or --max-customers. Sync once and search, hygiene and analytics answer from the local store without new API calls.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [DNSFilter](https://msp-skills.compoundingteams.com/skills/dnsfilter.md) · [Exchange Online](https://msp-skills.compoundingteams.com/skills/exchange-online.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the Cisco Umbrella API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
