# Sophos Central + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Sophos Central
> API. Not affiliated with, endorsed by, or sponsored by Sophos Limited.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/sophos-central/">
    <img src="../../docs/assets/video/sophos-central/animated-og.gif" alt="Sophos Central demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/sophos-central/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

The Sophos Central partner, organization and tenant APIs, routed to the right tenant and region automatically, plus a cross-tenant triage queue, fleet tamper remediation, a multi-tenant SIEM collector and hostname-based containment. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Sophos Central skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `sophos-central-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `sophos-central-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `sophos-central-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `sophos-central-mcp --transport http` (with `SOPHOS_CENTRAL_MCP_HTTP_TOKEN` set; it refuses to start without one) and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `sophos-central-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Sophos Central MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/sophos-central-v0.1.0/sophos-central-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Sophos Central release on the [releases page](https://github.com/servosity/msp-skills/releases?q=sophos-central).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install sophos-central@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Sophos Central Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.ps1 | iex`. Then authenticate per the README and run `sophos-central-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.sh)
```

The installer drops both `sophos-central-cli` (the CLI) and `sophos-central-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
sophos-central-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sophos-central/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update sophos-central@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/sophos-central --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/sophos-central --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `sophos-central-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the sophos-central skill from https://github.com/servosity/msp-skills/tree/main/skills/sophos-central. The skill defines how its required CLI (`sophos-central-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Sophos Central portal):

```bash
SOPHOS_CENTRAL_CLIENT_ID=<value> SOPHOS_CENTRAL_CLIENT_SECRET=<value> sophos-central-cli doctor
```

Create the credential in Sophos Central first: **Partner Dashboard > Settings & Policies > API Credentials** for a partner, or **Global Settings > API Credentials Management** for an organization or a single customer. `doctor` checks config, paths and API reachability, then mints a token with your credential and reports whether Sophos accepted it. A partner or organization credential picks one customer for tenant-scoped commands with `--tenant <tenant-id>` (or `SOPHOS_CENTRAL_TENANT_ID`); the `fleet` commands walk every customer on their own.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What needs attention across all my customers today? | `sophos-central-cli fleet triage` |
| Which customers have the worst Account Health Check scores, and who slipped since the last refresh? | `sophos-central-cli fleet health --limit 10` |
| Where are the security and commercial gaps, customer by customer? | `sophos-central-cli fleet gaps --kind all` |
| Which machines have tamper protection off across every customer? | `sophos-central-cli fleet tamper` |
| Isolate this machine without looking up which tenant it is in | `sophos-central-cli contain "WS-FIN-07" --scan` |
| What did each customer bill last month, and does it match what is deployed? | `sophos-central-cli fleet revenue --month 2026-09` |
| Feed every customer's SIEM events and alerts into my log pipeline | `sophos-central-cli fleet siem-collect --stream both` |
| Refresh every customer tenant into the local store | `sophos-central-cli fleet refresh` |
| Which credential am I using, and is it a partner, organization or tenant? | `sophos-central-cli whoami` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Sophos Central integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 60 customers have machines with tamper protection off, and whose health score dropped since last week".

This skill syncs Sophos Central into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `fleet triage`, `fleet gaps` and `fleet revenue` join across tenants, endpoints, alerts, Account Health Check scores, licences and partner billing - work a stateless API wrapper can't do.

