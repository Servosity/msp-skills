# Domotz CLI

**Every Domotz endpoint, plus a local SQLite fleet mirror that answers cross-site questions.**

domotz-cli gives MSPs and AV integrators full command-line and agent-native access to Domotz Collectors, devices, variables, alerts, and network topology. It syncs your whole fleet into a local database so cross-site rollups  -  fleet health, every offline device, new-device detection, one unified inventory export  -  become single offline queries instead of agent-by-agent API sweeps.

Learn more at [Domotz](https://www.domotz.com/).

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `domotz-cli` and `domotz-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.ps1 | iex
   ```
3. Verify: `domotz-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=domotz). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/domotz --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/domotz --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `domotz-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the domotz skill from https://github.com/Servosity/msp-skills/tree/main/skills/domotz. The skill defines how its required CLI (`domotz-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=domotz).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `DOMOTZ_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

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

### Remote MCP over HTTP

`domotz-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `DOMOTZ_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `DOMOTZ_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export DOMOTZ_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
domotz-mcp --transport http
```

Set `DOMOTZ_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Authenticate with an API key from the Domotz Portal (Settings > API Key). Set DOMOTZ_API_KEY, and set your region/cell (shown beside the key, e.g. us-east-1-cell-1) via DOMOTZ_REGION so the CLI targets api-<region>.domotz.com.

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
- **`fleet unalerted`**  -  Devices no enabled alert profile watches, across every Collector, VITAL first: the devices that can go down without anyone being told.

  _Pick this for alerting hygiene: a VITAL device with no enabled alert profile fails silently. For devices Domotz cannot authenticate to, use 'fleet unmonitored'._

  ```bash
  domotz-cli fleet unalerted --vital-only --json
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

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `DOMOTZ_CONFIG_DIR`, `DOMOTZ_DATA_DIR`, `DOMOTZ_STATE_DIR`, or `DOMOTZ_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `DOMOTZ_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export DOMOTZ_HOME=/srv/domotz
domotz-cli doctor
```

On Windows (PowerShell):

```powershell
$env:DOMOTZ_HOME = "C:\ProgramData\domotz"
domotz-cli doctor
```

Under `DOMOTZ_HOME=/srv/domotz`, the four dirs resolve to `/srv/domotz/config`, `/srv/domotz/data`, `/srv/domotz/state`, and `/srv/domotz/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:DOMOTZ_HOME = "C:\srv\domotz"
domotz-cli doctor
```

Under `$env:DOMOTZ_HOME = "C:\srv\domotz"`, the four dirs resolve to `C:\srv\domotz\config`, `C:\srv\domotz\data`, `C:\srv\domotz\state`, and `C:\srv\domotz\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "domotz": {
      "command": "domotz-mcp",
      "env": {
        "DOMOTZ_HOME": "/srv/domotz"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `DOMOTZ_DATA_DIR` overrides an explicit `--home` for that kind. Use `DOMOTZ_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `DOMOTZ_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `domotz-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `domotz-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `domotz-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `DOMOTZ_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `DOMOTZ_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

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


### drift


- **`domotz-cli drift`** - Diffs two local snapshots for a site to surface configuration and inventory changes over time.


### fleet


- **`domotz-cli fleet health`** - One status board across every Domotz Collector: online/offline agents, degraded sites, and down-device counts per site.

- **`domotz-cli fleet offline`** - Every unreachable or offline device across all sites in one prioritized list, with site and importance.

- **`domotz-cli fleet new`** - Devices first-seen anywhere in the fleet within a time window  -  a rogue-device security signal.

- **`domotz-cli fleet inventory`** - One asset table  -  vendor, model, type, OS, serial, site  -  for every device across every agent.

- **`domotz-cli fleet alerts`** - Alert profiles (alerting rules) and their device bindings, aggregated across the fleet.

- **`domotz-cli fleet ip-conflicts`** - IP address conflicts across all sites in one prioritized list.

- **`domotz-cli fleet breakdown`** - Counts of devices by type, vendor, and OS across the fleet for capacity and security posture.

- **`domotz-cli fleet speedtest`** - Per-site WAN speed-test history aggregated into fleet min/avg/max and a worst-site ranking.

- **`domotz-cli fleet agents`** - Only the sites (Collectors) that are offline or degraded right now, with location, organization, and version, in one prioritized list.

- **`domotz-cli fleet stale`** - Collectors whose local snapshot has gone quiet  -  last-synced or last-seen older than a threshold, a silent monitoring blind spot.

- **`domotz-cli fleet triggers`** - TCP service sensors (eyes) currently DOWN across every device on every agent, in one fleet-wide list.

- **`domotz-cli fleet unmonitored`** - Devices Domotz can't fully monitor  -  failed or missing authentication and SNMP status  -  surfaced fleet-wide as a coverage-gap audit.

- **`domotz-cli fleet events`** - Per-agent activity-log and network-event history merged into one chronological fleet-wide timeline.

- **`domotz-cli fleet unalerted`** - Devices no enabled alert profile watches, across every Collector, VITAL first: the devices that can go down without anyone being told.


### topology


- **`domotz-cli topology`** - Cached network topology for a site, summarized (node/edge counts, gateways) and queryable offline.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`domotz-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`domotz-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`domotz-cli learnings list`** - Inspect taught rows
- **`domotz-cli learnings forget <query>`** - Undo a teach
- **`domotz-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`domotz-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`domotz-cli teach-pattern`** - Install a query/resource template up front
- **`domotz-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `DOMOTZ_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `domotz-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
domotz-cli agent list

# JSON for scripting and agents
domotz-cli agent list --json
# Filter to specific fields
domotz-cli agent list --json --select access_right,creation_time,display_name

# Dry run  -  show the request without sending
domotz-cli agent list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
domotz-cli agent list --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

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

Run `domotz-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/domotz-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\domotz-cli\config.toml` on Windows; `--home`, `DOMOTZ_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `DOMOTZ_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `DOMOTZ_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `DOMOTZ_REGION` | endpoint | Yes |  |
| `DOMOTZ_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `domotz-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- The API key (`X-Api-Key: <key>`) and every API request go only to the scheme, host and port of the configured base URL (`https://api-<region>.domotz.com/public-api/v1` built from `DOMOTZ_REGION`, default `us-east-1-cell-1`, or `DOMOTZ_BASE_URL`). A redirect to any other host is refused, so the key never reaches it, and a region that is not a single host-name label is refused before anything is sent.
- Any output, file, local store write or MCP result that would carry `DOMOTZ_API_KEY`, the stored key, or a credential from a response (a device's SNMP communities, an SNMPv3 user, authentication key or encryption key, a device or custom driver username and password), in any encoding, is withheld whole: the program prints "[credkit: withheld from stdout: it carried a credential this program holds]" (naming the sink) in its place. Leave those fields out with `--select` to print the rest. Responses are never cached on disk, and credential-named fields reach the local store as "[redacted]".
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the key from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, `domotz_execute` (every Domotz endpoint that changes something) and `import` return a preview and send only with `confirm: true`, and a tool argument can never name a file to read or write.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `domotz-cli doctor` to check credentials
- Verify the environment variable is set: `echo $DOMOTZ_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Set DOMOTZ_API_KEY to a valid Portal API key; run domotz-cli doctor to confirm.
- **404 / wrong host on requests**  -  Set DOMOTZ_REGION to your account region+cell (e.g. eu-west-1-cell-1) from the Portal API Keys page; the base URL is api-<region>.domotz.com.
- **fleet commands return empty**  -  The store-backed fleet reports read what the last `domotz-cli sync` wrote (every sync is a full walk), and a report on a resource never synced refuses rather than answering empty; an empty answer after a finished sync means the account has none. `fleet events`, `ip-conflicts`, `speedtest`, `triggers` and `unalerted` read the API live and need DOMOTZ_API_KEY.
- **A report says the last sync did not finish, or that a resource was never synced**  -  The local store holds only part of that resource, so the report refuses to answer from it. Run the sync command the message prints (it names the resource), then run the report again.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**public-api-examples**](https://github.com/domotz/public-api-examples)  -  Python (30 stars)
- [**node-red-domotz**](https://github.com/domotz/node-red-domotz)  -  JavaScript (10 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
