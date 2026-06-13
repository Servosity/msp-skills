# ConnectWise Automate CLI

**Every ConnectWise Automate endpoint plus a local SQLite mirror that answers cross-client fleet questions the per-server web UI can't**

ConnectWise Automate's value is locked behind a per-server console built for one-endpoint-at-a-time work. This CLI syncs your whole fleet  -  computers, clients, locations, alerts, and patch history  -  into local SQLite, then answers the questions MSPs actually ask across clients: where the offline agents are (stale-agents), who's behind on patches (patch-compliance), and what changed overnight (since). Everything is offline, scriptable, and built for AI agents.

## Install

For the short install path see [README.md](./README.md). For wiring the MCP
server into every agent (Claude Desktop, ChatGPT, Codex, and more), see
[mcp-install.md](./mcp-install.md). This file is the command reference.

## Authentication

Automate is per-server: set CONNECTWISE_AUTOMATE_SERVER to your host (e.g. company.hostedrmm.com) and CONNECTWISE_AUTOMATE_CLIENT_ID to your registered integration GUID (required for v2020.11+). Mint a bearer token with `apitoken mint --username <u> --password <p>`, then export CONNECTWISE_AUTOMATE_TOKEN with the returned AccessToken. Tokens are short-lived; refresh with `apitoken refresh`.

## Quick Start

