# Salesbuildr CLI

**Every Salesbuildr resource as a scriptable command, plus offline margin and pipeline analytics.**

salesbuildr-cli mirrors the full Salesbuildr Public API  -  companies, contacts, products, opportunities, quotes, pricing books, templates and more  -  with agent-native JSON output and a local SQLite store. The local store is what unlocks the commands no other Salesbuildr tool has: stale-quote radar, low-margin line detection, pricing drift, pipeline velocity, win-rate, MRR forecast, and company whitespace.

## Install

The recommended path installs both the `salesbuildr-cli` binary and the `pp-salesbuildr` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install salesbuildr
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install salesbuildr --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install salesbuildr --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install salesbuildr --agent claude-code
npx -y @mvanhorn/printing-press-library install salesbuildr --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/sales-and-crm/salesbuildr/cmd/salesbuildr-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/salesbuildr-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install salesbuildr --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-salesbuildr --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-salesbuildr --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install salesbuildr --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/salesbuildr-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `SALESBUILDR_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/sales-and-crm/salesbuildr/cmd/salesbuildr-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "salesbuildr": {
      "command": "salesbuildr-mcp",
      "env": {
        "SALESBUILDR_TENANT": "<tenant>",
        "SALESBUILDR_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Set SALESBUILDR_API_KEY to an API key generated under Admin > Integrations > API Key, and SALESBUILDR_TENANT to your tenant subdomain (acme for https://acme.salesbuildr.com). Advanced: SALESBUILDR_BASE_URL overrides the full base URL and must include the /public-api suffix (https://acme.salesbuildr.com/public-api). The CLI sends the key in the api-key header on every request.

## Quick Start

```bash
# confirm the API key and tenant base URL are wired up
salesbuildr-cli doctor

# pull companies, contacts, products, opportunities and quotes into the local store
salesbuildr-cli sync

# see which open quotes are aging and the revenue at risk
salesbuildr-cli quote stale --days 14

# find quote lines priced below a 20% markup floor
salesbuildr-cli quote thin --floor 20 --json

# conversion by rep from the synced pipeline
salesbuildr-cli opportunity winrate --by owner

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local analytics that compound
- **`quote stale`**  -  Surface quotes that have been sent or approved but are aging past a cutoff, with the dollar value still at risk.

  _Reach for this when you need the one list the web UI never shows: which open quotes are slipping and how much revenue is on the line._

  ```bash
  salesbuildr-cli quote stale --days 14 --agent
  ```
- **`product velocity`**  -  Rank catalog products by how often they appear across quote line items and the total value quoted  -  your de-facto bestsellers.

  _Reach for this for most-quoted products by frequency and value; to find products a company has never been quoted, use 'company whitespace' instead._

  ```bash
  salesbuildr-cli product velocity --limit 20 --agent
  ```
- **`reconcile-psa`**  -  Find rows across companies, contacts, products, and opportunities whose external identifier is missing  -  the records that silently miss a PSA sync.

  _Reach for this before an Autotask/ConnectWise/HaloPSA sync to catch records that will be skipped for lack of an external id._

  ```bash
  salesbuildr-cli reconcile-psa --agent
  ```
- **`company whitespace`**  -  Show catalog products a company has never been quoted  -  the cross-sell gaps.

  _Use this before an account review to find concrete cross-sell opportunities grounded in what the company has not bought._

  ```bash
  salesbuildr-cli company whitespace 1234 --agent
  ```

### Margin intelligence
- **`quote thin`**  -  Find every quote line item across all open quotes whose markup falls below a floor.

  _Use this before quotes go out to catch under-priced lines that silently erode margin when distributor costs move._

  ```bash
  salesbuildr-cli quote thin --floor 20 --agent
  ```
- **`pricing drift`**  -  Flag per-company pricing-book prices that have diverged from the master catalog price or cost.

  _Run this when a distributor price-list lands to see exactly which pricing books drifted away from catalog._

  ```bash
  salesbuildr-cli pricing drift --agent
  ```

### Pipeline analytics
- **`quote funnel`**  -  See stage-by-stage conversion of the quote lifecycle  -  count and dollar value at draft, sent, approved, declined, and expired.

  _Reach for this when you need quote-lifecycle conversion with dollars at each stage; for opportunity-pipeline conversion use 'opportunity winrate' instead._

  ```bash
  salesbuildr-cli quote funnel --agent
  ```
- **`opportunity velocity`**  -  Aggregate sales-cycle duration and time-in-stage across the opportunity pipeline.

  _Pick this when you need to know where deals stall and how long the pipeline actually takes to close._

  ```bash
  salesbuildr-cli opportunity velocity --agent
  ```
