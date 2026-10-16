# SonicWall CLI

**Cyber-insurance posture for a whole SonicWall fleet: firmware, advisories, MFA, SSL-VPN exposure and licences, one row per firewall.**

Reads Network Security Manager (NSM SaaS) and MySonicWall with one MySonicWall API key. 'posture' answers the insurer questionnaire per firewall, 'exposure' triages a SonicWall advisory down to the firewalls with SSL-VPN reachable on WAN, and 'mfa-audit' lists every local account without a one-time password and flags firewalls whose users sign in through SAML or a directory. Read-only by design.

Learn more at [SonicWall](https://www.sonicwall.com).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `sonicwall-cli` and `sonicwall-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.ps1 | iex
   ```
3. Verify: `sonicwall-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=sonicwall). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/sonicwall --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/sonicwall --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `sonicwall-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the sonicwall skill from https://github.com/Servosity/msp-skills/tree/main/skills/sonicwall. The skill defines how its required CLI (`sonicwall-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=sonicwall).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `SONICWALL_MSW_API_KEY` (and `SONICWALL_REGION` for EU portals) when Claude Desktop prompts you. Leave `SONICWALL_NSM_TOKEN` empty: run `sonicwall-cli nsm-login` in a terminal instead, and again whenever NSM tools exit 4 (the token lasts 8 hours, 15 minutes idle). A pasted token would expire and keep overriding the saved one.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/sonicwall/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`%APPDATA%\Claude\claude_desktop_config.json` on Windows, `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "sonicwall": {
      "command": "sonicwall-mcp",
      "env": {
        "SONICWALL_REGION": "uswest",
        "SONICWALL_MSW_API_KEY": "<your MySonicWall API key>"
      }
    }
  }
}
```

</details>

## Authentication

You need one credential: a MySonicWall API key (MySonicWall > My Workspace > User Groups > User list > Generate My API Key; valid up to a year, and MSW API permission must be enabled for your user). Export it as SONICWALL_MSW_API_KEY; MySonicWall commands, licence checks and blind-spots read it from the environment on every run. NSM calls need a separate NSM session token minted from that key: run 'sonicwall-cli nsm-login' (add --tenant "<name or product group ID>" when the key sees more than one NSM tenant, the usual MSP case) and it performs SonicWall's three-step exchange and saves the token. The token lasts 8 hours with a 15-minute idle timeout; when NSM calls exit 4, rerun nsm-login. Exit 4 from MySonicWall commands means the key is missing or not permitted. If SONICWALL_NSM_TOKEN is set it overrides the saved token. EU portals: export SONICWALL_REGION=eucentral (default uswest). For the MCP server, pass SONICWALL_MSW_API_KEY (and SONICWALL_REGION when not uswest) in the server's env block.

## Quick Start

```bash
# Mint and save the NSM session token from your MySonicWall API key
sonicwall-cli nsm-login

# Check config and verify the NSM token with one authenticated read (GET /tenants)
sonicwall-cli doctor

# Pull the fleet inventory and advisory matches into the local database
sonicwall-cli sync --resources devices,devices-inventory-tenant,vulnerabilities,tenants

# One insurance-posture row per firewall
sonicwall-cli posture --agent

# Which firewalls the SSL-VPN access-control advisory (CVE-2024-40766) hits, and which expose SSL-VPN on WAN
sonicwall-cli exposure SNWLID-2024-0015

```

## Unique Features

### Cyber-insurance evidence
- **`posture`**  -  One row per firewall with firmware currency, end-of-support date, published advisories, soonest licence expiry, admin TOTP, SSL-VPN exposure and SSL-VPN users without one-time passwords; each check is pass, warn, fail or unknown, and the licence check needs SONICWALL_MSW_API_KEY.

  _Reach for this when an insurer or compliance questionnaire asks whether the SonicWall fleet is patched, MFA-protected and licensed; it answers per firewall with pass, warn, fail or unknown._

  ```bash
  sonicwall-cli posture --agent
  ```
- **`licenses renewals`**  -  Every licence and security service expiring in the next N days (expired ones included), per firewall and client, flagged when the hardware is near end of support; needs SONICWALL_MSW_API_KEY.

  _Use it to build the renewal pipeline and spot boxes that should be replaced rather than renewed._

  ```bash
  sonicwall-cli licenses renewals --days 90 --agent
  ```
