# Better Stack CLI

**Every Better Stack Uptime feature, plus an offline SQLite mirror and cross-resource fleet analytics  -  what's down and who's paged, coverage gaps, MTTA/MTTR, flapping, on-call gaps, and status-page drift  -  that the API alone can't answer.**

betterstack-cli mirrors your whole Better Stack Uptime account (monitors, heartbeats, incidents, on-call, escalation policies, status pages) into a local SQLite store, then answers operational questions no single API call can: `coverage` finds monitors that won't page anyone, `mttr` rolls up incident response times, `flapping` ranks your noisiest monitors, and `fleet` shows the whole account on one screen. Covers the official Terraform provider's resource surface (full create/update/delete on monitors and heartbeats; create/delete on groups, policies, and status pages), with agent-native output and typed exit codes throughout.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `betterstack-cli` and `betterstack-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.ps1 | iex
   ```
3. Verify: `betterstack-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=betterstack). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/betterstack --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/betterstack --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `betterstack-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the betterstack skill from https://github.com/Servosity/msp-skills/tree/main/skills/betterstack. The skill defines how its required CLI (`betterstack-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=betterstack).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `BETTERSTACK_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/betterstack/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "betterstack": {
      "command": "betterstack-mcp",
      "env": {
        "BETTERSTACK_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`betterstack-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `BETTERSTACK_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `BETTERSTACK_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export BETTERSTACK_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
betterstack-mcp --transport http
```

Set `BETTERSTACK_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Auth is a Better Stack Uptime API token (Authorization: Bearer). Create one under Better Stack → Settings → API tokens, then set BETTERSTACK_API_TOKEN. Run `betterstack-cli doctor` to confirm the token and reachability.

## Quick Start

```bash
# confirm the token works and the API is reachable
betterstack-cli doctor

# mirror the account into the local SQLite store
betterstack-cli sync

# see monitors, heartbeats, open incidents, and on-call at a glance
betterstack-cli fleet

# find monitors with no escalation policy or alert channel
betterstack-cli coverage

# page through monitors live, agent-friendly JSON
betterstack-cli monitors list --per-page 250 --json

```

## Unique Features

### Cross-resource analytics the API can't answer
- **`fleet`**  -  One-screen health of the whole Better Stack account: monitors up/down/paused, heartbeats, open incidents, and who's on call now.

  _Reach for this first when an agent needs the operational state of the entire account in one call instead of five._

  ```bash
  betterstack-cli fleet --agent
  ```
- **`coverage`**  -  Find monitors with no escalation policy or no alert channel  -  the ones that will go down silently and page no one.

  _Use before an incident to prove every monitor actually escalates to a human._

  ```bash
  betterstack-cli coverage --agent
  ```
- **`down`**  -  Focused triage list of monitors currently down or degraded, joined to their open incidents and whether anyone is actually paged for each.

  _Reach for this at the start of a shift or during an outage to see what is down and who is (or is not) being paged, in one call._

  ```bash
  betterstack-cli down --agent
  ```
- **`triage`**  -  Open incidents ranked by age and acknowledgement state  -  never-acknowledged first  -  joined to the affected monitor.

  _Use this to prioritize incident response: it surfaces the open incidents nobody has acknowledged yet, oldest first._

  ```bash
  betterstack-cli triage --agent
  ```
- **`statuspage-audit`**  -  Flags status pages showing operational while a backing monitor has an open incident, and status-page resources pointing at missing or paused monitors.

  _Use this before or during an incident to catch public status pages that are silently out of sync with reality._

  ```bash
  betterstack-cli statuspage-audit --agent
  ```
- **`group-health`**  -  Per-group health rollup: monitor and heartbeat up/down counts plus open incidents for every monitor group and heartbeat group.

  _For MSP-style accounts where one group is one client, this answers per-client health in a single call._

  ```bash
  betterstack-cli group-health --agent
  ```

### Incident intelligence
- **`mttr`**  -  Mean time to acknowledge and resolve, computed across incidents over a window and broken down by monitor.

  _Use for on-call retros and SLA reporting without exporting to a spreadsheet._

  ```bash
  betterstack-cli mttr --days 30 --agent
  ```
- **`flapping`**  -  Rank monitors by how many incidents they generated in a window to surface the noisy, flapping, or misconfigured ones.

  _Use to find alert fatigue sources before tuning thresholds._

  ```bash
  betterstack-cli flapping --days 7 --top 10 --agent
  ```

### On-call and resilience
- **`oncall-gaps`**  -  Detect on-call calendars with nobody currently on call.

  _Use to confirm someone is actually reachable on every rotation before relying on paging._

  ```bash
  betterstack-cli oncall-gaps --agent
  ```
- **`heartbeat-risk`**  -  Rank heartbeats by risk: tight period+grace windows, paused-but-expected, and non-up status.

  _Use to catch fragile cron/scheduled-task check-ins before they false-alarm or silently miss._

  ```bash
  betterstack-cli heartbeat-risk --agent
  ```

## Recipes

### Find unprotected monitors

```bash
betterstack-cli coverage --agent
```

Lists monitors with no escalation policy or alert channel so you can fix paging gaps before an outage.

### 30-day incident response report

```bash
betterstack-cli mttr --days 30 --agent
```

Computes MTTA and MTTR across the window from the local mirror  -  no spreadsheet export needed.

### Narrow a verbose monitor payload

```bash
betterstack-cli monitors list --per-page 250 --agent --select data.id,data.attributes.pronounceable_name,data.attributes.status
```

Uses --select dotted paths to pull only id, name, and status from the JSON:API envelope so agents don't burn context on full monitor objects.

### Acknowledge an incident safely

```bash
betterstack-cli incidents acknowledge 12345 --by oncall@example.com --dry-run
```

Shows the exact request without sending; drop --dry-run to actually acknowledge.

### Rank the noisiest monitors

```bash
betterstack-cli flapping --days 7 --top 10 --agent
```

Surfaces the monitors generating the most incidents so you can tune alert thresholds.

## Usage

Run `betterstack-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `BETTERSTACK_CONFIG_DIR`, `BETTERSTACK_DATA_DIR`, `BETTERSTACK_STATE_DIR`, or `BETTERSTACK_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `BETTERSTACK_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export BETTERSTACK_HOME=/srv/betterstack
betterstack-cli doctor
```

On Windows (PowerShell):

```powershell
$env:BETTERSTACK_HOME = "C:\ProgramData\betterstack"
betterstack-cli doctor
```

Under `BETTERSTACK_HOME=/srv/betterstack`, the four dirs resolve to `/srv/betterstack/config`, `/srv/betterstack/data`, `/srv/betterstack/state`, and `/srv/betterstack/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:BETTERSTACK_HOME = "C:\srv\betterstack"
betterstack-cli doctor
```

Under `$env:BETTERSTACK_HOME = "C:\srv\betterstack"`, the four dirs resolve to `C:\srv\betterstack\config`, `C:\srv\betterstack\data`, `C:\srv\betterstack\state`, and `C:\srv\betterstack\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "betterstack": {
      "command": "betterstack-mcp",
      "env": {
        "BETTERSTACK_HOME": "/srv/betterstack"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `BETTERSTACK_DATA_DIR` overrides an explicit `--home` for that kind. Use `BETTERSTACK_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `BETTERSTACK_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `betterstack-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `betterstack-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `betterstack-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `BETTERSTACK_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `BETTERSTACK_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### heartbeat-groups

Heartbeat groups

- **`betterstack-cli heartbeat-groups create`** - Create a heartbeat group
- **`betterstack-cli heartbeat-groups delete`** - Delete a heartbeat group
- **`betterstack-cli heartbeat-groups get`** - Get a heartbeat group by ID
- **`betterstack-cli heartbeat-groups list`** - List heartbeat groups

### heartbeats

Heartbeats  -  cron/scheduled-task check-ins

- **`betterstack-cli heartbeats create`** - Create a heartbeat
- **`betterstack-cli heartbeats delete`** - Delete a heartbeat
- **`betterstack-cli heartbeats get`** - Get a heartbeat by ID
- **`betterstack-cli heartbeats list`** - List heartbeats
- **`betterstack-cli heartbeats update`** - Update a heartbeat (sends PATCH)

### incidents

Incidents  -  outages and alerts across monitors and heartbeats

- **`betterstack-cli incidents acknowledge`** - Acknowledge an incident
- **`betterstack-cli incidents delete`** - Delete an incident
- **`betterstack-cli incidents get`** - Get an incident by ID
- **`betterstack-cli incidents list`** - List incidents
- **`betterstack-cli incidents resolve`** - Resolve an incident

### monitor-groups

Monitor groups

- **`betterstack-cli monitor-groups create`** - Create a monitor group
- **`betterstack-cli monitor-groups delete`** - Delete a monitor group
- **`betterstack-cli monitor-groups get`** - Get a monitor group by ID
- **`betterstack-cli monitor-groups list`** - List monitor groups

### monitors

Uptime monitors (HTTP, keyword, ping, TCP, heartbeat-backed)

- **`betterstack-cli monitors create`** - Create a monitor
- **`betterstack-cli monitors delete`** - Delete a monitor
- **`betterstack-cli monitors get`** - Get a monitor by ID
- **`betterstack-cli monitors list`** - List all monitors
- **`betterstack-cli monitors update`** - Update a monitor (sends PATCH)

### on-calls

On-call calendars and current on-call shifts

- **`betterstack-cli on-calls get`** - Get an on-call calendar by ID (includes current on-call users)
- **`betterstack-cli on-calls list`** - List on-call calendars

### policies

Escalation policies

- **`betterstack-cli policies create`** - Create an escalation policy
- **`betterstack-cli policies delete`** - Delete an escalation policy
- **`betterstack-cli policies get`** - Get an escalation policy by ID
- **`betterstack-cli policies list`** - List escalation policies

### status-page-resources

Resources (monitored components) shown on a status page

- **`betterstack-cli status-page-resources delete`** - Remove a resource from a status page
- **`betterstack-cli status-page-resources get`** - Get a status page resource
- **`betterstack-cli status-page-resources list`** - List resources on a status page

### status-page-sections

Sections within a status page

- **`betterstack-cli status-page-sections create`** - Create a status page section
- **`betterstack-cli status-page-sections delete`** - Delete a status page section
- **`betterstack-cli status-page-sections get`** - Get a status page section
- **`betterstack-cli status-page-sections list`** - List sections of a status page

### status-pages

Public status pages

- **`betterstack-cli status-pages create`** - Create a status page
- **`betterstack-cli status-pages delete`** - Delete a status page
- **`betterstack-cli status-pages get`** - Get a status page by ID
- **`betterstack-cli status-pages list`** - List status pages


### coverage


- **`betterstack-cli coverage`** - Find monitors with no escalation policy or no alert channel  -  the ones that will go down silently and page no one.


### down


- **`betterstack-cli down`** - Focused triage list of monitors currently down or degraded, joined to their open incidents and whether anyone is actually paged for each.


### flapping


- **`betterstack-cli flapping`** - Rank monitors by how many incidents they generated in a window to surface the noisy, flapping, or misconfigured ones.


### fleet


- **`betterstack-cli fleet`** - One-screen health of the whole Better Stack account: monitors up/down/paused, heartbeats, open incidents, and who's on call now.


### group-health


- **`betterstack-cli group-health`** - Per-group health rollup: monitor and heartbeat up/down counts plus open incidents for every monitor group and heartbeat group.


### heartbeat-risk


- **`betterstack-cli heartbeat-risk`** - Rank heartbeats by risk: tight period+grace windows, paused-but-expected, and non-up status.


### mttr


- **`betterstack-cli mttr`** - Mean time to acknowledge and resolve, computed across incidents over a window and broken down by monitor.


### oncall-gaps


- **`betterstack-cli oncall-gaps`** - Detect on-call calendars with nobody currently on call.


### statuspage-audit


- **`betterstack-cli statuspage-audit`** - Flags status pages showing operational while a backing monitor has an open incident, and status-page resources pointing at missing or paused monitors.


### triage


- **`betterstack-cli triage`** - Open incidents ranked by age and acknowledgement state  -  never-acknowledged first  -  joined to the affected monitor.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`betterstack-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`betterstack-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`betterstack-cli learnings list`** - Inspect taught rows
- **`betterstack-cli learnings forget <query>`** - Undo a teach
- **`betterstack-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`betterstack-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`betterstack-cli teach-pattern`** - Install a query/resource template up front
- **`betterstack-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `BETTERSTACK_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `betterstack-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
betterstack-cli heartbeat-groups list

# JSON for scripting and agents
betterstack-cli heartbeat-groups list --json
# Filter to specific fields
betterstack-cli heartbeat-groups list --json --select id,name,sort_index

# Dry run  -  show the request without sending
betterstack-cli heartbeat-groups list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
betterstack-cli heartbeat-groups list --agent
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

## Freshness

This CLI owns bounded freshness for registered store-backed read command paths. In `--data-source auto` mode, covered commands check the local SQLite store before serving results; stale or missing resources trigger a bounded refresh, and refresh failures fall back to the existing local data with a warning. `--data-source local` never refreshes, and `--data-source live` reads the API without mutating the local store.

Set `BETTERSTACK_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths:
- `betterstack-cli coverage`
- `betterstack-cli down`
- `betterstack-cli flapping`
- `betterstack-cli fleet`
- `betterstack-cli group-health`
- `betterstack-cli heartbeat-groups`
- `betterstack-cli heartbeat-groups get`
- `betterstack-cli heartbeat-groups list`
- `betterstack-cli heartbeat-risk`
- `betterstack-cli heartbeats`
- `betterstack-cli heartbeats get`
- `betterstack-cli heartbeats list`
- `betterstack-cli incidents`
- `betterstack-cli incidents get`
- `betterstack-cli incidents list`
- `betterstack-cli monitor-groups`
- `betterstack-cli monitor-groups get`
- `betterstack-cli monitor-groups list`
- `betterstack-cli monitors`
- `betterstack-cli monitors get`
- `betterstack-cli monitors list`
- `betterstack-cli mttr`
- `betterstack-cli on-calls`
- `betterstack-cli on-calls get`
- `betterstack-cli on-calls list`
- `betterstack-cli oncall-gaps`
- `betterstack-cli policies`
- `betterstack-cli policies get`
- `betterstack-cli policies list`
- `betterstack-cli status-pages`
- `betterstack-cli status-pages get`
- `betterstack-cli status-pages list`
- `betterstack-cli statuspage-audit`
- `betterstack-cli triage`

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
betterstack-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `betterstack-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/betterstack-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\betterstack-cli\config.toml` on Windows; `--home`, `BETTERSTACK_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `BETTERSTACK_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `BETTERSTACK_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `BETTERSTACK_API_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `betterstack-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- The bearer token (`Authorization: Bearer <token>`) and every API request go only to the scheme, host and port of the configured base URL (`https://uptime.betterstack.com/api/v2`, or `BETTERSTACK_BASE_URL`). A redirect to any other host is refused, so the token never reaches it.
- Any output, file or MCP result that would repeat `BETTERSTACK_API_TOKEN`, the stored token, or a credential-named value from a response (an escalation policy's `incident_token`, a monitor's `auth_password`) prints "[text withheld: it repeated a secret value this program holds]" in its place, in any encoding. Responses are never cached on disk, and credential-named fields are dropped before anything reaches the local store.
- Only `credentials.toml` (mode 0600) holds a credential; `auth set-token` reads the token from stdin, never from an argument.
- `--dry-run` sends nothing. Over MCP, a tool that changes something in Better Stack (every create, update and delete tool, `incidents_acknowledge`, `incidents_resolve`, `import`) returns a preview and sends only with `confirm: true`, and a tool argument can never name a file to read or write.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `betterstack-cli doctor` to check credentials
- Verify the environment variable is set: `echo $BETTERSTACK_API_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Set BETTERSTACK_API_TOKEN to a valid Uptime API token (Settings → API tokens), then re-run `doctor`.
- **fleet/coverage/mttr return empty or 'run sync first'**  -  Run `betterstack-cli sync` to populate the local mirror before querying analytics.
- **List command only shows 50 rows**  -  Pass `--per-page 250` (the API max) or `--page N`; the `sync` command walks every page automatically.
- **A report says the last sync did not finish, or that a resource was never synced**  -  The local store holds only part of that resource, so the report refuses to answer from it. Run the sync command the message prints (it names the resource), then run the report again.
