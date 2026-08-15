# UniFi CLI

**Every UniFi Network API operation, plus drift detection, topology, and rule prediction no other UniFi tool has.**

unifi-network-cli wraps the full local Network integration API (devices, clients, firewall, ACL, networks, VPN, switching) with a local SQLite mirror. That mirror is what lets it answer questions the live API can't: what changed since yesterday, what device just joined, and which firewall rule would match a given packet.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer places the `unifi-network-cli` and `unifi-network-mcp` binaries on your PATH. Registering the skill with your agent, and wiring `unifi-network-mcp` into Claude Desktop or ChatGPT, are separate steps - see the [README](./README.md) install table and [mcp-install.md](./mcp-install.md):

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/unifi-network/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/unifi-network/install.ps1 | iex
   ```
3. Verify: `unifi-network-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=unifi-network).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `UNIFI_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install unifi-network-cli/cmd/unifi-network-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "unifi": {
      "command": "unifi-network-mcp",
      "env": {
        "UNIFI_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Generate a local API key from the gateway's own UI (Settings -> Control Plane -> Integrations -> Create API Key) and set UNIFI_API_KEY. The gateway's self-signed certificate is handled automatically for private/loopback/link-local hosts; no --insecure flag needed for the common case.

## Quick Start

```bash
# Confirm config and connectivity without making a live call.
unifi-network-cli doctor --dry-run

# Find the site ID you'll use for every other command.
unifi-network-cli sites

# Populate the local mirror so drift/topology/newcomer have a baseline.
unifi-network-cli sync

# See which clients sit behind which device.
unifi-network-cli topology

# Check what changed in the last day.
unifi-network-cli drift --since 24h

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`topology`**  -  Group every synced client under the device it is attached to, a device-to-client tree built entirely from local mirror data, no live crawl needed.

  _Reach for this when an agent needs to see which clients sit behind which device without walking every device endpoint individually._

  ```bash
  unifi-network-cli topology --site default --json
  ```
- **`drift`**  -  Show what changed in site config (networks, firewall, wifi, DNS) since the last drift run for this site.

  _Use after a suspected config change to see exactly what moved, without manually diffing the controller UI._

  ```bash
  unifi-network-cli drift --site default --since 24h --json
  ```
- **`newcomer`**  -  List devices and clients first seen since a given sync, for spotting new hardware joining the network.

  _Use for periodic security review of what joined the network recently._

  ```bash
  unifi-network-cli newcomer --since 7d --json
  ```

### Agent-native plumbing
- **`port-audit`**  -  Review port utilization and PoE status across every switch on a site in one table.

  _Use before adding new PoE devices to check headroom, or to find unused ports across a stack._

  ```bash
  unifi-network-cli port-audit --site default --json
  ```
- **`guest report`**  -  Summarize guest network usage: active vouchers and connected guest clients, from local data.

  _Use for a quick guest-network health check without cross-referencing three separate UI screens._

  ```bash
  unifi-network-cli guest report --site default --json
  ```
- **`rule-predict`**  -  Predict which firewall policy would match a hypothetical packet before making a live change.

  _Use to check the effect of a proposed firewall change before applying it live._

  ```bash
  unifi-network-cli rule-predict --src 10.0.3.0/24 --dst 10.0.0.1 --port 443 --json
  ```

## Recipes

### Find who just joined the network

```bash
unifi-network-cli newcomer --since 7d --json --select id,name,mac
```

Narrow a potentially large newcomer list down to just the fields needed to identify each device.

### Audit switch port headroom before adding a PoE device

```bash
unifi-network-cli port-audit --site default --json
```

Lists PoE status and free ports across every switch on the site.

### Check what a firewall change would match

```bash
unifi-network-cli rule-predict --src 10.0.3.0/24 --dst 10.0.0.1 --port 443 --json
```

Simulates rule evaluation order against the synced ruleset before making a live change.

## Usage

Run `unifi-network-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `UNIFI_CONFIG_DIR`, `UNIFI_DATA_DIR`, `UNIFI_STATE_DIR`, or `UNIFI_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `UNIFI_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export UNIFI_HOME=/srv/unifi
unifi-network-cli doctor
```

Under `UNIFI_HOME=/srv/unifi`, the four dirs resolve to `/srv/unifi/config`, `/srv/unifi/data`, `/srv/unifi/state`, and `/srv/unifi/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "unifi": {
      "command": "unifi-network-mcp",
      "env": {
        "UNIFI_HOME": "/srv/unifi"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `UNIFI_DATA_DIR` overrides an explicit `--home` for that kind. Use `UNIFI_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `UNIFI_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `unifi-network-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### countries

Manage countries

- **`unifi-network-cli countries`** - Returns ISO-standard country codes and names,
used for region-based configuration or regulatory compliance.

<details>
<summary>Filterable properties (click to expand)</summary>

|Name|Type|Allowed functions|
|-|-|-|
|`code`|`STRING`|`eq` `ne` `in` `notIn`|
|`name`|`STRING`|`eq` `ne` `in` `notIn` `like`|
</details>

### dpi

Manage dpi

