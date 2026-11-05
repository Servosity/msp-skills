> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/connectsecure/

# The ConnectSecure MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by ConnectSecure.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for ConnectSecure. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects ConnectSecure to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI which open vulnerabilities across every client are most likely to be exploited, and get one ranked list naming the client and the machine behind every finding. It also tells you which clients a new CVE hits and what fixes exist, whose scan data you cannot trust, which suppressions are quietly hiding risk, and what goes in a client's QBR risk slide.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/connectsecure/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/connectsecure)

## Instead of clicking through ConnectSecure, just ask

**Instead of** Export each client's vulnerability report from the ConnectSecure portal and merge them in a spreadsheet to find the worst exposures across the book
**just ask:** *"Which critical and high vulnerabilities across all our clients are most likely to be exploited?"*
Your agent runs: `connectsecure-cli exposure --min-epss 0.5 --severity Critical,High`

**Instead of** Open a CVE in the portal, then click through every affected asset to work out which client it belongs to and whether a fix exists
**just ask:** *"A new CVE just dropped. Which of our clients does it hit, and is there a fix?"*
Your agent runs: `connectsecure-cli cve-impact "CVE-2024-3400"`

**Instead of** Build each QBR risk slide by hand from the company dashboard, the remediation report and last quarter's export
**just ask:** *"Pull together Acme Dental's QBR risk slide, including what we fixed in the last 90 days."*
Your agent runs: `connectsecure-cli qbr "Acme Dental" --since 90d`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/connectsecure/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which open vulnerabilities across every client are most likely to be exploited? | `connectsecure-cli exposure --min-epss 0.5 --severity Critical,High` |
| What on the CISA Known Exploited list showed up this week? | `connectsecure-cli exposure --kev --new-since 7d` |
| Which clients carry the most risk, side by side? | `connectsecure-cli portfolio --sort max-epss` |
| A CVE just made the news. Which clients and assets does it hit, and is there a fix? | `connectsecure-cli cve-impact "CVE-2024-3400"` |
| Which assets have scan data we cannot trust? | `connectsecure-cli blind-spots --stale 14d --agent-stale 48h` |
| Which suppressions expired, never expire, or were approved by the person who asked for them? | `connectsecure-cli suppression-audit --within 14d` |
| Which clients have risky Active Directory accounts or weak password policies? | `connectsecure-cli ad-hygiene --stale-logon 90d --min-length 12` |
| What goes in this client's QBR risk slide? | `connectsecure-cli qbr "Acme Dental" --since 90d` |
| Is my auth and connectivity actually working? | `connectsecure-cli doctor` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/connectsecure/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/connectsecure/guide.md).

## What makes this one different

A stateless wrapper over the ConnectSecure API answers one endpoint at a time. The questions an MSP asks are cross-client: rank every open finding at asset level, flag untrustworthy scans, audit every suppression. This skill syncs a curated set of ConnectSecure reports into a local SQLite mirror and answers those as one local join, and it never writes credential records, integration secrets, scheduled-job passwords, attack-surface creds and hashes or the agent uninstall secret to that mirror.

The ConnectSecure portal is where you scan, remediate and produce branded client reports, and this skill does not replace it. It puts the cross-client questions - exposure ranking, CVE blast radius, blind spots, suppression hygiene, AD hygiene - one sentence away in the AI you already use, and returns the rows behind each answer.

## The pain this closes

