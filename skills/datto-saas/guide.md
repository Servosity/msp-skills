# Datto SaaS Protection CLI

**Every Datto SaaS Protection (Backupify) client in one local database: seat audits, protection gaps, backup health ranking, exit inventories and plan-first bulk seat changes.**

The Partner Portal shows one client at a time. This CLI syncs every client's domains, seats and backup history into SQLite so you can reconcile billing with seats reconcile, find unprotected seats with seats gaps, rank backups with backup-health, and hand a departing client a full exit-inventory. Seat changes are planned locally with seats bulk-plan; seats bulk-change is the one command that sends a change.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `datto-saas-cli` and `datto-saas-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-saas/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-saas/install.ps1 | iex
   ```
3. Verify: `datto-saas-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=datto-saas). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/datto-saas --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/datto-saas --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `datto-saas-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the datto-saas skill from https://github.com/Servosity/msp-skills/tree/main/skills/datto-saas. The skill defines how its required CLI (`datto-saas-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=datto-saas).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `DATTO_SAAS_PUBLIC_KEY` and `DATTO_SAAS_SECRET_KEY` (the Partner Portal public and secret key) when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-saas/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/datto-saas/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "datto-saas": {
      "command": "datto-saas-mcp",
      "env": {
        "DATTO_SAAS_PUBLIC_KEY": "<your-public-key>",
        "DATTO_SAAS_SECRET_KEY": "<your-secret-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`datto-saas-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `DATTO_SAAS_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `DATTO_SAAS_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export DATTO_SAAS_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
datto-saas-mcp --transport http
```

Set `DATTO_SAAS_MCP_READ_ONLY=1` (exactly `1`) and the server lists only read-only tools: `datto-saas_execute`, `sync`, `export`, the exit-inventory recipe and the learn-store tools (`teach`, `teach_lookup`, `teach_pattern`, `teach_playbook`, `playbook_amend`, `learnings_confirm`, `learnings_forget`, `learnings_reject`, `workflow_archive`) are not registered.

Over MCP the API endpoints are reached through `datto-saas_search`, `datto-saas_read` and `datto-saas_execute`. The one write, `seats.bulk-change`, goes through `datto-saas_execute`: it returns a preview and sends only when `confirm` is boolean `true`.

## Authentication

Datto SaaS Protection uses the Datto REST API key pair from the Partner Portal (Admin, Integrations, API Keys). Set DATTO_SAAS_PUBLIC_KEY to the public key and DATTO_SAAS_SECRET_KEY to the secret key; the CLI sends them as HTTP Basic auth to api.datto.com. One partner key sees every client you manage.

## Quick Start

```bash
# Confirm the CLI runs (dry run: no API call and no configuration check)
datto-saas-cli doctor --dry-run

# Pull every domain, then each client's seats and backup history, into the local store
datto-saas-cli sync

# Find seats added in the last 30 days that are not protected
datto-saas-cli seats gaps --since 30d

# Rank clients whose backup coverage is under 95 percent
datto-saas-cli backup-health --below 95

# Preview the License batches for one client before changing anything
datto-saas-cli seats bulk-plan --action License --state Unprotected --customer 123456

```

## Unique Features

### Safe seat changes
- **`seats bulk-plan`**  -  Turn a seat selection into ready-to-run License, Pause or Unlicense batches, grouped by subscription and seat type and capped at 100 ids, without sending anything.

  _Reach for this before any seat change: it previews which seats would move, skips seats already in the target state, blocks clients with several subscriptions until --subscription picks one, and prints the seats bulk-change command per batch. Check blocked and skipped before trusting an empty plan._

  ```bash
  datto-saas-cli seats bulk-plan --action License --state Unprotected --customer 123456 --subscription Classic:Office365:654321 --agent
  ```

### Fleet seat audit
- **`seats reconcile`**  -  See, per client, where Datto's seatsUsed disagrees with billable and Active seats before you invoice.

  _Use it for monthly invoice prep or a cost-leak sweep; it reports signed deltas and flags instead of raw seat lists._

  ```bash
  datto-saas-cli seats reconcile --agent
  ```
- **`seats gaps`**  -  List seats across all clients that are Unprotected (Paused too with --include-paused), newest first, with each seat's remoteId for a License plan.

  _Use it to answer who joined a tenant and is not covered, across the whole fleet in one call._

  ```bash
  datto-saas-cli seats gaps --since 30d --agent
  ```
- **`seats roster`**  -  Every synced seat across all clients in one table, joined with its customer's domains, product types and subscription ids; --csv for a fleet-wide export.

  _Reach for this for a fleet-wide seat export or to hand a client's seat list to billing; use seats summary for counts._

  ```bash
  datto-saas-cli seats roster --customer 123456 --state Unprotected,Paused --agent
  ```
- **`seats summary`**  -  Seat counts per client and seat type, split by Active, Paused, Archived and Unprotected state and the billable flag.

  _Reach for this for plain counts by type and state; use seats reconcile to compare counts with Datto's seatsUsed._

  ```bash
  datto-saas-cli seats summary --agent
  ```

### Backup health
- **`backup-health`**  -  Rank clients worst first by backup coverage, then consecutive short backup windows and perfect-backup rate.

  _Use it for the daily or weekly backup triage instead of opening each client's report._

  ```bash
  datto-saas-cli backup-health --below 95 --agent
  ```

### Client offboarding
- **`exit-inventory`**  -  Build one manifest for a client leaving SaaS Protection: domains, subscription ids, retention, every seat with remoteId, storage, last backups and a blockers checklist.

  _Use it to plan a migration or offboarding from facts and to confirm nothing is still un-backed-up before cancellation. Use --data-source live for a real offboarding: the local fallback has no detailed backup stats._

  ```bash
  datto-saas-cli exit-inventory 123456 --agent
  ```

## Recipes

### Reconcile seats before invoicing

```bash
datto-saas-cli seats reconcile --agent
```

Per client, compares summed domain seatsUsed with billable and Active seats and flags mismatches.

### Narrow a large seat roster

```bash
datto-saas-cli seats roster --agent --select saas_customer_name,domains,name,seat_type,seat_state,remote_id
```

The fleet roster is wide; select only the columns an agent needs.

### Exit inventory for a departing client

```bash
datto-saas-cli exit-inventory 123456 --agent
```

One manifest with domains, subscription ids, seats, storage and a blockers checklist.

### Plan unlicensing archived seats

```bash
datto-saas-cli seats bulk-plan --action Unlicense --state Archived --customer 123456 --subscription Classic:Office365:654321 --agent
```

Groups archived seats into batches of at most 100 and prints the seats bulk-change command for each; sends nothing. Check blocked and skipped before trusting an empty plan. Unlicense stops protecting the seat, so confirm how long the archived data is retained before running the batches.

## Usage

Run `datto-saas-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `DATTO_SAAS_CONFIG_DIR`, `DATTO_SAAS_DATA_DIR`, `DATTO_SAAS_STATE_DIR`, or `DATTO_SAAS_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `DATTO_SAAS_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export DATTO_SAAS_HOME=/srv/datto-saas
datto-saas-cli doctor
```

On Windows (PowerShell):

```powershell
$env:DATTO_SAAS_HOME = "C:\ProgramData\datto-saas"
datto-saas-cli doctor
```

Under `DATTO_SAAS_HOME=/srv/datto-saas`, the four dirs resolve to `/srv/datto-saas/config`, `/srv/datto-saas/data`, `/srv/datto-saas/state`, and `/srv/datto-saas/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:DATTO_SAAS_HOME = "C:\srv\datto-saas"
datto-saas-cli doctor
```

Under `$env:DATTO_SAAS_HOME = "C:\srv\datto-saas"`, the four dirs resolve to `C:\srv\datto-saas\config`, `C:\srv\datto-saas\data`, `C:\srv\datto-saas\state`, and `C:\srv\datto-saas\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "datto-saas": {
      "command": "datto-saas-mcp",
      "env": {
        "DATTO_SAAS_HOME": "/srv/datto-saas"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `DATTO_SAAS_DATA_DIR` overrides an explicit `--home` for that kind. Use `DATTO_SAAS_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `DATTO_SAAS_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `datto-saas-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `datto-saas-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `datto-saas-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `DATTO_SAAS_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), no API data is written to the store (the learning loop's own records still are; add `--no-learn` to keep those out too), and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `DATTO_SAAS_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### activity-log

Datto platform activity log (shared reporting endpoint).

- **`datto-saas-cli activity-log`** - List Datto platform activity log entries, filtered by client, user, target or time window. Entries are ordered by date; the direction is not documented. Wire format: GET /v1/report/activity-log with optional clientName, since, sinceUnits, target, targetType, user, _page and _perPage; the response is a pagination plus items envelope.

### applications

Per-customer backup history by suite and application.

- **`datto-saas-cli applications <saasCustomerId>`** - Show one customer's backup report: used bytes, backup history per suite and app, and optional remote IDs per seat type. The remote IDs and per-seat-type counts are returned with includeRemoteID=1. Wire format: GET /v1/saas/{saasCustomerId}/applications?daysUntil=N&includeRemoteID=0|1; the response is a pagination plus items envelope.

### backup-stats

Per-customer detailed backup statistics.

- **`datto-saas-cli backup-stats <saasCustomerId>`** - Show detailed backup statistics for one SaaS customer. Wire format: GET /v1/saas/{saasCustomerId}/detailedBackupStats with no query parameters; returns one JSON object whose shape is not publicly documented, so every field is optional.

### domains

Protected Microsoft 365 tenants and Google Workspace domains.

- **`datto-saas-cli domains`** - List every protected Microsoft 365 tenant and Google Workspace domain, with its saasCustomerId and subscription id. One row per domain; a customer with several domains appears once per domain. Wire format: GET /v1/saas/domains with no query parameters; the response is a bare JSON array of domain objects.

### seats

Protected seats (users, shared mailboxes, sites, team sites, teams, shared drives).

- **`datto-saas-cli seats bulk-change <saasCustomerId> <externalSubscriptionId>`** - License, pause, or unlicense up to 100 seats of one subscription in one call. This is the only write: it changes which seats are protected and billed. It sends immediately: there is no confirmation prompt and `--yes` is not required. Preview with `--dry-run`, which applies the same seat-type, action and 100-id checks as a real send, or plan batches with `seats bulk-plan`. Wire format: PUT /v1/saas/{saasCustomerId}/{externalSubscriptionId}/bulkSeatChange with JSON body {"seat_type": "User", "action_type": "License", "ids": ["<remoteId>"]}. Take saasCustomerId and externalSubscriptionId from the domains list and ids from the seats list remoteId. Google Workspace tenants must be on Seat Management 2.0. Never replayed automatically, because a 5xx may already have been applied.
- **`datto-saas-cli seats list <saasCustomerId>`** - List the seats of one SaaS customer with seat type, state, billable flag, date added and remoteId. Wire format: GET /v1/saas/{saasCustomerId}/seats with an optional seatType query; the response is a bare JSON array of seats.


### backup-health


- **`datto-saas-cli backup-health`** - Rank clients worst first by backup coverage, then consecutive short backup windows and perfect-backup rate.


### exit-inventory


- **`datto-saas-cli exit-inventory <customer>`** - Build one manifest for a client leaving SaaS Protection: domains, subscription ids, retention, every seat with remoteId, storage, last backups and a blockers checklist.


### seats (local audit and planning)

Run `datto-saas-cli sync` first: these commands read the local store, and an unsynced store returns empty results, which is not an all-clear.

- **`datto-saas-cli seats bulk-plan`** - Turn a seat selection into ready-to-run License, Pause or Unlicense batches, grouped by subscription and seat type and capped at 100 ids, without sending anything.

- **`datto-saas-cli seats reconcile`** - See, per client, where Datto's seatsUsed disagrees with billable and Active seats before you invoice.

- **`datto-saas-cli seats gaps`** - List seats across all clients that are Unprotected (Paused too with --include-paused), newest first, with each seat's remoteId for a License plan.

- **`datto-saas-cli seats roster`** - Every synced seat across all clients in one table, joined with its customer's domains, product types and subscription ids; --csv for a fleet-wide export.

- **`datto-saas-cli seats summary`** - Seat counts per client and seat type, split by Active, Paused, Archived and Unprotected state and the billable flag.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`datto-saas-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`datto-saas-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`datto-saas-cli learnings list`** - Inspect taught rows
- **`datto-saas-cli learnings forget <query>`** - Undo a teach
- **`datto-saas-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`datto-saas-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`datto-saas-cli teach-pattern`** - Install a query/resource template up front
- **`datto-saas-cli teach-lookup`** - Add an entity mapping (e.g. o365 -> Office365, unlicensed -> Unprotected) for pattern substitution

Pass `--no-learn` or set `DATTO_SAAS_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `datto-saas-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
datto-saas-cli activity-log

# JSON for scripting and agents
datto-saas-cli activity-log --json
# Filter to specific fields
datto-saas-cli activity-log --json --select category,id,message

# Dry run  -  show the request without sending
datto-saas-cli activity-log --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
datto-saas-cli activity-log --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **No create endpoints** - `--idempotent` only turns an HTTP 409 into success, so `seats bulk-change` refuses it
- **No confirmation prompt** - `seats bulk-change` is the only write and sends immediately; `--yes` is not required. Preview with `--dry-run` or plan with `seats bulk-plan`
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` other error (for example `--data-source live` on a local-only command), `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Freshness

This CLI owns bounded freshness for registered store-backed read command paths. In `--data-source auto` mode, covered commands check the local SQLite store before serving results; stale or missing resources trigger a bounded refresh, and refresh failures fall back to the existing local data with a warning. `--data-source local` never refreshes, and `--data-source live` reads the API without mutating the local store.

Set `DATTO_SAAS_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths:
- `datto-saas-cli domains`

The seat, backup-health and exit-inventory commands are not covered: the pre-read refresh only re-fetches the domains list, not the per-client seats and backup history those commands read. Run `datto-saas-cli sync` to refresh them; they print a stale-data hint on stderr. The local-only commands (`seats roster`, `seats summary`, `seats reconcile`, `seats gaps`, `seats bulk-plan`, `backup-health`) reject `--data-source live`.

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
datto-saas-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `datto-saas-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/datto-saas-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\datto-saas-cli\config.toml` on Windows; `--home`, `DATTO_SAAS_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `DATTO_SAAS_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `DATTO_SAAS_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `DATTO_SAAS_PUBLIC_KEY` | per_call | Yes | Datto REST API public key (Partner Portal, Admin, Integrations, API Keys). |
| `DATTO_SAAS_SECRET_KEY` | per_call | Yes | Datto REST API secret key from the same key pair. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `datto-saas-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `datto-saas-cli doctor` to check credentials
- Verify both variables are set: `echo $DATTO_SAAS_PUBLIC_KEY` and `test -n "$DATTO_SAAS_SECRET_KEY" && echo secret key set`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run `datto-saas-cli domains` to list saasCustomerIds, or `datto-saas-cli seats list <saasCustomerId>` for a client's seats

### API-specific
- **HTTP 401 on every command**  -  Check DATTO_SAAS_PUBLIC_KEY and DATTO_SAAS_SECRET_KEY are the key pair from Partner Portal Admin, Integrations, API Keys and that the key is active
- **Novel commands return nothing or warn the store is unsynced**  -  Run datto-saas-cli sync first; seats and backup history are fetched per client from the domains list
- **bulkSeatChange rejects the seat type or action**  -  Both values are case-sensitive: seat types User, SharedMailbox, SharedDrive, Site, TeamSite, Team and actions License, Pause, Unlicense
- **Google Workspace seat changes fail**  -  bulkSeatChange works only on Seat Management 2.0 tenants; change Seat Management 1.0 seats in the portal
- **applications returns little history**  -  Pass --days-until up to 30; the API defaults to 10 days

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**Datto-PowerShellWrapper**](https://github.com/Celerium/Datto-PowerShellWrapper)  -  PowerShell (7 stars)
- [**bifrost-community**](https://github.com/gobifrost/bifrost-community)  -  Python (3 stars)
- [**datto-saas-protection-mcp**](https://github.com/WYRE-AI/datto-saas-protection-mcp)  -  TypeScript (2 stars)
- [**lgp-mcp-servers**](https://github.com/Logiphys/lgp-mcp-servers)  -  Go (2 stars)
- [**node-datto-saas-protection**](https://github.com/WYRE-AI/node-datto-saas-protection)  -  TypeScript (1 stars)
- [**Celerium.DattoBCDR**](https://github.com/Celerium/Celerium.DattoBCDR)  -  PowerShell (1 stars)
- [**dattoapi**](https://github.com/SimonBuckner/dattoapi)  -  Go
- [**n8n-nodes-datto-backup**](https://github.com/ajoshuasmith/n8n-nodes-datto-backup)  -  TypeScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
