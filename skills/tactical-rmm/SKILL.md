---
name: tactical-rmm
description: "Every Tactical RMM endpoint as a typed command, plus an offline SQLite mirror and cross-entity fleet queries the web UI can't express. Trigger phrases: `tactical rmm fleet health`, `which agents are offline`, `triage my rmm fleet`, `patch posture by client`, `run a script on all windows agents`, `use tactical-rmm`, `run tactical-rmm`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Tactical RMM"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - tactical-rmm-cli
    install:
      - kind: script
        bins: [tactical-rmm-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/tactical-rmm/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/tactical-rmm/install.ps1
---

# Tactical RMM - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `tactical-rmm-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/tactical-rmm/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/tactical-rmm/install.ps1 | iex
   ```
3. Verify: `tactical-rmm-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `tactical-rmm-cli` and `tactical-rmm-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

A terminal-native, agent-native control plane for self-hosted Tactical RMM. Mirror your fleet into local SQLite, run cross-entity queries (fleet health, triage, patch posture, coverage) that no single API call returns, fan scripts out across a filtered cohort, and pipe clean JSON anywhere.

## When to Use This CLI

Operating a self-hosted Tactical RMM fleet from the terminal or an agent: triaging endpoints, sweeping software/patch posture, fanning a script to a cohort, or pulling clean JSON for automation.

## Anti-triggers

Do not use this CLI for:
- Ticketing/PSA work (invoices, tickets, time entries) - Tactical RMM is an RMM; use your PSA's tooling instead
- Cloud/hosted RMMs (NinjaOne, Datto RMM, N-central) - this CLI only speaks self-hosted Tactical RMM's API
- Installing or enrolling new agents on endpoints - use the TRMM installer/deployment flow, not this CLI
- Editing automation policies or check definitions in bulk - policy authoring stays in the web UI; this CLI reads and reports on them

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Cross-entity fleet intelligence
- **`fleet health`**  -  Whole-fleet posture in one command: online/offline/overdue agents, failing checks, pending reboots, outstanding patches, active alerts.

  _Size up an environment before drilling in._

  ```bash
  tactical-rmm-cli fleet health
  ```
- **`triage`**  -  Ranked list of agents needing attention, scored across offline state, failing checks, reboots and patches.

  _Decide what to fix first across hundreds of endpoints._

  ```bash
  tactical-rmm-cli triage --limit 20
  ```
- **`agents stale`**  -  Agents whose last check-in exceeds a threshold, with client and site.

  _Find abandoned endpoints before they become a gap._

  ```bash
  tactical-rmm-cli agents stale --days 7
  ```
- **`since`**  -  What changed across the fleet since a window: new alerts, newly-offline agents.

  _Start a shift seeing only what moved._

  ```bash
  tactical-rmm-cli since 2h
  ```
- **`clients scorecard`**  -  One posture row per client: agent count, online share, failing checks, pending patches, open alerts.

  _Use for per-customer health rollups and QBR prep instead of clicking through every client._

  ```bash
  tactical-rmm-cli clients scorecard --agent
  ```

### Patch & inventory
- **`patch posture`**  -  Per-client/site rollup of pending Windows updates and reboots.

  _Answer who is behind on patches in one call._

  ```bash
  tactical-rmm-cli patch posture --by client
  ```
- **`software find`**  -  Which agents have a given software package installed across the fleet.

  _Answer who is exposed during a CVE scramble._

  ```bash
  tactical-rmm-cli software find openssl
  ```
- **`services down`**  -  Agents where a named Windows service is stopped, across the whole fleet. Live fan-out: requires TRMM_API_KEY (agent list comes from the local store).

  _Use during incidents to find every endpoint with a critical service stopped._

  ```bash
  tactical-rmm-cli services down --name Spooler --agent
  ```

### Remote execution
- **`agents bulk-run`**  -  Run a script/command across every agent matching a local-store filter (client/site/os/online).

  _Push a fix or audit to a whole cohort at once._

  ```bash
  tactical-rmm-cli agents bulk-run --command whoami --filter os=windows,online=true
  ```
- **`maintenance set`**  -  Put a filtered cohort of agents into maintenance mode in one command, with a reminder window (re-run with --clear to end it; Tactical RMM has no server-side expiry).

  _Reach for this when silencing monitoring for a planned window across many agents at once; previews by default, mutates only with --execute and a live TRMM_API_KEY._

  ```bash
  tactical-rmm-cli maintenance set --filter client=Acme,site=HQ --until 4h
  ```
- **`actions pending`**  -  Queued agent actions across the fleet grouped by agent and age, so stuck dispatches surface. Live fan-out: requires TRMM_API_KEY (agent list comes from the local store).

  _Run after a bulk dispatch to see which agents haven't returned results._

  ```bash
  tactical-rmm-cli actions pending --agent
  ```

### Monitoring quality
- **`checks flapping`**  -  Checks that repeatedly flipped pass/fail within a window, from local snapshots (`--snapshot` reads each agent's checks live).

  _Catch noisy monitors that erode alert trust._

  ```bash
  tactical-rmm-cli checks flapping --window 24h
  ```
- **`alerts digest`**  -  Grouped alert summary by client/severity/type over a time window.

  _Turn an alert firehose into a shift-handoff summary._

  ```bash
  tactical-rmm-cli alerts digest --since 24h
  ```
- **`coverage`**  -  Agents missing checks (unmonitored endpoints).

  _Find unmonitored endpoints before they fail silently._

  ```bash
  tactical-rmm-cli coverage
  ```
- **`checks worst`**  -  Failing checks ranked by blast radius - how many agents are red on each check. Reads each failing agent's checks live, so it needs `TRMM_API_KEY`; the agent list comes from the local store.

  _Use this to find the single failure hurting the most endpoints before triaging agent-by-agent._

  ```bash
  tactical-rmm-cli checks worst --limit 10 --agent
  ```

## Command Reference

**accounts**  -  Manage accounts

- `tactical-rmm-cli accounts accounts`  -  GET /accounts/apikeys/
- `tactical-rmm-cli accounts accounts-apikeys`  -  POST /accounts/apikeys/
- `tactical-rmm-cli accounts accounts-apikeys-2`  -  DELETE /accounts/apikeys/{pk}/
- `tactical-rmm-cli accounts accounts-apikeys-3`  -  PUT /accounts/apikeys/{pk}/
- `tactical-rmm-cli accounts accounts-reset2fa`  -  PUT /accounts/reset2fa/
- `tactical-rmm-cli accounts accounts-resetpw`  -  PUT /accounts/resetpw/
- `tactical-rmm-cli accounts accounts-roles`  -  GET /accounts/roles/
- `tactical-rmm-cli accounts accounts-roles-2`  -  POST /accounts/roles/
- `tactical-rmm-cli accounts accounts-roles-3`  -  DELETE /accounts/roles/{pk}/
- `tactical-rmm-cli accounts accounts-roles-4`  -  GET /accounts/roles/{pk}/
- `tactical-rmm-cli accounts accounts-roles-5`  -  PUT /accounts/roles/{pk}/
- `tactical-rmm-cli accounts accounts-sessions`  -  DELETE /accounts/sessions/{pk}/
- `tactical-rmm-cli accounts accounts-users`  -  GET /accounts/users/
- `tactical-rmm-cli accounts accounts-users-10`  -  GET /accounts/users/{pk}/sessions/
- `tactical-rmm-cli accounts accounts-users-2`  -  POST /accounts/users/
- `tactical-rmm-cli accounts accounts-users-3`  -  POST /accounts/users/reset/
- `tactical-rmm-cli accounts accounts-users-4`  -  PUT /accounts/users/reset/
- `tactical-rmm-cli accounts accounts-users-5`  -  POST /accounts/users/reset_totp/
- `tactical-rmm-cli accounts accounts-users-6`  -  PUT /accounts/users/reset_totp/
- `tactical-rmm-cli accounts accounts-users-7`  -  POST /accounts/users/setup_totp/
- `tactical-rmm-cli accounts accounts-users-8`  -  PATCH /accounts/users/ui/
- `tactical-rmm-cli accounts accounts-users-9`  -  DELETE /accounts/users/{pk}/sessions/

**agents**  -  Manage agents

- `tactical-rmm-cli agents agents`  -  GET /agents/
- `tactical-rmm-cli agents agents-actions`  -  GET /agents/actions/bulk/
- `tactical-rmm-cli agents agents-agentid`  -  DELETE /agents/{agent_id}/
- `tactical-rmm-cli agents agents-agentid-2`  -  GET /agents/{agent_id}/
- `tactical-rmm-cli agents agents-agentid-3`  -  PUT /agents/{agent_id}/
- `tactical-rmm-cli agents agents-bulkrecovery`  -  GET /agents/bulkrecovery/
- `tactical-rmm-cli agents agents-history`  -  GET /agents/history/
- `tactical-rmm-cli agents agents-installer`  -  GET /agents/installer/
- `tactical-rmm-cli agents agents-maintenance`  -  GET /agents/maintenance/bulk/
- `tactical-rmm-cli agents agents-notes`  -  GET /agents/notes/
- `tactical-rmm-cli agents agents-notes-2`  -  POST /agents/notes/
- `tactical-rmm-cli agents agents-notes-3`  -  DELETE /agents/notes/{pk}/
- `tactical-rmm-cli agents agents-notes-4`  -  GET /agents/notes/{pk}/
- `tactical-rmm-cli agents agents-notes-5`  -  PUT /agents/notes/{pk}/
- `tactical-rmm-cli agents agents-scripthistory`  -  GET /agents/scripthistory/
- `tactical-rmm-cli agents agents-update`  -  GET /agents/update/
- `tactical-rmm-cli agents agents-versions`  -  GET /agents/versions/

**alerts**  -  Manage alerts

- `tactical-rmm-cli alerts alerts`  -  PATCH /alerts/
- `tactical-rmm-cli alerts alerts-bulk`  -  POST /alerts/bulk/
- `tactical-rmm-cli alerts alerts-endpoint`  -  POST /alerts/
- `tactical-rmm-cli alerts alerts-pk`  -  DELETE /alerts/{pk}/
- `tactical-rmm-cli alerts alerts-pk-2`  -  GET /alerts/{pk}/
- `tactical-rmm-cli alerts alerts-pk-3`  -  PUT /alerts/{pk}/
- `tactical-rmm-cli alerts alerts-templates`  -  GET /alerts/templates/
- `tactical-rmm-cli alerts alerts-templates-2`  -  POST /alerts/templates/
- `tactical-rmm-cli alerts alerts-templates-3`  -  DELETE /alerts/templates/{pk}/
- `tactical-rmm-cli alerts alerts-templates-4`  -  GET /alerts/templates/{pk}/
- `tactical-rmm-cli alerts alerts-templates-5`  -  PUT /alerts/templates/{pk}/
- `tactical-rmm-cli alerts alerts-templates-6`  -  GET /alerts/templates/{pk}/related/

**automation**  -  Manage automation

- `tactical-rmm-cli automation automation`  -  DELETE /automation/patchpolicy/
- `tactical-rmm-cli automation automation-checks`  -  GET /automation/checks/{check}/status/
- `tactical-rmm-cli automation automation-patchpolicy`  -  POST /automation/patchpolicy/
- `tactical-rmm-cli automation automation-patchpolicy-2`  -  PUT /automation/patchpolicy/
- `tactical-rmm-cli automation automation-patchpolicy-3`  -  POST /automation/patchpolicy/reset/
- `tactical-rmm-cli automation automation-patchpolicy-4`  -  DELETE /automation/patchpolicy/{pk}/
- `tactical-rmm-cli automation automation-patchpolicy-5`  -  POST /automation/patchpolicy/{pk}/
- `tactical-rmm-cli automation automation-patchpolicy-6`  -  PUT /automation/patchpolicy/{pk}/
- `tactical-rmm-cli automation automation-policies`  -  GET /automation/policies/
- `tactical-rmm-cli automation automation-policies-2`  -  POST /automation/policies/
- `tactical-rmm-cli automation automation-policies-3`  -  GET /automation/policies/overview/
- `tactical-rmm-cli automation automation-policies-4`  -  DELETE /automation/policies/{pk}/
- `tactical-rmm-cli automation automation-policies-5`  -  GET /automation/policies/{pk}/
- `tactical-rmm-cli automation automation-policies-6`  -  PUT /automation/policies/{pk}/
- `tactical-rmm-cli automation automation-policies-7`  -  GET /automation/policies/{pk}/related/
- `tactical-rmm-cli automation automation-policies-8`  -  GET /automation/policies/{policy}/checks/
- `tactical-rmm-cli automation automation-policies-9`  -  GET /automation/policies/{policy}/tasks/
- `tactical-rmm-cli automation automation-tasks`  -  GET /automation/tasks/{task}/run/
- `tactical-rmm-cli automation automation-tasks-2`  -  POST /automation/tasks/{task}/run/
- `tactical-rmm-cli automation automation-tasks-3`  -  GET /automation/tasks/{task}/status/
- `tactical-rmm-cli automation automation-tasks-4`  -  POST /automation/tasks/{task}/status/

**autotasks**  -  Manage autotasks

- `tactical-rmm-cli autotasks autotasks`  -  GET /autotasks/
- `tactical-rmm-cli autotasks autotasks-endpoint`  -  POST /autotasks/
- `tactical-rmm-cli autotasks autotasks-pk`  -  DELETE /autotasks/{pk}/
- `tactical-rmm-cli autotasks autotasks-pk-2`  -  GET /autotasks/{pk}/
- `tactical-rmm-cli autotasks autotasks-pk-3`  -  PUT /autotasks/{pk}/

**checks**  -  Manage checks

- `tactical-rmm-cli checks checks`  -  GET /checks/
- `tactical-rmm-cli checks checks-endpoint`  -  POST /checks/
- `tactical-rmm-cli checks checks-pk`  -  DELETE /checks/{pk}/
- `tactical-rmm-cli checks checks-pk-2`  -  GET /checks/{pk}/
- `tactical-rmm-cli checks checks-pk-3`  -  PUT /checks/{pk}/

**clients**  -  Manage clients

- `tactical-rmm-cli clients clients`  -  GET /clients/
- `tactical-rmm-cli clients clients-deployments`  -  DELETE /clients/deployments/
- `tactical-rmm-cli clients clients-deployments-2`  -  GET /clients/deployments/
- `tactical-rmm-cli clients clients-deployments-3`  -  POST /clients/deployments/
- `tactical-rmm-cli clients clients-deployments-4`  -  DELETE /clients/deployments/{pk}/
- `tactical-rmm-cli clients clients-deployments-5`  -  GET /clients/deployments/{pk}/
- `tactical-rmm-cli clients clients-deployments-6`  -  POST /clients/deployments/{pk}/
- `tactical-rmm-cli clients clients-endpoint`  -  POST /clients/
- `tactical-rmm-cli clients clients-pk`  -  DELETE /clients/{pk}/
- `tactical-rmm-cli clients clients-pk-2`  -  GET /clients/{pk}/
- `tactical-rmm-cli clients clients-pk-3`  -  PUT /clients/{pk}/
- `tactical-rmm-cli clients clients-sites`  -  GET /clients/sites/
- `tactical-rmm-cli clients clients-sites-2`  -  POST /clients/sites/
- `tactical-rmm-cli clients clients-sites-3`  -  DELETE /clients/sites/{pk}/
- `tactical-rmm-cli clients clients-sites-4`  -  GET /clients/sites/{pk}/
- `tactical-rmm-cli clients clients-sites-5`  -  PUT /clients/sites/{pk}/

**core**  -  Manage core

- `tactical-rmm-cli core core`  -  GET /core/clearcache/
- `tactical-rmm-cli core core-codesign`  -  DELETE /core/codesign/
- `tactical-rmm-cli core core-codesign-2`  -  GET /core/codesign/
- `tactical-rmm-cli core core-codesign-3`  -  PATCH /core/codesign/
- `tactical-rmm-cli core core-codesign-4`  -  POST /core/codesign/
- `tactical-rmm-cli core core-customfields`  -  GET /core/customfields/
- `tactical-rmm-cli core core-customfields-2`  -  PATCH /core/customfields/
- `tactical-rmm-cli core core-customfields-3`  -  POST /core/customfields/
- `tactical-rmm-cli core core-customfields-4`  -  DELETE /core/customfields/{pk}/
- `tactical-rmm-cli core core-customfields-5`  -  GET /core/customfields/{pk}/
- `tactical-rmm-cli core core-customfields-6`  -  PUT /core/customfields/{pk}/
- `tactical-rmm-cli core core-dashinfo`  -  GET /core/dashinfo/
- `tactical-rmm-cli core core-emailtest`  -  GET /core/emailtest/
- `tactical-rmm-cli core core-keystore`  -  GET /core/keystore/
- `tactical-rmm-cli core core-keystore-2`  -  POST /core/keystore/
- `tactical-rmm-cli core core-keystore-3`  -  DELETE /core/keystore/{pk}/
- `tactical-rmm-cli core core-keystore-4`  -  PUT /core/keystore/{pk}/
- `tactical-rmm-cli core core-openai`  -  POST /core/openai/generate/
- `tactical-rmm-cli core core-schedules`  -  GET /core/schedules/
- `tactical-rmm-cli core core-schedules-2`  -  POST /core/schedules/
- `tactical-rmm-cli core core-schedules-3`  -  DELETE /core/schedules/{pk}/
- `tactical-rmm-cli core core-schedules-4`  -  PUT /core/schedules/{pk}/
- `tactical-rmm-cli core core-servermaintenance`  -  GET /core/servermaintenance/
- `tactical-rmm-cli core core-settings`  -  GET /core/settings/
- `tactical-rmm-cli core core-settings-2`  -  PUT /core/settings/
- `tactical-rmm-cli core core-smstest`  -  POST /core/smstest/
- `tactical-rmm-cli core core-status`  -  GET /core/status/
- `tactical-rmm-cli core core-urlaction`  -  GET /core/urlaction/
- `tactical-rmm-cli core core-urlaction-2`  -  POST /core/urlaction/
- `tactical-rmm-cli core core-urlaction-3`  -  PATCH /core/urlaction/run/
- `tactical-rmm-cli core core-urlaction-4`  -  DELETE /core/urlaction/{pk}/
- `tactical-rmm-cli core core-urlaction-5`  -  PUT /core/urlaction/{pk}/
- `tactical-rmm-cli core core-urlaction-6`  -  POST /core/urlaction/run/test/
- `tactical-rmm-cli core core-v2`  -  GET /core/v2/status/
- `tactical-rmm-cli core core-version`  -  GET /core/version/
- `tactical-rmm-cli core core-webtermperms`  -  GET /core/webtermperms/

**logs**  -  Manage logs

- `tactical-rmm-cli logs logs`  -  PATCH /logs/audit/
- `tactical-rmm-cli logs logs-debug`  -  PATCH /logs/debug/
- `tactical-rmm-cli logs logs-pendingactions`  -  DELETE /logs/pendingactions/
- `tactical-rmm-cli logs logs-pendingactions-2`  -  GET /logs/pendingactions/
- `tactical-rmm-cli logs logs-pendingactions-3`  -  DELETE /logs/pendingactions/{pk}/
- `tactical-rmm-cli logs logs-pendingactions-4`  -  GET /logs/pendingactions/{pk}/

**scripts**  -  Manage scripts

- `tactical-rmm-cli scripts scripts`  -  GET /scripts/
- `tactical-rmm-cli scripts scripts-endpoint`  -  POST /scripts/
- `tactical-rmm-cli scripts scripts-pk`  -  DELETE /scripts/{pk}/
- `tactical-rmm-cli scripts scripts-pk-2`  -  GET /scripts/{pk}/
- `tactical-rmm-cli scripts scripts-pk-3`  -  PUT /scripts/{pk}/
- `tactical-rmm-cli scripts scripts-snippets`  -  GET /scripts/snippets/
- `tactical-rmm-cli scripts scripts-snippets-2`  -  POST /scripts/snippets/
- `tactical-rmm-cli scripts scripts-snippets-3`  -  DELETE /scripts/snippets/{pk}/
- `tactical-rmm-cli scripts scripts-snippets-4`  -  GET /scripts/snippets/{pk}/
- `tactical-rmm-cli scripts scripts-snippets-5`  -  PUT /scripts/snippets/{pk}/

**services**  -  Manage services

- `tactical-rmm-cli services services`  -  GET /services/{agent_id}/
- `tactical-rmm-cli services services-agentid`  -  GET /services/{agent_id}/{svcname}/
- `tactical-rmm-cli services services-agentid-2`  -  POST /services/{agent_id}/{svcname}/
- `tactical-rmm-cli services services-agentid-3`  -  PUT /services/{agent_id}/{svcname}/

**software**  -  Manage software

- `tactical-rmm-cli software software`  -  GET /software/
- `tactical-rmm-cli software software-agentid`  -  GET /software/{agent_id}/
- `tactical-rmm-cli software software-agentid-2`  -  POST /software/{agent_id}/
- `tactical-rmm-cli software software-agentid-3`  -  PUT /software/{agent_id}/
- `tactical-rmm-cli software software-chocos`  -  GET /software/chocos/
- `tactical-rmm-cli software software-endpoint`  -  POST /software/
- `tactical-rmm-cli software software-endpoint-2`  -  PUT /software/

**winupdate**  -  Manage winupdate

- `tactical-rmm-cli winupdate winupdate`  -  GET /winupdate/{agent_id}/
- `tactical-rmm-cli winupdate winupdate-pk`  -  PUT /winupdate/{pk}/


**actions**


- `tactical-rmm-cli actions pending`  -  Queued agent actions across the fleet grouped by agent and age, so stuck dispatches surface. Live fan-out: requires TRMM_API_KEY (agent list comes from the local store).


**agents**


- `tactical-rmm-cli agents stale`  -  Agents whose last check-in exceeds a threshold, with client and site.

- `tactical-rmm-cli agents bulk-run`  -  Run a script/command across every agent matching a local-store filter (client/site/os/online).


**alerts**


- `tactical-rmm-cli alerts digest`  -  Grouped alert summary by client/severity/type over a time window.


**checks**


- `tactical-rmm-cli checks flapping`  -  Checks that repeatedly flipped pass/fail within a window, from local snapshots (`--snapshot` reads each agent's checks live).

- `tactical-rmm-cli checks worst`  -  Failing checks ranked by blast radius - how many agents are red on each check. Reads each failing agent's checks live, so it needs `TRMM_API_KEY`; the agent list comes from the local store.


**clients**


- `tactical-rmm-cli clients scorecard`  -  One posture row per client: agent count, online share, failing checks, pending patches, open alerts.


**coverage**


- `tactical-rmm-cli coverage`  -  Agents missing checks (unmonitored endpoints).


**fleet**


- `tactical-rmm-cli fleet health`  -  Whole-fleet posture in one command: online/offline/overdue agents, failing checks, pending reboots, outstanding patches, active alerts.


**maintenance**


- `tactical-rmm-cli maintenance set`  -  Put a filtered cohort of agents into maintenance mode in one command, with a reminder window (re-run with --clear to end it; Tactical RMM has no server-side expiry).


**patch**


- `tactical-rmm-cli patch posture`  -  Per-client/site rollup of pending Windows updates and reboots.


**services**


- `tactical-rmm-cli services down`  -  Agents where a named Windows service is stopped, across the whole fleet. Live fan-out: requires TRMM_API_KEY (agent list comes from the local store).


**since**


- `tactical-rmm-cli since`  -  What changed across the fleet since a window: new alerts, newly-offline agents.


**software**


- `tactical-rmm-cli software find`  -  Which agents have a given software package installed across the fleet.


**triage**


- `tactical-rmm-cli triage`  -  Ranked list of agents needing attention, scored across offline state, failing checks, reboots and patches.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
tactical-rmm-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning fleet triage

```bash
tactical-rmm-cli triage --limit 25
```

Ranked attention list across offline state, failing checks, reboots, patches.

### CVE exposure sweep

```bash
tactical-rmm-cli software find openssl --select agent_id,name,version
```

Which agents carry a package, narrowed to the fields that matter.

### Patch posture by client

```bash
tactical-rmm-cli patch posture --by client
```

Roll pending Windows updates up to each customer.

### Shift handoff: what changed

```bash
tactical-rmm-cli since 8h
```

New alerts and newly-offline agents since your last shift.

## Auth Setup

Authenticate with a Tactical RMM API key in the X-API-KEY header (Settings > Global Settings > API Keys; bypasses 2FA). Set TRMM_API_KEY and point the CLI at your self-hosted API host (e.g. https://api.your-domain.com). No tenant is baked in.

Run `tactical-rmm-cli doctor` to verify setup.

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
  tactical-rmm-cli accounts accounts --agent
  ```
- **Previewable**  -  `--dry-run` shows the request without sending. A completed dry run exits 0 with `"success": true` and `"dry_run": true` and no HTTP `status`; validation failures still exit non-zero with `"success": false`.
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

- Use `--home <dir>` for one invocation, or set `TACTICAL_RMM_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `TACTICAL_RMM_CONFIG_DIR`, `TACTICAL_RMM_DATA_DIR`, `TACTICAL_RMM_STATE_DIR`, `TACTICAL_RMM_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `TACTICAL_RMM_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `tactical-rmm-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "tactical-rmm": {
        "command": "tactical-rmm-mcp",
        "env": {
          "TACTICAL_RMM_HOME": "/srv/tactical-rmm"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `TACTICAL_RMM_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `TACTICAL_RMM_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `TACTICAL_RMM_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `TACTICAL_RMM_DATA_SOURCE` sets the `--data-source` default.

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
tactical-rmm-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "tactical-rmm-cli learnings confirm 12"] }
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
       materially more, record the divergence via `tactical-rmm-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `tactical-rmm-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `tactical-rmm-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
tactical-rmm-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
tactical-rmm-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
tactical-rmm-cli teach-playbook \
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
tactical-rmm-cli playbook amend \
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

`tactical-rmm-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `TACTICAL_RMM_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
tactical-rmm-cli feedback "the --since flag is inclusive but docs say exclusive"
tactical-rmm-cli feedback --stdin < notes.txt
tactical-rmm-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `TACTICAL_RMM_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `TACTICAL_RMM_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
tactical-rmm-cli profile save briefing --json
tactical-rmm-cli --profile briefing accounts accounts
tactical-rmm-cli profile list --json
tactical-rmm-cli profile show briefing
tactical-rmm-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `tactical-rmm-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add tactical-rmm-mcp -- tactical-rmm-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `tactical-rmm-mcp --transport http` requires `TACTICAL_RMM_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `TACTICAL_RMM_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `TACTICAL_RMM_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which tactical-rmm-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   tactical-rmm-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `tactical-rmm-cli <command> --help`.
