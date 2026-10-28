> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/threatdown/

# The ThreatDown MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Malwarebytes Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for ThreatDown. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects ThreatDown to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "which machines went dark this week?" or "which customer needs attention first?" and get one answer across every ThreatDown OneView site. OneView answers one site at a time, and its endpoint searches only take explicit account IDs. This skill resolves every site for you, ranks site health, reconciles seats, and its respond commands preview isolations, scans and reboots before any job is sent.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/threatdown/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/threatdown)

## Instead of clicking through ThreatDown, just ask

**Instead of** Re-running ThreatDown's "endpoints not seen for more than 24 hours" PowerShell example, or filtering each customer site's Endpoints page by Last Sync
**just ask:** *"Which endpoints haven't checked in for a day, across all my sites?"*
Your agent runs: `threatdown-cli fleet stale --hours 24 --agent`

**Instead of** Clicking through every customer site in OneView to see who has infected, isolated, unprotected or reboot-pending machines
**just ask:** *"Which customer needs attention first?"*
Your agent runs: `threatdown-cli fleet health --agent`

**Instead of** Looking up a machine's site and endpoint ID, then hand-building a jobs request to isolate it
**just ask:** *"Isolate LAPTOP-042, but show me what you'd do first."*
Your agent runs: `threatdown-cli respond isolate --host LAPTOP-042 --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/threatdown/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which endpoints haven't checked in for 24 hours, across every site? | `threatdown-cli fleet stale --hours 24 --agent` |
| Which customer site needs attention first? | `threatdown-cli fleet health --agent` |
| Which sites are over- or under-allocated on seats, and which subscriptions are expiring? | `threatdown-cli seats reconcile --agent` |
| Which scans or isolations never ran? | `threatdown-cli respond pending --since 7d --agent` |
| Which threats hit more than one customer this month? | `threatdown-cli threats spread --since 30d --min-sites 2 --agent` |
| Which infected or suspicious machine should I contain next? | `threatdown-cli threats queue --since 7d --agent` |
| Give me everything about one customer while they're on the phone. | `threatdown-cli customer report "Acme Corp" --agent` |
| Isolate a machine, with a preview first. | `threatdown-cli respond isolate --host LAPTOP-042 --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/threatdown/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/threatdown/guide.md).

## What makes this one different