- **`opportunity winrate`**  -  Compute won/lost ratios grouped by owner, stage, or category.

  _Use this to report conversion by rep or stage without rebuilding it from memory each week._

  ```bash
  salesbuildr-cli opportunity winrate --by owner --agent
  ```
- **`opportunity mrr-forecast`**  -  Weight open-pipeline monthly revenue and profit by probability, churn-adjusted.

  _Reach for this for a fast weighted MRR forecast across the open pipeline._

  ```bash
  salesbuildr-cli opportunity mrr-forecast --agent
  ```

## Recipes


### Weekly pipeline review

```bash
salesbuildr-cli sync && salesbuildr-cli quote stale --days 14 && salesbuildr-cli opportunity velocity
```

Refresh the store, then list aging quotes and pipeline cycle-time in one pass.

### Margin guard before sending

```bash
salesbuildr-cli quote thin --floor 20 --agent
```

List only the under-priced lines with the fields that matter, narrowed for an agent.

### Distributor price-list landed

```bash
salesbuildr-cli sync && salesbuildr-cli pricing drift
```

After re-syncing the catalog, flag pricing books whose prices drifted from catalog.

### Account-review cross-sell

```bash
salesbuildr-cli company whitespace "Acme Managed IT" --json
```

Catalog products this company has never been quoted, ready to pipe into prep notes.

### Catalog margin audit in SQL

```bash
salesbuildr-cli sql "SELECT name, mpn, markup, price FROM product WHERE markup < 25 ORDER BY markup"
```

Query the synced catalog offline for products priced below a 25% markup floor.

## Usage

Run `salesbuildr-cli --help` for the full command reference and flag list.

## Commands

### category

Manage category

- **`salesbuildr-cli category public-get-all-categories`** - Get all categories
- **`salesbuildr-cli category public-get-all-categories-as-tree`** - Get all categories as tree

### company

Manage company

- **`salesbuildr-cli company public-create`** - Create
- **`salesbuildr-cli company public-delete`** - Delete
- **`salesbuildr-cli company public-delete-by-external-identifier`** - Delete by ext ID
- **`salesbuildr-cli company public-get`** - Get
- **`salesbuildr-cli company public-get-by-external-identifier`** - Get by ext ID
- **`salesbuildr-cli company public-get-list`** - Search
- **`salesbuildr-cli company public-update`** - Update
- **`salesbuildr-cli company public-upsert`** - Create or update a company using the ext ID as key

### contact

Manage contact

- **`salesbuildr-cli contact company-public-create`** - Create
- **`salesbuildr-cli contact company-public-delete`** - Delete by ext ID
- **`salesbuildr-cli contact company-public-delete-by-id`** - Delete by ID
- **`salesbuildr-cli contact company-public-get`** - Get
- **`salesbuildr-cli contact company-public-get-by-external-identifier`** - Get by ext ID
- **`salesbuildr-cli contact company-public-get-list`** - Search contacts
- **`salesbuildr-cli contact company-public-undelete-by-ext-id`** - Undelete by ext ID
- **`salesbuildr-cli contact company-public-update`** - Update
- **`salesbuildr-cli contact company-public-upsert`** - Upsert by ext ID
- **`salesbuildr-cli contact company-public-upsert-by-email`** - Upsert by email address

### contract

Manage contract

- **`salesbuildr-cli contract`** - ⚠️ **Beta API** - This endpoint is in beta and may change without notice. Use with caution in production environments.

### field

Manage field

- **`salesbuildr-cli field public-get-values`** - Get the list of values from a field
- **`salesbuildr-cli field public-update-values`** - Replace existing field values with new list

### opportunity

Manage opportunity

- **`salesbuildr-cli opportunity public-get-by-id`** - Get by ID
- **`salesbuildr-cli opportunity public-get-opportunities`** - Search
- **`salesbuildr-cli opportunity public-lose`** - Lose by ext ID
- **`salesbuildr-cli opportunity public-upsert`** - Create or update a opportunity using the ext ID as key
- **`salesbuildr-cli opportunity public-win`** - Win by ext ID

### pricing-book

Manage pricing book

- **`salesbuildr-cli pricing-book public-create`** - Create a new pricing book
- **`salesbuildr-cli pricing-book public-get`** - Get pricing book by ID
- **`salesbuildr-cli pricing-book public-get-all`** - Get pricing books
- **`salesbuildr-cli pricing-book public-update`** - Update an existing pricing book

### product

Manage product

