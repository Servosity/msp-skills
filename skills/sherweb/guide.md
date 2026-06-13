# Sherweb CLI

**Every Sherweb Partner API capability, plus a local SQLite store, offline analytics, and margin/drift/orphan joins no other Sherweb tool has.**

sherweb-cli wraps the full documented Sherweb Distributor and Service Provider surface  -  customers, subscriptions, payable and receivable charges, platforms, catalog, orders  -  then adds what the portal and every existing wrapper lack: an offline SQLite copy, offline analytics, agent-native --json/--select, --dry-run on every mutation, and cross-entity reporting like margin per customer (receivable minus payable), bill drift across snapshots, and orphaned-subscription detection.

## Install

The recommended path installs both the `sherweb-cli` binary and the `pp-sherweb` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install sherweb
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install sherweb --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install sherweb --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install sherweb --agent claude-code
npx -y @mvanhorn/printing-press-library install sherweb --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/commerce/sherweb/cmd/sherweb-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/sherweb-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install sherweb --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-sherweb --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-sherweb --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install sherweb --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/sherweb-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `SHERWEB_CLIENT_ID` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/commerce/sherweb/cmd/sherweb-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "sherweb": {
      "command": "sherweb-mcp",
      "env": {
        "SHERWEB_CLIENT_ID": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Sherweb uses composed auth: an OAuth2 client-credentials bearer token (minted from https://api.sherweb.com/auth/oidc/connect/token with a per-API scope of distributor and/or service-provider) PLUS an Azure APIM gateway key sent as the Ocp-Apim-Subscription-Key header on every call. Set SHERWEB_CLIENT_ID, SHERWEB_CLIENT_SECRET, and SHERWEB_SUBSCRIPTION_KEY (all from cumulus.sherweb.com Security > APIs). The client mints and caches the token; the subscription key rides on every request including the token request.

## Quick Start

```bash
# Confirm the three credentials resolve and the token mints before anything else.
sherweb-cli doctor

# Pull customers, subscriptions, and payable/receivable charges into the local SQLite store.
sherweb-cli sync

# The headline join: net margin per customer for a billing month.
sherweb-cli margin --month 2026-04

# Find active subscriptions that no customer is being billed for.
sherweb-cli orphans

# Roll up Microsoft 365 seats across every customer from the local store.
sherweb-cli fleet-subs --product "Microsoft 365"

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`margin`**  -  See net margin per customer (what they owe you minus what you owe Sherweb) for any billing month.

  _Reach for this at monthly close to find margin leaks instead of rebuilding a payable-vs-receivable spreadsheet._

  ```bash
  sherweb-cli margin --month 2026-04 --agent --select customerName,payable,receivable,margin
  ```
- **`drift`**  -  Diff the latest payable-charges snapshot against the prior sync to flag charges that appeared, vanished, or changed.

  _Use it after each sync to catch surprise charges before the invoice lands._

  ```bash
  sherweb-cli drift --since 2026-03-01 --agent
  ```
- **`orphans`**  -  List active subscriptions with zero receivable charges in the latest period  -  zombie subs you're paying for but not billing on.

  _Run it on the weekly inventory sweep to stop paying Sherweb for subscriptions no customer is funding._

  ```bash
  sherweb-cli orphans --agent
  ```
- **`margin-trend`**  -  See each customer's margin across the last N monthly closes so profitability slides surface before an account goes negative.

  _Reach for this when the question is whether an account is getting less profitable over time, not a single month's number._

  ```bash
  sherweb-cli margin-trend --customer 0aa00000-0000-0000-0000-000000000000 --last 6 --agent
  ```
- **`usage-leak`**  -  Flag metered platform usage that has no matching receivable charge - consumption you absorb instead of billing.

  _Reach for this during billing reconciliation to catch metered consumption silently eating margin._

  ```bash
  sherweb-cli usage-leak --customer 0aa00000-0000-0000-0000-000000000000 --agent
  ```

### Fleet visibility
- **`fleet-subs`**  -  One table across ALL customers' subscriptions grouped by product/SKU, with total seats and customer counts.

  _Use it to answer 'how many seats of X across the whole book' without paging customer-by-customer._

  ```bash
  sherweb-cli fleet-subs --product "Microsoft 365" --agent
  ```
- **`right-size`**  -  Flag subscriptions where seats paid differ from metered seats used, per customer.

  _Reach for it before renewals to trim over-provisioned seats and catch under-provisioning._

  ```bash
  sherweb-cli right-size --customer 00000000-0000-0000-0000-000000000000 --agent
  ```
- **`sub-changes`**  -  Flag subscriptions added, cancelled, or quantity-changed between two syncs across the whole customer book.

  _Reach for this to audit what changed across all customers since the last sync instead of paging customer by customer._

  ```bash
  sherweb-cli sub-changes --since 30d --agent
  ```

### Mutation safety
- **`amend-preview`**  -  Preview the monthly cost change of a seat amendment before committing it, computed from stored pricing.

  _Use it on every seat-change ticket so you see the dollar impact before the amendment fires._

  ```bash
  sherweb-cli amend-preview --customer 00000000-0000-0000-0000-000000000000 --sub SUB123 --qty 25 --agent
  ```

## Recipes


### Monthly margin close

```bash
sherweb-cli sync && sherweb-cli deep-sync && sherweb-cli margin --month 2026-04 --agent --select customerName,payable,receivable,margin
```

Sync customers + payable charges, fan out per-customer receivables with deep-sync, then emit per-customer margin as structured rows ready to pipe into a close workbook.

### Catch surprise charges

```bash
sherweb-cli drift --since 2026-03-01 --agent
```

Diff this period's payable charges against the prior snapshot to flag anything new or changed.

### Zombie subscription sweep

```bash
sherweb-cli orphans --agent
```

List active subscriptions with no receivable charges so you stop paying Sherweb for unbilled seats.

### Right-size before renewal

```bash
sherweb-cli right-size --customer 00000000-0000-0000-0000-000000000000 --agent
```

Compare seats paid against metered usage to trim over-provisioned subscriptions.

### Preview a seat change

```bash
sherweb-cli amend-preview --customer 00000000-0000-0000-0000-000000000000 --sub SUB123 --qty 25
```

See the monthly cost delta of an amendment before committing it with the real amend command.

## Usage

Run `sherweb-cli --help` for the full command reference and flag list.

## Commands

### distributor

Manage distributor

- **`sherweb-cli distributor`** - Returns the charges you are billed by Sherweb for a billing period: products, quantities, list/net pricing, fees, taxes, and deductions.

### service-provider

Manage service provider

- **`sherweb-cli service-provider amend-subscriptions`** - Submit one or more subscription quantity changes. Returns a tracking id.
- **`sherweb-cli service-provider cancel-subscriptions`** - Cancel one or more of a customer's subscriptions
- **`sherweb-cli service-provider get-customer-catalog`** - Get the product catalog available to a specific customer
- **`sherweb-cli service-provider get-customer-catalog-items-pricing`** - Get pricing for specific catalog items for a customer
- **`sherweb-cli service-provider get-customer-platform-details`** - Get details for one provisioned platform of a customer
- **`sherweb-cli service-provider get-customer-platform-meter-usages`** - Get metered usage for a customer's platform
- **`sherweb-cli service-provider get-customer-platforms-configurations`** - Get the platform configurations provisioned for a customer
- **`sherweb-cli service-provider get-platform-required-parameters`** - Get the required provisioning parameters for given SKUs/platform
- **`sherweb-cli service-provider get-platforms-for-skus`** - Resolve which platforms can fulfill given SKUs
- **`sherweb-cli service-provider get-receivable-charges`** - Get receivable charges (what a customer owes you) for a date
- **`sherweb-cli service-provider get-subscription-meters`** - Get metered subscriptions for a customer
- **`sherweb-cli service-provider get-subscriptions-pricing`** - Get pricing information for a customer's subscriptions
- **`sherweb-cli service-provider get-tracking-status`** - Get the status of an order or amendment by tracking id
- **`sherweb-cli service-provider list-customers`** - List the customers under your reseller account
- **`sherweb-cli service-provider list-platforms`** - List the provisioning platforms available to you
- **`sherweb-cli service-provider list-subscriptions`** - List a customer's subscriptions with details
- **`sherweb-cli service-provider place-order`** - Place a marketplace order for a customer
- **`sherweb-cli service-provider validate-order`** - Validate an order cart for a customer before placing it


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
sherweb-cli distributor

# JSON for scripting and agents
sherweb-cli distributor --json

# Filter to specific fields
sherweb-cli distributor --json --select id,name,status

# Dry run  -  show the request without sending
sherweb-cli distributor --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
sherweb-cli distributor --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
sherweb-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/sherweb-partner-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SHERWEB_CLIENT_ID` | per_call | Yes | Set to your API credential. |
| `SHERWEB_CLIENT_SECRET` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `sherweb-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `sherweb-cli doctor` to check credentials
- Verify the environment variable is set: `echo $SHERWEB_CLIENT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Check SHERWEB_SUBSCRIPTION_KEY  -  Sherweb rejects the token request itself without the Ocp-Apim-Subscription-Key header; run sherweb-cli doctor.
- **403 Forbidden**  -  Your client credentials lack the required scope; ensure the API credential in cumulus.sherweb.com grants both distributor and service-provider.
- **429 Rate limit exceeded**  -  Sherweb is throttling; the client backs off and retries, but reduce sync frequency or narrow --customer scope.
- **margin shows nothing for a customer**  -  Run sherweb-cli sync first  -  margin reads the local store, not the live API, so it needs a recent sync of payable and receivable charges.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**djust270/Sherweb-API**](https://github.com/djust270/Sherweb-API)  -  PowerShell
- [**wyre-technology/sherweb-mcp**](https://github.com/wyre-technology/sherweb-mcp)  -  TypeScript
- [**am3cramirez/sherweb-mcp**](https://github.com/am3cramirez/sherweb-mcp)  -  TypeScript
- [**sherweb/Public-Apis**](https://github.com/sherweb/Public-Apis)  -  C#
- [**KelvinTegelaar/CIPP-API**](https://github.com/KelvinTegelaar/CIPP-API)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
