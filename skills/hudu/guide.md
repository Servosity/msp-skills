# Hudu CLI

**Every Hudu cmdlet, plus an offline SQLite mirror, cross-entity audits, and agent-native output no PowerShell module or read-only MCP ships.**

The first general-purpose Hudu CLI. It covers the full ~120-operation Hudu API surface with write support, adds full write parity plus a local SQLite mirror not present in read-only MCP servers, and makes documentation-hygiene audits possible offline: completeness scoring, stale-password and stale-article detection, an expiration radar, a cross-tenant hygiene rollup, and integration reconciliation  -  none of which exist in the Hudu UI or API. Built for MSP technicians who live in a terminal and for AI agents that need --json/--select and typed exit codes.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `hudu-cli` and `hudu-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.ps1 | iex
   ```
3. Verify: `hudu-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=hudu). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/hudu --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/hudu --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `hudu-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the hudu skill from https://github.com/Servosity/msp-skills/tree/main/skills/hudu. The skill defines how its required CLI (`hudu-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=hudu).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `HUDU_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. A bundle carries the five platform binaries the builder downloads - macOS (`darwin-arm64`, `darwin-amd64`), Linux (`linux-arm64`, `linux-amd64`) and Windows (`windows-amd64`). Windows on ARM is released as a standalone binary but is not bundled, so use the manual config below there.

