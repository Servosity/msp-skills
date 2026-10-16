---
layout: default
title: "Cisco Duo MCP Server - Free, Open Source, Runs Locally | MSP Skills"
description: "Fix Duo MFA lockouts in one command and see every MSP subaccount at once, with signed Admin API calls."
permalink: /skills/cisco-duo/
skill_name: "Cisco Duo MCP"
image: /assets/social/cisco-duo/wide-1200x630.png
verification: awaiting
faqs:
  - q: "Is there an MCP server for Cisco Duo?"
    a: "Yes - this one. A free, open source MCP server and Claude Code Skill for Cisco Duo, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds."
  - q: "Is the Cisco Duo MCP server safe for client data?"
    a: "Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CISCO_DUO_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CISCO_DUO_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page."
  - q: "Does this work with ChatGPT?"
    a: "Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS: set CISCO_DUO_MCP_HTTP_TOKEN, run cisco-duo-mcp --transport http, and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. ChatGPT connectors sign in with OAuth or not at all and cannot send that bearer token, so ChatGPT also needs an OAuth-capable gateway in front of the endpoint that adds it. Step-by-step in the install guide."
  - q: "Do I need to know how to code?"
    a: "No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once."
  - q: "Is my Cisco Duo data safe?"
    a: "Your data stays on your machine. The CLI, MCP server and the local mirror are all local, and the Duo secret key is used only to sign each request; it is never sent to Duo or anywhere else. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills."
  - q: "Do I need a Duo MSP (parent) account?"
    a: "No. With an Admin API application in a single Duo account, every command except the fleet commands works against that account. As an MSP, create the Admin API application in your parent account with subaccount permissions (at least Grant accounts - Read): --account-id (or CISCO_DUO_ACCOUNT_ID) then runs any command against one customer, and fleet summary, fleet risk and fleet find-user cover every customer at once."
  - q: "Which Duo Admin API permissions does it need?"
    a: "Grant only what your workflow uses; the permission set is the strongest control you have. Read-only work (triage, audits, failures) needs Grant resource - Read, Grant read log and Grant read information. As an MSP, --account-id and the fleet commands also need Grant accounts - Read, because the CLI looks each customer up in your parent account's subaccount list, and the user-limit column of fleet summary needs Grant user limits - Read. Unlocking users, issuing bypass codes, swapping phones and re-sending enrollment emails add Grant resource - Write. Administrator commands need Grant administrators - Read or - Write, and billing set-user-limit needs Grant user limits - Write."
  - q: "Is it safe to let an AI issue bypass codes?"
    a: "Only with a human in the loop, and the commands are built for that. lockout fix, phone-swap and enrollment chase print a plan and change nothing until you add --apply. A bypass code from lockout fix is single-use, expires in an hour by default and keeps any existing codes. Verify the caller through a channel other than the ticket first (users send-verification-push sends them a Duo Push). Set CISCO_DUO_MCP_READ_ONLY=1 and the MCP server hides lockout fix, phone-swap and enrollment chase, and its API tool refuses every non-GET call."
  - q: "Will this hit my Duo API rate limits?"
    a: "Rarely. The client paces itself to Duo's rate-limit responses and backs off and retries when Duo answers 429. The fleet commands make a few calls per customer subaccount; fleet risk also pages users and bypass codes, caps that scan with --max-scan-pages, and flags any customer where the cap stopped it. Duo recommends polling authentication logs at most once a minute, so do not loop failures faster than that."
  - q: "Will this replace the Duo Admin Panel?"
    a: "No, it complements it. Policies, applications and directory sync stay in the Admin Panel. This skill answers the help-desk and MSP questions the panel makes you click through - lockout diagnosis, phone swaps, cross-customer risk and capacity - from your AI."
  - q: "What does it cost?"
    a: "Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use."
howto:
  - name: "Run the one-line installer"
    text: "macOS/Linux: bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-duo/install.sh) - Windows PowerShell: iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-duo/install.ps1 | iex"
  - name: "Authenticate"
    text: "Enter your Cisco Duo credentials once, then run cisco-duo-cli doctor to check the install."
  - name: "Ask your first question"
    text: "Ask your AI agent a Cisco Duo question in plain language; it runs cisco-duo-cli for you."
