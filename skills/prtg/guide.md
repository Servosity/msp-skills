# PRTG Network Monitor CLI

**Every PRTG API v2 endpoint, plus a fleet view across every client's PRTG core: alarms, status, handoffs, hygiene and safe bulk actions in one command.**

Register each client's PRTG core once with 'core add', then run 'fleet alarms', 'fleet status', 'fleet handoff' or 'fleet drift' to see every core at once, and 'fleet ack', 'fleet pause' and 'fleet resume' to act across them with a preview first. Every v2 operation is also a typed command you can point at one core with --core.

Learn more at [PRTG Network Monitor](https://www.paessler.com/prtg).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `prtg-cli` and `prtg-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/prtg/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/prtg/install.ps1 | iex
   ```
3. Verify: `prtg-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=prtg). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine prtg-cli`. On Unix, mark it executable: `chmod +x prtg-cli`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/prtg --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/prtg --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `prtg-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the prtg skill from https://github.com/Servosity/msp-skills/tree/main/skills/prtg. The skill defines how its required CLI (`prtg-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=prtg).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `PRTG_SERVER` (your core's host:port, e.g. `prtg.acme.example:1616`; the default `prtg.example.com` does not resolve) and `PRTG_API_KEY` when Claude Desktop prompts you. Add `PRTG_SKIP_TLS_VERIFY=true` only for a core with a self-signed certificate.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/prtg/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/prtg/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "prtg": {
      "command": "prtg-mcp",
      "env": {
        "PRTG_SERVER": "prtg.acme.example:1616",
        "PRTG_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`prtg-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `PRTG_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `PRTG_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export PRTG_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
prtg-mcp --transport http
```

Set `PRTG_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Each PRTG core has its own API key (PRTG: Account Settings > My Account > API Keys), sent as 'Authorization: Bearer <key>'. For one core, set PRTG_SERVER to its host[:port] (the PRTG application server listens on 1616 by default) and PRTG_API_KEY to the key. Fleet commands only see cores registered with 'core add <name> --server <host[:port]>'; put every core's key in PRTG_FLEET_KEYS as name=key pairs separated by commas (acme=KEY1,globex=KEY2), and keys are never written to disk. Check registered cores with 'core list' (key_set must be true) and 'doctor --core <name>'. A registered core with a self-signed certificate takes 'core add <name> --insecure-tls' (that core only); for single-core use pass --insecure or PRTG_SKIP_TLS_VERIFY=true.

## Quick Start

```bash
# Confirm the CLI is installed; prints a one-line dry-run notice and contacts nothing.
prtg-cli doctor --dry-run

# Register a client's core, then add acme=<its API key> to PRTG_FLEET_KEYS (fleet commands only see registered cores).
prtg-cli core add acme --server prtg.acme.example:1616

# One row per core with reachability and sensor counts by state.
prtg-cli fleet status --agent

# Every down sensor across all cores, highest priority first, then oldest.
prtg-cli fleet alarms --status down --agent

# Drill into one core with any v2 endpoint and the PRTG filter language.
prtg-cli sensors list-experimental --core acme --filter 'status = down' --agent

```

## Managing many cores

Fleet commands read every core registered with `core add`; `PRTG_SERVER` and `PRTG_API_KEY`
alone configure single-core commands only.

```bash
prtg-cli core add acme --server prtg.acme.example:1616 --tag onprem,gold
prtg-cli core add globex --server 10.20.0.5:1616 --insecure-tls   # self-signed certificate, this core only
export PRTG_FLEET_KEYS='acme=...,globex=...'   # name=key pairs; keys are never written to disk
# PowerShell: $env:PRTG_FLEET_KEYS = "acme=...,globex=..."
prtg-cli core list           # key_set must be true
prtg-cli fleet alarms --cores acme,globex --concurrency 4 --agent
prtg-cli fleet status --tag gold --agent
prtg-cli sensors list-experimental --core acme --agent
prtg-cli core remove acme
```

`fleet ack`, `fleet pause` and `fleet resume` only preview (rows read `would acknowledge`,
`would pause`) until you add the global `--yes`:

```bash
prtg-cli fleet ack --filter 'name = "Ping"' --message 'TKT-4411' --duration 2h --cores acme,globex --yes
```

Fleet commands exit 0 when some cores fail (those rows carry `error`); they exit non-zero
when every core in scope fails or a sent action fails on any core. Read each row's
`error`, `action` and `failed` fields rather than relying on the exit code alone.

### Known limitations

- `fleet alarms`, `fleet handoff`, `fleet silenced` and `fleet drift` read PRTG's typed list
  endpoints (`/sensors/alarms`, `/sensors`, `/devices`, `/groups`, `/probes`). Paessler marks them
  deprecated in favour of `/experimental/*` but still serves them with no removal version; they
  also work on cores older than 24.3. `sync` reads the same lists.
- The fleet commands were verified against mock cores, not a live PRTG installation.
- PRTG API v2 is not available on PRTG clusters; Hosted Monitor support varies by version.

## Unique Features

### One view across every core
- **`fleet alarms`**  -  See every down, warning and unusual sensor across all of your clients' PRTG cores in one list: worst state first (down, warning, unusual, acknowledged), then higher priority, then oldest.

  _Reach for this at the start of a shift or when asked what is broken across clients._

  ```bash
  prtg-cli fleet alarms --status down --unacked --agent
  ```
- **`fleet status`**  -  One row per client core with reachability and sensor counts by state, worst first.

  _Use it for a quick health glance across every client before drilling into one._

  ```bash
  prtg-cli fleet status --agent
  ```
- **`fleet handoff`**  -  What changed across all cores since a point in time: new alarms, recoveries, and new acknowledgements or pauses with who and why.

  _Use it for shift handoffs and 'what happened overnight' questions._

  ```bash
  prtg-cli fleet handoff --since 12h --agent
  ```
- **`fleet silenced`**  -  Everything someone paused or acknowledged across all cores, with who, when, until when and why, flagging indefinite ones left too long.

  _Use it after maintenance windows and in weekly hygiene reviews to catch muted outages._

  ```bash
  prtg-cli fleet silenced --indefinite --older-than 7d --agent
  ```
- **`fleet exec`**  -  Run any read-only API v2 query on every core and get one merged table with a core column.

  _Use it for cross-client questions the dedicated fleet commands do not cover._

  ```bash
  prtg-cli fleet exec /experimental/probes --agent
  ```

### Estate hygiene
- **`fleet drift`**  -  PRTG estate hygiene across cores: versions behind your newest core, expiring licenses or maintenance, sensor overuse, disconnected probes, rejected API keys and unreachable servers.

  _Use it for the weekly estate review instead of a hand-kept spreadsheet._

  ```bash
  prtg-cli fleet drift --within 30d --agent
  ```
- **`fleet sync`**  -  Refresh the offline mirror of every registered core in one run, each into its own isolated store.

  _Run it before offline searches or analytics across clients, or nightly from cron._

  ```bash
  prtg-cli fleet sync --resources sensors,devices --agent
  ```

### Safe actions across cores
- **`fleet ack`**  -  Acknowledge matching down sensors on every core at once with a ticket message; previews per core and sends only with --yes (the API key needs the Acknowledge access level).

  _Use it to silence a known incident that spans several clients without opening each core._

  ```bash
  prtg-cli fleet ack --filter 'name = "Ping"' --message 'TKT-4411 ISP outage' --duration 2h
  ```
- **`fleet pause`**  -  Pause matching devices, groups, sensors or probes on every core for a maintenance window; previews per core and sends only with --yes (Write access level).

  _Use it before multi-client patch nights instead of pausing in every core's web UI._

  ```bash
  prtg-cli fleet pause --kind device --filter 'name = "fw01"' --duration 2h --message 'Patch night'
  ```
- **`fleet resume`**  -  Resume matching objects on every core after maintenance and optionally trigger an immediate scan; previews per core and sends only with --yes (Write access level).

  _Use it to close a maintenance window across clients and confirm everything comes back green._

  ```bash
  prtg-cli fleet resume --kind device --filter 'name = "fw01"' --scan
  ```

## Recipes

### Shift-start sweep

```bash
prtg-cli fleet alarms --status down,warning --unacked --agent
```

Unacknowledged down and warning sensors across every client, highest priority and oldest first.

### Narrow a large sensor list

```bash
prtg-cli sensors list-experimental --core acme --filter 'status = down' --agent --select id,name,status,status_since
```

Keep only the fields an agent needs from one core's sensor list.

### Overnight handoff

```bash
prtg-cli fleet handoff --since 12h --agent
```

New alarms, recoveries and new acknowledgements or pauses across cores in the last 12 hours.

### Forgotten pauses

```bash
prtg-cli fleet silenced --indefinite --older-than 7d --agent
```

Objects paused or acknowledged with no end date for more than a week, with who did it.

### Weekly estate review

```bash
prtg-cli fleet drift --within 30d --agent
```

Version skew, licenses and maintenance ending within 30 days, sensor overuse and disconnected probes.

## Usage

Run `prtg-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml`, saved profiles, `fleet.json` (registered cores) and per-core homes under `cores/<name>/` |
| `data` | Durable local data: `credentials.toml`, `data.db`, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `PRTG_CONFIG_DIR`, `PRTG_DATA_DIR`, `PRTG_STATE_DIR`, or `PRTG_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `PRTG_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export PRTG_HOME=/srv/prtg
prtg-cli doctor
```

On Windows (PowerShell):

```powershell
$env:PRTG_HOME = "C:\ProgramData\prtg"
prtg-cli doctor
```

Under `PRTG_HOME=/srv/prtg`, the four dirs resolve to `/srv/prtg/config`, `/srv/prtg/data`, `/srv/prtg/state`, and `/srv/prtg/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:PRTG_HOME = "C:\srv\prtg"
prtg-cli doctor
```

Under `$env:PRTG_HOME = "C:\srv\prtg"`, the four dirs resolve to `C:\srv\prtg\config`, `C:\srv\prtg\data`, `C:\srv\prtg\state`, and `C:\srv\prtg\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "prtg": {
      "command": "prtg-mcp",
      "env": {
        "PRTG_HOME": "/srv/prtg"
      }
    }
  }
}
```

Precedence matters across machines: an ambient per-kind variable such as `PRTG_DATA_DIR` overrides an explicit `--home` for that kind. Use `PRTG_HOME` or the per-kind variables for durable relocation; treat `--home` as the weaker per-invocation lever. With `--core` (and inside fleet commands), `--home` and `--config` are rejected and other `PRTG_*` variables are dropped; each core uses its own home under `<config dir>/cores/<name>/`.

Relocation is one-way. Unsetting `PRTG_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `prtg-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `prtg-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `prtg-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `PRTG_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `PRTG_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### ai

Manage ai

- **`prtg-cli ai aistatus`** - Returns whether the PRTG AI Agent service is registered with the Windows Service Control Manager, whether it is running, and whether the agent is connected to PRTG.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).

This endpoint is disabled by default. Enable it with the `aianalyzeendpoint` feature toggle. While disabled, the endpoint stays visible in this reference but returns `403 FEATURE_NOT_ENABLED` for every request.

The answer comes from the Windows Service Control Manager, not from the PRTG AI Agent. Nothing is sent to the agent, because a question you have to ask the agent cannot tell you the agent is missing.

The response carries `registration`, `state` and `connected` together, in one round trip: a service that is not installed also reports `state: stopped` here, and `registration` is what tells that apart from an installed-but-stopped service.

`state` comes from the Windows Service Control Manager and `connected` from the PRTG side of the transport. They answer different questions: a service can be running while nothing is attached, which is what a crash-looping or blocked agent looks like.

The PRTG AI Agent ships only as a Windows service, so this endpoint is unsupported on every other platform.
- **`prtg-cli ai analyze-aiincidents`** - Analyzes the currently active alarms with AI and returns potential incidents.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).

This endpoint is enabled by default. Disable it with the `aianalyzeendpoint` feature toggle. While disabled, the endpoint stays visible in this reference but returns `403 FEATURE_NOT_ENABLED` for every request.

### autodiscoveries

Manage autodiscoveries

- **`prtg-cli autodiscoveries`** - Returns a list of all running auto-discoveries.

### autodiscoverytasks

Manage autodiscoverytasks

- **`prtg-cli autodiscoverytasks delete-auto-discovery-task`** - Deletes a given auto-discovery task

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli autodiscoverytasks get-auto-discovery-task`** - Returns auto-discovery task by the given task ID

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli autodiscoverytasks list-auto-discovery-task-overview`** - Returns a overview list of all auto-discovery tasks.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli autodiscoverytasks modify-auto-discovery-task`** - Patch a auto-discovery task

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli autodiscoverytasks start-auto-discovery-task`** - Starts a auto-discovery task by the given task ID

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).

### channels

Manage channels

- **`prtg-cli channels channels`** - Returns a list of channels.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli channels get`** - Returns the metrics and settings of a channel.
- **`prtg-cli channels list`** - Returns a list of channels.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be replaced with the '/experimental/channels' endpoint.
- **`prtg-cli channels list-measurements`** - Get a list of the last measurements with references to the corresponding channels.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be removed.

