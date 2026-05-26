# Servosity Claude Code Skill and MCP Server - install, commands, examples

> Published by Servosity Inc. for MSP partners. Servosity is a trademark of
> Servosity Inc. Apache-2.0 licensed.

A Servosity Claude Code Skill and Servosity MCP server in one package, so your AI agent (Claude Code, Codex, Claude Desktop, or ChatGPT Desktop) can triage your backup fleet, surface stale backup sets, diff yesterday's snapshot against today, and answer cross-engine questions without anyone clicking through the partner portal.

The CLI wraps the Servosity REST API surface available to authenticated MSP partners, with a local fleet mirror, snapshot history, and cross-engine rollups so commands like `attention`, `drift`, and `stale-backups` work offline once cached.

## Install as a Claude Code Skill (also works for Codex)

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.ps1 | iex
```

The installer drops both `servosity-cli` and `servosity-mcp` into your user bin path. Claude Code and Codex will discover the Skill via `SKILL.md` in this directory.

Verify:

```bash
servosity-cli --version
```

## Install as an MCP server (Claude Desktop, ChatGPT Desktop)

Same install command also installs `servosity-mcp`. Then wire it into your desktop agent. Full per-agent instructions in [mcp-install.md](./mcp-install.md). The TL;DR Claude Desktop config:

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

## Authenticate

Get your partner API token from the Servosity partner portal, then:

```bash
SERVOSITY_MSP_TOKEN=<token> servosity-cli doctor
```

`doctor` confirms the token works and the local mirror is reachable before you run anything that touches client data.

## First command

```bash
servosity-cli attention --json
```

One screen across your whole book of clients: merged open issues, stale backups, and in-flight DR events, ranked per company. Run this in the morning to triage what needs follow-up without clicking through the portal.

## All commands

Full reference: [guide.md](./guide.md). Highlights:

- `servosity-cli attention` - cross-client morning triage
- `servosity-cli drift --from yesterday --to now` - what changed overnight
- `servosity-cli stale-backups` - stale-backup-sets by company, age, engine
- `servosity-cli sync --full` - first-time hydration of the local fleet mirror
- `servosity-cli company show 4421` - per-client situational awareness (metadata, contracts, backups across 3 engines, open issues)
- `servosity-cli find "image manager"` - one FTS5 query across companies, issues, and backups

For the AI-agent operating contract (`--agent`, `--dry-run`, when to confirm before mutating), read [AGENTS.md](./AGENTS.md).

## Safety model

The skill authenticates with one partner token, scoped to **your reseller
account only** (no cross-reseller access). Every mutating command plans by
default: it runs `--dry-run` until you drop `--dry-run` and pass `--confirm`.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `attention`, `drift`, `stale-backups`, `backup-facts`, `find`, `company show`, `restore-queue list` | Allow |
| Write (routine) | `triage`, `clear`, `stale-issues`, notes/comments | Allow with `--confirm`; log the plan first |
| Credential / security | `credentials rotate/delete`, `current-user *-mfa-*`, `resellers agent-install-token`, `*-backups encryption-key update` | Human-in-the-loop only |
| Destructive | `companies delete`, `backups delete`, `restic-backups restic-prune`, `users delete` | Human-in-the-loop only |
| Admin (hidden) | `admin ...` | Operator-only, not for agents |

Keep autonomous agents to **Read plus planned writes**; gate everything below
that behind a human. Full matrix and lock-down guidance in
[governance.md](./governance.md).
