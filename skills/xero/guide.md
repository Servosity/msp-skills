# Xero CLI

**Every Xero Accounting read plus a local SQLite ledger  -  aging, reconciliation, and GL tie-out that no other Xero tool computes offline.**

A Go CLI for the Xero Accounting API across invoices, contacts, accounts, payments, bank transactions, items, and the immutable journals feed. It syncs the org into a queryable local store, then answers the questions the web UI, SDKs, and endpoint-mirroring MCP servers cannot in a single call: AR/AP aging (`aging`), payment and bank reconciliation gaps (`reconcile`, `bank-recon`), general-ledger-to-invoice tie-out (`tie-out`), and a running-balance ledger walk (`ledger`).

Learn more at [Xero](https://developer.xero.com).

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `xero-cli` binary and the `pp-xero` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install xero
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install xero --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install xero --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install xero --agent claude-code
npx -y @mvanhorn/printing-press-library install xero --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/payments/xero/cmd/xero-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/xero-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install xero --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-xero --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-xero --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install xero --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/xero-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `XERO_ACCESS_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/payments/xero/cmd/xero-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "xero": {
      "command": "xero-mcp",
      "env": {
        "XERO_ACCESS_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Xero requires two credentials on every call: an OAuth2 Bearer access token and a required Xero-Tenant-Id header. Set XERO_ACCESS_TOKEN (obtained via the OAuth2 authorization-code flow or a Xero Custom Connection client-credentials grant) and XERO_TENANT_ID (the target organisation's tenant id from GET /connections). Run `doctor` to confirm both are present before syncing.

## Quick Start

```bash
# confirm XERO_ACCESS_TOKEN and XERO_TENANT_ID are set and the API is reachable
xero-cli doctor

# mirror all seven Accounting entities into the local SQLite store
xero-cli sync

# bucket outstanding receivables by how overdue they are, offline
xero-cli aging --json

# surface invoices that are owed but have no applied payment
xero-cli reconcile --json

# prove the general ledger ties to the outstanding invoices
xero-cli tie-out --json

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Receivables & exposure
- **`aging`**  -  Bucket outstanding invoices into current / 1-30 / 31-60 / 61-90 / 90+ days overdue with totals per bucket, computed locally from your synced ledger.

  _Reach for this to answer 'who owes us and how overdue' in one offline call instead of paging the web UI per org._

  ```bash
  xero-cli aging --json
  ```
- **`exposure`**  -  Rank contacts by total outstanding amount due across their authorised invoices, with an overdue split and invoice count; drill into one contact for a running-balance statement.

  _Use to find concentration of receivable risk  -  who owes the most and how overdue  -  before a collections push._

  ```bash
  xero-cli exposure --json
  ```

### Reconciliation
- **`reconcile`**  -  Cross-join invoices and payments locally to surface AUTHORISED invoices with an amount still due and no (or partial) applied payment, plus payments not fully applied.

  _Pick this when you need the gap between the ledger and applied cash, not a raw list of either._

  ```bash
  xero-cli reconcile --json
  ```
- **`bank-recon`**  -  List bank transactions still flagged unreconciled and match them to invoices and payments by contact and exact amount to suggest likely reconciliation matches.

  _Use before a reconciliation session to see unreconciled spend and its probable invoice/payment matches up front._

  ```bash
  xero-cli bank-recon --json
  ```

### Ledger audit & tie-out
- **`tie-out`**  -  Sum the immutable journal lines by account code and compare the receivable/payable control-account totals against the sum of outstanding invoice amounts, reporting the variance.

  _Reach for this at period close to prove the books tie before signing off  -  variance of zero is the closure signal._

  ```bash
  xero-cli tie-out --json
  ```
- **`ledger`**  -  Replay the immutable journals feed for one account code as an ordered running-balance statement (date, journal number, debit, credit, running balance), entirely offline.

  _Use to audit exactly what posted to a single account without paging the read-only Journals view in the web UI._

  ```bash
  xero-cli ledger 200 --json
  ```

### Agent-native plumbing
- **`since`**  -  List records across every synced entity whose last-modified timestamp is newer than a given time, read entirely from the local store's sync cursor with zero API calls.

  _Pick this when an agent needs the org delta cheaply instead of re-pulling everything against the rate limit._

  ```bash
  xero-cli since 2026-05-01 --json
  ```
- **`snapshot`**  -  One offline call returning total receivable and payable outstanding, overdue count, unreconciled bank-transaction count, and per-table sync staleness as a single structured object.

  _Reach for this as an agent's single state-of-the-org call instead of fanning out across multiple list endpoints._

  ```bash
  xero-cli snapshot --json
  ```

## Recipes


### Monday AR sweep

```bash
xero-cli aging --json
```

Buckets every outstanding invoice by days overdue so you can build a chase list without exporting and pivoting in a spreadsheet.

### Narrow a verbose invoice list for an agent

```bash
xero-cli invoices get --agent --select Invoices.InvoiceNumber,Invoices.Contact.Name,Invoices.AmountDue,Invoices.Status
```

Xero invoice payloads are large and deeply nested; dotted-path --select returns only the fields an agent needs, keeping context small.

### Find the cash-application gap

```bash
xero-cli reconcile --json
```

Cross-joins invoices and payments locally to show authorised invoices with an amount still due and no applied payment.

### Prove the books tie at close

```bash
xero-cli tie-out --json
```

Compares the immutable general-ledger control accounts against the outstanding invoice totals and reports the variance.

### Audit one account's postings

```bash
xero-cli ledger 200 --json
```

Replays the journal feed for account code 200 as a running-balance statement from the local store.

## Usage

Run `xero-cli --help` for the full command reference and flag list.

## Commands

### accounts

Manage accounts

- **`xero-cli accounts create`** - Creates a new chart of accounts
- **`xero-cli accounts delete`** - Deletes a chart of accounts
- **`xero-cli accounts get`** - Retrieves the full chart of accounts
- **`xero-cli accounts get-accountid`** - Retrieves a single chart of accounts by using a unique account Id
- **`xero-cli accounts update`** - Updates a chart of accounts

### bank-transactions

Manage bank transactions

- **`xero-cli bank-transactions create`** - Creates one or more spent or received money transaction
- **`xero-cli bank-transactions get`** - Retrieves any spent or received money transactions
- **`xero-cli bank-transactions get-banktransactions`** - Retrieves a single spent or received money transaction by using a unique bank transaction Id
- **`xero-cli bank-transactions update`** - Updates a single spent or received money transaction
- **`xero-cli bank-transactions update-or-create`** - Updates or creates one or more spent or received money transaction

### contacts

Manage contacts

- **`xero-cli contacts create`** - Creates multiple contacts (bulk) in a Xero organisation
- **`xero-cli contacts get`** - Retrieves all contacts in a Xero organisation
- **`xero-cli contacts get-by-number`** - Retrieves a specific contact by contact number in a Xero organisation
- **`xero-cli contacts get-contactid`** - Retrieves a specific contacts in a Xero organisation using a unique contact Id
- **`xero-cli contacts update`** - Updates a specific contact in a Xero organisation
- **`xero-cli contacts update-or-create`** - Updates or creates one or more contacts in a Xero organisation

### invoices

Manage invoices

- **`xero-cli invoices create`** - Creates one or more sales invoices or purchase bills
- **`xero-cli invoices get`** - Retrieves sales invoices or purchase bills
- **`xero-cli invoices get-invoiceid`** - Retrieves a specific sales invoice or purchase bill using a unique invoice Id
- **`xero-cli invoices update`** - Updates a specific sales invoices or purchase bills
- **`xero-cli invoices update-or-create`** - Updates or creates one or more sales invoices or purchase bills

### items

Manage items

- **`xero-cli items create`** - Creates one or more items
- **`xero-cli items delete`** - Deletes a specific item
- **`xero-cli items get`** - Retrieves items
- **`xero-cli items get-itemid`** - Retrieves a specific item using a unique item Id
- **`xero-cli items update`** - Updates a specific item
- **`xero-cli items update-or-create`** - Updates or creates one or more items

### journals

Manage journals

- **`xero-cli journals get`** - Retrieves journals
- **`xero-cli journals get-by-number`** - Retrieves a specific journal using a unique journal number.
- **`xero-cli journals get-journalid`** - Retrieves a specific journal using a unique journal Id.

### payments

Manage payments

- **`xero-cli payments create`** - Creates a single payment for invoice or credit notes
- **`xero-cli payments create-endpoint`** - Creates multiple payments for invoices or credit notes
- **`xero-cli payments delete`** - Updates a specific payment for invoices and credit notes
- **`xero-cli payments get`** - Retrieves payments for invoices and credit notes
- **`xero-cli payments get-paymentid`** - Retrieves a specific payment for invoices and credit notes using a unique payment Id


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
xero-cli accounts get

# JSON for scripting and agents
xero-cli accounts get --json

# Filter to specific fields
xero-cli accounts get --json --select id,name,status

# Dry run  -  show the request without sending
xero-cli accounts get --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
xero-cli accounts get --agent
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
xero-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/xero-accounting-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `XERO_ACCESS_TOKEN` | per_call | No | Set to your API credential. |
| `XERO_OAUTH2` | per_call | No | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `xero-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `xero-cli doctor` to check credentials
- Verify the environment variable is set: `echo $XERO_ACCESS_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized / AuthenticationUnsuccessful**  -  Refresh XERO_ACCESS_TOKEN  -  Xero access tokens expire after 30 minutes; re-run the OAuth flow or custom-connection token mint.
- **403 or empty results when the token is valid**  -  Set XERO_TENANT_ID to the correct organisation  -  the tenant id from GET /connections is required on every call.
- **Transcendence commands (aging, reconcile, tie-out) return empty**  -  Run `xero-cli sync` first  -  these read the local store, not the live API.
- **429 rate limited**  -  Xero allows 60 calls/minute and 5000/day per tenant; the CLI's adaptive limiter backs off, but prefer the synced store + local analytics over repeated live calls.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**xero-mcp-server**](https://github.com/XeroAPI/xero-mcp-server)  -  TypeScript
- [**osodevops/xero-cli**](https://github.com/osodevops/xero-cli)  -  Go
- [**xero-command-line**](https://github.com/XeroAPI/xero-command-line)  -  JavaScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
