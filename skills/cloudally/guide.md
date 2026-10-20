# CloudAlly CLI

**Restore or download every CloudAlly mailbox as it stood on a past date, and find failing backups across every customer.**

Drives CloudAlly backups of Microsoft 365 and Google Workspace for one company account or, with a partner login, every customer. The restore group plans, submits, watches and remembers multi-mailbox restores to a chosen backup date; the download group builds and collects year-end mailbox archives; `backup snapshots` lists a mailbox's past backups, which the documented API cannot; and `backup health` sweeps every customer for stale or failing backups.

Learn more at [CloudAlly](https://www.cloudally.com).

Created by [@sonofskywalker3](https://github.com/sonofskywalker3) (Jeff Moore).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `cloudally-cli` and `cloudally-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=cloudally). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`. On Windows, run `Unblock-File <binary>` on the downloaded .exe and put it in `%LOCALAPPDATA%\Programs\msp-skills`, the folder the installer adds to PATH.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/cloudally --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/cloudally --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `cloudally-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the cloudally skill from https://github.com/Servosity/msp-skills/tree/main/skills/cloudally. The skill defines how its required CLI (`cloudally-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle - Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=cloudally).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. The extension runs `cloudally-cli` for its tools, so also install the CLI with the installer above (Windows: `install.ps1`).
4. Fill in `CLOUDALLY_CLIENT_ID` and `CLOUDALLY_CLIENT_SECRET` when Claude Desktop prompts you. Leave `CLOUDALLY_ACCESS_TOKEN` empty: run `cloudally-cli auth login --partner` (or `auth login` for one company) in a terminal once instead. The MCP server reads the saved token and refreshes it.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cloudally/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cloudally/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`%APPDATA%\Claude\claude_desktop_config.json` on Windows, `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "cloudally": {
      "command": "cloudally-mcp",
      "env": {
        "CLOUDALLY_CLIENT_ID": "<your CloudAlly API client id>",
        "CLOUDALLY_CLIENT_SECRET": "<your CloudAlly API client secret>"
      }
    }
  }
}
```

</details>

## Authentication

CloudAlly needs three things on every request: a client-id header, a client-secret header (the API key pair created under Integrations > REST API in the partner portal) and a Bearer access token. The key pair identifies the integration; what the CLI can reach depends only on which login you use. Set CLOUDALLY_CLIENT_ID and CLOUDALLY_CLIENT_SECRET, then set CLOUDALLY_EMAIL and CLOUDALLY_PASSWORD to one company's CloudAlly account and run `cloudally-cli auth login`: that is the default, and every command is limited to that one company. For partner-wide work (backup health across customers, switching between customers with `auth use-account <accountId>`), set CLOUDALLY_PARTNER_EMAIL and CLOUDALLY_PARTNER_PASSWORD and run `cloudally-cli auth login --partner`; that login can see and change every customer account. Tokens are stored in the credentials file and refreshed automatically. Past backup dates, downloads and download links come from the customer portal, which the CLI reaches through the partner handoff, so those commands need the partner login and `--account <id>`.

## Quick Start

```bash
# Confirm the client headers and token are configured and the API is reachable
cloudally-cli doctor

# List the company's Microsoft 365 backup tasks to get the task id
cloudally-cli tasks list --source MS365 --agent

# See every mailbox in that task and its backup status
cloudally-cli tasks get 64b0e9 --agent --select entity,status

# Preview which backup each mailbox would be restored from
# (dates before the latest backup need auth login --partner and --account <id>)
cloudally-cli restore plan --task 64b0e9 --date 2026-10-01

# Preview each mailbox and the backup it would be restored from; without --yes nothing is sent (exit 2)
cloudally-cli restore submit --task 64b0e9 --date 2026-10-01

```

## Unique Features

### Batch restore by date
- **`restore plan`** - Preview every mailbox-and-date restore a batch would submit, with the exact backup each mailbox will be restored from.

  _Reach for this before any multi-mailbox restore so the operator can review what will be restored and from which backup._

  ```bash
  cloudally-cli restore plan --task 64b0e9 --date 2026-10-01 --date 2026-10-03 --agent
  ```
- **`restore submit`** - Submit one restore per mailbox per date with a local ledger and a restore guard (`restore-guard.db` in the state directory) shared by every login and token that uses that state directory: a restore already sent from it is never sent again unless you pass --new-batch. Another OS user, `--home`, `CLOUDALLY_HOME` or `CLOUDALLY_STATE_DIR` has its own guard, and `recovery restore` and the MCP `cloudally_execute` tool bypass it; --resume keeps the batch's recorded target account and restore options.

  _Use this for every restore, a single mailbox included (`--entities <mailbox>`); `recovery restore` skips the plan, ledger, guard and confirmation._

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

  _Use this for year-end or legal-hold archives of many mailboxes, and for any MS365 mailbox. Pass `--utc-offset` so `--on-date` means the account's own calendar day, and run it once without `--yes` first: that prints the plan and creates nothing (exit 2; exit 0 with nothing to create when every mailbox was skipped, so read the skipped list before calling the set complete)._

  ```bash
  cloudally-cli download submit --account 5f1c2a --task 64b0e9 --on-date 2025-12-31 --utc-offset -5
  ```
- **`download fetch`** - Download every ready archive to disk, one folder per year, mailbox and sub-source, minting and streaming each 60-second link in one step and resuming safely after an interruption.

  _Use this to collect the archives `download submit` created; `--plan-only` lists what it would write. `--dest` must already exist, even with `--plan-only` (`mkdir archives`; PowerShell: `New-Item -ItemType Directory archives`)._

  ```bash
  cloudally-cli download fetch --account 5f1c2a --dest ./archives --year 2025 --batch 20261015-201500-ab12 --plan-only
  ```
- **`download start`** - Build one archive of one mailbox from the backup taken on or before a date, for single-archive sources (MS365 tasks go through `download submit`). It runs as a one-archive batch through the download guard, so running it again creates nothing; confirm the job with the printed `download submit --batch <key> --resume --yes`.

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

Windows PowerShell 5.1 has no `&&`; run the second command only when the first succeeded:

```powershell
cloudally-cli sync --resources distributor_accounts-backup-statuses
if ($LASTEXITCODE -eq 0) {
  cloudally-cli backup health --stale-days 2 --group-by account
}
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
cloudally-cli download submit --account 5f1c2a --task 64b0e9 --on-date 2025-12-31 --utc-offset -5
cloudally-cli download submit --account 5f1c2a --task 64b0e9 --on-date 2025-12-31 --utc-offset -5 --yes
```

Creates one job per mailbox and sub-source for the backups taken on 31 December in the account's own timezone. A mailbox with no successful backup that day is listed as skipped, never archived from another date. Each row is saved as REQUESTED before its create is sent. A create whose result is unknown (timeout, dropped connection, server error) is UNKNOWN and is never sent again by `--resume` on its own: `--resume` first marks jobs it finds on the portal CREATED; add `--retry-unknown` only after `download list` shows they are missing. Rows that never appear on the portal are marked MISSING; re-send them with `--batch <key> --resume --yes`. The batch records `--include-deleted`, `--include-legal-hold` and `--format`, and `--resume` uses the recorded values (a different value is refused). Running the same submit again while its batch has archives created, possibly created or being sent is refused, with the `--batch <key> --resume --yes` command printed (a run with a different `--utc-offset` is a different submit, since it selects another day's backups); `--new-batch` starts a second batch and creates every export again, so never pass it on an agent's own initiative. The batch key and the resume command are printed before the first create. `--timeout` limits each API and portal call, not the whole run, so the reconcile wait runs to the end. Each create carries a unique job label (`cloudally-cli <id>`, visible in the portal's job list); a row is confirmed only by the job with its label (or, if the portal drops labels, by the one job of that snapshot created after the row was requested), never by an older export of the same snapshot, and a confirmed row stays created even after the job leaves the list. `download submit` claims every archive (account, task, mailbox, sub-source, snapshot, format, include flags) in one download guard (`download-guard.db` in the state directory) just before creating it, so no two batches or runs create the same archive; a zip and a PST, or the same snapshot with and without legal-hold items, are different archives. A new batch creates only the archives no other batch has created and lists the rest as SKIPPED (this batch's `download fetch --batch` downloads them through the holding batch's confirmed jobs), so a pilot mailbox followed by the whole task creates only the rest and one fetch collects all of it; it is refused only while another batch's create of one of its archives is still unsettled, with that batch's resume command. Batches recorded before this version are imported into the guard automatically. A row whose job is not found is EMPTY (an empty mailbox) only when this run sent it and the portal accepted it, the portal's job count grew by exactly the creates this run had accepted, and nothing else in this state directory (another download or restore, or a create whose answer was lost) could have added a job; otherwise it is MISSING. When a confirmed job ends in failure (any status naming one: FAILED, CANCELLED, ABORTED, STOPPED, ERROR), `download fetch` (or the next resume) marks it JOB_FAILED and `download submit --batch <key> --resume --yes` creates that archive exactly once more. If a job of the same mailbox, sub-source and snapshot is listed but cannot be proved to be the batch's (its label was lost or edited), the row is held as ambiguous and never re-created: collect it with the printed `download fetch --on-date ... --entities ...` exactly as printed (its `--on-date` is the snapshot's UTC day, so do not add `--utc-offset`) and check it is the export wanted. A batch from an earlier version is resumed by stating its include flags (`--resume --include-deleted=<true|false> --include-legal-hold=<true|false>`). The command exits 0 when every row is created, empty or held by another batch, 6 while rows are FAILED, MISSING, JOB_FAILED, UNKNOWN, ambiguous or REQUESTED and not yet confirmed, and 2 when it refused before sending anything; an interrupted reconcile wait exits 6 and prints the `--batch <key> --resume --yes` command, which confirms what was created and sends nothing that may already exist. Any other non-zero exit (1 when the portal session, ledger or guard could not be opened or saved, 3 or 4 from the task read) means stop and read the message; if a batch key was printed, continue only with `download submit --batch <key> --resume --yes`. A create the portal accepted whose job is still not listed an hour later becomes MISSING, and the next `--resume` sends it again. The job list never shows an export that finished empty, so if the same rows come back MISSING after a resend, stop resending, check `download list`, and leave the next step to a person. `download submit --resume` and `download fetch --batch` refuse (exit 2) an `--account` other than the batch's own. When the portal's job listing stops at its cap or cannot be read, nothing is concluded from a row's absence: no row becomes MISSING or EMPTY, and MISSING and UNKNOWN rows are not sent again in that run.

### Collect the finished archives

```bash
cloudally-cli download fetch --account 5f1c2a --dest ./archives --year 2025 --batch 20261015-201500-ab12
```

Use the batch key `download submit` printed. Collect by `--batch`, not `--on-date`: fetch reads `--on-date` as the UTC day of the backup unless it is given the submit's `--utc-offset`, so without it fetch would miss the batch's archives or pick up another day's export. Create the folder first (`mkdir archives`; PowerShell: `New-Item -ItemType Directory archives`). Writes `<dest>/2025/<mailbox>/<sub-source>/<archive>.zip` with a small `.cloudally.json` record beside each archive. Re-runs skip an archive only when its record matches, so it can be re-run until every job is collected; existing files are never overwritten, and a name held by another file gets the export id added. Folders and files are readable only by the current user on macOS and Linux; on Windows they inherit the permissions of the `--dest` folder, so choose a private one. It exits 6 if any archive FAILED, is MISSING (including a body of the wrong size, an error page, or a link whose host resolves to a private address) or is UNLISTED (the portal's job listing stopped at its cap of 5,000 jobs, so the archive may still exist; nothing is suggested to create it), 8 when nothing failed but some jobs are still NOT_READY (run the same command later), and 3 when no download job matched at all. If the portal has already expired a confirmed archive, fetch says so and prints a `--new-batch` command; check `download list` first, and leave that decision to a person. With `--batch` it downloads only the jobs the batch confirmed, plus the archives other batches hold for it (through their confirmed jobs, matched by job id); confirm the rest with `download submit --batch <key> --resume --yes`; it never resends an UNKNOWN row, but a row the portal accepted over an hour ago and still does not list is MISSING and is sent again. `download watch` exits 0 when the job is READY_FOR_DOWNLOAD, 6 when it finished any other way (failed, cancelled, aborted, stopped or partly done), 3 when the job id is not on the account, and 8 with `--once` while it is still being built or when interrupted (Ctrl-C, or closing the terminal on macOS and Linux), printing the command to watch it again. Links to single-label hosts or `.local`, `.internal`, `.lan`, `.home.arpa`, `.localhost` and `.corp` names are refused. An archive streams as long as data keeps arriving; `--timeout` does not cut it off. `.partial` files left by a killed run are removed once they are an hour old. When several fetches run for the same customer, start them a few seconds apart.

