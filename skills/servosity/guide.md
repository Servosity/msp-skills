# Servosity CLI

**The first MSP-fleet CLI for backup. Every Servosity API endpoint as a typed command, plus a local mirror that lets you ask questions the dashboard can't  -  across your whole book of clients.**

No competitor in the MSP backup space ships a fleet-wide CLI. Reach for servosity-cli when you need to triage attention across every client at once, generate the backup section of a QBR in 30 seconds, watch every restore queue during DR from one terminal, or reconcile your Servosity bill against what you're invoicing clients. Every response is agent-native: --json, --select, --csv, --dry-run, typed exit codes.

For the short install path see [README.md](./README.md). This file is the command reference.

## Authentication

Authenticate with your Servosity partner API token. Export SERVOSITY_MSP_TOKEN with your reseller-scoped token (find or rotate it in the Servosity partner portal). The CLI auto-resolves your reseller ID from your company list (override with SERVOSITY_MSP_RESELLER_ID) on first run  -  you never type it.

## Quick Start

```bash
# Confirm token works and API is reachable
servosity-cli doctor

# Pull companies, backups, and issues into the local SQLite mirror
servosity-cli sync

# Morning fleet sweep  -  what needs my attention across my book
servosity-cli attention --top 10

# Friday review  -  clients with stalled backups to follow up on
servosity-cli stale-backups --days 7

# Generate the backup section of a client's QBR as a PDF
servosity-cli qbr <company> --quarter 2026-Q1 --format pdf --out report.pdf

```

## Unique Features

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

## Usage

Run `servosity-cli --help` for the full command reference and flag list.

## Commands

### agent-login

Manage agent login

- **`servosity-cli agent-login create`** - Create
- **`servosity-cli agent-login list`** - List

### agent-sessions

Manage agent sessions

- **`servosity-cli agent-sessions <agent_session_id>`** - Read

### backup-job-report

Manage backup job report

- **`servosity-cli backup-job-report <backup_destination_id> <backup_id> <backup_job_id> <backup_set_id>`** - View detailed backup report for a backup job and destination.

### backup-job-report-summary

Manage backup job report summary

- **`servosity-cli backup-job-report-summary <backup_destination_id> <backup_id> <backup_job_id> <backup_set_id>`** - View summary backup report for a backup job and destination.

### backup-job-status

Manage backup job status

- **`servosity-cli backup-job-status <backup_id>`** - List backup job status for a backup account on a specific date.

### backup-jobs

Manage backup jobs

- **`servosity-cli backup-jobs <backup_id>`** - List backup jobs for a backup account.

### backup-plans

Manage backup plans

- **`servosity-cli backup-plans list`** - List backup plans.
- **`servosity-cli backup-plans read`** - View a backup plan.

### backup-search

Manage backup search

- **`servosity-cli backup-search`** - List

### backup-sets

Manage backup sets

- **`servosity-cli backup-sets create`** - Create a backup-set for a backup account.
- **`servosity-cli backup-sets delete`** - Delete a backup-set for a backup account.
- **`servosity-cli backup-sets list`** - List backup-sets for a backup account.
- **`servosity-cli backup-sets read`** - View a backup-set for a backup account.
- **`servosity-cli backup-sets update`** - Accepts a json body with the following optional parameters.

`ReadOnly`: Boolean

`Name`: String
Backup set name

`ShadowCopyEnabled`: Boolean
Enable Windows' Volume Shadow Copy for open file backup

`DeleteTempFile`: Boolean
Remove temporary files after backup

`LogRetentionDays`: Integer
Number of days to keep the backup set log

`FollowLink`: Boolean
Follow link of the backup files

`CompressType`: String
The value can be one of the following: "GzipBestSpeedCompression" (Fast), "GzipDefaultCompression" (Normal)

`LanDomain`: String
Windows User Authentication domain/host name

`LanUsername`: String
Windows User Authentication user name

`LanPassword`: String
Windows User Authentication user password

`WorkingDir`: String
Temporary Driectory for storing backup files

`UploadPermission`: Boolean
Enable to backup permission attribute of files

`ReminderSettings`

`InFileDeltaSettings`

`LocalCopySettings`

`RetentionPolicySettings`

`CdpSettingsV6`

`CdpSettingsV7`

`BandwidthControlSettings`

