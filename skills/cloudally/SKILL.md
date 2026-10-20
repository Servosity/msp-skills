---
name: cloudally
description: "Use when the user asks about CloudAlly backups of Microsoft 365 or Google Workspace: restores many mailboxes to one past backup date, builds and downloads year-end mailbox archives, lists a mailbox's past backups (which CloudAlly's documented API cannot), and finds stale or failing backups across every partner customer. Trigger phrases: `restore all mailboxes to a date in CloudAlly`, `batch restore CloudAlly mailboxes`, `download every mailbox as of December 31`, `year-end mailbox archive from CloudAlly`, `list a CloudAlly mailbox's past backups`, `which CloudAlly backups are failing`, `check CloudAlly restore progress`, `use cloudally`, `run cloudally-cli`."
author: "Jeff Moore"
license: "Apache-2.0"
vendor: "CloudAlly"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - cloudally-cli
    install:
      - kind: script
        bins: [cloudally-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cloudally/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cloudally/install.ps1
---

# CloudAlly - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `cloudally-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cloudally/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cloudally/install.ps1 | iex
   ```
3. Verify: `cloudally-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `cloudally-cli` and `cloudally-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Log in as one company, or as a partner that reaches every customer, and drive CloudAlly backups from scripts or an agent. The restore group plans, submits, watches and remembers multi-mailbox restores to a chosen backup date. The download group builds year-end mailbox archives for every mailbox backed up on one date and collects them to disk. `backup snapshots` reads a mailbox's past backups from the customer portal, which the documented API cannot, and `backup health` sweeps every customer for stale or failing backups.

## When to Use This CLI

Use this CLI for anything that touches one CloudAlly company account (the default login) or, with a partner login, every customer account: listing backup tasks, restoring mailboxes and sites from a backup date, downloading mailbox archives as they stood on a past date, managing tasks and entities, and checking backup health across customers. It is the right tool when more than one mailbox needs restoring to the same date, when that date is in the past and each mailbox has to be resolved to the backup taken on or before it, or when restore history older than a week is needed.

## Anti-triggers

Do not use this CLI for:
- Do not use it to restore individual emails or files inside a mailbox by picking items in a browser; it restores whole entities (mailbox, site, drive) or JSON-specified item lists only.
- Do not use it for Microsoft 365 administration itself (creating users, licenses, mailbox settings); it only drives CloudAlly backups of those tenants.
- Do not use it to create API keys; client-id and client-secret are created in the CloudAlly partner portal.
- Do not use it for billing reconciliation on indirect partner accounts; CloudAlly blocks billing endpoints by default for them.

## Unique Capabilities

### Batch restore by date
- **`restore plan`** - Preview every mailbox-and-date restore a batch would submit, with the exact backup each mailbox will be restored from.

  _Reach for this before any multi-mailbox restore so the operator can review what will be restored and from which backup._

  ```bash
  cloudally-cli restore plan --task 64b0e9 --date 2026-10-01 --date 2026-10-03 --agent
  ```
- **`restore submit`** - Submit one restore per mailbox per date with a local ledger and a restore guard (`restore-guard.db` in the state directory) shared by every login and token that uses that state directory: a restore already sent from it is never sent again unless you pass --new-batch. Another OS user, `--home`, `CLOUDALLY_HOME` or `CLOUDALLY_STATE_DIR` has its own guard, and `recovery restore` and the MCP `cloudally_execute` tool bypass it; --resume keeps the batch's recorded target account and restore options.

  _Use this instead of looping over recovery restore when more than one mailbox or date is involved._

  ```bash
  cloudally-cli restore submit --task 64b0e9 --date 2026-10-01 --dry-run
  ```
- **`restore watch`** - Follow a submitted batch until every restore finishes, with a per-mailbox tally and a non-zero exit if any failed.

  _Pick this to track a batch you submitted; use tasks list --type RESTORE only for raw job listings._

  ```bash
  cloudally-cli restore watch --batch 20261015-120000-ab12 --interval 30s --agent
  ```
- **`restore history`** - See what was restored for which company and when, from the local ledger, without calling the API.

  _Use this for audit questions about past restores; it never calls the API._

  ```bash
  cloudally-cli restore history --since 2026-09-01 --agent
  ```
- **`backup snapshots`** - List every past backup run for one mailbox in a given month, with each run's date in both ISO and the raw yyyyMMddHHmmss form a restore body needs.

  _Use this to confirm a past restore point exists before restoring anything. It is read-only and needs a partner login plus --account._

  ```bash
  cloudally-cli backup snapshots --account 5f1c2a --task 64b0e9 --entity jane.doe@contoso.com --month 2025-12 --agent
  ```

### Mailbox downloads to a past date
- **`download submit`** - Create one download (export) job per mailbox and sub-source for every mailbox backed up on one date, record every row in a local ledger, and reconcile it against the portal.

  _Use this for year-end or legal-hold archives of many mailboxes, and for any MS365 mailbox. Pass `--utc-offset` so `--on-date` means the account's own calendar day, and `--dry-run` first._

  ```bash
  cloudally-cli download submit --account 5f1c2a --task 64b0e9 --on-date 2025-12-31 --utc-offset -5 --dry-run
  ```
- **`download fetch`** - Download every ready archive to disk, one folder per year, mailbox and sub-source, minting and streaming each 60-second link in one step and resuming safely after an interruption.

  _Use this to collect the archives `download submit` created; `--plan-only` lists what it would write._

  ```bash
  cloudally-cli download fetch --account 5f1c2a --dest ./archives --year 2025 --on-date 2025-12-31 --plan-only
  ```
- **`download start`** - Build one archive of one mailbox from the backup taken on or before a date, for single-archive sources (MS365 tasks go through `download submit`).

  ```bash
  cloudally-cli download start --account 5f1c2a --task 64b0e9 --entity jane.doe@contoso.com --date 2025-12-31 --dry-run
  ```
- **`download list`**, **`download watch`** and **`download link`** - List an account's download jobs, follow one to READY_FOR_DOWNLOAD, and mint a fresh link for a finished one. A link is valid for about 60 seconds: hand it to a program, never to a ticket.

  ```bash
  cloudally-cli download watch --account 5f1c2a --job 9f2b1c --once
  ```

### Partner operations
- **`backup health`** - Find every mailbox or site across all customer accounts that has not backed up in N days or whose last attempt failed, with the error text.

  _Use this for a partner-wide health check instead of paging through distributor-accounts backup-statuses._

  ```bash
  cloudally-cli backup health --stale-days 2 --group-by account --agent
  ```

## Command Reference

**distributor** - Manage distributor

- `cloudally-cli distributor bills` - bills
- `cloudally-cli distributor create-user` - Create a secondary partner under a specific reseller.
- `cloudally-cli distributor delete-user` - Delete a secondary user from a partner.
- `cloudally-cli distributor details` - Returns partner details

**distributor_account_tasks** - Manage distributor account tasks

- `cloudally-cli distributor-account-tasks activate` - Activate a backup task to run on schedule, if it was paused.
- `cloudally-cli distributor-account-tasks activate-all` - Activate all accounts in a task for a backup.
- `cloudally-cli distributor-account-tasks activate-entity` - Activate a specific account in a task for a backup.
- `cloudally-cli distributor-account-tasks add-entity` - Manually add a site or account for a backup in a backup task.
- `cloudally-cli distributor-account-tasks backup-entity-now` - Run a backup for the specified account or site.
- `cloudally-cli distributor-account-tasks backup-now` - Run a backup task with all the activated accounts or sites in it.
- `cloudally-cli distributor-account-tasks cancel` - Cancel the specified running task (an entire backup, restore or download tasks).
- `cloudally-cli distributor-account-tasks cancel-entity` - Cancel the specified running task of a specified account or a site.
- `cloudally-cli distributor-account-tasks delete` - Delete a backup task, removing all the data.
- `cloudally-cli distributor-account-tasks get` - Get information for the specified task
- `cloudally-cli distributor-account-tasks list` - List all tasks of a specified type: backup, restore, download (paginated).
- `cloudally-cli distributor-account-tasks pause` - Pause execution by schedule for all accounts or sites of a backup task.
- `cloudally-cli distributor-account-tasks pause-entity` - Pause execution by schedule of a specified account or a site.
- `cloudally-cli distributor-account-tasks remove-entity` - Deactivate an account or site from a backup, removing all the coorresponding data.

**distributor_accounts** - Manage distributor accounts

- `cloudally-cli distributor-accounts access-token` - Access token
- `cloudally-cli distributor-accounts backup-statuses` - Get all active backup statuses.
- `cloudally-cli distributor-accounts create` - Create a Cloudally account (trial status).
- `cloudally-cli distributor-accounts list` - Get all users belonging to the partner.
- `cloudally-cli distributor-accounts sign-in-token` - Sign in token
- `cloudally-cli distributor-accounts tasks` - tasks
- `cloudally-cli distributor-accounts unsubscribe` - Cancel subscription and delete a Cloudally account.

**distributor_recovery** - Manage distributor recovery

- `cloudally-cli distributor-recovery export` - The documented export call; on the live API it answers 200 and creates no job. Use `download submit` or `download start`.
- `cloudally-cli distributor-recovery restore` - Create a restore job for one entity (mailbox, site, drive) of a backup task from a specific backup date. Writes back into the customer's live mailbox, site or drive.

**distributor_resellers** - Manage distributor resellers

- `cloudally-cli distributor-resellers accounts` - accounts
- `cloudally-cli distributor-resellers all-accounts` - All accounts
- `cloudally-cli distributor-resellers all-backup-statuses` - Get backup status for all resellers
- `cloudally-cli distributor-resellers all-bills` - Get bills for all resellers
- `cloudally-cli distributor-resellers all-tasks` - Get backup tasks for all resellers
- `cloudally-cli distributor-resellers backup-statuses` - Get all active backup statuses.
- `cloudally-cli distributor-resellers bills` - bills
- `cloudally-cli distributor-resellers create` - Create a reseller
- `cloudally-cli distributor-resellers create-account` - Create a Cloudally account (trial status).
- `cloudally-cli distributor-resellers create-user` - Create a secondary user under a specific reseller.
- `cloudally-cli distributor-resellers delete-user` - Delete a secondary user from a reseller.
- `cloudally-cli distributor-resellers get` - Returns reseller by partnerId
- `cloudally-cli distributor-resellers list` - Returns list of current distributor's resellers
- `cloudally-cli distributor-resellers tasks` - tasks
- `cloudally-cli distributor-resellers unsubscribe-account` - Cancel subscription and delete a Cloudally account.
- `cloudally-cli distributor-resellers update` - update a reseller

**recovery** - Manage recovery

- `cloudally-cli recovery export` - The documented export call; on the live API it answers 200 and creates no job. Use `download submit` or `download start`.
- `cloudally-cli recovery restore` - Create a restore job for one entity (mailbox, site, drive) of a backup task from a specific backup date. Writes back into the customer's live mailbox, site or drive.

**reseller** - Manage reseller

- `cloudally-cli reseller bills` - bills
- `cloudally-cli reseller create-user` - Create a secondary user under the reseller.
- `cloudally-cli reseller delete-user` - Delete a secondary user from a reseller.
- `cloudally-cli reseller details` - Returns partner details

**reseller_account_tasks** - Manage reseller account tasks

- `cloudally-cli reseller-account-tasks activate` - Activate a backup task to run on schedule, if it was paused.
- `cloudally-cli reseller-account-tasks activate-all` - Activate all accounts in a task for a backup.
- `cloudally-cli reseller-account-tasks activate-entity` - Activate a specific account in a task for a backup.
- `cloudally-cli reseller-account-tasks add-entity` - Manually add a site or account for a backup in a backup task.
- `cloudally-cli reseller-account-tasks backup-entity-now` - Run a backup for the specified account or site.
- `cloudally-cli reseller-account-tasks backup-now` - Run a backup task with all the activated accounts or sites in it.
- `cloudally-cli reseller-account-tasks cancel` - Cancel the specified running task (an entire backup, restore or download tasks).
- `cloudally-cli reseller-account-tasks cancel-entity` - Cancel the specified running task of a specified account or a site.
- `cloudally-cli reseller-account-tasks delete` - Delete a backup task, removing all the data.
- `cloudally-cli reseller-account-tasks get` - Get information for the specified task
- `cloudally-cli reseller-account-tasks list` - List all tasks of a specified type: backup, restore, download (paginated).
- `cloudally-cli reseller-account-tasks pause` - Pause execution by schedule for all accounts or sites of a backup task.
- `cloudally-cli reseller-account-tasks pause-entity` - Pause execution by schedule of a specified account or a site.
- `cloudally-cli reseller-account-tasks remove-entity` - Deactivate an account or site from a backup, removing all the coorresponding data.

**reseller_accounts** - Manage reseller accounts

- `cloudally-cli reseller-accounts access-token` - Access token
- `cloudally-cli reseller-accounts backup-statuses` - Get all active backup statuses.
- `cloudally-cli reseller-accounts create` - Create a Cloudally account (trial status).
- `cloudally-cli reseller-accounts list` - Get all users belonging to the partner.
- `cloudally-cli reseller-accounts sign-in-token` - Sign in token
- `cloudally-cli reseller-accounts tasks` - tasks
- `cloudally-cli reseller-accounts unsubscribe` - Cancel subscription and delete a Cloudally account.

**reseller_recovery** - Manage reseller recovery

- `cloudally-cli reseller-recovery export` - The documented export call; on the live API it answers 200 and creates no job. Use `download submit` or `download start`.
- `cloudally-cli reseller-recovery restore` - Create a restore job for one entity (mailbox, site, drive) of a backup task from a specific backup date. Writes back into the customer's live mailbox, site or drive.

**session** - Manage session

- `cloudally-cli session login` - Returns the access and refresh tokens.
- `cloudally-cli session partner-login` - Returns the access and refresh tokens.
- `cloudally-cli session refresh` - Returns the access and refresh tokens.

**tasks** - Manage tasks

- `cloudally-cli tasks activate` - Activate a backup task to run on schedule, if it was paused.
- `cloudally-cli tasks activate-all` - Activate all accounts in a task for a backup.
- `cloudally-cli tasks activate-entity` - Activate a specific account in a task for a backup.
- `cloudally-cli tasks add-entity` - Manually add a site or account for a backup in a backup task.
- `cloudally-cli tasks backup-entity-now` - Run a backup for the specified account or site.
- `cloudally-cli tasks backup-now` - Run a backup task with all the activated accounts or sites in it.
- `cloudally-cli tasks cancel` - Cancel the specified running task (an entire backup, restore or download tasks).
- `cloudally-cli tasks cancel-entity` - Cancel the specified running task of a specified account or a site.
- `cloudally-cli tasks delete` - Delete a backup task, removing all the data.
- `cloudally-cli tasks get` - Get information for the specified task
- `cloudally-cli tasks list` - List all tasks of a specified type: backup, restore, download (paginated).
- `cloudally-cli tasks pause` - Pause execution by schedule for all accounts or sites of a backup task.
- `cloudally-cli tasks pause-entity` - Pause execution by schedule of a specified account or a site.
- `cloudally-cli tasks remove-entity` - Deactivate an account or site from a backup, removing all the coorresponding data.

**user** - Manage user

- `cloudally-cli user bills` - Get current billing forecast.
- `cloudally-cli user create-primary` - Create a Cloudally account (trial status).
- `cloudally-cli user create-secondary` - Create a Cloudally user in a Cloudally account.
- `cloudally-cli user delete` - Cancel subscription and delete a Cloudally account.
- `cloudally-cli user me` - Get Cloudally Account settings.
- `cloudally-cli user secondaries` - Get list of the Cloudally users of a Cloudally account.
- `cloudally-cli user status` - Get status for all tasks of user.
- `cloudally-cli user update-password` - Update password
- `cloudally-cli user update-permissions` - Update permission setting of a Cloudally account user.
- `cloudally-cli user update-settings` - Update Cloudally account settings.


**backup**


- `cloudally-cli backup health` - Find every mailbox or site across all customer accounts that has not backed up in N days or whose last attempt failed, with the error text.

- `cloudally-cli backup snapshots` - List every past backup run for one mailbox in a given month, with each run's date in both ISO and the raw yyyyMMddHHmmss form a restore body needs.


**restore**


- `cloudally-cli restore plan` - Preview every mailbox-and-date restore a batch would submit, with the exact backup each mailbox will be restored from.

- `cloudally-cli restore submit` - Submit one restore per mailbox per date with a local ledger and a restore guard (`restore-guard.db` in the state directory) shared by every login and token that uses that state directory: a restore already sent from it is never sent again unless you pass --new-batch. Another OS user, `--home`, `CLOUDALLY_HOME` or `CLOUDALLY_STATE_DIR` has its own guard, and `recovery restore` and the MCP `cloudally_execute` tool bypass it; --resume keeps the batch's recorded target account and restore options.

- `cloudally-cli restore watch` - Follow a submitted batch until every restore finishes, with a per-mailbox tally and a non-zero exit if any failed.

- `cloudally-cli restore history` - See what was restored for which company and when, from the local ledger, without calling the API.


**download**


- `cloudally-cli download submit` - Create one download (export) job per mailbox and sub-source for every mailbox backed up on one date, with a local ledger and a reconcile pass.

- `cloudally-cli download fetch` - Download every ready archive to disk, one folder per year, mailbox and sub-source.

- `cloudally-cli download start` - Build one archive of one mailbox (single-archive sources; MS365 goes through download submit).

- `cloudally-cli download list` - List an account's download (export) jobs.

- `cloudally-cli download watch` - Follow one download job until its archive is ready.

- `cloudally-cli download link` - Mint a fresh 60-second link for each file of a finished download job.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
cloudally-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match - fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Plan a batch restore and save it for review

```bash
cloudally-cli restore plan --task 64b0e9 --date 2026-10-01 --plan-out plan.json
```

Resolve which backup each mailbox would be restored from and save the plan for review.

### Submit the batch after reviewing the plan

```bash
cloudally-cli restore submit --task 64b0e9 --date 2026-10-01 --concurrency 3 --yes
```

Sends one restore per mailbox from the same plan restore plan showed (or pass --plan-file plan.json to submit a saved plan exactly). Without --yes nothing is sent.

### Follow a batch to completion

```bash
cloudally-cli restore watch --batch 20261015-120000-ab12 --interval 30s --agent --select rows.entity,rows.status,rows.task_id
```

Polls the account's RESTORE tasks, correlates them to the batch and prints only the columns an agent needs.

### Partner-wide health sweep (partner login)

```bash
cloudally-cli sync --resources distributor_accounts-backup-statuses && cloudally-cli backup health --stale-days 2 --group-by account
```

Needs `auth login --partner`; refreshes the statuses feed for every customer, then lists stale or failing entities grouped by customer.

### What did we restore for a customer last quarter

```bash
cloudally-cli restore history --since 2026-07-01 --csv
```

Reads this login's local ledger only, so it works offline. The partner login, each `auth use-account` customer and each company login keep separate ledgers, so run it under the login that submitted the batches.

### Check which backup a past date resolves to for one mailbox

```bash
cloudally-cli backup snapshots --account 5f1c2a --task 64b0e9 --entity jane.doe@contoso.com --month 2025-12
```

Lists that mailbox's backup runs for the month from the portal's backup calendar, so the restore point is confirmed before anything is restored.

### Plan an end-of-year restore for every mailbox on a task

```bash
cloudally-cli restore plan --account 5f1c2a --task 64b0e9 --date 2025-12-31 --plan-out eoy.json
```

Resolves each mailbox to its newest successful backup at or before 2025-12-31 from the portal's backup calendar. Mailboxes with no backup by that date are listed as NO_BACKUP and left out of the submission.

### Year-end archive of every mailbox on a task (partner login)

```bash
cloudally-cli download submit --account 5f1c2a --task 64b0e9 --on-date 2025-12-31 --utc-offset -5 --dry-run
cloudally-cli download submit --account 5f1c2a --task 64b0e9 --on-date 2025-12-31 --utc-offset -5 --yes
```

Creates one job per mailbox and sub-source for the backups taken on 31 December in the account's own timezone. A mailbox with no successful backup that day is listed as skipped, never archived from another date. Each row is saved as REQUESTED before its create is sent. A create whose result is unknown (timeout, dropped connection, server error) is UNKNOWN and is never sent again by `--resume` on its own: `--resume` first marks jobs it finds on the portal CREATED; add `--retry-unknown` only after `download list` shows they are missing. Rows that never appear on the portal are marked MISSING; re-send them with `--batch <key> --resume --yes`. The batch records `--include-deleted`, `--include-legal-hold` and `--format`, and `--resume` uses the recorded values (a different value is refused). Running the same submit again while its batch has archives created, possibly created or being sent is refused, with the `--batch <key> --resume --yes` command printed; `--new-batch` starts a second batch and creates every export again, so never pass it on an agent's own initiative. The batch key and the resume command are printed before the first create. `--timeout` limits each API and portal call, not the whole run, so the reconcile wait runs to the end. Each create carries a unique job label (`cloudally-cli <id>`, visible in the portal's job list); a row is confirmed only by the job with its label (or, if the portal drops labels, by the one job of that snapshot created after the row was requested), never by an older export of the same snapshot, and a confirmed row stays created even after the job leaves the list. Without `--new-batch`, a submit leaves out (SKIPPED) any archive another batch for the same task created or may have created. A batch from an earlier version is resumed by stating its include flags (`--resume --include-deleted=<true|false> --include-legal-hold=<true|false>`). The command exits 6 while rows are FAILED, UNKNOWN or MISSING, and 2 when it refused before sending anything.

### Collect the finished archives

```bash
cloudally-cli download fetch --account 5f1c2a --dest ./archives --year 2025 --on-date 2025-12-31
```

Create the folder first (`mkdir archives`; PowerShell: `New-Item -ItemType Directory archives`). Writes `<dest>/2025/<mailbox>/<sub-source>/<archive>.zip` with a small `.cloudally.json` record beside each archive. Re-runs skip an archive only when its record matches, so it can be re-run until every job is collected; existing files are never overwritten, and a name held by another file gets the export id added. Folders and files are readable only by the current user on macOS and Linux; on Windows they inherit the permissions of the `--dest` folder, so choose a private one. It exits 6 if any archive FAILED or is MISSING (including a body of the wrong size, an error page, or a link whose host resolves to a private address), 8 when nothing failed but some jobs are still NOT_READY (run the same command later), and 3 when no download job matched at all. With `--batch` it downloads only the jobs the batch confirmed; confirm the rest with `download submit --batch <key> --resume --yes`, which sends nothing that may already exist. `download watch` exits 0 when the job is ready, 6 when it failed or was cancelled, and 8 with `--once` while it is still being built. Links to single-label hosts or `.local`, `.internal`, `.lan`, `.home.arpa`, `.localhost` and `.corp` names are refused. An archive streams as long as data keeps arriving; `--timeout` does not cut it off. `.partial` files left by a killed run are removed once they are an hour old. When several fetches run for the same customer, start them a few seconds apart.

## How CloudAlly behaves (read before restoring or downloading)

These were observed against a production CloudAlly partner tenant, not taken from the API reference. They decide which command to use.

- **The documented API knows only the latest backup.** `tasks get` and the status feeds report each mailbox's most recent backup per sub-source. Every past backup date comes from the customer portal's backup calendar (`getSnapshotsCalendar`), which `backup snapshots`, `restore plan`, `download start` and `download submit` read for you. The portal is reached through the partner handoff: the partner token calls `POST /v2/distributors/accounts/{id}/partner-access-token`, which returns a single-use opaque token that is spent at `app.cloudally.com/newapp?accessToken=`. So any past-date read needs `auth login --partner` and `--account <id>`.
- **`extid` is mandatory for the calendar.** Without the mailbox's external id the calendar answers success with no rows, which reads as "no backups". The CLI looks it up from the task; pass `backup snapshots --ext-id` only when the task record lacks it.
- **Snapshot timestamps are UTC.** A nightly backup that runs late in the evening local time is stamped the next UTC day, so the run stamped `2022-12-31` for a US Eastern account holds the 30 December state. `download submit --utc-offset -5` selects by the account's own calendar day; leaving it at 0 matches the UTC date, which is usually the day before the one intended.
- **The documented export endpoint does nothing.** `POST .../recovery/export` (the generated `distributor-recovery export`, `reseller-recovery export` and `recovery export` commands) answers 200 with an empty body and creates no job. Downloads are created the way the portal creates them, through `download start` (single-archive sources) and `download submit` (MS365, any number of mailboxes).
- **An MS365 mailbox is one download job per sub-source.** Mail, calendar, contacts, tasks and notes (`O365MAIL`, `O365CALENDAR`, `O365CONTACTS`, `O365TASKS`, `O365NOTES`) are separate jobs, each asked for with that sub-source's own timestamp. A job for the parent `MS365` source is accepted and creates nothing, which is why `download start` refuses MS365 tasks and points at `download submit`.
- **Download links last 60 seconds.** A link is a presigned object-storage URL. `download fetch` mints and streams each one in the same step. Never paste a link into a ticket or chat; it is dead within a minute and is a credential until then. Customers can collect finished downloads themselves in the CloudAlly portal (their account name > Jobs > Download results).
- **Finished download jobs cannot be deleted.** `tasks delete` (and the distributor and reseller variants) is `DELETE .../backup-tasks/{id}`: it deletes a backup task together with its backed-up data. Never point it at a download job id.
- **A restore is sent once per state directory, and an agent never re-sends one.** Restores write into live mailboxes. Run `restore plan` first (or `restore submit` without `--yes`, which lists each mailbox and the backup it resolves to and sends nothing; `--dry-run` without `--plan-file` prints placeholder bodies only), show the human the plan, and pass `--yes` only after they confirm the account, task, mailboxes and dates. `restore submit` claims each restore in a guard (`restore-guard.db` in the state directory, shared by every login and token that uses it) and refuses a run that would send one again. Another OS user, `--home`, `CLOUDALLY_HOME` or `CLOUDALLY_STATE_DIR` has its own guard, and `recovery restore` and the MCP `cloudally_execute` tool bypass it, so run restores from one account and one state directory. The guard also refuses restores recorded by any older ledger in the data directory, and any restore of the same task and mailbox within 30 minutes of one already sent. Never pass `--new-batch` on your own initiative: it restores those mailboxes a second time. If `restore submit` reports UNKNOWN rows, run only `restore watch --batch <id> --once` and show the human the result; the printed `--resume --retry-unknown --yes` command is theirs to run, and only for rows watch still finds no RESTORE task for. `restore history` shows the ledger of the login you are on (partner, each `auth use-account` customer and each company login keep their own). `restore submit` exits 0 only when every row was accepted, 2 when it refused before sending, 4 on a 401/403, and 5 when any row failed, is UNKNOWN or was never sent (for example after Ctrl-C), printing the exact `--resume` command; `restore watch` exits 0 only when every restore finished and 5 while one failed, is UNKNOWN or was never submitted. `--timeout` bounds each API or portal request, never a whole submit or watch; `restore watch --max-wait` (default 2h) bounds the watch.
- **One portal session per customer at a time.** Starting several portal sessions for the same customer in the same instant can invalidate each other's unspent handoff tokens (seen as `portal session is not authenticated (status 401)`). When running several `download fetch` or `download submit` processes for one customer, start them a few seconds apart (about 6 seconds was enough).

## Auth Setup

CloudAlly needs three things on every request: a client-id header, a client-secret header (the API key pair created under Integrations > REST API in the partner portal) and a Bearer access token. The key pair identifies the integration; what the CLI can reach depends only on which login you use. Set CLOUDALLY_CLIENT_ID and CLOUDALLY_CLIENT_SECRET, then set CLOUDALLY_EMAIL and CLOUDALLY_PASSWORD to one company's CloudAlly account and run `cloudally-cli auth login`: that is the default, and every command is limited to that one company. For partner-wide work (backup health across customers, switching between customers with `auth use-account <accountId>`), set CLOUDALLY_PARTNER_EMAIL and CLOUDALLY_PARTNER_PASSWORD and run `cloudally-cli auth login --partner`; that login can see and change every customer account. Tokens are stored in the credentials file and refreshed automatically. Past backup dates, downloads and download links come from the customer portal, reached through the partner handoff, so those commands need the partner login and `--account <id>`.

Run `cloudally-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json` - one JSON document on stdout (sync progress events go to stderr)
- `--compact` - keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain` - tabular rows (collection envelopes unwrap to the row array)
- `--quiet` - one identity value per row, no envelope

- **Pipeable** - JSON on stdout, errors on stderr
- **Filterable** - `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  cloudally-cli tasks list --agent --select id,source,status
  ```
- **Previewable** - `--dry-run` shows the request without sending
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Non-interactive** - never prompts, every input is a flag
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Explicit retries** - use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set - piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `CLOUDALLY_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `CLOUDALLY_CONFIG_DIR`, `CLOUDALLY_DATA_DIR`, `CLOUDALLY_STATE_DIR`, `CLOUDALLY_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `CLOUDALLY_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, the local store (`data-<hash>.db`, one per CloudAlly user and API URL), cookies, and auth sidecars. `state` contains persisted queries, jobs, `teach.log`, `restore-guard.db` (the restore guard, shared by every login and token that uses this state directory), download batch ledgers, and `auth-context.json`, which every `auth login` writes and which, after `auth login --partner`, also holds the partner access and refresh token; `auth logout` deletes it. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `cloudally-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "cloudally": {
        "command": "cloudally-mcp",
        "env": {
          "CLOUDALLY_HOME": "/srv/cloudally"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `CLOUDALLY_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `CLOUDALLY_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, pass the question as an argv or MCP tool argument to `recall --agent`. Do not interpolate user-controlled text into a shell command line.

Quoted `recall "<question>"` breaks on an apostrophe, which is ordinary English. A quoted heredoc breaks when a body line equals the delimiter, and that delimiter is published in these docs. Write the question with a non-shell file-writing tool, then read it back as data:

```bash
# Write the question verbatim with your file-writing tool (no shell involved).
# Command substitution on a file only ever yields data - the shell never
# parses the file's bytes as syntax.
QUERY=$(cat /path/to/question.txt)
cloudally-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "cloudally-cli learnings confirm 12"] }
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
       materially more, record the divergence via `cloudally-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `cloudally-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities - direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `cloudally-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall - argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
cloudally-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
cloudally-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
cloudally-cli teach-playbook \
  --query "$QUERY" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know - a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback - fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`. Pass the query and note as argv/MCP arguments, or write each with a non-shell file tool and read them back (`QUERY=$(cat ...)`, `NOTE=$(cat ...)`). Do not interpolate either string into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
NOTE=$(cat /path/to/note.txt)
cloudally-cli playbook amend \
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

`cloudally-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `CLOUDALLY_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
cloudally-cli feedback "the --since flag is inclusive but docs say exclusive"
cloudally-cli feedback --stdin < notes.txt
cloudally-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `CLOUDALLY_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CLOUDALLY_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
cloudally-cli profile save briefing --json
cloudally-cli --profile briefing tasks list
cloudally-cli profile list --json
cloudally-cli profile show briefing
cloudally-cli profile delete briefing --yes
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

If `--wait` times out or polling fails after the job was accepted, the command exits `8` and prints the job ID plus a recovery command that fetches the result. The job may still be running and may already be billed, so run the recovery command instead of resubmitting.

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
| 8 | Async job submitted but not finished (run the printed recovery command; do not resubmit) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `cloudally-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add cloudally-mcp --env CLOUDALLY_CLIENT_ID="<your client id>" --env CLOUDALLY_CLIENT_SECRET="<your client secret>" -- cloudally-mcp
   ```
3. Run `cloudally-cli auth login --partner` (or `auth login` for one company) once in a terminal; the MCP server reads the saved token and refreshes it.
   Through MCP, `restore_submit`, `download_submit` and `download_start` only preview (the server exposes no `--yes`), and `download fetch` is CLI-only: review the plan, then submit from a terminal.
4. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which cloudally-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above, and read "How CloudAlly behaves" before any restore or download.
3. Execute with the `--agent` flag:
   ```bash
   cloudally-cli backup health --agent
   ```
4. If ambiguous, drill into subcommand help, for example `cloudally-cli download --help`.
