> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/exchange-online/

# The Exchange Online MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Microsoft Corporation.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Exchange Online. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Exchange Online to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "is mail leaving this client's tenant?" or "which mailboxes still use EWS?" and get the answer from Exchange Online's REST admin endpoints, one customer tenant per call, with no PowerShell session to open. Forwarding audits, inbox-rule sweeps, compromise triage, offboarding with a per-step receipt and a cross-tenant quarantine queue, from one local binary and MCP server.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/exchange-online/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/exchange-online)

## Instead of clicking through Exchange Online, just ask

**Instead of** Connecting Exchange Online PowerShell to one client tenant, running Get-Mailbox, Get-InboxRule for every mailbox, the outbound spam policy and the remote domains, then piecing together whether mail can auto-forward out
**just ask:** *"Is any mail auto-forwarding out of this client's tenant, and does policy block it?"*
Your agent runs: `exchange-online-cli forwarding-audit 0550b473-9fd2-4dbb-a058-a1640b4bf458 --sweep --agent`

**Instead of** Exporting Get-CASMailbox to a spreadsheet and checking each row against the org-wide EWS setting to see who breaks when Microsoft switches EWS off
**just ask:** *"Which mailboxes still have EWS, IMAP, POP or SMTP AUTH effectively turned on?"*
Your agent runs: `exchange-online-cli ews-posture 0550b473-9fd2-4dbb-a058-a1640b4bf458 --all --agent`

**Instead of** Working a leaver ticket by hand: convert to shared, hide from the address list, remove phones, grant the manager access, then hoping every step stuck
**just ask:** *"Plan the Exchange side of offboarding jane@example.com and give her manager access"*
Your agent runs: `exchange-online-cli offboard 0550b473-9fd2-4dbb-a058-a1640b4bf458 jane@example.com --manager boss@example.com --plan --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/exchange-online/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Can mail auto-forward out of this tenant, and does the outbound spam policy or a remote domain block it? | `exchange-online-cli forwarding-audit 0550b473-9fd2-4dbb-a058-a1640b4bf458 --sweep --agent` |
| Which inbox rules in this tenant forward, redirect, delete or hide mail? | `exchange-online-cli inbox-rules sweep 0550b473-9fd2-4dbb-a058-a1640b4bf458 --agent` |
| Which mailboxes still have EWS, IMAP, POP, ActiveSync or SMTP AUTH effectively on? | `exchange-online-cli ews-posture 0550b473-9fd2-4dbb-a058-a1640b4bf458 --all --agent` |
| What compromise evidence does this one mailbox show right now? | `exchange-online-cli bec triage 0550b473-9fd2-4dbb-a058-a1640b4bf458 alex@example.com --agent` |
| Which Exchange offboarding steps are done or still pending for this leaver? | `exchange-online-cli offboard 0550b473-9fd2-4dbb-a058-a1640b4bf458 jane@example.com --manager boss@example.com --plan --agent` |
| Which mailboxes can this user open, send as or send on behalf of? | `exchange-online-cli access-map 0550b473-9fd2-4dbb-a058-a1640b4bf458 jane@example.com --agent` |
| Why did this person not get the email, and what should I do next? | `exchange-online-cli where-is 0550b473-9fd2-4dbb-a058-a1640b4bf458 --recipient alex@example.com --since 48h --agent` |
| Which quarantine release requests are waiting longest across every tenant I snapshotted? | `exchange-online-cli quarantine queue --older-than 24h --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/exchange-online/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/exchange-online/guide.md).

## What makes this one different

This connector runs the Exchange cmdlets MSPs use every week through Microsoft's REST admin endpoints, so there is no PowerShell module or session to keep alive, and it adds compound commands (forwarding-audit, inbox-rules sweep, bec triage, ews-posture, offboard with a per-step receipt) that each combine several cmdlets into one answer. CIPP covers much of the same ground as a hosted multi-tenant portal; this is a local binary and MCP server your own AI agent drives, one tenant token at a time.

Microsoft's own tooling is the Exchange admin center and the ExchangeOnlineManagement PowerShell module, one tenant at a time; this complements them by giving an AI agent JSON answers and previewed writes over the same admin endpoints, without replacing either.

## The pain this closes

- Microsoft began a phased shutdown of Exchange Web Services (EWS) in Exchange Online on October 1, 2026, with full retirement on April 1, 2027 (Microsoft 365 Message Center MC1227454). Every MSP has to find which client mailboxes and apps still depend on EWS, IMAP, POP or SMTP AUTH, and the effective answer per mailbox depends on both the org default and the mailbox override.
- Business email compromise usually leaves an inbox rule or a forwarding address behind, and the rules live per mailbox (Practical365, "Good Use of Microsoft 365 Auditing Can Halt Business Email Compromise Attempts"). Checking a tenant means walking every mailbox, then the outbound spam policy and remote domains, and doing it again for the next client.
- Exchange Online administration still means a PowerShell session per customer tenant. Offboarding, permission changes, quarantine release and the missing-email ticket are repeated by hand across dozens of tenants, with no receipt that each step actually happened.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/exchange-online/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/exchange-online/install.ps1 | iex
```

After install, authenticate once with your Exchange Online credentials, then verify with `exchange-online-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `mailboxes list`, `inbox-rules sweep` (removed over MCP in read-only mode), `forwarding-audit`, `ews-posture`, `bec triage`, `access-map`, `message-trace list`, `where-is`, `quarantine list` | Allow |
| Write (routine) | `mailboxes set`, `mailbox-permissions add`, `send-as add`, `send-on-behalf set`, `folder-permissions set`, `auto-reply set`, `client-access set`, `distribution-groups add-member`, `transport-rules enable`, `quarantine release`, `blocked-senders remove` | Preview with --dry-run, then a reviewed write (over MCP, confirm: true) |
| Destructive / config | `mobile-devices wipe`, `mobile-devices remove`, `quarantine delete`, `inbox-rules remove`, `mailbox-permissions remove`, `send-as remove`, `offboard`, `transport-rules disable` | Human-in-the-loop only |