### core

Register the PRTG core servers you manage (one per client) for `--core` and the fleet commands.

- **`prtg-cli core add <name> --server <host[:port]> [--tag a,b] [--insecure-tls]`** - Register (or update) a core; its API key goes in `PRTG_FLEET_KEYS` as `<name>=<key>`; `--insecure-tls` skips certificate checks for that core only
- **`prtg-cli core list`** - List registered cores and whether each key env var is set (`key_set`)
- **`prtg-cli core remove <name>`** - Remove a core and delete its local mirror and cache

### devices

Manage devices

- **`prtg-cli devices create-sensor`** - Creates a new sensor in a device.

Specify a sensor kind like 'ping', 'http' or 'paessler.snmp.traffic_sensor'. To get a list of all creatable sensor kinds,
call the endpoint /experimental/schemas/{parent} in the Schemas section.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli devices delete`** - Deletes a device with all child sensors and channels.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli devices get`** - Returns the metrics and settings of a device.
- **`prtg-cli devices get-icons`** - Returns a list of device icons.
- **`prtg-cli devices get-templates-experimental`** - Returns a list of device templates.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli devices list`** - Returns a list of devices.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but is replaced with the '/experimental/devices' endpoint.
- **`prtg-cli devices list-experimental`** - Returns a list of devices.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli devices metascan`** - Execute a meta-scan on a given device for a given sensor kind

