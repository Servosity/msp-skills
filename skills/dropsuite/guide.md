# Dropsuite CLI

**Every Dropsuite (NinjaOne SaaS Backup) read, plus fleet-wide backup health, coverage gaps and billing reconciliation from one reseller credential.**

Dropsuite's portal and API work one client at a time, with a different token per organization. This CLI resolves every organization's token from your reseller credential, so health, coverage, billing and hygiene answer for the whole fleet in one command. Health snapshots and a restore-test ledger build a dated evidence trail the API does not keep. Built from Dropsuite's public API reference and tested against a local mock; it has not yet been confirmed against a live partner tenant.

Learn more at [Dropsuite](https://dropsuite.com).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `dropsuite-cli` and `dropsuite-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.ps1 | iex
   ```
3. Verify: `dropsuite-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=dropsuite). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/dropsuite --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/dropsuite --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `dropsuite-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the dropsuite skill from https://github.com/Servosity/msp-skills/tree/main/skills/dropsuite. The skill defines how its required CLI (`dropsuite-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=dropsuite).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `DROPSUITE_RESELLER_TOKEN` and `DROPSUITE_ACCESS_TOKEN` (and `DROPSUITE_HOST` if your partner host is not dropsuite.us) when Claude Desktop prompts you. The per-organization tools read one client's data: set `DROPSUITE_ORG` (organization id or owner email) in the server environment to point them at that client.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/dropsuite/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "dropsuite": {
      "command": "dropsuite-mcp",
      "env": {
        "DROPSUITE_HOST": "dropsuite.us",
        "DROPSUITE_RESELLER_TOKEN": "<reseller-token>",
        "DROPSUITE_ACCESS_TOKEN": "<authentication-token>"
      }
    }
  }
}
```

</details>

## Authentication

Every call sends two headers from Settings > API Settings: X-Reseller-Token (DROPSUITE_RESELLER_TOKEN) and X-Access-Token (DROPSUITE_ACCESS_TOKEN, your reseller Authentication Token). GET /users and GET /users/{id} take that reseller token. Everything else (backup data such as accounts and onedrives, and an organization's tenants, delegated users and retention policies) takes one organization's own token: pass --org <organization id or owner email> and the CLI resolves it from the organizations list, or set DROPSUITE_ORG to route an MCP server or shell to one organization. The fleet commands (health, coverage, billing, hygiene, locate, evidence) resolve every organization's token themselves and need the reseller token. The API URL is per partner: set DROPSUITE_HOST to its host (default dropsuite.us), or DROPSUITE_BASE_URL to the whole URL. The CLI only reads, except that portal get-login-link, organizations get-support-sso-link, locate --sso-link and evidence --with-sso-link mint a portal sign-in URL (a login credential; a new organization link invalidates the previous one). Every token the CLI holds (both reseller tokens, and each organization token it reads from GET /users) is withheld from all output, files and the local store in any encoding; there is no response cache; and requests, redirects included, go only to the configured API host. Over MCP, the sign-in link calls return a preview until confirm is true.

## Quick Start

```bash
# Check config, both tokens and that the partner host answers
dropsuite-cli doctor

# List every client organization your reseller credential can see (confirms the reseller token works)
dropsuite-cli organizations list --agent --select id,email,seats_used,seats_available

# Read one client's mailboxes; --org resolves that organization's token
dropsuite-cli accounts list --org 26561-14 --agent

# Fleet-wide failed, stuck and stale backups in one run (also saves a snapshot for drift and evidence)
dropsuite-cli health --stale-after 48h --failing --agent

# Tenant mailboxes that are not backed up yet, with coverage percent per client
dropsuite-cli coverage --agent

