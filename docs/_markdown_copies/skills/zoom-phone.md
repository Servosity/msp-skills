> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/zoom-phone/

# The Zoom Phone MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Zoom Communications, Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Zoom Phone. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Zoom Phone to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask which client users have no verified 911 address, where the main number really rings after hours, or what a departing employee still holds, and get the answer from every client Zoom Phone account at once. The CLI mirrors each account locally for the audits, reads live data for routing and missed calls, and a change made through the MCP server runs only with confirm: true; in the CLI a change is sent at once, and --dry-run is the opt-in preview.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/zoom-phone/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/zoom-phone)

## Instead of clicking through Zoom Phone, just ask

**Instead of** Clicking through every user, common-area phone and site in each client's Zoom admin portal to check emergency addresses
**just ask:** *"Which users and phones at Acme have no verified emergency address?"*
Your agent runs: `zoom-phone-cli e911 audit --agent`

**Instead of** Opening each auto receptionist and call queue to work out why after-hours calls to the main number go nowhere
**just ask:** *"Where does a call to our main number ring after hours?"*
Your agent runs: `zoom-phone-cli routing trace +14155550123 --agent`

**Instead of** Hunting through call queues, shared lines, pickup groups, desk phones and numbers to remove a departing employee
**just ask:** *"Plan Jane's Zoom Phone offboarding."*
Your agent runs: `zoom-phone-cli offboard jane@example.com --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/zoom-phone/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which users, common areas and sites have a missing or unverified emergency address? | `zoom-phone-cli e911 audit --json --select findings.kind,findings.name,findings.detail` |
| Which phone numbers are unassigned, pending, or still held by an inactive user? | `zoom-phone-cli numbers inventory --agent` |
| Which desk phones are offline, unassigned or on drifted firmware, by site? | `zoom-phone-cli devices health --agent` |
| Which external callers missed us this week and have not been called back? | `zoom-phone-cli callbacks --since 7d --agent` |
| Which IVR keys point at nothing, and which call queues have nobody to answer? | `zoom-phone-cli routing audit --agent` |
| What does this user hold before we remove them? | `zoom-phone-cli footprint jane@example.com --agent` |
| Run the E911 audit across every client account at once | `zoom-phone-cli fleet -- e911 audit` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/zoom-phone/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/zoom-phone/guide.md).

## What makes this one different

This is a Go CLI and MCP server that covers the whole Zoom Phone API (420 tools) and adds audits no single API call answers: E911 coverage, dead routing, number and desk-phone hygiene, unreturned missed calls and an offboarding plan. It keeps one saved profile per client account and runs any read across all of them with fleet. The community Python server [FujitsuPolycom/zoom-phone-mcp](https://github.com/FujitsuPolycom/zoom-phone-mcp) exposes 117 tools for one account.

Zoom ships official MCP servers for Meetings, Team Chat and other products but none for Zoom Phone; this adds Phone, works across every client account you manage, and hands structured JSON to the AI agent you already use.

## The pain this closes

- Every client is a separate Zoom account with its own admin portal, so a question as simple as 'whose 911 address is missing' means logging into each tenant and clicking user by user.
- Call routing lives in layers (auto receptionists, IVR keys, call queues, closed-hours rules), so a dead IVR key or an empty queue is found by a customer whose call went nowhere.
- Offboarding a user touches queues, shared lines, pickup and monitoring groups, desk phones, numbers and the calling plan, and the portal shows each one on a different page.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/zoom-phone/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/zoom-phone/install.ps1 | iex
```

After install, authenticate once with your Zoom Phone credentials, then verify with `zoom-phone-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | e911 audit, numbers inventory, devices health, callbacks, routing trace, routing audit, footprint, offboard (plan only), sync, search, fleet with a read command, and every list or get that returns no stored secret | Allow |
| Write (routine) | users add-call-forward-setting, call-queues members and settings changes, sites and common-areas updates, outbound caller ID and calling rules, client codes on call history | Preview with --dry-run (opt-in in the CLI; MCP writes always need confirm: true), then a reviewed write |
| Destructive / config | every delete command, offboard --apply, fleet --allow-writes | Human-in-the-loop only |
| Endpoint, messaging and E911 | devices reboot-phone, firmware-update-rules and provision-templates changes, sms post-message, whatsapp post-whats-app-message, emergency-addresses and emergency-services changes | Human-in-the-loop only |
| Credential | any command that can return a stored secret field (devices get-adevice, common-areas list-activation-codes, users phone-settings, setting-templates get, ported-numbers get-details), roles changes, auth and accounts commands | Human-in-the-loop only |

Reads (the audits, routing traces, missed-call lists, footprints, sync and every Zoom Phone GET) change nothing and are safe to allow. Changes to a client account (call forwarding, queue members, settings) should be previewed and then approved: in the CLI a write is sent as soon as it runs and --dry-run is opt-in, while the MCP server runs a write only with confirm: true; deletes, the offboard plan's --apply, phone reboots, firmware and provisioning rules, SMS and emergency-address changes stay human-in-the-loop. Any command that can return a stored secret (device SIP passwords, PINs, activation codes) and any change to roles is credential tier. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/zoom-phone/governance.md).

## Frequently asked questions

### Is there an MCP server for Zoom Phone?

Yes - this one. A free, open source MCP server and Claude Code Skill for Zoom Phone, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Zoom Phone MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `ZOOM_PHONE_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `ZOOM_PHONE_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Zoom MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Zoom data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data. Credentials are saved with the current token in a credentials file readable only by your user, and are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### What Zoom credentials does it need?

A Server-to-Server OAuth app in the Zoom App Marketplace, created in each client account, with the phone admin read scopes your workflow needs (add write scopes only if you want changes). You give the CLI its Account ID, Client ID and Client Secret; save one profile per client with accounts connect.

### Will it hit my Zoom rate limits?

Zoom Phone limits are per account and shared by every app. The E911, number and desk-phone audits read the local mirror, so after a sync they make no API calls. Live reads (routing, callbacks, footprint group membership) back off on HTTP 429 and honor Retry-After.

## More Telephony connectors

Run more than one Telephony tool, or comparing options? These connectors work the same way: [NetSapiens](https://msp-skills.compoundingteams.com/skills/netsapiens.md)

## Status

Beta. Validated against the Zoom Phone API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