---

# The Cisco Duo MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Cisco Systems, Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Cisco Duo. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Cisco Duo to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI why a user is locked out of Duo and get the cause and the one fix that applies, then let it unlock them, issue a single-use bypass code, or move them to a new phone after you approve the plan. For an MSP it reads every customer subaccount at once: who can skip MFA, who never enrolled, and which customers are near their user limit.

<sub>New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](/what-is-an-mcp-server/)</sub>

[Install in 60s →](#install){: .btn .btn-primary} &nbsp; [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/cisco-duo){: .btn}

## Instead of clicking through Cisco Duo, just ask

**Instead of** Log in to the Duo Admin Panel, switch to the customer's account, search the user, then read their status, groups, phones, bypass codes and recent failed logins across four pages before deciding what to fix
**just ask:** *"Jane at Acme is locked out of Duo. What's wrong and what's the fix?"*
<sub>Your agent runs: <code>cisco-duo-cli lockout triage "jane@example.com" --account-id "Acme Corp"</code></sub>

**Instead of** Open every customer account one at a time before a security review, counting users in bypass, locked out, or never enrolled
**just ask:** *"Which of our customers have users who can skip MFA right now?"*
<sub>Your agent runs: <code>cisco-duo-cli fleet risk</code></sub>

**Instead of** Ask the caller which company they work for, then search one customer account after another until the username turns up
**just ask:** *"Which customer is jdoe@example.com in, and are they locked out?"*
<sub>Your agent runs: <code>cisco-duo-cli fleet find-user "jdoe@example.com"</code></sub>


## See it in 30 seconds

<video controls preload="metadata" style="width:100%; max-width:960px; border-radius:12px;" poster="/assets/social/cisco-duo/wide-1200x630.png" src="/assets/video/cisco-duo/demo-30s.mp4">Your browser does not support the video tag. <a href="/assets/video/cisco-duo/demo-30s.mp4">Watch the 30-second demo</a>.</video>

<sub>Demo data is simulated. Every command shown exists in the real CLI.</sub>

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Why can't this user get through Duo, and which fix applies? | `cisco-duo-cli lockout triage "jane@example.com" --account-id "Acme Corp"` |
| Unlock them and give them a one-hour, single-use bypass code (preview first) | `cisco-duo-cli lockout fix "jane@example.com" --account-id "Acme Corp" --bypass` |
| They got a new phone number. Move their Duo to it. | `cisco-duo-cli phone-swap "jane@example.com" --number +15551234567 --remove-old` |
| Which customer is this person in? | `cisco-duo-cli fleet find-user "jdoe@example.com"` |
| Which customers have users in bypass, locked out, or never enrolled? | `cisco-duo-cli fleet risk` |
| Which customers are near their Duo user limit or low on telephony credits? | `cisco-duo-cli fleet summary` |
| Who in this tenant can skip MFA, and who issued their bypass codes? | `cisco-duo-cli audit bypass --older-than 30d` |
| Who is still unenrolled after a week? Re-send their enrollment emails. | `cisco-duo-cli enrollment chase --older-than 7d --limit 25` |
| What failed or fraud-flagged logins happened this week, and for whom? | `cisco-duo-cli failures --since 7d` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/cisco-duo/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/cisco-duo/guide.md).

## What makes this one different

Duo's official client libraries ([duo_client_python](https://github.com/duosecurity/duo_client_python), [duo_api_golang](https://github.com/duosecurity/duo_api_golang)) sign requests and leave the workflow to you, and a thin MCP wrapper maps one tool to one endpoint. A lockout is never one endpoint, and an MSP's customers each sit behind their own API host. This skill signs every request itself, routes any command to a customer by name, and ships the composite answers: lockout triage reads the user, groups, phones, bypass codes and recent failures and returns the fix that applies; the fleet commands fan out across every subaccount in one call.

The Duo Admin Panel shows one customer at a time and spreads a lockout diagnosis across several pages. This skill answers the help-desk and MSP questions from the AI you already use, across every subaccount at once, and its MFA fixes (lockout fix, phone-swap, enrollment chase) are previewed plans you approve before anything changes. It complements the Admin Panel: policies and application setup stay there.

