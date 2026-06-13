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
      - kind: go
        bins: [tactical-rmm-cli]
        module: github.com/mvanhorn/printing-press-library/library/monitoring/tactical-rmm/cmd/tactical-rmm-cli
---

# Tactical RMM  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `tactical-rmm-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install tactical-rmm --cli-only
   ```
2. Verify: `tactical-rmm-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/tactical-rmm/cmd/tactical-rmm-cli@latest
```

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
  tactical-rmm-cli software find <name>
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
  tactical-rmm-cli maintenance set --filter site=HQ --until 4h
  ```
- **`actions pending`**  -  Queued agent actions across the fleet grouped by agent and age, so stuck dispatches surface. Live fan-out: requires TRMM_API_KEY (agent list comes from the local store).

  _Run after a bulk dispatch to see which agents haven't returned results._

  ```bash
  tactical-rmm-cli actions pending --agent
  ```

### Monitoring quality
- **`checks flapping`**  -  Checks that repeatedly flipped pass/fail within a window, from local snapshots.

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
- **`checks worst`**  -  Failing checks ranked by blast radius - how many agents are red on each check.

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


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
tactical-rmm-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Morning fleet triage

```bash
tactical-rmm-cli triage --limit 25
```

Ranked attention list across offline state, failing checks, reboots, patches.

### CVE exposure sweep

```bash
tactical-rmm-cli software find <name> --select agent_id,name,version
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

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  tactical-rmm-cli accounts accounts --agent --select id,name,status
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
tactical-rmm-cli feedback "the --since flag is inclusive but docs say exclusive"
tactical-rmm-cli feedback --stdin < notes.txt
tactical-rmm-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/tactical-rmm-cli/feedback.jsonl`. They are never POSTed unless `TACTICAL_RMM_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `TACTICAL_RMM_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `tactical-rmm-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/monitoring/tactical-rmm/cmd/tactical-rmm-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add tactical-rmm-mcp -- tactical-rmm-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which tactical-rmm-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   tactical-rmm-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `tactical-rmm-cli <command> --help`.
