---
name: threecx
description: "Run every client's 3CX phone system from one CLI: trunk, version, licence and backup health across the whole fleet, plus the full Configuration API on any single PBX. Trigger phrases: `check all our 3CX phone systems`, `which client has a 3CX trunk down`, `3CX licence renewals this quarter`, `which PBXes are behind on 3CX updates`, `are 3CX backups running for every client`, `find broken 3CX call routing`, `what changed on our phone systems overnight`, `use threecx`, `run threecx`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "3CX"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - threecx-cli
    install:
      - kind: script
        bins: [threecx-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threecx/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threecx/install.ps1
---

# 3CX - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `threecx-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threecx/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threecx/install.ps1 | iex
   ```
3. Verify: `threecx-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `threecx-cli` and `threecx-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Register each client PBX once with its own service-principal credential, then 'fleet health', 'fleet trunks', 'fleet licences', 'fleet versions' and 'fleet backups' answer for every phone system at once, and 'fleet changes' shows what moved since the last run. Any of the generated XAPI commands targets one PBX with --pbx, and 'fleet exec' fans a read-only query across all of them.

## When to Use This CLI

Use this CLI when an agent or MSP technician needs the state of many 3CX V20 phone systems at once (trunks, versions, licence and maintenance expiry, backups, broken call routing) or needs to run a Configuration API call against a specific client's PBX. Start with 'fleet list --json': an empty list means no phone system is registered yet, so every fleet report will be empty until someone runs 'fleet add'.

## Anti-triggers

Do not use this CLI for:
- Placing, answering or transferring live calls (use the 3CX Call Control API).
- Buying or renewing 3CX licences (use the 3CX partner portal; this CLI only reports expiry).
- 3CX V18 or older systems, which have no Configuration API.
- Fleet-wide writes: 'fleet exec' is read-only; change one PBX at a time with --pbx and --dry-run.
- Changing which host or key a client PBX uses: run 'fleet add' yourself; it is not exposed to agents over MCP.

## Unique Capabilities

### Fleet health across every PBX
- **`fleet health`**  -  One row per client phone system with pass, warn, fail, unknown (not reported) or error (unreadable) for version, licence and maintenance expiry, last backup, trunks and extensions registered, disk, stopped services and capacity.

  _Reach for this first for any 'how are our phone systems' question; it answers for the whole fleet in one call._

  ```bash
  threecx-cli fleet health --agent
  ```
- **`fleet trunks`**  -  Every SIP trunk on every client phone system with state (online, offline, issue, unknown or error), certificate days left and configuration issues, problems first.

  _Use this to answer 'which clients have a dead SIP trunk right now' without logging in to each PBX._

  ```bash
  threecx-cli fleet trunks --offline --agent
  ```
- **`fleet licences`**  -  Licence and maintenance expiry, edition, seat headroom and reseller of record for every client phone system, soonest expiry first.

  _Use this for the renewals review and upsell list; it replaces a hand-copied spreadsheet._

  ```bash
  threecx-cli fleet licences --within 60d --csv
  ```
- **`fleet versions`**  -  Each phone system's 3CX build against the newest build in your fleet, with auto-update state, pending updates and expired maintenance that blocks updates.

  _Use this to pick patch-window targets and spot PBXes that cannot update._

  ```bash
  threecx-cli fleet versions --behind --agent
  ```
- **`fleet backups`**  -  Backup age, schedule, rotation and newest backup file for every client phone system, flagged never, stale, unscheduled, no files or shrunk.

  _Use this before any change window to prove every PBX has a recent restorable backup._

  ```bash
  threecx-cli fleet backups --stale 48h --agent
  ```

### Fleet-wide queries
- **`fleet exec`**  -  Run any read-only Configuration API query on every client phone system and get one merged table with a pbx column; credential fields are redacted and token-minting or download functions are refused.

  _Use this for cross-client questions like 'list users on every PBX' or 'which PBX owns this DID'._

  ```bash
  threecx-cli fleet exec /Users --param '$select=Number,FirstName,LastName' --agent
  ```

### Local history and integrity
- **`audit`**  -  Find ring groups, queues, IVRs and inbound rules that route to deleted, disabled or missing extensions, on one phone system or the whole fleet.

  _Run this after a moves/adds/changes ticket to catch broken call routing before the client does._

  ```bash
  threecx-cli audit --agent
  ```
- **`fleet changes`**  -  What changed across the fleet between two saved health or trunk runs: trunks offline or back, version bumped, licence renewed, backup gone stale, PBX unreachable.

  _Use this to see overnight regressions without reading every row of the health board._

  ```bash
  threecx-cli fleet changes --since 24h --agent
  ```

## Fleet operating notes

- **Check the registry first.** Run `threecx-cli fleet list --json`. An empty list means no phone system is registered, and every fleet report returns `[]` with exit 0: that is "nothing registered", not "all healthy". Ask the operator to run `fleet add`; `fleet add`, `fleet remove` are operator commands and are not exposed over MCP.
- **Missing keys.** Each entry's API key is read from `<env_prefix>_CLIENT_SECRET` (`secret_env` in `fleet list` names it); `secret_set: false` means that PBX will come back as an error row. The MCP server needs each `THREECX_<NAME>_CLIENT_SECRET` (and `THREECX_HOME`, if relocated) in its env block.
- **Error rows.** A PBX that cannot be read (unreachable, missing key, 401, 403 without the System Owner role, below the 8SC licence) is a row with an `error` field (`status: "error"` in `fleet health`, `state: "error"` in `fleet trunks`); the rest of the run continues with exit 0. When every PBX in scope fails, the command exits 5 after printing the rows.
- **Alerting exit code.** `fleet health --fail-on warn|fail` and `fleet trunks --fail-on-offline` exit 8 when they find a problem, after printing the full report.
- **Trunk states.** `online`, `offline`, `issue` (configuration issue), `unknown` (the PBX did not report registration), or `error`. `--offline` keeps everything that is not `online`.
- **Backup flags.** `NEVER`, `STALE` (older than `--stale`), `UNSCHEDULED`, `NO_FILES` (a backup is reported complete but no backup file is listed to restore from), `FILE_STALE` (the newest backup file you could restore is older than `--stale`), `SHRUNK` (newest file under half the previous one, usually recordings or voicemail dropped from the backup), `SCHEDULE_UNKNOWN` (neither the backup settings nor the status report whether a backup is scheduled), `SIZE_UNKNOWN` (the newest file's size is not reported, so it cannot be judged complete).
- **Unknown is not healthy.** A verdict is healthy only when every input it uses was reported; a fact the PBX did not report never reads as healthy and never drops a row. A `fleet health` check missing any of its inputs is `unknown` (its detail names them) (worse than `warn`, so `--fail-on warn` catches it; `fleet changes` reports a check going unknown as `<check>_unknown`, never as improved). `fleet versions` shows `behind` or `maintenance_expired` as `null`, names the missing facts in `unknown`, counts update entries of unreported state in `unknown_updates`, and `--behind` keeps the PBX. `fleet licences --within` drops a PBX only when its activation and both expiries were reported and it is active with both expiries outside the window; otherwise it is kept, with the unreported inputs named in `unknown`. `audit` reports a reference to a user whose enabled state was not reported as `unknown`.
- **History.** `fleet changes` compares saved runs of `fleet health` and `fleet trunks`; with fewer than two saved runs it returns `[]`. `--since` picks the last run at or before that age, or the oldest run if none is that old. Runs that queried different PBXes (`--only`, `--tag`, `--pbx`) are compared only for the PBXes both runs covered.
- **Narrowing.** `--only a,b` and `--tag gold` narrow a fleet run; an unknown name, or a tag no PBX carries, exits 2 and lists the registered names or tags in use.
- **One PBX.** `--pbx <name>` targets any command at one registered system; it cannot be combined with `--home` or `--config`. Check one system's setup with `threecx-cli doctor --pbx <name>`.
- **fleet exec.** GET only, under `/xapi/v1`. Credential-minting, token, remote-access and download functions (`Generate*`, `APIToken`, `GetOAuth`, `GetMicrosoftAccessToken`, `GetRequestHelpLink`, `Init*`, `Download*`, `Export*`, `MyTokens`, `SecurityTokens`) are refused. Fields whose names look like credentials or provisioning links are replaced with `[redacted]`, but that is name-based: pass `$select` with only the fields you need.

## Command Reference

**active-calls**  -  Manage active calls

- `threecx-cli active-calls`  -  Get entities from ActiveCalls

**active-calls-id**  -  Manage active calls id

- `threecx-cli active-calls-id <Id>`  -  Invoke action DropCall

**activity-log**  -  Manage activity log

- `threecx-cli activity-log get-filter`  -  Invoke function GetFilter
- `threecx-cli activity-log get-logs`  -  Invoke function GetLogs
- `threecx-cli activity-log purge-logs`  -  Invoke action PurgeLogs

**aisettings**  -  Manage aisettings

- `threecx-cli aisettings add-vector-store-files`  -  Invoke action AddVectorStoreFiles
- `threecx-cli aisettings batch-delete-files`  -  Invoke action BatchDeleteFiles
- `threecx-cli aisettings create-vector-store`  -  Invoke action CreateVectorStore
- `threecx-cli aisettings delete-open-ai-file`  -  Invoke action DeleteOpenAiFile
- `threecx-cli aisettings delete-vector-store`  -  Invoke action DeleteVectorStore
- `threecx-cli aisettings delete-vector-store-file`  -  Invoke action DeleteVectorStoreFile
- `threecx-cli aisettings get`  -  Get AISettings
- `threecx-cli aisettings get-airesources`  -  Invoke function GetAIResources
- `threecx-cli aisettings get-aitemplate-contents`  -  Invoke function GetAITemplateContents
- `threecx-cli aisettings get-current-airesources`  -  Invoke function GetCurrentAIResources
- `threecx-cli aisettings get-vector-store`  -  Invoke function GetVectorStore
- `threecx-cli aisettings get-vector-store-files`  -  Invoke function GetVectorStoreFiles
- `threecx-cli aisettings get-vector-stores`  -  Invoke function GetVectorStores
- `threecx-cli aisettings update`  -  Update AISettings
- `threecx-cli aisettings update-vector-store`  -  Invoke action UpdateVectorStore

**amazon-integration-settings**  -  Manage amazon integration settings

- `threecx-cli amazon-integration-settings generate-iam-policy-file`  -  Invoke function GenerateIamPolicyFile
- `threecx-cli amazon-integration-settings get`  -  Get AmazonIntegrationSettings
- `threecx-cli amazon-integration-settings update`  -  Update AmazonIntegrationSettings

**anti-hacking-settings**  -  Manage anti hacking settings

- `threecx-cli anti-hacking-settings get`  -  Get AntiHackingSettings
- `threecx-cli anti-hacking-settings update`  -  Update AntiHackingSettings

**backups**  -  Manage backups

- `threecx-cli backups backup`  -  Invoke action Backup
- `threecx-cli backups get-can-create`  -  Invoke function GetCanCreateBackup
- `threecx-cli backups get-failover-scripts`  -  Invoke function GetFailoverScripts
- `threecx-cli backups get-failover-settings`  -  Invoke function GetBackupFailoverSettings
- `threecx-cli backups get-local-size`  -  Invoke function GetLocalBackupSize
- `threecx-cli backups get-repository-settings`  -  Invoke function GetBackupRepositorySettings
- `threecx-cli backups get-restore-settings`  -  Invoke function GetRestoreSettings
- `threecx-cli backups get-settings`  -  Invoke function GetBackupSettings
- `threecx-cli backups get-sftp-host-key`  -  Invoke action GetSftpHostKey
- `threecx-cli backups list`  -  Get entities from Backups
- `threecx-cli backups purge-local`  -  Invoke action PurgeLocalBackups
- `threecx-cli backups set-failover-settings`  -  Invoke action SetBackupFailoverSettings
- `threecx-cli backups set-repository-settings`  -  Invoke action SetBackupRepositorySettings
- `threecx-cli backups set-restore-settings`  -  Invoke action SetRestoreSettings
- `threecx-cli backups set-settings`  -  Invoke action SetBackupSettings

**backups-file-name**  -  Manage backups file name

- `threecx-cli backups-file-name delete-backups`  -  Delete entity from Backups
- `threecx-cli backups-file-name get-backup-extras`  -  Invoke function GetBackupExtras
- `threecx-cli backups-file-name restore`  -  Invoke action Restore

**black-list-numbers**  -  Manage black list numbers

- `threecx-cli black-list-numbers bulk-numbers-delete`  -  Invoke action BulkNumbersDelete
- `threecx-cli black-list-numbers create`  -  Add new entity to BlackListNumbers
- `threecx-cli black-list-numbers list`  -  Get entities from BlackListNumbers

**black-list-numbers-id**  -  Manage black list numbers id

- `threecx-cli black-list-numbers-id delete-black-list-number`  -  Delete entity from BlackListNumbers
- `threecx-cli black-list-numbers-id get-black-list-number`  -  Get entity from BlackListNumbers by key
- `threecx-cli black-list-numbers-id update-black-list-number`  -  Update entity in BlackListNumbers

**blocklist**  -  Manage blocklist

- `threecx-cli blocklist bulk-ips-delete`  -  Invoke action BulkIpsDelete
- `threecx-cli blocklist create-addr`  -  Add new entity to Blocklist
- `threecx-cli blocklist list-addr`  -  Get entities from Blocklist

**blocklist-id**  -  Manage blocklist id

- `threecx-cli blocklist-id delete-blocklist-addr`  -  Delete entity from Blocklist
- `threecx-cli blocklist-id get-blocklist-addr`  -  Get entity from Blocklist by key
- `threecx-cli blocklist-id update-blocklist-addr`  -  Update entity in Blocklist

**call-cost-settings**  -  Manage call cost settings

- `threecx-cli call-cost-settings export-call-costs`  -  Invoke function ExportCallCosts
- `threecx-cli call-cost-settings list`  -  Get entities from CallCostSettings
- `threecx-cli call-cost-settings update-cost`  -  Invoke action UpdateCost

**call-flow-apps**  -  Manage call flow apps

- `threecx-cli call-flow-apps create`  -  Add new entity to CallFlowApps
- `threecx-cli call-flow-apps list`  -  Get entities from CallFlowApps

**call-flow-apps-id**  -  Manage call flow apps id

- `threecx-cli call-flow-apps-id delete-call-flow-app`  -  Delete entity from CallFlowApps
- `threecx-cli call-flow-apps-id delete-file`  -  Invoke action DeleteFile
- `threecx-cli call-flow-apps-id get-call-flow-app`  -  Get entity from CallFlowApps by key
- `threecx-cli call-flow-apps-id get-files`  -  Invoke function GetFiles
- `threecx-cli call-flow-apps-id update-call-flow-app`  -  Update entity in CallFlowApps

**call-flow-scripts**  -  Manage call flow scripts

- `threecx-cli call-flow-scripts`  -  Get entities from CallFlowScripts

**call-history-view**  -  Manage call history view

- `threecx-cli call-history-view download-call-history`  -  Invoke function DownloadCallHistory
- `threecx-cli call-history-view list`  -  Get entities from CallHistoryView

**call-parking-settings**  -  Manage call parking settings

- `threecx-cli call-parking-settings get`  -  Get CallParkingSettings
- `threecx-cli call-parking-settings update`  -  Update CallParkingSettings

**call-types-settings**  -  Manage call types settings

- `threecx-cli call-types-settings get`  -  Get CallTypesSettings
- `threecx-cli call-types-settings update`  -  Update CallTypesSettings

**cdrsettings**  -  Manage cdrsettings

- `threecx-cli cdrsettings get`  -  Get CDRSettings
- `threecx-cli cdrsettings update`  -  Update CDRSettings

**chat-history-view**  -  Manage chat history view

- `threecx-cli chat-history-view download-chat-history`  -  Invoke function DownloadChatHistory
- `threecx-cli chat-history-view list`  -  Get entities from ChatHistoryView

**chat-log-settings**  -  Manage chat log settings

- `threecx-cli chat-log-settings get`  -  Get ChatLogSettings
- `threecx-cli chat-log-settings update`  -  Update ChatLogSettings

**chat-messages-history-view**  -  Manage chat messages history view

- `threecx-cli chat-messages-history-view download-chat-messages-history`  -  Invoke function DownloadChatMessagesHistory
- `threecx-cli chat-messages-history-view list`  -  Get entities from ChatMessagesHistoryView

**codecs-settings**  -  Manage codecs settings

- `threecx-cli codecs-settings get`  -  Get CodecsSettings
- `threecx-cli codecs-settings update`  -  Update CodecsSettings

**conference-settings**  -  Manage conference settings

- `threecx-cli conference-settings delete-api-key`  -  Invoke action DeleteApiKey
- `threecx-cli conference-settings generate-api-key`  -  Invoke function GenerateApiKey
- `threecx-cli conference-settings get`  -  Get ConferenceSettings
- `threecx-cli conference-settings get-mcurequest-status`  -  Invoke function GetMCURequestStatus
- `threecx-cli conference-settings get-mcurow`  -  Invoke function GetMCURow
- `threecx-cli conference-settings get-mcurows`  -  Invoke function GetMCURows
- `threecx-cli conference-settings get-onboard-mcu-data`  -  Invoke function GetOnboardMcuData
- `threecx-cli conference-settings get-onboard-meetings`  -  Invoke function GetOnboardMeetings
- `threecx-cli conference-settings get-web-meeting-zones`  -  Invoke function GetWebMeetingZones
- `threecx-cli conference-settings update`  -  Update ConferenceSettings
- `threecx-cli conference-settings update-mcurequest-status`  -  Invoke action UpdateMCURequestStatus

**console-restrictions**  -  Manage console restrictions

- `threecx-cli console-restrictions get`  -  Get ConsoleRestrictions
- `threecx-cli console-restrictions update`  -  Update ConsoleRestrictions

**contacts**  -  Manage contacts

- `threecx-cli contacts create`  -  Add new entity to Contacts
- `threecx-cli contacts create-by-number`  -  Invoke action CreateContactByNumber
- `threecx-cli contacts delete-by-id`  -  Invoke action DeleteContactsById
- `threecx-cli contacts delete-by-type`  -  Invoke action DeleteContactsByType
- `threecx-cli contacts delete-department-by-type`  -  Invoke action DeleteDepartmentContactsByType
- `threecx-cli contacts delete-personal-by-type`  -  Invoke action DeletePersonalContactsByType
- `threecx-cli contacts get-by-number`  -  Invoke function GetContactsByNumber
- `threecx-cli contacts get-dir-search-settings`  -  Invoke function GetDirSearchSettings
- `threecx-cli contacts list`  -  Get entities from Contacts
- `threecx-cli contacts set-dir-search-settings`  -  Invoke action SetDirSearchSettings

**contacts-id**  -  Manage contacts id

- `threecx-cli contacts-id delete-contact`  -  Delete entity from Contacts
- `threecx-cli contacts-id get-contact`  -  Get entity from Contacts by key
- `threecx-cli contacts-id update-contact`  -  Update entity in Contacts

**countries**  -  Manage countries

- `threecx-cli countries`  -  Get entities from Countries

**country-codes**  -  Manage country codes

- `threecx-cli country-codes get`  -  Get CountryCodes
- `threecx-cli country-codes update`  -  Update CountryCodes

**crm-integration**  -  Manage crm integration

- `threecx-cli crm-integration delete-crm-contacts`  -  Invoke action DeleteCrmContacts
- `threecx-cli crm-integration get`  -  Get CrmIntegration
- `threecx-cli crm-integration get-crm-template-source`  -  Invoke function GetCrmTemplateSource
- `threecx-cli crm-integration get-oauth`  -  Invoke function GetOAuth
- `threecx-cli crm-integration set-oauth-state`  -  Invoke action SetOAuthState
- `threecx-cli crm-integration test`  -  Invoke action Test
- `threecx-cli crm-integration update`  -  Update CrmIntegration

**crm-templates**  -  Manage crm templates

- `threecx-cli crm-templates`  -  Invoke function GeCrmtTemplates

**crm-templates-name**  -  Manage crm templates name

- `threecx-cli crm-templates-name delete-crm-template`  -  Delete entity from CrmTemplates
- `threecx-cli crm-templates-name get-crm-template`  -  Get entity from CrmTemplates by key

**custom-prompts**  -  Manage custom prompts

- `threecx-cli custom-prompts list`  -  Get entities from CustomPrompts
- `threecx-cli custom-prompts make-call-record`  -  Invoke action MakeCallRecordCustomPrompt

**custom-prompts-filename**  -  Manage custom prompts filename

- `threecx-cli custom-prompts-filename <Filename>`  -  Delete entity from CustomPrompts

**data-connector-settings**  -  Manage data connector settings

- `threecx-cli data-connector-settings get`  -  Get DataConnectorSettings
- `threecx-cli data-connector-settings synchronize`  -  Invoke action Synchronize
- `threecx-cli data-connector-settings test-data-connector`  -  Invoke action TestDataConnector
- `threecx-cli data-connector-settings update`  -  Update DataConnectorSettings

**defs**  -  Manage defs

- `threecx-cli defs get`  -  Get Defs
- `threecx-cli defs get-license-parameters`  -  Invoke function GetLicenseParameters
- `threecx-cli defs get-routes`  -  Invoke action GetRoutes
- `threecx-cli defs get-system-parameters`  -  Invoke function GetSystemParameters
- `threecx-cli defs has-system-owner`  -  Invoke function HasSystemOwner
- `threecx-cli defs list-codecs`  -  Get Codecs from Defs
- `threecx-cli defs list-gateway-parameter-values`  -  Get GatewayParameterValues from Defs
- `threecx-cli defs list-gateway-parameters`  -  Get GatewayParameters from Defs
- `threecx-cli defs list-time-zones`  -  Get TimeZones from Defs
- `threecx-cli defs report-log-event`  -  Invoke action ReportLogEvent
- `threecx-cli defs send-email`  -  Invoke action SendEmail

**device-infos**  -  Manage device infos

- `threecx-cli device-infos`  -  Get entities from DeviceInfos

**device-infos-id**  -  Manage device infos id

- `threecx-cli device-infos-id delete-device-info`  -  Delete entity from DeviceInfos
- `threecx-cli device-infos-id get-device-info`  -  Get entity from DeviceInfos by key
- `threecx-cli device-infos-id provision`  -  Invoke action Provision

**dial-code-settings**  -  Manage dial code settings

- `threecx-cli dial-code-settings get`  -  Get DialCodeSettings
- `threecx-cli dial-code-settings update`  -  Update DialCodeSettings

**did-numbers**  -  Manage did numbers

- `threecx-cli did-numbers`  -  Get entities from DidNumbers

**dnproperties**  -  Manage dnproperties

- `threecx-cli dnproperties create-dnproperty`  -  Invoke action CreateDNProperty
- `threecx-cli dnproperties delete-dnproperty`  -  Invoke action DeleteDNProperty
- `threecx-cli dnproperties get-dnproperty-by-name`  -  Invoke function GetDNPropertyByName
- `threecx-cli dnproperties get-properties-by-dn`  -  Invoke function GetPropertiesByDn
- `threecx-cli dnproperties update-dnproperty`  -  Invoke action UpdateDNProperty

**e164-settings**  -  Manage e164 settings

- `threecx-cli e164-settings get`  -  Get E164Settings
- `threecx-cli e164-settings update`  -  Update E164Settings

**email-template**  -  Manage email template

- `threecx-cli email-template`  -  Get entities from EmailTemplate

**email-template-template-path**  -  Manage email template template path

- `threecx-cli email-template-template-path get-email-template`  -  Get entity from EmailTemplate by key
- `threecx-cli email-template-template-path set-default`  -  Invoke action SetDefault
- `threecx-cli email-template-template-path update-email-template`  -  Update entity in EmailTemplate

**emergency-geo-locations**  -  Manage emergency geo locations

- `threecx-cli emergency-geo-locations list`  -  Get entities from EmergencyGeoLocations
- `threecx-cli emergency-geo-locations update`  -  Invoke action Update

**emergency-notifications-settings**  -  Manage emergency notifications settings

- `threecx-cli emergency-notifications-settings get`  -  Get EmergencyNotificationsSettings
- `threecx-cli emergency-notifications-settings update`  -  Update EmergencyNotificationsSettings

**event-logs**  -  Manage event logs

- `threecx-cli event-logs download`  -  Invoke function DownloadEventLogs
- `threecx-cli event-logs list`  -  Get entities from EventLogs
- `threecx-cli event-logs purge`  -  Invoke action PurgeEventLog

**fax**  -  Manage fax

- `threecx-cli fax bulk-delete`  -  Invoke action BulkFaxDelete
- `threecx-cli fax create`  -  Add new entity to Fax
- `threecx-cli fax init`  -  Invoke function InitFax
- `threecx-cli fax list`  -  Get entities from Fax

**fax-id**  -  Manage fax id

- `threecx-cli fax-id delete-fax`  -  Delete entity from Fax
- `threecx-cli fax-id get-fax`  -  Get entity from Fax by key
- `threecx-cli fax-id update-fax`  -  Update entity in Fax

**fax-number-number**  -  Manage fax number number

- `threecx-cli fax-number-number <Number>`  -  Get entity from Fax by key (Number)

**fax-server-settings**  -  Manage fax server settings

- `threecx-cli fax-server-settings clean-up-fax`  -  Invoke action CleanUpFax
- `threecx-cli fax-server-settings get`  -  Get FaxServerSettings
- `threecx-cli fax-server-settings get-fax-files-size`  -  Invoke function GetFaxFilesSize
- `threecx-cli fax-server-settings update`  -  Update FaxServerSettings

**firewall**  -  Manage firewall

- `threecx-cli firewall get-last-result`  -  Invoke function GetLastResult
- `threecx-cli firewall get-state`  -  Get Firewall
- `threecx-cli firewall start-check`  -  Invoke action StartCheck
- `threecx-cli firewall stop-check`  -  Invoke action StopCheck

**firmwares**  -  Manage firmwares

- `threecx-cli firmwares clean-up`  -  Invoke action CleanUp
- `threecx-cli firmwares get-state`  -  Invoke function GetFirmwareState
- `threecx-cli firmwares list`  -  Get entities from Firmwares

**firmwares-id**  -  Manage firmwares id

- `threecx-cli firmwares-id delete-firmware`  -  Delete entity from Firmwares
- `threecx-cli firmwares-id push-firmware-for-phones`  -  Invoke action PushFirmwareForPhones
- `threecx-cli firmwares-id set-as-default`  -  Invoke action SetAsDefault

**fxs**  -  Manage fxs

- `threecx-cli fxs create`  -  Add new entity to Fxs
- `threecx-cli fxs list`  -  Get entities from Fxs

**fxs-mac-address**  -  Manage fxs mac address

- `threecx-cli fxs-mac-address delete-fxs`  -  Delete entity from Fxs
- `threecx-cli fxs-mac-address get-fxs`  -  Get entity from Fxs by key
- `threecx-cli fxs-mac-address regenerate-web-credentials`  -  Invoke action RegenerateWebCredentials
- `threecx-cli fxs-mac-address update-fxs`  -  Update entity in Fxs

**fxs-templates**  -  Manage fxs templates

- `threecx-cli fxs-templates create`  -  Add new entity to FxsTemplates
- `threecx-cli fxs-templates list`  -  Get entities from FxsTemplates

**fxs-templates-id**  -  Manage fxs templates id

- `threecx-cli fxs-templates-id delete-fxs-template`  -  Delete entity from FxsTemplates
- `threecx-cli fxs-templates-id get-fxs-template`  -  Get entity from FxsTemplates by key
- `threecx-cli fxs-templates-id update-fxs-template`  -  Update entity in FxsTemplates

**general-settings-for-apps**  -  Manage general settings for apps

- `threecx-cli general-settings-for-apps get`  -  Get GeneralSettingsForApps
- `threecx-cli general-settings-for-apps update`  -  Update GeneralSettingsForApps

**general-settings-for-pbx**  -  Manage general settings for pbx

- `threecx-cli general-settings-for-pbx get`  -  Get GeneralSettingsForPbx
- `threecx-cli general-settings-for-pbx update`  -  Update GeneralSettingsForPbx

**get-directory-info**  -  Manage get directory info

- `threecx-cli get-directory-info`  -  Invoke actionImport GetDirectoryInfo

**get-prompt-set-updates**  -  Manage get prompt set updates

- `threecx-cli get-prompt-set-updates`  -  Invoke functionImport GetPromptSetUpdates

**get-server-crm-updates**  -  Manage get server crm updates

- `threecx-cli get-server-crm-updates`  -  Invoke functionImport GetServerCrmUpdates

**get-update-details-key-key-id-id**  -  Manage get update details key key id id

- `threecx-cli get-update-details-key-key-id-id <key> <id>`  -  Invoke functionImport GetUpdateDetails

**get-update-settings**  -  Manage get update settings

- `threecx-cli get-update-settings`  -  Invoke functionImport GetUpdateSettings

**get-updates**  -  Manage get updates

- `threecx-cli get-updates`  -  Invoke functionImport GetUpdates

**get-updates-stats**  -  Manage get updates stats

- `threecx-cli get-updates-stats`  -  Invoke functionImport GetUpdatesStats

**google-settings**  -  Manage google settings

- `threecx-cli google-settings get`  -  Get GoogleSettings
- `threecx-cli google-settings get-google-users`  -  Invoke action GetGoogleUsers
- `threecx-cli google-settings update`  -  Update GoogleSettings

**groups**  -  Manage groups

- `threecx-cli groups create`  -  Add new entity to Groups
- `threecx-cli groups delete-company-by-id`  -  Invoke action DeleteCompanyById
- `threecx-cli groups delete-company-by-number`  -  Invoke action DeleteCompanyByNumber
- `threecx-cli groups link-partner`  -  Invoke action LinkGroupPartner
- `threecx-cli groups list`  -  Get entities from Groups
- `threecx-cli groups replace-license-key`  -  Invoke action ReplaceGroupLicenseKey
- `threecx-cli groups unlink-partner`  -  Invoke action UnlinkGroupPartner

**groups-id**  -  Manage groups id

- `threecx-cli groups-id delete-group`  -  Delete entity from Groups
- `threecx-cli groups-id get-group`  -  Get entity from Groups by key
- `threecx-cli groups-id get-restrictions`  -  Invoke function GetRestrictions
- `threecx-cli groups-id list-members`  -  Get Members from Groups
- `threecx-cli groups-id list-rights`  -  Get Rights from Groups
- `threecx-cli groups-id update-group`  -  Update entity in Groups

**holidays**  -  Manage holidays

- `threecx-cli holidays create`  -  Add new entity to Holidays
- `threecx-cli holidays list`  -  Get entities from Holidays

**holidays-id**  -  Manage holidays id

- `threecx-cli holidays-id delete-holiday`  -  Delete entity from Holidays
- `threecx-cli holidays-id get-holiday`  -  Get entity from Holidays by key
- `threecx-cli holidays-id update-holiday`  -  Update entity in Holidays

**hotel-rooms-id**  -  Manage hotel rooms id

- `threecx-cli hotel-rooms-id assign`  -  Invoke action Assign
- `threecx-cli hotel-rooms-id clear`  -  Invoke action Clear

**hotel-services**  -  Manage hotel services

- `threecx-cli hotel-services get`  -  Get HotelServices
- `threecx-cli hotel-services update`  -  Update HotelServices

**inbound-rules**  -  Manage inbound rules

- `threecx-cli inbound-rules bulk-delete`  -  Invoke action BulkInboundRulesDelete
- `threecx-cli inbound-rules create`  -  Add new entity to InboundRules
- `threecx-cli inbound-rules export-caller-id-rules`  -  Invoke function ExportCallerIdRules
- `threecx-cli inbound-rules list`  -  Get entities from InboundRules

**inbound-rules-id**  -  Manage inbound rules id

- `threecx-cli inbound-rules-id delete-inbound-rule`  -  Delete entity from InboundRules
- `threecx-cli inbound-rules-id get-inbound-rule`  -  Get entity from InboundRules by key
- `threecx-cli inbound-rules-id update-inbound-rule`  -  Update entity in InboundRules

**install-updates**  -  Manage install updates

- `threecx-cli install-updates`  -  Invoke actionImport InstallUpdates

**last-cdr-and-chat-message-timestamp**  -  Manage last cdr and chat message timestamp

- `threecx-cli last-cdr-and-chat-message-timestamp`  -  Get entities from LastCdrAndChatMessageTimestamp

**license-status**  -  Manage license status

- `threecx-cli license-status edit-license-info`  -  Invoke action EditLicenseInfo
- `threecx-cli license-status get`  -  Get LicenseStatus
- `threecx-cli license-status link-partner`  -  Invoke action LinkPartner
- `threecx-cli license-status partner-info`  -  Invoke function PartnerInfo
- `threecx-cli license-status refresh`  -  Invoke action RefreshLicenseStatus
- `threecx-cli license-status replace-license-key`  -  Invoke action ReplaceLicenseKey
- `threecx-cli license-status unlink-partner`  -  Invoke action UnlinkPartner

**logging-settings**  -  Manage logging settings

- `threecx-cli logging-settings get`  -  Get LoggingSettings
- `threecx-cli logging-settings update`  -  Update LoggingSettings

**mail-settings**  -  Manage mail settings

- `threecx-cli mail-settings get`  -  Get MailSettings
- `threecx-cli mail-settings test-email`  -  Invoke action TestEmail
- `threecx-cli mail-settings update`  -  Update MailSettings

**mcp-servers**  -  Manage mcp servers

- `threecx-cli mcp-servers exchange`  -  Invoke action Exchange
- `threecx-cli mcp-servers get-server-metadata`  -  Invoke function GetServerMetadata
- `threecx-cli mcp-servers register`  -  Invoke action Register

**microsoft365-integration**  -  Manage microsoft365 integration

- `threecx-cli microsoft365-integration authorize-presence`  -  Invoke action AuthorizePresence
- `threecx-cli microsoft365-integration deauthorize-presence`  -  Invoke action DeauthorizePresence
- `threecx-cli microsoft365-integration get`  -  Get Microsoft365Integration
- `threecx-cli microsoft365-integration get-m365-groups`  -  Invoke action GetM365Groups
- `threecx-cli microsoft365-integration get-m365-users`  -  Invoke action GetM365Users
- `threecx-cli microsoft365-integration get-microsoft-access-token`  -  Invoke function GetMicrosoftAccessToken
- `threecx-cli microsoft365-integration get-microsoft365-directory`  -  Invoke function GetMicrosoft365Directory
- `threecx-cli microsoft365-integration get-users-by-principal-names`  -  Invoke action GetUsersByPrincipalNames
- `threecx-cli microsoft365-integration test-presence`  -  Invoke action TestPresence
- `threecx-cli microsoft365-integration test-subscription`  -  Invoke function TestSubscription
- `threecx-cli microsoft365-integration update`  -  Update Microsoft365Integration

**microsoft365-teams-integration**  -  Manage microsoft365 teams integration

- `threecx-cli microsoft365-teams-integration check-fqdn-record`  -  Invoke function CheckFqdnRecord
- `threecx-cli microsoft365-teams-integration check-map-users-script`  -  Invoke function CheckMapUsersScript
- `threecx-cli microsoft365-teams-integration get`  -  Get Microsoft365TeamsIntegration
- `threecx-cli microsoft365-teams-integration get-dial-plan-script`  -  Invoke function GetDialPlanScript
- `threecx-cli microsoft365-teams-integration get-map-users-script`  -  Invoke function GetMapUsersScript
- `threecx-cli microsoft365-teams-integration update`  -  Update Microsoft365TeamsIntegration

**music-on-hold-settings**  -  Manage music on hold settings

- `threecx-cli music-on-hold-settings get`  -  Get MusicOnHoldSettings
- `threecx-cli music-on-hold-settings update`  -  Update MusicOnHoldSettings

**my-group**  -  Manage my group

- `threecx-cli my-group get`  -  Get MyGroup
- `threecx-cli my-group get-partner-info`  -  Invoke function GetMyGroupPartnerInfo
- `threecx-cli my-group get-restrictions`  -  Invoke function GetRestrictions
- `threecx-cli my-group link-partner`  -  Invoke action LinkMyGroupPartner
- `threecx-cli my-group list-members`  -  Get Members from MyGroup
- `threecx-cli my-group list-rights`  -  Get Rights from MyGroup
- `threecx-cli my-group replace-license-key`  -  Invoke action ReplaceMyGroupLicenseKey
- `threecx-cli my-group unlink-partner`  -  Invoke action UnlinkMyGroupPartner
- `threecx-cli my-group update`  -  Update MyGroup

**my-tokens**  -  Manage my tokens

- `threecx-cli my-tokens`  -  Get entities from MyTokens

**my-tokens-id**  -  Manage my tokens id

- `threecx-cli my-tokens-id <Id>`  -  Invoke action RevokeToken

**my-user**  -  Manage my user

- `threecx-cli my-user generate-prov-link`  -  Invoke function GenerateProvLink
- `threecx-cli my-user get`  -  Get MyUser
- `threecx-cli my-user issue-download-ticket`  -  Invoke action IssueDownloadTicket
- `threecx-cli my-user list-forwarding-profiles`  -  Get ForwardingProfiles from MyUser
- `threecx-cli my-user list-greetings`  -  Get Greetings from MyUser
- `threecx-cli my-user list-groups`  -  Get Groups from MyUser
- `threecx-cli my-user update`  -  Update MyUser

**network-interfaces**  -  Manage network interfaces

- `threecx-cli network-interfaces`  -  Get entities from NetworkInterfaces

**network-settings**  -  Manage network settings

- `threecx-cli network-settings get`  -  Get NetworkSettings
- `threecx-cli network-settings get-ifaces`  -  Invoke function GetIfaces
- `threecx-cli network-settings update`  -  Update NetworkSettings

**notification-settings**  -  Manage notification settings

- `threecx-cli notification-settings get`  -  Get NotificationSettings
- `threecx-cli notification-settings update`  -  Update NotificationSettings

**oauth-clients**  -  Manage oauth clients

- `threecx-cli oauth-clients`  -  Get entities from OAuthClients

**oauth-clients-id**  -  Manage oauth clients id

- `threecx-cli oauth-clients-id delete-oauth-client`  -  Delete entity from OAuthClients
- `threecx-cli oauth-clients-id get-oauth-client`  -  Get entity from OAuthClients by key
- `threecx-cli oauth-clients-id refresh-client-metadata`  -  Invoke action RefreshClientMetadata

**office-hours**  -  Manage office hours

- `threecx-cli office-hours get`  -  Get OfficeHours
- `threecx-cli office-hours update`  -  Update OfficeHours

**outbound-rules**  -  Manage outbound rules

- `threecx-cli outbound-rules create`  -  Add new entity to OutboundRules
- `threecx-cli outbound-rules get-emergency`  -  Invoke function GetEmergencyOutboundRules
- `threecx-cli outbound-rules list`  -  Get entities from OutboundRules
- `threecx-cli outbound-rules move-up-down`  -  Invoke action MoveUpDown
- `threecx-cli outbound-rules purge`  -  Invoke action Purge

**outbound-rules-id**  -  Manage outbound rules id

- `threecx-cli outbound-rules-id delete-outbound-rule`  -  Delete entity from OutboundRules
- `threecx-cli outbound-rules-id get-outbound-rule`  -  Get entity from OutboundRules by key
- `threecx-cli outbound-rules-id update-outbound-rule`  -  Update entity in OutboundRules

**parameters**  -  Manage parameters

- `threecx-cli parameters create`  -  Add new entity to Parameters
- `threecx-cli parameters get-by-name`  -  Invoke function GetParameterByName
- `threecx-cli parameters list`  -  Get entities from Parameters

**parameters-id**  -  Manage parameters id

- `threecx-cli parameters-id delete-parameter`  -  Delete entity from Parameters
- `threecx-cli parameters-id get-parameter`  -  Get entity from Parameters by key
- `threecx-cli parameters-id update-parameter`  -  Update entity in Parameters

**parkings**  -  Manage parkings

- `threecx-cli parkings create`  -  Add new entity to Parkings
- `threecx-cli parkings list`  -  Get entities from Parkings

**parkings-id**  -  Manage parkings id

- `threecx-cli parkings-id delete-parking`  -  Delete entity from Parkings
- `threecx-cli parkings-id get-parking`  -  Get entity from Parkings by key
- `threecx-cli parkings-id list-parking-groups`  -  Get Groups from Parkings
- `threecx-cli parkings-id update-parking`  -  Update entity in Parkings

**parkings-number-number**  -  Manage parkings number number

- `threecx-cli parkings-number-number <Number>`  -  Get entity from Parkings by key (Number)

**peers**  -  Manage peers

- `threecx-cli peers get-report`  -  Invoke function GetReportPeers
- `threecx-cli peers list`  -  Get entities from Peers
- `threecx-cli peers retreive-by-numbers`  -  Invoke action RetreivePeersByNumbers

**peers-number-number**  -  Manage peers number number

- `threecx-cli peers-number-number <Number>`  -  Get entity from Peers by key (Number)

**phone-book-settings**  -  Manage phone book settings

- `threecx-cli phone-book-settings get`  -  Get PhoneBookSettings
- `threecx-cli phone-book-settings update`  -  Update PhoneBookSettings

**phone-logos**  -  Manage phone logos

- `threecx-cli phone-logos`  -  Get entities from PhoneLogos

**phone-logos-filename**  -  Manage phone logos filename

- `threecx-cli phone-logos-filename <Filename>`  -  Delete entity from PhoneLogos

**phone-templates**  -  Manage phone templates

- `threecx-cli phone-templates create`  -  Add new entity to PhoneTemplates
- `threecx-cli phone-templates list`  -  Get entities from PhoneTemplates

**phone-templates-id**  -  Manage phone templates id

- `threecx-cli phone-templates-id delete-phone-template`  -  Delete entity from PhoneTemplates
- `threecx-cli phone-templates-id get-phone-template`  -  Get entity from PhoneTemplates by key
- `threecx-cli phone-templates-id update-phone-template`  -  Update entity in PhoneTemplates

**phones-settings**  -  Manage phones settings

- `threecx-cli phones-settings get`  -  Get PhonesSettings
- `threecx-cli phones-settings update`  -  Update PhonesSettings

**playlists**  -  Manage playlists

- `threecx-cli playlists create`  -  Add new entity to Playlists
- `threecx-cli playlists delete-file`  -  Invoke action DeletePlaylistFile
- `threecx-cli playlists download-file`  -  Invoke function DownloadPlaylistFile
- `threecx-cli playlists list`  -  Get entities from Playlists

**playlists-name**  -  Manage playlists name

- `threecx-cli playlists-name delete-playlist`  -  Delete entity from Playlists
- `threecx-cli playlists-name get-playlist`  -  Get entity from Playlists by key
- `threecx-cli playlists-name update-playlist`  -  Update entity in Playlists

**prompt-sets**  -  Manage prompt sets

- `threecx-cli prompt-sets get-active`  -  Invoke function GetActive
- `threecx-cli prompt-sets list`  -  Get entities from PromptSets

**prompt-sets-id**  -  Manage prompt sets id

- `threecx-cli prompt-sets-id copy`  -  Invoke action Copy
- `threecx-cli prompt-sets-id delete-prompt-set`  -  Delete entity from PromptSets
- `threecx-cli prompt-sets-id get-prompt-set`  -  Get entity from PromptSets by key
- `threecx-cli prompt-sets-id list-prompts`  -  Get Prompts from PromptSets
- `threecx-cli prompt-sets-id play-prompt`  -  Invoke action PlayPrompt
- `threecx-cli prompt-sets-id record-prompt`  -  Invoke action RecordPrompt
- `threecx-cli prompt-sets-id set-active`  -  Invoke action SetActive
- `threecx-cli prompt-sets-id set-alternate-pronunciation`  -  Invoke action SetAlternatePronunciation
- `threecx-cli prompt-sets-id update-prompt-set`  -  Update entity in PromptSets

**purge-all-logs**  -  Manage purge all logs

- `threecx-cli purge-all-logs`  -  Invoke actionImport PurgeAllLogs

**purge-calls**  -  Manage purge calls

- `threecx-cli purge-calls`  -  Invoke actionImport PurgeCalls

**purge-chats**  -  Manage purge chats

- `threecx-cli purge-chats`  -  Invoke actionImport PurgeChats

**queues**  -  Manage queues

- `threecx-cli queues create`  -  Add new entity to Queues
- `threecx-cli queues get-first-available-number`  -  Invoke function GetFirstAvailableQueueNumber
- `threecx-cli queues list`  -  Get entities from Queues

**queues-id**  -  Manage queues id

- `threecx-cli queues-id delete-queue`  -  Delete entity from Queues
- `threecx-cli queues-id get-queue`  -  Get entity from Queues by key
- `threecx-cli queues-id list-agents`  -  Get Agents from Queues
- `threecx-cli queues-id list-managers`  -  Get Managers from Queues
- `threecx-cli queues-id reset-queue-statistics`  -  Invoke action ResetQueueStatistics
- `threecx-cli queues-id update-queue`  -  Update entity in Queues

**queues-number-number**  -  Manage queues number number

- `threecx-cli queues-number-number <Number>`  -  Get entity from Queues by key (Number)

**receptionists**  -  Manage receptionists

- `threecx-cli receptionists create`  -  Add new entity to Receptionists
- `threecx-cli receptionists get-first-available-number`  -  Invoke function GetFirstAvailableReceptionistNumber
- `threecx-cli receptionists list`  -  Get entities from Receptionists

**receptionists-id**  -  Manage receptionists id

- `threecx-cli receptionists-id delete-receptionist`  -  Delete entity from Receptionists
- `threecx-cli receptionists-id get-receptionist`  -  Get entity from Receptionists by key
- `threecx-cli receptionists-id list-forwards`  -  Get Forwards from Receptionists
- `threecx-cli receptionists-id update-receptionist`  -  Update entity in Receptionists

**receptionists-number-number**  -  Manage receptionists number number

- `threecx-cli receptionists-number-number <Number>`  -  Get entity from Receptionists by key (Number)

**recordings**  -  Manage recordings

- `threecx-cli recordings archive`  -  Invoke action Archive
- `threecx-cli recordings bulk-archive`  -  Invoke action BulkRecordingsArchive
- `threecx-cli recordings bulk-delete`  -  Invoke action BulkRecordingsDelete
- `threecx-cli recordings download`  -  Invoke function DownloadRecording
- `threecx-cli recordings get-repository-settings`  -  Invoke function GetRecordingRepositorySettings
- `threecx-cli recordings list`  -  Get entities from Recordings
- `threecx-cli recordings purge-archive`  -  Invoke action PurgeArchive
- `threecx-cli recordings purge-local`  -  Invoke action PurgeLocal
- `threecx-cli recordings set-repository-settings`  -  Invoke action SetRecordingRepositorySettings
- `threecx-cli recordings transcribe`  -  Invoke action TranscribeRecordings

**remote-archiving-settings**  -  Manage remote archiving settings

- `threecx-cli remote-archiving-settings archive-chats`  -  Invoke action ArchiveChats
- `threecx-cli remote-archiving-settings archive-faxes`  -  Invoke action ArchiveFaxes
- `threecx-cli remote-archiving-settings archive-recordings`  -  Invoke action ArchiveRecordings
- `threecx-cli remote-archiving-settings archive-voicemail`  -  Invoke action ArchiveVoicemail
- `threecx-cli remote-archiving-settings get`  -  Get RemoteArchivingSettings
- `threecx-cli remote-archiving-settings update`  -  Update RemoteArchivingSettings

**report-abandoned-chats-statistics**  -  Manage report abandoned chats statistics

- `threecx-cli report-abandoned-chats-statistics download-abandoned-chats-statistics`  -  Invoke function DownloadAbandonedChatsStatistics
- `threecx-cli report-abandoned-chats-statistics get-abandoned-chats-statistics-data`  -  Invoke function GetAbandonedChatsStatisticsData

**report-abandoned-queue-calls**  -  Manage report abandoned queue calls

- `threecx-cli report-abandoned-queue-calls download-abandoned-queue-calls`  -  Invoke function DownloadAbandonedQueueCalls
- `threecx-cli report-abandoned-queue-calls get-abandoned-queue-calls-data`  -  Invoke function GetAbandonedQueueCallsData

**report-agent-login-history**  -  Manage report agent login history

- `threecx-cli report-agent-login-history download-agent-login-history`  -  Invoke function DownloadAgentLoginHistory
- `threecx-cli report-agent-login-history get-agent-login-history-data`  -  Invoke function GetAgentLoginHistoryData

**report-agents-in-queue-statistics**  -  Manage report agents in queue statistics

- `threecx-cli report-agents-in-queue-statistics download-agents-in-queue-statistics`  -  Invoke function DownloadAgentsInQueueStatistics
- `threecx-cli report-agents-in-queue-statistics get-agents-in-queue-statistics-data`  -  Invoke function GetAgentsInQueueStatisticsData

**report-audit-log**  -  Manage report audit log

- `threecx-cli report-audit-log download-audit-log`  -  Invoke function DownloadAuditLog
- `threecx-cli report-audit-log get-audit-log-data`  -  Invoke function GetAuditLogData

**report-average-queue-waiting-time**  -  Manage report average queue waiting time

- `threecx-cli report-average-queue-waiting-time download-average-queue-waiting-time-report`  -  Invoke function DownloadAverageQueueWaitingTimeReport
- `threecx-cli report-average-queue-waiting-time get-average-queue-waiting-time-data`  -  Invoke function GetAverageQueueWaitingTimeData

**report-breaches-sla**  -  Manage report breaches sla

- `threecx-cli report-breaches-sla download-breaches-sla`  -  Invoke function DownloadBreachesSla
- `threecx-cli report-breaches-sla get-breaches-sla-data`  -  Invoke function GetBreachesSlaData

**report-call-cost-by-extension-group**  -  Manage report call cost by extension group

- `threecx-cli report-call-cost-by-extension-group download-call-cost-by-extension-group`  -  Invoke function DownloadCallCostByExtensionGroup
- `threecx-cli report-call-cost-by-extension-group get-call-cost-by-extension-group-data`  -  Invoke function GetCallCostByExtensionGroupData

**report-call-distribution**  -  Manage report call distribution

- `threecx-cli report-call-distribution download-get-get-call-distribution`  -  Invoke function DownloadGetGetCallDistribution
- `threecx-cli report-call-distribution get-call-distribution`  -  Invoke function GetCallDistribution

**report-call-log-data**  -  Manage report call log data

- `threecx-cli report-call-log-data download-call-log`  -  Invoke function DownloadCallLog
- `threecx-cli report-call-log-data get-call-log-data`  -  Invoke function GetCallLogData
- `threecx-cli report-call-log-data get-call-quality-report`  -  Invoke function GetCallQualityReport
- `threecx-cli report-call-log-data get-old-call-log-data`  -  Invoke function GetOldCallLogData
- `threecx-cli report-call-log-data get-old-call-quality-report`  -  Invoke function GetOldCallQualityReport

**report-chat-log**  -  Manage report chat log

- `threecx-cli report-chat-log download-chat-log`  -  Invoke function DownloadChatLog
- `threecx-cli report-chat-log get-chat-log`  -  Invoke function GetChatLog

**report-detailed-queue-statistics**  -  Manage report detailed queue statistics

- `threecx-cli report-detailed-queue-statistics download-detailed-queue-statistics`  -  Invoke function DownloadDetailedQueueStatistics
- `threecx-cli report-detailed-queue-statistics get-detailed-queue-statistics-data`  -  Invoke function GetDetailedQueueStatisticsData

**report-extension-statistics**  -  Manage report extension statistics

- `threecx-cli report-extension-statistics download-extension-statistics`  -  Invoke function DownloadExtensionStatistics
- `threecx-cli report-extension-statistics get-extension-statistics-data`  -  Invoke function GetExtensionStatisticsData

**report-extension-statistics-by-group**  -  Manage report extension statistics by group

- `threecx-cli report-extension-statistics-by-group download-extension-statistics-by-group`  -  Invoke function DownloadExtensionStatisticsByGroup
- `threecx-cli report-extension-statistics-by-group get-extension-statistics-by-group-data`  -  Invoke function GetExtensionStatisticsByGroupData

**report-extensions-statistics-by-ring-groups**  -  Manage report extensions statistics by ring groups

- `threecx-cli report-extensions-statistics-by-ring-groups download-extensions-statistics-by-ring-groups`  -  Invoke function DownloadExtensionsStatisticsByRingGroups
- `threecx-cli report-extensions-statistics-by-ring-groups get-extensions-statistics-by-ring-groups-data`  -  Invoke function GetExtensionsStatisticsByRingGroupsData

**report-inbound-calls**  -  Manage report inbound calls

- `threecx-cli report-inbound-calls download-get-inbound-calls`  -  Invoke function DownloadGetInboundCalls
- `threecx-cli report-inbound-calls get-inbound-calls`  -  Invoke function GetInboundCalls

**report-inbound-rules**  -  Manage report inbound rules

- `threecx-cli report-inbound-rules download-inbound-rules`  -  Invoke function DownloadInboundRules
- `threecx-cli report-inbound-rules get-inbound-rules-data`  -  Invoke function GetInboundRulesData

**report-outbound-calls**  -  Manage report outbound calls

- `threecx-cli report-outbound-calls download-get-outbound-calls`  -  Invoke function DownloadGetOutboundCalls
- `threecx-cli report-outbound-calls get-outbound-calls`  -  Invoke function GetOutboundCalls

**report-queue-agents-chat-statistics**  -  Manage report queue agents chat statistics

- `threecx-cli report-queue-agents-chat-statistics download-queue-agents-chat-statistics`  -  Invoke function DownloadQueueAgentsChatStatistics
- `threecx-cli report-queue-agents-chat-statistics get-queue-agents-chat-statistics-data`  -  Invoke function GetQueueAgentsChatStatisticsData

**report-queue-agents-chat-statistics-totals**  -  Manage report queue agents chat statistics totals

- `threecx-cli report-queue-agents-chat-statistics-totals download-queue-agents-chat-statistics-totals`  -  Invoke function DownloadQueueAgentsChatStatisticsTotals
- `threecx-cli report-queue-agents-chat-statistics-totals get-queue-agents-chat-statistics-totals-data`  -  Invoke function GetQueueAgentsChatStatisticsTotalsData

**report-queue-an-un-calls**  -  Manage report queue an un calls

- `threecx-cli report-queue-an-un-calls download-queue-an-un-calls-report`  -  Invoke function DownloadQueueAnUnCallsReport
- `threecx-cli report-queue-an-un-calls get-queue-an-un-calls-data`  -  Invoke function GetQueueAnUnCallsData

**report-queue-answered-calls-by-wait-time**  -  Manage report queue answered calls by wait time

- `threecx-cli report-queue-answered-calls-by-wait-time download-queue-answered-calls-by-wait-time`  -  Invoke function DownloadQueueAnsweredCallsByWaitTime
- `threecx-cli report-queue-answered-calls-by-wait-time get-queue-answered-calls-by-wait-time-data`  -  Invoke function GetQueueAnsweredCallsByWaitTimeData

**report-queue-callbacks**  -  Manage report queue callbacks

- `threecx-cli report-queue-callbacks download-queue-callbacks`  -  Invoke function DownloadQueueCallbacks
- `threecx-cli report-queue-callbacks get-queue-callbacks-data`  -  Invoke function GetQueueCallbacksData

**report-queue-chat-performance**  -  Manage report queue chat performance

- `threecx-cli report-queue-chat-performance download-queue-chat-performance`  -  Invoke function DownloadQueueChatPerformance
- `threecx-cli report-queue-chat-performance get-queue-chat-performance-data`  -  Invoke function GetQueueChatPerformanceData

**report-queue-failed-callbacks**  -  Manage report queue failed callbacks

- `threecx-cli report-queue-failed-callbacks download-queue-failed-callbacks`  -  Invoke function DownloadQueueFailedCallbacks
- `threecx-cli report-queue-failed-callbacks get-queue-failed-callbacks-data`  -  Invoke function GetQueueFailedCallbacksData

**report-queue-performance-overview**  -  Manage report queue performance overview

- `threecx-cli report-queue-performance-overview download-queue-performance-overview`  -  Invoke function DownloadQueuePerformanceOverview
- `threecx-cli report-queue-performance-overview get-queue-performance-overview-data`  -  Invoke function GetQueuePerformanceOverviewData

**report-queue-performance-totals**  -  Manage report queue performance totals

- `threecx-cli report-queue-performance-totals download-queue-performance-totals`  -  Invoke function DownloadQueuePerformanceTotals
- `threecx-cli report-queue-performance-totals get-queue-performance-totals-data`  -  Invoke function GetQueuePerformanceTotalsData

**report-ring-group-statistics**  -  Manage report ring group statistics

- `threecx-cli report-ring-group-statistics download-ring-group-statistics`  -  Invoke function DownloadRingGroupStatistics
- `threecx-cli report-ring-group-statistics get-ring-group-statistics-data`  -  Invoke function GetRingGroupStatisticsData

**report-statistic-sla**  -  Manage report statistic sla

- `threecx-cli report-statistic-sla download-statistic-sla`  -  Invoke function DownloadStatisticSla
- `threecx-cli report-statistic-sla get-statistic-sla-data`  -  Invoke function GetStatisticSlaData

**report-team-queue-general-statistics**  -  Manage report team queue general statistics

- `threecx-cli report-team-queue-general-statistics download-team-queue-general-statistics`  -  Invoke function DownloadTeamQueueGeneralStatistics
- `threecx-cli report-team-queue-general-statistics get-team-queue-general-statistics-data`  -  Invoke function GetTeamQueueGeneralStatisticsData

**report-user-activity**  -  Manage report user activity

- `threecx-cli report-user-activity download-get-user-activity`  -  Invoke function DownloadGetUserActivity
- `threecx-cli report-user-activity get-user-activity`  -  Invoke function GetUserActivity

**ring-groups**  -  Manage ring groups

- `threecx-cli ring-groups create`  -  Add new entity to RingGroups
- `threecx-cli ring-groups get-first-available-number`  -  Invoke function GetFirstAvailableRingGroupNumber
- `threecx-cli ring-groups list`  -  Get entities from RingGroups

**ring-groups-id**  -  Manage ring groups id

- `threecx-cli ring-groups-id delete-ring-group`  -  Delete entity from RingGroups
- `threecx-cli ring-groups-id get-ring-group`  -  Get entity from RingGroups by key
- `threecx-cli ring-groups-id list-ring-group-members`  -  Get Members from RingGroups
- `threecx-cli ring-groups-id update-ring-group`  -  Update entity in RingGroups

**ring-groups-number-number**  -  Manage ring groups number number

- `threecx-cli ring-groups-number-number <Number>`  -  Get entity from RingGroups by key (Number)

**sbcs**  -  Manage sbcs

- `threecx-cli sbcs create`  -  Add new entity to Sbcs
- `threecx-cli sbcs list`  -  Get entities from Sbcs

**sbcs-name**  -  Manage sbcs name

- `threecx-cli sbcs-name delete-sbc`  -  Delete entity from Sbcs
- `threecx-cli sbcs-name get-sbc`  -  Get entity from Sbcs by key
- `threecx-cli sbcs-name push-config`  -  Invoke action PushConfig
- `threecx-cli sbcs-name update-sbc`  -  Update entity in Sbcs

**scheduled-reports**  -  Manage scheduled reports

- `threecx-cli scheduled-reports create`  -  Add new entity to ScheduledReports
- `threecx-cli scheduled-reports list`  -  Get entities from ScheduledReports

**scheduled-reports-id**  -  Manage scheduled reports id

- `threecx-cli scheduled-reports-id delete-scheduled-report`  -  Delete entity from ScheduledReports
- `threecx-cli scheduled-reports-id get-scheduled-report`  -  Get entity from ScheduledReports by key
- `threecx-cli scheduled-reports-id update-scheduled-report`  -  Update entity in ScheduledReports

**secure-sip-settings**  -  Manage secure sip settings

- `threecx-cli secure-sip-settings get`  -  Get SecureSipSettings
- `threecx-cli secure-sip-settings update`  -  Update SecureSipSettings

**security-tokens**  -  Manage security tokens

- `threecx-cli security-tokens`  -  Get entities from SecurityTokens

**security-tokens-id**  -  Manage security tokens id

- `threecx-cli security-tokens-id <Id>`  -  Invoke action RevokeToken

**service-principals**  -  Manage service principals

- `threecx-cli service-principals create`  -  Add new entity to ServicePrincipals
- `threecx-cli service-principals list`  -  Get entities from ServicePrincipals

**service-principals-id**  -  Manage service principals id

- `threecx-cli service-principals-id delete-service-principal`  -  Delete entity from ServicePrincipals
- `threecx-cli service-principals-id generate-app-token`  -  Invoke action GenerateAppToken
- `threecx-cli service-principals-id get-service-principal`  -  Get entity from ServicePrincipals by key
- `threecx-cli service-principals-id update-service-principal`  -  Update entity in ServicePrincipals

**services**  -  Manage services

- `threecx-cli services disable`  -  Invoke action Disable
- `threecx-cli services enable`  -  Invoke action Enable
- `threecx-cli services garbage-collect`  -  Invoke action GarbageCollect
- `threecx-cli services list-info`  -  Get entities from Services
- `threecx-cli services restart`  -  Invoke action Restart
- `threecx-cli services restart-operating-system`  -  Invoke action RestartOperatingSystem
- `threecx-cli services start`  -  Invoke action Start
- `threecx-cli services stop`  -  Invoke action Stop

**set-update-settings**  -  Manage set update settings

- `threecx-cli set-update-settings`  -  Invoke actionImport SetUpdateSettings

**sip-devices**  -  Manage sip devices

- `threecx-cli sip-devices bulk-switch-firmware`  -  Invoke action BulkSwitchFirmware
- `threecx-cli sip-devices bulk-update-firmware`  -  Invoke action BulkUpdateFirmware
- `threecx-cli sip-devices list`  -  Get entities from SipDevices

**sip-devices-id**  -  Manage sip devices id

- `threecx-cli sip-devices-id clear-firmware`  -  Invoke action ClearFirmware
- `threecx-cli sip-devices-id move-phone`  -  Invoke action MovePhone
- `threecx-cli sip-devices-id push-firmware`  -  Invoke action PushFirmware

**syslog-settings**  -  Manage syslog settings

- `threecx-cli syslog-settings get`  -  Get SyslogSettings
- `threecx-cli syslog-settings test-connection`  -  Invoke action TestConnection
- `threecx-cli syslog-settings update`  -  Update SyslogSettings

**system-status**  -  Manage system status

- `threecx-cli system-status apitoken`  -  Invoke function APIToken
- `threecx-cli system-status get`  -  Get SystemStatus
- `threecx-cli system-status get-remote-access-status`  -  Invoke function GetRemoteAccessStatus
- `threecx-cli system-status get-request-help-link`  -  Invoke function GetRequestHelpLink
- `threecx-cli system-status get-version-type`  -  Invoke function GetVersionType
- `threecx-cli system-status is-request-help-enabled`  -  Invoke function IsRequestHelpEnabled
- `threecx-cli system-status my-phone-request-id-telemetry`  -  Invoke function MyPhoneRequestIdTelemetry
- `threecx-cli system-status network-telemetry`  -  Invoke function NetworkTelemetry
- `threecx-cli system-status request-help`  -  Invoke action RequestHelp
- `threecx-cli system-status revoke-remote-access`  -  Invoke action RevokeRemoteAccess
- `threecx-cli system-status service-telemetry`  -  Invoke function ServiceTelemetry
- `threecx-cli system-status set-chat-log-status`  -  Invoke action SetChatLogStatus
- `threecx-cli system-status set-multi-company-mode`  -  Invoke action SetMultiCompanyMode
- `threecx-cli system-status start-dbmaintenance`  -  Invoke action StartDBMaintenance
- `threecx-cli system-status system-database-information`  -  Invoke function SystemDatabaseInformation
- `threecx-cli system-status system-extensions`  -  Invoke function SystemExtensions
- `threecx-cli system-status system-health-status`  -  Invoke function SystemHealthStatus
- `threecx-cli system-status system-telemetry`  -  Invoke function SystemTelemetry

**tenant-properties**  -  Manage tenant properties

- `threecx-cli tenant-properties create-property`  -  Add new entity to TenantProperties
- `threecx-cli tenant-properties list-property`  -  Get entities from TenantProperties

**tenant-properties-name**  -  Manage tenant properties name

- `threecx-cli tenant-properties-name delete-property`  -  Delete entity from TenantProperties
- `threecx-cli tenant-properties-name get-property`  -  Get entity from TenantProperties by key
- `threecx-cli tenant-properties-name update-property`  -  Update entity in TenantProperties

**trunk-templates**  -  Manage trunk templates

- `threecx-cli trunk-templates create`  -  Add new entity to TrunkTemplates
- `threecx-cli trunk-templates list`  -  Get entities from TrunkTemplates

**trunk-templates-id**  -  Manage trunk templates id

- `threecx-cli trunk-templates-id delete-trunk-template`  -  Delete entity from TrunkTemplates
- `threecx-cli trunk-templates-id get-trunk-template`  -  Get entity from TrunkTemplates by key
- `threecx-cli trunk-templates-id update-trunk-template`  -  Update entity in TrunkTemplates

**trunks**  -  Manage trunks

- `threecx-cli trunks create`  -  Add new entity to Trunks
- `threecx-cli trunks finalize-provisioning`  -  Invoke action FinalizeTrunkProvisioning
- `threecx-cli trunks get-first-available-number`  -  Invoke function GetFirstAvailableTrunkNumber
- `threecx-cli trunks get-provider-phones`  -  Invoke action GetProviderPhones
- `threecx-cli trunks init`  -  Invoke function InitTrunk
- `threecx-cli trunks init-master-bridge`  -  Invoke function InitMasterBridge
- `threecx-cli trunks init-slave-bridge`  -  Invoke function InitSlaveBridge
- `threecx-cli trunks list`  -  Get entities from Trunks
- `threecx-cli trunks refresh-registration`  -  Invoke action RefreshRegistration
- `threecx-cli trunks set-routes`  -  Invoke action SetRoutes

**trunks-id**  -  Manage trunks id

- `threecx-cli trunks-id delete-trunk`  -  Delete entity from Trunks
- `threecx-cli trunks-id export-trunk`  -  Invoke function ExportTrunk
- `threecx-cli trunks-id get-trunk`  -  Get entity from Trunks by key
- `threecx-cli trunks-id run-analysis`  -  Invoke action RunAnalysis
- `threecx-cli trunks-id test-inbound-call`  -  Invoke action TestInboundCall
- `threecx-cli trunks-id test-outbound-call`  -  Invoke action TestOutboundCall
- `threecx-cli trunks-id update-trunk`  -  Update entity in Trunks

**trunks-number-number**  -  Manage trunks number number

- `threecx-cli trunks-number-number <Number>`  -  Get entity from Trunks by key (Number)

**users**  -  Manage users

- `threecx-cli users batch-delete`  -  Invoke action BatchDelete
- `threecx-cli users bulk-update`  -  Invoke action BulkUpdate
- `threecx-cli users create`  -  Add new entity to Users
- `threecx-cli users download-greeting`  -  Invoke function DownloadGreeting
- `threecx-cli users export-extensions`  -  Invoke function ExportExtensions
- `threecx-cli users get-duplicated-emails`  -  Invoke action GetDuplicatedEmails
- `threecx-cli users get-first-available-extension-number`  -  Invoke function GetFirstAvailableExtensionNumber
- `threecx-cli users get-first-available-hotdesking-number`  -  Invoke function GetFirstAvailableHotdeskingNumber
- `threecx-cli users get-multi-edit-greetings`  -  Invoke action GetMultiEditGreetings
- `threecx-cli users get-phone-registrar`  -  Invoke function GetPhoneRegistrar
- `threecx-cli users get-phone-registrars`  -  Invoke action GetPhoneRegistrars
- `threecx-cli users install-firmware`  -  Invoke action InstallFirmware
- `threecx-cli users list`  -  Get entities from Users
- `threecx-cli users make-call`  -  Invoke action MakeCall
- `threecx-cli users multi-delete-greeting`  -  Invoke action MultiDeleteGreeting
- `threecx-cli users multi-update`  -  Invoke action MultiUserUpdate
- `threecx-cli users reboot-phone`  -  Invoke action RebootPhone
- `threecx-cli users regenerate-passwords`  -  Invoke action RegeneratePasswords
- `threecx-cli users reprovision-all-phones`  -  Invoke action ReprovisionAllPhones
- `threecx-cli users reprovision-phone`  -  Invoke action ReprovisionPhone

**users-id**  -  Manage users id

- `threecx-cli users-id delete-user`  -  Delete entity from Users
- `threecx-cli users-id generate-prov-link`  -  Invoke function GenerateProvLink
- `threecx-cli users-id get-user`  -  Get entity from Users by key
- `threecx-cli users-id has-duplicated-email`  -  Invoke function HasDuplicatedEmail
- `threecx-cli users-id list-forwarding-profiles`  -  Get ForwardingProfiles from Users
- `threecx-cli users-id list-greetings`  -  Get Greetings from Users
- `threecx-cli users-id list-groups`  -  Get Groups from Users
- `threecx-cli users-id make-call-user-record-greeting`  -  Invoke action MakeCallUserRecordGreeting
- `threecx-cli users-id regenerate`  -  Invoke action Regenerate
- `threecx-cli users-id resend-rps`  -  Invoke action ResendRps
- `threecx-cli users-id revoke-rps`  -  Invoke action RevokeRps
- `threecx-cli users-id send-welcome-email`  -  Invoke action SendWelcomeEmail
- `threecx-cli users-id set-monitor-status`  -  Invoke action SetMonitorStatus
- `threecx-cli users-id update-user`  -  Update entity in Users

**users-number-number**  -  Manage users number number

- `threecx-cli users-number-number <Number>`  -  Get entity from Users by key (Number)

**voicemail-settings**  -  Manage voicemail settings

- `threecx-cli voicemail-settings create-converter-configuration`  -  Invoke action CreateConverterConfiguration
- `threecx-cli voicemail-settings delete-all-user-voicemails`  -  Invoke action DeleteAllUserVoicemails
- `threecx-cli voicemail-settings delete-converter-configuration`  -  Invoke action DeleteConverterConfiguration
- `threecx-cli voicemail-settings get`  -  Get VoicemailSettings
- `threecx-cli voicemail-settings get-configured-converters`  -  Invoke function GetConfiguredConverters
- `threecx-cli voicemail-settings get-connected-converters-data`  -  Invoke function GetConnectedConvertersData
- `threecx-cli voicemail-settings get-converter-request-status`  -  Invoke function GetConverterRequestStatus
- `threecx-cli voicemail-settings get-transcribe-languages`  -  Invoke function GetTranscribeLanguages
- `threecx-cli voicemail-settings update`  -  Update VoicemailSettings

**website-links**  -  Manage website links

- `threecx-cli website-links bulk-links-delete`  -  Invoke action BulkLinksDelete
- `threecx-cli website-links create-weblink`  -  Add new entity to WebsiteLinks
- `threecx-cli website-links list-weblink`  -  Get entities from WebsiteLinks
- `threecx-cli website-links validate-link`  -  Invoke action ValidateLink

**website-links-link**  -  Manage website links link

- `threecx-cli website-links-link delete-weblink`  -  Delete entity from WebsiteLinks
- `threecx-cli website-links-link get-weblink`  -  Get entity from WebsiteLinks by key
- `threecx-cli website-links-link update-weblink`  -  Update entity in WebsiteLinks


**audit**


- `threecx-cli audit`  -  Find ring groups, queues, IVRs and inbound rules that route to deleted, disabled or missing extensions, on one phone system or the whole fleet.


**fleet**


- `threecx-cli fleet health`  -  One row per client phone system with pass, warn, fail, unknown (not reported) or error (unreadable) for version, licence and maintenance expiry, last backup, trunks and extensions registered, disk, stopped services and capacity.

- `threecx-cli fleet trunks`  -  Every SIP trunk on every client phone system with state (online, offline, issue, unknown or error), certificate days left and configuration issues, problems first.

- `threecx-cli fleet licences`  -  Licence and maintenance expiry, edition, seat headroom and reseller of record for every client phone system, soonest expiry first.

- `threecx-cli fleet versions`  -  Each phone system's 3CX build against the newest build in your fleet, with auto-update state, pending updates and expired maintenance that blocks updates.

- `threecx-cli fleet backups`  -  Backup age, schedule, rotation and newest backup file for every client phone system, flagged never, stale, unscheduled, no files or shrunk.

- `threecx-cli fleet exec`  -  Run any read-only Configuration API query on every client phone system and get one merged table with a pbx column; credential fields are redacted and token-minting or download functions are refused.

- `threecx-cli fleet changes`  -  What changed across the fleet between two saved health or trunk runs: trunks offline or back, version bumped, licence renewed, backup gone stale, PBX unreachable.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
threecx-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Morning fleet check

```bash
threecx-cli fleet health --agent
```

One pass/warn/fail row per client PBX covering version, licence, backup, trunks, disk and services.

### Dead trunks only, narrow fields

```bash
threecx-cli fleet trunks --offline --agent --select pbx,trunk,state,configuration_issue,error
```

Trunks that are offline, misconfigured or in an unknown state across the fleet, plus any PBX that could not be read, with just the fields a triage note needs.

### Renewals sheet

```bash
threecx-cli fleet licences --within 60d --csv
```

Licences and maintenance contracts expiring in the next 60 days or already expired, plus any PBX that could not be read, soonest first, ready for a spreadsheet.

### Users on every PBX

```bash
threecx-cli fleet exec /Users --param '$select=Number,FirstName,LastName,EmailAddress' --agent
```

One merged list of extensions across all clients with a pbx column.

### What changed overnight

```bash
threecx-cli fleet changes --since 24h --agent
```

Trunks that dropped, versions that moved, renewals that landed and backups that went stale between the last two saved runs; needs at least two earlier 'fleet health' or 'fleet trunks' runs.

## Auth Setup

Each 3CX phone system has its own Configuration API (XAPI) credential. In the PBX admin console go to Integrations > API, add a client, tick '3CX Configuration API Access', and give it a department and the least role that works: System Admin covers the fleet reports; only reports and call history need System Owner. The client ID is the route point's DN number; the API key is shown once. The CLI exchanges them at https://<pbx-fqdn>/connect/token for a 60-minute bearer token and re-mints automatically. For a fleet, run 'fleet add <name> --fqdn <host> --client-id <dn>' once per PBX and export each key as THREECX_<NAME>_CLIENT_SECRET (or <PREFIX>_CLIENT_SECRET with --env-prefix <PREFIX>); the fleet registry never stores keys, and 'fleet list' shows which variables are missing. For a single PBX without the registry, set THREECX_FQDN, THREECX_CLIENT_ID and THREECX_CLIENT_SECRET. Check one system with 'doctor --pbx <name>'. The Configuration API requires an 8SC or higher licence. Do not pass --client-secret to 'auth login': it saves the key to credentials.toml; use the env-var paths instead.

Run `threecx-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color`.

Global format flags share one contract on promoted, novel, sync, and `--deliver` paths:

- `--json`  -  one JSON document on stdout (sync progress events go to stderr)
- `--compact`  -  keep identity/status/timestamp fields; does not change the document vs stream shape
- `--csv` / `--plain`  -  tabular rows (collection envelopes unwrap to the row array). `--csv` prefixes `'` on a cell or header key that starts with `=`, `+`, `-`, `@`, tab, CR, or LF (fullwidth `＝＋－＠` too) so a spreadsheet does not run it as a formula; a plain signed number such as `-5` or `+2.5e3` is unchanged.
- `--quiet`  -  one identity value per row, no envelope

- **Pipeable**  -  JSON on stdout, errors on stderr
- **Filterable**  -  `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  threecx-cli active-calls --agent
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Explicit retries**  -  use `--idempotent` only when an already-existing create should count as success, and use `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set  -  piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `THREECX_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `THREECX_CONFIG_DIR`, `THREECX_DATA_DIR`, `THREECX_STATE_DIR`, `THREECX_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `THREECX_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `threecx-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "threecx": {
        "command": "threecx-mcp",
        "env": {
          "THREECX_HOME": "/srv/threecx"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `THREECX_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `THREECX_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `THREECX_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `THREECX_DATA_SOURCE` sets the `--data-source` default.

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
threecx-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "threecx-cli learnings confirm 12"] }
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
       materially more, record the divergence via `threecx-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `threecx-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `threecx-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
threecx-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
threecx-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
threecx-cli teach-playbook \
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
threecx-cli playbook amend \
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

`threecx-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `THREECX_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
threecx-cli feedback "the --since flag is inclusive but docs say exclusive"
threecx-cli feedback --stdin < notes.txt
threecx-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `THREECX_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `THREECX_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` as a private 0600 file (exclusive randomly named temp in the destination directory, then rename; a planted symlink is not followed). Binary-response commands write decoded payload bytes (not the base64 JSON envelope) and print a small JSON receipt on stdout; `--json`/`--csv` do not refuse when this sink is set. |
| `webhook:<url>` | POST the output body to the URL (`application/json`). Redirects are not followed; a 3xx response fails the delivery and the body is not sent to the `Location`. |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures, including redirects, return non-zero and log the URL and HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled or recurring agent reuses the same saved flags while providing different input each run.

```
threecx-cli profile save briefing --json
threecx-cli --profile briefing active-calls
threecx-cli profile list --json
threecx-cli profile show briefing
threecx-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

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
| 8 | Fleet problem found (`fleet health --fail-on`, `fleet trunks --fail-on-offline`) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `threecx-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add threecx-mcp -- threecx-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `threecx-mcp --transport http` requires `THREECX_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `THREECX_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `THREECX_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which threecx-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   threecx-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `threecx-cli <command> --help`.
