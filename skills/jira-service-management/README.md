# Jira Service Management + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Jira Service Management
> API. Not affiliated with, endorsed by, or sponsored by Atlassian Pty Ltd.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/jira-service-management/">
    <img src="../../docs/assets/video/jira-service-management/animated-og.gif" alt="Jira Service Management demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/jira-service-management/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every Jira Service Management Cloud request-API (servicedeskapi) and Assets endpoint in one binary, plus SLA breach ranking across queues, internal notes that stay internal, ticket timelines, and Assets (CMDB)-to-ticket joins. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Jira Service Management skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `jira-service-management-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `jira-service-management-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `jira-service-management-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `jira-service-management-mcp --transport http` and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `jira-service-management-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Jira Service Management MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/jira-service-management-v0.1.0/jira-service-management-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Jira Service Management release on the [releases page](https://github.com/servosity/msp-skills/releases?q=jira-service-management).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install jira-service-management@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Jira Service Management Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.ps1 | iex`. Then authenticate per the README and run `jira-service-management-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.sh)
```

The installer drops both `jira-service-management-cli` (the CLI) and `jira-service-management-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
jira-service-management-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jira-service-management/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update jira-service-management@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/jira-service-management --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/jira-service-management --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `jira-service-management-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the jira-service-management skill from https://github.com/servosity/msp-skills/tree/main/skills/jira-service-management. The skill defines how its required CLI (`jira-service-management-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Create an Atlassian API token at [id.atlassian.com/manage-profile/security/api-tokens](https://id.atlassian.com/manage-profile/security/api-tokens), signed in as an agent on your JSM site. Then set three variables: your site subdomain (`acme` for acme.atlassian.net), your Atlassian email, and the token.

**macOS / Linux:**

```bash
export JIRA_SERVICE_MANAGEMENT_SITE=acme
export JIRA_SERVICE_MANAGEMENT_EMAIL=you@example.com
export JIRA_SERVICE_MANAGEMENT_API_TOKEN=<api-token>
jira-service-management-cli doctor
```

**Windows (PowerShell):**

```powershell
$env:JIRA_SERVICE_MANAGEMENT_SITE = "acme"
$env:JIRA_SERVICE_MANAGEMENT_EMAIL = "you@example.com"
$env:JIRA_SERVICE_MANAGEMENT_API_TOKEN = "<api-token>"
jira-service-management-cli doctor
```

Assets (CMDB) commands also need your site's cloudId and Assets workspaceId (Assets is part of JSM Premium and Enterprise). Run `jira-service-management-cli cmdb discover --save` once to look both up and store them in the config file.

`doctor` checks config and paths, names any missing SITE, EMAIL or API_TOKEN, and verifies the token with one authenticated read (GET /servicedesk).


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which tickets breach SLA in the next two hours, across every queue? | `jira-service-management-cli breaches --within 2h` |
| What happened on this ticket, internal notes included, before I reply? | `jira-service-management-cli request timeline SD-123` |
| Add a note for the other techs that the customer will never see | `jira-service-management-cli request note SD-123 "Escalated to network team"` |
| How are Acme Corp's open tickets doing before my client call? | `jira-service-management-cli client "Acme Corp"` |
| What was our SLA breach rate by request type over the last 30 days? | `jira-service-management-cli sla-report --since 30d --group-by requesttype` |
| Which client staff are in no organization, or in two? | `jira-service-management-cli roster 3` |
| Which tickets have touched this server? | `jira-service-management-cli cmdb tickets srv-01` |
| Which laptops are in our RMM export but missing from Assets? | `jira-service-management-cli cmdb reconcile --type Laptop --file rmm.csv --key Name --compare Serial,OS` |

`client` and `sla-report` read the local copy: run `jira-service-management-cli sync --resources organization,request,requesttype,servicedesk` first. Everything else reads live.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Jira Service Management integrations and MCP servers proxy each question into a live API call. That's fine for one record. It breaks down when you ask "which of our 300 open tickets breach SLA next": no single API call ranks tickets across queues, so the AI has to walk every queue, drop duplicates, and read each ticket's SLA metrics itself, one tool call at a time.

This skill does that work inside the binary. `breaches` walks every queue live, de-duplicates tickets and ranks them by SLA time remaining; `request timeline` merges comments (labeled public or internal), status history, approvals and SLA state into one view; `cmdb tickets` joins Assets (CMDB) objects to every ticket connected to them. For period questions it also keeps a **local SQLite copy** with full-text search: after a `sync`, `sla-report`, `client` and `search` answer from the local copy with no API calls.

Atlassian's official Rovo MCP Server is hosted by Atlassian and covers Jira and Confluence plus a growing set of JSM tools: request search, create, transition and approvals ([developer docs](https://developer.atlassian.com/cloud/rovo-mcp/guides/supported-tools/)), operations alerts and Assets search ([support docs](https://support.atlassian.com/atlassian-rovo-mcp-server/docs/supported-tools/)). The community [mcp-atlassian](https://github.com/sooperset/mcp-atlassian) server covers Jira and Confluence broadly, including service desk queues and internal comments. This connector complements them: it runs on your machine, ranks tickets by JSM's own SLA timers across queues, keeps a local copy for period SLA reports and per-client views, and diffs Assets against your RMM or PSA export.

## The pain this closes

Internal notes reach customers by accident. Atlassian's own public tracker carries the request ([JSDCLOUD-17210](https://jira.atlassian.com/browse/JSDCLOUD-17210)): agents "don't know if they are making an internal or external facing comment and may accidentally have selected the wrong one." An AI agent posting through a generic comment call has the same problem. Meanwhile no single API call ranks open tickets across queues by SLA time left, so "what breaches next" is a queue walk plus a per-ticket SLA read, and Assets (CMDB) only knows what someone imported, so drift against your RMM shows up during an outage.

- `jira-service-management-cli request note SD-123 "..."` - always posts public=false and errors if JSM stores the comment as public.
- `jira-service-management-cli request timeline SD-123` - every comment labeled PUBLIC or INTERNAL, before you reply.
- `jira-service-management-cli breaches --within 2h` - every queue walked once, tickets de-duplicated and ranked by SLA time remaining.
- `jira-service-management-cli roster 3` - client staff in no organization or in two, and organizations not attached to the desk.
- `jira-service-management-cli cmdb reconcile --type Laptop --file rmm.csv --key Name` - Assets diffed against an RMM or PSA export.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Jira Service Management MCP server runs locally, but it speaks HTTP natively: set `JIRA_SERVICE_MANAGEMENT_MCP_HTTP_TOKEN` to a bearer token, start it with `jira-service-management-mcp --transport http` (it serves `http://127.0.0.1:7777/mcp` by default) and put that endpoint behind an HTTPS tunnel or your own reverse proxy. No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Jira Service Management credentials once.

### Is my Jira Service Management data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Does this work with Jira Service Management Data Center or Server?

No. It targets Jira Service Management Cloud (your-site.atlassian.net). Data Center and Server use different hosts and authentication, and this connector does not call them.

### Do I need JSM Premium for Assets (CMDB)?

Only for the Assets commands. Assets is part of JSM Premium and Enterprise. Queues, SLAs, requests, comments, customers and organizations work on any JSM Cloud plan. For Assets, run `jira-service-management-cli cmdb discover --save` once: it finds your site's cloudId and Assets workspaceId and stores them in the config file.

### Will an internal note ever reach the customer?

Use `request note` for anything internal: it always posts with public=false, and if JSM reports the stored comment as public it exits with an error so the agent knows. `request comment add` is the customer-reply command: pass `--public` for a customer-visible reply. It sends no public field when you omit the flag, so never use it for internal notes. Read `request timeline` before replying: it labels every comment PUBLIC or INTERNAL.

### Will this hit my Atlassian API rate limits?

Live commands make normal API calls and pace themselves to the rate-limit headers Atlassian returns (`--rate-limit` overrides it). `breaches` reads each queue and then each ticket's SLA, so scope it with `--desk` and `--queue` on large sites. `sla-report`, `client` and `search` read the local copy after a `sync` and make no API calls.


### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `breaches`, `request timeline`, `servicedesk queue list-issues`, `sla-report`, `roster`, `client`, `cmdb tickets`, `object find-by-aql` | Allow |
| Write (routine) | `request note`, `request comment add`, `request transition perform`, `request create`, `request approval answer`, `object create` / `update` | Preview with `--dry-run`, then a reviewed write |
| Customers and organizations | `customer create`, `servicedesk customer add` / `invite` / `remove`, `organization create`, `organization user add` / `remove`, `customer revoke-portal-only-access-for-user` | Human-approved: these change who can raise tickets for a client |
| Destructive / config | `organization delete`, `object delete`, `objectschema delete`, `objecttype delete`, `servicedesk requesttype delete`, `importsource token generate`, `api-import` | Human-in-the-loop only |

Writes send immediately: `--dry-run` is an opt-in preview, not a default. Set `JIRA_SERVICE_MANAGEMENT_MCP_READ_ONLY=1` in the MCP server's environment and it drops `request_note`, `sync`, `import`, `export`, `workflow_archive`, `learnings_forget`, `learnings_reject` and `cmdb_reconcile` (which reads a local file); its generic `execute` tool refuses any non-GET API call, and `cmdb_discover` refuses `--save`. The local learning tools (`teach`, `teach_*`, `playbook_amend`, `learnings_confirm`) stay: they write only to the CLI's own local store, never to your site.

The strongest control is the **Atlassian account behind the API token** - the CLI can only do what that account is permitted to do, so create the token as an agent with the access you want the AI to have. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Jira Service Management API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-16._
