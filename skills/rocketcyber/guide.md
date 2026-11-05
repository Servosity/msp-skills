# RocketCyber CLI

**The first CLI and MCP server for RocketCyber Managed SOC, with triage and posture analytics no console page or API call computes.**

Every RocketCyber Customer API v3 endpoint as an agent-ready command, including the suppression rules and CSV report export that the PowerShell module lacks. On top sit computed SOC analytics: a cross-account triage board, incident MTTR, stale-agent detection, Defender risk ranking, and secure-score trends, backed by an offline SQLite store with full-text search.

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `rocketcyber-cli` and `rocketcyber-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.ps1 | iex
   ```
3. Verify: `rocketcyber-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=rocketcyber). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/rocketcyber --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/rocketcyber --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `rocketcyber-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the rocketcyber skill from https://github.com/Servosity/msp-skills/tree/main/skills/rocketcyber. The skill defines how its required CLI (`rocketcyber-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=rocketcyber).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `ROCKETCYBER_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/rocketcyber/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "rocketcyber": {
      "command": "rocketcyber-mcp",
      "env": {
        "ROCKETCYBER_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`rocketcyber-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `ROCKETCYBER_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `ROCKETCYBER_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export ROCKETCYBER_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
rocketcyber-mcp --transport http
```

Set `ROCKETCYBER_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

RocketCyber uses a single bearer API token. In the RocketCyber console, go to Provider settings and copy your API token, then export it as ROCKETCYBER_API_TOKEN. US partners use the default base URL; EU and AP partners should set the regional API host (for example https://api-eu.rocketcyber.com/v3) as the base URL in the config file. Verify with rocketcyber-cli doctor.

## Quick Start

```bash
# Health check: verifies config, token presence, and API reachability without side effects
rocketcyber-cli doctor --dry-run


# Confirm auth works and see your provider account plus client account IDs
rocketcyber-cli account --json


# Open SOC incidents across accounts - the core triage feed
rocketcyber-cli incidents --status open --json


# Snapshot agents, incidents, and detection apps into the local SQLite store
rocketcyber-cli sync --resources agents,incidents,apps --full


# One ranked board: open incidents + event verdicts + offline agents
rocketcyber-cli triage --since 24h --agent

```

## Unique Features

### SOC triage that compounds

- **`triage`**  -  One ranked board of open incidents, event verdict counts, and offline agents across every client account.

  _Reach for this when asked for an overall SOC health snapshot or what broke overnight, instead of three separate list calls._

  ```bash
  rocketcyber-cli triage --since 24h --agent
  ```
- **`agents stale`**  -  Devices that stopped reporting beyond a time window, grouped by client account.

  _Use this for fleet-hygiene sweeps (which devices went dark this week) rather than paging the live agents endpoint._

  ```bash
  rocketcyber-cli agents stale --since 7d --json
  ```

### Posture analytics

- **`incidents mttr`**  -  Mean and median time-to-resolve plus open-incident aging buckets, computed from incident created/resolved timestamps.

  _Use this for SLA and QBR reporting questions like how fast incidents get resolved, instead of fetching raw incidents and computing by hand._

  ```bash
  rocketcyber-cli incidents mttr --since 90d --json
  ```
- **`defender riskiest`**  -  Devices-at-risk ranked by weighted malicious and suspicious detection counts.

  _Use this when asked which machines need attention first, instead of parsing raw Defender JSON._

  ```bash
  rocketcyber-cli defender riskiest --account-id 2 --top 10 --json
  ```
- **`office trend`**  -  First/last/delta/direction computed over the Microsoft 365 secure-score daily series.

  _Use this for is-our-365-posture-improving questions instead of dumping the raw daily series._

  ```bash
  rocketcyber-cli office trend --account-id 2 --json
  ```
- **`suppression audit`**  -  Alert-suppression rules classified by status and age, flagging stale rules that may hide real detections.

  _Use this for suppression-rule hygiene reviews instead of fetching rules one by one._

  ```bash
  rocketcyber-cli suppression audit --stale-after 90d --json
  ```

## Recipes


### Overnight SOC triage in one command

```bash
rocketcyber-cli triage --since 24h --json
```

Fans out to incidents, event summaries, and agents, then returns one ranked cross-account board with partial-failure accounting.

### Narrow Defender risk to the fields that matter

```bash
rocketcyber-cli defender --account-id 2 --agent --select devicesAtRisk.data.hostname,devicesAtRisk.data.detections.malicious
```

The defender payload is deeply nested - dotted --select paths keep agent context small.

### Quarterly MTTR evidence

```bash
rocketcyber-cli incidents mttr --since 90d --json
```

Mean/median resolution hours plus aging buckets, computed from synced incident timestamps.

### Full-text hunt across incident remediation text

```bash
rocketcyber-cli search "ransomware" --type incidents --limit 20
```

FTS5 over synced incident title, description, and remediation - no API endpoint can text-search these.

### Find devices that went dark this week

```bash
rocketcyber-cli agents stale --since 7d --json
```

Filters synced agents on lastConnected age, a dimension the live API cannot filter by.

## Usage

Run `rocketcyber-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `ROCKETCYBER_CONFIG_DIR`, `ROCKETCYBER_DATA_DIR`, `ROCKETCYBER_STATE_DIR`, or `ROCKETCYBER_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `ROCKETCYBER_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export ROCKETCYBER_HOME=/srv/rocketcyber
rocketcyber-cli doctor
```

On Windows (PowerShell):

```powershell
$env:ROCKETCYBER_HOME = "C:\ProgramData\rocketcyber"
rocketcyber-cli doctor
```

Under `ROCKETCYBER_HOME=/srv/rocketcyber`, the four dirs resolve to `/srv/rocketcyber/config`, `/srv/rocketcyber/data`, `/srv/rocketcyber/state`, and `/srv/rocketcyber/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:ROCKETCYBER_HOME = "C:\srv\rocketcyber"
rocketcyber-cli doctor
```

Under `$env:ROCKETCYBER_HOME = "C:\srv\rocketcyber"`, the four dirs resolve to `C:\srv\rocketcyber\config`, `C:\srv\rocketcyber\data`, `C:\srv\rocketcyber\state`, and `C:\srv\rocketcyber\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "rocketcyber": {
      "command": "rocketcyber-mcp",
      "env": {
        "ROCKETCYBER_HOME": "/srv/rocketcyber"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `ROCKETCYBER_DATA_DIR` overrides an explicit `--home` for that kind. Use `ROCKETCYBER_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `ROCKETCYBER_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `rocketcyber-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. A GET answer is cached on disk for up to five minutes (`--no-cache` skips it). Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `rocketcyber-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `rocketcyber-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `ROCKETCYBER_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `ROCKETCYBER_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### account

Provider account information and client account hierarchy

- **`rocketcyber-cli account`** - Get account information, including child customer accounts

### agents

RocketCyber agents (monitored devices) and their connectivity

- **`rocketcyber-cli agents`** - List agents (devices) with inventory and connectivity filters

### apps

Detection apps catalog (threat detection modules)

- **`rocketcyber-cli apps`** - List detection apps and their status for an account

### defender

Microsoft Defender health and devices-at-risk telemetry

- **`rocketcyber-cli defender`** - Get Defender detection summary, devices at risk, and device health

### events

Verdict-classified detection events per detection app

- **`rocketcyber-cli events list`** - List detection events for an app, filtered by verdict and date window
- **`rocketcyber-cli events summary`** - Per-app event verdict counts for an account

### firewalls

Firewall log sources feeding the SOC

- **`rocketcyber-cli firewalls`** - List firewall log sources, optionally with ingest counters

### incidents

SOC-published incidents with status and lifecycle timestamps

- **`rocketcyber-cli incidents`** - List SOC incidents with status, title, and date filters

### office

Microsoft 365 secure-score telemetry

- **`rocketcyber-cli office`** - Get the Microsoft 365 secure-score daily progress series

### reports

CSV report export (reportApi)

- **`rocketcyber-cli reports`** - Export events or incidents as a CSV report

### suppression

Alert-suppression rules that filter SOC noise

- **`rocketcyber-cli suppression rule`** - Get a single suppression rule by ID
- **`rocketcyber-cli suppression rules`** - List alert-suppression rules with status and ownership filters


### agents


- **`rocketcyber-cli agents stale`** - Devices that stopped reporting beyond a time window, grouped by client account.


### defender


- **`rocketcyber-cli defender riskiest`** - Devices-at-risk ranked by weighted malicious and suspicious detection counts.


### incidents


- **`rocketcyber-cli incidents mttr`** - Mean and median time-to-resolve plus open-incident aging buckets, computed from incident created/resolved timestamps.


### office


- **`rocketcyber-cli office trend`** - First/last/delta/direction computed over the Microsoft 365 secure-score daily series.


### suppression


- **`rocketcyber-cli suppression audit`** - Alert-suppression rules classified by status and age, flagging stale rules that may hide real detections.


### triage


- **`rocketcyber-cli triage`** - One ranked board of open incidents, event verdict counts, and offline agents across every client account.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`rocketcyber-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`rocketcyber-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`rocketcyber-cli learnings list`** - Inspect taught rows
- **`rocketcyber-cli learnings forget <query>`** - Undo a teach
- **`rocketcyber-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`rocketcyber-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`rocketcyber-cli teach-pattern`** - Install a query/resource template up front
- **`rocketcyber-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `ROCKETCYBER_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `rocketcyber-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
rocketcyber-cli account

# JSON for scripting and agents
rocketcyber-cli account --json
# Filter to specific fields
rocketcyber-cli account --json --select name,hierarchy,path

# Dry run  -  show the request without sending
rocketcyber-cli account --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
rocketcyber-cli account --agent
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

## Freshness

This CLI owns bounded freshness for registered store-backed read command paths. In `--data-source auto` mode, covered commands check the local SQLite store before serving results; stale or missing resources trigger a bounded refresh, and refresh failures fall back to the existing local data with a warning. `--data-source local` never refreshes, and `--data-source live` reads the API without mutating the local store.

Set `ROCKETCYBER_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths:
- `rocketcyber-cli agents`
- `rocketcyber-cli apps`
- `rocketcyber-cli firewalls`
- `rocketcyber-cli incidents`

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
rocketcyber-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `rocketcyber-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/rocketcyber-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\rocketcyber-cli\config.toml` on Windows; `--home`, `ROCKETCYBER_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `ROCKETCYBER_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `ROCKETCYBER_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `ROCKETCYBER_API_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `rocketcyber-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- The API token (`Authorization: Bearer <ROCKETCYBER_API_TOKEN>`) and every API request go only to the scheme, host and port of the configured base URL (`https://api-us.rocketcyber.com/v3`, or `ROCKETCYBER_BASE_URL` for another region). A redirect to any other host is refused, so the token never reaches it.
- The shared credential kit holds `ROCKETCYBER_API_TOKEN`, `ROCKETCYBER_MCP_HTTP_TOKEN` and every value in `config.toml` and `credentials.toml` from the first instruction of each binary. Any output, file, local-store write, MCP result, child process or request that would repeat one of them, in any encoding the kit reads, is replaced whole by `[credkit: withheld from <sink>: it carried a credential this program holds]`. A read answer is cached on disk for up to five minutes (0600 files, checked the same way; `--no-cache` skips it).
- A program that lost part of its standard output exits 74 instead of 0. A token that also occurs in ordinary text (for example one repeated letter) is refused at startup with exit 78.
- Only `credentials.toml` (mode 0600) holds a credential, or `config.toml` (also 0600) when agentcookie manages the token; `auth set-token` reads the token from stdin, never from an argument. A `base_url` saved to `config.toml` keeps no userinfo or query.
- `--dry-run` sends nothing. The RocketCyber Customer API is read-only, so no command or MCP tool changes RocketCyber data; a tool argument can never name a file to read or write.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `rocketcyber-cli doctor` to check credentials
- Verify the environment variable is set: `echo $ROCKETCYBER_API_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific

- **401 Authentication Error on every call**  -  Regenerate the API token in the RocketCyber console (Provider settings) and re-export ROCKETCYBER_API_TOKEN
- **Valid token but all calls return 401 (EU/AP partner)**  -  Your tenant lives on a regional host - set base_url to your region (for example https://api-eu.rocketcyber.com/v3) in ~/.config/rocketcyber-cli/config.toml
- **events list returns an error about a missing app id**  -  The events endpoint requires --app-id - find IDs with rocketcyber-cli apps
- **agents stale or incidents mttr returns empty results**  -  These read the local store - run rocketcyber-cli sync --resources agents,incidents --full first

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**Celerium.RocketCyber**](https://github.com/Celerium/Celerium.RocketCyber)  -  PowerShell
- [**RocketCyber-PowerShellWrapper**](https://github.com/Celerium/RocketCyber-PowerShellWrapper)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
