# Action1 CLI

**Every Action1 endpoint, plus the fleet-wide patch and vulnerability views the org-siloed API cannot give you.**

Action1's REST API is organization-siloed  -  every call is scoped to one org and nothing is kept over time. This CLI mirrors the full API, then fans out across all your organizations into a local SQLite store so you can rank the worst-patched endpoints fleet-wide (fleet patch-posture), triage CVEs by blast radius and CISA KEV status (fleet vuln-triage), find dark agents (fleet stale), and diff patch drift week over week (fleet patch-drift). Agent-native output, typed exit codes, and offline search throughout.

Learn more at [Action1](https://app.action1.com/support/).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `action1-cli` and `action1-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.ps1 | iex
   ```
3. Verify: `action1-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=action1). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/action1 --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/action1 --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `action1-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the action1 skill from https://github.com/Servosity/msp-skills/tree/main/skills/action1. The skill defines how its required CLI (`action1-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=action1).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `ACTION1_OAUTH2` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/action1/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "action1": {
      "command": "action1-mcp",
      "env": {
        "ACTION1_OAUTH2": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`action1-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `ACTION1_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `ACTION1_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export ACTION1_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
action1-mcp --transport http
```

Set `ACTION1_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Action1 uses OAuth2 token-mint. Generate a Client ID and Client Secret on the API Credentials page in the Action1 console, then set ACTION1_CLIENT_ID and ACTION1_CLIENT_SECRET. The CLI POSTs them to /oauth2/token (JSON body) to obtain a bearer token automatically. Set ACTION1_ORG_ID to scope the fleet commands to one organization by default. For the EU or Australia data center set ACTION1_BASE_URL to https://app.eu.action1.com/api/3.0 or https://app.au.action1.com/api/3.0; the token is minted from the same host. ACTION1_OAUTH2 takes a pre-minted bearer token instead of the client pair. Per-organization API commands like 'endpoints managed <orgId>' take the organization id as their first argument.

## Quick Start

```bash
# confirm credentials mint a token and the API is reachable
action1-cli doctor

# list your organizations  -  the orgId most commands take
action1-cli organizations get --json

# walk every organization: endpoints (with missing-update summaries), vulnerabilities, installed software and automation runs land in the local store
action1-cli sync --full

# the worst-patched endpoints across the whole fleet
action1-cli fleet patch-posture --limit 25

# CVEs ranked by blast radius, known-exploited first
action1-cli fleet vuln-triage --kev-only

```

## Unique Features

### Cross-org views the API can't give you
- **`fleet patch-posture`**  -  See every endpoint across all your client organizations ranked by how many updates it is missing  -  one fleet-wide view.

  _Reach for this when an agent needs the worst-patched machines across the whole book of business, not one client at a time._

  ```bash
  action1-cli fleet patch-posture --agent --limit 25
  ```
- **`fleet vuln-triage`**  -  Rank CVEs across every organization by how many endpoints they hit, weighted by CVSS and the CISA Known-Exploited flag.

  _Pick this to answer 'what should we patch first across all clients' with exploit-aware prioritization._

  ```bash
  action1-cli fleet vuln-triage --kev-only --agent
  ```
- **`fleet software-rollup`**  -  Deduplicate installed software across the whole fleet into app name x version spread x install count.

  _Use this for license counting and version-spread audits across all clients at once._

  ```bash
  action1-cli fleet software-rollup --name "Google Chrome" --agent
  ```
- **`fleet automation-health`**  -  Success and failure rates across automation instances and their per-endpoint results, aggregated across organizations.

  _Use this to spot automations that are quietly failing on a subset of endpoints across clients._

  ```bash
  action1-cli fleet automation-health --agent
  ```
- **`fleet org-scorecard`**  -  One row per client organization: endpoint count, missing updates, open CVEs, KEV exposure, and stale agents  -  the per-client posture number MSPs report to account managers.

  _Reach for this when an agent needs a per-client posture summary across every organization, not per-endpoint detail._

  ```bash
  action1-cli fleet org-scorecard --agent
  ```
- **`fleet reboot-pending`**  -  Every endpoint fleet-wide where an installed update is waiting on a reboot to finish  -  the action queue that closes out a patch cycle.

  _Reach for this when an agent needs the concrete list of machines blocking patch-cycle completion, not a composite health score._

  ```bash
  action1-cli fleet reboot-pending --agent
  ```

### Fleet health & drift
- **`fleet stale`**  -  Surface endpoints that have not checked in for N days, or are offline, across all organizations.

  _Use this to find agents that silently stopped reporting before they become a coverage gap._

  ```bash
  action1-cli fleet stale --days 14 --agent
  ```
- **`fleet patch-drift`**  -  Diff two synced snapshots to show which updates were remediated and which newly appeared since last sync.

  _Reach for this to prove remediation progress week over week, or catch regressions._

  ```bash
  action1-cli fleet patch-drift --agent
  ```
- **`fleet health-score`**  -  A single composite score per endpoint from missing updates, open vulnerabilities, reboot-required, and staleness  -  fleet-ranked.

  _Pick this when you need a triage-ranked worst-to-best list of machines across the fleet._

  ```bash
  action1-cli fleet health-score --agent --limit 50
  ```

## Recipes

### Worst-patched endpoints, fleet-wide

```bash
action1-cli fleet patch-posture --agent --limit 25
```

Cross-org rollup of endpoints ranked by missing-update count.

### Known-exploited CVE triage

```bash
action1-cli fleet vuln-triage --kev-only --agent --limit 25
```

CISA KEV CVEs across all orgs, ranked by affected endpoints and CVSS.

### Find dark agents

```bash
action1-cli fleet stale --days 14 --agent
```

Endpoints that stopped checking in over two weeks ago across the fleet.

### Narrow a verbose endpoint list

```bash
action1-cli endpoints managed 00000000-0000-0000-0000-000000000000 --agent --select items.name,items.OS,items.last_seen,items.online_status
```

orgId is positional (00000000-... = all organizations); dotted --select trims the large managed-endpoint payload to just the fields an agent needs.

### Prove remediation progress

```bash
action1-cli fleet patch-drift --agent --limit 50
```

What got patched and what newly appeared since the previous sync.

## Usage

Run `action1-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `ACTION1_CONFIG_DIR`, `ACTION1_DATA_DIR`, `ACTION1_STATE_DIR`, or `ACTION1_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `ACTION1_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export ACTION1_HOME=/srv/action1
action1-cli doctor
```

On Windows (PowerShell):

```powershell
$env:ACTION1_HOME = "C:\ProgramData\action1"
action1-cli doctor
```

Under `ACTION1_HOME=/srv/action1`, the four dirs resolve to `/srv/action1/config`, `/srv/action1/data`, `/srv/action1/state`, and `/srv/action1/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:ACTION1_HOME = "C:\srv\action1"
action1-cli doctor
```

Under `$env:ACTION1_HOME = "C:\srv\action1"`, the four dirs resolve to `C:\srv\action1\config`, `C:\srv\action1\data`, `C:\srv\action1\state`, and `C:\srv\action1\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "action1": {
      "command": "action1-mcp",
      "env": {
        "ACTION1_HOME": "/srv/action1"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `ACTION1_DATA_DIR` overrides an explicit `--home` for that kind. Use `ACTION1_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `ACTION1_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `action1-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `action1-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `action1-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `ACTION1_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `ACTION1_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### audit

Manage audit

- **`action1-cli audit events-get`** - **Requires permission: `view_audit`**

'The Audit Trail contains event records associated with user actions, such as logins, remote access, changes to configuration, and API usage.'
- **`action1-cli audit events-id-get`** - **Requires permission: `view_audit`**

Get a specific audit record by its ID.
- **`action1-cli audit export-get`** - **Requires permission: `view_audit`**

Exports audit data. Use parameters to filter out exported data.

The only currently supported format is 'csv'.

### automations

Manage automations

- **`action1-cli automations actions-templates-get`** - Gets a list of existing action templates.
- **`action1-cli automations actions-templates-template-id-get`** - Gets details for an action template specified by its ID.
- **`action1-cli automations policies-instances-org-id-get`** - **Requires permission: `view_automations`**

Gets a list of running and completed automations for the specified organization.
- **`action1-cli automations policies-instances-org-id-id-get`** - **Requires permission: `view_automations`**

Gets details about a automation instance specified by its ID.
- **`action1-cli automations policies-instances-org-id-instance-id-endpoint-results-endpoint-id-details-get`** - **Requires permission: `view_automations`**

Gets details about the automation instance applied to an endpoint specified by its ID.
- **`action1-cli automations policies-instances-org-id-instance-id-endpoint-results-get`** - **Requires permission: `view_automations`**

Gets a list of endpoints where the automation instance is being applied or has been executed.
- **`action1-cli automations policies-instances-org-id-instance-id-stop-post`** - **Requires permission: `manage_automations`**

Stops applying a automation instance and aborts actions running on a remote endpoint.
- **`action1-cli automations policies-instances-org-id-post`** - **Requires permission: `run_automations`**

Immediately runs an instance of the automation on the specified endpoints in the specified organization. This call will fail if 'orgId' is set to 'all'.
- **`action1-cli automations policies-schedules-org-id-get`** - **Requires permission: `view_automations`**

Lists existing scheduled automations. Use parameters to filter out automations in the returned results.
- **`action1-cli automations policies-schedules-org-id-id-actions-action-id-delete`** - **Requires permission: `manage_automations`**

Deletes a specified action from a scheduled automation.
- **`action1-cli automations policies-schedules-org-id-id-delete`** - **Requires permission: `manage_automations`**

Deletes a scheduled automation specified by its ID.
- **`action1-cli automations policies-schedules-org-id-id-deployment-statuses-get`** - **Requires permission: `view_automations`**

Gets the deployment statuses of a automation specified by its ID.
- **`action1-cli automations policies-schedules-org-id-id-get`** - **Requires permission: `view_automations`**

Gets a scheduled automation specified by its ID.
- **`action1-cli automations policies-schedules-org-id-id-patch`** - **Requires permission: `manage_automations`**

Modifies the automation schedule settings and parameters of one of the actions included in the automation.
- **`action1-cli automations policies-schedules-org-id-post`** - **Requires permission: `manage_automations`**

Schedules a new automation. The automation will be executed against a specified list of endpoints in the specified organization.

### cve-descriptions

Manage cve descriptions

- **`action1-cli cve-descriptions <cveId>`** - **Requires permission: `view_vulnerabilities`**

Retrieves detailed information about a specific vulnerability in general, not within a context of any organization. 

This includes the Common Vulnerabilities and Exposures (CVE) details such as the CVSS score, attack vector, severity, and other relevant attributes. 

This information provides a comprehensive overview of the vulnerability, aiding in understanding its nature and potential impact.

### data-sources

A data source is a scripting template that queries certain endpoint data, such as disk volume or local group members, and presents it in a structured way consumed by reports or alerts. Data sources can be built-in or custom (defined by 'builtin' parameter). Built-in data sources are maintained by Action1 and cannot be modified. Custom data sources can be created and maintained by the user.

- **`action1-cli data-sources all-post`** - **Requires permission: `manage_data_sources`**

Creates a new custom data source. At this time, all data sources are enterprise-wide.
- **`action1-cli data-sources org-id-get`** - **Requires one of the following permissions: `manage_data_sources`, `manage_reports`**

Gets a list of existing data sources. To filter out built-in data sources, set the 'builtin' parameter to 'yes'. For custom, set 'builtin' to 'no'.
- **`action1-cli data-sources org-id-id-delete`** - **Requires permission: `manage_data_sources`**

Deletes an existing custom data source.

Note that you cannot remove built-in data sources.

At this time, all data sources are enterprise-wide. The 'orgId' has to be set to 'all'.
- **`action1-cli data-sources org-id-id-get`** - **Requires one of the following permissions: `manage_data_sources`, `manage_reports`**

Gets details about a specific data source. At this time, all data sources are enterprise-wide. The 'orgId' has to be set to 'all'.
- **`action1-cli data-sources org-id-id-patch`** - **Requires permission: `manage_data_sources`**

Updates a custom data source specified by its ID. At this time, all data sources are enterprise-wide. The 'orgId' has to be set to 'all'.

### endpoints

The endpoint object represents a server, workstation, or other device managed by Action1 within a specific organization. Endpoints can only belong to one Action1 organization at one time, but can be moved between organizations.

- **`action1-cli endpoints agent-installation`** - **Requires permission: `manage_endpoints`**

In order to manage an endpoint, the Action1 agent needs to be installed on it first. Agent binary files are specific to each organization. Use this call to obtain a URL to download the agent installation file for a specific organization and agent type.
- **`action1-cli endpoints connector-installation-org-id-windows-exe-get`** - **Requires permission: `manage_endpoints`**

Obtains an URL to download the Deployer installation file. The Action1 Deployer installation files are specific to each organization. The Deployer services currently run on Windows only.
- **`action1-cli endpoints connectors-org-id-deployer-id-delete`** - **Requires permission: `manage_endpoints`**

In rare cases it is impossible to uninstall the Action1 Deployer (a computer it was deployed on was removed from the organization, OS reinstalled, etc.).

Use this call to delete the Deployer object without uninstalling the service.

Fails if the Deployer status is not 'Pending Install' (you must try to uninstall the Deployer first via POST '/API/endpoints/deployers/{org-id}/{deployer-id}/uninstall)''.
- **`action1-cli endpoints connectors-org-id-deployer-id-get`** - **Requires permission: `manage_endpoints`**

Obtains the current information about the specified Action1 Deployer service.
- **`action1-cli endpoints connectors-org-id-get`** - **Requires permission: `manage_endpoints`**

Lists all Action1 Deployer services in the specified organization. Use parameters to filter returned results.
- **`action1-cli endpoints discovery-org-id-get`** - **Requires permission: `manage_endpoints`**

Obtains the current Agent Deployment settings in a specified organization. If 'orgId' is to 'all', the call will fail.
- **`action1-cli endpoints discovery-org-id-patch`** - **Requires permission: `manage_endpoints`**

Updates the Agent Deployment configuration for a specific organization.
- **`action1-cli endpoints groups`** - **Requires permission: `manage_endpoints`**

Creates a new endpoint group within the specified organization. ('include_filter':[…] and 'exclude_filter':[…] -- can be omitted upon creation.)
- **`action1-cli endpoints groups-group-id`** - **Requires permission: `view_endpoints`**

Gets a specific endpoint group by its ID.
- **`action1-cli endpoints groups-group-id-contents-get`** - **Requires permission: `view_endpoints`**

Lists all endpoints included in the specified group. Use parameters to filter the returned results.
- **`action1-cli endpoints groups-group-id-contents-post`** - **Requires permission: `manage_endpoints`**

Joins endpoints with specified IDs or names to the endpoint group.

Removes endpoints with specific IDs.

If the specified endpoint is already in the group via filters, its 'added_via' attribute will be updated to 'manual' (from 'criteria') and the call will succeed.
- **`action1-cli endpoints groups-group-id-delete`** - **Requires permission: `manage_endpoints`**

Deletes an existing group in the specified organization.
- **`action1-cli endpoints groups-group-id-patch`** - **Requires permission: `manage_endpoints`**

Changes settings for an existing endpoint group in the specified organization. If
certain fields are not specified in the object you submit, they won't get changed. I.e., you can just change the group name without changing the group filters.
- **`action1-cli endpoints groups-org-id`** - **Requires permission: `view_endpoints`**

Lists existing endpoint groups. Use filters and narrow down and sort the returned results.
- **`action1-cli endpoints managed`** - **Requires permission: `view_endpoints`**

Lists all endpoints within the organization. Use filters to narrow down returned results.
- **`action1-cli endpoints managed-id`** - **Requires permission: `view_endpoints`**

Obtains current information about the endpoint with the specified ID.
- **`action1-cli endpoints managed-id-delete`** - **Requires permission: `manage_endpoints`**

Removes a specified endpoint and attempts to uninstall its agent.
- **`action1-cli endpoints managed-id-missing-updates`** - **Requires permission: `view_endpoints`**

Obtains a list of missing software updates for a specific endpoint. Use filters to narrow down the returned results.
- **`action1-cli endpoints managed-id-move`** - **Requires permission: `manage_endpoints`**

Moves the endpoint to another organization.
- **`action1-cli endpoints managed-id-patch`** - **Requires permission: `manage_endpoint_attributes`**

Changes the user-defined 'comment', 'name' and custom attributes for the specified endpoint.
- **`action1-cli endpoints managed-id-remote-sessions-post`** - **Requires permission: `remote_connect`**

Starts a remote assistance session. Supply `orgId`, `endpointId`, and
`--connection-type assistance`; `current_ip` is optional. The credential needs
Remote Connect in the endpoint's scope. On HTTP 403, stop and ask an authorized
administrator to check that scope; do not grant privileges automatically.

Use the returned `id` to poll the remote-session GET with `orgId`, `endpointId`,
and `sessionId` every five seconds for up to one minute, stopping on API errors.
Once `connected` is `yes`, open `remote_session` in a browser and verify the
intended desktop before interacting. Keep the connection URL private. A timeout
is unverified; do not automatically create another session.
- **`action1-cli endpoints managed-id-remote-sessions-session-id-get`** - **Requires permission: `remote_connect`**

Gets details for an existing remote session using `orgId`, `endpointId`, and
`sessionId`. Follow the bounded polling and private connection URL handling above.
A session request alone does not prove the desktop is connected.
- **`action1-cli endpoints managed-id-remote-sessions-session-id-patch`** - **Requires permission: `remote_connect`**

Changes the 'current_monitor' parameter for a specific remote session.
- **`action1-cli endpoints status`** - Retrieves information if any endpoints were added to an organization.

### enterprise

Manage enterprise

- **`action1-cli enterprise get`** - Gets settings for an enterprise. You can query data for your enterprise only.
- **`action1-cli enterprise patch`** - **Requires permission: `manage_enterprise`**

Updates settings for a current enterprise. You can only access your enterprise.
- **`action1-cli enterprise request-closure`** - **Requires permission: `manage_enterprise`**

The request is available only for a free Action1 account.

If need to close an enterprise with a paid subscription, please contact your Action1 representative.

The Action1 enterprise associated with your account will enter a 30 day period of pending closure. Following the 30-day period, we will attempt to uninstall any remaining agents in your Action1 enterprise, and all your data will be permanently erased from Action1 cloud servers.
- **`action1-cli enterprise revoke-closure`** - **Requires permission: `manage_enterprise`**

The Action1 enterprise will be immediately reactivated and all your data will be retained.

### installed-software

Manage installed software


### logs

Manage logs

- **`action1-cli logs <orgId>`** - **Requires permission: `manage_endpoints`**

Gets diagnostic logs. Use parameters to pre-filter returned results. For example, get the first 50 log records of the level "normal" and higher, sorted by time in descending order.

### me

Manage me

- **`action1-cli me me`** - Gets settings for the currently authenticated user.
- **`action1-cli me patch`** - Updates settings for the currently authenticated user.
Only 'timezone' and 'session_timeout' settings are currently updatable.
- **`action1-cli me subscriptions-get`** - Gets a list of report subscriptions.
- **`action1-cli me subscriptions-post`** - **Requires permission: `view_reports`**

Creates a new report subscription.
- **`action1-cli me subscriptions-subscription-id-delete`** - Removes the report subscription.
- **`action1-cli me subscriptions-subscription-id-patch`** - **Requires permission: `view_reports`**

Updates the report subscription.

### oauth2

Manage oauth2

- **`action1-cli oauth2`** - Generates a token. Provide valid API credentials (Client ID and Client Secret) which are configured in API credentials page in Action1 console.

### organizations

Manage organizations

- **`action1-cli organizations get`** - Gets a list of organizations within the current Action1 enterprise.
- **`action1-cli organizations org-id-delete`** - **Requires permission: `manage_organizations`**

Removes an organization from the enterprise.

This method will fail if a specified organization still contains endpoints or if it is the last organization in the enterprise.
- **`action1-cli organizations org-id-patch`** - **Requires permission: `manage_organizations`**

Updates settings for an organization specified by its ID.
- **`action1-cli organizations post`** - **Requires permission: `manage_organizations`**

Creates a new organization. When a new organization is created, all the default roles are automatically created for it.

### permissions

Manage permissions

- **`action1-cli permissions`** - Gets a list of available permission templates.

### remote_search

Manage remote search

- **`action1-cli remote-search <orgId>`** - **Requires permissions: `view_endpoints`, `view_software_repository`, `view_reports`**

Searches for Action1 objects that contain a phrase specified in the 'query' parameter in one of their fields. The search returns up to 10 objects within the specified organization.

### reportdata

Manage reportdata


### reports

Manage reports

- **`action1-cli reports all-get`** - Gets a list of existing reports.

At this time all reports are enterprise-wide.
- **`action1-cli reports org-id-category-id-get`** - Gets a list of reports and categories.

At this time all reports are enterprise-wide.
- **`action1-cli reports org-id-custom-id-delete`** - **Requires permission: `manage_reports`**

Deletes a custom report specified by its ID.

You cannot delete built-in reports.
- **`action1-cli reports org-id-custom-id-patch`** - **Requires permission: `manage_reports`**

Updates a custom report. You cannot changes a custom report's category.

Custom reports are enterprise-wide.
- **`action1-cli reports org-id-custom-post`** - **Requires permission: `manage_reports`**

Creates a custom report in the predefined Custom report category. You cannot create custom reports in other categories.

Custom reports are based on the data source. To learn more about data sources, see '/data-sources/*' calls.

Custom reports are enterprise-wide.

### roles

Manage roles

- **`action1-cli roles get`** - **Requires permission: `manage_roles`**

Gets a list of available roles.
- **`action1-cli roles id-delete`** - **Requires permission: `manage_roles`**

Deletes a role specified by its ID.
- **`action1-cli roles id-get`** - **Requires permission: `manage_roles`**

Gets details about a role specified by its ID.
- **`action1-cli roles id-patch`** - **Requires permission: `manage_roles`**

Updates a role specified by its ID.
- **`action1-cli roles post`** - **Requires permission: `manage_roles`**

Creates a new role.

### scripts

Manage scripts

- **`action1-cli scripts org-id-get`** - **Requires permission: `use_scripts`**

Gets a list of existing scripts from the Script Library. To filter out built-in scripts, set the 'builtin' parameter to 'yes'. For custom, set 'builtin' to 'no'.

At this time, all scripts are enterprise-wide. The 'orgId' has to be set to 'all'.
- **`action1-cli scripts org-id-id-delete`** - **Requires permission: `manage_scripts`**

Deletes an existing custom script specified by its ID.

At this time, all scripts are enterprise-wide.
- **`action1-cli scripts org-id-id-get`** - **Requires permission: `use_scripts`**

Gets details for a script specified by its ID.

At this time, all scripts are enterprise-wide.
- **`action1-cli scripts org-id-id-patch`** - **Requires permission: `manage_scripts`**

Updates details for an existing custom script specified by its ID.

At this time, all scripts are enterprise-wide.

The 'params' array (script parameters) included in the returned object is a read-only collection that is derived from the script text.
- **`action1-cli scripts org-id-post`** - **Requires permission: `manage_scripts`**

Creates a new custom script and adds it to the Script Library.

At this time, all scripts are enterprise-wide, The 'orgId' shall be set to 'all'. In the future, the organization-specific scripts will be supported.

The 'params' array (script parameters) included in the returned object is a read-only collection that is derived from the script text.

### setting-templates

Manage setting templates

- **`action1-cli setting-templates org-id-get`** - Gets a list of existing setting templates. Setting templates are maintained by Action1 and cannot be modified.

At this time, all setting templates are enterprise-wide, even when applied to a specific organization only.
- **`action1-cli setting-templates org-id-template-id-get`** - Gets details about a setting template specified by its ID. Setting templates are maintained by Action1 and cannot be modified.

At this time, all setting templates are enterprise-wide.

### settings

Manage settings

- **`action1-cli settings org-id-get`** - **Requires permission: `manage_advanced_settings`**

Lists all existing settings.

At this time, all settings are enterprise-wide.
- **`action1-cli settings org-id-id-delete`** - **Requires permission: `manage_advanced_settings`**

Deletes an existing setting specified by its ID.

At this time, all settings are enterprise-wide.
- **`action1-cli settings org-id-id-get`** - **Requires permission: `manage_advanced_settings`**

Gets details about the setting configuration.

At this time, all settings are enterprise-wide, but the scope of application can be limited to per organization or group.
- **`action1-cli settings org-id-id-patch`** - **Requires permission: `manage_advanced_settings`**

Updates an existing setting specified by its ID.

At this time, all settings are enterprise-wide.
- **`action1-cli settings org-id-post`** - **Requires permission: `manage_advanced_settings`**

Creates a new setting.

At this time, all setting templates are enterprise-wide.

### software-repository

Action1 Software Repository is a continuously updated private application repository that hosts the latest versions of all supported applications to keep your endpoints secure and patched. Action1 maintains built-in software repository packages that are available to all customers as well as enables users to create private software packages to be shared within their organizations only. The package object is a container of software versions and initially, it should contain the first available version of the software (child object called "version"). New versions are added continuously as the respective vendor releases them. Each version object contains the software binary setup files in MSI, EXE or ZIP format (up to 32Gb in size) and deployment settings, such as silent install switches. It may also include the additional actions to be executed before or after a software installation or uninstallation (e.g., reboot or run a script).

- **`action1-cli software-repository packages-all-get`** - **Requires permission: `view_software_repository`**

Gets a list of Software Repository packages for the entire enterprise. Use parameters to filter out packages in the returned results.
- **`action1-cli software-repository packages-all-package-id-delete`** - **Requires permission: `manage_software_repository`**

Deletes a custom Software Repository package specified by its ID.

Built-in Software Repository packages cannot be deleted. This API will return an error if you attempt to send a DELETE request for a built-in package.
- **`action1-cli software-repository packages-all-package-id-get`** - **Requires permission: `view_software_repository`**

Gets details for a Software Repository package specified by its ID. Set the "fields" parameter to "*" or "versions" to include the non-default "versions" container of children objects. Example: /software-repository/{orgId}/{Adobe_Adobe_Acrobat_Reader_DC_1570155380192_builtin}?fields=*
- **`action1-cli software-repository packages-all-package-id-patch`** - **Requires permission: `manage_software_repository`**

Modifies one or more properties of a custom Software Repository package, including contents of the child "version" objects.

Built-in packages cannot be modified except for 'EULA_accepted' setting. This method will return an error if you attempt to send a PATCH request for a built-in package and specify any properties except for 'EULA_accepted'.

The 'version.additional_actions.name' property shall always be present if the parameters of an additional action are to be modified. To change the 'name' parameter of an additional action, the action needs to be deleted (DELETE method) and then added back with the new name (PATCH request).
- **`action1-cli software-repository packages-all-post`** - **Requires permission: `manage_software_repository`**

Create a new custom Software Repository package object (with no versions) and set its initial basic properties. To create package versions, use POST /software-repository/{orgId}/{packageId}/versions method.

### subscription

View Action licenses, start or extend a free trial, or request a price quote.

- **`action1-cli subscription license-enterprise-get`** - **Requires permission: `manage_enterprise`**

Gets details about the enterprise license. You can get only get information about your enterprise.
- **`action1-cli subscription license-enterprise-quote-post`** - **Requires permission: `manage_enterprise`**

Sends a quote request to the Action1 Sales department. The actual quote request will be reviewed by someone in Sales and a quote will be sent via email.
- **`action1-cli subscription license-enterprise-trial-post`** - **Requires permission: `manage_enterprise`**

Sends a request to start a free trial for a current user or requests a trial extension for an expired or exceeded trial.
- **`action1-cli subscription license-usage-enterprise-get`** - **Requires permission: `manage_enterprise`**

Gets details about license usage for the entire enterprise.
- **`action1-cli subscription license-usage-organizations-get`** - **Requires permission: `manage_organizations`**

Gets details about license usage with statistics for each organization individually.
- **`action1-cli subscription license-usage-organizations-org-id-get`** - **Requires permission: `manage_organizations`**

Gets details about license usage for an organization specified by its ID.

### updates

Manage updates

- **`action1-cli updates org-id-get`** - **Requires one of the following permissions: `approve_updates`, `view_dashboards`, `manage_automations`**

Gets a list of all missing updates. Use parameters to filter out updates in the returned results.
- **`action1-cli updates org-id-package-id-get`** - **Requires one of the following permissions: `approve_updates`, `view_dashboards`, `manage_automations`**

Gets a list of updates available for a package specified by its ID. Use parameters to filter out updates in the returned results.

### users

Manage users

- **`action1-cli users get`** - **Requires permission: `view_users`**

Gets a list of users within the current Action1 enterprise.
- **`action1-cli users id-delete`** - **Requires permission: `manage_users`**

Deletes an existing user.
- **`action1-cli users id-get`** - **Requires permission: `view_users`**

Gets an existing user.
- **`action1-cli users id-patch`** - **Requires permission: `manage_users`**

Updates an existing user.
- **`action1-cli users post`** - **Requires permission: `manage_users`**

Creates a new user.

To create an SSO user, please first review the following documentation:
- EntraId: https://www.action1.com/documentation/sso-authentication-with-entra-id/
- Duo: https://www.action1.com/documentation/sso-authentication-with-duo/
- Google: https://www.action1.com/documentation/sso-authentication-with-google/
- Okta: https://www.action1.com/documentation/sso-authentication-with-google/

### vulnerabilities

Manage vulnerabilities

- **`action1-cli vulnerabilities org-id-cve-id-get`** - **Requires permission: `view_vulnerabilities`**

Retrieves detailed information about a specific vulnerability as it exists in the context of a specific organization, including the Common Vulnerabilities and Exposures (CVE) details such as the CVSS score, attack vector, severity, and other relevant attributes. 

This information provides a comprehensive overview of the vulnerability, aiding in understanding its nature and potential impact.
- **`action1-cli vulnerabilities org-id-get`** - **Requires permission: `view_vulnerabilities`**

Retrieves a comprehensive list of all vulnerable software installed on the organization's managed endpoints. 

This includes information about the specific vulnerabilities, affected versions, and affected endpoints. The data provides insights into potential risks, enabling targeted remediation efforts.


### fleet


- **`action1-cli fleet patch-posture`** - See every endpoint across all your client organizations ranked by how many updates it is missing  -  one fleet-wide view.

- **`action1-cli fleet vuln-triage`** - Rank CVEs across every organization by how many endpoints they hit, weighted by CVSS and the CISA Known-Exploited flag.

- **`action1-cli fleet stale`** - Surface endpoints that have not checked in for N days, or are offline, across all organizations.

- **`action1-cli fleet patch-drift`** - Diff two synced snapshots to show which updates were remediated and which newly appeared since last sync.

- **`action1-cli fleet software-rollup`** - Deduplicate installed software across the whole fleet into app name x version spread x install count.

- **`action1-cli fleet health-score`** - A single composite score per endpoint from missing updates, open vulnerabilities, reboot-required, and staleness  -  fleet-ranked.

- **`action1-cli fleet automation-health`** - Success and failure rates across automation instances and their per-endpoint results, aggregated across organizations.

- **`action1-cli fleet org-scorecard`** - One row per client organization: endpoint count, missing updates, open CVEs, KEV exposure, and stale agents  -  the per-client posture number MSPs report to account managers.

- **`action1-cli fleet reboot-pending`** - Every endpoint fleet-wide where an installed update is waiting on a reboot to finish  -  the action queue that closes out a patch cycle.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`action1-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`action1-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`action1-cli learnings list`** - Inspect taught rows
- **`action1-cli learnings forget <query>`** - Undo a teach
- **`action1-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`action1-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`action1-cli teach-pattern`** - Install a query/resource template up front
- **`action1-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `ACTION1_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `action1-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
action1-cli enterprise get

# JSON for scripting and agents
action1-cli enterprise get --json
# Filter to specific fields
action1-cli enterprise get --json --select allowed_fast_closure,closure_requested_at,closure_requested_by

# Dry run  -  show the request without sending
action1-cli enterprise get --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
action1-cli enterprise get --agent
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

## Health Check

```bash
action1-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `action1-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/action1-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\action1-cli\config.toml` on Windows; `--home`, `ACTION1_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `ACTION1_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `ACTION1_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `ACTION1_OAUTH2` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `action1-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

### Keeping credentials off disk (optional)

By default the connector caches the token it mints in `credentials.toml` (mode 0600,
under the data directory `action1-cli doctor` names) so the next command does not have
to mint another one. Set `ACTION1_NO_CONFIG_WRITE=1` to turn that off: nothing is
written, and each run mints a fresh token instead. Use it when your secrets live in
Keychain, Windows Credential Manager, or any launcher that injects them as environment
variables at process start, and you do not want a copy on disk.

| What changes | With `ACTION1_NO_CONFIG_WRITE=1` |
| --- | --- |
| Token cache | Not written. A fresh token is minted per invocation. |
| `auth set-token` | Refuses, naming the variable, instead of reporting a save that did not happen. |
| `auth logout` | Still clears an existing stored credential. The erase is not a credential write. |
| MCP server | Honours the same variable, so a Claude Desktop install gets no token cache either. |

Set it in the shell for CLI use (`export ACTION1_NO_CONFIG_WRITE=1`; PowerShell:
`$env:ACTION1_NO_CONFIG_WRITE = "1"`), or through the install prompt of the same name
for the MCP server. Any value other than blank, `0`, `false`, `no` or `off` turns it on.
See issue #270.

## Credential safety

- The `Authorization` header, the client credentials and every API request go only to the scheme, host and port of `ACTION1_BASE_URL` (default `https://app.action1.com/api/3.0`). The token is minted from `<base URL>/oauth2/token` on that same host, so an EU or Australia base URL mints from its own region. A redirect to any other host is refused, so a credential never reaches it.
- Any output unit that would repeat a credential this program holds, in any encoding, is replaced whole by a notice such as "[credkit: withheld from stdout: it carried a credential this program holds]": stdout, stderr, files, the local store and every MCP message. Held: `ACTION1_OAUTH2`, `ACTION1_CLIENT_SECRET`, the unique part of `ACTION1_CLIENT_ID` (the `api-key-` prefix and `@action1.com` stay printable, so the API host is never hidden), a minted token, and the token fields Action1 returns. Responses are never cached on disk.
- Only `credentials.toml` (mode 0600) holds a credential. A client ID and secret supplied by the environment are never written to disk, and nor is the token minted for them: each process mints its own and keeps it in memory (one mint per CLI run; the MCP server reuses its token until it expires). A token is cached in `credentials.toml` only beside the client pair stored there (not at all under `ACTION1_NO_CONFIG_WRITE=1`), and a stored token is only ever sent for the client it was minted for. `auth set-token` reads the token from stdin, never from an argument.
- `--dry-run` sends nothing and never mints a token. Over MCP, a call that changes Action1 (`action1_execute`, `import`) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `action1-cli doctor` to check credentials
- Verify the environment variable is set: `echo $ACTION1_OAUTH2`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 or token mint fails**  -  Check ACTION1_CLIENT_ID and ACTION1_CLIENT_SECRET come from the API Credentials page (not your login); run action1-cli doctor.
- **Empty results from an org-scoped command**  -  Per-org commands take the orgId as the first argument (from organizations get); fleet views read the local store, so run sync first.
- **fleet commands return []**  -  Run action1-cli sync --full first  -  fleet views read the local store.
- **Wrong data center**  -  Set ACTION1_BASE_URL to your console's region: https://app.eu.action1.com/api/3.0 (EU) or https://app.au.action1.com/api/3.0 (Australia); the default is North America.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**PSAction1**](https://github.com/Action1Corp/PSAction1)  -  PowerShell
- [**Action1MCP**](https://github.com/ghively/Action1MCP)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
