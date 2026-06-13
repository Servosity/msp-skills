---
name: acronis
description: "The first real CLI for the Acronis Cyber Protect Cloud platform  -  every tenant, agent, and usage metric mirrored locally, with cross-tenant rollups no single API call returns. Trigger phrases: `acronis backup health across tenants`, `find offline acronis agents`, `acronis usage and billing report`, `which acronis customers aren't protected`, `acronis agent version compliance`, `use acronis cyber protect`, `run acronis-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Acronis"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - acronis-cli
---

# Acronis Cyber Protect Cloud  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `acronis-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install acronis --cli-only
   ```
2. Verify: `acronis-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails before this CLI has a public-library category, install Node or use the category-specific Go fallback after publish.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Manage your whole MSP estate from one Go binary: tenants, users, agents, offering items, usage, billing reports, tasks, and activities  -  all synced to a local SQLite store. Then answer questions the Acronis console can't: which customers' backups failed last night (health), which agents went silently offline (agents stale), and where you're billing for protection that isn't running (coverage --unprotected).

## When to Use This CLI

Use this CLI when you manage Acronis Cyber Protect Cloud as an MSP and need answers across many customer tenants at once  -  fleet backup health, offline-agent sweeps, agent-version compliance after a rollout, usage/billing reconciliation at month-end. It is also the right tool for scripting Acronis provisioning and reporting without re-implementing token exchange, pagination, and the datacenter-host dance in every script.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Cross-tenant rollups
- **`health`**  -  See backup success / failure / stale across your entire book of customer tenants in one table.

  _Reach for this when an agent or tech needs the one-screen 'which customers' backups failed last night' answer the Acronis console can't give across tenants._

  ```bash
  acronis-cli health --agent
  ```
- **`agents stale`**  -  List backup agents that haven't checked in within a threshold, across every tenant, sorted by customer.

  _Use this to catch silently-offline agents before the customer calls  -  the most common MSP backup-failure root cause._

  ```bash
  acronis-cli agents stale --older-than 7d --agent
  ```
- **`alerts repeat`**  -  Rank resources and tenants by how many distinct days in a window had a failed or missed backup.

  _Use this to separate one-off backup blips from chronically-failing resources that need real remediation._

  ```bash
  acronis-cli alerts repeat --days 14 --agent
  ```
- **`failures`**  -  Flat list of every failed or missed backup task across all tenants in a recent window, newest first.

  _Reach for this when the question is 'show me each backup that failed last night' as individual actionable rows, not rollup counts._

  ```bash
  acronis-cli failures --since 24h --agent
  ```
- **`freshness`**  -  Time since the last successful backup per tenant, flagged against an SLA threshold  -  including tenants never backed up.

  _Reach for this when an agent needs SLA breach detection: which customers have gone too long without a good backup._

  ```bash
  acronis-cli freshness --sla 48h --breached --agent
  ```
- **`customer`**  -  One cross-resource snapshot of a single customer: tenant record, users, licenses, usage, agents, and 7-day backup outcomes joined.

  _Reach for this before a customer call: the full per-customer picture in one command instead of six console drill-downs._

  ```bash
  acronis-cli customer TENANT_ID --agent
  ```

### Billing & licensing
- **`reconcile usages`**  -  Flag usage with no matching offering item and offering items with zero usage, per tenant.

  _Reach for this at month-end to catch under-billing (usage with no SKU) and waste (paid SKUs with zero usage) before invoices go out._

  ```bash
  acronis-cli reconcile usages --tenant <tenant_id> --agent
  ```
- **`coverage`**  -  Surface tenants that pay for protection but have no online agent or no recent successful backup.

  _Use this to find the highest-liability customers  -  billed for backup, not actually protected._

  ```bash
  acronis-cli coverage --unprotected --agent
  ```
- **`usages drift`**  -  Compare per-tenant, per-metric usage between two stored snapshots to see what grew or shrank.

  _Reach for this to explain month-over-month invoice changes and spot runaway storage growth early._

  ```bash
  acronis-cli usages drift --from 2026-04-01 --to 2026-05-01 --agent
  ```
- **`tenants offering-items inventory`**  -  Estate-wide rollup of which offering items and editions are enabled, with per-SKU tenant counts.

  _Reach for this for license trueups and edition migrations: which SKUs are deployed where, in one table._

  ```bash
  acronis-cli tenants offering-items inventory --agent
  ```

### Fleet posture
- **`agents compliance`**  -  Show the distribution of agent versions across the estate and flag tenants behind the target version.

  _Use this after a release rollout to confirm every customer's agents updated, for security and support consistency._

  ```bash
  acronis-cli agents compliance --target 16.0 --agent
  ```
- **`tree`**  -  Render the Partner -> Customer -> Folder -> Unit hierarchy with per-node agent and user counts.

  _Reach for this to understand the shape of a partner's book of business at a glance._

  ```bash
  acronis-cli tree --depth 3
  ```
- **`tenants audit`**  -  Flag enabled customer tenants missing users, offering items, agents, or OAuth clients  -  onboarding drift in one table.

  _Reach for this after onboarding waves: catches half-provisioned tenants before they become missed-backup tickets._

  ```bash
  acronis-cli tenants audit --agent
  ```

## Command Reference

**agent-manager**  -  Manage agent manager