## How CloudAlly behaves (read before restoring or downloading)

These were observed against a production CloudAlly partner tenant, not taken from the API reference. They decide which command to use.

- **The documented API knows only the latest backup.** `tasks get` and the status feeds report each mailbox's most recent backup per sub-source. Every past backup date comes from the customer portal's backup calendar (`getSnapshotsCalendar`), which `backup snapshots`, `restore plan`, `download start` and `download submit` read for you. The portal is reached through the partner handoff: the partner token calls `POST /v2/distributors/accounts/{id}/partner-access-token`, which returns a single-use opaque token that is spent at `app.cloudally.com/newapp?accessToken=`. So any past-date read needs `auth login --partner` and `--account <id>`.
- **`extid` is mandatory for the calendar.** Without the mailbox's external id the calendar answers success with no rows, which reads as "no backups". The CLI looks it up from the task; pass `backup snapshots --ext-id` only when the task record lacks it.
- **Snapshot timestamps are UTC.** A nightly backup that runs late in the evening local time is stamped the next UTC day, so the run stamped `2022-12-31` for a US Eastern account holds the 30 December state. `download submit --utc-offset -5` selects by the account's own calendar day; leaving it at 0 matches the UTC date, which is usually the day before the one intended.
- **The documented export endpoint does nothing.** `POST .../recovery/export` (the generated `distributor-recovery export`, `reseller-recovery export` and `recovery export` commands) answers 200 with an empty body and creates no job. Downloads are created the way the portal creates them, through `download start` (single-archive sources) and `download submit` (MS365, any number of mailboxes).
- **An MS365 mailbox is one download job per sub-source.** Mail, calendar, contacts, tasks and notes (`O365MAIL`, `O365CALENDAR`, `O365CONTACTS`, `O365TASKS`, `O365NOTES`) are separate jobs, each asked for with that sub-source's own timestamp. A job for the parent `MS365` source is accepted and creates nothing, which is why `download start` refuses MS365 tasks and points at `download submit`.
- **Download links last 60 seconds.** A link is a presigned object-storage URL. `download fetch` mints and streams each one in the same step. Never paste a link into a ticket or chat; it is dead within a minute and is a credential until then. Customers can collect finished downloads themselves in the CloudAlly portal (their account name > Jobs > Download results).
- **Finished download jobs cannot be deleted.** `tasks delete` (and the distributor and reseller variants) is `DELETE .../backup-tasks/{id}`: it deletes a backup task together with its backed-up data. Never point it at a download job id.
- **A restore is sent once per state directory, and an agent never re-sends one.** Restores write into live mailboxes. Run `restore plan` first (or `restore submit` without `--yes`, which lists each mailbox and the backup it resolves to and sends nothing; `--dry-run` without `--plan-file` prints placeholder bodies only), show the human the plan, and pass `--yes` only after they confirm the account, task, mailboxes and dates. `restore submit` claims each restore in a guard (`restore-guard.db` in the state directory, shared by every login and token that uses it) and refuses a run that would send one again. Another OS user, `--home`, `CLOUDALLY_HOME` or `CLOUDALLY_STATE_DIR` has its own guard, and `recovery restore` and the MCP `cloudally_execute` tool bypass it, so run restores from one account and one state directory. The guard also refuses restores recorded by any older ledger in the data directory, and any restore of the same task and mailbox whose resolved backup is within 30 minutes of one already sent (the same backup reported two ways). Never pass `--new-batch` on your own initiative: it restores those mailboxes a second time. If `restore submit` reports UNKNOWN rows, run only `restore watch --batch <id> --once` and show the human the result; the printed `--resume --retry-unknown --yes` command is theirs to run, and only for rows watch still finds no RESTORE task for. `restore history` shows the ledger of the login you are on (partner, each `auth use-account` customer and each company login keep their own). `restore submit` exits 0 only when every row was accepted, 1 when the ledger or guard cannot be read (nothing sent), 2 when it refused before sending, 3 when no mailbox has a backup at or before the date or the task id is not found, 4 on a 401/403, 10 when `auth-context.json` cannot be trusted (nothing sent), and 5 when any row failed, is UNKNOWN or was never sent (for example after Ctrl-C), printing the exact `--resume` command, or when reading the task failed before anything was sent; `restore watch` exits 0 only when every row was submitted and its RESTORE task reports success (COMPLETED, COMPLETE, SUCCEEDED, SUCCEED, SUCCESS, DONE or FINISHED); 5 means something needs a person (a restore failed, was cancelled or only partly succeeded, a task reports a status the CLI does not recognise (printed as received), a row is still UNKNOWN, a row was never submitted, or a restore was accepted but no RESTORE task ever appeared: check `tasks list --type RESTORE` and do not resend); 8 means restores are still running, scheduled or paused (`--once`, `--max-wait` or Ctrl-C), so run watch again later. A `--retry-unknown` resend that is refused leaves the row UNKNOWN, because the first send may still have been accepted. `--timeout` bounds each API or portal request in `restore plan`, `submit` and `watch`, never the whole run; `restore watch --max-wait` (default 2h) bounds the watch. An interrupted submit (Ctrl-C or a tool timeout) says how many restores were already accepted and prints the `--resume` command; never report it as nothing restored.
- **One portal session per customer at a time.** Starting several portal sessions for the same customer in the same instant can invalidate each other's unspent handoff tokens (seen as `portal session is not authenticated (status 401)`). When running several `download fetch` or `download submit` processes for one customer, start them a few seconds apart (about 6 seconds was enough).

