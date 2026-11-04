> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/knowbe4/

# The KnowBe4 MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by KnowBe4, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for KnowBe4. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects KnowBe4 to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

KnowBe4's console reports one tenant, one phishing test, one chart at a time. This skill syncs your KMSAT data into a local SQLite mirror and answers the questions the portal can't: which users clicked the bait in multiple phishing tests, whose risk score is deteriorating this quarter, and who clicked a phish but finished zero training - across every client, in seconds, from your terminal.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/knowbe4/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/knowbe4)

## Instead of clicking through KnowBe4, just ask

**Instead of** Exporting a phishing-test CSV from every client tenant and pivot-tabling to find who failed more than once
**just ask:** *"Which users clicked the bait in two or more phishing tests in the last 90 days?"*
Your agent runs: `knowbe4-cli repeat-clickers --min-clicks 2 --since 90d --top 25`

**Instead of** Clicking through each user's risk-score chart in the console to guess who is getting worse
**just ask:** *"Rank the users whose risk score worsened the most this quarter"*
Your agent runs: `knowbe4-cli risk-drift --window 90d --worsened --top 20`

**Instead of** Cross-referencing the phishing report against the training report by hand to find people who failed and never trained
**just ask:** *"Who clicked a phish but has no passed training to show for it?"*
Your agent runs: `knowbe4-cli untrained-clickers --since 180d`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/knowbe4/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Who clicked the bait in more than one phishing test? | `knowbe4-cli repeat-clickers --min-clicks 2 --since 90d` |
| Whose risk score is getting worse this quarter? | `knowbe4-cli risk-drift --window 90d --worsened --top 20` |
| Who clicked a phish but never passed training? | `knowbe4-cli untrained-clickers --since 180d` |
| Which active users have zero training or zero phishing coverage? | `knowbe4-cli coverage-gaps` |
| Is training actually working for the Finance group? | `knowbe4-cli phish-prone-trend --group "Finance" --since 12mo` |
| Who are my highest-risk users, with the why behind the score? | `knowbe4-cli risk-leaderboard --top 25` |
| Which departments are driving our risk up? | `knowbe4-cli group-risk-contribution --window 90d --top 10` |
| Assemble the full client quarterly review in one command | `knowbe4-cli qbr --since 90d` |
| Who never reports a simulated phish? | `knowbe4-cli report-rate --bottom 25` |
| Is my synced data fresh enough to trust a clicker hunt? | `knowbe4-cli freshness` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/knowbe4/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/knowbe4/guide.md).

## What makes this one different

Most KnowBe4 integrations and MCP servers proxy each question into a single live API call - fine for one user, one test. But repeat-clicker hunts, risk drift, and untrained-clicker anti-joins need data fused across many phishing tests and against the training records the console keeps in a separate report. This skill syncs KnowBe4 into a local SQLite mirror, so those cross-test, cross-report questions become one instant offline join instead of a wall of API pulls.

KnowBe4's console and Virtual Risk Officer give you per-tenant dashboards; this skill adds the cross-client, cross-test rollups and anti-joins the portal never exposes - repeat clickers across every phishing test, risk drift ranked across all users, and clicked-but-untrained remediation lists - all from your own synced data and pointed at by your AI agent.

## The pain this closes

- MSPs running KnowBe4 across dozens of client tenants live in CSV exports: the console answers one account and one phishing test at a time, so 'who are my repeat clickers across all clients' becomes an afternoon of spreadsheets.
- At QBR time a vCISO needs the short list - who is deteriorating, who never reports, who failed and never trained - but the portal shows risk numbers without the why, and nothing correlates phishing results against training completion.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/knowbe4/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/knowbe4/install.ps1 | iex
```

After install, authenticate once with your KnowBe4 credentials, then verify with `knowbe4-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | account info, users list, groups list, phishing-tests list, training-enrollments list, risk-leaderboard, repeat-clickers, sync, search, qbr | Allow |
| Write (routine) | events create, import | Preview with --dry-run, then a reviewed write |
| Destructive / config | events delete | Human-in-the-loop only |

The skill reads your KnowBe4 reporting data - accounts, users, groups, phishing tests, training, risk scores - and can sync it to a local SQLite mirror; all of that is read-only and safe to let an agent run, and the bundled MCP server exposes only those read tools. The only write paths are CLI-only: pushing or deleting custom risk events on a user's timeline (a separate, opt-in User Event API key) and bulk import. Keep those human-reviewed and preview them with --dry-run first. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/knowbe4/governance.md).

## Frequently asked questions

### Is there an MCP server for KnowBe4?

Yes - this one. A free, open source MCP server and Claude Code Skill for KnowBe4, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the KnowBe4 MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `KNOWBE4_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `KNOWBE4_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way knowbe4-mcp runs, and shares MCP configuration with Codex: add knowbe4-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my KnowBe4 data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local, and the bundled MCP server exposes read-only reporting tools only. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Does this replace the KnowBe4 console or need a special partner API?

Neither. It uses your standard KMSAT Reporting API key (Account Settings - API - enable Reporting API) and your region (us, eu, ca, uk, or de). It reads what your account already exposes and adds the cross-client rollups the console doesn't. The one write path that needs extra setup is pushing custom risk events, which uses a separate, opt-in User Event API key.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the KnowBe4 API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