Some sensors use a metascan to retrieve important information from a device which is relevant for sensor creation, e.g. the available disks on a device.
Specify a sensor kind like 'snmpdiskfree' to perform the metascan. Get a list of all creatable sensor kinds, and whether they have a metascan,
by calling the endpoint /experimental/schemas/{parent} in the Schemas section.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli devices metascan-result`** - Retrieve the meta scan progress/result

This endpoint does not yet support all sensor kinds. Currently only the following sensor kind is supported:
- snmpdiskfree

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli devices multi-pause`** - Pauses all devices that match a filter.
- **`prtg-cli devices multi-resume`** - Resumes all devices that match a filter.
- **`prtg-cli devices multi-scan-now`** - Triggers a scan of all devices that match a filter.
- **`prtg-cli devices trigger-autodiscovery`** - Trigger autodiscovery for a given device

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli devices update`** - Edits a device.

### groups

Manage groups

- **`prtg-cli groups create-device-for`** - Creates a new device in a group.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli groups create-for`** - Creates a new group in a group.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli groups delete`** - Deletes a group with all child groups and devices.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli groups get`** - Returns the metrics and settings of a group.
- **`prtg-cli groups list`** - Get a list of all groups that match a filter.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but is replaced with the '/experimental/groups' endpoint.
- **`prtg-cli groups list-experimental`** - Returns a list of groups.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli groups multi-pause`** - Pauses all groups that match a filter.
- **`prtg-cli groups multi-resume`** - Resumes all groups that match a filter.
- **`prtg-cli groups multi-scan-now`** - Triggers a scan of all groups that match a filter.
- **`prtg-cli groups trigger-autodiscovery`** - Trigger autodiscovery for a given group

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli groups update`** - Edits a group.