## Usage

Run `cloudally-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, the local store (`data-<hash>.db`, one per CloudAlly user and API URL; `data-nocredential.db` for runs with no credential), cookies, browser-session proof files, and other auth sidecars. `auth-context.json` (every `auth login` writes it; after `auth login --partner` it holds the partner access and refresh token), `restore-guard.db`, `download-guard.db` and the download batch ledgers are in the `state` dir; `auth logout` deletes `auth-context.json` |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `CLOUDALLY_CONFIG_DIR`, `CLOUDALLY_DATA_DIR`, `CLOUDALLY_STATE_DIR`, or `CLOUDALLY_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `CLOUDALLY_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export CLOUDALLY_HOME=/srv/cloudally
cloudally-cli doctor
```

Windows (PowerShell): `$env:CLOUDALLY_HOME = "C:\cloudally"`

### Local data per CloudAlly user

Synced rows, restore history and resume are kept per CloudAlly user, not per token or API key:

- Each partner user has their own store, named by their login email, so an administrator and a restricted user of one partner never see each other's cached data.
- `auth use-account <id>` gives each customer account its own store per partner user; each company login has its own store.
- Token refreshes, signing in again and rotating the API key pair keep the same store for the same user and API URL; changing `CLOUDALLY_BASE_URL` or signing in as another user starts a new one.
- A token from `CLOUDALLY_ACCESS_TOKEN` or `auth set-token` gets a store of its own, so history from your normal login is not visible there. The restore guard (`restore-guard.db`) and the download guard (`download-guard.db`) still span every login and token that uses this state directory.
- If `auth-context.json` is refused (for example other users can read it), cannot be read, or belongs to another session, `restore submit`, `restore watch` and `restore history` stop with an error instead of showing empty history. Fix its permissions (`chmod 600 <file>` on macOS and Linux), or on any OS delete the file and sign in again (`auth login`, or `auth login --partner` then `auth use-account <id>`).