```

## Unique Features

### Fleet-wide answers from one credential
- **`health`**  -  See every failed, stuck or stale mailbox, OneDrive, SharePoint site, Team, shared drive and Google Drive across your active clients in one run.

  _Reach for this for the morning backup sweep or any 'is anything failing across my clients' question; add --exit-code for Nagios-style exit codes._

  ```bash
  dropsuite-cli health --stale-after 48h --failing --agent
  ```
- **`coverage`**  -  List mailboxes that exist in each client's Microsoft 365 or Google Workspace tenant but are not added to backup, with coverage percent per client.

  _Use this to answer 'are all my users backed up?' for an insurer, QBR or new-hire check._

  ```bash
  dropsuite-cli coverage --agent
  ```
- **`billing`**  -  Compare each client's seat counters and shared mailboxes with what your PSA invoices, and flag over-allocation and suspended clients still holding seats.

  _Use this for month-end billing true-up instead of copying seat numbers client by client._

  ```bash
  dropsuite-cli billing --invoice psa-seats.csv --agent
  ```
- **`hygiene`**  -  Find deactivated or deleted mailboxes still holding storage and invitations nobody accepted, across every client.

  _Use this for the weekly offboarding pass and storage cleanup._

  ```bash
  dropsuite-cli hygiene --older-than 30d --agent
  ```
- **`locate`**  -  Find which client owns an email address, whether its mailbox and OneDrive are protected, and when they last backed up.

  _Use this for 'is this person backed up?' or restore tickets when you do not know which client they belong to._

  ```bash
  dropsuite-cli locate jane@example.com --agent
  ```

### Evidence that compounds locally
- **`evidence`**  -  Produce one client's dated proof of protection: coverage, last successful backups, item counts, retention policies, freshness history and recorded restore tests.

  _Use this for audit, cyber-insurance or QBR evidence; it never performs a restore._

  ```bash
  dropsuite-cli evidence --org 26561-14 --window 90d --markdown
  ```
- **`restore-test`**  -  Record the result of a restore test you performed in the portal and see which clients are due for one.

  _Use this to schedule and prove restore testing across clients; it records results, it does not restore, and due reads organizations from earlier health or sync runs._

  ```bash
  dropsuite-cli restore-test due --every 90d --pick --agent
  ```
- **`drift`**  -  See what changed between health runs: objects that left backup, backups that stopped advancing and item-count drops.

  _Use this to catch silent regressions that a single status check misses; it needs at least two earlier health runs._

  ```bash
  dropsuite-cli drift --since 7d --agent
  ```

## Recipes

### Morning backup sweep

```bash
dropsuite-cli health --stale-after 24h --failing --agent
```

Graded rows for every failing, stuck or stale object across active clients; add --exit-code for monitoring (2 = crit, 3 = unknown, 1 = warn).

### Seat counters for every client

```bash
dropsuite-cli organizations list --agent --select id,email,seats_used,seats_available,customer_deactivated
```

Narrows the organization records to their seat counters; billing adds active, required and deactivated seats and shared mailboxes.

### Month-end billing true-up

```bash
dropsuite-cli billing --invoice psa-seats.csv --flagged-only --agent
```

The CSV needs a key column (external_id, org_id, id or email) and a quantity column; pick the billed counter with --billable-field (default seats_used, which Dropsuite does not document as the billed one).

### Insurance evidence for one client

```bash
dropsuite-cli evidence --org 26561-14 --window 90d --markdown
```

Dated proof of coverage, last backups, retention and recorded restore tests; freshness history needs earlier health runs.

### Log a restore test

```bash
dropsuite-cli restore-test record --org 26561-14 --object jane@example.com --kind mailbox --result pass --note "restored 3 messages"
```

Records a restore you performed in the portal; restore-test due --every 90d --pick lists clients that need one.

## Usage

Run `dropsuite-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, and other auth sidecars |
| `state` | Runtime state such as persisted queries and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `DROPSUITE_CONFIG_DIR`, `DROPSUITE_DATA_DIR`, `DROPSUITE_STATE_DIR`, or `DROPSUITE_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `DROPSUITE_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export DROPSUITE_HOME=/srv/dropsuite
dropsuite-cli doctor
```

On Windows (PowerShell):

```powershell
$env:DROPSUITE_HOME = "C:\ProgramData\dropsuite"
dropsuite-cli doctor
```

Under `DROPSUITE_HOME=/srv/dropsuite`, the four dirs resolve to `/srv/dropsuite/config`, `/srv/dropsuite/data`, `/srv/dropsuite/state`, and `/srv/dropsuite/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:DROPSUITE_HOME = "C:\srv\dropsuite"
dropsuite-cli doctor
```

Under `$env:DROPSUITE_HOME = "C:\srv\dropsuite"`, the four dirs resolve to `C:\srv\dropsuite\config`, `C:\srv\dropsuite\data`, `C:\srv\dropsuite\state`, and `C:\srv\dropsuite\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "dropsuite": {
      "command": "dropsuite-mcp",
      "env": {
        "DROPSUITE_HOME": "/srv/dropsuite"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `DROPSUITE_DATA_DIR` overrides an explicit `--home` for that kind. Use `DROPSUITE_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `DROPSUITE_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `dropsuite-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. The response cache is off: no API response is written under the cache directory (Dropsuite's organization list carries every organization's token). Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `dropsuite-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `dropsuite-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `DROPSUITE_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `DROPSUITE_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### accounts

An organization's backed-up mailboxes (needs `--org`)

- **`dropsuite-cli accounts list`** - List an organization's backed-up mailboxes with their backup status and last backup. Wire format: GET /api/accounts with optional email, username and backup_status query filters, sent with the organization's own token (use --org); the response is a bare JSON array. Each row has last_backup, current_backup_status (observed values Completed, Running, Preparing Backup, Connecting, Connection Failed), storage, msg_count and deactivated_since; the nested user object carries masked credentials.
- **`dropsuite-cli accounts list-connection-failures`** - List an organization's mailboxes whose backup cannot connect to the mail server. Wire format: GET /api/accounts/connection_failures; the response is an object with result_set, pagination (current_page, per_page, total_entries, total_pages) and api_version.
- **`dropsuite-cli accounts list-deactivated`** - List an organization's mailboxes whose backup is deactivated. Wire format: GET /api/accounts/deactivated_accounts with an optional email query filter; the response is a bare JSON array with deactivated_since set.
- **`dropsuite-cli accounts list-deleted`** - List an organization's mailboxes that were deleted from backup. Wire format: GET /api/accounts/deleted_accounts with an optional email query filter; the response is a bare JSON array with flg_deleted true.
- **`dropsuite-cli accounts list-invites`** - Show the pending backup invitation sent to a mailbox owner. It includes who requested it, when it was last sent and whether it expired. Wire format: GET /api/accounts/invites with an optional email query filter; the vendor's sample response is a single invite object.

### calendars

List calendars (needs `--org`)

- **`dropsuite-cli calendars`** - List an organization's calendar backups per mailbox. Wire format: GET /api/calendars with optional email, page and backup_status query parameters (at most 25 rows per page); the response is a bare JSON array with calendar_schedule_count, last_backup and current_backup_status.

### contacts

List contacts (needs `--org`)

- **`dropsuite-cli contacts`** - List an organization's contact backups per mailbox. Wire format: GET /api/contacts with optional email, page and backup_status query parameters (at most 25 rows per page); the response is a bare JSON array with contact_count, last_backup and current_backup_status.

### google-drives

List google drives (needs `--org`)

- **`dropsuite-cli google-drives`** - List an organization's backed-up Google Drives. Wire format: GET /api/googledrives with an optional page query parameter; the response is a bare JSON array with file_count, storage, last_backup and drive type.

### journals

An organization's journal mailboxes (needs `--org`)

- **`dropsuite-cli journals get`** - Get the archiver journal mailbox of an organization (Archiver product only). Wire format: GET /api/journals; the response is an object with id and email.
- **`dropsuite-cli journals get-ndr`** - Get the NDR (non-delivery report) journal mailbox of an organization. Wire format: GET /api/ndr_journals; the response is an object with id and email.

### onedrives

List onedrives (needs `--org`)

- **`dropsuite-cli onedrives`** - List an organization's backed-up OneDrive accounts with file counts and last backup. Wire format: GET /api/onedrives with optional email, page and backup_status query parameters (at most 25 rows per page); the response is a bare JSON array with file_count, storage, last_backup, last_activity and current_backup_status.

### organizations

List and get client organizations (subscription, tenants, delegated users, retention)

- **`dropsuite-cli organizations get <id>`** - Get one organization's subscription, including the seat counters used for billing. Wire format: GET /api/users/{id} with the reseller's own Authentication Token. Fields include seats_used, seats_available, active_seats, required_seats, deactivated_seats, paid_shared_mailboxes, free_shared_mailboxes and auto_license. Ids look like 26561-14 on current deployments.
- **`dropsuite-cli organizations get-support-sso-link --email <user email>`** - Generate a portal sign-in link for one user email, using the reseller's own token. Wire format: GET /api/users/generate_support_sso_link with a required email query parameter; the response is an object with redirect_url. The URL is a login credential; treat it as a secret.
- **`dropsuite-cli organizations list`** - List every client organization (tenant user) under the reseller, with seats and storage counters. Wire format: GET /api/users with optional email and external_id query filters, sent with the reseller's own Authentication Token; the response is a bare JSON array. Dropsuite calls organizations users. The authentication_token and reseller_token fields are credentials and are masked in output.
- **`dropsuite-cli organizations list-delegated-users <id>`** - List an organization's delegated portal users and their roles. Delegated users can sign in to the end-user portal although their own mailbox is not backed up; each row has role, login_enabled and activated. Wire format: GET /api/users/{id}/delegated with page and per_page query parameters, sent with that organization's own token (the CLI resolves it); the response is a pagination envelope with a data array. dropsuite.us serves it; the older regional hosts answer HTML 404.
- **`dropsuite-cli organizations list-retention-policies <id>`** - List an organization's retention policies. Each policy says how long items are kept (period_number with period_unit days, months or years) and which items it covers. Wire format: GET /api/users/{id}/retention_policies with type, page and per_page query parameters, sent with that organization's own token (the CLI resolves it); the response is a pagination envelope with a data array. dropsuite.us serves it; the older regional hosts answer HTML 404.
- **`dropsuite-cli organizations list-tenant-accounts <id> <tenant_id> --type m365`** - List a tenant's mailboxes and whether each is backed up, available to add, or excluded. Use status=available to find unprotected mailboxes. Wire format: GET /api/users/{id}/tenants/{tenant_id}/accounts with a required type (m365 or gws) and optional status, page and per_page query parameters, sent with that organization's own token (the CLI resolves it); the response is a pagination envelope with a data array. The vendor reference lists this route without the tenant segment, but the live route includes it. dropsuite.us serves it; the older regional hosts answer HTML 404.
- **`dropsuite-cli organizations list-tenants <id>`** - List an organization's Microsoft 365 and Google Workspace tenants. Each tenant has its domain, a status code and total_users. Wire format: GET /api/users/{id}/tenants with type (m365 or gws), page and per_page query parameters, sent with that organization's own token (the CLI resolves it); the response is a pagination envelope with a data array. Status codes: 0 queued, 1 and 13 recreate application, 7 completed. dropsuite.us serves it; the older regional hosts answer HTML 404.

### portal

An organization's portal: sub-users, sign-in link, hosted Exchange status (needs `--org`)

- **`dropsuite-cli portal get-hosted-exchange-status --exchange-request-id <id>`** - Get the status of a hosted Exchange PowerShell connection request. The request id comes from the vendor's POST /users/hosted_exchange, which this read-only CLI does not call. Wire format: GET /api/users/hosted_exchange with a required exchange_request_id query parameter.
- **`dropsuite-cli portal get-login-link`** - Generate a single-use portal sign-in URL for an organization, for example to run a restore test. The link expires after 30 minutes, works once, and each call invalidates the previous link; it is a login credential. Wire format: GET /api/users/generate_redirect_link with an optional role query parameter; the response is an object with redirect_url.
- **`dropsuite-cli portal list-sub-users`** - List the sub-users who can sign in to an organization's portal, with their role. Wire format: GET /api/users/sub_users; the response is a bare JSON array with email and current_role (for example User View And Restore). The authentication_token field is a credential and is masked in output.

### reference

Reference lists: countries, time zones, languages

- **`dropsuite-cli reference list-countries`** - List the countries Dropsuite accepts. Wire format: GET /api/countries; only X-Reseller-Token is required; the response is a bare JSON array.
- **`dropsuite-cli reference list-locales`** - List the portal languages as a map of language name to locale code (for example english to en). Wire format: GET /api/locales; only X-Reseller-Token is required.
- **`dropsuite-cli reference list-timezones`** - List the time zones Dropsuite accepts. Wire format: GET /api/timezones; only X-Reseller-Token is required; the response is a bare JSON array.

### server

API server status

- **`dropsuite-cli server`** - Check that the Dropsuite API server is up. Wire format: GET /api/status; the response body is the JSON string OK. The dropsuite.us host answers without credentials; the older regional hosts answer 401 without credentials.

### shared-drive-backups

List shared drive backups (needs `--org`)

- **`dropsuite-cli shared-drive-backups <id> <drive_id>`** - List the backup runs of one Google shared drive. Wire format: GET /api/shareddrives/domains/{id}/drives/{drive_id}/backups with an optional page query parameter; the response is a bare JSON array with start_time, end_time, total_files, current_num_file, a numeric status, backup_storage and retries.

### shared-drive-domains

List shared drive domains (needs `--org`)

- **`dropsuite-cli shared-drive-domains`** - List an organization's Google Workspace shared drive domains. Wire format: GET /api/shareddrives/domains with optional domain_name and page query parameters; the response is a bare JSON array with file_count, storage and last_backup.

### shared-drives

List shared drives (needs `--org`)

- **`dropsuite-cli shared-drives <id>`** - List the shared drives in one Google Workspace domain. Wire format: GET /api/shareddrives/domains/{id}/drives with optional drive_name and page query parameters; the response is a bare JSON array with sd_drive, active, file_count, storage and last_backup.

### sharepoint-domains

List sharepoint domains (needs `--org`)

- **`dropsuite-cli sharepoint-domains`** - List an organization's backed-up SharePoint domains. Wire format: GET /api/sharepoints/domains with optional domain_name and page query parameters (at most 25 rows per page); the response is a bare JSON array with site_count, file_count, storage and last_backup. Some deployments answer HTTP 500 when the organization has no SharePoint data.

### sharepoint-sites

List sharepoint sites (needs `--org`)

- **`dropsuite-cli sharepoint-sites <domain_id>`** - List the backed-up SharePoint sites in one domain. Wire format: GET /api/sharepoints/domains/{domain_id}/sites with optional site_name, page and backup_status query parameters; the response is a bare JSON array with sp_site, file_count, storage, last_backup, deactivated_since and current_backup_status.

### tasks

List tasks (needs `--org`)

- **`dropsuite-cli tasks`** - List an organization's task backups per mailbox. Wire format: GET /api/tasks with optional email, page and backup_status query parameters (at most 25 rows per page); the response is a bare JSON array with task_count, last_backup and current_backup_status.

### teams

List teams (needs `--org`)

- **`dropsuite-cli teams <domain_id>`** - List the backed-up Teams and Microsoft 365 Groups in one domain. Wire format: GET /api/teams_and_groups/domains/{domain_id}/teams with optional group_name, page and backup_status query parameters; the response is a bare JSON array with group_name, group_type, last_backup, deactivated_since and current_backup_status.

### teams-domains

List teams domains (needs `--org`)

- **`dropsuite-cli teams-domains`** - List an organization's backed-up Teams and Groups domains. Wire format: GET /api/teams_and_groups/domains with optional domain_name and page query parameters (at most 25 rows per page); the response is a bare JSON array with group_count and last_backup. Some deployments answer HTTP 500 when the organization has no Teams data.


### billing


- **`dropsuite-cli billing`** - Compare each client's seat counters and shared mailboxes with what your PSA invoices, and flag over-allocation and suspended clients still holding seats.


### coverage


- **`dropsuite-cli coverage`** - List mailboxes that exist in each client's Microsoft 365 or Google Workspace tenant but are not added to backup, with coverage percent per client.


### drift


- **`dropsuite-cli drift`** - See what changed between health runs: objects that left backup, backups that stopped advancing and item-count drops.


### evidence


- **`dropsuite-cli evidence`** - Produce one client's dated proof of protection: coverage, last successful backups, item counts, retention policies, freshness history and recorded restore tests.


### health


- **`dropsuite-cli health`** - See every failed, stuck or stale mailbox, OneDrive, SharePoint site, Team, shared drive and Google Drive across your active clients in one run.


### hygiene


- **`dropsuite-cli hygiene`** - Find deactivated or deleted mailboxes still holding storage and invitations nobody accepted, across every client.


### locate


- **`dropsuite-cli locate`** - Find which client owns an email address, whether its mailbox and OneDrive are protected, and when they last backed up.


### restore-test


- **`dropsuite-cli restore-test record --org <id> --object <mailbox or site> --kind mailbox --result pass`** - Record a restore test you performed in the portal (local ledger only).
- **`dropsuite-cli restore-test list`** - List recorded restore tests, newest first.
- **`dropsuite-cli restore-test due --every 90d --pick`** - Organizations with no passing restore test in the window (reads organizations from earlier health or sync runs).




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`dropsuite-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`dropsuite-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`dropsuite-cli learnings list`** - Inspect taught rows
- **`dropsuite-cli learnings forget <query>`** - Undo a teach
- **`dropsuite-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`dropsuite-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`dropsuite-cli teach-pattern`** - Install a query/resource template up front
- **`dropsuite-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `DROPSUITE_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `dropsuite-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
dropsuite-cli accounts list --org 26561-14

# JSON for scripting and agents
dropsuite-cli accounts list --org 26561-14 --json
# Filter to specific fields
dropsuite-cli accounts list --org 26561-14 --json --select added_on,current_backup_status,deactivated_since

# Dry run  -  show the request without sending
dropsuite-cli accounts list --org 26561-14 --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
dropsuite-cli accounts list --org 26561-14 --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Read-only** - every API call is a GET; portal get-login-link, organizations get-support-sso-link, locate --sso-link and evidence --with-sso-link mint a portal sign-in URL (a login credential)
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` generic error (and `health --exit-code` warn), `2` usage error (and `health --exit-code` crit), `3` not found (and `health --exit-code` unknown), `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `DROPSUITE_HOST` resolves `{host}`

Base URL: `https://{host}/api`

## Health Check

```bash
dropsuite-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `dropsuite-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/dropsuite-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\dropsuite-cli\config.toml` on Windows; `--home`, `DROPSUITE_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `DROPSUITE_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `DROPSUITE_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `DROPSUITE_RESELLER_TOKEN` | per_call | Yes | Reseller Token from Settings > API Settings, sent as X-Reseller-Token. |
| `DROPSUITE_ACCESS_TOKEN` | per_call | Yes | Authentication Token from Settings > API Settings (the reseller token), sent as X-Access-Token. |
| `DROPSUITE_HOST` | endpoint | No | Partner API host (default `dropsuite.us`). |
| `DROPSUITE_BASE_URL` | endpoint | No | Whole API URL including `/api`; overrides `DROPSUITE_HOST`. |
| `DROPSUITE_ORG` | routing | No | Organization id or owner email; organization-scoped commands and MCP tools use its token when no `--org` is given. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `dropsuite-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `dropsuite-cli doctor` to check credentials
- Verify both tokens are set: `echo $DROPSUITE_RESELLER_TOKEN $DROPSUITE_ACCESS_TOKEN`
- An organization-scoped command (accounts, onedrives, sharepoint-*, ...) without `--org` also exits 4: add `--org <organization id or owner email>`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **errors.base: Sorry, account is not found or you have no access to it**  -  That command reads one organization's data: add --org <organization id or owner email>, or set DROPSUITE_ORG
- **401 Unauthorized on organizations list**  -  Export both DROPSUITE_RESELLER_TOKEN and DROPSUITE_ACCESS_TOKEN (the reseller Authentication Token), then run dropsuite-cli doctor and dropsuite-cli organizations list
- **HTML 404 on organizations list-tenants, list-delegated-users or list-retention-policies**  -  Your partner host is an older deployment that does not serve those routes; coverage reports such organizations as unsupported-on-deployment and locate falls back to the owner email domain
- **500 Internal Server Error from sharepoint-domains or teams-domains, and health shows unknown rows**  -  Dropsuite answers 500 for organizations with no SharePoint or Teams data; health retries about 7 seconds and reports an unknown row, so health --exit-code exits 3 even when backups are fine. Pass --treat-500-as-empty, or narrow with --kinds mailbox,onedrive, if those organizations have no SharePoint or Teams
- **health --exit-code exits 2**  -  With --exit-code, 2 means at least one crit row (Nagios convention), not a usage error; read the rows with --failing --json
- **drift or restore-test due returns nothing**  -  Both read local history: run health at least twice for drift, and health (or sync --resources organizations) before restore-test due

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**MSPbotsAI/dropsuite-mcp**](https://github.com/MSPbotsAI/dropsuite-mcp)  -  Python
- [**cabsalon93/Hycu-M365Hosted-Centreon**](https://github.com/cabsalon93/Hycu-M365Hosted-Centreon)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
