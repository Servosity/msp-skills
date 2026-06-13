# CrowdStrike Falcon CLI

**Every CrowdStrike Falcon MSP operation, plus a Flight-Control-aware local store that answers fleet-wide questions across all your tenants at once  -  something no other Falcon tool (including the official MCP server) does.**

Match the official Falcon CLIs feature-for-feature on alerts, devices, incidents, Spotlight vulnerabilities, prevention policies, and MSSP Flight Control  -  then go beyond them. fleet sync pulls every child tenant into one local SQLite store keyed by CID, so fleet scorecard, fleet vulns, fleet stale, and fleet policy-drift answer cross-tenant questions instantly and offline, with agent-native JSON on every command.

## Install

The recommended path installs both the `crowdstrike-cli` binary and the `pp-crowdstrike` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install crowdstrike
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install crowdstrike --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install crowdstrike --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install crowdstrike --agent claude-code
npx -y @mvanhorn/printing-press-library install crowdstrike --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/crowdstrike/cmd/crowdstrike-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/crowdstrike-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install crowdstrike --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-crowdstrike --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-crowdstrike --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install crowdstrike --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/crowdstrike-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `FALCON_CLIENT_ID` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/crowdstrike/cmd/crowdstrike-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "crowdstrike": {
      "command": "crowdstrike-mcp",
      "env": {
        "FALCON_CLIENT_ID": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Auth is OAuth2 client_credentials. Create an API client in the Falcon console (Support > API Clients & Keys) with scopes for the resources you use, then set FALCON_CLIENT_ID and FALCON_CLIENT_SECRET and run 'crowdstrike-cli auth login' to mint and cache a bearer token (auto-refreshed before expiry). For MSSP/Flight Control, a single parent-CID client operates on child tenants by minting a member_cid-scoped token; the fleet commands handle this for you, minting a per-tenant token for each child CID (or for a single tenant you name). Pick your cloud by overriding the base/token URL (US-1 default, US-2, EU-1, or GovCloud).

## Quick Start

```bash
# Mint and cache an OAuth2 token from FALCON_CLIENT_ID/SECRET
crowdstrike-cli auth login

# Verify the token mints and the API is reachable
crowdstrike-cli doctor

# Pull every Flight Control tenant into the local store
crowdstrike-cli fleet sync --all-cids

# See posture across the whole fleet at a glance
crowdstrike-cli fleet scorecard --json

# Instant offline search across every tenant's synced data
crowdstrike-cli fleet search <term> --json

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-tenant fleet intelligence
- **`fleet sync`**  -  Pull hosts, alerts, vulnerabilities, policies, and the Flight Control fabric from every child tenant into one local store keyed by CID. Requires a parent-CID API client with Flight Control (MSSP) scope; without it, sync degrades to the single authenticated CID.

  _Reach for this first: it builds the offline fleet store every other fleet command reads from._

  ```bash
  crowdstrike-cli fleet sync --all-cids --json
  ```
- **`fleet scorecard`**  -  One posture board across all tenants: host count, sensor coverage, open critical alerts, critical vulns, and policy posture per CID.

  _Use when an agent needs a whole-book-of-business health summary without dozens of live calls._

  ```bash
  crowdstrike-cli fleet scorecard --json
  ```
- **`fleet vulns`**  -  Rank and filter Spotlight vulnerabilities across every tenant at once, by severity or CVE.

  _Use to answer 'every critical exposure across all customers' in one shot._

  ```bash
  crowdstrike-cli fleet vulns --severity critical --json
  ```
- **`fleet stale`**  -  Find hosts across all tenants whose sensor has not checked in within N days.

  _Use to catch coverage gaps (offline sensors) before they become blind spots._

  ```bash
  crowdstrike-cli fleet stale --days 14 --json
  ```
- **`fleet policy-drift`**  -  Diff each tenant's prevention policy settings against a baseline CID to surface under-protected tenants.

  _Use to enforce a security baseline across a multi-tenant book._

  ```bash
  crowdstrike-cli fleet policy-drift --json
  ```
- **`fleet alerts`**  -  One severity-sorted new-alert queue spanning all tenants, from the local store.

  _The MSP analyst's morning view: what needs attention across every customer, ranked._

  ```bash
  crowdstrike-cli fleet alerts --status new --json
  ```
- **`fleet tenants`**  -  See the whole Flight Control fabric - every child CID, its CID groups, user groups, and role grants - as one offline roster.

  _Reach for this when you need the multi-tenant RBAC and tenant roster without walking five live MSSP endpoints._

  ```bash
  crowdstrike-cli fleet tenants --agent
  ```
- **`fleet remediate`**  -  Group fleet-wide exposure by remediation action - which single fix clears the most hosts and tenants.

  _Use this to turn a vulnerability list into a patch worklist ranked by cross-tenant blast radius._

  ```bash
  crowdstrike-cli fleet remediate --severity critical --agent
  ```
- **`fleet trend`**  -  See which tenants got worse since the last sync - week-over-week deltas in open critical alerts and critical vulnerabilities.

  _Use this for degradation detection across the book of business - the live API only ever shows current state._

  ```bash
  crowdstrike-cli fleet trend --agent
  ```

### Offline intelligence
- **`fleet search`**  -  Full-text search across synced hosts, alerts, vulnerabilities, and policies across all tenants, instantly and offline.

  _Use for instant lookups without burning live API quota or waiting on pagination._

  ```bash
  crowdstrike-cli fleet search <term> --json --select cid,kind,name
  ```

## Recipes


### Whole-fleet critical exposure

```bash
crowdstrike-cli fleet vulns --severity critical --agent --select cid,cve,hostname,severity
```

Pulls every critical Spotlight vuln across all tenants from the store and narrows to the fields an agent needs.

### Morning triage queue

```bash
crowdstrike-cli fleet alerts --status new --agent --select cid,severity,name,device.hostname
```

One severity-sorted new-alert queue across every tenant, field-narrowed for agent context.

### Coverage audit

```bash
crowdstrike-cli fleet stale --days 30 --json
```

Lists hosts whose sensor has gone dark for 30+ days across the whole fleet.

### Baseline enforcement

```bash
crowdstrike-cli fleet policy-drift --json
```

Surfaces tenants whose prevention policy is weaker than the baseline CID.

### Tenant coverage at a glance

```bash
crowdstrike-cli fleet scorecard --agent --select cid,coverage_pct,critical_vulns,open_critical_alerts
```

Narrows the per-tenant posture board to the fields an agent needs to triage the fleet.

## Usage

Run `crowdstrike-cli --help` for the full command reference and flag list.

## Commands

### alerts

Detections/alerts (modern Alerts API, replaces the decommissioned Detects API)

- **`crowdstrike-cli alerts get-queries-v2`** - Retrieves all Alerts ids that match a given query.
- **`crowdstrike-cli alerts patch-entities-v3`** - Perform actions on Alerts identified by composite ID(s) in request. Each action has a name and a description which describes what the action does. If a request adds and removes tag in a single request, the order of processing would be to remove tags before adding new ones in.
- **`crowdstrike-cli alerts post-aggregates-v2`** - Retrieves aggregate values for Alerts across all CIDs.
- **`crowdstrike-cli alerts post-combined-v1`** - Retrieves all Alerts that match a particular FQL filter. This API is intended for retrieval of large amounts of Alerts(>10k) using a pagination based on a `after` token. If you need to use `offset` pagination, consider using GET /alerts/queries/alerts/* and POST /alerts/entities/alerts/* APIs.
- **`crowdstrike-cli alerts post-entities-v2`** - Retrieves all Alerts given their composite ids.

### devices

Manage devices

- **`crowdstrike-cli devices combined-by-filter`** - Search for hosts in your environment by platform, hostname, IP, and other criteria. Returns full device records.
- **`crowdstrike-cli devices combined-hidden-by-filter`** - Search for hidden hosts in your environment by platform, hostname, IP, and other criteria. Returns full device records.
- **`crowdstrike-cli devices create-host-groups`** - Create Host Groups by specifying details about the group to create
- **`crowdstrike-cli devices delete-host-groups`** - Delete a set of Host Groups by specifying their IDs
- **`crowdstrike-cli devices entities-perform-action`** - Performs the specified action on the provided group IDs.
- **`crowdstrike-cli devices get-details-v2`** - Get details on one or more hosts by providing host IDs as a query parameter. Supports up to a maximum 100 IDs.
- **`crowdstrike-cli devices get-host-groups`** - Retrieve a set of Host Groups by specifying their IDs
- **`crowdstrike-cli devices get-online-state-v1`** - Get the online status for one or more hosts by specifying each host’s unique ID. Successful requests return an HTTP 200 response and the status for each host identified by a `state` of `online`, `offline`, or `unknown` for each host, identified by host `id`. QueryDevicesByFilter to get a list of host IDs.
- **`crowdstrike-cli devices perform-action-v2`** - Take various actions on the hosts in your environment. Contain or lift containment on a host. Delete or restore a host.
- **`crowdstrike-cli devices perform-group-action`** - Perform the specified action on the Host Groups specified in the request
- **`crowdstrike-cli devices post-details-v2`** - Get details on one or more hosts by providing host IDs in a POST body. Supports up to a maximum 5000 IDs.
- **`crowdstrike-cli devices query-by-filter`** - Search for hosts in your environment by platform, hostname, IP, and other criteria.
- **`crowdstrike-cli devices query-by-filter-scroll`** - Search for hosts in your environment by platform, hostname, IP, and other criteria with continuous pagination capability (based on offset pointer which expires after 2 minutes with no maximum limit)
- **`crowdstrike-cli devices query-combined-group-members`** - Search for members of a Host Group in your environment by providing an FQL filter and paging details. Returns a set of host details which match the filter criteria
- **`crowdstrike-cli devices query-combined-host-groups`** - Search for Host Groups in your environment by providing an FQL filter and paging details. Returns a set of Host Groups which match the filter criteria
- **`crowdstrike-cli devices query-get-network-address-history-v1`** - Retrieve history of IP and MAC addresses of devices.
- **`crowdstrike-cli devices query-group-members`** - Search for members of a Host Group in your environment by providing an FQL filter and paging details. Returns a set of Agent IDs which match the filter criteria
- **`crowdstrike-cli devices query-hidden`** - Retrieve hidden hosts that match the provided filter criteria.
- **`crowdstrike-cli devices query-host-groups`** - Search for Host Groups in your environment by providing an FQL filter and paging details. Returns a set of Host Group IDs which match the filter criteria
- **`crowdstrike-cli devices query-login-history-v2`** - Retrieve details about recent interactive login sessions for a set of devices powered by the Host Timeline. A max of 10 device ids can be specified
- **`crowdstrike-cli devices update-host-groups`** - Update Host Groups by specifying the ID of the group and details to update
- **`crowdstrike-cli devices update-tags`** - Append or remove one or more Falcon Grouping Tags on one or more hosts. Tags must be of the form FalconGroupingTags/

### incidents

Incidents and behaviors: cross-detection correlation and triage

- **`crowdstrike-cli incidents crowd-score`** - DEPRECATED: the incidentapi will be removed in March 2026. Query environment wide CrowdScore and return the entity data
- **`crowdstrike-cli incidents get`** - DEPRECATED: the incidentapi will be removed in March 2026. Get details on incidents by providing incident IDs
- **`crowdstrike-cli incidents get-behaviors`** - DEPRECATED: the incidentapi will be removed in March 2026. Get details on behaviors by providing behavior IDs
- **`crowdstrike-cli incidents perform-action`** - DEPRECATED: the incidentapi will be removed in March 2026. Perform a set of actions on one or more incidents, such as adding tags or comments or updating the incident name or description
- **`crowdstrike-cli incidents query`** - DEPRECATED: the incidentapi will be removed in March 2026. Search for incidents by providing an FQL filter, sorting, and paging details
- **`crowdstrike-cli incidents query-behaviors`** - DEPRECATED: the incidentapi will be removed in March 2026. Search for behaviors by providing an FQL filter, sorting, and paging details

### mssp

Manage mssp

- **`crowdstrike-cli mssp add-cidgroup-members`** - Add new CID group member.
- **`crowdstrike-cli mssp add-role`** - Create a link between user group and CID group, with zero or more additional roles. The call does not replace any existing link between them. User group ID and CID group ID have to be specified in request.
- **`crowdstrike-cli mssp add-user-group-members`** - Add new user group member. Maximum 500 members allowed per user group.
- **`crowdstrike-cli mssp create-cidgroups`** - Create new CID groups. Name is a required field but description is an optional field. Maximum 500 CID groups allowed.
- **`crowdstrike-cli mssp create-user-groups`** - Create new user groups. Name is a required field but description is an optional field. Maximum 500 user groups allowed per customer.
- **`crowdstrike-cli mssp delete-cidgroup-members-v2`** - Delete CID group members. Prevents removal of a cid group a cid group if it is only part of one cid group.
- **`crowdstrike-cli mssp delete-cidgroups`** - Delete CID groups by ID.
- **`crowdstrike-cli mssp delete-user-group-members`** - Delete user group members entry.
- **`crowdstrike-cli mssp delete-user-groups`** - Delete user groups by ID.
- **`crowdstrike-cli mssp deleted-roles`** - Delete links or additional roles between user groups and CID groups. User group ID and CID group ID have to be specified in request. Only specified roles are removed if specified in request payload, else association between User Group and CID group is dissolved completely (if no roles specified).
- **`crowdstrike-cli mssp get-children`** - Get link to child customer by child CID(s)
- **`crowdstrike-cli mssp get-children-v2`** - Get link to child customer by child CID(s)
- **`crowdstrike-cli mssp get-cidgroup-by-id-v2`** - Get CID Groups by ID.
- **`crowdstrike-cli mssp get-cidgroup-members-by-v2`** - Get CID group members by CID Group ID.
- **`crowdstrike-cli mssp get-roles-by-id`** - Get link between user group and CID group by ID. Link ID is a string consisting of multiple components, but should be treated as opaque.
- **`crowdstrike-cli mssp get-user-group-members-by-idv2`** - Get user group members by user group ID.
- **`crowdstrike-cli mssp get-user-groups-by-idv2`** - Get user groups by ID.
- **`crowdstrike-cli mssp query-children`** - Query for customers linked as children
- **`crowdstrike-cli mssp query-cidgroup-members`** - Query a CID groups members by associated CID.
- **`crowdstrike-cli mssp query-cidgroups`** - Query CID groups.
- **`crowdstrike-cli mssp query-roles`** - Query links between user groups and CID groups. At least one of CID group ID or user group ID should also be provided. Role ID is optional.
- **`crowdstrike-cli mssp query-user-group-members`** - Query user group member by user UUID.
- **`crowdstrike-cli mssp query-user-groups`** - Query user groups.
- **`crowdstrike-cli mssp update-cidgroups`** - Update existing CID groups. CID group ID is expected for each CID group definition provided in request body. Name is a required field but description is an optional field. Empty description will override existing value. CID group member(s) remain unaffected.
- **`crowdstrike-cli mssp update-user-groups`** - Update existing user group(s). User group ID is expected for each user group definition provided in request body. Name is a required field but description is an optional field. Empty description will override existing value. User group member(s) remain unaffected.

### policy

Manage policy

- **`crowdstrike-cli policy create-prevention-policies`** - Create Prevention Policies by specifying details about the policy to create
- **`crowdstrike-cli policy delete-prevention-policies`** - Delete a set of Prevention Policies by specifying their IDs
- **`crowdstrike-cli policy get-prevention-policies`** - Retrieve a set of Prevention Policies by specifying their IDs
- **`crowdstrike-cli policy perform-prevention-policies-action`** - Perform the specified action on the Prevention Policies specified in the request
- **`crowdstrike-cli policy query-combined-prevention-members`** - Search for members of a Prevention Policy in your environment by providing an FQL filter and paging details. Returns a set of host details which match the filter criteria
- **`crowdstrike-cli policy query-combined-prevention-policies`** - Search for Prevention Policies in your environment by providing an FQL filter and paging details. Returns a set of Prevention Policies which match the filter criteria
- **`crowdstrike-cli policy query-prevention-members`** - Search for members of a Prevention Policy in your environment by providing an FQL filter and paging details. Returns a set of Agent IDs which match the filter criteria
- **`crowdstrike-cli policy query-prevention-policies`** - Search for Prevention Policies in your environment by providing an FQL filter and paging details. Returns a set of Prevention Policy IDs which match the filter criteria
- **`crowdstrike-cli policy set-prevention-policies-precedence`** - Sets the precedence of Prevention Policies based on the order of IDs specified in the request. The first ID specified will have the highest precedence and the last ID specified will have the lowest. You must specify all non-Default Policies for a platform when updating precedence
- **`crowdstrike-cli policy update-prevention-policies`** - Update Prevention Policies by specifying the ID of the policy and details to update

### spotlight

Manage spotlight

- **`crowdstrike-cli spotlight combined-query-installed-patches`** - Gets installed patches information for hosts.
- **`crowdstrike-cli spotlight combined-query-vulnerabilities`** - Search for Vulnerabilities in your environment by providing an FQL filter and paging details. Returns a set of Vulnerability entities which match the filter criteria
- **`crowdstrike-cli spotlight get-remediations`** - Get details on remediations by providing one or more IDs
- **`crowdstrike-cli spotlight get-vulnerabilities`** - Get details on vulnerabilities by providing one or more IDs
- **`crowdstrike-cli spotlight query-vulnerabilities`** - Search for Vulnerabilities in your environment by providing an FQL filter and paging details. Returns a set of Vulnerability IDs which match the filter criteria


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
crowdstrike-cli incidents get

# JSON for scripting and agents
crowdstrike-cli incidents get --json

# Filter to specific fields
crowdstrike-cli incidents get --json --select id,name,status

# Dry run  -  show the request without sending
crowdstrike-cli incidents get --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
crowdstrike-cli incidents get --agent
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

## Health Check

```bash
crowdstrike-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/crowdstrike-falcon-msp-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `FALCON_CLIENT_ID` | auth_flow_input | Yes | CrowdStrike API client ID (Falcon console > API Clients & Keys). |
| `FALCON_CLIENT_SECRET` | auth_flow_input | Yes | Set during initial auth setup. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `crowdstrike-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `crowdstrike-cli doctor` to check credentials
- Verify the environment variable is set: `echo $FALCON_CLIENT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401/403 from the token endpoint**  -  Confirm FALCON_CLIENT_ID/FALCON_CLIENT_SECRET and that the API client has the right scopes; re-run 'auth login'.
- **Calls succeed but return another region's empty data**  -  Set the correct cloud: override base/token URL to api.us-2, api.eu-1, or api.laggar.gcw.crowdstrike.com.
- **fleet commands return nothing**  -  Run 'fleet sync' first  -  fleet rollups read the local store, not the live API.
- **fleet sync only covers one tenant**  -  Use a parent-CID API client with Flight Control scope; without it, cross-tenant member_cid minting is unavailable.
- **Incidents commands error upstream**  -  The Falcon Incidents API is deprecated upstream; prefer alerts-based triage where possible.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**Falcon-Toolkit**](https://github.com/CrowdStrike/Falcon-Toolkit)  -  Python
- [**psfalcon**](https://github.com/CrowdStrike/psfalcon)  -  PowerShell
- [**falcon-cli**](https://github.com/CrowdStrike/falcon-cli)  -  Go
- [**falcon-mcp**](https://github.com/CrowdStrike/falcon-mcp)  -  Python
- [**gofalcon**](https://github.com/CrowdStrike/gofalcon)  -  Go

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