We found no other MCP server for ThreatDown OneView on GitHub as of October 2026. The integrations that do exist, such as the [Sumo Logic](https://help.sumologic.com/docs/platform-services/automation-service/app-central/integrations/threatdown-oneview) and [Datadog (Crest Data)](https://docs.datadoghq.com/integrations/crest-data-systems-threatdown-oneview/) apps, move OneView data into a SIEM or monitoring tool, and community scripts like [nebula2nebula](https://github.com/B10sp4rt4n/nebula2nebula) target migrations. This skill fans every question out across all your sites, syncs sites, endpoints, detections and jobs into a local SQLite mirror, and previews every `respond` job (isolate, restore, scan, reboot, update) before it is sent; the raw job commands and the MCP server's generic `threatdown_execute` tool send immediately.

ThreatDown's own AI features, such as the AI-assisted Security Advisor and AI Detection & Response, work inside the ThreatDown console. This skill complements them by bringing every OneView site into the AI agent you already use, answering cross-site questions in one command without replacing OneView.

## The pain this closes

- OneView's endpoint search is a POST that needs an explicit list of account IDs, so a cross-site question like "what went dark since Friday" means listing sites, collecting each site's account ID and paging 2,000 rows at a time. ThreatDown's own answer is a PowerShell script in a support article.
- Site health lives on separate pages and filters: infected, suspicious, isolated, unprotected, reboot-pending and not-scanned machines are each a different view, so deciding which customer needs you first takes a click-through of every site.
- Seat allocations live on each site's subscription while deployed and recently seen machines live on the Endpoints page, so catching an over- or under-allocated site before billing is a spreadsheet job.
- A scan or isolation sent to an offline machine sits waiting or quietly expires, and nothing tells you which jobs never ran.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/threatdown/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/threatdown/install.ps1 | iex
```

After install, authenticate once with your ThreatDown credentials, then verify with `threatdown-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet stale, fleet health, seats reconcile, respond pending, threats spread, threats queue, customer report, fleet sync, sync, search, analytics, tail (it strips the secret fields), the respond commands without --confirm (preview only), and every list / get / search / export command except the credential reads under Credential and admin and endpoints diag-log | Allow |
| Endpoint actions | respond isolate / restore / scan / reboot / update with --confirm, endpoint-jobs issue, endpoint-jobs bulk-issue, endpoint-jobs quarantine-bulk, endpoint-jobs parent-reissue, endpoint-jobs parent-children-reissue, the endpoint-jobs cancels, grid jobs-bulk, endpoints sa-remediate, suspicious-activity remediate, endpoints diag-log, import endpoint_jobs, and the MCP threatdown_execute tool on a jobs endpoint | Human-in-the-loop, explicit confirmation |
| Security config | exclusions create / update, policies create / update / promote and the other policy writes, groups assign-endpoints, firewall, content-filtering, device-control, app-block and AIDR rules, schedules, webhooks and notifications create / update, syslog and email-protection settings, and every other write not in the tiers around it | Preview with --dry-run, then a human-approved write |
| Destructive | endpoints delete / delete-bulk, grid endpoints-delete, sites delete, policies delete, exclusions delete, users delete, subscriptions delete, webhooks delete, notifications delete, and every other delete command | Human-in-the-loop only, explicit confirmation |
| Credential and admin | every policies read (list, get, search, system-default, the -v2 forms, secret-get / secret-get-v2), sites secret, installation-tokens search, installers (download links may embed the site's enrollment credential), webhooks list / get, notifications list / get, and export of policies or notifications (their output carries uninstall passwords, enrollment tokens or webhook signing keys), installation-tokens create / send / revoke, policies secret-set (tamper-protection password), users create / update, sites create / update, import of users or sites, subscriptions create / update, the email-protection register, migrate and authorize commands, auth login, auth set-token | Human-in-the-loop only |

The skill is read-first: the fleet reports, seat reconciliation, threat triage, customer report, sync, search and nearly every list, get and search command are read-only and safe to let an agent run; the exceptions are the reads that return tamper-protection passwords and enrollment tokens (every policies read, sites secret, installation-tokens search, installers, webhooks and notifications list / get, and export of policies or notifications), which belong with the credential commands, and the local mirror never stores those secrets. The respond commands send isolation, restore, scan, reboot and update jobs to real machines but preview until you add --confirm; the raw job commands (endpoint-jobs issue, grid jobs-bulk), import endpoint_jobs and the MCP server's generic threatdown_execute tool send immediately, so keep them human-in-the-loop or set THREATDOWN_MCP_READ_ONLY=1. The scopes you tick on the OneView OAuth client (read, write, execute) are the outer limit on anything an agent can do. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/threatdown/governance.md).

## Frequently asked questions

### Is there an MCP server for ThreatDown?

Yes - this one. A free, open source MCP server and Claude Code Skill for ThreatDown, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the ThreatDown MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `THREATDOWN_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `THREATDOWN_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay on your machine, but not only in your environment: each token the CLI mints is cached in its credentials.toml (mode 0600), and `auth login` given `--client-id` or `--client-secret` also saves that client ID and secret there, so removing the environment variables does not revoke access - run `threatdown-cli auth logout` or delete the OAuth client in OneView, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way threatdown-mcp runs, and shares MCP configuration with Codex: add threatdown-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my ThreatDown data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Is this for ThreatDown OneView or ThreatDown Nebula?

OneView, the multi-site console ThreatDown (Malwarebytes) gives MSPs. It talks to the OneView API at api.threatdown.com/oneview and works across every site you manage. A single-company Nebula console is not a OneView account; it uses a different API, which this skill does not cover.

### What credentials do I need?

An OAuth client ID and secret from OneView: open Integrate in the left menu, click Add, name the app and tick the scopes. ThreatDown says a Global or Site Administrator has to create it, and the secret is shown only once. Read covers every report; the respond commands (isolate, restore, scan, reboot, update) also need execute, and other changes need write. Set THREATDOWN_CLIENT_ID and THREATDOWN_CLIENT_SECRET, then run threatdown-cli doctor.

### Can my AI isolate or scan client machines?

Only through commands you allow. The respond commands preview by default: they list the machines that would get the job and send nothing until you add --confirm (or the global --yes), and they refuse more than 25 endpoints unless you raise --max-targets. Other paths send immediately: endpoint-jobs issue, endpoint-jobs bulk-issue, grid jobs-bulk, import endpoint_jobs, and the MCP server's generic threatdown_execute tool. For a reporting agent, create the OAuth client with read only, or set THREATDOWN_MCP_READ_ONLY=1 so the MCP server drops its write tools and refuses every non-GET call, and endpoints diag-log, through threatdown_execute.

### Has it been tested against a live OneView tenant?

Not yet. Every command was built from ThreatDown's published OneView API specification and tested against a local mock of that API, not a real tenant. If you run it against your own OneView account, tell us what worked or broke on the GitHub issue tracker or at a Build Session; a real MSP run is what earns the live-verified badge.

### Will this replace the OneView console?

No. OneView stays where you set policies, manage subscriptions and deploy agents. This skill answers cross-site reporting and triage questions from your AI agent or terminal, and sends response jobs: the respond commands preview first, while the raw job commands send immediately.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the ThreatDown API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
