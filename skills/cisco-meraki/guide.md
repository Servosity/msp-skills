# Cisco Meraki CLI

**Every Meraki Dashboard operation, plus cross-org fleet views that read every page, built for MSPs running many client orgs.**

One API key reaches every organization you administer, so this CLI treats them as one fleet: fleet health and fleet down show what is broken across all clients, fleet renewals lists license and hardware deadlines, and fleet find resolves a MAC, IP or serial to the client that owns it. Fleet commands walk every Link-header page; when some orgs fail they still exit 0 with the rows they could read and name the failed orgs on stderr (fleet health also marks each unreachable org's row, and fleet fanout carries the error in each org's row under --json). Generated list commands read every page with --all, and every write supports --dry-run.

Learn more at [Cisco Meraki](https://developer.cisco.com/meraki/api-v1/).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `cisco-meraki-cli` and `cisco-meraki-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.ps1 | iex
   ```
3. Verify: `cisco-meraki-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=cisco-meraki). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/cisco-meraki --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/cisco-meraki --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `cisco-meraki-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the cisco-meraki skill from https://github.com/Servosity/msp-skills/tree/main/skills/cisco-meraki. The skill defines how its required CLI (`cisco-meraki-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=cisco-meraki).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `MERAKI_DASHBOARD_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. The bundle carries macOS (Apple Silicon and Intel), Linux x64 and Windows x64 binaries; on Linux or Windows on ARM, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-meraki/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "cisco-meraki": {
      "command": "cisco-meraki-mcp",
      "env": {
        "MERAKI_DASHBOARD_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Meraki uses one Dashboard API key, sent as the X-Cisco-Meraki-API-Key header. Enable API access for each organization (Organization > Settings > Dashboard API access), then generate a key under My Profile > API access. The key reaches every organization that admin can see, which is what the fleet commands fan out across. Export it as MERAKI_DASHBOARD_API_KEY or store it with auth set-token. Dashboards in Canada, China, India or FedRAMP need CISCO_MERAKI_BASE_URL set to that region's host.

## Quick Start

```bash
# Check the binary and its command tree without touching the network; run doctor without --dry-run once MERAKI_DASHBOARD_API_KEY is set
cisco-meraki-cli doctor --dry-run

# Mirror organizations, networks, devices, licenses and change logs from every org into local SQLite
cisco-meraki-cli sync --full

# See which client org is in the worst shape right now
cisco-meraki-cli fleet health --agent

# List every offline or alerting device and failed WAN uplink across clients
cisco-meraki-cli fleet down --agent

# List license and hardware deadlines due in the next 90 days, plus any already overdue
cisco-meraki-cli fleet renewals --within 90d

