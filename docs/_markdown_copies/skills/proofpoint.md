> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/proofpoint/

# The Proofpoint TAP MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Proofpoint, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Proofpoint TAP. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Proofpoint TAP to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Proofpoint TAP's dashboard answers one threat, one clicker, one campaign at a time, and every SIEM pull spends against a hard daily quota. This skill backfills clicks, messages, campaigns, Very Attacked People, and clickers into a local SQLite store, then answers the questions the console can't: who is both heavily attacked and clicking, every event that touched one user, and a full incident brief from a single threatId - offline, in seconds.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/proofpoint/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/proofpoint)

## Instead of clicking through Proofpoint TAP, just ask

**Instead of** Pulling SIEM exports hour by hour to reconstruct what got through overnight, then re-querying the same window every time you need it again
**just ask:** *"What malicious clicks and messages got through in the last 12 hours?"*
Your agent runs: `proofpoint-cli backfill --since 12h --agent`

**Instead of** Cross-referencing the Very Attacked People report against the top-clickers report in two separate dashboard exports to find your real problem people
**just ask:** *"Who is both Very Attacked and a top clicker right now?"*
Your agent runs: `proofpoint-cli risk-overlap --window 30 --agent`

**Instead of** Opening three TAP screens - threat summary, forensics, and the message events - just to brief a single alert
**just ask:** *"Give me the full incident brief for this threatId"*
Your agent runs: `proofpoint-cli incident "threat-abc123" --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/proofpoint/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What malicious clicks and messages got through overnight? | `proofpoint-cli backfill --since 12h` |
| Who is both Very Attacked and a top clicker? | `proofpoint-cli risk-overlap --window 30` |
| Give me the full incident brief for a threatId | `proofpoint-cli incident "threat-abc123"` |
| What indicators should I block from this threat? | `proofpoint-cli iocs --threat-id "threat-abc123" --csv` |
| Show me every event that touched one user | `proofpoint-cli user "jane.doe@example.com"` |
| Who are my Very Attacked People this month? | `proofpoint-cli people list-vap --window 30` |
| Which permitted clicks and delivered threats still need a response? | `proofpoint-cli siem list-issues` |
| What threats are inside this campaign? | `proofpoint-cli campaign-threats "campaign-xyz789"` |
| Decode this urldefense-rewritten link to its real target | `proofpoint-cli url --urls "https://urldefense.com/v3/__https://example.com__;!!abc"` |
| Is my synced threat data fresh enough to trust an offline query? | `proofpoint-cli workflow status` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/proofpoint/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/proofpoint/guide.md).

## What makes this one different

Most Proofpoint TAP integrations and MCP servers proxy each question into a single live API call - fine for one lookup, but every call spends against TAP's hard daily quota and none of them join across endpoints. This skill backfills SIEM events, campaigns, Very Attacked People, and clickers into a local SQLite store, so repeat investigations, cross-endpoint joins like attacked-and-clicking, and per-user timelines become instant offline queries instead of a wall of rate-limited API pulls.

Proofpoint's TAP dashboard gives you per-threat and per-person screens; this skill adds the cross-endpoint rollups the portal never exposes - the people who are both Very Attacked and clicking, every local event that touched one user, and a one-shot incident brief from a single threatId - all from your own synced data and pointed at by your AI agent.

## The pain this closes

- MSPs running Proofpoint TAP hit the API's daily ceilings fast - 1,800 SIEM requests and only 50 campaign-id lookups per rolling 24 hours - so any investigation that re-queries the same window, or loops over yesterday's clicks, runs out of quota before it runs out of questions.
- The TAP dashboard reports one threat, one campaign, one Very Attacked Person at a time. The questions that matter at response time - who is both attacked and clicking, every event that touched this user, what is actually inside this campaign - cross endpoints the console never joins, so they become manual export-and-pivot work.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/proofpoint/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/proofpoint/install.ps1 | iex
```

After install, authenticate once with your Proofpoint TAP credentials, then verify with `proofpoint-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | siem list-issues, siem list-clicks-permitted, people list-vap, people list-top-clickers, campaign get, campaign-threats, threat, incident, iocs, forensics, risk-overlap, user, url, search, export, sync, workflow status | Allow |
| Write (routine) | import | Preview with --dry-run, then a reviewed write |
| Credential / security | auth set-token, auth logout | Human-in-the-loop only |

The skill reads your Proofpoint TAP threat data - SIEM click and message events, campaigns, Very Attacked People, top clickers, and forensic evidence - and can sync it into a local SQLite mirror; all of that is read-only and safe to let an agent run, and the bundled MCP server exposes only those read tools. The one API write path is CLI-only bulk import, which supports --dry-run - preview it and keep it human-reviewed. The local auth commands (auth set-token, auth logout) manage your stored TAP credentials and should stay operator-only. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/proofpoint/governance.md).

## Frequently asked questions

### Is there an MCP server for Proofpoint TAP?

Yes - this one. A free, open source MCP server and Claude Code Skill for Proofpoint TAP, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Proofpoint TAP MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `PROOFPOINT_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `PROOFPOINT_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way proofpoint-mcp runs, and shares MCP configuration with Codex: add proofpoint-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Proofpoint data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local, and the bundled MCP server exposes read-only threat-intelligence tools only. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this blow through my Proofpoint TAP API limits?

No - avoiding that is the point. TAP caps you at 1,800 SIEM requests and 50 campaign-id lookups per rolling 24 hours. The skill backfills once into a local SQLite store, then answers repeat and cross-endpoint questions from that mirror, so re-querying a window or looping over users costs zero additional API calls. Live calls fire only when you ask for fresh data.

### Do I need a special Proofpoint partner API or Essentials admin access?

No. It uses your standard TAP (Targeted Attack Protection) Service Principal and Secret, created under Settings then Connected Applications in the TAP dashboard. It reads the Threat Insight endpoints your account already exposes; it does not require Proofpoint Essentials administration or a separate partner program.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [Cisco Umbrella](https://msp-skills.compoundingteams.com/skills/cisco-umbrella.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [DNSFilter](https://msp-skills.compoundingteams.com/skills/dnsfilter.md) · [Exchange Online](https://msp-skills.compoundingteams.com/skills/exchange-online.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [Keeper Admin and MSP Console](https://msp-skills.compoundingteams.com/skills/keeper.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the Proofpoint TAP API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
