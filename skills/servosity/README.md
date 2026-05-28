# Servosity + AI — for Claude, ChatGPT, Codex, Cursor, and any agent that speaks MCP

> Published by Servosity Inc. for MSP partners. Servosity is a trademark of Servosity Inc. Apache-2.0 licensed.

Add **fleet-wide backup triage, stale-backup-set detection, overnight drift, per-client situational awareness, and cross-engine analytics** to the AI you already use — **Claude Code**, **Claude Desktop**, **ChatGPT** (Plus/Pro+), **Codex**, **Cursor**, **Windsurf**, **Cline**, **Continue**, **Gemini**, or **GitHub Copilot**. Free, open source, runs on your laptop. A local fleet mirror means your AI can answer cross-client questions the partner portal can't show on one screen — Friday-email-ready in seconds. Built for MSP partners. No code required.

## Works with your agent

The four agents MSP owners actually use:

| Your AI agent | How to install the Servosity skill |
| --- | --- |
| **Claude Desktop** | Run installer, add the MCP block to `claude_desktop_config.json` (see "Add to Claude Desktop" below). |
| **ChatGPT** (Plus, Pro, Team, Business, Enterprise, Education)¹ | Run installer, expose `servosity-mcp` over HTTPS, register as a Developer Mode connector. |
| **Claude Code** | Paste the install prompt below into the chat, or run the installer yourself. |
| **Codex CLI** | Same as Claude Code — paste the prompt or run the installer. |

¹ ChatGPT requires a paid plan (Free tier does not yet expose Developer Mode). The Servosity MCP server is stdio — to use it with ChatGPT, expose it over HTTPS via the `mcp-remote` bridge or your own endpoint. See [mcp-install.md](./mcp-install.md).

> **Also works with** Cursor, Windsurf, Cline, Continue.dev, Zed, GitHub Copilot, and Gemini CLI — plus [Hermes](https://hermes-agent.nousresearch.com) via MCP and [OpenClaw](#install-for-openclaw) via the pre-wired frontmatter. Full per-tool wire-up: **[docs/which-agent.md](../../docs/which-agent.md)**.

## Install in 60 seconds

### Path A — let your AI install it (recommended)

Paste this into **Claude Code** or **Codex CLI**:

> Set up the **Servosity** skill from https://github.com/servosity/msp-skills — read `skills/servosity/SKILL.md`, run its install steps, then run `servosity-cli doctor` to confirm. Walk me through authentication.

### Path B — run the installer yourself

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

The installer drops both `servosity-cli` and `servosity-mcp` into your user bin path. Claude Code and Codex discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
servosity-cli --version
```

### Add to Claude Desktop

After running the installer above, add this block to your `claude_desktop_config.json` and restart Claude Desktop:

```json
{
  "mcpServers": {
    "servosity": {
      "command": "servosity-mcp",
      "env": {
        "SERVOSITY_MSP_TOKEN": "<your-partner-token>"
      }
    }
  }
}
```

Full per-agent wire-up (Cursor, Windsurf, Cline, Continue, ChatGPT, Gemini, Copilot): [mcp-install.md](./mcp-install.md) and [docs/which-agent.md](../../docs/which-agent.md).

<!-- pp-hermes-install-anchor -->
### Install for Hermes

From the Hermes CLI:

```bash
hermes skills install servosity/msp-skills/skills/servosity --force
```

Inside a Hermes chat session:

```
/skills install servosity/msp-skills/skills/servosity --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `servosity-mcp` server directly — same install path, same env vars.

### Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the servosity skill from https://github.com/servosity/msp-skills/tree/main/skills/servosity. The skill defines how its required CLI (`servosity-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

### Authenticate

Get your partner API token from the Servosity partner portal, then:

```bash
SERVOSITY_MSP_TOKEN=<token> servosity-cli doctor
```

`doctor` confirms the token works and the local mirror is reachable before you run anything that touches client data.

## What this skill does

Outcome-first, with the single command that answers each question:

| Question your MSP keeps asking | Command |
| --- | --- |
| What needs my attention across every client this morning? | `servosity-cli attention` |
| Which backups went stale across all clients? | `servosity-cli stale-backups --days 7` |
| What changed across my fleet overnight? | `servosity-cli drift --from yesterday --to now` |
| What's the whole story for this client, on one screen? | `servosity-cli company show <id>` |
| Search across companies, backups, and issues at once | `servosity-cli find "image manager"` |
| Triage what's worth keeping in the alert queue | `servosity-cli triage` |
| Clear known-safe alert noise so real failures show | `servosity-cli clear` |
| List DR restores in flight | `servosity-cli restore-queue list` |
| First-time hydration of the local fleet mirror | `servosity-cli sync --full` |

Full command reference: [guide.md](./guide.md). For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), see [AGENTS.md](./AGENTS.md).

## What makes this different

Most AI integrations for backup and DR vendors proxy each question into a live API call against the partner portal. That's fine for one client. It fails when you're asking *"which of my 47 clients have a backup set that hasn't succeeded in 7 days, sliced by engine?"* — the partner portal scatters that across per-client pages, three engine views, and an alert queue full of noise.

This skill syncs Servosity into a **local fleet mirror** with snapshot history and FTS5 search. Cross-engine, cross-client questions become one local query: instant, offline, and the AI sees the answer, not a context window full of paginated JSON. Compound commands like `attention`, `drift`, `stale-backups`, and `company show` join across companies, three backup engines, issues, contracts, and DR events — work a stateless API wrapper can't do.