- **`unifi-network-cli dpi get-application-categories`** - Returns predefined Deep Packet Inspection (DPI) application categories used for traffic identification and filtering.

<details>
<summary>Filterable properties (click to expand)</summary>

|Name|Type|Allowed functions|
|-|-|-|
|`id`|`INTEGER`|`eq` `ne` `in` `notIn`|
|`name`|`STRING`|`eq` `ne` `in` `notIn` `like`|
</details>
- **`unifi-network-cli dpi get-applications`** - Lists DPI-recognized applications grouped under categories. Useful for firewall or traffic analytics integration.

<details>
<summary>Filterable properties (click to expand)</summary>

|Name|Type|Allowed functions|
|-|-|-|
|`id`|`INTEGER`|`eq` `ne` `in` `notIn`|
|`name`|`STRING`|`eq` `ne` `in` `notIn` `like`|
</details>

### info

Manage info

- **`unifi-network-cli info`** - Retrieve general information about the UniFi Network application.

### pending-devices

Manage pending devices

- **`unifi-network-cli pending-devices`** - Retrieve a paginated list of devices pending adoption, including basic device information.

<details>
<summary>Filterable properties (click to expand)</summary>

|Name|Type|Allowed functions|
|-|-|-|
|`macAddress`|`STRING`|`eq` `ne` `in` `notIn`|
|`ipAddress`|`STRING`|`eq` `ne` `in` `notIn`|
|`model`|`STRING`|`eq` `ne` `in` `notIn`|
|`state`|`STRING`|`eq` `ne` `in` `notIn`|
|`supported`|`BOOLEAN`|`eq` `ne`|
|`firmwareVersion`|`STRING`|`isNull` `isNotNull` `eq` `ne` `gt` `ge` `lt` `le` `like` `in` `notIn`|
|`firmwareUpdatable`|`BOOLEAN`|`eq` `ne`|
|`features`|`SET(STRING)`|`isEmpty` `contains` `containsAny` `containsAll` `containsExactly`|
</details>

### sites

Endpoints for listing and managing UniFi sites within a local Network application.
Site ID is required for most other API requests.

- **`unifi-network-cli sites`** - Retrieve a paginated list of local sites managed by this Network application.
Site ID is required for other UniFi Network API calls.

<details>
<summary>Filterable properties (click to expand)</summary>

|Name|Type|Allowed functions|
|-|-|-|
|`id`|`UUID`|`eq` `ne` `in` `notIn`|
|`internalReference`|`STRING`|`eq` `ne` `in` `notIn`|
|`name`|`STRING`|`eq` `ne` `in` `notIn`|
</details>


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`unifi-network-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`unifi-network-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`unifi-network-cli learnings list`** - Inspect taught rows
- **`unifi-network-cli learnings forget <query>`** - Undo a teach
- **`unifi-network-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`unifi-network-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`unifi-network-cli teach-pattern`** - Install a query/resource template up front
- **`unifi-network-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `UNIFI_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `unifi-network-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
unifi-network-cli countries

# JSON for scripting and agents
unifi-network-cli countries --json

# Filter to specific fields
unifi-network-cli countries --json --select id,name,status

# Dry run  -  show the request without sending
unifi-network-cli countries --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
unifi-network-cli countries --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
unifi-network-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `unifi-network-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/unifi-network-cli/config.toml`; `--home`, `UNIFI_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `UNIFI_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `unifi-network-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `unifi-network-cli doctor` to check credentials
- Verify the environment variable is set: `echo $UNIFI_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **x509: certificate signed by unknown authority**  -  This should auto-resolve for private/loopback/link-local gateway hosts; if your gateway is reachable via a public hostname, set UNIFI_INSECURE_SKIP_VERIFY=1 explicitly.
- **401 Unauthorized**  -  Regenerate the API key under Settings -> Control Plane -> Integrations on the gateway and re-export UNIFI_API_KEY.
- **drift/newcomer/topology return empty**  -  Run 'unifi-network-cli sync' first to populate the local mirror; these commands are local-mirror-only.

## Known Gaps

- **`port-audit` is the only novel command that is not local-mirror-only.**
  Per-port link/PoE state appears in no list/sync response for this API  - 
  only a per-device detail fetch returns it  -  so `port-audit` reads device
  IDs from the local mirror, then fetches interfaces live, one call per
  switching or gateway device.
- **`topology` makes no live call, and so cannot show device-to-device
  uplink chaining.** That chaining is also detail-only, and `topology`
  deliberately stays local: it groups synced clients under the device each
  is attached to, leaving every device at the top level. A switch behind a
  switch is not nested.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**homelab-mcp**](https://github.com/bjeans/homelab-mcp)  -  Python (39 stars)
- [**go-unifi-mcp**](https://github.com/claytono/go-unifi-mcp)  -  Go (3 stars)
- [**unifi-mcp**](https://github.com/gordcurrie/unifi-mcp)  -  Go (2 stars)
- [**unifi-cli**](https://github.com/lucasilverentand/unifi-cli)  -  TypeScript
- [**mcp-unifi**](https://github.com/pete-builds/mcp-unifi)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
