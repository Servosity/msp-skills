# Domotz CLI

**Every Domotz endpoint, plus a local SQLite fleet mirror that answers cross-site questions.**

domotz-cli gives MSPs and AV integrators full command-line and agent-native access to Domotz Collectors, devices, variables, alerts, and network topology. It syncs your whole fleet into a local database so cross-site rollups  -  fleet health, every offline device, new-device detection, one unified inventory export  -  become single offline queries instead of agent-by-agent API sweeps.

Learn more at [Domotz](https://www.domotz.com/).

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `domotz-cli` binary and the `pp-domotz` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install domotz
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install domotz --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install domotz --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install domotz --agent claude-code
npx -y @mvanhorn/printing-press-library install domotz --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/domotz/cmd/domotz-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/domotz-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install domotz --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-domotz --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-domotz --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install domotz --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/domotz-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `DOMOTZ_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/domotz/cmd/domotz-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "domotz": {
      "command": "domotz-mcp",
      "env": {
        "DOMOTZ_REGION": "<region>",
        "DOMOTZ_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Authenticate with an API key from the Domotz Portal (Settings > API Key). Set DOMOTZ_API_KEY (DOMOTZ_PUBLIC_API_KEY is also accepted as a fallback), and set your region/cell (shown beside the key, e.g. us-east-1-cell-1) via DOMOTZ_REGION so the CLI targets api-<region>.domotz.com.

## Quick Start

```bash
# verify API key, region, and reachability before anything else
domotz-cli doctor

# list your Domotz Collectors (sites)
domotz-cli agent list --json

# mirror agents and devices into the local store
domotz-cli sync --full

# one status board across every site
domotz-cli fleet health --agent

# every offline device across the whole fleet
domotz-cli fleet offline --json

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-fleet rollups the API can't do in one call
- **`fleet health`**  -  One status board across every Domotz Collector: online/offline agents, degraded sites, and down-device counts per site.

  _Pick this when an agent needs the single-glance 'is anything on fire across all my sites' answer instead of looping the agent list endpoint._

  ```bash
  domotz-cli fleet health --agent
  ```
- **`fleet offline`**  -  Every unreachable or offline device across all sites in one prioritized list, with site and importance.

  _Use to triage outages fleet-wide without paginating per-agent device lists._

  ```bash
  domotz-cli fleet offline --json --select site,display_name,importance
  ```
- **`fleet new`**  -  Devices first-seen anywhere in the fleet within a time window  -  a rogue-device security signal.

  _Reach for this on a security sweep: surface unexpected devices that appeared on any monitored network overnight._

  ```bash
  domotz-cli fleet new --since 24h --json
  ```
- **`fleet inventory`**  -  One asset table  -  vendor, model, type, OS, serial, site  -  for every device across every agent.

  _Use for client asset reports or a fleet-wide CMDB export in one command._

  ```bash
  domotz-cli fleet inventory --csv > assets.csv
  ```
- **`fleet alerts`**  -  Alert profiles (alerting rules) and their device bindings, aggregated across the fleet.

  _Pick this to review every site's alerting rules in one place instead of walking each agent._

  ```bash
  domotz-cli fleet alerts --json
  ```
- **`fleet ip-conflicts`**  -  IP address conflicts across all sites in one prioritized list.

  _Use to catch addressing problems fleet-wide before they cause silent outages._

  ```bash
  domotz-cli fleet ip-conflicts --json
  ```
- **`fleet agents`**  -  Only the sites (Collectors) that are offline or degraded right now, with location, organization, and version, in one prioritized list.

  _Pick this when the question is 'which SITES are down'  -  not individual devices ('fleet offline') and not the full healthy-included board ('fleet health')._

  ```bash
  domotz-cli fleet agents --agent
  ```
- **`fleet triggers`**  -  TCP service sensors (eyes) currently DOWN across every device on every agent, in one fleet-wide list.

  _Pick this for currently-down TCP service checks across the fleet; alert-profile rules and bindings live in 'fleet alerts'._

  ```bash
  domotz-cli fleet triggers --agent
  ```
- **`fleet events`**  -  Per-agent activity-log and network-event history merged into one chronological fleet-wide timeline.

  _Pick this for the fleet-wide activity feed; for newly-appeared devices use 'fleet new', for currently-down devices use 'fleet offline'._

  ```bash
  domotz-cli fleet events --since 24h --agent
  ```

### Fleet analytics
- **`fleet breakdown`**  -  Counts of devices by type, vendor, and OS across the fleet for capacity and security posture.

  _Reach for this to answer 'how many of X do we manage' across all clients at once._

  ```bash
  domotz-cli fleet breakdown --by vendor --json
  ```
- **`fleet speedtest`**  -  Per-site WAN speed-test history aggregated into fleet min/avg/max and a worst-site ranking.

  _Use to spot the slowest client circuits across the fleet from one command._

  ```bash
  domotz-cli fleet speedtest --json
  ```
- **`fleet stale`**  -  Collectors whose local snapshot has gone quiet  -  last-synced or last-seen older than a threshold, a silent monitoring blind spot.

  _Pick this to find agents that stopped reporting (Collector offline or sync gap) before trusting any other fleet rollup._

  ```bash
  domotz-cli fleet stale --max-age 24h --agent
  ```
- **`fleet unmonitored`**  -  Devices Domotz can't fully monitor  -  failed or missing authentication and SNMP status  -  surfaced fleet-wide as a coverage-gap audit.

  _Pick this for monitoring hygiene (creds/SNMP gaps), not for devices that are simply offline  -  that is 'fleet offline'._

  ```bash
  domotz-cli fleet unmonitored --agent
  ```

### Per-site, offline
- **`topology`**  -  Cached network topology for a site, summarized (node/edge counts, gateways) and queryable offline.

  _Pick this to inspect a site's topology fast and offline after a sync._

  ```bash
  domotz-cli topology --agent-id 12345 --json
  ```
- **`drift`**  -  Diffs two local snapshots for a site to surface configuration and inventory changes over time.

  _Use after a maintenance window to see exactly what changed on a network._

  ```bash
  domotz-cli drift --agent-id 12345 --json
  ```

## Recipes


### Triage outages fleet-wide

```bash
domotz-cli fleet offline --json --select site,display_name,importance
```

Lists every offline device across all sites with just the fields you need for triage.

### Narrow a verbose device payload

```bash
domotz-cli fleet inventory --json --select site,display_name,type,vendor,os
```

Device records are deeply nested; dotted --select paths pull only the columns that matter and keep agent context small.

### Overnight rogue-device sweep

```bash
domotz-cli fleet new --since 24h --json
```

Surfaces devices first-seen anywhere in the fleet in the last day for a security review.

### Client asset report

```bash
domotz-cli fleet inventory --csv > assets.csv
```

Exports a single vendor/model/type/OS/serial/site table across every managed device.

### Read a site's topology offline

```bash
domotz-cli topology --agent-id 12345 --cached --json
```

Re-reads the last cached topology graph summary without re-hitting the API (fetch live first by running without --cached).

## Usage

Run `domotz-cli --help` for the full command reference and flag list.

## Commands

### agent

Manage agent

- **`domotz-cli agent count`** - Counts the collectors.
- **`domotz-cli agent delete`** - Deletes a collector.
- **`domotz-cli agent get`** - Returns the details of a collector.
- **`domotz-cli agent get-list-uptime`** - Returns the uptime of all collectors.
- **`domotz-cli agent list`** - Returns the list of collectors accessible by the user.

### alert-profile

Manage alert profile

- **`domotz-cli alert-profile get-agent`** - Returns the alert profile bindings of a collector.
- **`domotz-cli alert-profile get-alert-profiles2`** - Returns the list of configured alert profiles. You can configure alert profiles on the Domotz Portal. Alert profiles define the association between a list of events and a notification channel (email, webhook or slack).
- **`domotz-cli alert-profile get-devices`** - Returns the alert profile bindings of the devices of a collector.

### area

Manage area

- **`domotz-cli area`** - Returns all the areas of a Company. Note: This API is restricted to users on the Enterprise Plan. Please contact <a href="mailto:sales@domotz.com">sales@domotz.com</a> to learn more.

### custom-driver

Manage custom driver

- **`domotz-cli custom-driver get`** - Returns details of a Custom Driver.
- **`domotz-cli custom-driver list`** - Retrieves the list of available Custom Drivers.
- **`domotz-cli custom-driver list-associations`** - Retrieves a list of all Custom Driver associations for a collector.
- **`domotz-cli custom-driver re-enable-associations`** - Re-enable all disabled Custom Drivers for the current user.

### custom-tag

Manage custom tag

- **`domotz-cli custom-tag create`** - Creates a new Tag.
- **`domotz-cli custom-tag delete`** - Deletes a Tag and removes it from Collectors and Devices.
- **`domotz-cli custom-tag edit`** - Updates one or more properties of an existing Tag.
- **`domotz-cli custom-tag get`** - Retrieves all Tags available in the account, including their metadata and usage counts.

### device-profile

Manage device profile

- **`domotz-cli device-profile`** - Returns the list of the available device profiles.

### inventory

Manage inventory

- **`domotz-cli inventory create-field`** - Creates a new Inventory Field - the user will be able to set key-values pairs on every device.
- **`domotz-cli inventory delete`** - Clears the inventory.
- **`domotz-cli inventory delete-field`** - Deletes the Inventory Field.
- **`domotz-cli inventory get`** - Enumerates all the Inventory fields.
- **`domotz-cli inventory update-field`** - Updates the Inventory Field.

### meta

Manage meta

- **`domotz-cli meta`** - Returns information about API usage and limits.

### rbac

Manage rbac

- **`domotz-cli rbac create-user`** - Create a new RBAC User.
- **`domotz-cli rbac create-user-group`** - Create a new RBAC User group.
- **`domotz-cli rbac delete-user`** - Delete an RBAC User by user ID.
- **`domotz-cli rbac delete-user-group`** - Delete an RBAC User group by user group ID.
- **`domotz-cli rbac edit-user`** - Update an RBAC User by user ID. Note: User groups are replaced by those provided in the request; omit required_authentication_type to reset the User's required authentication type.
- **`domotz-cli rbac edit-user-group`** - Update an RBAC User group by user group ID. Note: Users and roles are replaced by those provided in the request.
- **`domotz-cli rbac get-role`** - Retrieve a Role and its Permissions. Note: When 'is_applied_to_all_entities' is true, 'entity_ids' is omitted.
- **`domotz-cli rbac get-roles`** - List all RBAC roles and associated user groups.
- **`domotz-cli rbac get-user`** - Retrieve RBAC User details by User ID.
- **`domotz-cli rbac get-user-group`** - Retrieve RBAC User group details by user group ID.
- **`domotz-cli rbac get-user-groups`** - List all RBAC User groups with their details.
- **`domotz-cli rbac get-users`** - List all RBAC Users with their details.

### type

Manage type

- **`domotz-cli type list-device-base`** - Returns the device types list.
- **`domotz-cli type list-device-detected`** - Returns the detected device types list.

### user

Manage user

- **`domotz-cli user`** - Returns the account information.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
domotz-cli agent list

# JSON for scripting and agents
domotz-cli agent list --json

# Filter to specific fields
domotz-cli agent list --json --select id,name,status

# Dry run  -  show the request without sending
domotz-cli agent list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
domotz-cli agent list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `DOMOTZ_REGION` resolves `{region}`

Base URL: `https://api-{region}.domotz.com/public-api/v1`

## Health Check

```bash
domotz-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/domotz-public-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `DOMOTZ_REGION` | endpoint | Yes |  |
| `DOMOTZ_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `domotz-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `domotz-cli doctor` to check credentials
- Verify the environment variable is set: `echo $DOMOTZ_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Set DOMOTZ_API_KEY (or the DOMOTZ_PUBLIC_API_KEY fallback) to a valid Portal API key; run domotz-cli doctor to confirm.
- **404 / wrong host on requests**  -  Set DOMOTZ_REGION to your account region+cell (e.g. eu-west-1-cell-1) from the Portal API Keys page; the base URL is api-<region>.domotz.com.
- **fleet commands return empty**  -  Run domotz-cli sync --full first; fleet rollups read the local store.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**public-api-examples**](https://github.com/domotz/public-api-examples)  -  Python (30 stars)
- [**node-red-domotz**](https://github.com/domotz/node-red-domotz)  -  JavaScript (10 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