Under `CLOUDALLY_HOME=/srv/cloudally`, the four dirs resolve to `/srv/cloudally/config`, `/srv/cloudally/data`, `/srv/cloudally/state`, and `/srv/cloudally/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `CLOUDALLY_DATA_DIR` overrides an explicit `--home` for that kind. Use `CLOUDALLY_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `CLOUDALLY_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `cloudally-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### distributor

Manage distributor

- **`cloudally-cli distributor bills`** - bills
- **`cloudally-cli distributor create-user`** - Create a secondary partner under a specific reseller. Use role='administrator' for full permissions or role='custom' with a permissions block. Set accessType='all' for access to all accounts, or 'custom' with accountIds for specific accounts.
- **`cloudally-cli distributor delete-user`** - Delete a secondary user from a partner.
- **`cloudally-cli distributor details`** - Returns partner details

### distributor_account_tasks

Manage distributor account tasks

- **`cloudally-cli distributor-account-tasks activate`** - Activate a backup task to run on schedule, if it was paused.
- **`cloudally-cli distributor-account-tasks activate-all`** - Activate all accounts in a task for a backup.
- **`cloudally-cli distributor-account-tasks activate-entity`** - Activate a specific account in a task for a backup.
- **`cloudally-cli distributor-account-tasks add-entity`** - Manually add a site or account for a backup in a backup task.
- **`cloudally-cli distributor-account-tasks backup-entity-now`** - Run a backup for the specified account or site.
- **`cloudally-cli distributor-account-tasks backup-now`** - Run a backup task with all the activated accounts or sites in it.
- **`cloudally-cli distributor-account-tasks cancel`** - Cancel the specified running task (an entire backup, restore or download tasks).
- **`cloudally-cli distributor-account-tasks cancel-entity`** - Cancel the specified running task of a specified account or a site.
- **`cloudally-cli distributor-account-tasks delete`** - Delete a backup task, removing all the data.
- **`cloudally-cli distributor-account-tasks get`** - Get information for the specified task
- **`cloudally-cli distributor-account-tasks list`** - List all tasks of a specified type: backup, restore, download (paginated).
- **`cloudally-cli distributor-account-tasks pause`** - Pause execution by schedule for all accounts or sites of a backup task.
- **`cloudally-cli distributor-account-tasks pause-entity`** - Pause execution by schedule of a specified account or a site.
- **`cloudally-cli distributor-account-tasks remove-entity`** - Deactivate an account or site from a backup, removing all the coorresponding data.

