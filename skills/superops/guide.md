# SuperOps CLI

**Every SuperOps PSA+RMM entity in your terminal, plus a local SQLite mirror that answers cross-entity questions the web UI can't.**

SuperOps unifies PSA and RMM on one relational database; this CLI syncs your whole tenant into local SQLite so you can grep, jq, and join across tickets, assets, clients, contracts, and invoices offline. Match every entity the GraphQL API exposes, then transcend with commands like sla-watch, unbilled, at-risk-assets, and alert-coverage that no single SuperOps call answers.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `superops-cli` and `superops-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/superops/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/superops/install.ps1 | iex
   ```
3. Verify: `superops-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=superops). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/superops --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/superops --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `superops-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the superops skill from https://github.com/Servosity/msp-skills/tree/main/skills/superops. The skill defines how its required CLI (`superops-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=superops).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `SUPEROPS_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS (Intel and Apple Silicon), Windows x64 and Linux x64; for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "superops": {
      "command": "superops-mcp",
      "env": {
        "SUPEROPS_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`superops-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `SUPEROPS_MCP_HTTP_TOKEN` is set to a random token of at least 32 characters (letters, digits, `-._~+/=`), and every request must send `Authorization: Bearer <token>`. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`.

```bash
export SUPEROPS_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
superops-mcp --transport http
```

Set `SUPEROPS_MCP_READ_ONLY=1` (exactly `1`) to list only read-only tools: it removes `raw`, `raw_mutation`, `sync`, `workflow_archive`, `jobs_prune` and the learn-store tools (`teach`, `teach_lookup`, `teach_pattern`, `teach_playbook`, `playbook_amend`, `learnings_confirm`, `learnings_reject`, `learnings_forget`). `raw_query` stays and refuses any document that is not a query.

## Authentication

Authenticate with a SuperOps API token (Settings - My Profile - API token) plus your tenant subdomain (Settings - MSP Information). Set SUPEROPS_API_TOKEN, SUPEROPS_SUBDOMAIN, and optionally SUPEROPS_REGION (us or eu). Every GraphQL request sends Authorization: Bearer <token> and the CustomerSubDomain header.

## Quick Start

```bash
# Verify token, subdomain, and API reachability before anything else
superops-cli doctor

# Pull the tenant into local SQLite so offline and cross-entity commands work
superops-cli sync

# List recent tickets as structured output an agent can parse
superops-cli tickets list --first 20 --agent

# See who is about to breach SLA grouped by technician
superops-cli sla-watch --by tech

# Surface worklog time that never got invoiced
superops-cli unbilled --since 2026-05-01

```

## Unique Features

### Cross-entity insight from local state
- **`sla-watch`**  -  See which open tickets are breaching or about to breach SLA, grouped by technician or client.

  _Reach for this to answer 'who is about to miss SLA and on whose queue' in one call instead of five filtered views._

  ```bash
  superops-cli sla-watch --by tech --window 4h --agent
  ```
- **`unbilled`**  -  Total billable worklog time per client from the local store, to reconcile against invoices.

  _Reach for this at month-end to see where billable time sits before invoicing._

  ```bash
  superops-cli unbilled --since 2026-05-01 --agent
  ```
- **`at-risk-assets`**  -  List assets missing a critical patch that also have an active (unresolved) alert.

  _Reach for this to prioritize remediation on endpoints that are both vulnerable and actively alerting._

  ```bash
  superops-cli at-risk-assets --client acme --agent
  ```
- **`alert-coverage`**  -  Partition alerts into open (uncovered) vs resolved, grouped by client.

  _Reach for this to catch clients with alerts still sitting unhandled  -  work nobody is tracking._

  ```bash
  superops-cli alert-coverage --client acme --agent
  ```
- **`client-360`**  -  One offline bundle of a client plus its sites, users, contracts, open tickets, assets, and open invoices.

  _Reach for this before a QBR or escalation to load the full client picture in one command._

  ```bash
  superops-cli client-360 acme --agent
  ```
- **`stale-tickets`**  -  Open tickets with no ticket update or worklog activity in N days.

  _Reach for this to catch neglected tickets before they turn into SLA misses or angry clients._

  ```bash
  superops-cli stale-tickets --days 7 --agent
  ```

### Agent-native plumbing
- **`context-ticket`**  -  Assemble a ticket plus its worklogs, client, and SLA into one agent-shaped JSON blob from the local store.

  _Reach for this as an AI triage agent's single read to ground a decision without six round-trips._

  ```bash
  superops-cli context-ticket 12345 --agent --select ticket.subject,ticket.status,worklogs
  ```

## Recipes

### Morning SLA triage by technician

```bash
superops-cli sla-watch --by tech --window 4h
```

Groups at-risk tickets per tech so the service desk knows where to push first.

### Month-end billable time per client

```bash
superops-cli unbilled --since 2026-05-01 --agent --select client,billableHours
```

Totals billable worklog hours per client with just the fields billing needs.

### Patch remediation priorities

```bash
superops-cli at-risk-assets --client acme
```

Endpoints that are both missing critical patches and carrying an active (unresolved) alert.

### Agent context for a ticket

```bash
superops-cli context-ticket 12345 --agent --select ticket.subject,ticket.status,client,sla,worklogs
```

Pairs --agent with --select on a deeply nested bundle so an AI agent gets only the fields it needs instead of a multi-KB blob.

### Idle ticket sweep

```bash
superops-cli stale-tickets --days 7 --agent
```

Open tickets with no ticket update or worklog activity in a week, longest idle first.

## Usage

Run `superops-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `SUPEROPS_CONFIG_DIR`, `SUPEROPS_DATA_DIR`, `SUPEROPS_STATE_DIR`, or `SUPEROPS_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `SUPEROPS_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export SUPEROPS_HOME=/srv/superops
superops-cli doctor
```

On Windows (PowerShell):

```powershell
$env:SUPEROPS_HOME = "C:\ProgramData\superops"
superops-cli doctor
```

Under `SUPEROPS_HOME=/srv/superops`, the four dirs resolve to `/srv/superops/config`, `/srv/superops/data`, `/srv/superops/state`, and `/srv/superops/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:SUPEROPS_HOME = "C:\srv\superops"
superops-cli doctor
```

Under `$env:SUPEROPS_HOME = "C:\srv\superops"`, the four dirs resolve to `C:\srv\superops\config`, `C:\srv\superops\data`, `C:\srv\superops\state`, and `C:\srv\superops\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "superops": {
      "command": "superops-mcp",
      "env": {
        "SUPEROPS_HOME": "/srv/superops"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `SUPEROPS_DATA_DIR` overrides an explicit `--home` for that kind. Use `SUPEROPS_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `SUPEROPS_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `superops-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: `sync`, and in `--data-source auto` mode a `<resource>` read, write what they return to the SQLite store in the data directory (`data-<hash>.db`, one per token, subdomain and host; `data.db` only when no token is set); the `list` subcommands (`tickets list` and the others) print without storing, so run `sync` for an offline copy. A record that repeats a credential the CLI holds is never stored: `sync` refuses that page. Each store records the tenant (subdomain and host) that first opened it for writing, which it does only while the file holds no data at all. A store of another tenant, or one that records no tenant, is refused for reading and writing, also when named with `--db`; a store that records no tenant and holds no data is filled by `superops-cli sync` (with the same `--db`), and a local command such as `stale-tickets` claims it for its tenant before reading it. GET responses are cached for five minutes under `http/` in the cache directory, kept apart per token, subdomain and host. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `superops-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `superops-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `SUPEROPS_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `SUPEROPS_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### alerts

Manage RMM alerts

- **`superops-cli alerts`** - List alerts

### assets

Manage SuperOps assets and endpoints

- **`superops-cli assets <id>`** - Get an asset by ID
- **`superops-cli assets`** - List assets

### clients

Manage SuperOps clients (accounts)

- **`superops-cli clients <id>`** - Get a client by account ID
- **`superops-cli clients`** - List clients

### contracts

Manage client contracts

- **`superops-cli contracts`** - List client contracts

### invoices

Manage invoices

- **`superops-cli invoices <id>`** - Get an invoice by ID
- **`superops-cli invoices`** - List invoices

### it-docs

Manage IT documentation

- **`superops-cli it-docs`** - List IT documentation

### kb

Manage knowledge base articles

- **`superops-cli kb`** - List knowledge base items

### service-items

Manage service catalog items

- **`superops-cli service-items`** - List service items

### sites

Manage client sites

- **`superops-cli sites`** - List client sites

### tasks

Manage tasks

- **`superops-cli tasks <id>`** - Get a task by ID
- **`superops-cli tasks`** - List tasks

### technicians

Manage technicians

- **`superops-cli technicians`** - List technicians

### tickets

Manage SuperOps tickets

- **`superops-cli tickets <id>`** - Get a ticket by display ID
- **`superops-cli tickets`** - List tickets

### users

Manage client users (contacts)

- **`superops-cli users`** - List client users

### worklogs

Manage worklog time entries

- **`superops-cli worklogs`** - List worklog entries


### alert-coverage


- **`superops-cli alert-coverage`** - Partition alerts into open (uncovered) vs resolved, grouped by client.


### at-risk-assets


- **`superops-cli at-risk-assets`** - List assets missing a critical patch that also have an active (unresolved) alert.


### client-360


- **`superops-cli client-360`** - One offline bundle of a client plus its sites, users, contracts, open tickets, assets, and open invoices.


### context-ticket


- **`superops-cli context-ticket`** - Assemble a ticket plus its worklogs, client, and SLA into one agent-shaped JSON blob from the local store.


### sla-watch


- **`superops-cli sla-watch`** - See which open tickets are breaching or about to breach SLA, grouped by technician or client.


### stale-tickets


- **`superops-cli stale-tickets`** - Open tickets with no ticket update or worklog activity in N days.


### unbilled


- **`superops-cli unbilled`** - Total billable worklog time per client from the local store, to reconcile against invoices.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`superops-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`superops-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`superops-cli learnings list`** - Inspect taught rows
- **`superops-cli learnings forget <query>`** - Undo a teach
- **`superops-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`superops-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`superops-cli teach-pattern`** - Install a query/resource template up front
- **`superops-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `SUPEROPS_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `superops-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
superops-cli alerts

# JSON for scripting and agents
superops-cli alerts --json
# Filter to specific fields
superops-cli alerts --json --select id,message,description

# Dry run  -  show the request without sending
superops-cli alerts --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
superops-cli alerts --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `8` async job submitted but not finished (rerun the printed recovery command; do not resubmit), `10` config error.

## Health Check

```bash
superops-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `superops-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/superops-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\superops-cli\config.toml` on Windows; `--home`, `SUPEROPS_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `SUPEROPS_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `SUPEROPS_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SUPEROPS_API_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `superops-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `superops-cli doctor` to check credentials
- Verify the environment variable is set: `echo $SUPEROPS_API_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **graphql access denied / UNAUTHENTICATED**  -  Regenerate the API token in Settings - My Profile and re-export SUPEROPS_API_TOKEN; only one token is valid per user at a time.
- **Empty results despite data existing**  -  Confirm SUPEROPS_SUBDOMAIN matches Settings - MSP Information exactly; the CustomerSubDomain header scopes every query to that tenant.
- **Wrong data center / connection errors**  -  Set SUPEROPS_REGION=eu if your tenant is hosted in the EU (euapi.superops.ai); default is us.
- **HTTP 429 / rate limited**  -  SuperOps caps at 800 requests per minute; lower sync --concurrency or let the adaptive limiter back off.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**wyre-technology/superops-mcp**](https://github.com/wyre-technology/superops-mcp)  -  Python
- [**dovetechnow/superops-tools**](https://github.com/dovetechnow/superops-tools)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
