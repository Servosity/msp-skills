# Salesbuildr CLI

**Every Salesbuildr resource as a scriptable command, plus offline margin and pipeline analytics.**

salesbuildr-cli mirrors the full Salesbuildr Public API  -  companies, contacts, products, opportunities, quotes, pricing books, templates and more  -  with agent-native JSON output and a local SQLite store. The local store is what unlocks the commands no other Salesbuildr tool has: stale-quote radar, low-margin line detection, pricing drift, pipeline velocity, win-rate, MRR forecast, and company whitespace.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `salesbuildr-cli` and `salesbuildr-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/salesbuildr/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/salesbuildr/install.ps1 | iex
   ```
3. Verify: `salesbuildr-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=salesbuildr). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/salesbuildr --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/salesbuildr --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `salesbuildr-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the salesbuildr skill from https://github.com/Servosity/msp-skills/tree/main/skills/salesbuildr. The skill defines how its required CLI (`salesbuildr-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=salesbuildr).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `SALESBUILDR_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. A bundle carries the five platform binaries the builder downloads - macOS (`darwin-arm64`, `darwin-amd64`), Linux (`linux-arm64`, `linux-amd64`) and Windows (`windows-amd64`). Windows on ARM is released as a standalone binary but is not bundled, so use the manual config below there.