### distributor_accounts

Manage distributor accounts

- **`cloudally-cli distributor-accounts access-token`** - Access token
- **`cloudally-cli distributor-accounts backup-statuses`** - Get all active backup statuses.
- **`cloudally-cli distributor-accounts create`** - Create a Cloudally account (trial status).
- **`cloudally-cli distributor-accounts list`** - Get all users belonging to the partner.
- **`cloudally-cli distributor-accounts sign-in-token`** - Sign in token
- **`cloudally-cli distributor-accounts tasks`** - tasks
- **`cloudally-cli distributor-accounts unsubscribe`** - Cancel subscription and delete a Cloudally account.

### distributor_recovery

Manage distributor recovery

- **`cloudally-cli distributor-recovery export`** - The documented export call. On the live API it answers 200 and creates no job; use `download submit` or `download start` instead.
- **`cloudally-cli distributor-recovery restore`** - Create a restore job for one entity (mailbox, site, drive) of a backup task from a specific backup date. One call restores one entity at once, with no plan, ledger, restore guard or confirmation (human only); for any restore an agent runs, use `restore plan` then `restore submit --entities <mailbox>`, never a loop of this command. It writes back into the customer's live mailbox, site or drive.

### distributor_resellers

Manage distributor resellers

- **`cloudally-cli distributor-resellers accounts`** - accounts
- **`cloudally-cli distributor-resellers all-accounts`** - All accounts
- **`cloudally-cli distributor-resellers all-backup-statuses`** - Get backup status for all resellers
- **`cloudally-cli distributor-resellers all-bills`** - Get bills for all resellers
- **`cloudally-cli distributor-resellers all-tasks`** - Get backup tasks for all resellers
- **`cloudally-cli distributor-resellers backup-statuses`** - Get all active backup statuses.
- **`cloudally-cli distributor-resellers bills`** - bills
- **`cloudally-cli distributor-resellers create`** - Create a reseller
- **`cloudally-cli distributor-resellers create-account`** - Create a Cloudally account (trial status).
- **`cloudally-cli distributor-resellers create-user`** - Create a secondary user under a specific reseller. Use role='administrator' for full permissions or role='custom' with a permissions block. Set accessType='all' for access to all accounts, or 'custom' with accountIds for specific accounts.
- **`cloudally-cli distributor-resellers delete-user`** - Delete a secondary user from a reseller.
- **`cloudally-cli distributor-resellers get`** - Returns reseller by partnerId
- **`cloudally-cli distributor-resellers list`** - Returns list of current distributor's resellers
- **`cloudally-cli distributor-resellers tasks`** - tasks
- **`cloudally-cli distributor-resellers unsubscribe-account`** - Cancel subscription and delete a Cloudally account.
- **`cloudally-cli distributor-resellers update`** - update a reseller

### recovery

Manage recovery

- **`cloudally-cli recovery export`** - The documented export call. On the live API it answers 200 and creates no job; use `download submit` or `download start` instead.
- **`cloudally-cli recovery restore`** - Create a restore job for one entity (mailbox, site, drive) of a backup task from a specific backup date. One call restores one entity at once, with no plan, ledger, restore guard or confirmation (human only); for any restore an agent runs, use `restore plan` then `restore submit --entities <mailbox>`, never a loop of this command. It writes back into the customer's live mailbox, site or drive.

