# ConnectWise Automate CLI

**Every ConnectWise Automate endpoint plus a local SQLite mirror that answers cross-client fleet questions the per-server web UI can't**

ConnectWise Automate's value is locked behind a per-server console built for one-endpoint-at-a-time work. This CLI syncs your whole fleet  -  computers, clients, locations, alerts, and patch history  -  into local SQLite, then answers the questions MSPs actually ask across clients: where the offline agents are (stale-agents), who's behind on patches (patch-compliance), and what changed overnight (since). Everything is offline, scriptable, and agent-native.

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `connectwise-automate-cli` and `connectwise-automate-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-automate/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectwise-automate/install.ps1 | iex
   ```
3. Verify: `connectwise-automate-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=connectwise-automate). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/connectwise-automate --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/connectwise-automate --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `connectwise-automate-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the connectwise-automate skill from https://github.com/Servosity/msp-skills/tree/main/skills/connectwise-automate. The skill defines how its required CLI (`connectwise-automate-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=connectwise-automate).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `CONNECTWISE_AUTOMATE_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary with the install script under Install above, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "connectwise-automate": {
      "command": "connectwise-automate-mcp",
      "env": {
        "CONNECTWISE_AUTOMATE_SERVER": "<server>",
        "CONNECTWISE_AUTOMATE_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`connectwise-automate-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `CONNECTWISE_AUTOMATE_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `CONNECTWISE_AUTOMATE_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export CONNECTWISE_AUTOMATE_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
connectwise-automate-mcp --transport http
```

Set `CONNECTWISE_AUTOMATE_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Automate is per-server: set CONNECTWISE_AUTOMATE_SERVER to your host (e.g. company.hostedrmm.com) and CONNECTWISE_AUTOMATE_CLIENT_ID to your registered integration GUID (required for v2020.11+). Mint a bearer token with `apitoken mint --username <u> --password <p>`: the CLI saves it to its 0600 credential store (as `auth set-token` does) and never prints it, and later commands use it unless CONNECTWISE_AUTOMATE_TOKEN is set. Tokens are short-lived: mint a new one with `apitoken mint`, or run `apitoken refresh --token <current>` when you hold the current token (for example in CONNECTWISE_AUTOMATE_TOKEN); either way the new token is saved the same way.

## Quick Start

```bash
# First export CONNECTWISE_AUTOMATE_SERVER (your host) and CONNECTWISE_AUTOMATE_CLIENT_ID (your integration GUID), then mint a bearer token (saved to the credential store, never printed)
connectwise-automate-cli apitoken mint --username you --password '****'


# Confirm auth + server reachability
connectwise-automate-cli doctor


# Pull the fleet into local SQLite
connectwise-automate-cli sync


# Whole-fleet posture in one roll-up
connectwise-automate-cli fleet-health --agent

```

## Unique Features

### Cross-client roll-ups the per-server UI can't do

- **`fleet-health`**  -  See every agent across every client in one roll-up: online/offline, last-contact age, and open-alert count, grouped by client.

  _Reach for this when an agent needs the whole-fleet posture in one shot instead of paging the Computers endpoint client by client._

  ```bash
  connectwise-automate-cli fleet-health --agent
  ```
- **`stale-agents`**  -  List computers not seen in N days, grouped by client  -  the offline agents bleeding license and hiding risk.

  _Use before license true-ups or security reviews to find agents that stopped checking in._

  ```bash
  connectwise-automate-cli stale-agents --days 30 --agent
  ```
- **`patch-compliance`**  -  Per-client patch posture from synced patch history joined to computers  -  worst offenders first.

  _Pull this before a QBR or a security conversation to name the clients that are behind._

  ```bash
  connectwise-automate-cli patch-compliance --agent
  ```
- **`client-rollup`**  -  One-line-per-client snapshot: computers, locations, offline agents, and open alerts  -  built for the client review.

  _The fastest way to brief on a client's whole environment without clicking through the console._

  ```bash
  connectwise-automate-cli client-rollup --agent
  ```

### Triage and inventory

- **`alert-triage`**  -  Open alerts across every client, ranked by priority and joined to computer → location → client, with duplicates collapsed.

  _Start the morning here to see what actually needs a human across the whole book of business._

  ```bash
  connectwise-automate-cli alert-triage --min-priority 3 --agent
  ```
- **`os-inventory`**  -  Fleet-wide operating-system distribution with end-of-life OSes (Windows 7, Server 2008/2012) flagged for upgrade planning.

  _Use for security and hardware-refresh planning when you need the EOL exposure across every client at once._

  ```bash
  connectwise-automate-cli os-inventory --eol-only --agent
  ```
- **`since`**  -  Fleet activity in the last N hours from the records' own timestamps: alerts created, agents that checked in, and patches installed.

  _Run first thing to catch overnight activity across the fleet without paging each endpoint._

  ```bash
  connectwise-automate-cli since --hours 24 --agent
  ```
- **`agent-versions`**  -  Automate agent versions across every client, newest first, with the computers still behind counted per client.

  _Run after a server upgrade to find the agents that did not update: they are the ones that stop patching and running scripts first._

  ```bash
  connectwise-automate-cli agent-versions --behind-only --agent
  ```

## Recipes


### Morning triage across all clients

```bash
connectwise-automate-cli alert-triage --min-priority 3 --agent --select client,computer,priority,message
```

Narrows the alert payload to the four fields that matter so an agent doesn't burn context on the full alert objects.

### Find offline agents for a license true-up

```bash
connectwise-automate-cli stale-agents --days 45 --agent
```

Lists computers not seen in 45 days, grouped by client, straight from the local store.

### Patch posture for a QBR

```bash
connectwise-automate-cli patch-compliance --agent
```

Per-client patch percentages, worst first, ready to paste into a client review.

### Filter live computers without syncing

```bash
connectwise-automate-cli computers list --condition "Status='Offline'" --order-by "LastContact asc" --agent
```

Uses Automate's own condition/orderby query against the live API when you don't want a full sync.

## Usage

Run `connectwise-automate-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `CONNECTWISE_AUTOMATE_CONFIG_DIR`, `CONNECTWISE_AUTOMATE_DATA_DIR`, `CONNECTWISE_AUTOMATE_STATE_DIR`, or `CONNECTWISE_AUTOMATE_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `CONNECTWISE_AUTOMATE_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export CONNECTWISE_AUTOMATE_HOME=/srv/connectwise-automate
connectwise-automate-cli doctor
```

On Windows (PowerShell):

```powershell
$env:CONNECTWISE_AUTOMATE_HOME = "C:\ProgramData\connectwise-automate"
connectwise-automate-cli doctor
```

Under `CONNECTWISE_AUTOMATE_HOME=/srv/connectwise-automate`, the four dirs resolve to `/srv/connectwise-automate/config`, `/srv/connectwise-automate/data`, `/srv/connectwise-automate/state`, and `/srv/connectwise-automate/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:CONNECTWISE_AUTOMATE_HOME = "C:\srv\connectwise-automate"
connectwise-automate-cli doctor
```

Under `$env:CONNECTWISE_AUTOMATE_HOME = "C:\srv\connectwise-automate"`, the four dirs resolve to `C:\srv\connectwise-automate\config`, `C:\srv\connectwise-automate\data`, `C:\srv\connectwise-automate\state`, and `C:\srv\connectwise-automate\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "connectwise-automate": {
      "command": "connectwise-automate-mcp",
      "env": {
        "CONNECTWISE_AUTOMATE_HOME": "/srv/connectwise-automate"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `CONNECTWISE_AUTOMATE_DATA_DIR` overrides an explicit `--home` for that kind. Use `CONNECTWISE_AUTOMATE_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `CONNECTWISE_AUTOMATE_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `connectwise-automate-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `connectwise-automate-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `connectwise-automate-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `CONNECTWISE_AUTOMATE_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `CONNECTWISE_AUTOMATE_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### alerts

Open monitor alerts across the fleet

- **`connectwise-automate-cli alerts get`** - Get a single alert by Id
- **`connectwise-automate-cli alerts list`** - List open alerts across all computers

### apitoken

Mint and refresh API bearer tokens

- **`connectwise-automate-cli apitoken mint`** - Mint a bearer token from username + password (needs CONNECTWISE_AUTOMATE_SERVER + clientId header)
- **`connectwise-automate-cli apitoken refresh`** - Refresh an existing (still-valid) bearer token

### clients

Clients (companies / customers) in Automate

- **`connectwise-automate-cli clients get`** - Get a single client by Id
- **`connectwise-automate-cli clients list`** - List all clients

### commands

Available commands that can be executed on agents

- **`connectwise-automate-cli commands get`** - Get a single command by Id
- **`connectwise-automate-cli commands list`** - List all available commands

### computers

Managed endpoints (agents)  -  the core RMM inventory

- **`connectwise-automate-cli computers alerts`** - Open alerts for one computer
- **`connectwise-automate-cli computers command-execute`** - Execute a command on one computer (WRITE  -  runs a real command on the agent)
- **`connectwise-automate-cli computers command-history`** - Recent command execution history for one computer
- **`connectwise-automate-cli computers get`** - Get a single computer by Id
- **`connectwise-automate-cli computers list`** - List computers across the fleet (paginated, filterable)
- **`connectwise-automate-cli computers patching-stats`** - Patch installation statistics for one computer
- **`connectwise-automate-cli computers software`** - Installed software inventory for one computer

### contacts

Client contacts

- **`connectwise-automate-cli contacts`** - List all client contacts

### groups

Computer groups (organizational + policy grouping)

- **`connectwise-automate-cli groups get`** - Get a single group by Id
- **`connectwise-automate-cli groups list`** - List all groups

### locations

Locations (sites) belonging to clients

- **`connectwise-automate-cli locations get`** - Get a single location by Id
- **`connectwise-automate-cli locations list`** - List all locations

### monitors

Monitors and their per-monitor statistics

- **`connectwise-automate-cli monitors list`** - List monitors with their alerting statistics
- **`connectwise-automate-cli monitors sensor-checks`** - List sensor checks

### network-devices

Discovered network devices (non-agent)

- **`connectwise-automate-cli network-devices`** - List discovered network devices

### patching

Patch history, compliance information, and patch policies

- **`connectwise-automate-cli patching approval-policies`** - Patch approval policies
- **`connectwise-automate-cli patching deploy-approved`** - Deploy all approved patches (WRITE  -  triggers fleet patch deployment)
- **`connectwise-automate-cli patching deploy-security`** - Deploy all security patches (WRITE  -  triggers fleet patch deployment)
- **`connectwise-automate-cli patching information`** - Global patch information / catalog status
- **`connectwise-automate-cli patching list`** - Fleet-wide patch installation history
- **`connectwise-automate-cli patching microsoft-policies`** - Microsoft update policies
- **`connectwise-automate-cli patching reattempt-failed`** - Reattempt failed patches (WRITE  -  retries failed patch installs)
- **`connectwise-automate-cli patching thirdparty-policies`** - Third-party update policies

### scripts

Automation scripts and their run state

- **`connectwise-automate-cli scripts list`** - List all scripts
- **`connectwise-automate-cli scripts running`** - List scripts currently running across the fleet
- **`connectwise-automate-cli scripts schedules`** - List scheduled script runs

### server

Automate server metadata (used by doctor / health)

- **`connectwise-automate-cli server db-time`** - Current database server time
- **`connectwise-automate-cli server get`** - Server version and metadata


### agent-versions


- **`connectwise-automate-cli agent-versions`** - Automate agent versions across every client, newest first, with the computers still behind counted per client.


### alert-triage


- **`connectwise-automate-cli alert-triage`** - Open alerts across every client, ranked by priority and joined to computer → location → client, with duplicates collapsed.


### client-rollup


- **`connectwise-automate-cli client-rollup`** - One-line-per-client snapshot: computers, locations, offline agents, and open alerts  -  built for the client review.


### fleet-health


- **`connectwise-automate-cli fleet-health`** - See every agent across every client in one roll-up: online/offline, last-contact age, and open-alert count, grouped by client.


### os-inventory


- **`connectwise-automate-cli os-inventory`** - Fleet-wide operating-system distribution with end-of-life OSes (Windows 7, Server 2008/2012) flagged for upgrade planning.


### patch-compliance


- **`connectwise-automate-cli patch-compliance`** - Per-client patch posture from synced patch history joined to computers  -  worst offenders first.


### since


- **`connectwise-automate-cli since`** - Fleet activity in the last N hours from the records' own timestamps: alerts created, agents that checked in, and patches installed.


### stale-agents


- **`connectwise-automate-cli stale-agents`** - List computers not seen in N days, grouped by client  -  the offline agents bleeding license and hiding risk.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`connectwise-automate-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`connectwise-automate-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`connectwise-automate-cli learnings list`** - Inspect taught rows
- **`connectwise-automate-cli learnings forget <query>`** - Undo a teach
- **`connectwise-automate-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`connectwise-automate-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`connectwise-automate-cli teach-pattern`** - Install a query/resource template up front
- **`connectwise-automate-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `CONNECTWISE_AUTOMATE_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `connectwise-automate-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
connectwise-automate-cli alerts list

# JSON for scripting and agents
connectwise-automate-cli alerts list --json
# Filter to specific fields
connectwise-automate-cli alerts list --json --select Id,ComputerId,ComputerName

# Dry run  -  show the request without sending
connectwise-automate-cli alerts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
connectwise-automate-cli alerts list --agent
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

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `CONNECTWISE_AUTOMATE_SERVER` resolves `{server}`

Base URL: `https://{server}/cwa/api/v1`

## Health Check

```bash
connectwise-automate-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `connectwise-automate-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/connectwise-automate-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\connectwise-automate-cli\config.toml` on Windows; `--home`, `CONNECTWISE_AUTOMATE_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `CONNECTWISE_AUTOMATE_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `CONNECTWISE_AUTOMATE_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `CONNECTWISE_AUTOMATE_SERVER` | endpoint | Yes |  |
| `CONNECTWISE_AUTOMATE_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `connectwise-automate-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `connectwise-automate-cli doctor` to check credentials
- Verify the environment variable is set: `echo $CONNECTWISE_AUTOMATE_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific

- **401 Unauthorized on every call**  -  Token expired (they are short-lived). Re-mint with `apitoken mint` or `apitoken refresh` (the new token is saved for you; an exported CONNECTWISE_AUTOMATE_TOKEN still wins, so update or unset it).
- **403 / clientId errors on v2020.11+ servers**  -  Set CONNECTWISE_AUTOMATE_CLIENT_ID to your registered integration GUID; it is sent as the clientId header on every request.
- **Requests hit YOUR_SERVER.hostedrmm.com**  -  CONNECTWISE_AUTOMATE_SERVER is unset  -  export your real server host or set base_url in config.
- **Empty results from list commands**  -  Pagination is page/page-size (max 1000). Run `sync` first for offline queries, or pass --condition to filter the live call.

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**AutomateAPI**](https://github.com/gavsto/AutomateAPI)  -  PowerShell
- [**connectwise-rest**](https://github.com/covenanttechnologysolutions/connectwise-rest)  -  TypeScript
- [**PS-CWAutomate**](https://github.com/jasonrush/PS-CWAutomate)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