## The pain this closes

Backup and DR is where "silent failure" hurts most. The pains MSP owners name:

- **Silent backup failures discovered too late.** A backup that quietly stopped succeeding is invisible until a client needs a restore — the worst possible moment to find out.
- **No fleet-wide view.** Each client's backup state lives in its own portal view; there is no single screen that says "across my whole book, here's what's stale, failing, or in-flight right now."
- **Alert-queue noise buries the real failure.** Dozens of repeat and known-safe issues pile up per client; the one that matters hides in the pile.
- **Per-client questions mean portal archaeology.** Answering "is this client OK?" means clicking through metadata, three backup engines, contracts, and issues by hand.

This skill turns per-client portal views into fleet-wide, offline-fast intelligence: `attention` is one screen across every client; `stale-backups` is the Friday-email list, ready; `drift` opens Monday with situational awareness instead of a blank slate; `triage` / `clear` / `stale-issues` batch-suppress known-safe noise so the queue shows only what's new; `company show` / `find` answer per-client and cross-fleet questions in one command.

## Frequently asked questions

### Does this work with ChatGPT?

Yes, on **Plus, Pro, Team, Business, Enterprise, and Education** plans (Free tier does not yet expose Developer Mode). ChatGPT connects to **remote** MCP servers over HTTPS, not local stdio binaries. The Servosity MCP server is local, so for ChatGPT you expose it via the `mcp-remote` bridge or your own HTTPS endpoint. Step-by-step in [mcp-install.md](./mcp-install.md).

### Does this work with Codex, Cursor, Windsurf, Cline, Copilot, or Gemini?

Yes — all of them speak MCP. Cross-tool install commands are in the matrix above and the deep-dive in [docs/which-agent.md](../../docs/which-agent.md). Tool-specific gotchas (Copilot uses `servers` not `mcpServers`, Cline's Windows `npx` issue, ChatGPT's tier + bridge) are documented there.

### Do I need to be a Servosity MSP partner?

Yes. The Servosity API surface this skill wraps is the one available to **authenticated MSP partners**. You'll need a partner API token from the Servosity partner portal. The skill itself is free; the token is part of your Servosity partner agreement. Not a Servosity partner yet? Reach out via [servosity.com](https://www.servosity.com).

### Will this replace my Servosity partner portal?

No. The portal is still where you do administrative work, configure backups, and onboard clients. This skill makes the portal's **data** answerable by AI — so morning triage, Friday stale-backup reports, drift checks, and ad-hoc cross-client questions don't require portal archaeology. The portal is unchanged; you're adding a new way to ask the data questions.

### Do I need to know how to code?

No. The recommended install is to paste one sentence into Claude Code or Codex — your agent reads `SKILL.md` and does the install. The fallback is a one-line installer per OS (bash or PowerShell). Neither path requires writing code. You'll enter your Servosity partner token once.

### Is my Servosity / client data safe?

Your data stays on **your machine**. The CLI and MCP server are local binaries. The fleet mirror sits in a directory under your user account. The AI agent only sees what the CLI returns — typically a query result, not raw bulk data. The partner token is read from your environment or your agent's config; never bundled into this repo or transmitted anywhere by MSP Skills. The token is scoped to **your reseller account only** (no cross-reseller access).

### Can I run this on Windows?

Yes. The PowerShell installer above is the Windows path. CLI and MCP binaries are native Windows builds.

### What does it cost?

Free. Apache-2.0 licensed. Servosity does not charge for the API access required to run this skill — the partner-portal API is part of your existing partner agreement. You pay only for whichever AI agent you use (Claude, ChatGPT, Codex, etc.), billed by your AI provider.

## Safety model

The skill authenticates with one partner token, scoped to **your reseller account only** (no cross-reseller access). Every mutating command plans by default: it runs `--dry-run` until you drop `--dry-run` and pass `--confirm`.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `attention`, `drift`, `stale-backups`, `backup-facts`, `find`, `company show`, `restore-queue list` | Allow |
| Write (routine) | `triage`, `clear`, `stale-issues`, notes/comments | Allow with `--confirm`; log the plan first |
| Credential / security | `credentials rotate/delete`, `current-user *-mfa-*`, `resellers agent-install-token`, `*-backups encryption-key update` | Human-in-the-loop only |
| Destructive | `companies delete`, `backups delete`, `restic-backups restic-prune`, `users delete` | Human-in-the-loop only |
| Admin (hidden) | `admin ...` | Operator-only, not for agents |

Keep autonomous agents to **Read plus planned writes**; gate everything below that behind a human. Full matrix and lock-down guidance in [governance.md](./governance.md).

## Status

Already used inside Servosity's own backup / DR operations; published here for MSP partners. The public surface is in beta and being validated with MSPs in live Build Sessions. RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

---

**Standards.** Conforms to the open [Agent Skills spec](https://agentskills.io) (Anthropic, Dec 2025; 40+ agents). MCP-compatible — works with any MCP-capable agent including [Hermes](https://hermes-agent.nousresearch.com). OpenClaw-ready (frontmatter pre-wired, awaiting OpenClaw launch).

Maintained by [Servosity](https://www.servosity.com). Apache-2.0 licensed. Built with [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press). _Last updated: 2026-05-28._
