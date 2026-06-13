# Hudu CLI

**Every Hudu cmdlet, plus an offline SQLite mirror, cross-entity audits, and agent-native output no PowerShell module or read-only MCP ships.**

The first general-purpose Hudu CLI. It covers the full ~120-operation Hudu API surface with write support, adds full write parity plus a local SQLite mirror not present in read-only MCP servers, and makes documentation-hygiene audits possible offline: completeness scoring, stale-password and stale-article detection, an expiration radar, a cross-tenant hygiene rollup, and integration reconciliation  -  none of which exist in the Hudu UI or API. Built for MSP technicians who live in a terminal and for AI agents that need --json/--select and typed exit codes.

## Install

The recommended path installs both the `hudu-cli` binary and the `pp-hudu` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install hudu
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install hudu --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install hudu --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install hudu --agent claude-code
npx -y @mvanhorn/printing-press-library install hudu --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/other/hudu/cmd/hudu-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/hudu-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install hudu --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-hudu --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-hudu --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install hudu --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/hudu-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `HUDU_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/other/hudu/cmd/hudu-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

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

## Usage

Run `hudu-cli --help` for the full command reference and flag list.

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
- **`hudu-cli asset-passwords get`** - Get a single password entry
- **`hudu-cli asset-passwords list`** - List password vault entries (paginated). The secret is returned only on single get, never the list, per Hudu.
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
- **`hudu-cli ip-addresses list`** - List IP address records (paginated)
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

- **`hudu-cli matchers list`** - List integration matchers (paginated)
- **`hudu-cli matchers update`** - Update a matcher (link/unlink an integrator record to a Hudu company)

### networks

Network records

- **`hudu-cli networks create`** - Create a network
- **`hudu-cli networks delete`** - Delete a network
- **`hudu-cli networks get`** - Get a single network
- **`hudu-cli networks list`** - List networks (paginated)
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
- **`hudu-cli rack-storage-items list`** - List rack storage items (paginated)
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
- **`hudu-cli vlan-zones list`** - List VLAN zones (paginated)
- **`hudu-cli vlan-zones update`** - Update a VLAN zone

### vlans

VLAN records

- **`hudu-cli vlans create`** - Create a VLAN
- **`hudu-cli vlans delete`** - Delete a VLAN
- **`hudu-cli vlans get`** - Get a single VLAN
- **`hudu-cli vlans list`** - List VLANs (paginated)
- **`hudu-cli vlans update`** - Update a VLAN

### websites

Monitored websites with SSL/domain expiration tracking

- **`hudu-cli websites create`** - Add a monitored website
- **`hudu-cli websites delete`** - Delete a monitored website
- **`hudu-cli websites get`** - Get a single website
- **`hudu-cli websites list`** - List monitored websites (paginated)
- **`hudu-cli websites update`** - Update a monitored website


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
hudu-cli activity-logs

# JSON for scripting and agents
hudu-cli activity-logs --json

# Filter to specific fields
hudu-cli activity-logs --json --select id,name,status

# Dry run  -  show the request without sending
hudu-cli activity-logs --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
hudu-cli activity-logs --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
hudu-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/hudu-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

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
