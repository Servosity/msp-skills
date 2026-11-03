# ConnectWise Control CLI

**Manage ConnectWise Control (ScreenConnect) sessions, users and the audit log from one CLI.**

connectwise-control-cli drives a ConnectWise Control (ScreenConnect) instance from the terminal or an AI agent: list and inspect remote-support and access sessions, read session detail and the audit log, rename and tag sessions, manage instance users, and run an approved command on a guest machine. `sessions stale` finds access agents that went dark and `security access-review` checks every instance user in one call.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `connectwise-control-cli` and `connectwise-control-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-control/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-control/install.ps1 | iex
   ```
3. Verify: `connectwise-control-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=connectwise-control). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/connectwise-control --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/connectwise-control --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `connectwise-control-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the connectwise-control skill from https://github.com/Servosity/msp-skills/tree/main/skills/connectwise-control. The skill defines how its required CLI (`connectwise-control-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=connectwise-control).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in your instance URL (`CONNECTWISE_CONTROL_BASE_URL`), user name and password when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-control/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-control/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "connectwise-control": {
      "command": "connectwise-control-mcp",
      "env": {
        "CONNECTWISE_CONTROL_BASE_URL": "<your-instance-url>",
        "CONNECTWISE_CONTROL_USERNAME": "<your-user-name>",
        "CONNECTWISE_CONTROL_PASSWORD": "<your-password>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`connectwise-control-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `CONNECTWISE_CONTROL_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `CONNECTWISE_CONTROL_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export CONNECTWISE_CONTROL_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
connectwise-control-mcp --transport http
```

Set `CONNECTWISE_CONTROL_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Auth is HTTP Basic with an instance user name and password against your instance base URL. Set CONNECTWISE_CONTROL_BASE_URL, CONNECTWISE_CONTROL_USERNAME and CONNECTWISE_CONTROL_PASSWORD, then run `connectwise-control-cli doctor` to confirm the login works.

## Quick Start

```bash
# confirm the base URL and login work
connectwise-control-cli doctor

# list the session groups on the instance
connectwise-control-cli session-groups --json

# list access sessions live
connectwise-control-cli sessions list --session-type Access --json

# find access agents that have not connected in 30 days
connectwise-control-cli sessions stale --session-type Access --days 30

# review every instance user
connectwise-control-cli security access-review --json

```

## Unique Features

### Instance hygiene
- **`sessions stale`**  -  List sessions whose guest has not connected for at least --days (default 30), oldest first, plus sessions with no recorded connection.

  _Use it to find dead access agents before a license true-up or a client offboarding._

  ```bash
  connectwise-control-cli sessions stale --session-type Access --days 30 --agent
  ```
- **`security access-review`**  -  Flag instance users that are disabled, have an expired password, or belong to no security group, with each user's groups listed.

  _Use it for a quarterly access review of who can open remote sessions on customer machines._

  ```bash
  connectwise-control-cli security access-review --agent
  ```

## Recipes

### Find dark access agents

```bash
connectwise-control-cli sessions stale --session-type Access --days 60 --agent
```

Lists access sessions whose guest has not connected in 60 days, oldest first.

### Quarterly user access review

```bash
connectwise-control-cli security access-review --agent
```

Flags disabled users, expired passwords and users in no security group.

## Usage

Run `connectwise-control-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `CONNECTWISE_CONTROL_CONFIG_DIR`, `CONNECTWISE_CONTROL_DATA_DIR`, `CONNECTWISE_CONTROL_STATE_DIR`, or `CONNECTWISE_CONTROL_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `CONNECTWISE_CONTROL_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export CONNECTWISE_CONTROL_HOME=/srv/connectwise-control
connectwise-control-cli doctor
```

On Windows (PowerShell):

```powershell
$env:CONNECTWISE_CONTROL_HOME = "C:\ProgramData\connectwise-control"
connectwise-control-cli doctor
```

Under `CONNECTWISE_CONTROL_HOME=/srv/connectwise-control`, the four dirs resolve to `/srv/connectwise-control/config`, `/srv/connectwise-control/data`, `/srv/connectwise-control/state`, and `/srv/connectwise-control/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:CONNECTWISE_CONTROL_HOME = "C:\srv\connectwise-control"
connectwise-control-cli doctor
```

Under `$env:CONNECTWISE_CONTROL_HOME = "C:\srv\connectwise-control"`, the four dirs resolve to `C:\srv\connectwise-control\config`, `C:\srv\connectwise-control\data`, `C:\srv\connectwise-control\state`, and `C:\srv\connectwise-control\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "connectwise-control": {
      "command": "connectwise-control-mcp",
      "env": {
        "CONNECTWISE_CONTROL_HOME": "/srv/connectwise-control"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `CONNECTWISE_CONTROL_DATA_DIR` overrides an explicit `--home` for that kind. Use `CONNECTWISE_CONTROL_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `CONNECTWISE_CONTROL_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `connectwise-control-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `connectwise-control-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `connectwise-control-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `CONNECTWISE_CONTROL_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `CONNECTWISE_CONTROL_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

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


### security


- **`connectwise-control-cli security access-review`** - Flag instance users that are disabled, have an expired password, or belong to no security group, with each user's groups listed.


### sessions


- **`connectwise-control-cli sessions stale`** - List sessions whose guest has not connected for at least --days (default 30), oldest first, plus sessions with no recorded connection.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`connectwise-control-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`connectwise-control-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`connectwise-control-cli learnings list`** - Inspect taught rows
- **`connectwise-control-cli learnings forget <query>`** - Undo a teach
- **`connectwise-control-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`connectwise-control-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`connectwise-control-cli teach-pattern`** - Install a query/resource template up front
- **`connectwise-control-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `CONNECTWISE_CONTROL_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `connectwise-control-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
connectwise-control-cli session-groups

# JSON for scripting and agents
connectwise-control-cli session-groups --json
# Filter to specific fields
connectwise-control-cli session-groups --json --select name,sessionCount,sessionType

# Dry run  -  show the request without sending
connectwise-control-cli session-groups --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
connectwise-control-cli session-groups --agent
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
connectwise-control-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `connectwise-control-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/connectwise-control-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\connectwise-control-cli\config.toml` on Windows; `--home`, `CONNECTWISE_CONTROL_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `CONNECTWISE_CONTROL_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `CONNECTWISE_CONTROL_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `CONNECTWISE_CONTROL_BASE_URL` | config | Yes | Your instance base URL, for example `https://company.screenconnect.com`. |
| `CONNECTWISE_CONTROL_USERNAME` | per_call | Yes | Your instance user name (HTTP Basic auth). |
| `CONNECTWISE_CONTROL_PASSWORD` | per_call | Yes | Your instance password (HTTP Basic auth). |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `connectwise-control-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- The HTTP Basic credentials (`CONNECTWISE_CONTROL_USERNAME`, `CONNECTWISE_CONTROL_PASSWORD`) and every API request go only to the scheme, host and port of `CONNECTWISE_CONTROL_BASE_URL`, your instance (for example `https://company.screenconnect.com`). A redirect to any other host is refused, so the credentials never reach it.
- Any output, file, local-store write or MCP result that would repeat a credential this program holds (the user name, the password, the Basic header, the MCP HTTP token, a session `accessToken` or the join `url` that repeats it) is withheld whole: a "[credkit: withheld from ...]" notice stands in its place, in any encoding the shared credential kit reads. Responses are never cached on disk.
- `sessions get-access-token` issues a one-time token that grants remote control of a guest machine. The token, and the join URL that carries it, are withheld from every output, so the command cannot hand a join link to a script or an agent: join the session from the instance web app.
- Only `credentials.toml` (mode 0600) holds a credential. `auth set-credentials` takes the user name and password as arguments, which your shell may keep in its history; the environment variables avoid that.
- `--dry-run` sends nothing and writes no local file. Over MCP, a tool that changes something (runs a command on a guest, queues a session event, renames or tags a session, issues an access token, saves or deletes an instance user) returns a preview and acts only with `confirm: true`, and a tool argument can never name a file to read or write.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `connectwise-control-cli doctor` to check credentials
- Verify the environment variable is set: `echo $CONNECTWISE_CONTROL_USERNAME`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Check CONNECTWISE_CONTROL_USERNAME and CONNECTWISE_CONTROL_PASSWORD against the web console login, then run doctor again.
- **doctor says the base URL is still the placeholder**  -  Set CONNECTWISE_CONTROL_BASE_URL to your own instance, for example https://company.screenconnect.com.
- **A write is refused with an anti-forgery error**  -  Mutating calls need the instance x-anti-forgery-token header; reads work with Basic auth alone.
