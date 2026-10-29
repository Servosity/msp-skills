---
name: kaseya-vsa
description: "Every VSA 10 API endpoint, plus a local fleet database that answers patch, health, and software questions across all your clients offline. Trigger phrases: `which devices are missing critical patches`, `kaseya vsa fleet health`, `run a vsa script on a device and get the output`, `who has this software installed in vsa`, `use kaseya vsa`, `run kaseya-vsa`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Kaseya VSA"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - kaseya-vsa-cli
    install:
      - kind: script
        bins: [kaseya-vsa-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.ps1
---

# Kaseya VSA 10 Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `kaseya-vsa-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/kaseya-vsa/install.ps1 | iex
   ```
3. Verify: `kaseya-vsa-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `kaseya-vsa-cli` and `kaseya-vsa-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Sync devices, assets, organizations, and audit logs once, then ask fleet-wide questions without burning the per-endpoint rate limit: health lists devices failing posture checks, patch-exposure ranks missing KBs by how many devices lack them (or by CVSS with --sort cvss), and software finds version drift. scripts run-wait and tasks run-wait run VSA automation and return the output in one call.

## When to Use This CLI

Use this CLI for Kaseya VSA 10 RMM work across many client organizations: fleet health triage, patch exposure, software inventory, running scripts or tasks and collecting their output, and audit review. health, patch-exposure, software and audit-logs digest read the local mirror only, so run 'sync --resources organizations,sites,assets,audit_logs' first; an empty result with scanned_devices 0 means not synced. scripts run-wait and tasks run-wait execute code on managed endpoints immediately with no confirmation prompt; use --dry-run to preview the request.

## Anti-triggers

Do not use this CLI for:
- Kaseya VSA 9 servers that only expose /api/v1.0
- Installing OS patches directly (the v3 API exposes patch policies and pending updates, not patch installs)
- PSA ticketing (use a PSA connector such as Autotask or Kaseya BMS)
- Live remote control sessions

## Unique Capabilities

### Fleet analytics from the local store
- **`health`**  -  From the synced local mirror (run sync first), list devices across every client that are offline, have antivirus off or stale, firewall or UAC off, low disk, or high CPU or memory; rows are fail or unknown, with scanned and per-check counts.

  _Reach for this for Monday-morning fleet triage instead of paging assets and inspecting each record._

  ```bash
  kaseya-vsa-cli health --check offline,av,firewall --offline-for 72h --agent
  ```
- **`patch-exposure`**  -  From the synced local mirror (run sync first), rank pending OS updates across all clients by KB, client, or device, filtered by severity, CVSS, KB, or CVE.

  _Use this to answer which KB or CVE is the worst exposure and where it lives._

  ```bash
  kaseya-vsa-cli patch-exposure --by kb --severity critical,important --min-cvss 7 --agent
  ```
- **`software`**  -  From the synced local mirror (run sync first), find which devices run an application, its version spread, devices below a version, or devices missing it.

  _Use this for version drift, risky remote-access tools, or devices missing a security agent._

  ```bash
  kaseya-vsa-cli software "Google Chrome" --versions --agent
  ```
- **`audit-logs digest`**  -  From the synced local mirror (run sync first), see who did what in the tenant over a time window, with audit categories and event types decoded to names.

  _Use this for weekly review of remote control, API token, account, and bulk-action activity._

  ```bash
  kaseya-vsa-cli audit-logs digest --since 7d --agent
  ```

### Agent-native automation
- **`devices brief`**  -  Get one device's online state, notification counts, security posture, disks, pending-update counts by severity, antivirus, policies, and recent audit activity from six live API calls.

  _Use this first when triaging a single machine escalation._

  ```bash
  kaseya-vsa-cli devices brief FRONTDESK-01 --agent
  ```
- **`scripts run-wait`**  -  Run an automation script on a managed device (starts immediately, no confirmation) and get its exit code and output when it finishes.

  _Use this when an agent needs the result of a script, not just an execution ID._

  ```bash
  kaseya-vsa-cli scripts run-wait 11111111-2222-3333-4444-555555555555 --device FRONTDESK-01 --max-wait 10m --agent
  ```
- **`tasks run-wait`**  -  Run a saved task on chosen devices (starts immediately, no confirmation) or attach to a running execution, and collect per-device, per-script exit codes and output.

  _Use this to collect which devices failed a task execution and why, or to run a task on named devices and wait for the results._

  ```bash
  kaseya-vsa-cli tasks run-wait 1234 --attach 10450 --failed-only --agent
  ```

## HTTP Transport

This CLI uses Chrome-compatible HTTP transport for browser-facing endpoints. It does not require a resident browser process for normal API calls.

`--ca-cert`, `KASEYA_VSA_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `KASEYA_VSA_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. `doctor` reports the trust in use and names the setting that fixes a chain or hostname failure.

## Command Reference

**assets**  -  Manage assets

- `kaseya-vsa-cli assets get-device`  -  Returns the assets for a specific device.
- `kaseya-vsa-cli assets list`  -  Returns a list of assets.

**audit_logs**  -  Manage audit logs

- `kaseya-vsa-cli audit-logs`  -  Returns a list of audit log entries.

**custom_field_usage**  -  Manage custom field usage

- `kaseya-vsa-cli custom-field-usage <customFieldId>`  -  Returns a list of places where the Custom Field is used.

**custom_fields**  -  List and get custom fields; assign, update, or unassign their values

- `kaseya-vsa-cli custom-fields assign`  -  Assigns a VSA 10 Custom Field.
- `kaseya-vsa-cli custom-fields get`  -  Returns the custom field details.
- `kaseya-vsa-cli custom-fields list`  -  Returns a list of Custom Fields.
- `kaseya-vsa-cli custom-fields unassign`  -  Unassigns a VSA 10 Custom Field.
- `kaseya-vsa-cli custom-fields update-assigned`  -  Updates the Value of a VSA 10 Custom Field.

**devices**  -  List and get devices, their antivirus, notifications, policies and custom fields; move, publish, or brief one

- `kaseya-vsa-cli devices get`  -  Returns the device details.
- `kaseya-vsa-cli devices get-antivirus`  -  Returns the antivirus status of a device.
- `kaseya-vsa-cli devices list`  -  Returns a list of devices.
- `kaseya-vsa-cli devices list-applied-policies`  -  Returns a list of policies and profiles applied to a device.
- `kaseya-vsa-cli devices list-custom-fields`  -  Returns the device custom fields.
- `kaseya-vsa-cli devices list-notifications`  -  Return the device notifications.
- `kaseya-vsa-cli devices move`  -  Moves a Device to a different Group.
- `kaseya-vsa-cli devices publish`  -  Publish registers or updates a VSA 10 Device instance.

**endpoint_protection**  -  Manage endpoint protection

- `kaseya-vsa-cli endpoint-protection <policyId>`  -  Returns the details of the endpoint protection policy.

**environment**  -  Manage environment

- `kaseya-vsa-cli environment`  -  Returns environment information for the instance.

**group_custom_fields**  -  Manage group custom fields

- `kaseya-vsa-cli group-custom-fields <groupId>`  -  Return the group custom fields.

**groups**  -  Manage groups

- `kaseya-vsa-cli groups create`  -  Creates a new Group within a Site.
- `kaseya-vsa-cli groups delete`  -  Deletes a Group.
- `kaseya-vsa-cli groups get`  -  Returns the group details.
- `kaseya-vsa-cli groups get-package`  -  Returns the Name and Download URL for Group install package. Rate limit: 600 requests per hour.
- `kaseya-vsa-cli groups list`  -  Returns a list of active (not deleted) groups.
- `kaseya-vsa-cli groups update`  -  Update a Group.

**notifications**  -  Manage notifications

- `kaseya-vsa-cli notifications create`  -  Creates a device notification.
- `kaseya-vsa-cli notifications delete`  -  Deletes a specific notification.
- `kaseya-vsa-cli notifications get`  -  Returns the notification details.
- `kaseya-vsa-cli notifications list`  -  Returns the device notifications.

**organization_custom_fields**  -  Manage organization custom fields

- `kaseya-vsa-cli organization-custom-fields <organizationId>`  -  Return the organization custom fields.

**organizations**  -  Manage organizations

- `kaseya-vsa-cli organizations create`  -  Creates a new Organization.
- `kaseya-vsa-cli organizations delete`  -  Deletes an Organization.
- `kaseya-vsa-cli organizations get`  -  Returns the organization details.
- `kaseya-vsa-cli organizations list`  -  Returns a list of organizations.
- `kaseya-vsa-cli organizations update`  -  Updates an Organization.

**patch_management**  -  Get patch management global rules and policy details

- `kaseya-vsa-cli patch-management get-global-rules`  -  Returns the global rules for patch management.
- `kaseya-vsa-cli patch-management get-policy`  -  Returns the details of the patch management policy.

**rate_limits**  -  Manage rate limits

- `kaseya-vsa-cli rate-limits`  -  Returns rate limits information.

**scope_usage**  -  Manage scope usage

- `kaseya-vsa-cli scope-usage <scopeId>`  -  Returns a list of places where the Scope is used.

**scopes**  -  Manage scopes

- `kaseya-vsa-cli scopes get`  -  Returns the scope details.
- `kaseya-vsa-cli scopes list`  -  Returns a list of Scopes.

**scripts**  -  Manage scripts

- `kaseya-vsa-cli scripts get`  -  Returns the automation script details.
- `kaseya-vsa-cli scripts get-execution`  -  Returns the details of a script execution. Note that execution details may not be available if the device is offline.
- `kaseya-vsa-cli scripts list`  -  Returns a list of automation scripts.
- `kaseya-vsa-cli scripts list-executions`  -  Returns script executions for the specified device. Note that executions may not be available if the device is offline.
- `kaseya-vsa-cli scripts run`  -  Runs a specific automation script.

**site_custom_fields**  -  Manage site custom fields

- `kaseya-vsa-cli site-custom-fields <siteId>`  -  Return the Site custom fields.

**sites**  -  Manage sites

- `kaseya-vsa-cli sites create`  -  Creates a new Site within an Organization.
- `kaseya-vsa-cli sites delete`  -  Deletes a Site.
- `kaseya-vsa-cli sites get`  -  Returns the Site details.
- `kaseya-vsa-cli sites list`  -  Returns a list of Sites.
- `kaseya-vsa-cli sites update`  -  Updates a Site.

**tasks**  -  Manage tasks

- `kaseya-vsa-cli tasks get`  -  Returns the task details.
- `kaseya-vsa-cli tasks get-execution`  -  Returns the status of a task execution.
- `kaseya-vsa-cli tasks get-execution-script-output`  -  Returns the output from a script executed as part of a task on a specific device.
- `kaseya-vsa-cli tasks list`  -  Returns a list of automation tasks.
- `kaseya-vsa-cli tasks list-execution-devices`  -  Returns the execution status for devices on which the task has been executed.
- `kaseya-vsa-cli tasks list-execution-scripts`  -  Returns a list of scripts that were executed as part of a task on a specific device.
- `kaseya-vsa-cli tasks run`  -  Runs a specific automation task.

**webhooks**  -  Manage webhooks

- `kaseya-vsa-cli webhooks create`  -  Creates a Notification Webhook.
- `kaseya-vsa-cli webhooks delete`  -  Deletes a Specific Notification Webhook.
- `kaseya-vsa-cli webhooks get`  -  Returns the notification webhook details.
- `kaseya-vsa-cli webhooks list`  -  Returns a list of Notification Webhooks.
- `kaseya-vsa-cli webhooks regenerate-secret-key`  -  Re-generates a Notification Webhook Secret Key.
- `kaseya-vsa-cli webhooks update`  -  Updates a Notification Webhook.

**workflows**  -  Manage workflows

- `kaseya-vsa-cli workflows cancel-executions`  -  Cancels specified workflow executions.
- `kaseya-vsa-cli workflows get`  -  Returns the workflow details.
- `kaseya-vsa-cli workflows get-execution`  -  Returns the details of a workflow execution.
- `kaseya-vsa-cli workflows list`  -  Returns a list of workflows.
- `kaseya-vsa-cli workflows list-executions`  -  Returns running workflow executions and the latest completed workflow executions.
- `kaseya-vsa-cli workflows run`  -  Runs a specific automation workflow.


**audit-logs**


- `kaseya-vsa-cli audit-logs digest`  -  From the synced local mirror (run sync first), see who did what in the tenant over a time window, with audit categories and event types decoded to names.


**devices**


- `kaseya-vsa-cli devices brief`  -  Get one device's online state, notification counts, security posture, disks, pending-update counts by severity, antivirus, policies, and recent audit activity from six live API calls.


**health**


- `kaseya-vsa-cli health`  -  From the synced local mirror (run sync first), list devices across every client that are offline, have antivirus off or stale, firewall or UAC off, low disk, or high CPU or memory; rows are fail or unknown, with scanned and per-check counts.


**patch-exposure**


- `kaseya-vsa-cli patch-exposure`  -  From the synced local mirror (run sync first), rank pending OS updates across all clients by KB, client, or device, filtered by severity, CVSS, KB, or CVE.


**scripts**


- `kaseya-vsa-cli scripts run-wait`  -  Run an automation script on a managed device (starts immediately, no confirmation) and get its exit code and output when it finishes.


**software**


- `kaseya-vsa-cli software`  -  From the synced local mirror (run sync first), find which devices run an application, its version spread, devices below a version, or devices missing it.


**tasks**


- `kaseya-vsa-cli tasks run-wait`  -  Run a saved task on chosen devices (starts immediately, no confirmation) or attach to a running execution, and collect per-device, per-script exit codes and output.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
kaseya-vsa-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Offline devices by client

```bash
kaseya-vsa-cli health --check offline --offline-for 7d --by org --agent
```

Counts devices that had not checked in for a week as of the last sync, per organization.

### Critical KBs across the fleet

```bash
kaseya-vsa-cli patch-exposure --by kb --severity critical --min-cvss 8 --agent
```

Ranks the critical updates missing on the most devices.

### Narrow a device list

```bash
kaseya-vsa-cli devices list --filter "contains(tolower(GroupName), 'acme')" --agent --select Identifier,Name,GroupName,OrganizationName
```

Uses an OData filter and keeps only the fields an agent needs.

### Run a script and wait

```bash
kaseya-vsa-cli scripts run-wait 11111111-2222-3333-4444-555555555555 --device FRONTDESK-01 --agent
```

Runs a script and returns its exit code and output.

## Auth Setup

VSA 10 uses HTTP Basic over HTTPS with an API token created under Administration > Configuration > API Access. Set KASEYA_VSA_TOKEN_ID and KASEYA_VSA_TOKEN_SECRET, and set KASEYA_VSA_SERVER to your server host (for example acme.vsax.net); there is no working default server. KASEYA_VSA_BASE_URL overrides the whole base URL (https://<server>/api/v3) if you need it. This is the VSA 10 (api/v3) API, not the VSA 9 /api/v1.0 API.

Run `kaseya-vsa-cli doctor` to verify setup.

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
  kaseya-vsa-cli assets list --agent --select Identifier,Name,AntivirusEnabled,AntivirusUpToDate,FirewallEnabled
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

- Use `--home <dir>` for one invocation, or set `KASEYA_VSA_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `KASEYA_VSA_CONFIG_DIR`, `KASEYA_VSA_DATA_DIR`, `KASEYA_VSA_STATE_DIR`, `KASEYA_VSA_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `KASEYA_VSA_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `kaseya-vsa-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "kaseya-vsa": {
        "command": "kaseya-vsa-mcp",
        "env": {
          "KASEYA_VSA_HOME": "/srv/kaseya-vsa"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `KASEYA_VSA_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `KASEYA_VSA_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; this connector keeps no on-disk response cache. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `KASEYA_VSA_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `KASEYA_VSA_DATA_SOURCE` sets the `--data-source` default.

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
kaseya-vsa-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "kaseya-vsa-cli learnings confirm 12"] }
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
       materially more, record the divergence via `kaseya-vsa-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `kaseya-vsa-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `kaseya-vsa-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
kaseya-vsa-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
kaseya-vsa-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
kaseya-vsa-cli teach-playbook \
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
kaseya-vsa-cli playbook amend \
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

`kaseya-vsa-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `KASEYA_VSA_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
kaseya-vsa-cli feedback "the --since flag is inclusive but docs say exclusive"
kaseya-vsa-cli feedback --stdin < notes.txt
kaseya-vsa-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `KASEYA_VSA_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `KASEYA_VSA_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
kaseya-vsa-cli profile save briefing --json
kaseya-vsa-cli --profile briefing assets list
kaseya-vsa-cli profile list --json
kaseya-vsa-cli profile show briefing
kaseya-vsa-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `kaseya-vsa-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add kaseya-vsa-mcp -- kaseya-vsa-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `kaseya-vsa-mcp --transport http` requires `KASEYA_VSA_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `KASEYA_VSA_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `KASEYA_VSA_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which kaseya-vsa-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   kaseya-vsa-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `kaseya-vsa-cli <command> --help`.
