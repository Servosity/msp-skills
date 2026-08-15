---
name: auvik
description: "Every Auvik endpoint as a command, plus the cross-client answers the Auvik UI and API cannot give you. Trigger phrases: `what is end of life across my clients`, `which devices have no config backup`, `what devices disappeared from auvik`, `reconcile auvik billable device counts`, `why can't auvik see this device`, `which client is generating the most alerts`, `use auvik`, `run auvik`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Auvik"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - auvik-cli
    install:
      - kind: go
        bins: [auvik-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/auvik/cmd/auvik-cli
---

# Auvik  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `auvik-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install auvik --cli-only
   ```
2. Verify: `auvik-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.6 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/auvik/cmd/auvik-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Auvik holds the richest network-truth dataset an MSP has, behind a read-only JSON:API with bracketed filters, cursor pagination, and per-region hosts. Every existing tool is a language binding that hands you typed structs and leaves the question unanswered. This CLI mirrors Auvik into local SQLite so you can ask things no Auvik surface supports: what is end-of-life across every client (eol), which devices are missing a required config line (configuration grep --not), and which devices disappeared since last sync (inventory diff) - a removal Auvik never reports.

## When to Use This CLI

Use this CLI for read-only questions about network inventory, device health, config history, alerts, billing usage, and SaaS app inventory held in Auvik - especially questions that span more than one client tenant. It is strongest where Auvik itself is weakest: cross-tenant rollups, absence queries over device configs, and detecting devices that disappeared. The only write this API supports anywhere is dismissing an alert. Run 'sync' before the local-store commands (eol, changes, configuration audit, inventory diff, usage reconcile, device discovery-gaps, alert noise, asm shadow); use the typed endpoint commands or the 'api' escape hatch for live single-tenant lookups.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI to change device configuration - Auvik's API is read-only apart from dismissing an alert, and this CLI cannot push config.
- Do not use this CLI to create, edit, or delete devices, networks, tenants, credentials, or SNMP poller settings; no such endpoints exist.
- Do not use this CLI to open PSA or ticketing records; it has no PSA integration.
- Do not use this CLI for real-time streaming or sub-minute monitoring; statistics endpoints are historical time-windowed series.
- Do not use this CLI to remote-control or tunnel into a device; that is the Auvik UI's remote-access feature, not the API.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Cross-client answers Auvik cannot give
- **`eol`**  -  See every device approaching or past end-of-support across all your clients at once, bucketed by urgency.

  _Reach for this when asked what hardware is aging out across a book of business  -  it is the one question Auvik's UI cannot answer at all._

  ```bash
  auvik-cli eol --agent
  ```
- **`changes`**  -  Merge config revisions, audit entries, notes, and alerts for one device into a single chronological story.

  _Use this to answer 'what happened to this device and who touched it' without opening four screens._

  ```bash
  auvik-cli changes 5f2b91c4 --agent
  ```
- **`inventory diff`**  -  List devices added, removed, or changed fleet-wide since the last sync, attributed to each client.

  _Reach for this when a device count moved and you need to know which devices caused it._

  ```bash
  auvik-cli inventory diff --since 7d --agent
  ```

### Config integrity at fleet scale
- **`configuration audit`**  -  Find devices with no configuration backup, a stale backup, or no running-config backup, across every client.

  _Reach for this to prove which devices are unprotected across a whole book of business; it cannot search config text, because the API does not expose it._

  ```bash
  auvik-cli configuration audit --agent
  ```
- **`device discovery-gaps`**  -  List every device Auvik cannot fully poll, per client, with the credential state behind each gap.

  _Answers 'why can't we see this box' in one command instead of three screens._

  ```bash
  auvik-cli device discovery-gaps --agent
  ```

### Billing and count integrity
- **`usage reconcile`**  -  Put each client's billable usage count next to the actual synced inventory and show the device rows behind the difference.

  _Use this before invoicing when a client's billable count disagrees with the agreement._

  ```bash
  auvik-cli usage reconcile --agent
  ```
- **`asm shadow`**  -  Surface SaaS apps with active users but no license record, and licenses nobody is using, per client.

  _Reach for this when building a client's software-spend narrative._

  ```bash
  auvik-cli asm shadow --agent
  ```

### Alert memory
- **`alert noise`**  -  Rank devices and clients by alert volume over a window, with device names, types, clients and severity mix resolved.

  _Use this for the shift handoff or the chronic-offender conversation. Auvik publishes no dismissal timestamp, so this reports dismissal counts, not time-to-dismiss._

  ```bash
  auvik-cli alert noise --since 30d --agent
  ```

## Command Reference

**alert**  -  The Auvik Alert API allows you to dismiss an alert that Auvik has triggered.

There is a single endpoint availble within the Alert API.

- Dismiss Alert. Dismiss a single alert.

- `auvik-cli alert dismiss-single`  -  Use the Dismiss Alert API to dismiss a specific alert that Auvik has triggered
- `auvik-cli alert read-multiple-info`  -  Use the Read Multiple Alerts’ Info API to pull the collected information about the various alerts that Auvik has
- `auvik-cli alert read-single-info`  -  Use the Read Single Alert’s Info API to pull the collected information about a specific alert that Auvik has triggered.

**asm**  -  The Auvik SaaS Management API allows you to access all SaaS related data collected via the Auvik SaaS Management product. The Auvik SaaS Management API includes all core data included as a part of the product including Applications, Monitored Users, Security Logs, and other key data.

- `auvik-cli asm read-multiple-asmapp-info`  -  Use the Read Multiple ASM Applications' Info to retrieve the information related to the SaaS applications discovered
- `auvik-cli asm read-multiple-asmclient-info`  -  The Read Multiple ASM Clients' Info API returns relevant meta data about clients that exist within the Auvik SaaS
- `auvik-cli asm read-multiple-asmlicense-info`  -  Use the Read Multiple ASM Licenses' Info endpoint to retrieve information about an application licenses discovered
- `auvik-cli asm read-multiple-asmsecurity-log-info`  -  Use the Read Multiple ASM Security Logs' Info API to retrieve in depth information about the security logs within an
- `auvik-cli asm read-multiple-asmtag-info`  -  Use the Read Multiple ASM Tags' Info API to retrieve information about tags configured within an ASM client.
- `auvik-cli asm read-multiple-asmuser-info`  -  Use the Read Multiple ASM Users' Info API to retrieve information about any monitored users that exist within a

**authentication**  -  Manage authentication

- `auvik-cli authentication`  -  Use the Verify Credentials API to verify your credentials are correct before making a call to an endpoint.

**auvik-inventory**  -  Manage auvik inventory

- `auvik-cli auvik-inventory read-multiple-device-discovery-status-v2`  -  Returns the discovery status of multiple devices for a tenant.
- `auvik-cli auvik-inventory read-multiple-device-info-v2`  -  Use the Read Multiple Devices’ Info API to pull the collected information about the various devices Auvik has
- `auvik-cli auvik-inventory read-multiple-device-lifecycle-v2`  -  Returns lifecycle information for multiple devices for a tenant.
- `auvik-cli auvik-inventory read-multiple-interface-info-v2`  -  Use the Read Multiple Interfaces’ Info API to pull the collected information about the various interfaces Auvik has
- `auvik-cli auvik-inventory read-single-device-discovery-status-v2`  -  Returns the discovery status of a single device. You will need the device ID for the specific device.
- `auvik-cli auvik-inventory read-single-device-info-v2`  -  Use the Read Single Device’s Info API to pull the collected information about a specific device Auvik has discovered.
- `auvik-cli auvik-inventory read-single-device-lifecycle-v2`  -  Returns lifecycle information for a single device. You will need the device ID for the specific device.
- `auvik-cli auvik-inventory read-single-interface-info-v2`  -  Use the Read Single Interface’s Info API to pull the collected information about a specific interface Auvik has

**auvik-stat**  -  Manage auvik stat

- `auvik-cli auvik-stat`  -  Use the Read Service Statistics API to fetch detailed statistics of a client's (and client's children if a multi-client)

