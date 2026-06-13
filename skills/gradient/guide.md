# Gradient MSP CLI

**The Synthesize partner API from the terminal - every endpoint, plus a usage-push ledger, billing drift detection, and alert-to-ticket tracing no other Gradient tool has.**

Gradient MSP's Synthesize platform reconciles vendor usage against MSP billing, but its only tooling is a PowerShell SDK that makes you write a script project per integration. This CLI covers the full vendor API surface agent-natively with an offline SQLite mirror of your accounts, and adds what the API itself cannot: a local ledger of every count you push (usage drift), debounce-aware bulk pushes (usage push), mapping-hygiene rollups (hygiene unmapped), and async alert-to-ticket confirmation (alert send --wait).

## Install

The recommended path installs both the `gradient-cli` binary and the `pp-gradient` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install gradient
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install gradient --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install gradient --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install gradient --agent claude-code
npx -y @mvanhorn/printing-press-library install gradient --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/payments/gradient/cmd/gradient-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/gradient-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install gradient --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-gradient --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-gradient --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install gradient --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/gradient-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `GRADIENT_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/payments/gradient/cmd/gradient-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "gradient": {
      "command": "gradient-mcp",
      "env": {
        "GRADIENT_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Synthesize issues a Vendor API key and a Partner API key pair (Synthesize UI: Integrations -> Custom -> Generate API Tokens; shown once). Every request sends a GRADIENT-TOKEN header whose value is base64("<vendorKey>:<partnerKey>"). Set GRADIENT_VENDOR_API_KEY and GRADIENT_PARTNER_API_KEY and the CLI derives the header for you, or set GRADIENT_TOKEN to the pre-computed value directly. Verify with `gradient-cli doctor`.

## Quick Start

```bash
# Health check: confirms config, env vars, and connectivity plan without calling the API
gradient-cli doctor --dry-run

# Verify credentials - the vendor-recommended auth check returns your integration identity and status
gradient-cli integration get --agent

# Mirror accounts into the local SQLite store for offline analytics and stale-data checks
gradient-cli sync --resources accounts

# See which accounts and services still need mapping before billing can reconcile
gradient-cli hygiene unmapped --agent

# Validate a counts file shape before sending anything
gradient-cli usage push --file ./counts.csv --dry-run

# After a real push: see exactly which accounts' usage changed since the previous run
gradient-cli usage drift --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Billing you can audit
- **`usage push`**  -  Push a whole file of unit counts (CSV or JSON) in one shot and trigger exactly one billing rebuild instead of one per call.

  _Reach for this whenever more than one unit count needs to land in Synthesize; it is the nightly sync primitive._

  ```bash
  gradient-cli usage push --file ./counts.csv --agent
  ```
- **`usage drift`**  -  See exactly which accounts' usage changed between your last two pushes, with old-to-new deltas.

  _Run before every invoice cycle to know what moved - the reconciliation pre-flight no API call can answer._

  ```bash
  gradient-cli usage drift --agent
  ```

### Alert-to-ticket correlation
- **`alert send`**  -  Dispatch an alert and wait until the PSA ticket actually exists, instead of fire-and-forget.

  _Use when an alert must verifiably become a ticket - CI hooks and monitoring bridges need the confirmation, not the enqueue._

  ```bash
  gradient-cli alert send --account 123456789 --title "Backup failure" --description "Nightly job failed" --wait --agent
  ```
- **`alert trace`**  -  Review every alert you have dispatched - messageId, account, time, last-known ticket state - and filter to ones that never became tickets.

  _The fastest answer to 'which of my alerts are stuck in the queue' across days of dispatches._

  ```bash
  gradient-cli alert trace --stuck --agent
  ```

### Mapping hygiene
- **`hygiene unmapped`**  -  One rollup of every unmapped account and every service without a vendor SKU, shaped as an actionable work queue.

  _Run it after every account/service push to see what mapping work remains before billing can reconcile._

  ```bash
  gradient-cli hygiene unmapped --agent
  ```
- **`status ready`**  -  A single go/no-go verdict on whether the integration is ready to flip to active: live status, last sync age, and outstanding mapping gaps.

  _Run before 'integration update-status --status active' to avoid activating an integration with unmapped accounts._

  ```bash
  gradient-cli status ready --agent
  ```

