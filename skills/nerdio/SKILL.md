---
name: nerdio
description: "The first non-PowerShell client for the Nerdio Manager for MSP API - cross-account AVD fleet audits, async-job plumbing, and offline search no other Nerdio tool has. Trigger phrases: `list nerdio accounts`, `audit autoscale across customers`, `which AVD hosts are running`, `nerdio billing rollup`, `wait for nerdio job`, `use nerdio`, `run nerdio-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Nerdio Manager"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - nerdio-cli
    install:
      - kind: go
        bins: [nerdio-cli]
        module: github.com/mvanhorn/printing-press-library/library/cloud/nerdio/cmd/nerdio-cli
---

# Nerdio Manager  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `nerdio-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install nerdio --cli-only
   ```
2. Verify: `nerdio-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/cloud/nerdio/cmd/nerdio-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Manage Azure Virtual Desktop fleets across every customer account from one terminal: audit autoscale posture fleet-wide with `fleet autoscale-audit`, sweep session-host power state with `fleet host-estate`, reconcile billing with `fleet billing-rollup`, and tame NMM's async job model with `job wait`. Includes a local SQLite store with full-text search over accounts, profiles, and scripted actions - the only offline Nerdio client in existence.

## When to Use This CLI

Reach for this CLI when operating Azure Virtual Desktop estates through Nerdio Manager for MSP: listing customer accounts, auditing or changing host-pool autoscale, power-cycling session hosts, exporting invoices and usage for PSA reconciliation, executing scripted actions across accounts, and polling NMM's async jobs. It is the right tool for cross-account fleet questions ('which pools have autoscale off', 'what hosts are running', 'who spiked usage') that the web UI only answers one account at a time.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI for Nerdio Manager for Enterprise (NME) - that product has a different API surface and help site
- Do not use this CLI for the Nerdio Distributor API (install registration/billing for distributors) - different auth and base path
- Do not use secure-variables list output in logs or shared context - values are stored secrets
- Do not use this CLI to create Entra ID app registrations or assign Azure roles - do that in the Azure portal first

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Async job plumbing
- **`job wait`**  -  Wait for any NMM async job to finish - polls until Completed/Failed/Cancelled and exits with a typed code reflecting the outcome.

  _Run this after any mutation (provisioning, scripted actions, backup) instead of hand-writing a polling loop._

  ```bash
  nerdio-cli job wait 4821 --interval 10s --timeout 30m --agent
  ```

### Cross-account fleet ops
- **`fleet autoscale-audit`**  -  See every host pool across all customer accounts whose autoscale is disabled or diverges from your baseline - the Monday cost-control sweep as one command.

  _Use this for cross-customer autoscale posture instead of scripting foreach loops over per-account API calls._

  ```bash
  nerdio-cli fleet autoscale-audit --accounts 101,102 --agent
  ```
- **`fleet host-estate`**  -  One table of every session host across all customer accounts with pool, account, and power state - the weekend power-sweep view.

  _Use this to answer 'what is running right now across all customers' in one call._

  ```bash
  nerdio-cli fleet host-estate --running-only --agent --select items.account,items.host,items.power_state
  ```
- **`scripted-actions fan-run`**  -  Execute one scripted action across many customer accounts, collect every returned job ID, and optionally wait for all of them to finish.

  _Use this to push one operation fleet-wide instead of hand-looping account IDs in PowerShell._

  ```bash
  nerdio-cli scripted-actions fan-run 42 --accounts 101,102,103 --wait
  ```

### Billing intelligence
- **`fleet billing-rollup`**  -  Per-account billed/paid/unpaid/usage rollup for a billing period, joined to account names - PSA-reconciliation-ready without Excel.

  _Use this for the weekly unpaid-invoice check and month-end reconciliation export._

  ```bash
  nerdio-cli fleet billing-rollup --period 2026-05-01:2026-05-31 --unpaid-only --agent
  ```
- **`usages drift`**  -  Flag customer accounts whose consumption grew or shrank beyond a threshold between two periods.

  _Use this before invoicing to catch consumption surprises early._

  ```bash
  nerdio-cli usages drift --from 2026-04-01:2026-04-30 --to 2026-05-01:2026-05-31 --min-pct 20 --agent
  ```

## Command Reference

**accounts**  -  MSP customer accounts managed by this NMM installation

- `nerdio-cli accounts`  -  List all customer accounts

**app-roles**  -  NMM application roles and assignments

- `nerdio-cli app-roles assignments`  -  List app role assignments
- `nerdio-cli app-roles roles`  -  List available app roles

**autoscale-profiles**  -  Reusable autoscale profiles

