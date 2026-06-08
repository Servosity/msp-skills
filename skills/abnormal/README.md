# Abnormal Security + AI - for ChatGPT, Claude, GitHub Copilot, Microsoft 365 Copilot, Gemini, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the Abnormal Security
> API. Not affiliated with, endorsed by, or sponsored by Abnormal Security Corporation.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/abnormal/">
    <img src="../../docs/assets/video/abnormal/animated-og.gif" alt="Abnormal Security demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/abnormal/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

The full Abnormal Security REST API in your terminal and your agents - with a local threat store, ranked SOC triage, blocking remediation, and one-shot client reporting. Works with the AI you already use - **ChatGPT** (Plus/Pro+), **Claude Desktop**, **Codex**, **Claude Code**, **Claude Cowork**, and **GitHub Copilot** - plus **Microsoft 365 Copilot / Copilot Studio** and **Google Gemini** via the remote path. Free, open source, runs on your laptop. Built for MSP owners. No code required.

## Works with your agent

The six agents MSP owners actually use (self-serve, works today):

| Your AI agent | How to install the Abnormal Security skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `abnormal-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `abnormal-mcp` over HTTPS, register as a Developer Mode connector. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Code** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |
| **GitHub Copilot** (VS Code) | Run installer, add `abnormal-mcp` to `mcp.json` under the `servers` key, then pick **Agent** mode. |

For ChatGPT, the Abnormal Security MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

### Also for the Microsoft and Google stacks

Big install base, but an honest heads-up: these are the **remote / enterprise** path, not the local binary you just installed.

| Agent | What it takes |
| --- | --- |
| **Microsoft 365 Copilot / Copilot Studio** | **Not self-serve.** Host `abnormal-mcp` over HTTPS, then wire it into Copilot Studio (**Tools > Add a tool > Model Context Protocol > Server URL**) or a declarative agent. Needs a Copilot Studio license + tenant admin. See [mcp-install.md](./mcp-install.md). |
| **Google Gemini** | **Gemini CLI** is local - same as Claude Code. The **Gemini app** is remote - same HTTPS path as ChatGPT. See [mcp-install.md](./mcp-install.md). |

> **Skill-native agents (also covered):** [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw) read this skill's `SKILL.md` directly *and* speak MCP - see their install sections below. Also works with Cursor, Windsurf, Cline, Continue.dev, and Zed via MCP. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download Abnormal Security MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/abnormal-v0.1.0/abnormal-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every Abnormal Security release on the [releases page](https://github.com/servosity/msp-skills/releases?q=abnormal).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install abnormal@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the Abnormal Security Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.ps1 | iex`. Then authenticate per the README and run `abnormal-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.sh)
```

The installer drops both `abnormal-cli` (the CLI) and `abnormal-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
abnormal-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/abnormal/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update abnormal@msp-skills`.

### Add to Claude Desktop, GitHub Copilot, Gemini CLI, Microsoft 365 Copilot, or another MCP client

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up - one section per agent, including the GitHub Copilot `servers` key and the remote Microsoft 365 Copilot / Copilot Studio path. Claude Desktop's Settings > Extensions panel is the simplest path; the MCP config block (for users who prefer editing JSON) is documented in mcp-install.md.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/abnormal --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/abnormal --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `abnormal-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the abnormal skill from https://github.com/servosity/msp-skills/tree/main/skills/abnormal. The skill defines how its required CLI (`abnormal-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Set the credentials the CLI needs (from your Abnormal Security portal):

```bash
ABNORMAL_API_TOKEN=<value> abnormal-cli doctor
```

`doctor` confirms the credentials work before you run anything that touches data.


## What this skill does

