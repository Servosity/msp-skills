> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/apple-business/

# The Apple Business MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Apple Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Apple Business. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Apple Business to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask which devices across every client never reached an MDM server, which lose AppleCare in the next 90 days, or which returned Macs are still Activation Locked, and get the list. Apple Business plus your AI agent reads a local store per customer org, so questions the portal answers one org and one serial at a time become one answer. It also moves serials between MDM servers, with a --dry-run preview.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/apple-business/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/apple-business)

## Instead of clicking through Apple Business, just ask

**Instead of** Sign in to each client's Apple Business Manager, filter devices by Unassigned, and check each serial's MDM server by hand
**just ask:** *"Which devices across my clients never got assigned to an MDM server?"*
Your agent runs: `apple-business-cli devices unassigned --all-orgs --agent`

**Instead of** Check AppleCare coverage one serial at a time before every renewal conversation
**just ask:** *"Which client devices lose AppleCare in the next 90 days?"*
Your agent runs: `apple-business-cli applecare expiring --within 90d --all-orgs --agent`

**Instead of** Find out a returned Mac is still Activation Locked when the technician tries to redeploy it
**just ask:** *"Which devices are still Activation Locked to a user?"*
Your agent runs: `apple-business-cli devices locked --type user --all-orgs --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/apple-business/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which devices are not assigned to any MDM server, and is a missing default server the reason? | `apple-business-cli devices unassigned --added-within 30d --agent` |
| Which devices lose AppleCare in the next 90 days, grouped by purchase order? | `apple-business-cli applecare expiring --within 90d --group-by order --agent` |
| Which devices have no active AppleCare coverage at all? | `apple-business-cli applecare expiring --uncovered --agent` |
| Which devices are Activation Locked to a user, and who is the owner when known? | `apple-business-cli devices locked --type user --agent` |
| Per client org: how many devices, how many unassigned, expiring, uncovered and user-locked, and how many stale MDM servers? | `apple-business-cli fleet summary --within 90d --agent` |
| Which MDM servers stopped checking in with Apple but still own devices? | `apple-business-cli mdm-servers health --issues-only --agent` |
| Everything about one device: org record, MDM server, AppleCare and Activation Lock | `apple-business-cli devices lookup C02XL0ABJGH5 --agent` |
| Who changed what in the organization this week? | `apple-business-cli audit-events digest --since 7d --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/apple-business/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/apple-business/guide.md).

## What makes this one different

This skill syncs each customer org's devices and MDM servers into its own local SQLite store, and devices enrich adds AppleCare and Activation Lock per device, so cross-client questions (unassigned devices, expiring coverage, user locks) are one offline query across every org. An MCP server that forwards each tool call to the Apple Business API answers one serial at a time, and AppleCare and Activation Lock are per-device endpoints.

Apple Business Manager stays your place to set default server assignments and manage accounts; Apple offers no AI integration for it. This skill answers the fleet questions across every client's organization in the AI agent you already use, and complements the portal rather than replacing it.

## The pain this closes

- A new Mac that was never assigned to the right MDM server in Apple Business Manager does not show up for zero-touch enrollment, and the technician finds out at the client's desk (Jamf Community, July 2024: the serial "does not appear in a PreStage Enrollment section").
- AppleCare coverage and Activation Lock are per-device lookups, so a fleet-wide view before a renewal or a hardware refresh means checking serials one at a time across every client org.
- Every client is a separate Apple Business organization with its own API account, so nothing in the portal rolls them up into one view.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/apple-business/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/apple-business/install.ps1 | iex
```

After install, authenticate once with your Apple Business credentials, then verify with `apple-business-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | devices list, devices lookup, devices unassigned, devices locked, applecare expiring, fleet summary, mdm-servers health, audit-events digest, sync, devices enrich | Allow |
| Write (routine) | blueprints create, blueprints update, blueprints add-apps, configurations create, configurations update, mdm-servers create, mdm-servers update, import | Preview with --dry-run, then a reviewed write |
| Device assignment | devices assign, devices unassign, device-activities create (decide which MDM server a customer device enrolls with, and can set an MDM migration deadline) | Human-in-the-loop only |
| Destructive | blueprints delete, blueprints remove-devices, configurations delete, mdm-servers delete, local-data purge | Human-in-the-loop only, explicit confirmation |
| Credential | auth login, auth set-token, auth logout | Human-in-the-loop only |

It reads your Apple Business inventory, MDM servers, AppleCare coverage, Activation Lock, users and audit log, and can create, change and delete Blueprints, Configurations and MDM servers and move devices between MDM servers. Over MCP every write previews and runs only with confirm set to true. Let an agent read freely and preview writes; keep device assignment, deletes and authentication changes with a human. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/apple-business/governance.md).

## Frequently asked questions

### Is there an MCP server for Apple Business?

Yes - this one. A free, open source MCP server and Claude Code Skill for Apple Business, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Apple Business MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `APPLE_BUSINESS_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `APPLE_BUSINESS_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Apple Business MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Apple Business data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What do I need from each client's Apple Business Manager?

An API account. An Administrator opens Settings > API in Apple Business, adds an API account and downloads its private key (.pem, offered once). The account's Client ID and Key ID plus that key are the three values the skill needs. API accounts belong to one organization, so each client gets its own.

### Will this hit Apple's API rate limits?

It paces itself when Apple Business answers 429. Sync once, then the fleet reports read the local store and make no API calls; devices enrich is the only per-device fan-out, bounded by --concurrency, and --skip-fresh lets a rerun resume.

### Does it replace my MDM (Jamf, Intune, Kandji, Mosyle)?

No. Apple Business Manager decides which MDM server a device enrolls with; this skill reads and changes that assignment and reports on the devices. Your MDM still manages them after enrollment.

### What do I need from each client's Apple Business Manager?

An API account. An Administrator opens Settings > API in Apple Business, adds an API account and downloads its private key (.pem, offered once). The account's Client ID and Key ID plus that key are the three values the skill needs. API accounts belong to one organization, so each client gets its own.

### Will this hit Apple's API rate limits?

It paces itself when Apple answers 429. Sync once, then the fleet reports read the local store and make no API calls; devices enrich is the only per-device fan-out, bounded by --concurrency, and --skip-fresh lets a rerun resume.

### Does it replace my MDM (Jamf, Intune, Kandji, Mosyle)?

No. Apple Business Manager decides which MDM server a device enrolls with; this skill reads and changes that assignment and reports on the devices. Your MDM still manages them after enrollment.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

## Status

Beta. Validated against the Apple Business API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
