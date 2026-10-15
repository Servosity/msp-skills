---
name: connectsecure
description: "Use when the user asks to rank ConnectSecure (formerly CyberCNS) vulnerabilities across every client, find which clients and assets a CVE hits, prep a client QBR risk pack, find assets with stale or unauthenticated scans or silent agents, audit expired or self-approved vulnerability suppressions, review Active Directory password hygiene across clients, or run any ConnectSecure V4 API operation. Trigger phrases: `which clients have critical vulnerabilities`, `connectsecure exposure across all clients`, `are we affected by this CVE`, `prep the QBR risk numbers`, `find stale or unauthenticated scans`, `audit vulnerability suppressions`, `check AD password hygiene across clients`, `use connectsecure`, `run connectsecure`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "ConnectSecure"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - connectsecure-cli
    install:
      - kind: script
        bins: [connectsecure-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/connectsecure/install.ps1
---

# ConnectSecure  -  Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `connectsecure-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

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

The installer downloads the `connectsecure-cli` and `connectsecure-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

The CLI mints and refreshes the ConnectSecure JWT from your tenant, client ID and secret, and covers the full V4 API, including create, update and delete endpoints. Sync companies, assets and vulnerability reports into local SQLite, then run exposure, portfolio, blind-spots, suppression-audit and ad-hygiene to rank risk across every client offline; qbr and cve-impact call the live API.

## When to Use This CLI

Use this CLI when an agent needs ConnectSecure vulnerability, asset, agent, compliance or Active Directory data, especially questions that span all of an MSP's clients at once. It is the right tool for cross-client triage, QBR prep, scan coverage checks and CVE impact lookups, and for scripted changes to companies, tags, suppressions and schedules.

## Anti-triggers

Do not use this CLI for:
- Deploying or installing the ConnectSecure agent on endpoints; use your RMM.
- Real-time EDR alert response; ConnectSecure is a scanning and assessment platform.
- Editing report templates visually; use the ConnectSecure portal report builder.

## Unique Capabilities

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

## Safety for agents

- **Read freely:** the seven analysis commands, `sync`, `search`, and every `list` / `get` except the credential reads below.
- **Ask a human first, every time:** `credentials list` / `get`, `integration-credentials list` / `get`, `integration-rules list` / `get`, `scheduler list` / `get`, `attack-surface-results list` / `get` and `agents uninstall-secret` return stored scan, integration and job secrets (and `credentials create` / `update`, `integration-credentials create` / `update`, `integration-rules create` / `update` and `scheduler create` / `update` send one) (and attack-surface creds and hashes) in plaintext, and `custom-domains list` / `get` return TLS certificate details whose contents ConnectSecure does not document (`custom-domains create` uploads the private key); `tail` on those resources prints them each poll; `export credentials` (and `export integration_credentials`, `export integration_rules`, `export scheduler`, `export attack_surface_results`, `export custom_domains`) prints them to stdout or writes them to the file named by `-o`; `agent-credentials-mapping create` / `update` changes which stored credentials a scan agent uses, and `company-mappings create` / `update` which integration credential a client uses; and `--deliver webhook:<url>` on any command posts its output off the machine.
- **Preview, then ask:** any create, update or `report-builder` upload runs immediately - pass `--dry-run` first and show the user the request. `patching patch-now` (alias `patching create`) and `scheduler create` / `update` / `update-schedule` change software on client machines (a schedule can force a reboot, tenant-wide), `companies scan` / `companies external-scan` send scan traffic at a client network, `agents reset` (alias `agents create`) moves agents between companies, `suppress-vulnerability create` / `update` hides a finding from every report, `discovery-settings`, `application-baseline-rules`, `pii-scan-settings`, `agent-discoverysettings-mapping` and `attack-surface-domain` `create` / `update` change what gets scanned and reported, and `assets create` / `update` can switch the scan credentials and discovery settings an asset uses or deprecate it out of reporting; never run these, `scheduler remove-schedule`, `assets bulk-deprecate`, `import` (one API write per line of a file) or any `delete` without explicit confirmation.
- **MCP read-only mode:** when the server runs with `CONNECTSECURE_MCP_READ_ONLY=1` it refuses every non-GET call and hides `sync`, `export`, `import` and `tail`; credential reads through `connectsecure_execute` still pass. MCP tools also never take a local file path for the CLI to read (`teach` / `teach-playbook` notes and playbook files, `import --input`, the `custom-domains create` certificate and key, `report-builder` uploads); run those from the CLI.
- Credential records, integration credentials, integration rules, scheduled jobs, attack-surface results, custom domains (as a precaution: their certificate details are undocumented) and the agent uninstall secret are never written to the local store or response cache, so `sync --resources credentials` refuses before any request, stores nothing and exits non-zero by design; ask the live API when you need them.
- To create or update credentials, integration credentials, integration rules or scheduled jobs, pass the JSON body on stdin with `--stdin` (wrapped as `{"data": {...}}`, the shape `--dry-run` shows for the flag form; `--stdin` sends it exactly as given) instead of `--data-params` / `--data-integration-params` / `--data-password`, so the secret stays out of process arguments and shell history; `--dry-run` masks secret fields in its preview.
- You never need `session`: the CLI mints its own token. If a user insists, pipe the encoded token in with `--client-auth-token -` so the client secret stays out of process arguments.
- Refresh the mirror with `sync --full`, not a bare `sync`: only a full run prunes findings that were remediated since the last sync, so a bare re-sync leaves them counted as open in `exposure` and `portfolio`.
- Every other GET is cached on disk for 5 minutes. After a fix or suppression, re-run `qbr` or `cve-impact` with `--no-cache` or you get the earlier answer.

## Command Reference

**agent_credentials_mapping**  -  Manage agent credentials mapping

- `connectsecure-cli agent-credentials-mapping create`  -  Create agent credential mapping
- `connectsecure-cli agent-credentials-mapping delete`  -  Delete agent credential mapping
- `connectsecure-cli agent-credentials-mapping get`  -  Retrieve agent credential mapping
- `connectsecure-cli agent-credentials-mapping list`  -  Retrieve agent credentials mapping
- `connectsecure-cli agent-credentials-mapping update`  -  Update agent credential mapping

**agent_discoverysettings_mapping**  -  Manage agent discoverysettings mapping

- `connectsecure-cli agent-discoverysettings-mapping create`  -  Create agent discoverysetting mapping
- `connectsecure-cli agent-discoverysettings-mapping delete`  -  Delete agent discoverysetting mapping
- `connectsecure-cli agent-discoverysettings-mapping get`  -  Retrieve agent discoverysetting mapping
- `connectsecure-cli agent-discoverysettings-mapping list`  -  Retrieve agent discoverysettings mapping
- `connectsecure-cli agent-discoverysettings-mapping update`  -  Update agent discoverysetting mapping

**agents**  -  Manage agents

- `connectsecure-cli agents get`  -  Retrieve agent
- `connectsecure-cli agents list`  -  Retrieve agents
- `connectsecure-cli agents reset`  -  Migrate Agents
- `connectsecure-cli agents uninstall-secret`  -  Get uninstall secret token

**app_baseline_plan_assets**  -  Manage app baseline plan assets

- `connectsecure-cli app-baseline-plan-assets get`  -  Retrieve app baseline plan asset
- `connectsecure-cli app-baseline-plan-assets list`  -  Retrieve app baseline plan assets

**app_baseline_plan_company**  -  Manage app baseline plan company

- `connectsecure-cli app-baseline-plan-company get`  -  Retrieve app baseline plan company
- `connectsecure-cli app-baseline-plan-company list`  -  Retrieve app baseline plan company

**app_baseline_plan_global**  -  Manage app baseline plan global

- `connectsecure-cli app-baseline-plan-global get`  -  Retrieve app baseline plan global
- `connectsecure-cli app-baseline-plan-global list`  -  Retrieve app baseline plan global

**application_baseline_rules**  -  Manage application baseline rules

- `connectsecure-cli application-baseline-rules create`  -  Create application baseline rule
- `connectsecure-cli application-baseline-rules delete`  -  Delete application baseline rule
- `connectsecure-cli application-baseline-rules get`  -  Retrieve application baseline rule
- `connectsecure-cli application-baseline-rules list`  -  Retrieve application baseline rules
- `connectsecure-cli application-baseline-rules update`  -  Update application baseline rule

**asset_firewall_policy**  -  Manage asset firewall policy

- `connectsecure-cli asset-firewall-policy get`  -  Retrieve asset firewall policy
- `connectsecure-cli asset-firewall-policy list`  -  Retrieve asset firewall policy

**asset_installed_drivers**  -  Manage asset installed drivers

- `connectsecure-cli asset-installed-drivers get`  -  Retrieve asset installed driver
- `connectsecure-cli asset-installed-drivers list`  -  Retrieve asset installed drivers

**asset_interface**  -  Manage asset interface

- `connectsecure-cli asset-interface get`  -  Retrieve asset interface
- `connectsecure-cli asset-interface list`  -  Retrieve asset interface

**asset_msdt**  -  Manage asset msdt

- `connectsecure-cli asset-msdt get`  -  Retrieve asset msdt
- `connectsecure-cli asset-msdt list`  -  Retrieve asset msdt

**asset_ports**  -  Manage asset ports

- `connectsecure-cli asset-ports get`  -  Retrieve asset port
- `connectsecure-cli asset-ports list`  -  Retrieve asset ports

**asset_security_report_data**  -  Manage asset security report data

- `connectsecure-cli asset-security-report-data get`  -  Retrieve asset security report datum
- `connectsecure-cli asset-security-report-data list`  -  Retrieve asset security report data

**asset_shares**  -  Manage asset shares

- `connectsecure-cli asset-shares get`  -  Retrieve asset share
- `connectsecure-cli asset-shares list`  -  Retrieve asset shares

**asset_storages**  -  Manage asset storages

- `connectsecure-cli asset-storages get`  -  Retrieve asset storage
- `connectsecure-cli asset-storages list`  -  Retrieve asset storages

**asset_unqouted_services**  -  Manage asset unqouted services

- `connectsecure-cli asset-unqouted-services get`  -  Retrieve asset unqouted service
- `connectsecure-cli asset-unqouted-services list`  -  Retrieve asset unqouted services

**asset_user_shares**  -  Manage asset user shares

- `connectsecure-cli asset-user-shares get`  -  Retrieve asset user share
- `connectsecure-cli asset-user-shares list`  -  Retrieve asset user shares

**asset_video_info**  -  Manage asset video info

- `connectsecure-cli asset-video-info get`  -  Retrieve asset video info
- `connectsecure-cli asset-video-info list`  -  Retrieve asset video info

**asset_view**  -  Manage asset view

- `connectsecure-cli asset-view get`  -  Retrieve asset view
- `connectsecure-cli asset-view list`  -  Retrieve asset view

**asset_windows_compatibility**  -  Manage asset windows compatibility

- `connectsecure-cli asset-windows-compatibility`  -  Retrieve asset windows compatibility

**asset_windows_reboot_required**  -  Manage asset windows reboot required

- `connectsecure-cli asset-windows-reboot-required get`  -  Retrieve asset window reboot required
- `connectsecure-cli asset-windows-reboot-required list`  -  Retrieve asset windows reboot required

**assets**  -  Manage assets

- `connectsecure-cli assets bulk-deprecate`  -  Bulk deprecate assets
- `connectsecure-cli assets create`  -  Create asset
- `connectsecure-cli assets delete`  -  Delete asset
- `connectsecure-cli assets remediation-plan`  -  Retrieve records
- `connectsecure-cli assets update`  -  Update asset

**attack_surface_domain**  -  Manage attack surface domain

- `connectsecure-cli attack-surface-domain create`  -  Create attack surface domain
- `connectsecure-cli attack-surface-domain delete`  -  Delete attack surface domain
- `connectsecure-cli attack-surface-domain get`  -  Retrieve attack surface domain
- `connectsecure-cli attack-surface-domain list`  -  Retrieve attack surface domain
- `connectsecure-cli attack-surface-domain update`  -  Update attack surface domain

**attack_surface_results**  -  Manage attack surface results

- `connectsecure-cli attack-surface-results get`  -  Retrieve attack surface result
- `connectsecure-cli attack-surface-results list`  -  Retrieve attack surface results

**backup_software**  -  Manage backup software

- `connectsecure-cli backup-software create`  -  Create backup software
- `connectsecure-cli backup-software delete`  -  Delete backup software
- `connectsecure-cli backup-software get`  -  Retrieve backup software
- `connectsecure-cli backup-software list`  -  Retrieve backup software
- `connectsecure-cli backup-software update`  -  Update backup software

**bios_info**  -  Manage bios info

- `connectsecure-cli bios-info get`  -  Retrieve bio info
- `connectsecure-cli bios-info list`  -  Retrieve bios info

**browser_extensions**  -  Manage browser extensions

- `connectsecure-cli browser-extensions get`  -  Retrieve browser extension
- `connectsecure-cli browser-extensions list`  -  Retrieve browser extensions

**ciphers_view**  -  Manage ciphers view

- `connectsecure-cli ciphers-view get`  -  Retrieve cipher view
- `connectsecure-cli ciphers-view list`  -  Retrieve ciphers view

**companies**  -  Manage companies

- `connectsecure-cli companies create`  -  Create company
- `connectsecure-cli companies delete`  -  Delete company
- `connectsecure-cli companies external-scan`  -  Scan external endpoint
- `connectsecure-cli companies get`  -  Retrieve company
- `connectsecure-cli companies list`  -  Retrieve companies
- `connectsecure-cli companies scan`  -  Scan Company
- `connectsecure-cli companies update`  -  Update company

**company_mappings**  -  Manage company mappings

- `connectsecure-cli company-mappings create`  -  Create company mapping
- `connectsecure-cli company-mappings delete`  -  Delete company mapping
- `connectsecure-cli company-mappings get`  -  Retrieve company mapping
- `connectsecure-cli company-mappings list`  -  Retrieve company mappings
- `connectsecure-cli company-mappings update`  -  Update company mapping

**company_stats**  -  Manage company stats

- `connectsecure-cli company-stats get`  -  Retrieve company stat
- `connectsecure-cli company-stats list`  -  Retrieve company stats

**compliance**  -  Manage compliance

- `connectsecure-cli compliance`  -  Retrieve Compliance Types

**compliance_assessment**  -  Manage compliance assessment

- `connectsecure-cli compliance-assessment create`  -  Create compliance assessment
- `connectsecure-cli compliance-assessment delete`  -  Delete compliance assessment
- `connectsecure-cli compliance-assessment get`  -  Retrieve compliance assessment
- `connectsecure-cli compliance-assessment list`  -  Retrieve compliance assessment
- `connectsecure-cli compliance-assessment update`  -  Update compliance assessment

**credentials**  -  Manage credentials

- `connectsecure-cli credentials create`  -  Create credential
- `connectsecure-cli credentials delete`  -  Delete credential
- `connectsecure-cli credentials get`  -  Retrieve credential
- `connectsecure-cli credentials list`  -  Retrieve credentials
- `connectsecure-cli credentials update`  -  Update credential

**custom_domains**  -  Manage custom domains

- `connectsecure-cli custom-domains create`  -  Custom Domain
- `connectsecure-cli custom-domains get`  -  Retrieve custom domain
- `connectsecure-cli custom-domains list`  -  Retrieve custom domains

**custom_profile**  -  Manage custom profile

- `connectsecure-cli custom-profile create`  -  Create custom profile
- `connectsecure-cli custom-profile delete`  -  Delete custom profile
- `connectsecure-cli custom-profile get`  -  Retrieve custom profile
- `connectsecure-cli custom-profile list`  -  Retrieve custom profile
- `connectsecure-cli custom-profile update`  -  Update custom profile

**custom_ticketing_template**  -  Manage custom ticketing template

- `connectsecure-cli custom-ticketing-template create`  -  Create custom ticketing template
- `connectsecure-cli custom-ticketing-template delete`  -  Delete custom ticketing template
- `connectsecure-cli custom-ticketing-template get`  -  Retrieve custom ticketing template
- `connectsecure-cli custom-ticketing-template list`  -  Retrieve custom ticketing template
- `connectsecure-cli custom-ticketing-template update`  -  Update custom ticketing template

**cve**  -  Manage cve

- `connectsecure-cli cve`  -  Retrieve records

**discovery_settings**  -  Manage discovery settings

- `connectsecure-cli discovery-settings create`  -  Create discovery setting
- `connectsecure-cli discovery-settings delete`  -  Delete discovery setting
- `connectsecure-cli discovery-settings get`  -  Retrieve discovery setting
- `connectsecure-cli discovery-settings list`  -  Retrieve discovery settings
- `connectsecure-cli discovery-settings update`  -  Update discovery setting

**edr**  -  Manage edr

- `connectsecure-cli edr create`  -  Create edr
- `connectsecure-cli edr delete`  -  Delete edr
- `connectsecure-cli edr get`  -  Retrieve edr
- `connectsecure-cli edr list`  -  Retrieve edr
- `connectsecure-cli edr update`  -  Update edr

**event_set**  -  Manage event set

- `connectsecure-cli event-set create`  -  Create event set
- `connectsecure-cli event-set delete`  -  Delete event set
- `connectsecure-cli event-set get`  -  Retrieve event set
- `connectsecure-cli event-set list`  -  Retrieve event set
- `connectsecure-cli event-set update`  -  Update event set

**firewall_groups**  -  Manage firewall groups

- `connectsecure-cli firewall-groups get`  -  Retrieve firewall group
- `connectsecure-cli firewall-groups list`  -  Retrieve firewall groups

**firewall_interfaces**  -  Manage firewall interfaces

- `connectsecure-cli firewall-interfaces get`  -  Retrieve firewall interface
- `connectsecure-cli firewall-interfaces list`  -  Retrieve firewall interfaces

**firewall_license**  -  Manage firewall license

- `connectsecure-cli firewall-license get`  -  Retrieve firewall license
- `connectsecure-cli firewall-license list`  -  Retrieve firewall license

**firewall_rules**  -  Manage firewall rules

- `connectsecure-cli firewall-rules get`  -  Retrieve firewall rule
- `connectsecure-cli firewall-rules list`  -  Retrieve firewall rules

**firewall_users**  -  Manage firewall users

- `connectsecure-cli firewall-users get`  -  Retrieve firewall user
- `connectsecure-cli firewall-users list`  -  Retrieve firewall users

**firewall_zones**  -  Manage firewall zones

- `connectsecure-cli firewall-zones get`  -  Retrieve firewall zone
- `connectsecure-cli firewall-zones list`  -  Retrieve firewall zones

**integration_credentials**  -  Manage integration credentials

- `connectsecure-cli integration-credentials create`  -  Create integration credential
- `connectsecure-cli integration-credentials delete`  -  Delete integration credential
- `connectsecure-cli integration-credentials get`  -  Retrieve integration credential
- `connectsecure-cli integration-credentials list`  -  Retrieve integration credentials
- `connectsecure-cli integration-credentials update`  -  Update integration credential

**integration_rules**  -  Manage integration rules

- `connectsecure-cli integration-rules create`  -  Create integration rule
- `connectsecure-cli integration-rules delete`  -  Delete integration rule
- `connectsecure-cli integration-rules get`  -  Retrieve integration rule
- `connectsecure-cli integration-rules list`  -  Retrieve integration rules
- `connectsecure-cli integration-rules update`  -  Update integration rule

**jobs_view**  -  Manage jobs view

- `connectsecure-cli jobs-view get`  -  Retrieve job view
- `connectsecure-cli jobs-view list`  -  Retrieve jobs view

**patching**  -  Manage patching

- `connectsecure-cli patching patch-now`  -  Trigger application/OS patch (now or scheduled)
- `connectsecure-cli patching settings`  -  Retrieve records

**pii_scan_settings**  -  Manage pii scan settings

- `connectsecure-cli pii-scan-settings create`  -  Create pii scan setting
- `connectsecure-cli pii-scan-settings delete`  -  Delete pii scan setting
- `connectsecure-cli pii-scan-settings get`  -  Retrieve pii scan setting
- `connectsecure-cli pii-scan-settings list`  -  Retrieve pii scan settings
- `connectsecure-cli pii-scan-settings update`  -  Update pii scan setting

**report_builder**  -  Manage report builder

- `connectsecure-cli report-builder cover-page`  -  Report Settings
- `connectsecure-cli report-builder create-job`  -  Create Report
- `connectsecure-cli report-builder download-default-template`  -  Report Settings
- `connectsecure-cli report-builder report-link`  -  Download report
- `connectsecure-cli report-builder standard-report-settings`  -  Report Settings
- `connectsecure-cli report-builder standard-reports`  -  List standard report
- `connectsecure-cli report-builder update-cover-page`  -  Create Report
- `connectsecure-cli report-builder update-standard-report-settings`  -  Create Report
- `connectsecure-cli report-builder upload-template`  -  Report Settings

**report_jobs_view**  -  Manage report jobs view

- `connectsecure-cli report-jobs-view get`  -  Retrieve report job view
- `connectsecure-cli report-jobs-view list`  -  Retrieve report jobs view

**report_queries**  -  Manage report queries

- `connectsecure-cli report-queries ad-basic-info`  -  Retrieve ad basic info
- `connectsecure-cli report-queries ad-computers-view`  -  Retrieve ad computers view
- `connectsecure-cli report-queries ad-domain-details`  -  Retrieve ad domain details
- `connectsecure-cli report-queries ad-gpos-details`  -  Retrieve ad gpos details
- `connectsecure-cli report-queries ad-gpos-view`  -  Retrieve ad gpos view
- `connectsecure-cli report-queries ad-group-computers`  -  Retrieve ad group computers
- `connectsecure-cli report-queries ad-group-users`  -  Retrieve ad group users
- `connectsecure-cli report-queries ad-groups-view`  -  Retrieve ad groups view
- `connectsecure-cli report-queries ad-ous-view`  -  Retrieve ad ous view
- `connectsecure-cli report-queries ad-password-policies`  -  Retrieve ad password policies
- `connectsecure-cli report-queries ad-roles`  -  Retrieve ad roles
- `connectsecure-cli report-queries ad-roles-details`  -  Retrieve ad roles details
- `connectsecure-cli report-queries ad-roles-member`  -  Retrieve ad roles member
- `connectsecure-cli report-queries ad-user-licenses`  -  Retrieve ad user licenses
- `connectsecure-cli report-queries ad-users-view`  -  Retrieve ad users view
- `connectsecure-cli report-queries adaudit`  -  Retrieve adaudit
- `connectsecure-cli report-queries application-count`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-by-os`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-by-os-software-details`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-by-os-software-details-suppressed`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-by-product`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-by-product-suppressed`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-by-product-suppressed-tag`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-by-product-tag`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-net`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-net-suppressed`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-net-suppressed-tag`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-net-tag`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-os-patch`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-patching-asset-details`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-suppressed`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-suppressed-by-os`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-suppressed-tag`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-suppressed-tag-by-os`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-tag`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-tag-by-os`  -  Retrieve records
- `connectsecure-cli report-queries application-vulnerabilities-v2`  -  Retrieve records
- `connectsecure-cli report-queries asset-compliance-details`  -  Retrieve records
- `connectsecure-cli report-queries asset-compliance-report-data`  -  Retrieve records
- `connectsecure-cli report-queries asset-critical-vulnerabilities`  -  Retrieve records
- `connectsecure-cli report-queries asset-firewall-rules`  -  Retrieve asset firewall rules
- `connectsecure-cli report-queries asset-iptables-rules`  -  Retrieve asset iptables rules
- `connectsecure-cli report-queries asset-open-ports`  -  Retrieve asset open ports
- `connectsecure-cli report-queries asset-patches-info`  -  Retrieve asset patches info
- `connectsecure-cli report-queries asset-ports-view`  -  Retrieve records
- `connectsecure-cli report-queries asset-processes-running`  -  Retrieve asset processes running
- `connectsecure-cli report-queries asset-registry-misconfiguration`  -  Retrieve asset registry misconfiguration
- `connectsecure-cli report-queries asset-security-report-data`  -  Retrieve records
- `connectsecure-cli report-queries asset-security-report-data-bulk`  -  Retrieve asset security report data
- `connectsecure-cli report-queries asset-services`  -  Retrieve asset services
- `connectsecure-cli report-queries asset-software`  -  Retrieve records
- `connectsecure-cli report-queries asset-stats`  -  Retrieve asset stats
- `connectsecure-cli report-queries asset-users`  -  Retrieve asset users
- `connectsecure-cli report-queries asset-wise-vulnerabilities`  -  Retrieve records
- `connectsecure-cli report-queries assets`  -  Retrieve assets
- `connectsecure-cli report-queries assets-by-application`  -  Retrieve records
- `connectsecure-cli report-queries assets-by-application-suppressed`  -  Retrieve records
- `connectsecure-cli report-queries azure-ad-logs`  -  Retrieve azure ad logs
- `connectsecure-cli report-queries azure-licenses`  -  Retrieve azure licenses
- `connectsecure-cli report-queries azure-secure-score`  -  Retrieve azure secure score
- `connectsecure-cli report-queries cert-info-view`  -  Retrieve records
- `connectsecure-cli report-queries companies-by-application`  -  Retrieve records
- `connectsecure-cli report-queries companies-by-application-suppressed`  -  Retrieve records
- `connectsecure-cli report-queries companies-by-problem-group`  -  Retrieve records
- `connectsecure-cli report-queries companies-by-problem-group-suppressed`  -  Retrieve records
- `connectsecure-cli report-queries compliance-asset-info`  -  Retrieve records
- `connectsecure-cli report-queries compliance-check-asset-count`  -  Retrieve records
- `connectsecure-cli report-queries compliance-check-company-count`  -  Retrieve records
- `connectsecure-cli report-queries compliance-check-count`  -  Retrieve records
- `connectsecure-cli report-queries compliance-check-count-by-section`  -  Retrieve records
- `connectsecure-cli report-queries compliance-count`  -  Retrieve records
- `connectsecure-cli report-queries compliance-internal-checks`  -  Retrieve records
- `connectsecure-cli report-queries compliance-maturity`  -  Retrieve records
- `connectsecure-cli report-queries cron-jobs`  -  Retrieve cron jobs
- `connectsecure-cli report-queries distinct-agents-name`  -  Retrieve records
- `connectsecure-cli report-queries distinct-asset-ip`  -  Retrieve records
- `connectsecure-cli report-queries distinct-asset-name`  -  Retrieve records
- `connectsecure-cli report-queries distinct-discovered-protocols`  -  Retrieve records
- `connectsecure-cli report-queries distinct-os`  -  Retrieve records
- `connectsecure-cli report-queries distinct-platform`  -  Retrieve records
- `connectsecure-cli report-queries distinct-software`  -  Retrieve records
- `connectsecure-cli report-queries distinct-tags`  -  Retrieve records
- `connectsecure-cli report-queries event-stats`  -  Retrieve event stats
- `connectsecure-cli report-queries event-tickets`  -  Retrieve event tickets
- `connectsecure-cli report-queries external-asset-externalscan`  -  Retrieve records
- `connectsecure-cli report-queries external-asset-ports-data`  -  Retrieve records
- `connectsecure-cli report-queries external-asset-ssl-attack`  -  Retrieve records
- `connectsecure-cli report-queries external-asset-ssl-ciphers`  -  Retrieve records
- `connectsecure-cli report-queries external-asset-vulnerabilities`  -  Retrieve records
- `connectsecure-cli report-queries get-assets-by-problem`  -  Retrieve records
- `connectsecure-cli report-queries get-assets-problem`  -  Retrieve records
- `connectsecure-cli report-queries get-computer-details`  -  Retrieve get computer details
- `connectsecure-cli report-queries get-groups-details`  -  Retrieve get groups details
- `connectsecure-cli report-queries get-ous-details`  -  Retrieve get ous details
- `connectsecure-cli report-queries get-remediate-records`  -  Retrieve records
- `connectsecure-cli report-queries get-remediation`  -  Retrieve records
- `connectsecure-cli report-queries get-user-details`  -  Retrieve get user details
- `connectsecure-cli report-queries job-details`  -  Retrieve job details
- `connectsecure-cli report-queries job-details-view`  -  Retrieve job details view
- `connectsecure-cli report-queries kernel-modules`  -  Retrieve kernel modules
- `connectsecure-cli report-queries lightweight-assets`  -  Retrieve records
- `connectsecure-cli report-queries notification-tickets-view`  -  Retrieve records
- `connectsecure-cli report-queries notification-tickets-view-legacy`  -  Retrieve notification tickets view
- `connectsecure-cli report-queries os-pending-patches`  -  Retrieve records
- `connectsecure-cli report-queries os-pending-patches-companies`  -  Retrieve records
- `connectsecure-cli report-queries ports-assets-details`  -  Retrieve records
- `connectsecure-cli report-queries ports-count`  -  Retrieve records
- `connectsecure-cli report-queries ports-view`  -  Retrieve records
- `connectsecure-cli report-queries problem-group-summary`  -  Retrieve records
- `connectsecure-cli report-queries problem-group-summary-asset-company-count`  -  Retrieve records
- `connectsecure-cli report-queries problems-info`  -  Retrieve records
- `connectsecure-cli report-queries problems-remediations-summary`  -  Retrieve records
- `connectsecure-cli report-queries problems-ssl-for-asset`  -  Retrieve records
- `connectsecure-cli report-queries problems-summary`  -  Retrieve records
- `connectsecure-cli report-queries problems-summary-asset-details`  -  Retrieve records
- `connectsecure-cli report-queries problems-summary-global`  -  Retrieve records
- `connectsecure-cli report-queries problems-summary-group-by-companies`  -  Retrieve records
- `connectsecure-cli report-queries problems-summary-tag`  -  Retrieve records
- `connectsecure-cli report-queries registry-problems-company`  -  Retrieve records
- `connectsecure-cli report-queries registry-problems-remediation`  -  Retrieve records
- `connectsecure-cli report-queries registry-problems-remediation-asset-details`  -  Retrieve records
- `connectsecure-cli report-queries registry-problems-summary`  -  Retrieve records
- `connectsecure-cli report-queries remediate-records`  -  Retrieve records
- `connectsecure-cli report-queries remediate-records-assets`  -  Retrieve records
- `connectsecure-cli report-queries remediate-records-companies`  -  Retrieve records
- `connectsecure-cli report-queries remediate-records-days`  -  Retrieve records
- `connectsecure-cli report-queries remediated-registry-solution-plan`  -  Retrieve records
- `connectsecure-cli report-queries remediation-companies`  -  Retrieve records
- `connectsecure-cli report-queries remediation-plan-asset-details`  -  Retrieve records
- `connectsecure-cli report-queries remediation-plan-asset-details-by-epss`  -  Retrieve records
- `connectsecure-cli report-queries remediation-plan-asset-epss-details`  -  Retrieve records
- `connectsecure-cli report-queries remediation-plan-by-company`  -  Retrieve records
- `connectsecure-cli report-queries remediation-plan-include-company`  -  Retrieve records
- `connectsecure-cli report-queries remediation-plan-include-company-days`  -  Retrieve records
- `connectsecure-cli report-queries remediation-velocity-application`  -  Retrieve records
- `connectsecure-cli report-queries remediation-velocity-application-asset-details`  -  Retrieve records
- `connectsecure-cli report-queries remediation-velocity-company`  -  Retrieve records
- `connectsecure-cli report-queries resolved-remediation`  -  Retrieve records
- `connectsecure-cli report-queries risk-score`  -  Retrieve records
- `connectsecure-cli report-queries selinux-settings`  -  Retrieve selinux settings
- `connectsecure-cli report-queries suid-permissions`  -  Retrieve suid permissions
- `connectsecure-cli report-queries suppress-vulnerability-problems`  -  Retrieve records
- `connectsecure-cli report-queries suppress-vulnerability-problems-sid`  -  Retrieve records
- `connectsecure-cli report-queries suppress-vulnerability-solution`  -  Retrieve records
- `connectsecure-cli report-queries suppressed-problems`  -  Retrieve records
- `connectsecure-cli report-queries sw-problems-remediations-view`  -  Retrieve records
- `connectsecure-cli report-queries sw-problems-remediations-view-assetwise`  -  Retrieve records
- `connectsecure-cli report-queries sw-problems-remediations-view-vul`  -  Retrieve records
- `connectsecure-cli report-queries system-events-view`  -  Retrieve system events view
- `connectsecure-cli report-queries system-events-view-ticketid`  -  Retrieve system events view
- `connectsecure-cli report-queries tags-view`  -  Retrieve records
- `connectsecure-cli report-queries total-asset-count`  -  Retrieve records
- `connectsecure-cli report-queries ufw-firewall-rules`  -  Retrieve ufw firewall rules
- `connectsecure-cli report-queries unconfirmed-key-check`  -  Retrieve records
- `connectsecure-cli report-queries unconfirmed-open-ports-key-check`  -  Retrieve records
- `connectsecure-cli report-queries user-enabled-stats`  -  Retrieve user enabled stats
- `connectsecure-cli report-queries user-event-stats`  -  Retrieve user event stats
- `connectsecure-cli report-queries user-locked-stats`  -  Retrieve user locked stats
- `connectsecure-cli report-queries vulnerabilities-count`  -  Retrieve records
- `connectsecure-cli report-queries vulnerabilities-details`  -  Retrieve records
- `connectsecure-cli report-queries vulnerabilities-details-suppressed`  -  Retrieve records

**scheduler**  -  Manage scheduler

- `connectsecure-cli scheduler create`  -  Create scheduler
- `connectsecure-cli scheduler delete`  -  Delete scheduler
- `connectsecure-cli scheduler get`  -  Retrieve scheduler
- `connectsecure-cli scheduler list`  -  Retrieve scheduler
- `connectsecure-cli scheduler remove-schedule`  -  Update schedule
- `connectsecure-cli scheduler update`  -  Update scheduler
- `connectsecure-cli scheduler update-schedule`  -  Update schedule

**session**  -  Manage session

- `connectsecure-cli session`  -  Authorize

**suppress_vulnerability**  -  Manage suppress vulnerability

- `connectsecure-cli suppress-vulnerability create`  -  Create suppres vulnerability
- `connectsecure-cli suppress-vulnerability delete`  -  Delete suppres vulnerability
- `connectsecure-cli suppress-vulnerability get`  -  Retrieve suppres vulnerability
- `connectsecure-cli suppress-vulnerability list`  -  Retrieve suppress vulnerability
- `connectsecure-cli suppress-vulnerability update`  -  Update suppres vulnerability

**tag_rules**  -  Manage tag rules

- `connectsecure-cli tag-rules create`  -  Create tag rule
- `connectsecure-cli tag-rules delete`  -  Delete tag rule
- `connectsecure-cli tag-rules get`  -  Retrieve tag rule
- `connectsecure-cli tag-rules list`  -  Retrieve tag rules
- `connectsecure-cli tag-rules update`  -  Update tag rule

**tags**  -  Manage tags

- `connectsecure-cli tags create`  -  Create tag
- `connectsecure-cli tags delete`  -  Delete tag
- `connectsecure-cli tags get`  -  Retrieve tag
- `connectsecure-cli tags list`  -  Retrieve tags
- `connectsecure-cli tags update`  -  Update tag

**users**  -  Manage users

- `connectsecure-cli users`  -  Retrieve Users

**windows_protection_status**  -  Manage windows protection status

- `connectsecure-cli windows-protection-status get`  -  Retrieve window protection status
- `connectsecure-cli windows-protection-status list`  -  Retrieve windows protection status


**ad-hygiene**


- `connectsecure-cli ad-hygiene`  -  Reads the synced local store (run sync first): per client, risky Active Directory accounts (password never expires, not required, no MFA, stale logon), weak password policies and Azure secure score.


**blind-spots**


- `connectsecure-cli blind-spots`  -  Reads the synced local store (run sync first): list assets whose scan data cannot be trusted (stale discovery, unauthenticated scan, silent agent), sorted by client with reason codes; the unauthenticated check is a keyword heuristic unless you pass --auth-ok.


**cve-impact**


- `connectsecure-cli cve-impact`  -  Live API call: for one CVE, show affected clients with asset counts, affected software, available fixes and CVSS/EPSS, and report any assets the API could not attribute to a client.


**exposure**


- `connectsecure-cli exposure`  -  Reads the synced local store (run sync first): rank open vulnerabilities across all clients at asset level, one row per problem x asset x client, top 50 by default (--limit 0 for all), filtered by EPSS, severity, KEV or first-seen window.


**portfolio**


- `connectsecure-cli portfolio`  -  Reads the synced local store (run sync first): one row per client with asset count, critical and high findings, max EPSS, KEV count, risk score, stale assets and active suppressions.


**qbr**


- `connectsecure-cli qbr`  -  Live API call: one client's QBR numbers in one call, with risk score, latest company stats, open findings by age, top fixes and remediation speed; compliance pass/fail is added only when --compliance-type, --compliance-platform and --compliance-section are all set.


**suppression-audit**


- `connectsecure-cli suppression-audit`  -  Reads the synced local store (run sync first): flag suppressions that expired but are still active, expire soon, never expire, or were unapproved or self-approved, with the suppressed issue's current EPSS.




## Freshness Contract

This printed CLI owns bounded freshness only for registered store-backed read command paths. In `--data-source auto` mode, those paths check `sync_state` and may run a bounded refresh before reading local data. `--data-source local` never refreshes. `--data-source live` reads the API (through the 5-minute on-disk response cache; add `--no-cache` to force a fresh call) and does not mutate the local store. Set `CONNECTSECURE_NO_AUTO_REFRESH=1` to skip the freshness hook without changing source selection.

Covered paths: Pre-read refresh applies to the generated list and get commands of synced resources, for example `companies list`, `agents list`, `tags list` and `edr list`. Credential resources (`credentials`, `integration-credentials`, `integration-rules`), scheduled jobs (`scheduler`, whose rows carry a password field), attack-surface results (`attack-surface-results`, whose rows carry creds and hashes fields), custom domains (`custom-domains`, a precaution: their certificate details are undocumented) and the agent uninstall secret are never cached or synced locally. The local novel commands (`exposure`, `portfolio`, `blind-spots`, `suppression-audit`, `ad-hygiene`) read the store directly and print a `hint:` line naming any resource that was never synced.

When JSON output uses the generated provenance envelope, freshness metadata appears at `meta.freshness`. Treat it as current-cache freshness for the covered command path, not a guarantee of complete historical backfill or API-specific enrichment.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
connectsecure-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

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

## Auth Setup

ConnectSecure uses a token exchange, not OAuth. Set CONNECTSECURE_BASE_URL to your pod URL (Profile > API Documentation, for example https://pod106.myconnectsecure.com, not the portal login URL), plus CONNECTSECURE_TENANT (your portal login tenant), CONNECTSECURE_CLIENT_ID and CONNECTSECURE_CLIENT_SECRET (Settings > Users > user > API Key). The CLI calls POST /w/authorize with Client-Auth-Token set to base64 of tenant, a literal plus sign, client_id, a colon and client_secret, caches the returned JWT and user ID, and sends Authorization: Bearer plus X-USER-ID on every call. If you already have a token, set CONNECTSECURE_ACCESS_TOKEN and CONNECTSECURE_USER_ID instead.

Run `connectsecure-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array)
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  connectsecure-cli companies list --agent --select id,name,domain
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Writes are not confirmation-gated**  -  API writes (create, update, delete, scans) run immediately and are not `--yes`-gated; preview them with `--dry-run` first. `--yes` only answers this CLI's own local prompts, such as `profile delete`.
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local (`qbr` and `cve-impact` print their report object at the top level unless `--agent` wraps it in the same envelope; `export` and `tail` stream with no envelope: `export` prints raw records as JSONL, or one JSON array with `--format json`, and `tail` prints one `{"event", "timestamp", "data"}` object per line) (`live` also covers a response served from the 5-minute on-disk cache). A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `CONNECTSECURE_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `CONNECTSECURE_CONFIG_DIR`, `CONNECTSECURE_DATA_DIR`, `CONNECTSECURE_STATE_DIR`, `CONNECTSECURE_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `CONNECTSECURE_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, the store `data-<hash>.db` (its path is `db_path` in `doctor --agent`), cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `connectsecure-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

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

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `CONNECTSECURE_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `CONNECTSECURE_HOME`, or `doctor` will not find credentials left under the former root.

## Automatic learning

This CLI ships a self-capturing learning loop. The CLI does its own bookkeeping: every invocation is journaled locally, a failed flag followed by a corrected retry auto-derives a `flag_alias` candidate, and a `teach` on a query family without a playbook auto-synthesizes a `playbook_candidate` from the session's journal. Your job is judgment only: `recall` first, act on surfaced candidates, `teach` the final answer, `playbook amend` when you observe a correction. You never record failures by hand.

### Step 1: `recall` before any discovery

Before list/search/drill commands on a new user question, pass the question as an argv or MCP tool argument to `recall --agent`. Do not interpolate user-controlled text into a shell command line.

Quoted `recall "<question>"` breaks on an apostrophe, which is ordinary English. A quoted heredoc breaks when a body line equals the delimiter, and that delimiter is published in these docs. Write the question with a non-shell file-writing tool, then read it back as data:

```bash
# Write the question verbatim with your file-writing tool (no shell involved).
# Command substitution on a file only ever yields data  -  the shell never
# parses the file's bytes as syntax.
QUERY=$(cat /path/to/question.txt)
connectsecure-cli recall "$QUERY" --agent
```

Prefer MCP: pass the question as the tool's query argument. `"$QUERY"` after a file read is argv-safe; putting the question itself in the command text is not.

The response envelope:

```json
{
  "query": "...",
  "normalized": "<normalized form>",
  "query_entities": ["..."],
  "found": true | false,
  "match_score": 0.0,
  "results": [
    { "resource_id": "...", "resource_type": "...", "venue": "...",
      "confidence": 2, "entity_match": "exact|partial|unknown",
      "source": "taught|preseed|pattern", "warnings": ["..."] }
  ],
  "mismatches": [ /* only when --debug-mismatches */ ],
  "warnings": [ /* top-level */ ],
  "candidates": [
    { "id": 12, "class": "flag_alias | playbook_candidate",
      "summary": "...", "sightings": 3, "last_seen": "...",
      "rationale": "...",
      "next_action": ["<trial command>", "connectsecure-cli learnings confirm 12"] }
  ],
  "playbook": {
    "query_family": "...",
    "playbook": {
      "steps": [ { "cmd": "<command with {slot} substitution>", "purpose": "..." } ],
      "entity_slots": ["$ENTITY"],
      "expected_tool_calls": 3
    },
    "slots_resolved": { "$ENTITY": { "token": "<live token>", "canonical": "<canonical>" } },
    "notes": "<workarounds + gotchas for this query family>"
  },
  "notes": "<duplicate surface for non-playbook callers>"
}
```

Empty-store short-circuit: if the store has no learnings, playbooks, or candidates yet (recall finds nothing and `learnings list` and `learnings candidates` are both empty), skip recall for the rest of this session instead of taxing every query; resume recall-first once something has been taught.

### Step 2: decision tree

Read `candidates`, `playbook`, `notes`, `results[0]`, and warnings in that order:

```
if Candidates present (warnings include "candidates_present"):
    -> candidates are try-then-confirm, never facts. Follow each candidate's
       two-step next_action verbatim: run the trial command first, then run
       `learnings confirm <id>` only after the trial verified the behavior.
       Reject a wrong candidate with `learnings reject <id>`.
    -> NEVER re-teach something recall surfaced as a candidate; confirm or
       reject that candidate instead of teaching a duplicate.
    -> candidates ride alongside playbooks and resource hits, not instead of
       them; continue with the branches below after acting on them.

if Playbook present:
    -> READ Playbook.notes verbatim FIRST (workarounds + gotchas the CLI surface doesn't expose)
    -> replay Playbook.steps in order, substituting Playbook.slots_resolved entries
       for the entity slot tokens. If a step's slot is unresolved, fall back to
       discovery for that step only.
    -> the Playbook's expected_tool_calls is a budget; if you find yourself running
       materially more, record the divergence via `connectsecure-cli playbook amend`
       at end-of-session.

elif Notes present (no Playbook):
    -> read Notes verbatim before any discovery step; they carry known gotchas
       for this query family even when no structured choreography exists yet.

elif Found AND Results[0].EntityMatch == "exact" AND Results[0].Confidence >= 2:
    -> skip discovery; fetch live data for Results[*].ResourceID in parallel

elif Found AND Results[0].EntityMatch == "partial":
    -> candidate hint, NOT a hit; read the resource title to validate before trusting

elif (any row in Mismatches[] when --debug-mismatches was passed):
    -> treat as cold start; the stored learning is for a different entity
       (different canonical resolved from query_entities)

else:  // Found == false, no playbook, no notes
    -> cold start; run discovery normally; teach the answer afterward (Step 4).
       If the family has no playbook yet, that teach auto-synthesizes a
       playbook candidate from this session's journal - you do not need to
       record one by hand.
```

Playbook and Notes are orthogonal to the per-resource path. A recall response can carry both a Playbook AND a `Results[]` hit - use both: the Playbook tells you which choreography to run; the resource hits short-circuit specific steps. Default to skipping `mismatches`; pass `--debug-mismatches` only when investigating cold-start surprises.

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `connectsecure-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `connectsecure-cli sync --full` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
connectsecure-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
connectsecure-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
connectsecure-cli teach-playbook \
  --query "$QUERY" \
  --playbook-file ~/playbooks/<shape>.json \
  --notes-file ~/playbooks/<shape>-notes.md
```

Playbook files are JSON with `steps`, `entity_slots`, `expected_tool_calls`. Notes files are markdown carrying the gotchas verbatim. File-free callers (MCP-only agents) pass the same content inline: `--playbook-json` and `--playbook-notes` on the integrated `teach` form, `--playbook-json` and `--notes` on `teach-playbook`. On the integrated `teach` form, the playbook flags are optional - omit them entirely for a resource-only teach. On the standalone `teach-playbook` form, at least one of the playbook and notes flags must be set; both empty is rejected. Playbooks are keyed on the structural query family (entities stripped) so a recipe taught from one entity-shaped query applies to every other query of the same shape, with `slots_resolved` binding the live query's canonical at recall time.

When you DO find a playbook on a future recall, treat it as ground truth: replay the steps with `slots_resolved` substitutions, skip the discovery that the choreography already documents, and read `notes` before any step.

### Step 6: `playbook amend &` when your debug response identifies a correction

If your debug-protocol response identifies a concrete correction the notes or playbook should know  -  a workaround, an undocumented endpoint shape, a stale field name, observed schema drift, an empty-payload fallback  -  fire `playbook amend` BEFORE emitting your user-facing response. Same fire-and-forget posture as `teach`. Pass the query and note as argv/MCP arguments, or write each with a non-shell file tool and read them back (`QUERY=$(cat ...)`, `NOTE=$(cat ...)`). Do not interpolate either string into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
NOTE=$(cat /path/to/note.txt)
connectsecure-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between seasons, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-team / per-athlete / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`connectsecure-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `CONNECTSECURE_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
connectsecure-cli feedback "the --since flag is inclusive but docs say exclusive"
connectsecure-cli feedback --stdin < notes.txt
connectsecure-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `CONNECTSECURE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CONNECTSECURE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
connectsecure-cli profile save briefing --json
connectsecure-cli --profile briefing companies list
connectsecure-cli profile list --json
connectsecure-cli profile show briefing
connectsecure-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Async Jobs

Four read-only views poll for a job to finish: `jobs-view list`, `report-jobs-view list`, `report-queries job-details` and `report-queries job-details-view`. They accept `--wait` (block until the job reaches a terminal status), `--wait-timeout` (default 10m, 0 means no timeout) and `--wait-interval` (default 2s, backing off to 30s). When `--wait` times out the command exits `8` and prints a recovery command; run it instead of re-running the job.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 6 | Partial failure |
| 7 | Rate limited (wait and retry) |
| 8 | Async job submitted but not finished (run the printed recovery command; do not resubmit) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `connectsecure-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add connectsecure-mcp \
     --env CONNECTSECURE_BASE_URL=https://pod106.myconnectsecure.com \
     --env CONNECTSECURE_TENANT=<portal-login-tenant> \
     --env CONNECTSECURE_CLIENT_ID=<api-client-id> \
     --env CONNECTSECURE_CLIENT_SECRET=<api-client-secret> \
     -- connectsecure-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which connectsecure-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   connectsecure-cli exposure --min-epss 0.5 --severity Critical,High --agent
   ```
4. If ambiguous, drill into subcommand help, for example `connectsecure-cli report-queries --help`.
