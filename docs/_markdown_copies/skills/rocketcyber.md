> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/rocketcyber/

# The RocketCyber MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Kaseya.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for RocketCyber. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects RocketCyber to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "what broke across all my RocketCyber clients overnight?" and get one ranked board - open incidents, malicious event counts, and offline agents - instead of clicking through a per-client console. The same skill ranks devices at risk, computes incident MTTR for QBRs, trends Microsoft 365 secure scores, and flags stale suppression rules that quietly hide real detections. All from the terminal.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/rocketcyber/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/rocketcyber)

## Instead of clicking through RocketCyber, just ask

**Instead of** Logging into the RocketCyber console for each client and eyeballing the incidents tab to see what fired overnight
**just ask:** *"What broke across all my RocketCyber clients in the last 24 hours?"*
Your agent runs: `rocketcyber-cli triage --since 24h --agent`

**Instead of** Exporting Defender detections to a spreadsheet and sorting by count to find the worst machines
**just ask:** *"Which devices are most at risk right now?"*
Your agent runs: `rocketcyber-cli defender riskiest --account-id 2 --top 10 --json`

**Instead of** Scrolling the suppression-rules list by hand to find old rules that might be hiding live detections
**just ask:** *"Which suppression rules are stale and could be masking alerts?"*
Your agent runs: `rocketcyber-cli suppression audit --stale-after 90d --json`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/rocketcyber/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What broke across all my clients overnight? | `rocketcyber-cli triage --since 24h` |
| Which devices went dark this week? | `rocketcyber-cli agents stale --since 7d` |
| How fast is my SOC actually resolving incidents? | `rocketcyber-cli incidents mttr --since 90d` |
| Which machines are riskiest in Defender right now? | `rocketcyber-cli defender riskiest --top 10` |
| Is this client's Microsoft 365 posture improving? | `rocketcyber-cli office trend --account-id 2` |
| Which suppression rules are stale and may hide detections? | `rocketcyber-cli suppression audit --stale-after 90d` |
| What detection events fired, by verdict? | `rocketcyber-cli events summary --account-id 2` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/rocketcyber/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/rocketcyber/guide.md).

## What makes this one different

Most RocketCyber integrations proxy each question into a single live API call - fine for one record, useless for "across all 47 clients last quarter." This skill syncs RocketCyber into a local SQLite mirror with full-text search, so cross-account rollups like triage, MTTR, and secure-score trends become one offline query. Your agent sees the computed answer, not pages of raw JSON.

The RocketCyber console shows you live dashboards one client at a time. This skill doesn't replace the SOC - it gives your AI agent a terminal-native, multi-account read of the same data plus analytics the console won't compute for you: incident MTTR, device risk ranking, secure-score trend, and suppression-rule hygiene.

## The pain this closes

- SOC alert fatigue: open incidents, detection events, and offline agents each live on a different console tab, and you switch tabs per client account to answer one question.
- QBR and SLA prep means hand-computing MTTR and screenshotting secure-score charts, because the console shows the dashboard but won't hand you the trend or the number.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/rocketcyber/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/rocketcyber/install.ps1 | iex
```

After install, authenticate once with your RocketCyber credentials, then verify with `rocketcyber-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | triage, incidents, agents (+ stale), defender (+ riskiest), events list/summary, office trend, suppression rules/audit, reports, search, sync | Allow |
| Write (routine) | import <resource> (create/upsert from a JSONL file) | Preview with --dry-run, then a reviewed write |
| Credential / config | auth set-token, auth logout | Human-in-the-loop only |

The skill reads your RocketCyber SOC data - incidents, agents, detection events, Defender and Microsoft 365 posture, and suppression rules - and computes the analytics locally. The only command that writes to the API is `import` (create/upsert from a JSONL file), and it supports `--dry-run` to preview every request before sending. `auth set-token` and `auth logout` manage your stored credential. Keep autonomous agents on read plus previewed imports, and require a human for credential changes. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/rocketcyber/governance.md).

## Frequently asked questions

### Is there an MCP server for RocketCyber?

Yes - this one. A free, open source MCP server and Claude Code Skill for RocketCyber, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the RocketCyber MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `ROCKETCYBER_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `ROCKETCYBER_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way rocketcyber-mcp runs, and shares MCP configuration with Codex: add rocketcyber-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your RocketCyber API token once.

### Is my RocketCyber data safe?

Your data stays on your machine. The CLI, MCP server, and the local SQLite mirror are all local. The AI sees query results, not raw bulk data, and your API token is never bundled or transmitted by MSP Skills - only sent to the RocketCyber API you point it at.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What RocketCyber access do I need?

A RocketCyber provider account and an API token you generate in the RocketCyber app. The skill talks to the RocketCyber Customer API v3 (US region by default; set ROCKETCYBER_BASE_URL for the EU endpoint) and reads your own SOC data - incidents, agents, detection events, Defender, Microsoft 365 posture, and suppression rules - scoped to the accounts your token can see.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the RocketCyber API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
