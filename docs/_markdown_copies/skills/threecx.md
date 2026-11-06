> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/threecx/

# The 3CX MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by 3CX Ltd.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for 3CX. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects 3CX to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask which client's 3CX phone system needs you today and get one ranked answer across every PBX you manage: trunks down, builds behind, licences and maintenance running out, backups gone stale. Each PBX keeps its own API credential, nothing is cached between runs, and the full 3CX Configuration API stays one command away on any single system.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/threecx/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/threecx)

## Instead of clicking through 3CX, just ask

**Instead of** Logging into every client's 3CX admin console each morning to read the PBX Status page
**just ask:** *"Which of our 3CX phone systems needs attention today?"*
Your agent runs: `threecx-cli fleet health --agent`

**Instead of** Waiting for a client to call in because their SIP trunk dropped overnight
**just ask:** *"Is any client's 3CX trunk offline or misconfigured right now?"*
Your agent runs: `threecx-cli fleet trunks --offline --agent`

**Instead of** Keeping a spreadsheet of 3CX licence and maintenance renewal dates per client
**just ask:** *"Which 3CX licences or maintenance plans expire in the next 60 days?"*
Your agent runs: `threecx-cli fleet licences --within 60d --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/threecx/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which client phone systems are failing or warning, and why? | `threecx-cli fleet health --agent` |
| Which trunks are offline, misconfigured or in an unknown state across every client? | `threecx-cli fleet trunks --offline --agent` |
| Whose licence or maintenance expires in the next 60 days? | `threecx-cli fleet licences --within 60d --agent` |
| Which PBXes are behind the newest 3CX build in our fleet or have updates pending? | `threecx-cli fleet versions --behind --agent` |
| Which clients have no backup in the last two days? | `threecx-cli fleet backups --stale 48h --problems --agent` |
| What changed on our phone systems since yesterday? | `threecx-cli fleet changes --since 24h --agent` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/threecx/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/threecx/guide.md).

## What makes this one different

This connector is built for a fleet: it registers every client PBX with its own credential and answers health, trunk, licence, version and backup questions for all of them in one parallel run, with a pass, warn, fail or unknown verdict per PBX. Single-PBX projects such as the 3cx-xapi CLI in the Printing Press library and SSIG-IT/3cx-mcp-server cover one phone system per connection.

3CX's own MCP server (a V20 preview) connects one user to one PBX; this complements it by comparing every client's phone system side by side and keeping a local history so you can see what changed between runs.

## The pain this closes

- Every 3CX tool, from the admin console to 3CX's own MCP preview, talks to one phone system at a time, so an MSP with 40 clients checks 40 consoles to answer one question.
- A dead SIP trunk, a lapsed maintenance plan or a backup that quietly stopped is found when the client calls, not before: resellers on the 3CX forum have built their own polling apps to catch these across instances.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/threecx/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/threecx/install.ps1 | iex
```

After install, authenticate once with your 3CX credentials, then verify with `threecx-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | fleet health, fleet trunks, fleet licences, fleet versions, fleet backups, fleet changes, fleet exec, users list, trunks list, system-status | Allow |
| Write (routine) | users-id update-user, queues-id update-queue, holidays-id update-holiday, office-hours update, import | Preview with --dry-run, then a reviewed write |
| Phone-system control | services restart, services restart-operating-system, install-updates, users reboot-phone, users reprovision-all-phones, sip-devices bulk-update-firmware, backups-file-name restore, users make-call | Human-in-the-loop only |
| Credential / identity | any command that can return or mint a credential: conference-settings generate-api-key, system-status apitoken, service-principals-id generate-app-token, users-id generate-prov-link, users regenerate-passwords, service-principals create, oauth-clients | Human-in-the-loop only |
| Destructive | every delete and purge: users-id delete-user, users batch-delete, purge-calls, purge-all-logs, recordings purge-local, backups purge-local, fleet remove, local-data purge | Human-in-the-loop only, explicit confirmation |

The fleet commands and every GET command only read. Writes (setting changes, user edits), phone-system control (service restarts, phone reboots, firmware pushes, updates, backup restores), credential commands (API keys, tokens, provisioning links) and deletes or purges are separate tiers that should need a human. Scope each PBX's API client to the department and role your workflow needs. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/threecx/governance.md).

## Frequently asked questions

### Is there an MCP server for 3CX?

Yes - this one. A free, open source MCP server and Claude Code Skill for 3CX, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the 3CX MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `THREECX_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `THREECX_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, on paid ChatGPT plans. ChatGPT connects to remote MCP servers over HTTPS, so it needs the 3CX MCP server's token-protected HTTP mode hosted over HTTPS. Step-by-step in the install guide.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my 3CX data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Does it change anything on my clients' phone systems?

The fleet commands only read. The single-PBX commands cover the whole Configuration API, including writes such as restarting services or rebooting phones; in the MCP server every write returns a preview until the agent sends confirm: true, and on the CLI you preview any write with --dry-run first. MCP read-only mode (THREECX_MCP_READ_ONLY) removes the write tools entirely.

### Which 3CX versions and licences does this need?

3CX V20 or later with the Configuration API, which 3CX documents as needing an 8SC or higher licence. On each PBX, create an API client under Admin Console > Integrations > API with Configuration API access; its client ID is the DN number shown there and the API key is shown once. A PBX on a lower edition reports an error in its own row instead of stopping the whole fleet run.

## More Other connectors

Run more than one Other tool, or comparing options? These connectors work the same way: [Resource Guru](https://msp-skills.compoundingteams.com/skills/resourceguru.md) · [VMware vSphere](https://msp-skills.compoundingteams.com/skills/vsphere.md)

## Status

Beta. Validated against the 3CX API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
