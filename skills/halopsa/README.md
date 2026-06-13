# HaloPSA + AI - for Claude, ChatGPT, Codex, Cursor, and any agent that speaks MCP

> Unofficial. Community-built Claude Code Skill and MCP server for the HaloPSA, HaloITSM, and HaloCRM APIs. Not affiliated with, endorsed by, or sponsored by Halo Service Solutions Ltd. HaloPSA, HaloITSM, and HaloCRM are trademarks of Halo Service Solutions Ltd.

<!-- media:start -->
<p align="center">
  <a href="https://msp-skills.compoundingteams.com/skills/halopsa/">
    <img src="../../docs/assets/video/halopsa/animated-og.gif" alt="HaloPSA demo - animated preview" width="600">
  </a>
</p>
<p align="center"><sub>▶ <a href="https://msp-skills.compoundingteams.com/skills/halopsa/">Watch the 30-second demo</a> - demo data is simulated; every command shown exists in the real CLI.</sub></p>
<!-- media:end -->

Add **HaloPSA ticket triage, SLA-breach pre-emption, per-client situational awareness, and cross-client analytics** to the AI you already use - **Claude Code**, **Claude Desktop**, **ChatGPT** (Plus/Pro+), **Codex**, **Cursor**, **Windsurf**, **Cline**, **Continue**, **Gemini**, or **GitHub Copilot**. Free, open source, runs on your laptop. A local SQLite mirror means your AI can answer cross-client questions the live Halo API can't return in one shot - no rate-limit hits during QBR prep. Built for MSP owners. No code required.

## Works with your agent

The five agents MSP owners actually use:

| Your AI agent | How to install the HaloPSA skill |
| --- | --- |
| **Claude Desktop** | Run installer, then **Settings > Extensions** to register `halopsa-mcp` (no JSON editing). |
| **ChatGPT** (paid plans) | Run installer, expose `halopsa-mcp` over HTTPS, register as a Developer Mode connector. |
| **Claude Code** | Paste the install prompt below. |
| **Codex CLI** | Paste the install prompt below. |
| **Claude Cowork** | Paste the install prompt below. |

For ChatGPT, the HaloPSA MCP server is stdio - to use it with ChatGPT you expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