`FilterSettings`

`ScheduleSettings`

`DestinationSettings`

`SelectedSourceList`

`DeselectedSourceList`

`PreCommandList`

`PostCommandList`

`AllowedIPList`

`ApplicationSettings`

`DestinationList`

`EnableOpenDirect`: Boolean
Note: Cannot be changed once set

### backups

Manage backups

- **`servosity-cli backups create`** - Create a backup account.
- **`servosity-cli backups delete`** - Delete a backup account, also deleting all backup data.
- **`servosity-cli backups list`** - List backup accounts.
- **`servosity-cli backups mfa-codes`** - Mfa codes
- **`servosity-cli backups partial-update`** - Partial update
- **`servosity-cli backups read`** - View a backup account.
- **`servosity-cli backups update`** - Update a backup account.

### companies

Manage companies

- **`servosity-cli companies create`** - Create a company.
- **`servosity-cli companies delete`** - Delete a company, also deleting all backup accounts and backup data.
- **`servosity-cli companies fully-managed`** - List fully-managed companies.
- **`servosity-cli companies fully-managed-ng`** - List fully-managed companies.
- **`servosity-cli companies list`** - List companies.
- **`servosity-cli companies partial-update`** - Partial update
- **`servosity-cli companies read`** - View a company.
- **`servosity-cli companies summary`** - List companies with account summaries.
- **`servosity-cli companies summary-ng`** - Summary ng
- **`servosity-cli companies update`** - Update a company.

### company-notes

Manage company notes

- **`servosity-cli company-notes create`** - Create
- **`servosity-cli company-notes delete`** - Delete
- **`servosity-cli company-notes list`** - List
- **`servosity-cli company-notes partial-update`** - Partial update
- **`servosity-cli company-notes read`** - Read
- **`servosity-cli company-notes update`** - Update

### components

Manage components

- **`servosity-cli components`** - List

### contracts

Manage contracts

- **`servosity-cli contracts create`** - Create
- **`servosity-cli contracts get-by-token`** - Get by token
- **`servosity-cli contracts list`** - List
- **`servosity-cli contracts partial-update`** - Partial update
- **`servosity-cli contracts read`** - Read
- **`servosity-cli contracts signatures`** - Signatures
- **`servosity-cli contracts update`** - Update

### credentials

Manage credentials

- **`servosity-cli credentials create`** - Create
- **`servosity-cli credentials delete`** - Delete
- **`servosity-cli credentials list`** - List
- **`servosity-cli credentials partial-update`** - Partial update
- **`servosity-cli credentials read`** - Read
- **`servosity-cli credentials update`** - Update

### current-user

Manage current user

- **`servosity-cli current-user api-token-delete`** - Delete the current user's API token. A new one will be generated when requested.
- **`servosity-cli current-user api-token-list`** - You will receive JSON response with `token`.

To make API calls with the token, add an `Authorization` header to your request in this form:

`Authorization: Token XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX`
- **`servosity-cli current-user create`** - Change the password of the current logged in user.
- **`servosity-cli current-user groups-list`** - Groups list
- **`servosity-cli current-user helpjuice-sso-create`** - Helpjuice sso create
- **`servosity-cli current-user hubspot-sso-create`** - Hubspot sso create
- **`servosity-cli current-user list`** - Get information about the current logged in user.
- **`servosity-cli current-user mfa-backup-codes-list`** - Get unused backup codes.
If no unused codes are left, remove all and generate new codes.
- **`servosity-cli current-user mfa-backup-codes-update`** - Remove all backup codes and generate new codes.
- **`servosity-cli current-user notifications-delete`** - Notifications delete
- **`servosity-cli current-user notifications-list`** - Get current user notifications
- **`servosity-cli current-user profile-create`** - Profile create
- **`servosity-cli current-user profile-list`** - Profile list
- **`servosity-cli current-user start-mfa-create`** - Start mfa create
- **`servosity-cli current-user start-mfa-list`** - Start mfa list
- **`servosity-cli current-user start-mfa-verify-create`** - Start mfa verify create
- **`servosity-cli current-user verified-mfa-delete`** - Verified mfa delete
- **`servosity-cli current-user verified-mfa-list`** - Verified mfa list
- **`servosity-cli current-user verified-mfa-send-code-create`** - Verified mfa send code create

### download

