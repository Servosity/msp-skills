# SuperOps CLI

**Every SuperOps PSA+RMM entity in your terminal, plus a local SQLite mirror that answers cross-entity questions the web UI can't.**

SuperOps unifies PSA and RMM on one relational database; this CLI syncs your whole tenant into local SQLite so you can grep, jq, and join across tickets, assets, clients, contracts, and invoices offline. Match every entity the GraphQL API exposes, then transcend with commands like sla-watch, unbilled, at-risk-assets, and alert-coverage that no single SuperOps call answers.

## Install

The recommended path installs both the `superops-cli` binary and the `pp-superops` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install superops
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install superops --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install superops --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install superops --agent claude-code
npx -y @mvanhorn/printing-press-library install superops --agent claude-code --agent codex
```

### Without Node

The generated install path is category-agnostic until this CLI is published. If `npx` is not available before publish, install Node or use the category-specific Go fallback from the public-library entry after publish.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/superops-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install superops --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-superops --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-superops --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install superops --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/superops-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `SUPEROPS_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "superops": {
      "command": "superops-mcp",
      "env": {
        "SUPEROPS_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Authenticate with a SuperOps API token (Settings - My Profile - API token) plus your tenant subdomain (Settings - MSP Information). Set SUPEROPS_API_TOKEN, SUPEROPS_SUBDOMAIN, and optionally SUPEROPS_REGION (us or eu). Every GraphQL request sends Authorization: Bearer <token> and the CustomerSubDomain header.

## Quick Start

```bash
# Verify token, subdomain, and API reachability before anything else
superops-cli doctor

# Pull the tenant into local SQLite so offline and cross-entity commands work
superops-cli sync

# List recent tickets as structured output an agent can parse
superops-cli tickets list --first 20 --agent

# See who is about to breach SLA grouped by technician
superops-cli sla-watch --by tech

# Surface worklog time that never got invoiced
superops-cli unbilled --since 2026-05-01

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-entity insight from local state
- **`sla-watch`**  -  See which open tickets are breaching or about to breach SLA, grouped by technician or client.

  _Reach for this to answer 'who is about to miss SLA and on whose queue' in one call instead of five filtered views._

  ```bash
  superops-cli sla-watch --by tech --window 4h --agent
  ```
- **`unbilled`**  -  Find logged worklog time that never landed on an invoice, totaled in dollars per client.

  _Reach for this at month-end to surface revenue leaking out of the billing pipeline._

  ```bash
  superops-cli unbilled --since 2026-05-01 --agent
  ```
- **`at-risk-assets`**  -  List assets missing a critical patch that also have an active (unresolved) alert.

  _Reach for this to prioritize remediation on endpoints that are both vulnerable and actively alerting._

  ```bash
  superops-cli at-risk-assets --client acme --agent
  ```
- **`alert-coverage`**  -  Partition alerts into open (uncovered) vs resolved, grouped by client.

  _Reach for this to catch clients with alerts still sitting unhandled  -  work nobody is tracking._

  ```bash
  superops-cli alert-coverage --client acme --agent
  ```
- **`client-360`**  -  One offline bundle of a client plus its sites, users, contracts, open tickets, assets, and open invoices.

  _Reach for this before a QBR or escalation to load the full client picture in one command._

  ```bash
  superops-cli client-360 <client> --agent
  ```
- **`stale-tickets`**  -  Open tickets with no conversation, note, or worklog activity in N days.

  _Reach for this to catch neglected tickets before they turn into SLA misses or angry clients._

  ```bash
  superops-cli stale-tickets --days 7 --agent
  ```

### Agent-native plumbing
- **`context-ticket`**  -  Assemble a ticket plus its worklogs, client, and SLA into one agent-shaped JSON blob (conversation/notes fetched live).

  _Reach for this as an AI triage agent's single read to ground a decision without six round-trips._

  ```bash
  superops-cli context-ticket 12345 --agent --select ticket.subject,client.name,sla.name
  ```

## Recipes


### Morning SLA triage by technician

```bash
superops-cli sla-watch --by tech --window 4h
```

Groups at-risk tickets per tech so the service desk knows where to push first.

### Month-end revenue leak check

```bash
superops-cli unbilled --agent --select client.name,worklog.minutes,worklog.amount
```

Lists unbilled worklog per client with just the fields billing needs.

### Patch remediation priorities

```bash
superops-cli at-risk-assets --client acme
```

Endpoints that are both missing critical patches and carrying an active (unresolved) alert.

### Agent context for a ticket

```bash
superops-cli context-ticket 12345 --agent --select ticket.subject,ticket.status,client.name,asset.hostName,sla.name
```

Pairs --agent with --select on a deeply nested bundle so an AI agent gets only the fields it needs instead of a multi-KB blob.

### Offline full-text search

```bash
superops-cli search 'disk full' --agent
```

FTS5 over synced tickets, assets, clients, and KB with no live API call.

## Usage

Run `superops-cli --help` for the full command reference and flag list.

## Commands

### alerts

Manage RMM alerts

- **`superops-cli alerts`** - List alerts

### assets

Manage SuperOps assets and endpoints

- **`superops-cli assets <id>`** - Get an asset by ID
- **`superops-cli assets`** - List assets

### clients

Manage SuperOps clients (accounts)

- **`superops-cli clients <id>`** - Get a client by account ID
- **`superops-cli clients`** - List clients

### contracts

Manage client contracts

- **`superops-cli contracts`** - List client contracts

### invoices

Manage invoices

- **`superops-cli invoices <id>`** - Get an invoice by ID
- **`superops-cli invoices`** - List invoices

### it-docs

Manage IT documentation

- **`superops-cli it-docs`** - List IT documentation

### kb

Manage knowledge base articles

- **`superops-cli kb`** - List knowledge base items

### service-items

Manage service catalog items

- **`superops-cli service-items`** - List service items

### sites

Manage client sites

- **`superops-cli sites`** - List client sites

### tasks

Manage tasks

- **`superops-cli tasks <id>`** - Get a task by ID
- **`superops-cli tasks`** - List tasks

### technicians

Manage technicians

- **`superops-cli technicians`** - List technicians

### tickets

Manage SuperOps tickets

- **`superops-cli tickets <id>`** - Get a ticket by display ID
- **`superops-cli tickets`** - List tickets

### users

Manage client users (contacts)

- **`superops-cli users`** - List client users

### worklogs

Manage worklog time entries

- **`superops-cli worklogs`** - List worklog entries


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
superops-cli alerts

# JSON for scripting and agents
superops-cli alerts --json

# Filter to specific fields
superops-cli alerts --json --select id,name,status

# Dry run  -  show the request without sending
superops-cli alerts --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
superops-cli alerts --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
superops-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/superops-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SUPEROPS_API_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `superops-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `superops-cli doctor` to check credentials
- Verify the environment variable is set: `echo $SUPEROPS_API_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **graphql access denied / UNAUTHENTICATED**  -  Regenerate the API token in Settings - My Profile and re-export SUPEROPS_API_TOKEN; only one token is valid per user at a time.
- **Empty results despite data existing**  -  Confirm SUPEROPS_SUBDOMAIN matches Settings - MSP Information exactly; the CustomerSubDomain header scopes every query to that tenant.
- **Wrong data center / connection errors**  -  Set SUPEROPS_REGION=eu if your tenant is hosted in the EU (euapi.superops.ai); default is us.
- **HTTP 429 / rate limited**  -  SuperOps caps at 800 requests per minute; lower sync --concurrency or let the adaptive limiter back off.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**wyre-technology/superops-mcp**](https://github.com/wyre-technology/superops-mcp)  -  Python
- [**dovetechnow/superops-tools**](https://github.com/dovetechnow/superops-tools)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
