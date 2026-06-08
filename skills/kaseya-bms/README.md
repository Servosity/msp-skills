# Kaseya BMS + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Kaseya BMS
> API. Not affiliated with, endorsed by, or sponsored by Kaseya US LLC.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/kaseya-bms/">
    <img src="../../docs/assets/video/kaseya-bms/animated-og.gif" alt="Kaseya BMS demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/kaseya-bms/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

The first dedicated CLI and MCP server for Kaseya BMS - the full PSA surface plus offline sync, full-text search, and the queue, contract-burn, and unbilled-time analytics the web grid can't compute. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Kaseya BMS skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `kaseya-bms-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `kaseya-bms-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `kaseya-bms-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Kaseya BMS MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `kaseya-bms-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Kaseya BMS MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/kaseya-bms-v0.1.0/kaseya-bms-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Kaseya BMS release on the [releases page](https://github.com/servosity/msp-skills/releases?q=kaseya-bms).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install kaseya-bms@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Kaseya BMS Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-bms/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-bms/install.ps1 | iex`. Then authenticate per the README and run `kaseya-bms-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-bms/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-bms/install.sh)
```

The installer drops both `kaseya-bms-cli` (the CLI) and `kaseya-bms-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
kaseya-bms-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-bms/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-bms/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update kaseya-bms@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/kaseya-bms --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/kaseya-bms --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `kaseya-bms-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the kaseya-bms skill from https://github.com/servosity/msp-skills/tree/main/skills/kaseya-bms. The skill defines how its required CLI (`kaseya-bms-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Kaseya BMS issues short-lived JWTs. The simplest path is to mint one from your BMS
API-user credentials and let the CLI store it:

```bash
export KASEYA_BMS_USERNAME="api-user@yourmsp.com"
export KASEYA_BMS_PASSWORD="<password>"   # env-only; never passed on the command line
export KASEYA_BMS_TENANT="yourmsp"        # your company/tenant name from My Settings
kaseya-bms-cli auth login                 # exchanges these for a JWT and saves it
```

Already have a JWT? Skip the login and pass it directly instead:

```bash
KASEYA_BMS_TOKEN=<jwt> kaseya-bms-cli doctor
```

Non-US tenant? Point at your region with `KASEYA_BMS_BASE_URL` (default
`https://api.bms.kaseya.com`). `doctor` confirms the credentials work before you run
anything that touches data; when commands start returning a 401 Security Error, run
`kaseya-bms-cli auth login` again to refresh the token.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which queues are underwater and what's going stale before standup? | `kaseya-bms-cli queue-health --agent` |
| Which open tickets haven't been touched in a week, oldest first? | `kaseya-bms-cli stale-tickets --days 7 --agent` |
| Who's overloaded and who can take the next ticket? | `kaseya-bms-cli workload --agent` |
| How much of each contract have we burned this quarter? | `kaseya-bms-cli contract-burn --window-days 90 --agent` |
| What approved billable time is ready to invoice, by account? | `kaseya-bms-cli unbilled --agent` |
| What's the open sales pipeline by stage, and which deals have slipped? | `kaseya-bms-cli pipeline --agent` |
| Find every ticket mentioning a phrase across the whole tenant | `kaseya-bms-cli search "VPN outage"` |
| Sync the tenant into a local mirror for instant offline queries | `kaseya-bms-cli sync` |

Beyond these, the CLI covers the full BMS surface - 472 commands across the service
desk, CRM, contracts, finance, projects, inventory, and integrations.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Kaseya BMS integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "how many approved billable hours are unbilled across every client this month" or "which contracts are most burned down heading into renewal" - and the BMS API's 1,500-requests-per-hour-per-endpoint limit punishes you for asking it live.

This skill syncs Kaseya BMS into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `workload` (open tickets joined with hours logged per technician) and `contract-burn` (time logs routed through their tickets' contracts) join across tickets, time logs, and contracts - work a stateless API wrapper can't do.

## The pain this closes

Ask any MSP running Kaseya BMS what they think of the reporting, and the answer on r/msp and in the MSPGeek community is consistent: it's the weak spot. The questions a service desk asks every morning - which queues are underwater, which tickets have gone stale, who's buried and who can take the next escalation - are an export to Excel and a pivot table, not a click. Month-end is the same chore: pulling approved-but-unbilled time per client for the invoice run, and seeing how much of a prepaid contract a client has burned before the renewal call.

This skill turns those into one command each, answered from the local mirror:

- `kaseya-bms-cli queue-health --agent` - open ticket volume by queue, priority, and status, with stale counts flagged before standup.
- `kaseya-bms-cli stale-tickets --days 7 --agent` - the specific open tickets nobody has touched in a week, oldest first.
- `kaseya-bms-cli workload --agent` - open load per technician joined with hours logged, so you can balance the next dispatch.
- `kaseya-bms-cli unbilled --agent` - approved, billable, not-yet-billed time grouped by account, in hours - the ready-to-bill review.
- `kaseya-bms-cli contract-burn --window-days 90 --agent` - hours consumed and percent of period elapsed per contract, at-risk first.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Kaseya BMS MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Kaseya BMS credentials once.

### Is my Kaseya BMS data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my Kaseya BMS API rate limits?

Rarely. BMS allows 1,500 requests per hour per endpoint. Reads default to the local SQLite mirror (`--data-source auto`), so day-to-day questions cost **zero** API calls; you only spend the budget when you `sync` or explicitly query live with `--data-source live`.

### Do I need to be a Kaseya customer, and how do I authenticate?

Yes - you need a Kaseya BMS tenant and an API user. The skill authenticates as that user with `kaseya-bms-cli auth login` (your BMS username, password, and tenant name), or you can paste a pre-minted JWT via `KASEYA_BMS_TOKEN`. It can only ever do what that BMS user is permitted to do.

### Will this replace my Kaseya BMS portal?

No. It's a faster path for the questions you ask every day - queue health, unbilled time, pipeline - and for letting an AI agent drive the service desk. The BMS portal stays your system of record.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** (243) | `queue-health`, `stale-tickets`, `unbilled`, `pipeline`, `servicedesk get-ticket`, `search` | Allow |
| **Write - routine** (124) | `servicedesk post-ticket`, `servicedesk assign-ticket`, `crm post-account`, `finance mark-invoices-as-sent`, `import` | Preview with `--dry-run`, then a reviewed write |
| **Credential / security** | `integrations get-itgpassword-value`, `integrations get-vsa-access-info`, `security refresh-token`, `auth login` | Human-in-the-loop only |
| **Destructive** (21) | `servicedesk delete-ticket`, `crm delete-contact`, `system delete-attachment` | Human-in-the-loop only |
| **Admin** (75) | the `admin` group: webhooks, workflows, services, K1 access control, Teams channels | Operator-only, not for agents |

The `mcp:read-only` annotation on every GET command lets MCP clients gate writes automatically; the routine-write tier covers every `POST`/`PUT`/`PATCH` plus the verb-less `import`. The strongest control is the **scope you grant the Kaseya BMS credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Kaseya BMS API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-07._
