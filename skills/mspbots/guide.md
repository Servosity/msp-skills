# MSPbots CLI

**The first MSPbots tool anywhere  -  readable filters, alias-named resources, full exports, and the KPI history MSPbots itself doesn't keep.**

MSPbots' Public API shares your BI datasets and widgets but ships with 19-digit IDs, a comma-encoded filter DSL, manual pagination, and no history. This CLI turns a shared API key into a usable data faucet: register aliases once, filter with readable predicates, export whole tables in one command, and snapshot KPIs into local SQLite for diffs and trends no MSPbots surface can show.

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `mspbots-cli` and `mspbots-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mspbots/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mspbots/install.ps1 | iex
   ```
3. Verify: `mspbots-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=mspbots). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/mspbots --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/mspbots --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `mspbots-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the mspbots skill from https://github.com/Servosity/msp-skills/tree/main/skills/mspbots. The skill defines how its required CLI (`mspbots-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=mspbots).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `MSPBOTS_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mspbots/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/mspbots/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "mspbots": {
      "command": "mspbots-mcp",
      "env": {
        "MSPBOTS_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`mspbots-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `MSPBOTS_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `MSPBOTS_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export MSPBOTS_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
mspbots-mcp --transport http
```

Set `MSPBOTS_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

MSPbots uses a raw API key sent as an `apikey` request header. An MSPbots admin creates the key in the MSPbots app under Settings → Public API (Add New Key, type Custom), flips the global Enable Public API toggle ON, and explicitly binds each dataset and widget the key may read. Set the key in your environment as `MSPBOTS_API_KEY`. Every endpoint is read-only; a key can never write anything. If a pull returns "resource unbound", the dataset or widget has not been bound to your key  -  that binding happens in the MSPbots UI, not in this CLI.

## Quick Start

```bash
# Verify the binary, config, and connectivity checks before touching the API
mspbots-cli doctor --dry-run


# Name a dataset your admin bound to the key  -  aliases replace 19-digit IDs everywhere
mspbots-cli registry add open-tickets 1534956341424005122 --type dataset


# Fetch the first page of rows as clean JSON
mspbots-cli pull open-tickets --page-size 10 --json


# Store a timestamped local copy  -  this builds the history MSPbots doesn't keep
mspbots-cli snapshot open-tickets


# See week-over-week movement once two or more snapshots exist
mspbots-cli trend open-tickets --column "Open Count"

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local state that compounds

- **`registry`**  -  Name your bound datasets and widgets once, then use readable aliases everywhere instead of 19-digit resource IDs.

  _Agents and scripts can address resources by stable, human-readable names instead of copy-pasted snowflake IDs._

  ```bash
  mspbots-cli registry add open-tickets 1534956341424005122 --type dataset
  ```
- **`snapshot`**  -  Capture point-in-time copies of any dataset or widget into local SQLite  -  the history MSPbots doesn't keep.

  _Run it on a schedule and every later question about "how has this changed" becomes answerable offline._

  ```bash
  mspbots-cli snapshot open-tickets
  ```
- **`trend`**  -  Time-series and point-over-point deltas for any numeric column across stored snapshots.

  _Answers "is this KPI up or down since last week"  -  the question the live API structurally cannot answer._

  ```bash
  mspbots-cli trend open-tickets --column "Open Count"
  ```
- **`diff`**  -  Row-level added/removed/changed comparison between two stored snapshots of the same resource.

  _Shows exactly which tickets entered or left a queue between two captures, not just the count._

  ```bash
  mspbots-cli diff open-tickets
  ```

### Agent-native plumbing

- **`pull`**  -  Write filters like "Update Date >= 2026-05-29" and the CLI compiles them into MSPbots' comma-encoded operator DSL.

  _Reach for this instead of hand-building query strings; it handles operator encoding, spaces in column names, and URL-escaping._

  ```bash
  mspbots-cli pull open-tickets --where "Update Date >= 2026-05-29" --where "Status = Open" --json
  ```
- **`export`**  -  Dump an entire dataset or widget to CSV or JSONL, walking every page automatically.

  _One command replaces a babysat pagination loop when feeding spreadsheets or downstream pipelines._

  ```bash
  mspbots-cli export open-tickets --format csv
  ```
- **`describe`**  -  Sample live rows and infer the column names and types of a dataset or widget.

  _Run it before building --where filters so column names and types are known instead of guessed._

  ```bash
  mspbots-cli describe open-tickets
  ```

## Recipes


### Register and pull a shared dataset

```bash
mspbots-cli registry add sla-queue 1534956341424005122 --type dataset
```

One-time alias setup; every later command addresses the resource as sla-queue.

### Filtered pull with readable predicates

```bash
mspbots-cli pull sla-queue --where "Update Date >= 2026-05-01" --where "Status = Open" --json
```

The CLI compiles readable operators into MSPbots' comma-encoded query DSL and URL-encodes spaced column names.

### Agent-shaped KPI read

```bash
mspbots-cli pull sla-queue --agent --select row_count,rows
```

Returns only the row count and rows fields in agent-envelope JSON  -  bounded context for LLM consumption.

### Full CSV export for finance

```bash
mspbots-cli export sla-queue --format csv
```

Walks every page via current/size automatically and streams one clean CSV.

### Week-over-week KPI movement

```bash
mspbots-cli trend sla-queue --column "Open Count"
```

Aggregates the column across stored snapshots; pair with a scheduled `snapshot` to keep the series growing.

## Usage

Run `mspbots-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `MSPBOTS_CONFIG_DIR`, `MSPBOTS_DATA_DIR`, `MSPBOTS_STATE_DIR`, or `MSPBOTS_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `MSPBOTS_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export MSPBOTS_HOME=/srv/mspbots
mspbots-cli doctor
```

On Windows (PowerShell):

```powershell
$env:MSPBOTS_HOME = "C:\ProgramData\mspbots"
mspbots-cli doctor
```

Under `MSPBOTS_HOME=/srv/mspbots`, the four dirs resolve to `/srv/mspbots/config`, `/srv/mspbots/data`, `/srv/mspbots/state`, and `/srv/mspbots/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:MSPBOTS_HOME = "C:\srv\mspbots"
mspbots-cli doctor
```

Under `$env:MSPBOTS_HOME = "C:\srv\mspbots"`, the four dirs resolve to `C:\srv\mspbots\config`, `C:\srv\mspbots\data`, `C:\srv\mspbots\state`, and `C:\srv\mspbots\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "mspbots": {
      "command": "mspbots-mcp",
      "env": {
        "MSPBOTS_HOME": "/srv/mspbots"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `MSPBOTS_DATA_DIR` overrides an explicit `--home` for that kind. Use `MSPBOTS_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `MSPBOTS_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `mspbots-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `mspbots-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `mspbots-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `MSPBOTS_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `MSPBOTS_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### dataset

Fetch rows of datasets bound to your Public API key

- **`mspbots-cli dataset <resourceId>`** - Fetch one page of rows from a dataset bound to your API key

### widget

Fetch data of widgets bound to your Public API key

- **`mspbots-cli widget <resourceId>`** - Fetch one page of data from a bound widget (widgets with measure or calculate layers are not supported by the Public API)


### describe


- **`mspbots-cli describe`** - Sample live rows and infer the column names and types of a dataset or widget.


### diff


- **`mspbots-cli diff`** - Row-level added/removed/changed comparison between two stored snapshots of the same resource.


### export


- **`mspbots-cli export`** - Dump an entire dataset or widget to CSV or JSONL, walking every page automatically.


### pull


- **`mspbots-cli pull`** - Write filters like 'Update Date >= 2026-05-29' and the CLI compiles them into MSPbots' comma-encoded operator DSL.


### registry


- **`mspbots-cli registry`** - Name your bound datasets and widgets once, then use readable aliases everywhere instead of 19-digit resource IDs.


### snapshot


- **`mspbots-cli snapshot`** - Capture point-in-time copies of any dataset or widget into local SQLite  -  the history MSPbots doesn't keep.


### trend


- **`mspbots-cli trend`** - Time-series and point-over-point deltas for any numeric column across stored snapshots.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`mspbots-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`mspbots-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`mspbots-cli learnings list`** - Inspect taught rows
- **`mspbots-cli learnings forget <query>`** - Undo a teach
- **`mspbots-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`mspbots-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`mspbots-cli teach-pattern`** - Install a query/resource template up front
- **`mspbots-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `MSPBOTS_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `mspbots-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
mspbots-cli dataset mock-value

# JSON for scripting and agents
mspbots-cli dataset mock-value --json
# Filter to specific fields by name
mspbots-cli dataset mock-value --json --select <field>[,<field>...]

# Dry run  -  show the request without sending
mspbots-cli dataset mock-value --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
mspbots-cli dataset mock-value --agent
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

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
mspbots-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `mspbots-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/mspbots-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\mspbots-cli\config.toml` on Windows; `--home`, `MSPBOTS_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `MSPBOTS_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `MSPBOTS_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `MSPBOTS_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `mspbots-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `mspbots-cli doctor` to check credentials
- Verify the environment variable is set: `echo $MSPBOTS_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific

- **HTTP 401 {"message":"Invalid API key in request"}**  -  Check MSPBOTS_API_KEY; keys are created at Settings → Public API in the MSPbots app, and the global Enable Public API toggle must be ON.
- **Resource unbound error on a valid key**  -  The dataset/widget is not bound to your key  -  an MSPbots admin must add it under Settings → Public API → Datasets/Widget → Add.
- **HTTP 502 when pulling a widget**  -  Known intermittent gateway issue on heavy widgets  -  retry with a smaller --page-size; note widgets with measure or calculate layers are not supported by the Public API at all.
- **Filtered pull returns zero rows unexpectedly**  -  Column names are exact (including spaces and casing)  -  run `mspbots-cli describe <alias>` to see the inferred columns before writing --where filters.
- **Requests start failing after rapid pulls**  -  The API enforces rate limits (thresholds undocumented)  -  space out calls or reduce export page size.

---

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