> **Also works with** Cursor, Windsurf, Cline, Continue.dev, Zed, GitHub Copilot, and Gemini CLI - plus [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](#install-for-openclaw), both via MCP and the pre-wired skill frontmatter. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

> **Run more than one agent?** Install across all 51+ supported agents in one command: `npx skills add Servosity/msp-skills@latest` (requires Node.js, then run the per-skill installer for the CLI/MCP binaries). See [docs/which-agent.md](../../docs/which-agent.md#install-across-all-your-agents-at-once).

## Install in 60 seconds

### Fastest for Claude Desktop - one-click `.mcpb`

[**Download HaloPSA MCP (.mcpb)**](https://github.com/servosity/msp-skills/releases/download/halopsa-v0.1.1/halopsa-mcp.mcpb) - then open **Claude Desktop > Settings > Extensions** and select the file. One click, no JSON, no shell. (Browse every HaloPSA release on the [releases page](https://github.com/servosity/msp-skills/releases?q=halopsa).)

Prefer the Claude Code plugin? Add the marketplace once, then install - works immediately, no directory listing required:

```
/plugin marketplace add Servosity/msp-skills
/plugin install halopsa@msp-skills
```

### Path A - paste one prompt into your AI agent (recommended)

Copy this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:

> Install the HaloPSA Skill and MCP server from Servosity/msp-skills in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.sh)`. If it uses Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.ps1 | iex`. Then authenticate with `halopsa-cli auth login` (Configuration > Integrations > Halo PSA API in your tenant gives you the credentials) and run `halopsa-cli --help` to explore.

The same prompt works in any agent that can run shell.

### Path B - run the installer yourself

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
```

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.sh)
```

The installer drops both `halopsa-cli` (the CLI) and `halopsa-mcp` (the MCP server) into your user bin path. Claude Code, Codex, and Cowork discover the Skill via `SKILL.md` in this directory; the binary is what the Skill actually drives.

Verify:

```bash
halopsa-cli --version
```

### Upgrade to the latest version

The installer always fetches the current release - re-run it to upgrade:

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
```

Claude Desktop `.mcpb` users: download the latest `.mcpb` (top of this section) and re-select it in **Settings > Extensions**. Claude Code plugin users: `/plugin update halopsa@msp-skills`.

### Add to Claude Desktop, Cursor, Windsurf, Cline, Continue, Gemini, or Copilot

After the installer runs, see **[mcp-install.md](./mcp-install.md)** and **[docs/which-agent.md](../../docs/which-agent.md)** for the per-agent wire-up. Claude Desktop's Settings > Extensions panel is the simplest path.

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/halopsa --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/halopsa --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `halopsa-mcp` server directly - same install path, same env vars.

### Install for OpenClaw

[OpenClaw](https://docs.openclaw.ai) is GA. Install the skill directly from this repo:

```bash
openclaw skills install git:Servosity/msp-skills/skills/halopsa@main
```

Or tell your OpenClaw agent (copy this):

> Install the halopsa skill from https://github.com/Servosity/msp-skills/tree/main/skills/halopsa. The `metadata.openclaw` frontmatter declares the required CLI (`halopsa-cli`) and env vars; run `openclaw doctor` if anything is missing.

`openclaw` also speaks MCP natively (see `openclaw mcp set` in the [OpenClaw MCP docs](https://docs.openclaw.ai/cli/mcp)) so you can wire `halopsa-mcp` as an MCP server alongside the skill.

### Authenticate

HaloPSA uses OAuth2 client_credentials. Create an API application in your tenant under **Configuration → Integrations → Halo PSA API** (Authentication Method: Client ID and Secret, Services), then:

```bash
HALOPSA_TENANT=<yourtenant> halopsa-cli auth login \
  --client-id <id> --client-secret <secret>
```

The CLI caches the access token and auto-refreshes before expiry.

## What this skill does

Outcome-first, with the single command that answers each question:

| Question your MSP keeps asking | Command |
| --- | --- |
| What's about to breach SLA in the next 24 hours? | `halopsa-cli sla breaching --within 24h` |
| What's the dispatcher view across all agents and teams? | `halopsa-cli triage --team Support` |
| Who's overloaded right now? | `halopsa-cli agent workload` |
| What's the whole story for this client, on one screen? | `halopsa-cli client card "Acme Corp"` |
| How much contract time is left across every contract? | `halopsa-cli contracts burn` |
| Which clients have stale tickets aging out? | `halopsa-cli tickets age-out` |
| What changed in Halo since this morning? | `halopsa-cli tickets changed-since 9:00` |
| What's the asset history for this device? | `halopsa-cli asset history <id>` |
| Which KB article should the tech link to? | `halopsa-cli kbarticle suggest --ticket 12345` |
| First-time hydration of the local SQLite mirror | `halopsa-cli sync --full` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most HaloPSA MCP servers and AI integrations proxy each question into a live API call. That's fine for one ticket. It dies at QBR time, when you're asking *"how many backup-failure tickets across all 47 clients last quarter, grouped by contract type?"* - that's 47 paginated REST calls, HaloPSA rate-limit headaches (limits aren't publicly documented; they vary by hosting), and a context window full of raw JSON the model has to re-read.

This skill syncs HaloPSA into a **local SQLite mirror** with full-text search. Aggregate questions become one local SQL join: instant, offline, and the AI sees the answer, not the raw data. Compound commands like `triage`, `client card`, and `contracts burn` join across tickets, clients, contracts, time, and assets - work a stateless API wrapper can't do.

It complements HaloPSA's built-in ChatGPT integration (which is great for per-ticket work like rewriting replies and sentiment-flagging). The two are non-overlapping.

## The pain this closes

Two pains MSP owners name repeatedly:

- **Cost-to-serve is rising faster than contract value.** The squeeze is operational: queue overload, SLA fires, unpriced work eating margin on every ticket.
- **Key-person dependency.** "If your best technician left tomorrow, would your clients notice within 30 days?" For most MSPs the answer is yes, because situational awareness lives in one person's head - and in a PSA nobody else triages quickly.

This skill puts the PSA's cross-entity truth one sentence away from any team member or their agent: `triage` / `sla breaching` surface what will breach before it does; `client card` gives any teammate one situational-awareness panel on demand; `contracts burn` catches unpriced work early; `agent workload` makes the queue legible across the whole team. The knowledge of how to read the PSA moves from a person into a Skill any teammate (or their agent) can run.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The HaloPSA MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes - all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md). Tool-specific gotchas (Copilot uses `servers` not `mcpServers`, Cline's Windows `npx` issue, ChatGPT's tier + bridge) are documented there.

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex - your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your HaloPSA API credentials once.

### Is my HaloPSA data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The SQLite mirror sits in a directory under your user account. The AI agent only sees what the CLI returns - typically a query result, not raw bulk data. Credentials are read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills.

### Will this hit my HaloPSA API rate limits?

Almost never. HaloPSA's rate limits aren't publicly documented and vary between cloud-hosted and self-hosted instances. This skill syncs once with `sync --full`, then incrementally; subsequent triage, SLA, client-card, and cross-client analytics queries run against the local mirror, not the live API. The big-batch queries that get you 429'd with API-passthrough tools become one SQL join here.

### How is this different from HaloPSA's built-in ChatGPT integration?

HaloPSA's built-in ChatGPT integration is great for **single-ticket** work - rewriting replies, summarizing one ticket, sentiment-flagging. MSP Skills is the **cross-client analytics + MSP-owner-on-the-couch** layer: questions across thousands of tickets, multi-system queries that join HaloPSA with Servosity, ad-hoc reports HaloPSA's UI doesn't surface. The two complement; you don't pick one.

### Can I run this on Windows?

Yes. The PowerShell installer above is the Windows path. CLI and MCP binaries are native Windows builds. Cline users on Windows may need a small `npx` workaround documented in [docs/which-agent.md](../../docs/which-agent.md).

### What does it cost?

Free. Apache-2.0 licensed. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), and that's billed by your AI provider, not by us.

## Safety model

The skill authenticates to **your own Halo tenant** with an OAuth application you create and scope. It defaults to discovery and dry-run before any mutation.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `triage`, `sla breaching`, `client card`, `contracts burn` | Allow |
| Write (routine) | ticket updates, notes, assignment | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | deletes, configuration changes | Human-in-the-loop only |

The strongest control is the **scope you grant the Halo OAuth application** - the CLI can only do what that application is permitted to do. Full details, including how to lock it down, are in [governance.md](./governance.md).

## Status

Beta. Validated against the HaloPSA API surface and being validated with MSPs running it live against their own production tenant in our weekly Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible - works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com) and [OpenClaw](https://docs.openclaw.ai), with `metadata.openclaw` frontmatter pre-wired so `openclaw skills install` works directly.

Maintained by [Servosity](https://www.servosity.com) for the MSP community. Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). See [TRADEMARKS.md](../../TRADEMARKS.md) for vendor non-affiliation. _Last updated: 2026-05-28._
