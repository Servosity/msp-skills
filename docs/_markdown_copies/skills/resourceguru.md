> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/resourceguru/

# The Resource Guru MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Resource Guru Limited.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Resource Guru. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Resource Guru to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI 'who is overbooked next week?' or 'who has capacity for this project?' and get an answer Resource Guru's reports never show: booked-vs-available utilization for every resource on every day. The skill syncs your whole schedule into a local mirror, then computes per-day utilization, fleet-wide overbooking, who is on the bench, and remaining capacity offline - no report export, no pivot table.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/resourceguru/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/resourceguru)

## Instead of clicking through Resource Guru, just ask

**Instead of** Download a booking report and pivot it in a spreadsheet to eyeball who is overcommitted day by day
**just ask:** *"Who is overbooked between June 1 and June 30?"*
Your agent runs: `resourceguru-cli overbooked --start 2026-06-01 --end 2026-06-30 --agent`

**Instead of** Click through each person's schedule to guess who has room for new work
**just ask:** *"Who has slack next week to take on a new project?"*
Your agent runs: `resourceguru-cli bench --start 2026-06-08 --end 2026-06-14 --threshold 50 --agent`

**Instead of** Open the calendar and add up free hours by hand before promising a deadline
**just ask:** *"How much bookable capacity is left in July?"*
Your agent runs: `resourceguru-cli capacity --start 2026-07-01 --end 2026-07-31 --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/resourceguru/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Who is overbooked across the whole fleet this month? | `resourceguru-cli overbooked --start 2026-06-01 --end 2026-06-30 --agent` |
| What is each person's utilization, day by day? | `resourceguru-cli utilization --start 2026-06-01 --end 2026-06-30 --heatmap --agent` |
| Who has slack to take on new work next week? | `resourceguru-cli bench --start 2026-06-08 --end 2026-06-14 --threshold 50 --agent` |
| How much bookable capacity is left next month? | `resourceguru-cli capacity --start 2026-07-01 --end 2026-07-31 --agent` |
| What changed on the schedule in the last week? | `resourceguru-cli since 7d --agent` |
| How is workload distributed across the team? | `resourceguru-cli load --json` |
| Find a booking, client, or project anywhere in the synced schedule | `resourceguru-cli search "website redesign"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/resourceguru/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/resourceguru/guide.md).

## What makes this one different

Most Resource Guru integrations proxy each question into a single live API call - fine for one record, useless for 'every resource, every day, this quarter.' This skill syncs your whole schedule into a local SQLite mirror first, so utilization, overbooked, bench, and capacity are computed offline across the entire fleet in one pass, and your agent sees the answer instead of raw bulk data.

Resource Guru's portal answers 'is this person free right now' and its reports give range-level rollups; this skill adds the per-day, fleet-wide utilization math the reports never expose and hands it to your AI agent as a typed command instead of a spreadsheet export.

## The pain this closes

- Resource Guru's reporting gives range-level rollups and has to be downloaded and pivoted in a spreadsheet to answer day-level questions about who is over- or under-booked.
- The booking-clash warning flags one over-allocation at a time on write, but there is no single view of every overcommitted resource-day across the whole fleet for a window.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/resourceguru/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/resourceguru/install.ps1 | iex
```

After install, authenticate once with your Resource Guru credentials, then verify with `resourceguru-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | utilization, overbooked, bench, capacity, since, load, search, accounts list, and any get | Allow |
| Write (routine) | bookings create, bookings update, clients create, projects update, activity-types create | Preview with --dry-run, then a reviewed write |
| Destructive / config | bookings delete, clients delete, resources delete, calendars delete, webhooks delete | Human-in-the-loop only |

The skill reads your schedule and computes analytics locally, and it can also create, update, and delete bookings, clients, projects, and other objects through the same API the web app uses. Reads are always safe. Keep autonomous agents to reads plus previewed (--dry-run) writes, and require a human for any delete or configuration change. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/resourceguru/governance.md).

## Frequently asked questions

### Is there an MCP server for Resource Guru?

Yes - this one. A free, open source MCP server and Claude Code Skill for Resource Guru, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Resource Guru MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `RESOURCEGURU_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `RESOURCEGURU_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way resourceguru-mcp runs, and shares MCP configuration with Codex: add resourceguru-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Resource Guru data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this hit my Resource Guru API rate limits?

No. After the first sync, utilization, overbooked, bench, capacity, and search all read the local SQLite mirror with zero API calls. You only touch the API when you sync or run a live read, and the CLI honors a configurable --rate-limit.

### Do I need to be a Resource Guru admin?

No. You need an account that can read the schedule, authenticated with your account email and password over HTTP Basic. Read-only analytics never need write or admin scope - scope the credential to what you actually use.

### Will this replace the Resource Guru web app?

No. It reports and analyzes; it is not where you edit the schedule. Writes go through the same API the web app uses, so confirm any mutation in Resource Guru afterward. The unique value is the per-day, fleet-wide utilization the portal does not surface.

## Status

Beta. Validated against the Resource Guru API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
