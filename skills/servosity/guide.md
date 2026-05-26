# Servosity Claude Code Skill and MCP Server - command reference

> Unofficial. Community-built Claude Code Skill and MCP server for the Servosity
> partner API. Servosity is a trademark of Servosity Inc.

Servosity REST API surface available to authenticated MSP partners. All operations are scoped to the authenticated reseller. Admin-only endpoints (cross-reseller listing, billing back-office, support tooling) are not included.

For the short install path see [README.md](./README.md). This file is the command reference.

## Quick Start

### 1. Install

See [Install](#install) above.

### 2. Set Up Credentials

Get your API key from your API provider's developer portal. The key typically looks like a long alphanumeric string.

```bash
export SERVOSITY_MSP_TOKEN="<paste-your-key>"
```

You can also persist this in your config file at `~/.config/servosity-partner-msp-pp-cli/config.toml`.

### 3. Verify Setup

```bash
servosity-cli doctor
```

This checks your configuration and credentials.

### 4. Try Your First Command

```bash
servosity-cli agent-login list
```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Fleet-wide intelligence
- **`attention`**  -  One screen across your whole book of clients. Merges open issues, stale backups, and in-flight DR events into a per-company ranked view, then persists the result so tomorrow's drift command can compare.

  _Reach for this in the morning to triage what needs follow-up across every client without clicking through a portal._

  ```bash
  servosity-cli attention --json
  ```
- **`drift`**  -  Diff two snapshots the CLI collected  -  show which companies got worse, which recovered, and which are new since a past anchor. Default compares yesterday-to-now on the attention metric.

  _Use Monday morning to start with situation awareness instead of treating every week as a fresh slate._

  ```bash
  servosity-cli drift --metric attention --from yesterday --to now --json
  ```
- **`stale-backups`**  -  Slice the synced `/reports/stale-backup-sets/` snapshot by reseller, company, age window, and backup engine  -  entirely offline once synced. Use --refresh to re-pull freshness from the live report.

  _Run this Friday afternoon to compile the list of clients you need to email about a stalled backup._

  ```bash
  servosity-cli stale-backups --days 7 --engine restic --json
  ```
- **`backup-facts`**  -  Unified view across Servosity's three backup engines (classic, restic, DR) for one company or all. Engine, ID, hostname, last_successful_at, status  -  joined from three local store tables into one table.

  _Reach for this when triaging a client who has multiple engines protecting different devices and you need to know which engine is failing where._

  ```bash
  servosity-cli backup-facts --company 4421 --status fail --json
  ```

- **`find`**  -  SQLite FTS5 across companies (name, billing notes), issues (title and comments), and backups (descriptive name, last error)  -  one query hits the whole fleet.

  _Use when you remember a phrase but not which entity owned it  -  one call replaces hunting through three list pages._

  ```bash
  servosity-cli find "image manager" --in issues,backups --json
  ```

### Per-company quick view
- **`company show`**  -  One command pulls a company's metadata + addresses + contracts + all backups across three engines + open issues + agent sessions into one human or `--json` view.

  _Use when a customer asks "is my backup OK?"  -  one call, every relevant fact, ready to paste into a ticket._

  ```bash
  servosity-cli company show 4421 --json
  ```

### Daily ops efficiency
- **`triage`**  -  List open issues with filters, then batch ignore / archive / reactivate / comment in one invocation. Plans by default; pass --confirm to mutate. Typed exit codes.

  _Use when the issue queue is bursty or during a planned-outage window where many alerts cluster around one client._

  ```bash
  servosity-cli triage --company 4421 --json
  servosity-cli triage --company 4421 --ignore 18,22,29 --ignore-until "6am tomorrow" --confirm
  ```

### Disaster recovery
- **`restore-queue list`**  -  List per-company restore queues across the whole book; `--watch` repolls on an interval and prints diffs since the last tick.

  _Use during an active disaster recovery event when multiple clients have restores in flight._

  ```bash
  servosity-cli restore-queue list --json
  servosity-cli restore-queue list --watch --interval 30s
  ```

### Tier-One support workflows
- **`clear`**  -  Resolve one or more names as companies (then resellers) and batch-ignore their active issues until a human-readable time. Defaults to --dry-run; pass --confirm to mutate.

  _Use when a partner is doing planned maintenance and you want their alert noise paused until morning  -  one command instead of dozens of UI clicks._

  ```bash
  servosity-cli clear "ACME Corp" --until "6am tomorrow"
  servosity-cli clear "ACME Corp" --until "6am tomorrow" --confirm
  ```
- **`stale-issues`**  -  Pull your FMDB companies, fetch active issues, classify known-safe-to-archive patterns from a shipped rule table, auto-archive the safe ones, ignore non-dashboard noise, and print unknowns for review. Defaults to --dry-run.

  _Run this every weekday before standup to clear the obvious stale noise off your dashboard so triage focuses on what's actually new._

  ```bash
  servosity-cli stale-issues --mine --json
  servosity-cli stale-issues --mine --cutoff "11pm yesterday" --auto-archive-known --confirm
  ```

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

Config file: `~/.config/servosity-partner-msp-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SERVOSITY_MSP_TOKEN` | per_call | Yes | Set to your API credential. |

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `servosity-cli doctor` to check credentials
- Verify the environment variable is set: `echo $SERVOSITY_MSP_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

---

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
