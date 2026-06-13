# CrowdStrike + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the CrowdStrike
> API. Not affiliated with, endorsed by, or sponsored by CrowdStrike, Inc..

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/crowdstrike/">
    <img src="../../docs/assets/video/crowdstrike/animated-og.gif" alt="CrowdStrike Falcon demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/crowdstrike/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every CrowdStrike Falcon MSP operation, plus a Flight-Control-aware local store that answers fleet-wide questions across all your tenants at once  -  something no other Falcon tool (including the official MCP server) does. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the CrowdStrike skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `crowdstrike-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `crowdstrike-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `crowdstrike-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the CrowdStrike MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `crowdstrike-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download CrowdStrike MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/crowdstrike-v0.1.0/crowdstrike-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every CrowdStrike release on the [releases page](https://github.com/servosity/msp-skills/releases?q=crowdstrike).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install crowdstrike@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the CrowdStrike Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/crowdstrike/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/crowdstrike/install.ps1 | iex`. Then authenticate per the README and run `crowdstrike-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/crowdstrike/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/crowdstrike/install.sh)
```

The installer drops both `crowdstrike-cli` (the CLI) and `crowdstrike-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
crowdstrike-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/crowdstrike/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/crowdstrike/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update crowdstrike@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/crowdstrike --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/crowdstrike --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `crowdstrike-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the crowdstrike skill from https://github.com/servosity/msp-skills/tree/main/skills/crowdstrike. The skill defines how its required CLI (`crowdstrike-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your CrowdStrike portal):

```bash
CROWDSTRIKE_OAUTH_SCOPE=<value> FALCON_CLIENT_ID=<value> FALCON_CLIENT_SECRET=<value> crowdstrike-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What should I triage first across all my client tenants right now? | `crowdstrike-cli fleet alerts --status new` |
| Rank the critical vulnerabilities across every tenant? | `crowdstrike-cli fleet vulns --severity critical` |
| Which hosts haven't reported a sensor heartbeat lately? | `crowdstrike-cli fleet stale --days 14` |
| Give me one posture scorecard per tenant for the QBR deck? | `crowdstrike-cli fleet scorecard` |
| Which tenants are under-protected versus my prevention-policy baseline? | `crowdstrike-cli fleet policy-drift` |
| Which single fix clears the most hosts and tenants? | `crowdstrike-cli fleet remediate --severity critical` |
| Which tenants got worse since the last sync? | `crowdstrike-cli fleet trend` |
| Map every child CID, CID group, and role grant across my MSSP? | `crowdstrike-cli fleet tenants` |
| Pull every child tenant's Falcon data into a local mirror? | `crowdstrike-cli fleet sync --all-cids` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most CrowdStrike integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which of my 40 client tenants have critical, actively-exploited vulnerabilities still open" and the answer means paging Spotlight CID by CID through Flight Control.

This skill syncs every child CID into one **local SQLite store keyed by CID** with full-text search. Cross-tenant questions become a single offline query: instant, offline, and the AI sees the answer, not the raw data. Commands like `fleet scorecard`, `fleet policy-drift`, and `fleet remediate` join hosts, alerts, vulnerabilities, and prevention policies across every tenant at once - and time-aware views like `fleet trend` diff against snapshotted history, work a stateless API wrapper can't do.

## The pain this closes

The Falcon console scopes to one CID at a time. Run CrowdStrike across a book of client tenants and every cross-client question - who has the worst open detections, whose sensors went dark, which tenant climbed in critical vulns after Patch Tuesday - means switching CID through Flight Control and re-reading the same screens tenant by tenant. MSPs raise this single-pane gap on r/msp and in the CrowdStrike community repeatedly: Flight Control gives you parent-level child management, but no one view ranks every CID's alerts, vulnerabilities, or sensor health side by side.

This skill closes that gap by syncing every CID into one local store and answering the book-wide questions directly:

- `crowdstrike-cli fleet alerts --status new` - one severity-sorted detection queue across every tenant
- `crowdstrike-cli fleet vulns --severity critical` - rank Spotlight criticals across the whole fleet
- `crowdstrike-cli fleet stale --days 14` - every silent sensor, all tenants, one sweep
- `crowdstrike-cli fleet scorecard` - per-CID posture board for the QBR
- `crowdstrike-cli fleet policy-drift` - which tenants fall short of your prevention-policy baseline

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The CrowdStrike MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your CrowdStrike credentials once.

### Is my CrowdStrike data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Do I need a parent (MSSP) CID for the fleet commands?

Yes. The cross-tenant `fleet` commands need a parent-CID Falcon API client with Flight Control (MSSP) scope so `fleet sync` can discover and pull every child CID. Without it, sync degrades gracefully to the single authenticated CID and the rollups simply cover that one tenant. The per-CID commands (`alerts`, `devices`, `spotlight`, `policy`) work against any single tenant's client.

### Will this hit my CrowdStrike API rate limits?

The local store exists so reads stop hitting the API. After `fleet sync`, every cross-tenant view runs against local SQLite with zero API calls, and live calls respect a `--rate-limit` throttle. The `fleet trend` and `fleet policy-drift` analytics need at least two syncs to have history to diff.

### What scopes does the Falcon API client need?

Read scopes for the entities you query - Alerts, Hosts, Spotlight Vulnerabilities, Prevention Policies, and a parent-CID Flight Control / MSSP read scope for the `fleet` commands. Add write scopes (Hosts write, Prevention Policies write) only if you intend to contain hosts or edit policies. Mint a read-only client for reporting and keep write scope for the rare case you need it - the client's scopes are the real permission boundary.

### Does it replace the Falcon console?

No. The console stays best for hunting, RTR sessions, policy authoring, and the interactive response workflow inside one CID. This skill adds cross-tenant queries and scriptable actions to your AI agent so you stop scoping into each CID to answer book-wide questions.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet scorecard`, `fleet alerts --status new`, `fleet vulns --severity critical`, `fleet stale --days 14`, `alerts post-combined-v1`, `devices query-by-filter`, `spotlight query-vulnerabilities`, `mssp query-children`, `search "<query>"` | Allow |
| Write (routine) | `alerts patch-entities-v3` (update/assign alerts), `devices perform-action-v2` (contain/lift, or delete/restore a host), `devices update-tags`, `policy update-prevention-policies`, `policy create-prevention-policies` | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | `devices delete-host-groups`, `policy delete-prevention-policies`, `policy set-prevention-policies-precedence`, `mssp delete-cidgroups`, `mssp delete-user-groups`, `mssp deleted-roles` | Human-in-the-loop only |

The strongest control is the **scope you grant the CrowdStrike credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the CrowdStrike API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-06._
