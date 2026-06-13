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
      - kind: go
        bins: [hudu-cli]
        module: github.com/mvanhorn/printing-press-library/library/other/hudu/cmd/hudu-cli
---

# Hudu  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `hudu-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer. It defaults binaries to `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows:
   ```bash
   npx -y @mvanhorn/printing-press-library install hudu --cli-only
   ```
2. Verify: `hudu-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If the `npx` install fails (no Node, offline, etc.), fall back to a direct Go install (requires Go 1.26.4 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so add that directory to `$PATH` instead:

```bash
go install github.com/mvanhorn/printing-press-library/library/other/hudu/cmd/hudu-cli@latest
```

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

The first general-purpose Hudu CLI. It covers the full ~120-operation Hudu API surface with write support, adds full write parity plus a local SQLite mirror not present in read-only MCP servers, and makes documentation-hygiene audits possible offline: completeness scoring, stale-password and stale-article detection, an expiration radar, a cross-tenant hygiene rollup, and integration reconciliation  -  none of which exist in the Hudu UI or API. Built for MSP technicians who live in a terminal and for AI agents that need --json/--select and typed exit codes.

## When to Use This CLI

Use this CLI when an agent or technician needs to read, write, or audit Hudu IT documentation from the terminal: bulk asset and company maintenance, password-vault and expiration audits, documentation-completeness scoring before a QBR, new-client onboarding scaffolding, or reconciling PSA/RMM integration records. Prefer `sync` then local `audit`/`search`/`analytics` for anything that would otherwise make many live calls against the 300/min limit.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI for fuzzy keyword discovery when you mean full-text search across resources  -  that is `search`, not `resolve` (resolve is exact URL/name lookup).
- Do not use the write path of `onboard` with a company-scoped API key  -  creating layouts/folders needs a global key; preview (`onboard` without --apply) works regardless.
- Do not use this CLI to read or export password secret values  -  the vault mirror stores metadata only (name, username, dates); fetch secrets in the Hudu portal.
- Do not use this CLI against Hudu instances older than 2.4.0 for flags/flag-types or recent procedure-task shapes  -  run `doctor` first; those endpoints are version-gated.
- Do not script bulk live reads in a tight loop  -  Hudu rate-limits at 300 req/min over a 5-minute window; `sync` once and run audits/search locally instead.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

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
- `hudu-cli asset-passwords get`  -  Get a single password entry
- `hudu-cli asset-passwords list`  -  List password vault entries (paginated). The secret is returned only on single get, never the list, per Hudu.
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
- `hudu-cli ip-addresses list`  -  List IP address records (paginated)
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

- `hudu-cli matchers list`  -  List integration matchers (paginated)
- `hudu-cli matchers update`  -  Update a matcher (link/unlink an integrator record to a Hudu company)

**networks**  -  Network records

- `hudu-cli networks create`  -  Create a network
- `hudu-cli networks delete`  -  Delete a network
- `hudu-cli networks get`  -  Get a single network
- `hudu-cli networks list`  -  List networks (paginated)
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
- `hudu-cli rack-storage-items list`  -  List rack storage items (paginated)
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
- `hudu-cli vlan-zones list`  -  List VLAN zones (paginated)
- `hudu-cli vlan-zones update`  -  Update a VLAN zone

**vlans**  -  VLAN records

- `hudu-cli vlans create`  -  Create a VLAN
- `hudu-cli vlans delete`  -  Delete a VLAN
- `hudu-cli vlans get`  -  Get a single VLAN
- `hudu-cli vlans list`  -  List VLANs (paginated)
- `hudu-cli vlans update`  -  Update a VLAN

**websites**  -  Monitored websites with SSL/domain expiration tracking

- `hudu-cli websites create`  -  Add a monitored website
- `hudu-cli websites delete`  -  Delete a monitored website
- `hudu-cli websites get`  -  Get a single website
- `hudu-cli websites list`  -  List monitored websites (paginated)
- `hudu-cli websites update`  -  Update a monitored website


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
hudu-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

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

## Auth Setup

Hudu is per-tenant. Set HUDU_BASE_URL to your instance's /api/v1 URL (self-hosted, *.huducloud.com, or a custom domain) and HUDU_API_KEY to a key created under Admin -> API Keys. The key is sent as the x-api-key header. Keys are global or company-scoped; a company-scoped key cannot call the global `assets list` (use `assets list-by-company`). Run `doctor` to confirm the key, base URL, and reachability; use `api-info get` for the Hudu version. Save the key with `auth set-token` (or env vars) and check it with `auth status`.

Run `hudu-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  hudu-cli activity-logs --agent --select id,name,status
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
hudu-cli feedback "the --since flag is inclusive but docs say exclusive"
hudu-cli feedback --stdin < notes.txt
hudu-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/hudu-cli/feedback.jsonl`. They are never POSTed unless `HUDU_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `HUDU_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `hudu-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/other/hudu/cmd/hudu-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add hudu-mcp -- hudu-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which hudu-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   hudu-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `hudu-cli <command> --help`.
