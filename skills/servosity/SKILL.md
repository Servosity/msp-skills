---
name: servosity
description: "Use when the user asks where their attention is needed across Servosity clients, which backups went stale, what changed since yesterday, to build QBR backup reports, draft stale-backup follow-up emails, watch restore queues during a DR event, reconcile the Servosity bill against client invoicing, find unprovisioned agents, audit retention tiers against a required minimum, or group this month's backup failures by root cause. Wraps the Servosity partner API plus a local fleet mirror with snapshot history. Trigger phrases: `what needs my attention on servosity`, `fleet stale backups`, `show me the QBR backup report for`, `triage servosity issues`, `drift since yesterday on servosity`, `watch restore queue`, `reconcile servosity bill`, `servosity retention audit`, `cluster servosity backup failures`, `Servosity + ChatGPT`, `Servosity + Claude`, `use servosity`, `run servosity-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Servosity"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - servosity-cli
    install:
      - kind: script
        bins: [servosity-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.ps1
---

# Servosity Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `servosity-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.ps1 | iex
   ```
3. Verify: `servosity-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `servosity-cli` and `servosity-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

No competitor in the MSP backup space ships a fleet-wide CLI. Reach for servosity-cli when you need to triage attention across every client at once, generate the backup section of a QBR in 30 seconds, watch every restore queue during DR from one terminal, or reconcile your Servosity bill against what you're invoicing clients. Every response is agent-native: --json, --select, --csv, --dry-run, typed exit codes.

## When to Use This CLI

Use servosity-cli when you're managing backups across a book of MSP clients on Servosity. It is the right tool for the morning attention sweep, the Friday stale-backup hunt, ad-hoc 'is ACME OK?' checks, batch issue triage during planned outages, restore-queue oversight during DR, monthly bill reconciliation, and quarterly client QBR backup sections. The local SQLite mirror + snapshot history make any 'what changed since X?' question answerable  -  which the web portal cannot do.

## When NOT to Use This CLI

- Not for initiating or managing restores  -  watch queues with `restore-queue watch`, but drive actual restore operations from the Servosity portal.
- Not for editing backup configurations, schedules, or retention policies on individual agents; this is fleet visibility + partner-API operations only.
- Not a cross-reseller view: the partner token is reseller-scoped, so one CLI instance sees one reseller's book.
- Not an alerting daemon  -  it polls on demand; wire `--json` output into your own scheduler/RMM for notifications.
- Routing within the CLI: per-company ranked list → `attention`; one fleet number set → `fleet-health`; what changed between runs → `drift`; single client deep-dive → `qbr`; retention below the contract → `retention-audit`; one root cause behind many failures → `failure-clusters`.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Fleet-wide intelligence
- **`attention`**  -  One screen across your whole book of clients. Merges open issues, stale backups into a per-company ranked view, then persists the result so tomorrow's drift command can compare.

  _Reach for this in the morning to triage what needs follow-up across every client without clicking through a portal._

  ```bash
  servosity-cli attention --top 10 --json
  ```
- **`drift`**  -  Diff two snapshots the CLI collected  -  show which companies got worse, which recovered, and which are new since a past anchor. Default compares yesterday-to-now on the attention metric.

  _Use Monday morning to start with situation awareness instead of treating every week as a fresh slate._

  ```bash
  servosity-cli drift --metric attention --from yesterday --to now --json
  ```
- **`stale-backups`**  -  Slice the stale-backup-sets report by company, age window, and backup engine  -  entirely offline once cached. Use --refresh to repull from the API.

  _Run this Friday afternoon to compile the list of clients you need to email about a stalled backup._

  ```bash
  servosity-cli stale-backups --days 7 --engine restic --json
  ```
- **`backup-facts`**  -  Unified view across Servosity's three backup engines (classic, restic, DR) for one company or all. Engine, ID, hostname, last_successful_at, state, and freshness-derived health  -  joined from three local store tables into one table.

  _Reach for this when triaging a client who has multiple engines protecting different devices and you need to know which engine is failing where._

  ```bash
  servosity-cli backup-facts --company 4421 --status stale --json
  ```

### Client-facing reporting
- **`qbr`**  -  Generate the backup section of a client's Quarterly Business Review as Markdown, HTML, or PDF. Backup success (restic/DR backups with a successful run in the 7 days before quarter end), restore tests run this quarter, coverage map across all three engines, open issues, storage trend.

  _Use this 1-2 weeks before a client QBR. Saves 30-60 min of manual deck-building per client._

  ```bash
  servosity-cli qbr 4421 --quarter 2026-Q1 --format pdf --out acme-q1.pdf
  ```

### Daily ops efficiency
- **`triage`**  -  List open issues with filters, then batch-mutate them (ignore / archive / reactivate / comment) in one invocation with --dry-run support and typed exit codes.

  _Use when the issue queue is bursty or during a planned-outage window where many alerts cluster around one client._

  ```bash
  servosity-cli triage --company 4421 --ignore 550e8400-e29b-41d4-a716-446655440000 --comment 'scheduled outage' --dry-run
  ```

### Disaster recovery
- **`restore-queue watch`**  -  Watch every active company's restore queue across the book during a DR event. Polls each company periodically and prints diffs since the last tick.

  _Use during an active disaster recovery event when multiple clients have restores in flight._

  ```bash
  servosity-cli restore-queue watch --interval 30s --json
  ```

### Business operations
- **`bill --reconcile`**  -  Pull the MSP's monthly Servosity bill and reconcile it against a CSV of what the MSP invoices their clients. Flags clients Servosity bills you for that you have not invoiced, and clients you invoice that have no Servosity backups.

  _Run this every month-end before invoicing clients. Catches clients you forgot to invoice. The partner bill carries no prices, so this is a presence check, not a dollar delta._

  ```bash
  servosity-cli bill --reconcile invoiced-2026-05.csv --month 2026-05 --json
  ```
- **`unprovisioned`**  -  List agents installed on client machines but not yet pulling backups, ranked by client. Surfaces lost revenue from incomplete onboardings.

  _Run weekly to catch agents installed during onboarding that never successfully phoned home._

  ```bash
  servosity-cli unprovisioned --age 24h --json
  ```
- **`storage-trend`**  -  Linear-regression forecast of when a specific client will hit a capacity threshold. Reads the historical storage_bytes time series from local snapshots; with --snapshot, persists a new measurement for future runs.

  _Run quarterly per high-storage client to identify upsell opportunities before they hit a hard limit._

  ```bash
  servosity-cli storage-trend 4421 --weeks 12 --threshold 1TB --json
  ```

### Local state that compounds
- **`email-draft`**  -  Generate ready-to-paste follow-up email bodies for every client with a stale backup, filled from the local store (client name, hosts, days stale, last success).

  _Reach for this on the Friday follow-up sweep to turn the stale list into sendable emails in one step._

  ```bash
  servosity-cli email-draft --stale --days 7
  ```

### Cross-tenant intelligence
- **`fleet-health`**  -  One fleet-wide scorecard: share of backups with a success in the last 24h, companies with stale backups, and open issues, with week-over-week deltas.

  _Reach for this when you need the owner-glance fleet number set, not the per-company list._

  ```bash
  servosity-cli fleet-health --json
  ```

### Reporting that writes itself
- **`qbr-all`**  -  Generate every client's QBR backup report in one pass, one file per company.

  _Reach for this at quarter end to produce the whole book's QBR backup sections at once._

  ```bash
  servosity-cli qbr-all --quarter 2026-Q1 --out ./qbrs/
  ```

### Fleet hygiene
- **`retention-audit`**  -  Every protected host across the classic, restic and DR engines whose retention tier is below a required minimum, or that is not eligible for the Servosity recovery guarantee, grouped by client. Reads the local store; no live calls.

  _Reach for this before a compliance review or when onboarding a regulated client, to prove every host meets the contracted retention and is covered by the guarantee._

  ```bash
  servosity-cli retention-audit --min 7YEAR --company 4421
  ```

### Morning triage
- **`failure-clusters`**  -  Pull this month's restic and DR backup-failure text for every client, normalize away IDs, paths, numbers and timestamps, and group identical root causes, ranked by how many hosts and clients each one hits.

  _Reach for this when the failure count jumps, to find the one root cause worth fixing first across the whole book._

  ```bash
  servosity-cli failure-clusters --month 2026-10 --top 10
  ```

## Command Reference

**agent-login**  -  Manage agent login

- `servosity-cli agent-login create`  -  Create
- `servosity-cli agent-login list`  -  List

**agent-sessions**  -  Manage agent sessions

- `servosity-cli agent-sessions <agent_session_id>`  -  Read

**backup-job-report**  -  Manage backup job report

- `servosity-cli backup-job-report <backup_id> <backup_set_id> <backup_job_id> <backup_destination_id>`  -  View detailed backup report for a backup job and destination.

**backup-job-report-summary**  -  Manage backup job report summary

- `servosity-cli backup-job-report-summary <backup_id> <backup_set_id> <backup_job_id> <backup_destination_id>`  -  View summary backup report for a backup job and destination.

**backup-job-status**  -  Manage backup job status

- `servosity-cli backup-job-status <backup_id>`  -  List backup job status for a backup account on a specific date.

**backup-jobs**  -  Manage backup jobs

- `servosity-cli backup-jobs <backup_id>`  -  List backup jobs for a backup account.

**backup-plans**  -  Manage backup plans

- `servosity-cli backup-plans list`  -  List backup plans.
- `servosity-cli backup-plans read`  -  View a backup plan.

**backup-search**  -  Manage backup search

- `servosity-cli backup-search`  -  List

**backup-sets**  -  Manage backup sets

- `servosity-cli backup-sets create`  -  Create a backup-set for a backup account.
- `servosity-cli backup-sets delete`  -  Delete a backup-set for a backup account.
- `servosity-cli backup-sets list`  -  List backup-sets for a backup account.
- `servosity-cli backup-sets read`  -  View a backup-set for a backup account.
- `servosity-cli backup-sets update`  -  Accepts a json body with the following optional parameters.

**backups**  -  Manage backups

- `servosity-cli backups create`  -  Create a backup account.
- `servosity-cli backups delete`  -  Delete a backup account, also deleting all backup data.
- `servosity-cli backups list`  -  List backup accounts.
- `servosity-cli backups mfa-codes`  -  Mfa codes
- `servosity-cli backups partial-update`  -  Partial update
- `servosity-cli backups read`  -  View a backup account.
- `servosity-cli backups update`  -  Update a backup account.

**companies**  -  Manage companies

- `servosity-cli companies create`  -  Create a company.
- `servosity-cli companies delete`  -  Delete a company, also deleting all backup accounts and backup data.
- `servosity-cli companies fully-managed`  -  List fully-managed companies.
- `servosity-cli companies list`  -  List companies.
- `servosity-cli companies partial-update`  -  Partial update
- `servosity-cli companies read`  -  View a company.
- `servosity-cli companies summary`  -  List companies with account summaries.
- `servosity-cli companies summary-ng`  -  Summary ng
- `servosity-cli companies update`  -  Update a company.

**company-notes**  -  Manage company notes

- `servosity-cli company-notes create`  -  Create
- `servosity-cli company-notes delete`  -  Delete
- `servosity-cli company-notes list`  -  List
- `servosity-cli company-notes partial-update`  -  Partial update
- `servosity-cli company-notes read`  -  Read
- `servosity-cli company-notes update`  -  Update

**components**  -  Manage components

- `servosity-cli components`  -  List

**contracts**  -  Manage contracts

- `servosity-cli contracts create`  -  Create
- `servosity-cli contracts get-by-token`  -  Get by token
- `servosity-cli contracts list`  -  List
- `servosity-cli contracts partial-update`  -  Partial update
- `servosity-cli contracts read`  -  Read
- `servosity-cli contracts signatures`  -  Signatures
- `servosity-cli contracts update`  -  Update

**credentials**  -  Manage credentials

- `servosity-cli credentials create`  -  Create
- `servosity-cli credentials delete`  -  Delete
- `servosity-cli credentials list`  -  List
- `servosity-cli credentials partial-update`  -  Partial update
- `servosity-cli credentials read`  -  Read
- `servosity-cli credentials update`  -  Update

**current-user**  -  Manage current user

- `servosity-cli current-user api-token-delete`  -  Delete the current user's API token. A new one will be generated when requested.
- `servosity-cli current-user api-token-list --reveal-token`  -  Retrieve the live API token. Sensitive, human-only output; excluded from MCP and sync.
- `servosity-cli current-user create`  -  Change the password of the current logged in user.
- `servosity-cli current-user groups-list`  -  Groups list
- `servosity-cli current-user helpjuice-sso-create`  -  Helpjuice sso create
- `servosity-cli current-user hubspot-sso-create`  -  Hubspot sso create
- `servosity-cli current-user list`  -  Get information about the current logged in user.
- `servosity-cli current-user mfa-backup-codes-list`  -  Get unused backup codes. If no unused codes are left, remove all and generate new codes.
- `servosity-cli current-user mfa-backup-codes-update`  -  Remove all backup codes and generate new codes.
- `servosity-cli current-user notifications-delete`  -  Notifications delete
- `servosity-cli current-user notifications-list`  -  Get current user notifications
- `servosity-cli current-user profile-create`  -  Profile create
- `servosity-cli current-user profile-list`  -  Profile list
- `servosity-cli current-user start-mfa-create`  -  Start mfa create
- `servosity-cli current-user start-mfa-list`  -  Start mfa list
- `servosity-cli current-user start-mfa-verify-create`  -  Start mfa verify create
- `servosity-cli current-user verified-mfa-delete`  -  Verified mfa delete
- `servosity-cli current-user verified-mfa-list`  -  Verified mfa list
- `servosity-cli current-user verified-mfa-send-code-create`  -  Verified mfa send code create

**download**  -  Manage download

- `servosity-cli download`  -  Servosity one windows list

**dr-backups**  -  Manage dr backups

- `servosity-cli dr-backups create`  -  Create a DR backup account.
- `servosity-cli dr-backups delete`  -  Delete a DR backup account.
- `servosity-cli dr-backups list`  -  List
- `servosity-cli dr-backups partial-update`  -  Update a DR backup account.
- `servosity-cli dr-backups read`  -  Read
- `servosity-cli dr-backups update`  -  Update a DR backup account.

**fully_managed_companies**  -  Manage fully managed companies

- `servosity-cli fully-managed-companies`  -  List fully-managed companies.

**issue-comments**  -  Manage issue comments

- `servosity-cli issue-comments delete`  -  Delete
- `servosity-cli issue-comments update`  -  Update

**issues**  -  Manage issues

- `servosity-cli issues list`  -  List
- `servosity-cli issues read`  -  Read

**report-subscriptions**  -  Manage report subscriptions

- `servosity-cli report-subscriptions read`  -  Read
- `servosity-cli report-subscriptions unsubscribe`  -  Unsubscribe
- `servosity-cli report-subscriptions verify`  -  Verify

**reports**  -  Manage reports

- `servosity-cli reports account-list`  -  Get a report of backup account types for each company and reseller in CSV format.
- `servosity-cli reports classic-usage-list`  -  Get a usage report for all backup accounts in CSV format.
- `servosity-cli reports clients-list`  -  Get a report of backup account client versions.
- `servosity-cli reports dr-from-email-list`  -  Get a report of user profiles.
- `servosity-cli reports maxio-price-points-list`  -  Get CSV with all Maxio price points.
- `servosity-cli reports product-list`  -  Product list
- `servosity-cli reports stale-backup-sets-list`  -  Get a report of all backup set last backup complete times.
- `servosity-cli reports usage-list`  -  Usage list
- `servosity-cli reports user-profiles-list`  -  Get a report of user profiles.

**resellers**  -  Manage resellers

- `servosity-cli resellers partial-update`  -  Partial update
- `servosity-cli resellers read`  -  View a reseller.
- `servosity-cli resellers update`  -  Update a reseller.

**restic-backups**  -  Manage restic backups

- `servosity-cli restic-backups create`  -  Create a restic backup account.
- `servosity-cli restic-backups delete`  -  Delete a restic backup account.
- `servosity-cli restic-backups list`  -  List
- `servosity-cli restic-backups partial-update`  -  Update a restic backup account.
- `servosity-cli restic-backups read`  -  Read
- `servosity-cli restic-backups update`  -  Update a restic backup account.

**screenshot**  -  Manage screenshot

- `servosity-cli screenshot <key>`  -  Read

**stats**  -  Manage stats

- `servosity-cli stats list`  -  List
- `servosity-cli stats live-list`  -  Live list
- `servosity-cli stats user-list`  -  User list

**users**  -  Manage users

- `servosity-cli users create`  -  Create
- `servosity-cli users delete`  -  Remove a user from a reseller or company group.
- `servosity-cli users list`  -  List
- `servosity-cli users request-password-recovery-create`  -  Request password recovery for a user.
- `servosity-cli users reset-password-create`  -  Pass only `token` to confirm the token is valid. Pass `token` and `password` to set the user's password.


**attention**


- `servosity-cli attention`  -  One screen across your whole book of clients. Merges open issues, stale backups into a per-company ranked view, then persists the result so tomorrow's drift command can compare.


**backup-facts**


- `servosity-cli backup-facts`  -  Unified view across Servosity's three backup engines (classic, restic, DR) for one company or all. Engine, ID, hostname, last_successful_at, state, and freshness-derived health  -  joined from three local store tables into one table.


**bill**


- `servosity-cli bill`  -  Pull the MSP's monthly Servosity bill and reconcile it against a CSV of what the MSP invoices their clients. Flags clients Servosity bills you for that you have not invoiced, and clients you invoice that have no Servosity backups.


**drift**


- `servosity-cli drift`  -  Diff two snapshots the CLI collected  -  show which companies got worse, which recovered, and which are new since a past anchor. Default compares yesterday-to-now on the attention metric.


**email-draft**


- `servosity-cli email-draft`  -  Generate ready-to-paste follow-up email bodies for every client with a stale backup, filled from the local store (client name, hosts, days stale, last success).


**failure-clusters**


- `servosity-cli failure-clusters`  -  Pull this month's restic and DR backup-failure text for every client, normalize away IDs, paths, numbers and timestamps, and group identical root causes, ranked by how many hosts and clients each one hits.


**fleet-health**


- `servosity-cli fleet-health`  -  One fleet-wide scorecard: share of backups with a success in the last 24h, companies with stale backups, and open issues, with week-over-week deltas.


**qbr**


- `servosity-cli qbr`  -  Generate the backup section of a client's Quarterly Business Review as Markdown, HTML, or PDF. Backup success (restic/DR backups with a successful run in the 7 days before quarter end), restore tests run this quarter, coverage map across all three engines, open issues, storage trend.


**qbr-all**


- `servosity-cli qbr-all`  -  Generate every client's QBR backup report in one pass, one file per company.


**restore-queue**


- `servosity-cli restore-queue watch`  -  Watch every active company's restore queue across the book during a DR event. Polls each company periodically and prints diffs since the last tick.


**retention-audit**


- `servosity-cli retention-audit`  -  Every protected host across the classic, restic and DR engines whose retention tier is below a required minimum, or that is not eligible for the Servosity recovery guarantee, grouped by client. Reads the local store; no live calls.


**stale-backups**


- `servosity-cli stale-backups`  -  Slice the stale-backup-sets report by company, age window, and backup engine  -  entirely offline once cached. Use --refresh to repull from the API.


**storage-trend**


- `servosity-cli storage-trend`  -  Linear-regression forecast of when a specific client will hit a capacity threshold. Reads the historical storage_bytes time series from local snapshots; with --snapshot, persists a new measurement for future runs.


**triage**


- `servosity-cli triage`  -  List open issues with filters, then batch-mutate them (ignore / archive / reactivate / comment) in one invocation with --dry-run support and typed exit codes.


**unprovisioned**


- `servosity-cli unprovisioned`  -  List agents installed on client machines but not yet pulling backups, ranked by client. Surfaces lost revenue from incomplete onboardings.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
servosity-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning attention sweep with field projection

```bash
servosity-cli attention --top 5 --json --select companies.company_name,companies.score,companies.open_issues
```

Narrow the output to just the fields an agent cares about  -  keeps token usage low and pipes cleanly to jq or downstream tools.

### Friday stale-backup follow-up list

```bash
servosity-cli stale-backups --days 7 --engine restic --csv
```

CSV output for paste-into-spreadsheet workflows when you're compiling the list of clients to email.

### Client QBR pack

```bash
servosity-cli qbr 4421 --quarter 2026-Q1 --format pdf --out acme-q1-backup.pdf
```

Generates a self-contained PDF with cover page, backup success, restore tests, coverage table, open issues, and storage trend table. Hand it to the account lead 1-2 weeks before the QBR.

### Bill reconciliation against invoicing CSV

```bash
servosity-cli bill --reconcile invoiced-2026-05.csv --month 2026-05 --json
```

CSV columns: company_id, company_name, invoiced_amount. The partner bill carries billed backups per company but no prices, so this is a presence check: it flags clients Servosity bills you for that are not invoiced, and invoiced clients with no billed backups. It does not compare dollar amounts. Rows sort by billed backups.

### Restore-queue watch during DR event

```bash
servosity-cli restore-queue watch --interval 30s --json
```

Emits NDJSON, one tick per line. Pipe to `tee dr-event.log` to capture the timeline of every queue change for the post-mortem.

## Auth Setup

Authenticate with your Servosity partner API token. Export SERVOSITY_MSP_TOKEN with your reseller-scoped token (find or rotate it in the Servosity partner portal). The CLI auto-resolves your reseller ID from your company list (override with SERVOSITY_MSP_RESELLER_ID) on first run  -  you never type it.

Run `servosity-cli doctor` to verify setup.

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
  servosity-cli agent-login list --agent
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

- Use `--home <dir>` for one invocation, or set `SERVOSITY_MSP_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `SERVOSITY_MSP_CONFIG_DIR`, `SERVOSITY_MSP_DATA_DIR`, `SERVOSITY_MSP_STATE_DIR`, `SERVOSITY_MSP_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `SERVOSITY_MSP_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `servosity-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "servosity": {
        "command": "servosity-mcp",
        "env": {
          "SERVOSITY_MSP_HOME": "/srv/servosity"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `SERVOSITY_MSP_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `SERVOSITY_MSP_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `SERVOSITY_MSP_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `SERVOSITY_MSP_DATA_SOURCE` sets the `--data-source` default.

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
servosity-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "servosity-cli learnings confirm 12"] }
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
       materially more, record the divergence via `servosity-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `servosity-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `servosity-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
servosity-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
servosity-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
servosity-cli teach-playbook \
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
servosity-cli playbook amend \
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

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the generator. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`servosity-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `SERVOSITY_MSP_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
servosity-cli feedback "the --since flag is inclusive but docs say exclusive"
servosity-cli feedback --stdin < notes.txt
servosity-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `SERVOSITY_MSP_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `SERVOSITY_MSP_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
servosity-cli profile save briefing --json
servosity-cli --profile briefing agent-login list
servosity-cli profile list --json
servosity-cli profile show briefing
servosity-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `servosity-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add servosity-mcp -- servosity-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `servosity-mcp --transport http` requires `SERVOSITY_MSP_MCP_HTTP_TOKEN` (callers send `Authorization: Bearer <token>`), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. `SERVOSITY_MSP_MCP_READ_ONLY=1` drops every tool not marked read-only (the local-write learn-loop tools such as `recall` and `teach` go too) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which servosity-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   servosity-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `servosity-cli <command> --help`.
