> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/abnormal/

# The Abnormal Security MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Abnormal Security Corporation.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Abnormal Security. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Abnormal Security to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Abnormal Security plus your AI answers the SOC questions the portal makes you click for: what new email threats are still unremediated right now, what numbers go in this quarter's client security report, and an employee's full account-takeover risk picture - each in one command. It syncs your tenant to a local store, ranks the threat queue, and confirms remediations actually completed.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/abnormal/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/abnormal)

## Instead of clicking through Abnormal Security, just ask

**Instead of** Logging into the Abnormal portal every morning and scrolling the Threat Log to find what is new and not yet remediated
**just ask:** *"What new email threats hit us in the last 24 hours that nobody has remediated yet?"*
Your agent runs: `abnormal-cli triage --since 24h --top 20 --agent`

**Instead of** Screenshotting dashboard tiles into a client QBR deck by hand every quarter
**just ask:** *"Pull last quarter's attacks-stopped and impersonation numbers for the client report"*
Your agent runs: `abnormal-cli report-snapshot --since 90d --csv`

**Instead of** Clicking through the account-takeover case, then the employee profile, then their logins across three portal screens
**just ask:** *"Give me the full account-takeover risk picture for jane@acme.com"*
Your agent runs: `abnormal-cli employee-risk "jane@acme.com"`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/abnormal/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| What new, unremediated email threats need attention right now? | `abnormal-cli triage --since 24h --top 20` |
| Pull a client-ready security report for the quarter | `abnormal-cli report-snapshot --since 90d --csv` |
| What is the account-takeover risk picture for this employee? | `abnormal-cli employee-risk "vip@acme.com"` |
| Is this vendor showing email-compromise signs? | `abnormal-cli vendor-risk "acme-supplies.com"` |
| Remediate a threat and block until it actually completes | `abnormal-cli remediate-watch <threat|case> <id>` |
| List the latest Abnormal cases | `abnormal-cli cases retrieve --all` |
| How many attacks did we stop this week? | `abnormal-cli aggregations attack-stopped-retrieve` |
| Find threats from a spoofed sender | `abnormal-cli threats retrieve --sender "ceo@spoofed.com"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/abnormal/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/abnormal/guide.md).

## What makes this one different

Most Abnormal Security integrations proxy each question into a live API call - fine for one record, but it falls over the moment you ask across a quarter of threats or every employee at once. This skill syncs your tenant into a local SQLite mirror with full-text search, so cross-entity questions - a threat joined to its messages, an employee joined to their open cases and 30-day logins - resolve as one offline query, and your AI sees the answer instead of raw bulk data.

Abnormal's own portal and AI detection stay the system of record and the place detection policy lives. This skill adds a terminal-and-agent surface the portal does not: a ranked triage queue, blocking remediation receipts, and one-shot client reporting your AI can drive without a human clicking through screens.

## The pain this closes

- Email threat triage means living in the portal - every shift starts by manually scrolling the Threat Log to spot what is new and still unremediated, with no ranked queue.
- Client security reporting is a copy-paste chore - screenshotting dashboard tiles into a QBR deck instead of pulling attacks-seen, attacks-stopped, and impersonation numbers once.
- Confirming a remediation actually finished means refreshing the portal and hoping - there is no terminal receipt that the action reached a terminal state.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/abnormal/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/abnormal/install.ps1 | iex
```

After install, authenticate once with your Abnormal Security credentials, then verify with `abnormal-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | triage, report-snapshot, threats/cases/vendors/employee retrieve, employee-risk, vendor-risk, aggregations, soar (API-token metadata only), search | Allow |
| Write (routine) | cases create (update case status), detection360 reports-create, api-resources resources-create-create / resources-update-partial-update / resources-actions-create | Preview with --dry-run, then a reviewed write |
| Remediation / destructive | threats create (remediate/unremediate), email-search search-remediate-create (delete/move mail), remediate-watch, import | Human-in-the-loop only |

The skill authenticates with a single ABNORMAL_API_TOKEN scoped to your tenant. Read commands - triage, reporting, threat/case/vendor/employee lookups, and API-token metadata - are always safe and can run unattended. Routine writes such as updating a case status, submitting a misclassification report, or managing API resources should be previewed with --dry-run and approved. Remediation actions that delete or move mail, remediate threats, or bulk-import records are human-in-the-loop only. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/abnormal/governance.md).

## Frequently asked questions

### Is there an MCP server for Abnormal Security?

Yes - this one. A free, open source MCP server and Claude Code Skill for Abnormal Security, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Abnormal Security MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `ABNORMAL_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `ABNORMAL_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Not with this build. ChatGPT connects only to remote MCP servers over HTTPS, which means running abnormal-mcp in HTTP mode, and in this build HTTP mode asks for no password, listens on every network interface, and can be reached by a web page you open. Do not run it or put it behind a tunnel until the connector is reprinted; the security note at the top of the install guide explains why. Until then, use Claude Desktop, Claude Code or Codex.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Abnormal Security data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Abnormal API rate limits?

It does not have to. A sync pulls your tenant into a local SQLite mirror once, then triage, search, and reporting answer from disk - so repeat questions never touch the API. For live calls you can cap throughput with --rate-limit and page large pulls.

### Do I need to be an Abnormal partner or customer?

You need API access to an Abnormal Security tenant and a token generated from the portal's integration settings. The REST API does not require a separate partner tier - just a credential scoped to what you want the skill to do.

### Can it actually remediate, or only read?

It can remediate threats and delete or move malicious messages through the remediation commands, and remediate-watch blocks until Abnormal reports the action reached a terminal state. Those actions are gated behind a human-in-the-loop policy - see the safety model below.

### Will it replace the Abnormal portal?

No. Detection tuning, policy, and configuration stay in the portal. This is a read-first, action-on-approval surface for your terminal and your agents, not a replacement UI.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the Abnormal Security API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