## The pain this closes

- A Duo lockout ticket is never one lookup. The answer is spread across the user's status, a group that can override it, their phones, any bypass codes already issued, and the last day of failed logins, and in the Admin Panel each of those is a different page.
- An MSP runs every customer as a separate Duo subaccount with its own API host, so a question as simple as 'which customer is this caller in' or 'who across our book can skip MFA' means opening each account in turn.
- Bypass codes outlive the emergency they were issued for. A code with no expiry or unlimited reuse is a standing hole in MFA, and nothing in the console lists them across a tenant with the admin who issued each one.

## Install

Works in any of these agents - pick yours:

| Agent | Quick install |
| --- | --- |
| **Claude Desktop** | [Step-by-step →](/integrations/claude-desktop/) |
| **ChatGPT** (Plus/Pro+) | [Step-by-step →](/integrations/chatgpt/) |
| **Claude Code** | [Step-by-step →](/integrations/claude-code/) |
| **Codex CLI** | [Step-by-step →](/integrations/codex/) |
| Cursor, Windsurf, Cline, Continue, Zed, Copilot, Gemini, Hermes, OpenClaw | [Which agent? →](/which-agent/) |

**Quickest path** for everyone else (terminal):

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cisco-duo/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/cisco-duo/install.ps1 | iex
```

After install, authenticate once with your Cisco Duo credentials, then verify with `cisco-duo-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | lockout triage, fleet summary, fleet risk, fleet find-user, audit bypass, audit stale, failures, sync, search, export, every list and get, accounts list, and lockout fix, phone-swap and enrollment chase without --apply (they only print a plan) | Allow |
| Write (routine) | users send-verification-push, phones send-sms-installation, groups create, tokens resync, identity-verification start, identity-verification cancel | Preview with --dry-run (or the command's own preview), then a reviewed write |
| MFA access / credential | lockout fix --apply (unlock, bypass code, reactivation), phone-swap --apply, users create-bypass-codes, phones create-activation-url, phones send-sms-activation, phones send-sms-passcodes, phones set-quarantine-status (releasing quarantine lets a device authenticate again), users add-phone, users add-token, phones create, phones update (a new number redirects SMS and phone-call factors), users bulk-create (can create users with status bypass), groups delete (deleting a Disabled group lifts its hold on every member), directory-sync sync-user (re-syncing a directory user can re-enable them, attach phones and send enrollment), import <resource> --input file.jsonl (an import can set any field a create can, status bypass included), users enroll (emails a single-use enrollment code to any address and returns the code to the caller), enrollment chase --apply (emails enrollment links to unenrolled users' addresses on file; skips users in a Disabled group), users create and users update (either can set a user to bypass), users add-group, users remove-group and groups update (a Bypass or Disabled group overrides a user's own status), registered-devices block-device and unblock-device | Human-in-the-loop only, after verifying the caller through a separate channel |
| Destructive | users delete, users bulk-trash, users remove-phone, users remove-token, phones delete, tokens delete, bypass-codes delete, webauthn delete, desktop-authenticators delete, phone-swap --delete-old | Human-in-the-loop only, explicit confirmation |
| Admin / billing | admins create, admins update, admins delete, admins reset, admins clear-inactivity, accounts create, accounts delete (removes a whole customer subaccount), billing set-edition, billing set-user-limit, billing set-telephony-credits | Operator-only, not for agents |
| Bulk write | users bulk-restore (Duo restores users as disabled) | Human-in-the-loop only, explicit confirmation. Never unattended |

The skill reads Duo users, phones, groups, bypass codes, hardware tokens, admins, authentication logs and MSP subaccounts, and it can unlock users, issue bypass codes, attach and activate phones, send enrollment emails, delete users and factors, manage administrators, and create or delete customer subaccounts. Reads, sync and every preview are safe to allow. Anything that grants an authentication factor - an unlock, a bypass code, an activation link, a new phone - is human-in-the-loop only, after the caller's identity is verified. Deleting users or factors, administrator changes, billing changes and subaccount create or delete are operator-only. The strongest control is the Admin API application's permission set: an application without Grant resource - Write cannot unlock a user, issue a bypass code or touch a phone, and one without Grant administrators - Write or Grant accounts - Write cannot change administrators or customer accounts. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/cisco-duo/governance.md).