### reseller

Manage reseller

- **`cloudally-cli reseller bills`** - bills
- **`cloudally-cli reseller create-user`** - Create a secondary user under the reseller. Use role='administrator' for full permissions or role='custom' with a permissions block. Set accessType='all' for access to all accounts, or 'custom' with accountIds for specific accounts.
- **`cloudally-cli reseller delete-user`** - Delete a secondary user from a reseller.
- **`cloudally-cli reseller details`** - Returns partner details

### reseller_account_tasks

Manage reseller account tasks

- **`cloudally-cli reseller-account-tasks activate`** - Activate a backup task to run on schedule, if it was paused.
- **`cloudally-cli reseller-account-tasks activate-all`** - Activate all accounts in a task for a backup.
- **`cloudally-cli reseller-account-tasks activate-entity`** - Activate a specific account in a task for a backup.
- **`cloudally-cli reseller-account-tasks add-entity`** - Manually add a site or account for a backup in a backup task.
- **`cloudally-cli reseller-account-tasks backup-entity-now`** - Run a backup for the specified account or site.
- **`cloudally-cli reseller-account-tasks backup-now`** - Run a backup task with all the activated accounts or sites in it.
- **`cloudally-cli reseller-account-tasks cancel`** - Cancel the specified running task (an entire backup, restore or download tasks).
- **`cloudally-cli reseller-account-tasks cancel-entity`** - Cancel the specified running task of a specified account or a site.
- **`cloudally-cli reseller-account-tasks delete`** - Delete a backup task, removing all the data.
- **`cloudally-cli reseller-account-tasks get`** - Get information for the specified task
- **`cloudally-cli reseller-account-tasks list`** - List all tasks of a specified type: backup, restore, download (paginated).
- **`cloudally-cli reseller-account-tasks pause`** - Pause execution by schedule for all accounts or sites of a backup task.
- **`cloudally-cli reseller-account-tasks pause-entity`** - Pause execution by schedule of a specified account or a site.
- **`cloudally-cli reseller-account-tasks remove-entity`** - Deactivate an account or site from a backup, removing all the coorresponding data.

### reseller_accounts

Manage reseller accounts

- **`cloudally-cli reseller-accounts access-token`** - Access token
- **`cloudally-cli reseller-accounts backup-statuses`** - Get all active backup statuses.
- **`cloudally-cli reseller-accounts create`** - Create a Cloudally account (trial status).
- **`cloudally-cli reseller-accounts list`** - Get all users belonging to the partner.
- **`cloudally-cli reseller-accounts sign-in-token`** - Sign in token
- **`cloudally-cli reseller-accounts tasks`** - tasks
- **`cloudally-cli reseller-accounts unsubscribe`** - Cancel subscription and delete a Cloudally account.

### reseller_recovery

Manage reseller recovery

- **`cloudally-cli reseller-recovery export`** - The documented export call. On the live API it answers 200 and creates no job; use `download submit` or `download start` instead.
- **`cloudally-cli reseller-recovery restore`** - Create a restore job for one entity (mailbox, site, drive) of a backup task from a specific backup date. One call restores one entity at once, with no plan, ledger, restore guard or confirmation (human only); for any restore an agent runs, use `restore plan` then `restore submit --entities <mailbox>`, never a loop of this command. It writes back into the customer's live mailbox, site or drive.

### session

Manage session

- **`cloudally-cli session login`** - Returns the access and refresh tokens.
- **`cloudally-cli session partner-login`** - Returns the access and refresh tokens.
- **`cloudally-cli session refresh`** - Returns the access and refresh tokens.

### tasks

Manage tasks

- **`cloudally-cli tasks activate`** - Activate a backup task to run on schedule, if it was paused.
- **`cloudally-cli tasks activate-all`** - Activate all accounts in a task for a backup.
- **`cloudally-cli tasks activate-entity`** - Activate a specific account in a task for a backup.
- **`cloudally-cli tasks add-entity`** - Manually add a site or account for a backup in a backup task.
- **`cloudally-cli tasks backup-entity-now`** - Run a backup for the specified account or site.
- **`cloudally-cli tasks backup-now`** - Run a backup task with all the activated accounts or sites in it.
- **`cloudally-cli tasks cancel`** - Cancel the specified running task (an entire backup, restore or download tasks).
- **`cloudally-cli tasks cancel-entity`** - Cancel the specified running task of a specified account or a site.
- **`cloudally-cli tasks delete`** - Delete a backup task, removing all the data.
- **`cloudally-cli tasks get`** - Get information for the specified task
- **`cloudally-cli tasks list`** - List all tasks of a specified type: backup, restore, download (paginated).
- **`cloudally-cli tasks pause`** - Pause execution by schedule for all accounts or sites of a backup task.
- **`cloudally-cli tasks pause-entity`** - Pause execution by schedule of a specified account or a site.
- **`cloudally-cli tasks remove-entity`** - Deactivate an account or site from a backup, removing all the coorresponding data.

### user

Manage user