| Question your MSP keeps asking | Command |
| --- | --- |
| What new, unremediated email threats need attention right now? | `abnormal-cli triage --since 24h --top 20` |
| Pull a client-ready security report for the quarter | `abnormal-cli report-snapshot --since 90d --csv` |
| What is the account-takeover risk picture for this employee? | `abnormal-cli employee-risk "vip@acme.com"` |
| Is this vendor showing email-compromise signs? | `abnormal-cli vendor-risk "acme-supplies.com"` |
| Remediate a threat and block until it actually completes | `abnormal-cli remediate-watch <threat\|case> <id>` |
| List the latest Abnormal cases | `abnormal-cli cases retrieve --all` |
| How many attacks did we stop this week? | `abnormal-cli aggregations attack-stopped-retrieve` |
| Find threats from a spoofed sender | `abnormal-cli threats retrieve --sender "ceo@spoofed.com"` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most Abnormal Security integrations and MCP servers proxy each question into a live API call. That's fine for one record. It dies at scale, when you're asking how many attacks you stopped across a 90-day client report, or which employees were impersonated last quarter, or every still-unremediated threat from the overnight feed.

This skill syncs Abnormal Security into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `triage`, `employee-risk`, and `vendor-risk` join across threats, cases, employee logins, and vendor activity in a single call - work a stateless API wrapper can't do.

## The pain this closes

On r/msp, email-security threads circle the same two complaints: alert fatigue (analysts living in yet another portal, scrolling a threat log with no ranked queue) and the quarterly grind of turning a security dashboard into a client-ready report by hand. The work that matters - what's new, what's unremediated, what to tell the client - gets buried under clicking.

This skill maps that pain to commands:

- **`triage --since 24h --top 20`** - ranked queue of the newest, highest-severity, still-unremediated threats, so a shift starts on what matters.
- **`report-snapshot --since 90d --csv`** - attacks seen, attacks stopped, impersonation breakdowns, and trending attacks in one client-ready table.
- **`employee-risk "vip@acme.com"`** - one account-takeover picture per employee: profile, Genome identity, 30-day logins, and open cases naming them.
- **`vendor-risk "acme-supplies.com"`** - vendor-email-compromise picture: details, recent activity, and open vendor cases.
- **`remediate-watch <threat|case> <id>`** - remediate and block until Abnormal reports a terminal state, with a typed exit code receipt.

See [pain-point.md](./pain-point.md) for the longer narrative.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Abnormal Security MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md).

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Abnormal Security credentials once.

### Is my Abnormal Security data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my Abnormal API rate limits?

It doesn't have to. A `sync` pulls your tenant into a local SQLite mirror once, then `triage`, `search`, and reporting answer from disk - so repeat questions never touch the API. For live calls you can cap throughput with `--rate-limit` and page large pulls.

### Do I need to be an Abnormal partner or customer?

You need API access to an Abnormal Security tenant and a token generated from the portal's integration settings. The REST API doesn't require a separate partner tier - just a credential scoped to what you want the skill to do.

### Can it actually remediate, or only read?

It can remediate threats and delete or move malicious messages through the remediation commands, and `remediate-watch` blocks until Abnormal reports the action reached a terminal state. Those actions are gated behind a human-in-the-loop policy - see the safety model below.

### Will it replace the Abnormal portal?

No. Detection tuning, policy, and configuration stay in the portal. This is a read-first, action-on-approval surface for your terminal and your agents, not a replacement UI.

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| **Read** | `triage`, `report-snapshot`, `threats retrieve`, `cases retrieve`, `vendors`, `employee-risk`, `vendor-risk`, `soar` (API-token metadata only) | Allow |
| **Write (routine)** | `cases create` (update case status), `detection360 reports-create`, `api-resources resources-create-create` / `resources-update-partial-update` | Preview with `--dry-run`, then a reviewed write |
| **Remediation / destructive** | `threats create` (remediate/unremediate), `email-search search-remediate-create` (delete/move mail), `remediate-watch`, `import` | Human-in-the-loop only |

The strongest control is the **scope you grant the Abnormal Security credentials** - the CLI can only do what the credentials are permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the Abnormal Security API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-06-07._