- `nerdio-cli autoscale-profiles account-get`  -  Get an account autoscale profile
- `nerdio-cli autoscale-profiles account-list`  -  List autoscale profiles for an account
- `nerdio-cli autoscale-profiles get`  -  Get an MSP-level autoscale profile
- `nerdio-cli autoscale-profiles list`  -  List MSP-level autoscale profiles

**backup**  -  Azure Backup operations for a customer account

- `nerdio-cli backup disable`  -  Disable backup for a protected item
- `nerdio-cli backup enable`  -  Enable backup for a resource with a policy
- `nerdio-cli backup protected-items`  -  List protected items for an account
- `nerdio-cli backup recovery-points`  -  List recovery points for a protected item
- `nerdio-cli backup restore`  -  Restore a protected item from a recovery point
- `nerdio-cli backup run`  -  Trigger an on-demand backup of a protected item

**cost-estimator**  -  Azure cost estimates built in NMM

- `nerdio-cli cost-estimator get`  -  Get a cost estimate by ID
- `nerdio-cli cost-estimator list`  -  List saved cost estimates

**desktop-images**  -  Golden desktop images managed by NMM

- `nerdio-cli desktop-images changelog`  -  Get desktop image change log
- `nerdio-cli desktop-images get`  -  Get desktop image details
- `nerdio-cli desktop-images list`  -  List desktop images for an account
- `nerdio-cli desktop-images schedules`  -  Get desktop image schedule configurations
- `nerdio-cli desktop-images start`  -  Start (power on) a desktop image VM
- `nerdio-cli desktop-images stop`  -  Stop (power off) a desktop image VM

**devices**  -  Intune-managed devices (v1-beta API)

- `nerdio-cli devices app-failures`  -  List app installation failures on a device
- `nerdio-cli devices apps`  -  List apps installed on a device
- `nerdio-cli devices bitlocker-keys`  -  Get BitLocker recovery keys for a device
- `nerdio-cli devices compliance`  -  Get compliance state for a device
- `nerdio-cli devices get`  -  Get an Intune device by ID
- `nerdio-cli devices hardware`  -  Get hardware inventory for a device
- `nerdio-cli devices laps`  -  Get local admin password (LAPS) for a device
- `nerdio-cli devices list`  -  List Intune devices for an account
- `nerdio-cli devices sync`  -  Trigger an Intune sync on a device

**directories**  -  Active Directory configurations

- `nerdio-cli directories account`  -  List directory configurations for an account
- `nerdio-cli directories list`  -  List MSP-level directory configurations

**environment-variables**  -  Environment variables for scripted actions

- `nerdio-cli environment-variables account`  -  List environment variables for an account
- `nerdio-cli environment-variables list`  -  List MSP-level environment variables

**fslogix**  -  FSLogix profile storage configurations

- `nerdio-cli fslogix <account_id>`  -  List FSLogix configurations for an account

**groups**  -  Entra ID groups within a customer account

- `nerdio-cli groups <account_id> <group_id>`  -  Get a group by ID

**host-pools**  -  AVD host pools within a customer account

- `nerdio-cli host-pools ad`  -  Get host pool Active Directory settings
- `nerdio-cli host-pools assigned-users`  -  List users assigned to a host pool
- `nerdio-cli host-pools autoscale`  -  Get host pool autoscale configuration
- `nerdio-cli host-pools avd`  -  Get host pool AVD settings
- `nerdio-cli host-pools create`  -  Create a host pool in an account
- `nerdio-cli host-pools delete`  -  Delete a host pool
- `nerdio-cli host-pools fslogix`  -  Get host pool FSLogix configuration
- `nerdio-cli host-pools list`  -  List host pools for an account
- `nerdio-cli host-pools rdp`  -  Get host pool RDP settings
- `nerdio-cli host-pools schedules`  -  Get host pool schedule configurations
- `nerdio-cli host-pools session-timeouts`  -  Get host pool session timeout settings
- `nerdio-cli host-pools sessions`  -  List active user sessions on a host pool
- `nerdio-cli host-pools set-autoscale`  -  Update host pool autoscale configuration (pass full config JSON via --stdin)
- `nerdio-cli host-pools tags`  -  Get host pool Azure tags
- `nerdio-cli host-pools vm-deployment`  -  Get host pool VM deployment settings

**hosts**  -  Session hosts within a host pool

- `nerdio-cli hosts list`  -  List session hosts in a host pool
- `nerdio-cli hosts restart`  -  Restart a session host VM
- `nerdio-cli hosts schedules`  -  Get schedule configurations for a session host
- `nerdio-cli hosts start`  -  Start a session host VM
- `nerdio-cli hosts stop`  -  Stop (deallocate) a session host VM

**invoices**  -  MSP billing invoices