- **`cloudally-cli user bills`** - Get current billing forecast.
- **`cloudally-cli user create-primary`** - Create a Cloudally account (trial status).
- **`cloudally-cli user create-secondary`** - Create a Cloudally user in a Cloudally account.
- **`cloudally-cli user delete`** - Cancel subscription and delete a Cloudally account.
- **`cloudally-cli user me`** - Get Cloudally Account settings.
- **`cloudally-cli user secondaries`** - Get list of the Cloudally users of a Cloudally account.
- **`cloudally-cli user status`** - Get status for all tasks of user.
- **`cloudally-cli user update-password`** - Update password
- **`cloudally-cli user update-permissions`** - Update permission setting of a Cloudally account user.
- **`cloudally-cli user update-settings`** - Update Cloudally account settings.


### backup


- **`cloudally-cli backup health`** - Find every mailbox or site across all customer accounts that has not backed up in N days or whose last attempt failed, with the error text.

- **`cloudally-cli backup snapshots`** - List every past backup run for one mailbox in a given month, with each run's date in both ISO and the raw yyyyMMddHHmmss form a restore body needs.


### restore


- **`cloudally-cli restore plan`** - Preview every mailbox-and-date restore a batch would submit, with the exact backup each mailbox will be restored from.

- **`cloudally-cli restore submit`** - Submit one restore per mailbox per date with a local ledger and a restore guard (`restore-guard.db` in the state directory) shared by every login and token that uses that state directory: a restore already sent from it is never sent again unless you pass --new-batch. Another OS user, `--home`, `CLOUDALLY_HOME` or `CLOUDALLY_STATE_DIR` has its own guard, and `recovery restore` and the MCP `cloudally_execute` tool bypass it; --resume keeps the batch's recorded target account and restore options.

- **`cloudally-cli restore watch`** - Follow a submitted batch until every restore finishes, with a per-mailbox tally and a non-zero exit if any failed.

- **`cloudally-cli restore history`** - See what was restored for which company and when, from the local ledger, without calling the API.


### download


- **`cloudally-cli download submit`** - Create one download (export) job per mailbox and sub-source for every mailbox backed up on one date, with a local ledger and a reconcile pass.

- **`cloudally-cli download fetch`** - Download every ready archive to disk, one folder per year, mailbox and sub-source.

- **`cloudally-cli download start`** - Build one archive of one mailbox (single-archive sources; MS365 goes through download submit).

- **`cloudally-cli download list`** - List an account's download (export) jobs.

- **`cloudally-cli download watch`** - Follow one download job until its archive is ready.

- **`cloudally-cli download link`** - Mint a fresh 60-second link for each file of a finished download job.


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`cloudally-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`cloudally-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`cloudally-cli learnings list`** - Inspect taught rows
- **`cloudally-cli learnings forget <query>`** - Undo a teach
- **`cloudally-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`cloudally-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`cloudally-cli teach-pattern`** - Install a query/resource template up front
- **`cloudally-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `CLOUDALLY_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `cloudally-cli` opens the database, older binaries refuse it with a version error - upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
cloudally-cli tasks list

# JSON for scripting and agents
cloudally-cli tasks list --json
# Filter to specific fields
cloudally-cli tasks list --json --select id,source,status

# Dry run - show the request without sending
cloudally-cli tasks list --dry-run

# Agent mode - JSON + compact + no prompts in one flag
cloudally-cli tasks list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error (for `restore submit` and `restore watch`: rows failed, are UNKNOWN or need a person; read the message and never re-run the original command), `6` partial failure, `7` rate limited, `8` async job submitted but not finished (rerun the printed recovery command; do not resubmit), `10` config error.

## Health Check

```bash
cloudally-cli doctor
```

Checks configuration and paths, then verifies the stored token with one authenticated read (`GET /v2/user`): it reports the credential as valid, rejected (run `auth login` again) or not verified, and turns the API row red when `base_url` is not the CloudAlly API. Add `--fail-on error` to make a failed check exit non-zero.

## Configuration

