> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/sentinelone/

# The SentinelOne MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by SentinelOne, Inc.

**✓ Live-verified by an MSP via GitHub report** against a production tenant · 2026-06-16. The version it verified was not recorded.

Yes - there is an MCP server for SentinelOne. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects SentinelOne to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Running SentinelOne across a book of customer sites? Ask your AI "what should I triage first across every client," "which endpoints went dark or dropped to detect-only," or "where did this malicious file spread," and get one cross-site answer the console can't compose. Every site is mirrored into a local store, so one triage worklist, one fleet-health rollup, and one posture scorecard replace the morning ritual of flipping the console scope selector tenant by tenant.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/sentinelone/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/sentinelone)

## Instead of clicking through SentinelOne, just ask

**Instead of** Logging into the SentinelOne console every morning and flipping the scope selector site by site to work out which open threats actually matter most across all your clients
**just ask:** *"Give me the ranked triage worklist of every open threat across all my client sites"*
Your agent runs: `sentinelone-cli threats triage`

**Instead of** During an incident, searching each endpoint in turn to find everywhere a malicious file landed and checking one by one whether it has actually been contained
**just ask:** *"Show me every endpoint this hash touched and which are still active, not mitigated"*
Your agent runs: `sentinelone-cli threats blast-radius "3f5a9c2e1b7d8a4f6c0e2d1a9b8c7f6e5d4c3b2a"`

**Instead of** Checking each client's agents one console scope at a time to catch the ones that went offline, fell behind on agent version, or quietly dropped from Protect to detect-only mode
**just ask:** *"Which endpoints are decaying worst first - offline, out-of-date, infected, or under-protected?"*
Your agent runs: `sentinelone-cli fleet-health stale`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/sentinelone/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What should I triage first across all my client sites right now? | `sentinelone-cli threats triage` |
| Where did this malicious file spread, and which endpoints are still active? | `sentinelone-cli threats blast-radius "Mimikatz"` |
| Which endpoints are decaying - offline, out-of-date, infected, or under-protected? | `sentinelone-cli fleet-health stale --min-score 50` |
| Which clients have protection gaps (detect-only, Ranger off, firewall off)? | `sentinelone-cli coverage gaps` |
| What changed across the whole fleet since yesterday? | `sentinelone-cli whatchanged --since 24h` |
| Which threats keep coming back after we mitigated them? | `sentinelone-cli threats recurrence` |
| Are we hitting our mitigation SLA, and where are the breaches? | `sentinelone-cli threats mttr --sla 4` |
| Rank my clients by risk so I know which tenant to call first? | `sentinelone-cli sites risk` |
| Give me one posture scorecard per client for the QBR deck? | `sentinelone-cli posture` |
| Pull every site's SentinelOne data into a local mirror for offline queries? | `sentinelone-cli sync` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/sentinelone/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/sentinelone/guide.md).

## What makes this one different

The SentinelOne Management API is paginated and scope-bound: a live API wrapper answering a book-wide question has to page through and re-query site by site, burning agent context on raw JSON it then has to summarize. This skill syncs every site into a local SQLite mirror and keeps a history snapshot on each sync, so cross-site questions become one offline query the agent reads as a finished answer - and time-aware questions a stateless wrapper simply cannot answer (what changed since yesterday, mitigation MTTR and SLA breaches, version-rollout progress, verdict flips between syncs) fall out of that stored history.

It complements the SentinelOne console and Purple AI rather than replacing them: the console stays best for deep hunting, policy authoring, and the response workflow inside one scope, while this skill brings the whole console's cross-site rollups - one triage worklist, one fleet-health and posture view, one blast-radius trace - to whichever AI agent you already use, answering the side-by-side, all-clients-at-once questions no single console screen composes.

## The pain this closes

