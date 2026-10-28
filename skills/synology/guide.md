# Synology DSM + Active Backup for Business CLI

**Every client NAS's Active Backup jobs and storage health, mirrored into one local SQLite database and checked with one command.**

Talks to each Synology NAS directly with its own DSM login and mirrors Active Backup for Business job history plus volume, disk and SMART health into SQLite. After fleet add and fleet sync, fleet status, abb failures and abb stale answer the MSP morning check across every client NAS, with exit code 6 an RMM can alert on.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `synology-cli` and `synology-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.ps1 | iex
   ```
3. Verify: `synology-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=synology). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/synology --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/synology --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `synology-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the synology skill from https://github.com/Servosity/msp-skills/tree/main/skills/synology. The skill defines how its required CLI (`synology-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=synology).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `SYNOLOGY_BASE_URL` (the NAS address, for example `https://192.168.1.50:5001`), `SYNOLOGY_ACCOUNT` and `SYNOLOGY_PASSWORD` when Claude Desktop prompts you. A two-factor account also needs `SYNOLOGY_DEVICE_ID`, or a one-time `SYNOLOGY_OTP_CODE` for the first login.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/synology/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "synology": {
      "command": "synology-mcp",
      "env": {
        "SYNOLOGY_BASE_URL": "https://192.168.1.50:5001",
        "SYNOLOGY_ACCOUNT": "<dsm-account>",
        "SYNOLOGY_PASSWORD": "<dsm-password>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`synology-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `SYNOLOGY_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `SYNOLOGY_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export SYNOLOGY_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
synology-mcp --transport http
```

Set `SYNOLOGY_MCP_READ_ONLY=1` to drop every tool that is not read-only: the NAS writes (`abb_backup`, `abb_cancel`, `session_logout`) and the local writers (`sync`, `fleet_sync`, `export`, `teach`, learnings and playbook writes). Any value other than blank, `0`, `false`, `no` or `off` turns it on.

## Authentication

DSM has no API keys. The CLI logs in with SYNO.API.Auth using SYNOLOGY_ACCOUNT and SYNOLOGY_PASSWORD (sent as a form POST, never stored) against SYNOLOGY_BASE_URL (for example https://192.168.1.50:5001), sends the session id and SynoToken on every call, and logs in again when DSM reports an expired session. Run 'synology-cli auth login' once to test the login and cache the session. For a two-factor account run it with a fresh SYNOLOGY_OTP_CODE: the CLI caches the trusted-device token DSM returns, so the code is not needed again (SYNOLOGY_DEVICE_ID overrides the cached token). Then unset the code: every credential the CLI holds (password, one-time code, device token, session id, bearer token) is withheld from all output in any encoding, so while a 6-digit code is set any output that carries those six digits is withheld. Fleet entries read <prefix>_ACCOUNT, <prefix>_PASSWORD and optional <prefix>_OTP_CODE / <prefix>_DEVICE_ID, and cache the trusted-device token per NAS the same way. DSM's self-signed certificate: pin it with SYNOLOGY_TLS_FINGERPRINT=<sha256> or 'fleet add --tls-fingerprint <sha256>', the value checked in DSM (Control Panel > Security > Certificate); 'fleet add --pin-cert' only displays the fingerprint the NAS presents and saves nothing; SYNOLOGY_SKIP_TLS_VERIFY=1 trusts any certificate on a network you control.

## Quick Start

```bash
# Verify-safe first run; drop --dry-run to see resolved paths and which SYNOLOGY_* env vars are set and to test the DSM login
synology-cli doctor --dry-run

# Logs in with SYNOLOGY_ACCOUNT/SYNOLOGY_PASSWORD and caches the session (add SYNOLOGY_OTP_CODE once for two-factor)
synology-cli auth login

# Live check of one NAS: every Active Backup task and how its last run ended
synology-cli abb tasks --select task_name,last_result.status,last_result.time_end

# Register a client NAS; its login comes from SYNOLOGY_ACME_ACCOUNT and SYNOLOGY_ACME_PASSWORD
synology-cli fleet add acme --url https://nas.acme.example:5001 --env-prefix SYNOLOGY_ACME

# Mirror ABB job history and storage health from every registered NAS
synology-cli fleet sync

# One row per client: backups, stale devices, storage, sync age
synology-cli fleet status

# The morning list of failed, partial and missed backups across clients
synology-cli abb failures --since 24h

```

## Unique Features

### Fleet-wide backup assurance
- **`fleet status`**  -  One row per client NAS with backup outcomes, stale devices, worst storage state and sync age; exits 6 when any NAS is warning, critical or unknown (--no-fail exits 0).

  _Reach for this first when asked whether every client's backups and storage are OK; a NAS that stopped reporting shows UNKNOWN, never OK._

  ```bash
  synology-cli fleet status --agent
  ```
- **`abb failures`**  -  Every Active Backup task across the fleet whose latest run in a window failed, partially succeeded, was canceled, or never ran.

  _Use for the morning backup check instead of opening the ABB portal on each NAS._

  ```bash
  synology-cli abb failures --since 24h --agent
  ```
- **`abb stale`**  -  Protected PCs, servers and VMs whose last successful backup is older than a threshold, or that never succeeded.

  _Catches machines that silently stopped getting good backups, which no single ABB screen flags._

  ```bash
  synology-cli abb stale --older-than 36h --agent
  ```
- **`fleet sync`**  -  Mirror ABB job status and storage health from every NAS registered with fleet add into one local database, each NAS with its own login.

  _Run this before the fleet rollups; one unreachable NAS is recorded and does not abort the run._

  ```bash
  synology-cli fleet sync --agent
  ```
- **`abb unprotected`**  -  Devices and tasks that dropped out of Active Backup protection across the fleet: a machine removed from its task, a task deleted, or a task whose schedule is off while it still holds devices; exits 6 when any exist or a NAS mirror is not synced or ABB stopped answering (--no-fail exits 0).

  _abb stale only sees devices still in a task, so a PC someone quietly removed from backup disappears from every other report; this is where it shows up._

  ```bash
  synology-cli abb unprotected --since 30d --agent
  ```
- **`abb change-rate`**  -  Devices whose newest good backup moved far more data than their own median (a mass file change) or nothing at all for several runs in a row, across the fleet; exits 6 when any device is flagged or a NAS mirror is not synced or ABB stopped answering (--no-fail exits 0).

  _A backup that succeeds while moving forty times its usual data, or zero bytes, reads as a plain success everywhere else._

  ```bash
  synology-cli abb change-rate --since 30d --agent
  ```

### Storage health
- **`fleet health`**  -  Degraded or crashed volumes and pools, disks with bad SMART status, hot disks and nearly full volumes across every NAS, with days-to-full when history exists.

  _Use for the weekly storage review across clients; exit code is non-zero when anything needs attention._

  ```bash
  synology-cli fleet health --full-pct 85 --disk-temp 55 --agent
  ```
- **`fleet capacity`**  -  Every volume on every NAS with used percent, growth per day and days until full from retained storage snapshots, soonest first; exits 6 when a volume fills inside the horizon (default 30 days), a volume's capacity is unknown, or a NAS's storage cannot be read (--no-fail exits 0).

  _fleet health only projects volumes already over the full threshold; a volume at 60 percent that fills in twelve days shows up here first._

  ```bash
  synology-cli fleet capacity --horizon 30d --csv
  ```

### Drill-down
- **`abb triage`**  -  Why one ABB task failed: its newest non-success run, its detail lines and the ABB log from that window, errors first. Live call to one NAS; DSM returns log types and parameters, not message text.

  _Use after abb failures names a task, before re-running it._

  ```bash
  synology-cli abb triage "Office PCs" --nas acme --agent
  ```
- **`abb device`**  -  Find a machine by host name across every NAS and see which task protects it, its last success and its recent runs.

  _Answers the ticket question 'is this machine backed up, and when was the last good one?'_

  ```bash
  synology-cli abb device BOB-LAPTOP --agent
  ```
- **`abb trend`**  -  Per task or device: run count, success rate, median and p90 duration, and an overrun flag when the latest run is far slower than usual.

  _Spots tasks that keep partially succeeding or keep getting slower before they break._

  ```bash
  synology-cli abb trend --since 30d --agent
  ```

## Recipes

### Morning backup check across clients

```bash
synology-cli abb failures --since 24h --agent
```

After fleet sync, list every failed, partial or missed ABB run in the last day across client NAS units.

### Machines with no good backup in 3 days

```bash
synology-cli abb stale --older-than 72h --agent
```

Device-level staleness computed from job history, including devices that never succeeded.

### Narrow a verbose task list

```bash
synology-cli abb tasks --agent --select task_id,task_name,last_result.status,devices.host_name
```

ABB task rows carry dozens of fields; select only the outcome and the protected hosts.

### Why did a task fail

```bash
synology-cli abb triage "Office PCs" --nas acme --agent
```

Newest failed run, its detail lines and the ABB log from that window, errors first.

### Fleet storage review

```bash
synology-cli fleet health --full-pct 85 --agent
```

Degraded volumes, bad SMART disks, hot disks and nearly full volumes on every client NAS.

## Usage

Run `synology-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | `config.toml`, saved profiles and the `fleet.json` NAS registry (env-var names only, never a password) |
| `data` | `credentials.toml`, `data.db`, the `fleet.db` mirror and `session-<scheme>-<host>_<port>-<hash>.json` per NAS (DSM session id, SynoToken and two-factor trusted-device token, mode 0600) |
| `state` | Runtime state: the learn journal and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `SYNOLOGY_CONFIG_DIR`, `SYNOLOGY_DATA_DIR`, `SYNOLOGY_STATE_DIR`, or `SYNOLOGY_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `SYNOLOGY_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export SYNOLOGY_HOME=/srv/synology
synology-cli doctor
```

On Windows (PowerShell):

```powershell
$env:SYNOLOGY_HOME = "C:\ProgramData\synology"
synology-cli doctor
```

Under `SYNOLOGY_HOME=/srv/synology`, the four dirs resolve to `/srv/synology/config`, `/srv/synology/data`, `/srv/synology/state`, and `/srv/synology/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:SYNOLOGY_HOME = "C:\srv\synology"
synology-cli doctor
```

Under `$env:SYNOLOGY_HOME = "C:\srv\synology"`, the four dirs resolve to `C:\srv\synology\config`, `C:\srv\synology\data`, `C:\srv\synology\state`, and `C:\srv\synology\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "synology": {
      "command": "synology-mcp",
      "env": {
        "SYNOLOGY_HOME": "/srv/synology"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `SYNOLOGY_DATA_DIR` overrides an explicit `--home` for that kind. Use `SYNOLOGY_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `SYNOLOGY_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `synology-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `synology-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `synology-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Credentials never reach output:** a line on stdout or stderr, an MCP result, an export, a `--deliver` file, a receipt or a stored row that carries a credential the CLI holds (in any encoding: escaped, base64, hex, nested) is withheld whole, with a notice in its place, and a NAS answer that carries one is withheld as one unit. There is no length floor, so a very short password matches ordinary text and withholds most output, or the CLI refuses to start. A one-time code stays held while it is set, so an answer containing that number is withheld: after the first login, unset `SYNOLOGY_OTP_CODE` or `<prefix>_OTP_CODE`.
- **Fleet mirror and sessions:** The fleet mirror (`fleet.db`: client NAS names, host names and error lines) and the per-NAS DSM session files are outside `local-data purge`. `SYNOLOGY_NO_LOCAL_DATA=1` stops `fleet sync` writing the mirror (it refuses to run), and the per-NAS session files are still kept. `fleet remove <name>` purges one NAS's mirror rows, deleting `fleet.db` removes the whole mirror, and `auth logout --all` forgets every cached session.
- **Run without a local copy:** set `SYNOLOGY_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync`, `workflow archive` and `fleet sync` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `SYNOLOGY_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### abb

Active Backup for Business tasks, job results, logs and storage

- **`synology-cli abb backup`** - Start a backup now for one or more tasks
- **`synology-cli abb cancel`** - Cancel the running backup of one or more tasks
- **`synology-cli abb hypervisors`** - List the VMware and Hyper-V hypervisors registered in Active Backup for Business
- **`synology-cli abb logs`** - Read the Active Backup for Business activity log for the package, tasks and devices
- **`synology-cli abb result-detail`** - Show the detail log lines (errors, warnings, skipped paths) of one job result
- **`synology-cli abb results`** - List job results across tasks (backups plus version deletions and restores; filter {"job_action":1} for backups only) with status, times and error counts
- **`synology-cli abb settings`** - Show Active Backup for Business package settings and certificate information
- **`synology-cli abb storage`** - List Active Backup for Business storage destinations with dedup size, device counts and volume
- **`synology-cli abb tasks`** - List Active Backup for Business tasks with their last job result, protected devices, progress and next run time
- **`synology-cli abb transfers`** - List protected devices with every transfer (bytes, status, start, end) inside a time window

### folder

Shared folders

- **`synology-cli folder`** - List shared folders, including the ActiveBackupforBusiness destination share

### group

Local DSM groups

- **`synology-cli group list`** - List local DSM groups on the NAS, for example to find the administrators group
- **`synology-cli group members`** - List the accounts in one group, for example administrators

### hyperbackup

Hyper Backup tasks (the NAS's own backup to another NAS, USB or cloud)

- **`synology-cli hyperbackup task-status`** - Show one Hyper Backup task's last result, last success, next run and health state without contacting the destination
- **`synology-cli hyperbackup tasks`** - List Hyper Backup tasks (the NAS's own backups to another NAS, USB or cloud) with their destination type

### log

DSM system log

- **`synology-cli log`** - Read the DSM system log with level, date range and keyword filters

### package

Installed DSM packages

- **`synology-cli package`** - List installed packages with version and running state; confirms Active Backup for Business is installed

### session

DSM API discovery and session control

- **`synology-cli session apis`** - List every API namespace this NAS exposes with its version range and CGI path; shows whether Active Backup for Business is installed
- **`synology-cli session logout`** - Invalidate the current DSM session id so it can no longer be used on this NAS

### storage

Volumes, storage pools, disks and SMART data

- **`synology-cli storage disks`** - List physical disks with model, serial number, temperature and SMART health status
- **`synology-cli storage overview`** - Show Storage Manager's disks, storage pools and volumes with their status in one call
- **`synology-cli storage pools`** - List storage pools with RAID type, status and member disks
- **`synology-cli storage smart`** - Show the SMART attribute table for one disk
- **`synology-cli storage smart-schedule`** - List the scheduled SMART tests (quick or extended) configured for the NAS's disks
- **`synology-cli storage volumes`** - List volumes with status, size, used space and file system

### system

DSM system identity, health and load

- **`synology-cli system dsm`** - Show DSM product identity (model, version string, serial) as the web UI reports it
- **`synology-cli system get`** - Show model, serial number, DSM version, uptime and system temperature
- **`synology-cli system health-check`** - Show the aggregate system health verdict the DSM dashboard shows
- **`synology-cli system network`** - Show the NAS hostname, DNS servers, default gateway and network interface settings
- **`synology-cli system ups`** - Show the state of the uninterruptible power supply (UPS) attached to the NAS, if any
- **`synology-cli system utilization`** - Show current CPU, memory, disk and network utilization

### user

Local DSM user accounts

- **`synology-cli user`** - List local user accounts, for example to audit who can log in to a client NAS


### abb


- **`synology-cli abb failures`** - Every Active Backup task across the fleet whose latest run in a window failed, partially succeeded, was canceled, or never ran.

- **`synology-cli abb stale`** - Protected PCs, servers and VMs whose last successful backup is older than a threshold, or that never succeeded.

- **`synology-cli abb triage`** - Why one ABB task failed: its newest non-success run, its detail lines and the ABB log from that window, errors first. Live call to one NAS; DSM returns log types and parameters, not message text.

- **`synology-cli abb device`** - Find a machine by host name across every NAS and see which task protects it, its last success and its recent runs.

- **`synology-cli abb trend`** - Per task or device: run count, success rate, median and p90 duration, and an overrun flag when the latest run is far slower than usual.

- **`synology-cli abb unprotected`** - Devices and tasks that dropped out of Active Backup protection across the fleet: a machine removed from its task, a task deleted, or a task whose schedule is off while it still holds devices; exits 6 when any exist (--no-fail exits 0).

- **`synology-cli abb change-rate`** - Devices whose newest good backup moved far more data than their own median (a mass file change) or nothing at all for several runs in a row, across the fleet; exits 6 when any device is flagged.


### fleet


- **`synology-cli fleet status`** - One row per client NAS with backup outcomes, stale devices, worst storage state and sync age; exits 6 when any NAS is warning, critical or unknown (--no-fail exits 0).

- **`synology-cli fleet sync`** - Mirror ABB job status and storage health from every NAS registered with fleet add into one local database, each NAS with its own login.

- **`synology-cli fleet health`** - Degraded or crashed volumes and pools, disks with bad SMART status, hot disks and nearly full volumes across every NAS, with days-to-full when history exists.

- **`synology-cli fleet capacity`** - Every volume on every NAS with used percent, growth per day and days until full from retained storage snapshots, soonest first; exits 6 when a volume will fill inside the horizon (default 30 days).




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`synology-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`synology-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`synology-cli learnings list`** - Inspect taught rows
- **`synology-cli learnings forget <query>`** - Undo a teach
- **`synology-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`synology-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`synology-cli teach-pattern`** - Install a query/resource template up front
- **`synology-cli teach-lookup`** - Add an entity mapping (e.g. a NAS nickname or client alias) for pattern substitution

Pass `--no-learn` or set `SYNOLOGY_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `synology-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
synology-cli folder

# JSON for scripting and agents
synology-cli folder --json
# Filter to specific fields by name
synology-cli abb tasks --json --select task_name,last_result.status

# Dry run  -  show the request without sending
synology-cli folder --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
synology-cli folder --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select task_name,last_result.status` returns only the fields you need (dotted paths reach nested fields)
- **Previewable** - `--dry-run` shows the request without sending
- **No confirmation prompts** - `abb backup`, `abb cancel` and `session logout` act on the NAS immediately with no confirmation prompt on the CLI; preview with `--dry-run` first. Over MCP the same three tools (`abb_backup`, `abb_cancel`, `session_logout`) return a preview and send nothing until called with `"confirm": true`. `fleet add` and `fleet remove` change only the local registry and are not MCP tools.
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error (`fleet sync`: a NAS failed to sync), `6` findings (a fleet/abb report ran and something needs attention: failed or missed backups, stale devices, storage problems, a NAS that is not synced; `--no-fail` exits 0), `7` rate limited, `10` config error (`fleet` and `abb` reports: no NAS configured).

## Health Check

```bash
synology-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `synology-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/synology-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\synology-cli\config.toml` on Windows; `--home`, `SYNOLOGY_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `SYNOLOGY_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `SYNOLOGY_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SYNOLOGY_ACCOUNT` | auth_flow_input | Yes | DSM account name used for the SYNO.API.Auth login |
| `SYNOLOGY_BASE_URL` | config | Yes | NAS address with scheme and port, for example `https://192.168.1.50:5001` (DSM HTTPS 5001; a plain `http://` address gets no credential, localhost included) |
| `SYNOLOGY_PASSWORD` | auth_flow_input | Yes | DSM account password; sent as a form POST, never stored |
| `SYNOLOGY_DEVICE_ID` | auth_flow_input | No | Two-factor trusted-device token; optional override for the token `auth login` caches |
| `SYNOLOGY_OTP_CODE` | auth_flow_input | No | One-time two-factor code for the first `auth login` |
| `SYNOLOGY_TLS_FINGERPRINT` | config | No | SHA-256 of the NAS certificate to pin (self-signed DSM certificates) |
| `SYNOLOGY_SKIP_TLS_VERIFY` | config | No | `1` trusts any certificate; only on a network you control |
| `SYNOLOGY_<NAME>_ACCOUNT`, `_PASSWORD`, `_OTP_CODE`, `_DEVICE_ID` | fleet | Per NAS | Login for a NAS registered with `fleet add <name>` (prefix set by `--env-prefix`) |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `synology-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `synology-cli doctor` to check credentials
- Verify the environment variable is set: `echo $SYNOLOGY_ACCOUNT`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run `synology-cli abb tasks` or `synology-cli fleet list` to see valid task ids and NAS names

### API-specific
- **DSM error 403 at login (a one-time code is required)**  -  Run 'synology-cli auth login' once with a fresh 6-digit code in SYNOLOGY_OTP_CODE (fleet: <prefix>_OTP_CODE and 'fleet sync'); the CLI caches the trusted-device token, then unset the OTP
- **DSM error 102 on abb commands**  -  Active Backup for Business is not installed on that NAS; check with 'synology-cli package'
- **DSM error 105 (insufficient permission)**  -  Use an account in the DSM administrators group: Active Backup for Business management and Storage Manager calls need administrator rights
- **does not answer as a DSM WebAPI ... the password was not sent** (or **... the session was not sent**)  -  The address did not answer `SYNO.API.Info` the way DSM does, so no login was attempted. Check SYNOLOGY_BASE_URL (or the fleet entry's --url): use the https:// address if DSM redirects http:// to https:// (redirects are never followed), and make a reverse proxy in front of DSM pass `/webapi/query.cgi`
- **DSM login: refused before sending ... the password was not sent, with a proxy variable set**  -  A proxy URL with a user or password must be `https://`: over plain `http://` the proxy's password would cross the network in clear, so the CLI sends nothing. Use a TLS proxy or one with no password.
- **the NAS certificate does not match the pinned SHA-256 fingerprint**  -  The NAS renewed its certificate. Check the fingerprint the error names in DSM (Control Panel > Security > Certificate), then re-pin with the value you checked: 'synology-cli fleet add <name> --tls-fingerprint <sha256>' (keeps the address, settings and mirrored history) or set SYNOLOGY_TLS_FINGERPRINT to it. A pin mismatch is also what an interception looks like: never re-pin without that check
- **the NAS certificate failed verification**  -  Most NAS units use a self-signed certificate: pin it with 'fleet add <name> --url ... --tls-fingerprint <sha256>' using the value DSM shows (Control Panel > Security > Certificate; '--pin-cert' instead only displays the presented fingerprint to compare) (fleet) or SYNOLOGY_TLS_FINGERPRINT=<sha256> (single NAS); SYNOLOGY_SKIP_TLS_VERIFY=1 trusts any certificate on a network you control
- **fleet status shows UNKNOWN for a NAS**  -  Its last fleet sync failed or is old; run 'synology-cli fleet sync' and read that NAS's sync_error
- **abb failures or fleet status lists a weekly task as no_run_in_window**  -  The default window is 24h (failures) / 26h (status); widen it for less frequent tasks, for example 'synology-cli abb failures --since 8d'
- **fleet commands exit 10 with 'no NAS configured'**  -  Register a NAS with 'synology-cli fleet add <name> --url https://<nas>:5001' (or set SYNOLOGY_BASE_URL, SYNOLOGY_ACCOUNT, SYNOLOGY_PASSWORD), then run 'synology-cli fleet sync'
- **A fleet report shows nas_not_synced**  -  That NAS has no fresh mirror (never synced, last sync failed, or older than --max-sync-age); run 'synology-cli fleet sync --nas <name>' and read its error

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**N4S4/synology-api**](https://github.com/N4S4/synology-api)  -  Python (597 stars)
- [**rafalr100/synology-mcp**](https://github.com/rafalr100/synology-mcp)  -  Python (5 stars)
- [**linuser/Zabbix-Active-Backup-for-Business-Monitoring**](https://github.com/linuser/Zabbix-Active-Backup-for-Business-Monitoring)  -  Shell (4 stars)
- [**printing-press-library synology-cli**](https://github.com/mvanhorn/printing-press-library/tree/main/library/devices/synology)  -  Go
- [**sunnyvale-it/synology-abb-prometheus-exporter**](https://github.com/sunnyvale-it/synology-abb-prometheus-exporter)  -  Go

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
