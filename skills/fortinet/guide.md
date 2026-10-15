# Fortinet CLI

**Read every FortiGate's configuration and live state, then audit the whole client fleet for firmware, SSL-VPN exposure, admin accounts and licence expiry in one command.**

Typed, read-only commands cover the FortiOS REST API (cmdb configuration tables and monitor endpoints) on any FortiGate. A devices file turns them into fleet commands: fleet audit grades every client box, fleet health shows operational state, and fleet backup plus fleet changes track configuration drift. fmg commands read FortiManager ADOMs and managed devices, and cloud commands read the FortiGate Cloud inventory.

Learn more at [Fortinet](https://www.fortinet.com/products/next-generation-firewall).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `fortinet-cli` and `fortinet-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/fortinet/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/fortinet/install.ps1 | iex
   ```
3. Verify: `fortinet-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=fortinet). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/fortinet --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/fortinet --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `fortinet-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the fortinet skill from https://github.com/Servosity/msp-skills/tree/main/skills/fortinet. The skill defines how its required CLI (`fortinet-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=fortinet).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in your FortiGate API URL (`FORTINET_BASE_URL`, e.g. `https://203.0.113.10:8443/api/v2`) and `FORTINET_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS (Apple Silicon and Intel), Linux (`amd64`) and Windows (`amd64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/fortinet/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/fortinet/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "fortinet": {
      "command": "fortinet-mcp",
      "env": {
        "FORTINET_BASE_URL": "https://<fortigate>/api/v2",
        "FORTINET_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Each FortiGate uses its own REST API administrator token (System > Administrators > Create New > REST API Admin, or `execute api-user generate-key <name>`), sent as `Authorization: Bearer`; your source IP must be one of that API user's trusted hosts. Single device: set FORTINET_BASE_URL=https://<fortigate>/api/v2 and FORTINET_API_TOKEN. Fleet: list devices in <config dir>/devices.toml (or the file named by FORTINET_DEVICES or --devices), each with token_file naming a file that holds that device's token (on macOS and Linux the CLI refuses it unless chmod 600; on Windows protect it with NTFS permissions); token values never go in the devices file. Without a devices file, fleet and audit commands check only the single FORTINET_BASE_URL device. Self-signed certificates: install the device CA, or opt in with --insecure, FORTINET_SKIP_TLS_VERIFY=true, or insecure = true for one device in devices.toml. FortiManager: FORTINET_FMG_URL plus an API key in FORTINET_FMG_API_KEY (FortiManager 7.2.2 or later; FORTINET_FMG_INSECURE=true for a self-signed certificate). FortiGate Cloud: a FortiCloud IAM API user in FORTINET_CLOUD_API_USER and FORTINET_CLOUD_API_PASSWORD, FORTINET_CLOUD_REGION (global, us, eu or jp) and, with FortiCloud Organizations, FORTINET_CLOUD_ACCOUNT_ID; repeated failed logins lock the FortiCloud account for 15-30 minutes, so the CLI caches the token and logs in at most once per run.

## Quick Start

```bash
# Confirms the binary runs, without calling any device.
fortinet-cli doctor --dry-run

# Proves the token and trusted host work on the FORTINET_BASE_URL device: model, hostname, serial and FortiOS version.
fortinet-cli monitor status --agent

# Lists the devices the fleet commands will query and whether each one is ready (URL and token resolved).
fortinet-cli fleet devices

# Runs the four posture checks across every device.
fortinet-cli fleet audit --agent

# Builds the renewal queue for licences expiring in the next 60 days.
fortinet-cli audit licenses --within 60d

```

## Unique Features

### Fleet security posture
- **`fleet audit`**  -  One PASS/WARN/FAIL/UNKNOWN matrix across every FortiGate in your devices file for firmware, SSL-VPN exposure, admin accounts and FortiGuard licence expiry.

  _Reach for this when asked which client firewalls are at risk; one call replaces dozens of per-device lookups, and --fail-on fail makes it exit 6 on any FAIL or unchecked (UNKNOWN) device for cron or CI._

  ```bash
  fortinet-cli fleet audit --new --agent
  ```
- **`audit firmware`**  -  Flags FortiGates below your per-branch firmware floor and how many patches each is behind the newest image offered in its branch.

  _Use it after a Fortinet PSIRT advisory to list exactly which boxes are below your patched floor; run 'monitor upgrade-paths' on a device for its supported upgrade hops._

  ```bash
  fortinet-cli audit firmware --min-version 7.2.10,7.4.5 --agent
  ```
- **`audit sslvpn`**  -  Grades each FortiGate and VDOM on whether SSL-VPN is enabled, listening on a WAN interface, and open to any source address.

  _SSL-VPN is the most exploited FortiGate surface; this answers 'who is exposed' across clients in one call._

  ```bash
  fortinet-cli audit sslvpn --agent
  ```
- **`audit admins`**  -  Flags admins with no trusted hosts (FAIL when HTTPS/SSH management is open on WAN), super_admin accounts without two-factor, WAN interfaces allowing HTTP or Telnet, and API users trusted from broad networks or using super_admin.

  _Use it for access reviews and after staff changes; it shows which admin accounts are a real exposure, not just present._

  ```bash
  fortinet-cli audit admins --broad-prefix 16 --agent
  ```
- **`audit licenses`**  -  One renewal queue of FortiGuard, FortiCare and service licences across the fleet, sorted by days left.

  _Use it monthly for renewals so no client silently loses FortiGuard protection._

  ```bash
  fortinet-cli audit licenses --within 60d --agent
  ```

### Fleet operations
- **`fleet health`**  -  One PASS/WARN/FAIL/UNKNOWN row per FortiGate: HA config sync, IPsec tunnels down, SD-WAN members down, CPU and memory pressure, expiring certificates; anything it could not read makes the row UNKNOWN.

  _Use it for the weekly NOC check or first thing in an incident to see which boxes are unhealthy._

  ```bash
  fortinet-cli fleet health --agent
  ```
- **`policies hygiene`**  -  Lists unused, any-any accept, logging-disabled and disabled firewall policies per VDOM.

  _Use it before a policy cleanup or audit to find dead and risky rules without exporting anything._

  ```bash
  fortinet-cli policies hygiene --unused-days 90 --agent
  ```
- **`fleet changes`**  -  Shows which FortiOS objects were added, removed or modified between two saved 'fleet backup' files (needs at least two backups per device; secrets are redacted, so a secret-only change is not reported).

  _Use it to answer 'what changed on this client's firewall this week' during change review or after an incident._

  ```bash
  fortinet-cli fleet changes --since 7d --agent
  ```

## Recipes

### Weekly posture diff

```bash
fortinet-cli fleet audit --new --agent
```

Shows only findings that were not present in the previous run; add --fail-on fail to exit 6 on any FAIL or UNKNOWN.

### FortiManager firmware and sync view

```bash
fortinet-cli fmg devices --agent --select devices.adom,devices.name,devices.firmware,devices.conn_status,devices.conf_status
```

Lists every managed FortiGate per client ADOM with only the firmware and sync fields an agent needs.

### SSL-VPN exposure across clients

```bash
fortinet-cli audit sslvpn --agent
```

Grades every device and VDOM on SSL-VPN status, WAN listening and source restriction.

### Dead and risky policies

```bash
fortinet-cli policies hygiene --unused-days 90 --agent
```

Joins policy config with hit counters to list unused, any-any and logging-disabled rules.

### Back up every FortiGate, then see what changed

```bash
fortinet-cli fleet backup
```

Writes dated 0600 config files under <data dir>/backups (they contain encrypted secrets); after the next scheduled backup, fleet changes --since 7d lists added, removed and modified objects.

## Usage

Run `fortinet-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml`, `devices.toml` (fleet inventory) and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, `backups/` (from `fleet backup`), `fleet_audit.sqlite` (audit history for `fleet audit --new`) |
| `state` | Runtime state such as persisted queries, `teach.log` and `fortigate-cloud-token.json` (cached FortiGate Cloud token, 0600) |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `FORTINET_CONFIG_DIR`, `FORTINET_DATA_DIR`, `FORTINET_STATE_DIR`, or `FORTINET_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `FORTINET_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export FORTINET_HOME=/srv/fortinet
fortinet-cli doctor
```

Under `FORTINET_HOME=/srv/fortinet`, the four dirs resolve to `/srv/fortinet/config`, `/srv/fortinet/data`, `/srv/fortinet/state`, and `/srv/fortinet/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "fortinet": {
      "command": "fortinet-mcp",
      "env": {
        "FORTINET_HOME": "/srv/fortinet"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `FORTINET_DATA_DIR` overrides an explicit `--home` for that kind. Use `FORTINET_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `FORTINET_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `fortinet-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### address_groups

Manage address groups

- **`fortinet-cli address-groups get`** - Get one address-groups entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall/addrgrp/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall_addrgrp` (mkey `name`) and plugins/modules/fortios_firewall_addrgrp.py DOCUMENTATION for fields.
- **`fortinet-cli address-groups list`** - Firewall address groups (firewall addrgrp). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall/addrgrp returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall_addrgrp` (mkey `name`) and plugins/modules/fortios_firewall_addrgrp.py DOCUMENTATION for fields.

### addresses

Manage addresses

- **`fortinet-cli addresses get`** - Get one addresses entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall/address/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall_address` (mkey `name`) and plugins/modules/fortios_firewall_address.py DOCUMENTATION for fields.
- **`fortinet-cli addresses list`** - Firewall address objects (firewall address). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall/address returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall_address` (mkey `name`) and plugins/modules/fortios_firewall_address.py DOCUMENTATION for fields.

### admin_profiles

Manage admin profiles

- **`fortinet-cli admin-profiles get`** - Get one admin-profiles entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/accprofile/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_accprofile` (mkey `name`) and plugins/modules/fortios_system_accprofile.py DOCUMENTATION for fields.
- **`fortinet-cli admin-profiles list`** - Administrator access profiles (system accprofile) with read/read-write permission groups. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/accprofile returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_accprofile` (mkey `name`) and plugins/modules/fortios_system_accprofile.py DOCUMENTATION for fields.

### administrators

Manage administrators

- **`fortinet-cli administrators get`** - Get one administrators entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/admin/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_admin` (mkey `name`) and plugins/modules/fortios_system_admin.py DOCUMENTATION for fields.
- **`fortinet-cli administrators list`** - Administrator accounts (system admin). Read trusthost1-10 and two-factor to spot admins reachable from anywhere without MFA. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/admin returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_admin` (mkey `name`) and plugins/modules/fortios_system_admin.py DOCUMENTATION for fields.

### api_users

Manage api users

- **`fortinet-cli api-users get`** - Get one api-users entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/api-user/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_api-user` (mkey `name`) and plugins/modules/fortios_system_api_user.py DOCUMENTATION for fields.
- **`fortinet-cli api-users list`** - REST API administrators (system api-user) and their trusted hosts. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/api-user returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_api-user` (mkey `name`) and plugins/modules/fortios_system_api_user.py DOCUMENTATION for fields.

### dhcp_servers

Manage dhcp servers

- **`fortinet-cli dhcp-servers get`** - Get one dhcp-servers entry by id. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system.dhcp/server/<id> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system.dhcp_server` (mkey `id`) and plugins/modules/fortios_system_dhcp_server.py DOCUMENTATION for fields.
- **`fortinet-cli dhcp-servers list`** - DHCP server scopes (system dhcp server). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system.dhcp/server returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system.dhcp_server` (mkey `id`) and plugins/modules/fortios_system_dhcp_server.py DOCUMENTATION for fields.

### interfaces

Manage interfaces

- **`fortinet-cli interfaces get`** - Get one interfaces entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/interface/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_interface` (mkey `name`) and plugins/modules/fortios_system_interface.py DOCUMENTATION for fields.
- **`fortinet-cli interfaces list`** - Interface configuration (system interface), including allowaccess (which management protocols listen on each interface). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/interface returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_interface` (mkey `name`) and plugins/modules/fortios_system_interface.py DOCUMENTATION for fields.

### ipsec_phase1

Manage ipsec phase1

- **`fortinet-cli ipsec-phase1 get`** - Get one ipsec-phase1 entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/vpn.ipsec/phase1-interface/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `vpn.ipsec_phase1-interface` (mkey `name`) and plugins/modules/fortios_vpn_ipsec_phase1_interface.py DOCUMENTATION for fields.
- **`fortinet-cli ipsec-phase1 list`** - IPsec phase 1 (route-based) tunnel definitions (vpn ipsec phase1-interface). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/vpn.ipsec/phase1-interface returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `vpn.ipsec_phase1-interface` (mkey `name`) and plugins/modules/fortios_vpn_ipsec_phase1_interface.py DOCUMENTATION for fields.

### ipsec_phase2

Manage ipsec phase2

- **`fortinet-cli ipsec-phase2 get`** - Get one ipsec-phase2 entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/vpn.ipsec/phase2-interface/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `vpn.ipsec_phase2-interface` (mkey `name`) and plugins/modules/fortios_vpn_ipsec_phase2_interface.py DOCUMENTATION for fields.
- **`fortinet-cli ipsec-phase2 list`** - IPsec phase 2 selectors (vpn ipsec phase2-interface). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/vpn.ipsec/phase2-interface returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `vpn.ipsec_phase2-interface` (mkey `name`) and plugins/modules/fortios_vpn_ipsec_phase2_interface.py DOCUMENTATION for fields.

### ldap_servers

Manage ldap servers

- **`fortinet-cli ldap-servers get`** - Get one ldap-servers entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/user/ldap/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `user_ldap` (mkey `name`) and plugins/modules/fortios_user_ldap.py DOCUMENTATION for fields.
- **`fortinet-cli ldap-servers list`** - LDAP authentication servers (user ldap). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/user/ldap returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `user_ldap` (mkey `name`) and plugins/modules/fortios_user_ldap.py DOCUMENTATION for fields.

### local_in_policies

Manage local in policies

- **`fortinet-cli local-in-policies get`** - Get one local-in-policies entry by policyid. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall/local-in-policy/<policyid> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall_local-in-policy` (mkey `policyid`) and plugins/modules/fortios_firewall_local_in_policy.py DOCUMENTATION for fields.
- **`fortinet-cli local-in-policies list`** - Local-in policies (firewall local-in-policy) that restrict traffic to the FortiGate itself. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall/local-in-policy returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall_local-in-policy` (mkey `policyid`) and plugins/modules/fortios_firewall_local_in_policy.py DOCUMENTATION for fields.

### local_users

Manage local users

- **`fortinet-cli local-users get`** - Get one local-users entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/user/local/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `user_local` (mkey `name`) and plugins/modules/fortios_user_local.py DOCUMENTATION for fields.
- **`fortinet-cli local-users list`** - Local firewall users (user local), including two-factor state. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/user/local returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `user_local` (mkey `name`) and plugins/modules/fortios_user_local.py DOCUMENTATION for fields.

### monitor

Manage monitor

- **`fortinet-cli monitor admin-sessions`** - Administrators currently logged in. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/current-admins. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_current-admins`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor aps`** - FortiAPs managed by this FortiGate. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/wifi/managed_ap. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `wifi_managed_ap`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor auth-users`** - Users currently authenticated through firewall policies. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/user/firewall. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `user_firewall`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor central-management`** - Connection status to FortiManager / central management. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/central-management/status. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_central-management_status`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor certificates`** - Local, CA and remote certificates with validity window. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/available-certificates. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_available-certificates`. Response fields: WagnerAG/checkmk_fortigate cmk_addons/plugins/fortios/agent_based/fortios_certificates.py (Certificate).
- **`fortinet-cli monitor config-revisions`** - Saved configuration revisions. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/config-revision. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_config-revision`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor dhcp-leases`** - Active DHCP leases. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/dhcp. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_dhcp`. Response fields: WagnerAG/checkmk_fortigate cmk_addons/plugins/fortios/agent_based/fortios_dhcp_lease.py (ip, mac, status, server_mkey).
- **`fortinet-cli monitor firmware-images`** - Running firmware image and the images FortiGuard offers for upgrade. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/firmware. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_firmware`. Response fields: WagnerAG/checkmk_fortigate cmk_addons/plugins/fortios/agent_based/fortios_firmware.py (FirmwareImage, FirmwareResults).
- **`fortinet-cli monitor fortiguard`** - FortiGuard server list and reachability. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/fortiguard/server-info. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_fortiguard_server-info`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor fortitokens`** - FortiToken inventory and activation state. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/user/fortitoken. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `user_fortitoken`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor ha-checksums`** - Per-member configuration checksums; mismatches mean HA is out of sync. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/ha-checksums. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_ha-checksums`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor ha-peers`** - HA cluster members and which unit is primary. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/ha-peer. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_ha-peer`. Response fields: WagnerAG/checkmk_fortigate cmk_addons/plugins/fortios/agent_based/fortios_ha_peer.py (HAPeer).
- **`fortinet-cli monitor interfaces`** - Live interface link, speed, counters. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/interface. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_interface`. Response fields: WagnerAG/checkmk_fortigate cmk_addons/plugins/fortios/agent_based/fortios_interface.py (Interface).
- **`fortinet-cli monitor ipsec`** - IPsec tunnels with phase-2 selector status. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/vpn/ipsec. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `vpn_ipsec`. Response fields: WagnerAG/checkmk_fortigate cmk_addons/plugins/fortios/agent_based/fortios_ipsec.py (ProxySource, tunnel fields).
- **`fortinet-cli monitor license-status`** - Licence and FortiGuard/FortiCare entitlement status with expiry epochs. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/license/status. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `license_status`. Response fields: WagnerAG/checkmk_fortigate cmk_addons/plugins/fortios/agent_based/fortios_license.py (FortiGuardModule, FortiCareModule, AntivirusModule, ...).
- **`fortinet-cli monitor log-disk`** - Local log disk usage. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/log/current-disk-usage. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `log_current-disk-usage`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor ntp`** - NTP server reachability. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/ntp/status. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_ntp_status`. Response fields: WagnerAG/checkmk_fortigate cmk_addons/plugins/fortios/agent_based/fortios_ntp.py.
- **`fortinet-cli monitor performance`** - Performance snapshot (CPU, memory, sessions, uptime). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/performance/status. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_performance_status`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor policy-stats`** - Per-policy hit counters, bytes and last-used timestamps (monitor firewall policy). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/firewall/policy. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `firewall_policy`. Response fields: Claudio-Vidhi/SentinelNet services/fortigate_service.py get_policies_with_stats (joins on policyid; hit_count, bytes, active_sessions, last_used; AGPL, behavior evidence only).
- **`fortinet-cli monitor resources`** - CPU, memory, session and disk usage. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/resource/usage. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_resource_usage`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor routes`** - IPv4 routing table (all route types). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/router/ipv4. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `router_ipv4`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor sdwan-health`** - SD-WAN performance SLA health-check results per member. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/virtual-wan/health-check. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `virtual-wan_health-check`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor sdwan-members`** - SD-WAN member interface state. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/virtual-wan/members. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `virtual-wan_members`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor sslvpn-sessions`** - Connected SSL-VPN users and their tunnel/web sub-sessions. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/vpn/ssl. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `vpn_ssl`. Response fields: WagnerAG/checkmk_fortigate cmk_addons/plugins/fortios/agent_based/fortios_sslvpn.py (Session, Subsession).
- **`fortinet-cli monitor sslvpn-stats`** - SSL-VPN usage statistics. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/vpn/ssl/stats. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `vpn_ssl_stats`. Response fields are not publicly documented; results are passed through untyped.
- **`fortinet-cli monitor status`** - System status: model, hostname; serial, FortiOS version and build are in the envelope. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/status. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_status`. Response fields: WagnerAG/checkmk_fortigate cmk_addons/plugins/fortios/agent_based/fortios_identity.py, fortios_device_info_inventory.py.
- **`fortinet-cli monitor switches`** - FortiSwitches managed by this FortiGate. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/switch-controller/managed-switch/status. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `switch-controller_managed-switch_status`. Response fields: WagnerAG/checkmk_fortigate cmk_addons/plugins/fortios/agent_based/fortios_managed_switch.py.
- **`fortinet-cli monitor upgrade-paths`** - Supported firmware upgrade paths for this model. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/monitor/system/firmware/upgrade-paths. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_monitor_fact.py selector `system_firmware_upgrade-paths`. Response fields are not publicly documented; results are passed through untyped.

### policies

Manage policies

- **`fortinet-cli policies get`** - Get one policies entry by policyid. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall/policy/<policyid> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall_policy` (mkey `policyid`) and plugins/modules/fortios_firewall_policy.py DOCUMENTATION for fields.
- **`fortinet-cli policies list`** - IPv4/IPv6 firewall policies (firewall policy) in sequence order. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall/policy returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall_policy` (mkey `policyid`) and plugins/modules/fortios_firewall_policy.py DOCUMENTATION for fields.

### radius_servers

Manage radius servers

- **`fortinet-cli radius-servers get`** - Get one radius-servers entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/user/radius/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `user_radius` (mkey `name`) and plugins/modules/fortios_user_radius.py DOCUMENTATION for fields.
- **`fortinet-cli radius-servers list`** - RADIUS authentication servers (user radius). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/user/radius returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `user_radius` (mkey `name`) and plugins/modules/fortios_user_radius.py DOCUMENTATION for fields.

### service_groups

Manage service groups

- **`fortinet-cli service-groups get`** - Get one service-groups entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall.service/group/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall.service_group` (mkey `name`) and plugins/modules/fortios_firewall_service_group.py DOCUMENTATION for fields.
- **`fortinet-cli service-groups list`** - Firewall service groups (firewall service group). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall.service/group returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall.service_group` (mkey `name`) and plugins/modules/fortios_firewall_service_group.py DOCUMENTATION for fields.

### services

Manage services

- **`fortinet-cli services get`** - Get one services entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall.service/custom/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall.service_custom` (mkey `name`) and plugins/modules/fortios_firewall_service_custom.py DOCUMENTATION for fields.
- **`fortinet-cli services list`** - Custom firewall services (firewall service custom). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall.service/custom returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall.service_custom` (mkey `name`) and plugins/modules/fortios_firewall_service_custom.py DOCUMENTATION for fields.

### settings

Manage settings

- **`fortinet-cli settings autoupdate`** - FortiGuard definitions auto-update schedule (system autoupdate schedule). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system.autoupdate/schedule returns a single object in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system.autoupdate_schedule` (no mkey) and plugins/modules/fortios_system_autoupdate_schedule.py DOCUMENTATION for fields.
- **`fortinet-cli settings central-management`** - Central management settings (system central-management): FortiManager / FortiGate Cloud link. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/central-management returns a single object in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_central-management` (no mkey) and plugins/modules/fortios_system_central_management.py DOCUMENTATION for fields.
- **`fortinet-cli settings dns`** - DNS resolver settings (system dns). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/dns returns a single object in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_dns` (no mkey) and plugins/modules/fortios_system_dns.py DOCUMENTATION for fields.
- **`fortinet-cli settings fortianalyzer`** - FortiAnalyzer log forwarding (log fortianalyzer setting). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/log.fortianalyzer/setting returns a single object in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `log.fortianalyzer_setting` (no mkey) and plugins/modules/fortios_log_fortianalyzer_setting.py DOCUMENTATION for fields.
- **`fortinet-cli settings fortiguard`** - FortiGuard update and rating settings (system fortiguard), including automatic firmware upgrade. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/fortiguard returns a single object in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_fortiguard` (no mkey) and plugins/modules/fortios_system_fortiguard.py DOCUMENTATION for fields.
- **`fortinet-cli settings global`** - Global system settings (system global): hostname, admin ports, admin lockout and timeout. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/global returns a single object in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_global` (no mkey) and plugins/modules/fortios_system_global.py DOCUMENTATION for fields.
- **`fortinet-cli settings ha`** - High-availability configuration (system ha). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/ha returns a single object in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_ha` (no mkey) and plugins/modules/fortios_system_ha.py DOCUMENTATION for fields.
- **`fortinet-cli settings ntp`** - NTP settings (system ntp). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/ntp returns a single object in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_ntp` (no mkey) and plugins/modules/fortios_system_ntp.py DOCUMENTATION for fields.
- **`fortinet-cli settings password-policy`** - Administrator password policy (system password-policy). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/password-policy returns a single object in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_password-policy` (no mkey) and plugins/modules/fortios_system_password_policy.py DOCUMENTATION for fields.
- **`fortinet-cli settings syslog`** - Syslog forwarding (log syslogd setting). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/log.syslogd/setting returns a single object in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `log.syslogd_setting` (no mkey) and plugins/modules/fortios_log_syslogd_setting.py DOCUMENTATION for fields.
- **`fortinet-cli settings vpn-ssl`** - SSL-VPN settings (vpn ssl settings): status, listening port, source interfaces and source addresses. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/vpn.ssl/settings returns a single object in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `vpn.ssl_settings` (no mkey) and plugins/modules/fortios_vpn_ssl_settings.py DOCUMENTATION for fields.

### sslvpn_portals

Manage sslvpn portals

- **`fortinet-cli sslvpn-portals get`** - Get one sslvpn-portals entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/vpn.ssl.web/portal/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `vpn.ssl.web_portal` (mkey `name`) and plugins/modules/fortios_vpn_ssl_web_portal.py DOCUMENTATION for fields.
- **`fortinet-cli sslvpn-portals list`** - SSL-VPN web portals (vpn ssl web portal). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/vpn.ssl.web/portal returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `vpn.ssl.web_portal` (mkey `name`) and plugins/modules/fortios_vpn_ssl_web_portal.py DOCUMENTATION for fields.

### static_routes

Manage static routes

- **`fortinet-cli static-routes get`** - Get one static-routes entry by seq-num. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/router/static/<seq-num> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `router_static` (mkey `seq_num`) and plugins/modules/fortios_router_static.py DOCUMENTATION for fields.
- **`fortinet-cli static-routes list`** - Static routes (router static). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/router/static returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `router_static` (mkey `seq_num`) and plugins/modules/fortios_router_static.py DOCUMENTATION for fields.

### user_groups

Manage user groups

- **`fortinet-cli user-groups get`** - Get one user-groups entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/user/group/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `user_group` (mkey `name`) and plugins/modules/fortios_user_group.py DOCUMENTATION for fields.
- **`fortinet-cli user-groups list`** - User groups (user group) used in policies and SSL-VPN authentication rules. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/user/group returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `user_group` (mkey `name`) and plugins/modules/fortios_user_group.py DOCUMENTATION for fields.

### vdoms

Manage vdoms

- **`fortinet-cli vdoms get`** - Get one vdoms entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/vdom/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_vdom` (mkey `name`) and plugins/modules/fortios_system_vdom.py DOCUMENTATION for fields.
- **`fortinet-cli vdoms list`** - Virtual domains configured on the FortiGate (system vdom). Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/system/vdom returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `system_vdom` (mkey `name`) and plugins/modules/fortios_system_vdom.py DOCUMENTATION for fields.

### vips

Manage vips

- **`fortinet-cli vips get`** - Get one vips entry by name. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall/vip/<name> (URL-encoded) returns a one-element `results` array. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall_vip` (mkey `name`) and plugins/modules/fortios_firewall_vip.py DOCUMENTATION for fields.
- **`fortinet-cli vips list`** - Virtual IPs / port forwards (firewall vip): which internal hosts are published to the internet. Read-only call to the FortiOS REST API on one FortiGate; the response envelope carries the results together with the device serial number, FortiOS version and build. GET /api/v2/cmdb/firewall/vip returns every row in `results`. Source: fortinet-ansible-dev/ansible-galaxy-fortios-collection plugins/modules/fortios_configuration_fact.py selector `firewall_vip` (mkey `name`) and plugins/modules/fortios_firewall_vip.py DOCUMENTATION for fields.


### audit


- **`fortinet-cli audit firmware`** - Flags FortiGates below your per-branch firmware floor and how many patches each is behind the newest image offered in its branch.

- **`fortinet-cli audit sslvpn`** - Grades each FortiGate and VDOM on whether SSL-VPN is enabled, listening on a WAN interface, and open to any source address.

- **`fortinet-cli audit admins`** - Flags admins with no trusted hosts (FAIL when HTTPS/SSH management is open on WAN), super_admin accounts without two-factor, WAN interfaces allowing HTTP or Telnet, and API users trusted from broad networks or using super_admin.

- **`fortinet-cli audit licenses`** - One renewal queue of FortiGuard, FortiCare and service licences across the fleet, sorted by days left.


### fleet


- **`fortinet-cli fleet audit`** - One PASS/WARN/FAIL/UNKNOWN matrix across every FortiGate in your devices file for firmware, SSL-VPN exposure, admin accounts and FortiGuard licence expiry.

- **`fortinet-cli fleet health`** - One PASS/WARN/FAIL/UNKNOWN row per FortiGate: HA config sync, IPsec tunnels down, SD-WAN members down, CPU and memory pressure, expiring certificates; anything it could not read makes the row UNKNOWN.

- **`fortinet-cli fleet changes`** - Shows which FortiOS objects were added, removed or modified between two saved 'fleet backup' files (needs at least two backups per device; secrets are redacted, so a secret-only change is not reported).


### policies


- **`fortinet-cli policies hygiene`** - Lists unused, any-any accept, logging-disabled and disabled firewall policies per VDOM.




### Inventory, backups and many-client views

- **`fortinet-cli fleet devices`** - List the devices the fleet commands will query (devices file, or the single `FORTINET_BASE_URL` device) and whether each is ready; token values are never printed.
- **`fortinet-cli fleet backup`** - Download each FortiGate's full configuration into dated 0600 files under `<data dir>/backups` (or `--out-dir`); backups contain encrypted secrets. Exits 5 if any backup fails.
- **`fortinet-cli fmg status`** / **`fmg adoms`** / **`fmg devices`** - Read-only FortiManager view (needs `FORTINET_FMG_URL` and `FORTINET_FMG_API_KEY`): version, client ADOMs, and managed FortiGates with firmware, connection and config-sync status.
- **`fortinet-cli cloud devices`** / **`cloud device <serial>`** - Read-only FortiGate Cloud inventory (needs `FORTINET_CLOUD_API_USER` and `FORTINET_CLOUD_API_PASSWORD`, plus `FORTINET_CLOUD_REGION` / `FORTINET_CLOUD_ACCOUNT_ID` when applicable).

Without a devices file, `fleet *`, `audit *` and `policies hygiene` check only the `FORTINET_BASE_URL` device.

### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`fortinet-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`fortinet-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`fortinet-cli learnings list`** - Inspect taught rows
- **`fortinet-cli learnings forget <query>`** - Undo a teach
- **`fortinet-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`fortinet-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`fortinet-cli teach-pattern`** - Install a query/resource template up front
- **`fortinet-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `FORTINET_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `fortinet-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
fortinet-cli address-groups list

# JSON for scripting and agents
fortinet-cli address-groups list --json
# Filter to specific fields
fortinet-cli address-groups list --json --select category,color,comment

# Dry run  -  show the request without sending
fortinet-cli address-groups list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
fortinet-cli address-groups list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending (fleet, audit, policies hygiene, fmg and cloud commands report the action they would run)
- **Read-only** - never creates, updates or deletes anything on FortiGate, FortiManager or FortiGate Cloud; local writes only (backups, audit history, the sync store, profiles)
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` other error, `2` usage error, `3` not found, `4` auth error, `5` API error (including a failed `fleet backup`), `6` findings at or above `--fail-on` (only `fleet audit` and `audit *`, only when `--fail-on` is passed; UNKNOWN counts), `7` rate limited, `10` config error.

## Health Check

```bash
fortinet-cli doctor
```

Verifies configuration, credentials, and connectivity for the single `FORTINET_BASE_URL` FortiGate. For other setups use `fortinet-cli fleet devices` (devices file), `fmg status` (FortiManager) and `cloud devices` (FortiGate Cloud).

## Configuration

Run `fortinet-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/fortinet-cli/config.toml`; `--home`, `FORTINET_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `FORTINET_BASE_URL` | config | Single device | FortiGate REST API root, e.g. `https://203.0.113.10:8443/api/v2` (the built-in default is a placeholder). |
| `FORTINET_API_TOKEN` | per_call | Single device | REST API administrator token for that FortiGate. |
| `FORTINET_DEVICES` | config | Fleet | Path to the devices file (default `<config dir>/devices.toml`); each device names its own `token_file` (chmod 600). |
| `FORTINET_SKIP_TLS_VERIFY` | config | No | `true` accepts self-signed certificates (same as `--insecure`). |
| `FORTINET_FMG_URL` | config | FortiManager | `https://<fortimanager>`. |
| `FORTINET_FMG_API_KEY` | per_call | FortiManager | REST API admin key (FortiManager 7.2.2 or later). |
| `FORTINET_FMG_INSECURE` | config | No | `true` accepts a self-signed FortiManager certificate. |
| `FORTINET_CLOUD_API_USER` | per_call | FortiGate Cloud | FortiCloud IAM API user ID. |
| `FORTINET_CLOUD_API_PASSWORD` | per_call | FortiGate Cloud | FortiCloud IAM API user password. |
| `FORTINET_CLOUD_REGION` | config | No | `global` (default), `us`, `eu` or `jp`: where the devices were provisioned. |
| `FORTINET_CLOUD_ACCOUNT_ID` | config | With FortiCloud Organizations | Account to read. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `fortinet-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors**
- Single device (exit code 4): run `fortinet-cli auth status` and `fortinet-cli doctor`; check the token is set without printing it: `[ -n "$FORTINET_API_TOKEN" ] && echo set`
- Fleet and audit commands: a rejected token is an UNKNOWN finding with code `unauthorized` (exit 0 unless `--fail-on` is passed)
- `fmg` / `cloud`: rejected credentials exit 4; missing settings exit 10
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized from a FortiGate**  -  The token is wrong or your IP is not a trusted host: add your source IP under System > Administrators > (your REST API admin) > Trusted Hosts. Fleet commands report this as an UNKNOWN finding with code unauthorized.
- **x509: certificate signed by unknown authority**  -  FortiGates ship a self-signed certificate: install the device CA, or pass --insecure (or FORTINET_SKIP_TLS_VERIFY=true); in devices.toml set insecure = true for that device only.
- **403 Forbidden on a table, or UNKNOWN / not-checked items in fleet health**  -  The REST API admin profile lacks read access to that area; grant read permission to System and monitor data in its admin profile (and Firewall, so `audit sslvpn` can resolve named SSL-VPN source addresses).
- **fleet audit shows UNKNOWN for a device**  -  The device could not be read (timeout, 401, TLS); run fortinet-cli fleet devices, then retry that device alone with --device <name>.
- **fmg command fails with status -11**  -  FortiManager answers -11 for both a bad API key and missing permission: check FORTINET_FMG_API_KEY, the API admin's profile and ADOM scope, and its trusted hosts.
- **FortiGate Cloud login returns invalid_grant**  -  Check FORTINET_CLOUD_API_USER and FORTINET_CLOUD_API_PASSWORD; repeated failures lock the FortiCloud account for 15-30 minutes, so wait before retrying.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**ansible-galaxy-fortios-collection**](https://github.com/fortinet-ansible-dev/ansible-galaxy-fortios-collection)  -  Python (106 stars)
- [**fortigate-mcp-server**](https://github.com/alpadalar/fortigate-mcp-server)  -  Python (37 stars)
- [**ansible-galaxy-fortimanager-collection**](https://github.com/fortinet-ansible-dev/ansible-galaxy-fortimanager-collection)  -  Python (22 stars)
- [**fotoobo**](https://github.com/migros/fotoobo)  -  Python (21 stars)
- [**checkmk_fortigate**](https://github.com/WagnerAG/checkmk_fortigate)  -  Python (15 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