- **`mfa-audit`**  -  Per-account remediation list: admin TOTP per firewall and every local SSL-VPN user (direct or nested group member) without a one-time password, with SAML, RADIUS, LDAP or TACACS+ sign-in marked as delegated.

  _Use it when the question is which accounts lack MFA, the gap attackers used against SonicWall SSL-VPN in 2024 and 2025._

  ```bash
  sonicwall-cli mfa-audit --agent
  ```
- **`blind-spots`**  -  Find firewalls registered in MySonicWall that NSM cannot see, with their registered firmware's advisories and support expiry; needs SONICWALL_MSW_API_KEY and an NSM token.

  _Run it before trusting any posture report: firewalls outside NSM are invisible to every other command._

  ```bash
  sonicwall-cli blind-spots --agent
  ```

### Patch and exposure
- **`exposure`**  -  Rank the SonicWall PSIRT advisories NSM matched to your fleet, or name one advisory (SNWLID ID) to list every affected firewall and whether SSL-VPN is enabled on a zone whose name contains WAN; a CVE ID only matches when NSM's advisory text names it.

  _Use it the moment a SonicWall advisory drops (for example CVE-2024-40766) to pick the firewalls that need tonight's change window._

  ```bash
  sonicwall-cli exposure SNWLID-2024-0015 --agent
  ```
- **`firmware drift`**  -  See how far each firewall's running SonicOS build is behind the newest available build and which advisories the upgrade clears.

  _Use it for the weekly patch review instead of comparing firmware strings by hand._

  ```bash
  sonicwall-cli firmware drift --agent
  ```

