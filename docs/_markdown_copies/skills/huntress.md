> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/huntress/

# The Huntress MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Huntress Labs, Inc.

**✓ Live-verified by @Xenith-B (MSP)** against a production tenant · 2026-06-26 · [receipt →](https://github.com/Servosity/msp-skills/issues/144). Verified on v0.1.0; the current release has changed since.

Yes - there is an MCP server for Huntress. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Huntress to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask "which Huntress incidents are oldest across all my clients?" and get one age-sorted queue spanning every organization - the cross-tenant view the per-org portal never shows. Triage incidents fleet-wide, find coverage gaps and dark agents, reconcile invoiced seats against deployed agents, and trace an indicator's blast radius across your whole account, in plain English, from one local mirror.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/huntress/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/huntress)

## Instead of clicking through Huntress, just ask

**Instead of** Log into the Huntress portal and open each client organization one at a time to eyeball which incidents are oldest.
**just ask:** *"Show me every open Huntress incident across all my clients, oldest first."*
Your agent runs: `huntress-cli fleet-incidents --sort age`

**Instead of** Export the agent list and the invoice, then reconcile seats against deployed agents in a spreadsheet.
**just ask:** *"Am I paying for more Huntress seats than I have agents deployed?"*
Your agent runs: `huntress-cli billing-reconcile`

**Instead of** Search each org's agents by hand to find the machines that stopped checking in.
**just ask:** *"Which Huntress agents haven't called home in a week?"*
Your agent runs: `huntress-cli stale-agents --days 7`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/huntress/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which incidents are oldest across every client org? | `huntress-cli fleet-incidents --sort age` |
| Where are my posture gaps - stale callbacks, disabled Defender or firewall? | `huntress-cli coverage-gaps` |
| Has this IP or file hash touched any of my clients? | `huntress-cli blast-radius --indicator 203.0.113.10` |
| Am I billed for more seats than I have agents deployed? | `huntress-cli billing-reconcile` |
| Which agents went dark in the last week? | `huntress-cli stale-agents --days 7` |
| What is my mean time-to-resolve per client? | `huntress-cli mttr --group-by org` |
| What changed across the fleet since my last shift? | `huntress-cli handoff --since 12h` |
| Give me a QBR scorecard for one client. | `huntress-cli org-scorecard --org 12345` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/huntress/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/huntress/guide.md).

## What makes this one different

Most Huntress integrations proxy each question straight to the live API - one record, one organization at a time. This skill syncs your whole account into a local SQLite mirror, so cross-tenant questions (one incident queue for every client, fleet-wide blast radius, seat-vs-agent reconciliation) resolve as a single local join: instant, offline, and the AI sees the answer rather than raw bulk data.

Huntress exposes no public AI assistant over its API, and the portal scopes to one organization at a time. This skill adds the rollups the portal never pre-computes - a unified cross-org incident queue, posture gaps, billing reconciliation, agent drift between runs, MTTR, and shift-handoff reports - without replacing the portal you already use.

## The pain this closes

- The Huntress portal scopes one organization at a time. Run dozens of client tenants and there is no single queue that tells you which incident - anywhere - is the oldest and most urgent right now.
- Invoiced seats and deployed agents drift apart silently; MSPs discover they are over- or under-billed only when someone reconciles by hand at month-end.
- During an incident you need to know whether an IP or file hash touched any other client, but the API returns raw account-wide lists (only the first 10 remediations per incident, 60 requests per minute for the whole account) - the fleet-wide correlation is left to you.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/huntress/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/huntress/install.ps1 | iex
```

After install, authenticate once with your Huntress credentials, then verify with `huntress-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet-incidents, coverage-gaps, blast-radius, billing-reconcile, mttr, org-scorecard, stale-agents, search | Allow |
| Write (routine) | organizations update-parameters, accounts memberships update-parameters, unwanted-access-rules update-parameters | Preview with --dry-run, then a reviewed write with --yes |
| Incident actions | accounts incident-reports post-v1-accounts-account-id-id-remediations-bulk-approval, incident-reports remediations bulk-rejection-parameters, incident-reports resolution post-v1-incident-reports-id | Human-in-the-loop only, explicit --yes |
| Destructive / config | organizations delete-v1-id, accounts delete-v1-id, unwanted-access-rules delete-v1-id | Human-in-the-loop only, explicit --yes |

The skill authenticates with your Huntress API key and secret and is read-first: every rollup, search, and report is non-mutating and safe to let an agent run. Every command that changes your tenant (create, update, delete, incident resolution, remediation approval or rejection, subscription changes, and import) refuses to run and sends nothing unless you pass --yes to send it or --dry-run to preview the request. An agent can still pass --yes itself, so keep autonomous agents to reads plus dry-run previews and require a human to approve any write, remediation approval, or delete. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/huntress/governance.md).

## Frequently asked questions

### Is there an MCP server for Huntress?

Yes - this one. A free, open source MCP server and Claude Code Skill for Huntress, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Huntress MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `HUNTRESS_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `HUNTRESS_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way huntress-mcp runs, and shares MCP configuration with Codex: add huntress-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote server. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Huntress data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need to be a Huntress partner?

You need a Huntress account with API credentials (a key and secret) generated from your portal. Reseller credentials unlock the cross-account reseller-rollup; a single-account credential drives everything else.

### Will this hit my Huntress API rate limits?

Sync pulls your account into a local mirror once, then every rollup and search runs against that local copy - so repeated questions cost zero API calls. The CLI also honors a configurable --rate-limit on the requests it does make.

### Will this replace my Huntress portal?

No - it complements it. The portal stays your console for configuration and deep investigation; this skill answers the cross-tenant and historical questions the portal can only show one organization at a time.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [DNSFilter](https://msp-skills.compoundingteams.com/skills/dnsfilter.md) · [Exchange Online](https://msp-skills.compoundingteams.com/skills/exchange-online.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Live-verified. Confirmed by @Xenith-B (MSP) against their own production tenant ([receipt](https://github.com/Servosity/msp-skills/issues/144)). Verified on v0.1.0; the current release has changed since. More MSPs run it live in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
