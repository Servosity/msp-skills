---
name: servosity
description: "Use when the user asks where their attention is needed across Servosity clients, which backups went stale, what changed since yesterday, to build QBR backup reports, draft stale-backup follow-up emails, watch restore queues during a DR event, reconcile the Servosity bill, or find unprovisioned agents. Wraps the Servosity partner API plus a local fleet mirror with snapshot history. Trigger phrases: `what needs my attention on servosity`, `fleet stale backups`, `show me the QBR backup report for`, `triage servosity issues`, `drift since yesterday on servosity`, `watch restore queue`, `reconcile servosity bill`, `Servosity + ChatGPT`, `Servosity + Claude`, `use servosity`, `run servosity-cli`."
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
---

# Servosity Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `servosity-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/servosity/install.ps1 | iex
   ```
3. Verify: `servosity-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

No competitor in the MSP backup space ships a fleet-wide CLI. Reach for servosity-cli when you need to triage attention across every client at once, generate the backup section of a QBR in 30 seconds, watch every restore queue during DR from one terminal, or reconcile your Servosity bill against what you're invoicing clients. Every response is agent-native: --json, --select, --csv, --dry-run, typed exit codes.

## When to Use This CLI

Use servosity-cli when you're managing backups across a book of MSP clients on Servosity. It is the right tool for the morning attention sweep, the Friday stale-backup hunt, ad-hoc 'is ACME OK?' checks, batch issue triage during planned outages, restore-queue oversight during DR, monthly bill reconciliation, and quarterly client QBR backup sections. The local SQLite mirror + snapshot history make any 'what changed since X?' question answerable  -  which the web portal cannot do.

## When NOT to Use This CLI

- Not for initiating or managing restores  -  watch queues with `restore-queue watch`, but drive actual restore operations from the Servosity portal.
- Not for editing backup configurations, schedules, or retention policies on individual agents; this is fleet visibility + partner-API operations only.
- Not a cross-reseller view: the partner token is reseller-scoped, so one CLI instance sees one reseller's book.
- Not an alerting daemon  -  it polls on demand; wire `--json` output into your own scheduler/RMM for notifications.
- Routing within the CLI: per-company ranked list → `attention`; one fleet number set → `fleet-health`; what changed between runs → `drift`; single client deep-dive → `qbr`.

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
- **`qbr`**  -  Generate the backup section of a client's Quarterly Business Review as Markdown, HTML, or PDF. Job success rate, restore tests run this quarter, coverage map across all three engines, open issues, storage trend.

  _Use this 1-2 weeks before a client QBR. Saves 30-60 min of manual deck-building per client._

  ```bash
  servosity-cli qbr 4421 --quarter 2026-Q1 --format pdf --out acme-q1.pdf
  ```

### Daily ops efficiency
- **`triage`**  -  List open issues with filters, then batch-mutate them (ignore / archive / reactivate / comment) in one invocation with --dry-run support and typed exit codes.

  _Use when the issue queue is bursty or during a planned-outage window where many alerts cluster around one client._

  ```bash
  servosity-cli triage --company 4421 --ignore 18,22,29 --comment 'scheduled outage' --dry-run
  ```

### Disaster recovery
- **`restore-queue watch`**  -  Watch every active company's restore queue across the book during a DR event. Polls each company periodically and prints diffs since the last tick.

  _Use during an active disaster recovery event when multiple clients have restores in flight._

  ```bash
  servosity-cli restore-queue watch --interval 30s --json
  ```

### Business operations
- **`bill --reconcile`**  -  Pull the MSP's monthly Servosity bill and compare line-by-line against a CSV of what the MSP is invoicing their clients. Surfaces drift  -  clients under- or over-charged.

  _Run this every month-end before invoicing clients. Catches missed line items and pricing mismatches._

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
- **`fleet-health`**  -  One fleet-wide scorecard: 24h job success rate, companies with stale backups, and open issues, with week-over-week deltas.

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

## Command Reference

**agent-login**  -  Manage agent login

- `servosity-cli agent-login create`  -  Create
- `servosity-cli agent-login list`  -  List

**agent-sessions**  -  Manage agent sessions

- `servosity-cli agent-sessions <agent_session_id>`  -  Read

**backup-job-report**  -  Manage backup job report

- `servosity-cli backup-job-report <backup_destination_id> <backup_id> <backup_job_id> <backup_set_id>`  -  View detailed backup report for a backup job and destination.

**backup-job-report-summary**  -  Manage backup job report summary

- `servosity-cli backup-job-report-summary <backup_destination_id> <backup_id> <backup_job_id> <backup_set_id>`  -  View summary backup report for a backup job and destination.

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
- `servosity-cli companies fully-managed-ng`  -  List fully-managed companies.
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
- `servosity-cli current-user api-token-list`  -  You will receive JSON response with `token`.
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

**issue-comments**  -  Manage issue comments

- `servosity-cli issue-comments delete`  -  Delete
- `servosity-cli issue-comments update`  -  Update

**issues**  -  Manage issues

- `servosity-cli issues archived`  -  Archived
- `servosity-cli issues ignored`  -  Ignored
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


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
servosity-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

Generates a self-contained PDF with cover page, job success rate, restore tests, coverage table, open issues, and storage trend table. Hand it to the account lead 1-2 weeks before the QBR.

### Bill reconciliation against invoicing CSV

```bash
servosity-cli bill --reconcile invoiced-2026-05.csv --month 2026-05 --json
```

CSV columns: company_id, company_name, invoiced_amount. Output shows delta vs Servosity's bill, sorted by absolute delta  -  catches under-billing before month-end close.

### Restore-queue watch during DR event

```bash
servosity-cli restore-queue watch --interval 30s --json
```

Emits NDJSON, one tick per line. Pipe to `tee dr-event.log` to capture the timeline of every queue change for the post-mortem.

## Auth Setup

Authenticate with your Servosity partner API token. Export SERVOSITY_MSP_TOKEN with your reseller-scoped token (find or rotate it in the Servosity partner portal). The CLI auto-resolves your reseller ID from your company list (override with SERVOSITY_MSP_RESELLER_ID) on first run  -  you never type it.

Run `servosity-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  servosity-cli agent-login list --agent --select id,name,status
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
servosity-cli feedback "the --since flag is inclusive but docs say exclusive"
servosity-cli feedback --stdin < notes.txt
servosity-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/servosity-cli/feedback.jsonl`. They are never POSTed unless `SERVOSITY_MSP_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `SERVOSITY_MSP_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `servosity-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

The installer above drops `servosity-mcp` alongside the CLI. Register it:

```bash
claude mcp add servosity-mcp -- servosity-mcp
```

Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which servosity-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   servosity-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `servosity-cli <command> --help`.
