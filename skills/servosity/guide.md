# Servosity CLI

**The first MSP-fleet CLI for backup. Every Servosity API endpoint as a typed command, plus a local mirror that lets you ask questions the dashboard can't  -  across your whole book of clients.**

No competitor in the MSP backup space ships a fleet-wide CLI. Reach for servosity-cli when you need to triage attention across every client at once, generate the backup section of a QBR in 30 seconds, watch every restore queue during DR from one terminal, or reconcile your Servosity bill against what you're invoicing clients. Every response is agent-native: --json, --select, --csv, --dry-run, typed exit codes.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `servosity-cli` and `servosity-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=servosity). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/servosity --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/servosity --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `servosity-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the servosity skill from https://github.com/Servosity/msp-skills/tree/main/skills/servosity. The skill defines how its required CLI (`servosity-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=servosity).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `SERVOSITY_MSP_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/servosity/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "servosity": {
      "command": "servosity-mcp",
      "env": {
        "SERVOSITY_MSP_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`servosity-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `SERVOSITY_MSP_MCP_HTTP_TOKEN` is set, and every request must send `Authorization: Bearer <token>`. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`.

```bash
export SERVOSITY_MSP_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
servosity-mcp --transport http
```

Set `SERVOSITY_MSP_MCP_READ_ONLY=1` (exactly `1`) to drop every tool not marked read-only and make the raw execute tool refuse state-changing calls. The local-write learn-loop tools (`recall`, `teach` and the rest) are dropped too.

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

## Usage

Run `servosity-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `SERVOSITY_MSP_CONFIG_DIR`, `SERVOSITY_MSP_DATA_DIR`, `SERVOSITY_MSP_STATE_DIR`, or `SERVOSITY_MSP_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `SERVOSITY_MSP_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export SERVOSITY_MSP_HOME=/srv/servosity
servosity-cli doctor
```

On Windows (PowerShell):

```powershell
$env:SERVOSITY_MSP_HOME = "C:\ProgramData\servosity"
servosity-cli doctor
```

Under `SERVOSITY_MSP_HOME=/srv/servosity`, the four dirs resolve to `/srv/servosity/config`, `/srv/servosity/data`, `/srv/servosity/state`, and `/srv/servosity/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:SERVOSITY_MSP_HOME = "C:\srv\servosity"
servosity-cli doctor
```

Under `$env:SERVOSITY_MSP_HOME = "C:\srv\servosity"`, the four dirs resolve to `C:\srv\servosity\config`, `C:\srv\servosity\data`, `C:\srv\servosity\state`, and `C:\srv\servosity\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `SERVOSITY_MSP_DATA_DIR` overrides an explicit `--home` for that kind. Use `SERVOSITY_MSP_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `SERVOSITY_MSP_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `servosity-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `servosity-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `servosity-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `SERVOSITY_MSP_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `SERVOSITY_MSP_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

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

- **`servosity-cli backup-job-report <backup_id> <backup_set_id> <backup_job_id> <backup_destination_id>`** - View detailed backup report for a backup job and destination.

### backup-job-report-summary

Manage backup job report summary

- **`servosity-cli backup-job-report-summary <backup_id> <backup_set_id> <backup_job_id> <backup_destination_id>`** - View summary backup report for a backup job and destination.

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
- **`servosity-cli current-user api-token-list --reveal-token`** - Retrieve the live API token with explicit human consent. This sensitive command is excluded from MCP and sync.

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

### fully_managed_companies

Manage fully managed companies

- **`servosity-cli fully-managed-companies`** - List fully-managed companies.

### issue-comments

Manage issue comments

- **`servosity-cli issue-comments delete`** - Delete
- **`servosity-cli issue-comments update`** - Update

### issues

Manage issues

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


### attention


- **`servosity-cli attention`** - One screen across your whole book of clients. Merges open issues, stale backups into a per-company ranked view, then persists the result so tomorrow's drift command can compare.


### backup-facts


- **`servosity-cli backup-facts`** - Unified view across Servosity's three backup engines (classic, restic, DR) for one company or all. Engine, ID, hostname, last_successful_at, state, and freshness-derived health  -  joined from three local store tables into one table.


### bill


- **`servosity-cli bill`** - Pull the MSP's monthly Servosity bill and reconcile it against a CSV of what the MSP invoices their clients. Flags clients Servosity bills you for that you have not invoiced, and clients you invoice that have no Servosity backups.


### drift


- **`servosity-cli drift`** - Diff two snapshots the CLI collected  -  show which companies got worse, which recovered, and which are new since a past anchor. Default compares yesterday-to-now on the attention metric.


### email-draft


- **`servosity-cli email-draft`** - Generate ready-to-paste follow-up email bodies for every client with a stale backup, filled from the local store (client name, hosts, days stale, last success).


### failure-clusters


- **`servosity-cli failure-clusters`** - Pull this month's restic and DR backup-failure text for every client, normalize away IDs, paths, numbers and timestamps, and group identical root causes, ranked by how many hosts and clients each one hits.


### fleet-health


- **`servosity-cli fleet-health`** - One fleet-wide scorecard: share of backups with a success in the last 24h, companies with stale backups, and open issues, with week-over-week deltas.


### qbr


- **`servosity-cli qbr`** - Generate the backup section of a client's Quarterly Business Review as Markdown, HTML, or PDF. Backup success (restic/DR backups with a successful run in the 7 days before quarter end), restore tests run this quarter, coverage map across all three engines, open issues, storage trend.


### qbr-all


- **`servosity-cli qbr-all`** - Generate every client's QBR backup report in one pass, one file per company.


### restore-queue


- **`servosity-cli restore-queue watch`** - Watch every active company's restore queue across the book during a DR event. Polls each company periodically and prints diffs since the last tick.


### retention-audit


- **`servosity-cli retention-audit`** - Every protected host across the classic, restic and DR engines whose retention tier is below a required minimum, or that is not eligible for the Servosity recovery guarantee, grouped by client. Reads the local store; no live calls.


### stale-backups


- **`servosity-cli stale-backups`** - Slice the stale-backup-sets report by company, age window, and backup engine  -  entirely offline once cached. Use --refresh to repull from the API.


### storage-trend


- **`servosity-cli storage-trend`** - Linear-regression forecast of when a specific client will hit a capacity threshold. Reads the historical storage_bytes time series from local snapshots; with --snapshot, persists a new measurement for future runs.


### triage


- **`servosity-cli triage`** - List open issues with filters, then batch-mutate them (ignore / archive / reactivate / comment) in one invocation with --dry-run support and typed exit codes.


### unprovisioned


- **`servosity-cli unprovisioned`** - List agents installed on client machines but not yet pulling backups, ranked by client. Surfaces lost revenue from incomplete onboardings.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`servosity-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`servosity-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`servosity-cli learnings list`** - Inspect taught rows
- **`servosity-cli learnings forget <query>`** - Undo a teach
- **`servosity-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`servosity-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`servosity-cli teach-pattern`** - Install a query/resource template up front
- **`servosity-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `SERVOSITY_MSP_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `servosity-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
servosity-cli agent-login list

# JSON for scripting and agents
servosity-cli agent-login list --json
# Filter to specific fields by name
servosity-cli agent-login list --json --select <field>[,<field>...]

# Dry run  -  show the request without sending
servosity-cli agent-login list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
servosity-cli agent-login list --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

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

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
servosity-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `servosity-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/servosity-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\servosity-cli\config.toml` on Windows; `--home`, `SERVOSITY_MSP_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `SERVOSITY_MSP_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `SERVOSITY_MSP_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

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
