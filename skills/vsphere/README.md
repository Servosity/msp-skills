# VMware + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the VMware
> API. Not affiliated with, endorsed by, or sponsored by Broadcom Inc.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/vsphere/">
    <img src="../../docs/assets/video/vsphere/animated-og.gif" alt="VMware vSphere demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/vsphere/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Read-only vCenter inventory for migration planning: blockers, snapshots, per-core licence exposure and Proxmox mapping, as JSON an agent can use. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the VMware skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `vsphere-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, host `vsphere-mcp` over HTTPS with `VSPHERE_MCP_HTTP_TOKEN` set, then register it as a Developer Mode connector if your connector settings can send the bearer token. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `vsphere-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, run `vsphere-mcp --transport http` with `VSPHERE_MCP_HTTP_TOKEN` set, on an HTTPS host; every request sends that token. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not available in this build.** Copilot Studio takes a remote server only, which needs the release with token-protected HTTP mode. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote only, which needs the release with token-protected HTTP mode. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download VMware MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/vsphere-v0.1.2/vsphere-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every VMware release on the [releases page](https://github.com/servosity/msp-skills/releases?q=vsphere).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install vsphere@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the VMware Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.ps1 | iex`. Then authenticate per the README and run `vsphere-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.sh)
```

The installer drops both `vsphere-cli` (the CLI) and `vsphere-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
vsphere-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/vsphere/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update vsphere@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/vsphere --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/vsphere --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `vsphere-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the vsphere skill from https://github.com/servosity/msp-skills/tree/main/skills/vsphere. The skill defines how its required CLI (`vsphere-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (a vCenter SSO user with the Read-Only role is enough; `VSPHERE_TLS_FINGERPRINT` only for a self-signed certificate):

```bash
VSPHERE_BASE_URL=https://<vcenter> VSPHERE_USERNAME=<user> VSPHERE_PASSWORD=<password> vsphere-cli doctor
```

On Windows (PowerShell):

```powershell
$env:VSPHERE_BASE_URL = "https://<vcenter>"
$env:VSPHERE_USERNAME = "<user>"
$env:VSPHERE_PASSWORD = "<password>"
vsphere-cli doctor
```

`doctor` checks config, paths and API reachability, then verifies the credential by logging in and reading its own session (GET /api/session): `valid`, `invalid (HTTP 401)`, or an ERROR that names VSPHERE_BASE_URL when the host is not a vCenter Server.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| Which VMs are blocked from migrating, and what is blocking each one? | `vsphere-cli migration blockers --severity block --agent` |
| Which snapshots are older than a week, and how much delta disk does each of those VMs carry? | `vsphere-cli snapshots audit --older-than 7d --agent` |
| How many licensable cores does this vCenter have under the 16-core-per-CPU minimum? | `vsphere-cli licence exposure --agent` |
| How much vCPU, RAM and storage does the Proxmox target need? | `vsphere-cli migration capacity --powered-on-only --agent` |
| Which Proxmox VE settings does this VM map to? | `vsphere-cli migration proxmox-map --vm vm-1021 --agent` |
| Can I get an RVTools-format export without a Windows machine? | `vsphere-cli rvtools export --out ./acme-rvtools` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most VMware integrations and MCP servers proxy each question into a live API call. That's fine for one record. It falls short when the question is "which of this client's 400 VMs can move to Proxmox this weekend, and what will Broadcom charge for the rest?"

This skill can sync VMware lists into a **local SQLite mirror** with full-text search. The compound commands (`migration blockers`, `snapshots audit`, `licence exposure`, `migration proxmox-map`) read vCenter live through both APIs and join VMs, hosts, hardware, snapshots and disk layouts into one answer, so the AI sees the answer, not the raw data - work a stateless API wrapper can't do. They need a reachable vCenter and refuse `--data-source local`.

## The pain this closes

Broadcom's per-core subscriptions multiplied VMware bills for many MSPs and their clients ([ServeTheHome](https://www.servethehome.com/vmware-vcsp-customers-seeing-10x-or-more-cost-increases-under-broadcom/) reported 10x or more for cloud service providers). Every client with a vCenter now asks what staying costs and what leaving takes, and answering still means RVTools on a Windows jump box and an afternoon of spreadsheet work.

- `vsphere-cli migration blockers --severity block --agent` - every VM that cannot move yet, and why.
- `vsphere-cli licence exposure --agent` - licensable cores under the 16-core-per-CPU minimum.
- `vsphere-cli snapshots audit --older-than 7d --agent` - forgotten snapshots before they stall a cutover.
- `vsphere-cli rvtools export --out ./acme-rvtools` - the RVTools CSV layout without Windows.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The VMware vSphere MCP server runs locally over stdio by default. Its HTTP mode (`vsphere-mcp --transport http`) serves `http://127.0.0.1:7777/mcp`, refuses to start until `VSPHERE_MCP_HTTP_TOKEN` is set, requires `Authorization: Bearer <token>` on every request, and needs `--tls-cert`/`--tls-key` for any other address. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your VMware credentials once.

### Is my VMware data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Does it change anything in vCenter?

No. Every vCenter call is a read. A Read-Only role on the vCenter SSO user is enough.

### Which vCenter versions does it support?

Inventory reads need vCenter 7.0 Update 2 or later. Snapshots, licence exposure, host hardware and capacity need vCenter 8.0 Update 1 or later; on older vCenters those checks are reported as unknown, never as a pass.

### Our vCenter uses a self-signed certificate. Will it connect?

Yes. Set VSPHERE_TLS_FINGERPRINT to the certificate's SHA-256 fingerprint and the CLI accepts only that certificate. Without it, normal certificate checks apply.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `vm list`, `migration blockers`, `snapshots audit`, `licence exposure`, `migration capacity`, `migration proxmox-map`, `session`, `doctor` | Allow |
| Write (local files only) | `sync`, `export --output`, `rvtools export --out`, `profile save`, `teach` | Allow; they write only on your machine, never to vCenter |
| Credential | `auth set-credentials`, `auth logout` | Human-in-the-loop only |
| Destructive (local data) | `local-data purge --yes`, `learnings forget`, `learnings purge` | Human-in-the-loop only; local-data purge previews unless --yes |

The strongest control is the **scope you grant the VMware credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the VMware API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-10-28._