### health

Manage health

- **`prtg-cli health`** - Returns the status of the PRTG application server.

### libraries

Manage libraries

- **`prtg-cli libraries`** - Returns a list of all libraries that match a filter.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be removed.

### lookup-definitions

Manage lookup definitions

- **`prtg-cli lookup-definitions get-overview`** - Returns a lookup definition.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be replaced.
- **`prtg-cli lookup-definitions list`** - Returns a list of lookups definitions.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be replaced.

### objects

Manage objects

- **`prtg-cli objects count`** - Returns a list of how many objects of each kind exist.
- **`prtg-cli objects list`** - Get a list of all probes, groups, devices, and sensors that match a filter.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be removed.
- **`prtg-cli objects multi-delete`** - Delete multiple objects based on the specified filter. All children will also be deleted.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli objects objects`** - Returns a list of all probes, groups, devices, sensors, and channels that match a filter.

### probe-network

Manage probe network

- **`prtg-cli probe-network <id>`** - Returns the network information of a probe.

### probes

Manage probes

- **`prtg-cli probes create-auto-discovery-task`** - Creates a new auto-discovery task on a probe

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli probes create-device-for`** - Creates a new device on a probe.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli probes create-group-for`** - Creates a new group in a probe.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli probes delete`** - Deletes a probe with all child groups and devices.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli probes get`** - Returns the metrics and settings of a probe.
- **`prtg-cli probes list`** - Get a list of all probes that match a filter.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but is replaced with the '/experimental/probes' endpoint.
- **`prtg-cli probes list-experimental`** - Returns a list of probes.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli probes multi-pause`** - Pauses all probes that match a filter.
- **`prtg-cli probes multi-resume`** - Resumes all probes that match a filter.
- **`prtg-cli probes multi-scan-now`** - Triggers a scan of all probes that match a filter.
- **`prtg-cli probes update`** - Edits a probe.

