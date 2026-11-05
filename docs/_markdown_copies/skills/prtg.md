> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/prtg/

# The PRTG Network Monitor MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Paessler GmbH.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for PRTG Network Monitor. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects PRTG Network Monitor to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

PRTG plus your AI answers the questions that span every client's PRTG core at once: what is down right now and since when, what changed overnight, what someone paused or acknowledged and forgot, and which cores are behind on version, license or maintenance. Register each core once; every fleet command asks all of them in one run and names a core that fails instead of counting it as healthy.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/prtg/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/prtg)

## Instead of clicking through PRTG Network Monitor, just ask

**Instead of** Logging in to each client's PRTG web interface in turn at the start of a shift and reading its alarm list, because every client runs its own core
**just ask:** *"What is down across all my clients' PRTG cores right now?"*
Your agent runs: `prtg-cli fleet alarms --status down --unacked --agent`

**Instead of** Asking the night tech what happened, then scrolling each core's sensor lists for new alarms and recoveries since the last shift
**just ask:** *"What changed across every PRTG core in the last 12 hours?"*
Your agent runs: `prtg-cli fleet handoff --since 12h --agent`

**Instead of** Pausing the same firewall in five PRTG web interfaces before patch night and hoping nobody forgets to resume them afterwards
**just ask:** *"Pause fw01 on every client core for two hours of patching - show me first"*
Your agent runs: `prtg-cli fleet pause --kind device --filter 'name = "fw01"' --duration 2h --message 'Patch night'`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/prtg/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which client core is in the worst shape right now? | `prtg-cli fleet status --agent` |
| What is down and unacknowledged across every core, oldest first? | `prtg-cli fleet alarms --status down --unacked --agent` |
| What changed overnight: new alarms and recoveries, with how each sensor came back? | `prtg-cli fleet handoff --since 12h --agent` |
| What has someone paused or acknowledged with no end date for more than a week? | `prtg-cli fleet silenced --indefinite --older-than 7d --agent` |
| Which cores are behind on version, have a license or maintenance ending, or a disconnected probe? | `prtg-cli fleet drift --within 30d --agent` |
| Which probes does each client have, and are they connected? | `prtg-cli fleet exec /experimental/probes --agent` |
| Acknowledge the ISP outage alarm on every client with the ticket number - preview first | `prtg-cli fleet ack --filter 'name = "Ping"' --message 'TKT-4411 ISP outage' --duration 2h` |
| After maintenance, resume the paused devices everywhere and rescan them | `prtg-cli fleet resume --kind device --filter 'name = "fw01"' --scan` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/prtg/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/prtg/guide.md).

## What makes this one different

Most PRTG integrations talk to one PRTG server with one key. This connector is built for the MSP case of one core per client: register each core once with core add (its key stays in an environment variable, never on disk) and the fleet commands ask every core in one run, sort the merged result worst first, and keep a core that fails as an error row instead of counting it as healthy. It also wraps every PRTG API v2 operation as a typed command you can point at one core with --core.

PRTG Desktop and PRTG MultiBoard show several PRTG servers to a person at a desk; this connector gives your AI agent the same cross-core view as data it can query, plus previewed bulk actions (acknowledge, pause, resume) across every core at once. It complements the PRTG web interface, it does not replace it.

## The pain this closes

- Many MSPs run one PRTG core per client. Paessler's MSP offering says PRTG MSP Edition supports running clients on fully separate instances, and each instance has its own web interface, alarm list and API key, so 'what is down for every client' means opening each core.
- Paessler retired the PRTG Enterprise Console, its multi-server desktop client, from PRTG 19.4.53 and points users to PRTG Desktop and PRTG MultiBoard instead (Paessler blog, 'PRTG Enterprise Console EOL - Switch to PRTG Desktop Now'). Those are viewers for a person at a desk, not something an AI agent or a script can ask.
- Muted sensors hide outages: an acknowledgement or pause with no end date stays silent until someone remembers it, and there is no single list of everything paused or acknowledged across cores.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/prtg/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/prtg/install.ps1 | iex
```

After install, authenticate once with your PRTG Network Monitor credentials, then verify with `prtg-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet alarms, fleet status, fleet handoff, fleet silenced, fleet drift, fleet exec, sensors get, devices list-experimental, sync, search | Allow |
| Write (routine) | fleet ack, fleet pause, fleet resume, sensors acknowledge sensor, sensors pause sensor, devices multi-scan-now, sensors update, objects move object, import | Preview first (fleet commands preview by default; --dry-run elsewhere; MCP confirm), then a reviewed write |
| Acts on client networks | devices trigger-autodiscovery, groups trigger-autodiscovery, autodiscoverytasks start-auto-discovery-task, devices metascan | Human-in-the-loop only |
| Credential / identity | any command that can emit a secret (session login, users api-keys create, system license-info), users api-keys and password reset commands, users pause and resume | Human-in-the-loop only |
| Destructive | sensors delete, devices delete, groups delete, probes delete, objects multi-delete, autodiscoverytasks delete-auto-discovery-task, users delete-apikey | Human-in-the-loop only, explicit confirmation |

The connector reads PRTG monitoring data freely, and changes PRTG only through write commands: acknowledgements, pauses, resumes and scans day to day; deleting sensors, devices, groups and probes; and user and API key management. Let an agent run reads and the fleet previews on its own, approve every --yes and every MCP confirm, and keep deletes, discovery scans of client networks and anything touching users or keys behind a human. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/prtg/governance.md).

## Frequently asked questions

### Is there an MCP server for PRTG Network Monitor?

Yes - this one. A free, open source MCP server and Claude Code Skill for PRTG Network Monitor, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the PRTG Network Monitor MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `PRTG_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `PRTG_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the PRTG Network Monitor MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my PRTG data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Which PRTG versions and editions does it work with?

It uses PRTG API v2, which the PRTG application server serves on port 1616 by default. Paessler documents API v2 as not available on PRTG clusters, so use it with single-node PRTG Network Monitor cores. The fleet views read the typed list endpoints Paessler marks deprecated in favour of the experimental ones; PRTG still serves them, and they also work on cores older than 24.3.

### Can an agent pause or acknowledge things on my clients' cores by accident?

fleet ack, fleet pause and fleet resume only preview what they would do on each core until you add --yes. Over MCP, every tool that changes PRTG returns that preview until it is called again with confirm: true, and setting PRTG_MCP_READ_ONLY=1 makes the MCP server refuse every write. Single-core write commands such as sensors pause sensor send when you run them, so keep agents to reads and --dry-run previews and see the safety model below.

### Where do the API keys live?

In environment variables. For one core set PRTG_SERVER and PRTG_API_KEY; for many, core add records each core's server and PRTG_FLEET_KEYS holds every core's key as name=key pairs (acme=KEY1,globex=KEY2); the keys are never written to disk. Create keys in PRTG under Account Settings > My Account > API Keys, with the lowest access level your workflow needs.

## More Network Monitoring connectors

Run more than one Network Monitoring tool, or comparing options? These connectors work the same way: [Cisco Meraki](https://msp-skills.compoundingteams.com/skills/cisco-meraki.md) · [Domotz](https://msp-skills.compoundingteams.com/skills/domotz.md) · [UniFi Network](https://msp-skills.compoundingteams.com/skills/unifi-network.md)

## Status

Beta. Validated against the PRTG Network Monitor API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
