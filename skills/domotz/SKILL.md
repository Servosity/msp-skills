---
name: domotz
description: "Every Domotz endpoint, plus a local SQLite fleet mirror that answers cross-site questions. Trigger phrases: `which domotz sites are down`, `list offline devices across all sites`, `export domotz device inventory`, `check fleet health in domotz`, `find new devices on the network`, `use domotz`, `run domotz-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Domotz"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - domotz-cli
    install:
      - kind: go
        bins: [domotz-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/domotz/cmd/domotz-cli
---

# Domotz  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `domotz-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install domotz --cli-only
   ```
2. Verify: `domotz-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/domotz/cmd/domotz-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

domotz-cli gives MSPs and AV integrators full command-line and agent-native access to Domotz Collectors, devices, variables, alerts, and network topology. It syncs your whole fleet into a local database so cross-site rollups  -  fleet health, every offline device, new-device detection, one unified inventory export  -  become single offline queries instead of agent-by-agent API sweeps.

## When to Use This CLI

Use domotz-cli when an agent or operator needs to query a fleet of Domotz-monitored networks from the terminal: checking which sites are down, finding every offline or newly-appeared device across all clients, exporting a unified asset inventory, or reading variables, alerts, and topology with structured JSON output. It is the right choice over raw API calls whenever the question spans more than one agent.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI for non-Domotz monitoring stacks (PRTG, Zabbix, Datto RMM, NinjaOne)  -  it only talks to the Domotz Public API.
- Do not use it for ticketing/PSA actions (creating tickets, billing)  -  pair it with a PSA tool instead.
- Do not use it for ad-hoc network scans of arbitrary hosts; it reports what Domotz Collectors already monitor.
- Do not use the fleet store-backed commands as a live source without a recent 'sync --full'  -  they read the local mirror.

## Unique Capabilities

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

## Command Reference

**agent**  -  Manage agent

- `domotz-cli agent count`  -  Counts the collectors.
- `domotz-cli agent delete`  -  Deletes a collector.
- `domotz-cli agent get`  -  Returns the details of a collector.
- `domotz-cli agent get-list-uptime`  -  Returns the uptime of all collectors.
- `domotz-cli agent list`  -  Returns the list of collectors accessible by the user.

**alert-profile**  -  Manage alert profile

- `domotz-cli alert-profile get-agent`  -  Returns the alert profile bindings of a collector.
- `domotz-cli alert-profile get-alert-profiles2`  -  Returns the list of configured alert profiles. You can configure alert profiles on the Domotz Portal.
- `domotz-cli alert-profile get-devices`  -  Returns the alert profile bindings of the devices of a collector.

**area**  -  Manage area

- `domotz-cli area`  -  Returns all the areas of a Company. Note: This API is restricted to users on the Enterprise Plan.

**custom-driver**  -  Manage custom driver

- `domotz-cli custom-driver get`  -  Returns details of a Custom Driver.
- `domotz-cli custom-driver list`  -  Retrieves the list of available Custom Drivers.
- `domotz-cli custom-driver list-associations`  -  Retrieves a list of all Custom Driver associations for a collector.
- `domotz-cli custom-driver re-enable-associations`  -  Re-enable all disabled Custom Drivers for the current user.

**custom-tag**  -  Manage custom tag

- `domotz-cli custom-tag create`  -  Creates a new Tag.
- `domotz-cli custom-tag delete`  -  Deletes a Tag and removes it from Collectors and Devices.
- `domotz-cli custom-tag edit`  -  Updates one or more properties of an existing Tag.
- `domotz-cli custom-tag get`  -  Retrieves all Tags available in the account, including their metadata and usage counts.

**device-profile**  -  Manage device profile

- `domotz-cli device-profile`  -  Returns the list of the available device profiles.

**inventory**  -  Manage inventory

- `domotz-cli inventory create-field`  -  Creates a new Inventory Field - the user will be able to set key-values pairs on every device.
- `domotz-cli inventory delete`  -  Clears the inventory.
- `domotz-cli inventory delete-field`  -  Deletes the Inventory Field.
- `domotz-cli inventory get`  -  Enumerates all the Inventory fields.
- `domotz-cli inventory update-field`  -  Updates the Inventory Field.

**meta**  -  Manage meta

- `domotz-cli meta`  -  Returns information about API usage and limits.

**rbac**  -  Manage rbac

- `domotz-cli rbac create-user`  -  Create a new RBAC User.
- `domotz-cli rbac create-user-group`  -  Create a new RBAC User group.
- `domotz-cli rbac delete-user`  -  Delete an RBAC User by user ID.
- `domotz-cli rbac delete-user-group`  -  Delete an RBAC User group by user group ID.
- `domotz-cli rbac edit-user`  -  Update an RBAC User by user ID.
- `domotz-cli rbac edit-user-group`  -  Update an RBAC User group by user group ID. Note: Users and roles are replaced by those provided in the request.
- `domotz-cli rbac get-role`  -  Retrieve a Role and its Permissions. Note: When 'is_applied_to_all_entities' is true, 'entity_ids' is omitted.
- `domotz-cli rbac get-roles`  -  List all RBAC roles and associated user groups.
- `domotz-cli rbac get-user`  -  Retrieve RBAC User details by User ID.
- `domotz-cli rbac get-user-group`  -  Retrieve RBAC User group details by user group ID.
- `domotz-cli rbac get-user-groups`  -  List all RBAC User groups with their details.
- `domotz-cli rbac get-users`  -  List all RBAC Users with their details.

**type**  -  Manage type

- `domotz-cli type list-device-base`  -  Returns the device types list.
- `domotz-cli type list-device-detected`  -  Returns the detected device types list.

**user**  -  Manage user

- `domotz-cli user`  -  Returns the account information.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
domotz-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

## Auth Setup

Authenticate with an API key from the Domotz Portal (Settings > API Key). Set DOMOTZ_API_KEY (DOMOTZ_PUBLIC_API_KEY is also accepted as a fallback), and set your region/cell (shown beside the key, e.g. us-east-1-cell-1) via DOMOTZ_REGION so the CLI targets api-<region>.domotz.com.

Run `domotz-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  domotz-cli agent list --agent --select id,name,status
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
domotz-cli feedback "the --since flag is inclusive but docs say exclusive"
domotz-cli feedback --stdin < notes.txt
domotz-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/domotz-cli/feedback.jsonl`. They are never POSTed unless `DOMOTZ_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `DOMOTZ_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
domotz-cli profile save briefing --json
domotz-cli --profile briefing agent list
domotz-cli profile list --json
domotz-cli profile show briefing
domotz-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `domotz-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/domotz/cmd/domotz-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add domotz-mcp -- domotz-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which domotz-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   domotz-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `domotz-cli <command> --help`.