### schemas

Manage schemas

- **`prtg-cli schemas get`** - Returns a OAS schema to use in other endpoints.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but is replaced with the following endpoints:
- `/experimental/schemas/{parentid}/post` for creating objects
- `/experimental/schemas/{id}/patch` for updating objects 
- `/experimental/schemas/{id}/get` for reading objects
- **`prtg-cli schemas get-create-by-parent-id`** - Returns an OAS schema for use in `POST` endpoints.
- **`prtg-cli schemas get-kinds-for-parent`** - Returns a list of all object kinds that you can create on a given parent object.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli schemas get-read-by-object-id`** - Returns OAS schema for use in `GET` endpoints for a specific object.
- **`prtg-cli schemas get-update-by-object-id`** - Returns an OAS schema for use in `PATCH` endpoints for a specific object.

### sensor-status-summary

Manage sensor status summary

- **`prtg-cli sensor-status-summary global-sensor-summary`** - Returns a summary of all sensor states.
- **`prtg-cli sensor-status-summary sensor-summary`** - Returns a summary of sensor states for a probe, group, or device.

### sensors

Manage sensors

- **`prtg-cli sensors delete`** - Deletes a sensor with all child channels.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli sensors get`** - Returns metrics and settings of a sensor.
- **`prtg-cli sensors list`** - Returns a list of all sensors that match a filter.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but is replaced with the '/experimental/sensors' endpoint.
- **`prtg-cli sensors list-alarms`** - Returns a list of sensors with alarms that match a filter.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be removed.
- **`prtg-cli sensors list-experimental`** - Returns a list of sensors.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli sensors multi-acknowledge`** - Acknowledges the alarms of all sensors that match a filter.
- **`prtg-cli sensors multi-pause`** - Pauses all sensors that match a filter.
- **`prtg-cli sensors multi-resume`** - Resumes all sensors that match a filter.
- **`prtg-cli sensors multi-scan-now`** - Triggers a scan of all sensors that match a filter.
- **`prtg-cli sensors update`** - Edits a sensor.

This endpoint does not yet support all sensor settings. Currently only the following settings are supported: 

- basic-section: name, priority and tags
- all settings in the section channelgroup

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).

### session

Manage session

- **`prtg-cli session login`** - Creates a new user session.
- **`prtg-cli session logout`** - Ends an active user session.
- **`prtg-cli session renew`** - Renews an active user session.

### setting-lookups

Manage setting lookups

- **`prtg-cli setting-lookups <name>`** - Returns possible setting lookup values.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be replaced.

### settings

Manage settings

- **`prtg-cli settings`** - Returns a list of settings that are available for unauthenticated users.

### system

Manage system

- **`prtg-cli system license-info`** - Returns PRTG license information.
- **`prtg-cli system prtgfeature-flags`** - Returns a list of all feature-flags.
- **`prtg-cli system prtgfeature-toggles`** - Returns a list of all enabled feature-toggles.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be removed.
- **`prtg-cli system version-info`** - Returns information on the PRTG version and appserver version.

### timeseries

Manage timeseries

- **`prtg-cli timeseries graph-set`** - Returns time series data of a sensor for predefined time frames.

This endpoint is experimental. For more information, see the PRTG API v2 Overview: Endpoint Maturity (PRTG API v2 documentation).
- **`prtg-cli timeseries time-series`** - Get time series data

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be removed.

### usergroups

Manage usergroups

- **`prtg-cli usergroups get-user-group`** - Returns the settings of a usergroup.
- **`prtg-cli usergroups get-user-group-overview`** - Return information about a user group.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be replaced.
- **`prtg-cli usergroups list-user-groups`** - Get a list of all user groups that match a filter.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be replaced.
- **`prtg-cli usergroups list-user-groups-osk`** - Returns a list of user groups.

This endpoint is experimental. See Overview: Maturity of endpoints (PRTG API v2 documentation) for more context.

### users

Manage users

- **`prtg-cli users delete-apikey`** - Deletes an API key.
- **`prtg-cli users find-apikey`** - Returns details of an API key.
- **`prtg-cli users get`** - Returns the metrics and settings of a user.
- **`prtg-cli users get-overview`** - Returns information about a user account.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be replaced.
- **`prtg-cli users list`** - Returns a list of all user accounts.

⚠ Deprecated as of PRTG 24.3.100: This endpoint still works, but will be replaced.
- **`prtg-cli users list-osk`** - Returns a list of users.

This endpoint is experimental. See Overview: Maturity of endpoints (PRTG API v2 documentation) for more context.
- **`prtg-cli users modify-apikey`** - Edits an API key.
- **`prtg-cli users multi-pause-request`** - Pauses all user accounts that match a filter.
- **`prtg-cli users multi-resume`** - Resumes all user accounts that match a filter.
- **`prtg-cli users reset-password`** - Set a new password for a user account with the token from the password reset process.

**Note:** This route is not available when using PPHM.
- **`prtg-cli users trigger-password-reset`** - Triggers the password reset process for a user account.

**Note:** This route is not available when using PPHM.


### fleet


- **`prtg-cli fleet alarms`** - See every down, warning and unusual sensor across all of your clients' PRTG cores in one list: worst state first (down, warning, unusual, acknowledged), then higher priority, then oldest.

- **`prtg-cli fleet status`** - One row per client core with reachability and sensor counts by state, worst first.

- **`prtg-cli fleet handoff`** - What changed across all cores since a point in time: new alarms, recoveries, and new acknowledgements or pauses with who and why.

- **`prtg-cli fleet silenced`** - Everything someone paused or acknowledged across all cores, with who, when, until when and why, flagging indefinite ones left too long.

- **`prtg-cli fleet drift`** - PRTG estate hygiene across cores: versions behind your newest core, expiring licenses or maintenance, sensor overuse, disconnected probes, rejected API keys and unreachable servers.

- **`prtg-cli fleet ack`** - Acknowledge matching down sensors on every core at once with a ticket message; previews per core and sends only with --yes (the API key needs the Acknowledge access level).

- **`prtg-cli fleet pause`** - Pause matching devices, groups, sensors or probes on every core for a maintenance window; previews per core and sends only with --yes (Write access level).

- **`prtg-cli fleet resume`** - Resume matching objects on every core after maintenance and optionally trigger an immediate scan; previews per core and sends only with --yes (Write access level).

- **`prtg-cli fleet sync`** - Refresh the offline mirror of every registered core in one run, each into its own isolated store.

- **`prtg-cli fleet exec`** - Run any read-only API v2 query on every core and get one merged table with a core column.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`prtg-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`prtg-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`prtg-cli learnings list`** - Inspect taught rows
- **`prtg-cli learnings forget <query>`** - Undo a teach
- **`prtg-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`prtg-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`prtg-cli teach-pattern`** - Install a query/resource template up front
- **`prtg-cli teach-lookup`** - Add an entity mapping (e.g. a status alias such as red -> DOWN, or a client alias) for pattern substitution

Pass `--no-learn` or set `PRTG_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `prtg-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
prtg-cli autodiscoveries

# JSON for scripting and agents
prtg-cli autodiscoveries --json
# Filter to specific fields
prtg-cli autodiscoveries --json --select active,autodiscovery_type,last_autodiscovery

# Dry run  -  show the request without sending
prtg-cli autodiscoveries --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
prtg-cli autodiscoveries --agent
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

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `8` async job submitted but not finished (rerun the printed recovery command; do not resubmit), `10` config error. Fleet commands exit 0 when some cores fail (those rows carry `error`) and `5` when every core in scope fails or a sent action fails.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `PRTG_SERVER` resolves `{server}` (the PRTG core host[:port])
- `PRTG_BASE_URL` overrides the whole base URL
- `--core <name>` pins both from the core registry (`core add`)

Base URL: `https://{server}/api/v2`