> **Claude Desktop bundle:** the `.mcpb` launches on macOS (Intel and Apple Silicon), Windows x64 and Linux x64. Every tool shells out to the companion `hudu-cli`. Releases cut from 2026-09-18 on ship that CLI inside the bundle; older bundles contain only the MCP server, so if yours lacks the companion, run the installer above first (or set `HUDU_CLI_PATH` to an existing binary). Details: [#331](https://github.com/Servosity/msp-skills/issues/331).

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "hudu": {
      "command": "hudu-mcp",
      "env": {
        "HUDU_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`hudu-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `HUDU_MCP_HTTP_TOKEN` is set, and every request must send `Authorization: Bearer <token>`. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`.

```bash
export HUDU_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
hudu-mcp --transport http
```

Set `HUDU_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Hudu is per-tenant. Set HUDU_BASE_URL to your instance's /api/v1 URL (self-hosted, *.huducloud.com, or a custom domain) and HUDU_API_KEY to a key created under Admin -> API Keys. The key is sent as the x-api-key header. Keys are global or company-scoped; a company-scoped key cannot call the global `assets list` (use `assets list-by-company`). Run `doctor` to confirm the key, base URL, and reachability; use `api-info get` for the Hudu version. Save the key with `auth set-token` (or env vars) and check it with `auth status`.

## Quick Start

```bash
# Confirm config, auth, and API reachability before anything else
hudu-cli doctor

# Mirror companies, assets, passwords, articles, websites, and expirations into local SQLite
hudu-cli sync

# Score documentation completeness across every client, worst-first
hudu-cli audit completeness --cross-tenant

# Find credentials overdue for rotation  -  the feature Hudu itself lacks
hudu-cli audit stale-passwords --older-than 180d

# See every SSL cert, domain, and warranty expiring in the next month
hudu-cli audit expirations --within 30d

```

## Unique Features

### Documentation hygiene audits
- **`audit completeness`**  -  Score how completely each company's assets fill their layout's required fields, worst-first.

  _Reach for this when an agent needs a hard documentation-hygiene number per client before a QBR instead of eyeballing asset lists._

  ```bash
  hudu-cli audit completeness --cross-tenant --agent
  ```
- **`audit stale-passwords`**  -  Find vault passwords not rotated within a threshold, grouped by company.

  _Reach for this when auditing credential rotation for compliance  -  the answer doesn't exist anywhere in the Hudu UI or API._

  ```bash
  hudu-cli audit stale-passwords --older-than 180d --agent
  ```
- **`audit expirations`**  -  Windowed, typed view of expiring SSL certs, domains, warranties, and passwords sorted by days remaining.

  _Reach for this to answer 'what expires in the next month across all clients' without hammering the 300/min rate limit._

  ```bash
  hudu-cli audit expirations --within 30d --type ssl --agent
  ```
- **`audit summary`**  -  One worst-first hygiene scorecard per company combining completeness, stale passwords, expirations due, and stale articles.

  _Reach for this when an agent needs the bottom-N worst-documented clients across every hygiene dimension in one call instead of running four audits._

  ```bash
  hudu-cli audit summary --limit 5 --agent
  ```
- **`audit layout-drift`**  -  Find assets carrying custom fields not in their layout's current schema, or missing newly-added fields.

  _Reach for this after a bulk layout migration to catch assets left in an inconsistent field state._

  ```bash
  hudu-cli audit layout-drift --layout-name Server --agent
  ```
- **`audit stale-articles`**  -  Rank knowledge-base articles by how long since they were last updated.

  _Reach for this to find documentation that hasn't been touched in a year and likely lies about the current environment._

  ```bash
  hudu-cli audit stale-articles --older-than 365d --company 42 --agent
  ```

### Multi-tenant operations
- **`onboard`**  -  Preview (default) and apply with --apply a saved bundle of asset layouts, folder tree, and procedures-from-template to a new company; the write path needs a global (not company-scoped) API key.

  _Reach for this when standing up a new client so documentation structure never drifts between technicians._

  ```bash
  hudu-cli onboard --init --template msp-standard
  ```
- **`reconcile`**  -  Bulk bidirectional check of which integrator (PSA/RMM) records resolve to a live Hudu asset and which are orphaned.

  _Reach for this when auditing whether a PSA/RMM integration has gone stale, before something breaks in production._

  ```bash
  hudu-cli reconcile --integrator cw_manage --agent
  ```

### Reliability
- **`doctor`**  -  Check CLI health: config, auth, API reachability, credential presence, and local-cache freshness.

  _Reach for this first against any tenant to confirm the key, base URL, and reachability before running other commands._

  ```bash
  hudu-cli doctor --agent
  ```
- **`resolve`**  -  Paste a Hudu object URL or exact name and get the object plus its company, layout, and relations.

  _Reach for this when an agent has a Hudu link or an exact asset name and needs full object context without paging list endpoints._

  ```bash
  hudu-cli resolve "DC01" --agent
  ```

## Recipes

### Cross-client hygiene scorecard, worst-first

```bash
hudu-cli audit summary --agent --select company_name,hygiene_score
```

One ranked table combining completeness, stale passwords, expirations due, and stale articles per company  -  hand the bottom five to the team as tickets.

### Worst-documented clients before a QBR

```bash
hudu-cli audit completeness --cross-tenant --agent --select company_name,completeness_pct
```

Ranks every client by how completely their assets fill required layout fields, narrowed to just the company and score columns.

### Credentials overdue for rotation

```bash
hudu-cli audit stale-passwords --older-than 180d --agent
```

Lists vault entries (names only, never secrets) untouched for six months, grouped by company.

### Everything expiring this month

```bash
hudu-cli audit expirations --within 30d --agent
```

Pulls SSL certs, domains, warranties, and password expiries from the local mirror sorted by days remaining.

### Find stale knowledge-base articles for one client

```bash
hudu-cli audit stale-articles --older-than 365d --company 42 --agent --select name,updated_at
```

Surfaces documentation that hasn't been updated in a year for a single company, showing only title and last-updated.

### Audit a PSA integration for drift

```bash
hudu-cli reconcile --integrator cw_manage --agent
```

Reports which ConnectWise integrator records no longer resolve to a live Hudu asset, both directions.

### Keep a scheduled daily sync fast

A full `sync` fetches every resource's complete collection. A few resources (`public-photos`, `procedures`, `procedure-tasks`) hold the bulk of the data and dominate the runtime. For a scheduled daily job, skip them or sync incrementally:

```bash
# Skip the heavy resources; everything else still syncs
hudu-cli sync --exclude public-photos,procedures,procedure-tasks

# Or sync only the resources you name
hudu-cli sync --resources companies,assets,asset-passwords

# Or refresh incrementally instead of a full re-pull
hudu-cli sync --since 7d
hudu-cli sync --latest-only
```

Run a full `hudu-cli sync` on a slower cadence (e.g. weekly) to backfill the excluded resources. Some Hudu deployments return the complete `procedure-tasks` collection while ignoring pagination parameters. Sync detects a repeated primary-key set before upsert and reconciles `sync_complete.total` to the local cache row count without truncating deployments that honor pagination.

### Find out where a slow sync is actually spending its time

`sync` reports one `duration_ms` per resource, and that clock starts only after a
worker picks the resource up, so it cannot tell a slow API from a slow local
write - or from a resource that simply sat in the queue. `--phase-trace` adds a
`sync_phases` event per resource that splits the wall time six ways:

```bash
hudu-cli sync --resources companies,articles,folders --concurrency 4 --phase-trace
```

```json
{"event":"sync_phases","resource":"folders","queue_wait_ms":62,"api_fetch_ms":49,"rate_limit_wait_ms":0,"retry_wait_ms":0,"id_extract_ms":0,"store_upsert_ms":14,"pages":2,"requests":2,"retries":0,"dominant_phase":"queue_wait"}
```

| Phase | What it measures |
| --- | --- |
| `queue_wait_ms` | Enqueue until a worker picked the resource up. Not covered by `duration_ms` at all. |
| `api_fetch_ms` | Time on the wire, with the two waits below already subtracted out. |
| `rate_limit_wait_ms` | The client-side rate limiter and request-budget waits, including the refill wait after a 429. |
| `retry_wait_ms` | 429 `Retry-After`, 5xx and network-error backoff sleeps. |
| `id_extract_ms` | Pulling items and primary keys out of each page. |
| `store_upsert_ms` | The SQLite batch upsert, which serializes every worker on one write lock. |

`dominant_phase` names the one phase that is strictly larger than all the
others, and `none` when there is no such phase - either nothing was measured
(a resource that failed before it reached the API reports every phase as 0ms)
or two phases tie. `none` means "read the numbers, there is no single lever
here", never "the first phase in the list won". A lightweight resource whose
`dominant_phase` is `queue_wait` is not slow at all - it was waiting behind
heavier resources, and raising `--concurrency` is the lever. One whose
`dominant_phase` is `store_upsert` is contending on the local database, where
lowering `--concurrency` helps more than raising it.

The flag is off by default and adds no output when unset. The event carries
durations and counts only - never a credential, a URL, or a response body.

### Sync matchers for an integration

Hudu's `/matchers` endpoint requires an `integration_id` and returns HTTP 500 without one, so a flat `sync` skips it. There is no integrations-list endpoint; integration IDs live in synced asset `cards` data. After a full sync, list the IDs from the local cache:

```sql
SELECT DISTINCT
  json_extract(card.value, '$.integrator_id')   AS integration_id,
  json_extract(card.value, '$.integrator_name') AS integration_name
FROM resources, json_each(json_extract(data, '$.cards')) AS card
WHERE resource_type = 'assets'
  AND json_extract(card.value, '$.integrator_id') IS NOT NULL
ORDER BY integration_id;
```

Then fetch matchers per integration:

```bash
hudu-cli matchers list --integration-id 2
hudu-cli sync --resource-param matchers:integration_id=6
```

## Usage

Run `hudu-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `HUDU_CONFIG_DIR`, `HUDU_DATA_DIR`, `HUDU_STATE_DIR`, or `HUDU_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `HUDU_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export HUDU_HOME=/srv/hudu
hudu-cli doctor
```

On Windows (PowerShell):

```powershell
$env:HUDU_HOME = "C:\ProgramData\hudu"
hudu-cli doctor
```

Under `HUDU_HOME=/srv/hudu`, the four dirs resolve to `/srv/hudu/config`, `/srv/hudu/data`, `/srv/hudu/state`, and `/srv/hudu/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:HUDU_HOME = "C:\srv\hudu"
hudu-cli doctor
```

Under `$env:HUDU_HOME = "C:\srv\hudu"`, the four dirs resolve to `C:\srv\hudu\config`, `C:\srv\hudu\data`, `C:\srv\hudu\state`, and `C:\srv\hudu\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "hudu": {
      "command": "hudu-mcp",
      "env": {
        "HUDU_HOME": "/srv/hudu"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `HUDU_DATA_DIR` overrides an explicit `--home` for that kind. Use `HUDU_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `HUDU_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `hudu-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Password-vault passwords and OTP secrets, and the values of password-type asset fields, are never part of any of it: the client drops the secrets from every vault response and replaces each filled password-type asset field value with `[withheld: password field]` before anything is printed, returned to an agent, stored or cached, and an error from a vault or asset request shows only its HTTP status, never Hudu's error text. A store written by an older version is scrubbed the first time this version opens it, an older cached response is never served, and vault and asset responses (including those Hudu redirected to) are never cached, so no purge is needed after upgrading. The API key lives in `credentials.toml` in the data directory when you save it with `auth set-token` (owner-only permissions); otherwise it is read from `HUDU_API_KEY`. The learn journal lives under `learn/` in the state directory. `hudu-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `hudu-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `HUDU_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `HUDU_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### activity-logs

Read-only audit/activity log

- **`hudu-cli activity-logs`** - List activity-log entries (paginated)

### api-info

Instance metadata  -  Hudu version (used to gate version-specific endpoints)

- **`hudu-cli api-info`** - Get the Hudu instance version and API info

### articles

Knowledge-base articles

- **`hudu-cli articles archive`** - Archive an article
- **`hudu-cli articles create`** - Create a knowledge-base article
- **`hudu-cli articles delete`** - Delete an article
- **`hudu-cli articles get`** - Get a single article (with body content)
- **`hudu-cli articles list`** - List knowledge-base articles (paginated)
- **`hudu-cli articles unarchive`** - Unarchive an article
- **`hudu-cli articles update`** - Update a knowledge-base article

### asset-layouts

Asset layouts  -  the custom-field schemas that type assets

- **`hudu-cli asset-layouts create`** - Create an asset layout. fields is a JSON array of field definitions; validate field_type (an invalid type silently bricks the layout).
- **`hudu-cli asset-layouts get`** - Get a single asset layout (with its field definitions)
- **`hudu-cli asset-layouts list`** - List asset layouts (paginated)
- **`hudu-cli asset-layouts update`** - Update an asset layout. Validate field_type client-side  -  an invalid value silently corrupts the layout and its assets (HTTP 500 on read).

### asset-passwords

Password vault entries

- **`hudu-cli asset-passwords archive`** - Archive a password entry
- **`hudu-cli asset-passwords create`** - Create a password vault entry
- **`hudu-cli asset-passwords delete`** - Delete a password vault entry
- **`hudu-cli asset-passwords get`** - Get a single password entry (metadata only: this CLI withholds the password and OTP secret; fetch them in the Hudu portal)
- **`hudu-cli asset-passwords list`** - List password vault entries (paginated). This CLI withholds the password and OTP secret; fetch them in the Hudu portal.
- **`hudu-cli asset-passwords unarchive`** - Unarchive a password entry
- **`hudu-cli asset-passwords update`** - Update a password vault entry

### assets

Assets  -  configuration items typed by an asset layout, owned by a company

- **`hudu-cli assets archive`** - Archive an asset
- **`hudu-cli assets create`** - Create an asset under a company. custom_fields is a JSON array string matching the asset layout's fields.
- **`hudu-cli assets delete`** - Delete an asset
- **`hudu-cli assets get`** - Get a single asset
- **`hudu-cli assets list`** - Global asset list/search (paginated). NOTE: fails on company-scoped API keys  -  use list-by-company instead.
- **`hudu-cli assets list-by-company`** - List assets for a single company (works with company-scoped keys)
- **`hudu-cli assets move-layout`** - Move an asset to a different asset layout
- **`hudu-cli assets unarchive`** - Unarchive an asset
- **`hudu-cli assets update`** - Update an asset. Sending a different company_id in the body MOVES the asset to that company.

### cards

Integration card lookup  -  resolve an external integrator record to its Hudu object

- **`hudu-cli cards`** - Look up Hudu objects by an external integrator's record id

### companies

Client companies  -  the root tenant-of-record everything else hangs off

- **`hudu-cli companies archive`** - Archive a company
- **`hudu-cli companies create`** - Create a company
- **`hudu-cli companies delete`** - Delete a company
- **`hudu-cli companies get`** - Get a single company by id
- **`hudu-cli companies list`** - List/search companies (paginated)
- **`hudu-cli companies unarchive`** - Unarchive a company
- **`hudu-cli companies update`** - Update a company

### expirations

Read-only rollup of dated items (SSL, domains, warranties, passwords)

- **`hudu-cli expirations`** - List upcoming/expiring items across the instance (paginated)

### exports

Data exports (recent Hudu versions)

- **`hudu-cli exports create`** - Start an export
- **`hudu-cli exports create-s3`** - Start an export targeted at S3
- **`hudu-cli exports get`** - Get a single export's status
- **`hudu-cli exports list`** - List/poll exports (paginated)

### flag-types

Flag type definitions (Hudu 2.4.0+)

- **`hudu-cli flag-types create`** - Create a flag type. Requires Hudu 2.4.0+.
- **`hudu-cli flag-types delete`** - Delete a flag type. Requires Hudu 2.4.0+.
- **`hudu-cli flag-types list`** - List flag types (paginated). Requires Hudu 2.4.0+.
- **`hudu-cli flag-types update`** - Update a flag type. Requires Hudu 2.4.0+.

### flags

Flags (Hudu 2.4.0+)  -  gate on api-info version before use

- **`hudu-cli flags create`** - Create a flag. Requires Hudu 2.4.0+.
- **`hudu-cli flags delete`** - Delete a flag. Requires Hudu 2.4.0+.
- **`hudu-cli flags list`** - List flags (paginated). Requires Hudu 2.4.0+.
- **`hudu-cli flags update`** - Update a flag. Requires Hudu 2.4.0+.

### folders

Folders for organizing articles and assets

- **`hudu-cli folders create`** - Create a folder
- **`hudu-cli folders get`** - Get a single folder
- **`hudu-cli folders list`** - List folders (paginated)
- **`hudu-cli folders update`** - Update a folder

### groups

User/permission groups (read-only)

- **`hudu-cli groups get`** - Get a single group
- **`hudu-cli groups list`** - List groups (paginated)

### ip-addresses

IP address records

- **`hudu-cli ip-addresses create`** - Create an IP address record
- **`hudu-cli ip-addresses delete`** - Delete an IP address record
- **`hudu-cli ip-addresses get`** - Get a single IP address record
- **`hudu-cli ip-addresses list`** - List IP address records (not paginated: returns the whole collection)
- **`hudu-cli ip-addresses update`** - Update an IP address record

### lists

Custom lists (dropdown option sets)

- **`hudu-cli lists create`** - Create a custom list. list_items is a JSON array of option objects.
- **`hudu-cli lists delete`** - Delete a custom list
- **`hudu-cli lists get`** - Get a single list (with its options)
- **`hudu-cli lists list`** - List custom lists (paginated)
- **`hudu-cli lists update`** - Update a custom list

### magic-dash

Magic Dash  -  custom dashboard tiles on a company page

- **`hudu-cli magic-dash delete`** - Delete a Magic Dash tile by id
- **`hudu-cli magic-dash list`** - List Magic Dash tiles (paginated)
- **`hudu-cli magic-dash set`** - Create or update a Magic Dash tile (upsert keyed by company_name + title)

### matchers

Integration matchers  -  reconcile Hudu records to PSA/RMM records

- **`hudu-cli matchers list`** - List integration matchers for one integration (paginated; --integration-id is required)
- **`hudu-cli matchers update`** - Update a matcher (link/unlink an integrator record to a Hudu company)

### networks

Network records

- **`hudu-cli networks create`** - Create a network
- **`hudu-cli networks delete`** - Delete a network
- **`hudu-cli networks get`** - Get a single network
- **`hudu-cli networks list`** - List networks (not paginated: returns the whole collection)
- **`hudu-cli networks update`** - Update a network

### procedure-tasks

Tasks within a procedure

- **`hudu-cli procedure-tasks create`** - Create a procedure task (body shape changed at Hudu 2.4.1)
- **`hudu-cli procedure-tasks delete`** - Delete a procedure task
- **`hudu-cli procedure-tasks get`** - Get a single procedure task
- **`hudu-cli procedure-tasks list`** - List procedure tasks (paginated)
- **`hudu-cli procedure-tasks update`** - Update a procedure task

### procedures

Process templates and running procedures (UI label: Processes)

- **`hudu-cli procedures create`** - Create a procedure (process template)
- **`hudu-cli procedures create-from-template`** - Instantiate a procedure from a template into a company
- **`hudu-cli procedures delete`** - Delete a procedure
- **`hudu-cli procedures duplicate`** - Duplicate a procedure
- **`hudu-cli procedures get`** - Get a single procedure
- **`hudu-cli procedures kickoff`** - Kick off (start) a procedure run
- **`hudu-cli procedures list`** - List procedures (paginated)
- **`hudu-cli procedures update`** - Update a procedure

### public-photos

Public photo gallery (inline images for articles)

- **`hudu-cli public-photos`** - List public photos (paginated)

### rack-storage-items

Items mounted in a rack storage unit

- **`hudu-cli rack-storage-items create`** - Mount an item in a rack
- **`hudu-cli rack-storage-items delete`** - Delete a rack storage item
- **`hudu-cli rack-storage-items get`** - Get a single rack storage item
- **`hudu-cli rack-storage-items list`** - List rack storage items (not paginated: returns the whole collection)
- **`hudu-cli rack-storage-items update`** - Update a rack storage item

### rack-storages

Rack storage units

- **`hudu-cli rack-storages create`** - Create a rack storage unit (Hudu uses a POST with id segment for this resource)
- **`hudu-cli rack-storages delete`** - Delete a rack storage unit
- **`hudu-cli rack-storages get`** - Get a single rack storage unit
- **`hudu-cli rack-storages list`** - List rack storage units (paginated)
- **`hudu-cli rack-storages update`** - Update a rack storage unit

### relations

Generic many-to-many links between any two entities

- **`hudu-cli relations create`** - Create a relation between two entities
- **`hudu-cli relations delete`** - Delete a relation
- **`hudu-cli relations list`** - List relations (paginated)

### uploads

File uploads

- **`hudu-cli uploads delete`** - Delete an upload
- **`hudu-cli uploads get`** - Get a single upload's metadata
- **`hudu-cli uploads list`** - List uploads (paginated)

### users

Users (read-only)

- **`hudu-cli users get`** - Get a single user
- **`hudu-cli users list`** - List users (paginated)

### vlan-zones

VLAN zones

- **`hudu-cli vlan-zones create`** - Create a VLAN zone
- **`hudu-cli vlan-zones delete`** - Delete a VLAN zone
- **`hudu-cli vlan-zones get`** - Get a single VLAN zone
- **`hudu-cli vlan-zones list`** - List VLAN zones (not paginated: returns the whole collection)
- **`hudu-cli vlan-zones update`** - Update a VLAN zone

### vlans

VLAN records

- **`hudu-cli vlans create`** - Create a VLAN
- **`hudu-cli vlans delete`** - Delete a VLAN
- **`hudu-cli vlans get`** - Get a single VLAN
- **`hudu-cli vlans list`** - List VLANs (not paginated: returns the whole collection)
- **`hudu-cli vlans update`** - Update a VLAN

### websites

Monitored websites with SSL/domain expiration tracking

- **`hudu-cli websites create`** - Add a monitored website
- **`hudu-cli websites delete`** - Delete a monitored website
- **`hudu-cli websites get`** - Get a single website
- **`hudu-cli websites list`** - List monitored websites (paginated)
- **`hudu-cli websites update`** - Update a monitored website


### audit


- **`hudu-cli audit completeness`** - Score how completely each company's assets fill their layout's required fields, worst-first.

- **`hudu-cli audit stale-passwords`** - Find vault passwords not rotated within a threshold, grouped by company.

- **`hudu-cli audit expirations`** - Windowed, typed view of expiring SSL certs, domains, warranties, and passwords sorted by days remaining.

- **`hudu-cli audit summary`** - One worst-first hygiene scorecard per company combining completeness, stale passwords, expirations due, and stale articles.

- **`hudu-cli audit layout-drift`** - Find assets carrying custom fields not in their layout's current schema, or missing newly-added fields.

- **`hudu-cli audit stale-articles`** - Rank knowledge-base articles by how long since they were last updated.


### onboard


- **`hudu-cli onboard`** - Preview (default) and apply with --apply a saved bundle of asset layouts, folder tree, and procedures-from-template to a new company; the write path needs a global (not company-scoped) API key.


### reconcile


- **`hudu-cli reconcile`** - Bulk bidirectional check of which integrator (PSA/RMM) records resolve to a live Hudu asset and which are orphaned.


### resolve


- **`hudu-cli resolve`** - Paste a Hudu object URL or exact name and get the object plus its company, layout, and relations.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`hudu-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`hudu-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`hudu-cli learnings list`** - Inspect taught rows
- **`hudu-cli learnings forget <query>`** - Undo a teach
- **`hudu-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`hudu-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`hudu-cli teach-pattern`** - Install a query/resource template up front
- **`hudu-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `HUDU_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `hudu-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
hudu-cli activity-logs

# JSON for scripting and agents
hudu-cli activity-logs --json
# Filter to specific fields
hudu-cli activity-logs --json --select id,user_id,user_email

# Dry run  -  show the request without sending
hudu-cli activity-logs --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
hudu-cli activity-logs --agent
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
hudu-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `hudu-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/hudu-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\hudu-cli\config.toml` on Windows; `--home`, `HUDU_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `HUDU_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `HUDU_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `HUDU_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `hudu-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `hudu-cli doctor` to check credentials
- Verify the environment variable is set: `echo $HUDU_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **HTTP 429 Too Many Requests**  -  Hudu allows 300 requests/minute over a 5-minute window; the CLI backs off automatically  -  prefer `sync` + local `audit`/`search` over repeated live list calls.
- **`assets list` returns 401/403 but other commands work**  -  Your API key is company-scoped; use `assets list-by-company <company_id>` instead, or generate a global key.
- **`flags` or `exports` commands fail with 404**  -  Those endpoints require Hudu 2.4.0+  -  run `api-info get` to check your instance version.
- **Base URL errors / connection refused**  -  Set HUDU_BASE_URL to the full /api/v1 URL including https://, e.g. https://yourname.huducloud.com/api/v1.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**lwhitelock/HuduAPI**](https://github.com/lwhitelock/HuduAPI)  -  PowerShell (64 stars)
- [**msoukhomlinov/n8n-nodes-hudu**](https://github.com/msoukhomlinov/n8n-nodes-hudu)  -  TypeScript (18 stars)
- [**jlbyh2o/hudu-mcp**](https://github.com/jlbyh2o/hudu-mcp)  -  TypeScript (7 stars)
- [**wyre-technology/hudu-mcp**](https://github.com/wyre-technology/hudu-mcp)  -  TypeScript (3 stars)
- [**DevSkillsIT/Skills-MCP-Hudu**](https://github.com/DevSkillsIT/Skills-MCP-Hudu)  -  TypeScript (2 stars)
- [**realchrisolin/HuduAPI**](https://github.com/realchrisolin/HuduAPI)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