Run `cloudally-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/cloudally-cli/config.toml` (`%USERPROFILE%\.config\cloudally-cli\config.toml` on Windows); `--home`, `CLOUDALLY_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `CLOUDALLY_CLIENT_ID` | credential | Yes | API application client id, sent as the `client-id` header on every request (partner portal > Integrations > REST API). |
| `CLOUDALLY_CLIENT_SECRET` | credential | Yes | API application client secret, sent as the `client-secret` header on every request. |
| `CLOUDALLY_ACCESS_TOKEN` | credential | No | Bearer token override. Leave it unset and run `auth login`, which saves and refreshes the token; when set it overrides the stored token on every command. |
| `CLOUDALLY_EMAIL` | login | No | Company account login email used by `auth login` (the default login, limited to that one company). |
| `CLOUDALLY_PASSWORD` | login | No | Company account login password used by `auth login`. |
| `CLOUDALLY_PARTNER_EMAIL` | login | No | Partner portal login email used by `auth login --partner` (reaches every customer account; needed for past backup dates and downloads). |
| `CLOUDALLY_PARTNER_PASSWORD` | login | No | Partner portal login password used by `auth login --partner`. |
| `CLOUDALLY_BASE_URL` | endpoint | No (default `https://api.cloudally.com`) | Sends every API call to this base URL instead; for tests and mock servers. |
| `CLOUDALLY_PORTAL_URL` | endpoint | No (default `https://app.cloudally.com`) | Sends the customer-portal handoff to this origin instead; for tests and mock servers. Required when `CLOUDALLY_BASE_URL` is set: portal commands refuse to send a partner handoff to production while the API points elsewhere. |
| `CLOUDALLY_CONFIG` | path | No | Config file path (default `~/.config/cloudally-cli/config.toml`). |
| `CLOUDALLY_MCP_READ_ONLY` | MCP | No | Set to `1` so the MCP server hides tools that change CloudAlly state and refuses non-GET execute calls. Tools still write local state: the response cache and learning entries. |
| `CLOUDALLY_MCP_HTTP_TOKEN` | MCP | Only with `--transport http` | Bearer token HTTP MCP callers must send; the HTTP transport refuses to start without it. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `cloudally-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `cloudally-cli doctor` to check credentials
- Run `cloudally-cli auth login --partner` (or `auth login`) again; a set `CLOUDALLY_ACCESS_TOKEN` overrides the stored token, so unset it
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **HTTP 403 with an empty body on every command** - The client-id or client-secret header is missing or wrong: set CLOUDALLY_CLIENT_ID and CLOUDALLY_CLIENT_SECRET, then run cloudally-cli doctor
- **401 The user is not logged in** - The access token expired and could not be refreshed: run cloudally-cli auth login again
- **restore watch shows UNMATCHED rows** - The restore was submitted but no RESTORE task appeared yet; rerun restore watch --once after a minute, or check tasks list --type RESTORE in that account
- **backup health says no local data** - Run cloudally-cli sync --resources distributor_accounts-backup-statuses first; the sweep reads the synced feed
- **auth use-account says no partner token available** - Set CLOUDALLY_PARTNER_EMAIL and CLOUDALLY_PARTNER_PASSWORD and run cloudally-cli auth login --partner
- **auth use-account or a stored login seems ignored** - CLOUDALLY_ACCESS_TOKEN in the environment overrides the stored token on every command; unset it
- **backup snapshots or restore plan finds no past backups for a mailbox that has them** - The calendar needs the mailbox's external id; the CLI reads it from the task. Pass `backup snapshots --ext-id <id>` when the task record lacks one.
- **A download for the right day holds the day before** - Snapshot dates are UTC; pass `download submit --utc-offset <hours>` (for example -5 for US Eastern) so `--on-date` means the account's own day.
- **distributor-recovery export (or recovery export) succeeds but no download appears** - That documented endpoint creates no job. Use `download submit` or `download start`.
- **portal session is not authenticated (status 401)** - Several portal sessions for the same customer started at the same instant invalidated each other's handoff tokens. Start parallel `download fetch` or `download submit` runs a few seconds apart, or run them one at a time.
- **A link from download link stopped working** - Links last about 60 seconds. Run `download link` again for a fresh one, or let `download fetch` mint and stream in one step.
- **restore submit lists restores that may already have been accepted** - Check the batch with cloudally-cli restore watch --batch <id> --once, and again after a few minutes: a RESTORE task can appear late (see UNMATCHED above), and watch settles every row it finds one for. Only for rows still UNKNOWN after that, and after checking tasks list --type RESTORE in that account, the human may then run the command the notice prints (your original submit with --resume --retry-unknown --yes added); it sends the rows still UNKNOWN plus any never sent or refused before they ran, and nothing CloudAlly accepted.
- **restore submit refuses: a restore was already sent** - The restore guard (restore-guard.db in the state dir) spans every login and token that uses this state directory. Look the batch up with restore history --batch <id> under the login named; send only the remaining mailboxes with the printed --entities command. A batch recorded before per-user stores (in data.db) is refused too but no longer shows in restore history; treat it as sent.
- **restore submit --resume finds no earlier batch** - Each login (the partner login, or one company after auth use-account) keeps its own ledger. Run restore history under the login that started the batch. Dropping --resume starts a new batch, which the restore guard refuses for any restore already sent from this state directory; never add --new-batch to get past it.
- **A portal command refuses with CLOUDALLY_PORTAL_URL** - CLOUDALLY_BASE_URL points away from production, so the CLI will not send a partner handoff token to the production portal. Set CLOUDALLY_PORTAL_URL to the matching portal, or unset CLOUDALLY_BASE_URL.
- **restore submit refuses: batch already has restores CloudAlly accepted** - Re-running would restore those mailboxes twice. Run the --resume command the error prints. --new-batch restores every mailbox again; it is a human decision, never an agent's.
- **--resume refuses: different options** - A resumed batch always restores to the target and options it started with. Drop the flag to resume with the recorded values. To send only the mailboxes this batch never sent, with the new options, run the --entities command the error prints. Do not use --new-batch: it restores every mailbox in the batch again, accepted ones included.
- **restore watch exits 5: failed after CloudAlly accepted them** - Those restores ran and may have written part of the mailbox; neither --resume nor a new batch sends them again on its own. Check what they restored first. To restore just those mailboxes a second time on purpose (a human decision, never an agent's), run the original restore submit with --entities <those mailboxes> --new-batch --yes, as the error prints.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**cloudally (Ruby gem)**](https://github.com/jancotanis/cloudally) - Ruby
- [**n8n-nodes-cloudally**](https://github.com/davejlong/n8n-nodes-cloudally) - TypeScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
