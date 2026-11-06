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
      - kind: script
        bins: [domotz-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/domotz/install.ps1
---

# Domotz - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `domotz-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

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

The installer downloads the `domotz-cli` and `domotz-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

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


**drift**


- `domotz-cli drift`  -  Diffs two local snapshots for a site to surface configuration and inventory changes over time.


**fleet**


- `domotz-cli fleet health`  -  One status board across every Domotz Collector: online/offline agents, degraded sites, and down-device counts per site.

- `domotz-cli fleet offline`  -  Every unreachable or offline device across all sites in one prioritized list, with site and importance.

- `domotz-cli fleet new`  -  Devices first-seen anywhere in the fleet within a time window  -  a rogue-device security signal.

- `domotz-cli fleet inventory`  -  One asset table  -  vendor, model, type, OS, serial, site  -  for every device across every agent.

- `domotz-cli fleet alerts`  -  Alert profiles (alerting rules) and their device bindings, aggregated across the fleet.

- `domotz-cli fleet ip-conflicts`  -  IP address conflicts across all sites in one prioritized list.

- `domotz-cli fleet breakdown`  -  Counts of devices by type, vendor, and OS across the fleet for capacity and security posture.

- `domotz-cli fleet speedtest`  -  Per-site WAN speed-test history aggregated into fleet min/avg/max and a worst-site ranking.

- `domotz-cli fleet agents`  -  Only the sites (Collectors) that are offline or degraded right now, with location, organization, and version, in one prioritized list.

- `domotz-cli fleet stale`  -  Collectors whose local snapshot has gone quiet  -  last-synced or last-seen older than a threshold, a silent monitoring blind spot.

- `domotz-cli fleet triggers`  -  TCP service sensors (eyes) currently DOWN across every device on every agent, in one fleet-wide list.

- `domotz-cli fleet unmonitored`  -  Devices Domotz can't fully monitor  -  failed or missing authentication and SNMP status  -  surfaced fleet-wide as a coverage-gap audit.

- `domotz-cli fleet events`  -  Per-agent activity-log and network-event history merged into one chronological fleet-wide timeline.

- `domotz-cli fleet unalerted`  -  Devices no enabled alert profile watches, across every Collector, VITAL first: the devices that can go down without anyone being told.


**topology**


- `domotz-cli topology`  -  Cached network topology for a site, summarized (node/edge counts, gateways) and queryable offline.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
domotz-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

Authenticate with an API key from the Domotz Portal (Settings > API Key). Set DOMOTZ_API_KEY, and set your region/cell (shown beside the key, e.g. us-east-1-cell-1) via DOMOTZ_REGION so the CLI targets api-<region>.domotz.com.

Run `domotz-cli doctor` to verify setup.

## Credential safety

- The API key (`X-Api-Key: <key>`) and every API request go only to the scheme, host and port of the configured base URL (`https://api-<region>.domotz.com/public-api/v1` built from `DOMOTZ_REGION`, default `us-east-1-cell-1`, or `DOMOTZ_BASE_URL`). A redirect to any other host is refused, so the key never reaches it, and a region that is not a single host-name label is refused before anything is sent.
- Any output, file, local store write or MCP result that would carry `DOMOTZ_API_KEY`, the stored key, or a credential from a response (a device's SNMP communities, an SNMPv3 user, authentication key or encryption key, a device or custom driver username and password), in any encoding, is withheld whole: the program prints "[credkit: withheld from stdout: it carried a credential this program holds]" (naming the sink) in its place. Leave those fields out with `--select` to print the rest. Responses are never cached on disk, and credential-named fields reach the local store as "[redacted]".
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the key from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, `domotz_execute` (every Domotz endpoint that changes something) and `import` return a preview and send only with `confirm: true`, and a tool argument can never name a file to read or write.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  domotz-cli agent list --agent --select access_right,creation_time,display_name
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `DOMOTZ_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `DOMOTZ_CONFIG_DIR`, `DOMOTZ_DATA_DIR`, `DOMOTZ_STATE_DIR`, `DOMOTZ_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `DOMOTZ_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `domotz-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

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

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `DOMOTZ_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `DOMOTZ_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `DOMOTZ_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `DOMOTZ_DATA_SOURCE` sets the `--data-source` default.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, pass the question as an argv or MCP tool argument to `recall --agent`. Do not interpolate user-controlled text into a shell command line.

Quoted `recall "<question>"` breaks on an apostrophe, which is ordinary English. A quoted heredoc breaks when a body line equals the delimiter, and that delimiter is published in these docs. Write the question with a non-shell file-writing tool, then read it back as data:

```bash
# Write the question verbatim with your file-writing tool (no shell involved).
# Command substitution on a file only ever yields data  -  the shell never
# parses the file's bytes as syntax.
QUERY=$(cat /path/to/question.txt)
domotz-cli recall "$QUERY" --agent
```

Prefer MCP: pass the question as the tool's query argument. `"$QUERY"` after a file read is argv-safe; putting the question itself in the command text is not.

The response envelope:

```json
{
  "query": "...",
  "normalized": "<normalized form>",
  "query_entities": ["..."],
  "found": true | false,
  "match_score": 0.0,
  "results": [
    { "resource_id": "...", "resource_type": "...", "venue": "...",
      "confidence": 2, "entity_match": "exact|partial|unknown",
      "source": "taught|preseed|pattern", "warnings": ["..."] }
  ],
  "mismatches": [ /* only when --debug-mismatches */ ],
  "warnings": [ /* top-level */ ],
  "candidates": [
    { "id": 12, "class": "flag_alias | playbook_candidate",
      "summary": "...", "sightings": 3, "last_seen": "...",
      "rationale": "...",
      "next_action": ["<trial command>", "domotz-cli learnings confirm 12"] }
  ],
  "playbook": {
    "query_family": "...",
    "playbook": {
      "steps": [ { "cmd": "<command with {slot} substitution>", "purpose": "..." } ],
      "entity_slots": ["$ENTITY"],
      "expected_tool_calls": 3
    },
    "slots_resolved": { "$ENTITY": { "token": "<live token>", "canonical": "<canonical>" } },
    "notes": "<workarounds + gotchas for this query family>"
  },
  "notes": "<duplicate surface for non-playbook callers>"
}
```

Empty-store short-circuit: if the store has no learnings, playbooks, or candidates yet (recall finds nothing and `learnings list` and `learnings candidates` are both empty), skip recall for the rest of this session instead of taxing every query; resume recall-first once something has been taught.

### Step 2: decision tree

Read `candidates`, `playbook`, `notes`, `results[0]`, and warnings in that order:

```
if Candidates present (warnings include "candidates_present"):
    -> candidates are try-then-confirm, never facts. Follow each candidate's
       two-step next_action verbatim: run the trial command first, then run
       `learnings confirm <id>` only after the trial verified the behavior.
       Reject a wrong candidate with `learnings reject <id>`.
    -> NEVER re-teach something recall surfaced as a candidate; confirm or
       reject that candidate instead of teaching a duplicate.
    -> candidates ride alongside playbooks and resource hits, not instead of
       them; continue with the branches below after acting on them.

if Playbook present:
    -> READ Playbook.notes verbatim FIRST (workarounds + gotchas the CLI surface doesn't expose)
    -> replay Playbook.steps in order, substituting Playbook.slots_resolved entries
       for the entity slot tokens. If a step's slot is unresolved, fall back to
       discovery for that step only.
    -> the Playbook's expected_tool_calls is a budget; if you find yourself running
       materially more, record the divergence via `domotz-cli playbook amend`
       at end-of-session.

elif Notes present (no Playbook):
    -> read Notes verbatim before any discovery step; they carry known gotchas
       for this query family even when no structured choreography exists yet.

elif Found AND Results[0].EntityMatch == "exact" AND Results[0].Confidence >= 2:
    -> skip discovery; fetch live data for Results[*].ResourceID in parallel

elif Found AND Results[0].EntityMatch == "partial":
    -> candidate hint, NOT a hit; read the resource title to validate before trusting

elif (any row in Mismatches[] when --debug-mismatches was passed):
    -> treat as cold start; the stored learning is for a different entity
       (different canonical resolved from query_entities)

else:  // Found == false, no playbook, no notes
    -> cold start; run discovery normally; teach the answer afterward (Step 4).
       If the family has no playbook yet, that teach auto-synthesizes a
       playbook candidate from this session's journal - you do not need to
       record one by hand.
```

Playbook and Notes are orthogonal to the per-resource path. A recall response can carry both a Playbook AND a `Results[]` hit - use both: the Playbook tells you which choreography to run; the resource hits short-circuit specific steps. Default to skipping `mismatches`; pass `--debug-mismatches` only when investigating cold-start surprises.

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `domotz-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `domotz-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
domotz-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
domotz-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
domotz-cli teach-playbook \
  --query "$QUERY" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`. Pass the query and note as argv/MCP arguments, or write each with a non-shell file tool and read them back (`QUERY=$(cat ...)`, `NOTE=$(cat ...)`). Do not interpolate either string into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
NOTE=$(cat /path/to/note.txt)
domotz-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between seasons, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-team / per-athlete / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`domotz-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `DOMOTZ_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
domotz-cli feedback "the --since flag is inclusive but docs say exclusive"
domotz-cli feedback --stdin < notes.txt
domotz-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `DOMOTZ_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `DOMOTZ_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` as a private 0600 file (exclusive randomly named temp in the destination directory, then rename; a planted symlink is not followed). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`). Redirects are not followed; a 3xx response fails the delivery and the body is not sent to the `Location`. |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures, including redirects, return non-zero and log the URL and HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

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
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `domotz-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add domotz-mcp -- domotz-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `domotz-mcp --transport http` requires `DOMOTZ_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `DOMOTZ_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `DOMOTZ_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which domotz-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   domotz-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `domotz-cli <command> --help`.
