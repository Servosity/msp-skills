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
      - kind: script
        bins: [nerdio-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.ps1
---

# Nerdio Manager - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `nerdio-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/nerdio/install.ps1 | iex
   ```
3. Verify: `nerdio-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `nerdio-cli` and `nerdio-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

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
  nerdio-cli fleet host-estate --running-only --agent --select items.account,items.power_state
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


**fleet**


- `nerdio-cli fleet autoscale-audit`  -  See every host pool across all customer accounts whose autoscale is disabled or diverges from your baseline - the Monday cost-control sweep as one command.

- `nerdio-cli fleet host-estate`  -  One table of every session host across all customer accounts with pool, account, and power state - the weekend power-sweep view.

- `nerdio-cli fleet billing-rollup`  -  Per-account billed/paid/unpaid/usage rollup for a billing period, joined to account names - PSA-reconciliation-ready without Excel.


**job**


- `nerdio-cli job wait`  -  Wait for any NMM async job to finish - polls until Completed/Failed/Cancelled and exits with a typed code reflecting the outcome.


**scripted-actions**


- `nerdio-cli scripted-actions fan-run`  -  Execute one scripted action across many customer accounts, collect every returned job ID, and optionally wait for all of them to finish.


**usages**


- `nerdio-cli usages drift`  -  Flag customer accounts whose consumption grew or shrank beyond a threshold between two periods.




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
- `nerdio-cli invoices`
- `nerdio-cli invoices get`
- `nerdio-cli invoices list`
- `nerdio-cli resource-groups`
- `nerdio-cli resource-groups list`
- `nerdio-cli schedules`
- `nerdio-cli schedules get`
- `nerdio-cli schedules list`
- `nerdio-cli scripted-actions`
- `nerdio-cli scripted-actions list`

When JSON output uses the generated provenance envelope, freshness metadata appears at `meta.freshness`. Treat it as current-cache freshness for the covered command path, not a guarantee of complete historical backfill or API-specific enrichment.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
nerdio-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Monday autoscale sweep

```bash
nerdio-cli fleet autoscale-audit --agent --max-accounts 50
```

Fan out across every customer account and flag host pools with autoscale disabled or diverging from baseline, with failed accounts reported separately.

### Weekend power check

```bash
nerdio-cli fleet host-estate --running-only --agent --select items.account,items.power_state
```

One narrow table of every session host still running across all customers - the deep response trimmed to account and power state.

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

## Credential safety

- Every API request and the bearer token go only to the scheme, host and port of `NERDIO_BASE_URL`, your own NMM instance; the shipped placeholder (`nmm.example.com`) is refused, never dialled with a credential. The client ID and secret go only to `NERDIO_TOKEN_URL`, your Entra tenant's token endpoint, as the token request itself, and only when that endpoint is on Entra ID's sign-in hosts (`login.microsoftonline.com`, `login.microsoftonline.us`, `login.partner.microsoftonline.cn`, `login.chinacloudapi.cn`) or on your NMM instance's own origin; a token URL anywhere else is refused. With no token URL (or the `YOUR_TENANT_ID` placeholder) or no `NERDIO_OAUTH_SCOPE`, nothing is sent and the error names the variable. A redirect to any other host is refused, so a credential never reaches it.
- Any output, file, MCP result or outgoing request that would repeat `NERDIO_CLIENT_ID`, `NERDIO_CLIENT_SECRET`, a minted or stored token, or a value piped in with `--stdin` is withheld whole: it prints "[credkit: withheld from <where>: it carried a credential this program holds]" in its place. A response field named as a token (`accessToken`, `sessionToken` and the like) prints as "[withheld]" and the rest of the record prints as usual. Responses are never cached on disk.
- Customer secrets you ask for by name (`devices laps`, `devices bitlocker-keys`, `secure-variables list`) are printed to you and written nowhere: they are never synced, cached or stored.
- Only `credentials.toml` (mode 0600) holds a credential. A client ID and secret supplied by the environment are never written to disk, and nor is the token minted for them: the process keeps it in memory (the MCP server reuses it until it expires). A token is cached in `credentials.toml` only beside the client pair stored there (not at all under `NERDIO_NO_CONFIG_WRITE=1`), and a token is only ever sent for the client and token URL it was minted for.
- `--dry-run` sends nothing and never mints a token. Over MCP, a call that changes Nerdio returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

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
  nerdio-cli accounts --agent --select id,name
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

- Use `--home <dir>` for one invocation, or set `NERDIO_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `NERDIO_CONFIG_DIR`, `NERDIO_DATA_DIR`, `NERDIO_STATE_DIR`, `NERDIO_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `NERDIO_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `nerdio-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "nerdio": {
        "command": "nerdio-mcp",
        "env": {
          "NERDIO_HOME": "/srv/nerdio"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `NERDIO_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `NERDIO_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are never cached on disk. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `NERDIO_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `NERDIO_DATA_SOURCE` sets the `--data-source` default.

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
nerdio-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "nerdio-cli learnings confirm 12"] }
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
       materially more, record the divergence via `nerdio-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `nerdio-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `nerdio-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
nerdio-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
nerdio-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
nerdio-cli teach-playbook \
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
nerdio-cli playbook amend \
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

`nerdio-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `NERDIO_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
nerdio-cli feedback "the --since flag is inclusive but docs say exclusive"
nerdio-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `NERDIO_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `NERDIO_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `nerdio-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add nerdio-mcp -- nerdio-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `nerdio-mcp --transport http` requires `NERDIO_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `NERDIO_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `NERDIO_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which nerdio-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   nerdio-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `nerdio-cli <command> --help`.
