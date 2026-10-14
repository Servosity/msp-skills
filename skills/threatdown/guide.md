# ThreatDown OneView CLI

**Every ThreatDown OneView API, plus fleet-wide stale-endpoint, health and seat reports across all your sites, and guarded isolate/scan/restore jobs.**

The fleet, threats, respond, seats and customer commands resolve every site's account id for you, so cross-site questions are one command; the generated endpoint, detection and job searches take --account-ids, which 'sites list' shows. fleet stale turns the vendor's endpoints-not-seen-in-24-hours script into one command, fleet health ranks sites by what needs attention, and respond issues scan, isolate, restore, reboot and update jobs with a target preview and a blast-radius guard.

Learn more at [ThreatDown OneView](https://www.threatdown.com).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `threatdown-cli` and `threatdown-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.ps1 | iex
   ```
3. Verify: `threatdown-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=threatdown). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/threatdown --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/threatdown --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `threatdown-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the threatdown skill from https://github.com/Servosity/msp-skills/tree/main/skills/threatdown. The skill defines how its required CLI (`threatdown-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=threatdown).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `THREATDOWN_CLIENT_ID` and `THREATDOWN_CLIENT_SECRET` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threatdown/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "threatdown": {
      "command": "threatdown-mcp",
      "env": {
        "THREATDOWN_CLIENT_ID": "<your-client-id>",
        "THREATDOWN_CLIENT_SECRET": "<your-client-secret>"
      }
    }
  }
}
```

</details>

## Authentication

OneView uses OAuth2 client credentials. Create an API client in OneView (Integrate), then set THREATDOWN_CLIENT_ID and THREATDOWN_CLIENT_SECRET. The CLI exchanges them at https://api.threatdown.com/oneview/oauth2/token with HTTP Basic client authentication and caches the bearer token. It requests the read, write and execute scopes by default; set THREATDOWN_OAUTH_SCOPE (for example 'read write') to request fewer. The respond commands need the execute scope. The token is cached, so after changing THREATDOWN_OAUTH_SCOPE run 'threatdown-cli auth login' to mint a new one. A 403 means either the OAuth scope or the creating user's OneView permission is missing.

## Quick Start

```bash
# Check config, credentials and API reachability
threatdown-cli doctor

# Cache every site's endpoints, detections and jobs locally
threatdown-cli fleet sync --since 7d

# Endpoints across every site not seen in the last day
threatdown-cli fleet stale --hours 24

# One row per site ranked by what needs attention
threatdown-cli fleet health

# Preview an isolation; add --confirm to issue it
threatdown-cli respond isolate --host LAPTOP-042

