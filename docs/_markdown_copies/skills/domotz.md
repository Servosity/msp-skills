> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/domotz/

# The Domotz MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Domotz Inc..

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Domotz. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Domotz to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "which client sites have devices down right now?" and get one answer instead of clicking through the Domotz portal Collector by Collector. domotz-cli syncs every Collector into a local mirror, so cross-site rollups - fleet health, every offline device, overnight new-device sweeps, one unified asset inventory - come back as single offline queries your agent can run in seconds.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/domotz/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/domotz)

## Instead of clicking through Domotz, just ask

**Instead of** Open the Domotz portal and click into each Collector one at a time to tally which sites have devices offline
**just ask:** *"Which client sites have devices down right now?"*
Your agent runs: `domotz-cli fleet health --agent`

**Instead of** Export a device list from every site and merge the spreadsheets into one asset report for the client QBR
**just ask:** *"Give me one asset inventory across every site"*
Your agent runs: `domotz-cli fleet inventory --csv`

**Instead of** Walk each network by hand looking for devices that appeared overnight
**just ask:** *"Show me new devices that appeared anywhere in the last 24 hours"*
Your agent runs: `domotz-cli fleet new --since 24h --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/domotz/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Is anything on fire across all my sites? | `domotz-cli fleet health --agent` |
| Which Collectors (sites) are offline or degraded right now? | `domotz-cli fleet agents --agent` |
| What devices are offline across every client? | `domotz-cli fleet offline --agent` |
| What new devices appeared on any network in the last day? | `domotz-cli fleet new --since 24h --agent` |
| Give me one asset inventory across every site | `domotz-cli fleet inventory --csv` |
| Where are IP conflicts across the fleet? | `domotz-cli fleet ip-conflicts --agent` |
| Which devices can't be fully monitored (auth or SNMP gaps)? | `domotz-cli fleet unmonitored --agent` |
| How many devices of each vendor do we manage? | `domotz-cli fleet breakdown --by vendor --agent` |
| Which Collectors have gone quiet (stale sync)? | `domotz-cli fleet stale --agent` |
| Find a hostname or IP anywhere in the synced fleet | `domotz-cli search "<query>"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/domotz/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/domotz/guide.md).

## What makes this one different

Most Domotz integrations proxy each question into a live, agent-scoped API call - fine for one device, but "across all my sites" means looping every Collector and paginating each one. This skill syncs the whole fleet into a local SQLite mirror, so cross-site rollups run as a single offline query and your AI sees the answer, not raw pages of JSON.

The Domotz portal and mobile app are built for working one site at a time. This skill adds what no single portal view does: fleet-wide rollups across every Collector from the terminal, scriptable and agent-ready. It complements the portal rather than replacing it.

## The pain this closes

- Domotz is organized per-site: the portal shows one Collector at a time, so "is anything down across all my clients?" means clicking through every agent by hand.
- Building a fleet-wide asset inventory for a QBR or a client report means exporting devices from each site and stitching the spreadsheets together.
- Rogue-device and IP-conflict checks happen network by network - there's no single view that surfaces what changed across the whole fleet overnight.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/domotz/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/domotz/install.ps1 | iex
```

After install, authenticate once with your Domotz credentials, then verify with `domotz-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | Fleet rollups (health, offline, inventory, breakdown), device/agent get & list, search, status & history, counts. (Reading SNMP authentication is the exception - it returns secrets and is treated as credential-tier.) | Allow |
| Write (routine) | Create/edit monitoring objects (SNMP & TCP sensors, tags, alert-profile bindings), set inventory fields, import, control power outlets, apply device profiles | Preview with --dry-run, then a reviewed write |
| Destructive / credential | Delete agents, devices, sensors, tags; clear inventory; set or read SNMP/device credentials; RBAC user and role changes | Human-in-the-loop only |

The skill reads your Domotz fleet - Collectors, devices, variables, alerts, and topology - and can also create and delete monitoring objects, control device power outlets, and change SNMP credentials. Reads are safe to run unattended with one exception: reading SNMP authentication returns community strings and keys, so treat it like a secret. Keep an autonomous agent to reads plus previewed writes, and require a human for anything that deletes, controls hardware, or touches credentials. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/domotz/governance.md).

## Frequently asked questions

### Is there an MCP server for Domotz?

Yes - this one. A free, open source MCP server and Claude Code Skill for Domotz, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Domotz MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `DOMOTZ_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `DOMOTZ_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Not with this build. ChatGPT connects only to remote MCP servers over HTTPS, which means running domotz-mcp in HTTP mode, and in this build HTTP mode asks for no password, listens on every network interface, and can be reached by a web page you open. Do not run it or put it behind a tunnel until the connector is reprinted; the security note at the top of the install guide explains why. Until then, use Claude Desktop, Claude Code or Codex.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Domotz data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need a special Domotz plan to use the API?

No add-on is required - you generate an API Key from the Domotz Portal under Settings > Services > API Keys on your existing account. A handful of endpoints (company areas, team moves, some RBAC) are Enterprise-plan only; everything else works on standard accounts, and the CLI returns a clear error when a plan gates an endpoint.

## More Network Monitoring connectors

Run more than one Network Monitoring tool, or comparing options? These connectors work the same way: [Cisco Meraki](https://msp-skills.compoundingteams.com/skills/cisco-meraki.md) · [UniFi Network](https://msp-skills.compoundingteams.com/skills/unifi-network.md)

## Status

Beta. Validated against the Domotz API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
