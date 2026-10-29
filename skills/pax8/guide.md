# Pax8 CLI

**Every Pax8 Partner API endpoint, plus an offline store that reconciles billing, tracks MRR, and catches usage overages no Pax8 tool surfaces.**

pax8-cli mirrors the full Pax8 Partner API as typed, agent-native commands, then extends it with a local SQLite store. Reconcile invoices against subscriptions, compute MRR and margin, catch metered overages before they bill, and pull a true customer-360  -  the reports the portal makes you build by hand.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `pax8-cli` and `pax8-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.ps1 | iex
   ```
3. Verify: `pax8-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=pax8). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/pax8 --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/pax8 --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `pax8-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the pax8 skill from https://github.com/Servosity/msp-skills/tree/main/skills/pax8. The skill defines how its required CLI (`pax8-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=pax8).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `PAX8_CLIENT_ID` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/pax8/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "pax8": {
      "command": "pax8-mcp",
      "env": {
        "PAX8_CLIENT_ID": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`pax8-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `PAX8_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `PAX8_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export PAX8_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
pax8-mcp --transport http
```

Set `PAX8_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Pax8 uses OAuth2 client-credentials. Set PAX8_CLIENT_ID and PAX8_CLIENT_SECRET (create an API key in the Pax8 portal under Integrations). The CLI exchanges them at https://token-manager.pax8.com/oauth/token with the required audience (override with PAX8_AUDIENCE, default api://p8p.client) and caches the bearer token until it expires. Run `pax8-cli doctor` to verify the token exchange; `pax8-cli auth login` mints and stores a token.

## Quick Start

```bash
# confirm OAuth client-credentials exchange works before anything else
pax8-cli doctor

# pull companies, subscriptions, invoices, and usage into the local store
pax8-cli sync

# list the customers you serve through Pax8
pax8-cli companies list --json

# flag billing mismatches between invoices and subscriptions
pax8-cli reconcile

# see monthly recurring revenue and margin from the synced store
pax8-cli mrr

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`reconcile`**  -  Flag invoice lines that no longer match an active subscription, and active subscriptions that never got billed; --draft pre-checks the next unposted invoice.

  _Reach for this when an MSP needs to catch billing leakage the Pax8 portal can't surface in one place._

  ```bash
  pax8-cli reconcile --agent
  ```
- **`mrr`**  -  Compute monthly recurring revenue and margin from subscriptions and product pricing, trended across syncs, with a per-product breakdown.

  _Reach for this when an agent needs the revenue number the portal makes you build in a spreadsheet._

  ```bash
  pax8-cli mrr --agent --select totals.mrr,totals.margin
  ```
- **`overage`**  -  Aggregate usage-lines per subscription and surface overages before they land on the customer invoice.

  _Reach for this to catch metered overages proactively instead of after the invoice posts._

  ```bash
  pax8-cli overage --agent
  ```
- **`since`**  -  Show subscription changes in a window: synced history events plus subscriptions created or updated in it.

  _Reach for this to answer 'what changed in my book of business' without diffing exports by hand._

  ```bash
  pax8-cli since 7d --agent
  ```
- **`company show`**  -  One view of a company with its subscriptions, contacts, invoices, and usage.

  _Reach for this when an agent needs a full picture of one customer in a single call._

  ```bash
  pax8-cli company show <companyId> --agent --select subscriptions.productName,subscriptions.quantity
  ```
- **`spend`**  -  Roll up invoice-items by company to show what each customer is costing across invoices.

  _Reach for this to rank customers by spend without exporting every invoice._

  ```bash
  pax8-cli spend --agent
  ```

## Recipes

### Billing reconciliation for the current period

```bash
pax8-cli reconcile --agent
```

Surfaces invoice lines with no active subscription and active subscriptions that were never billed.

### MRR and margin, narrowed for an agent

```bash
pax8-cli mrr --agent --select totals.mrr,totals.margin,byProduct.productName
```

Returns just the revenue numbers an agent needs from a wide aggregate, using dotted select paths.

### Customer 360 in one call

```bash
pax8-cli company show <companyId> --agent --select subscriptions.productName,subscriptions.quantity
```

Joins a company to its subscriptions and invoices locally and narrows the deeply nested result.

### What changed this week

```bash
pax8-cli since 7d --agent
```

Reads synced subscription history plus subscriptions created or updated in the window: new, cancelled, and quantity-changed.

### Find a product by vendor

```bash
pax8-cli products find-all --vendor-name Microsoft --json
```

Lists the products one vendor sells through Pax8, straight from the Partner API.

## Usage

Run `pax8-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `PAX8_CONFIG_DIR`, `PAX8_DATA_DIR`, `PAX8_STATE_DIR`, or `PAX8_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `PAX8_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export PAX8_HOME=/srv/pax8
pax8-cli doctor
```

On Windows (PowerShell):

```powershell
$env:PAX8_HOME = "C:\ProgramData\pax8"
pax8-cli doctor
```

Under `PAX8_HOME=/srv/pax8`, the four dirs resolve to `/srv/pax8/config`, `/srv/pax8/data`, `/srv/pax8/state`, and `/srv/pax8/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:PAX8_HOME = "C:\srv\pax8"
pax8-cli doctor
```

Under `$env:PAX8_HOME = "C:\srv\pax8"`, the four dirs resolve to `C:\srv\pax8\config`, `C:\srv\pax8\data`, `C:\srv\pax8\state`, and `C:\srv\pax8\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "pax8": {
      "command": "pax8-mcp",
      "env": {
        "PAX8_HOME": "/srv/pax8"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `PAX8_DATA_DIR` overrides an explicit `--home` for that kind. Use `PAX8_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `PAX8_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `pax8-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `pax8-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `pax8-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `PAX8_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `PAX8_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### companies

Companies represent the customers your organization serves through the Pax8 platform. Fetch a list of your company records, or a specific company by its companyId

- **`pax8-cli companies create-company`** - Creates a new company. You can optionally include contacts in the request body to create the company with contacts in a single request. If no contacts are provided, the company will be placed in an "inactive" status until the [company has primary contacts added](#tag/Contacts). Once contacts are added, the company will move to "Active".
- **`pax8-cli companies find`** - Returns a paginated list of all your companies filtered by optional parameters
- **`pax8-cli companies get`** - Returns a single company record matching the ```companyId``` you specify
- **`pax8-cli companies update-company`** - Updates an existing Company. ATTENTION - at least one parameter has to be modified.

### invoices

Manage invoices

- **`pax8-cli invoices find-partner`** - Fetch a paginated list of invoices. Default page is 0 and default size is 10. The maximum page size is 200
- **`pax8-cli invoices find-partner-draft-items`** - Fetch a paginated list of draft invoice items before they are finalized into invoices. Default page is 0 and default size is 10. The maximum page size is 200.

DISCLAIMER: For future-dated items, the data represents a snapshot of charges that will be applied to a future invoice as of the request date. This is not an actual invoice. Clients should be aware that the final invoice generated on the 5th of the month may differ. This endpoint does not include charges for usage-based products billed in arrears. Tax calculations are based on current account taxability status and current month tax rates, both of which are subject to change for future invoices.
- **`pax8-cli invoices get-partner`** - Fetch a paginated list of invoices. Default page is 0 and default size is 10. The maximum page size is 200

### orders

Orders describe your purchases of Pax8 products. Endpoints let you create new product orders and query past purchases

- **`pax8-cli orders create`** - Create a new order. Currently NOT supported for scheduled orders(orders with a future date).
- **`pax8-cli orders find`** - Returns a paginated list of orders. Currently NOT supported for scheduled orders(orders with a future date).
- **`pax8-cli orders find-by-id`** - Returns the Order record specified by OrderId. Currently NOT supported for scheduled orders

### products

The Products resource lets you fetch all the information required to complete an Order -- product descriptions, pricing, provisioning details, and the dependencies associated with each product

- **`pax8-cli products find-all`** - Returns a paginated list of Pax8 products filtered by optional query parameters
- **`pax8-cli products find-by-id`** - Returns only the product record for the productId you specify

### subscriptions

Subscriptions describe terms of service, including price and the billing start/end dates for a specified product/company combination. Resources let you update, cancel, and fetch details of existing subscriptions

- **`pax8-cli subscriptions delete`** - Cancels the Subscription specified by subscriptionId
- **`pax8-cli subscriptions find`** - Fetch a paginated list of subscriptions. Default page is 0 and default size is 10. The maximum page size is 200
- **`pax8-cli subscriptions find-by-id`** - Returns the Subscription record specified by the subscriptionId
- **`pax8-cli subscriptions update`** - Updates a subscription. Currently NOT supported for subscriptions with a future date. At least one of the following fields are required: Price, BillingTerm, Quantity, StartDate

### usage-summaries

Manage usage summaries

- **`pax8-cli usage-summaries <usageSummaryId>`** - Fetch a paginated list of usage summaries. Default page is 0 and default size is 10. The maximum page size is 200


### company


- **`pax8-cli company show`** - One view of a company with its subscriptions, contacts, invoices, and usage.


### mrr


- **`pax8-cli mrr`** - Compute monthly recurring revenue and margin from subscriptions and product pricing, trended across syncs, with a per-product breakdown.


### overage


- **`pax8-cli overage`** - Aggregate usage-lines per subscription and surface overages before they land on the customer invoice.


### reconcile


- **`pax8-cli reconcile`** - Flag invoice lines that no longer match an active subscription, and active subscriptions that never got billed; --draft pre-checks the next unposted invoice.


### since


- **`pax8-cli since`** - Show subscription changes in a window: synced history events plus subscriptions created or updated in it.


### spend


- **`pax8-cli spend`** - Roll up invoice-items by company to show what each customer is costing across invoices.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`pax8-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`pax8-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`pax8-cli learnings list`** - Inspect taught rows
- **`pax8-cli learnings forget <query>`** - Undo a teach
- **`pax8-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`pax8-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`pax8-cli teach-pattern`** - Install a query/resource template up front
- **`pax8-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `PAX8_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `pax8-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
pax8-cli companies get mock-value

# JSON for scripting and agents
pax8-cli companies get mock-value --json
# Filter to specific fields
pax8-cli companies get mock-value --json --select address,billOnBehalfOfEnabled,contacts

# Dry run  -  show the request without sending
pax8-cli companies get mock-value --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
pax8-cli companies get mock-value --agent
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
pax8-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `pax8-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/pax8-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\pax8-cli\config.toml` on Windows; `--home`, `PAX8_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `PAX8_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `PAX8_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `PAX8_CLIENT_ID` | per_call | Yes | Set to your API credential. |
| `PAX8_CLIENT_SECRET` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `pax8-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- The client ID and secret go only to the token URL (`PAX8_TOKEN_URL`, default `https://token-manager.pax8.com/oauth/token`), and that host receives nothing but the token request. The bearer token and every API request go only to the scheme, host and port of `PAX8_BASE_URL` (default `https://api.pax8.com/v1`). A redirect to any other host is refused, so a credential never reaches it.
- Any output, file or MCP result that would repeat the client ID, the client secret, a minted token or a credential-named value from a response prints "[text withheld: it repeated a secret value this program holds]" in its place. Responses are never cached on disk, and credential-named fields are dropped before anything reaches the local store.
- Only `credentials.toml` (mode 0600) holds a credential. Set `PAX8_NO_CONFIG_WRITE=1` to keep even the minted token off disk.
- `--dry-run` sends nothing and mints no token. Over MCP, a tool that creates, updates, orders or cancels returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

### Keeping credentials off disk (optional)

By default the connector caches the token it mints in the config file so the next
command does not have to mint another one. Set `PAX8_NO_CONFIG_WRITE=1` to turn that
off: nothing is written, and each run mints a fresh token instead. Use it when
your secrets live in Keychain, Windows Credential Manager, or any launcher that
injects them as environment variables at process start, and you do not want a
plaintext copy on disk.

| What changes | With `PAX8_NO_CONFIG_WRITE=1` |
| --- | --- |
| Token cache | Not written. A fresh token is minted per invocation. |
| `auth login` / `auth set-token` | Refuse, naming the variable, instead of reporting a save that did not happen. |
| `auth logout` | Still clears an existing config file. The erase is not a credential write. |
| MCP server | Honours the same variable, so a Claude Desktop install gets no plaintext token cache either. |

Set it in the shell for CLI use, or through the install prompt of the same name
for the MCP server. Any value other than blank, `0`, `false`, `no` or `off`
turns it on. See issue #270.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `pax8-cli doctor` to check credentials
- Verify the environment variable is set: `echo $PAX8_CLIENT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Confirm PAX8_CLIENT_ID and PAX8_CLIENT_SECRET are set and run `pax8-cli doctor`; the token exchange needs the correct audience (PAX8_AUDIENCE).
- **doctor reports token exchange failed**  -  Re-create the API credential in the Pax8 portal (Integrations) and ensure the secret was copied exactly; secrets are shown only once.
- **reconcile or mrr returns empty**  -  Run `pax8-cli sync` first; analytics commands read the local store, not the live API.
- **list commands paginate slowly**  -  Use `pax8-cli sync` once, then query the offline store with `search`, `analytics`, and `--csv`.
