# Axcient + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Axcient
> API. Not affiliated with, endorsed by, or sponsored by Axcient, Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/axcient/">
    <img src="../../docs/assets/video/axcient/animated-og.gif" alt="Axcient x360Recover demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/axcient/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

<!-- first-party-note:start -->
> **Also from Servosity.** Backup & DR is Servosity's own field - the first-party [Servosity connector](../servosity) brings this same fleet-wide, local-mirror approach (fleet attention, stale backups, restores, QBR reporting) to Servosity Backup and DR.
<!-- first-party-note:end -->

Every x360Recover endpoint, plus the fleet-wide backup-health answers the API alone can't give - offline, joined, and agent-ready. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Axcient skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `axcient-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `axcient-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `axcient-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Axcient MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `axcient-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Axcient MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/axcient-v0.1.0/axcient-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Axcient release on the [releases page](https://github.com/servosity/msp-skills/releases?q=axcient).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install axcient@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Axcient Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/axcient/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/axcient/install.ps1 | iex`. Then authenticate per the README and run `axcient-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/axcient/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/axcient/install.sh)
```

The installer drops both `axcient-cli` (the CLI) and `axcient-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
axcient-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/axcient/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/axcient/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update axcient@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/axcient --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/axcient --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `axcient-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the axcient skill from https://github.com/servosity/msp-skills/tree/main/skills/axcient. The skill defines how its required CLI (`axcient-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Axcient portal):

```bash
AXCIENT_API_KEY=<value> axcient-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Whose backups failed or went stale across every client last night? | `axcient-cli health` |
| Give me one row per client: devices total, failing, stale, RPO-breach, AutoVerify-fail | `axcient-cli client-rollup` |
| Which devices are past their recovery-point objective, grouped by client? | `axcient-cli rpo --hours 24` |
| Produce backup-compliance evidence for one client (restore-point age + AutoVerify + RPO verdict) | `axcient-cli compliance --client 42 --hours 24 --csv` |
| What does each client consume for invoice reconciliation this month? | `axcient-cli billing --csv` |
| Which devices does each appliance protect, and what state are those backups in? | `axcient-cli appliance-map` |
| Find everything matching a client or device name | `axcient-cli search "Acme Corp"` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Axcient integrations and MCP servers proxy each question into a live API call. That's fine for one device. It dies at scale, when you're asking "which of my 200 protected devices across 40 clients breached RPO last night" - because the x360Recover API has no rollup endpoint and won't even join a device back to its client for you, so the wrapper loops per appliance, then per device, then per job.

This skill syncs Axcient into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `health`, `client-rollup`, and `compliance` join across appliances, devices, backup jobs, restore points, and AutoVerify results - the client-to-device correlation the raw API omits - work a stateless API wrapper can't do.

## The pain this closes

The x360Recover Public API is built around per-entity endpoints - one call for appliances, another for a device, another for that device's jobs - and it does not hand you the client-to-device mapping directly. So "whose backups failed last night," the question every MSP asks each morning, has no single fleet-wide endpoint: you walk each device by hand or click through the portal one client at a time. And a job that "succeeded" can still be out of compliance - the newest restore point is hours stale, or AutoVerify never booted the image - a trap MSPs on r/msp keep describing, where a recovery point turns out not to be there only when a restore is actually needed.

This skill turns those into one command each:

- **`health`** - every device fleet-wide whose latest job failed or went stale, grouped by client.
- **`rpo`** - devices whose newest restore point is older than your recovery-point objective.
- **`compliance`** - exportable per-device evidence pairing restore-point age with AutoVerify boot-proof and an RPO verdict.
- **`client-rollup`** - one row per client: devices total, failing, stale, RPO-breach, AutoVerify-fail.
- **`billing`** - protected-system counts and storage per client for month-end reconciliation.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Axcient MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Axcient credentials once.

### Is my Axcient data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my Axcient API rate limits?

The local mirror exists so reads stop hitting the API. After the first `sync`, the fleet views (`health`, `client-rollup`, `rpo`, `compliance`, `billing`, `appliance-map`) run against local SQLite with **zero API calls**. Live calls respect a `--rate-limit` throttle, and sync is incremental - it only fetches what changed since the last checkpoint.

### What kind of Axcient credential do I need?

An organization-scoped API key created in the x360Portal (**Settings > API Keys**, admin role required). The CLI sends it as the `X-Api-Key` header and can only see what that key is scoped to. Set it as `AXCIENT_API_KEY`; nothing is written to disk.

### Can I try it without a real tenant?

Yes. Axcient hosts a public mock server - set `AXCIENT_BASE_URL=https://ax-pub-recover.wiremockapi.cloud/x360recover` with any non-empty `AXCIENT_API_KEY` and the whole CLI runs against fixtures, no real credentials needed.

### Does this cover x360Sync or x360Cloud?

No. This skill is the **x360Recover** (BCDR) public API only - vaults, appliances, devices, jobs, restore points, AutoVerify, and usage. x360Sync and x360Cloud are separate products with separate APIs.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `health`, `client-rollup`, `rpo`, `compliance`, `billing`, `appliance-map`, `clients get`, `vault get`, `organization`, `search "Acme Corp"` | Allow |
| Write (routine) | `vault threshold set-by-vault-id <vault_id> --threshold 60`, `import <resource> --input data.jsonl` (bulk - one create/upsert POST per record) | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | `client vault get-d2c-agent-token-by-client-and-ids <client_id> <vault_id>` (mints a direct-to-cloud agent install token) | Human-in-the-loop only |

The strongest control is the **scope you grant the Axcient credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Axcient API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-07._
