# ConnectWise Manage + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the ConnectWise Manage
> API. Not affiliated with, endorsed by, or sponsored by ConnectWise, LLC.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/connectwise-manage/">
    <img src="../../docs/assets/video/connectwise-manage/animated-og.gif" alt="ConnectWise PSA (Manage) demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/connectwise-manage/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Every ConnectWise PSA workflow from the terminal  -  with a typed conditions query builder, offline SQLite sync, and cross-entity views (unbilled work, account 360, board triage) the PSA web UI can't give you. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the ConnectWise Manage skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `connectwise-manage-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `connectwise-manage-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `connectwise-manage-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the ConnectWise Manage MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `connectwise-manage-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download ConnectWise Manage MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/connectwise-manage-v0.1.0/connectwise-manage-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every ConnectWise Manage release on the [releases page](https://github.com/servosity/msp-skills/releases?q=connectwise-manage).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install connectwise-manage@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the ConnectWise Manage Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-manage/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-manage/install.ps1 | iex`. Then authenticate per the README and run `connectwise-manage-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-manage/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-manage/install.sh)
```

The installer drops both `connectwise-manage-cli` (the CLI) and `connectwise-manage-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
connectwise-manage-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-manage/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-manage/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update connectwise-manage@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/connectwise-manage --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/connectwise-manage --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `connectwise-manage-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the connectwise-manage skill from https://github.com/servosity/msp-skills/tree/main/skills/connectwise-manage. The skill defines how its required CLI (`connectwise-manage-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs - an **API Member** with public/private keys from your Manage instance (System > Members > API Members) plus a **clientId** from the [ConnectWise developer portal](https://developer.connectwise.com):

```bash
export CW_COMPANY_ID=<your company id>
export CW_PUBLIC_KEY=<api member public key>
export CW_PRIVATE_KEY=<api member private key>
export CW_CLIENT_ID=<clientid from developer.connectwise.com>
export CW_SITE=<region host, e.g. api-na.myconnectwise.net, or your on-prem host>
connectwise-manage-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data. The API Member's security role is the permission boundary - scope it to what you want the AI to reach.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which tickets did we touch this week that have zero time logged? | `connectwise-manage-cli unbilled --since 7d` |
| Which clients are about to blow through their block-hours agreement? | `connectwise-manage-cli agreement-burn --period 30d` |
| What does the Help Desk board look like right now - age, owner, priority? | `connectwise-manage-cli board "Help Desk"` |
| Which open tickets has nobody touched in five days? | `connectwise-manage-cli stale --days 5` |
| Who has bandwidth for the next ticket? | `connectwise-manage-cli workload` |
| Which tickets are sitting unassigned? | `connectwise-manage-cli board "Help Desk" --unassigned` |
| Everything about one client - contacts, agreements, configurations, open tickets? | `connectwise-manage-cli account AcmeCorp` |
| Write me a valid conditions filter for the API | `connectwise-manage-cli condition build --field board/name --op = --value "Help Desk"` |

Beyond the views: the full ConnectWise PSA REST surface (service, time, company, finance, project, sales, procurement, system) is exposed as typed subcommands - `service get-tickets`, `time post-entries`, `company get-companies` and hundreds more - every one also available as an MCP tool.

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most ConnectWise Manage integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking "which tickets did we close across every client this month with no time logged against them" - and every live call has to thread the strict conditions syntax, where one wrong quote returns silently empty results.

This skill syncs ConnectWise Manage into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `unbilled`, `account`, and `agreement-burn` join across tickets, time entries, companies, contacts, agreements, and configurations - work a stateless API wrapper can't do. And when you do need a live filtered query, `condition build` assembles a validated conditions expression so the API call is right the first time.

## The pain this closes

Unlogged time is direct revenue leakage. MSP consultancy Bering McKinley prices the cost plainly - "lost billable hours, lower profitability, bad decision-making caused by incomplete data, and unhappy clients" - and third-party reviews call time entry the most consistently reported ConnectWise pain point in the MSP community. Techs close tickets without entering time, and nobody notices until the invoice run, because Manage has no single view that joins closed tickets to their logged hours. The same export-to-Excel reflex shows up for agreement burn and client-360 questions: the data is in the PSA, but composing it across entities means portal screen after portal screen or a paid BI add-on.

What this skill does about it:

- `connectwise-manage-cli unbilled --since 7d` - tickets you touched or closed this week with zero (or under-threshold) hours logged. Run it before every billing cutoff.
- `connectwise-manage-cli agreement-burn --period 30d` - hours consumed vs allotment per agreement, with an over-limit flag. Spot the unprofitable client before they blow the block.
- `connectwise-manage-cli stale --days 5` - open tickets rotting with no update, oldest first. The daily board-hygiene pass.
- `connectwise-manage-cli workload` - open count and oldest age per tech, so the next ticket goes to whoever is lightest.
- `connectwise-manage-cli account AcmeCorp` - the five-screen client picture in one card.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The ConnectWise Manage MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your ConnectWise Manage credentials once.

### Is my ConnectWise Manage data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my ConnectWise API rate limits?

The local mirror exists so reads stop hitting the API. After the first sync, the cross-entity views (`unbilled`, `account`, `agreement-burn`, `board`, `stale`, `workload`) run against local SQLite with zero API calls. Live calls respect a `--rate-limit` throttle, and sync is incremental - it only fetches what changed since the last checkpoint.

### Is this for ConnectWise PSA or ConnectWise Manage?

Same product - ConnectWise renamed Manage to ConnectWise PSA. This skill targets the Manage REST API (`v4_6_release/apis/3.0`), which is the API both names refer to. It does **not** cover ConnectWise Automate - that's a separate product with a separate API.

### Does it work with on-premises ConnectWise Manage?

Yes. `CW_SITE` accepts your own server's hostname as well as the cloud region hosts; the CLI builds the standard `v4_6_release/apis/3.0` base URL either way.

### Will this replace my ConnectWise portal?

No - and it isn't trying to. The portal stays best for in-app workflows like drag-and-drop dispatch and invoice editing. This skill brings the PSA's data to whichever AI agent you already use, and answers the cross-entity questions no single portal screen composes.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `unbilled`, `account`, `agreement-burn`, `board`, `stale`, `workload`, `search`, any `get-*` | Allow |
| Write (routine) | `time post-entries`, `service post-tickets`, `service patch-tickets-by-id`, `company patch-companies-by-id` | Preview with `--dry-run` (opt-in, not a default), then a reviewed write |
| Destructive | `service delete-tickets-by-id`, `company delete-companies-by-id`, `company delete-configurations-bulk` | Human-in-the-loop only |
| Credential / security | `system post-members-by-member-identifier-tokens`, `company post-contacts-request-password` | Human-in-the-loop only |

Writes are **not gated in the binary by default** - `--dry-run` previews a request without sending, but it's a flag your agent must pass. Put the gate in your agent's policy: preview, show the exact command, get approval, then run.

The strongest control is the **scope you grant the ConnectWise Manage credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the ConnectWise Manage API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-05._
