# MSPbots CLI

**The first MSPbots tool anywhere  -  readable filters, alias-named resources, full exports, and the KPI history MSPbots itself doesn't keep.**

MSPbots' Public API shares your BI datasets and widgets but ships with 19-digit IDs, a comma-encoded filter DSL, manual pagination, and no history. This CLI turns a shared API key into a usable data faucet: register aliases once, filter with readable predicates, export whole tables in one command, and snapshot KPIs into local SQLite for diffs and trends no MSPbots surface can show.

## Install

The recommended path installs both the `mspbots-cli` binary and the `pp-mspbots` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install mspbots
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install mspbots --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install mspbots --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install mspbots --agent claude-code
npx -y @mvanhorn/printing-press-library install mspbots --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/mspbots/cmd/mspbots-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/mspbots-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install mspbots --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-mspbots --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-mspbots --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install mspbots --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/mspbots-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `MSPBOTS_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/mspbots/cmd/mspbots-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

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

## Authentication

MSPbots uses a raw API key sent as an `apikey` request header. An MSPbots admin creates the key in the MSPbots app under Settings → Public API (Add New Key, type Custom), flips the global Enable Public API toggle ON, and explicitly binds each dataset and widget the key may read. Set the key in your environment as `MSPBOTS_API_KEY`. Every endpoint is read-only; a key can never write anything. If a pull returns "resource unbound", the dataset or widget has not been bound to your key  -  that binding happens in the MSPbots UI, not in this CLI.

## Quick Start

```bash
# Verify the binary, config, and connectivity checks before touching the API
mspbots-cli doctor --dry-run

# Name a dataset your admin bound to the key  -  aliases replace 19-digit IDs everywhere
mspbots-cli registry add <alias> <resourceId> --type dataset

# Fetch the first page of rows as clean JSON
mspbots-cli pull <alias> --page-size 10 --json

# Store a timestamped local copy  -  this builds the history MSPbots doesn't keep
mspbots-cli snapshot <alias>

# See week-over-week movement once two or more snapshots exist
mspbots-cli trend <alias> --column "Open Count"

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`registry`**  -  Name your bound datasets and widgets once, then use readable aliases everywhere instead of 19-digit resource IDs.

  _Agents and scripts can address resources by stable, human-readable names instead of copy-pasted snowflake IDs._

  ```bash
  mspbots-cli registry add <alias> <resourceId> --type dataset
  ```
- **`snapshot`**  -  Capture point-in-time copies of any dataset or widget into local SQLite  -  the history MSPbots doesn't keep.

  _Run it on a schedule and every later question about "how has this changed" becomes answerable offline._

  ```bash
  mspbots-cli snapshot <alias>
  ```
- **`trend`**  -  Time-series and point-over-point deltas for any numeric column across stored snapshots.

  _Answers "is this KPI up or down since last week"  -  the question the live API structurally cannot answer._

  ```bash
  mspbots-cli trend <alias> --column "Open Count"
  ```
- **`diff`**  -  Row-level added/removed/changed comparison between two stored snapshots of the same resource.

  _Shows exactly which tickets entered or left a queue between two captures, not just the count._

  ```bash
  mspbots-cli diff <alias>
  ```

### Agent-native plumbing
- **`pull`**  -  Write filters like "Update Date >= 2026-05-29" and the CLI compiles them into MSPbots' comma-encoded operator DSL.

  _Reach for this instead of hand-building query strings; it handles operator encoding, spaces in column names, and URL-escaping._

  ```bash
  mspbots-cli pull <alias> --where "Update Date >= 2026-05-29" --where "Status = Open" --json
  ```
- **`export`**  -  Dump an entire dataset or widget to CSV or JSONL, walking every page automatically.

  _One command replaces a babysat pagination loop when feeding spreadsheets or downstream pipelines._

  ```bash
  mspbots-cli export <alias> --format csv
  ```
- **`describe`**  -  Sample live rows and infer the column names and types of a dataset or widget.

  _Run it before building --where filters so column names and types are known instead of guessed._

  ```bash
  mspbots-cli describe <alias>
  ```

## Recipes


### Register and pull a shared dataset

```bash
mspbots-cli registry add <alias> <resourceId> --type dataset
```

One-time alias setup; every later command addresses the resource by its alias.

### Filtered pull with readable predicates

```bash
mspbots-cli pull <alias> --where "Update Date >= 2026-05-01" --where "Status = Open" --json
```

The CLI compiles readable operators into MSPbots' comma-encoded query DSL and URL-encodes spaced column names.

### Agent-shaped KPI read

```bash
mspbots-cli pull <alias> --agent --select row_count,rows
```

Returns only the row count and rows fields in agent-envelope JSON  -  bounded context for LLM consumption.

### Full CSV export for finance

```bash
mspbots-cli export <alias> --format csv
```

Walks every page via current/size automatically and streams one clean CSV.

### Week-over-week KPI movement

```bash
mspbots-cli trend <alias> --column "Open Count"
```

Aggregates the column across stored snapshots; pair with a scheduled `snapshot` to keep the series growing.

## Usage

Run `mspbots-cli --help` for the full command reference and flag list.

## Commands

### dataset

Fetch rows of datasets bound to your Public API key

- **`mspbots-cli dataset <resourceId>`** - Fetch one page of rows from a dataset bound to your API key

### widget

Fetch data of widgets bound to your Public API key

- **`mspbots-cli widget <resourceId>`** - Fetch one page of data from a bound widget (widgets with measure or calculate layers are not supported by the Public API)


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
mspbots-cli dataset <id>

# JSON for scripting and agents
mspbots-cli dataset <id> --json

# Filter to specific fields
mspbots-cli dataset <id> --json --select id,name,status

# Dry run  -  show the request without sending
mspbots-cli dataset <id> --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
mspbots-cli dataset <id> --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
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

Config file: `~/.config/mspbots-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

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
