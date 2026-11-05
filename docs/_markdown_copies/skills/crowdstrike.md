> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/crowdstrike/

# The CrowdStrike Falcon MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by CrowdStrike, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for CrowdStrike Falcon. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects CrowdStrike Falcon to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Running CrowdStrike Falcon across a book of client tenants? Ask your AI "what should I triage first across every CID," "which sensors went silent," or "where are the critical vulnerabilities," and get one cross-tenant answer the Falcon console can't compose. Every child CID is mirrored into one local store keyed by CID, so a single scorecard, vuln ranking, and stale-sensor sweep replace flipping Flight Control tenant by tenant.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/crowdstrike/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/crowdstrike)

## Instead of clicking through CrowdStrike Falcon, just ask

**Instead of** Logging into the Falcon console and switching CID by CID through Flight Control to work out which open detections actually matter most across all your clients
**just ask:** *"Give me one severity-sorted alert queue across every tenant"*
Your agent runs: `crowdstrike-cli fleet alerts --status new`

**Instead of** Pulling each tenant's Spotlight report in turn to find the critical, actively-exploited vulnerabilities hiding across your whole book of business
**just ask:** *"Rank the critical vulnerabilities across every CrowdStrike tenant"*
Your agent runs: `crowdstrike-cli fleet vulns --severity critical`

**Instead of** Checking each customer's host list one Flight Control scope at a time to catch the sensors that quietly stopped checking in
**just ask:** *"Which hosts haven't reported a sensor heartbeat in two weeks, across all tenants?"*
Your agent runs: `crowdstrike-cli fleet stale --days 14`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/crowdstrike/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What should I triage first across all my client tenants right now? | `crowdstrike-cli fleet alerts --status new` |
| Rank the critical vulnerabilities across every tenant? | `crowdstrike-cli fleet vulns --severity critical` |
| Which hosts haven't reported a sensor heartbeat lately? | `crowdstrike-cli fleet stale --days 14` |
| Give me one posture scorecard per tenant for the QBR deck? | `crowdstrike-cli fleet scorecard` |
| Which tenants are under-protected versus my prevention-policy baseline? | `crowdstrike-cli fleet policy-drift` |
| Which single fix clears the most hosts and tenants? | `crowdstrike-cli fleet remediate --severity critical` |
| Which tenants got worse since the last sync? | `crowdstrike-cli fleet trend` |
| Map every child CID, CID group, and role grant across my MSSP? | `crowdstrike-cli fleet tenants` |
| Search every synced host, alert, vuln, and policy across all tenants? | `crowdstrike-cli fleet search "<query>"` |
| Pull every child tenant's Falcon data into a local mirror for offline queries? | `crowdstrike-cli fleet sync --all-cids` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/crowdstrike/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/crowdstrike/guide.md).

## What makes this one different

The Falcon API is paginated and CID-scoped: a live API wrapper answering a book-wide question has to page through and re-query each child CID, burning agent context on raw JSON it then has to summarize. This skill syncs every CID into one local SQLite store keyed by CID and snapshots history on each sync, so cross-tenant questions become a single offline query the agent reads as a finished answer - and time-aware questions a stateless wrapper simply cannot answer (which tenants got worse since the last sync, week-over-week critical-alert and vuln deltas, which one remediation clears the most hosts) fall out of that stored history.

It complements the Falcon console, Flight Control, and Charlotte AI rather than replacing them: the console stays best for deep hunting, policy authoring, and the response workflow inside one CID, while this skill brings the cross-tenant rollups - one alert queue, one vuln ranking, one posture scorecard, one policy-drift and remediation view - to whichever AI agent you already use, answering the side-by-side, all-CIDs-at-once questions no single console screen composes.

## The pain this closes

