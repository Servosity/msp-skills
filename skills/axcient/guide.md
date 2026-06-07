# Axcient x360Recover CLI

**Every x360Recover endpoint, plus the fleet-wide backup-health answers the API alone can't give  -  offline, joined, and agent-ready.**

The only CLI and MCP surface for Axcient's x360Recover BCDR platform. It absorbs the full public API (vaults, appliances, devices, jobs, restore points, AutoVerify, usage, D2C agent tokens), then syncs everything into local SQLite so commands like 'health', 'client-rollup', and 'compliance' answer fleet-wide questions the per-entity API cannot  -  including the client-to-device correlation the raw API famously omits.

## Install

For the short install path (one-line installer, Claude Desktop `.mcpb`, and the per-agent
wire-up) see [README.md](./README.md) and [mcp-install.md](./mcp-install.md). This file is
the command reference.

## Authentication

Authentication uses an organization-scoped API key sent as the X-Api-Key header. Generate one in the x360Portal (Settings > API Keys, admin role required) and export it as AXCIENT_API_KEY. To evaluate the CLI without any credentials, point it at Axcient's public mock server: export AXCIENT_BASE_URL=https://ax-pub-recover.wiremockapi.cloud/x360recover and any non-empty AXCIENT_API_KEY value.

## Quick Start

```bash
# Verify config, auth wiring, and API reachability before anything else
axcient-cli doctor --dry-run

# Pull the fleet into the local store so joins and offline search work
axcient-cli sync --resources clients,device,vault,appliance

# The morning sweep: failed or stale backups across every client, grouped
axcient-cli health --agent

# One-line-per-client backup posture for the whole fleet
axcient-cli client-rollup --agent

# Offline full-text search over synced hostnames and names
axcient-cli search "server" --type device

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Fleet health that compounds locally
- **`health`**  -  See every device fleet-wide whose latest backup job failed or went stale, grouped by client, in one command.

  _Run this first for any 'which backups are broken right now' question across an MSP fleet instead of walking per-client endpoints._

  ```bash
  axcient-cli health --agent
  ```
- **`client-rollup`**  -  One row per client: devices total, failing, stale, RPO-breach, and AutoVerify-fail counts  -  the dashboard view MSPs build by hand today.

  _Reach for this when asked for a per-client backup posture summary rather than aggregating device lists yourself._

  ```bash
  axcient-cli client-rollup --agent
  ```
- **`billing`**  -  Roll up protected-system counts and storage usage for every client into one exportable table for invoice reconciliation.

  _Use this at month-end to reconcile what each client consumes against what they're invoiced, without per-client page visits._

  ```bash
  axcient-cli billing --csv
  ```
- **`appliance-map`**  -  Show which devices each appliance protects alongside each device's latest backup-job state.

  _Use this when triaging an appliance to see everything it protects and what state those backups are in._

  ```bash
  axcient-cli appliance-map --agent
  ```

### Compliance receipts
- **`compliance`**  -  Per-device compliance rows pairing newest restore-point age with AutoVerify boot-proof results and an RPO pass/fail verdict, exportable as CSV or JSON.

  _Use this to produce backup-compliance evidence for reports and audits instead of screenshotting the web UI device by device._

  ```bash
  axcient-cli compliance --client 42 --hours 24 --csv
  ```
- **`rpo`**  -  Flag devices whose newest restore point is older than a recovery-point objective threshold, grouped by client.

  _A job can succeed while RPO still slips; use this for restore-point-age questions and 'health' for job-status questions._

  ```bash
  axcient-cli rpo --hours 24 --agent
  ```

## Recipes


### Morning fleet sweep

```bash
axcient-cli sync --resources clients,device && axcient-cli health --agent
```

Refresh the store then list every failed/stale device grouped by client - the daily NOC triage in two commands.

### QBR compliance evidence

```bash
axcient-cli compliance --client 42 --hours 24 --csv
```

Exportable per-device rows pairing restore-point age with AutoVerify boot-proof for one client's review.

### Narrow a deep device payload

```bash
axcient-cli device get-by-org-id-org-level --agent --select id_,name,current_health_status.status
```

Device objects are deeply nested; --select keeps agent context small by returning only the fields that matter.

### Month-end billing reconciliation

```bash
axcient-cli billing --csv
```

Protected-system counts and storage per client in one table to match against invoices.

### Keyless evaluation against the public mock

```bash
axcient-cli organization --json
```

Axcient hosts a public wiremock fixture server - export AXCIENT_BASE_URL=https://ax-pub-recover.wiremockapi.cloud/x360recover (any non-empty AXCIENT_API_KEY) and the whole CLI works with no real tenant.

## Usage

Run `axcient-cli --help` for the full command reference and flag list.

## Commands

### appliance

Manage appliance

- **`axcient-cli appliance get`** - This request returns list of appliances belonging to the organization related to the user
- **`axcient-cli appliance get-by-id`** - This request returns information about appliance and assigned devices.

### client

These requests are used for managing clients


### clients

These requests are used for managing clients

- **`axcient-cli clients get`** - This request returns information about clients and their health.
- **`axcient-cli clients get-by-id`** - Returns the client information for organization related to the user by client ID

### device

These requests are used for managing devices

- **`axcient-cli device get-by-id-org-level`** - This request returns information about device by its id.
- **`axcient-cli device get-by-org-id-org-level`** - This request returns information about all devices
- **`axcient-cli device get-vault-restore-point-by-asio-endpoint-id-org-level`** - Returns information about device's restore points grouped by Cloud Vaults it is replicated on

### organization

Manage organization

- **`axcient-cli organization`** - This request returns information about organization retrieved by auth data

### user

Manage user

- **`axcient-cli user get-all-for-org`** - This request returns brief information about all organization users.
- **`axcient-cli user get-single-for-org`** - Returns information about user related to organization.

### vault

These requests are used for managing vaults

- **`axcient-cli vault get`** - This request returns information about vaults and assigned devices.
- **`axcient-cli vault get-by-id`** - This request returns information about vault and assigned devices.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
axcient-cli appliance get

# JSON for scripting and agents
axcient-cli appliance get --json

# Filter to specific fields
axcient-cli appliance get --json --select id_,alias,health_status

# Dry run  -  show the request without sending
axcient-cli appliance get --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
axcient-cli appliance get --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id_,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
axcient-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/axcient-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `AXCIENT_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `axcient-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `axcient-cli doctor` to check credentials
- Verify the environment variable is set: `echo $AXCIENT_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Regenerate the API key in x360Portal Settings > API Keys and re-export AXCIENT_API_KEY; keys are organization-scoped and admin-issued.
- **health/rpo/compliance return empty results**  -  Run 'axcient-cli sync --resources clients,device' first  -  these fleet-view commands read the local store, not the live API.
- **Want to try the CLI without an Axcient tenant**  -  Set AXCIENT_BASE_URL=https://ax-pub-recover.wiremockapi.cloud/x360recover with any non-empty AXCIENT_API_KEY to use Axcient's public mock server.
- **Device output has no client attribution**  -  Use the synced store (sync then health/client-rollup)  -  the upstream device object omits client_id; the local store restores the relationship.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**AxcientAPI**](https://github.com/adamburley/AxcientAPI)  -  PowerShell
- [**Hudu-Axcient-Dashboard**](https://github.com/adamburley/Hudu-Axcient-Dashboard)  -  PowerShell
- [**ax-api-python-samples**](https://github.com/cfogg-axcient/ax-api-python-samples)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
