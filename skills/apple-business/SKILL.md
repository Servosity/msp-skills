---
name: apple-business
description: "Every Apple Business API call, plus offline fleet answers: unassigned devices, AppleCare expiry and Activation Lock. Trigger phrases: `which MDM is this serial assigned to`, `find Apple devices not assigned to an MDM`, `AppleCare expiring for this customer`, `assign these serials to Jamf in Apple Business`, `Apple Business audit log`, `use apple-business`, `run apple-business-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Apple"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - apple-business-cli
    install:
      - kind: script
        bins: [apple-business-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.ps1
---

# Apple Business Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `apple-business-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/apple-business/install.ps1 | iex
   ```
3. Verify: `apple-business-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `apple-business-cli` and `apple-business-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Inventory, device management services, AppleCare coverage, Activation Lock, users, groups, Blueprints, Configurations and the audit log, all scriptable with --json, --select and --dry-run. Sync an org into SQLite and run devices enrich once to answer fleet questions offline; each customer org's API account gets its own local store, and fleet summary rolls them up.

## When to Use This CLI

Use this CLI when an agent needs facts about Apple devices an organization owns in Apple Business: which serials exist, which device management service each is assigned to, AppleCare coverage, Activation Lock, and who changed what in the audit log. It is also the right tool to assign or unassign devices and to manage Blueprints, Configurations and device management services. Fleet reports (devices unassigned, applecare expiring, devices locked, fleet summary) read the local store: run sync, then devices enrich, first. Releasing devices from the org (device-activities create --data-attributes-activity-type RELEASE_DEVICES) is irreversible; only do it on an explicit request.

## Anti-triggers

Do not use this CLI for:
- Do not use it to send MDM commands to enrolled devices (lock, wipe, restart) or to manage apps inside a third-party MDM such as Jamf, Kandji or Intune; Apple Business only assigns devices to those services.
- Do not use it for Apple Business Connect (Maps place cards, Brands, Locations); that is a different Apple API.
- Do not use it for Apple School Manager orgs; the base URL and scope differ (api-school.apple.com, school.api).

## Unique Capabilities

### Live device workflows
- **`devices lookup`**  -  One row per device with its org record, assigned MDM server, AppleCare and Activation Lock, by serial (live) or by MAC, IMEI, EID or order number (needs sync first).

  _Reach for this when a ticket names one device and you need everything Apple Business knows about it in one call._

  ```bash
  apple-business-cli devices lookup C02XL0ABJGH5 --agent
  ```
- **`devices assign`**  -  Assign or move serials to a device management service by name; with --wait, poll the activity and verify every serial landed.

  _Use this instead of hand-building an orgDeviceActivities body when devices must reach the right MDM._

  ```bash
  apple-business-cli devices assign --server "Jamf Prod" C02XL0ABJGH5 --wait --dry-run
  ```
- **`devices unassign`**  -  Remove serials from whatever device management service they are on now, one activity per server; with --wait, verify each.

  _Use this when returning or re-homing devices without knowing which MDM currently holds them._

  ```bash
  apple-business-cli devices unassign C02XL0ABJGH5 --wait --dry-run
  ```
- **`devices enrich`**  -  Fetch AppleCare coverage, Activation Lock status and the serial-to-server map for every synced device into the local store, resumable.

  _Run this after sync and before any AppleCare or Activation Lock report._

  ```bash
  apple-business-cli devices enrich --applecare --activation-lock --servers --skip-fresh 7d
  ```

### Fleet reports from the local store
- **`applecare expiring`**  -  Devices whose AppleCare ends within a window (or, with --uncovered, with no active coverage), with optional counts by order, family, server or org. Needs sync and devices enrich --applecare.

  _Use this for renewal quotes and QBR material instead of thousands of per-device calls._

  ```bash
  apple-business-cli applecare expiring --within 90d --group-by order --agent
  ```
- **`devices unassigned`**  -  Org-owned devices not assigned to any MDM server, with days since it joined the org and a flag when no server defaults their product family. Needs sync of devices and mdm_servers.

  _Run this weekly so new devices enroll at unboxing instead of failing in front of the user._

  ```bash
  apple-business-cli devices unassigned --added-within 7d --agent
  ```
- **`devices locked`**  -  Devices currently Activation Locked, user locks by default, with the owner when built-in device management data is synced. Needs devices enrich --activation-lock.

  _Use this before redeploying or releasing hardware so a user lock does not strand a device._

  ```bash
  apple-business-cli devices locked --type user --agent
  ```
- **`mdm-servers health`**  -  Each MDM server's status, device count, days since it last connected to Apple and its default product families, flagging stale servers (ACTIVE, owns devices, silent past --stale) and INACTIVE or DELETED servers that still own devices.

  _Use this when devices stop checking in or fleet summary shows stale servers: it names the server and how long it has been silent._

  ```bash
  apple-business-cli mdm-servers health --all-orgs --issues-only --agent
  ```

### Cross-customer and audit
- **`audit-events digest`**  -  A readable change log over a relative window: one flattened row per event, newest first, plus per-category counts.

  _Use this for weekly change reviews and security evidence instead of raw audit-events list calls._

  ```bash
  apple-business-cli audit-events digest --since 7d --agent
  ```
- **`fleet summary`**  -  One row of counts per customer org: devices, unassigned, AppleCare expiring and uncovered, user-locked, stale MDM servers, sync age. Each org needs sync and devices enrich under its own API account.

  _Use this for a Monday cross-customer view before drilling into one org._

  ```bash
  apple-business-cli fleet summary --within 90d --agent
  ```
- **`audit-events actors`**  -  Who made changes over a relative window: one row per actor (admin user or API account, grouped by actor id) with event counts, non-success outcomes, categories and event types.

  _Use this for a SOC 2 or customer security review: who touched the org, how often, and which changes failed._

  ```bash
  apple-business-cli audit-events actors --since 30d --agent
  ```

## Command Reference

**apps**  -  Manage apps

- `apple-business-cli apps get`  -  Get information about a specific app for an organization using the built-in device management in Apple Business.
- `apple-business-cli apps list`  -  Get a list of apps for an organization using the built-in device management in Apple Business.

**audit_events**  -  Manage audit events

- `apple-business-cli audit-events`  -  Get a list of audit events in an organization that satisfies the query criteria.

**blueprints**  -  Manage blueprints

- `apple-business-cli blueprints add-apps`  -  Add apps to a Blueprint.
- `apple-business-cli blueprints add-configurations`  -  Add Configurations to a Blueprint.
- `apple-business-cli blueprints add-devices`  -  Add devices to a Blueprint.
- `apple-business-cli blueprints add-packages`  -  Add packages to a Blueprint.
- `apple-business-cli blueprints add-user-groups`  -  Add user groups to a Blueprint.
- `apple-business-cli blueprints add-users`  -  Add users to a Blueprint.
- `apple-business-cli blueprints create`  -  Create a new Blueprint in an organization.
- `apple-business-cli blueprints delete`  -  Delete a Blueprint from an organization.
- `apple-business-cli blueprints get`  -  Get information about a specific Blueprint in an organization.
- `apple-business-cli blueprints list`  -  Get a list of Blueprints in an organization.
- `apple-business-cli blueprints list-apps`  -  Get a list of app IDs associated with a Blueprint.
- `apple-business-cli blueprints list-configurations`  -  Get a list of Configuration IDs associated with a Blueprint.
- `apple-business-cli blueprints list-devices`  -  Get a list of device IDs associated with a Blueprint.
- `apple-business-cli blueprints list-packages`  -  Get a list of package IDs associated with a Blueprint.
- `apple-business-cli blueprints list-user-groups`  -  Get a list of user group IDs associated with a Blueprint.
- `apple-business-cli blueprints list-users`  -  Get a list of user IDs associated with a Blueprint.
- `apple-business-cli blueprints remove-apps`  -  Remove apps from a Blueprint.
- `apple-business-cli blueprints remove-configurations`  -  Remove Configurations from a Blueprint.
- `apple-business-cli blueprints remove-devices`  -  Remove devices from a Blueprint.
- `apple-business-cli blueprints remove-packages`  -  Remove packages from a Blueprint.
- `apple-business-cli blueprints remove-user-groups`  -  Remove user groups from a Blueprint.
- `apple-business-cli blueprints remove-users`  -  Remove users from a Blueprint.
- `apple-business-cli blueprints update`  -  Update an existing Blueprint in an organization.

**configurations**  -  Manage configurations

- `apple-business-cli configurations create`  -  Create a new custom Configuration in an organization.
- `apple-business-cli configurations delete`  -  Delete a Configuration from an organization.
- `apple-business-cli configurations get`  -  Get information about a specific Configuration in an organization.
- `apple-business-cli configurations list`  -  Get a list of Configurations in an organization.
- `apple-business-cli configurations update`  -  Update an existing custom Configuration in an organization.

**device_activities**  -  Manage device activities

- `apple-business-cli device-activities create`  -  Assign or unassign devices to a device management service, manage a device management service migration for devices
- `apple-business-cli device-activities get`  -  Get information for an organization device activity that a device management action, such as assign or unassign

**devices**  -  Manage devices

- `apple-business-cli devices activation-lock`  -  Get the Activation Lock status of an organization device.
- `apple-business-cli devices applecare-coverage`  -  Get a list of AppleCare coverage resources for an organization device.
- `apple-business-cli devices assigned-server`  -  Get the assigned device management service information for a device.
- `apple-business-cli devices assigned-server-id`  -  Get the assigned device management service ID information for a device.
- `apple-business-cli devices get`  -  Get information about a device in an organization.
- `apple-business-cli devices list`  -  Get a list of devices in an organization that enroll using Automated Device Enrollment.

**mdm_devices**  -  Manage mdm devices

- `apple-business-cli mdm-devices details`  -  Get detailed information about a device enrolled in Apple Device Management service.
- `apple-business-cli mdm-devices list`  -  Get a list of devices enrolled in Apple Device Management service.

**mdm_servers**  -  Manage mdm servers

- `apple-business-cli mdm-servers create`  -  Create a new device management service in an organization.
- `apple-business-cli mdm-servers delete`  -  Delete a device management service from an organization.
- `apple-business-cli mdm-servers devices`  -  Get a list of device serial numbers assigned to a device management service.
- `apple-business-cli mdm-servers get`  -  Get the information for a specific device management service.
- `apple-business-cli mdm-servers list`  -  Get a list of device management services in an organization.
- `apple-business-cli mdm-servers update`  -  Update an existing device management service in an organization.

**org_units**  -  Manage org units

- `apple-business-cli org-units get`  -  Get information about a specific organizational unit in an organization.
- `apple-business-cli org-units list`  -  Get a list of organizational units in an organization.
- `apple-business-cli org-units users`  -  Get a list of user IDs for an organizational unit in an organization.

**packages**  -  Manage packages

- `apple-business-cli packages get`  -  Get information about a specific package for an organization using Apple Business's built-in device management.
- `apple-business-cli packages list`  -  Get a list of packages for an organization using Apple Business's built-in device management.

**user_groups**  -  Manage user groups

- `apple-business-cli user-groups get`  -  Get information about a specific user group in an organization.
- `apple-business-cli user-groups list`  -  Get a list of user groups in an organization.
- `apple-business-cli user-groups users`  -  Get a list of user IDs for a user group in an organization.

**users**  -  Manage users

- `apple-business-cli users get`  -  Get information about a specific user in an organization.
- `apple-business-cli users list`  -  Get a list of users in an organization.


**applecare**


- `apple-business-cli applecare expiring`  -  Devices whose AppleCare ends within a window (or, with --uncovered, with no active coverage), with optional counts by order, family, server or org. Needs sync and devices enrich --applecare.


**audit-events**


- `apple-business-cli audit-events digest`  -  A readable change log over a relative window: one flattened row per event, newest first, plus per-category counts.


**devices**


- `apple-business-cli devices lookup`  -  One row per device with its org record, assigned MDM server, AppleCare and Activation Lock, by serial (live) or by MAC, IMEI, EID or order number (needs sync first).

- `apple-business-cli devices assign`  -  Assign or move serials to a device management service by name; with --wait, poll the activity and verify every serial landed.

- `apple-business-cli devices unassign`  -  Remove serials from whatever device management service they are on now, one activity per server; with --wait, verify each.

- `apple-business-cli devices enrich`  -  Fetch AppleCare coverage, Activation Lock status and the serial-to-server map for every synced device into the local store, resumable.

- `apple-business-cli devices unassigned`  -  Org-owned devices not assigned to any MDM server, with days since it joined the org and a flag when no server defaults their product family. Needs sync of devices and mdm_servers.

- `apple-business-cli devices locked`  -  Devices currently Activation Locked, user locks by default, with the owner when built-in device management data is synced. Needs devices enrich --activation-lock.


**fleet**


- `apple-business-cli fleet summary`  -  One row of counts per customer org: devices, unassigned, AppleCare expiring and uncovered, user-locked, stale MDM servers, sync age. Each org needs sync and devices enrich under its own API account.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
apple-business-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Weekly new-device intake

```bash
apple-business-cli devices assign --server "Jamf Prod" --from-unassigned --family Mac --added-within 7d --wait
```

After sync, assigns last week's unassigned Macs to one server by name, re-checks each live, waits and verifies; preview the selection first with devices unassigned --family Mac --added-within 7d, or add --dry-run to see the request with the resolved serials.

### AppleCare renewals this quarter

```bash
apple-business-cli applecare expiring --within 90d --group-by order --agent
```

After sync and devices enrich --applecare: devices whose coverage ends in 90 days, with a count per purchase order for quoting.

### Narrow a large device payload for an agent

```bash
apple-business-cli devices list --agent --select id,attributes.serialNumber,attributes.productFamily,attributes.status
```

List items are already unwrapped from Apple's data envelope, so select fields by attributes.<name>; --agent alone compacts to ids.

### Last week's changes

```bash
apple-business-cli audit-events digest --since 7d --agent
```

Device moves, API key events and domain changes, one flattened row per event, newest first, plus per-category counts.

### Before redeploying returns

```bash
apple-business-cli devices locked --type user --agent
```

After devices enrich --activation-lock: user Activation Locks need the previous owner; the owner is shown when built-in device management data (mdm_devices, users) is synced.

## Auth Setup

Apple Business uses OAuth 2 client credentials with a signed client assertion. An Organization Administrator opens Settings > API in Apple Business, selects Add API Account, then Generate & Download to save the private key (.pem, generated once); Edit shows the Client ID (BUSINESSAPI.<uuid>) and Key ID. Set APPLE_BUSINESS_CLIENT_ID, APPLE_BUSINESS_KEY_ID and APPLE_BUSINESS_PRIVATE_KEY (prefer the path to the .pem file so the key text is never copied to disk), or run auth login --client-id <id> --client-secret <path.pem> --key-id <key id>. The CLI signs a 10-minute ES256 assertion, exchanges it at account.apple.com for a one-hour bearer token, keeps it (in the private credential store after auth login; in memory only for an account given in the environment or --config), and re-mints on expiry or 401. Several customer orgs: API accounts belong to one organization, so use one per customer and switch by changing the three APPLE_BUSINESS_* variables (or --config <org>.toml with business_client_id, business_private_key and business_key_id). The explicit account always wins over a saved login, and each Client ID syncs into its own store in the shared data directory, which --all-orgs and fleet summary read. The --client-profile flag is not used by this CLI. A `--config <org>.toml` file (or APPLE_BUSINESS_CONFIG) is one customer's whole saved identity: `auth login --config` saves the account and key reference in that file, a run under it never uses the shared saved login, and `auth logout --config` clears only that file.

To use a bearer token minted elsewhere instead, set APPLE_BUSINESS_ACCESS_TOKEN: while it is set, it is the run's whole identity. It replaces any configured or saved account, requests send it, its data stays in its own local store, the private key is not read and nothing is minted or saved. It expires in an hour and is not renewed. Leave it blank otherwise.

Run `apple-business-cli doctor` to verify setup.

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
  apple-business-cli apps list --agent --select id
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

- Use `--home <dir>` for one invocation, or set `APPLE_BUSINESS_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `APPLE_BUSINESS_CONFIG_DIR`, `APPLE_BUSINESS_DATA_DIR`, `APPLE_BUSINESS_STATE_DIR`, `APPLE_BUSINESS_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `APPLE_BUSINESS_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir, or under `--config` in that 0600 file. `auth login`, `auth set-token` and `auth logout` are refused over a config file other users can change while that file names hosts, TLS settings or headers: review it and `chmod 600` it first. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `apple-business-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "apple-business": {
        "command": "apple-business-mcp",
        "env": {
          "APPLE_BUSINESS_HOME": "/srv/apple-business"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `APPLE_BUSINESS_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `APPLE_BUSINESS_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; there is no response cache (every live read goes to Apple). `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `APPLE_BUSINESS_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `APPLE_BUSINESS_DATA_SOURCE` sets the `--data-source` default.

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
apple-business-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "apple-business-cli learnings confirm 12"] }
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
       materially more, record the divergence via `apple-business-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `apple-business-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `apple-business-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
apple-business-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
apple-business-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
apple-business-cli teach-playbook \
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
apple-business-cli playbook amend \
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

`apple-business-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `APPLE_BUSINESS_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
apple-business-cli feedback "the --since flag is inclusive but docs say exclusive"
apple-business-cli feedback --stdin < notes.txt
apple-business-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `APPLE_BUSINESS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `APPLE_BUSINESS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
apple-business-cli profile save briefing --json
apple-business-cli --profile briefing apps list
apple-business-cli profile list --json
apple-business-cli profile show briefing
apple-business-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `apple-business-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code, passing the API account (or run `apple-business-cli auth login` first and omit the `-e` flags):
   ```bash
   claude mcp add apple-business-mcp -e APPLE_BUSINESS_CLIENT_ID=BUSINESSAPI.<uuid> -e APPLE_BUSINESS_KEY_ID=<key-id> -e APPLE_BUSINESS_PRIVATE_KEY=/path/to/private-key.pem -- apple-business-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `apple-business-mcp --transport http` requires `APPLE_BUSINESS_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `APPLE_BUSINESS_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `APPLE_BUSINESS_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which apple-business-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   apple-business-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `apple-business-cli <command> --help`.