**billing**  -  Manage billing

- `auvik-cli billing read-client-usage`  -  Use the Read Client Usage API to pull a summary of a client’s (and client’s children if a multi-client)
- `auvik-cli billing read-device-usage`  -  Use the Read Device Usage API to pull a summary of a client’s (and client’s children if a multi-client)

**inventory**  -  Manage inventory

- `auvik-cli inventory read-multiple-component-info`  -  Use the Read Multiple Components’ Info API to pull collected information about various device components Auvik has
- `auvik-cli inventory read-multiple-configurations`  -  Use the Read Multiple Device Configuration API to pull all device configurations.
- `auvik-cli inventory read-multiple-device-details`  -  Use the Read Multiple Devices’ Details API to pull extra collected information about the various devices Auvik has
- `auvik-cli inventory read-multiple-device-extended-detail`  -  Use the Read Multiple Devices’ Extended Details API to get many devices’ extended details.
- `auvik-cli inventory read-multiple-device-info`  -  Use the Read Multiple Devices’ Info API to pull the collected information about the various devices Auvik has
- `auvik-cli inventory read-multiple-device-lifecycle`  -  Use the Read Multiple Devices’ Lifecycle API to pull the collected lifecycle information about the various devices
- `auvik-cli inventory read-multiple-device-warranty`  -  Use the Read Multiple Devices’ Warranty API to pull the collected warranty information about the various devices Auvik
- `auvik-cli inventory read-multiple-entity-audit`  -  Use the Read Multiple Entity Audits API pull information about multiple entity audits for you clients.
- `auvik-cli inventory read-multiple-entity-note`  -  Use the Read Multiple Entity Notes API pull information about multiple entity notes.
- `auvik-cli inventory read-multiple-interface-info`  -  Use the Read Multiple Interfaces Info API to pull the collected information about the various device interfaces Auvik
- `auvik-cli inventory read-multiple-network-details`  -  Use the Read Multiple Networks’ Details API to pull extra collected information about the various networks Auvik has
- `auvik-cli inventory read-multiple-network-info`  -  Use the Read Multiple Networks’ Info API to pull the collected information about the various networks Auvik has
- `auvik-cli inventory read-single-component-info`  -  Use the Read Single Component’s Info API to pull collected information about a specific device component Auvik has
- `auvik-cli inventory read-single-configuration`  -  Use the Read Single Device Configuration API to pull a single device configuration.
- `auvik-cli inventory read-single-device-details`  -  Use the Read Single Device’s Details API to pull extra collected information about a specific device Auvik has
- `auvik-cli inventory read-single-device-extended-detail`  -  Use the Read Single Device’s Extended Details API to get a device’s extended details.
- `auvik-cli inventory read-single-device-info`  -  Use the Read Single Device’s Info API to pull the collected information about a specific device Auvik has discovered.
- `auvik-cli inventory read-single-device-lifecycle`  -  Use the Read Single Device’s Lifecycle Info API to pull the collected information about a specific device Auvik has
- `auvik-cli inventory read-single-device-warranty`  -  Use the Read Single Device’s Warranty Info API to pull the collected information about a specific device Auvik has
- `auvik-cli inventory read-single-entity-audit`  -  Use the Single Multiple Entity Audit API pull information about a single entity audit.
- `auvik-cli inventory read-single-entity-note`  -  Use the Read Single Entity Note API to pull the information about a specific entity note.
- `auvik-cli inventory read-single-interface-info`  -  Use the Read Single Interface Info API to pull the collected information about a specific device interface Auvik has
- `auvik-cli inventory read-single-network-details`  -  Use the Read Single Networks’s Details API to pull extra collected information about a specific network Auvik has
- `auvik-cli inventory read-single-network-info`  -  Use the Read Single Network’s Info API to pull the collected information about a specific network Auvik has discovered.

