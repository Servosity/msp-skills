> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/cork/

# The Cork MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Cork Protection, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Cork. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Cork to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI why a client's Cork risk score moved and get the component that actually drove it, not just the number. It reads your whole book of business at once: which clients slipped this week, what to patch first by what is genuinely being exploited, which compliance events have blown their remediation window, and which connector is reporting healthy while its data quietly went stale.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/cork/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/cork)

## Instead of clicking through Cork, just ask

**Instead of** Export two score snapshots from Cork and subtract the four components in a spreadsheet to work out what moved before a QBR
**just ask:** *"Why did this client's Cork score drop this month?"*
Your agent runs: `cork-cli score attribute &lt;client-uuid&gt; --since 30d`

**Instead of** Pull the vulnerability inventory and re-sort it by hand because the API can only sort by vendor and product name
**just ask:** *"What should we patch first across all our clients?"*
Your agent runs: `cork-cli vulnerabilities triage`

**Instead of** Open each integration in the Cork portal and eyeball its last sync time against the clock
**just ask:** *"Which of our Cork connectors have quietly stopped syncing?"*
Your agent runs: `cork-cli integrations health`

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Why did this client's risk score move, and which component drove it? | `cork-cli score attribute <client-uuid> --since 30d` |
| Which clients across the whole book slipped the most this week? | `cork-cli score regressions --since 7d` |
| What should we patch first, ranked by what is actually being exploited? | `cork-cli vulnerabilities triage` |
| An advisory just named a CVE. Are we exposed, and where? | `cork-cli vulnerabilities exposure CVE-2023-21608` |
| Which compliance events have blown their remediation window? | `cork-cli compliance overdue` |
| Which connectors report healthy but stopped syncing? | `cork-cli integrations health` |
| Did this client get fully monitored after onboarding, or did an endpoint fall through? | `cork-cli coverage gaps --client <client-uuid>` |
| Which unwarranted clients are carrying the most risk right now? | `cork-cli warranties exposure` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/cork/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/cork/guide.md).

## What makes this one different

A stateless API mirror can answer one client at a time, right now. Every question an MSP actually asks of Cork is cross-client, cross-time, or both, and the API exposes no cross-client aggregate endpoint to build them from. This skill syncs Cork into a local SQLite mirror and answers those questions as one offline join: score attribution across a window, regression ranking over the whole book, exploitability-first triage, and a compliance join the API leaves in two separate endpoints.

The Cork portal shows one client's score, one client's compliance feed, one connector's status. This skill answers the questions that span all of them at once - who regressed, what to patch first, which connector went stale, who has no warranty - and it explains a score move by differencing its components instead of just reporting the new number.

## The pain this closes

- Cork reports a per-client risk score but never differences the four components behind it, so working out why a score moved means exporting two snapshots and subtracting by hand before every QBR.
- There is no cross-client aggregate endpoint in the Cork API at all, so 'which clients got worse this week' needs a fan-out across every client with nothing stored to compare yesterday against - which means in practice nobody asks it, and regressions surface when a client notices.
- A Cork integration can report connection_status ok while its last sync is days old. Both fields ship in the same payload and nothing compares the timestamp to now, so the dashboard stays green while every risk number behind it silently drifts.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-desktop.md) |
| **ChatGPT** (Plus/Pro+) | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/chatgpt.md) |
| **Claude Code** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/claude-code.md) |
| **Codex CLI** | [Step-by-step →](https://msp-skills.compoundingteams.com/integrations/codex.md) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](https://msp-skills.compoundingteams.com/which-agent.md) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cork/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cork/install.ps1 | iex
```

After install, authenticate once with your Cork credentials, then verify with `cork-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | score attribute, score regressions, vulnerabilities triage, vulnerabilities exposure, compliance overdue, integrations health, coverage gaps, warranties exposure, sync, search, export, and every list/get command except the secret-returning reads in the Credential row | Allow |
| Write (routine) | integrations connect, integrations update, integrations resync integration | Preview with --dry-run, then a reviewed write |
| Credential / security | integrations credentials, integrations credentials get-integration (printed verbatim, not redacted), integrations raw-data get-integration (returns a presigned URL that downloads the connector's full raw data with no further auth), the credential fields of integrations update | Human-in-the-loop only, never in a blanket allow-all-reads policy |
| Destructive / endpoint-affecting | integrations delete, software install (installs a package on a real customer device through its RMM integration) | Human-in-the-loop only, explicit confirmation |
| Admin | distributor provision-partner | Operator-only, not for agents |
| Bulk write | import <resource> --input file.jsonl - one POST per line into the write endpoints above, continuing past failures | Human-in-the-loop only, explicit confirmation. Never unattended |

The skill reads your Cork clients, devices, risk scores, compliance events, vulnerabilities, integrations, warranties, and invoices, and it can connect, update, resync, or delete integrations, install a software package on a mapped device through that device's RMM integration, and provision a partner account. Reads and sync are always safe to allow. Routine integration writes should be previewed with --dry-run, then approved. Deleting an integration, reading or replacing connector credentials, installing software on a customer endpoint, and provisioning a partner are human-in-the-loop only. The strongest control is the credential itself: a Cork API key inherits the permissions of the user who created it, so a key minted by a read-only user makes the destructive tiers unreachable. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/cork/governance.md).

## Frequently asked questions

### Is there an MCP server for Cork?

Yes - this one. A free, open source MCP server and Claude Code Skill for Cork, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Cork MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Two paths can move data off the machine, both opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CORK_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CORK_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file). Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Not with this build. ChatGPT connects only to remote MCP servers over HTTPS, and cork-mcp speaks stdio only, so that would mean putting a stdio-to-HTTP bridge in front of it, and the bridge adds no password. Do not set one up or put it behind a tunnel until the connector is reprinted; the security note at the top of the install guide explains why. Until then, use Claude Desktop, Claude Code or Codex.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your Cork API key once.

### Is my Cork data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local, and the MCP server speaks stdio only so it opens no network listener. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Cork API rate limits?

`sync` fills a local SQLite mirror, and search and score regressions answer from it without touching the API. export is NOT one of them - it paginates the live API and is one of the heaviest readers here, so pass --limit unless you mean to pull everything. The other seven analysis commands fetch live on every run, and two of them fan out (coverage gaps per connector, compliance overdue per client), so those are the ones to watch. Each caps its own scan and tells you when it truncated.

### Why does my Cork key get a 403 on some commands?

A Cork API key inherits the permissions of the user who created it. A 403 on the distributor or integration endpoints usually means the key was minted by an operator without that scope, not that the key is wrong. That property is also your best control: mint a read-only key for read-only work.

### Will this replace the Cork portal?

No, it complements it. The portal is still where you configure integrations and watch a single client. This skill answers the cross-client questions the portal makes you click through - score attribution, regression ranking, exploitability triage, stale connectors, coverage gaps - from your AI.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the Cork API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