- `nerdio-cli invoices get`  -  Get an invoice by ID
- `nerdio-cli invoices list`  -  List invoices in a billing period

**job**  -  Async jobs returned by NMM mutations

- `nerdio-cli job get`  -  Get an async job by ID
- `nerdio-cli job retry`  -  Restart a failed job
- `nerdio-cli job tasks`  -  List tasks of an async job

**networks**  -  Azure virtual networks for a customer account

- `nerdio-cli networks all`  -  List all networks visible to an account
- `nerdio-cli networks link`  -  Link an existing network to an account
- `nerdio-cli networks list`  -  List networks managed by NMM for an account

**provisioning**  -  Customer account provisioning operations

- `nerdio-cli provisioning link-network`  -  Link a network during account provisioning
- `nerdio-cli provisioning link-tenant`  -  Link an existing Entra tenant as a new NMM account (returns a job; poll jobs get)

**recovery-vaults**  -  Azure Recovery Services vaults for a customer account

- `nerdio-cli recovery-vaults all`  -  List all recovery vaults visible to an account
- `nerdio-cli recovery-vaults create`  -  Create a recovery vault
- `nerdio-cli recovery-vaults delete-policy`  -  Delete a backup policy
- `nerdio-cli recovery-vaults link`  -  Link an existing recovery vault to an account
- `nerdio-cli recovery-vaults linked`  -  List recovery vaults linked to an account
- `nerdio-cli recovery-vaults policies`  -  List backup policies in a recovery vault
- `nerdio-cli recovery-vaults policy`  -  Get a backup policy by name
- `nerdio-cli recovery-vaults region-policies`  -  Get vault policy info for an Azure region
- `nerdio-cli recovery-vaults unlink`  -  Unlink a recovery vault from an account

**reservations**  -  Azure VM reserved instances for a customer account

- `nerdio-cli reservations create`  -  Create a reservation
- `nerdio-cli reservations delete`  -  Delete a reservation
- `nerdio-cli reservations get`  -  Get a reservation by ID
- `nerdio-cli reservations list`  -  List reservations for an account
- `nerdio-cli reservations resources`  -  List resources attached to a reservation
- `nerdio-cli reservations update`  -  Update a reservation

**resource-groups**  -  Azure resource groups linked to NMM

- `nerdio-cli resource-groups account-link`  -  Link a resource group to an account
- `nerdio-cli resource-groups account-list`  -  List resource groups linked to an account
- `nerdio-cli resource-groups account-set-default`  -  Set the default resource group for an account
- `nerdio-cli resource-groups account-unlink`  -  Unlink a resource group from an account
- `nerdio-cli resource-groups link`  -  Link a resource group at MSP level
- `nerdio-cli resource-groups list`  -  List MSP-level linked resource groups
- `nerdio-cli resource-groups set-default`  -  Set the default MSP-level resource group
- `nerdio-cli resource-groups unlink`  -  Unlink an MSP-level resource group

**schedules**  -  Reusable schedules

- `nerdio-cli schedules account-configurations`  -  Get configurations for an account schedule
- `nerdio-cli schedules account-get`  -  Get an account schedule
- `nerdio-cli schedules account-list`  -  List schedules for an account
- `nerdio-cli schedules configurations`  -  Get configurations for an MSP-level schedule
- `nerdio-cli schedules get`  -  Get an MSP-level schedule
- `nerdio-cli schedules list`  -  List MSP-level schedules

**scripted-actions**  -  Scripted actions (MSP-level and per-account)

- `nerdio-cli scripted-actions account-list`  -  List scripted actions for an account
- `nerdio-cli scripted-actions list`  -  List MSP-level scripted actions
- `nerdio-cli scripted-actions run`  -  Execute an MSP-level scripted action (returns a job)
- `nerdio-cli scripted-actions run-account`  -  Execute a scripted action in an account context (returns a job)
- `nerdio-cli scripted-actions schedule`  -  Get the schedule for an account scripted action
- `nerdio-cli scripted-actions unschedule`  -  Remove the schedule from an account scripted action

**secure-variables**  -  Secure variables for scripted actions (values are secrets)

- `nerdio-cli secure-variables account-create`  -  Create a secure variable in an account
- `nerdio-cli secure-variables account-delete`  -  Delete a secure variable from an account
- `nerdio-cli secure-variables account-list`  -  List secure variables for an account (may expose stored secret values)
- `nerdio-cli secure-variables account-update`  -  Update a secure variable in an account
- `nerdio-cli secure-variables create`  -  Create an MSP-level secure variable
- `nerdio-cli secure-variables delete`  -  Delete an MSP-level secure variable
- `nerdio-cli secure-variables list`  -  List MSP-level secure variables (may expose stored secret values)
- `nerdio-cli secure-variables update`  -  Update an MSP-level secure variable