### Local history
- **`posture changes`**  -  What changed in fleet posture between saved posture runs of the same scope (--fleet, --tenant or --serial picks it; the default is the newest run's): newly vulnerable, MFA turned off, SSL-VPN newly on WAN, licence lapsed, firmware fixed. Local only: run posture now and again later, with the same SONICWALL_MSW_API_KEY and NSM tenant.

  _Use it for the Monday question of what got worse since last week._

  ```bash
  sonicwall-cli posture changes --fleet --since 7d --agent
  ```

## Recipes

### Insurance evidence for one firewall

```bash
sonicwall-cli posture --serial 2CB8ED000001 --agent
```

Pass, warn, fail or unknown for firmware, advisories, end of support, licences, admin TOTP and SSL-VPN exposure on one firewall; warn marks firmware behind, SSL-VPN on WAN, support ending within a year, a licence within 30 days and delegated MFA. Drop --serial for the whole fleet.

### Advisory triage

```bash
sonicwall-cli exposure SNWLID-2024-0015 --agent
```

Lists firewalls matched to SonicWall advisory SNWLID-2024-0015 (CVE-2024-40766) and marks which have SSL-VPN enabled on a zone whose name contains WAN (custom-named internet zones are missed); an empty list for a CVE ID means no advisory text matched, not zero exposure.

### Lean inventory for an agent

```bash
sonicwall-cli devices list --agent --select serialNumber,friendlyName,firmwareVersion,liveStatus
```

Narrows the large device payload to the four fields most posture questions need.

### Renewal pipeline

```bash
sonicwall-cli licenses renewals --days 90 --agent
```

Returns {days, renewals[], fetch_failures[]} (inside .results under --agent; fetch_failures only when a lookup failed): every licence expiring within 90 days, already expired, or with no readable expiry (expiry_unknown), with replace_instead yes/no/unknown. A firewall in fetch_failures was not checked.

### What got worse this week

```bash
sonicwall-cli posture changes --fleet --since 7d --agent
```

Compares the newest saved posture run with the latest same-scope run at least 7 days old (or the oldest one). Reads local history only, so run posture first and again later.

## Usage

Run `sonicwall-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `SONICWALL_CONFIG_DIR`, `SONICWALL_DATA_DIR`, `SONICWALL_STATE_DIR`, or `SONICWALL_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `SONICWALL_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export SONICWALL_HOME=/srv/sonicwall
sonicwall-cli doctor
```

On Windows (PowerShell):

```powershell
$env:SONICWALL_HOME = "C:\ProgramData\sonicwall"
sonicwall-cli doctor
```

Under `SONICWALL_HOME=/srv/sonicwall`, the four dirs resolve to `/srv/sonicwall/config`, `/srv/sonicwall/data`, `/srv/sonicwall/state`, and `/srv/sonicwall/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "sonicwall": {
      "command": "sonicwall-mcp",
      "env": {
        "SONICWALL_HOME": "/srv/sonicwall"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `SONICWALL_DATA_DIR` overrides an explicit `--home` for that kind. Use `SONICWALL_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `SONICWALL_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `sonicwall-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### nsm-login / nsm-get

- **`sonicwall-cli nsm-login`** - Mint and save the NSM session token from SONICWALL_MSW_API_KEY (add `--tenant` when the key sees several NSM tenants).
- **`sonicwall-cli nsm-get /path --device <serial>`** - GET a vendor-documented NSM or SonicOS-passthrough read route that has no dedicated command (credential and state-changing routes are refused; credential fields are redacted).

### alerts

Manage alerts

- **`sonicwall-cli alerts count`** - Get alert count.
- **`sonicwall-cli alerts list`** - Get alerts log data.

### config-audit

Manage config audit

- **`sonicwall-cli config-audit count`** - To get config list count of device / devices for tenants.
- **`sonicwall-cli config-audit diff`** - To get config diff.
- **`sonicwall-cli config-audit list`** - To get config list of device / devices for tenants.

### device-groups

Manage device groups

- **`sonicwall-cli device-groups get`** - Gets device group information by group ID.
- **`sonicwall-cli device-groups list`** - Retrieves list of device groups by tenant.

### devices

Manage devices

- **`sonicwall-cli devices get`** - Gets device information by device serial number.
- **`sonicwall-cli devices inventory`** - Retrieves inventory list of device by tenant.
- **`sonicwall-cli devices lifecycle`** - Gets product life cycle information by device serial number. Life-cycle milestones for the device model (eos, lod, armb, lrmb, oneyldo).
- **`sonicwall-cli devices list`** - Retrieves list of device by tenant. Each device carries serialNumber, friendlyName, firmwareVersion, ipAddress, liveStatus, synchronized, fwUpgradeStatus and zeroTouchStatus.
- **`sonicwall-cli devices locations`** - Gets Devices info aggregated on location.
- **`sonicwall-cli devices online-status`** - Gets online status information by device serial number.
- **`sonicwall-cli devices summary`** - Retrieves summary of device list by tenant id.
- **`sonicwall-cli devices trend`** - Retrieves trend of device list.

### firewall

Manage firewall

- **`sonicwall-cli firewall access-rules`** - Retrieve an IPv4 access rule policy. IPv4 access rules (from/to zone, action, source/destination, service, enable).
- **`sonicwall-cli firewall address-objects`** - Retrieve IPv4 address object configuration.
- **`sonicwall-cli firewall administration`** - Retrieve administration configuration. Includes admin TOTP, password constraints, user lockout, web management and SonicOS API settings. Admin TOTP is administration.admin.one_time_password.totp.
- **`sonicwall-cli firewall license-report`** - Retrieve License status.
- **`sonicwall-cli firewall license-security-services`** - Retrieve License status.
- **`sonicwall-cli firewall local-groups`** - Retrieve user local group configuration.
- **`sonicwall-cli firewall local-users`** - Retrieve local user configuration. Local users with their one-time-password method (none, OTP via e-mail, or TOTP).
- **`sonicwall-cli firewall security-services-status`** - Retrieve security services status.
- **`sonicwall-cli firewall sslvpn-access`** - Retrieve SSL VPN server accesses configuration. Zones with SSL VPN access enabled; each access entry has a zone name and an enable flag.
- **`sonicwall-cli firewall sslvpn-portal`** - Retrieve SSL VPN portal configuration.
- **`sonicwall-cli firewall sslvpn-server`** - Retrieve SSL VPN server configuration.
- **`sonicwall-cli firewall sslvpn-sessions`** - Retrieve SSL VPN sessions statistics.
- **`sonicwall-cli firewall system-status`** - Retrieve system status.
- **`sonicwall-cli firewall user-auth`** - Retrieve user authentication base settings.

### firmware

Manage firmware

- **`sonicwall-cli firmware available`** - Gets new firmware information by device serial number. Newer firmware builds available for the device (availableVersions[] with version, releaseDate, releaseType).
- **`sonicwall-cli firmware available-list`** - get available firmware list for serialNumber.
- **`sonicwall-cli firmware devices-on-version`** - List the devices running a given firmware version. Lists the devices running the given firmware version (the vendor summary for this path is copied from another operation).
- **`sonicwall-cli firmware upgrade-history`** - get firmware upgrade status history.
- **`sonicwall-cli firmware upgrade-status`** - get latest firmware-upgrade status for serialNumber.
- **`sonicwall-cli firmware version-list`** - get firmware version list.

### licenses

Manage licenses

- **`sonicwall-cli licenses expired`** - Retrieves list of device with expired licenses by tenant.
- **`sonicwall-cli licenses expiring`** - Gets License expiration summary. Count of expiring licences per service (serviceCode, serviceName, count) within the next N days.
- **`sonicwall-cli licenses get`** - Gets license information by device serial number.
- **`sonicwall-cli licenses service-status`** - Gets whether license in enabled for this service of this serial number.
- **`sonicwall-cli licenses services`** - Gets status of services for a serial number.

### msw-activity

Manage msw activity

- **`sonicwall-cli msw-activity detail`** - View user activity log details.
- **`sonicwall-cli msw-activity export`** - View user activity logs for all the users from organization.

### msw-licenses

Manage msw licenses

- **`sonicwall-cli msw-licenses`** - License report.

### msw-products

Manage msw products

- **`sonicwall-cli msw-products applicable-services`** - Gets List of Applicable Products, Services and DataCenter Details for Provisioning a Prodduct or Editing a Provision.
- **`sonicwall-cli msw-products firmware`** - Get a product firmware versions, including latest available firmware.
- **`sonicwall-cli msw-products list`** - Get a list of Products.
- **`sonicwall-cli msw-products services`** - Get product details like license type, node counts and expiry date.

### msw-system

Manage msw system

- **`sonicwall-cli msw-system`** - GET /api/version.

### msw-tenants

Manage msw tenants

- **`sonicwall-cli msw-tenants access-code`** - Get the access code to generate API keys from your cloud subscriptions. Step 2 of the NSM token exchange. Body fields: tenantId (the productGroupID) and tileName ISNSMSAFEENABLED; the code is returned in content.accessCode. Prefer the `nsm-login` command.
- **`sonicwall-cli msw-tenants address`** - Get tenant address information.
- **`sonicwall-cli msw-tenants cloud`** - Get the list of tenantIds for all active cloud subscriptions. Step 1 of the NSM token exchange. Each tenant carries productGroupID and cloudServices[].tenantSerial under content.arrTenants[].
- **`sonicwall-cli msw-tenants firewalls`** - Returns a  list of eligible firewalls for a given tenantId (productgroupId). When called with callerService=CSE and a valid system API-key.
- **`sonicwall-cli msw-tenants list`** - Get a list of tenants.

### msw-users

Manage msw users

- **`sonicwall-cli msw-users groups`** - Get a list of user groups.
- **`sonicwall-cli msw-users list`** - Get a list of users and associated user groups.

### nsm-session

Manage nsm session

- **`sonicwall-cli nsm-session refresh`** - Provides a new session token. Invalidates the old token. Returns a new NSM session token and invalidates the old one (a credential mutation).
- **`sonicwall-cli nsm-session sso`** - GET SSO token from URL for user authentication. NSM will return Bearer Token after successful login. Step 3 of the documented MySonicWall-key to NSM-token exchange. Body fields: tenantserial, code, client. On success NSM returns the bearer token in status.info[0].message (FortiSOAR connector-sonicwall-nsm reads it there). Token lasts 8 hours with a 15-minute inactivity timeout (MySonicWall API guide, 'Generating NSM Token using MySonicWall API Key'). Prefer the `nsm-login` command, which runs all three steps.

### nsm-system

Manage nsm system

- **`sonicwall-cli nsm-system health`** - GET /healthcheck. Unauthenticated; returns status.info[0].message HEALTH OK when NSM is up.
- **`sonicwall-cli nsm-system status`** - Show the system status.
- **`sonicwall-cli nsm-system version`** - Get version. Unauthenticated; returns productName, version and buildVersion of the NSM SaaS instance.

### tenants

Manage tenants

- **`sonicwall-cli tenants by-serial`** - To get tenant by serial.
- **`sonicwall-cli tenants count`** - To get tenants count.
- **`sonicwall-cli tenants default`** - To get defaut tenant.
- **`sonicwall-cli tenants get`** - To get tenant by id.
- **`sonicwall-cli tenants list`** - To get all tenants.

### vulnerabilities

Manage vulnerabilities

- **`sonicwall-cli vulnerabilities by-firmware`** - Get vulnerable devices by firmware version. Advisories that apply to one SonicOS firmware version string.
- **`sonicwall-cli vulnerabilities devices`** - Get vulnerable devices by tenant IDs. Devices matched to published SonicWall PSIRT advisories; each entry lists vulnList[] with advisoryID, title, impact, haveWorkaroud (sic) and noticeLink.
- **`sonicwall-cli vulnerabilities serials`** - Get device serialNumbers with vulnerable exp by tenant IDs. Serial numbers of firewalls with a vulnerable firmware exposure, by tenant.


### blind-spots


- **`sonicwall-cli blind-spots`** - Find firewalls registered in MySonicWall that NSM cannot see, with their registered firmware's advisories and support expiry; needs SONICWALL_MSW_API_KEY and an NSM token.


### exposure


- **`sonicwall-cli exposure`** - Rank the SonicWall PSIRT advisories NSM matched to your fleet, or name one advisory (SNWLID ID) to list every affected firewall and whether SSL-VPN is enabled on a zone whose name contains WAN; a CVE ID only matches when NSM's advisory text names it.


### firmware


- **`sonicwall-cli firmware drift`** - See how far each firewall's running SonicOS build is behind the newest available build and which advisories the upgrade clears.


### licenses


- **`sonicwall-cli licenses renewals`** - Every licence and security service expiring in the next N days (expired ones included), per firewall and client, flagged when the hardware is near end of support; needs SONICWALL_MSW_API_KEY.


### mfa-audit


- **`sonicwall-cli mfa-audit`** - Per-account remediation list: admin TOTP per firewall and every local SSL-VPN user (direct or nested group member) without a one-time password, with SAML, RADIUS, LDAP or TACACS+ sign-in marked as delegated.


### posture


- **`sonicwall-cli posture`** - One row per firewall with firmware currency, end-of-support date, published advisories, soonest licence expiry, admin TOTP, SSL-VPN exposure and SSL-VPN users without one-time passwords; each check is pass, warn, fail or unknown, and the licence check needs SONICWALL_MSW_API_KEY.

- **`sonicwall-cli posture changes`** - What changed in fleet posture between saved posture runs of the same scope (--fleet, --tenant or --serial picks it; the default is the newest run's): newly vulnerable, MFA turned off, SSL-VPN newly on WAN, licence lapsed, firmware fixed. Local only: run posture now and again later, with the same SONICWALL_MSW_API_KEY and NSM tenant.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`sonicwall-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`sonicwall-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`sonicwall-cli learnings list`** - Inspect taught rows
- **`sonicwall-cli learnings forget "$QUERY" --all`** - Undo a teach
- **`sonicwall-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`sonicwall-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`sonicwall-cli teach-pattern`** - Install a query/resource template up front
- **`sonicwall-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `SONICWALL_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `sonicwall-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
sonicwall-cli alerts list

# JSON for scripting and agents
sonicwall-cli alerts list --json
# Filter to specific fields
sonicwall-cli alerts list --json --select hits,max_score,timestamp

# Dry run  -  show the request without sending
sonicwall-cli alerts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
sonicwall-cli alerts list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only** - no command changes firewall, NSM or MySonicWall configuration; the only POSTs are read-style queries and the NSM token exchange (`nsm-login`, `nsm-session`, `msw-tenants access-code`)
- **Unknown is never pass** - posture, mfa-audit, firmware drift and licenses renewals mark anything they could not read as `unknown` (or list it in `fetch_failures`), and they and exposure exit 3 instead of returning an empty list when no NSM firewall matches the token and filter. exposure exits 6 when a firewall has no reported SonicOS version or the device inventory is unreadable; blind-spots exits 6 when NSM firewalls are missing from the MySonicWall registry the key reads, drops a product by default only when it is a known non-firewall family, and lists anything unrecognised with `product_class: unknown`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `1` other error, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `SONICWALL_REGION` resolves `{region}`

Base URL: `https://nsm-{region}.sonicwall.com/api/manager`

## Health Check

```bash
sonicwall-cli doctor
```

Checks configuration and paths, then verifies the NSM session token with one authenticated read (`GET /tenants`): it reports the token as valid, rejected (run `nsm-login` again) or not verified, and names `SONICWALL_REGION` when the NSM host is in doubt. Run `sonicwall-cli nsm-login` first; rerun it when NSM calls exit 4. Add `--fail-on error` to make a failed check exit non-zero.

## Configuration

Run `sonicwall-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/sonicwall-cli/config.toml` (`%USERPROFILE%\.config\sonicwall-cli\config.toml` on Windows); `--home`, `SONICWALL_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SONICWALL_MSW_API_KEY` | credential | The key is needed for `nsm-login`, MySonicWall commands, licence checks and `blind-spots`; set it here or as `msw_api_key` in the config file | MySonicWall API key, sent as `X-API-KEY`. `nsm-login` mints the NSM session token from it. |
| `SONICWALL_NSM_TOKEN` | credential | No | NSM bearer token override. Leave it unset and run `nsm-login`, which saves the token; when set it overrides the saved token, so unset it once it expires (8 hours, 15 minutes idle). |
| `SONICWALL_REGION` | endpoint | No (default `uswest`) | NSM SaaS region in `https://nsm-{region}.sonicwall.com`; set `eucentral` for EU portals. |
| `SONICWALL_BASE_URL` | endpoint | No | Sends every call (NSM and MySonicWall) to this base URL instead; for tests and mock servers. |
| `SONICWALL_CONFIG` | path | No | Config file path (default `~/.config/sonicwall-cli/config.toml`). |
| `SONICWALL_MCP_READ_ONLY` | MCP | No | Set to `1` so the MCP server hides tools that change NSM, MySonicWall or firewall state and refuses non-GET execute calls. Tools still write local state: the response cache, `posture` snapshots and learning entries. |
| `SONICWALL_MCP_HTTP_TOKEN` | MCP | Only with `--transport http` | Bearer token HTTP MCP callers must send; the HTTP transport refuses to start without it. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `sonicwall-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- NSM commands: run `sonicwall-cli nsm-login` again (the token lasts 8 hours, 15 minutes idle)
- MySonicWall commands, `nsm-login`, licence checks: check `SONICWALL_MSW_API_KEY` is set and has MySonicWall API permission
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **NSM commands exit 4 with E_INVALIDAUTH**  -  The NSM token expired (8 hours, 15 minutes idle); run 'sonicwall-cli nsm-login' again
- **MySonicWall commands return HTTP 401**  -  Check SONICWALL_MSW_API_KEY; if the key is new, ask SonicWall (thirdpartyintegrations@sonicwall.com) to enable MySonicWall API permission for your user
- **nsm-login fails or the NSM host does not answer, and your NSM portal is in Europe**  -  export SONICWALL_REGION=eucentral and rerun nsm-login
- **A firewall is missing from devices list and posture**  -  Only firewalls onboarded to NSM SaaS are visible; run 'sonicwall-cli blind-spots' to list registered firewalls NSM cannot see
- **sync warns about firewall resources**  -  SonicOS passthrough reads need a target firewall; sync the inventory with 'sync --resources devices,devices-inventory-tenant,vulnerabilities,tenants' and read config with 'firewall <command> --x-device-id <serial>'
- **nsm-login exits 2 listing several tenants**  -  Pick one: sonicwall-cli nsm-login --tenant "<name or product group ID>"
- **posture changes says only one run is saved**  -  History is stored per MySonicWall key and NSM tenant (per token when no key is set); keep SONICWALL_MSW_API_KEY set and use the same nsm-login tenant, or pass the same --db to posture and posture changes. Runs are also kept per filter: after a 'posture --serial' or '--tenant' run, the default compares that scope, so pass --fleet (or the same filters you gave posture)
- **posture overall is never pass**  -  Set SONICWALL_MSW_API_KEY; without it every licence check is unknown, so overall can be at best unknown

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**PSSonicWall**](https://github.com/mmeseguer/PSSonicWall)  -  PowerShell (18 stars)
- [**sonicapi**](https://github.com/hbonath/sonicapi)  -  Python (14 stars)
- [**SonicWALL-Security-Services-Audit**](https://github.com/Entech-Ramon/SonicWALL-Security-Services-Audit)  -  PowerShell (6 stars)
- [**r7-surcom-connectors (SonicWall Capture Client)**](https://github.com/rapid7/r7-surcom-connectors)  -  Python (6 stars)
- [**CodeBlue SonicWall scripts**](https://github.com/CodeBlue-Technology-LLC/Scripts)  -  PowerShell (1 stars)
- [**connector-sonicwall-nsm (FortiSOAR)**](https://github.com/fortinet-fortisoar/connector-sonicwall-nsm)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
