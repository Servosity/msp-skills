---
name: connectwise-control
description: "Use when the user asks to list, search, or inspect ConnectWise Control (ScreenConnect) remote-support and access sessions, run a command on a guest machine, rename or tag sessions, manage instance users, or read the audit log. Turns the ScreenConnect instance surface into typed commands with an offline SQLite mirror. Trigger phrases: `list connectwise control sessions`, `screenconnect session detail`, `run command on a screenconnect guest`, `connectwise control audit log`, `use connectwise control`, `run connectwise-control-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "ConnectWise"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - connectwise-control-cli
---

# ConnectWise Control Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `connectwise-control-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectwise-control/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connectwise-control/install.ps1 | iex
   ```
3. Verify: `connectwise-control-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

Drive a ConnectWise Control (ScreenConnect) instance from the terminal or an AI agent: list and inspect remote-support and access sessions, read session detail and the audit log, manage session names and custom properties, manage instance users, and run an approved command on a guest machine. Output is JSON-first with an offline SQLite mirror for fast session lookups. Authentication is HTTP Basic with your instance login (`CONNECTWISE_CONTROL_USERNAME` / `CONNECTWISE_CONTROL_PASSWORD`) against your instance base URL (`CONNECTWISE_CONTROL_BASE_URL`).

## HTTP Transport

This CLI uses Chrome-compatible HTTP transport for browser-facing endpoints. It does not require a resident browser process for normal API calls.

## Command Reference

**audit**  -  Audit metadata and audit-log queries (AuditService).

- `connectwise-control-cli audit get-info`  -  Returns metadata about the audit subsystem (available event types and retention).
- `connectwise-control-cli audit query-log`  -  Queries audit-log records by session, time window, and event type.

**security**  -  Users, roles, and security configuration (SecurityService).

- `connectwise-control-cli security delete-user`  -  Deletes an instance user. Requires the `x-anti-forgery-token` header. Wire format: positional array `['<userName>']`.
- `connectwise-control-cli security get-configuration`  -  Returns the instance security configuration, including users and roles. Wire format: empty positional array `[]`.
- `connectwise-control-cli security save-user`  -  Creates a new instance user or updates an existing one. Requires the `x-anti-forgery-token` header.

**session_groups**  -  Session groups that organize sessions (SessionGroupService).

- `connectwise-control-cli session-groups`  -  Returns all session groups configured on the instance.

**sessions**  -  Remote support, access, and meeting sessions (PageService).

- `connectwise-control-cli sessions add-event-to`  -  Queues a session event (such as Wake-on-LAN) against one or more sessions.
- `connectwise-control-cli sessions get-access-token`  -  Issues a one-time access token/URL to join a session. Wire format: positional array `['<group>', '<sessionID>']`.
- `connectwise-control-cli sessions get-detail`  -  Returns full detail (connections and recent events) for one session.
- `connectwise-control-cli sessions list`  -  Returns the live session list for a session type and group.
- `connectwise-control-cli sessions run-command`  -  Queues a command or control event on a session.
- `connectwise-control-cli sessions update-custom-property`  -  Sets one custom property value on a session.
- `connectwise-control-cli sessions update-name`  -  Renames a session. Wire format: positional array `['<group>', '<sessionID>', '<newName>']`.


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
connectwise-control-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query.

## Auth Setup

ConnectWise Control authenticates with **HTTP Basic** auth - the same user name and
password you use for the web console - against your instance base URL. Set all three:

```bash
export CONNECTWISE_CONTROL_BASE_URL="https://company.screenconnect.com"
export CONNECTWISE_CONTROL_USERNAME="<your-instance-user>"
export CONNECTWISE_CONTROL_PASSWORD="<your-instance-password>"
```

Or persist them in `~/.config/connectwise-control-cli/config.toml`.

Run `connectwise-control-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  connectwise-control-cli session-groups --agent --select id,name,status
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
connectwise-control-cli feedback "the --since flag is inclusive but docs say exclusive"
connectwise-control-cli feedback --stdin < notes.txt
connectwise-control-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/connectwise-control-cli/feedback.jsonl`. They are never POSTed unless `CONNECTWISE_CONTROL_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CONNECTWISE_CONTROL_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration.

```
connectwise-control-cli profile save briefing --json
connectwise-control-cli --profile briefing session-groups
connectwise-control-cli profile list --json
connectwise-control-cli profile show briefing
connectwise-control-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `connectwise-control-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add connectwise-control-mcp -- connectwise-control-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which connectwise-control-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   connectwise-control-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `connectwise-control-cli <command> --help`.
