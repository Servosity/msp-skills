# Blackpoint Cyber CompassOne + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Blackpoint Cyber
> API. Not affiliated with, endorsed by, or sponsored by Blackpoint Cyber.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/blackpoint/">
    <img src="../../docs/assets/video/blackpoint/animated-og.gif" alt="Blackpoint Cyber demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/blackpoint/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

CompassOne from the terminal: tenants, detections, assets, vulnerabilities, posture, reports and Cloud Response admin, plus a cross-tenant fleet view of open detections, stale agents and coverage gaps. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Blackpoint Cyber skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `blackpoint-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `blackpoint-mcp` over HTTPS with `BLACKPOINT_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `blackpoint-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `blackpoint-mcp --transport http` with `BLACKPOINT_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `blackpoint-mcp` over HTTPS with `BLACKPOINT_MCP_HTTP_TOKEN` set, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent, if it can send the bearer token. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Blackpoint Cyber MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/blackpoint-v0.1.6/blackpoint-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Blackpoint Cyber release on the [releases page](https://github.com/servosity/msp-skills/releases?q=blackpoint).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install blackpoint@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Blackpoint Cyber Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.ps1 | iex`. Then authenticate per the README and run `blackpoint-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.sh)
```

The installer drops both `blackpoint-cli` (the CLI) and `blackpoint-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
blackpoint-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blackpoint/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update blackpoint@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/blackpoint --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/blackpoint --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `blackpoint-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the blackpoint skill from https://github.com/servosity/msp-skills/tree/main/skills/blackpoint. The skill defines how its required CLI (`blackpoint-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Blackpoint Cyber portal):

```bash
BLACKPOINT_API_TOKEN=<value> blackpoint-cli doctor
```

`doctor` checks config, paths, and API reachability, then sends one authenticated read and reports the credential as valid, rejected (401), scope-limited (403), or pointed at the wrong base URL (404).


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which open detections are oldest across every client? | `blackpoint-cli triage --older-than 24h` |
| Which devices' agents stopped reporting? | `blackpoint-cli stale-agents --days 14` |
| Which customers have no recent scan, no escalation contacts or no posture rating? | `blackpoint-cli gaps --scan-days 30` |
| Which exploited CVE affects the most customers? | `blackpoint-cli cve-spread --exploited` |
| Which tenants have the weakest posture score, and in which NIST function? | `blackpoint-cli leaderboard` |
| Which travel exceptions have expired or end this week? | `blackpoint-cli geo-audit --expiring 7d` |
| What does one client look like right now? | `blackpoint-cli overview "Acme Dental"` |
| What are this client's numbers for the monthly report? | `blackpoint-cli client-report "Acme Dental" --since 30d` |

The cross-tenant commands (`triage`, `stale-agents`, `gaps`, `cve-spread`, `leaderboard`, `geo-audit`, `overview`) read the local mirror: run `blackpoint-cli fleet-sync` first, and again whenever you want fresh numbers. Before the first sync they return an empty list with a hint on stderr. `client-report` and the endpoint commands call the API live.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Blackpoint Cyber integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which open detection, across all 40 of my CompassOne tenants, has waited longest?" Every CompassOne detection, asset, vulnerability and scan call is scoped to one tenant, so the live answer is one call per customer.

This skill syncs Blackpoint Cyber into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `triage`, `gaps` and `cve-spread` join across every tenant's detections, devices, vulnerabilities, scans, contact groups and posture ratings - work a stateless API wrapper can't do.

## The pain this closes

