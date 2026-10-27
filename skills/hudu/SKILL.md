---
name: hudu
description: "Every Hudu cmdlet, plus an offline SQLite mirror, cross-entity audits, and agent-native output no PowerShell module or read-only MCP ships. Trigger phrases: `hudu hygiene scorecard`, `audit hudu documentation`, `find stale hudu passwords`, `what hudu certs expire soon`, `score hudu documentation completeness`, `onboard a new client in hudu`, `use hudu`, `run hudu`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Hudu"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - hudu-cli
    install:
      - kind: script
        bins: [hudu-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/hudu/install.ps1
---

# Hudu - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `hudu-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

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

The installer downloads the `hudu-cli` and `hudu-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

The first general-purpose Hudu CLI. It covers the full ~120-operation Hudu API surface with write support, adds full write parity plus a local SQLite mirror not present in read-only MCP servers, and makes documentation-hygiene audits possible offline: completeness scoring, stale-password and stale-article detection, an expiration radar, a cross-tenant hygiene rollup, and integration reconciliation  -  none of which exist in the Hudu UI or API. Built for MSP technicians who live in a terminal and for AI agents that need --json/--select and typed exit codes.

## When to Use This CLI

Use this CLI when an agent or technician needs to read, write, or audit Hudu IT documentation from the terminal: bulk asset and company maintenance, password-vault and expiration audits, documentation-completeness scoring before a QBR, new-client onboarding scaffolding, or reconciling PSA/RMM integration records. Prefer `sync` then local `audit`/`search`/`analytics` for anything that would otherwise make many live calls against the 300/min limit.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI for fuzzy keyword discovery when you mean full-text search across resources  -  that is `search`, not `resolve` (resolve is exact URL/name lookup).
- Do not use the write path of `onboard` with a company-scoped API key  -  creating layouts/folders needs a global key; preview (`onboard` without --apply) works regardless.
- Do not use this CLI to read or export password secret values  -  it withholds every vault password and OTP secret and the value of every password-type asset field, which are never shown, stored or cached (the mirror keeps vault metadata only: name, username, URL, folder, dates; a filled password-type asset field reads `[withheld: password field]`); fetch secrets in the Hudu portal.
- Do not use this CLI against Hudu instances older than 2.4.0 for flags/flag-types or recent procedure-task shapes  -  run `doctor` first; those endpoints are version-gated.
- Do not script bulk live reads in a tight loop  -  Hudu rate-limits at 300 req/min over a 5-minute window; `sync` once and run audits/search locally instead.

## Unique Capabilities

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

## Command Reference

**activity-logs**  -  Read-only audit/activity log

- `hudu-cli activity-logs`  -  List activity-log entries (paginated)

**api-info**  -  Instance metadata  -  Hudu version (used to gate version-specific endpoints)

- `hudu-cli api-info`  -  Get the Hudu instance version and API info

**articles**  -  Knowledge-base articles

- `hudu-cli articles archive`  -  Archive an article
- `hudu-cli articles create`  -  Create a knowledge-base article
- `hudu-cli articles delete`  -  Delete an article
- `hudu-cli articles get`  -  Get a single article (with body content)
- `hudu-cli articles list`  -  List knowledge-base articles (paginated)
- `hudu-cli articles unarchive`  -  Unarchive an article
- `hudu-cli articles update`  -  Update a knowledge-base article

**asset-layouts**  -  Asset layouts  -  the custom-field schemas that type assets

- `hudu-cli asset-layouts create`  -  Create an asset layout.
- `hudu-cli asset-layouts get`  -  Get a single asset layout (with its field definitions)
- `hudu-cli asset-layouts list`  -  List asset layouts (paginated)
- `hudu-cli asset-layouts update`  -  Update an asset layout.

**asset-passwords**  -  Password vault entries

- `hudu-cli asset-passwords archive`  -  Archive a password entry
- `hudu-cli asset-passwords create`  -  Create a password vault entry
- `hudu-cli asset-passwords delete`  -  Delete a password vault entry
- `hudu-cli asset-passwords get`  -  Get a single password entry (metadata only: this CLI withholds the password and OTP secret; fetch them in the Hudu portal)
- `hudu-cli asset-passwords list`  -  List password vault entries (paginated). This CLI withholds the password and OTP secret; fetch them in the Hudu portal.
- `hudu-cli asset-passwords unarchive`  -  Unarchive a password entry
- `hudu-cli asset-passwords update`  -  Update a password vault entry

**assets**  -  Assets  -  configuration items typed by an asset layout, owned by a company

- `hudu-cli assets archive`  -  Archive an asset
- `hudu-cli assets create`  -  Create an asset under a company. custom_fields is a JSON array string matching the asset layout's fields.
- `hudu-cli assets delete`  -  Delete an asset
- `hudu-cli assets get`  -  Get a single asset
- `hudu-cli assets list`  -  Global asset list/search (paginated). NOTE: fails on company-scoped API keys  -  use list-by-company instead.
- `hudu-cli assets list-by-company`  -  List assets for a single company (works with company-scoped keys)
- `hudu-cli assets move-layout`  -  Move an asset to a different asset layout
- `hudu-cli assets unarchive`  -  Unarchive an asset
- `hudu-cli assets update`  -  Update an asset. Sending a different company_id in the body MOVES the asset to that company.

**cards**  -  Integration card lookup  -  resolve an external integrator record to its Hudu object

- `hudu-cli cards`  -  Look up Hudu objects by an external integrator's record id

**companies**  -  Client companies  -  the root tenant-of-record everything else hangs off

- `hudu-cli companies archive`  -  Archive a company
- `hudu-cli companies create`  -  Create a company
- `hudu-cli companies delete`  -  Delete a company
- `hudu-cli companies get`  -  Get a single company by id
- `hudu-cli companies list`  -  List/search companies (paginated)
- `hudu-cli companies unarchive`  -  Unarchive a company
- `hudu-cli companies update`  -  Update a company

**expirations**  -  Read-only rollup of dated items (SSL, domains, warranties, passwords)

- `hudu-cli expirations`  -  List upcoming/expiring items across the instance (paginated)

**exports**  -  Data exports (recent Hudu versions)

- `hudu-cli exports create`  -  Start an export
- `hudu-cli exports create-s3`  -  Start an export targeted at S3
- `hudu-cli exports get`  -  Get a single export's status
- `hudu-cli exports list`  -  List/poll exports (paginated)

**flag-types**  -  Flag type definitions (Hudu 2.4.0+)

- `hudu-cli flag-types create`  -  Create a flag type. Requires Hudu 2.4.0+.
- `hudu-cli flag-types delete`  -  Delete a flag type. Requires Hudu 2.4.0+.
- `hudu-cli flag-types list`  -  List flag types (paginated). Requires Hudu 2.4.0+.
- `hudu-cli flag-types update`  -  Update a flag type. Requires Hudu 2.4.0+.

**flags**  -  Flags (Hudu 2.4.0+)  -  gate on api-info version before use

- `hudu-cli flags create`  -  Create a flag. Requires Hudu 2.4.0+.
- `hudu-cli flags delete`  -  Delete a flag. Requires Hudu 2.4.0+.
- `hudu-cli flags list`  -  List flags (paginated). Requires Hudu 2.4.0+.
- `hudu-cli flags update`  -  Update a flag. Requires Hudu 2.4.0+.

**folders**  -  Folders for organizing articles and assets

- `hudu-cli folders create`  -  Create a folder
- `hudu-cli folders get`  -  Get a single folder
- `hudu-cli folders list`  -  List folders (paginated)
- `hudu-cli folders update`  -  Update a folder

**groups**  -  User/permission groups (read-only)

- `hudu-cli groups get`  -  Get a single group
- `hudu-cli groups list`  -  List groups (paginated)

**ip-addresses**  -  IP address records

- `hudu-cli ip-addresses create`  -  Create an IP address record
- `hudu-cli ip-addresses delete`  -  Delete an IP address record
- `hudu-cli ip-addresses get`  -  Get a single IP address record
- `hudu-cli ip-addresses list`  -  List IP address records (not paginated: returns the whole collection)
- `hudu-cli ip-addresses update`  -  Update an IP address record

**lists**  -  Custom lists (dropdown option sets)

- `hudu-cli lists create`  -  Create a custom list. list_items is a JSON array of option objects.
- `hudu-cli lists delete`  -  Delete a custom list
- `hudu-cli lists get`  -  Get a single list (with its options)
- `hudu-cli lists list`  -  List custom lists (paginated)
- `hudu-cli lists update`  -  Update a custom list

**magic-dash**  -  Magic Dash  -  custom dashboard tiles on a company page

- `hudu-cli magic-dash delete`  -  Delete a Magic Dash tile by id
- `hudu-cli magic-dash list`  -  List Magic Dash tiles (paginated)
- `hudu-cli magic-dash set`  -  Create or update a Magic Dash tile (upsert keyed by company_name + title)

**matchers**  -  Integration matchers  -  reconcile Hudu records to PSA/RMM records

- `hudu-cli matchers list`  -  List integration matchers for one integration (paginated; --integration-id is required)
- `hudu-cli matchers update`  -  Update a matcher (link/unlink an integrator record to a Hudu company)

**networks**  -  Network records

- `hudu-cli networks create`  -  Create a network
- `hudu-cli networks delete`  -  Delete a network
- `hudu-cli networks get`  -  Get a single network
- `hudu-cli networks list`  -  List networks (not paginated: returns the whole collection)
- `hudu-cli networks update`  -  Update a network

**procedure-tasks**  -  Tasks within a procedure

- `hudu-cli procedure-tasks create`  -  Create a procedure task (body shape changed at Hudu 2.4.1)
- `hudu-cli procedure-tasks delete`  -  Delete a procedure task
- `hudu-cli procedure-tasks get`  -  Get a single procedure task
- `hudu-cli procedure-tasks list`  -  List procedure tasks (paginated)
- `hudu-cli procedure-tasks update`  -  Update a procedure task

**procedures**  -  Process templates and running procedures (UI label: Processes)

- `hudu-cli procedures create`  -  Create a procedure (process template)
- `hudu-cli procedures create-from-template`  -  Instantiate a procedure from a template into a company
- `hudu-cli procedures delete`  -  Delete a procedure
- `hudu-cli procedures duplicate`  -  Duplicate a procedure
- `hudu-cli procedures get`  -  Get a single procedure
- `hudu-cli procedures kickoff`  -  Kick off (start) a procedure run
- `hudu-cli procedures list`  -  List procedures (paginated)
- `hudu-cli procedures update`  -  Update a procedure

**public-photos**  -  Public photo gallery (inline images for articles)

- `hudu-cli public-photos`  -  List public photos (paginated)

**rack-storage-items**  -  Items mounted in a rack storage unit

- `hudu-cli rack-storage-items create`  -  Mount an item in a rack
- `hudu-cli rack-storage-items delete`  -  Delete a rack storage item
- `hudu-cli rack-storage-items get`  -  Get a single rack storage item
- `hudu-cli rack-storage-items list`  -  List rack storage items (not paginated: returns the whole collection)
- `hudu-cli rack-storage-items update`  -  Update a rack storage item

**rack-storages**  -  Rack storage units

- `hudu-cli rack-storages create`  -  Create a rack storage unit (Hudu uses a POST with id segment for this resource)
- `hudu-cli rack-storages delete`  -  Delete a rack storage unit
- `hudu-cli rack-storages get`  -  Get a single rack storage unit
- `hudu-cli rack-storages list`  -  List rack storage units (paginated)
- `hudu-cli rack-storages update`  -  Update a rack storage unit

**relations**  -  Generic many-to-many links between any two entities

- `hudu-cli relations create`  -  Create a relation between two entities
- `hudu-cli relations delete`  -  Delete a relation
- `hudu-cli relations list`  -  List relations (paginated)

**uploads**  -  File uploads

- `hudu-cli uploads delete`  -  Delete an upload
- `hudu-cli uploads get`  -  Get a single upload's metadata
- `hudu-cli uploads list`  -  List uploads (paginated)

**users**  -  Users (read-only)

- `hudu-cli users get`  -  Get a single user
- `hudu-cli users list`  -  List users (paginated)

**vlan-zones**  -  VLAN zones

- `hudu-cli vlan-zones create`  -  Create a VLAN zone
- `hudu-cli vlan-zones delete`  -  Delete a VLAN zone
- `hudu-cli vlan-zones get`  -  Get a single VLAN zone
- `hudu-cli vlan-zones list`  -  List VLAN zones (not paginated: returns the whole collection)
- `hudu-cli vlan-zones update`  -  Update a VLAN zone

**vlans**  -  VLAN records

- `hudu-cli vlans create`  -  Create a VLAN
- `hudu-cli vlans delete`  -  Delete a VLAN
- `hudu-cli vlans get`  -  Get a single VLAN
- `hudu-cli vlans list`  -  List VLANs (not paginated: returns the whole collection)
- `hudu-cli vlans update`  -  Update a VLAN

**websites**  -  Monitored websites with SSL/domain expiration tracking

- `hudu-cli websites create`  -  Add a monitored website
- `hudu-cli websites delete`  -  Delete a monitored website
- `hudu-cli websites get`  -  Get a single website
- `hudu-cli websites list`  -  List monitored websites (paginated)
- `hudu-cli websites update`  -  Update a monitored website


**audit**


- `hudu-cli audit completeness`  -  Score how completely each company's assets fill their layout's required fields, worst-first.

- `hudu-cli audit stale-passwords`  -  Find vault passwords not rotated within a threshold, grouped by company.

- `hudu-cli audit expirations`  -  Windowed, typed view of expiring SSL certs, domains, warranties, and passwords sorted by days remaining.

- `hudu-cli audit summary`  -  One worst-first hygiene scorecard per company combining completeness, stale passwords, expirations due, and stale articles.

- `hudu-cli audit layout-drift`  -  Find assets carrying custom fields not in their layout's current schema, or missing newly-added fields.

- `hudu-cli audit stale-articles`  -  Rank knowledge-base articles by how long since they were last updated.


**onboard**


- `hudu-cli onboard`  -  Preview (default) and apply with --apply a saved bundle of asset layouts, folder tree, and procedures-from-template to a new company; the write path needs a global (not company-scoped) API key.


**reconcile**


- `hudu-cli reconcile`  -  Bulk bidirectional check of which integrator (PSA/RMM) records resolve to a live Hudu asset and which are orphaned.


**resolve**


- `hudu-cli resolve`  -  Paste a Hudu object URL or exact name and get the object plus its company, layout, and relations.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
hudu-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

## Auth Setup

Hudu is per-tenant. Set HUDU_BASE_URL to your instance's /api/v1 URL (self-hosted, *.huducloud.com, or a custom domain) and HUDU_API_KEY to a key created under Admin -> API Keys. The key is sent as the x-api-key header. Keys are global or company-scoped; a company-scoped key cannot call the global `assets list` (use `assets list-by-company`). Run `doctor` to confirm the key, base URL, and reachability; use `api-info get` for the Hudu version. Save the key with `auth set-token` (or env vars) and check it with `auth status`.

Run `hudu-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  hudu-cli activity-logs --agent --select id,user_id,user_email
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `HUDU_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `HUDU_CONFIG_DIR`, `HUDU_DATA_DIR`, `HUDU_STATE_DIR`, `HUDU_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `HUDU_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `hudu-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

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

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `HUDU_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `HUDU_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. Vault passwords, OTP secrets and password-type asset field values are never part of either; a store written by an older release is scrubbed the first time this release opens it. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `HUDU_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `HUDU_DATA_SOURCE` sets the `--data-source` default.

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
hudu-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "hudu-cli learnings confirm 12"] }
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
       materially more, record the divergence via `hudu-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `hudu-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `hudu-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
hudu-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
hudu-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
hudu-cli teach-playbook \
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
hudu-cli playbook amend \
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

`hudu-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `HUDU_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
hudu-cli feedback "the --since flag is inclusive but docs say exclusive"
hudu-cli feedback --stdin < notes.txt
hudu-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `HUDU_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `HUDU_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
hudu-cli profile save briefing --json
hudu-cli --profile briefing activity-logs
hudu-cli profile list --json
hudu-cli profile show briefing
hudu-cli profile delete briefing --yes
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
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `hudu-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add hudu-mcp -- hudu-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `hudu-mcp --transport http` requires `HUDU_MCP_HTTP_TOKEN` (callers send `Authorization: Bearer <token>`), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. `HUDU_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which hudu-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   hudu-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `hudu-cli <command> --help`.
