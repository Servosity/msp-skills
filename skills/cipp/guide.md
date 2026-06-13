# CIPP CLI

**First single-binary CLI for CIPP  -  offline SQLite store, fleet posture analytics, and cross-tenant fan-out no other CIPP tool has.**

CIPP (CyberDrain Improved Partner Portal) is how MSPs manage many Microsoft 365 tenants from one place. This CLI wraps the full 496-endpoint API as typed commands, then goes further: `fanout --save` pulls tenants, users, licenses, and standards into a local SQLite database so you can run fleet-wide queries (`posture`, `licenses waste`, `users stale`), detect baseline drift (`standards drift`), and execute throttle-aware bulk operations (`bulk`) that survive Microsoft Graph 429s  -  all with --json, --dry-run, and typed exit codes.

Learn more at [CIPP](https://docs.cipp.app).

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `cipp-cli` binary and the `pp-cipp` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install cipp
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install cipp --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install cipp --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install cipp --agent claude-code
npx -y @mvanhorn/printing-press-library install cipp --agent claude-code --agent codex
```

### Without Node

The generated install path is category-agnostic until this CLI is published. If `npx` is not available before publish, install Node or use the category-specific Go fallback from the public-library entry after publish.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/cipp-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install cipp --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-cipp --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-cipp --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install cipp --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/cipp-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `CIPP_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "cipp": {
      "command": "cipp-mcp",
      "env": {
        "CIPP_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

CIPP is self-hosted, so point the CLI at your own instance and authenticate with an API client created in CIPP > Integrations > CIPP-API. Run `cipp-cli auth login` with your Client ID, Client Secret, Tenant ID, and base URL (which must include /api); the CLI performs the OAuth2 client-credentials exchange against login.microsoftonline.com and caches the bearer token to expiry. A static bearer token (CIPP_API_KEY) is also accepted. If you get 401/403, check the API client's IP allowlist and that your base URL ends in /api.

## Quick Start

```bash
# One-time: exchange client credentials for a cached bearer token.
cipp-cli auth login --client-id <id> --client-secret <secret> --tenant-id <tid> --base-url https://your-cipp.example.com/api

# Verify connectivity and surface the common IP-allowlist / /api-path traps before anything else.
cipp-cli doctor

# List your client tenants  -  the root key every other command is scoped by.
cipp-cli list-tenants --json

# Pull every tenant's users into the local store  -  the population path the offline analytics commands read.
cipp-cli fanout --endpoint /ListUsers --all-tenants --save

# See MFA posture across every tenant in one table.
cipp-cli posture --dimension mfa --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-tenant leverage
- **`fanout`**  -  Run any read command across every client tenant at once, with throttle-aware backoff and resume after a halt.

  _Reach for this whenever a question spans the whole MSP fleet instead of a single tenant; it replaces dozens of UI tenant-switches with one call._

  ```bash
  cipp-cli fanout --endpoint /ListUsers --all-tenants --save
  ```
- **`posture`**  -  One table of every tenant's MFA, Conditional Access, Standards, and BPA posture  -  the QBR rollup the UI never renders.

  _Use it to answer 'which tenants are out of compliance right now' for a QBR or cyber-insurance attestation without click-per-tenant._

  ```bash
  cipp-cli posture --dimension mfa --agent
  ```
- **`licenses waste`**  -  Surface assigned-but-unused licenses and CSP billing mismatches across all tenants.

  _Run it at renewal to recover real dollars from dormant seats and pool-vs-billed gaps._

  ```bash
  cipp-cli licenses waste --all-tenants --agent
  ```

### Drift and time-travel
- **`standards drift`**  -  Show every tenant whose security baseline regressed since the last sync.

  _Catch baseline drift before an audit finds it  -  the snapshot-to-snapshot diff is invisible in the live UI._

  ```bash
  cipp-cli standards drift --agent
  ```
- **`users stale`**  -  Flag licensed accounts with no sign-in in N days across every tenant.

  _Use it for security cleanup and cost recovery  -  dormant licensed accounts are both an attack surface and wasted spend._

  ```bash
  cipp-cli users stale --days 90 --all-tenants --agent
  ```

### Bulk and automation
- **`bulk`**  -  Drive add-user / offboard / remove-user / set-forwarding actions from a CSV with Retry-After backoff and resume-after-429 checkpointing.

  _Use it for burst onboarding/offboarding across tenants; the dry-run previews every destructive step before anything fires._

  ```bash
  cipp-cli bulk --from offboards.csv --dry-run
  ```

## Recipes


### Fleet MFA posture for a QBR

```bash
cipp-cli posture --dimension mfa --agent --select tenant,metrics
```

Narrow the cross-tenant matrix to just MFA columns for a clean QBR export.

### Find dormant licensed accounts

```bash
cipp-cli users stale --days 90 --all-tenants --json
```

Surface licensed accounts fleet-wide with no sign-in in 90 days for cleanup and cost recovery.

### List users across all tenants

```bash
cipp-cli fanout --endpoint /ListUsers --all-tenants --save --json
```

Run one read command fleet-wide with throttle-aware backoff, narrowing the deeply-nested response to the fields that matter.

### What drifted since last week

```bash
cipp-cli standards drift --agent
```

Diff each tenant's security baseline against the prior synced snapshot.

## Usage

Run `cipp-cli --help` for the full command reference and flag list.

## Commands

### add-alert

Manage add alert

- **`cipp-cli add-alert`** - Add alert

### add-apdevice

Manage add apdevice

- **`cipp-cli add-apdevice`** - Adds Autopilot devices to a tenant via Partner Center API
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### add-assignment-filter

Manage add assignment filter

- **`cipp-cli add-assignment-filter`** - Add assignment filter

### add-assignment-filter-template

Manage add assignment filter template

- **`cipp-cli add-assignment-filter-template`** - Add assignment filter template

### add-autopilot-config

Manage add autopilot config

- **`cipp-cli add-autopilot-config`** - Add autopilot config

### add-bpatemplate

Manage add bpatemplate

- **`cipp-cli add-bpatemplate`** - Add bpatemplate

### add-capolicy

Manage add capolicy

- **`cipp-cli add-capolicy`** - Add capolicy

### add-catemplate

Manage add catemplate

- **`cipp-cli add-catemplate`** - Add catemplate

### add-choco-app

Manage add choco app

- **`cipp-cli add-choco-app`** - Add choco app

### add-connection-filter

Manage add connection filter

- **`cipp-cli add-connection-filter`** - Add connection filter

### add-connection-filter-template

Manage add connection filter template

- **`cipp-cli add-connection-filter-template`** - Add connection filter template

### add-contact

Manage add contact

- **`cipp-cli add-contact`** - Add contact

### add-contact-templates

Manage add contact templates

- **`cipp-cli add-contact-templates`** - Add contact templates

### add-defender-deployment

Manage add defender deployment

- **`cipp-cli add-defender-deployment`** - Add defender deployment

### add-domain

Manage add domain

- **`cipp-cli add-domain`** - Add domain

### add-edit-transport-rule

Manage add edit transport rule

- **`cipp-cli add-edit-transport-rule`** - This function creates a new transport rule or edits an existing one (mail flow rule).
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### add-enrollment

Manage add enrollment

- **`cipp-cli add-enrollment`** - Add enrollment

### add-equipment-mailbox

Manage add equipment mailbox

- **`cipp-cli add-equipment-mailbox`** - Add equipment mailbox

### add-ex-connector

Manage add ex connector

- **`cipp-cli add-ex-connector`** - Add ex connector

### add-ex-connector-template

Manage add ex connector template

- **`cipp-cli add-ex-connector-template`** - Add ex connector template

### add-group

Manage add group

- **`cipp-cli add-group`** - Add group

### add-group-team

Manage add group team

- **`cipp-cli add-group-team`** - Add group team

### add-group-template

Manage add group template

- **`cipp-cli add-group-template`** - Add group template

### add-guest

Manage add guest

- **`cipp-cli add-guest`** - Add guest

### add-intune-reusable-setting

Manage add intune reusable setting

- **`cipp-cli add-intune-reusable-setting`** - Add intune reusable setting

### add-intune-reusable-setting-template

Manage add intune reusable setting template

- **`cipp-cli add-intune-reusable-setting-template`** - Add intune reusable setting template

### add-intune-template

Manage add intune template

- **`cipp-cli add-intune-template`** - Add intune template

### add-jitadmin-template

Manage add jitadmin template

- **`cipp-cli add-jitadmin-template`** - Add jitadmin template

### add-mspapp

Manage add mspapp

- **`cipp-cli add-mspapp`** - Add mspapp

### add-named-location

Manage add named location

- **`cipp-cli add-named-location`** - Add named location

### add-office-app

Manage add office app

- **`cipp-cli add-office-app`** - Add office app

### add-policy

Manage add policy

- **`cipp-cli add-policy`** - Add policy

### add-quarantine-policy

Manage add quarantine policy

- **`cipp-cli add-quarantine-policy`** - Add quarantine policy

### add-room-list

Manage add room list

- **`cipp-cli add-room-list`** - Add room list

### add-room-mailbox

Manage add room mailbox

- **`cipp-cli add-room-mailbox`** - Add room mailbox

### add-safe-links-policy-from-template

Manage add safe links policy from template

- **`cipp-cli add-safe-links-policy-from-template`** - This function deploys SafeLinks policies and rules from templates to selected tenants.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### add-safe-links-policy-template

Manage add safe links policy template

- **`cipp-cli add-safe-links-policy-template`** - Add safe links policy template

### add-scheduled-item

Manage add scheduled item

- **`cipp-cli add-scheduled-item`** - Add scheduled item

### add-shared-mailbox

Manage add shared mailbox

- **`cipp-cli add-shared-mailbox`** - Add shared mailbox

### add-site

Manage add site

- **`cipp-cli add-site`** - Add site

### add-site-bulk

Manage add site bulk

- **`cipp-cli add-site-bulk`** - Add site bulk

### add-spam-filter

Manage add spam filter

- **`cipp-cli add-spam-filter`** - Add spam filter

### add-spam-filter-template

Manage add spam filter template

- **`cipp-cli add-spam-filter-template`** - Add spam filter template

### add-standards-deploy

Manage add standards deploy

- **`cipp-cli add-standards-deploy`** - Add standards deploy

### add-standards-template

Manage add standards template

- **`cipp-cli add-standards-template`** - Add standards template

### add-store-app

Manage add store app

- **`cipp-cli add-store-app`** - Add store app

### add-team

Manage add team

- **`cipp-cli add-team`** - Add team

### add-tenant

Manage add tenant

- **`cipp-cli add-tenant`** - Add tenant

### add-tenant-allow-block-list

Manage add tenant allow block list

- **`cipp-cli add-tenant-allow-block-list`** - Add tenant allow block list

### add-test-report

Manage add test report

- **`cipp-cli add-test-report`** - Add test report

### add-transport-rule

Manage add transport rule

- **`cipp-cli add-transport-rule`** - Add transport rule

### add-transport-template

Manage add transport template

- **`cipp-cli add-transport-template`** - Add transport template

### add-user

Manage add user

- **`cipp-cli add-user`** - Create a new user in a tenant, optionally scheduled

### add-user-bulk

Manage add user bulk

- **`cipp-cli add-user-bulk`** - Bulk-create users in a tenant via CSV import

### add-user-defaults

Manage add user defaults

- **`cipp-cli add-user-defaults`** - Add user defaults

### add-win32-script-app

Manage add win32 script app

- **`cipp-cli add-win32-script-app`** - Add win32script app

### best-practice-analyser-list

Manage best practice analyser list

- **`cipp-cli best-practice-analyser-list`** - Best practice analyser_list

### cippdbtests-run

Manage cippdbtests run

- **`cipp-cli cippdbtests-run`** - Cippdbtests run

### cippoffboarding-job

Manage cippoffboarding job

- **`cipp-cli cippoffboarding-job`** - Cippoffboarding job

### cippstandards-run

Manage cippstandards run

- **`cipp-cli cippstandards-run`** - Cippstandards run

### create-safe-links-policy-template

Manage create safe links policy template

- **`cipp-cli create-safe-links-policy-template`** - This function creates a new Safe Links policy template from scratch.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### delete-sharepoint-site

Manage delete sharepoint site

- **`cipp-cli delete-sharepoint-site`** - Delete sharepoint site

### delete-test-report

Manage delete test report

- **`cipp-cli delete-test-report`** - Delete test report

### deploy-contact-templates

Manage deploy contact templates

- **`cipp-cli deploy-contact-templates`** - This function deploys contact(s) from template(s) to selected tenants.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### edit-anti-phishing-filter

Manage edit anti phishing filter

- **`cipp-cli edit-anti-phishing-filter`** - Edit anti phishing filter

### edit-assignment-filter

Manage edit assignment filter

- **`cipp-cli edit-assignment-filter`** - Edit assignment filter

### edit-capolicy

Manage edit capolicy

- **`cipp-cli edit-capolicy`** - Edit capolicy

### edit-contact

Manage edit contact

- **`cipp-cli edit-contact`** - Edit contact

### edit-contact-templates

Manage edit contact templates

- **`cipp-cli edit-contact-templates`** - Edit contact templates

### edit-equipment-mailbox

Manage edit equipment mailbox

- **`cipp-cli edit-equipment-mailbox`** - Edit equipment mailbox

### edit-ex-connector

Manage edit ex connector

- **`cipp-cli edit-ex-connector`** - Edit ex connector

### edit-group

Manage edit group

- **`cipp-cli edit-group`** - Edit group

### edit-intune-policy

Manage edit intune policy

- **`cipp-cli edit-intune-policy`** - Edit intune policy

### edit-intune-script

Manage edit intune script

- **`cipp-cli edit-intune-script`** - Edit intune script

### edit-jitadmin-template

Manage edit jitadmin template

- **`cipp-cli edit-jitadmin-template`** - Edit jitadmin template

### edit-malware-filter

Manage edit malware filter

- **`cipp-cli edit-malware-filter`** - Edit malware filter

### edit-policy

Manage edit policy

- **`cipp-cli edit-policy`** - Edit policy

### edit-quarantine-policy

Manage edit quarantine policy

- **`cipp-cli edit-quarantine-policy`** - Edit quarantine policy

### edit-room-list

Manage edit room list

- **`cipp-cli edit-room-list`** - Edit room list

### edit-room-mailbox

Manage edit room mailbox

- **`cipp-cli edit-room-mailbox`** - Edit room mailbox

### edit-safe-attachments-filter

Manage edit safe attachments filter

- **`cipp-cli edit-safe-attachments-filter`** - Edit safe attachments filter

### edit-safe-links-policy

Manage edit safe links policy

- **`cipp-cli edit-safe-links-policy`** - This function modifies an existing Safe Links policy and its associated rule.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### edit-safe-links-policy-template

Manage edit safe links policy template

- **`cipp-cli edit-safe-links-policy-template`** - This function updates an existing Safe Links policy template.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### edit-spam-filter

Manage edit spam filter

- **`cipp-cli edit-spam-filter`** - Edit spam filter

### edit-tenant

Manage edit tenant

- **`cipp-cli edit-tenant`** - Edit tenant

### edit-tenant-offboarding-defaults

Manage edit tenant offboarding defaults

- **`cipp-cli edit-tenant-offboarding-defaults`** - Edit tenant offboarding defaults

### edit-transport-rule

Manage edit transport rule

- **`cipp-cli edit-transport-rule`** - Edit transport rule

### edit-user

Manage edit user

- **`cipp-cli edit-user`** - Edit user

### edit-user-aliases

Manage edit user aliases

- **`cipp-cli edit-user-aliases`** - Edit user aliases

### exec-access-checks

Manage exec access checks

- **`cipp-cli exec-access-checks`** - Exec access checks

### exec-add-alert

Manage exec add alert

- **`cipp-cli exec-add-alert`** - Exec add alert

### exec-add-gdaprole

Manage exec add gdaprole

- **`cipp-cli exec-add-gdaprole`** - Exec add gdaprole

### exec-add-multi-tenant-app

Manage exec add multi tenant app

- **`cipp-cli exec-add-multi-tenant-app`** - Exec add multi tenant app

### exec-add-spn

Manage exec add spn

- **`cipp-cli exec-add-spn`** - Exec add spn

### exec-add-tenant

Manage exec add tenant

- **`cipp-cli exec-add-tenant`** - Exec add tenant

### exec-add-trusted-ip

Manage exec add trusted ip

- **`cipp-cli exec-add-trusted-ip`** - Exec add trusted ip

### exec-alerts-list

Manage exec alerts list

- **`cipp-cli exec-alerts-list`** - Exec alerts list

### exec-api-client

Manage exec api client

- **`cipp-cli exec-api-client`** - Exec api client

### exec-apipermission-list

Manage exec apipermission list

- **`cipp-cli exec-apipermission-list`** - Exec apipermission list

### exec-app-approval

Manage exec app approval

- **`cipp-cli exec-app-approval`** - Exec app approval

### exec-app-approval-template

Manage exec app approval template

- **`cipp-cli exec-app-approval-template`** - Exec app approval template

### exec-app-insights-query

Manage exec app insights query

- **`cipp-cli exec-app-insights-query`** - Exec app insights query

### exec-app-permission-template

Manage exec app permission template

- **`cipp-cli exec-app-permission-template`** - Exec app permission template

### exec-app-upload

Manage exec app upload

- **`cipp-cli exec-app-upload`** - Exec app upload

### exec-application

Manage exec application

- **`cipp-cli exec-application`** - Exec application

### exec-assign-apdevice

Manage exec assign apdevice

- **`cipp-cli exec-assign-apdevice`** - Exec assign apdevice

### exec-assign-app

Manage exec assign app

- **`cipp-cli exec-assign-app`** - Exec assign app

### exec-assign-policy

Manage exec assign policy

- **`cipp-cli exec-assign-policy`** - Exec assign policy

### exec-assignment-filter

Manage exec assignment filter

- **`cipp-cli exec-assignment-filter`** - Exec assignment filter

### exec-audit-log-search

Manage exec audit log search

- **`cipp-cli exec-audit-log-search`** - Exec audit log search

### exec-auto-extend-gdap

Manage exec auto extend gdap

- **`cipp-cli exec-auto-extend-gdap`** - Exec auto extend gdap

### exec-az-bobby-tables

Manage exec az bobby tables

- **`cipp-cli exec-az-bobby-tables`** - This function is used to interact with Azure Tables. This is advanced functionality used for external integrations or SuperAdmin functionality.
    .FUNCTIONALITY
        Entrypoint
    .ROLE
        CIPP.SuperAdmin.ReadWrite
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)
    $AllowList = @(
        'Add-AzDataTableEntity'
        'Add-CIPPAzDataTableEntity'
        'Update-AzDataTableEntity'
        'Get-AzDataTableEntity'
        'Get-CIPPAzDataTableEntity'
        'Get-AzDataTable'
        'New-AzDataTable'
        'Remove-AzDataTableEntity'
        'Remove-AzDataTable'
    )

### exec-backend-urls

Manage exec backend urls

- **`cipp-cli exec-backend-urls`** - Exec backend urls

### exec-backup-retention-config

Manage exec backup retention config

- **`cipp-cli exec-backup-retention-config`** - Exec backup retention config

### exec-beccheck

Manage exec beccheck

- **`cipp-cli exec-beccheck`** - Exec beccheck

### exec-becremediate

Manage exec becremediate

- **`cipp-cli exec-becremediate`** - Exec becremediate

### exec-bitlocker-search

Manage exec bitlocker search

- **`cipp-cli exec-bitlocker-search`** - Exec bitlocker search

### exec-bpa

Manage exec bpa

- **`cipp-cli exec-bpa`** - Exec bpa

### exec-branding-settings

Manage exec branding settings

- **`cipp-cli exec-branding-settings`** - Exec branding settings

### exec-breach-search

Manage exec breach search

- **`cipp-cli exec-breach-search`** - Exec breach search

### exec-bulk-license

Manage exec bulk license

- **`cipp-cli exec-bulk-license`** - Exec bulk license

### exec-cacheck

Manage exec cacheck

- **`cipp-cli exec-cacheck`** - Exec cacheck

### exec-caexclusion

Manage exec caexclusion

- **`cipp-cli exec-caexclusion`** - Exec caexclusion

### exec-caservice-exclusion

Manage exec caservice exclusion

- **`cipp-cli exec-caservice-exclusion`** - Exec caservice exclusion

### exec-cipp-function

Manage exec cipp function

- **`cipp-cli exec-cipp-function`** - This function is used to execute a CIPPCore function from an HTTP request. This is advanced functionality used for external integrations or SuperAdmin functionality.
    .FUNCTIONALITY
        Entrypoint
    .ROLE
        CIPP.SuperAdmin.ReadWrite
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)
    $BlockList = @(
        'Get-GraphToken'
        'Get-GraphTokenFromCert'
        'Get-ClassicAPIToken'
    )

### exec-cipp-logs-sas

Manage exec cipp logs sas

- **`cipp-cli exec-cipp-logs-sas`** - Creates a long-lived, read-only SAS URL for the CippLogs Azure Storage table.
    .FUNCTIONALITY
        Entrypoint
    .ROLE
        CIPP.AppSettings.ReadWrite
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### exec-cipp-replacemap

Manage exec cipp replacemap

- **`cipp-cli exec-cipp-replacemap`** - Exec cipp replacemap

### exec-cippdbcache

Manage exec cippdbcache

- **`cipp-cli exec-cippdbcache`** - Exec cippdbcache

### exec-clone-template

Manage exec clone template

- **`cipp-cli exec-clone-template`** - Exec clone template

### exec-clr-imm-id

Manage exec clr imm id

- **`cipp-cli exec-clr-imm-id`** - Exec clr imm id

### exec-combined-setup

Manage exec combined setup

- **`cipp-cli exec-combined-setup`** - Exec combined setup

### exec-community-repo

Manage exec community repo

- **`cipp-cli exec-community-repo`** - This function makes changes to a community repository in table storage
    .FUNCTIONALITY
        Entrypoint,AnyTenant
    .ROLE
        CIPP.Core.ReadWrite
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### exec-convert-mailbox

Manage exec convert mailbox

- **`cipp-cli exec-convert-mailbox`** - Exec convert mailbox

### exec-copy-for-sent

Manage exec copy for sent

- **`cipp-cli exec-copy-for-sent`** - Exec copy for sent

### exec-cpvpermissions

Manage exec cpvpermissions

- **`cipp-cli exec-cpvpermissions`** - Exec cpvpermissions

### exec-cpvrefresh

Manage exec cpvrefresh

- **`cipp-cli exec-cpvrefresh`** - This endpoint is used to trigger a refresh of CPV for all tenants

### exec-create-app-template

Manage exec create app template

- **`cipp-cli exec-create-app-template`** - Exec create app template

### exec-create-default-groups

Manage exec create default groups

- **`cipp-cli exec-create-default-groups`** - This function creates a set of default tenant groups that are commonly used
    .FUNCTIONALITY
        Entrypoint,AnyTenant
    .ROLE
        Tenant.Groups.ReadWrite
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### exec-create-samapp

Manage exec create samapp

- **`cipp-cli exec-create-samapp`** - Exec create samapp

### exec-create-tap

Manage exec create tap

- **`cipp-cli exec-create-tap`** - Exec create tap

### exec-csplicense

Manage exec csplicense

- **`cipp-cli exec-csplicense create`** - Exec csplicense
- **`cipp-cli exec-csplicense list`** - Exec csplicense

### exec-custom-data

Manage exec custom data

- **`cipp-cli exec-custom-data`** - Exec custom data

### exec-custom-role

Manage exec custom role

- **`cipp-cli exec-custom-role`** - Exec custom role

### exec-delete-gdaprelationship

Manage exec delete gdaprelationship

- **`cipp-cli exec-delete-gdaprelationship`** - Exec delete gdaprelationship

### exec-delete-gdaprole-mapping

Manage exec delete gdaprole mapping

- **`cipp-cli exec-delete-gdaprole-mapping`** - Exec delete gdaprole mapping

### exec-delete-safe-links-policy

Manage exec delete safe links policy

- **`cipp-cli exec-delete-safe-links-policy`** - This function deletes a Safe Links rule and its associated policy.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)
    $APIName = $Request.Params.CIPPEndpoint
    $Headers = $Request.Headers

### exec-device-action

Manage exec device action

- **`cipp-cli exec-device-action`** - Exec device action

### exec-device-code-logon

Manage exec device code logon

- **`cipp-cli exec-device-code-logon`** - Exec device code logon

### exec-device-delete

Manage exec device delete

- **`cipp-cli exec-device-delete`** - Exec device delete

### exec-device-passcode-action

Manage exec device passcode action

- **`cipp-cli exec-device-passcode-action`** - Exec device passcode action

### exec-diagnostics-presets

Manage exec diagnostics presets

- **`cipp-cli exec-diagnostics-presets`** - Exec diagnostics presets

### exec-disable-user

Manage exec disable user

- **`cipp-cli exec-disable-user`** - Exec disable user

### exec-dismiss-risky-user

Manage exec dismiss risky user

- **`cipp-cli exec-dismiss-risky-user`** - Exec dismiss risky user

### exec-dns-config

Manage exec dns config

- **`cipp-cli exec-dns-config`** - Exec dns config

### exec-domain-action

Manage exec domain action

- **`cipp-cli exec-domain-action`** - Exec domain action

### exec-domain-analyser

Manage exec domain analyser

- **`cipp-cli exec-domain-analyser`** - Exec domain analyser

### exec-drift-clone

Manage exec drift clone

- **`cipp-cli exec-drift-clone`** - Exec drift clone

### exec-durable-functions

Manage exec durable functions

- **`cipp-cli exec-durable-functions`** - Exec durable functions

### exec-edit-calendar-permissions

Manage exec edit calendar permissions

- **`cipp-cli exec-edit-calendar-permissions`** - Exec edit calendar permissions

### exec-edit-mailbox-permissions

Manage exec edit mailbox permissions

- **`cipp-cli exec-edit-mailbox-permissions`** - Exec edit mailbox permissions

### exec-edit-template

Manage exec edit template

- **`cipp-cli exec-edit-template`** - Exec edit template

### exec-email-forward

Manage exec email forward

- **`cipp-cli exec-email-forward`** - Exec email forward

### exec-enable-archive

Manage exec enable archive

- **`cipp-cli exec-enable-archive`** - Exec enable archive

### exec-enable-auto-expanding-archive

Manage exec enable auto expanding archive

- **`cipp-cli exec-enable-auto-expanding-archive`** - Exec enable auto expanding archive

### exec-exchange-role-repair

Manage exec exchange role repair

- **`cipp-cli exec-exchange-role-repair`** - Exec exchange role repair

### exec-exclude-licenses

Manage exec exclude licenses

- **`cipp-cli exec-exclude-licenses`** - Exec exclude licenses

### exec-exclude-tenant

Manage exec exclude tenant

- **`cipp-cli exec-exclude-tenant`** - Exec exclude tenant

### exec-extension-mapping

Manage exec extension mapping

- **`cipp-cli exec-extension-mapping`** - Exec extension mapping

### exec-extension-ninja-one-queue

Manage exec extension ninja one queue

- **`cipp-cli exec-extension-ninja-one-queue`** - Exec extension ninja one queue

### exec-extension-sync

Manage exec extension sync

- **`cipp-cli exec-extension-sync`** - Exec extension sync

### exec-extension-test

Manage exec extension test

- **`cipp-cli exec-extension-test`** - Exec extension test

### exec-extensions-config

Manage exec extensions config

- **`cipp-cli exec-extensions-config`** - Exec extensions config

### exec-feature-flag

Manage exec feature flag

- **`cipp-cli exec-feature-flag`** - Exec feature flag

### exec-gdapaccess-assignment

Manage exec gdapaccess assignment

- **`cipp-cli exec-gdapaccess-assignment`** - Exec gdapaccess assignment

### exec-gdapinvite

Manage exec gdapinvite

- **`cipp-cli exec-gdapinvite`** - Exec gdapinvite

### exec-gdapinvite-approved

Manage exec gdapinvite approved

- **`cipp-cli exec-gdapinvite-approved`** - Exec gdapinvite approved

### exec-gdapremove-garole

Manage exec gdapremove garole

- **`cipp-cli exec-gdapremove-garole`** - Exec gdapremove garole

### exec-gdaprole-template

Manage exec gdaprole template

- **`cipp-cli exec-gdaprole-template`** - Exec gdaprole template

### exec-gdaptrace

Manage exec gdaptrace

- **`cipp-cli exec-gdaptrace`** - GDAP Access Path Testing:
        1. Validates input parameters (TenantFilter and UPN)
        2. Retrieves customer tenant information
        3. Gets all active GDAP relationships for the customer tenant
        4. Locates the UPN in the partner tenant
        5. Gets user's transitive group memberships (handles nested groups automatically)
        6. For each GDAP relationship:
           - Retrieves all access assignments (mapped security groups)
           - For each group: checks user membership (direct or nested) and traces the path
           - Maps roles to relationships and groups
        7. For each of the 15 GDAP roles:
           - Finds all relationships/groups that have this role assigned
           - Checks if user is a member of any group with this role
           - Builds complete access path showing how user gets the role (if they do)
        8. Returns comprehensive JSON with role-centric view and complete path traces

### exec-geo-iplookup

Manage exec geo iplookup

- **`cipp-cli exec-geo-iplookup`** - Exec geo iplookup

### exec-get-local-admin-password

Manage exec get local admin password

- **`cipp-cli exec-get-local-admin-password`** - Exec get local admin password

### exec-get-recovery-key

Manage exec get recovery key

- **`cipp-cli exec-get-recovery-key`** - Exec get recovery key

### exec-git-hub-action

Manage exec git hub action

- **`cipp-cli exec-git-hub-action`** - Call GitHub API
    .ROLE
        CIPP.Extension.ReadWrite
    .FUNCTIONALITY
        Entrypoint,AnyTenant
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### exec-graph-explorer-preset

Manage exec graph explorer preset

- **`cipp-cli exec-graph-explorer-preset`** - Exec graph explorer preset

### exec-groups-delete

Manage exec groups delete

- **`cipp-cli exec-groups-delete`** - Exec groups delete

### exec-groups-delivery-management

Manage exec groups delivery management

- **`cipp-cli exec-groups-delivery-management`** - Exec groups delivery management

### exec-groups-hide-from-gal

Manage exec groups hide from gal

- **`cipp-cli exec-groups-hide-from-gal`** - Exec groups hide from gal

### exec-hide-from-gal

Manage exec hide from gal

- **`cipp-cli exec-hide-from-gal`** - Exec hide from gal

### exec-hveuser

Manage exec hveuser

- **`cipp-cli exec-hveuser`** - Exec hveuser

### exec-incidents-list

Manage exec incidents list

- **`cipp-cli exec-incidents-list`** - Exec incidents list

### exec-jitadmin

Manage exec jitadmin

- **`cipp-cli exec-jitadmin`** - Just-in-time admin management API endpoint. This function can create users, add roles, remove roles, delete, or disable a user.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### exec-jitadmin-settings

Manage exec jitadmin settings

- **`cipp-cli exec-jitadmin-settings`** - Exec jitadmin settings

### exec-license-search

Manage exec license search

- **`cipp-cli exec-license-search`** - Exec license search

### exec-list-app-id

Manage exec list app id

- **`cipp-cli exec-list-app-id create`** - Exec list app id
- **`cipp-cli exec-list-app-id list`** - Exec list app id

### exec-list-backup

Manage exec list backup

- **`cipp-cli exec-list-backup`** - Exec list backup

### exec-log-retention-config

Manage exec log retention config

- **`cipp-cli exec-log-retention-config`** - Exec log retention config

### exec-mail-test

Manage exec mail test

- **`cipp-cli exec-mail-test`** - Exec mail test

### exec-mailbox-mobile-devices

Manage exec mailbox mobile devices

- **`cipp-cli exec-mailbox-mobile-devices`** - Exec mailbox mobile devices

### exec-mailbox-restore

Manage exec mailbox restore

- **`cipp-cli exec-mailbox-restore`** - Exec mailbox restore

### exec-maintenance-scripts

Manage exec maintenance scripts

- **`cipp-cli exec-maintenance-scripts`** - Exec maintenance scripts

### exec-manage-retention-policies

Manage exec manage retention policies

- **`cipp-cli exec-manage-retention-policies`** - Exec manage retention policies

### exec-manage-retention-tags

Manage exec manage retention tags

- **`cipp-cli exec-manage-retention-tags`** - Exec manage retention tags

### exec-mdo-alerts-list

Manage exec mdo alerts list

- **`cipp-cli exec-mdo-alerts-list`** - Exec mdo alerts list

### exec-modify-cal-perms

Manage exec modify cal perms

- **`cipp-cli exec-modify-cal-perms`** - Exec modify cal perms

### exec-modify-contact-perms

Manage exec modify contact perms

- **`cipp-cli exec-modify-contact-perms`** - Exec modify contact perms

### exec-modify-mbperms

Manage exec modify mbperms

- **`cipp-cli exec-modify-mbperms`** - Exec modify mbperms

### exec-named-location

Manage exec named location

- **`cipp-cli exec-named-location`** - Exec named location

### exec-new-safe-links-policy

Manage exec new safe links policy

- **`cipp-cli exec-new-safe-links-policy`** - This function creates a new Safe Links policy and an associated rule.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### exec-notification-config

Manage exec notification config

- **`cipp-cli exec-notification-config`** - Exec notification config

### exec-offboard-tenant

Manage exec offboard tenant

- **`cipp-cli exec-offboard-tenant`** - Exec offboard tenant

### exec-offboard-user

Manage exec offboard user

- **`cipp-cli exec-offboard-user`** - Offboard a user with configurable options for mailbox conversion, access delegation, license removal, and more

### exec-offload-functions

Manage exec offload functions

- **`cipp-cli exec-offload-functions`** - Exec offload functions

### exec-onboard-tenant

Manage exec onboard tenant

- **`cipp-cli exec-onboard-tenant`** - Exec onboard tenant

### exec-one-drive-short-cut

Manage exec one drive short cut

- **`cipp-cli exec-one-drive-short-cut`** - Exec one drive short cut

### exec-onedrive-provision

Manage exec onedrive provision

- **`cipp-cli exec-onedrive-provision`** - Exec onedrive provision

### exec-partner-mode

Manage exec partner mode

- **`cipp-cli exec-partner-mode`** - Exec partner mode

### exec-partner-webhook

Manage exec partner webhook

- **`cipp-cli exec-partner-webhook`** - Exec partner webhook

### exec-password-config

Manage exec password config

- **`cipp-cli exec-password-config`** - Exec password config

### exec-password-never-expires

Manage exec password never expires

- **`cipp-cli exec-password-never-expires`** - Exec password never expires

### exec-per-user-mfa

Manage exec per user mfa

- **`cipp-cli exec-per-user-mfa`** - Exec per user mfa

### exec-permission-repair

Manage exec permission repair

- **`cipp-cli exec-permission-repair`** - Merges new permissions from the SAM manifest into the AppPermissions entry for CIPP-SAM.
    .FUNCTIONALITY
        Entrypoint
    .ROLE
        CIPP.AppSettings.ReadWrite
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### exec-quarantine-management

Manage exec quarantine management

- **`cipp-cli exec-quarantine-management`** - Exec quarantine management

### exec-remove-mailbox-rule

Manage exec remove mailbox rule

- **`cipp-cli exec-remove-mailbox-rule`** - Exec remove mailbox rule

### exec-remove-restricted-user

Manage exec remove restricted user

- **`cipp-cli exec-remove-restricted-user`** - Removes a user from the restricted senders list in Exchange Online.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### exec-remove-teams-voice-phone-number-assignment

Manage exec remove teams voice phone number assignment

- **`cipp-cli exec-remove-teams-voice-phone-number-assignment`** - Exec remove teams voice phone number assignment

### exec-remove-tenant

Manage exec remove tenant

- **`cipp-cli exec-remove-tenant`** - Exec remove tenant

### exec-rename-apdevice

Manage exec rename apdevice

- **`cipp-cli exec-rename-apdevice`** - Exec rename apdevice

### exec-reprocess-user-licenses

Manage exec reprocess user licenses

- **`cipp-cli exec-reprocess-user-licenses`** - Exec reprocess user licenses

### exec-reset-mfa

Manage exec reset mfa

- **`cipp-cli exec-reset-mfa`** - Exec reset mfa

### exec-reset-pass

Manage exec reset pass

- **`cipp-cli exec-reset-pass`** - Exec reset pass

### exec-restore-backup

Manage exec restore backup

- **`cipp-cli exec-restore-backup`** - Exec restore backup

### exec-restore-deleted

Manage exec restore deleted

- **`cipp-cli exec-restore-deleted`** - Exec restore deleted

### exec-revoke-sessions

Manage exec revoke sessions

- **`cipp-cli exec-revoke-sessions`** - Exec revoke sessions

### exec-run-backup

Manage exec run backup

- **`cipp-cli exec-run-backup`** - Exec run backup

### exec-run-tenant-group-rule

Manage exec run tenant group rule

- **`cipp-cli exec-run-tenant-group-rule`** - This function executes dynamic tenant group rules for immediate membership updates
    .FUNCTIONALITY
        Entrypoint,AnyTenant
    .ROLE
        Tenant.Groups.ReadWrite
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### exec-samapp-permissions

Manage exec samapp permissions

- **`cipp-cli exec-samapp-permissions`** - Exec samapp permissions

### exec-samroles

Manage exec samroles

- **`cipp-cli exec-samroles`** - Exec samroles

### exec-samsetup

Manage exec samsetup

- **`cipp-cli exec-samsetup`** - Exec samsetup

### exec-schedule-mailbox-vacation

Manage exec schedule mailbox vacation

- **`cipp-cli exec-schedule-mailbox-vacation`** - Exec schedule mailbox vacation

### exec-schedule-ooovacation

Manage exec schedule ooovacation

- **`cipp-cli exec-schedule-ooovacation`** - Exec schedule ooovacation

### exec-scheduler-billing-run

Manage exec scheduler billing run

- **`cipp-cli exec-scheduler-billing-run`** - Exec scheduler billing run

### exec-send-org-message

Manage exec send org message

- **`cipp-cli exec-send-org-message`** - Exec send org message

### exec-send-push

Manage exec send push

- **`cipp-cli exec-send-push`** - Exec send push

### exec-service-principals

Manage exec service principals

- **`cipp-cli exec-service-principals`** - Exec service principals

### exec-set-apdevice-group-tag

Manage exec set apdevice group tag

- **`cipp-cli exec-set-apdevice-group-tag`** - Exec set apdevice group tag

### exec-set-calendar-processing

Manage exec set calendar processing

- **`cipp-cli exec-set-calendar-processing`** - Exec set calendar processing

### exec-set-cippauto-backup

Manage exec set cippauto backup

- **`cipp-cli exec-set-cippauto-backup`** - Exec set cippauto backup

### exec-set-cloud-managed

Manage exec set cloud managed

- **`cipp-cli exec-set-cloud-managed`** - Sets the cloud-managed status of a user, group, or contact.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### exec-set-litigation-hold

Manage exec set litigation hold

- **`cipp-cli exec-set-litigation-hold`** - Exec set litigation hold

### exec-set-mailbox-email-size

Manage exec set mailbox email size

- **`cipp-cli exec-set-mailbox-email-size`** - Exec set mailbox email size

### exec-set-mailbox-locale

Manage exec set mailbox locale

- **`cipp-cli exec-set-mailbox-locale`** - Exec set mailbox locale

### exec-set-mailbox-quota

Manage exec set mailbox quota

- **`cipp-cli exec-set-mailbox-quota`** - Exec set mailbox quota

### exec-set-mailbox-retention-policies

Manage exec set mailbox retention policies

- **`cipp-cli exec-set-mailbox-retention-policies`** - Exec set mailbox retention policies

### exec-set-mailbox-rule

Manage exec set mailbox rule

- **`cipp-cli exec-set-mailbox-rule`** - Exec set mailbox rule

### exec-set-mdo-alert

Manage exec set mdo alert

- **`cipp-cli exec-set-mdo-alert`** - Exec set mdo alert

### exec-set-oo-o

Manage exec set oo o

- **`cipp-cli exec-set-oo-o`** - Exec set oo o

### exec-set-package-tag

Manage exec set package tag

- **`cipp-cli exec-set-package-tag`** - Exec set package tag

### exec-set-recipient-limits

Manage exec set recipient limits

- **`cipp-cli exec-set-recipient-limits`** - Exec set recipient limits

### exec-set-retention-hold

Manage exec set retention hold

- **`cipp-cli exec-set-retention-hold`** - Exec set retention hold

### exec-set-security-alert

Manage exec set security alert

- **`cipp-cli exec-set-security-alert`** - Exec set security alert

### exec-set-security-incident

Manage exec set security incident

- **`cipp-cli exec-set-security-incident`** - Exec set security incident

### exec-set-share-point-member

Manage exec set share point member

- **`cipp-cli exec-set-share-point-member`** - Exec set share point member

### exec-set-user-photo

Manage exec set user photo

- **`cipp-cli exec-set-user-photo`** - Exec set user photo

### exec-share-point-perms

Manage exec share point perms

- **`cipp-cli exec-share-point-perms`** - Exec share point perms

### exec-standard-convert

Manage exec standard convert

- **`cipp-cli exec-standard-convert`** - Exec standard convert

### exec-standards-run

Manage exec standards run

- **`cipp-cli exec-standards-run`** - Exec standards run

### exec-start-managed-folder-assistant

Manage exec start managed folder assistant

- **`cipp-cli exec-start-managed-folder-assistant`** - Exec start managed folder assistant

### exec-sync-apdevices

Manage exec sync apdevices

- **`cipp-cli exec-sync-apdevices`** - Exec sync apdevices

### exec-sync-dep

Manage exec sync dep

- **`cipp-cli exec-sync-dep`** - Syncs devices from Apple Business Manager to Intune
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)
    $APIName = $Request.Params.CIPPEndpoint
    $Headers = $Request.Headers

### exec-sync-vpp

Manage exec sync vpp

- **`cipp-cli exec-sync-vpp`** - Exec sync vpp

### exec-teams-voice-phone-number-assignment

Manage exec teams voice phone number assignment

- **`cipp-cli exec-teams-voice-phone-number-assignment`** - Exec teams voice phone number assignment

### exec-tenant-group

Manage exec tenant group

- **`cipp-cli exec-tenant-group`** - This function is used to manage tenant groups in CIPP
    .FUNCTIONALITY
        Entrypoint,AnyTenant
    .ROLE
        Tenant.Groups.ReadWrite
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### exec-test-run

Manage exec test run

- **`cipp-cli exec-test-run`** - Exec test run

### exec-time-settings

Manage exec time settings

- **`cipp-cli exec-time-settings`** - Exec time settings

### exec-token-exchange

Manage exec token exchange

- **`cipp-cli exec-token-exchange`** - Exec token exchange

### exec-universal-search

Manage exec universal search

- **`cipp-cli exec-universal-search`** - Exec universal search

### exec-universal-search-v2

Manage exec universal search v2

- **`cipp-cli exec-universal-search-v2`** - Exec universal search v2

### exec-update-drift-deviation

Manage exec update drift deviation

- **`cipp-cli exec-update-drift-deviation`** - Exec update drift deviation

### exec-update-refresh-token

Manage exec update refresh token

- **`cipp-cli exec-update-refresh-token`** - Exec update refresh token

### exec-update-secure-score

Manage exec update secure score

- **`cipp-cli exec-update-secure-score`** - Exec update secure score

### exec-user-bookmarks

Manage exec user bookmarks

- **`cipp-cli exec-user-bookmarks`** - Exec user bookmarks

### exec-user-settings

Manage exec user settings

- **`cipp-cli exec-user-settings`** - Exec user settings

### exec-webhook-subscriptions

Manage exec webhook subscriptions

- **`cipp-cli exec-webhook-subscriptions`** - Exec webhook subscriptions

### get-cipp-alerts

Manage get cipp alerts

- **`cipp-cli get-cipp-alerts`** - Get cipp alerts

### get-version

Manage get version

- **`cipp-cli get-version`** - Get version

### list-admin-portal-licenses

Manage list admin portal licenses

- **`cipp-cli list-admin-portal-licenses`** - List admin portal licenses

### list-alerts-queue

Manage list alerts queue

- **`cipp-cli list-alerts-queue`** - List alerts queue

### list-all-tenant-device-compliance

Manage list all tenant device compliance

- **`cipp-cli list-all-tenant-device-compliance`** - List all tenant device compliance

### list-anti-phishing-filters

Manage list anti phishing filters

- **`cipp-cli list-anti-phishing-filters`** - List anti phishing filters

### list-apdevices

Manage list apdevices

- **`cipp-cli list-apdevices`** - List apdevices

### list-api-test

Manage list api test

- **`cipp-cli list-api-test`** - List api test

### list-app-approval-templates

Manage list app approval templates

- **`cipp-cli list-app-approval-templates`** - List app approval templates

### list-app-consent-requests

Manage list app consent requests

- **`cipp-cli list-app-consent-requests`** - List app consent requests

### list-app-protection-policies

Manage list app protection policies

- **`cipp-cli list-app-protection-policies`** - List app protection policies

### list-app-status

Manage list app status

- **`cipp-cli list-app-status`** - List app status

### list-application-queue

Manage list application queue

- **`cipp-cli list-application-queue`** - List application queue

### list-apps

Manage list apps

- **`cipp-cli list-apps`** - List apps

### list-apps-repository

Manage list apps repository

- **`cipp-cli list-apps-repository`** - List apps repository

### list-assignment-filter-templates

Manage list assignment filter templates

- **`cipp-cli list-assignment-filter-templates`** - List assignment filter templates

### list-assignment-filters

Manage list assignment filters

- **`cipp-cli list-assignment-filters`** - List assignment filters

### list-audit-log-searches

Manage list audit log searches

- **`cipp-cli list-audit-log-searches`** - List audit log searches

### list-audit-log-test

Manage list audit log test

- **`cipp-cli list-audit-log-test`** - List audit log test

### list-audit-logs

Manage list audit logs

- **`cipp-cli list-audit-logs`** - List audit logs

### list-autopilotconfig

Manage list autopilotconfig

- **`cipp-cli list-autopilotconfig`** - List autopilotconfig

### list-available-tests

Manage list available tests

- **`cipp-cli list-available-tests`** - List available tests

### list-azure-adconnect-status

Manage list azure adconnect status

- **`cipp-cli list-azure-adconnect-status`** - List azure adconnect status

### list-basic-auth

Manage list basic auth

- **`cipp-cli list-basic-auth`** - List basic auth

### list-bpa

Manage list bpa

- **`cipp-cli list-bpa`** - List bpa

### list-bpatemplates

Manage list bpatemplates

- **`cipp-cli list-bpatemplates`** - List bpatemplates

### list-breaches-account

Manage list breaches account

- **`cipp-cli list-breaches-account`** - List breaches account

### list-breaches-tenant

Manage list breaches tenant

- **`cipp-cli list-breaches-tenant`** - List breaches tenant

### list-calendar-permissions

Manage list calendar permissions

- **`cipp-cli list-calendar-permissions`** - List calendar permissions

### list-catemplates

Manage list catemplates

- **`cipp-cli list-catemplates`** - List catemplates

### list-check-ext-alerts

Manage list check ext alerts

- **`cipp-cli list-check-ext-alerts`** - List check ext alerts

### list-community-repos

Manage list community repos

- **`cipp-cli list-community-repos`** - This function lists community repositories in Table Storage
    .FUNCTIONALITY
        Entrypoint,AnyTenant
    .ROLE
        CIPP.Core.Read
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### list-compliance-policies

Manage list compliance policies

- **`cipp-cli list-compliance-policies`** - List compliance policies

### list-conditional-access-policies

Manage list conditional access policies

- **`cipp-cli list-conditional-access-policies`** - List conditional access policies

### list-conditional-access-policy-changes

Manage list conditional access policy changes

- **`cipp-cli list-conditional-access-policy-changes`** - List conditional access policy changes

### list-connection-filter

Manage list connection filter

- **`cipp-cli list-connection-filter`** - List connection filter

### list-connection-filter-templates

Manage list connection filter templates

- **`cipp-cli list-connection-filter-templates`** - List connection filter templates

### list-contact-permissions

Manage list contact permissions

- **`cipp-cli list-contact-permissions`** - List contact permissions

### list-contact-templates

Manage list contact templates

- **`cipp-cli list-contact-templates`** - List contact templates

### list-contacts

Manage list contacts

- **`cipp-cli list-contacts`** - List contacts

### list-csplicenses

Manage list csplicenses

- **`cipp-cli list-csplicenses`** - List csplicenses

### list-cspsku

Manage list cspsku

- **`cipp-cli list-cspsku`** - List cspsku

### list-custom-data-mappings

Manage list custom data mappings

- **`cipp-cli list-custom-data-mappings`** - List custom data mappings

### list-custom-role

Manage list custom role

- **`cipp-cli list-custom-role`** - List custom role

### list-custom-variables

Manage list custom variables

- **`cipp-cli list-custom-variables`** - List custom variables

### list-dbcache

Manage list dbcache

- **`cipp-cli list-dbcache`** - List dbcache

### list-defender-state

Manage list defender state

- **`cipp-cli list-defender-state`** - List defender state

### list-defender-tvm

Manage list defender tvm

- **`cipp-cli list-defender-tvm`** - List defender tvm

### list-deleted-items

Manage list deleted items

- **`cipp-cli list-deleted-items`** - List deleted items

### list-detected-app-devices

Manage list detected app devices

- **`cipp-cli list-detected-app-devices`** - List detected app devices

### list-detected-apps

Manage list detected apps

- **`cipp-cli list-detected-apps`** - List detected apps

### list-device-details

Manage list device details

- **`cipp-cli list-device-details`** - List device details

### list-devices

Manage list devices

- **`cipp-cli list-devices`** - List devices

### list-diagnostics-presets

Manage list diagnostics presets

- **`cipp-cli list-diagnostics-presets`** - List diagnostics presets

### list-directory-objects

Manage list directory objects

- **`cipp-cli list-directory-objects`** - List directory objects

### list-domain-analyser

Manage list domain analyser

- **`cipp-cli list-domain-analyser`** - List domain analyser

### list-domain-health

Manage list domain health

- **`cipp-cli list-domain-health`** - List domain health

### list-domains

Manage list domains

- **`cipp-cli list-domains`** - List domains

### list-equipment

Manage list equipment

- **`cipp-cli list-equipment`** - List equipment

### list-ex-connector-templates

Manage list ex connector templates

- **`cipp-cli list-ex-connector-templates`** - List ex connector templates

### list-exchange-connectors

Manage list exchange connectors

- **`cipp-cli list-exchange-connectors`** - List exchange connectors

### list-excluded-licenses

Manage list excluded licenses

- **`cipp-cli list-excluded-licenses`** - List excluded licenses

### list-exo-request

Manage list exo request

- **`cipp-cli list-exo-request`** - List exo request

### list-extension-cache-data

Manage list extension cache data

- **`cipp-cli list-extension-cache-data`** - This function is used to list the extension cache data.
    .FUNCTIONALITY
        Entrypoint
    .ROLE
        CIPP.Core.Read
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)
    $TenantFilter = $Request.Query.tenantFilter ?? $Request.Body.tenantFilter
    $DataTypes = $Request.Query.dataTypes -split ',' ?? $Request.Body.dataTypes ?? 'All'

### list-extension-sync

Manage list extension sync

- **`cipp-cli list-extension-sync`** - List extension sync

### list-extensions-config

Manage list extensions config

- **`cipp-cli list-extensions-config`** - List extensions config

### list-external-tenant-info

Manage list external tenant info

- **`cipp-cli list-external-tenant-info`** - List external tenant info

### list-feature-flags

Manage list feature flags

- **`cipp-cli list-feature-flags`** - List feature flags

### list-function-parameters

Manage list function parameters

- **`cipp-cli list-function-parameters`** - List function parameters

### list-function-stats

Manage list function stats

- **`cipp-cli list-function-stats`** - List function stats

### list-gdapaccess-assignments

Manage list gdapaccess assignments

- **`cipp-cli list-gdapaccess-assignments`** - List gdapaccess assignments

### list-gdapinvite

Manage list gdapinvite

- **`cipp-cli list-gdapinvite`** - List gdapinvite

### list-gdaproles

Manage list gdaproles

- **`cipp-cli list-gdaproles`** - List gdaproles

### list-generic-test-function

Manage list generic test function

- **`cipp-cli list-generic-test-function`** - List generic test function

### list-git-hub-release-notes

Manage list git hub release notes

- **`cipp-cli list-git-hub-release-notes`** - Returns release metadata for the provided repository and semantic version. Hotfix
        versions (e.g. v8.5.2) map back to the base release tag (v8.5.0).
    .FUNCTIONALITY
        Entrypoint,AnyTenant
    .ROLE
        CIPP.Core.Read
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### list-global-address-list

Manage list global address list

- **`cipp-cli list-global-address-list`** - List global address list

### list-graph-bulk-request

Manage list graph bulk request

- **`cipp-cli list-graph-bulk-request`** - List graph bulk request

### list-graph-explorer-presets

Manage list graph explorer presets

- **`cipp-cli list-graph-explorer-presets`** - List graph explorer presets

### list-graph-request

Manage list graph request

- **`cipp-cli list-graph-request`** - List graph request

### list-group-sender-authentication

Manage list group sender authentication

- **`cipp-cli list-group-sender-authentication`** - List group sender authentication

### list-group-templates

Manage list group templates

- **`cipp-cli list-group-templates`** - List group templates

### list-groups

Manage list groups

- **`cipp-cli list-groups`** - List groups

### list-halo-clients

Manage list halo clients

- **`cipp-cli list-halo-clients`** - List halo clients

### list-inactive-accounts

Manage list inactive accounts

- **`cipp-cli list-inactive-accounts`** - List inactive accounts

### list-intune-intents

Manage list intune intents

- **`cipp-cli list-intune-intents`** - List intune intents

### list-intune-policy

Manage list intune policy

- **`cipp-cli list-intune-policy`** - List intune policy

### list-intune-reusable-setting-templates

Manage list intune reusable setting templates

- **`cipp-cli list-intune-reusable-setting-templates`** - List intune reusable setting templates

### list-intune-reusable-settings

Manage list intune reusable settings

- **`cipp-cli list-intune-reusable-settings`** - List intune reusable settings

### list-intune-script

Manage list intune script

- **`cipp-cli list-intune-script`** - List intune script

### list-intune-templates

Manage list intune templates

- **`cipp-cli list-intune-templates`** - List intune templates

### list-ipwhitelist

Manage list ipwhitelist

- **`cipp-cli list-ipwhitelist`** - List ipwhitelist

### list-jitadmin

Manage list jitadmin

- **`cipp-cli list-jitadmin`** - List Just-in-time admin users for a tenant or all tenants.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### list-jitadmin-templates

Manage list jitadmin templates

- **`cipp-cli list-jitadmin-templates`** - List jitadmin templates

### list-known-ipdb

Manage list known ipdb

- **`cipp-cli list-known-ipdb`** - List known ipdb

### list-licenses

Manage list licenses

- **`cipp-cli list-licenses`** - List licenses

### list-logs

Manage list logs

- **`cipp-cli list-logs`** - List logs

### list-mail-quarantine

Manage list mail quarantine

- **`cipp-cli list-mail-quarantine`** - List mail quarantine

### list-mail-quarantine-message

Manage list mail quarantine message

- **`cipp-cli list-mail-quarantine-message`** - List mail quarantine message

### list-mailbox-cas

Manage list mailbox cas

- **`cipp-cli list-mailbox-cas`** - List mailbox cas

### list-mailbox-forwarding

Manage list mailbox forwarding

- **`cipp-cli list-mailbox-forwarding`** - List mailbox forwarding

### list-mailbox-mobile-devices

Manage list mailbox mobile devices

- **`cipp-cli list-mailbox-mobile-devices`** - List mailbox mobile devices

### list-mailbox-restores

Manage list mailbox restores

- **`cipp-cli list-mailbox-restores`** - List mailbox restores

### list-mailbox-rules

Manage list mailbox rules

- **`cipp-cli list-mailbox-rules`** - List mailbox rules

### list-mailboxes

Manage list mailboxes

- **`cipp-cli list-mailboxes`** - List mailboxes

### list-malware-filters

Manage list malware filters

- **`cipp-cli list-malware-filters`** - List malware filters

### list-message-trace

Manage list message trace

- **`cipp-cli list-message-trace`** - List message trace

### list-mfausers

Manage list mfausers

- **`cipp-cli list-mfausers`** - List mfausers

### list-named-locations

Manage list named locations

- **`cipp-cli list-named-locations`** - List named locations

### list-new-user-defaults

Manage list new user defaults

- **`cipp-cli list-new-user-defaults`** - List new user defaults

### list-notification-config

Manage list notification config

- **`cipp-cli list-notification-config`** - List notification config

### list-oauth-apps

Manage list oauth apps

- **`cipp-cli list-oauth-apps`** - List oauth apps

### list-oo-o

Manage list oo o

- **`cipp-cli list-oo-o`** - List oo o

### list-org

Manage list org

- **`cipp-cli list-org`** - List org

### list-partner-relationships

Manage list partner relationships

- **`cipp-cli list-partner-relationships`** - List partner relationships

### list-pending-webhooks

Manage list pending webhooks

- **`cipp-cli list-pending-webhooks`** - List pending webhooks

### list-per-user-mfa

Manage list per user mfa

- **`cipp-cli list-per-user-mfa`** - List per user mfa

### list-potential-apps

Manage list potential apps

- **`cipp-cli list-potential-apps`** - List potential apps

### list-quarantine-policy

Manage list quarantine policy

- **`cipp-cli list-quarantine-policy`** - List quarantine policy

### list-restricted-users

Manage list restricted users

- **`cipp-cli list-restricted-users`** - Lists users from the restricted senders list in Exchange Online.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)
    # Interact with query parameters or the body of the request.
    $TenantFilter = $Request.Query.tenantFilter

### list-roles

Manage list roles

- **`cipp-cli list-roles`** - List roles

### list-room-lists

Manage list room lists

- **`cipp-cli list-room-lists`** - List room lists

### list-rooms

Manage list rooms

- **`cipp-cli list-rooms`** - List rooms

### list-safe-attachments-filters

Manage list safe attachments filters

- **`cipp-cli list-safe-attachments-filters`** - List safe attachments filters

### list-safe-links-policy

Manage list safe links policy

- **`cipp-cli list-safe-links-policy`** - This function is used to list the Safe Links policies in the tenant, including unmatched rules and policies.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)
    $APIName = $Request.Params.CIPPEndpoint
    $Headers = $Request.Headers

### list-safe-links-policy-details

Manage list safe links policy details

- **`cipp-cli list-safe-links-policy-details`** - This function retrieves details for a specific Safe Links policy and rule.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### list-safe-links-policy-template-details

Manage list safe links policy template details

- **`cipp-cli list-safe-links-policy-template-details`** - This function retrieves details for a specific Safe Links policy template.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### list-safe-links-policy-templates

Manage list safe links policy templates

- **`cipp-cli list-safe-links-policy-templates`** - List safe links policy templates

### list-scheduled-item-details

Manage list scheduled item details

- **`cipp-cli list-scheduled-item-details`** - List scheduled item details

### list-scheduled-items

Manage list scheduled items

- **`cipp-cli list-scheduled-items`** - List scheduled items

### list-service-health

Manage list service health

- **`cipp-cli list-service-health`** - List service health

### list-shared-mailbox-account-enabled

Manage list shared mailbox account enabled

- **`cipp-cli list-shared-mailbox-account-enabled`** - List shared mailbox account enabled

### list-shared-mailbox-statistics

Manage list shared mailbox statistics

- **`cipp-cli list-shared-mailbox-statistics`** - List shared mailbox statistics

### list-sharepoint-admin-url

Manage list sharepoint admin url

- **`cipp-cli list-sharepoint-admin-url`** - List sharepoint admin url

### list-sharepoint-quota

Manage list sharepoint quota

- **`cipp-cli list-sharepoint-quota`** - List sharepoint quota

### list-sharepoint-settings

Manage list sharepoint settings

- **`cipp-cli list-sharepoint-settings`** - List sharepoint settings

### list-sign-ins

Manage list sign ins

- **`cipp-cli list-sign-ins`** - List sign ins

### list-site-members

Manage list site members

- **`cipp-cli list-site-members`** - List site members

### list-sites

Manage list sites

- **`cipp-cli list-sites`** - List sites

### list-spam-filter-templates

Manage list spam filter templates

- **`cipp-cli list-spam-filter-templates`** - List spam filter templates

### list-spamfilter

Manage list spamfilter

- **`cipp-cli list-spamfilter`** - List spamfilter

### list-standard-templates

Manage list standard templates

- **`cipp-cli list-standard-templates`** - List standard templates

### list-standards

Manage list standards

- **`cipp-cli list-standards`** - List standards

### list-standards-compare

Manage list standards compare

- **`cipp-cli list-standards-compare`** - List standards compare

### list-teams

Manage list teams

- **`cipp-cli list-teams`** - List teams

### list-teams-activity

Manage list teams activity

- **`cipp-cli list-teams-activity`** - List teams activity

### list-teams-lis-location

Manage list teams lis location

- **`cipp-cli list-teams-lis-location`** - List teams lis location

### list-teams-voice

Manage list teams voice

- **`cipp-cli list-teams-voice`** - List teams voice

### list-tenant-alignment

Manage list tenant alignment

- **`cipp-cli list-tenant-alignment`** - List tenant alignment

### list-tenant-allow-block-list

Manage list tenant allow block list

- **`cipp-cli list-tenant-allow-block-list`** - List tenant allow block list

### list-tenant-details

Manage list tenant details

- **`cipp-cli list-tenant-details`** - List tenant details

### list-tenant-drift

Manage list tenant drift

- **`cipp-cli list-tenant-drift`** - List tenant drift

### list-tenant-groups

Manage list tenant groups

- **`cipp-cli list-tenant-groups`** - Entrypoint for listing tenant groups

### list-tenant-onboarding

Manage list tenant onboarding

- **`cipp-cli list-tenant-onboarding`** - List tenant onboarding

### list-tenants

Manage list tenants

- **`cipp-cli list-tenants`** - List tenants

### list-test-reports

Manage list test reports

- **`cipp-cli list-test-reports`** - Lists all available test reports from JSON files and database

### list-tests

Manage list tests

- **`cipp-cli list-tests`** - Lists tests for a tenant, optionally filtered by report ID

### list-transport-rules

Manage list transport rules

- **`cipp-cli list-transport-rules`** - List transport rules

### list-transport-rules-templates

Manage list transport rules templates

- **`cipp-cli list-transport-rules-templates`** - List transport rules templates

### list-user-conditional-access-policies

Manage list user conditional access policies

- **`cipp-cli list-user-conditional-access-policies`** - List user conditional access policies

### list-user-counts

Manage list user counts

- **`cipp-cli list-user-counts`** - List user counts

### list-user-devices

Manage list user devices

- **`cipp-cli list-user-devices`** - List user devices

### list-user-groups

Manage list user groups

- **`cipp-cli list-user-groups`** - List user groups

### list-user-mailbox-details

Manage list user mailbox details

- **`cipp-cli list-user-mailbox-details`** - List user mailbox details

### list-user-mailbox-rules

Manage list user mailbox rules

- **`cipp-cli list-user-mailbox-rules`** - List user mailbox rules

### list-user-photo

Manage list user photo

- **`cipp-cli list-user-photo`** - List user photo

### list-user-settings

Manage list user settings

- **`cipp-cli list-user-settings`** - List user settings

### list-user-signin-logs

Manage list user signin logs

- **`cipp-cli list-user-signin-logs`** - List user signin logs

### list-user-trusted-blocked-senders

Manage list user trusted blocked senders

- **`cipp-cli list-user-trusted-blocked-senders`** - List user trusted blocked senders

### list-users

Manage list users

- **`cipp-cli list-users`** - List users

### list-users-and-groups

Manage list users and groups

- **`cipp-cli list-users-and-groups`** - List users and groups

### list-webhook-alert

Manage list webhook alert

- **`cipp-cli list-webhook-alert`** - List webhook alert

### listmailbox-permissions

Manage listmailbox permissions

- **`cipp-cli listmailbox-permissions`** - Listmailbox permissions

### patch-user

Manage patch user

- **`cipp-cli patch-user`** - Patch user

### public-phishing-check

Manage public phishing check

- **`cipp-cli public-phishing-check`** - Public phishing check

### public-ping

Manage public ping

- **`cipp-cli public-ping`** - Public ping

### public-webhooks

Manage public webhooks

- **`cipp-cli public-webhooks`** - Public webhooks

### remove-apdevice

Manage remove apdevice

- **`cipp-cli remove-apdevice`** - Remove apdevice

### remove-app

Manage remove app

- **`cipp-cli remove-app`** - Remove app

### remove-assignment-filter-template

Manage remove assignment filter template

- **`cipp-cli remove-assignment-filter-template`** - Remove assignment filter template

### remove-autopilot-config

Manage remove autopilot config

- **`cipp-cli remove-autopilot-config`** - Remove autopilot config

### remove-bpatemplate

Manage remove bpatemplate

- **`cipp-cli remove-bpatemplate`** - Remove bpatemplate

### remove-capolicy

Manage remove capolicy

- **`cipp-cli remove-capolicy`** - Remove capolicy

### remove-catemplate

Manage remove catemplate

- **`cipp-cli remove-catemplate`** - Remove catemplate

### remove-connectionfilter-template

Manage remove connectionfilter template

- **`cipp-cli remove-connectionfilter-template`** - Remove connectionfilter template

### remove-contact

Manage remove contact

- **`cipp-cli remove-contact`** - Remove contact

### remove-contact-templates

Manage remove contact templates

- **`cipp-cli remove-contact-templates`** - Remove contact templates

### remove-deleted-object

Manage remove deleted object

- **`cipp-cli remove-deleted-object`** - Remove deleted object

### remove-ex-connector

Manage remove ex connector

- **`cipp-cli remove-ex-connector`** - Remove ex connector

### remove-ex-connector-template

Manage remove ex connector template

- **`cipp-cli remove-ex-connector-template`** - Remove ex connector template

### remove-group-template

Manage remove group template

- **`cipp-cli remove-group-template`** - Remove group template

### remove-intune-reusable-setting

Manage remove intune reusable setting

- **`cipp-cli remove-intune-reusable-setting`** - Remove intune reusable setting

### remove-intune-reusable-setting-template

Manage remove intune reusable setting template

- **`cipp-cli remove-intune-reusable-setting-template`** - Remove intune reusable setting template

### remove-intune-script

Manage remove intune script

- **`cipp-cli remove-intune-script`** - Remove intune script

### remove-intune-template

Manage remove intune template

- **`cipp-cli remove-intune-template`** - Remove intune template

### remove-jitadmin-template

Manage remove jitadmin template

- **`cipp-cli remove-jitadmin-template`** - Remove jitadmin template

### remove-policy

Manage remove policy

- **`cipp-cli remove-policy`** - Remove policy

### remove-quarantine-policy

Manage remove quarantine policy

- **`cipp-cli remove-quarantine-policy`** - Remove quarantine policy

### remove-queued-alert

Manage remove queued alert

- **`cipp-cli remove-queued-alert`** - Remove queued alert

### remove-queued-app

Manage remove queued app

- **`cipp-cli remove-queued-app`** - Remove queued app

### remove-safe-links-policy-template

Manage remove safe links policy template

- **`cipp-cli remove-safe-links-policy-template`** - Remove safe links policy template

### remove-scheduled-item

Manage remove scheduled item

- **`cipp-cli remove-scheduled-item`** - Removes a scheduled item from CIPP's scheduler.
    #>
    [CmdletBinding()]
    param($Request, $TriggerMetadata)

### remove-spamfilter

Manage remove spamfilter

- **`cipp-cli remove-spamfilter`** - Remove spamfilter

### remove-spamfilter-template

Manage remove spamfilter template

- **`cipp-cli remove-spamfilter-template`** - Remove spamfilter template

### remove-standard

Manage remove standard

- **`cipp-cli remove-standard`** - Remove standard

### remove-standard-template

Manage remove standard template

- **`cipp-cli remove-standard-template`** - Remove standard template

### remove-tenant-allow-block-list

Manage remove tenant allow block list

- **`cipp-cli remove-tenant-allow-block-list`** - Remove tenant allow block list

### remove-tenant-capabilities-cache

Manage remove tenant capabilities cache

- **`cipp-cli remove-tenant-capabilities-cache`** - Remove tenant capabilities cache

### remove-transport-rule

Manage remove transport rule

- **`cipp-cli remove-transport-rule`** - Remove transport rule

### remove-transport-rule-template

Manage remove transport rule template

- **`cipp-cli remove-transport-rule-template`** - Remove transport rule template

### remove-trusted-blocked-sender

Manage remove trusted blocked sender

- **`cipp-cli remove-trusted-blocked-sender`** - Remove trusted blocked sender

### remove-user

Manage remove user

- **`cipp-cli remove-user`** - Remove user

### remove-user-default-template

Manage remove user default template

- **`cipp-cli remove-user-default-template`** - Remove user default template

### set-auth-method

Manage set auth method

- **`cipp-cli set-auth-method`** - Set auth method


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
cipp-cli best-practice-analyser-list

# JSON for scripting and agents
cipp-cli best-practice-analyser-list --json

# Filter to specific fields
cipp-cli best-practice-analyser-list --json --select id,name,status

# Dry run  -  show the request without sending
cipp-cli best-practice-analyser-list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
cipp-cli best-practice-analyser-list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
cipp-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/cipp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `CIPP_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `cipp-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `cipp-cli doctor` to check credentials
- Verify the environment variable is set: `echo $CIPP_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 / 403 on every call**  -  Confirm the API client's IP allowlist includes this host, and that --base-url ends in /api (without it, requests hit the static frontend).
- **Requests return HTML instead of JSON**  -  Your base URL is missing /api and is hitting the web UI  -  re-run auth login with the /api suffix.
- **Calls fail intermittently on large tenants**  -  Microsoft Graph is throttling; the CLI honors Retry-After automatically  -  use fanout/bulk --resume to continue a halted batch.
- **Token expired mid-script**  -  auth login caches and auto-refreshes the client-credentials token; re-run auth login if the client secret rotated.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**CIPPAPIModule**](https://github.com/BNWEIN/CIPPAPIModule)  -  PowerShell
- [**cipp-mcp**](https://github.com/wyre-technology/cipp-mcp)  -  TypeScript
- [**CIPP-MCP**](https://github.com/davebirr/CIPP-MCP)  -  C#

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