```

## Unique Features

### Fleet health across every site
- **`fleet stale`**  -  Every endpoint not seen in N hours across all your sites, with site name and exact hours since last seen.

  _Reach for this for the Monday dark-machine sweep instead of paging endpoints site by site._

  ```bash
  threatdown-cli fleet stale --hours 24 --agent
  ```
- **`fleet health`**  -  One ranked row per site with total, online, stale, infected, suspicious, isolated, reboot-required, unprotected, scan-overdue and update-available counts plus an attention score.

  _Use it to decide which customer needs attention first._

  ```bash
  threatdown-cli fleet health --agent
  ```
- **`fleet sync`**  -  Pull sites, endpoints, detections and jobs from every managed site into one local database.

  _Required before threats spread, threats queue and customer report, which read only this local copy (the generic sync does not fill it); their --since window cannot reach further back than this sync's --since._

  ```bash
  threatdown-cli fleet sync --since 7d
  ```
- **`seats reconcile`**  -  Per-site seat allocations compared with deployed and recently seen endpoints, with expiry flags.

  _Use it before PSA billing sync to catch over- or under-allocated sites._

  ```bash
  threatdown-cli seats reconcile --agent
  ```

### Safe response actions
- **`respond isolate`**  -  Isolate endpoints by host name with a target preview and a 25-endpoint blast-radius guard; sibling commands respond restore, respond scan, respond reboot and respond update work the same way.

  _Use it when an agent must contain an endpoint safely without hand-building a jobs body. It only previews until you add --confirm (or the global --yes), and it needs the execute OAuth scope._

  ```bash
  threatdown-cli respond isolate --host LAPTOP-042
  ```
- **`respond pending`**  -  Jobs that timed out, expired or failed, plus jobs still waiting after --min-age (default 1h), grouped by site and command with a re-issue hint.

  _Use it to confirm that scans, isolations or RMM-issued jobs actually ran._

  ```bash
  threatdown-cli respond pending --since 7d --agent
  ```

### Detection triage from the local store
- **`threats spread`**  -  Every threat in a window ranked by reach (distinct sites and endpoints), by hash or threat name; add --min-sites 2 for cross-site threats only.

  _Use it to judge blast radius during detection triage. Reads the local copy, so run fleet sync first._

  ```bash
  threatdown-cli threats spread --since 7d --agent
  ```
- **`threats queue`**  -  Prioritized list of infected or suspicious endpoints with online and isolated state and a suggested response.

  _Use it to pick which endpoint to contain next. Reads the local copy, so run fleet sync first._

  ```bash
  threatdown-cli threats queue --since 7d --agent
  ```
- **`customer report`**  -  Everything about one customer site in one output: health, stale machines, top threats, isolated endpoints and pending jobs.

  _Use it when a customer calls or a ticket names a site. Reads the local copy, so run fleet sync first._

  ```bash
  threatdown-cli customer report "Acme Corp" --agent
  ```

## Recipes

### Monday dark-machine sweep

```bash
threatdown-cli fleet stale --hours 24 --agent
```

Lists every endpoint not seen in 24 hours with its site and hours since last seen.

### Narrow a large site list

```bash
threatdown-cli sites list --agent --select company_name,account_id
```

Pulls only the site name and Nebula account id from the full site payload; pass those ids to --account-ids on the generated search commands.

### Contain an infected laptop safely

```bash
threatdown-cli respond isolate --host LAPTOP-042
```

Resolves the exact host name across all sites and prints a preview; nothing is sent until you add --confirm (or the global --yes). More than 25 targets (--max-targets) is refused, a host name that matches more than one endpoint is refused unless you narrow it with --site or pass --all-matches, the global --dry-run is not the preview, and issuing needs the execute OAuth scope.

### Release an endpoint from isolation

```bash
threatdown-cli respond restore --host LAPTOP-042
```

Ends network isolation (command.edr.restore; it does not restore quarantined files) and skips endpoints that are not isolated. respond scan runs a threat scan (--remove quarantines, --scan-type ThreatScan|HyperScan), respond reboot restarts, respond update refreshes protection definitions.

### Refresh the local copy, then triage

```bash
threatdown-cli fleet sync --since 7d
```

Required before threats spread, threats queue and customer report, which read only this copy; then run threats queue --since 7d --agent --select queue.host_name,queue.kind,queue.suggested_command.

### Find jobs that never ran

```bash
threatdown-cli respond pending --since 7d --agent
```

Shows timed-out, expired and failed jobs plus jobs still waiting after --min-age (default 1h), grouped by site with a re-issue command.

## Usage

Run `threatdown-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `THREATDOWN_CONFIG_DIR`, `THREATDOWN_DATA_DIR`, `THREATDOWN_STATE_DIR`, or `THREATDOWN_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `THREATDOWN_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export THREATDOWN_HOME=/srv/threatdown
threatdown-cli doctor
```

Under `THREATDOWN_HOME=/srv/threatdown`, the four dirs resolve to `/srv/threatdown/config`, `/srv/threatdown/data`, `/srv/threatdown/state`, and `/srv/threatdown/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "threatdown": {
      "command": "threatdown-mcp",
      "env": {
        "THREATDOWN_HOME": "/srv/threatdown"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `THREATDOWN_DATA_DIR` overrides an explicit `--home` for that kind. Use `THREATDOWN_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `THREATDOWN_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `threatdown-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### account-info

Manage account info

- **`threatdown-cli account-info`** - Get account id associated to the requesting client

### aidr

Manage aidr

- **`threatdown-cli aidr metadata`** - AI Detection & Response operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/aidr/metadata
- **`threatdown-cli aidr rules-create`** - AI Detection & Response operation the vendor has not documented: POST /oneview/v1/accounts/{account_id}/aidr/rules
- **`threatdown-cli aidr rules-delete`** - AI Detection & Response operation the vendor has not documented: DELETE /oneview/v1/accounts/{account_id}/aidr/rules/{id}
- **`threatdown-cli aidr rules-get`** - AI Detection & Response operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/aidr/rules/{id}
- **`threatdown-cli aidr rules-list`** - AI Detection & Response operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/aidr/rules
- **`threatdown-cli aidr rules-update`** - AI Detection & Response operation the vendor has not documented: PUT /oneview/v1/accounts/{account_id}/aidr/rules/{id}
- **`threatdown-cli aidr tools-search`** - AI Detection & Response operation the vendor has not documented: POST /oneview/v1/aidr/tools/search

### app-block

App Block

- **`threatdown-cli app-block activity-export`** - Export app block activity
- **`threatdown-cli app-block activity-export-async`** - Export app block activity asynchronously
- **`threatdown-cli app-block activity-search`** - Search app block activity log
- **`threatdown-cli app-block activity-search-groupby`** - Search app block activity log groupby
- **`threatdown-cli app-block rules-create`** - Create a new App Block Rule
- **`threatdown-cli app-block rules-delete`** - Delete an existing App Block Rule
- **`threatdown-cli app-block rules-get`** - Get an App Block Rule by ID
- **`threatdown-cli app-block rules-search`** - Search App Block Rules
- **`threatdown-cli app-block rules-update`** - Update an existing App Block Rule
- **`threatdown-cli app-block rulesets-get`** - App Block operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/app-block/rulesets/{id}
- **`threatdown-cli app-block rulesets-list`** - App Block operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/app-block/rulesets
- **`threatdown-cli app-block settings-get`** - Get App Block settings for account
- **`threatdown-cli app-block settings-update`** - Create or update App Block settings for account

### asset-ignore-rules

Manage asset ignore rules

- **`threatdown-cli asset-ignore-rules create`** - Ignore Rules operation the vendor has not documented: POST /oneview/v1/accounts/{account_id}/assets/ignore-rules
- **`threatdown-cli asset-ignore-rules delete`** - Ignore Rules operation the vendor has not documented: DELETE /oneview/v1/accounts/{account_id}/assets/ignore-rules/{id}
- **`threatdown-cli asset-ignore-rules delete-bulk`** - Ignore Rules operation the vendor has not documented: DELETE /oneview/v1/accounts/{account_id}/assets/ignore-rules
- **`threatdown-cli asset-ignore-rules get`** - Ignore Rules operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/assets/ignore-rules/{id}
- **`threatdown-cli asset-ignore-rules list`** - Ignore Rules operation the vendor has not documented: GET /oneview/v1/accounts/{account_id}/assets/ignore-rules
- **`threatdown-cli asset-ignore-rules update`** - Ignore Rules operation the vendor has not documented: PUT /oneview/v1/accounts/{account_id}/assets/ignore-rules/{id}

### cases

Manage cases

- **`threatdown-cli cases action`** - Case Management operation the vendor has not documented: POST /oneview/v1/casemgmt/accounts/{account_id}/cases/action
- **`threatdown-cli cases comments-create`** - Post cases comments
- **`threatdown-cli cases comments-list`** - Get cases comments
- **`threatdown-cli cases favorite`** - Put Cases favorite
- **`threatdown-cli cases file`** - Get Cases file
- **`threatdown-cli cases multiplexer-bulk`** - Post multiplexer cases bulk
- **`threatdown-cli cases multiplexer-events`** - Get cases events
- **`threatdown-cli cases multiplexer-metrics-bulk`** - Post multiplexer metrics bulk
- **`threatdown-cli cases overview`** - Get cases overview
- **`threatdown-cli cases requests`** - Post Cases Requests
- **`threatdown-cli cases search`** - Search Cases across accounts
- **`threatdown-cli cases wall`** - Get cases wall

### content-filtering

Content Filtering

- **`threatdown-cli content-filtering blockpage-get`** - Get DNS Blockpage settings for account
- **`threatdown-cli content-filtering blockpage-search`** - Search DNS blockPages for a given set of accounts
- **`threatdown-cli content-filtering blockpage-update`** - Save DNS Blockpage settings for account
- **`threatdown-cli content-filtering categories`** - Get a collection of DNS content filtering categories with detailed information, including id, name, description and the respective list of subcategories.
- **`threatdown-cli content-filtering create`** - Create a new content filtering rule
- **`threatdown-cli content-filtering delete`** - Delete a content filtering rule
- **`threatdown-cli content-filtering domain`** - Get domain details
- **`threatdown-cli content-filtering enable`** - Enable/Disable a content filtering rule
- **`threatdown-cli content-filtering enable-for-policy`** - Enable/Disable a content filtering rule for a Nebula policy
- **`threatdown-cli content-filtering exclusions-get`** - Get DNS  Global Exclusions for account
- **`threatdown-cli content-filtering exclusions-search`** - Search DNS global exclusions for a given set of accounts
- **`threatdown-cli content-filtering exclusions-update`** - Save DNS  Global Exclusions for account
- **`threatdown-cli content-filtering frequent-domains`** - Get frequently used domain names
- **`threatdown-cli content-filtering get`** - Get a content filtering rule details by id
- **`threatdown-cli content-filtering policies`** - Get list of policies
- **`threatdown-cli content-filtering search`** - Get a list of content filtering rules for current account
- **`threatdown-cli content-filtering tlds`** - Get all known top level domains (TLDs)
- **`threatdown-cli content-filtering update`** - Update a content filtering rule

### cve

Manage cve

- **`threatdown-cli cve bulk`** - Get CVE details in bulk.
- **`threatdown-cli cve export`** - Export CVE data. You can export across multiple accounts. Specify your interested fields and the output format.

Supported output formats: csv, xlsx, json.

**Settings**
| Name | Description |
|------|-------------|
| groups | List of queries. The constraints accepted are the same accepted by the search assets software route. Refer to [Search CVE API](#operation/api.v2.rmm.search.cve) for more information about the constraints accepted. |
| download | If set to true, the server will set the `Content-Disposition` header using the `filename` and `format` |
| select | Which fields to select, and which field name to map to. See request schema for allowed values |
| type | Output encoding (for `csv` and `json` format) |
| filename | Name of the file to be set in `Content-Disposition` header if `download` = true |

**Examples**
**Download HTML file**
A request with this body sent by a browser will download a file called `nameofthefile.html`, displaying a table with columns: `Path`, `Name`, `Machine ID`, `Machine Name` for all detections on `windows` endpoints within account id `1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34`:

```json
	{
		"groups": [{
      "account_id": "1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34",
			"severity": "critical"
		}],
		"format": "html",
		"type": "string",
		"filename": "nameofthefile",
		"download": true,
		"select": [
            {"field": "account_id", "newField": "account_id"},
            {"field": "group_id", "newField": "groupId"},
            {"field": "vendor", "newField": "vendor"},
            {"field": "product", "newField": "product"},
            {"field": "installation_date", "newField": "installationDate"},
            {"field": "created_at", "newField": "createdAt"}
        ]
	}
```
- **`threatdown-cli cve export-async`** - Export CVE asynchronously
- **`threatdown-cli cve get`** - Get CVE details by id
- **`threatdown-cli cve ignore`** - Vulnerability Management operation the vendor has not documented: POST /oneview/v1/cve/ignore
- **`threatdown-cli cve search`** - Search CVE across all your managed accounts. The search will be scoped to the account_ids you specify in the request body.
- **`threatdown-cli cve search-groupby`** - Search CVE grouped by a specific field across all your managed account.

### detections

Detections contain information on threats such as malware, ransomware, and malicious URLs found across your sites.

- **`threatdown-cli detections export`** - Export detections data. You can export across multiple accounts. Specify your interested fields and the output format.

Supported output formats: csv, xlsx, json.

**Settings**
| Name | Description |
|------|-------------|
| groups | List of queries. The constraints accepted are the same accepted by the search detections route. Refer to [Search detections API](#operation/api.v2.rmm.search.detections) for more information about the constraints accepted. |
| download | If set to true, the server will set the `Content-Disposition` header using the `filename` and `format` |
| select | Which fields to select, and which field name to map to. See request schema for allowed values |
| type | Output encoding (for `csv` and `json` format) |
| filename | Name of the file to be set in `Content-Disposition` header if `download` = true |

**Examples**
**Download HTML file**
A request with this body sent by a browser will download a file called `nameofthefile.html`, displaying a table with columns: `Path`, `Name`, `Machine ID`, `Machine Name` for all detections on `windows` endpoints within account id `1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34`:

```json
	{
		"groups": [{
			"account_ids": ["1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34"],
			"os_info.os_platform": "windows"
		}],
		"format": "html",
		"type": "string",
		"filename": "nameofthefile",
		"download": true,
		"select": [{
			"field": "path",
			"newField": "Path"
		},{
			"field": "threat_name",
			"newField": "Name"
		}, {
			"field": "machine_id",
			"newField": "Machine ID"
		}, {
			"field": "machine_name",
			"newField": "Machine Name"
		}]
	}
```

**Download xlsx file**
A request with this body sent by a browser will download a file called `report.html`, displaying a table with columns: `Path`, `Name`, `Machine ID`, `Machine Name` for all `windows` endpoints within account id `1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34` and the endpoint with id `211a4cfc-557e-4ba7-a96f-70685a07c29e`:

```json
	{
		"groups": [{
			"account_ids": ["1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34"],
			"os_info.os_platform": "windows"
		}],
		"populate": { "account": true, "last_job": true },
		"format": "xlsx",
		"type": "base64",
		"filename": "report",
		"download": true,
		"select": [{
			"field": "path",
			"newField": "Path"
		},{
			"field": "threat_name",
			"newField": "Name"
		}, {
			"field": "machine_id",
			"newField": "Machine ID"
		}, {
			"field": "machine_name",
			"newField": "Machine Name"
		}]
	}
```
- **`threatdown-cli detections export-async`** - Export detections asynchronously
- **`threatdown-cli detections get`** - Get detection and its full information by its detection ID.
- **`threatdown-cli detections search`** - Search detections across all your managed accounts. The search will be scoped to the account_ids you specify in the request body.
- **`threatdown-cli detections submit-fp`** - Submit false positive for a detection

### device-control

The Device Control feature manages access to USB storage drives.

- **`threatdown-cli device-control exclusions-bulk`** - Create device exclusions in bulk.
- **`threatdown-cli device-control exclusions-create`** - Create a device exclusion.
- **`threatdown-cli device-control exclusions-delete`** - Delete a device exclusion.
- **`threatdown-cli device-control exclusions-get`** - Get a device exclusion by its ID.
- **`threatdown-cli device-control exclusions-list`** - Get all device exclusions.
- **`threatdown-cli device-control exclusions-search`** - Search device control exclusions.
- **`threatdown-cli device-control exclusions-suggestions`** - Get excludable devices.
- **`threatdown-cli device-control exclusions-update`** - Update a device exclusion.
- **`threatdown-cli device-control export`** - Export Device Control events. You can export across multiple accounts. Specify your interested fields and the output format.

Supported output formats: csv, xlsx, json.

**Settings**
| Name | Description |
|------|-------------|
| groups | List of queries. The constraints accepted are the same accepted by the search device control event route. Refer to [Search Device Control Events API](#operation/api.v2.rmm.search.devicecontrol) for more information about the constraints accepted. |
| download | If set to true, the server will set the `Content-Disposition` header using the `filename` and `format` |
| select | Which fields to select, and which field name to map to. See request schema for allowed values |
| type | Output encoding (for `csv` and `json` format) |
| filename | Name of the file to be set in `Content-Disposition` header if `download` = true |

**Examples**
**Download CSV file**
A request with this body sent by a browser will download a file called `nameofthefile.csv`, displaying a table with columns: `Date`, `Action Taken`, `Device`, `Machine Name` for all events with `Blocked` status, starting from `2022/02/01` within account id `1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34`:

```json
	{
   "download":true,
   "format":"csv",
   "groups":[
      {
         "status":"blocked",
         "account_ids":[
            "1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34"
         ],
         "start_date":"2022-02-01T00:00:00-00:00"
      }
   ],
   "populate":{
      "account":true,
      "endpoint":true,
      "group":true
   },
   "select":[
      {
         "field":"scanned_at",
         "newField":"Date"
      },
      {
         "field":"status",
         "newField":"Action taken"
      },
      {
         "field":"device_control.physical_disk_name",
         "newField":"Device"
      },
      {
         "field":"machine_name",
         "newField":"Endpoint"
      }
   ],
   "type":"string"
}
```
- **`threatdown-cli device-control export-async`** - Export device control events asynchronously
- **`threatdown-cli device-control get`** - Get device control event and its full information by the device control event ID.
- **`threatdown-cli device-control search`** - Search device control events across all your managed accounts. The search will be scoped to the account_ids you specify in the request body.
- **`threatdown-cli device-control search-groupby`** - Group device control events across all your managed accounts. The search will be scoped to the account_ids you specify in the request body.

### dns

DNS

- **`threatdown-cli dns export`** - Export DNS data. You can export across multiple accounts. Specify your interested fields and the output format.

Supported output formats: csv, xlsx, json.

**Settings**
| Name | Description |
|------|-------------|
| groups | List of queries. The constraints accepted are the same accepted by the search assets software route. Refer to [Search DNS API](#operation/api.nebula.search.dns) for more information about the constraints accepted. |
| download | If set to true, the server will set the `Content-Disposition` header using the `filename` and `format` |
| select | Which fields to select, and which field name to map to. See request schema for allowed values |
| type | Output encoding (for `csv` and `json` format) |
| filename | Name of the file to be set in `Content-Disposition` header if `download` = true |

**Examples**
**Download CSV file**
A request with this body sent by a browser will download a file called `nameofthefile.cvs`, displaying a table with columns: `accountId`, `groupId`, `groupName`, `Domain`, `IP_address` for all the dns logs of machine_id `1514cd0a-2ef3-4db3-b8a4-c89894d9aa34`:

```json
{
  "groups": [
    { "machine_id": "1514cd0a-2ef3-4db3-b8a4-c89894d9aa34" }
  ],
  "format": "csv",
  "type": "string",
  "filename": "nameofthefile",
  "download": true,
  "select": [
    {"field": "account_id", "newField": "accountId"},
    {"field": "group_id", "newField": "groupId"},
    {"field": "group_name", "newField": "groupName"},
    {"field": "log.query_name", "newField": "Domain"},
    {"field": "log.dst_ip", "newField": "IP_address"}
  ]
}
```
- **`threatdown-cli dns export-async`** - Export DNS Logs asynchronously
- **`threatdown-cli dns search`** - Search DNS Logs across all your managed accounts. The search will be scoped to the account_ids you specify in the request body.
- **`threatdown-cli dns search-groupby`** - Search DNS Logs grouped by a specific field across all your managed accounts.

### email-protection

Email Protection

- **`threatdown-cli email-protection 911-mailbox-delete`** - Delete email protection 911 mailbox
- **`threatdown-cli email-protection 911-mailbox-get`** - Get email protection 911 mailbox
- **`threatdown-cli email-protection 911-mailbox-update`** - Edit email protection 911 mailbox
- **`threatdown-cli email-protection allow-list-create`** - Create a new email protection configurations allow list entry
- **`threatdown-cli email-protection allow-list-delete`** - Delete an email protection configurations allow list entry
- **`threatdown-cli email-protection allow-list-search`** - Get email protection configurations allow list
- **`threatdown-cli email-protection allow-list-update`** - Update email protection configurations allow list
- **`threatdown-cli email-protection auto-sync-disable`** - Disable email protection account auto-sync
- **`threatdown-cli email-protection auto-sync-enable`** - Enable email protection account auto-sync
- **`threatdown-cli email-protection auto-sync-groups`** - Search email protection account auto-sync groups
- **`threatdown-cli email-protection auto-sync-status`** - Get email protection account auto-sync status
- **`threatdown-cli email-protection daily-usage`** - Get email protection account daily usage
- **`threatdown-cli email-protection details`** - Get email protection account details
- **`threatdown-cli email-protection disable`** - Disable an email protection account
- **`threatdown-cli email-protection emails`** - Search email protection emails
- **`threatdown-cli email-protection incidents-classify`** - Classify a specific incident for an account
- **`threatdown-cli email-protection incidents-export`** - Export email protection incidents for an account
- **`threatdown-cli email-protection incidents-get`** - Get the details of a email protection incident for an account
- **`threatdown-cli email-protection incidents-scanback`** - Search Scanback email protection incidents for an account
- **`threatdown-cli email-protection incidents-scanback-export`** - Export Scanback email protection incidents for an account
- **`threatdown-cli email-protection incidents-search`** - Search email protection incidents for an account
- **`threatdown-cli email-protection incidents-uncluster`** - Reclassify incident emails by moving emails from one incident to a new incident with a different classification
- **`threatdown-cli email-protection integrations-disable`** - Disable the integration with Google Workspace or O365
- **`threatdown-cli email-protection integrations-domains-add`** - Add email protection domains
- **`threatdown-cli email-protection integrations-domains-check`** - Check one or more domains for dkim, spf and dmarc records
- **`threatdown-cli email-protection integrations-domains-delete`** - Delete email protection domais
- **`threatdown-cli email-protection integrations-domains-list`** - Get email protection domains
- **`threatdown-cli email-protection integrations-gw-admin-consent-url`** - Get the admin consent URL for the Google Workspace integration
- **`threatdown-cli email-protection integrations-gw-authorize`** - Authorize Google Workspace integration
- **`threatdown-cli email-protection integrations-o365-admin-consent-url`** - Get the admin consent URL for the email protection integration
- **`threatdown-cli email-protection integrations-o365-authorize`** - Authorize access to the O365 tenant
- **`threatdown-cli email-protection integrations-o365-phishing-button-manifest`** - Get the manifest for the Phishing Button OWA XML Manifest
- **`threatdown-cli email-protection integrations-status`** - Get the status of the integration
- **`threatdown-cli email-protection mailboxes-export`** - Export email protection mailboxes for an account
- **`threatdown-cli email-protection mailboxes-search`** - Search email protection mailboxes for an account
- **`threatdown-cli email-protection mailboxes-update`** - Edit email protection mailboxes
- **`threatdown-cli email-protection migrate`** - Migrate an email protection direct customer account to a new MSP parent account
- **`threatdown-cli email-protection migration-eligibility`** - Check if an account is eligible for migration to a new parent
- **`threatdown-cli email-protection monthly-usage`** - Get email protection account monthly usage
- **`threatdown-cli email-protection notification-recipients-create`** - Create email protection notification recipients
- **`threatdown-cli email-protection notification-recipients-list`** - Get email protection notification recipients
- **`threatdown-cli email-protection register`** - Register an email protection account
- **`threatdown-cli email-protection register-msp`** - Register an email protection MSP account
- **`threatdown-cli email-protection register-msp-site`** - Register an email protection MSP site account
- **`threatdown-cli email-protection settings-get`** - Get email protection account settings
- **`threatdown-cli email-protection settings-update`** - Edit email protection account settings
- **`threatdown-cli email-protection site-integration-status`** - Get email protection site integration status
- **`threatdown-cli email-protection stats`** - Get email protection statistics for an account
- **`threatdown-cli email-protection stats-domains`** - Get email protection domains statistics
- **`threatdown-cli email-protection stats-emails`** - Get email protection statistics
- **`threatdown-cli email-protection stats-incidents`** - Get email protection incident statistics
- **`threatdown-cli email-protection stats-most-targeted-departments`** - Get email protection most targeted departments
- **`threatdown-cli email-protection stats-most-targeted-users`** - Get email protection most targeted users

### endpoint-jobs

Manage endpoint jobs

- **`threatdown-cli endpoint-jobs bulk-cancel`** - Cancel pending jobs across accounts
- **`threatdown-cli endpoint-jobs bulk-issue`** - Use this endpoint to issue a job across multiple accounts you manage.
This API endpoint allows you to specify queries instead of explicitly specifying endpoint ids. See example for more details.

See `Issue a job` for the commands list

Jobs are issued asynchronously and processed by endpoint agents when they get online. If you want to get notifications about the progress of a job, create a subscription to the `job_status_change` webhook event.


**Command specific data**
You can customize each command using specific settings. Supply those command-specific settings in the `data` parameter.

**command.threat.quarantine.restore**
Restore item from quarantine. Specify one or more detection ids
```json
{"restore_items":["7021ebec-b7b8-49b3-8709-f36bf5002c30"]}
```

**command.threat.quarantine.remove**
Delete item from quarantine. Specify one or more detection ids
```json
{"delete_items":["7021ebec-b7b8-49b3-8709-f36bf5002c30"]}
```

**command.threat.scan**
If you want to just report detections:
```json
{"scan_settings":{"type":"ThreatScan","remove":false}}
```

If you want to quarantine detected items:
```json
{"scan_settings":{"type":"ThreatScan","remove":true}}
```

**command.edr.isolation**
Specify the extent of your isolation. Can be any combination of `desktop`, `network`, or `process`.
```json
{"desktop":true,"network":true,"process":true}
```

**examples**
```json
{
    "endpoints": [{ "id": "<endpoint_id>", "account_id": "<account_id>" }]
}
```
If you want to issue a job on all your `Mac Os` endpoints across two of your manages sites, and on all your endpoints for a certain account with policy matching the string `serious policy`, do the following:

```json
    "groups": [{
        "account_ids": ["<account_id_a>", "<account_id_b>"],
        "os_info.os_platform": "Mac Os"
    }, {
        "account_ids": ["<account_id_a>"],
        "policy_name": "serious policy"
    }]
```
The constraints accepted are the same accepted by the search endpoints route. Refer to `POST -> /endpoints` for more information about the constraints accepted.

You can also send a mix of explicit endpoint ids and queries in the same request:
```json
    "endpoints": [{ "id": "<endpoint_id>", "account_id": "<account_id>" }],
    "groups": [{
        "account_ids": ["<account_id_a>", "<account_id_b>"],
        "os_info.os_platform": "Mac Os"
    }, {
        "account_ids": ["<account_id_a>"],
        "policy_name": "serious policy"
    }]
```
The response includes the explicit ids on which the job was issued.
- **`threatdown-cli endpoint-jobs export`** - Export jobs data. You can export across multiple accounts. Specify your interested fields and the output format.

Supported output formats: csv, xlsx, json.

**Settings**
| Name | Description |
|------|-------------|
| groups | List of queries. The constraints accepted are the same accepted by the search jobs route. Refer to `POST -> /jobs/search` for more information about the constraints accepted. |
| download | If set to true, the server will set the `Content-Disposition` header using the `filename` and `format` |
| select | Which fields to select, and which field name to map to. See request schema for allowed values |
| type | Output encoding (for `csv` and `json` format) |
| filename | Name of the file to be set in `Content-Disposition` header if `download` = true |
- **`threatdown-cli endpoint-jobs get`** - Get a job and its full information by its job ID.
- **`threatdown-cli endpoint-jobs issue`** - Use this endpoint to issue a job on one or more of your endpoints within an account

Possible `command`s are:

| Name | Description |
|------|-------------|
| command.threat.scan | Issue a scan on the target endpoint (machine) |
| command.asset.reboot | Reboot an endpoint |
| command.service.restart | Restart the endpoint agent |
| command.protection.update.now | Check for protection updates |
| command.machine.update.now | Check for software updates |
| command.asset.refresh | Refresh asset information about an endpoint |
| command.edr.isolation | Isolate a machine |
| command.threat.quarantine.remove | Delete item from quarantine |
| command.threat.quarantine.restore | Restore item from quarantine |
| command.bfp.rules.delete | Delete Active Block Rules |
| command.service.diag | Generate Diagnostic Logs |
| command.edr.fileupload | File Upload for Analysis |
| command.edr.unlock.force | Force Remove Endpoint Isolation |
| command.edr.lock | Isolating Endpoint |
| command.sequence.remediate | Remediated Endpoint |
| command.edr.unlock | Remove Endpoint Isolation |
| command.logging.level.set | Endpoint Logging Level Changed |
| command.threat.scan.remediate | Remediate Endpoint |
| command.policy.refresh | Refresh Policy |
| command.machine.plugin.updateparts | Install Software Updates |
| command.asset.patch | Install Software Patch |
| command.asset.updatesoftware | Install Software Update |
| command.asset.uninstallsoftware | Uninstall Software |

Jobs are issued asynchronously and processed by endpoint agents when they get online. If you want to get notifications about the progress of a job, create a subscription to the `job_status_change` webhook event.


**Command specific data**
You can customize each command using specific settings. Supply those command-specific settings in the `data` parameter.

**command.threat.quarantine.restore**
Restore item from quarantine. Specify one or more detection ids
```json
{"restore_items":["7021ebec-b7b8-49b3-8709-f36bf5002c30"]}
```

**command.threat.quarantine.remove**
Delete item from quarantine. Specify one or more detection ids
```json
{"delete_items":["7021ebec-b7b8-49b3-8709-f36bf5002c30"]}
```

**command.threat.scan**
If you want to just report detections:
```json
{"scan_settings":{"type":"ThreatScan","remove":false}}
```

If you want to quarantine detected items:
```json
{"scan_settings":{"type":"ThreatScan","remove":true}}
```

**command.edr.isolation**
Specify the extent of your isolation. Can be any combination of `desktop`, `network`, or `process`.
```json
{"desktop":true,"network":true,"process":true}
```

**command.asset.patch**
Install Software Patch
```json
{
  "patches": [
    {
      "category": "feature_pack",
      "product": "Silverlight",
      "severity": "critical",
      "title": "Microsoft Silverlight (KB4481252)",
      "vendor": "Microsoft Corporation"
    }
  ]
}
```

**command.asset.updatesoftware**
Install Software Update
```json
{
  "application_name": "Chrome",
  "current_version": "96.0.4664.45",
  "new_version": "97.0.2103.01",
  "product": 41,
  "vendor": "Google",
  "architecture": "x86",
  "language": "en-US"
}
```

**command.asset.uninstallsoftware**
Uninstall Software. Send a list of product_signature
```json
{
  "applications": [42,1024]
}
```

**Important considerations about this endpoint**
For performance reasons, if you supply more than 100 endpoint ids (`machine_ids`), you will not receive the list of job ids in the response. If you need to track the issued jobs, batch endpoints in groups smaller than 100 items.
- **`threatdown-cli endpoint-jobs parent-cancel`** - Cancel parent jobs.
- **`threatdown-cli endpoint-jobs parent-children-cancel`** - Cancel children jobs by correlation ID across multiple accounts.
- **`threatdown-cli endpoint-jobs parent-children-reissue`** - Reissue children jobs.
- **`threatdown-cli endpoint-jobs parent-children-search`** - Search children jobs by correlation ID across multiple accounts.
- **`threatdown-cli endpoint-jobs parent-children-summary`** - Get children jobs summary by correlation ID across multiple accounts.
- **`threatdown-cli endpoint-jobs parent-export`** - Export parent jobs.
- **`threatdown-cli endpoint-jobs parent-get`** - Get parent job by ID across multiple accounts.
- **`threatdown-cli endpoint-jobs parent-reissue`** - Reissue jobs by correlation IDs.
- **`threatdown-cli endpoint-jobs parent-search`** - Search parent jobs across multiple accounts.
- **`threatdown-cli endpoint-jobs parent-summary`** - Get parent jobs summary across multiple accounts.
- **`threatdown-cli endpoint-jobs quarantine-bulk`** - Jobs operation the vendor has not documented: POST /oneview/v1/accounts/{account_id}/jobs/quarantine/bulk
- **`threatdown-cli endpoint-jobs search`** - Search jobs across all your managed accounts. The search will be scoped to the account_ids you specify in the request body.
- **`threatdown-cli endpoint-jobs search-groupby`** - Search jobs grouped by a specific field across all your managed account.

### endpoints

An Endpoint is a device which has the ThreatDown Endpoint Agent installed.

- **`threatdown-cli endpoints agent-info`** - Retrieve agent information of a given endpoint using its endpoint ID.
- **`threatdown-cli endpoints assets`** - Retrieve asset information of a given endpoint using its endpoint ID.
- **`threatdown-cli endpoints delete`** - Delete an endpoint from the console using the endpoint ID.
- **`threatdown-cli endpoints delete-bulk`** - Use this endpoint to mark machines as deleted.
This API endpoint allows you to specify queries instead of explicitly specifying endpoint ids. See example for more details.

> **Be extremely cautious**: If you don't specify any constraint, all your machines will match the search and be deleted. For example, the following payload:
```json
    "groups": [{ 
        "account_ids": ["<account_id_a>", "<account_id_b>"]
    }]
```
will select all the machines in the two accounts. Always test the result of your query using the [Search endpoints APIs](#operation/api.v2.rmm.post.endpoints)



If you want to mark all of your `Mac Os` endpoints as deleted across two of your manages sites, and on all your endpoints for a certain account with policy matching the string `serious policy`, do the following:

```json
    "groups": [{ 
        "account_ids": ["<account_id_a>", "<account_id_b>"],
        "os_info.os_platform": "Mac Os"
    }, {
        "account_ids": ["<account_id_a>"],
        "policy_name": "serious policy"    
    }]
```
The constraints accepted are the same accepted by the search endpoints route. Refer to `POST -> /endpoints` for more information about the constraints accepted.

You can also send a mix of explicit endpoint ids and queries in the same request: 
```json
    "endpoints": [{ "id": "<endpoint_id>", "account_id": "<account_id>" }],
    "groups": [{ 
        "account_ids": ["<account_id_a>", "<account_id_b>"],
        "os_info.os_platform": "Mac Os"
    }, {
        "account_ids": ["<account_id_a>"],
        "policy_name": "serious policy"    
    }]
```
- **`threatdown-cli endpoints diag-log`** - Generate a diagnostic log for an endpoint using its ID.
- **`threatdown-cli endpoints export`** - Export endpoint data across multiple accounts. Specify your interested fields and the output format.

Supported output formats: csv, xlsx, json.

**Settings**
| Name | Description |
|------|-------------|
| groups | List of queries. The constraints accepted are the same accepted by the search endpoints route. Refer to `POST -> /endpoints` for more information about the constraints accepted. |
| endpoints | List of explicit endpoint ids. Accounts ids are required along with the endpoint ids |
| populate | Whether to populate the response with additional data. Currently supported `account` and `last_job` |
| download | If set to true, the server will set the `Content-Disposition` header using the `filename` and `format` |
| select | Which fields to select, and which field name to map to. See request schema for allowed values |
| type | Output encoding (for `csv` and `json` format) |
| filename | Name of the file to be set in `Content-Disposition` header if `download` = true |

**Examples**
**Download HTML file**
A request with this body sent by a browser will download a file called `nameofthefile.html`, displaying a table with columns: `Deleted`, `Task`, `ID`, `Name` for all `windows` endpoints within account id `1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34` and the endpoint with id `211a4cfc-557e-4ba7-a96f-70685a07c29e`:

```json
	{
		"groups": [{
			"account_ids": ["1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34"],
			"os_info.os_platform": "windows"
		}],
		"endpoints": [{ "id": "211a4cfc-557e-4ba7-a96f-70685a07c29e", "account_id": "1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34"}],
		"populate": { "account": true, "last_job": true },
		"format": "html",
		"type": "string",
		"filename": "nameofthefile",
		"download": true,
		"select": [{
			"field": "machine.is_deleted",
			"newField": "Deleted"
		},{
			"field": "machine.job.status",
			"newField": "Task"
		}, {
			"field": "machine.id",
			"newField": "ID"
		}, {
			"field": "machine.account.name",
			"newField": "Name"
		}]
	}
```

**Download xlsx file**
A request with this body sent by a browser will download a file called `report.html`, displaying a table with columns: `Deleted`, `Task`, `ID`, `Name` for all `windows` endpoints within account id `1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34` and the endpoint with id `211a4cfc-557e-4ba7-a96f-70685a07c29e`:

```json
	{
		"groups": [{
			"account_ids": ["1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34"],
			"os_info.os_platform": "windows"
		}],
		"endpoints": [{ "id": "211a4cfc-557e-4ba7-a96f-70685a07c29e", "account_id": "1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34"}],
		"populate": { "account": true, "last_job": true },
		"format": "xlsx",
		"type": "base64",
		"filename": "report",
		"download": true,
		"select": [{
			"field": "machine.is_deleted",
			"newField": "Deleted"
		},{
			"field": "machine.job.status",
			"newField": "Task"
		}, {
			"field": "machine.id",
			"newField": "ID"
		}, {
			"field": "machine.account.name",
			"newField": "Name"
		}]
	}
```

**Get JSON with all ids for programmatic usage**
A request with the following body sent by a browser will respond with a json body containing an array of objects with the following properties: `Deleted`, `Task`, `ID`, `Name` for all `windows` endpoints within account id `1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34` and the endpoint with id `211a4cfc-557e-4ba7-a96f-70685a07c29e`:

```json
	{
		"groups": [{
			"account_ids": ["1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34"],
			"os_info.os_platform": "windows"
		}],
		"endpoints": [{ "id": "211a4cfc-557e-4ba7-a96f-70685a07c29e", "account_id": "1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34"}],
		"populate": { "account": true, "last_job": true },
		"format": "json",
		"type": "string",
		"select": [{
			"field": "machine.is_deleted",
			"newField": "Deleted"
		},{
			"field": "machine.job.status",
			"newField": "Task"
		}, {
			"field": "machine.id",
			"newField": "ID"
		}, {
			"field": "machine.account.name",
			"newField": "Name"
		}]
	}
```

*Example response*
```
[
    {
        "Deleted": false,
        "Task": "PENDING",
        "ID": "211a4cfc-557e-4ba7-a96f-70685a07c29e",
        "Name": "Postman Test Account2040c322-c98a-4e98-bb67-9d3f94ff1867"
    },
    {
        "Deleted": false,
        "Task": "PENDING",
        "ID": "211a4cfc-557e-4ba7-a96f-70685a07c29e",
        "Name": "Postman Test Account2040c322-c98a-4e98-bb67-9d3f94ff1867"
    },
    {
        "Deleted": false,
        "Task": "PENDING",
        "ID": "49a92495-ff82-433d-b1ec-6e49376ddf06",
        "Name": "Postman Test Account2040c322-c98a-4e98-bb67-9d3f94ff1867"
    },
    {
        "Deleted": false,
        "ID": "cb35a2a2-e391-4fb5-b0d2-11693797c648",
        "Name": "Postman Test Account2040c322-c98a-4e98-bb67-9d3f94ff1867"
    },
    {
        "Deleted": false,
        "ID": "df974ca9-229c-40f2-93fe-a29bde6f77c8",
        "Name": "Postman Test Account2040c322-c98a-4e98-bb67-9d3f94ff1867"
    },
    {
        "Deleted": false,
        "ID": "e7093336-cd45-4492-bb0f-8bebc62db0c3",
        "Name": "Postman Test Account2040c322-c98a-4e98-bb67-9d3f94ff1867"
    }
]
```
- **`threatdown-cli endpoints export-async`** - Export endpoints asynchronously
- **`threatdown-cli endpoints get`** - Retrieve information of a single endpoint by its universally unique identifier.
- **`threatdown-cli endpoints network`** - Get network information about an endpoint using its endpoint ID.
- **`threatdown-cli endpoints quarantine`** - Get quarantined items by the endpoint's ID
- **`threatdown-cli endpoints sa-close`** - Close suspicious activity of an endpoint by its endpoint ID and suspicious activity ID.
- **`threatdown-cli endpoints sa-exclusions`** - Fetch suspicious activity of an endpoint using the endpoint ID and suspicious activity ID.
- **`threatdown-cli endpoints sa-open`** - Open suspicious activity of an endpoint by its endpoint ID and suspicious activity ID.
- **`threatdown-cli endpoints sa-remediate`** - Remediate suspicious activity of an endpoint using the endpoint ID and suspicious activity ID.
- **`threatdown-cli endpoints scan`** - Get a scan of an endpoint by ID.
- **`threatdown-cli endpoints scan-detections`** - Get a list of detections of a scan of an endpoint.
- **`threatdown-cli endpoints scans`** - Get a list of scans of an endpoint by its endpoint ID.
- **`threatdown-cli endpoints search`** - Search endpoints across all your managed accounts. The search will be scoped to the account_ids you specify in the request body. Constraints with the suffix `keyword` indicates an exact match. Constraints without the `keyword` suffix will be tokenized.

**Examples**
If you want to find endpoints belonging to the `Default group` (the search is case insensitive), use `group_name.keyword` as a constraint.
```json
{
    "account_ids": ["<account>"],
    "group_name.keyword": "default group"
}
```
Alternatively, if you want to find endpoints which group includes `department_one OR department_two`, use `group_name` as a constraint.
```json
{
    "account_ids": ["<account>"],
    "group_name": "department_one department_two department_three"
}
```
- **`threatdown-cli endpoints search-groupby`** - Search grouped endpoints
- **`threatdown-cli endpoints set-tags`** - Add an alias to an endpoint by its id. To remove the alias, send an empty string
- **`threatdown-cli endpoints site-search`** - Search endpoints within an account. The search scopes to the account_id you specify in the URL. Constraints with the suffix `keyword` indicates an exact match. Constraints without the `keyword` suffix will be tokenized.

**Examples**
If you want to find endpoints belonging to the `Default group` (the search is case insensitive), use `group_name.keyword` as a constraint.
```json
{
    "group_name.keyword": "default group"
}
```
Alternatively, if you want to find endpoints which group includes `department_one OR department_two`, use `group_name` as a constraint.
```json
{
    "group_name": "department_one department_two department_three"
}
```
- **`threatdown-cli endpoints status`** - Get endpoint status by its ID
- **`threatdown-cli endpoints suspicious-activity`** - Fetch suspicious activity of an endpoint using its endpoint ID.

### events

Events

- **`threatdown-cli events <account_id>`** - An event is a record of activity on your account, such as found a threat, completed a scan, or removed endpoint agent. Retrieve events associated with your account.

### exclusions

Exclusions allow you to prevent trusted applications, websites, and services from being detected by our security engine.

- **`threatdown-cli exclusions create`** - Use this API endpoint to add exclusions.

**Validation rules**
- type must be a number from the following list: 1, 2, 3, 4, 5, 6, 7, 8, 13, 14
- exploit_protection can only be true if type is 13
- ransomware_protection can only be true if type is 1 or 2
- malware_protection can only be true if type is 1, 2, 4, or 5
- website_protection can only be true if type is 6, 7, or 8
- bruteforce_protection can only be true if type is 7
- if type is 14, only suspicious_activity can and must be true
- if account_level is false, then policy_ids must be an array of UUID(s)
- value must be a valid domain or website (domain with protocol prefix) if type is 6
- value must be a valid IP address if type is 7
- **`threatdown-cli exclusions delete`** - Delete exclusion by the exclusion ID.
- **`threatdown-cli exclusions get`** - Get details of an exclusion by the exclusion ID
- **`threatdown-cli exclusions list`** - Get all exclusions
- **`threatdown-cli exclusions update`** - Update an exclusion by the exclusion ID

### firewall

Manage firewall

- **`threatdown-cli firewall policies-assign-policies`** - Set policies for firewall policy
- **`threatdown-cli firewall policies-create`** - Create a new firewall policy
- **`threatdown-cli firewall policies-delete`** - Delete a firewall policy
- **`threatdown-cli firewall policies-get`** - Get a firewall policy
- **`threatdown-cli firewall policies-history`** - Get firewall policy history
- **`threatdown-cli firewall policies-list`** - Get firewall policies
- **`threatdown-cli firewall policies-remove-rulesets`** - Delete rulesets from firewall policy
- **`threatdown-cli firewall policies-set-rulesets`** - Set rulesets for firewall policy
- **`threatdown-cli firewall policies-unassign-policies`** - Delete policies from firewall policy
- **`threatdown-cli firewall policies-update`** - Update a firewall policy
- **`threatdown-cli firewall rules-create`** - Create a new firewall rule
- **`threatdown-cli firewall rules-delete`** - Delete a firewall rule
- **`threatdown-cli firewall rules-get`** - Get a firewall rule
- **`threatdown-cli firewall rules-list`** - Get firewall rules
- **`threatdown-cli firewall rules-update`** - Update a firewall rule
- **`threatdown-cli firewall rulesets-create`** - Create a new firewall ruleset
- **`threatdown-cli firewall rulesets-delete`** - Delete a firewall ruleset
- **`threatdown-cli firewall rulesets-get`** - Get a firewall ruleset
- **`threatdown-cli firewall rulesets-list`** - Get firewall rulesets
- **`threatdown-cli firewall rulesets-remove-rules`** - Delete rules from firewall ruleset
- **`threatdown-cli firewall rulesets-set-rules`** - Set rules for firewall ruleset
- **`threatdown-cli firewall rulesets-update`** - Update a firewall ruleset

### flight-recorder

EDR customers can use Flight Recorder to search event data captured on endpoints that have suspicious activity monitoring enabled.

- **`threatdown-cli flight-recorder chart`** - Search occurrence of suspicious activity. Wildcards are not supported.
- **`threatdown-cli flight-recorder endpoints`** - Search flight recorder for endpoints that had processes with the selected parameters.
- **`threatdown-cli flight-recorder process-graph`** - Search the process graph.
- **`threatdown-cli flight-recorder processes`** - Retrieve a list of processes matching the search input for the specified endpoint.
- **`threatdown-cli flight-recorder raw-events`** - Retrieve a list of raw events matching the given filters for the specified endpoint.
- **`threatdown-cli flight-recorder recap`** - Search suspicious activity. Wildcards are not supported.
- **`threatdown-cli flight-recorder search`** - Search the flight recorder with advanced queries.

### grid

Using the following API, you can search endpoints, detections, software inventory, vulnerabilities, rid rules, os-patches, device control events and dns logs.

- **`threatdown-cli grid action-logs-export`** - Export action logs
- **`threatdown-cli grid action-logs-export-async`** - Export action logs asynchronously
- **`threatdown-cli grid action-logs-search`** - Search action logs
- **`threatdown-cli grid action-logs-search-groupby`** - Search grouped action logs
- **`threatdown-cli grid ai-domains-export`** - Grid operation the vendor has not documented: POST /oneview/v1/grid/ai-domains/export
- **`threatdown-cli grid ai-domains-export-async`** - Grid operation the vendor has not documented: POST /oneview/v1/grid/ai-domains/export/async
- **`threatdown-cli grid ai-domains-search`** - Grid operation the vendor has not documented: POST /oneview/v1/grid/ai-domains/search
- **`threatdown-cli grid ai-domains-search-groupby`** - Grid operation the vendor has not documented: POST /oneview/v1/grid/ai-domains/search-groupby
- **`threatdown-cli grid assets-software-export`** - Export software assets across accounts
- **`threatdown-cli grid assets-software-export-async`** - Export software assets across accounts
- **`threatdown-cli grid assets-software-search`** - Search software assets across accounts
- **`threatdown-cli grid assets-software-search-groupby`** - Search grouped software assets across accounts
- **`threatdown-cli grid cve-events-export`** - Grid operation the vendor has not documented: POST /oneview/v1/grid/cve/events/export
- **`threatdown-cli grid cve-events-search`** - Grid operation the vendor has not documented: POST /oneview/v1/grid/cve/events/search
- **`threatdown-cli grid cve-events-search-groupby`** - Grid operation the vendor has not documented: POST /oneview/v1/grid/cve/events/search-groupby
- **`threatdown-cli grid cve-export`** - Export CVEs across accounts
- **`threatdown-cli grid cve-export-async`** - Export CVEs across accounts
- **`threatdown-cli grid cve-search`** - Search CVEs across accounts
- **`threatdown-cli grid cve-search-groupby`** - Search grouped CVEs across accounts
- **`threatdown-cli grid detections-export`** - Export detections across accounts
- **`threatdown-cli grid detections-export-async`** - Export detections across accounts
- **`threatdown-cli grid detections-search`** - Search detections across accounts
- **`threatdown-cli grid detections-search-groupby`** - Search grouped detections across accounts
- **`threatdown-cli grid device-control-export`** - Search DNS activity across accounts
- **`threatdown-cli grid device-control-export-async`** - Export device control events asynchronously
- **`threatdown-cli grid device-control-search`** - Search device control events across accounts
- **`threatdown-cli grid device-control-search-groupby`** - Search grouped device control events across accounts
- **`threatdown-cli grid dns-export`** - Export DNS activity across accounts
- **`threatdown-cli grid dns-export-async`** - Export DNS activity across accounts
- **`threatdown-cli grid dns-search`** - Search DNS activity across accounts
- **`threatdown-cli grid dns-search-groupby`** - Search grouped DNS activity across accounts
- **`threatdown-cli grid endpoints-delete`** - Delete endpoints across accounts
- **`threatdown-cli grid endpoints-export`** - Export endpoints across accounts
- **`threatdown-cli grid endpoints-export-async`** - Export endpoints across accounts
- **`threatdown-cli grid endpoints-search`** - Search endpoints across accounts
- **`threatdown-cli grid endpoints-search-groupby`** - Search grouped endpoints across accounts
- **`threatdown-cli grid events-export`** - Export event data
- **`threatdown-cli grid events-export-async`** - Export events asynchronously
- **`threatdown-cli grid events-search`** - Search events
- **`threatdown-cli grid events-search-groupby`** - Search and group events
- **`threatdown-cli grid firewall-export`** - Export firewall activity
- **`threatdown-cli grid firewall-search`** - Search firewall activity
- **`threatdown-cli grid firewall-search-groupby`** - Search grouped firewall activity
- **`threatdown-cli grid groups-assign-endpoints`** - Move endpoints between groups across accounts
- **`threatdown-cli grid jobs-bulk`** - Issue jobs to endpoints across accounts
- **`threatdown-cli grid os-patches-export`** - Export OS Patches across accounts
- **`threatdown-cli grid os-patches-export-async`** - Export OS Patches across accounts
- **`threatdown-cli grid os-patches-search`** - Search OS Patches across accounts
- **`threatdown-cli grid os-patches-search-groupby`** - Search grouped OS Patches across accounts
- **`threatdown-cli grid rid-export`** - Export RID rules across accounts
- **`threatdown-cli grid rid-export-async`** - Export RID rules across accounts
- **`threatdown-cli grid rid-search`** - Search RID rules across accounts
- **`threatdown-cli grid rid-search-groupby`** - Search grouped RID rules across accounts
- **`threatdown-cli grid xdr-alerts-export`** - Export XDR Alerts across accounts
- **`threatdown-cli grid xdr-alerts-export-async`** - Export XDR Alerts across accounts
- **`threatdown-cli grid xdr-alerts-search`** - Search XDR Alerts across accounts
- **`threatdown-cli grid xdr-alerts-search-groupby`** - Search grouped XDR Alerts across accounts

### groups

Groups in OneView are used to contain and organize endpoints.

- **`threatdown-cli groups assign-endpoints`** - Use this endpoint to move machines in a group.
This API endpoint allows you to specify queries instead of explicitly specifying endpoint ids. See example for more details.

To assign all your `Mac Os` endpoints across two of your manages sites to a group, and on all your endpoints for a certain account with policy matching the string `serious policy`, use the following body:

```json
    "groups": [{ 
        "account_ids": ["<account_id_a>", "<account_id_b>"],
        "os_info.os_platform": "Mac Os"
    }, {
        "account_ids": ["<account_id_a>"],
        "policy_name": "serious policy"    
    }]
```
The constraints accepted are the same accepted by the search endpoints route. Refer to `POST -> /endpoints` for more information about the constraints accepted.

You can also send a mix of explicit endpoint ids and queries in the same request: 
```json
    "endpoints": [{ "id": "<endpoint_id>", "account_id": "<account_id>" }],
    "groups": [{ 
        "account_ids": ["<account_id_a>", "<account_id_b>"],
        "os_info.os_platform": "Mac Os"
    }, {
        "account_ids": ["<account_id_a>"],
        "policy_name": "serious policy"    
    }]
```
- **`threatdown-cli groups create`** - Create group.
- **`threatdown-cli groups delete`** - Delete group by the Group id.
- **`threatdown-cli groups get`** - Get group by the Group id.
- **`threatdown-cli groups list`** - Get all groups in an account.
- **`threatdown-cli groups search`** - Filter groups across accounts
- **`threatdown-cli groups update`** - Update group.

### installation-tokens

Use these APIs to generate, send, and revoke installation tokens used to activate Mobile Security for Business.

- **`threatdown-cli installation-tokens create`** - Generate an installation token to be used. The token will be only visible during this operation.
- **`threatdown-cli installation-tokens revoke`** - Revoke an installation token that has not been used yet.
- **`threatdown-cli installation-tokens search`** - List the installation tokens generated or used across multiple accounts
- **`threatdown-cli installation-tokens send`** - Send installation tokens to batches of emails

### installers

The Installers APIs allow you to deploy the endpoint agent to Windows and macOS devices.

- **`threatdown-cli installers <account_id>`** - Endpoint software can be installed in the following ways:

- Active Directory (AD) Group Policy
- System Center Configuration Manager (SCCM)
- Third-party deployment tools
- Manually on the endpoints
- Discovery & Deployment Tool
- Once the installation is complete, the endpoints display in the Nebula console.

To retrieve account-specific installers, use the following API endpoints. The response includes a download URL for all the available installers. These download URLs expire in one hour.

The available installers are:

| Name        | Description                                                                                                                                                               |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| win_msi     | This .msi installation package is lightweight. It includes the 32-bit Endpoint Agent, but does not include the .NET framework or other required software.                 |
| x64_msi     | This .msi installation package is lightweight. It includes the 64-bit Endpoint Agent, but does not include the .NET framework or other required software.                 |
| ea2_arm_msi | This .msi installation package is for Windows ARM devices running Windows 11. It includes the ARM Endpoint Agent and .NET 8 runtime.                                      |
| ea2_x64_msi | This .msi installation package is for the Endpoint Agent version 2. It includes the 64-bit Endpoint Agent and .NET 8 runtime.                                             |
| ea2_x86_msi | This .msi installation package is for the Endpoint Agent version 2. It includes the 32-bit Endpoint Agent and .NET 8 runtime.                                             |
| mac_os      | macOS 11 Big Sur to macOS 27 Golden Gate. This .pkg installer package includes the Endpoint Agent.                                                                   |

Please note: the hash fields remain to not change the response, however they will be empty strings. This is due to a change in installer generation.
If you require the installer's hash, check the response header under 'x-amz-meta-x-checksum-sha256'.
win_full and win_web have been deprecated but will remain in the response.

### notifications

This API lets you create notification subscriptions.

- **`threatdown-cli notifications bulk-delete`** - Delete multiple notification subscriptions in bulk.
- **`threatdown-cli notifications bulk-update`** - Update multiple notification subscriptions in bulk.
- **`threatdown-cli notifications change-owner`** - Change notification owner.
- **`threatdown-cli notifications create`** - Create notification subscription.
- **`threatdown-cli notifications delete`** - Delete notification by subscription id.
- **`threatdown-cli notifications delete-by-owner`** - Delete all notifications owned by a given owner.
- **`threatdown-cli notifications get`** - Get notification by subscription id.
- **`threatdown-cli notifications list`** - Get all notification subscriptions.
- **`threatdown-cli notifications remove-email`** - Remove an email address from all the recipients of the notification subscriptions of the account. If the subscription had only one delivery method and 1 recipient, delete the subscription.
- **`threatdown-cli notifications update`** - Update notification subscription.

### os-patches

OS Patches

- **`threatdown-cli os-patches export`** - Export OS Patch data. You can export across multiple accounts. Specify your interested fields and the output format.

Supported output formats: csv, xlsx, json.

**Settings**
| Name | Description |
|------|-------------|
| groups | List of queries. The constraints accepted are the same accepted by the search os patches route. Refer to [Search OS Patches API](#operation/api.v2.rmm.search.os-patches) for more information about the constraints accepted. |
| download | If set to true, the server will set the `Content-Disposition` header using the `filename` and `format` |
| select | Which fields to select, and which field name to map to. See request schema for allowed values |
| type | Output encoding (for `csv` and `json` format) |
| filename | Name of the file to be set in `Content-Disposition` header if `download` = true |

**Examples**
**Download HTML file**
A request with this body sent by a browser will download a file called `nameofthefile.html`, displaying a table with columns: `KB Id`, `Category`, `Description` for all detections on `windows` endpoints:

```json
	{
		"groups": [{
			"os_info.os_platform": "Windows"
		}],
		"format": "html",
		"type": "string",
		"filename": "nameofthefile",
		"download": true,
		"select": [
            {"field": "patch_id", "newField": "KB Id"},
            {"field": "category", "newField": "Category"},
            {"field": "description", "newField": "Description"}
        ]
	}
```
- **`threatdown-cli os-patches export-async`** - Export OS Patches asynchronously
- **`threatdown-cli os-patches search`** - Search OS Patches.
- **`threatdown-cli os-patches search-groupby`** - Search OS Patches grouped by a specific field.

### policies

A policy is a set of configurations that determine how the endpoint agent monitors your endpoints, such as protection and scan settings.

- **`threatdown-cli policies auto-isolation-get`** - Get policy auto-isolation options by policy ID
- **`threatdown-cli policies auto-isolation-set`** - Set policy auto-isolation options by policy ID
- **`threatdown-cli policies clone`** - Clone policy by id of an existing policy.
- **`threatdown-cli policies create`** - Create new Policy
- **`threatdown-cli policies create-v2`** - Create policy.
- **`threatdown-cli policies delete`** - Delete policy. by its id
- **`threatdown-cli policies delete-v2`** - Delete policy.
- **`threatdown-cli policies expire-endpoints-disable`** - Configure policy not to expire when inactive
- **`threatdown-cli policies expire-endpoints-set`** - Set property to expire endpoints on a policy
- **`threatdown-cli policies get`** - Get one policy by its id
- **`threatdown-cli policies get-v2`** - Get policy by id.
- **`threatdown-cli policies isolation-image-add`** - This route allows you to add an icon to display on the custom message screen.

Other routes in this API accept `Content-Type: application/json`.

This route is meant for file uploads and expects the request following `Content-Type: multipart/form-data` structure.

Technically it is _form upload_ and the field for linking a file is called `metadata`.

File Requirements:

FILE TYPE: BMP
OPTIMAL FILE SIZE: 150PX BY 150PX
FILE SIZE: 2MB (maximum)

Example usage:

curl --location --request POST 'https://cloud.malwarebytes.com/api/v2/rmm/accounts/<account_id>/policies/<policy_id>/isolation_image' \
--header 'Authorization: Bearer <access_token>' \
--header 'Content-Type: multipart/form-data;' \
--form 'metadata=@<path_to_file>'
- **`threatdown-cli policies isolation-image-delete`** - Delete policy isolation image.
- **`threatdown-cli policies isolation-image-get`** - Get policy isolation image by id.
- **`threatdown-cli policies list`** - Get all policies
- **`threatdown-cli policies list-v2`** - Get all policies.
- **`threatdown-cli policies promote`** - Assign policies to site accounts
- **`threatdown-cli policies promote-v2`** - Promote policy.
- **`threatdown-cli policies search`** - Get policies of multiple accounts
- **`threatdown-cli policies secret-get`** - Get policy tamper protection password
- **`threatdown-cli policies secret-get-v2`** - Get policy secret by id.
- **`threatdown-cli policies secret-set`** - Set policy tamper protection password
- **`threatdown-cli policies secret-set-v2`** - Change policy secret by id.
- **`threatdown-cli policies set-default`** - Mark policy as default
- **`threatdown-cli policies system-default`** - Get system default policy.
- **`threatdown-cli policies tamper-protection`** - Returns the effective tamper protection level (off, policy or site) for a policy resolved in the context of a specific site, along with the site override flag and presence indicators. The level is resolved against the site (child account) where the machine is installed, since a global policy can be owned by a parent account but the applied secret lives on the site. Use this before an uninstall to determine whether a passwordless uninstall is possible and, if not, which secret to retrieve. No secret value is returned.
- **`threatdown-cli policies update`** - Update policy. by its id
- **`threatdown-cli policies update-v2`** - Update policy.

### preferences

The Preferences APIs allow you to enable or disable all notifications of a specific type (email, webhook, slack, teams), without needing to modify or delete multiple notifications.

- **`threatdown-cli preferences account-logo-get`** - Get account logo preferences
- **`threatdown-cli preferences account-logo-update`** - Put account logo preferences
- **`threatdown-cli preferences notifications-get`** - Get notification delivery preferences.
- **`threatdown-cli preferences notifications-update`** - Modify notification delivery preferences.

### quarantine

When a harmful file is found on a device, it can be neutralized and placed in quarantine, preventing it from posing a threat.

- **`threatdown-cli quarantine export`** - Export quarantined items data. You can export only per one account at a time. Specify your interested fields and the output format.

Supported output formats: csv, xlsx, json.

**Settings**
| Name | Description |
|------|-------------|
| download | If set to true, the server will set the `Content-Disposition` header using the `filename` and `format` |
| select | Which fields to select, and which field name to map to. See request schema for allowed values |
| type | Output encoding (for `csv` and `json` format) |
| filename | Name of the file to be set in `Content-Disposition` header if `download` = true |

Currently the export API has a limit of 10000 items. Apply filters to narrow your search and get the desired results.

**Examples**
**Download HTML file**
A request with this body sent by a browser will download a file called `nameofthefile.html`, displaying a table with columns: `Path`, `Name`, `Machine ID`, `Machine Name` for all quarantind items:

```json
	{
		"format": "html",
		"type": "string",
		"filename": "nameofthefile",
		"download": true,
		"select": [{
			"field": "path",
			"newField": "Path"
		},{
			"field": "threat_name",
			"newField": "Name"
		}, {
			"field": "machine_id",
			"newField": "Machine ID"
		}, {
			"field": "machine_name",
			"newField": "Machine Name"
		}]
	}
```
- **`threatdown-cli quarantine list`** - Get all quarantined items scoped by account. Refer to the Jobs apis to restore and delete items from quarantine
- **`threatdown-cli quarantine search`** - Search all quarantined items scoped by (parent) account. It's possible to add some constraints to filter the result

### reports

Reports

- **`threatdown-cli reports change-owner`** - Change reports owner.
- **`threatdown-cli reports create`** - Create report.
- **`threatdown-cli reports delete`** - Delete report by id.
- **`threatdown-cli reports delete-bulk`** - Delete reports bulk.
- **`threatdown-cli reports generate`** - Generate report.
- **`threatdown-cli reports get`** - Get report by id.
- **`threatdown-cli reports list`** - Get all reports of the OneView account.
- **`threatdown-cli reports update`** - Update report.

### rid

Manage rid

- **`threatdown-cli rid export`** - Export RID rules
- **`threatdown-cli rid export-async`** - Export RID rules asynchronously
- **`threatdown-cli rid search`** - Search RID rules
- **`threatdown-cli rid search-groupby`** - Search grouped RID

### schedules

Scheduled scans allow you to automate scans to run based on a certain schedule, rather than manually triggering a scan.

- **`threatdown-cli schedules create`** - Create schedule. you must use "Add groups for schedule" to add a group route.
- **`threatdown-cli schedules delete`** - Delete a schedule by its schedule id.
- **`threatdown-cli schedules get`** - Get schedule by the schedule id.
- **`threatdown-cli schedules groups`** - Add groups to a schedule.
- **`threatdown-cli schedules list`** - Get schedules. You must create a separate request "Get schedule by id" to get the assigned groups of a schedule.
- **`threatdown-cli schedules search`** - Get schedules of multiple accounts
- **`threatdown-cli schedules update`** - Update schedule. You must use "add groups for schedule" to update group route.

### sites

Create and manage sites through APIs.

- **`threatdown-cli sites by-account`** - Get one site by the unique field nebula account_id
- **`threatdown-cli sites create`** - Create a new OneView site
- **`threatdown-cli sites delete`** - Delete a OneView site
- **`threatdown-cli sites get`** - Get one site by its unique identifier
- **`threatdown-cli sites list`** - Get a list of all your managed sites
- **`threatdown-cli sites mdr-get`** - Get a site's MDR settings
- **`threatdown-cli sites mdr-set`** - Set a site's MDR settings
- **`threatdown-cli sites secret`** - Returns the site (child account) level tamper protection password for a OneView site. Use this when the effective tamper protection level is "site". Returns an error if no site password is set.
- **`threatdown-cli sites update`** - Update a OneView site

### software

Manage software

- **`threatdown-cli software export`** - Export assets software data. You can export across multiple accounts. Specify your interested fields and the output format.

Supported output formats: csv, xlsx, json.

**Settings**
| Name | Description |
|------|-------------|
| groups | List of queries. The constraints accepted are the same accepted by the search assets software route. Refer to [Search assets software API](#operation/api.v2.rmm.search.assets.software) for more information about the constraints accepted. |
| download | If set to true, the server will set the `Content-Disposition` header using the `filename` and `format` |
| select | Which fields to select, and which field name to map to. See request schema for allowed values |
| type | Output encoding (for `csv` and `json` format) |
| filename | Name of the file to be set in `Content-Disposition` header if `download` = true |

**Examples**
**Download HTML file**
A request with this body sent by a browser will download a file called `nameofthefile.html`, displaying a table with columns: `Path`, `Name`, `Machine ID`, `Machine Name` for all detections on `windows` endpoints within account id `1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34`:

```json
	{
		"groups": [{
			"account_ids": ["1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34"],
			"vendor": "Mozilla"
		}],
		"format": "html",
		"type": "string",
		"filename": "nameofthefile",
		"download": true,
		"select": [
            {"field": "accountId", "newField": "account_id"},
            {"field": "groupId", "newField": "groupId"},
            {"field": "vendor", "newField": "vendor"},
            {"field": "product", "newField": "product"},
            {"field": "installationDate", "newField": "installationDate"},
            {"field": "createdAt", "newField": "createdAt"}
        ]
	}
```

**Download xlsx file**
A request with this body sent by a browser will download a file called `report.html`, displaying a table with columns: `Path`, `Name`, `Machine ID`, `Machine Name` for all `windows` endpoints within account id `1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34` and the endpoint with id `211a4cfc-557e-4ba7-a96f-70685a07c29e`:

```json
	{
		"groups": [{
        	"account_ids": ["1e14cd0a-2ef3-4db3-b8a4-c89894d9aa34"],
        	"vendor": "Mozilla"
        }],
		"populate": { "account": true, "last_job": true },
		"format": "xlsx",
		"type": "base64",
		"filename": "report",
		"download": true,
		"select": [
            {"field": "accountId", "newField": "account_id"},
            {"field": "groupId", "newField": "groupId"},
            {"field": "vendor", "newField": "vendor"},
            {"field": "product", "newField": "product"},
            {"field": "installationDate", "newField": "installationDate"},
            {"field": "createdAt", "newField": "createdAt"}
        ]
	}
```
- **`threatdown-cli software export-async`** - Export assets software asynchronously
- **`threatdown-cli software search`** - Search assets software across all your managed account. The search will be scoped to the account_ids you specify in the request body.
- **`threatdown-cli software search-groupby`** - Search assets software grouped by a specific field across all your managed account.

### subscriptions

Subscriptions allow you to enable and manage ThreatDown security services for your customers by setting terms, volume and duration.

- **`threatdown-cli subscriptions addons-get`** - [v2 - À la carte] Get site's add-on subscriptions
- **`threatdown-cli subscriptions addons-update`** - Use this endpoint to add or modify a current site's add-on subscription
* Please note the product values are not static
* The product values are provided in the `availability` API under `add_ons` field
* If the user would like to update the seat count, simply increase the values here

**Request payload examples:**
Update a subscription with
* ThreatDown DNS Filtering and 5 seats
```
[
    {
        "product": "BUS-MBDNS-01",
        "term_type": "paid",
        "term_length": 0,
        "add_on_allocation": 5
    }
]
```
Increase allocation of the previous DNS subscription from 5 to 20
* ThreatDown DNS Filtering and 20 seats
```
[
    {
        "product": "BUS-MBDNS-01",
        "term_type": "paid",
        "term_length": 0,
        "add_on_allocation": 20
    }
]
```
Update a subscription with
* ThreatDown DNS Filtering and 20 seats
* ThreatDown Application Block and 15 seats
```
[
    {
        "product": "BUS-MBDNS-01",
        "term_type": "paid",
        "term_length": 0,
        "add_on_allocation": 20
    },
    {
        "product": "BUS-MBAB-01",
        "term_type": "paid",
        "term_length": 0,
        "add_on_allocation": 15
    }
]
```
To remove all add-on modules of a site, simply send an empty list
```
[]
```
- **`threatdown-cli subscriptions all`** - Get all subscriptions data by site id.
- **`threatdown-cli subscriptions availability-v2`** - Use this endpoint to retrieve the available subscriptions for a given MSP account

**Field descriptions**
**products**
| Field name                 | Data type or example       | Description                                                                                                               |
|----------------------------|----------------------------|---------------------------------------------------------------------------------------------------------------------------|
| product                    | `ir`, `ep`, `edr` or `mob` | Contains a list of product objects that an MSP is allowed to create or add to each site                                   |
| max_workstation_allocation | Integer                    | This indicates the number of maximum workstation allocation (`ir`, `ep`, `edr` or `mob`) allowed for the specific product |
| max_server_allocation      | Integer                    | This indicates the number of maximum server allocation allowed (`eps` or `edrs`) for the specific product                 |
| is_workstation_allowed     | Boolean                    | This indicated if a user may use `ws` as a `machine_type` when creating a subscription                                    |
| is_server_allowed          | Boolean                    | This indicated if a user may use `ws-ser` as a `machine_type` when creating a subscription                                |
| is_trial_only              | Boolean                    | This indicated if a user may use `trial` only as a `term_type` when creating a subscription                               |
| is_paid_only               | Boolean                    | This indicated if a user may use `paid` only as a `term_type` when creating a subscription                                |


**add_ons**
Contains a list of add-on objects that an MSP is able to add to their site subscriptions
* add-ons require an active product subscription to be added to a site

| Field name      | Data type or example                          | Description                                                                            |
|-----------------|-----------------------------------------------|----------------------------------------------------------------------------------------|
| product         | `BUS-MBVPM-01`, `BUS-MBDNS-01`, `BUS-MBVM-01` | The add-on name the user will need to send when adding the module                      |
| max_allocation  | Integer                                       | This indicates the number of maximum add-on allocation allowed for the specific module |
| is_trial_only   | Boolean                                       | This indicated if a user may use `trial` only as a `term_type`                         |
| is_paid_only    | Boolean                                       | This indicated if a user may use `paid` only as a `term_type`                          |

**is_nfr_allowed**
This field indicates if an MSP is allowed to add an NFR subscription to a site
* this field will automatically return false if there is already an active site with NFR subscription

**Response payload examples:**
In this example, the MSP is eligible to the add the following subscription to a site
* Endpoint Protection
  * workstation
  * server
* Endpoint Detection and Response
  * workstation
  * server
* Add-ons
  * Vulnerability and Patch Management
  * Vulnerability Assessment
* NFR
  * The MSP is eligible to create a subscription with NFR `term_type`

The MSP is **NOT** eligible to the following subscriptions to a site:
* Incident Response
  * workstation
* Mobile Security
* Add-ons
  * ThreatDown Application Block
  * ThreatDown Managed Detection and Response
  * ThreatDown DNS Filtering

Since the allocations are `-1` for all the products, we can determine that the current MSP is a `Usage` billed MSP
```
{
    "products":
    [
        {
            "product": "ep",
            "max_workstation_allocation": -1,
            "max_server_allocation": -1,
            "is_workstation_allowed": true,
            "is_server_allowed": true,
            "is_trial_only": false,
            "is_paid_only": false
        },
        {
            "product": "edr",
            "max_workstation_allocation": -1,
            "max_server_allocation": -1,
            "is_workstation_allowed": true,
            "is_server_allowed": true,
            "is_trial_only": false,
            "is_paid_only": false
        }
    ],
    "add_ons":
    [
        {
            "product": "BUS-MBVPM-01",
            "max_allocation": -1,
            "is_trial_only": false,
            "is_paid_only": true
        },
        {
            "product": "BUS-MBVM-01",
            "max_allocation": -1,
            "is_trial_only": false,
            "is_paid_only": true
        }
    ],
    "is_nfr_allowed": true
}
```
- **`threatdown-cli subscriptions availability-v3`** - Use this endpoint to retrieve the available **Bundle** subscriptions for a given MSP account

**Field descriptions**
**Bundles**
| Field name | Data type or example | Description                                                |
|------------|----------------------|------------------------------------------------------------|
| core       | Allocation Object    | Available seat allocation for `core` bundle per device     |
| advanced   | Allocation Object    | Available seat allocation for `advanced` bundle per device |
| elite      | Allocation Object    | Available seat allocation for `elite` bundle per device    |
| ultimate   | Allocation Object    | Available seat allocation for `ultimate` bundle per device |

**Allocation Object**
| Field name  | Data type or example | Description                                                |
|-------------|----------------------|------------------------------------------------------------|
| workstation | Integer              | Available `workstation` seat allocation for a given bundle |
| server      | Integer              | Available `server` seat allocation for a given bundle      |
| mobile      | Integer              | Available `mobile` seat allocation for a given bundle      |


**is_nfr_allowed**
This field indicates if an MSP is allowed to add an NFR subscription to a site
* this field will automatically return false if there is already an active site with NFR subscription

**Response payload examples:**
In this example, the MSP is eligible to add one of the following bundle subscriptions
* Core bundle
* Elite bundle
* Ultimate

With term_type `paid`, `trial` or `nfr`

The MSP is **NOT** eligible to the following subscriptions to a site:
* Core Bundle Server - server allocation count is 0
* Advanced Bundle - not listed in the response

```
{
    "is_nfr_allowed": true,
    "core":
    {
        "workstation": 995,
        "server": 0,
        "mobile": 998
    },
    "elite":
    {
        "workstation": 777,
        "server": 784,
        "mobile": 998
    },
    "ultimate":
    {
        "workstation": 441,
        "server": 415,
        "mobile": 998
    }
}
```
- **`threatdown-cli subscriptions create`** - Use this endpoint to create a subscription for one of your manages site. After creating one subscription, your customer's data will be populated with an `account_id` field that you can use for security's management apis, like detections, endpoints and jobs.

**Available catalog codes**
| Catalog Code     | Description                                 |
| ---------------- | ------------------------------------------- |
| BUS-CLOUD-IR-01  | ThreatDown Incident Response                |
| BUS-CLOUD-EPP-01 | ThreatDown Endpoint Protection              |
| BUS-CLOUD-EPR-01 | ThreatDown Endpoint Protection And Response |
- **`threatdown-cli subscriptions create-v2`** - Use this endpoint to create a subscription for one of your managed sites.

After creating one subscription, your customer's data will be populated with an `account_id` field that you can use for security's management apis, like detections, endpoints and jobs.

- This api can only be used when it does not have a current subscription
- If a site was created through the OneView console, it will likely have a subscription already
- If site has an active subscription, use the update subscription api
- For adding add-on modules, see add-ons api

Available options will be provided by another api

- https://oneview.malwarebytes.com/oauth/documentation/msp#operation/api.v2.oneview.availability.subscriptionv2.id

**Acceptable values for each field**
**product**
Determines the product for the subscription

| Value | Description                                |
| ----- | ------------------------------------------ |
| ir    | ThreatDown Incident Response               |
| ep    | ThreatDown Endpoint Endpoint Protection    |
| edr   | ThreatDown Endpoint Detection And Response |
| mob   | ThreatDown Mobile Security for Business    |

**machine_type**
Determines the machine type of the subscription

- field not required for mobile entitlements

| Value  | Description            |
| ------ | ---------------------- |
| ws     | Workstation only       |
| ws-ser | Workstation and Server |

**term_type**
Determines whether the subscription is paid or trial

| Value | Description        | Billable |
| ----- | ------------------ | -------- |
| paid  | Paid subscription  | yes      |
| trial | Trial subscription | no       |

**term_length**
Determines the term length of a subscription, most notable for trial subscriptions

| Value | Description                     |
| ----- | ------------------------------- |
| 0     | set to 0 for paid subscriptions |
| 15    | set to 15 for 15 day trial      |
| 30    | set to 30 for 30 daytrial       |

**allocations**
Determines the seat count allocated for a certain product subscription

- If MSP is on Upfront billing
  - **Required field**
  - Allocations here are limited by the seats purchased
- If MSP is on Usage billing
  - **Not Required**
  - allocation here does not reflect as billable

| Value | Description                   |
| ----- | ----------------------------- |
| ir    | allocation for IR             |
| ep    | allocation for EP             |
| edr   | allocation for EDR            |
| eps   | allocation for EP for Server  |
| edrs  | allocation for EDR for Server |
| mob   | allocation for Mobile         |

**Request payload examples:**
**Create a subscription with ThreatDown Endpoint Protection**

```
{
    "product": "ep",
    "machine_type": "ws",
    "term_type": "paid",
    "term_length": 0,
    "allocations": {
        "ep": 10
    }
}
```

**Create a subscription with ThreatDown Endpoint Protection and Server**

```
{
    "product": "ep",
    "machine_type": "ws-ser",
    "term_type": "paid",
    "term_length": 0,
    "allocations": {
        "ep": 15,
        "eps": 20
    }
}
```

**Create a subscription with ThreatDown Mobile Security**

```
{
    "product": "mob",
    "term_type": "paid",
    "term_length": 0,
    "allocations": {
        "mob": 5
    }
}
```

**Create an NFR subscription with ThreatDown Endpoint Protection and Server**

```
{
    "product": "ep",
    "machine_type": "ws-ser",
    "term_type": "nfr",
    "term_length": 0,
    "allocations": {
        "ep": 15,
        "eps": 20
    }
}
```
- **`threatdown-cli subscriptions create-v3`** - Use this endpoint to create a subscription for one of your managed sites.

After creating one subscription, your customer's data will be populated with an `account_id` field that you can use for security's management apis, like detections, endpoints and jobs.

- This api can be used to both create a subscription and update a subscription
- If a site was created through the OneView console, it will likely have a subscription already

Available options will be provided by another api

- https://oneview.malwarebytes.com/oauth/documentation/msp#operation/api.v2.oneview.availability.subscriptionv3.id

**Acceptable values for each field**
**bundle**
Determines the bundle for the subscription

| value    | Description                       |
|----------|-----------------------------------|
| core     | Apply `core` bundle to a site     |
| advanced | Apply `advanced` bundle to a site |
| elite    | Apply `elite` bundle to a site    |
| ultimate | Apply `ultimate` bundle to a site |

**auto_convert_trial_to_paid**
Determines if a trial subscription will auto convert to paid. Set to `false` on paid subscriptions

**devices**
A list of device object containing the device type, number of seats and term type (paid, trial or nfr)

**Device Object fields**
| Field name      | Data type or example | Acceptable Values                    |
|-----------------|----------------------|--------------------------------------|
| type            | String               | `workstation`, `server` and `mobile` |
| seat_allocation | Integer              | Any integer limited by availability  |
| term_type       | String               | `paid`, `trial` or `nfr`             |


**Request payload examples:**
**Create a Paid ThreatDown Core Bundle with 511 workstation allocation and 2 mobile allocation**

```
{
    "bundle": "core",
    "auto_convert_trial_to_paid": false,
    "devices":
    [
        {
            "type": "workstation",
            "seat_allocation": 511,
            "term_type": "paid"
        },
        {
            "type": "mobile",
            "seat_allocation": 2,
            "term_type": "paid"
        }
    ]
}
```

**Create a Trial ThreatDown Elite Bundle with 10 workstation allocation and 10 server allocation with auto conversion to paid at the end of trial period**

```
{
    "bundle": "elite",
    "auto_convert_trial_to_paid": true,
    "devices":
    [
        {
            "type": "workstation",
            "seat_allocation": 10,
            "term_type": "trial"
        },
        {
            "type": "server",
            "seat_allocation": 10,
            "term_type": "trial"
        }
    ]
}
```

**Remove site bundle subscription**

```
{
    "bundle": "core",
    "auto_convert_trial_to_paid": false,
    "devices": []
}
```
- **`threatdown-cli subscriptions delete`** - Delete your site subscriptions. This will mark the subscription as expired
- **`threatdown-cli subscriptions entitlements`** - Get all entitlements
- **`threatdown-cli subscriptions expiry-report`** - Get information about your customer subscriptions
- **`threatdown-cli subscriptions extend`** - Extend a trial for an existing site.

Despite 404s often being a non-recoverable error, on this endpoing, 404s are sometimes recoverable. In this case, setting a retry limit (for example 3 times) is suggested instead of not retrying when a 404 is received.
- **`threatdown-cli subscriptions get`** - Get subscription data by site id.
- **`threatdown-cli subscriptions get-v2`** - Get active subscriptions for an existing site.
- **`threatdown-cli subscriptions get-v3`** - Use this endpoint to fetch the subscription for one of your managed sites.

**bundle**
Determines the bundle for the subscription

| value    | Description                       |
|----------|-----------------------------------|
| core     | Apply `core` bundle to a site     |
| advanced | Apply `advanced` bundle to a site |
| elite    | Apply `elite` bundle to a site    |
| ultimate | Apply `ultimate` bundle to a site |

**auto_convert_trial_to_paid**
Determines if a trial subscription will auto convert to paid. Set to `false` on paid subscriptions

**devices**
A list of device object containing the device type, number of seats and term type (paid, trial or nfr)

**Device Object fields**
| Field name      | Data type or example | Acceptable Values                          |
|-----------------|----------------------|--------------------------------------------|
| type            | String               | `workstation`, `server` and `mobile`       |
| seats_allocated | Integer              | Seat allocated on subscribe                |
| seat_used       | Integer              | Seat being utilized by installed endpoints |
| term_type       | String               | `paid`, `trial` or `nfr`                   |
| status          | String               | `active` or `expired` for trials           |
| term_ends_on    | String               | Site subscription expires on this date     |


**Response payload examples:**
**A site with trial of ThreatDown Advanced Bundle with 14 workstation seat allocation that will auto convert to paid on 2024-03-27**

```
{
  "bundle": "advanced",
  "auto_convert_trial_to_paid": true,
  "devices": [
    {
      "type": "workstation",
      "seat_allocation": 14,
      "seat_used": 0,
      "term_type": "trial",
      "status": "active",
      "term_ends_on": "2024-03-27T23:59:59+00:00"
    }
  ]
}
```
- **`threatdown-cli subscriptions quantity`** - Get information about the usage of sites
- **`threatdown-cli subscriptions update`** - Use this endpoint to upgrade/downgrade or change term types or seats
- **`threatdown-cli subscriptions update-v2`** - Use this endpoint to add or modify a current site subscription
* The user may add or remove a trial through this api
  * <p style="color:red">The user will not be able to re-add a trial once it has been removed</p>
  * Trials are limited to 100 seats
* The user may only trial a higher tier product to an existing Paid subscription
  * If the user has a paid EP subscription, the user may add an EDR trial (assuming the MSP is entitled to EDR)
  * If the user has a paid EDR subscription, the user may only add mobile as a trial

Note: There must be a POST before a PUT subscription can be used. 

**Request payload examples:**
**Update a subscription with**
* paid EP workstation and server
* trial EDR workstation and server
* trial Mobile Security

```
[
    {
        "product": "ep",
        "machine_type": "ws-ser",
        "term_type": "paid",
        "term_length": 0,
        "allocations": {
            "ep": 1,
            "eps": 0
        }
    },
    {
        "product": "edr",
        "machine_type": "ws-ser",
        "term_type": "trial",
        "term_length": 0,
        "allocations": {
            "edr": 10,
            "edrs": 20
        }
    },
    {
        "product": "mob",
        "term_type": "trial",
        "term_length": 0,
        "allocations": {
            "mob": 30
        }
    }
]
```

**Update a subscription with the following subscription**
* paid EDR workstation and server
* paid Mobile Security for Business

```
[
    {
        "product": "edr",
        "machine_type": "ws-ser",
        "term_type": "paid",
        "term_length": 0,
        "allocations": {
            "ep": 1,
            "eps": 0
        }
    },
    {
        "product": "mob",
        "term_type": "paid",
        "term_length": 0,
        "allocations": {
            "mob": 10
        }
    }
]
```

**Remove all current subscriptions of a site, simply send an empty list**
* <p style="color:red">WARNING: Removing all subscriptions will also remove all add-on modules</p>

```
[]
```

### suspicious-activity

Suspicious Activities are found using Endpoint Detection and Response (EDR).

- **`threatdown-cli suspicious-activity close`** - Suspicious Activity operation the vendor has not documented: POST /oneview/v1/accounts/{account_id}/sa/close
- **`threatdown-cli suspicious-activity details`** - Fetch details of suspicious activity.
- **`threatdown-cli suspicious-activity list`** - Fetch suspicious activity records of the account.
- **`threatdown-cli suspicious-activity open`** - Suspicious Activity operation the vendor has not documented: POST /oneview/v1/accounts/{account_id}/sa/open
- **`threatdown-cli suspicious-activity process-graph`** - Fetch process graph of suspicious activity.
- **`threatdown-cli suspicious-activity remediate`** - Suspicious Activity operation the vendor has not documented: POST /oneview/v1/accounts/{account_id}/sa/remediate

### syslog

Syslog

- **`threatdown-cli syslog create`** - Create new syslog config record.
- **`threatdown-cli syslog get`** - Get syslog config record by account id and config id.
- **`threatdown-cli syslog list`** - Get syslog config records by account id.
- **`threatdown-cli syslog machine-enable`** - Set syslog machine enabled/disabled.
- **`threatdown-cli syslog machines-assign`** - Assign syslog config machines.
- **`threatdown-cli syslog search`** - Search syslog configs by multiple accounts.
- **`threatdown-cli syslog update`** - Update syslog config by account_id and config id.

### usage

Usage

- **`threatdown-cli usage changelogs`** - Get entitlement history for an endpoint
- **`threatdown-cli usage histogram`** - Search usage histogram
- **`threatdown-cli usage logs`** - Search logs of usage
- **`threatdown-cli usage logs-summary`** - Search summary of usage
- **`threatdown-cli usage logs-summary-export`** - Export usage data
- **`threatdown-cli usage two-tier-logs-summary-export`** - Two tier usage summary export

### users

Provision and manage OneView users through APIs.

- **`threatdown-cli users create`** - Create a new OneView User
- **`threatdown-cli users delete`** - Delete one user by its id
- **`threatdown-cli users get`** - Get user by ID
- **`threatdown-cli users list`** - Get all users
- **`threatdown-cli users resend-invite`** - Resend user invite
- **`threatdown-cli users update`** - Update data about a user

### vulnerability-assessment

Vulnerability Assessment

- **`threatdown-cli vulnerability-assessment cve-stats`** - Get vulnerability assessment CVE statistics for accounts
- **`threatdown-cli vulnerability-assessment details`** - Get vulnerability assessment details for accounts
- **`threatdown-cli vulnerability-assessment report`** - Get vulnerability assessment report for accounts
- **`threatdown-cli vulnerability-assessment software-stats`** - Get vulnerability assessment software statistics for accounts
- **`threatdown-cli vulnerability-assessment summary`** - Get vulnerability assessment summary for accounts

### webhooks

ThreatDown can send webhook events that notify your application any time an event happens on your account.

- **`threatdown-cli webhooks create`** - Use this api endpoint to create a webhook subscription for a given account. This allows you to receive security events for the provided account.
- **`threatdown-cli webhooks delete`** - Delete a webhook subscription if you want to stop receiving notifications. If you want to temporarily disable notifications, update the subscription by setting enable to false.
- **`threatdown-cli webhooks get`** - Retrieve a subscription by its unique identifier.
- **`threatdown-cli webhooks list`** - Retrieve all webhook subscriptions of your account or your managed accounts.
- **`threatdown-cli webhooks update`** - Update a subscription by its unique identifier.


### customer


- **`threatdown-cli customer report`** - Everything about one customer site in one output: health, stale machines, top threats, isolated endpoints and pending jobs.


### fleet


- **`threatdown-cli fleet stale`** - Every endpoint not seen in N hours across all your sites, with site name and exact hours since last seen.

- **`threatdown-cli fleet health`** - One ranked row per site with total, online, stale, infected, suspicious, isolated, reboot-required, unprotected, scan-overdue and update-available counts plus an attention score.

- **`threatdown-cli fleet sync`** - Pull sites, endpoints, detections and jobs from every managed site into one local database.


### respond


- **`threatdown-cli respond isolate`** - Isolate endpoints by host name with a target preview and a 25-endpoint blast-radius guard; sibling commands respond restore, respond scan, respond reboot and respond update work the same way.

- **`threatdown-cli respond pending`** - Jobs that timed out, expired or failed, plus jobs still waiting after --min-age (default 1h), grouped by site and command with a re-issue hint.


### seats


- **`threatdown-cli seats reconcile`** - Per-site seat allocations compared with deployed and recently seen endpoints, with expiry flags.


### threats


- **`threatdown-cli threats spread`** - Every threat in a window ranked by reach (distinct sites and endpoints), by hash or threat name; add --min-sites 2 for cross-site threats only.

- **`threatdown-cli threats queue`** - Prioritized list of infected or suspicious endpoints with online and isolated state and a suggested response.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`threatdown-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`threatdown-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`threatdown-cli learnings list`** - Inspect taught rows
- **`threatdown-cli learnings forget <query>`** - Undo a teach
- **`threatdown-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`threatdown-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`threatdown-cli teach-pattern`** - Install a query/resource template up front
- **`threatdown-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `THREATDOWN_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `threatdown-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
threatdown-cli account-info

# JSON for scripting and agents
threatdown-cli account-info --json
# Filter to specific fields
threatdown-cli account-info --json --select account_id,nebula_parent_account_id

# Dry run  -  show the request without sending
threatdown-cli account-info --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
threatdown-cli account-info --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending (for `respond` verbs the real preview is the command without `--confirm`)
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `8` async job submitted but not finished (rerun the printed recovery command; do not resubmit), `10` config error.

## Health Check

```bash
threatdown-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `threatdown-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/threatdown-cli/config.toml`; `--home`, `THREATDOWN_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `THREATDOWN_CLIENT_ID` | per_call | Yes | OAuth2 client id of the OneView API client (OneView > Integrate). |
| `THREATDOWN_CLIENT_SECRET` | per_call | Yes | OAuth2 client secret paired with the client id. |
| `THREATDOWN_OAUTH_SCOPE` | optional | No | Space-separated scopes to request (default `execute read write`); respond commands need `execute`. Run `threatdown-cli auth login` after changing it. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `threatdown-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `threatdown-cli doctor` to check credentials
- Verify both variables are set: `echo $THREATDOWN_CLIENT_ID` and `test -n "$THREATDOWN_CLIENT_SECRET" && echo secret set`
- Check the cached token and its source: `threatdown-cli auth status`; mint a fresh one with `threatdown-cli auth login`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **403 Forbidden on a command that worked for another user**  -  The OAuth client's creating user lacks the OneView permission for that resource, or the client lacks the scope; check both in OneView and set THREATDOWN_OAUTH_SCOPE to match.
- **429 Too Many Requests during large syncs**  -  OneView allows 360 requests per minute; rerun with --rate-limit 4 or sync fewer resources at a time.
- **Job issued but never ran**  -  Run threatdown-cli respond pending --since 7d to see waiting, TIMED_OUT, EXPIRED or FAILED jobs and whether the endpoint is online.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**MalwareBytesExtractor**](https://github.com/peopleworks/MalwareBytesExtractor)  -  C# (2 stars)
- [**api-evangelist/malwarebytes**](https://github.com/api-evangelist/malwarebytes)  -  Markdown
- [**nebula2nebula**](https://github.com/B10sp4rt4n/nebula2nebula)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
