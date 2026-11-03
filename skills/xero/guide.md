# Xero CLI

**Every Xero Accounting read plus a local SQLite ledger  -  aging, reconciliation, and GL tie-out that no other Xero tool computes offline.**

A Go CLI for the Xero Accounting API across invoices, contacts, accounts, payments, bank transactions, items, and the immutable journals feed. It syncs the org into a queryable local store, then answers the questions the web UI, SDKs, and endpoint-mirroring MCP servers cannot in a single call: AR/AP aging (`aging`), payment and bank reconciliation gaps (`reconcile`, `bank-recon`), general-ledger-to-invoice tie-out (`tie-out`), and a running-balance ledger walk (`ledger`).

Learn more at [Xero](https://developer.xero.com).

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `xero-cli` and `xero-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.ps1 | iex
   ```
3. Verify: `xero-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=xero). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/xero --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/xero --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `xero-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the xero skill from https://github.com/Servosity/msp-skills/tree/main/skills/xero. The skill defines how its required CLI (`xero-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=xero).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `XERO_ACCESS_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/xero/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

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

### Remote MCP over HTTP

`xero-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `XERO_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `XERO_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export XERO_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
xero-mcp --transport http
```

Set `XERO_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

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
xero-cli invoices list --agent --select Invoices.InvoiceNumber,Invoices.Contact.Name,Invoices.AmountDue,Invoices.Status
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

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `XERO_CONFIG_DIR`, `XERO_DATA_DIR`, `XERO_STATE_DIR`, or `XERO_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `XERO_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export XERO_HOME=/srv/xero
xero-cli doctor
```

On Windows (PowerShell):

```powershell
$env:XERO_HOME = "C:\ProgramData\xero"
xero-cli doctor
```

Under `XERO_HOME=/srv/xero`, the four dirs resolve to `/srv/xero/config`, `/srv/xero/data`, `/srv/xero/state`, and `/srv/xero/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:XERO_HOME = "C:\srv\xero"
xero-cli doctor
```

Under `$env:XERO_HOME = "C:\srv\xero"`, the four dirs resolve to `C:\srv\xero\config`, `C:\srv\xero\data`, `C:\srv\xero\state`, and `C:\srv\xero\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "xero": {
      "command": "xero-mcp",
      "env": {
        "XERO_HOME": "/srv/xero"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `XERO_DATA_DIR` overrides an explicit `--home` for that kind. Use `XERO_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `XERO_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `xero-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `xero-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `xero-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `XERO_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `XERO_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

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


### aging


- **`xero-cli aging`** - Bucket outstanding invoices into current / 1-30 / 31-60 / 61-90 / 90+ days overdue with totals per bucket, computed locally from your synced ledger.


### bank-recon


- **`xero-cli bank-recon`** - List bank transactions still flagged unreconciled and match them to invoices and payments by contact and exact amount to suggest likely reconciliation matches.


### exposure


- **`xero-cli exposure`** - Rank contacts by total outstanding amount due across their authorised invoices, with an overdue split and invoice count; drill into one contact for a running-balance statement.


### ledger


- **`xero-cli ledger`** - Replay the immutable journals feed for one account code as an ordered running-balance statement (date, journal number, debit, credit, running balance), entirely offline.


### reconcile


- **`xero-cli reconcile`** - Cross-join invoices and payments locally to surface AUTHORISED invoices with an amount still due and no (or partial) applied payment, plus payments not fully applied.


### since


- **`xero-cli since`** - List records across every synced entity whose last-modified timestamp is newer than a given time, read entirely from the local store's sync cursor with zero API calls.


### snapshot


- **`xero-cli snapshot`** - One offline call returning total receivable and payable outstanding, overdue count, unreconciled bank-transaction count, and per-table sync staleness as a single structured object.


### tie-out


- **`xero-cli tie-out`** - Sum the immutable journal lines by account code and compare the receivable/payable control-account totals against the sum of outstanding invoice amounts, reporting the variance.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`xero-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`xero-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`xero-cli learnings list`** - Inspect taught rows
- **`xero-cli learnings forget <query>`** - Undo a teach
- **`xero-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`xero-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`xero-cli teach-pattern`** - Install a query/resource template up front
- **`xero-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `XERO_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `xero-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
xero-cli accounts get

# JSON for scripting and agents
xero-cli accounts get --json
# Filter to specific fields
xero-cli accounts get --json --select AccountID,AddToWatchlist,BankAccountNumber

# Dry run  -  show the request without sending
xero-cli accounts get --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
xero-cli accounts get --agent
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
xero-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `xero-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/xero-accounting-pp-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\xero-cli\config.toml` on Windows; `--home`, `XERO_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `XERO_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `XERO_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `XERO_ACCESS_TOKEN` | per_call | No | Set to your API credential. |
| `XERO_OAUTH2` | per_call | No | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `xero-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

### Keeping credentials off disk (optional)

By default the connector caches the token it mints in its credentials file so the
next command does not have to mint another one. Set `XERO_NO_CONFIG_WRITE=1` to turn
that off: nothing is written, and each run mints a fresh token instead. Use it when
your secrets live in Keychain, Windows Credential Manager, or any launcher that
injects them as environment variables at process start, and you do not want a
plaintext copy on disk.

| What changes | With `XERO_NO_CONFIG_WRITE=1` |
| --- | --- |
| Token cache | Not written. A fresh token is minted per invocation. |
| `auth login` | Refuses, naming the variable, instead of reporting a save that did not happen. |
| `auth set-token` | Refuses the same way. |
| `auth logout` | Still clears an existing credentials file. The erase is not a credential write. |
| MCP server | Honours the same variable, so a Claude Desktop install gets no plaintext token cache either. |

Set it in the shell for CLI use, or through the install prompt of the same name
for the MCP server. Any value other than blank, `0`, `false`, `no` or `off`
turns it on. See issue #270.

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