It reads mailboxes, permissions, inbox rules, mail flow, policies, message traces and quarantine in one customer tenant at a time. It can change mailboxes, permissions, client protocols, inbox and transport rules, quarantine and mobile devices, and run an offboarding sequence; over MCP every change previews until it is confirmed. Let an agent run reads freely, approve routine writes after a preview, and keep device wipes, quarantine deletes and offboarding with a human. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/exchange-online/governance.md).

## Frequently asked questions

### Is there an MCP server for Exchange Online?

Yes - this one. A free, open source MCP server and Claude Code Skill for Exchange Online, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Exchange Online MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `EXCHANGE_ONLINE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `EXCHANGE_ONLINE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Microsoft Exchange Online MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Microsoft Exchange Online data safe?

Your data stays on your machine. The CLI and MCP server run locally and talk only to Microsoft's Exchange Online endpoint; inbox-rule sweeps and quarantine snapshots are saved to a local database under your user account (secret fields stripped; cmdlet reads are not saved), and a held credential is withheld from every output and file. Credentials are never bundled or transmitted by MSP Skills.

### Do I still need Exchange Online PowerShell?

Not for the cmdlets this connector covers: it calls the same REST admin endpoints the ExchangeOnlineManagement v3 module uses, so no PowerShell session is needed. Bulk policy creation, Purview compliance and Exchange Server on-premises stay with PowerShell or other tools.

### What token does it need?

A bearer token for the resource https://outlook.office365.com, issued for the customer tenant you are working in, with an Exchange role that allows the cmdlets you run. Set it as EXCHANGE_ONLINE_TOKEN or save it with auth set-token; each call names the tenant ID.

### Can an AI agent change mailboxes on its own?

Not over MCP without your approval: every state-changing cmdlet and the offboard command return a preview until the call carries confirm: true, and EXCHANGE_ONLINE_MCP_READ_ONLY=1 removes every write tool. On the command line, preview writes with --dry-run (offboard with --plan) before running them.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](https://msp-skills.compoundingteams.com/skills/abnormal.md) · [Avanan](https://msp-skills.compoundingteams.com/skills/avanan.md) · [Bitdefender GravityZone](https://msp-skills.compoundingteams.com/skills/bitdefender.md) · [Blackpoint Cyber](https://msp-skills.compoundingteams.com/skills/blackpoint.md) · [Blumira](https://msp-skills.compoundingteams.com/skills/blumira.md) · [CIPP + Microsoft 365](https://msp-skills.compoundingteams.com/skills/cipp.md) · [Cisco Duo](https://msp-skills.compoundingteams.com/skills/cisco-duo.md) · [ConnectSecure](https://msp-skills.compoundingteams.com/skills/connectsecure.md) · [Cork](https://msp-skills.compoundingteams.com/skills/cork.md) · [CrowdStrike Falcon](https://msp-skills.compoundingteams.com/skills/crowdstrike.md) · [Fortinet](https://msp-skills.compoundingteams.com/skills/fortinet.md) · [Google Workspace](https://msp-skills.compoundingteams.com/skills/google-workspace.md) · [Huntress](https://msp-skills.compoundingteams.com/skills/huntress.md) · [KnowBe4](https://msp-skills.compoundingteams.com/skills/knowbe4.md) · [Microsoft Graph](https://msp-skills.compoundingteams.com/skills/microsoft-graph.md) · [Mimecast](https://msp-skills.compoundingteams.com/skills/mimecast.md) · [Proofpoint TAP](https://msp-skills.compoundingteams.com/skills/proofpoint.md) · [RocketCyber](https://msp-skills.compoundingteams.com/skills/rocketcyber.md) · [runZero](https://msp-skills.compoundingteams.com/skills/runzero.md) · [SaaS Alerts](https://msp-skills.compoundingteams.com/skills/saas-alerts.md) · [SentinelOne](https://msp-skills.compoundingteams.com/skills/sentinelone.md) · [SonicWall](https://msp-skills.compoundingteams.com/skills/sonicwall.md) · [Sophos Central](https://msp-skills.compoundingteams.com/skills/sophos-central.md) · [ThreatDown](https://msp-skills.compoundingteams.com/skills/threatdown.md) · [ThreatLocker](https://msp-skills.compoundingteams.com/skills/threatlocker.md) · [WatchGuard Cloud](https://msp-skills.compoundingteams.com/skills/watchguard.md)

## Status

Beta. Validated against the Exchange Online API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
