# Connectwise Control CLI

**Drive a ConnectWise Control (ScreenConnect) instance from the terminal: sessions, session groups, users, and the audit log, with an offline SQLite mirror.**

ConnectWise Control (ScreenConnect) is a web-console remote-support tool. This CLI maps its instance session, user, and audit surface to typed commands with JSON output, so you can list, inspect, and act on sessions from a script or an AI agent. Authentication is HTTP Basic with your instance login.

## Install

For the short install path see [README.md](./README.md). For wiring the MCP
server into every agent (Claude Desktop, ChatGPT, Codex, and more), see
[mcp-install.md](./mcp-install.md). This file is the command reference.

## Quick Start

### 1. Install

See [Install](#install) above.

### 2. Set Up Credentials

Get your API key from your API provider's developer portal. The key typically looks like a long alphanumeric string.

```bash
export CONNECTWISE_CONTROL_USERNAME="<paste-your-key>"
```

You can also persist this in your config file at `~/.config/connectwise-control-cli/config.toml`.

### 3. Verify Setup

```bash
connectwise-control-cli doctor
```

This checks your configuration and credentials.

### 4. Try Your First Command

```bash
connectwise-control-cli session-groups
```

## Usage

Run `connectwise-control-cli --help` for the full command reference and flag list.

## Commands

### audit

Audit metadata and audit-log queries (AuditService).

- **`connectwise-control-cli audit get-info`** - Returns metadata about the audit subsystem (available event types and retention). Wire format: empty positional array `[]`.
- **`connectwise-control-cli audit query-log`** - Queries audit-log records by session, time window, and event type. Wire format: a positional array carrying the query filter fields.

### security

Users, roles, and security configuration (SecurityService).

- **`connectwise-control-cli security delete-user`** - Deletes an instance user. Requires the `x-anti-forgery-token` header. Wire format: positional array `["<userName>"]`.
- **`connectwise-control-cli security get-configuration`** - Returns the instance security configuration, including users and roles. Wire format: empty positional array `[]`.
- **`connectwise-control-cli security save-user`** - Creates a new instance user or updates an existing one. Requires the `x-anti-forgery-token` header. Wire format: a positional array carrying the user record fields.

### session_groups

Session groups that organize sessions (SessionGroupService).

- **`connectwise-control-cli session-groups`** - Returns all session groups configured on the instance.

### sessions

Remote support, access, and meeting sessions (PageService).

- **`connectwise-control-cli sessions add-event-to`** - Queues a session event (such as Wake-on-LAN) against one or more sessions. Wire format: positional array `["<group>", ["<sessionID>"], <sessionEventType>, ""]`.
- **`connectwise-control-cli sessions get-access-token`** - Issues a one-time access token/URL to join a session. Wire format: positional array `["<group>", "<sessionID>"]`.
- **`connectwise-control-cli sessions get-detail`** - Returns full detail (connections and recent events) for one session. Wire format: positional array `["<group>", "<sessionID>"]`.
- **`connectwise-control-cli sessions list`** - Returns the live session list for a session type and group. Wire format: the endpoint expects a positional JSON array `[{ "HostSessionInfo": { "sessionType": <0|1|2>, "sessionGroupPathParts": ["<group>"], "filter": "<search>", "findSessionID": "<guid>", "sessionLimit": <n> }, "ActionCenterInfo": {} }, 0]` and returns sessions under `ResponseInfoMap.HostSessionInfo.Sessions`.
- **`connectwise-control-cli sessions run-command`** - Queues a command or control event on a session. Used to run a shell or PowerShell command on a guest, or to end a session. Wire format: positional array `[["<group>"], [ <commandObject> ]]` where the command object is an event such as `{ "SessionID": "<guid>", "EventType": 44, "Data": "<command>" }` (EventType 44 = QueuedCommand). Running a command requires the `x-anti-forgery-token` header.
- **`connectwise-control-cli sessions update-custom-property`** - Sets one custom property value on a session. Wire format: positional array `["<group>", "<sessionID>", <propertyIndex>, "<value>"]`. Requires the `x-anti-forgery-token` header.
- **`connectwise-control-cli sessions update-name`** - Renames a session. Wire format: positional array `["<group>", "<sessionID>", "<newName>"]`. Requires the `x-anti-forgery-token` header.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
connectwise-control-cli session-groups

# JSON for scripting and agents
connectwise-control-cli session-groups --json

# Filter to specific fields
connectwise-control-cli session-groups --json --select id,name,status

# Dry run  -  show the request without sending
connectwise-control-cli session-groups --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
connectwise-control-cli session-groups --agent
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
connectwise-control-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/connectwise-control-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `CONNECTWISE_CONTROL_USERNAME` | per_call | Yes |  |
| `CONNECTWISE_CONTROL_PASSWORD` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `connectwise-control-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `connectwise-control-cli doctor` to check credentials
- Verify the environment variable is set: `echo $CONNECTWISE_CONTROL_USERNAME`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

## HTTP Transport

This CLI uses Chrome-compatible HTTP transport for browser-facing endpoints. It does not require a resident browser process for normal API calls.

---

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
