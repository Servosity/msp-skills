# ThreatDown + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the ThreatDown
> API. Not affiliated with, endorsed by, or sponsored by Malwarebytes Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/threatdown/">
    <img src="../../docs/assets/video/threatdown/animated-og.gif" alt="ThreatDown demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/threatdown/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every ThreatDown OneView API, plus fleet-wide stale-endpoint, health and seat reports across all your sites, and guarded isolate/scan/restore jobs. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the ThreatDown skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `threatdown-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `threatdown-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `threatdown-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `THREATDOWN_MCP_HTTP_TOKEN=<a-bearer-token> threatdown-mcp --transport http` and expose that behind HTTPS - no bridge package is needed. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `threatdown-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download ThreatDown MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/threatdown-v0.1.0/threatdown-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every ThreatDown release on the [releases page](https://github.com/servosity/msp-skills/releases?q=threatdown).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install threatdown@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the ThreatDown Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.ps1 | iex`. Then authenticate per the README and run `threatdown-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.sh)
```

The installer drops both `threatdown-cli` (the CLI) and `threatdown-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
threatdown-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update threatdown@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/threatdown --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/threatdown --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `threatdown-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the threatdown skill from https://github.com/servosity/msp-skills/tree/main/skills/threatdown. The skill defines how its required CLI (`threatdown-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your ThreatDown portal):

```bash
THREATDOWN_CLIENT_ID=<value> THREATDOWN_CLIENT_SECRET=<value> threatdown-cli doctor
```

On a fresh install run `threatdown-cli auth login` first: it exchanges the client ID and secret for a token, and a wrong secret fails right there with HTTP 401. After that, `doctor` shows `Auth: configured` and probes the credential with an authenticated call to OneView's account endpoint (its `Credentials` row); before a token is cached it reads `Auth: not configured` and skips that probe.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which endpoints haven't checked in for 24 hours, across every site? | `threatdown-cli fleet stale --hours 24 --agent` |
| Which customer site needs attention first? | `threatdown-cli fleet health --agent` |
| Which sites are over- or under-allocated on seats, and which subscriptions are expiring? | `threatdown-cli seats reconcile --agent` |
| Which scans or isolations never ran? | `threatdown-cli respond pending --since 7d --agent` |
| Which threats hit more than one customer this month? | `threatdown-cli threats spread --since 30d --min-sites 2 --agent` |
| Which infected or suspicious machine should I contain next? | `threatdown-cli threats queue --since 7d --agent` |
| Everything about one customer while they're on the phone | `threatdown-cli customer report "Acme Corp" --agent` |
| Isolate a machine, with a preview first | `threatdown-cli respond isolate --host LAPTOP-042 --agent` |

`threats spread`, `threats queue` and `customer report` read the local copy, so run `threatdown-cli fleet sync` first (and again when you want fresh numbers). The `respond` commands send nothing until you add `--confirm`.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

