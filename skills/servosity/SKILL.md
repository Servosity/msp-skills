---
name: servosity
description: "Every Servosity backup and DR feature for MSP partners, plus a local fleet mirror, snapshot history, and cross-engine rollups the partner portal can't show. Trigger phrases: `what needs my attention across clients`, `which backups are stale`, `what changed in my fleet overnight`, `show a client's backup status`, `find backups failing across engines`, `triage backup issues`, `use servosity`, `run servosity`."
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

Servosity REST API surface available to authenticated MSP partners. All operations are scoped to the authenticated reseller. Admin-only endpoints (cross-reseller listing, billing back-office, support tooling) are not included.

## Unique Capabilities

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

## Auth Setup
Run `servosity-cli auth setup` to print the URL and steps for getting a key (add `--launch` to open the URL). Then set:

```bash
export SERVOSITY_MSP_TOKEN="<your-key>"
```

Or persist it in `~/.config/servosity-partner-msp-pp-cli/config.toml`.

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

Entries are stored locally at `~/.servosity-cli/feedback.jsonl`. They are never POSTed unless `SERVOSITY_MSP_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `SERVOSITY_MSP_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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

Install the MCP binary from this CLI's published public-library entry or pre-built release, then register it:

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
