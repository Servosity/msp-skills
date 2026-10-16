# ConnectSecure CLI

**Every ConnectSecure API operation, plus a local store that answers vulnerability x asset x client questions across your whole client base in one command.**

The CLI mints and refreshes the ConnectSecure JWT from your tenant, client ID and secret, and covers the full V4 API, including create, update and delete endpoints. Sync companies, assets and vulnerability reports into local SQLite, then run exposure, portfolio, blind-spots, suppression-audit and ad-hygiene to rank risk across every client offline; qbr and cve-impact call the live API.

Learn more at [ConnectSecure](https://connectsecure.com).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `connectsecure-cli` and `connectsecure-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.ps1 | iex
   ```
3. Verify: `connectsecure-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=connectsecure). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine connectsecure-cli`. On Unix, mark it executable: `chmod +x connectsecure-cli`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/connectsecure --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/connectsecure --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `connectsecure-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the connectsecure skill from https://github.com/Servosity/msp-skills/tree/main/skills/connectsecure. The skill defines how its required CLI (`connectsecure-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=connectsecure).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in the credentials when Claude Desktop prompts you. The bundle asks for `CONNECTSECURE_BASE_URL` (your pod URL), `CONNECTSECURE_TENANT`, `CONNECTSECURE_CLIENT_ID` and `CONNECTSECURE_CLIENT_SECRET`; the server mints its own token from them. A pre-minted `CONNECTSECURE_ACCESS_TOKEN` with `CONNECTSECURE_USER_ID` is optional and is used only until it expires.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS (one universal build for Apple Silicon and Intel), Linux (`amd64`) and Windows (`amd64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`%APPDATA%\Claude\claude_desktop_config.json` on Windows, `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "connectsecure": {
      "command": "connectsecure-mcp",
      "env": {
        "CONNECTSECURE_BASE_URL": "https://pod106.myconnectsecure.com",
        "CONNECTSECURE_TENANT": "<portal-login-tenant>",
        "CONNECTSECURE_CLIENT_ID": "<api-client-id>",
        "CONNECTSECURE_CLIENT_SECRET": "<api-client-secret>"
      }
    }
  }
}
```

</details>

## Authentication

ConnectSecure uses a token exchange, not OAuth. Set CONNECTSECURE_BASE_URL to your pod URL (Profile > API Documentation, for example https://pod106.myconnectsecure.com, not the portal login URL), plus CONNECTSECURE_TENANT (your portal login tenant), CONNECTSECURE_CLIENT_ID and CONNECTSECURE_CLIENT_SECRET (Settings > Users > user > API Key). The CLI calls POST /w/authorize with Client-Auth-Token set to base64 of tenant, a literal plus sign, client_id, a colon and client_secret, caches the returned JWT and user ID, and sends Authorization: Bearer plus X-USER-ID on every call. If you already have a token, set CONNECTSECURE_ACCESS_TOKEN and CONNECTSECURE_USER_ID instead.

## Quick Start

```bash
# Confirm the binary runs (dry run); drop --dry-run to check config, credentials and API reachability.
connectsecure-cli doctor --dry-run

# Confirm the token mint works and see your client companies.
connectsecure-cli companies list --limit 5 --agent

# Pull companies, assets, agents, vulnerability and AD reports into the local store.
connectsecure-cli sync

# Refresh later with --full: a bare re-run never prunes findings remediated since the last sync.
connectsecure-cli sync --full

# Rank the riskiest findings across every client at asset level.
connectsecure-cli exposure --min-epss 0.5 --severity Critical,High --agent

# One row per client to see who needs attention first.
connectsecure-cli portfolio --sort max-epss