## Recipes


### Nightly usage sync with one billing rebuild

```bash
gradient-cli usage push --file ./nightly-counts.csv --agent
```

Validates and fans out every account-service count with no_build until the final call, then records the run in the local ledger.

### What changed since the last push?

```bash
gradient-cli usage drift --agent
```

Joins the two most recent ledger runs and returns only the rows whose counts moved - the pre-invoice sanity check.

### Inspect the vendor SKU catalog fields agents care about

```bash
gradient-cli vendor get --agent --select data.name,data.skus.name,data.skus.category
```

The vendor profile nests the full SKU catalog; --select narrows the payload to the fields that matter.

### Dispatch an alert and confirm the ticket

```bash
gradient-cli alert send --account 123456789 --title "Backup failure" --description "Nightly job failed twice" --wait
```

Dispatches, captures the messageId, then polls the debug route until the PSA ticket exists or timeout.

### Pre-activation readiness check

```bash
gradient-cli status ready --agent
```

Combines live integration status with local mapping gaps into a single go/no-go before 'integration update-status --status active'.

## Usage

Run `gradient-cli --help` for the full command reference and flag list.

## Commands

### accounts

Manage accounts

- **`gradient-cli accounts create`** - Creates an account
- **`gradient-cli accounts list`** - Get Organization Accounts
- **`gradient-cli accounts update`** - Updates 1 or more fields per Account
- **`gradient-cli accounts update-one <accountId>`** - Updates 1 or more fields of a single Account

### alerting

Manage alerting

- **`gradient-cli alerting <accountId>`** - Adds an alert to the alerting queue

### billing

Manage billing

- **`gradient-cli billing <serviceId>`** - Set route for adding new unit count for one account, and one service. Subsequent calls will debounce until calls are complete. After which billing views will update.

### clients

Manage clients

- **`gradient-cli clients`** - DEPRECATED - This route is used to get the Client data needed to handling mapping externally.

### integration

Manage integration

- **`gradient-cli integration get`** - Preferred route for checking credentials
- **`gradient-cli integration update-status`** - Update the integration status for a partner. Set as pending when ready for inital mapping. Set as active when ready for sync.

### mappings

Manage mappings

- **`gradient-cli mappings create`** - Create Service for a Vendor for an Organization
- **`gradient-cli mappings update`** - Updates 1 or more fields of a single Service
- **`gradient-cli mappings update-bulk`** - Updates 1 or more fields for each Service

### services

Manage services

- **`gradient-cli services create`** - Create Service for a Vendor
- **`gradient-cli services get`** - Retrieves Vendor with VendorSku[] filtered by service id

### ticket_events

Manage ticket events

- **`gradient-cli ticket-events <messageId>`** - Checks for a ticket Event with the provided messageId to have created a ticket in the PSA

### vendor

Manage vendor

- **`gradient-cli vendor get`** - Get Vendor Details
- **`gradient-cli vendor update`** - Update details for a vendor


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
gradient-cli accounts list

# JSON for scripting and agents
gradient-cli accounts list --json

# Filter to specific fields
gradient-cli accounts list --json --select id,name,status

# Dry run  -  show the request without sending
gradient-cli accounts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
gradient-cli accounts list --agent
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
gradient-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/gradient-msp-synthesize-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `GRADIENT_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `gradient-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `gradient-cli doctor` to check credentials
- Verify the environment variable is set: `echo $GRADIENT_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized / UnauthorizedException on every call**  -  Set GRADIENT_VENDOR_API_KEY + GRADIENT_PARTNER_API_KEY (or GRADIENT_TOKEN); regenerate the pair in Synthesize -> Integrations -> Custom if lost - tokens are shown only once
- **Alert dispatched but no PSA ticket appears**  -  Ticket creation is async via a queue: run `gradient-cli alert trace --stuck` to find it, or dispatch with `alert send --wait` to poll until the ticket exists
- **Billing rebuilds repeatedly during a bulk count push**  -  Use `gradient-cli usage push --file <file>` - it sets no_build on every call except the last so Gradient rebuilds billing once
- **clients returns data but is marked deprecated**  -  Upstream deprecated the clients route; use `gradient-cli accounts list` and the mappings commands instead