**meta**  -  Manage meta

- `auvik-cli meta`  -  Pulls metadata information for a specific API endpoint and field.

**settings**  -  Manage settings

- `auvik-cli settings read-multiple-snmp-poller`  -  Use the Read Multiple SNMP Poller Settings API to pull the list of SNMP Poller Settings configured in Auvik.
- `auvik-cli settings read-multiple-snmp-poller-devices`  -  Use Read SNMP Poller Setting's Devices API to pull the list of devices that apply to a specific SNMP Poller Setting Id.
- `auvik-cli settings read-snmp-poller-single`  -  Use the Read Single SNMP Poller Setting API to pull details of a specific SNMP Poller Setting configured in Auvik.

**stat**  -  Manage stat

- `auvik-cli stat read-component-statistics`  -  Use the Read Component Statistics API to fetch detailed statistics of a client's (and client's children if a
- `auvik-cli stat read-device-availability-statistics`  -  Use the Read Device Availability Statistics API to fetch detailed availability statistics of a client’s (and client’s
- `auvik-cli stat read-device-statistics`  -  Use the Read Device Statistics API to fetch detailed statistics of a client’s (and client’s children if a multi-client)
- `auvik-cli stat read-interface-statistics`  -  Use the Read Interface Statistics API to fetch detailed statistics of a client's (and client's children if a
- `auvik-cli stat read-multiple-snmp-poller-setting-int-history`  -  Use the Read SNMP Poller Setting's History API to fetch the list of historical vaules for a SNMP Poller Setting.
- `auvik-cli stat read-multiple-snmp-poller-setting-string-history`  -  Use the Read SNMP Poller Setting's History API to fetch the list of historical vaules for a SNMP Poller Setting.
- `auvik-cli stat read-oid-statistics`  -  Use the Read OID Statistics API to fetch the last recorded value of a monitored device OID.
- `auvik-cli stat read-service-statistics`  -  Use the Read Service Statistics API to fetch detailed statistics of a client’s (and client’s children if a multi-client)

**tenants**  -  The Auvik Tenant API allows you to see if you have access to multi-clients or clients associated to your Auvik user account. The output from the API shows if you have permissions to a multi-client or client, but doesn’t show the associated role permissions.

 There are three endpoints within the Tenant API.

- Read Multiple Tenants: Pulls access detail about multiple multi-clients and clients associated with your Auvik user account.

- Read Multiple Tenants Detail: Pulls details for multiple multi-clients and clients associated with your Auvik user account.

- Read single Tenant Detail: Pulls detail for a specific multiple multi-client or client associated with your Auvik user account.

- `auvik-cli tenants read-multiple`  -  Use the Read Multiple Tenants API to pull access detail about multiple multi-clients and clients associated with your
- `auvik-cli tenants read-multiple-detail`  -  Use the Read Multiple Tenants API to pull details for multiple multi-clients and clients associated with your main
- `auvik-cli tenants read-single-detail`  -  Use the Read a Single Tenant API to pull detail about a specific multi-client and client associated with your main


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
auvik-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### End-of-support exposure across the whole book

```bash
auvik-cli eol --agent --select rows.client,rows.device_name,rows.make_model,rows.end_of_support
```

The quarterly-business-review answer in one line. The report is an envelope, so select through 'rows.' to narrow to the four fields that go on the slide.

### Prove which devices are not backed up

```bash
auvik-cli configuration audit --finding no_backup --agent
```

Auvik's API exposes backup metadata but not config bodies, so this is the fleet-wide compliance question it can answer: which devices have no configuration backup at all.

### Narrow a deeply nested JSON:API device response

```bash
auvik-cli device list --agent --select attributes.deviceName,attributes.deviceType,attributes.onlineStatus
```

Auvik wraps everything in JSON:API resource objects. The CLI strips the outer envelope, so select from 'attributes.' to cut a multi-KB device list down to the three fields you actually read.

### Explain a billable-count change before invoicing

```bash
auvik-cli usage reconcile --agent
```

Shows the device rows behind each client's count delta instead of just the number the usage endpoint returns.

### Reconstruct what happened to one device

```bash
auvik-cli changes 5f2b91c4 --agent
```

Merges config revisions, audit entries, notes, and alerts for that device into one chronological stream.

## Auth Setup

Auvik uses HTTP Basic authentication: your Auvik user email is the username and your API key is the password. Both are required - there is no single-token form. Save them with 'auvik-cli auth set-credentials <your-email> <your-api-key>', or export AUVIK_USERNAME and AUVIK_API_KEY. Check state any time with 'auvik-cli auth status'.

Set your region before your first call. The base URL is per-region and the built-in default is us1, which is WRONG for every tenant outside us1 - and a valid key against the wrong region returns 401 exactly like a bad key. Find your region in the URL of your Auvik dashboard and export AUVIK_BASE_URL to match: https://auvikapi.<region>.my.auvik.com where <region> is one of us1, us2, us3, us4, eu1, ca1, au1.

The API user also needs an appropriate role in every tenant you query, or that tenant returns 403 while others succeed.

Run `auvik-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  auvik-cli alert read-multiple-info --agent --select alertDefinitionId,description,detectedOn
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success

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

- Use `--home <dir>` for one invocation, or set `AUVIK_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `AUVIK_CONFIG_DIR`, `AUVIK_DATA_DIR`, `AUVIK_STATE_DIR`, `AUVIK_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `AUVIK_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `auvik-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "auvik": {
        "command": "auvik-mcp",
        "env": {
          "AUVIK_HOME": "/srv/auvik"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `AUVIK_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `AUVIK_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, run:

```bash
auvik-cli recall "<user's question>" --agent
```

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
      "next_action": ["<trial command>", "auvik-cli learnings confirm 12"] }
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
       materially more, record the divergence via `auvik-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `auvik-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `auvik-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately:

```bash
auvik-cli teach --query "<user's question>" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
auvik-cli teach \
  --query "<user's question>" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
auvik-cli teach-playbook \
  --query "<user's question>" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`.

```bash
auvik-cli playbook amend \
  --query "<exact recall query string>" \
  --add-note "<your concrete correction>"
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

`auvik-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `AUVIK_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
auvik-cli feedback "the --since flag is inclusive but docs say exclusive"
auvik-cli feedback --stdin < notes.txt
auvik-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `AUVIK_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `AUVIK_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
auvik-cli profile save briefing --json
auvik-cli --profile briefing alert read-multiple-info
auvik-cli profile list --json
auvik-cli profile show briefing
auvik-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `auvik-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/auvik/cmd/auvik-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add auvik-mcp -- auvik-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which auvik-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   auvik-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `auvik-cli <command> --help`.