```

## Unique Features

### Cross-client risk
- **`exposure`**  -  Reads the synced local store (run sync first): rank open vulnerabilities across all clients at asset level, one row per problem x asset x client, top 50 by default (--limit 0 for all), filtered by EPSS, severity, KEV or first-seen window.

  _Reach for this when asked which clients have the riskiest vulnerabilities and on which machines; it replaces an N+1 asset lookup loop._

  ```bash
  connectsecure-cli exposure --min-epss 0.5 --severity Critical,High --agent
  ```
- **`portfolio`**  -  Reads the synced local store (run sync first): one row per client with asset count, critical and high findings, max EPSS, KEV count, risk score, stale assets and active suppressions.

  _Use it to decide which clients need attention this week before drilling into exposure or qbr._

  ```bash
  connectsecure-cli portfolio --sort max-epss --agent
  ```
- **`cve-impact`**  -  Live API call: for one CVE, show affected clients with asset counts, affected software, available fixes and CVSS/EPSS, and report any assets the API could not attribute to a client.

  _Use it when a headline CVE lands and the question is which clients to call._

  ```bash
  connectsecure-cli cve-impact CVE-2024-3400 --agent
  ```

### Scan coverage
- **`blind-spots`**  -  Reads the synced local store (run sync first): list assets whose scan data cannot be trusted (stale discovery, unauthenticated scan, silent agent), sorted by client with reason codes; the unauthenticated check is a keyword heuristic unless you pass --auth-ok.

  _Use it before trusting a clean vulnerability report; missing scans hide findings._

  ```bash
  connectsecure-cli blind-spots --stale 14d --agent-stale 48h --agent
  ```
- **`ad-hygiene`**  -  Reads the synced local store (run sync first): per client, risky Active Directory accounts (password never expires, not required, no MFA, stale logon), weak password policies and Azure secure score.

  _Use it for identity-risk reviews across all clients without paging through each AD report._

  ```bash
  connectsecure-cli ad-hygiene --stale-logon 90d --min-length 12 --agent
  ```

### Client reporting
- **`qbr`**  -  Live API call: one client's QBR numbers in one call, with risk score, latest company stats, open findings by age, top fixes and remediation speed; compliance pass/fail is added only when --compliance-type, --compliance-platform and --compliance-section are all set.

  _Use it when preparing a client review; pipe the JSON into a writer for prose._

  ```bash
  connectsecure-cli qbr "Acme Dental" --since 90d --agent
  ```
- **`suppression-audit`**  -  Reads the synced local store (run sync first): flag suppressions that expired but are still active, expire soon, never expire, or were unapproved or self-approved, with the suppressed issue's current EPSS.

  _Use it for the accountability half of remediation reviews; risky suppressions sort first._

  ```bash
  connectsecure-cli suppression-audit --within 14d --agent
  ```

## Recipes

### Critical vulns, narrowed for an agent

```bash
connectsecure-cli report-queries vulnerabilities-details --condition "severity='Critical'" --limit 50 --agent --select problem_name,epss_score,base_score,affected_assets
```

Pull only the fields that matter from a large report query.

### Which clients does this CVE hit

```bash
connectsecure-cli cve-impact CVE-2024-3400 --agent
```

Per-client affected asset counts plus available fixes for one CVE (live API).

### Assets you cannot trust

```bash
connectsecure-cli blind-spots --stale 14d --agent-stale 48h
```

Stale, unauthenticated, or agent-silent assets sorted by client, with reason codes (local store).

### One client's QBR numbers

```bash
connectsecure-cli qbr "Acme Dental" --since 90d --agent
```

Risk score, latest stats, open findings by age, top fixes and remediation speed in one JSON pack (live API).

## Usage

Run `connectsecure-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, the store `data-<hash>.db` (`db_path` in `doctor --agent`), cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `CONNECTSECURE_CONFIG_DIR`, `CONNECTSECURE_DATA_DIR`, `CONNECTSECURE_STATE_DIR`, or `CONNECTSECURE_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `CONNECTSECURE_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export CONNECTSECURE_HOME=/srv/connectsecure
connectsecure-cli doctor
```

On Windows (PowerShell):

```powershell
$env:CONNECTSECURE_HOME = "C:\ProgramData\connectsecure"
connectsecure-cli doctor
```

Under `CONNECTSECURE_HOME=/srv/connectsecure`, the four dirs resolve to `/srv/connectsecure/config`, `/srv/connectsecure/data`, `/srv/connectsecure/state`, and `/srv/connectsecure/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "connectsecure": {
      "command": "connectsecure-mcp",
      "env": {
        "CONNECTSECURE_HOME": "/srv/connectsecure"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `CONNECTSECURE_DATA_DIR` overrides an explicit `--home` for that kind. Use `CONNECTSECURE_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `CONNECTSECURE_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `connectsecure-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### agent-credentials-mapping

Manage agent credentials mapping

- **`connectsecure-cli agent-credentials-mapping create`** - Create agent credential mapping
- **`connectsecure-cli agent-credentials-mapping delete`** - Delete agent credential mapping
- **`connectsecure-cli agent-credentials-mapping get`** - Retrieve agent credential mapping
- **`connectsecure-cli agent-credentials-mapping list`** - Retrieve agent credentials mapping
- **`connectsecure-cli agent-credentials-mapping update`** - Update agent credential mapping

### agent-discoverysettings-mapping

Manage agent discoverysettings mapping

- **`connectsecure-cli agent-discoverysettings-mapping create`** - Create agent discoverysetting mapping
- **`connectsecure-cli agent-discoverysettings-mapping delete`** - Delete agent discoverysetting mapping
- **`connectsecure-cli agent-discoverysettings-mapping get`** - Retrieve agent discoverysetting mapping
- **`connectsecure-cli agent-discoverysettings-mapping list`** - Retrieve agent discoverysettings mapping
- **`connectsecure-cli agent-discoverysettings-mapping update`** - Update agent discoverysetting mapping

### agents

Manage agents

- **`connectsecure-cli agents get`** - Retrieve agent
- **`connectsecure-cli agents list`** - Retrieve agents
- **`connectsecure-cli agents reset`** - Migrate Agents
- **`connectsecure-cli agents uninstall-secret`** - Get uninstall secret token (returned in plaintext; human-in-the-loop only, see Secrets below)

### app-baseline-plan-assets

Manage app baseline plan assets

- **`connectsecure-cli app-baseline-plan-assets get`** - Retrieve app baseline plan asset
- **`connectsecure-cli app-baseline-plan-assets list`** - Retrieve app baseline plan assets

### app-baseline-plan-company

Manage app baseline plan company

- **`connectsecure-cli app-baseline-plan-company get`** - Retrieve app baseline plan company
- **`connectsecure-cli app-baseline-plan-company list`** - Retrieve app baseline plan company

### app-baseline-plan-global

Manage app baseline plan global

- **`connectsecure-cli app-baseline-plan-global get`** - Retrieve app baseline plan global
- **`connectsecure-cli app-baseline-plan-global list`** - Retrieve app baseline plan global

### application-baseline-rules

Manage application baseline rules

- **`connectsecure-cli application-baseline-rules create`** - Create application baseline rule
- **`connectsecure-cli application-baseline-rules delete`** - Delete application baseline rule
- **`connectsecure-cli application-baseline-rules get`** - Retrieve application baseline rule
- **`connectsecure-cli application-baseline-rules list`** - Retrieve application baseline rules
- **`connectsecure-cli application-baseline-rules update`** - Update application baseline rule

### asset-firewall-policy

Manage asset firewall policy

- **`connectsecure-cli asset-firewall-policy get`** - Retrieve asset firewall policy
- **`connectsecure-cli asset-firewall-policy list`** - Retrieve asset firewall policy

### asset-installed-drivers

Manage asset installed drivers

- **`connectsecure-cli asset-installed-drivers get`** - Retrieve asset installed driver
- **`connectsecure-cli asset-installed-drivers list`** - Retrieve asset installed drivers

### asset-interface

Manage asset interface

- **`connectsecure-cli asset-interface get`** - Retrieve asset interface
- **`connectsecure-cli asset-interface list`** - Retrieve asset interface

### asset-msdt

Manage asset msdt

- **`connectsecure-cli asset-msdt get`** - Retrieve asset msdt
- **`connectsecure-cli asset-msdt list`** - Retrieve asset msdt

### asset-ports

Manage asset ports

- **`connectsecure-cli asset-ports get`** - Retrieve asset port
- **`connectsecure-cli asset-ports list`** - Retrieve asset ports

### asset-security-report-data

Manage asset security report data

- **`connectsecure-cli asset-security-report-data get`** - Retrieve asset security report datum
- **`connectsecure-cli asset-security-report-data list`** - Retrieve asset security report data

### asset-shares

Manage asset shares

- **`connectsecure-cli asset-shares get`** - Retrieve asset share
- **`connectsecure-cli asset-shares list`** - Retrieve asset shares

### asset-storages

Manage asset storages

- **`connectsecure-cli asset-storages get`** - Retrieve asset storage
- **`connectsecure-cli asset-storages list`** - Retrieve asset storages

### asset-unqouted-services

Manage asset unqouted services

- **`connectsecure-cli asset-unqouted-services get`** - Retrieve asset unqouted service
- **`connectsecure-cli asset-unqouted-services list`** - Retrieve asset unqouted services

### asset-user-shares

Manage asset user shares

- **`connectsecure-cli asset-user-shares get`** - Retrieve asset user share
- **`connectsecure-cli asset-user-shares list`** - Retrieve asset user shares

### asset-video-info

Manage asset video info

- **`connectsecure-cli asset-video-info get`** - Retrieve asset video info
- **`connectsecure-cli asset-video-info list`** - Retrieve asset video info

### asset-view

Manage asset view

- **`connectsecure-cli asset-view get`** - Retrieve asset view
- **`connectsecure-cli asset-view list`** - Retrieve asset view

### asset-windows-compatibility

Manage asset windows compatibility

- **`connectsecure-cli asset-windows-compatibility`** - Retrieve asset windows compatibility

### asset-windows-reboot-required

Manage asset windows reboot required

- **`connectsecure-cli asset-windows-reboot-required get`** - Retrieve asset window reboot required
- **`connectsecure-cli asset-windows-reboot-required list`** - Retrieve asset windows reboot required

### assets

Manage assets

- **`connectsecure-cli assets bulk-deprecate`** - Bulk deprecate assets
- **`connectsecure-cli assets create`** - Create asset
- **`connectsecure-cli assets delete`** - Delete asset
- **`connectsecure-cli assets remediation-plan`** - Retrieve records
- **`connectsecure-cli assets update`** - Update asset

### attack-surface-domain

Manage attack surface domain

- **`connectsecure-cli attack-surface-domain create`** - Create attack surface domain
- **`connectsecure-cli attack-surface-domain delete`** - Delete attack surface domain
- **`connectsecure-cli attack-surface-domain get`** - Retrieve attack surface domain
- **`connectsecure-cli attack-surface-domain list`** - Retrieve attack surface domain
- **`connectsecure-cli attack-surface-domain update`** - Update attack surface domain

### attack-surface-results

Manage attack surface results

- **`connectsecure-cli attack-surface-results get`** - Retrieve attack surface result
- **`connectsecure-cli attack-surface-results list`** - Retrieve attack surface results

### backup-software

Manage backup software

- **`connectsecure-cli backup-software create`** - Create backup software
- **`connectsecure-cli backup-software delete`** - Delete backup software
- **`connectsecure-cli backup-software get`** - Retrieve backup software
- **`connectsecure-cli backup-software list`** - Retrieve backup software
- **`connectsecure-cli backup-software update`** - Update backup software

### bios-info

Manage bios info

- **`connectsecure-cli bios-info get`** - Retrieve bio info
- **`connectsecure-cli bios-info list`** - Retrieve bios info

### browser-extensions

Manage browser extensions

- **`connectsecure-cli browser-extensions get`** - Retrieve browser extension
- **`connectsecure-cli browser-extensions list`** - Retrieve browser extensions

### ciphers-view

Manage ciphers view

- **`connectsecure-cli ciphers-view get`** - Retrieve cipher view
- **`connectsecure-cli ciphers-view list`** - Retrieve ciphers view

### companies

Manage companies

- **`connectsecure-cli companies create`** - Create company
- **`connectsecure-cli companies delete`** - Delete company
- **`connectsecure-cli companies external-scan`** - Scan external endpoint
- **`connectsecure-cli companies get`** - Retrieve company
- **`connectsecure-cli companies list`** - Retrieve companies
- **`connectsecure-cli companies scan`** - Scan Company
- **`connectsecure-cli companies update`** - Update company

### company-mappings

Manage company mappings

- **`connectsecure-cli company-mappings create`** - Create company mapping
- **`connectsecure-cli company-mappings delete`** - Delete company mapping
- **`connectsecure-cli company-mappings get`** - Retrieve company mapping
- **`connectsecure-cli company-mappings list`** - Retrieve company mappings
- **`connectsecure-cli company-mappings update`** - Update company mapping

### company-stats

Manage company stats

- **`connectsecure-cli company-stats get`** - Retrieve company stat
- **`connectsecure-cli company-stats list`** - Retrieve company stats

### compliance

Manage compliance

- **`connectsecure-cli compliance`** - Retrieve Compliance Types

### compliance-assessment

Manage compliance assessment

- **`connectsecure-cli compliance-assessment create`** - Create compliance assessment
- **`connectsecure-cli compliance-assessment delete`** - Delete compliance assessment
- **`connectsecure-cli compliance-assessment get`** - Retrieve compliance assessment
- **`connectsecure-cli compliance-assessment list`** - Retrieve compliance assessment
- **`connectsecure-cli compliance-assessment update`** - Update compliance assessment

### credentials

Manage credentials

- **`connectsecure-cli credentials create`** - Create credential
- **`connectsecure-cli credentials delete`** - Delete credential
- **`connectsecure-cli credentials get`** - Retrieve credential
- **`connectsecure-cli credentials list`** - Retrieve credentials
- **`connectsecure-cli credentials update`** - Update credential

### custom-domains

Manage custom domains

- **`connectsecure-cli custom-domains create`** - Custom Domain
- **`connectsecure-cli custom-domains get`** - Retrieve custom domain
- **`connectsecure-cli custom-domains list`** - Retrieve custom domains

### custom-profile

Manage custom profile

- **`connectsecure-cli custom-profile create`** - Create custom profile
- **`connectsecure-cli custom-profile delete`** - Delete custom profile
- **`connectsecure-cli custom-profile get`** - Retrieve custom profile
- **`connectsecure-cli custom-profile list`** - Retrieve custom profile
- **`connectsecure-cli custom-profile update`** - Update custom profile

### custom-ticketing-template

Manage custom ticketing template

- **`connectsecure-cli custom-ticketing-template create`** - Create custom ticketing template
- **`connectsecure-cli custom-ticketing-template delete`** - Delete custom ticketing template
- **`connectsecure-cli custom-ticketing-template get`** - Retrieve custom ticketing template
- **`connectsecure-cli custom-ticketing-template list`** - Retrieve custom ticketing template
- **`connectsecure-cli custom-ticketing-template update`** - Update custom ticketing template

### cve

Manage cve

- **`connectsecure-cli cve`** - Retrieve records

### discovery-settings

Manage discovery settings

- **`connectsecure-cli discovery-settings create`** - Create discovery setting
- **`connectsecure-cli discovery-settings delete`** - Delete discovery setting
- **`connectsecure-cli discovery-settings get`** - Retrieve discovery setting
- **`connectsecure-cli discovery-settings list`** - Retrieve discovery settings
- **`connectsecure-cli discovery-settings update`** - Update discovery setting

### edr

Manage edr

- **`connectsecure-cli edr create`** - Create edr
- **`connectsecure-cli edr delete`** - Delete edr
- **`connectsecure-cli edr get`** - Retrieve edr
- **`connectsecure-cli edr list`** - Retrieve edr
- **`connectsecure-cli edr update`** - Update edr

### event-set

Manage event set

- **`connectsecure-cli event-set create`** - Create event set
- **`connectsecure-cli event-set delete`** - Delete event set
- **`connectsecure-cli event-set get`** - Retrieve event set
- **`connectsecure-cli event-set list`** - Retrieve event set
- **`connectsecure-cli event-set update`** - Update event set

### firewall-groups

Manage firewall groups

- **`connectsecure-cli firewall-groups get`** - Retrieve firewall group
- **`connectsecure-cli firewall-groups list`** - Retrieve firewall groups

### firewall-interfaces

Manage firewall interfaces

- **`connectsecure-cli firewall-interfaces get`** - Retrieve firewall interface
- **`connectsecure-cli firewall-interfaces list`** - Retrieve firewall interfaces

### firewall-license

Manage firewall license

- **`connectsecure-cli firewall-license get`** - Retrieve firewall license
- **`connectsecure-cli firewall-license list`** - Retrieve firewall license

### firewall-rules

Manage firewall rules

- **`connectsecure-cli firewall-rules get`** - Retrieve firewall rule
- **`connectsecure-cli firewall-rules list`** - Retrieve firewall rules

### firewall-users

Manage firewall users

- **`connectsecure-cli firewall-users get`** - Retrieve firewall user
- **`connectsecure-cli firewall-users list`** - Retrieve firewall users

### firewall-zones

Manage firewall zones

- **`connectsecure-cli firewall-zones get`** - Retrieve firewall zone
- **`connectsecure-cli firewall-zones list`** - Retrieve firewall zones

### integration-credentials

Manage integration credentials

- **`connectsecure-cli integration-credentials create`** - Create integration credential
- **`connectsecure-cli integration-credentials delete`** - Delete integration credential
- **`connectsecure-cli integration-credentials get`** - Retrieve integration credential
- **`connectsecure-cli integration-credentials list`** - Retrieve integration credentials
- **`connectsecure-cli integration-credentials update`** - Update integration credential

### integration-rules

Manage integration rules

- **`connectsecure-cli integration-rules create`** - Create integration rule
- **`connectsecure-cli integration-rules delete`** - Delete integration rule
- **`connectsecure-cli integration-rules get`** - Retrieve integration rule
- **`connectsecure-cli integration-rules list`** - Retrieve integration rules
- **`connectsecure-cli integration-rules update`** - Update integration rule

### jobs-view

Manage jobs view

- **`connectsecure-cli jobs-view get`** - Retrieve job view
- **`connectsecure-cli jobs-view list`** - Retrieve jobs view

### patching

Manage patching

- **`connectsecure-cli patching patch-now`** - Trigger application/OS patch (now or scheduled)
- **`connectsecure-cli patching settings`** - Retrieve records

### pii-scan-settings

Manage pii scan settings

- **`connectsecure-cli pii-scan-settings create`** - Create pii scan setting
- **`connectsecure-cli pii-scan-settings delete`** - Delete pii scan setting
- **`connectsecure-cli pii-scan-settings get`** - Retrieve pii scan setting
- **`connectsecure-cli pii-scan-settings list`** - Retrieve pii scan settings
- **`connectsecure-cli pii-scan-settings update`** - Update pii scan setting

### report-builder

Manage report builder

- **`connectsecure-cli report-builder cover-page`** - Report Settings
- **`connectsecure-cli report-builder create-job`** - Create Report
- **`connectsecure-cli report-builder download-default-template`** - Report Settings
- **`connectsecure-cli report-builder report-link`** - Download report
- **`connectsecure-cli report-builder standard-report-settings`** - Report Settings
- **`connectsecure-cli report-builder standard-reports`** - List standard report
- **`connectsecure-cli report-builder update-cover-page`** - Create Report
- **`connectsecure-cli report-builder update-standard-report-settings`** - Create Report
- **`connectsecure-cli report-builder upload-template`** - Report Settings

### report-jobs-view

Manage report jobs view

- **`connectsecure-cli report-jobs-view get`** - Retrieve report job view
- **`connectsecure-cli report-jobs-view list`** - Retrieve report jobs view

### report-queries

Manage report queries

- **`connectsecure-cli report-queries ad-basic-info`** - Retrieve ad basic info
- **`connectsecure-cli report-queries ad-computers-view`** - Retrieve ad computers view
- **`connectsecure-cli report-queries ad-domain-details`** - Retrieve ad domain details
- **`connectsecure-cli report-queries ad-gpos-details`** - Retrieve ad gpos details
- **`connectsecure-cli report-queries ad-gpos-view`** - Retrieve ad gpos view
- **`connectsecure-cli report-queries ad-group-computers`** - Retrieve ad group computers
- **`connectsecure-cli report-queries ad-group-users`** - Retrieve ad group users
- **`connectsecure-cli report-queries ad-groups-view`** - Retrieve ad groups view
- **`connectsecure-cli report-queries ad-ous-view`** - Retrieve ad ous view
- **`connectsecure-cli report-queries ad-password-policies`** - Retrieve ad password policies
- **`connectsecure-cli report-queries ad-roles`** - Retrieve ad roles
- **`connectsecure-cli report-queries ad-roles-details`** - Retrieve ad roles details
- **`connectsecure-cli report-queries ad-roles-member`** - Retrieve ad roles member
- **`connectsecure-cli report-queries ad-user-licenses`** - Retrieve ad user licenses
- **`connectsecure-cli report-queries ad-users-view`** - Retrieve ad users view
- **`connectsecure-cli report-queries adaudit`** - Retrieve adaudit
- **`connectsecure-cli report-queries application-count`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-by-os`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-by-os-software-details`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-by-os-software-details-suppressed`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-by-product`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-by-product-suppressed`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-by-product-suppressed-tag`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-by-product-tag`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-net`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-net-suppressed`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-net-suppressed-tag`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-net-tag`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-os-patch`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-patching-asset-details`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-suppressed`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-suppressed-by-os`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-suppressed-tag`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-suppressed-tag-by-os`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-tag`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-tag-by-os`** - Retrieve records
- **`connectsecure-cli report-queries application-vulnerabilities-v2`** - Retrieve records
- **`connectsecure-cli report-queries asset-compliance-details`** - Retrieve records
- **`connectsecure-cli report-queries asset-compliance-report-data`** - Retrieve records
- **`connectsecure-cli report-queries asset-critical-vulnerabilities`** - Retrieve records
- **`connectsecure-cli report-queries asset-firewall-rules`** - Retrieve asset firewall rules
- **`connectsecure-cli report-queries asset-iptables-rules`** - Retrieve asset iptables rules
- **`connectsecure-cli report-queries asset-open-ports`** - Retrieve asset open ports
- **`connectsecure-cli report-queries asset-patches-info`** - Retrieve asset patches info
- **`connectsecure-cli report-queries asset-ports-view`** - Retrieve records
- **`connectsecure-cli report-queries asset-processes-running`** - Retrieve asset processes running
- **`connectsecure-cli report-queries asset-registry-misconfiguration`** - Retrieve asset registry misconfiguration
- **`connectsecure-cli report-queries asset-security-report-data`** - Retrieve records
- **`connectsecure-cli report-queries asset-security-report-data-bulk`** - Retrieve asset security report data
- **`connectsecure-cli report-queries asset-services`** - Retrieve asset services
- **`connectsecure-cli report-queries asset-software`** - Retrieve records
- **`connectsecure-cli report-queries asset-stats`** - Retrieve asset stats
- **`connectsecure-cli report-queries asset-users`** - Retrieve asset users
- **`connectsecure-cli report-queries asset-wise-vulnerabilities`** - Retrieve records
- **`connectsecure-cli report-queries assets`** - Retrieve assets
- **`connectsecure-cli report-queries assets-by-application`** - Retrieve records
- **`connectsecure-cli report-queries assets-by-application-suppressed`** - Retrieve records
- **`connectsecure-cli report-queries azure-ad-logs`** - Retrieve azure ad logs
- **`connectsecure-cli report-queries azure-licenses`** - Retrieve azure licenses
- **`connectsecure-cli report-queries azure-secure-score`** - Retrieve azure secure score
- **`connectsecure-cli report-queries cert-info-view`** - Retrieve records
- **`connectsecure-cli report-queries companies-by-application`** - Retrieve records
- **`connectsecure-cli report-queries companies-by-application-suppressed`** - Retrieve records
- **`connectsecure-cli report-queries companies-by-problem-group`** - Retrieve records
- **`connectsecure-cli report-queries companies-by-problem-group-suppressed`** - Retrieve records
- **`connectsecure-cli report-queries compliance-asset-info`** - Retrieve records
- **`connectsecure-cli report-queries compliance-check-asset-count`** - Retrieve records
- **`connectsecure-cli report-queries compliance-check-company-count`** - Retrieve records
- **`connectsecure-cli report-queries compliance-check-count`** - Retrieve records
- **`connectsecure-cli report-queries compliance-check-count-by-section`** - Retrieve records
- **`connectsecure-cli report-queries compliance-count`** - Retrieve records
- **`connectsecure-cli report-queries compliance-internal-checks`** - Retrieve records
- **`connectsecure-cli report-queries compliance-maturity`** - Retrieve records
- **`connectsecure-cli report-queries cron-jobs`** - Retrieve cron jobs
- **`connectsecure-cli report-queries distinct-agents-name`** - Retrieve records
- **`connectsecure-cli report-queries distinct-asset-ip`** - Retrieve records
- **`connectsecure-cli report-queries distinct-asset-name`** - Retrieve records
- **`connectsecure-cli report-queries distinct-discovered-protocols`** - Retrieve records
- **`connectsecure-cli report-queries distinct-os`** - Retrieve records
- **`connectsecure-cli report-queries distinct-platform`** - Retrieve records
- **`connectsecure-cli report-queries distinct-software`** - Retrieve records
- **`connectsecure-cli report-queries distinct-tags`** - Retrieve records
- **`connectsecure-cli report-queries event-stats`** - Retrieve event stats
- **`connectsecure-cli report-queries event-tickets`** - Retrieve event tickets
- **`connectsecure-cli report-queries external-asset-externalscan`** - Retrieve records
- **`connectsecure-cli report-queries external-asset-ports-data`** - Retrieve records
- **`connectsecure-cli report-queries external-asset-ssl-attack`** - Retrieve records
- **`connectsecure-cli report-queries external-asset-ssl-ciphers`** - Retrieve records
- **`connectsecure-cli report-queries external-asset-vulnerabilities`** - Retrieve records
- **`connectsecure-cli report-queries get-assets-by-problem`** - Retrieve records
- **`connectsecure-cli report-queries get-assets-problem`** - Retrieve records
- **`connectsecure-cli report-queries get-computer-details`** - Retrieve get computer details
- **`connectsecure-cli report-queries get-groups-details`** - Retrieve get groups details
- **`connectsecure-cli report-queries get-ous-details`** - Retrieve get ous details
- **`connectsecure-cli report-queries get-remediate-records`** - Retrieve records
- **`connectsecure-cli report-queries get-remediation`** - Retrieve records
- **`connectsecure-cli report-queries get-user-details`** - Retrieve get user details
- **`connectsecure-cli report-queries job-details`** - Retrieve job details
- **`connectsecure-cli report-queries job-details-view`** - Retrieve job details view
- **`connectsecure-cli report-queries kernel-modules`** - Retrieve kernel modules
- **`connectsecure-cli report-queries lightweight-assets`** - Retrieve records
- **`connectsecure-cli report-queries notification-tickets-view`** - Retrieve records
- **`connectsecure-cli report-queries notification-tickets-view-legacy`** - Retrieve notification tickets view
- **`connectsecure-cli report-queries os-pending-patches`** - Retrieve records
- **`connectsecure-cli report-queries os-pending-patches-companies`** - Retrieve records
- **`connectsecure-cli report-queries ports-assets-details`** - Retrieve records
- **`connectsecure-cli report-queries ports-count`** - Retrieve records
- **`connectsecure-cli report-queries ports-view`** - Retrieve records
- **`connectsecure-cli report-queries problem-group-summary`** - Retrieve records
- **`connectsecure-cli report-queries problem-group-summary-asset-company-count`** - Retrieve records
- **`connectsecure-cli report-queries problems-info`** - Retrieve records
- **`connectsecure-cli report-queries problems-remediations-summary`** - Retrieve records
- **`connectsecure-cli report-queries problems-ssl-for-asset`** - Retrieve records
- **`connectsecure-cli report-queries problems-summary`** - Retrieve records
- **`connectsecure-cli report-queries problems-summary-asset-details`** - Retrieve records
- **`connectsecure-cli report-queries problems-summary-global`** - Retrieve records
- **`connectsecure-cli report-queries problems-summary-group-by-companies`** - Retrieve records
- **`connectsecure-cli report-queries problems-summary-tag`** - Retrieve records
- **`connectsecure-cli report-queries registry-problems-company`** - Retrieve records
- **`connectsecure-cli report-queries registry-problems-remediation`** - Retrieve records
- **`connectsecure-cli report-queries registry-problems-remediation-asset-details`** - Retrieve records
- **`connectsecure-cli report-queries registry-problems-summary`** - Retrieve records
- **`connectsecure-cli report-queries remediate-records`** - Retrieve records
- **`connectsecure-cli report-queries remediate-records-assets`** - Retrieve records
- **`connectsecure-cli report-queries remediate-records-companies`** - Retrieve records
- **`connectsecure-cli report-queries remediate-records-days`** - Retrieve records
- **`connectsecure-cli report-queries remediated-registry-solution-plan`** - Retrieve records
- **`connectsecure-cli report-queries remediation-companies`** - Retrieve records
- **`connectsecure-cli report-queries remediation-plan-asset-details`** - Retrieve records
- **`connectsecure-cli report-queries remediation-plan-asset-details-by-epss`** - Retrieve records
- **`connectsecure-cli report-queries remediation-plan-asset-epss-details`** - Retrieve records
- **`connectsecure-cli report-queries remediation-plan-by-company`** - Retrieve records
- **`connectsecure-cli report-queries remediation-plan-include-company`** - Retrieve records
- **`connectsecure-cli report-queries remediation-plan-include-company-days`** - Retrieve records
- **`connectsecure-cli report-queries remediation-velocity-application`** - Retrieve records
- **`connectsecure-cli report-queries remediation-velocity-application-asset-details`** - Retrieve records
- **`connectsecure-cli report-queries remediation-velocity-company`** - Retrieve records
- **`connectsecure-cli report-queries resolved-remediation`** - Retrieve records
- **`connectsecure-cli report-queries risk-score`** - Retrieve records
- **`connectsecure-cli report-queries selinux-settings`** - Retrieve selinux settings
- **`connectsecure-cli report-queries suid-permissions`** - Retrieve suid permissions
- **`connectsecure-cli report-queries suppress-vulnerability-problems`** - Retrieve records
- **`connectsecure-cli report-queries suppress-vulnerability-problems-sid`** - Retrieve records
- **`connectsecure-cli report-queries suppress-vulnerability-solution`** - Retrieve records
- **`connectsecure-cli report-queries suppressed-problems`** - Retrieve records
- **`connectsecure-cli report-queries sw-problems-remediations-view`** - Retrieve records
- **`connectsecure-cli report-queries sw-problems-remediations-view-assetwise`** - Retrieve records
- **`connectsecure-cli report-queries sw-problems-remediations-view-vul`** - Retrieve records
- **`connectsecure-cli report-queries system-events-view`** - Retrieve system events view
- **`connectsecure-cli report-queries system-events-view-ticketid`** - Retrieve system events view
- **`connectsecure-cli report-queries tags-view`** - Retrieve records
- **`connectsecure-cli report-queries total-asset-count`** - Retrieve records
- **`connectsecure-cli report-queries ufw-firewall-rules`** - Retrieve ufw firewall rules
- **`connectsecure-cli report-queries unconfirmed-key-check`** - Retrieve records
- **`connectsecure-cli report-queries unconfirmed-open-ports-key-check`** - Retrieve records
- **`connectsecure-cli report-queries user-enabled-stats`** - Retrieve user enabled stats
- **`connectsecure-cli report-queries user-event-stats`** - Retrieve user event stats
- **`connectsecure-cli report-queries user-locked-stats`** - Retrieve user locked stats
- **`connectsecure-cli report-queries vulnerabilities-count`** - Retrieve records
- **`connectsecure-cli report-queries vulnerabilities-details`** - Retrieve records
- **`connectsecure-cli report-queries vulnerabilities-details-suppressed`** - Retrieve records

### scheduler

Manage scheduler

- **`connectsecure-cli scheduler create`** - Create scheduler
- **`connectsecure-cli scheduler delete`** - Delete scheduler
- **`connectsecure-cli scheduler get`** - Retrieve scheduler
- **`connectsecure-cli scheduler list`** - Retrieve scheduler
- **`connectsecure-cli scheduler remove-schedule`** - Update schedule
- **`connectsecure-cli scheduler update`** - Update scheduler
- **`connectsecure-cli scheduler update-schedule`** - Update schedule

### session

Manage session

- **`connectsecure-cli session`** - Authorize

### suppress-vulnerability

Manage suppress vulnerability

- **`connectsecure-cli suppress-vulnerability create`** - Create suppres vulnerability
- **`connectsecure-cli suppress-vulnerability delete`** - Delete suppres vulnerability
- **`connectsecure-cli suppress-vulnerability get`** - Retrieve suppres vulnerability
- **`connectsecure-cli suppress-vulnerability list`** - Retrieve suppress vulnerability
- **`connectsecure-cli suppress-vulnerability update`** - Update suppres vulnerability

### tag-rules

Manage tag rules

- **`connectsecure-cli tag-rules create`** - Create tag rule
- **`connectsecure-cli tag-rules delete`** - Delete tag rule
- **`connectsecure-cli tag-rules get`** - Retrieve tag rule
- **`connectsecure-cli tag-rules list`** - Retrieve tag rules
- **`connectsecure-cli tag-rules update`** - Update tag rule

### tags

Manage tags

- **`connectsecure-cli tags create`** - Create tag
- **`connectsecure-cli tags delete`** - Delete tag
- **`connectsecure-cli tags get`** - Retrieve tag
- **`connectsecure-cli tags list`** - Retrieve tags
- **`connectsecure-cli tags update`** - Update tag

### users

Manage users

- **`connectsecure-cli users`** - Retrieve Users

### windows-protection-status

Manage windows protection status

- **`connectsecure-cli windows-protection-status get`** - Retrieve window protection status
- **`connectsecure-cli windows-protection-status list`** - Retrieve windows protection status


### ad-hygiene


- **`connectsecure-cli ad-hygiene`** - Reads the synced local store (run sync first): per client, risky Active Directory accounts (password never expires, not required, no MFA, stale logon), weak password policies and Azure secure score.


### blind-spots


- **`connectsecure-cli blind-spots`** - Reads the synced local store (run sync first): list assets whose scan data cannot be trusted (stale discovery, unauthenticated scan, silent agent), sorted by client with reason codes; the unauthenticated check is a keyword heuristic unless you pass --auth-ok.


### cve-impact


- **`connectsecure-cli cve-impact`** - Live API call: for one CVE, show affected clients with asset counts, affected software, available fixes and CVSS/EPSS, and report any assets the API could not attribute to a client.


### exposure


- **`connectsecure-cli exposure`** - Reads the synced local store (run sync first): rank open vulnerabilities across all clients at asset level, one row per problem x asset x client, top 50 by default (--limit 0 for all), filtered by EPSS, severity, KEV or first-seen window.


### portfolio


- **`connectsecure-cli portfolio`** - Reads the synced local store (run sync first): one row per client with asset count, critical and high findings, max EPSS, KEV count, risk score, stale assets and active suppressions.


### qbr


- **`connectsecure-cli qbr`** - Live API call: one client's QBR numbers in one call, with risk score, latest company stats, open findings by age, top fixes and remediation speed; compliance pass/fail is added only when --compliance-type, --compliance-platform and --compliance-section are all set.


### suppression-audit


- **`connectsecure-cli suppression-audit`** - Reads the synced local store (run sync first): flag suppressions that expired but are still active, expire soon, never expire, or were unapproved or self-approved, with the suppressed issue's current EPSS.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`connectsecure-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`connectsecure-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`connectsecure-cli learnings list`** - Inspect taught rows
- **`connectsecure-cli learnings forget <query>`** - Undo a teach
- **`connectsecure-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`connectsecure-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`connectsecure-cli teach-pattern`** - Install a query/resource template up front
- **`connectsecure-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `CONNECTSECURE_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `connectsecure-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
connectsecure-cli companies list

# JSON for scripting and agents
connectsecure-cli companies list --json
# Filter to specific fields
connectsecure-cli companies list --json --select id,name,domain

# Dry run  -  show the request without sending
connectsecure-cli companies list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
connectsecure-cli companies list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Writes are not confirmation-gated** - API writes (create, update, delete, scans) run immediately and are not `--yes`-gated; preview them with `--dry-run` first. `--yes` only answers this CLI's own local prompts, such as `profile delete`.
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `8` async job submitted but not finished (rerun the printed recovery command; do not resubmit), `10` config error.

## Secrets

- **Credential tier (human-in-the-loop only):** `credentials`, `integration-credentials`, `integration-rules`, `scheduler` and `attack-surface-results` `list` / `get`, plus `agents uninstall-secret`, return stored secrets in plaintext, and `scheduler create` / `update` send a job password; `custom-domains list` / `get` return TLS certificate details whose contents ConnectSecure does not document, and `custom-domains create` uploads the private key. `tail` on those resources prints them on every poll, and `export` of them prints them to stdout or writes them to the file named by `-o`. `agent-credentials-mapping create` / `update` changes which stored credentials a scan agent uses, and `company-mappings create` / `update` which integration credential a client uses. Full tiers: [governance.md](governance.md).
- **Sending a secret:** create or update credentials, integration credentials, integration rules or scheduled jobs with the JSON body on stdin (`--stdin`, wrapped as `{"data": {...}}`, the shape `--dry-run` shows for the flag form) instead of `--data-params`, `--data-integration-params` or `--data-password`, so the secret stays out of process arguments and shell history. `custom-domains create` takes the private key as a file path (`--ssl-privkey <file>`), so the key never sits in process arguments either. `--dry-run` masks secret-named fields in its preview, and an API error that echoes a secret you sent is withheld instead of printed.
- **Parsing output:** with `--agent`, the analysis commands, `list` / `get`, `sync` and `doctor` wrap their output in `{"meta": ..., "results": ...}`; parse `.results`. With `--json`, `list` / `get` use the same envelope while `qbr` and `cve-impact` print their report object at the top level. `export` and `tail` stream bare JSON records in either mode.

## Freshness

This CLI owns bounded freshness for registered store-backed read command paths. In `--data-source auto` mode, covered commands check the local SQLite store before serving results; stale or missing resources trigger a bounded refresh, and refresh failures fall back to the existing local data with a warning. `--data-source local` never refreshes, and `--data-source live` reads the API (through the 5-minute on-disk response cache; add `--no-cache` to force a fresh call) without mutating the local store.

Set `CONNECTSECURE_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths: Pre-read refresh applies to the generated list and get commands of synced resources, for example `companies list`, `agents list`, `tags list` and `edr list`. Credential resources (`credentials`, `integration-credentials`, `integration-rules`), scheduled jobs (`scheduler`, whose rows carry a password field), attack-surface results (`attack-surface-results`, whose rows carry creds and hashes fields), custom domains (`custom-domains`, a precaution: their certificate details are undocumented) and the agent uninstall secret are never cached or synced locally. The local novel commands (`exposure`, `portfolio`, `blind-spots`, `suppression-audit`, `ad-hygiene`) read the store directly and print a `hint:` line naming any resource that was never synced.

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
connectsecure-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `connectsecure-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/connectsecure-cli/config.toml` (`%USERPROFILE%\.config\connectsecure-cli\config.toml` on Windows); `--home`, `CONNECTSECURE_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `CONNECTSECURE_BASE_URL` | config | Yes | Pod API URL from Profile > API Documentation, for example `https://pod106.myconnectsecure.com` (not the portal login URL). |
| `CONNECTSECURE_ACCESS_TOKEN` | per_call | No | Pre-minted JWT from `/w/authorize`. Set this OR tenant + client ID + secret; the CLI mints and caches the JWT itself. |
| `CONNECTSECURE_TENANT` | auth_flow_input | No | Tenant name you type on the ConnectSecure portal login screen. |
| `CONNECTSECURE_CLIENT_ID` | auth_flow_input | No | API client ID from Settings > Users > (user) > API Key. |
| `CONNECTSECURE_CLIENT_SECRET` | auth_flow_input | No | API client secret from Settings > Users > (user) > API Key. |
| `CONNECTSECURE_USER_ID` | per_call | No | `user_id` from `/w/authorize`; only needed with a pre-minted `CONNECTSECURE_ACCESS_TOKEN`. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `connectsecure-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `connectsecure-cli doctor` to check credentials
- Verify `CONNECTSECURE_BASE_URL` (your pod URL) plus either `CONNECTSECURE_TENANT`, `CONNECTSECURE_CLIENT_ID` and `CONNECTSECURE_CLIENT_SECRET`, or a pre-minted `CONNECTSECURE_ACCESS_TOKEN` with `CONNECTSECURE_USER_ID` are set, then run `connectsecure-cli auth status`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **DNS error or HTML login page instead of JSON**  -  CONNECTSECURE_BASE_URL must be the pod API host from Profile > API Documentation (https://podN.myconnectsecure.com), not portal.myconnectsecure.com.
- **Auth not configured or 401 on every call**  -  Set CONNECTSECURE_BASE_URL plus CONNECTSECURE_TENANT, CONNECTSECURE_CLIENT_ID and CONNECTSECURE_CLIENT_SECRET (or CONNECTSECURE_ACCESS_TOKEN plus CONNECTSECURE_USER_ID), then run connectsecure-cli doctor.
- **401 or 403 from /w/authorize**  -  Re-check CONNECTSECURE_TENANT (the login tenant name), CONNECTSECURE_CLIENT_ID and CONNECTSECURE_CLIENT_SECRET from Settings > Users > API Key; regenerate the key if the secret was lost.
- **400 on a list command with --condition**  -  Quote string values with single quotes inside the condition, for example --condition "severity='Critical' AND company_id=12".
- **Novel commands print a sync hint and no rows**  -  Run connectsecure-cli sync first; exposure, portfolio, blind-spots, suppression-audit and ad-hygiene read the local store.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**Decian-Inc/connectsecure-mcp**](https://github.com/Decian-Inc/connectsecure-mcp)  -  Python
- [**redanthrax/connectsecure-node**](https://github.com/redanthrax/connectsecure-node)  -  TypeScript
- [**MSPbotsAI/connectsecure-mcp**](https://github.com/MSPbotsAI/connectsecure-mcp)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
