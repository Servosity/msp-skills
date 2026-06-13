---
name: crowdstrike
description: "Every CrowdStrike Falcon MSP operation, plus a Flight-Control-aware local store that answers fleet-wide questions across all your tenants at once  -  something no other Falcon tool (including the official MCP server) does. Trigger phrases: `check crowdstrike alerts across all tenants`, `show stale falcon sensors`, `critical vulnerabilities across my crowdstrike fleet`, `crowdstrike tenant scorecard`, `list falcon child CIDs`, `use crowdstrike-cli`, `run crowdstrike-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "CrowdStrike"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - crowdstrike-cli
    install:
      - kind: go
        bins: [crowdstrike-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/crowdstrike/cmd/crowdstrike-cli
---

# CrowdStrike Falcon  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `crowdstrike-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install crowdstrike --cli-only
   ```
2. Verify: `crowdstrike-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/crowdstrike/cmd/crowdstrike-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Match the official Falcon CLIs feature-for-feature on alerts, devices, incidents, Spotlight vulnerabilities, prevention policies, and MSSP Flight Control  -  then go beyond them. fleet sync pulls every child tenant into one local SQLite store keyed by CID, so fleet scorecard, fleet vulns, fleet stale, and fleet policy-drift answer cross-tenant questions instantly and offline, with agent-native JSON on every command.

## When to Use This CLI

Reach for this CLI when an agent or operator needs to inspect or act on a CrowdStrike Falcon estate  -  especially an MSP/MSSP estate spanning many child CIDs. It is the right tool for cross-tenant posture questions (vulnerabilities, stale sensors, policy drift, alert triage) that would otherwise require dozens of live, paginated API calls, and for any scripted alert/device/incident/policy action with agent-native JSON output.

## Anti-triggers

Do not use this CLI for:
- Real-Time Response (RTR) shell sessions, file get/put, or script execution - not implemented; use Falcon-Toolkit or the Falcon console
- Sensor install/uninstall or update rollouts - use your RMM or the Falcon console
- Streaming detections into a SIEM - use the CrowdStrike SIEM connector / Falcon Data Replicator
- Legacy /detects/* Detects API workflows - decommissioned upstream; the Alerts API commands replace it
- Cross-tenant fleet rollups without a parent-CID Flight Control API client - they degrade to single-CID coverage

## Unique Capabilities

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

## Command Reference

**alerts**  -  Detections/alerts (modern Alerts API, replaces the decommissioned Detects API)

- `crowdstrike-cli alerts get-queries-v2`  -  Retrieves all Alerts ids that match a given query.
- `crowdstrike-cli alerts patch-entities-v3`  -  Perform actions on Alerts identified by composite ID(s) in request.
- `crowdstrike-cli alerts post-aggregates-v2`  -  Retrieves aggregate values for Alerts across all CIDs.
- `crowdstrike-cli alerts post-combined-v1`  -  Retrieves all Alerts that match a particular FQL filter.
- `crowdstrike-cli alerts post-entities-v2`  -  Retrieves all Alerts given their composite ids.

**devices**  -  Manage devices

- `crowdstrike-cli devices combined-by-filter`  -  Search for hosts in your environment by platform, hostname, IP, and other criteria. Returns full device records.
- `crowdstrike-cli devices combined-hidden-by-filter`  -  Search for hidden hosts in your environment by platform, hostname, IP, and other criteria. Returns full device records.
- `crowdstrike-cli devices create-host-groups`  -  Create Host Groups by specifying details about the group to create
- `crowdstrike-cli devices delete-host-groups`  -  Delete a set of Host Groups by specifying their IDs
- `crowdstrike-cli devices entities-perform-action`  -  Performs the specified action on the provided group IDs.
- `crowdstrike-cli devices get-details-v2`  -  Get details on one or more hosts by providing host IDs as a query parameter. Supports up to a maximum 100 IDs.
- `crowdstrike-cli devices get-host-groups`  -  Retrieve a set of Host Groups by specifying their IDs
- `crowdstrike-cli devices get-online-state-v1`  -  Get the online status for one or more hosts by specifying each host’s unique ID.
- `crowdstrike-cli devices perform-action-v2`  -  Take various actions on the hosts in your environment. Contain or lift containment on a host. Delete or restore a host.
- `crowdstrike-cli devices perform-group-action`  -  Perform the specified action on the Host Groups specified in the request
- `crowdstrike-cli devices post-details-v2`  -  Get details on one or more hosts by providing host IDs in a POST body. Supports up to a maximum 5000 IDs.
- `crowdstrike-cli devices query-by-filter`  -  Search for hosts in your environment by platform, hostname, IP, and other criteria.
- `crowdstrike-cli devices query-by-filter-scroll`  -  Search for hosts in your environment by platform, hostname, IP
- `crowdstrike-cli devices query-combined-group-members`  -  Search for members of a Host Group in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli devices query-combined-host-groups`  -  Search for Host Groups in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli devices query-get-network-address-history-v1`  -  Retrieve history of IP and MAC addresses of devices.
- `crowdstrike-cli devices query-group-members`  -  Search for members of a Host Group in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli devices query-hidden`  -  Retrieve hidden hosts that match the provided filter criteria.
- `crowdstrike-cli devices query-host-groups`  -  Search for Host Groups in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli devices query-login-history-v2`  -  Retrieve details about recent interactive login sessions for a set of devices powered by the Host Timeline.
- `crowdstrike-cli devices update-host-groups`  -  Update Host Groups by specifying the ID of the group and details to update
- `crowdstrike-cli devices update-tags`  -  Append or remove one or more Falcon Grouping Tags on one or more hosts. Tags must be of the form FalconGroupingTags/

**incidents**  -  Incidents and behaviors: cross-detection correlation and triage

- `crowdstrike-cli incidents crowd-score`  -  DEPRECATED: the incidentapi will be removed in March 2026. Query environment wide CrowdScore and return the entity data
- `crowdstrike-cli incidents get`  -  DEPRECATED: the incidentapi will be removed in March 2026. Get details on incidents by providing incident IDs
- `crowdstrike-cli incidents get-behaviors`  -  DEPRECATED: the incidentapi will be removed in March 2026. Get details on behaviors by providing behavior IDs
- `crowdstrike-cli incidents perform-action`  -  DEPRECATED: the incidentapi will be removed in March 2026.
- `crowdstrike-cli incidents query`  -  DEPRECATED: the incidentapi will be removed in March 2026.
- `crowdstrike-cli incidents query-behaviors`  -  DEPRECATED: the incidentapi will be removed in March 2026.

**mssp**  -  Manage mssp

- `crowdstrike-cli mssp add-cidgroup-members`  -  Add new CID group member.
- `crowdstrike-cli mssp add-role`  -  Create a link between user group and CID group, with zero or more additional roles.
- `crowdstrike-cli mssp add-user-group-members`  -  Add new user group member. Maximum 500 members allowed per user group.
- `crowdstrike-cli mssp create-cidgroups`  -  Create new CID groups. Name is a required field but description is an optional field. Maximum 500 CID groups allowed.
- `crowdstrike-cli mssp create-user-groups`  -  Create new user groups. Name is a required field but description is an optional field.
- `crowdstrike-cli mssp delete-cidgroup-members-v2`  -  Delete CID group members. Prevents removal of a cid group a cid group if it is only part of one cid group.
- `crowdstrike-cli mssp delete-cidgroups`  -  Delete CID groups by ID.
- `crowdstrike-cli mssp delete-user-group-members`  -  Delete user group members entry.
- `crowdstrike-cli mssp delete-user-groups`  -  Delete user groups by ID.
- `crowdstrike-cli mssp deleted-roles`  -  Delete links or additional roles between user groups and CID groups.
- `crowdstrike-cli mssp get-children`  -  Get link to child customer by child CID(s)
- `crowdstrike-cli mssp get-children-v2`  -  Get link to child customer by child CID(s)
- `crowdstrike-cli mssp get-cidgroup-by-id-v2`  -  Get CID Groups by ID.
- `crowdstrike-cli mssp get-cidgroup-members-by-v2`  -  Get CID group members by CID Group ID.
- `crowdstrike-cli mssp get-roles-by-id`  -  Get link between user group and CID group by ID.
- `crowdstrike-cli mssp get-user-group-members-by-idv2`  -  Get user group members by user group ID.
- `crowdstrike-cli mssp get-user-groups-by-idv2`  -  Get user groups by ID.
- `crowdstrike-cli mssp query-children`  -  Query for customers linked as children
- `crowdstrike-cli mssp query-cidgroup-members`  -  Query a CID groups members by associated CID.
- `crowdstrike-cli mssp query-cidgroups`  -  Query CID groups.
- `crowdstrike-cli mssp query-roles`  -  Query links between user groups and CID groups. At least one of CID group ID or user group ID should also be provided.
- `crowdstrike-cli mssp query-user-group-members`  -  Query user group member by user UUID.
- `crowdstrike-cli mssp query-user-groups`  -  Query user groups.
- `crowdstrike-cli mssp update-cidgroups`  -  Update existing CID groups. CID group ID is expected for each CID group definition provided in request body.
- `crowdstrike-cli mssp update-user-groups`  -  Update existing user group(s). User group ID is expected for each user group definition provided in request body.

**policy**  -  Manage policy

- `crowdstrike-cli policy create-prevention-policies`  -  Create Prevention Policies by specifying details about the policy to create
- `crowdstrike-cli policy delete-prevention-policies`  -  Delete a set of Prevention Policies by specifying their IDs
- `crowdstrike-cli policy get-prevention-policies`  -  Retrieve a set of Prevention Policies by specifying their IDs
- `crowdstrike-cli policy perform-prevention-policies-action`  -  Perform the specified action on the Prevention Policies specified in the request
- `crowdstrike-cli policy query-combined-prevention-members`  -  Search for members of a Prevention Policy in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli policy query-combined-prevention-policies`  -  Search for Prevention Policies in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli policy query-prevention-members`  -  Search for members of a Prevention Policy in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli policy query-prevention-policies`  -  Search for Prevention Policies in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli policy set-prevention-policies-precedence`  -  Sets the precedence of Prevention Policies based on the order of IDs specified in the request.
- `crowdstrike-cli policy update-prevention-policies`  -  Update Prevention Policies by specifying the ID of the policy and details to update

**spotlight**  -  Manage spotlight

- `crowdstrike-cli spotlight combined-query-installed-patches`  -  Gets installed patches information for hosts.
- `crowdstrike-cli spotlight combined-query-vulnerabilities`  -  Search for Vulnerabilities in your environment by providing an FQL filter and paging details.
- `crowdstrike-cli spotlight get-remediations`  -  Get details on remediations by providing one or more IDs
- `crowdstrike-cli spotlight get-vulnerabilities`  -  Get details on vulnerabilities by providing one or more IDs
- `crowdstrike-cli spotlight query-vulnerabilities`  -  Search for Vulnerabilities in your environment by providing an FQL filter and paging details.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
crowdstrike-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

## Auth Setup

Auth is OAuth2 client_credentials. Create an API client in the Falcon console (Support > API Clients & Keys) with scopes for the resources you use, then set FALCON_CLIENT_ID and FALCON_CLIENT_SECRET and run 'crowdstrike-cli auth login' to mint and cache a bearer token (auto-refreshed before expiry). For MSSP/Flight Control, a single parent-CID client operates on child tenants by minting a member_cid-scoped token; the fleet commands handle this for you, minting a per-tenant token for each child CID (or for a single tenant you name). Pick your cloud by overriding the base/token URL (US-1 default, US-2, EU-1, or GovCloud).

Run `crowdstrike-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  crowdstrike-cli incidents get --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
crowdstrike-cli feedback "the --since flag is inclusive but docs say exclusive"
crowdstrike-cli feedback --stdin < notes.txt
crowdstrike-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/crowdstrike-cli/feedback.jsonl`. They are never POSTed unless `CROWDSTRIKE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CROWDSTRIKE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - HeyGen's "Beacon" pattern.

```
crowdstrike-cli profile save briefing --json
crowdstrike-cli --profile briefing incidents get
crowdstrike-cli profile list --json
crowdstrike-cli profile show briefing
crowdstrike-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `crowdstrike-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/crowdstrike/cmd/crowdstrike-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add crowdstrike-mcp -- crowdstrike-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which crowdstrike-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   crowdstrike-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `crowdstrike-cli <command> --help`.
