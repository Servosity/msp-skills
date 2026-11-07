> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/mimecast/

# The Mimecast MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Mimecast Services Limited.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Mimecast. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Mimecast to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Mimecast plus your AI answers the questions your service desk gets every day: why a message never arrived, what is in the hold queue and which of it is safe to release, who clicked a phishing link and who else that sender reached, and whether mail is flowing. Each answer is one command that joins message tracking, the hold queue, the release and rejection logs and the click logs.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/mimecast/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/mimecast)

## Instead of clicking through Mimecast, just ask

**Instead of** A user says an invoice never arrived, so you open message tracking, then the hold queue, then the rejection log, then the held release log in the Administration Console to work out where it went
**just ask:** *"Why didn't alice@example.com get the invoice from billing@vendor.example?"*
Your agent runs: `mimecast-cli why --from billing@vendor.example --to alice@example.com --since 3d --agent`

**Instead of** Scrolling the admin hold queue one message at a time, reading each hold reason and guessing which ones are safe to release
**just ask:** *"What is in the hold queue today, grouped by reason, and what is safe to release?"*
Your agent runs: `mimecast-cli held triage --since 24h --group-by reason --agent`

**Instead of** After a phishing report, paging through URL Protect click logs by hand to find everyone else who clicked the same link
**just ask:** *"Who clicked this phishing link this week?"*
Your agent runs: `mimecast-cli ttp who-clicked https://phish.example/login --since 7d --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/mimecast/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Why didn't this message arrive: held, rejected, released or delivered, and by which policy? | `mimecast-cli why --from billing@vendor.example --to alice@example.com --since 3d --agent` |
| What is in the hold queue, grouped by hold reason, with a release-risk class per group? | `mimecast-cli held triage --since 24h --group-by reason --agent` |
| What arrived in the hold queue since I last looked? | `mimecast-cli held triage --since 24h --new --agent` |
| Release this held message for ticket T-1042, but only if its hold reason is safe - show me the decision first | `mimecast-cli held release-safe eNo1jr0OgjAYRd-lExample --ticket T-1042 --plan` |
| Who clicked this phishing link, and did anyone click through a warning? | `mimecast-cli ttp who-clicked https://phish.example/login --since 7d --agent` |
| Which users did this phishing sender reach, which clicked, and which still have it in the hold queue? | `mimecast-cli threats sender @phish.example --since 3d --agent` |
| Every threat, rejection and hold for alice@example.com this week, newest first | `mimecast-cli threats mailbox alice@example.com --since 7d --agent` |
| Who released risky held messages this week, and which releases have no ticket? | `mimecast-cli held release-audit --since 7d --agent` |
| Is mail flowing: queue backlog, inbound gaps and stuck deliveries in one OK/WARN/FAIL verdict? | `mimecast-cli mailflow --max-queue 200 --stale-after 30m --agent` |
| Which senders keep getting held and then released, so a narrow permit would stop the tickets? | `mimecast-cli senders suggest --since 30d --min-holds 3 --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/mimecast/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/mimecast/guide.md).

## What makes this one different

This connector answers whole service-desk questions in one command: why joins message tracking, the hold queue, the held release log and the rejection log into one verdict, held triage groups the hold queue with a release-risk class, and threats sender scopes a phishing campaign across the click, attachment and impersonation logs, rejections and holds. Most Mimecast integrations, such as the Cortex XSOAR Mimecast pack (https://github.com/demisto/content/tree/master/Packs/Mimecast), expose the API as one action per endpoint for a playbook to chain; this connector also ships every covered endpoint as a typed command.

The Mimecast Administration Console shows message tracking, the hold queue, the release log and the click logs on separate pages for a person to read; this connector gives your AI agent the joined answer as data, plus a hold-reason check before any release. It complements the console; it does not replace it.

## The pain this closes

- "Release my email" tickets are urgent and risky at the same time. Thread's public Mimecast runbook for MSP service desks tells techs to verify the requester, treat urgency as a caution flag, never release a malware verdict or a sandbox or impersonation hold on the requester's word alone, and, when in doubt, not release.
- Answering "why was this blocked" means reading several Administration Console pages for one message: message tracking, the hold queue, the held release log and the rejection log, each with its own search.
- A phishing report is never about one user. The same runbook says to treat an allowed click as an incident and to pull the click logs for sibling deliveries of the same URL, and the URL Protect log has no URL filter, so finding every clicker means paging the whole log.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/mimecast/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/mimecast/install.ps1 | iex
```