```

## Unique Features

### Every client org at once
- **`fleet health`**  -  One worst-first row per client org with online, alerting, offline and dormant device counts, failed WAN uplinks, active alerts, and whether the org is reachable at all.

  _Reach for this first when asked which client is in the worst shape right now; it answers in one call instead of one call per org._

  ```bash
  cisco-meraki-cli fleet health --agent
  ```
- **`fleet down`**  -  Every offline or alerting device and every failed MX WAN uplink across all orgs, with org and network names and, for offline devices, how long since the device last reported.

  _Use this to build a dispatch list of individual down devices and sites running on backup WAN across every client._

  ```bash
  cisco-meraki-cli fleet down --agent
  ```
- **`fleet find`**  -  Paste a MAC, IP, serial, network ID or name and get the org and network that own the matching device, inventory entry, MX uplink or, by MAC, client.

  _Use this whenever a ticket or alert names a device or client without saying which client org it belongs to._

  ```bash
  cisco-meraki-cli fleet find Q2QN-9J8L-SLPD --agent
  ```
- **`fleet fanout`**  -  Run any organization-scoped read against every org, fully paginated, with observed-at, item count, completeness and next-cursor metadata per org.

  _Use this for any cross-org read that has no dedicated command, when you need to know the answer is complete._

  ```bash
  cisco-meraki-cli fleet fanout organizations/{organizationId}/firmware/upgrades --agent
  ```

### Deadlines and access audits
- **`fleet renewals`**  -  License expirations (co-term, per-device or subscription, whichever each org uses) and end-of-sale and end-of-support hardware dates in one date-sorted list.

  _Use this for renewal and hardware-refresh pipelines across every client org._

  ```bash
  cisco-meraki-cli fleet renewals --within 90d --agent
  ```
- **`fleet admins`**  -  Dashboard admins across every org grouped by email, flagging missing two-factor auth, API key holders and stale access, with an offboarding view per person.

  _Use this for access reviews and technician offboarding across all client orgs._

  ```bash
  cisco-meraki-cli fleet admins --stale 90d --agent
  ```

### Change and outage history
- **`fleet changes`**  -  Configuration changes from every org in one list, filterable by admin, org, network and page, with old and new values.

  _Use this to answer who changed what in which client org this week._

  ```bash
  cisco-meraki-cli fleet changes --since 7d --agent
  ```
- **`fleet flaps`**  -  Devices ranked by minutes offline, then by number of status changes, over a window of up to 31 days, across every org.

  _Use this for the weekly look back at unstable devices, not for what is down right now._

  ```bash
  cisco-meraki-cli fleet flaps --since 7d --agent
  ```
- **`fleet blame`**  -  For one network ID or device serial: each time it went offline or started alerting within --since (default 7 days, up to 31), the configuration changes made on that network just before.

  _Use this in outage postmortems to find the change that probably caused it._

  ```bash
  cisco-meraki-cli fleet blame L_646829496481105433 --window 2h --agent
  ```

### Safe writes at fleet scale
- **`fleet push`**  -  Apply one settings change to many networks across orgs through action batches: it previews a per-network current-versus-new diff by default and sends batches only with --apply.

  _Use this for bulk configuration changes across many client networks; read the preview, then rerun with --apply._

  ```bash
  cisco-meraki-cli fleet push --path /networks/{networkId}/snmp --body snmp.json --network-tag managed
  ```

## Recipes

### Morning fleet triage

```bash
cisco-meraki-cli fleet down --agent --select organization_name,network_name,serial,kind,status,down_for
```

Narrow the cross-org down list to the fields a dispatcher needs.

### Who touched which client this week

```bash
cisco-meraki-cli fleet changes --since 7d --group-by admin --agent
```

Group every org's configuration changes by admin for the weekly review.

### Locate a MAC from a SOC alert

```bash
cisco-meraki-cli fleet find 00:18:0a:12:34:56 --agent
```

Resolve a client MAC to the org and network that owns it without guessing.

### Renewal pipeline

```bash
cisco-meraki-cli fleet renewals --within 90d --csv
```

Export license and end-of-support deadlines due within 90 days, plus overdue ones, across all orgs for quoting; add --include-expired=false to hide overdue items (CLI only: an MCP call cannot switch this default-on flag off, so filter out rows with negative days_remaining instead).

### Preview a bulk SNMP change

```bash
cisco-meraki-cli fleet push --path /networks/{networkId}/snmp --body snmp.json --network-tag managed
```

Shows which networks would change and which already match; nothing is sent until you rerun the same command with --apply.

## Usage

Run `cisco-meraki-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `CISCO_MERAKI_CONFIG_DIR`, `CISCO_MERAKI_DATA_DIR`, `CISCO_MERAKI_STATE_DIR`, or `CISCO_MERAKI_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `CISCO_MERAKI_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export CISCO_MERAKI_HOME=/srv/cisco-meraki
cisco-meraki-cli doctor
```

Under `CISCO_MERAKI_HOME=/srv/cisco-meraki`, the four dirs resolve to `/srv/cisco-meraki/config`, `/srv/cisco-meraki/data`, `/srv/cisco-meraki/state`, and `/srv/cisco-meraki/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "cisco-meraki": {
      "command": "cisco-meraki-mcp",
      "env": {
        "CISCO_MERAKI_HOME": "/srv/cisco-meraki"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `CISCO_MERAKI_DATA_DIR` overrides an explicit `--home` for that kind. Use `CISCO_MERAKI_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `CISCO_MERAKI_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `cisco-meraki-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### administered

Manage administered

- **`cisco-meraki-cli administered bind-licensing-subscription-subscription`** - Bind networks to a subscription
- **`cisco-meraki-cli administered claim-licensing-subscription-subscriptions`** - Claim a subscription into an organization.
- **`cisco-meraki-cli administered generate-identities-me-api-keys`** - Generates an API key for an identity. For users who have access to more than one organization, the change will take up to five minutes to propagate. If one of the organizations is currently under maintenance, the change may not propagate fully until after the maintenance has been completed.
- **`cisco-meraki-cli administered get-identities-me`** - Returns the identity of the current user.
- **`cisco-meraki-cli administered get-identities-me-api-keys`** - List the non-sensitive metadata associated with the API keys that belong to the user
- **`cisco-meraki-cli administered get-licensing-subscription-entitlements`** - Retrieve the list of purchasable entitlements
- **`cisco-meraki-cli administered get-licensing-subscription-subscriptions`** - List available subscriptions
- **`cisco-meraki-cli administered get-licensing-subscription-subscriptions-compliance-statuses`** - Get compliance status for requested subscriptions
- **`cisco-meraki-cli administered revoke-identities-me-api-keys`** - Revokes an identity's API key, using the last four characters of the key. For users who have access to more than one organization, the change will take up to five minutes to propagate. If one of the organizations is currently under maintenance, the change may not propagate fully until after the maintenance has been completed.
- **`cisco-meraki-cli administered validate-licensing-subscription-subscriptions-claim-key`** - Find a subscription by claim key. Returns 400 if the key has already been claimed.

### devices

Manage devices

- **`cisco-meraki-cli devices get`** - Return a single device
- **`cisco-meraki-cli devices update`** - Update the attributes of a device

### networks

Manage networks

- **`cisco-meraki-cli networks delete`** - Delete a network
- **`cisco-meraki-cli networks get`** - Return a network
- **`cisco-meraki-cli networks update`** - Update a network

### organizations

Manage organizations

- **`cisco-meraki-cli organizations create`** - Create a new organization
- **`cisco-meraki-cli organizations delete`** - Delete an organization
- **`cisco-meraki-cli organizations get`** - List the organizations that the user has privileges on
- **`cisco-meraki-cli organizations get-organizationid`** - Return an organization
- **`cisco-meraki-cli organizations update`** - Update an organization


### fleet


- **`cisco-meraki-cli fleet health`** - One worst-first row per client org with online, alerting, offline and dormant device counts, failed WAN uplinks, active alerts, and whether the org is reachable at all.

- **`cisco-meraki-cli fleet down`** - Every offline or alerting device and every failed MX WAN uplink across all orgs, with org and network names and, for offline devices, how long since the device last reported.

- **`cisco-meraki-cli fleet renewals`** - License expirations (co-term, per-device or subscription, whichever each org uses) and end-of-sale and end-of-support hardware dates in one date-sorted list.

- **`cisco-meraki-cli fleet changes`** - Configuration changes from every org in one list, filterable by admin, org, network and page, with old and new values.

- **`cisco-meraki-cli fleet find`** - Paste a MAC, IP, serial, network ID or name and get the org and network that own the matching device, inventory entry, MX uplink or, by MAC, client.

- **`cisco-meraki-cli fleet fanout`** - Run any organization-scoped read against every org, fully paginated, with observed-at, item count, completeness and next-cursor metadata per org.

- **`cisco-meraki-cli fleet push`** - Apply one settings change to many networks across orgs through action batches: it previews a per-network current-versus-new diff by default and sends batches only with --apply.

- **`cisco-meraki-cli fleet admins`** - Dashboard admins across every org grouped by email, flagging missing two-factor auth, API key holders and stale access, with an offboarding view per person.

- **`cisco-meraki-cli fleet flaps`** - Devices ranked by minutes offline, then by number of status changes, over a window of up to 31 days, across every org.

- **`cisco-meraki-cli fleet blame`** - For one network ID or device serial: each time it went offline or started alerting within --since (default 7 days, up to 31), the configuration changes made on that network just before.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`cisco-meraki-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`cisco-meraki-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`cisco-meraki-cli learnings list`** - Inspect taught rows
- **`cisco-meraki-cli learnings forget <query>`** - Undo a teach
- **`cisco-meraki-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`cisco-meraki-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`cisco-meraki-cli teach-pattern`** - Install a query/resource template up front
- **`cisco-meraki-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `CISCO_MERAKI_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `cisco-meraki-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
cisco-meraki-cli devices get Q2QN-9J8L-SLPD

# JSON for scripting and agents
cisco-meraki-cli devices get Q2QN-9J8L-SLPD --json
# Filter to specific fields
cisco-meraki-cli devices get Q2QN-9J8L-SLPD --json --select serial,name,model,networkId,lanIp

# Dry run  -  show the request without sending
cisco-meraki-cli devices get Q2QN-9J8L-SLPD --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
cisco-meraki-cli devices get Q2QN-9J8L-SLPD --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **No confirmation prompts** - writes and deletes run as soon as they are invoked, with no confirmation prompt; preview with `--dry-run` (or run `fleet push` without `--apply`) before mutating. `--yes` is only read by `profile delete` and client-profile commands.
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` generic error, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Endpoint

Base URL: `https://api.meraki.com/api/v1`. Set `CISCO_MERAKI_BASE_URL` to use a regional dashboard instead.

## Health Check

```bash
cisco-meraki-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `cisco-meraki-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/cisco-meraki-cli/config.toml`; `--home`, `CISCO_MERAKI_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `CISCO_MERAKI_BASE_URL` | endpoint | No | Regional dashboard base URL: `https://api.meraki.ca/api/v1` (Canada), `https://api.meraki.cn/api/v1` (China), `https://api.meraki.in/api/v1` (India), `https://api.gov-meraki.com/api/v1` (FedRAMP). |
| `MERAKI_DASHBOARD_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `cisco-meraki-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `cisco-meraki-cli doctor` to check credentials
- Verify the environment variable is set: `echo $MERAKI_DASHBOARD_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run `cisco-meraki-cli organizations get --all` to see which organizations the key can reach

### API-specific
- **HTTP 429 Too Many Requests during sync**  -  The Dashboard allows 10 requests per second per org; lower the request rate with --rate-limit 5 (requests per second) and, for sync, --concurrency 2, then rerun
- **HTTP 401 No valid authentication method found**  -  export MERAKI_DASHBOARD_API_KEY=<key> (Dashboard > Organization > API & Webhooks), then run cisco-meraki-cli doctor
- **An org is missing from fleet views or returns 404**  -  API access is disabled for that org; enable it under Organization > Settings > Dashboard API access
- **Your dashboard is in Canada, China, India or FedRAMP and calls fail**  -  Set CISCO_MERAKI_BASE_URL to https://api.meraki.ca/api/v1, https://api.meraki.cn/api/v1, https://api.meraki.in/api/v1 or https://api.gov-meraki.com/api/v1
- **fleet find or fleet changes with --data-source local returns nothing**  -  Populate the mirror first: cisco-meraki-cli sync --resources organizations (syncs orgs plus their networks, devices, inventory and change logs)

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**meraki/dashboard-api-python**](https://github.com/meraki/dashboard-api-python)  -  Python (351 stars)
- [**PackeTsar/meraki-cli**](https://github.com/PackeTsar/meraki-cli)  -  Python (101 stars)
- [**CiscoDevNet/meraki-magic-mcp-community**](https://github.com/CiscoDevNet/meraki-magic-mcp-community)  -  Python (45 stars)
- [**CiscoDevNet/cisco-meraki-mcp-official**](https://github.com/CiscoDevNet/cisco-meraki-mcp-official)  -  Python (14 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