- `acronis-cli agent-manager delete-agent`  -  Cancel registration of a specific Acronis agent.
- `acronis-cli agent-manager delete-agents`  -  Cancel registration and delete service accounts for multiple agents.
- `acronis-cli agent-manager force-agent-update`  -  Launch a forced agent update bypassing maintenance windows for specified agents.
- `acronis-cli agent-manager get-agent`  -  Retrieve details about a specific registered Acronis agent.
- `acronis-cli agent-manager get-agent-update-settings`  -  Fetch update configuration settings for agents or tenants.
- `acronis-cli agent-manager get-hardware-node`  -  Retrieve specific hardware node information including storage configuration.
- `acronis-cli agent-manager list-agents`  -  List all registered Acronis protection agents visible from a specified tenant.
- `acronis-cli agent-manager list-hardware-nodes`  -  List all hardware nodes visible from a specified tenant.
- `acronis-cli agent-manager update-agent-update-settings`  -  Store or update agent update configuration settings including maintenance windows.

**clients**  -  OAuth2 client credential management

- `acronis-cli clients create`  -  Create a new OAuth2 client credential for API authentication.
- `acronis-cli clients delete`  -  Delete an OAuth2 client credential.
- `acronis-cli clients get`  -  Retrieve details about a specific OAuth2 client.
- `acronis-cli clients list`  -  List OAuth2 client credentials registered in the system.

**idp**  -  Manage idp

- `acronis-cli idp request-token`  -  Request an OAuth2 access token using client credentials, authorization code, or other grant types.
- `acronis-cli idp revoke-token`  -  Revoke an OAuth2 access or refresh token.

**remote_search**  -  Manage remote search

- `acronis-cli remote-search`  -  Search for tenants and users by name, email, or login across the accessible hierarchy.

**reports**  -  Manage reports

- `acronis-cli reports`  -  Create a scheduled or on-demand usage report configuration.

**task-manager**  -  Manage task manager

- `acronis-cli task-manager get-activity`  -  Retrieve details about a specific task activity by ID.
- `acronis-cli task-manager get-task`  -  Retrieve details about a specific backup or protection task by ID.
- `acronis-cli task-manager list-activities`  -  Fetch a list of task activities with filtering and pagination.
- `acronis-cli task-manager list-tasks`  -  Fetch a list of backup and protection tasks with filtering, ordering, and pagination support.

**tenants**  -  Tenant hierarchy management and configuration

- `acronis-cli tenants create`  -  Create a new tenant as a child of an existing tenant.
- `acronis-cli tenants delete`  -  Delete a tenant. The tenant must have no children or active services.
- `acronis-cli tenants get`  -  Retrieve details about a specific tenant by ID.
- `acronis-cli tenants list`  -  List tenants in the hierarchy. Can filter by parent tenant UUID or retrieve by specific UUIDs.
- `acronis-cli tenants update`  -  Update tenant properties including name, contact, and enabled status.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
acronis-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Monday morning fleet triage

```bash
acronis-cli sync && acronis-cli health --agent --select tenant_id,failed,stale
```

Sync the estate then narrow the health rollup to just the failing and stale columns per tenant.

### Offline-agent sweep into a ticket

```bash
acronis-cli agents stale --older-than 3d --json --select tenant_id,hostname,last_seen | jq '.[]'
```

List agents silent for 3+ days with only the fields a ticket needs.

### Month-end billing reconciliation

```bash
acronis-cli reconcile usages --tenant <tenant_id> --agent
```

Flag usage with no matching SKU and SKUs with zero usage before invoicing.

### Post-rollout version audit

```bash
acronis-cli agents compliance --target 16.0 --json
```

Confirm every tenant's agents reached the target version after a release.

### Estate shape at a glance

```bash
acronis-cli tree --depth 3
```

Render the partner/customer/folder/unit hierarchy with per-node agent and user counts.

## Auth Setup

Acronis uses OAuth2 client credentials. Register an API client in the Acronis console to get a client_id, client_secret, and your datacenter region, then run `acronis-cli auth login`  -  it exchanges them at /api/2/idp/token for a JWT (valid 2 hours) and stores it. Tokens last 2 hours; re-run `auth login` to refresh. The datacenter host is per-partner; set it with --datacenter or ACRONIS_DATACENTER (e.g. us-cloud, eu2-cloud). You can also skip login and provide a JWT directly via ACRONIS_CYBER_PROTECT_BEARER_AUTH or `auth set-token`.

Run `acronis-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  acronis-cli clients list --agent --select id,name,status
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
acronis-cli feedback "the --since flag is inclusive but docs say exclusive"
acronis-cli feedback --stdin < notes.txt
acronis-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/acronis-cli/feedback.jsonl`. They are never POSTed unless `ACRONIS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `ACRONIS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
acronis-cli profile save briefing --json
acronis-cli --profile briefing clients list
acronis-cli profile list --json
acronis-cli profile show briefing
acronis-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Async Jobs

For endpoints that submit long-running work, the generator detects the submit-then-poll pattern (a `job_id`/`task_id`/`operation_id` field in the response plus a sibling status endpoint) and wires up three extra flags on the submitting command:

| Flag | Purpose |
|------|---------|
| `--wait` | Block until the job reaches a terminal status instead of returning the job ID immediately |
| `--wait-timeout` | Maximum wait duration (default 10m, 0 means no timeout) |
| `--wait-interval` | Initial poll interval (default 2s; grows with exponential backoff up to 30s) |

Use async submission without `--wait` when you want to fire-and-forget; use `--wait` when you want one command to return the finished artifact.

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

1. **Empty, `help`, or `--help`** → show `acronis-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

Install the MCP binary from this CLI's published public-library entry or pre-built release, then register it:

```bash
claude mcp add acronis-mcp -- acronis-mcp
```

Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which acronis-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   acronis-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `acronis-cli <command> --help`.