Live wrappers such as [rijul170/sophos-central-mcp](https://github.com/rijul170/sophos-central-mcp) take a tenant ID on each call and answer one tenant at a time. This connector also resolves the caller with `whoami` and routes every tenant call to that tenant's regional host for you, but `fleet refresh` first pulls every customer tenant into the local store with a per-tenant ok/forbidden/error ledger, so a missing customer shows up instead of silently dropping out of the answer. It complements the Sophos Central Partner Dashboard rather than replacing it: the dashboard stays the place for policy authoring, investigations and Sophos's own consolidated alert view.

## The pain this closes

Sophos Central is tenant-by-tenant underneath. Sophos's own [partner getting-started guide](https://developer.sophos.com/getting-started/) spells out the loop every MSP script has to run: `whoami` for the partner ID, page through the tenant list, then call each tenant's own regional API host with an `X-Tenant-ID` header. Community tooling usually stops at one tenant - the [SophosCentral-Powershell](https://github.com/jd1100/SophosCentral-Powershell) module says "There is currently no support for multiple tenants" - so every MSP rewrites the same loop, and the questions behind the Partner Dashboard's alert roll-up still mean drilling into one customer at a time.

- `sophos-central-cli fleet triage` - one ranked morning work queue across every customer.
- `sophos-central-cli fleet tamper` - every machine with tamper protection off, previewed first, switched back on only with `--yes`.
- `sophos-central-cli contain "WS-FIN-07" --scan` - isolate a machine by hostname mid-incident without knowing its tenant.
- `sophos-central-cli fleet revenue` - monthly Sophos billing per customer next to what is actually deployed.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Sophos Central MCP server runs locally, but it speaks HTTP natively: start it with `sophos-central-mcp --transport http` and `SOPHOS_CENTRAL_MCP_HTTP_TOKEN` set (it serves `http://127.0.0.1:7777/mcp` by default and requires that token as a bearer header) and put that endpoint behind an HTTPS tunnel or your own reverse proxy. No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Sophos Central credentials once.

### Is my Sophos Central data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### What Sophos Central API credential do I need?

An API credential (a service principal) created in Sophos Central: **Partner Dashboard > Settings & Policies > API Credentials** for a partner, or **Global Settings > API Credentials Management** for an organization or a single customer. Set `SOPHOS_CENTRAL_CLIENT_ID` and `SOPHOS_CENTRAL_CLIENT_SECRET`. The role you give it is the real permission boundary: **Service Principal Read-Only** covers every report here but cannot run Live Discover queries; isolation, scans and tamper protection need a role that allows them.

### Does it work with a partner, organization and single-tenant credential?

Yes. `whoami` tells the CLI which kind of credential it holds. A partner or organization credential walks every customer tenant in the `fleet` commands and picks one customer with `--tenant` (or `SOPHOS_CENTRAL_TENANT_ID`) for tenant-scoped commands such as `endpoints` or `alerts`. A single-tenant credential works on its own tenant.

### Will this hit Sophos Central API rate limits?

Calls go through a shared throttle of 1.5 requests per second by default (`--rate-limit`), under Sophos's own limits. `fleet refresh --plan` prints the tenant count and estimated calls and time before you run it. After a refresh, `fleet triage`, `fleet health` and `fleet gaps` read the local store and make no API calls.

### Why do some customers show as forbidden?

Sophos returns 403 to a partner for trial tenants and for term customers without Partner Assistance. `fleet refresh` records those in its per-tenant ledger and carries on, and `fleet triage` lists them as blind tenants so you can see what was not read. Trial tenants are skipped unless you pass `--include-trial`.

### Does it cover Sophos XDR or MDR Cases and Detections?

Not in this version. It covers the REST APIs: alerts, endpoints, policies, exclusions, Live Discover, XDR Data Lake queries, SIEM events and alerts, Account Health Check, licences and partner billing. Cases and Detections are not included.

### Does it replace the Sophos Central Partner Dashboard?

No. The dashboard stays best for policy authoring, investigations and day-to-day console work. This skill answers the across-every-customer questions and runs scripted actions from your AI agent.


### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet triage`, `fleet health`, `fleet gaps`, `fleet revenue`, `fleet refresh`, `whoami`, `tenants list`, `alerts list`, `endpoints list`, `search`; `contain` and `fleet tamper` without `--yes` only preview | Allow |
| Endpoint response | `contain --yes`, `endpoints set-isolation`, `endpoints bulk-isolate`, `endpoints scan`, `endpoints collect-forensic-log`, `endpoints collect-memory-dump`, `live-discover run-query`, `fleet tamper --yes` | Human-in-the-loop, explicit confirmation - these reach real customer machines |
| Protection and policy config | `exclusions add-scanning`, `allowed-items post-by-id`, `blocked-items post-by-id`, `policies update-policy`, `policies update-settings-of-policy`, `web-control update-web-decryption-excluded-sites` | Preview with `--dry-run`, then a human-approved write - an exclusion or allowed item can open a hole |
| Write (routine) | `alerts act-on`, `endpoint-groups create-tenant`, `endpoint-tags`, `health-check post-snooze`, `xdr start-query-run` | Preview with `--dry-run`, then a reviewed write |
| Credential and access | `endpoints get-tamper-protection`, `endpoints set-tamper-protection`, `migrations get-job` (return a tamper password or migration token; kept out of the MCP server), `auth login`, `partner-admins create`, `admins create-role-assignment` | Human-in-the-loop only |
| Destructive and commercial | `endpoints delete`, `endpoints bulk-delete`, `policies delete-policy`, `users delete-tenant-by-id`, `migrations create-sending-job`, `tenants create`, `tenants post-account-amendment` | Human-in-the-loop only, explicit confirmation |


The strongest control is the **scope you grant the Sophos Central credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Sophos Central API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-14._