We found no other MCP server for ThreatDown OneView on GitHub as of October 2026. The integrations that do exist, such as the [Sumo Logic](https://help.sumologic.com/docs/platform-services/automation-service/app-central/integrations/threatdown-oneview) and [Datadog (Crest Data)](https://docs.datadoghq.com/integrations/crest-data-systems-threatdown-oneview/) apps, move OneView data into a SIEM or monitoring tool. A question like "which threats hit more than one of my 40 customers this month" is still a per-site export and a spreadsheet.

This skill resolves every site's account ID for you and syncs sites, endpoints, detections and jobs into a **local SQLite mirror** with full-text search (`search --data-source local`). Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `threats spread`, `threats queue` and `customer report` join detections, endpoints, jobs and sites - work a stateless API wrapper can't do. ThreatDown's own AI features, such as the AI-assisted Security Advisor and AI Detection & Response, work inside the ThreatDown console; this skill complements them by bringing every OneView site into the agent you already use.

## The pain this closes

ThreatDown OneView answers one site at a time. The Monday question - "which machines went dark since Friday, across all my customers?" - has no single page. ThreatDown's own answer is a script: the support article **"API Example: Get endpoints not seen for more than 24 hours"** (article 35082574965011) ships PowerShell that lists every site, collects each account ID, pages the endpoint search and prints hours since last seen. The dashboard's **"Last sync 7+ days ago"** widget catches a machine only after a week of silence. Site health, seat counts and job outcomes have the same per-site shape.

- `threatdown-cli fleet stale --hours 24` - the vendor's PowerShell example as one command across every site.
- `threatdown-cli fleet health` - one ranked row per customer site, worst first.
- `threatdown-cli seats reconcile` - allocations against deployed and active machines, with expiring subscriptions flagged.
- `threatdown-cli respond pending --since 7d` - the scans, isolations and RMM-issued jobs that never ran.
- `threatdown-cli respond isolate --host LAPTOP-042` - finds the machine across every site and previews before sending.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The ThreatDown MCP server runs locally, but it speaks HTTP natively: start it with `THREATDOWN_MCP_HTTP_TOKEN=<a-bearer-token> threatdown-mcp --transport http` (it serves `http://127.0.0.1:7777/mcp` by default and refuses to start without that token; callers send it as `Authorization: Bearer`) and put that endpoint behind an HTTPS tunnel or your own reverse proxy. No bridge package is involved. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your ThreatDown credentials once.

### Is my ThreatDown data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Is this for ThreatDown OneView or ThreatDown Nebula?

OneView, the multi-site console ThreatDown (Malwarebytes) gives MSPs. It talks to the OneView API at `api.threatdown.com/oneview` and works across every site you manage. A single-company Nebula console is not a OneView account; it uses a different API, which this skill does not cover.

### What credentials do I need?

An OAuth client ID and secret from OneView: open **Integrate** in the left menu, click **Add**, name the app and tick the scopes. ThreatDown says a Global or Site Administrator has to create it, and the secret is shown only once. `read` covers every report; the `respond` commands (isolate, restore, scan, reboot, update) also need `execute`, and other changes need `write`. Set `THREATDOWN_CLIENT_ID` and `THREATDOWN_CLIENT_SECRET`, then run `threatdown-cli doctor`. To request fewer scopes than the default `execute read write`, set `THREATDOWN_OAUTH_SCOPE` (for example `read`) and run `threatdown-cli auth login`.

### Can my AI isolate or scan client machines?

Only through commands you allow. The `respond` commands preview by default: they list the machines that would get the job and send nothing until you add `--confirm` (or the global `--yes`), and they refuse more than 25 endpoints unless you raise `--max-targets`. Other paths send immediately: `endpoint-jobs issue`, `endpoint-jobs bulk-issue`, `grid jobs-bulk`, `import endpoint_jobs`, and the MCP server's generic `threatdown_execute` tool. For a reporting agent, create the OAuth client with `read` only, or set `THREATDOWN_MCP_READ_ONLY=1` so the MCP server drops its write tools and refuses every non-GET call, and `endpoints diag-log`, through `threatdown_execute`.

### Has it been tested against a live OneView tenant?

Not yet. Every command was built from ThreatDown's published OneView API specification and tested against a local mock of that API, not a real tenant. If you run it against your own OneView account, tell us what worked or broke on the [issue tracker](https://github.com/Servosity/msp-skills/issues) or at a Build Session; a real MSP run is what earns the live-verified badge.

### Will this replace the OneView console?

No. OneView stays where you set policies, manage subscriptions and deploy agents. This skill answers cross-site reporting and triage questions from your AI agent or terminal, and sends response jobs: the `respond` commands preview first, while the raw job commands send immediately.


### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `fleet stale`, `fleet health`, `seats reconcile`, `respond pending`, `threats spread`, `threats queue`, `customer report`, `fleet sync`, `search`, `tail` (it strips the secret fields), the `respond` commands without `--confirm`, every list / get / search / export command except the credential reads in the last row and `endpoints diag-log` | Allow |
| Endpoint actions | `respond isolate` / `restore` / `scan` / `reboot` / `update` with `--confirm`, `endpoint-jobs issue`, `endpoint-jobs bulk-issue`, `endpoint-jobs quarantine-bulk`, `endpoint-jobs parent-reissue`, `endpoint-jobs parent-children-reissue`, the `endpoint-jobs` cancels, `grid jobs-bulk`, `endpoints sa-remediate`, `suspicious-activity remediate`, `endpoints diag-log`, `import endpoint_jobs`, the MCP `threatdown_execute` tool on a jobs endpoint | Human-in-the-loop, explicit confirmation |
| Security config | `exclusions create` / `update`, `policies create` / `update` / `promote` and the other policy writes, `groups assign-endpoints`, firewall, content-filtering, device-control, app-block and AIDR rules, `schedules`, `webhooks` and `notifications` create / update, `syslog` and `email-protection` settings, and every other write not in the rows around it | Preview with `--dry-run`, then a human-approved write |
| Destructive | `endpoints delete` / `delete-bulk`, `grid endpoints-delete`, `sites delete`, `policies delete`, `exclusions delete`, `users delete`, `webhooks delete`, `notifications delete`, and every other `delete` command | Human-in-the-loop only, explicit confirmation |
| Credential and admin | Every `policies` read (including `secret-get` / `secret-get-v2`), `sites secret`, `installation-tokens search`, `installers`, `webhooks list` / `get`, `notifications list` / `get`, and `export` of policies or notifications (their output can carry tamper-protection passwords, enrollment credentials or webhook signing keys; the local mirror never stores those fields), `installation-tokens create` / `send` / `revoke`, `policies secret-set`, `users create` / `update`, `sites create` / `update`, `import` of users or sites, `subscriptions create` / `update`, the `email-protection` register, migrate and authorize commands | Human-in-the-loop only |


The strongest control is the **scope you tick on the OneView OAuth client** (`read`, `write`, `execute`) - the CLI can only do what that client is permitted to do. Set `THREATDOWN_MCP_READ_ONLY=1` to run the MCP server read-only. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Built from ThreatDown's published OneView API specification and tested against a local mock of that API; being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-14._