- Vulnerability scans rarely come back clean, so an MSP is staring at thousands of findings across dozens of clients (ChannelPro, "Too many vulnerabilities, not enough time"). ConnectSecure's per-client vulnerability report lists affected assets only by ID, so ranking findings across every client by machine means matching those IDs to the asset list yourself.
- "Which of our clients does this CVE hit?" takes two reports - the CVE report for fixes and the assets-by-problem report for affected machines - and a manual grouping of assets by client, on the morning an advisory lands.
- A clean result on an asset that has not been discovered in weeks, was scanned without credentials, or whose agent went silent is not clean, it is unknown. So is a vulnerability suppression that expired but was never lifted, or was approved by the same person who asked for it.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectsecure/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectsecure/install.ps1 | iex
```

After install, authenticate once with your ConnectSecure credentials, then verify with `connectsecure-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | exposure, portfolio, blind-spots, suppression-audit, ad-hygiene, qbr, cve-impact, sync, search, every list / get except the credential reads | Allow |
| Write (routine) | tags create, tag-rules update, companies update, custom-profile create, report-builder create-job, report-builder update-cover-page / update-standard-report-settings / upload-template, every other create / update not named below | Preview with --dry-run, then a reviewed write |
| Credential / security | credentials list / get / create / update, integration-credentials list / get / create / update, integration-rules list / get / create / update, attack-surface-results list / get, agents uninstall-secret, scheduler list / get / create / update, custom-domains list / get / create, agent-credentials-mapping create / update, company-mappings create / update, export credentials / integration_credentials / integration_rules / scheduler / attack_surface_results / custom_domains, tail on any of these, session, auth set-token | Human-in-the-loop only |
| Endpoint-affecting / posture | patching patch-now (alias patching create), scheduler create / update / update-schedule, companies scan, companies external-scan, agents reset (alias agents create), suppress-vulnerability create / update, discovery-settings / application-baseline-rules / pii-scan-settings / agent-discoverysettings-mapping / attack-surface-domain create / update, assets create / update (scan credentials, discovery settings, deprecation) | Human-in-the-loop only, explicit confirmation |
| Destructive | every delete (companies, assets, credentials and 19 more), assets bulk-deprecate, scheduler remove-schedule, import | Human-in-the-loop only, explicit confirmation |

Read commands - the seven analysis commands, sync, search, and every list or get except credential reads - change nothing and are safe for an agent. Credential reads, patching and patch schedules, scans, agent moves, suppressions and deletes are human-in-the-loop only. Set CONNECTSECURE_MCP_READ_ONLY=1 to make the MCP server refuse every write to ConnectSecure; it still allows credential reads, so scope the API user too. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/connectsecure/governance.md).

## Frequently asked questions

### Is there an MCP server for ConnectSecure?

Yes - this one. A free, open source MCP server and Claude Code Skill for ConnectSecure, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the ConnectSecure MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CONNECTSECURE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CONNECTSECURE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way connectsecure-mcp runs, and shares MCP configuration with Codex: add connectsecure-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your pod URL, tenant name, client ID and client secret once.

### Is my ConnectSecure data safe?

Your data stays on your machine. The CLI, MCP server, the local mirror and a cache of API responses are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills. Credential records, integration secrets, scheduled-job passwords, attack-surface creds and hashes, custom-domain certificate details and the agent uninstall secret are never written to the local mirror or the response cache, even if you ask sync for them.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Where do I find my pod URL and API credentials?

In the ConnectSecure portal: Settings > Users > (user) > API Key gives you the client ID and client secret, and Profile > API Documentation shows your pod URL (https://pod<N>.myconnectsecure.com). The tenant name is the one you type on the portal login screen. The CLI exchanges those for a short-lived token on first use.

### Will this hit my ConnectSecure API rate limits?

Lightly. `sync` reads a curated set of reports once and stores them locally, and exposure, portfolio, blind-spots, suppression-audit and ad-hygiene then answer from that mirror without calling the API. qbr and cve-impact call the live API, with GET responses cached on disk for 5 minutes (--no-cache forces a fresh call); cve-impact reads at most --max-pages pages of 100 affected assets. By default the CLI paces itself to the server's rate-limit headers.

### Will this replace the ConnectSecure portal?

No. Scanning, remediation, agent deployment and branded client reports stay in the portal. This skill answers the cross-client questions the portal makes you click through, from your AI.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [Cisco Umbrella](https://msp-skills.compoundingteams.com/skills/cisco-umbrella.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [DNSFilter](https://msp-skills.compoundingteams.com/skills/dnsfilter.md) · [Exchange Online](https://msp-skills.compoundingteams.com/skills/exchange-online.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the ConnectSecure API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