- The Falcon console scopes to one CID at a time. Run CrowdStrike across a book of client tenants and every cross-client question - who has the worst open detections, whose sensors went dark, which tenant is missing a prevention policy - means switching CID through Flight Control and re-reading the same screens tenant by tenant. MSPs raise this single-pane gap repeatedly: Flight Control hands you parent-level child management, but no one view ranks every CID's alerts, vulnerabilities, or sensor health side by side.
- Posture erodes quietly between logins. A sensor stops checking in, a host falls out of a prevention policy, a tenant's critical-vuln count climbs after Patch Tuesday - and unless someone scopes into that exact CID and reads that exact filter, it goes unnoticed until an incident. Spotlight and the host list expose these as per-tenant views, never as one fleet-wide 'what got worse since last week' or 'which sensors are silent right now' answer.
- QBR and audit prep is manual assembly. Building a per-tenant posture scorecard (host count, sensor coverage, open critical alerts, critical vulns, policy posture) or working out which single remediation clears the most hosts across the fleet means exporting each CID and stitching it by hand, because no single console call returns a CID-level composite or a fleet-wide remediation rollup.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/crowdstrike/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/crowdstrike/install.ps1 | iex
```

After install, authenticate once with your CrowdStrike Falcon credentials, then verify with `crowdstrike-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | crowdstrike-cli fleet scorecard; crowdstrike-cli fleet alerts --status new; crowdstrike-cli fleet vulns --severity critical; crowdstrike-cli fleet stale --days 14; crowdstrike-cli fleet trend; crowdstrike-cli alerts post-combined-v1; crowdstrike-cli devices query-by-filter; crowdstrike-cli spotlight query-vulnerabilities; crowdstrike-cli mssp query-children; crowdstrike-cli search "<query>" | Allow |
| Write (routine) | crowdstrike-cli alerts patch-entities-v3 (update or assign alerts); crowdstrike-cli devices perform-action-v2 (contain/lift, or delete/restore a host); crowdstrike-cli devices update-tags; crowdstrike-cli policy update-prevention-policies; crowdstrike-cli policy create-prevention-policies - writes send immediately; --dry-run is an opt-in preview, not a default | Preview with --dry-run, then a reviewed write |
| Destructive / config | crowdstrike-cli devices delete-host-groups; crowdstrike-cli policy delete-prevention-policies; crowdstrike-cli policy set-prevention-policies-precedence; crowdstrike-cli mssp delete-cidgroups; crowdstrike-cli mssp delete-user-groups; crowdstrike-cli mssp deleted-roles | Human-in-the-loop only |

The skill drives the crowdstrike-cli and crowdstrike-mcp binaries, authenticating with a Falcon API client (FALCON_CLIENT_ID + FALCON_CLIENT_SECRET, plus an optional CROWDSTRIKE_OAUTH_SCOPE) read from the environment, never logged and never sent anywhere except the CrowdStrike API. The read commands (every fleet rollup, the alerts/devices/spotlight/policy/mssp query and get commands, search, doctor) change nothing. Writes are not gated by default: --dry-run is an opt-in preview flag, so the recommended policy is an agent-level rule - preview with --dry-run, show the exact command, get approval, then run the write. Keep the destructive tier (devices delete-host-groups, policy delete-prevention-policies, the mssp delete-* and deleted-roles commands) and the devices perform-action-v2 host actions (contain, delete) human-only. The strongest control is the scope you grant the Falcon API client. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/crowdstrike/governance.md).

## Frequently asked questions

### Is there an MCP server for CrowdStrike Falcon?

Yes - this one. A free, open source MCP server and Claude Code Skill for CrowdStrike Falcon, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the CrowdStrike Falcon MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CROWDSTRIKE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CROWDSTRIKE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way crowdstrike-mcp runs, and shares MCP configuration with Codex: add crowdstrike-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my CrowdStrike data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need a parent (MSSP) CID for the fleet commands?

Yes. The cross-tenant fleet commands need a parent-CID Falcon API client with Flight Control (MSSP) scope so fleet sync can discover and pull every child CID. Without it, sync degrades gracefully to the single authenticated CID and the fleet rollups simply cover that one tenant. The per-CID commands (alerts, devices, spotlight, policy) work against any single tenant's client.

### Will this hit my CrowdStrike API rate limits?

The local store exists so reads stop hitting the API. After fleet sync, every cross-tenant view (fleet alerts, vulns, stale, scorecard, policy-drift, remediate, trend, tenants, search) runs against local SQLite with zero API calls, and live calls respect a --rate-limit throttle. The trend and policy-drift analytics need at least two syncs to have history to diff.

### What scopes does the Falcon API client need?

Read scopes for the entities you query - Alerts (read), Hosts (read), Spotlight Vulnerabilities (read), Prevention Policies (read), and for the fleet commands a parent-CID client with Flight Control / MSSP read. Add write scopes (Hosts write, Prevention Policies write) only if you intend to contain hosts or edit policies. Mint a read-only client for reporting workflows and keep write scope for the rare case you actually need it - the client's scopes are the real permission boundary.

### Does it replace the Falcon console?

No. The console stays best for hunting, RTR sessions, policy authoring, and the interactive response workflow inside one CID. This skill adds cross-tenant queries and scriptable actions to your AI agent so you stop scoping into each CID to answer book-wide questions.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [DNSFilter](https://msp-skills.compoundingteams.com/skills/dnsfilter.md) · [Exchange Online](https://msp-skills.compoundingteams.com/skills/exchange-online.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the CrowdStrike Falcon API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
