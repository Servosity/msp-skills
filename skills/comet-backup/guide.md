# Comet Backup CLI

**Fleet-wide backup health, failure triage and usage reports for every tenant on your Comet Server, plus the admin API calls an MSP uses day to day.**

Comet's console and emailed reports answer one customer at a time. Here a tenant is a Comet user account. `health` lists every protected item that failed, is overdue or never ran across all tenants; `triage` adds the error lines; `usage` breaks out storage, devices, boosters and Microsoft 365 seats per tenant; `snapshot` and `changes` keep a local history the API does not. Works with self-hosted and Comet-Hosted servers.

Learn more at [Comet Backup](https://docs.cometbackup.com/latest/api/api-guide).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `comet-backup-cli` and `comet-backup-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=comet-backup). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/comet-backup --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/comet-backup --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `comet-backup-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the comet-backup skill from https://github.com/Servosity/msp-skills/tree/main/skills/comet-backup. The skill defines how its required CLI (`comet-backup-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=comet-backup).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in your Comet Server (host:port), admin username and password (or a session key) when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/comet-backup/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/comet-backup/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "comet-backup": {
      "command": "comet-backup-mcp",
      "env": {
        "COMET_BACKUP_SERVER": "backup.example.com:8060",
        "COMET_BACKUP_USERNAME": "<admin-username>",
        "COMET_BACKUP_PASSWORD": "<admin-password>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`comet-backup-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `COMET_BACKUP_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `COMET_BACKUP_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export COMET_BACKUP_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
comet-backup-mcp --transport http
```

Set `COMET_BACKUP_MCP_READ_ONLY=1` (exactly `1`) to drop every tool not marked read-only, including `import`, `rerun_failed`, `snapshot`, `teach` and the other local-write tools, and make the raw execute tool refuse state-changing calls. Reads such as `health`, `triage`, `usage`, `changes` and `comet-backup_read` stay available.

## Authentication

Comet reads admin credentials from the POST form body, not a header. Set COMET_BACKUP_SERVER to your Comet Server's host and port (for example backup.example.com or 10.0.0.5:8060; the CLI connects over https), or COMET_BACKUP_BASE_URL to its full https URL, then COMET_BACKUP_USERNAME and COMET_BACKUP_PASSWORD for a Comet admin account. Every request goes only to that server: a redirect to any other host is refused. Comet recommends a dedicated API admin with a long random password and no 2FA. For an admin account that requires 2FA, obtain a session key outside this CLI with Comet's AdminAccountSessionStart API (POST /api/v1/admin/account/session-start with the password and TOTP code) and export COMET_BACKUP_SESSION_KEY; it takes precedence over the password. Personal API tokens (bkup_..., Comet 26.8.3+) are not supported: the public API docs do not document how to send one, and an authentication failure with one configured says so. On Comet-Hosted servers, create an Additional User with a local password for API access.

Credential tier: ten reads return credential material from Comet (password hashes, TOTP keys, Storage Vault and bucket keys, organization storage passwords, the passwords a protected item or policy can hold in its settings): `users get`, `users list-full`, `user-groups get`, `organizations`, `storage buckets`, `storage bucket` and `constellation last-report`, `policies get`, `policies list-full`, `protected-items`. The CLI prints every credential-named field they return as `[redacted]`, the local store keeps none of them, and the MCP server runs them only with `confirm: true` (and not at all under `COMET_BACKUP_MCP_READ_ONLY=1`). Keep them human-in-the-loop like any credential-tier command.

## Quick Start

```bash
# Confirm the CLI is installed; the dry-run only prints what doctor would check
comet-backup-cli doctor --dry-run

# Check config, the server URL and whether credentials are set (it does not log in)
comet-backup-cli doctor

# The real credential check: one authenticated call to the server
comet-backup-cli server version --json

# Every protected item that needs attention, across all tenants
comet-backup-cli health --state failed,overdue,never --agent

# Why last night's jobs failed, with error lines
comet-backup-cli triage --window 24h --agent

# Start the local history that changes compares
comet-backup-cli snapshot

```

## Unique Features

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

## Usage

Run `comet-backup-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, the `data-<scope>.db` snapshot store, and other local state |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `COMET_BACKUP_CONFIG_DIR`, `COMET_BACKUP_DATA_DIR`, `COMET_BACKUP_STATE_DIR`, or `COMET_BACKUP_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `COMET_BACKUP_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export COMET_BACKUP_HOME=/srv/comet-backup
comet-backup-cli doctor
```

On Windows (PowerShell):

```powershell
$env:COMET_BACKUP_HOME = "C:\ProgramData\comet-backup"
comet-backup-cli doctor
```

Under `COMET_BACKUP_HOME=/srv/comet-backup`, the four dirs resolve to `/srv/comet-backup/config`, `/srv/comet-backup/data`, `/srv/comet-backup/state`, and `/srv/comet-backup/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:COMET_BACKUP_HOME = "C:\srv\comet-backup"
comet-backup-cli doctor
```

Under `$env:COMET_BACKUP_HOME = "C:\srv\comet-backup"`, the four dirs resolve to `C:\srv\comet-backup\config`, `C:\srv\comet-backup\data`, `C:\srv\comet-backup\state`, and `C:\srv\comet-backup\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `COMET_BACKUP_DATA_DIR` overrides an explicit `--home` for that kind. Use `COMET_BACKUP_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `COMET_BACKUP_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `comet-backup-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. There is no response cache: Comet responses carry storage keys and password hashes, so raw responses are never written to disk, and store records keep every credential field as `[redacted]`. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `comet-backup-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `comet-backup-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `COMET_BACKUP_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), and nothing new is written to the store. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `COMET_BACKUP_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### backup_jobs

Backup, restore and retention job history: recent, by date range, by user, logs, cancel

- **`comet-backup-cli backup-jobs abandon`** - Mark a running job as abandoned.

This will change the status of a running job to abandoned.
This is intended to be used on jobs which are definitely no longer running but are stuck in the running state; it will not attempt to cancel the job. If the job is detected to still be running after being marked as abandoned, it will be revived.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/job/abandon with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 23.9.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminjobabandon-mark-a-running-job-as-abandoned
- **`comet-backup-cli backup-jobs all`** - Get jobs (All).

The jobs are returned in an unspecified order.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/get-jobs-all with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admingetjobsall-get-jobs-all
- **`comet-backup-cli backup-jobs cancel`** - Cancel a running job.

A request is sent to the live-connected device, asking it to cancel the operation. This may fail if there is no live-connection.
Only jobs from Comet 18.3.5 or newer can be cancelled. A job can only be cancelled if it has a non-empty CancellationID field in its properties.
If the device is running Comet 21.9.5 or later, this API will wait up to ten seconds for a confirmation from the client.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/job/cancel with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 18.3.5 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminjobcancel-cancel-a-running-job
- **`comet-backup-cli backup-jobs count`** - Count jobs (for custom search).

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/count-jobs-for-custom-search with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 22.12.2 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admincountjobsforcustomsearch-count-jobs-for-custom-search
- **`comet-backup-cli backup-jobs for-user`** - Get jobs (for user).

The jobs are returned in an unspecified order.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/get-jobs-for-user with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admingetjobsforuser-get-jobs-for-user
- **`comet-backup-cli backup-jobs get`** - Get properties of a single job.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/get-job-properties with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admingetjobproperties-get-properties-of-a-single-job
- **`comet-backup-cli backup-jobs log`** - Get the report log entries for a single job, in plaintext format.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/get-job-log with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admingetjoblog-get-the-report-log-entries-for-a-single-job-in-plaintext-format
- **`comet-backup-cli backup-jobs log-entries`** - Get the report log entries for a single job.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/get-job-log-entries with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 20.6.8 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admingetjoblogentries-get-the-report-log-entries-for-a-single-job
- **`comet-backup-cli backup-jobs query`** - Get jobs (for custom search).

The jobs are returned in an unspecified order.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/get-jobs-for-custom-search with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 18.9.2 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admingetjobsforcustomsearch-get-jobs-for-custom-search
- **`comet-backup-cli backup-jobs range`** - Get jobs (for date range).

The jobs are returned in an unspecified order.
If the Start parameter is later than End, they will be swapped.
This API will return all jobs that either started or ended within the supplied range.
Incomplete jobs have an end time of 0. You can use this API to find only incomplete jobs by setting both Start and End to 0.
Prior to Comet Server 22.6.0, additional Incomplete jobs may have been returned if you specified non-zero arguments for both Start and End.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/get-jobs-for-date-range with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.6.8 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admingetjobsfordaterange-get-jobs-for-date-range
- **`comet-backup-cli backup-jobs recent`** - Get jobs (Recent and incomplete).

The jobs are returned in an unspecified order.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/get-jobs-recent with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admingetjobsrecent-get-jobs-recent-and-incomplete

### constellation

Constellation bucket-usage auditing status and reports

- **`comet-backup-cli constellation last-report`** - Get Constellation bucket usage report (cached).

This API requires the Constellation Role to be enabled.

Wire format: POST /api/v1/admin/constellation/last-report with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminconstellationlastreport-get-constellation-bucket-usage-report-cached
- **`comet-backup-cli constellation status`** - Get Constellation status.

This API requires the Constellation Role to be enabled.

Wire format: POST /api/v1/admin/constellation/status with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminconstellationstatus-get-constellation-status

### devices

Devices with a live connection to the server: list online devices and dispatch backup, restore, update and vault maintenance commands

- **`comet-backup-cli devices apply-retention`** - Instruct a live connected device to apply retention rules now.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/apply-retention-rules with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.6.9 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherapplyretentionrules-instruct-a-live-connected-device-to-apply-retention-rules-now
- **`comet-backup-cli devices browse-snapshot`** - Request a list of stored objects inside an existing backup job.

The remote device must have given consent for an MSP to browse their files.
To service this request, the remote device must connect to the Storage Vault and load index data. There may be a small delay. If the remote device is running Comet 20.12.0 or later, the necessary index data is cached when this API is first called, for 15 minutes after the last repeated call. This can improve performance for interactively browsing an entire tree of stored objects.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/request-stored-objects with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 19.3.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherrequeststoredobjects-request-a-list-of-stored-objects-inside-an-existing-backup-job
- **`comet-backup-cli devices deep-verify`** - Instruct a live connected device to deeply verify Storage Vault content.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/deepverify-storage-vault with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 18.8.2 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherdeepverifystoragevault-instruct-a-live-connected-device-to-deeply-verify-storage-vault-content
- **`comet-backup-cli devices disconnect`** - Disconnect a live connected device.

The device will almost certainly attempt to reconnect.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/drop-connection with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherdropconnection-disconnect-a-live-connected-device
- **`comet-backup-cli devices kill`** - Instruct a live connected device to disconnect.

The device will terminate its live-connection process and will not reconnect.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/kill-process with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherkillprocess-instruct-a-live-connected-device-to-disconnect
- **`comet-backup-cli devices lobby`** - List live connected devices in lobby mode.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/installation/list-active with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 23.9.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admininstallationlistactive-list-live-connected-devices-in-lobby-mode
- **`comet-backup-cli devices online`** - List live connected devices.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/list-active with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherlistactive-list-live-connected-devices
- **`comet-backup-cli devices refetch-profile`** - Instruct a live connected device to refresh their profile.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/refetch-profile with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherrefetchprofile-instruct-a-live-connected-device-to-refresh-their-profile
- **`comet-backup-cli devices reindex`** - Instruct a live connected device to rebuild Storage Vault indexes now.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/reindex-storage-vault with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 18.6.9 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherreindexstoragevault-instruct-a-live-connected-device-to-rebuild-storage-vault-indexes-now
- **`comet-backup-cli devices run-backup`** - Instruct a live connected device to run a scheduled backup.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/run-backup with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherrunbackup-instruct-a-live-connected-device-to-run-a-scheduled-backup
- **`comet-backup-cli devices run-backup-custom`** - Instruct a live connected device to run a backup.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/run-backup-custom with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherrunbackupcustom-instruct-a-live-connected-device-to-run-a-backup
- **`comet-backup-cli devices run-restore`** - Instruct a live connected device to perform a local restore.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/run-restore with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.9.3 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherrunrestore-instruct-a-live-connected-device-to-perform-a-local-restore
- **`comet-backup-cli devices uninstall`** - Instruct a live connected device to self-uninstall the software.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/uninstall-software with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 20.3.5 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcheruninstallsoftware-instruct-a-live-connected-device-to-self-uninstall-the-software
- **`comet-backup-cli devices unlock`** - Instruct a live connected device to remove lock files from a Storage Vault.

Misuse can cause data loss!
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/unlock with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.9.4 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherunlock-instruct-a-live-connected-device-to-remove-lock-files-from-a-storage-vault
- **`comet-backup-cli devices update-software`** - Instruct a live connected device to download a software update.

This API requires the Auth Role to be enabled.
This API requires the Software Build Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/update-software with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherupdatesoftware-instruct-a-live-connected-device-to-download-a-software-update
- **`comet-backup-cli devices vault-snapshots`** - Request a list of Storage Vault snapshots from a live connected device.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/dispatcher/request-vault-snapshots with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 18.9.1 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindispatcherrequestvaultsnapshots-request-a-list-of-storage-vault-snapshots-from-a-live-connected-device

### organizations

Organizations on a multi-tenant Comet Server; each organization holds its own user accounts and admins

- **`comet-backup-cli organizations`** - List Organizations.

This API is only available for top-level administrator accounts, not for Tenant administrator accounts.

Wire format: POST /api/v1/admin/organization/list with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 20.9.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminorganizationlist-list-organizations

### policies

Policy objects applied to user accounts

- **`comet-backup-cli policies get`** - Retrieve a single policy object.

A hash is also returned, to allow atomic modification operations.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/policies/get with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 18.3.10 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminpoliciesget-retrieve-a-single-policy-object
- **`comet-backup-cli policies list`** - List all policy object names.

For the top-level organization, the API result includes all policies for all organizations, unless the TargetOrganization parameter is present.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/policies/list with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 18.3.10 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminpolicieslist-list-all-policy-object-names
- **`comet-backup-cli policies list-full`** - Get all policy objects.

For the top-level organization, the API result includes all policies for all organizations, unless the TargetOrganization parameter is present.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/policies/list-full with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 18.3.10 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminpolicieslistfull-get-all-policy-objects

### protected_items

Protected Items (backup sources) and their backup rules

- **`comet-backup-cli protected-items`** - Get a Protected Item with its backup rules.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/get-protected-item-with-backup-rules with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 25.9.5 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admingetprotecteditemwithbackuprules-get-a-protected-item-with-its-backup-rules

### quotas

Shared storage quota objects

- **`comet-backup-cli quotas get`** - Get properties for a shared storage quota.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/squota/get-with-hash with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 25.9.8 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminsquotagetwithhash-get-properties-for-a-shared-storage-quota
- **`comet-backup-cli quotas list`** - List available shared storage quota objects.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/squota/list-all with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 25.9.8 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminsquotalistall-list-available-shared-storage-quota-objects

### server

Comet Server version, statistics, logs, replication and update status

- **`comet-backup-cli server log-days`** - Get log files.

This API is only available for top-level administrator accounts, not for Tenant administrator accounts.

Wire format: POST /api/v1/admin/meta/list-available-log-days with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminmetalistavailablelogdays-get-log-files
- **`comet-backup-cli server logs`** - Get log file content.

On non-Windows platforms, log content uses LF line endings. On Windows, Comet changed from LF to CRLF line endings in 18.3.2.
This API does not automatically convert line endings; around the 18.3.2 timeframe, log content may even contain mixed line-endings.
This API is only available for top-level administrator accounts, not for Tenant administrator accounts.

Wire format: POST /api/v1/admin/meta/read-logs with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminmetareadlogs-get-log-file-content
- **`comet-backup-cli server news`** - Get News entries (Admin).

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/news/get-all with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminnewsgetall-get-news-entries-admin
- **`comet-backup-cli server replication`** - Get Replication status.

This API is only available for top-level administrator accounts, not for Tenant administrator accounts.

Wire format: POST /api/v1/admin/replication/state with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminreplicationstate-get-replication-status
- **`comet-backup-cli server stats`** - Get Comet Server historical statistics.

The returned key-value map is not necessarily ordered. Client-side code should sort the result before display.

Wire format: POST /api/v1/admin/meta/stats with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 18.6.2 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminmetastats-get-comet-server-historical-statistics
- **`comet-backup-cli server update-campaign`** - Get current campaign status.

This API is only available for top-level administrator accounts, not for Tenant administrator accounts.
This API requires the Software Build Role to be enabled.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/update-campaign/status with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.9.6 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminupdatecampaignstatus-get-current-campaign-status
- **`comet-backup-cli server update-news`** - Get software update news from the software provider.

Wire format: POST /api/v1/admin/meta/software-update-news with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 18.3.7 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminmetasoftwareupdatenews-get-software-update-news-from-the-software-provider
- **`comet-backup-cli server version`** - Get server properties.

Retrieve the version number and basic properties about the server.

Wire format: POST /api/v1/admin/meta/version with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminmetaversion-get-server-properties

### storage

Storage buckets, free space, and Storage Vault provisioning

- **`comet-backup-cli storage bucket`** - Retrieve properties for a single bucket.

This API can also be used to refresh the size measurement for a single bucket by passing a valid AfterTimestamp parameter.
This API requires the Storage Role to be enabled.

Wire format: POST /api/v1/admin/storage/bucket-properties with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 20.9.4 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminstoragebucketproperties-retrieve-properties-for-a-single-bucket
- **`comet-backup-cli storage buckets`** - List all buckets.

This API requires the Storage Role to be enabled.

Wire format: POST /api/v1/admin/storage/list-buckets with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminstoragelistbuckets-list-all-buckets
- **`comet-backup-cli storage free-space`** - Retrieve available space metrics.

Access to this API may be prevented on a per-administrator basis.
This API requires the Storage Role to be enabled.
This API is only available for top-level administrator accounts, not for Tenant administrator accounts.

Wire format: POST /api/v1/admin/storage/free-space with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 19.12.4 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminstoragefreespace-retrieve-available-space-metrics
- **`comet-backup-cli storage request-vault`** - Request a new Storage Vault on behalf of a user.

This action does not respect the "Prevent creating new Storage Vaults (via Request)" policy setting. New Storage Vaults can be requested regardless of the policy setting.
Prior to Comet 19.8.0, the response type was CometAPIResponseMessage (i.e. no DestinationID field in response).
The StorageProvider must exist for the target user account's organization.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/request-storage-vault with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 18.6.2 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminrequeststoragevault-request-a-new-storage-vault-on-behalf-of-a-user
- **`comet-backup-cli storage vault-providers`** - Get the available options for Requesting a Storage Vault.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/request-storage-vault-providers with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 18.6.2 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminrequeststoragevaultproviders-get-the-available-options-for-requesting-a-storage-vault

### user_groups

User groups and their member accounts

- **`comet-backup-cli user-groups get`** - Retrieve a single user group object.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/user-groups/get with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 24.12.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminusergroupsget-retrieve-a-single-user-group-object
- **`comet-backup-cli user-groups list`** - List all user group names.

For the top-level organization, the API result includes all user groups for all organizations, unless the TargetOrganization parameter is present.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/user-groups/list with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 24.12.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminusergroupslist-list-all-user-group-names
- **`comet-backup-cli user-groups list-full`** - Get all user group objects.

For the top-level organization, the API result includes all user groups for all organizations, unless the TargetOrganization parameter is present.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/user-groups/list-full with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 24.12.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminusergroupslistfull-get-all-user-group-objects

### users

Tenant user accounts on the Comet Server: list, inspect, create, delete, reset password, revoke devices

- **`comet-backup-cli users add`** - Add a new user account.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/add-user with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminadduser-add-a-new-user-account
- **`comet-backup-cli users delete`** - Delete user account.

This does not remove any storage buckets. Unused storage buckets will be cleaned up by the Constellation Role.
Any stored data can not be decrypted without the user profile. Misuse can cause data loss!
This also allows to uninstall software from active devices under the user account
This also removes all job history for the user account.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/delete-user with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindeleteuser-delete-user-account
- **`comet-backup-cli users disable-totp`** - Disable user account 2FA(TOTP) authentication.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/disable-user-totp with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 20.6.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#admindisableusertotp-disable-user-account-2fa-totp-authentication
- **`comet-backup-cli users get`** - Get user account profile.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/get-user-profile with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

The response contains credential material (account password hash, TOTP key, recovery code, and Storage Vault access credentials). Treat the output as secret.

Source: https://docs.cometbackup.com/latest/api/api-methods#admingetuserprofile-get-user-account-profile
- **`comet-backup-cli users list`** - List all user accounts.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/list-users with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminlistusers-list-all-user-accounts
- **`comet-backup-cli users list-full`** - List all user account profiles.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/list-users-full with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

The response contains credential material (account password hash, TOTP key, recovery code, and Storage Vault access credentials). Treat the output as secret.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminlistusersfull-list-all-user-account-profiles
- **`comet-backup-cli users reset-password`** - Reset user account password.

The user account must have a recovery code present. A new replacement recovery code will be generated automatically.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/reset-user-password with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 18.3.9 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminresetuserpassword-reset-user-account-password
- **`comet-backup-cli users revoke-device`** - Revoke device from user account.

It's possible to simply remove the Device section from the user's profile, however, using this dedicated API will also gracefully handle live connections.
This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/revoke-device with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.9.1 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminrevokedevice-revoke-device-from-user-account
- **`comet-backup-cli users set-profile`** - Modify user account profile.

This API requires the Auth Role to be enabled.

Wire format: POST /api/v1/admin/set-user-profile with an application/x-www-form-urlencoded body. The CLI adds the admin credential fields (Username, AuthType, Password or SessionKey) to the body itself.

Requires Comet Server 17.2.0 or later.

Source: https://docs.cometbackup.com/latest/api/api-methods#adminsetuserprofile-modify-user-account-profile


### changes


- **`comet-backup-cli changes`** - Compare the two newest local snapshots: problems opened or resolved, devices and protected items added or removed, storage growth. Run snapshot twice first.


### devices


- **`comet-backup-cli devices stale`** - Find offline devices: registered devices not connected now, ranked by how long since their last backup, flagged when they still carry billable protected items.

- **`comet-backup-cli devices outdated`** - List devices whose Comet client version differs from the server version, with the connection id an online device needs for devices update-software.


### health


- **`comet-backup-cli health`** - See the backup state of every protected item across every tenant in one list, including items that quietly stopped running.


### rerun-failed


- **`comet-backup-cli rerun-failed`** - Plan and, with --yes, re-run failed scheduled backups on devices that are online now; quota, cancelled, already-running, custom-job and offline-device failures are skipped.


### snapshot


- **`comet-backup-cli snapshot`** - Capture the current fleet into a local SQLite store so changes can compare runs.


### triage


- **`comet-backup-cli triage`** - Find out why backups failed: the failed and stuck jobs in a time window (warnings with --include-warnings), with the first error lines from each job log.


### usage


- **`comet-backup-cli usage`** - Get per-tenant storage against quota, billable devices, booster engines, Microsoft 365 accounts and VM guests; these are the server's own counts, not an invoice.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`comet-backup-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`comet-backup-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`comet-backup-cli learnings list`** - Inspect taught rows
- **`comet-backup-cli learnings forget <query>`** - Undo a teach
- **`comet-backup-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`comet-backup-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`comet-backup-cli teach-pattern`** - Install a query/resource template up front
- **`comet-backup-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `COMET_BACKUP_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `comet-backup-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
comet-backup-cli backup-jobs get --job-id 4f1c2a9e-7b3d-4e8a-9c2f-6d5e4b3a2c1d

# JSON for scripting and agents
comet-backup-cli backup-jobs get --job-id 4f1c2a9e-7b3d-4e8a-9c2f-6d5e4b3a2c1d --json
# Filter to specific fields
comet-backup-cli backup-jobs get --job-id 4f1c2a9e-7b3d-4e8a-9c2f-6d5e4b3a2c1d --json --select BackupRuleGUID,CancellationID,Classification

# Dry run  -  show the request without sending
comet-backup-cli backup-jobs get --job-id 4f1c2a9e-7b3d-4e8a-9c2f-6d5e4b3a2c1d --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
comet-backup-cli backup-jobs get --job-id 4f1c2a9e-7b3d-4e8a-9c2f-6d5e4b3a2c1d --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - commands whose help lists `--stdin` accept a JSON object of Comet parameters
- **Local history** - `snapshot` writes a credential-free fleet history to local SQLite and `changes` reads it offline
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` generic failure, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `COMET_BACKUP_SERVER` resolves `{server}`

Base URL: `https://{server}`

## Health Check

```bash
comet-backup-cli doctor
```

Checks configuration and the server URL, and signs in once (`POST /api/v1/admin/meta/version`) to test your credentials. A wrong password counts as a failed sign-in on your Comet Server.

## Configuration

Run `comet-backup-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/comet-backup-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\comet-backup-cli\config.toml` on Windows; `--home`, `COMET_BACKUP_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `COMET_BACKUP_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `COMET_BACKUP_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `COMET_BACKUP_SERVER` | endpoint | Yes, unless `COMET_BACKUP_BASE_URL` is set | Your Comet Server's host and port, for example `backup.example.com:8060`; the CLI connects over https. |
| `COMET_BACKUP_BASE_URL` | config | No | Full https server URL; overrides `COMET_BACKUP_SERVER`. The admin login is never sent over plain http. |
| `COMET_BACKUP_USERNAME` | per_call | Yes | Admin account name (form field Username). |
| `COMET_BACKUP_PASSWORD` | per_call | Yes, unless `COMET_BACKUP_SESSION_KEY` is set | Admin account password, sent in the request body as AuthType=Password. |
| `COMET_BACKUP_SESSION_KEY` | per_call | No | Admin session key from Comet's AdminAccountSessionStart API; takes precedence over the password. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `comet-backup-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Known limits

- Built and tested without access to a live Comet Server: the wire format follows Comet's public API reference and the official Go SDK, and was verified against a local mock. Treat the first run against your server as the real test.
- Personal API tokens (AuthType=Token, Comet 26.8.3+) are not supported; use an admin password or a session key.
- `usage` reports this server's own counts (storage, devices, boosters, Microsoft 365 accounts), not an invoice. Booster engines follow the official comet-munin-plugin list; VMware and Proxmox items are not flagged as boosters.
- `changes` compares local snapshots only; run `comet-backup-cli snapshot` on a schedule first.
- `health` works out overdue runs in each device's own timezone. When a device reports no timezone and its schedule is restricted to certain days or hours, the item is reported with `schedule_unresolved: true` and never flagged overdue; list them with `comet-backup-cli health --state unresolved`.
- `users get`, `users list-full` and the other credential-tier reads print every credential-named field Comet returns (password hashes, TOTP keys, Storage Vault and bucket keys) as `[redacted]`. The fleet commands (`health`, `usage`, `snapshot`, ...) never output those fields.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `comet-backup-cli auth status` to see which credentials are set (values are not printed)
- Run `comet-backup-cli server version --json` to test the login; fix credentials before retrying, since Comet throttles repeated failed logins
**Not found errors (exit code 3)**
- Check the resource ID is correct
- List valid IDs first, for example `comet-backup-cli users list` or `comet-backup-cli backup-jobs recent`

### API-specific
- **HTTP 449 or ERR_TOTP_REQUIRED**  -  The admin account requires 2FA; use a dedicated API admin without 2FA, or start a session and export COMET_BACKUP_SESSION_KEY
- **expected JSON, API returned HTML**  -  COMET_BACKUP_SERVER or COMET_BACKUP_BASE_URL points at the wrong host or port; set COMET_BACKUP_SERVER to host:port (for example backup.example.com:8060) or COMET_BACKUP_BASE_URL to the full https URL (for example https://host:8060)
- **403 with Invalid user credentials**  -  Fix COMET_BACKUP_USERNAME and COMET_BACKUP_PASSWORD before retrying; Comet throttles repeated failed logins
- **changes always returns an empty list**  -  changes compares local snapshots only; run comet-backup-cli snapshot at least twice (for example from cron)
- **x509 certificate error on a self-hosted server**  -  Install a certificate the machine trusts on the Comet Server (Comet's built-in Let's Encrypt on port 443 works); the CLI does not skip TLS verification

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**comet-whmcs-module**](https://github.com/CometBackup/comet-whmcs-module)  -  PHP (7 stars)
- [**comet-go-sdk**](https://github.com/CometBackup/comet-go-sdk)  -  Go (4 stars)
- [**comet-prometheus-exporter**](https://github.com/CometBackup/comet-prometheus-exporter)  -  PHP (1 stars)
- [**ingramcomputers/cometbackup**](https://github.com/ingramcomputers/cometbackup)  -  PowerShell (1 stars)
- [**comet-munin-plugin**](https://github.com/CometBackup/comet-munin-plugin)  -  Python
- [**comet-collectd-exporter**](https://github.com/CometBackup/comet-collectd-exporter)  -  Python
- [**cometctl**](https://github.com/lasseoe/cometctl)  -  Go

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
