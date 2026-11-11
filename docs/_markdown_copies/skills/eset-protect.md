> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/eset-protect/

# The ESET PROTECT MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by ESET, spol. s r.o..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for ESET PROTECT. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects ESET PROTECT to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI which customers have ESET devices that need attention, which machines stopped checking in, who is exposed to a new CVE, and what the morning detection queue looks like, across every customer at once. The CLI syncs your ESET PROTECT tree into a local SQLite mirror, rolls it up per customer, and shows the exact scan, isolate or release task before it is sent.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/eset-protect/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/eset-protect)

## Instead of clicking through ESET PROTECT, just ask

**Instead of** Reading the Managed Customers page, then drilling into each customer to find the devices that need attention, are muted or stopped checking in
**just ask:** *"Which of my customers have ESET devices that need attention or stopped checking in?"*
Your agent runs: `eset-protect-cli fleet health`

**Instead of** Searching the vulnerability view for a new CVE, then working out which customer each affected device belongs to
**just ask:** *"Which customers and devices are exposed to CVE-2024-3400, and is there a patch?"*
Your agent runs: `eset-protect-cli exposure --cve CVE-2024-3400`

**Instead of** Pulling one customer's device, detection, incident and patch numbers into a QBR deck by hand
**just ask:** *"Build Acme Dental's quarterly security summary from ESET."*
Your agent runs: `eset-protect-cli report customer "Acme Dental" --since 90d`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/eset-protect/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which customers have devices that need attention, are muted or stopped checking in? | `eset-protect-cli fleet health` |
| What should my techs work first this morning, detections and open incidents together? | `eset-protect-cli triage --since 24h --min-severity medium` |
| Which customers and devices are exposed to this CVE, and is a patch available? | `eset-protect-cli exposure --cve CVE-2024-3400` |
| Give me one customer's quarter: device health, detections, incidents, top CVEs and patches | `eset-protect-cli report customer "Acme Dental" --since 90d` |
| Which devices have not checked in for two weeks, most stale first? | `eset-protect-cli fleet stale --days 14` |
| Which security patches are still missing, ranked by the devices and CVE risk they fix? | `eset-protect-cli patches backlog --security-only` |
| Is the same file hash showing up at more than one customer this week? | `eset-protect-cli detections spread --since 7d --min-customers 2` |
| Isolate this machine from the network (show me the exact task first) | `eset-protect-cli respond isolate "FRONTDESK-01"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/eset-protect/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/eset-protect/guide.md).

## What makes this one different

MCP servers such as [maciekaz/ESET-MCP](https://github.com/maciekaz/ESET-MCP) and [Fenrindale/eset-protect-mcp](https://github.com/Fenrindale/eset-protect-mcp) expose the ESET Connect endpoints as tools and answer each question with live calls. This connector also covers the endpoints, and adds a local SQLite mirror of your ESET PROTECT tree, so per-customer rollups such as fleet health, the triage queue and the QBR one-pager are joins over synced devices, groups, detections and incidents, attributed to the customer each device belongs to.

ESET's own [ESET LiveAI](https://www.eset.com/us/business/solutions/liveai/) (formerly AI Advisor) is a generative AI assistant inside ESET PROTECT and ESET Inspect, in the ESET PROTECT Premium and Ultimate tiers. This skill complements it: it answers across-every-customer questions in whichever AI agent you already use, through the ESET Connect API, and it does not replace the ESET PROTECT console.

## The pain this closes

- The ESET Connect API returns devices, detections and incidents by device group, with no field that names the customer, so every per-customer answer an AI agent gives means walking the group tree first.
- ESET PROTECT's Managed Customers page shows per-customer device and detection counts; turning that into a worst-first list of stale and muted devices, a CVE exposure list or a QBR page is still manual work.
- Detections are split across API versions (v1 for ESET PROTECT and ESET Inspect, the v2 list for ESET Cloud Office Security) and incidents live in another service, so the API has no single morning queue.
- The API mints one-hour tokens with a password grant and answers slow requests with 202 and a response id to poll, which breaks simple scripts and some SIEM integrations.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/eset-protect/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/eset-protect/install.ps1 | iex
```

