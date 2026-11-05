> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/blumira/

# The Blumira MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Blumira, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Blumira. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Blumira to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Running Blumira across a book of client accounts? Ask your AI "what are the worst open findings everywhere," "which detections fell out of coverage this week," or "which domain controllers went dark," and get one cross-account answer the Blumira portal can't compose. Every sub-account is mirrored into a local store, so one ranked triage queue, one MTTR rollup, and one coverage-drift report replace dozens of one-account-at-a-time portal logins.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/blumira/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/blumira)

## Instead of clicking through Blumira, just ask

**Instead of** Logging into each client's Blumira account one at a time, sorting every open-findings list by priority, and hand-merging the worst ones into a spreadsheet to decide what your analysts work first
**just ask:** *"Show me the highest-priority open findings across every client account, ranked into one queue"*
Your agent runs: `blumira-cli triage --status open --priority high`

**Instead of** Opening each account's detection-rules page in turn to spot which rules are missing or switched off against your standard ruleset, the gaps an auditor or an attacker finds first
**just ask:** *"Which detection rules fell out of coverage versus our basis ruleset, across all accounts?"*
Your agent runs: `blumira-cli coverage --against basis`

**Instead of** Scrolling each account's agent list by hand for domain controllers that stopped checking in, the blind spots that mean you aren't actually watching a client's most important server
**just ask:** *"Which domain controllers went stale or unprotected across every client?"*
Your agent runs: `blumira-cli exposure --flag-dc-stale`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/blumira/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What are the worst open findings across all my client accounts right now? | `blumira-cli triage --status open` |
| What changed since my last sync, new, resolved, or status-changed findings? | `blumira-cli drift` |
| What's my mean-time-to-resolve per account over the last month? | `blumira-cli velocity --by account --window 30d` |
| Which open findings are about to breach my age-based SLA? | `blumira-cli sla --breach-in 4h` |
| Which detection rules are missing or disabled versus our basis ruleset? | `blumira-cli coverage --against basis` |
| Which domain controllers are stale or unprotected across every account? | `blumira-cli exposure --flag-dc-stale` |
| Which findings were resolved and then re-fired? | `blumira-cli audit --min-reopens 1` |
| Which detections keep firing over and over across accounts? | `blumira-cli recurring --window 90d` |
| Give me one per-account rollup of open findings, age, and agent health? | `blumira-cli overview` |
| Which findings mention this IOC, hostname, or user in their evidence? | `blumira-cli evidence-search "<ioc>"` |
| Pull every account's Blumira data into a local mirror for offline questions? | `blumira-cli sync` |
| Give me a flat finding-to-owner-to-status table to reconcile against my PSA? | `blumira-cli reconcile --status open` |
| Which analyst is carrying the most open findings, and how old are they? | `blumira-cli workload` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/blumira/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/blumira/guide.md).

## What makes this one different

Blumira's public API is scoped per account: a live wrapper answering a book-wide question has to re-query account by account, burning agent context on each round trip and handing back pages of raw JSON. This skill syncs every account into a local SQLite mirror, so cross-account questions, one ranked triage queue, a coverage-drift table, an MTTR rollup, a domain-controller exposure map, become one offline query the agent reads as an answer rather than re-deriving every time.

It complements the Blumira portal rather than replacing it. The portal stays best for configuring detections, tuning automated-response playbooks, and investigating one account in depth, while this skill brings the whole book to whichever AI agent you already use and answers the cross-account questions, one triage queue, one coverage-drift report, one MTTR rollup, that no single portal screen composes.

## The pain this closes

- Blumira is sold as detection-and-response that doesn't need a full SOC, but an MSP running it across a book of clients still has to triage every account's findings, and the portal makes you do it one account at a time, switching the active organization for each one.
- There's no single screen that joins findings, detection coverage, and agent health across every client. Answering "which accounts are behind on coverage" or "which domain controllers went dark this week" means opening each account's portal in turn and holding the comparison in your head.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/blumira/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/blumira/install.ps1 | iex
```

After install, authenticate once with your Blumira credentials, then verify with `blumira-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | triage, overview, drift, velocity, sla, coverage, exposure, recurring, audit, search, evidence-search, sync | Allow |
| Write (routine) | msp resolve-finding, msp set-finding-owners, msp add-account-finding-comment, org controller-direct-resolve-finding, org controller-direct-set-owners, org controller-direct-add-comment | Preview with --dry-run, then a reviewed write |
| Credential / config | Credentials live in auth login / auth set-token; the API exposes no delete or bulk-config command | Human-in-the-loop only |

The skill reads findings, detections, agents, and evidence through your Blumira API credential and mirrors them into a local store. It can add comments, resolve findings, and assign owners when you ask, but those writes are opt-in and best previewed with --dry-run first. The safe default for an autonomous agent is read plus planned (dry-run) writes; keep a human on anything that resolves a finding or reassigns ownership. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/blumira/governance.md).

## Frequently asked questions

### Is there an MCP server for Blumira?

Yes - this one. A free, open source MCP server and Claude Code Skill for Blumira, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Blumira MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Two paths can move data off the machine, both opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `BLUMIRA_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `BLUMIRA_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file). Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way blumira-mcp runs, and shares MCP configuration with Codex: add blumira-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, this connector speaks stdio only, and the bridge it would need is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Blumira data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need a Blumira partner account for the cross-account views?

The cross-account commands (triage, overview, coverage across every client) read Blumira's MSP sub-account API, so they need partner API credentials with sub-account access. A single-org account still gets every direct-org command, findings, evidence search, and agent and detection rollups, plus offline sync. Generate credentials under Settings > Organization > Generate API Credentials, then run `blumira-cli auth login`.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [Cisco Umbrella](https://msp-skills.compoundingteams.com/skills/cisco-umbrella.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [DNSFilter](https://msp-skills.compoundingteams.com/skills/dnsfilter.md) · [Exchange Online](https://msp-skills.compoundingteams.com/skills/exchange-online.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [Keeper Admin and MSP Console](https://msp-skills.compoundingteams.com/skills/keeper.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the Blumira API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
