> For AI agents: every page on this site is listed, with links to its Markdown copy, at https://msp-skills.compoundingteams.com/llms.txt. This is the Markdown copy of https://msp-skills.compoundingteams.com/skills/afi/

# The Afi MCP Server - free, local, built for MSPs

> Independent, open source, inspectable. Every line of code is on GitHub
> under Apache-2.0 - built for the MSP community, vendor-neutral by design.
> Not affiliated with, endorsed by, or sponsored by Afi.

**Passes all 4 mechanical gates** (build · command-surface · claims · install). Awaiting its first MSP receipt - [be the first, 60 seconds →](https://msp-skills.compoundingteams.com/verified/#receipt).

Yes - there is an MCP server for Afi. It's free, open source, and runs on your own machine, so your client data stays local unless you route it somewhere yourself. It connects Afi to Claude, ChatGPT, Copilot, or any MCP-capable agent, and installs in about 60 seconds.

Ask your AI 'which mailboxes aren't backed up in Afi?' and get the answer across every tenant at once. This skill walks your whole Afi fleet into a local store, then answers coverage gaps, stale backups, license drift, and per-tenant posture offline - no per-tenant portal clicking, no tripping Afi's rate limits - and runs a verified archive-then-release when someone leaves.

New to the term? An **MCP server** is the same thing ChatGPT calls an app or connector, Claude on the web calls a connector, and Claude Code calls a Skill. [One thing, many names →](https://msp-skills.compoundingteams.com/what-is-an-mcp-server.md)

[Install in 60s →](https://msp-skills.compoundingteams.com/skills/afi/#install)   [View on GitHub →](https://github.com/servosity/msp-skills/tree/main/skills/afi)

## Instead of clicking through Afi, just ask

**Instead of** Open the Afi portal, click into each client tenant, and eyeball which users actually have a backup policy attached.
**just ask:** *"Which resources across the whole fleet have no backup protection?"*
Your agent runs: `afi-cli coverage-gaps --agent`

**Instead of** Tab through every tenant's task history hunting for the nightly backups that quietly stopped landing.
**just ask:** *"Show me protected resources whose newest backup is older than 48 hours."*
Your agent runs: `afi-cli backup-stale --max-age 48h --agent`

**Instead of** Stitch two portal screens into a spreadsheet to compare licenses purchased against seats actually protected.
**just ask:** *"Where am I over- or under-provisioned on Afi licenses?"*
Your agent runs: `afi-cli reconcile-licenses --agent`

## See it in 30 seconds

[Watch the 30-second demo](https://msp-skills.compoundingteams.com/assets/video/afi/demo-30s.mp4)

Demo data is simulated. Every command shown exists in the real CLI.

## What it does

| Question your MSP keeps asking | Command your agent runs |
| --- | --- |
| Which resources have no backup protection at all? | `afi-cli coverage-gaps --agent` |
| Which protected resources have a stale backup (silent failures)? | `afi-cli backup-stale --max-age 48h --agent` |
| Is the whole fleet green this morning, or who failed? | `afi-cli fleet-health --failed-only --agent` |
| What is one tenant's full backup posture for a QBR or ticket? | `afi-cli tenant-scorecard <tenant-id> --agent` |
| Am I over- or under-licensed on Afi seats? | `afi-cli reconcile-licenses --agent` |
| Who is jane.doe@example.com in Afi, across Multi-Geo tenants? | `afi-cli resolve <email-or-id> --agent` |
| Safely back up then release a departing employee's mailbox? | `afi-cli offboard <resource-id> --tenant <tenant-id> --reason "employee departure"` |

Full command reference at [github.com/servosity/msp-skills/blob/main/skills/afi/guide.md](https://github.com/servosity/msp-skills/blob/main/skills/afi/guide.md).

## What makes this one different

Most Afi integrations proxy each question into a live API call - fine for one lookup, but it falls over when you ask a fleet-wide question across dozens of tenants, and Afi's rate limits punish the polling. This skill syncs the whole hierarchy into a local SQLite mirror once, then answers coverage, staleness, and licensing as offline SQL joins: instant, and gentle on the API.

The Afi portal answers one tenant at a time and has no cross-tenant rollup or coverage-gap report. This skill adds the fleet-wide views - coverage gaps, stale backups, license reconciliation, and a verified archive-then-release offboard - that neither the portal nor the rate-limited public API serves directly.

> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](https://msp-skills.compoundingteams.com/skills/servosity.md) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.

> **Comparing backup vendors?** One of our MSP partners told us, "My guys now spend a tenth of the time on backups." Fixing is included in every Servosity co-managed backup plan, and our engineers tell you what they did. [See how co-managed backup works](https://servosity.com/?utm_source=msp-skills&utm_medium=connector-note&utm_campaign=afi&utm_content=docs)

## The pain this closes

- A new mailbox or site gets created in Microsoft 365 or Google Workspace but never gets a backup policy attached - and nobody notices until a restore request arrives and the data was never being protected.
- A backup quietly stops landing on a protected resource; the portal still shows the policy attached, so the silent failure stays invisible until the day you actually need that archive.
- Verifying backup coverage across dozens of client tenants means walking the Afi portal one tenant tab at a time, and the public API throttles you for polling.

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
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/afi/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/afi/install.ps1 | iex
```

After install, authenticate once with your Afi credentials, then verify with `afi-cli --version`.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | coverage-gaps, backup-stale, fleet-health, tenant-scorecard, reconcile-licenses, resolve, fleet-sync, and every list/get command | Allow |
| Write (routine) | orgs create, import, tenants resources protections-protect, tenants jobs tasks-trigger | Preview with --dry-run, then a reviewed write |
| Destructive / config | offboard (releases protection), tenants resources protections-unprotect, tenants archives delete | Human-in-the-loop only |

The skill reads your Afi fleet (installations, orgs, tenants, resources, protections, policies, archives, quotas, and task stats) and can run a small set of writes: create a child org, import records, trigger a backup, and add a protection. Three commands are genuinely destructive - offboard, protections-unprotect, and archives delete - because they release backup coverage or delete an archive. Let an agent run reads freely; require a human to approve every write, and especially the destructive tier. Full details in [governance.md](https://github.com/servosity/msp-skills/blob/main/skills/afi/governance.md).

## Frequently asked questions

### Is there an MCP server for Afi?

Yes - this one. A free, open source MCP server and Claude Code Skill for Afi, built for MSPs. It runs locally on your machine, works with Claude, ChatGPT, Copilot, and any MCP-capable agent, and installs in about 60 seconds.

### Is the Afi MCP server safe for client data?

Yes, by design - and the exceptions are ones you switch on yourself. The CLI, the MCP server, and any local data mirror run on your own machine, and nothing is sent to MSP Skills or any third party unless you ask for it. Three paths can move data off the machine, all opt-in: `--deliver webhook:<url>` posts a command's output to a URL you name; `AFI_FEEDBACK_AUTO_SEND=true` posts feedback you typed to the URL in `AFI_FEEDBACK_ENDPOINT` (with no endpoint set, `feedback` only writes a local file); `--transport http` opens a local MCP listener you then choose whether to expose. Credentials stay in your environment, and every command is safety-tiered (read, write, destructive) so your agent only gets the permissions you grant. Full policy in the safety model on this page.

### Does this work with ChatGPT?

Yes, in the ChatGPT desktop app. It runs MCP servers locally (STDIO), the way afi-mcp runs, and shares MCP configuration with Codex: add afi-mcp under Settings > MCP servers as a STDIO server, or add it once to Codex's ~/.codex/config.toml. ChatGPT on the web is not supported by msp-skills yet: it needs a remote (HTTP) server, and this connector's HTTP mode is paused until it is reprinted; the security note at the top of the install guide explains why. If your team works there, a [hosted gateway](https://msp-skills.compoundingteams.com/answers/mcp-servers-for-msps-2026/#when-a-hosted-gateway-is-the-better-pick) is the better pick; check that your AI app is on the gateway's setup list.

### Do I need to know how to code?

No. Paste one sentence into Claude Code or Codex and your agent does the install, or run a one-line installer. You enter your credentials once.

### Is my Afi data safe?

Your data stays on your machine. The CLI, MCP server, and the local mirror are all local. The AI sees query results, not raw bulk data, and credentials are never bundled or transmitted by MSP Skills.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you already use.

### Will this trip Afi's API rate limits?

Not if you use it as designed. Afi throttles - and may suspend - applications that poll continuously, so this skill walks the fleet into a local store in one respectful, rate-limited pass with fleet-sync, then answers every question offline against that store. You sync on a schedule, not on every question.

### Do I need to be an Afi customer, and what access does the key need?

Yes - you need an Afi account and an Application API key (created in the Afi portal: org level under Configuration to Apps, or tenant level under Service to Settings to Apps). The key inherits the Application's installation scope, so the CLI sees exactly the orgs and tenants that Application is installed on. Each Application supports two keys for rotation.

### Will this replace the Afi portal?

No. Restores, exports, and policy editing still happen in the Afi portal - the public API does not expose them. This skill is the read, report, and guarded-offboard layer: it answers fleet questions and runs the verified archive-then-release, then hands you back to the portal for the actions only it can do.

## More Backup/DR connectors

Run more than one Backup/DR tool, or comparing options? These connectors work the same way: [Acronis Cyber Protect Cloud](https://msp-skills.compoundingteams.com/skills/acronis.md) · [Axcient x360Recover](https://msp-skills.compoundingteams.com/skills/axcient.md) · [Backup Radar](https://msp-skills.compoundingteams.com/skills/backup-radar.md) · [Cove Data Protection](https://msp-skills.compoundingteams.com/skills/cove.md) · [Datto BCDR](https://msp-skills.compoundingteams.com/skills/datto-bcdr.md) · [Proxmox](https://msp-skills.compoundingteams.com/skills/proxmox.md) · [Servosity](https://msp-skills.compoundingteams.com/skills/servosity.md) · [SkyKick](https://msp-skills.compoundingteams.com/skills/skykick.md) · [Synology](https://msp-skills.compoundingteams.com/skills/synology.md) · [Veeam](https://msp-skills.compoundingteams.com/skills/veeam.md)

## Status

Beta. Validated against the Afi API surface and being validated with MSPs running it live against their own production tenants in our weekly **[Build Sessions](https://compoundingteams.com/build-sessions)**.

Build Sessions are free and stay free - [The Build Room](https://compoundingteams.com) is where the deep work happens.

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).
