# Gradient MSP CLI

**The Synthesize partner API from the terminal - every endpoint, plus a usage-push ledger, billing drift detection, and alert-to-ticket tracing no other Gradient tool has.**

Gradient MSP's Synthesize platform reconciles vendor usage against MSP billing, but its PowerShell SDK makes you write a script project per integration. This CLI covers the full vendor API surface agent-natively with an offline SQLite mirror of your accounts, and adds what the API itself cannot: a local ledger of every count you push (usage drift), debounce-aware bulk pushes (usage push), mapping-hygiene rollups (hygiene unmapped), and async alert-to-ticket confirmation (alert send --wait).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `gradient-cli` and `gradient-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.ps1 | iex
   ```
3. Verify: `gradient-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=gradient). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/gradient --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/gradient --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `gradient-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the gradient skill from https://github.com/Servosity/msp-skills/tree/main/skills/gradient. The skill defines how its required CLI (`gradient-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=gradient).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `GRADIENT_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/gradient/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

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

### Remote MCP over HTTP

`gradient-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `GRADIENT_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `GRADIENT_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export GRADIENT_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
gradient-mcp --transport http
```

Set `GRADIENT_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

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

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `GRADIENT_CONFIG_DIR`, `GRADIENT_DATA_DIR`, `GRADIENT_STATE_DIR`, or `GRADIENT_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `GRADIENT_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export GRADIENT_HOME=/srv/gradient
gradient-cli doctor
```

On Windows (PowerShell):

```powershell
$env:GRADIENT_HOME = "C:\ProgramData\gradient"
gradient-cli doctor
```

Under `GRADIENT_HOME=/srv/gradient`, the four dirs resolve to `/srv/gradient/config`, `/srv/gradient/data`, `/srv/gradient/state`, and `/srv/gradient/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:GRADIENT_HOME = "C:\srv\gradient"
gradient-cli doctor
```

Under `$env:GRADIENT_HOME = "C:\srv\gradient"`, the four dirs resolve to `C:\srv\gradient\config`, `C:\srv\gradient\data`, `C:\srv\gradient\state`, and `C:\srv\gradient\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "gradient": {
      "command": "gradient-mcp",
      "env": {
        "GRADIENT_HOME": "/srv/gradient"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `GRADIENT_DATA_DIR` overrides an explicit `--home` for that kind. Use `GRADIENT_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `GRADIENT_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `gradient-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `gradient-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `gradient-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `GRADIENT_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `GRADIENT_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

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
- **`gradient-cli mappings update <serviceId>`** - Updates 1 or more fields of a single Service
- **`gradient-cli mappings update-bulk`** - Updates 1 or more fields for each Service

### services

Manage services

- **`gradient-cli services create`** - Create Service for a Vendor
- **`gradient-cli services get <serviceId>`** - Retrieves Vendor with VendorSku[] filtered by service id

### ticket_events

Manage ticket events

- **`gradient-cli ticket-events <messageId>`** - Checks for a ticket Event with the provided messageId to have created a ticket in the PSA

### vendor

Manage vendor

- **`gradient-cli vendor get`** - Get Vendor Details
- **`gradient-cli vendor update`** - Update details for a vendor


### alert


- **`gradient-cli alert send`** - Dispatch an alert and wait until the PSA ticket actually exists, instead of fire-and-forget.

- **`gradient-cli alert trace`** - Review every alert you have dispatched - messageId, account, time, last-known ticket state - and filter to ones that never became tickets.


### hygiene


- **`gradient-cli hygiene unmapped`** - One rollup of every unmapped account and every service without a vendor SKU, shaped as an actionable work queue.


### status


- **`gradient-cli status ready`** - A single go/no-go verdict on whether the integration is ready to flip to active: live status, last sync age, and outstanding mapping gaps.


### usage


- **`gradient-cli usage push`** - Push a whole file of unit counts (CSV or JSON) in one shot and trigger exactly one billing rebuild instead of one per call.

- **`gradient-cli usage drift`** - See exactly which accounts' usage changed between your last two pushes, with old-to-new deltas.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`gradient-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`gradient-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`gradient-cli learnings list`** - Inspect taught rows
- **`gradient-cli learnings forget <query>`** - Undo a teach
- **`gradient-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`gradient-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`gradient-cli teach-pattern`** - Install a query/resource template up front
- **`gradient-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `GRADIENT_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `gradient-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
gradient-cli accounts list

# JSON for scripting and agents
gradient-cli accounts list --json
# Filter to specific fields
gradient-cli accounts list --json --select description,id,isMapped

# Dry run  -  show the request without sending
gradient-cli accounts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
gradient-cli accounts list --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Health Check

```bash
gradient-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `gradient-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/gradient-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\gradient-cli\config.toml` on Windows; `--home`, `GRADIENT_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `GRADIENT_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `GRADIENT_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `GRADIENT_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `gradient-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- The `GRADIENT-TOKEN` header and every API request go only to the scheme, host and port of `GRADIENT_BASE_URL` (default `https://app.usegradient.com/api`). A redirect to any other host is refused, so the credential never reaches it.
- Any output, file or MCP result that would repeat `GRADIENT_TOKEN`, the vendor or partner API key, the header derived from them, or a credential-named value from a response prints "[text withheld: it repeated a secret value this program holds]" in its place. Responses are never cached on disk, credential-named fields are dropped before anything reaches the local store, and the push and alert ledgers pass the same withholding.
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the token from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, a tool that creates or changes Synthesize data (accounts, services, mappings, billing counts, alerts, integration status, vendor profile, usage push, import) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write (`usage push --file` is CLI-only).

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
- **Billing rebuilds repeatedly during a bulk count push**  -  Push the whole counts file in one run with usage push and its --file flag (CSV or JSON): it sets no_build on every call except the last, so Gradient rebuilds billing once
- **clients returns data but is marked deprecated**  -  Upstream deprecated the clients route; use `gradient-cli accounts list` and the mappings commands instead