```bash
# First export CONNECTWISE_AUTOMATE_SERVER (your host) and CONNECTWISE_AUTOMATE_CLIENT_ID (your integration GUID), then mint a bearer token and export the returned AccessToken as CONNECTWISE_AUTOMATE_TOKEN
connectwise-automate-cli apitoken mint --username you --password '****'

# Confirm auth + server reachability
connectwise-automate-cli doctor

# Pull the fleet into local SQLite
connectwise-automate-cli sync

# Whole-fleet posture in one roll-up
connectwise-automate-cli fleet-health --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-client roll-ups the per-server UI can't do
- **`fleet-health`**  -  See every agent across every client in one roll-up: online/offline, last-contact age, and open-alert count, grouped by client.

  _Reach for this when an agent needs the whole-fleet posture in one shot instead of paging the Computers endpoint client by client._

  ```bash
  connectwise-automate-cli fleet-health --agent
  ```
- **`stale-agents`**  -  List computers not seen in N days, grouped by client  -  the offline agents bleeding license and hiding risk.

  _Use before license true-ups or security reviews to find agents that stopped checking in._

  ```bash
  connectwise-automate-cli stale-agents --days 30 --agent
  ```
- **`patch-compliance`**  -  Per-client patch posture from synced patch history joined to computers  -  worst offenders first.

  _Pull this before a QBR or a security conversation to name the clients that are behind._

  ```bash
  connectwise-automate-cli patch-compliance --agent
  ```
- **`client-rollup`**  -  One-line-per-client snapshot: computers, locations, offline agents, and open alerts  -  built for the client review.

  _The fastest way to brief on a client's whole environment without clicking through the console._

  ```bash
  connectwise-automate-cli client-rollup --agent
  ```

### Triage and inventory
- **`alert-triage`**  -  Open alerts across every client, ranked by priority and joined to computer → location → client, with duplicates collapsed.

  _Start the morning here to see what actually needs a human across the whole book of business._

  ```bash
  connectwise-automate-cli alert-triage --min-priority 3 --agent
  ```
- **`os-inventory`**  -  Fleet-wide operating-system distribution with end-of-life OSes (Windows 7, Server 2008/2012) flagged for upgrade planning.

  _Use for security and hardware-refresh planning when you need the EOL exposure across every client at once._

  ```bash
  connectwise-automate-cli os-inventory --eol-only --agent
  ```
- **`since`**  -  Fleet activity in the last N hours from the records' own timestamps: alerts created, agents that checked in, and patches installed.

  _Run first thing to catch overnight activity across the fleet without paging each endpoint._

  ```bash
  connectwise-automate-cli since --hours 24 --agent
  ```

## Usage

Run `connectwise-automate-cli --help` for the full command reference and flag list.

## Commands

### alerts

Open monitor alerts across the fleet

- **`connectwise-automate-cli alerts get`** - Get a single alert by Id
- **`connectwise-automate-cli alerts list`** - List open alerts across all computers

### apitoken

Mint and refresh API bearer tokens

- **`connectwise-automate-cli apitoken mint`** - Mint a bearer token from username + password (needs CONNECTWISE_AUTOMATE_SERVER + clientId header)
- **`connectwise-automate-cli apitoken refresh`** - Refresh an existing (still-valid) bearer token

### clients

Clients (companies / customers) in Automate

- **`connectwise-automate-cli clients get`** - Get a single client by Id
- **`connectwise-automate-cli clients list`** - List all clients

### commands

Available commands that can be executed on agents

- **`connectwise-automate-cli commands get`** - Get a single command by Id
- **`connectwise-automate-cli commands list`** - List all available commands

### computers

Managed endpoints (agents)  -  the core RMM inventory

- **`connectwise-automate-cli computers alerts`** - Open alerts for one computer
- **`connectwise-automate-cli computers command-execute`** - Execute a command on one computer (WRITE  -  runs a real command on the agent)
- **`connectwise-automate-cli computers command-history`** - Recent command execution history for one computer
- **`connectwise-automate-cli computers get`** - Get a single computer by Id
- **`connectwise-automate-cli computers list`** - List computers across the fleet (paginated, filterable)
- **`connectwise-automate-cli computers patching-stats`** - Patch installation statistics for one computer
- **`connectwise-automate-cli computers software`** - Installed software inventory for one computer

### contacts

Client contacts

- **`connectwise-automate-cli contacts`** - List all client contacts

### groups

Computer groups (organizational + policy grouping)

- **`connectwise-automate-cli groups get`** - Get a single group by Id
- **`connectwise-automate-cli groups list`** - List all groups

### locations

Locations (sites) belonging to clients

- **`connectwise-automate-cli locations get`** - Get a single location by Id
- **`connectwise-automate-cli locations list`** - List all locations

### monitors

Monitors and their per-monitor statistics

- **`connectwise-automate-cli monitors list`** - List monitors with their alerting statistics
- **`connectwise-automate-cli monitors sensor-checks`** - List sensor checks

### network-devices

Discovered network devices (non-agent)

- **`connectwise-automate-cli network-devices`** - List discovered network devices

### patching

Patch history, compliance information, and patch policies

- **`connectwise-automate-cli patching approval-policies`** - Patch approval policies
- **`connectwise-automate-cli patching deploy-approved`** - Deploy all approved patches (WRITE  -  triggers fleet patch deployment)
- **`connectwise-automate-cli patching deploy-security`** - Deploy all security patches (WRITE  -  triggers fleet patch deployment)
- **`connectwise-automate-cli patching information`** - Global patch information / catalog status
- **`connectwise-automate-cli patching list`** - Fleet-wide patch installation history
- **`connectwise-automate-cli patching microsoft-policies`** - Microsoft update policies
- **`connectwise-automate-cli patching reattempt-failed`** - Reattempt failed patches (WRITE  -  retries failed patch installs)
- **`connectwise-automate-cli patching thirdparty-policies`** - Third-party update policies

### scripts

Automation scripts and their run state

- **`connectwise-automate-cli scripts list`** - List all scripts
- **`connectwise-automate-cli scripts running`** - List scripts currently running across the fleet
- **`connectwise-automate-cli scripts schedules`** - List scheduled script runs

### server

Automate server metadata (used by doctor / health)

- **`connectwise-automate-cli server db-time`** - Current database server time
- **`connectwise-automate-cli server info`** - Server version and metadata


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
connectwise-automate-cli alerts list

# JSON for scripting and agents
connectwise-automate-cli alerts list --json

# Filter to specific fields
connectwise-automate-cli alerts list --json --select id,name,status

# Dry run  -  show the request without sending
connectwise-automate-cli alerts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
connectwise-automate-cli alerts list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `CONNECTWISE_AUTOMATE_SERVER` resolves `{server}`

Base URL: `https://{server}/cwa/api/v1`

## Health Check

```bash
connectwise-automate-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/connectwise-automate-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `CONNECTWISE_AUTOMATE_SERVER` | endpoint | Yes |  |
| `CONNECTWISE_AUTOMATE_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `connectwise-automate-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `connectwise-automate-cli doctor` to check credentials
- Verify the environment variable is set: `echo $CONNECTWISE_AUTOMATE_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

---

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)

### API-specific
- **401 Unauthorized on every call**  -  Token expired (they are short-lived). Re-mint with `apitoken mint` or `apitoken refresh` and re-export CONNECTWISE_AUTOMATE_TOKEN.
- **403 / clientId errors on v2020.11+ servers**  -  Set CONNECTWISE_AUTOMATE_CLIENT_ID to your registered integration GUID; it is sent as the clientId header on every request.
- **Requests hit YOUR_SERVER.hostedrmm.com**  -  CONNECTWISE_AUTOMATE_SERVER is unset  -  export your real server host or set base_url in config.
- **Empty results from list commands**  -  Pagination is page/page-size (max 1000). Run `sync` first for offline queries, or pass --condition to filter the live call.