> **Claude Desktop bundle:** the `.mcpb` launches on macOS (Intel and Apple Silicon), Windows x64 and Linux x64. Every tool shells out to the companion `salesbuildr-cli`. Releases cut from 2026-09-18 on ship that CLI inside the bundle; older bundles contain only the MCP server, so if yours lacks the companion, run the installer above first (or set `SALESBUILDR_CLI_PATH` to an existing binary). Details: [#331](https://github.com/Servosity/msp-skills/issues/331).

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/salesbuildr/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/salesbuildr/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

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

### Remote MCP over HTTP

`salesbuildr-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `SALESBUILDR_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `SALESBUILDR_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export SALESBUILDR_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
salesbuildr-mcp --transport http
```

Set `SALESBUILDR_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

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

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `SALESBUILDR_CONFIG_DIR`, `SALESBUILDR_DATA_DIR`, `SALESBUILDR_STATE_DIR`, or `SALESBUILDR_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `SALESBUILDR_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export SALESBUILDR_HOME=/srv/salesbuildr
salesbuildr-cli doctor
```

On Windows (PowerShell):

```powershell
$env:SALESBUILDR_HOME = "C:\ProgramData\salesbuildr"
salesbuildr-cli doctor
```

Under `SALESBUILDR_HOME=/srv/salesbuildr`, the four dirs resolve to `/srv/salesbuildr/config`, `/srv/salesbuildr/data`, `/srv/salesbuildr/state`, and `/srv/salesbuildr/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:SALESBUILDR_HOME = "C:\srv\salesbuildr"
salesbuildr-cli doctor
```

Under `$env:SALESBUILDR_HOME = "C:\srv\salesbuildr"`, the four dirs resolve to `C:\srv\salesbuildr\config`, `C:\srv\salesbuildr\data`, `C:\srv\salesbuildr\state`, and `C:\srv\salesbuildr\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "salesbuildr": {
      "command": "salesbuildr-mcp",
      "env": {
        "SALESBUILDR_HOME": "/srv/salesbuildr"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `SALESBUILDR_DATA_DIR` overrides an explicit `--home` for that kind. Use `SALESBUILDR_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `SALESBUILDR_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `salesbuildr-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `salesbuildr-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `salesbuildr-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `SALESBUILDR_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `SALESBUILDR_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

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


### company


- **`salesbuildr-cli company whitespace`** - Show catalog products a company has never been quoted  -  the cross-sell gaps.


### opportunity


- **`salesbuildr-cli opportunity velocity`** - Aggregate sales-cycle duration and time-in-stage across the opportunity pipeline.

- **`salesbuildr-cli opportunity winrate`** - Compute won/lost ratios grouped by owner, stage, or category.

- **`salesbuildr-cli opportunity mrr-forecast`** - Weight open-pipeline monthly revenue and profit by probability, churn-adjusted.


### pricing


- **`salesbuildr-cli pricing drift`** - Flag per-company pricing-book prices that have diverged from the master catalog price or cost.


### product


- **`salesbuildr-cli product velocity`** - Rank catalog products by how often they appear across quote line items and the total value quoted  -  your de-facto bestsellers.


### quote


- **`salesbuildr-cli quote stale`** - Surface quotes that have been sent or approved but are aging past a cutoff, with the dollar value still at risk.

- **`salesbuildr-cli quote thin`** - Find every quote line item across all open quotes whose markup falls below a floor.

- **`salesbuildr-cli quote funnel`** - See stage-by-stage conversion of the quote lifecycle  -  count and dollar value at draft, sent, approved, declined, and expired.


### reconcile-psa


- **`salesbuildr-cli reconcile-psa`** - Find rows across companies, contacts, products, and opportunities whose external identifier is missing  -  the records that silently miss a PSA sync.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`salesbuildr-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`salesbuildr-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`salesbuildr-cli learnings list`** - Inspect taught rows
- **`salesbuildr-cli learnings forget <query>`** - Undo a teach
- **`salesbuildr-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`salesbuildr-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`salesbuildr-cli teach-pattern`** - Install a query/resource template up front
- **`salesbuildr-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `SALESBUILDR_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `salesbuildr-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
salesbuildr-cli category public-get-all-categories

# JSON for scripting and agents
salesbuildr-cli category public-get-all-categories --json
# Filter to specific fields
salesbuildr-cli category public-get-all-categories --json --select addingProductByAdminOnly,addons,billingCode

# Dry run  -  show the request without sending
salesbuildr-cli category public-get-all-categories --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
salesbuildr-cli category public-get-all-categories --agent
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

Run `salesbuildr-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/salesbuildr-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\salesbuildr-cli\config.toml` on Windows; `--home`, `SALESBUILDR_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `SALESBUILDR_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `SALESBUILDR_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SALESBUILDR_TENANT` | endpoint | Yes |  |
| `SALESBUILDR_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `salesbuildr-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- The `api-key` header and every API request go only to the scheme, host and port of the API base URL: `https://<SALESBUILDR_TENANT>.salesbuildr.com/public-api`, or `SALESBUILDR_BASE_URL` when set. A redirect to any other host is refused, so the key never reaches it. `SALESBUILDR_TENANT` must be one subdomain label (`acme` for `https://acme.salesbuildr.com`); any other value, or the placeholder `your-tenant`, refuses every request.
- Any output, file, local store write or MCP result that would carry `SALESBUILDR_API_KEY`, the stored key, an `--integration-key` value, or a credential field from a response (`password`, `otp`, `pin`, `passcode`, `integrationKey`), in any encoding, is withheld whole: the program prints "[credkit: withheld from stdout: it carried a credential this program holds]" (naming the sink) in its place. Responses are never cached on disk.
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the key from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, every call that creates, changes or deletes Salesbuildr data (`salesbuildr_execute`, `import`) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `salesbuildr-cli doctor` to check credentials
- Verify the environment variable is set without printing it: `[ -n "$SALESBUILDR_API_KEY" ] && echo set` (PowerShell: `if ($env:SALESBUILDR_API_KEY) { 'set' }`)
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 No API key provided**  -  Set SALESBUILDR_API_KEY (Admin > Integrations > API Key) and re-run doctor.
- **404 or connection error on every call**  -  Set SALESBUILDR_TENANT to your tenant subdomain (acme for https://acme.salesbuildr.com), not the docs portal. If you must override SALESBUILDR_BASE_URL, include the /public-api suffix.
- **429 Too Many Requests**  -  The tenant limit is 500 requests per 10 minutes; honor the Retry-After header  -  sync paginates and backs off automatically.
- **Analytics commands return empty**  -  Run sync first  -  stale, thin, velocity, winrate and whitespace read the local store, not the live API.