- **`salesbuildr-cli product public-create`** - Create product
- **`salesbuildr-cli product public-create-batch`** - Create products in batch. Maximum 100 products
- **`salesbuildr-cli product public-delete`** - Delete product
- **`salesbuildr-cli product public-delete-by-external-identifier`** - Delete product by external identifier
- **`salesbuildr-cli product public-get`** - Search
- **`salesbuildr-cli product public-get-by-external-identifier`** - Get product by external identifier
- **`salesbuildr-cli product public-get-id`** - Get product by ID
- **`salesbuildr-cli product public-update`** - Update product
- **`salesbuildr-cli product public-upsert-by-external-identifier`** - Upsert product by external identifier

### quote

Manage quote

- **`salesbuildr-cli quote public-create-approved`** - Create approved quote
- **`salesbuildr-cli quote public-create-draft`** - Create draft quote
- **`salesbuildr-cli quote public-get`** - Search
- **`salesbuildr-cli quote public-get-approved`** - Search approved
- **`salesbuildr-cli quote public-get-id`** - Get

### quote-discount-group

Manage quote discount group

- **`salesbuildr-cli quote-discount-group discount-group-public-create`** - Create new
- **`salesbuildr-cli quote-discount-group discount-group-public-get-all`** - Get all
- **`salesbuildr-cli quote-discount-group discount-group-public-get-by-id`** - Get by id
- **`salesbuildr-cli quote-discount-group discount-group-public-update`** - Update by id

### quote-template

Manage quote template

- **`salesbuildr-cli quote-template public-create`** - Creates a new quote template. All product references should use internal Sfindr product IDs.
- **`salesbuildr-cli quote-template public-get-by-id`** - Returns a single quote template with all its contents. The response can be used directly in PUT requests.
- **`salesbuildr-cli quote-template public-list`** - Returns a list of all quote templates ordered by their display order.
- **`salesbuildr-cli quote-template public-update`** - Updates an existing quote template. All product references should use internal Sfindr product IDs. Only provided fields will be updated.
- **`salesbuildr-cli quote-template public-validate-template`** - Validates template data including product references and attachments. Returns the validated/filtered data that would be used in create/update operations.

### quote-widget-template

Manage quote widget template

- **`salesbuildr-cli quote-widget-template public-create`** - Creates a new widget template. The widget configuration should match the structure used in copy/paste operations. All product references should use internal Sfindr product IDs.
- **`salesbuildr-cli quote-widget-template public-get-by-id`** - Returns a single widget template with all its contents. The response can be used directly in PUT requests.
- **`salesbuildr-cli quote-widget-template public-list`** - Returns a list of all widget templates ordered by their display order.
- **`salesbuildr-cli quote-widget-template public-update`** - Updates an existing widget template. Only provided fields will be updated. All product references should use internal Sfindr product IDs.
- **`salesbuildr-cli quote-widget-template public-validate-widget`** - Validates widget template data including product references. Returns the validated data that would be used in create/update operations.

### sync-state

Manage sync state

- **`salesbuildr-cli sync-state <id>`** - This endpoint is exempt from rate limiting


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
salesbuildr-cli category public-get-all-categories

# JSON for scripting and agents
salesbuildr-cli category public-get-all-categories --json

# Filter to specific fields
salesbuildr-cli category public-get-all-categories --json --select id,name,status

# Dry run  -  show the request without sending
salesbuildr-cli category public-get-all-categories --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
salesbuildr-cli category public-get-all-categories --agent
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

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `SALESBUILDR_TENANT` resolves `{tenant}`

Base URL: `https://{tenant}.salesbuildr.com/public-api`

## Health Check

```bash
salesbuildr-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/salesbuildr-public-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SALESBUILDR_TENANT` | endpoint | Yes |  |
| `SALESBUILDR_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `salesbuildr-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `salesbuildr-cli doctor` to check credentials
- Verify the environment variable is set: `echo $SALESBUILDR_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 No API key provided**  -  Set SALESBUILDR_API_KEY (Admin > Integrations > API Key) and re-run doctor.
- **404 or connection error on every call**  -  Set SALESBUILDR_TENANT to your tenant subdomain (acme for https://acme.salesbuildr.com), not the docs portal. If you must override SALESBUILDR_BASE_URL, include the /public-api suffix.
- **429 Too Many Requests**  -  The tenant limit is 500 requests per 10 minutes; honor the Retry-After header  -  sync paginates and backs off automatically.
- **Analytics commands return empty**  -  Run sync first  -  stale, thin, velocity, winrate and whitespace read the local store, not the live API.