**usages**  -  Consumption/usage data

- `nerdio-cli usages account`  -  Get usage for one customer account between dates
- `nerdio-cli usages msp`  -  Get MSP-level usage between dates

**users**  -  Entra ID users within a customer account

- `nerdio-cli users get`  -  Get a user by ID
- `nerdio-cli users mfa`  -  Get MFA registration status for a user
- `nerdio-cli users search`  -  Search/list users in an account (paginated POST search)

**workspaces**  -  AVD workspaces for a customer account

- `nerdio-cli workspaces create`  -  Create an AVD workspace
- `nerdio-cli workspaces list`  -  List AVD workspaces for an account
- `nerdio-cli workspaces sessions`  -  List sessions in a workspace


## Freshness Contract

This printed CLI owns bounded freshness only for registered store-backed read command paths. In `--data-source auto` mode, those paths check `sync_state` and may run a bounded refresh before reading local data. `--data-source local` never refreshes. `--data-source live` reads the API and does not mutate the local store. Set `NERDIO_NO_AUTO_REFRESH=1` to skip the freshness hook without changing source selection.

Covered paths:

- `nerdio-cli accounts`
- `nerdio-cli autoscale-profiles`
- `nerdio-cli autoscale-profiles get`
- `nerdio-cli autoscale-profiles list`
- `nerdio-cli cost-estimator`
- `nerdio-cli cost-estimator get`
- `nerdio-cli cost-estimator list`
- `nerdio-cli directories`
- `nerdio-cli directories list`
- `nerdio-cli environment-variables`
- `nerdio-cli environment-variables list`
- `nerdio-cli resource-groups`
- `nerdio-cli resource-groups list`
- `nerdio-cli schedules`
- `nerdio-cli schedules get`
- `nerdio-cli schedules list`
- `nerdio-cli scripted-actions`
- `nerdio-cli scripted-actions list`
- `nerdio-cli secure-variables`
- `nerdio-cli secure-variables list`

When JSON output uses the generated provenance envelope, freshness metadata appears at `meta.freshness`. Treat it as current-cache freshness for the covered command path, not a guarantee of complete historical backfill or API-specific enrichment.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
nerdio-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Recipes

### Monday autoscale sweep

```bash
nerdio-cli fleet autoscale-audit --agent
```

Fan out across every customer account and flag host pools with autoscale disabled or diverging from baseline, with failed accounts reported separately.

### Weekend power check

```bash
nerdio-cli fleet host-estate --running-only --agent --select items.account,items.host,items.power_state
```

One narrow table of every session host still running across all customers - the deep response trimmed to three fields.

### Unpaid invoice check

```bash
nerdio-cli fleet billing-rollup --period 2026-05-01:2026-05-31 --unpaid-only
```

Per-account unpaid balance for the period, joined to account names from the local store.

### Fleet-wide scripted action

```bash
nerdio-cli scripted-actions fan-run 42 --accounts 101,102,103 --wait
```

Run one scripted action in three customer accounts and block until every returned job reaches a terminal state.

### Find an account fast

```bash
nerdio-cli search "contoso" --type accounts
```

Full-text search the synced local store instead of paging the live API.

## Auth Setup

The NMM Partner API is per-instance: every MSP hosts their own Nerdio Manager installation, so there is no vendor-global endpoint. Create an API client in your NMM portal (Settings -> Integrations -> REST API), then set five environment variables: NERDIO_BASE_URL (your instance root, e.g. https://nmm.contoso.com), NERDIO_TOKEN_URL (https://login.microsoftonline.com/<TENANT_ID>/oauth2/v2.0/token), NERDIO_CLIENT_ID, NERDIO_CLIENT_SECRET, and NERDIO_OAUTH_SCOPE. The scope is the bare Application ID URI default - '<app-id>/.default' with NO api:// prefix. Adding api:// triggers AADSTS500011; Nerdio's own docs got this wrong for years.

Run `nerdio-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  nerdio-cli accounts --agent --select id,name,status
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
nerdio-cli feedback "the --since flag is inclusive but docs say exclusive"
nerdio-cli feedback --stdin < notes.txt
nerdio-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/nerdio-cli/feedback.jsonl`. They are never POSTed unless `NERDIO_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `NERDIO_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
nerdio-cli profile save briefing --json
nerdio-cli --profile briefing accounts
nerdio-cli profile list --json
nerdio-cli profile show briefing
nerdio-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `nerdio-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/cloud/nerdio/cmd/nerdio-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add nerdio-mcp -- nerdio-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which nerdio-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   nerdio-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `nerdio-cli <command> --help`.