Manage download

- **`servosity-cli download`** - Servosity one windows list

### dr-backups

Manage dr backups

- **`servosity-cli dr-backups create`** - Create a DR backup account.
- **`servosity-cli dr-backups delete`** - Delete a DR backup account.
- **`servosity-cli dr-backups list`** - List
- **`servosity-cli dr-backups partial-update`** - Update a DR backup account.
- **`servosity-cli dr-backups read`** - Read
- **`servosity-cli dr-backups update`** - Update a DR backup account.

### issue-comments

Manage issue comments

- **`servosity-cli issue-comments delete`** - Delete
- **`servosity-cli issue-comments update`** - Update

### issues

Manage issues

- **`servosity-cli issues archived`** - Archived
- **`servosity-cli issues ignored`** - Ignored
- **`servosity-cli issues list`** - List
- **`servosity-cli issues read`** - Read

### report-subscriptions

Manage report subscriptions

- **`servosity-cli report-subscriptions read`** - Read
- **`servosity-cli report-subscriptions unsubscribe`** - Unsubscribe
- **`servosity-cli report-subscriptions verify`** - Verify

### reports

Manage reports

- **`servosity-cli reports account-list`** - Get a report of backup account types for each company and reseller in CSV format.
- **`servosity-cli reports classic-usage-list`** - Get a usage report for all backup accounts in CSV format.
- **`servosity-cli reports clients-list`** - Get a report of backup account client versions.
- **`servosity-cli reports dr-from-email-list`** - Get a report of user profiles.
- **`servosity-cli reports maxio-price-points-list`** - Get CSV with all Maxio price points.
- **`servosity-cli reports product-list`** - Product list
- **`servosity-cli reports stale-backup-sets-list`** - Get a report of all backup set last backup complete times.
- **`servosity-cli reports usage-list`** - Usage list
- **`servosity-cli reports user-profiles-list`** - Get a report of user profiles.

### resellers

Manage resellers

- **`servosity-cli resellers partial-update`** - Partial update
- **`servosity-cli resellers read`** - View a reseller.
- **`servosity-cli resellers update`** - Update a reseller.

### restic-backups

Manage restic backups

- **`servosity-cli restic-backups create`** - Create a restic backup account.
- **`servosity-cli restic-backups delete`** - Delete a restic backup account.
- **`servosity-cli restic-backups list`** - List
- **`servosity-cli restic-backups partial-update`** - Update a restic backup account.
- **`servosity-cli restic-backups read`** - Read
- **`servosity-cli restic-backups update`** - Update a restic backup account.

### screenshot

Manage screenshot

- **`servosity-cli screenshot <key>`** - Read

### stats

Manage stats

- **`servosity-cli stats list`** - List
- **`servosity-cli stats live-list`** - Live list
- **`servosity-cli stats user-list`** - User list

### users

Manage users

- **`servosity-cli users create`** - Create
- **`servosity-cli users delete`** - Remove a user from a reseller or company group.
- **`servosity-cli users list`** - List
- **`servosity-cli users request-password-recovery-create`** - Request password recovery for a user.
- **`servosity-cli users reset-password-create`** - Pass only `token` to confirm the token is valid.

Pass `token` and `password` to set the user's password.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
servosity-cli agent-login list

# JSON for scripting and agents
servosity-cli agent-login list --json

# Filter to specific fields
servosity-cli agent-login list --json --select id,name,status

# Dry run  -  show the request without sending
servosity-cli agent-login list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
servosity-cli agent-login list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
servosity-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/servosity-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SERVOSITY_MSP_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `servosity-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `servosity-cli doctor` to check credentials
- Verify the environment variable is set: `echo $SERVOSITY_MSP_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **doctor reports authentication failure**  -  Export SERVOSITY_MSP_TOKEN with your reseller-scoped token. Get one from the Servosity partner portal.
- **qbr --format pdf returns 'PDF rendering requires Chrome'**  -  Install Chrome, Google Chrome, or Chromium. Or use --format md / --format html instead.
- **drift returns 'No snapshot found for metric ...'**  -  Run `servosity-cli attention` first to record the first snapshot, then run drift later to compare.
- **storage-trend says 'No historical data yet'**  -  Run `storage-trend <company> --snapshot` periodically (weekly cron is sensible) to build the trend line.