- The SentinelOne management console scopes to one Account or Site at a time. Run it across a book of customer sites and every cross-client question - who has the worst open threats, whose agents went dark, who is still on an old build - means flipping the scope selector and re-reading the same screens tenant by tenant. MSPs raise this single-pane gap on r/msp repeatedly: there is no one view that ranks every client's threats or fleet health side by side.
- Protection silently erodes. An endpoint drops to detect-only, an agent stops checking in, a version rollout stalls mid-wave, or an auto-mitigated threat gets re-opened - and unless someone scopes into that exact site and reads that exact filter, it goes unnoticed until it matters. The console exposes these as separate per-site filters, never as one fleet-wide 'what changed since yesterday' or 'who is under-protected right now' answer.
- QBR and incident prep is manual assembly. Building a per-client posture scorecard (agent health, coverage, open-threat count, MTTR, version compliance) or tracing one threat's blast radius across endpoints means exporting and stitching data by hand, because no single console call returns a tenant-level composite or an endpoint-joined containment view.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/sentinelone/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/sentinelone/install.ps1 | iex
```

After install, authenticate once with your SentinelOne credentials, then verify with `sentinelone-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | sentinelone-cli threats triage; sentinelone-cli threats blast-radius "<hash>"; sentinelone-cli fleet-health stale; sentinelone-cli coverage gaps; sentinelone-cli posture; sentinelone-cli sites risk; sentinelone-cli versions rollout; sentinelone-cli whatchanged --since 24h; sentinelone-cli exclusions audit; sentinelone-cli agents reclaim; sentinelone-cli versions plan; sentinelone-cli search | Allow |
| Write (routine) | sentinelone-cli agents initiate-scan (start a disk scan); sentinelone-cli threats mitigate (mitigate matching threats); sentinelone-cli agents disconnect-from-network (network-isolate an endpoint); sentinelone-cli agents update-software; sentinelone-cli exclusions create - each refuses and sends nothing unless you pass --yes (send it) or --dry-run (preview it) | Preview with --dry-run, then a reviewed write with --yes |
| Destructive / config | sentinelone-cli agents uninstall; sentinelone-cli agents decommission; sentinelone-cli exclusions delete; sentinelone-cli sites delete; sentinelone-cli config-override delete; sentinelone-cli users delete | Human-in-the-loop only |

The skill drives the sentinelone-cli and sentinelone-mcp binaries, authenticating with a SENTINELONE_API_TOKEN read from the environment, never logged and never sent anywhere except the SentinelOne API. The read commands (threats triage, blast-radius, recurrence, mttr, verdicts; fleet-health, coverage gaps, posture, sites risk, versions rollout, ranger exposure, exclusions audit, agents reclaim, versions plan, whatchanged, search) change nothing. Every command that changes the tenant refuses and sends nothing unless it is given --yes (send it) or --dry-run (preview it), and the MCP server asks for confirm on every non-read endpoint; the recommended policy is still preview with --dry-run, show the exact command, get approval, then run it with --yes. Keep the destructive and credential tiers (agents uninstall / decommission, exclusions delete, sites delete, config-override delete, users delete, uninstall-password, agents get-passphrase, users login-by-token and API-token commands) human-only. Credential resources are never synced into the local store, secret-named fields are dropped before anything is stored or cached, and the MCP server asks for confirm before any read that returns a credential. The strongest control is the role you scope the API token to. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/sentinelone/governance.md).

## Frequently asked questions

### Is there an MCP server for SentinelOne?

Yes - this one. A free, open source MCP server and Claude Code Skill for SentinelOne, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the SentinelOne MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `SENTINELONE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `SENTINELONE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way sentinelone-mcp runs, and shares MCP configuration with Codex: add sentinelone-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote server. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my SentinelOne data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my SentinelOne API rate limits?

The local mirror exists so reads stop hitting the API. After the first sync, the cross-site views (threats triage, blast-radius, fleet-health, coverage gaps, posture, sites risk, whatchanged) run against local SQLite with zero API calls, and live calls respect a --rate-limit throttle. The history-aware analytics (whatchanged, MTTR, versions rollout, verdicts --changed) need at least two syncs to have something to diff.

### What API token do I need, and how do I scope it?

A SentinelOne API token from your management console (a Service User token is the durable choice; a personal user token works but expires). The token inherits the role of the user that mints it, so that role is the real permission boundary - mint a read-scoped token for reporting workflows and keep write or admin scope for the rare case you actually need it.

### Does it work across more than one SentinelOne console?

Each install points at one console URL plus its token, which already spans every Account, Site, and Group that token can see - the usual MSSP setup. For genuinely separate consoles, run a profile per console (see 'sentinelone-cli profile') and point each at its own credential.

### Does it replace the SentinelOne console?

No. The console stays best for hunting, policy authoring, and the interactive response workflow. This skill adds cross-site queries and scriptable actions to your AI agent so you stop scoping into each site to answer book-wide questions.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [DNSFilter](https://msp-skills.compoundingteams.com/skills/dnsfilter.md) · [Exchange Online](https://msp-skills.compoundingteams.com/skills/exchange-online.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Live-verified. Confirmed by an MSP via GitHub report against their own production tenant. The version it verified was not recorded. More MSPs run it live in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