After install, authenticate once with your Mimecast credentials, then verify with `mimecast-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | why, held triage, held list, held release-logs, held release-audit, senders suggest, mailflow, ttp who-clicked, threats mailbox, threats sender, messages search, rejections, audit events, archive-logs searches | Allow |
| Write (routine) | held release-safe, held release, held reject, senders permit-or-block, ci-policies create, ci-policies update, ci-policies update-default, files, import | Preview first (held release-safe --plan; --dry-run elsewhere; MCP confirm), then a reviewed write |
| Mail flow and sending | email, gateway set-protection-mode, gateway set-outbound-ips, gateway update-details | Human-in-the-loop only |
| Credential / identity | any command that can emit a stored secret (journaling get returns journaling passwords, so its whole answer is withheld from output when it carries one; messages get-file returns a pre-signed download link), journaling create, journaling update, auth login, auth set-token, auth logout | Human-in-the-loop only |
| Destructive | held purge-sender, remediation remediate, remediation bulk, ci-policies delete, gateway delete-outbound-ips, journaling delete, senders bulk-delete, senders bulk-delete-for-recipient, senders bulk-delete-internal, local-data purge | Human-in-the-loop only, explicit confirmation |
| Admin | ci-onboarding (onboards a Cloud Integrated customer account) | Operator-only, not for agents |

The connector reads Mimecast message tracking, the hold queue, the TTP, rejection, audit and DLP logs freely, and changes Mimecast only through write commands: releasing and rejecting held mail and permitting or blocking senders day to day; policy, gateway and journaling changes; and deletions and security-event remediation. Let an agent run reads, held release-safe --plan and --dry-run previews on its own, approve every real release and every MCP confirm, and keep deletions, remediation, mail-routing changes, sending email and anything that touches credentials behind a human. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/mimecast/governance.md).

## Frequently asked questions

### Is there an MCP server for Mimecast?

Yes - this one. A free, open source MCP server and Claude Code Skill for Mimecast, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Mimecast MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `MIMECAST_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `MIMECAST_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Mimecast MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Mimecast data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Which Mimecast deployments and API version does it work with?

Mimecast API 2.0, with a client ID and secret from the Mimecast API 2.0 tile in the Administration Console (Administration > Services > API and Platform Integrations). API 1.0 access keys do not work. The hold queue, why, mailflow, message tracking and TTP log commands are for MX-based (Cloud Gateway) deployments; the remediation and ci-policies commands are for Cloud Integrated tenants.

### Can an agent release a phishing email by accident?

held release-safe refuses impersonation, malware, sandbox and policy holds unless --override gives a reason, and --plan shows the decision without releasing anything; held purge-sender only plans until you add --yes. Over MCP, every tool that changes Mimecast returns a preview until it is called again with confirm: true, and MIMECAST_MCP_READ_ONLY=1 makes the MCP server refuse every write. The raw held release and held reject commands send when you run them, so keep agents on held release-safe --plan and --dry-run previews.

### Does it work across my clients' Mimecast accounts?

With Mimecast delegate access, set MIMECAST_ACCOUNT_CODE (or pass --account-code on the workflow commands) to act on one customer account. It applies to the classic endpoints: hold queue, message tracking, logs and managed-sender permit or block. The REST endpoints (gateway, journaling, ci-policies, remediation, senders bulk-delete) have no account selector, so while an account code is set the CLI refuses their writes and warns on their reads. Delegation has not yet been verified against a live delegate tenant.

### Will it hit my Mimecast API rate limits?

Mimecast sets quotas per function and per API application. Read commands back off and retry on HTTP 429; writes (release, reject, permit or block) exit with code 7 instead of replaying, so rerun them after the wait. The workflow commands page the logs only up to --max-scan-pages pages (100 entries each) and say when a result was truncated.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [Cisco Umbrella](https://msp-skills.compoundingteams.com/skills/cisco-umbrella.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [DNSFilter](https://msp-skills.compoundingteams.com/skills/dnsfilter.md) · [ESET PROTECT](https://msp-skills.compoundingteams.com/skills/eset-protect.md) · [Exchange Online](https://msp-skills.compoundingteams.com/skills/exchange-online.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [Keeper Admin and MSP Console](https://msp-skills.compoundingteams.com/skills/keeper.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the Mimecast API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
