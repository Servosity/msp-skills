# HaloPSA Claude Code Skill and MCP Server - install, commands, examples

> Unofficial. Community-built Claude Code Skill and MCP server for the HaloPSA,
> HaloITSM, and HaloCRM APIs. Not affiliated with, endorsed by, or sponsored by
> Halo Service Solutions Ltd. HaloPSA, HaloITSM, and HaloCRM are trademarks of
> Halo Service Solutions Ltd.

A HaloPSA Claude Code Skill and HaloPSA MCP server in one package, so your AI agent (Claude Code, Codex, Claude Desktop, or ChatGPT Desktop) can triage your queue, audit SLA breaches, build a per-client situational-awareness card, and reconcile contract hours without anyone clicking through five tabs in the Halo UI.

The CLI wraps the full Halo REST API (952 endpoints across tickets, clients, assets, contracts, time, KB, and workflows) and stores a local SQLite mirror so cross-entity queries like `triage`, `client card`, and `contracts burn` are instant and answer questions the live API alone cannot.

## Install as a Claude Code Skill (also works for Codex)

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/halopsa/install.ps1 | iex
```

The installer drops both `halopsa-cli` (the CLI) and `halopsa-mcp` (the MCP server) into your user bin path. Claude Code and Codex will discover the Skill via `SKILL.md` in this directory; the binary is what the Skill actually drives.

After install, verify:

```bash
halopsa-cli --version
```

## Install as an MCP server (Claude Desktop, ChatGPT Desktop)

Same install command as above also installs `halopsa-mcp`. Then wire it into your desktop agent. Full per-agent instructions in [mcp-install.md](./mcp-install.md). The TL;DR Claude Desktop config:

```json
{
  "mcpServers": {
    "halopsa": {
      "command": "halopsa-mcp",
      "env": {
        "HALOPSA_TENANT": "<your-tenant>",
        "HALOPSA_CLIENT_ID": "<your-client-id>",
        "HALOPSA_CLIENT_SECRET": "<your-client-secret>"
      }
    }
  }
}
```

## Authenticate

HaloPSA uses OAuth2 client_credentials. Create an API application in your tenant under Configuration > Integrations > Halo PSA API (Authentication Method: Client ID and Secret, Services), then run:

```bash
HALOPSA_TENANT=<yourtenant> halopsa-cli auth login \
  --client-id <id> --client-secret <secret>
```

The CLI caches the access token and auto-refreshes before expiry.

## First command

```bash
halopsa-cli triage --team Support --json
```

Per-agent open ticket load, stale tickets, and 24-hour SLA-breach count in one table. The dispatcher view Halo's UI scatters across five tabs.

## All commands

Full reference: [guide.md](./guide.md). Highlights:

- `halopsa-cli triage` - dispatcher view
- `halopsa-cli sla breaching --within 24h` - pre-empt SLA breaches before a hand-off
- `halopsa-cli client card "Acme Corp"` - one-panel client situational awareness
- `halopsa-cli contracts burn` - contract hours remaining
- `halopsa-cli agent workload` - per-agent open load, billable hours, oldest ticket
- `halopsa-cli sync --full` - first-time hydration of the local SQLite mirror

For the AI-agent operating contract (when to use `--agent` flag, `--dry-run`, etc.), read [AGENTS.md](./AGENTS.md).

## Safety model

The skill authenticates to **your own Halo tenant** with an OAuth application you
create and scope. It defaults to discovery and dry-run before any mutation.

| Tier | Examples | Recommended agent policy |
| --- | --- | --- |
| Read | `triage`, `sla breaching`, `client card`, `contracts burn` | Allow |
| Write (routine) | ticket updates, notes, assignment | Preview with `--dry-run`, then a reviewed write |
| Destructive / config | deletes, configuration changes | Human-in-the-loop only |

The strongest control is the **scope you grant the Halo OAuth application** - the
CLI can only do what that application is permitted to do. Full details, including
how to lock it down, are in [governance.md](./governance.md).