## Health Check

```bash
prtg-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `prtg-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/prtg-cli/config.toml` (the core registry sits beside it in `fleet.json`) on macOS/Linux and `%USERPROFILE%\.config\prtg-cli\config.toml` on Windows; `--home`, `PRTG_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `PRTG_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `PRTG_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `PRTG_SERVER` | endpoint | Yes for single-core use | PRTG core host[:port], e.g. `prtg.acme.example:1616` (not needed with `--core` or fleet commands) |
| `PRTG_BASE_URL` | endpoint | No | Overrides the whole base URL (`https://<server>/api/v2`) |
| `PRTG_CORE` | selector | No | Same as `--core <name>`: run against one registered core |
| `PRTG_FLEET_KEYS` | per_core | For registered cores | API keys for cores registered with `core add`, as `name=key` pairs separated by commas or newlines (`acme=KEY1,globex=KEY2`) |
| `PRTG_SKIP_TLS_VERIFY` | transport | No | `true` skips certificate verification (self-signed cores); same as `--insecure` |
| `PRTG_CONFIG` | path | No | Explicit config file path |
| `PRTG_API_KEY` | per_call | Yes for single-core use | API key for the core in `PRTG_SERVER` (PRTG: Account Settings > My Account > API Keys) |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `prtg-cli doctor` reports `agentcookie: detected (managing credentials)` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `prtg-cli doctor` to check credentials
- Run `prtg-cli auth status` (single core) or `prtg-cli core list` (registered cores; `key_set` must be true) instead of printing the key
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **x509: certificate signed by unknown authority**  -  PRTG cores often use self-signed certificates: register that core with 'prtg-cli core add <name> --server <host:port> --insecure-tls' (applies to that core only). For single-core use pass --insecure or export PRTG_SKIP_TLS_VERIFY=true, which applies to every core a command touches.
- **connection refused or timeout on port 443**  -  API v2 is served by the PRTG application server on port 1616 by default: register the core as host:1616.
- **401 Unauthorized or a critical auth finding in fleet drift**  -  Run 'prtg-cli core list' (key_set must be true) or 'prtg-cli auth status' for a single core, then re-export the key.
- **403 Forbidden on fleet ack, pause or resume**  -  The API key needs the Acknowledge (ack) or Write (pause/resume) access level; create a key with that level in PRTG.
- **fleet commands return no rows**  -  Fleet commands only read cores registered with 'core add'; PRTG_SERVER and PRTG_API_KEY alone configure single-core commands.
- **503 LICENSE_INACTIVE or SERVICE_UNAVAILABLE**  -  The application server cannot reach the core or the license expired; 'fleet drift' lists which cores are affected.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**PrtgAPI**](https://github.com/lordmilko/PrtgAPI)  -  C# (323 stars)
- [**prtg (hutchris)**](https://github.com/hutchris/prtg)  -  Python (20 stars)
- [**prtg-mcp (mjhollh3c)**](https://github.com/mjhollh3c/prtg-mcp)  -  Python
- [**prtg-mcp (WYRE-AI)**](https://github.com/WYRE-AI/prtg-mcp)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