After install, authenticate once with your ESET PROTECT credentials, then verify with `eset-protect-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet health, triage, exposure, report customer, patches backlog, devices list, incidents list, detections list | Allow |
| Endpoint response | respond scan, respond isolate, respond release (preview unless --yes), device-tasks create, device-tasks update-targets | Human-in-the-loop, explicit confirmation |
| Protection and policy config | executables block, edr-rules create, edr-rule-exclusions create, policy-assignments assign-policy, devices move | Preview with --dry-run, then a human-approved write |
| Write (routine) | incidents close, incidents reopen, detection-groups resolve, devices rename, groups create | Preview with --dry-run, then a reviewed write |
| Credential and access | installers list, gpo-sccm-files-generate, mobile-devices-batch-get-enrollment-links, roles create, role-assignments-assign-role | Human-in-the-loop only |
| Destructive / high-risk | groups delete, policies delete, edr-rules delete, quarantined-objects-restore, quarantined-objects-purge, import | Human-in-the-loop only, explicit confirmation |

The skill signs in as the ESET Connect API user you create, so that user's access rights are the real boundary. Reads, reports and the local mirror are safe to allow. Response actions (scan, isolate, release), policy and EDR rule changes, quarantine restores and deletes reach real customer machines and need a human to approve the exact request. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/eset-protect/governance.md).

## Frequently asked questions

### Is there an MCP server for ESET PROTECT?

Yes - this one. A free, open source MCP server and Claude Code Skill for ESET PROTECT, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the ESET PROTECT MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `ESET_PROTECT_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `ESET_PROTECT_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. The API user's password stays in your environment and is never written to disk, but the access and refresh tokens the CLI mints are cached in its credentials.toml (owner-only permissions), so removing the environment variables does not revoke access - run `eset-protect-cli auth logout` on every machine that cached them, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans, if its connector settings can send a bearer token. ChatGPT connects to remote MCP servers over HTTPS, so you run eset-protect-mcp --transport http with ESET_PROTECT_MCP_HTTP_TOKEN set to at least 32 characters on an HTTPS host you control (it refuses to start without the token, and every request must send it). Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my ESET data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What ESET account do I need?

A dedicated ESET Connect API user. A Superuser or Root creates it in ESET PROTECT Hub, ESET Business Account or ESET MSP Administrator 2 and turns on the Integrations toggle; the Superuser or Root account itself cannot be given API access. The new user accepts the invitation email and finishes account setup, and a Hub user must also sign in to ESET PROTECT Hub once. Set ESET_PROTECT_USERNAME and ESET_PROTECT_PASSWORD; the CLI exchanges them for a one-hour token.

### Which region should I set?

Set ESET_PROTECT_REGION to the region of your ESET PROTECT console: eu (the default), de, us, ca or jpn. The jpn region has no quarantine or Cloud Office user API, and no v2 detection list.

### Will this hit ESET Connect rate limits?

ESET Connect allows 10 calls per second for each set of credentials, account and source IP, and answers 429 above that; under its Fair Use Policy a very large burst is slowed to 2 requests per second and the rest is rejected with 50x. The CLI slows down and retries when it sees a 429; set --rate-limit 8 to stay under the limit from the start and keep sync --concurrency low. Slow requests answered with 202 and a response id are polled until --timeout runs out (60 seconds by default). ESET answers 504 to a query that is not ready within 120 seconds, so pass --timeout 3m for large queries and narrow the query if ESET still answers 504.

### Can the AI isolate a machine or change a policy by itself?

The respond commands only print the task unless you add --yes, and over MCP they only preview. Other write commands run when invoked on the command line unless you pass --dry-run. Over MCP, the generic eset-protect_execute tool can reach any state-changing ESET Connect endpoint, but it only previews the call until the call carries confirm: true. For a read-only agent, set ESET_PROTECT_MCP_READ_ONLY=1 in the MCP server's environment: no tool that changes ESET is offered and installer and enrollment links are refused. Keep your AI app's approval prompt on.

### Does it work with on-premises ESET PROTECT?

No. It signs in through the ESET Connect cloud API, so it works with ESET PROTECT (cloud). An on-premises ESET PROTECT server uses a different API and sign-in, which this connector does not support.

### Does it replace the ESET PROTECT console?

No. The console stays the place for policy editing, investigations in ESET Inspect and day-to-day administration. This skill answers the across-every-customer questions and runs previewed actions from your AI agent.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [Cisco Umbrella](https://msp-skills.compoundingteams.com/skills/cisco-umbrella.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [DNSFilter](https://msp-skills.compoundingteams.com/skills/dnsfilter.md) · [Exchange Online](https://msp-skills.compoundingteams.com/skills/exchange-online.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Hook Security](https://msp-skills.compoundingteams.com/skills/hook-security.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [Keeper Admin and MSP Console](https://msp-skills.compoundingteams.com/skills/keeper.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the ESET PROTECT API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