CompassOne is built tenant by tenant: every detection, asset, vulnerability, scan and report call names one customer tenant (the API's `x-tenant-id` header), and the portal views follow the same shape. Reviews of Blackpoint for MSPs say the same thing from the operator's side: [Flamingo's Blackpoint Cyber review](https://www.flamingo.run/blog/blackpoint-cyber-review) notes reviewers find the partner portal "less polished than the detection engine behind it, with a learning curve on reporting and day-to-day navigation." At 30 or 80 customers, "which fire do I put out first?" and "who has no coverage?" become a tab-by-tab exercise.

This skill closes that gap:

- **One detection queue for every client** - `blackpoint-cli triage --older-than 24h` lists open detections across all tenants, oldest first, with tenant, age, alert count and ticket status.
- **Agents that went quiet** - `blackpoint-cli stale-agents --days 14` lists devices across every tenant whose agent has not reported, with Defender status.
- **Coverage gaps before an incident finds them** - `blackpoint-cli gaps --scan-days 30` flags tenants with no recent completed scan, no or empty escalation contact group, or no posture rating.
- **Which patch campaign helps the most customers** - `blackpoint-cli cve-spread --exploited` ranks exploited CVEs by tenants and assets affected.
- **Client reporting in one call** - `blackpoint-cli client-report "Acme Dental" --since 30d` gathers detection count and weekly trend, top entities and threats, posture and report runs for one customer.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Blackpoint Cyber MCP server runs locally over stdio by default. Its HTTP mode (`blackpoint-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `BLACKPOINT_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. See [mcp-install.md](./mcp-install.md). Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Blackpoint Cyber credentials once.

### Is my Blackpoint Cyber data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### What Blackpoint access do I need?

A CompassOne API token, created in the CompassOne portal under API tokens. A partner token reaches every customer tenant; a tenant token sees one customer. A read-only partner token is enough for every read and rollup command. Some routes need entitlements: a 403 on `vulnerabilities` or `users` means the account is not entitled, and fleet-sync records that tenant's slice as failed instead of empty.

### Will this hit my Blackpoint API rate limits?

The key quota is 2000 requests per 15 minutes. fleet-sync makes one or more calls per tenant and data kind, so on a large fleet limit it with `--tenant` and `--kinds`. After a sync, triage, gaps, stale-agents, cve-spread, leaderboard, geo-audit and overview read the local mirror and make no API calls; client-report and the endpoint commands call the API live.

### Will this replace the CompassOne portal?

No. The portal stays your console for investigations, Cloud Response configuration and reports. This connector answers the questions that span customers, and gives your AI agent a safe, previewable path to the few changes the API allows.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `triage`, `overview`, `client-report`, `gaps`, `stale-agents`, `cve-spread`, `leaderboard`, `geo-audit`, `fleet-sync`, `tenants list`, `detections list`, `vulnerabilities`, `posture`, `scans`, `search` | Allow |
| Credential | `darkweb-exposures` (lists breached accounts; each password is replaced by `[withheld]`), `cisco-onboardings create` (takes a Duo secret key), `auth set-token`, `auth logout` | Human-in-the-loop only |
| Write (routine) | `contact-groups create`, `contact-groups update`, `contact-groups add-member`, `contact-groups update-member`, `contact-groups assign-tenants`, `cisco-onboardings sync-users`, `cisco-onboardings complete`, `import` | Preview with --dry-run, then a reviewed write |
| Access policy (Cloud Response) | `ms365 approve-country`, `ms365 approve-user-country`, `ms365 remove-country`, `ms365 remove-user-country`, `cloud-connections approve-country`, `cloud-connections approve-user-country`, `cloud-connections remove-approved-country`, `cloud-connections remove-user-country` | Human-in-the-loop only |
| Destructive | `contact-groups delete`, `contact-groups delete-many`, `contact-groups delete-member`, `cisco-onboardings delete` | Human-in-the-loop only, explicit confirmation |

These policies are yours to apply in your agent setup; the binary does not enforce the Credential tier. The MCP server answers `darkweb-exposures` reads even with `BLACKPOINT_MCP_READ_ONLY=1` (read-only mode drops writes, not reads). Each breached password and each Duo secret key is replaced by `[withheld]` before any output, in every mode, and is never written to the response cache or the local store: read a breached password in the CompassOne portal.

From the CLI, a write sends as soon as you run it: pass `--dry-run` first to preview the exact request. Over MCP, every write returns a preview until the call passes `confirm: true`, and `BLACKPOINT_MCP_READ_ONLY=1` hides every write tool.

The strongest control is the **scope you grant the Blackpoint Cyber credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Blackpoint Cyber API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-28._
