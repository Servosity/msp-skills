> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/warranty/

# The Device Warranty MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Dell Inc., Lenovo and Apple Inc.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Device Warranty. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Device Warranty to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask which client devices go out of warranty this quarter and get the list, per client. Device Warranty plus your AI agent looks up Dell, Lenovo and Apple serials with whichever vendor credentials you have, keeps every answer in a local warranty mirror, and answers fleet questions offline: what expires soon, what to refresh next year, and which devices were never verified.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/warranty/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/warranty)

## Instead of clicking through Device Warranty, just ask

**Instead of** Paste service tags one at a time into Dell's and Lenovo's warranty pages, then copy each end date into a spreadsheet per client
**just ask:** *"Look up warranty for every device in this export for Acme Dental"*
Your agent runs: `warranty-cli lookup --file devices.csv --customer "Acme Dental" --agent`

**Instead of** Sort that spreadsheet by end date before every QBR to find the devices about to lose coverage
**just ask:** *"Which client devices are out of warranty or expire in the next 90 days?"*
Your agent runs: `warranty-cli report --within 90d --agent`

**Instead of** Guess next year's hardware refresh from purchase dates someone typed into the PSA
**just ask:** *"How many devices per client lose coverage in each of the next four quarters?"*
Your agent runs: `warranty-cli lifecycle --quarters 4 --age 4y --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/warranty/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which client devices are out of warranty or expire in the next 90 days? | `warranty-cli report --within 90d --agent` |
| Which of Acme Dental's devices have already expired? | `warranty-cli report --customer "Acme Dental" --status expired --agent` |
| What is the warranty on these serials right now? | `warranty-cli lookup 1ABC234 PF1ABCDE --agent` |
| How many devices per client lose coverage in each of the next four quarters, and how many are past four years old? | `warranty-cli lifecycle --quarters 4 --age 4y --agent` |
| Which devices could the report not speak for, and why? | `warranty-cli unresolved --agent` |
| Which vendors are configured, and what does each still need? | `warranty-cli doctor --json` |
| Which coverage expired or changed since I last checked? | `warranty-cli events list --since 0 --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/warranty/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/warranty/guide.md).

## What makes this one different

This skill keeps every lookup in a local warranty mirror per client, so fleet questions (what expires this quarter, what to refresh next year, what was never verified) are one offline query instead of one vendor call per serial. A wrapper that forwards each question to a vendor API answers one serial at a time and knows nothing about which client owns it.

Dell TechDirect, Lenovo eSupport and Apple Business stay the source of truth for each serial; this skill puts all three behind one command, remembers the answers per client, and answers the cross-client questions none of the portals can.

## The pain this closes

- Dell's own community has asked for years for a way to check many service tags at once ("Is there an easy way to enter multiple service tags to check the warranty status on all of my hardware?", Dell Community, May 2022; one reply had over 1,000 computers to check). The accepted answer is a TechDirect API key and scripts.
- Warranty end dates live in three vendor portals with three different credentials, so a per-client view means exports, copy and paste, and a spreadsheet that is stale by the next QBR.
- Devices nobody could verify (wrong serial, vendor not configured, never checked) silently drop out of the report, so a client finds out their server is out of warranty when it fails.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/warranty/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/warranty/install.ps1 | iex
```

After install, authenticate once with your Device Warranty credentials, then verify with `warranty-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | report, lifecycle, unresolved, events list, doctor, dell entitlements, dell summary, lenovo warranty, lenovo offering, lenovo contract, apple coverage, apple get, apple list, search | Allow |
| Write (local mirror only) | import, lookup, recheck, apple-pull, sync, workflow archive | Preview with --dry-run, then a reviewed write; over MCP a save needs confirm |
| Credential | auth login, auth set-token, auth logout, auth status | Human-in-the-loop only |
| Destructive | local-data purge (dry run unless --yes) | Human-in-the-loop only, explicit confirmation |

The skill reads warranty and coverage data from Dell, Lenovo and Apple and changes nothing at any vendor. It writes only its own local warranty mirror (import, lookup, recheck, apple-pull), which you can preview with --dry-run, and over MCP those saves need confirm. Let an agent read and report freely; keep a human on credential commands and on local-data purge. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/warranty/governance.md).

## Frequently asked questions

### Is there an MCP server for Device Warranty?

Yes - this one. A free, open source MCP server and Claude Code Skill for Device Warranty, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Device Warranty MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `WARRANTY_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `WARRANTY_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Device Warranty MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my client device data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need credentials for all three vendors?

No. Every vendor is optional. Dell needs a TechDirect Warranty API client ID and secret, Lenovo a ClientID token from your Lenovo account manager, Apple an Apple Business API account in the client's organization. A vendor without credentials is reported as vendor_unconfigured and never called, and the others keep working. warranty-cli doctor --json lists what each vendor still needs.

### Where do the serials come from?

Any CSV or JSON device export with a serial column (manufacturer, model and hostname are optional), for example from your RMM. warranty-cli import devices.csv --customer "Acme Dental" loads it into the mirror; apple-pull loads a client's Apple devices straight from Apple Business.

### Will this hit the vendors' rate limits?

It is built to stay inside them: Dell lookups go 100 service tags per call, Lenovo 25 serials per call spaced 250 ms apart, Apple one serial per call. recheck refreshes only stale or soon-expiring devices, and --max-calls caps a run.

## More Documentation connectors

Run more than one Documentation tool, or comparing options? These connectors work the same way: [Hudu](https://msp-skills.compoundingteams.com/skills/hudu.md) · [IT Glue](https://msp-skills.compoundingteams.com/skills/itglue.md) · [Liongard](https://msp-skills.compoundingteams.com/skills/liongard.md) · [PandaDoc](https://msp-skills.compoundingteams.com/skills/pandadoc.md)

## Status

Beta. Validated against the Device Warranty API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
