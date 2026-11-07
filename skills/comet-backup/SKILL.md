---
name: comet-backup
description: "Fleet-wide backup health, failure triage and usage reports for every tenant on your Comet Server, plus the admin API calls an MSP uses day to day. Trigger phrases: `which comet backups failed last night`, `comet backup health across all tenants`, `comet storage usage per customer`, `find offline comet devices`, `re-run failed comet backups`, `use comet-backup`, `run comet-backup`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Comet Backup"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - comet-backup-cli
    install:
      - kind: script
        bins: [comet-backup-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/comet-backup/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/comet-backup/install.ps1
---

# Comet Backup - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `comet-backup-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/comet-backup/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/comet-backup/install.ps1 | iex
   ```
3. Verify: `comet-backup-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `comet-backup-cli` and `comet-backup-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Comet's console and emailed reports answer one customer at a time. Here a tenant is a Comet user account. `health` lists every protected item that failed, is overdue or never ran across all tenants; `triage` adds the error lines; `usage` breaks out storage, devices, boosters and Microsoft 365 seats per tenant; `snapshot` and `changes` keep a local history the API does not. Works with self-hosted and Comet-Hosted servers.

## When to Use This CLI

Use this CLI when an MSP needs answers across all tenants on one Comet Server: which backups failed or stopped running, why, which devices are offline or outdated, and how much storage and how many seats each customer uses. It also covers the admin actions a technician runs from the console, such as running a backup now, cancelling a stuck job, unlocking a vault or requesting a Storage Vault, with --dry-run on every write.

## Anti-triggers

Do not use this CLI for:
- Do not use it for Comet Account Portal billing, invoices or licence purchases (account.cometbackup.com is a separate API)
- Do not use it for end-user self-service actions on a single backup account; this CLI uses the admin API
- Do not use it to change Comet Server configuration, branding or admin accounts; use the Comet web console
- Do not use it as a file restore browser for end users; restores need a live device and an admin dispatch

## Unique Capabilities

### Fleet-wide answers Comet's console gives one tenant at a time
- **`health`**  -  See the backup state of every protected item across every tenant in one list, including items that quietly stopped running.

  _Reach for this first in a morning backup review: one call answers which protected items are failed, overdue or never backed up fleet-wide._

  ```bash
  comet-backup-cli health --state failed,overdue,never --agent
  ```
- **`triage`**  -  Find out why backups failed: the failed and stuck jobs in a time window (warnings with --include-warnings), with the first error lines from each job log.

  _Use it when you need the reason a backup failed, not just the status code, without opening each job log._

  ```bash
  comet-backup-cli triage --window 24h --agent
  ```
- **`rerun-failed`**  -  Plan and, with --yes, re-run failed scheduled backups on devices that are online now; quota, cancelled, already-running, custom-job and offline-device failures are skipped.

  _Use it right after triage to retry fixable failures in one step; without --yes it only prints the plan._

  ```bash
  comet-backup-cli rerun-failed --window 24h --agent
  ```

### Billing and capacity
- **`usage`**  -  Get per-tenant storage against quota, billable devices, booster engines, Microsoft 365 accounts and VM guests; these are the server's own counts, not an invoice.

  _Use it before invoicing or a QBR, or to find tenants about to hit a storage quota._

  ```bash
  comet-backup-cli usage --over 80 --agent
  ```

### Device fleet hygiene
- **`devices stale`**  -  Find offline devices: registered devices not connected now, ranked by how long since their last backup, flagged when they still carry billable protected items.

  _Use it to chase dead machines that still cost money or have silently stopped backing up._

  ```bash
  comet-backup-cli devices stale --older-than 7d --agent
  ```
- **`devices outdated`**  -  List devices whose Comet client version differs from the server version, with the connection id an online device needs for devices update-software.

  _Use it after a Comet release to find the devices to pass to devices update-software._

  ```bash
  comet-backup-cli devices outdated --agent
  ```

### Local state that compounds
- **`changes`**  -  Compare the two newest local snapshots: problems opened or resolved, devices and protected items added or removed, storage growth. Run snapshot twice first.

  _Use it in a scheduled job that opens and closes PSA tickets, or to review a week of fleet changes._

  ```bash
  comet-backup-cli changes --only health --agent
  ```
- **`snapshot`**  -  Capture the current fleet into a local SQLite store so changes can compare runs.

  _Run it on a schedule so changes has history to compare._

  ```bash
  comet-backup-cli snapshot --keep 90
  ```

## Command Reference

**backup_jobs**  -  Backup, restore and retention job history: recent, by date range, by user, logs, cancel

- `comet-backup-cli backup-jobs abandon`  -  Mark a running job as abandoned. This will change the status of a running job to abandoned.
- `comet-backup-cli backup-jobs all`  -  Get jobs (All). The jobs are returned in an unspecified order. This API requires the Auth Role to be enabled.
- `comet-backup-cli backup-jobs cancel`  -  Cancel a running job. A request is sent to the live-connected device, asking it to cancel the operation.
- `comet-backup-cli backup-jobs count`  -  Count jobs (for custom search). This API requires the Auth Role to be enabled.
- `comet-backup-cli backup-jobs for-user`  -  Get jobs (for user). The jobs are returned in an unspecified order. This API requires the Auth Role to be enabled.
- `comet-backup-cli backup-jobs get`  -  Get properties of a single job. This API requires the Auth Role to be enabled.
- `comet-backup-cli backup-jobs log`  -  Get the report log entries for a single job, in plaintext format. This API requires the Auth Role to be enabled.
- `comet-backup-cli backup-jobs log-entries`  -  Get the report log entries for a single job. This API requires the Auth Role to be enabled.
- `comet-backup-cli backup-jobs query`  -  Get jobs (for custom search). The jobs are returned in an unspecified order.
- `comet-backup-cli backup-jobs range`  -  Get jobs (for date range). The jobs are returned in an unspecified order.
- `comet-backup-cli backup-jobs recent`  -  Get jobs (Recent and incomplete). The jobs are returned in an unspecified order.

**constellation**  -  Constellation bucket-usage auditing status and reports

- `comet-backup-cli constellation last-report`  -  Get Constellation bucket usage report (cached). This API requires the Constellation Role to be enabled.
- `comet-backup-cli constellation status`  -  Get Constellation status. This API requires the Constellation Role to be enabled.

**devices**  -  Devices with a live connection to the server: list online devices and dispatch backup, restore, update and vault maintenance commands

- `comet-backup-cli devices apply-retention`  -  Instruct a live connected device to apply retention rules now. This API requires the Auth Role to be enabled.
- `comet-backup-cli devices browse-snapshot`  -  Request a list of stored objects inside an existing backup job.
- `comet-backup-cli devices deep-verify`  -  Instruct a live connected device to deeply verify Storage Vault content. This API requires the Auth Role to be enabled.
- `comet-backup-cli devices disconnect`  -  Disconnect a live connected device. The device will almost certainly attempt to reconnect.
- `comet-backup-cli devices kill`  -  Instruct a live connected device to disconnect.
- `comet-backup-cli devices lobby`  -  List live connected devices in lobby mode. This API requires the Auth Role to be enabled.
- `comet-backup-cli devices online`  -  List live connected devices. This API requires the Auth Role to be enabled.
- `comet-backup-cli devices refetch-profile`  -  Instruct a live connected device to refresh their profile. This API requires the Auth Role to be enabled.
- `comet-backup-cli devices reindex`  -  Instruct a live connected device to rebuild Storage Vault indexes now. This API requires the Auth Role to be enabled.
- `comet-backup-cli devices run-backup`  -  Instruct a live connected device to run a scheduled backup. This API requires the Auth Role to be enabled.
- `comet-backup-cli devices run-backup-custom`  -  Instruct a live connected device to run a backup. This API requires the Auth Role to be enabled.
- `comet-backup-cli devices run-restore`  -  Instruct a live connected device to perform a local restore. This API requires the Auth Role to be enabled.
- `comet-backup-cli devices uninstall`  -  Instruct a live connected device to self-uninstall the software. This API requires the Auth Role to be enabled.
- `comet-backup-cli devices unlock`  -  Instruct a live connected device to remove lock files from a Storage Vault. Misuse can cause data loss!
- `comet-backup-cli devices update-software`  -  Instruct a live connected device to download a software update. This API requires the Auth Role to be enabled.
- `comet-backup-cli devices vault-snapshots`  -  Request a list of Storage Vault snapshots from a live connected device. This API requires the Auth Role to be enabled.

**organizations**  -  Organizations on a multi-tenant Comet Server; each organization holds its own user accounts and admins

- `comet-backup-cli organizations`  -  List Organizations.

**policies**  -  Policy objects applied to user accounts

- `comet-backup-cli policies get`  -  Retrieve a single policy object. A hash is also returned, to allow atomic modification operations.
- `comet-backup-cli policies list`  -  List all policy object names.
- `comet-backup-cli policies list-full`  -  Get all policy objects.

**protected_items**  -  Protected Items (backup sources) and their backup rules

- `comet-backup-cli protected-items`  -  Get a Protected Item with its backup rules. This API requires the Auth Role to be enabled.

**quotas**  -  Shared storage quota objects

- `comet-backup-cli quotas get`  -  Get properties for a shared storage quota. This API requires the Auth Role to be enabled.
- `comet-backup-cli quotas list`  -  List available shared storage quota objects. This API requires the Auth Role to be enabled.

**server**  -  Comet Server version, statistics, logs, replication and update status

- `comet-backup-cli server log-days`  -  Get log files. This API is only available for top-level administrator accounts, not for Tenant administrator accounts.
- `comet-backup-cli server logs`  -  Get log file content. On non-Windows platforms, log content uses LF line endings.
- `comet-backup-cli server news`  -  Get News entries (Admin). This API requires the Auth Role to be enabled.
- `comet-backup-cli server replication`  -  Get Replication status.
- `comet-backup-cli server stats`  -  Get Comet Server historical statistics. The returned key-value map is not necessarily ordered.
- `comet-backup-cli server update-campaign`  -  Get current campaign status.
- `comet-backup-cli server update-news`  -  Get software update news from the software provider.
- `comet-backup-cli server version`  -  Get server properties. Retrieve the version number and basic properties about the server.

**storage**  -  Storage buckets, free space, and Storage Vault provisioning

- `comet-backup-cli storage bucket`  -  Retrieve properties for a single bucket.
- `comet-backup-cli storage buckets`  -  List all buckets. This API requires the Storage Role to be enabled.
- `comet-backup-cli storage free-space`  -  Retrieve available space metrics. Access to this API may be prevented on a per-administrator basis.
- `comet-backup-cli storage request-vault`  -  Request a new Storage Vault on behalf of a user.
- `comet-backup-cli storage vault-providers`  -  Get the available options for Requesting a Storage Vault. This API requires the Auth Role to be enabled.

**user_groups**  -  User groups and their member accounts

- `comet-backup-cli user-groups get`  -  Retrieve a single user group object. This API requires the Auth Role to be enabled.
- `comet-backup-cli user-groups list`  -  List all user group names.
- `comet-backup-cli user-groups list-full`  -  Get all user group objects.

**users**  -  Tenant user accounts on the Comet Server: list, inspect, create, delete, reset password, revoke devices

- `comet-backup-cli users add`  -  Add a new user account. This API requires the Auth Role to be enabled.
- `comet-backup-cli users delete`  -  Delete user account. This does not remove any storage buckets.
- `comet-backup-cli users disable-totp`  -  Disable user account 2FA(TOTP) authentication. This API requires the Auth Role to be enabled.
- `comet-backup-cli users get`  -  Get user account profile. This API requires the Auth Role to be enabled.
- `comet-backup-cli users list`  -  List all user accounts. This API requires the Auth Role to be enabled.
- `comet-backup-cli users list-full`  -  List all user account profiles. This API requires the Auth Role to be enabled.
- `comet-backup-cli users reset-password`  -  Reset user account password. The user account must have a recovery code present.
- `comet-backup-cli users revoke-device`  -  Revoke device from user account.
- `comet-backup-cli users set-profile`  -  Modify user account profile. This API requires the Auth Role to be enabled.


**changes**


- `comet-backup-cli changes`  -  Compare the two newest local snapshots: problems opened or resolved, devices and protected items added or removed, storage growth. Run snapshot twice first.


**devices**


- `comet-backup-cli devices stale`  -  Find offline devices: registered devices not connected now, ranked by how long since their last backup, flagged when they still carry billable protected items.

- `comet-backup-cli devices outdated`  -  List devices whose Comet client version differs from the server version, with the connection id an online device needs for devices update-software.


**health**


- `comet-backup-cli health`  -  See the backup state of every protected item across every tenant in one list, including items that quietly stopped running.


**rerun-failed**


- `comet-backup-cli rerun-failed`  -  Plan and, with --yes, re-run failed scheduled backups on devices that are online now; quota, cancelled, already-running, custom-job and offline-device failures are skipped.


**snapshot**


- `comet-backup-cli snapshot`  -  Capture the current fleet into a local SQLite store so changes can compare runs.


**triage**


- `comet-backup-cli triage`  -  Find out why backups failed: the failed and stuck jobs in a time window (warnings with --include-warnings), with the first error lines from each job log.


**usage**


- `comet-backup-cli usage`  -  Get per-tenant storage against quota, billable devices, booster engines, Microsoft 365 accounts and VM guests; these are the server's own counts, not an invoice.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
comet-backup-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning health check, narrowed for an agent

```bash
comet-backup-cli health --state problem --agent --select tenant,device,item,state,hours_since_success
```

One row per protected item that needs attention (failed, warning, overdue or never backed up), trimmed to the fields a triage note needs.

### Group last night's failures by cause

```bash
comet-backup-cli triage --window 24h --group-by error --agent
```

Buckets failed jobs by normalized error message so a systemic problem (one bad vault, one expired credential) stands out.

### Tenants near quota, as CSV for the QBR deck

```bash
comet-backup-cli usage --over 80 --csv
```

Storage, device, booster and Microsoft 365 counts for every tenant at or above 80 percent of a quota.

### Update outdated clients after a release

```bash
comet-backup-cli devices outdated --online-only --agent
```

Lists connected devices whose client version differs from the server version, each with the connection id to pass to devices update-software --target-id.

### Daily ticket feed

```bash
comet-backup-cli snapshot >/dev/null && comet-backup-cli changes --only health --agent
```

Records today's fleet, then emits health changes since the previous snapshot (problem_opened, problem_resolved, problem_changed, device_offline, device_online) with a stable change_id (tenant/item); open tickets on problem_opened and close them on problem_resolved by filtering on kind.

## Auth Setup

Comet reads admin credentials from the POST form body, not a header. Set COMET_BACKUP_SERVER to your Comet Server's host and port (for example backup.example.com or 10.0.0.5:8060; the CLI connects over https), or COMET_BACKUP_BASE_URL to its full https URL, then COMET_BACKUP_USERNAME and COMET_BACKUP_PASSWORD for a Comet admin account. Every request goes only to that server: a redirect to any other host is refused. Comet recommends a dedicated API admin with a long random password and no 2FA. For an admin account that requires 2FA, obtain a session key outside this CLI with Comet's AdminAccountSessionStart API (POST /api/v1/admin/account/session-start with the password and TOTP code) and export COMET_BACKUP_SESSION_KEY; it takes precedence over the password. Personal API tokens (bkup_..., Comet 26.8.3+) are not supported: the public API docs do not document how to send one, and an authentication failure with one configured says so. On Comet-Hosted servers, create an Additional User with a local password for API access.

Credential tier: ten reads return credential material from Comet (password hashes, TOTP keys, Storage Vault and bucket keys, organization storage passwords, the passwords a protected item or policy can hold in its settings): `users get`, `users list-full`, `user-groups get`, `organizations`, `storage buckets`, `storage bucket` and `constellation last-report`, `policies get`, `policies list-full`, `protected-items`. The CLI prints every credential-named field they return as `[redacted]`, the local store keeps none of them, and the MCP server runs them only with `confirm: true` (and not at all under `COMET_BACKUP_MCP_READ_ONLY=1`). Keep them human-in-the-loop like any credential-tier command.

Run `comet-backup-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (warnings and hints go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  comet-backup-cli backup-jobs get --job-id 4f1c2a9e-7b3d-4e8a-9c2f-6d5e4b3a2c1d --agent --select BackupRuleGUID,CancellationID,Classification
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Local history**  -  `snapshot` writes a credential-free fleet history to local SQLite and `changes` reads it offline
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local"},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `COMET_BACKUP_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `COMET_BACKUP_CONFIG_DIR`, `COMET_BACKUP_DATA_DIR`, `COMET_BACKUP_STATE_DIR`, `COMET_BACKUP_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `COMET_BACKUP_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, the `data-<scope>.db` snapshot store, and other local state. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `comet-backup-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "comet-backup": {
        "command": "comet-backup-mcp",
        "env": {
          "COMET_BACKUP_HOME": "/srv/comet-backup"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `COMET_BACKUP_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `COMET_BACKUP_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; there is no response cache, and store records keep every credential field as `[redacted]`. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `COMET_BACKUP_NO_LOCAL_DATA=1` stops store writes and turns `auto` reads into live reads; `--data-source local` still reads an existing store, and `COMET_BACKUP_DATA_SOURCE` sets the `--data-source` default.

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
comet-backup-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "comet-backup-cli learnings confirm 12"] }
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
       materially more, record the divergence via `comet-backup-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `comet-backup-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "customer" teach satisfying a "tenant" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "acme" matching two tenants with that name). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
