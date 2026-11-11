> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/hook-security/

# The Hook Security MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Hook Security Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Hook Security. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Hook Security to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Running Hook Security for a book of clients? Ask your AI "which clients click the most," "who clicked and still has not finished training," or "draft this client's QBR" and get one answer instead of opening each client. An MSP key covers every client, and each answer says when a client could not be counted.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/hook-security/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/hook-security)

## Instead of clicking through Hook Security, just ask

**Instead of** Opening each client in Hook before the quarterly review to compare phishing click rates by hand
**just ask:** *"Which of my clients had the highest click rate this quarter?"*
Your agent runs: `hook-security-cli client-risk --start 2026-07-01 --end 2026-09-30`

**Instead of** Checking a campaign's results against the training list to work out who clicked and never finished
**just ask:** *"Who clicked in this campaign and still has not finished their training?"*
Your agent runs: `hook-security-cli clicked-untrained --campaign cmp_88213`

**Instead of** Pulling campaigns, click rates, reports and overdue training into a slide for one client's QBR
**just ask:** *"Give me the security awareness numbers for this client's QBR."*
Your agent runs: `hook-security-cli qbr org_4521 --days 90`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/hook-security/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which clients have the highest phishing click rate? | `hook-security-cli client-risk --top 10` |
| What is my click rate across every client, from totals rather than an average of averages? | `hook-security-cli client-risk` |
| Who clicked in this campaign and has not finished training? | `hook-security-cli clicked-untrained --campaign cmp_88213` |
| Who at this client clicked since July and has not finished training? | `hook-security-cli clicked-untrained --org org_4521 --since 2026-07-01` |
| Which clients have people overdue on training? | `hook-security-cli overdue-training --counts-only` |
| What are this client's QBR numbers for the last 90 days? | `hook-security-cli qbr org_4521 --days 90` |
| Who reported this simulation? | `hook-security-cli campaigns results list-campaign cmp_88213 --outcome reported --all` |
| How many billable users did each account have in October? | `hook-security-cli billing --month 2026-10 --all` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/hook-security/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/hook-security/guide.md).

## What makes this one different

A plain API wrapper answers one endpoint per call. This connector joins them: clicked-untrained checks click results against enrollments, client-risk computes a fleet click rate from total clicks over total sent, and qbr combines four reads for one window. Each answer reports what it could not count, such as a client Hook marks too_large or a scan that stopped early, instead of treating it as zero.

Hook has its own [remote MCP connection](https://www.hooksecurity.co/blog/hook-mcp-server), and it is the shortest path if you only want to chat with your data. This connector complements it with a local command line for scripts and scheduled reports, an optional local SQLite copy for search and SQL, and fixed commands whose output has the same fields every run.

## The pain this closes

- Hook's own announcement puts it plainly: the answers are "spread across a campaign page, a report and a training list." Who clicked, who has not finished, and which client needs attention are three separate lookups.
- Comparing clients means opening each one. Hook's announcement describes the MSP question as "one question across every client, instead of opening each one."
- The security awareness section of a QBR needs click rate, training completion and report rate with their trends, for every client, every quarter. Hook's QBR template says that pulling these numbers manually means the platform is doing only half the job.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/hook-security/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/hook-security/install.ps1 | iex
```

After install, authenticate once with your Hook Security credentials, then verify with `hook-security-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | client-risk, clicked-untrained, overdue-training, qbr, organizations, campaigns, enrollments, people, groups, courses, billing, search, sql | Allow |
| Local files only | sync, export, teach and the other local learning commands, local-data purge. None of them changes anything in Hook | Allow sync and export; a person runs local-data purge --yes |
| Credential | auth set-token, auth logout | Human-in-the-loop only |

Every Hook command only reads, because Hook's API is read-only, so an agent can run them freely within what the key covers. A few commands change files on your own computer, such as sync and export, and the MCP server leaves those out when HOOK_SECURITY_MCP_READ_ONLY is 1, its preset in the one-click install. Saving or clearing the API key stays with a person. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/hook-security/governance.md).

## Frequently asked questions

### Is there an MCP server for Hook Security?

Yes - this one. A free, open source MCP server and Claude Code Skill for Hook Security, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Hook Security MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `HOOK_SECURITY_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `HOOK_SECURITY_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Hook Security MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Hook Security data safe?

The CLI, MCP server and local copy all run on your computer, and your API key goes only to Hook. What a command returns goes to the AI you use, including people's names and emails on clicked-untrained and overdue-training. Use overdue-training --counts-only to leave those out.

### How is this different from Hook's own MCP connection?

Hook's [MCP connection](https://www.hooksecurity.co/blog/hook-mcp-server) is a remote server you add to your AI assistant. This is a Claude Code Skill and MCP server that run on your computer, with a command line you can script, an optional local copy of the data, and fixed commands for the cross-client questions. Both use the same kind of Hook API key.

### What kind of Hook API key do I need?

An admin creates one in Hook under Settings > API keys, and it is shown once. An MSP key reads the MSP and every client under it, including clients added later. An organization key reads only that organization, so client-risk needs an MSP key. Hook says keys are currently included in all paid plans on its new platform.

### Can the AI change anything in Hook?

No. Hook's API is read-only, so no command or MCP tool can change campaigns, training or people in Hook.

### Does it store my users' names and email addresses?

Read responses are saved on your computer in a short-lived cache, and sync saves a local SQLite copy. Neither is encrypted, so they rely on your account permissions and disk encryption. Set HOOK_SECURITY_NO_LOCAL_DATA=1 to keep no local copy, and run local-data purge --yes to erase it. Names and emails a command returns are also sent to whichever AI you use.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [Cisco Umbrella](https://msp-skills.compoundingteams.com/skills/cisco-umbrella.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [DNSFilter](https://msp-skills.compoundingteams.com/skills/dnsfilter.md) · [ESET PROTECT](https://msp-skills.compoundingteams.com/skills/eset-protect.md) · [Exchange Online](https://msp-skills.compoundingteams.com/skills/exchange-online.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [Keeper Admin and MSP Console](https://msp-skills.compoundingteams.com/skills/keeper.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the Hook Security API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
