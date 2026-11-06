> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/windows-server/

# The Windows Server MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Microsoft Corporation.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Windows Server. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Windows Server to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI "which of my Windows servers need attention today, and why?" and get one ok, warn, crit or unknown verdict per server: pending reboot, low disks, stopped automatic services, System log errors, patch age, pending updates, expiring certificates, Defender, time sync and failed scheduled tasks. It connects over WinRM with HTTPS, runs only fixed read scripts, and changes nothing except a service you confirm.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/windows-server/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/windows-server)

## Instead of clicking through Windows Server, just ask

**Instead of** Opening an RDP session on each client server every morning to check disks, services, Event Viewer and Windows Update
**just ask:** *"Which of my Windows servers need attention today, and why?"*
Your agent runs: `windows-server-cli health --agent`

**Instead of** Scrolling Event Viewer on a down ticket to work out why a service stopped before restarting it
**just ask:** *"Which automatic services are stopped on fs01, and what stopped them?"*
Your agent runs: `windows-server-cli services down --server fs01 --agent`

**Instead of** Finding out a certificate expired when the client calls because their site or VPN stopped working
**just ask:** *"Which certificates on the default server expire in the next 30 days?"*
Your agent runs: `windows-server-cli certificates --expiring-days 30 --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/windows-server/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which of my servers need attention today, and why? | `windows-server-cli health --agent` |
| Which automatic services are stopped, and which Service Control Manager event explains each stop? | `windows-server-cli services down --agent` |
| Which System and Application errors are new on a server compared with its own last 7 days? | `windows-server-cli eventlog new --window 24h --baseline 7d --agent` |
| Who added or removed a local Administrators member this week, and when? | `windows-server-cli local-admins changes --hours 168 --agent` |
| What changed in roles, shares, local admins, certificates and updates since the last snapshot? | `windows-server-cli inventory diff --agent` |
| Which volumes are low on free space? | `windows-server-cli volumes --agent --select server,drive_letter,size_bytes,percent_free` |
| Which scheduled tasks failed their last run? | `windows-server-cli scheduled-tasks --failed --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/windows-server/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/windows-server/guide.md).

## What makes this one different

Most Windows MCP servers and scripts give the agent a PowerShell prompt on the server. This one cannot run arbitrary script: every command is one of a fixed set of hash-pinned read scripts with typed, validated arguments, the server comes from your config by name (never from the agent), and certificate verification is always on. Its compound commands roll every check into one verdict per server and diff inventory between runs.

Windows Server has no built-in AI assistant for MSPs, and Windows Admin Center shows one server at a time. This adds a verdict across every server in your config, the reason behind each stopped service, change history between runs, and it complements whatever RMM agent you already run.

## The pain this closes

- Daily server checks are still a manual round on many MSP desks: "monitor critical server services" daily, then disk space and Event Viewer's application, security and system logs weekly, as the Active Directory Pro Windows Server maintenance checklist lays it out, one server at a time.
- When something breaks, the first hour goes to working out what changed: a new local administrator, a removed role, a service that stopped, or an error that has never fired on that server before.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/windows-server/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/windows-server/install.ps1 | iex
```

After install, authenticate once with your Windows Server credentials, then verify with `windows-server-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read (server) | health, system, volumes, services list, services get, services down, eventlog, eventlog new, installed-updates, pending-updates, features, local-admins, local-admins changes, shares, scheduled-tasks, certificates, defender, time-sync, hyperv-vms | Allow |
| Read (local) | events list, inventory diff, servers list, search, analytics | Allow |
| Write (local only) | inventory snapshot, health (records history), sync, local-data purge (preview unless --yes) | Allow; review local-data purge |
| Credential | servers login, servers logout, auth set-credentials, auth logout | Human-in-the-loop only |
| Server action (changes a customer server) | services restart, services start | Human-in-the-loop only: read the preview, then run with --yes or confirm: true |

It reads server state over WinRM with fixed scripts and writes nothing to the server except two service actions: restart a service, or start a stopped automatic one, each previewed first and sent only with --yes (CLI) or confirm: true (MCP). Locally it writes the server list's stored passwords (0600 store), the health and inventory history, and the agent learning store. Keep agents on reads; have a human approve every service action. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/windows-server/governance.md).

## Frequently asked questions

### Is there an MCP server for Windows Server?

Yes - this one. A free, open source MCP server and Claude Code Skill for Windows Server, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Windows Server MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `WINDOWS_SERVER_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `WINDOWS_SERVER_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the Windows Server MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Windows Server data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Do I need to install an agent on each server?

No. It uses WinRM over HTTPS (port 5986), which is built into Windows Server. Enable an HTTPS listener and give the CLI a local or domain account in the server's Administrators group. Nothing is installed on the server.

### Can the AI run its own PowerShell on my client's server?

No. Every command runs one fixed script that ships inside the binary and is checked by hash before it runs. Arguments such as a service name are validated against a strict pattern and passed as typed data, never pasted into script text.

### What does it change on a server?

Only two things, and only after a preview: restarting one service, or starting one stopped automatic service. Over MCP the change runs only with confirm: true; on the CLI only with --yes.

## More Monitoring connectors

Run more than one Monitoring tool, or comparing options? These connectors work the same way: [Better Stack](https://msp-skills.compoundingteams.com/skills/betterstack.md) · [Domain and Certificate Tracker](https://msp-skills.compoundingteams.com/skills/domain-cert-tracker.md)

## Status

Beta. Validated against the Windows Server API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