comet-backup-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "storage vault") satisfies future queries under another alias (e.g., "vault", "destination") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
comet-backup-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
comet-backup-cli teach-playbook \
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
comet-backup-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between Comet versions, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-tenant / per-device / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`comet-backup-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `COMET_BACKUP_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
comet-backup-cli feedback "the --since flag is inclusive but docs say exclusive"
comet-backup-cli feedback --stdin < notes.txt
comet-backup-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `COMET_BACKUP_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `COMET_BACKUP_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
comet-backup-cli profile save briefing --json
comet-backup-cli --profile briefing backup-jobs get --job-id 4f1c2a9e-7b3d-4e8a-9c2f-6d5e4b3a2c1d
comet-backup-cli profile list --json
comet-backup-cli profile show briefing
comet-backup-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Known limits

- Built and tested without access to a live Comet Server: the wire format follows Comet's public API reference and the official Go SDK, and was verified against a local mock. Treat the first run against your server as the real test.
- Personal API tokens (AuthType=Token, Comet 26.8.3+) are not supported; use an admin password or a session key.
- `usage` reports this server's own counts (storage, devices, boosters, Microsoft 365 accounts), not an invoice. Booster engines follow the official comet-munin-plugin list; VMware and Proxmox items are not flagged as boosters.
- `changes` compares local snapshots only; run `comet-backup-cli snapshot` on a schedule first.
- `health` works out overdue runs in each device's own timezone. When a device reports no timezone and its schedule is restricted to certain days or hours, the item is reported with `schedule_unresolved: true` and never flagged overdue; list them with `comet-backup-cli health --state unresolved`.
- `users get`, `users list-full` and the other credential-tier reads print every credential-named field Comet returns (password hashes, TOTP keys, Storage Vault and bucket keys) as `[redacted]`. The fleet commands (`health`, `usage`, `snapshot`, ...) never output those fields.
- A request carrying `[redacted]` is refused: writing a redacted profile back with `users set-profile` would overwrite the real keys. `--deliver webhook:` leaves those members out of the body.
- `agent-context` prints only the withheld notice while a credential is set: its output is too long for the filter's budget. Read it with no credential in the environment, or use the MCP `context` tool.
- `--deliver webhook:` sends to the URL you give it and nowhere else: one POST, no redirect followed. A delivery that would carry a credential this program holds (your Comet password or session key) is refused.
- The admin login is sent only over https, to the one configured server. A Comet Server on plain http needs a TLS listener first; for a private certificate authority set `COMET_BACKUP_CA_CERT`.
- Any output that repeats a credential this program holds, in any encoding the filter reads, is replaced whole by `[credkit: withheld from <sink>: it carried a credential this program holds]`. A very short password withholds more often. Filtering adds under 1.5 s to a 600 KB answer.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Generic failure |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `comet-backup-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add comet-backup-mcp --env COMET_BACKUP_SERVER=backup.example.com:8060 --env COMET_BACKUP_USERNAME=api-admin --env COMET_BACKUP_PASSWORD=<admin-password> -- comet-backup-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `comet-backup-mcp --transport http` requires `COMET_BACKUP_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `COMET_BACKUP_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `COMET_BACKUP_MCP_READ_ONLY=1` drops every tool not marked read-only (the local-write tools `snapshot`, `teach` and the learnings and playbook writers too) and makes the raw execute tool refuse state-changing calls; reads stay available.

## Direct Use

1. Check if installed: `which comet-backup-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   comet-backup-cli health --state problem --agent
   ```
4. If ambiguous, drill into subcommand help: `comet-backup-cli devices --help`.
