# QuoteWerks Web CLI

**Typed quote tools for QuoteWerks Web that use the real column names, preview typed writes before sending them, and add margin, win-rate and pipeline answers from a local mirror.**

The QuoteWerks Web REST API is generic table access, so agents guess column names and quietly build bad quotes. This CLI ships typed commands for finding documents, adding and repricing lines from your product databases, and re-quoting; each previews by default and writes only with --confirm. A local mirror adds margin, win-rate, pipeline-aging and cost-drift reports no single API call answers. Column names come from the vendor's documentation and the REST API is in beta.

Learn more at [QuoteWerks Web](https://www.quotewerks.com).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `quotewerks-cli` and `quotewerks-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.ps1 | iex
   ```
3. Verify: `quotewerks-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=quotewerks). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/quotewerks --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/quotewerks --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `quotewerks-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the quotewerks skill from https://github.com/Servosity/msp-skills/tree/main/skills/quotewerks. The skill defines how its required CLI (`quotewerks-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download `quotewerks-mcp.mcpb` from the [latest release](https://github.com/Servosity/msp-skills/releases?q=quotewerks). One bundle covers every supported platform.
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `QUOTEWERKS_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. If the bundle does not install on your machine, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/quotewerks/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "quotewerks": {
      "command": "quotewerks-mcp",
      "env": {
        "QUOTEWERKS_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Create an API key in QuoteWerks Web under Setup > Options > Zapier and API (the API requires MasterRights) and export it as QUOTEWERKS_API_KEY, or save it with 'echo "$QUOTEWERKS_API_KEY" | quotewerks-cli auth set-token' (the key is read from stdin). It is sent as the X-API-Key header. Read commands work with any key; creating or changing documents needs a key with 'Allow write access' (AllowWrite), otherwise QuoteWerks returns 403. QuoteWerks Web (cloud) tenants only; the REST API is in beta.

## Quick Start

```bash
# Check the install without touching the network (drop --dry-run to test the key and reachability)
quotewerks-cli doctor --dry-run

# Find a customer's quotes with typed filters instead of raw filter JSON
quotewerks-cli docs search --customer "Acme" --type QUOTE --agent

# Read one document and all of its lines
quotewerks-cli docs get 1042 --agent

# Copy recent documents, lines and product databases locally
quotewerks-cli mirror --since 180d

# List open quotes older than 30 days to chase
quotewerks-cli pipeline --older-than 30d

```

## Unique Features

These go beyond the raw REST API and the Zapier QuoteWerks app: typed commands with the real column names, previewed writes, and analysis of a local mirror.

### Typed quote tools
- **`docs search`**  -  Find quotes, orders and invoices by customer, number, status, rep, type or date without guessing QuoteWerks column names.

  _Reach for this before a raw 'tables query' on DocumentHeaders; it knows the real column names and catches silently ignored filters._

  ```bash
  quotewerks-cli docs search --customer "Acme" --type QUOTE --agent
  ```
- **`docs get`**  -  Get one quote, order or invoice plus every line item, with line types decoded and per-line margin.

  _Use this whenever you need the full contents of one document; it replaces a header fetch plus a hand-built item filter._

  ```bash
  quotewerks-cli docs get 1042 --agent
  ```
- **`items add`**  -  Add a line to a quote by part number, with price, cost, list and description filled in from your product databases (the lowest-cost match wins; an unmatched part is flagged, not priced).

  _Use this to add a product line safely; it previews by default and writes only with --confirm and a write-enabled API key._

  ```bash
  quotewerks-cli items add 1042 --part 20UES0X100 --qty 12 --json
  ```
- **`items reprice`**  -  See old versus new cost, price and margin for quote lines before changing them; lines with no known cost are skipped rather than priced at zero.

  _Use this when a distributor cost moved or a customer asks for a better price; nothing changes until --confirm (write-enabled API key)._

  ```bash
  quotewerks-cli items reprice 1042 --part 20UES0X100 --margin 18 --json
  ```
- **`docs create`**  -  Start a new quote from typed flags; it previews by default, and with --confirm (API key with write access) returns the new RecGUID and quote number.

  _Use this to start an empty quote; follow with 'items add' for lines._

  ```bash
  quotewerks-cli docs create --customer "Acme Dental" --name "Q4 laptop refresh" --json
  ```
- **`products search`**  -  Search parts across all of your QuoteWerks product databases at once.

  _Use this to find a part number, price or cost before quoting._

  ```bash
  quotewerks-cli products search "ThinkPad" --limit 20 --json
  ```
- **`requote`**  -  Copy an earlier quote or order into a new quote with today's product-database prices and new quantities.

  _Use this for 'same as last time' requests once you have the earlier document number (find it with docs search); it previews the new quote and writes only with --confirm._

  ```bash
  quotewerks-cli requote 1042 --qty 20UES0X100=12 --json
  ```

### Safe writes and audits
- **`docs lint`**  -  Catch price-below-cost, zero-cost, bad-quantity and low-margin lines on a quote before the customer sees it; add --strict to exit 1 when errors are found.

  _Run this on any quote an integration or agent built before it is sent._

  ```bash
  quotewerks-cli docs lint 1042 --min-margin 10 --json
  ```

### Pipeline and margin intelligence
- **`mirror`**  -  Copy quotes, line items and product databases into a local database; margin, winrate, pipeline, cost-drift and changes read this copy.

  _Run this before margin, winrate, pipeline, cost-drift or changes; they read the mirror._

  ```bash
  quotewerks-cli mirror --since 180d --json
  ```
- **`margin`**  -  See profit and margin percent by customer, rep, month or manufacturer from the local mirror (orders by default; run mirror first).

  _Use this for QBR and weekly sales-review margin questions._

  ```bash
  quotewerks-cli margin --by rep --since 90d --json
  ```
- **`pipeline`**  -  See open quote count and value by age bucket, or list the stale quotes to chase, from the local mirror (run mirror first).

  _Use this for the Monday pipeline review and follow-up list._

  ```bash
  quotewerks-cli pipeline --older-than 30d --json
  ```
- **`cost-drift`**  -  Find open-quote lines whose quoted cost no longer matches your product database, from the local mirror (run mirror first).

  _Run this after a distributor price update to see which quotes went thin._

  ```bash
  quotewerks-cli cost-drift --threshold 2 --json
  ```
- **`winrate`**  -  See won, lost and open quotes and the win rate by rep, customer or month from the local mirror; QuoteValet only records online acceptance, so pass --won-status for deals won offline.

  _Use this for sales-review and QBR win-rate questions._

  ```bash
  quotewerks-cli winrate --by rep --since 90d --json
  ```
- **`changes`**  -  See which documents were created, accepted, lost, re-statused, re-valued or removed between two mirror runs (the first mirror only records a baseline).

  _Use this for a 'what moved this week' pass over quotes._

  ```bash
  quotewerks-cli changes --since 7d --json
  ```

## Recipes

### Read one quote's lines compactly

```bash
quotewerks-cli docs get 1042 --agent --select header.DocNo,header.GrandTotal,lines.ManufacturerPartNumber,lines.UnitPrice
```

Narrows a large header and line payload to the fields an agent needs.

### Re-quote last order with new quantities

```bash
quotewerks-cli requote 1042 --qty 20UES0X100=12 --json
```

Previews a new quote copied from document 1042 with current product-database prices; add --confirm to create it.

### Reprice a line to an 18% margin

```bash
quotewerks-cli items reprice 1042 --part 20UES0X100 --margin 18 --json
```

Shows the before and after cost, price and margin; add --confirm to apply.

### Find quotes hurt by a distributor price change

```bash
quotewerks-cli cost-drift --threshold 2 --json
```

Lists open-quote lines whose quoted cost moved at least 2% from the product database (reads the local mirror; run mirror first).

### Audit an integration-built quote

```bash
quotewerks-cli docs lint 1042 --min-margin 10 --json
```

Flags price-below-cost, zero-cost and low-margin lines and exits nonzero on errors.

## Usage

Run `quotewerks-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `QUOTEWERKS_CONFIG_DIR`, `QUOTEWERKS_DATA_DIR`, `QUOTEWERKS_STATE_DIR`, or `QUOTEWERKS_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `QUOTEWERKS_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export QUOTEWERKS_HOME=/srv/quotewerks
quotewerks-cli doctor
```

Under `QUOTEWERKS_HOME=/srv/quotewerks`, the four dirs resolve to `/srv/quotewerks/config`, `/srv/quotewerks/data`, `/srv/quotewerks/state`, and `/srv/quotewerks/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "quotewerks": {
      "command": "quotewerks-mcp",
      "env": {
        "QUOTEWERKS_HOME": "/srv/quotewerks"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `QUOTEWERKS_DATA_DIR` overrides an explicit `--home` for that kind. Use `QUOTEWERKS_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `QUOTEWERKS_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `quotewerks-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### agent

Manage agent

- **`quotewerks-cli agent ask`** - Ask the QuoteWerks Web AI agent a natural-language question about your data
- **`quotewerks-cli agent report-draft`** - One turn of report drafting, previewed as `run_as` with that user's document rights.
- **`quotewerks-cli agent report-eligibility`** - Which of the tenant's reports the assistant can open -- the exact check /load makes, run over
all of them, so the Ask AI tab never offers a report it would then refuse. No model call and no
preview; not user-specific (who may SEE a report's rows is decided when it runs).
- **`quotewerks-cli agent report-load`** - An existing report as the starting draft of a conversation -- previewed, no model call. Later
turns pass `base_report_rec_guid` so their drafts merge into this report's stored settings.
- **`quotewerks-cli agent report-preview`** - Re-run a draft -- typically with new prompt answers from the form. No model call.

### cloud_settings

Manage cloud settings

- **`quotewerks-cli cloud-settings get`** - Get a single resource by ID.
- **`quotewerks-cli cloud-settings list`** - List resources with pagination, filtering, and sorting.
- **`quotewerks-cli cloud-settings schema`** - Get the schema (columns) for this resource.

### identity

Manage identity

- **`quotewerks-cli identity health`** - Health check endpoint for the authentication service.

Returns: 200 OK if the service is running
- **`quotewerks-cli identity userinfo`** - Show the tenant account number and user name for the credential in use (works with the X-API-Key header).

### reports

Manage reports

- **`quotewerks-cli reports run`** - Executes a saved report and returns its rendered content.

`run_as` defaults to the authenticated user, so an ordinary caller gets exactly the rows they
would get running the report in the UI.
- **`quotewerks-cli reports schedulability`** - Reports whether a definition can run without a person present, and why not if it cannot.

Exists so the QuoteWerksWeb schedule dialog can refuse *before* a schedule is saved, naming
the specific inputs at fault, rather than failing at 3am with nobody watching. Also returns
the prompt inventory, which is what a later per-schedule override UI would bind to.

### tables

Manage tables

- **`quotewerks-cli tables delete`** - Delete a row by primary key. Requires API key with AllowWrite permission.
- **`quotewerks-cli tables insert`** - Create a new row in a table. Requires API key with AllowWrite permission.

Request body must follow JSON:API format:
```json
{"data": {"type": "TableName", "attributes": {"Field1": "value", ...}}}
```
- **`quotewerks-cli tables list`** - List all available tables in the tenant database.
- **`quotewerks-cli tables query`** - Search a table with the filter document in the request body.

Identical semantics to `GET /{table_name}`  -  same system filters, same
field restrictions, same `meta.ignored_filters` contract  -  for callers
whose filter does not fit in a URL. Paginate by re-posting with
`page.number + 1`; `links` is omitted because a GET-shaped URL cannot
carry this body.
- **`quotewerks-cli tables row`** - Get a single row by primary key. Returns JSON:API format with optional relationship sideloading.
- **`quotewerks-cli tables rows`** - List rows from a table. Returns JSON:API format with pagination links and optional relationship sideloading.
- **`quotewerks-cli tables schema`** - Get the schema (columns) for a specific table, including write metadata.
- **`quotewerks-cli tables update`** - Update a row by primary key. Requires API key with AllowWrite permission.

Request body must follow JSON:API format:
```json
{"data": {"type": "TableName", "id": "123", "attributes": {"Field1": "new value"}}}
```
- **`quotewerks-cli tables values`** - List the distinct values of one column in a table


### changes


- **`quotewerks-cli changes`** - See which documents were created, accepted, lost, re-statused, re-valued or removed between two mirror runs (the first mirror only records a baseline).


### cost-drift


- **`quotewerks-cli cost-drift`** - Find open-quote lines whose quoted cost no longer matches your product database, from the local mirror (run mirror first).


### docs


- **`quotewerks-cli docs search`** - Find quotes, orders and invoices by customer, number, status, rep, type or date without guessing QuoteWerks column names.

- **`quotewerks-cli docs get`** - Get one quote, order or invoice plus every line item, with line types decoded and per-line margin.

- **`quotewerks-cli docs create`** - Start a new quote from typed flags; it previews by default, and with --confirm (API key with write access) returns the new RecGUID and quote number.

- **`quotewerks-cli docs lint`** - Catch price-below-cost, zero-cost, bad-quantity and low-margin lines on a quote before the customer sees it; add --strict to exit 1 when errors are found.


### items


- **`quotewerks-cli items list`** - Find quote lines by document, part number or description
- **`quotewerks-cli items add`** - Add a line to a quote by part number, with price, cost, list and description filled in from your product databases (the lowest-cost match wins; an unmatched part is flagged, not priced).

- **`quotewerks-cli items reprice`** - See old versus new cost, price and margin for quote lines before changing them; lines with no known cost are skipped rather than priced at zero.


### margin


- **`quotewerks-cli margin`** - See profit and margin percent by customer, rep, month or manufacturer from the local mirror (orders by default; run mirror first).


### mirror


- **`quotewerks-cli mirror`** - Copy quotes, line items and product databases into a local database; margin, winrate, pipeline, cost-drift and changes read this copy.


### pipeline


- **`quotewerks-cli pipeline`** - See open quote count and value by age bucket, or list the stale quotes to chase, from the local mirror (run mirror first).


### products


- **`quotewerks-cli products search`** - Search parts across all of your QuoteWerks product databases at once.


### requote


- **`quotewerks-cli requote`** - Copy an earlier quote or order into a new quote with today's product-database prices and new quantities.


### winrate


- **`quotewerks-cli winrate`** - See won, lost and open quotes and the win rate by rep, customer or month from the local mirror; QuoteValet only records online acceptance, so pass --won-status for deals won offline.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`quotewerks-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`quotewerks-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`quotewerks-cli learnings list`** - Inspect taught rows
- **`quotewerks-cli learnings forget <query>`** - Undo a teach
- **`quotewerks-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`quotewerks-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`quotewerks-cli teach-pattern`** - Install a query/resource template up front
- **`quotewerks-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `QUOTEWERKS_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `quotewerks-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
quotewerks-cli cloud-settings list

# JSON for scripting and agents
quotewerks-cli cloud-settings list --json
# Filter to specific fields
quotewerks-cli cloud-settings list --json --select id

# Dry run  -  show the request without sending
quotewerks-cli cloud-settings list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
quotewerks-cli cloud-settings list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the HTTP request without sending on generated endpoint commands (`tables`, `cloud-settings`, `agent`, `reports`); on the typed commands it only confirms the command would run, and the typed write commands already preview by default
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - typed writes (`docs create`, `items add`, `items reprice`, `requote`) preview by default and write only with `--confirm` (needs an API key with write access); `--yes` does not confirm them. The raw `tables insert`, `tables update`, `tables delete` and `import` commands write immediately, so run them with `--dry-run` first
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
quotewerks-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `quotewerks-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/quotewerks-cli/config.toml`; `--home`, `QUOTEWERKS_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `QUOTEWERKS_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `quotewerks-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `quotewerks-cli doctor` to check credentials
- Verify the environment variable is set: `echo $QUOTEWERKS_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run `quotewerks-cli docs search --number <DocNo>` or `quotewerks-cli tables list` to see what exists

### API-specific
- **401 Invalid or missing authentication**  -  export QUOTEWERKS_API_KEY=<key from Setup > Options > Zapier and API>, then run quotewerks-cli doctor
- **403 on docs create, items add or items reprice --confirm**  -  Create a QuoteWerks Web API key with 'Allow write access' enabled and use it for writes
- **422 when creating a quote with --rep**  -  Pass a SalesRep that is an existing QuoteWerks Web user, or omit --rep
- **margin, winrate, pipeline, cost-drift or changes return nothing**  -  Run quotewerks-cli mirror first; these commands read the local mirror, and changes needs two mirror runs
- **error says the server ignored a filter**  -  Check the column name with quotewerks-cli tables schema DocumentHeaders