## Frequently asked questions

### Is there an MCP server for Cisco Duo?

Yes - this one. A free, open source MCP server and Claude Code Skill for Cisco Duo, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Cisco Duo MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `CISCO_DUO_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `CISCO_DUO_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS: set CISCO_DUO_MCP_HTTP_TOKEN, run cisco-duo-mcp --transport http, and put its /mcp endpoint behind an HTTPS tunnel or your own reverse proxy. ChatGPT connectors sign in with OAuth or not at all and cannot send that bearer token, so ChatGPT also needs an OAuth-capable gateway in front of the endpoint that adds it. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Cisco Duo data safe?

Your data stays on your machine. The CLI, MCP server and the local mirror are all local, and the Duo secret key is used only to sign each request; it is never sent to Duo or anywhere else. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### Do I need a Duo MSP (parent) account?

No. With an Admin API application in a single Duo account, every command except the fleet commands works against that account. As an MSP, create the Admin API application in your parent account with subaccount permissions (at least Grant accounts - Read): --account-id (or CISCO_DUO_ACCOUNT_ID) then runs any command against one customer, and fleet summary, fleet risk and fleet find-user cover every customer at once.

### Which Duo Admin API permissions does it need?

Grant only what your workflow uses; the permission set is the strongest control you have. Read-only work (triage, audits, failures) needs Grant resource - Read, Grant read log and Grant read information. As an MSP, --account-id and the fleet commands also need Grant accounts - Read, because the CLI looks each customer up in your parent account's subaccount list, and the user-limit column of fleet summary needs Grant user limits - Read. Unlocking users, issuing bypass codes, swapping phones and re-sending enrollment emails add Grant resource - Write. Administrator commands need Grant administrators - Read or - Write, and billing set-user-limit needs Grant user limits - Write.

### Is it safe to let an AI issue bypass codes?

Only with a human in the loop, and the commands are built for that. lockout fix, phone-swap and enrollment chase print a plan and change nothing until you add --apply. A bypass code from lockout fix is single-use, expires in an hour by default and keeps any existing codes. Verify the caller through a channel other than the ticket first (users send-verification-push sends them a Duo Push). Set CISCO_DUO_MCP_READ_ONLY=1 and the MCP server hides lockout fix, phone-swap and enrollment chase, and its API tool refuses every non-GET call.

### Will this hit my Duo API rate limits?

Rarely. The client paces itself to Duo's rate-limit responses and backs off and retries when Duo answers 429. The fleet commands make a few calls per customer subaccount; fleet risk also pages users and bypass codes, caps that scan with --max-scan-pages, and flags any customer where the cap stopped it. Duo recommends polling authentication logs at most once a minute, so do not loop failures faster than that.

### Will this replace the Duo Admin Panel?

No, it complements it. Policies, applications and directory sync stay in the Admin Panel. This skill answers the help-desk and MSP questions the panel makes you click through - lockout diagnosis, phone swaps, cross-customer risk and capacity - from your AI.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.


## More Security connectors

Run more than one Security tool, or comparing options? These connectors work the same way: [Abnormal Security](/skills/abnormal/) · [Avanan](/skills/avanan/) · [Blumira](/skills/blumira/) · [CIPP + Microsoft 365](/skills/cipp/) · [ConnectSecure](/skills/connectsecure/) · [Cork](/skills/cork/) · [CrowdStrike Falcon](/skills/crowdstrike/) · [Fortinet](/skills/fortinet/) · [Google Workspace](/skills/google-workspace/) · [Huntress](/skills/huntress/) · [KnowBe4](/skills/knowbe4/) · [Microsoft Graph](/skills/microsoft-graph/) · [Proofpoint TAP](/skills/proofpoint/) · [RocketCyber](/skills/rocketcyber/) · [runZero](/skills/runzero/) · [SentinelOne](/skills/sentinelone/) · [Sophos Central](/skills/sophos-central/) · [ThreatDown](/skills/threatdown/) · [ThreatLocker](/skills/threatlocker/) · [WatchGuard Cloud](/skills/watchguard/)

## Status

Beta. Validated against the Cisco Duo API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
