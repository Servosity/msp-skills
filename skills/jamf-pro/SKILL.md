---
name: jamf-pro
description: "Every client's Jamf Pro in one CLI: register each instance once, then find devices, check posture and patch status, and audit policies across all of them. Trigger phrases: `which client has this Mac`, `jamf posture across clients`, `macs not checked in`, `patch status in jamf`, `broken jamf policies`, `use jamf-pro`, `run jamf-pro`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Jamf"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - jamf-pro-cli
    install:
      - kind: script
        bins: [jamf-pro-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.ps1
---

# Jamf Pro Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `jamf-pro-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.ps1 | iex
   ```
3. Verify: `jamf-pro-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `jamf-pro-cli` and `jamf-pro-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Covers the Jamf Pro API and the Classic API, including script, package and policy CRUD; list commands read every page with --all instead of stopping at the default 100. 'fleet find', 'fleet posture', 'fleet patch' and 'fleet policy-hygiene' answer cross-client questions that otherwise mean logging into each instance in turn; they need each client registered once with 'instance add'.

## When to Use This CLI

Use this CLI when an MSP or IT team manages one or more Jamf Pro instances and needs device lookups, compliance and patch rollups, policy and script hygiene, or script, package and policy CRUD from the terminal or an agent. It is strongest when the same question has to be asked of several client instances at once. The fleet commands read only instances registered with 'instance add' and return an empty result when none are registered; 'search' reads only the local store filled by 'sync'. This CLI was built from Jamf's published API reference without a live tenant, so confirm results against one instance before acting at scale.

## Anti-triggers

Do not use this CLI for:
- Do not use it to retrieve LAPS passwords or password audits, FileVault recovery keys, recovery-lock passwords or device lock PINs; those endpoints are deliberately not included
- Do not use it for Jamf Protect, Jamf School or Jamf Now; those are separate products with separate APIs
- Do not use it to wipe, unmanage or send MDM commands across many clients at once; destructive actions run against one instance at a time with --instance

## Unique Capabilities

### One view across every client
- **`fleet find`**  -  Find which client's Jamf Pro instance has a Mac, iPad or iPhone, by serial, name, username or email, across every registered instance at once.

  _Reach for this first when a ticket names a device or user but not the client._

  ```bash
  jamf-pro-cli fleet find C02XK1ABJG5H --agent
  ```
- **`fleet posture`**  -  Per-client counts of stale check-ins, Macs below a macOS floor, FileVault off, firewall off and expiring MDM profiles, with Macs that inventory cannot fully judge reported as incomplete.

  _Use for any cross-client compliance question about Macs._

  ```bash
  jamf-pro-cli fleet posture --stale-days 14 --min-os 14.6 --agent
  ```
- **`fleet patch`**  -  Patch titles behind latest and failing patch-policy installs (one patch_title row per client and title), plus failed managed OS updates (one os_updates row per client).

  _Use for weekly patch review across clients._

  ```bash
  jamf-pro-cli fleet patch --behind-only --agent
  ```
- **`fleet app`**  -  Which clients and computers have an installed application below a version floor.

  _Use when a vendor advisory lands and you need every exposed client._

  ```bash
  jamf-pro-cli fleet app "Google Chrome" --below 129.0.6668.100 --agent
  ```

### Keep every client clean
- **`fleet policy-hygiene`**  -  Flags policies that are disabled, can never run, have no scope, are expired, uncategorized, or point at a missing script or package.

  _Use to find broken policies before a client notices._

  ```bash
  jamf-pro-cli fleet policy-hygiene --agent
  ```
- **`fleet health`**  -  ADE and VPP token expiry, failed enrollment syncs, health check and Jamf Pro version spread for every client.

  _Use to catch a lapsing enrollment or VPP token before it stops onboarding._

  ```bash
  jamf-pro-cli fleet health --within 30d --agent
  ```
- **`fleet baseline`**  -  Compare every client instance against a golden instance: scripts and extension attributes by content; packages, policies, profiles and smart groups by name.

  _Use after changing an MSP standard to see which clients lag._

  ```bash
  jamf-pro-cli fleet baseline --golden msp-template --agent
  ```

### Many instances, one CLI
- **`instance`**  -  Register each client's Jamf Pro instance once (server and API client id; its secret goes in JAMF_PRO_FLEET_SECRETS), then target it with --instance or report on all of them with the fleet commands.

  _Run instance test before any fleet report to prove every client's API client still works._

  ```bash
  jamf-pro-cli instance test --agent
  ```
- **`fleet exec`**  -  Run any read-only Jamf Pro or Classic API GET on every client instance and get one merged table with an instance column; credential fields are redacted and credential endpoints refused.

  _Use for any cross-client read no dedicated fleet command covers._

  ```bash
  jamf-pro-cli fleet exec /api/v1/jamf-pro-version --agent
  ```

## Command Reference

**account_groups**  -  Manage account groups

- `jamf-pro-cli account-groups get`  -  Gets the account group for the given id. Required privilege: Read Account Groups
- `jamf-pro-cli account-groups list`  -  Get all account groups with pagination, sorting, and filtering support. Required privilege: Read Account Groups

**accounts**  -  Manage accounts

- `jamf-pro-cli accounts get`  -  Gets the user account for the given id. Required privilege: Read Accounts
- `jamf-pro-cli accounts list`  -  Get all user accounts with pagination, sorting, and filtering support. Required privilege: Read Accounts

**advanced_computer_searches**  -  Manage advanced computer searches

- `jamf-pro-cli advanced-computer-searches get`  -  Additional display fields are returned within the `Computer` object
- `jamf-pro-cli advanced-computer-searches list`  -  Finds all advanced computer searches

**api_integrations**  -  Manage api integrations

- `jamf-pro-cli api-integrations create`  -  Create API integration object. Required privilege: Create API Integrations
- `jamf-pro-cli api-integrations delete`  -  Removes specified API integration. Required privilege: Delete API Integrations
- `jamf-pro-cli api-integrations get`  -  Gets specified API integration object. Required privilege: Read API Integrations
- `jamf-pro-cli api-integrations list`  -  Get Jamf Pro API Integrations with Search Criteria. Required privilege: Read API Integrations
- `jamf-pro-cli api-integrations update`  -  Update specified API integration object. Required privilege: Update API Integrations

**api_role_privileges**  -  Manage api role privileges

- `jamf-pro-cli api-role-privileges`  -  Get role privileges. Required privilege: Read API Roles

**api_roles**  -  Manage api roles

- `jamf-pro-cli api-roles create`  -  Post to create new Role. Required privilege: Create API Roles
- `jamf-pro-cli api-roles delete`  -  Delete specific Role. Required privilege: Delete API Roles
- `jamf-pro-cli api-roles get`  -  Get specific Role. Required privilege: Read API Roles
- `jamf-pro-cli api-roles list`  -  Get roles with Search Criteria. Required privilege: Read API Roles
- `jamf-pro-cli api-roles update`  -  Update specific Role. Required privilege: Update API Roles

**buildings**  -  Manage buildings

- `jamf-pro-cli buildings add-history-note`  -  Adds specified Building history object notes. Required privilege: Update Buildings
- `jamf-pro-cli buildings create`  -  Create building record. Required privilege: Create Buildings
- `jamf-pro-cli buildings delete`  -  Removes specified building record. Required privilege: Delete Buildings
- `jamf-pro-cli buildings delete-multiple`  -  multiple many Buildings by their ids. Required privilege: Delete Buildings
- `jamf-pro-cli buildings get`  -  Gets specified Building object. Required privilege: Read Buildings
- `jamf-pro-cli buildings get-history`  -  Gets specified Building history object. Required privilege: Read Buildings
- `jamf-pro-cli buildings list`  -  Search for sorted and paged buildings. Required privilege: Read Buildings
- `jamf-pro-cli buildings update`  -  Update specified building object. Required privilege: Update Buildings

**categories**  -  Manage categories

- `jamf-pro-cli categories add-history-note`  -  Adds specified Category history object notes. Required privilege: Update Categories
- `jamf-pro-cli categories create`  -  Create category record. Required privilege: Create Categories
- `jamf-pro-cli categories delete`  -  Removes specified category record. Required privilege: Delete Categories
- `jamf-pro-cli categories delete-multiple`  -  Delete multiple Categories by their IDs. Required privilege: Delete Categories
- `jamf-pro-cli categories get`  -  Gets specified Category object. Required privilege: Read Categories
- `jamf-pro-cli categories get-history`  -  Gets specified Category history object. Required privilege: Read Categories
- `jamf-pro-cli categories list`  -  Gets `Category` objects. Required privileges: > - Read Categories > - Read Self Service
- `jamf-pro-cli categories update`  -  Update specified category object. Required privilege: Update Categories

**computer_applications**  -  Manage computer applications

- `jamf-pro-cli computer-applications get-by-application`  -  Finds computer applications by name
- `jamf-pro-cli computer-applications get-by-application-inventory`  -  Valid values for inventory are comma separated Display Fields for computers.
- `jamf-pro-cli computer-applications get-by-version`  -  Finds computer applications by name and version
- `jamf-pro-cli computer-applications get-by-version-inventory`  -  Generates additional inventory data for the devices matching the search criteria.

**computer_extension_attributes**  -  Manage computer extension attributes

- `jamf-pro-cli computer-extension-attributes add-history-note`  -  Add specified Computer Extension Attribute history object notes.
- `jamf-pro-cli computer-extension-attributes create`  -  Create Computer Extension Attribute to collect extra inventory information.
- `jamf-pro-cli computer-extension-attributes delete`  -  ID of the Computer Extension Attribute to be deleted. Required privilege: Delete Computer Extension Attributes
- `jamf-pro-cli computer-extension-attributes delete-multiple`  -  IDs of the Computer Extension Attribute to be deleted. Required privilege: Delete Computer Extension Attributes
- `jamf-pro-cli computer-extension-attributes get`  -  Gets specified Computer Extension Attribute object. Required privilege: Read Computer Extension Attributes
- `jamf-pro-cli computer-extension-attributes get-data-dependency`  -  Get smart group/advance search dependent objects for a specified computer extension attribute.
- `jamf-pro-cli computer-extension-attributes get-download`  -  Retrieves the specified Computer Extension Attribute in XML format based on the provided unique ID.
- `jamf-pro-cli computer-extension-attributes get-history`  -  Get specified Computer Extension Attribute history object. Required privilege: Read Computer Extension Attributes
- `jamf-pro-cli computer-extension-attributes get-templates`  -  Gets specified Computer Extension Attribute Template object. Required privilege: Read Computer Extension Attributes
- `jamf-pro-cli computer-extension-attributes list`  -  Retrieves All Computer Extension Attributes Configuration. Required privilege: Read Computer Extension Attributes
- `jamf-pro-cli computer-extension-attributes list-templates`  -  Retrieves All Computer Extension Attributes Templates. Required privilege: Read Computer Extension Attributes
- `jamf-pro-cli computer-extension-attributes update`  -  Update specified Computer Extension Attribute object. Required privilege: Update Computer Extension Attributes
- `jamf-pro-cli computer-extension-attributes upload`  -  Uploads a Computer Extension Attribute.

**computer_groups**  -  Manage computer groups

- `jamf-pro-cli computer-groups create-smart-groups`  -  Creates a Smart Computer Group with strict andOr enum validation. Required privilege: Create Smart Computer Groups
- `jamf-pro-cli computer-groups create-static-groups`  -  Create membership of a static computer group. Required privilege: Create Static Computer Groups
- `jamf-pro-cli computer-groups delete-smart-groups`  -  Remove specified Smart Computer Group. Returns 422 if the group has dependencies.
- `jamf-pro-cli computer-groups delete-static-groups`  -  Remove Static Computer Group by Id. Returns 422 if the group has dependencies.
- `jamf-pro-cli computer-groups get-smart-group-membership`  -  Gets the membership of a Smart Computer Group. Required privilege: Read Smart Computer Groups
- `jamf-pro-cli computer-groups get-smart-groups`  -  Get Smart Computer Group by Id. Required privilege: Read Smart Computer Groups
- `jamf-pro-cli computer-groups get-static-groups`  -  Get Static Computer Group by Id. Required privilege: Read Static Computer Groups
- `jamf-pro-cli computer-groups list`  -  Use it to get the list of all computer groups.
- `jamf-pro-cli computer-groups list-smart-groups`  -  Search for Smart Computer Groups. Required privilege: Read Smart Computer Groups
- `jamf-pro-cli computer-groups list-static-groups`  -  Search for Static Computer Groups. Required privilege: Read Static Computer Groups
- `jamf-pro-cli computer-groups update-smart-groups`  -  Updates a Smart Computer Group with strict andOr enum validation. Required privilege: Update Smart Computer Groups
- `jamf-pro-cli computer-groups update-static-groups`  -  Update membership of a static computer group. Required privilege: Update Static Computer Groups

**computer_history**  -  Manage computer history

- `jamf-pro-cli computer-history get`  -  Finds computer history by ID
- `jamf-pro-cli computer-history get-subset`  -  Subset values can also be appended using an ampersand to return multiple subsets (e.g. /subsets/General&Commands)

**computer_inventory**  -  Manage computer inventory

- `jamf-pro-cli computer-inventory create`  -  Creates Computer Inventory record. Required privilege: Create Computers
- `jamf-pro-cli computer-inventory delete`  -  Remove specified Computer record. Required privilege: Delete Computers
- `jamf-pro-cli computer-inventory erase`  -  Erase a computer. Required privilege: Send Computer Remote Wipe Command
- `jamf-pro-cli computer-inventory get`  -  Return General section of a Computer. Required privilege: Read Computers
- `jamf-pro-cli computer-inventory get-detail`  -  Return all sections of a computer. Required privilege: Read Computers
- `jamf-pro-cli computer-inventory list`  -  Return paginated Computer Inventory records. Required privilege: Read Computers
- `jamf-pro-cli computer-inventory remove-mdm-profile`  -  Remove a computer's MDM profile. Required privilege: Send Computer Unmanage Command
- `jamf-pro-cli computer-inventory update`  -  Update specific fields on a computer, then return the updated computer object. Required privilege: Update Computers

**computer_management**  -  Manage computer management

- `jamf-pro-cli computer-management get`  -  Finds computer management information by ID
- `jamf-pro-cli computer-management get-subset`  -  Subset values can also be appended using an ampersand to return multiple subsets (e.g. /subsets/General&Policies)

**computer_prestages**  -  Manage computer prestages

- `jamf-pro-cli computer-prestages create`  -  Create a computer prestage. Required privilege: Create Computer PreStage Enrollments
- `jamf-pro-cli computer-prestages create-scope`  -  Add device scope for a specific computer prestage. Required privilege: Update Computer PreStage Enrollments
- `jamf-pro-cli computer-prestages delete`  -  Deletes a Computer Prestage with the supplied id. Required privilege: Delete Computer PreStage Enrollments
- `jamf-pro-cli computer-prestages get`  -  Retrieves a Computer Prestage with the supplied id. Required privilege: Read Computer PreStage Enrollments
- `jamf-pro-cli computer-prestages get-scope`  -  Get device scope for a specific computer prestage. Required privilege: Read Computer PreStage Enrollments
- `jamf-pro-cli computer-prestages list`  -  Gets sorted and paged computer prestages. Required privilege: Read Computer PreStage Enrollments
- `jamf-pro-cli computer-prestages list-scope`  -  Get all device scope for all computer prestages. Required privilege: Read Computer PreStage Enrollments
- `jamf-pro-cli computer-prestages scope-delete-multiple`  -  Remove device scope for a specific computer prestage. Required privilege: Update Computer PreStage Enrollments
- `jamf-pro-cli computer-prestages update`  -  Updates a Computer Prestage. Required privilege: Update Computer PreStage Enrollments
- `jamf-pro-cli computer-prestages update-scope`  -  Replace device scope for a specific computer prestage. Required privilege: Update Computer PreStage Enrollments

**computers_preview**  -  Manage computers preview

- `jamf-pro-cli computers-preview`  -  Returns a list of computers. Required privilege: Read Computers

**departments**  -  Manage departments

- `jamf-pro-cli departments add-history-note`  -  Adds specified Department history object notes. Required privilege: Update Departments
- `jamf-pro-cli departments create`  -  Create department record. Required privilege: Create Departments
- `jamf-pro-cli departments delete`  -  Removes specified department record. Required privilege: Delete Departments
- `jamf-pro-cli departments delete-multiple`  -  Deletes all departments by ids passed in body. Required privilege: Delete Departments
- `jamf-pro-cli departments get`  -  Gets specified Department object. Required privilege: Read Departments
- `jamf-pro-cli departments get-history`  -  Gets specified Department history object. Required privilege: Read Departments
- `jamf-pro-cli departments list`  -  Search for Departments. Required privilege: Read Departments
- `jamf-pro-cli departments update`  -  Update specified department object. Required privilege: Update Departments

**device_enrollments**  -  Manage device enrollments

- `jamf-pro-cli device-enrollments add-history-note`  -  Adds device enrollment history object notes. Required privilege: Update Device Enrollment Program Instances
- `jamf-pro-cli device-enrollments delete`  -  Deletes a Device Enrollment Instance with the supplied id.
- `jamf-pro-cli device-enrollments disown`  -  Disowns devices from the given device enrollment instance.
- `jamf-pro-cli device-enrollments get`  -  Retrieves a Device Enrollment Instance with the supplied id.
- `jamf-pro-cli device-enrollments get-history`  -  Gets sorted and paged device enrollment history objects. Required privilege: Read Device Enrollment Program Instances
- `jamf-pro-cli device-enrollments get-syncs`  -  Get all instance sync states for a single instance. Required privilege: Read Device Enrollment Program Instances
- `jamf-pro-cli device-enrollments get-syncs-latest`  -  Get the latest sync state for a single instance. Required privilege: Read Device Enrollment Program Instances
- `jamf-pro-cli device-enrollments list`  -  Search for sorted and paged device enrollment instances. Required privilege: Read Device Enrollment Program Instances
- `jamf-pro-cli device-enrollments list-public-key`  -  Retrieve the Jamf Pro device enrollment public key. Required privilege: Read Device Enrollment Program Instances
- `jamf-pro-cli device-enrollments list-syncs`  -  Get all instance sync states for all instances. Required privilege: Read Device Enrollment Program Instances
- `jamf-pro-cli device-enrollments update`  -  Updates a Device Enrollment Instance with the supplied id.

**device_enrollments_devices**  -  Manage device enrollments devices

- `jamf-pro-cli device-enrollments-devices <id>`  -  Retrieves a list of devices assigned to the supplied id. Required privilege: Read Device Enrollment Program Instances

**device_users**  -  Manage device users

- `jamf-pro-cli device-users get`  -  Finds users by ID
- `jamf-pro-cli device-users list`  -  Finds all users

**disk_encryption_configurations**  -  Manage disk encryption configurations

- `jamf-pro-cli disk-encryption-configurations get`  -  Finds disk encryption configurations by ID
- `jamf-pro-cli disk-encryption-configurations list`  -  Finds all disk encryption configurations

**groups**  -  Manage groups

- `jamf-pro-cli groups delete`  -  Deletes a group by its platform UUID. Returns a 422 error if the group is being used as a dependency.
- `jamf-pro-cli groups get`  -  Returns group information for the given platform UUID.
- `jamf-pro-cli groups list`  -  Returns group information for all Mobile Device and Computer groups.
- `jamf-pro-cli groups update`  -  Updates a group by its platform UUID. For both smart and static groups, groupName and groupDescription can be updated.

**health_check**  -  Manage health check

- `jamf-pro-cli health-check list`  -  Get Jamf Pro API status.
- `jamf-pro-cli health-check list-status`  -  Returns metrics representing the request acceptance ratio for each concurrency group and time window on this Jamf Pro

**inventory_information**  -  Manage inventory information

- `jamf-pro-cli inventory-information`  -  Gets statistics about managed/unmanaged devices and computers in the inventory.

**jamf_pro_information**  -  Read basic information about the Jamf Pro server

- `jamf-pro-cli jamf-pro-information`  -  Get basic information about the Jamf Pro Server

**jamf_pro_version**  -  Read the Jamf Pro version

- `jamf-pro-cli jamf-pro-version`  -  Returns information about the Jamf Pro including the current version.

**local_admin_password**  -  Manage local admin password

- `jamf-pro-cli local-admin-password get-account-history`  -  Get the full history of all for a specific username on a target device.
- `jamf-pro-cli local-admin-password get-account-history-by-guid`  -  Get the full history of all for a specific user guid on a target device.
- `jamf-pro-cli local-admin-password get-accounts`  -  Get a full list of admin accounts that are LAPS capable.
- `jamf-pro-cli local-admin-password get-history`  -  Get the full history of all local admin passwords for all accounts for a specific management ID.
- `jamf-pro-cli local-admin-password list-pending-rotations`  -  Return information about all devices and usernames currently in the state of a pending LAPS rotation.
- `jamf-pro-cli local-admin-password list-settings`  -  Return information about the current LAPS settings.
- `jamf-pro-cli local-admin-password update-settings`  -  Update settings for LAPS. Required privilege: Update Local Admin Password Settings

**macos_configuration_profiles**  -  Manage macos configuration profiles

- `jamf-pro-cli macos-configuration-profiles get`  -  Finds OS X configuration profiles by ID
- `jamf-pro-cli macos-configuration-profiles get-subset`  -  Subset values can also be appended using an ampersand to return multiple subsets (e.g. /subsets/General&Scope)
- `jamf-pro-cli macos-configuration-profiles list`  -  Finds all OS X configuration profiles

**managed_software_updates**  -  Manage managed software updates

- `jamf-pro-cli managed-software-updates create-plans`  -  Creates a Managed Software Update Plan.
- `jamf-pro-cli managed-software-updates create-plans-group`  -  Creates Managed Software Update Plans for a Group.
- `jamf-pro-cli managed-software-updates get-plans`  -  Retrieves a Managed Software Update Plan.
- `jamf-pro-cli managed-software-updates get-plans-declarations`  -  Retrieves all Declarations associated with a Managed Software Update Plan.
- `jamf-pro-cli managed-software-updates get-plans-events`  -  Retrieves a Managed Software Update Plan Event Store.
- `jamf-pro-cli managed-software-updates get-plans-group`  -  Retrieves Managed Software Update Plans for a Group.
- `jamf-pro-cli managed-software-updates get-update-statuses-computer-groups`  -  Retrieve Managed Software Update Statuses for Computer Groups.
- `jamf-pro-cli managed-software-updates get-update-statuses-computers`  -  Retrieve Managed Software Update Statuses for Computers. Required privilege: Read Computers
- `jamf-pro-cli managed-software-updates get-update-statuses-mobile-device-groups`  -  Retrieve Managed Software Update Statuses for Mobile Device Groups.
- `jamf-pro-cli managed-software-updates get-update-statuses-mobile-devices`  -  Retrieve Managed Software Update Statuses for Mobile Devices. Required privilege: Read Mobile Devices
- `jamf-pro-cli managed-software-updates list-available-updates`  -  Retrieves available macOS and iOS Managed Software Updates
- `jamf-pro-cli managed-software-updates list-plans`  -  Retrieve Managed Software Update Plans.
- `jamf-pro-cli managed-software-updates list-plans-feature-toggle`  -  Retrieves current value of the Feature Toggle. Required privilege: Read Managed Software Updates
- `jamf-pro-cli managed-software-updates list-plans-feature-toggle-status`  -  Retrieves background status of the Feature Toggle. Required privilege: Read Managed Software Updates
- `jamf-pro-cli managed-software-updates list-update-statuses`  -  Retrieve Managed Software Update Statuses. Required privileges: > - Read Computers > - Read Mobile Devices
- `jamf-pro-cli managed-software-updates plans-feature-toggle-abandon`  -  'Break Glass' endpoint, not for nominal usage.
- `jamf-pro-cli managed-software-updates update-plans-feature-toggle`  -  Updates the value of the Feature Toggle - This endpoint is asynchronous

**mdm**  -  Manage mdm

- `jamf-pro-cli mdm blank-push`  -  Accepts a list of client management IDs and sends a blank push notification to each.
- `jamf-pro-cli mdm deploy-package`  -  Deploys packages to macOS devices using the InstallEnterpriseApplication MDM command.
- `jamf-pro-cli mdm list-commands`  -  Get information about mdm commands made by Jamf Pro. Required privilege: View MDM command information in Jamf Pro API
- `jamf-pro-cli mdm renew-profile`  -  Renews the device's MDM Profile, including the device identity certificate within the MDM Profile.
- `jamf-pro-cli mdm send-command`  -  Provided an MDM command type and appropriate information, will create and then queue said command.

**mobile_device_applications**  -  Manage mobile device applications

- `jamf-pro-cli mobile-device-applications get`  -  Finds mobile device applications by ID
- `jamf-pro-cli mobile-device-applications list`  -  Finds all mobile device applications

**mobile_device_configuration_profiles**  -  Manage mobile device configuration profiles

- `jamf-pro-cli mobile-device-configuration-profiles get`  -  Finds mobile device configuration profiles by ID
- `jamf-pro-cli mobile-device-configuration-profiles list`  -  Finds all mobile device configuration profiles

**mobile_device_extension_attributes**  -  Manage mobile device extension attributes

- `jamf-pro-cli mobile-device-extension-attributes add-history-note`  -  Add specified Mobile Device Extension Attribute history object notes.
- `jamf-pro-cli mobile-device-extension-attributes create`  -  Create Mobile Device Extension Attribute to collect extra inventory information.
- `jamf-pro-cli mobile-device-extension-attributes delete`  -  Deletes the Mobile Device Extension Attribute identified by the provided ID.
- `jamf-pro-cli mobile-device-extension-attributes get`  -  Gets specified Mobile Device Extension Attribute object. Required privilege: Read Mobile Device Extension Attributes
- `jamf-pro-cli mobile-device-extension-attributes get-data-dependency`  -  Get smart group dependent object for a specified mobile device extension attribute.
- `jamf-pro-cli mobile-device-extension-attributes get-history`  -  Get specified Mobile Device Extension Attribute history object.
- `jamf-pro-cli mobile-device-extension-attributes list`  -  Retrieves all mobile device extension attributes configuration.
- `jamf-pro-cli mobile-device-extension-attributes update`  -  Update specified Mobile Device Extension Attribute object. Required privilege: Update Mobile Device Extension Attributes

**mobile_device_groups**  -  Manage mobile device groups

- `jamf-pro-cli mobile-device-groups create-smart-groups`  -  Create a smart group with strict andOr enum validation. Required privilege: Create Smart Mobile Device Groups
- `jamf-pro-cli mobile-device-groups create-static-groups`  -  Create a static group. Required privilege: Create Static Mobile Device Groups
- `jamf-pro-cli mobile-device-groups delete-smart-groups`  -  Remove Smart Group by Id. Returns 422 if the group has dependencies.
- `jamf-pro-cli mobile-device-groups delete-static-groups`  -  Remove Static Group by Id. Returns 422 if the group has dependencies.
- `jamf-pro-cli mobile-device-groups get-smart-group-membership`  -  Get Smart Group Membership by Id. Required privileges: > - Read Smart Mobile Device Groups > - Read Mobile Devices
- `jamf-pro-cli mobile-device-groups get-smart-groups`  -  Get Smart Group by Id. Required privilege: Read Smart Mobile Device Groups
- `jamf-pro-cli mobile-device-groups get-static-group-membership`  -  Get Static Group Membership by Id. Required privileges: > - Read Static Mobile Device Groups > - Read Mobile Devices
- `jamf-pro-cli mobile-device-groups get-static-groups`  -  Get Static Group by Id. Required privilege: Read Static Mobile Device Groups
- `jamf-pro-cli mobile-device-groups list`  -  Returns the list of all mobile device groups filtered by site and permissions.
- `jamf-pro-cli mobile-device-groups list-smart-groups`  -  Get Smart Groups with pagination, sorting, and filtering support. Required privilege: Read Smart Mobile Device Groups
- `jamf-pro-cli mobile-device-groups list-static-groups`  -  Get Static Groups with pagination, sorting, and filtering support. Required privilege: Read Static Mobile Device Groups
- `jamf-pro-cli mobile-device-groups update-smart-groups`  -  Update a smart group with strict andOr enum validation. Required privilege: Update Smart Mobile Device Groups
- `jamf-pro-cli mobile-device-groups update-static-groups`  -  Partially update a static group. Only provided fields will be updated.

**mobile_device_prestages**  -  Manage mobile device prestages

- `jamf-pro-cli mobile-device-prestages add-history-note`  -  Adds mobile device prestage history object notes. Required privilege: Update Mobile Device PreStage Enrollments
- `jamf-pro-cli mobile-device-prestages create`  -  Create a mobile device prestage. Required privilege: Create Mobile Device PreStage Enrollments
- `jamf-pro-cli mobile-device-prestages create-scope`  -  Add device scope for a specific mobile device prestage. Required privilege: Update Mobile Device PreStage Enrollments
- `jamf-pro-cli mobile-device-prestages delete`  -  Deletes a Mobile Device Prestage with the supplied id. Required privilege: Delete Mobile Device PreStage Enrollments
- `jamf-pro-cli mobile-device-prestages get`  -  Retrieves a Mobile Device Prestage with the supplied id. Required privilege: Read Mobile Device PreStage Enrollments
- `jamf-pro-cli mobile-device-prestages get-history`  -  Gets sorted and paged mobile device prestage history objects.
- `jamf-pro-cli mobile-device-prestages get-scope`  -  Get device scope for a specific mobile device prestage. Required privilege: Read Mobile Device PreStage Enrollments
- `jamf-pro-cli mobile-device-prestages get-syncs`  -  Get all prestage sync states for a single prestage. Required privilege: Read Mobile Device PreStage Enrollments
- `jamf-pro-cli mobile-device-prestages get-syncs-latest`  -  Get the latest sync state for a single prestage. Required privilege: Read Mobile Device PreStage Enrollments
- `jamf-pro-cli mobile-device-prestages list`  -  Gets sorted and paged mobile device prestages. Required privilege: Read Mobile Device PreStage Enrollments
- `jamf-pro-cli mobile-device-prestages list-scope`  -  Get all device scope for all mobile device prestages. Required privilege: Read Mobile Device PreStage Enrollments
- `jamf-pro-cli mobile-device-prestages list-syncs`  -  Get all prestage sync states for all prestages. Required privilege: Read Mobile Device PreStage Enrollments
- `jamf-pro-cli mobile-device-prestages scope-delete-multiple`  -  Remove device scope for a specific mobile device prestage. Required privilege: Update Mobile Device PreStage Enrollments
- `jamf-pro-cli mobile-device-prestages update`  -  Updates a Mobile Device Prestage. Required privilege: Update Mobile Device PreStage Enrollments
- `jamf-pro-cli mobile-device-prestages update-scope`  -  Replace device scope for a specific mobile device prestage.

**mobile_devices**  -  Manage mobile devices

- `jamf-pro-cli mobile-devices erase`  -  Erase a Mobile Device. Required privilege: Send Mobile Device Remote Wipe Command
- `jamf-pro-cli mobile-devices get`  -  Get MobileDevice. Required privilege: Read Mobile Devices
- `jamf-pro-cli mobile-devices get-detail`  -  Get MobileDevice. Required privilege: Read Mobile Devices
- `jamf-pro-cli mobile-devices get-paired-devices`  -  Return paginated Mobile Device Inventory records of all paired devices for the device.
- `jamf-pro-cli mobile-devices list`  -  Gets Mobile Device objects. Required privilege: Read Mobile Devices
- `jamf-pro-cli mobile-devices list-detail`  -  Return paginated Mobile Device Inventory records. Required privilege: Read Mobile Devices
- `jamf-pro-cli mobile-devices unmanage`  -  Unmanage a Mobile Device. Required privilege: Unmanage Mobile Devices
- `jamf-pro-cli mobile-devices update`  -  Updates fields on a mobile device that are allowed to be modified by users. Required privilege: Update Mobile Devices

**network_segments**  -  Manage network segments

- `jamf-pro-cli network-segments get`  -  Finds network segments by ID
- `jamf-pro-cli network-segments list`  -  Finds all network segments

**packages**  -  Manage packages

- `jamf-pro-cli packages add-history-note`  -  Adds specified Package history object notes. Required privilege: Update Packages
- `jamf-pro-cli packages create`  -  Create package. Required privilege: Create Packages
- `jamf-pro-cli packages create-manifest`  -  Add a manifest to a package. Required privileges: > - Update Packages > - Read Packages
- `jamf-pro-cli packages delete`  -  Removes specified package. Required privilege: Delete Packages
- `jamf-pro-cli packages delete-manifest`  -  Delete the manifest file for a specified package. Required privileges: > - Update Packages > - Read Packages
- `jamf-pro-cli packages delete-multiple`  -  IDs of the packages to be deleted. Required privilege: Delete Packages
- `jamf-pro-cli packages get`  -  Gets specified Package object. Required privilege: Read Packages
- `jamf-pro-cli packages get-history`  -  Gets specified Package history object. Required privilege: Read Packages
- `jamf-pro-cli packages list`  -  Retrieves packages. Required privilege: Read Packages
- `jamf-pro-cli packages update`  -  Update specified package object. Required privilege: Update Packages
- `jamf-pro-cli packages upload`  -  Uploads a package. Required privileges: > - Update Packages > - Read Packages

**patch_management**  -  Manage patch management

- `jamf-pro-cli patch-management`  -  Accept Patch Management disclaimer. Required privilege: Update Patch Management Software Titles

**patch_policies**  -  Manage patch policies

- `jamf-pro-cli patch-policies create-dashboard`  -  Adds a patch policy to the dashboard. Required privilege: Read Patch Policies
- `jamf-pro-cli patch-policies delete-dashboard`  -  Removes a patch policy from the dashboard. Required privilege: Read Patch Policies
- `jamf-pro-cli patch-policies get-dashboard`  -  Returns whether or not the requested patch policy is on the dashboard. Required privilege: Read Patch Policies
- `jamf-pro-cli patch-policies list`  -  Retrieves a list of patch policies. Required privilege: Read Patch Policies
- `jamf-pro-cli patch-policies list-policy-details`  -  Retrieves a list of patch policies. Required privilege: Read Patch Policies

**patch_policy_logs**  -  Manage patch policy logs

- `jamf-pro-cli patch-policy-logs get-device`  -  Retrieves a single Patch Policy Log. Required privilege: Read Patch Policies
- `jamf-pro-cli patch-policy-logs get-device-details`  -  Return attempt details for a specific log. Required privilege: Read Patch Policies
- `jamf-pro-cli patch-policy-logs get-eligible-retry-count`  -  return the count of the patch policy logs for the patch policy id that are eligible for a retry attempt.
- `jamf-pro-cli patch-policy-logs list`  -  Retrieves Patch Policy Logs. Required privilege: Read Patch Policies
- `jamf-pro-cli patch-policy-logs retry`  -  Send retry attempts for specific devices. Required privilege: Update Patch Policies
- `jamf-pro-cli patch-policy-logs retry-all`  -  Send retry attempts for all devices. Required privilege: Update Patch Policies

**patch_software_title_configurations**  -  Manage patch software title configurations

- `jamf-pro-cli patch-software-title-configurations add-history-note`  -  Adds Patch Software Title Configuration history object notes.
- `jamf-pro-cli patch-software-title-configurations create`  -  Creates Patch Software Title Configurations using sToken. Required privilege: Create Patch Management Software Titles
- `jamf-pro-cli patch-software-title-configurations create-dashboard`  -  Adds a software title configuration to the dashboard. Required privilege: Read Patch Management Software Titles
- `jamf-pro-cli patch-software-title-configurations delete`  -  Deletes Patch Software Title Configurations with the supplied id.
- `jamf-pro-cli patch-software-title-configurations delete-dashboard`  -  Removes a software title configuration from the dashboard. Required privilege: Read Patch Management Software Titles
- `jamf-pro-cli patch-software-title-configurations get`  -  Retrieves Patch Software Title Configurations with the supplied id.
- `jamf-pro-cli patch-software-title-configurations get-dashboard`  -  Returns whether or not the requested software title configuration is on the dashboard.
- `jamf-pro-cli patch-software-title-configurations get-definitions`  -  Retrieves patch software title definitions with the supplied id.
- `jamf-pro-cli patch-software-title-configurations get-dependencies`  -  Retrieve list of Patch Software Title Configuration Dependencies.
- `jamf-pro-cli patch-software-title-configurations get-export-report`  -  Export Patch Reporting Data. Required privilege: Read Patch Management Software Titles
- `jamf-pro-cli patch-software-title-configurations get-extension-attributes`  -  Retrieves software title extension attributes with the supplied id.
- `jamf-pro-cli patch-software-title-configurations get-history`  -  Gets specified Patch Software Title Configuration history object.
- `jamf-pro-cli patch-software-title-configurations get-patch-report`  -  Retrieve Patch Software Title Configuration Patch Report. Required privilege: Read Patch Management Software Titles
- `jamf-pro-cli patch-software-title-configurations get-patch-summary`  -  Returns active patch summary. Required privilege: Read Patch Management Software Titles
- `jamf-pro-cli patch-software-title-configurations get-patch-summary-versions`  -  Returns patch versions. Required privilege: Read Patch Management Software Titles
- `jamf-pro-cli patch-software-title-configurations list`  -  Retrieves patch software title configurations. Required privilege: Read Patch Management Software Titles
- `jamf-pro-cli patch-software-title-configurations update`  -  Updates Patch Software Title Configurations. Required privilege: Update Patch Management Software Titles

**policies**  -  Manage policies

- `jamf-pro-cli policies create`  -  Creates a new policy from a Classic API XML body (root `<policy>`) passed with `--file` or `--stdin`; use `--id 0` so Jamf Pro assigns the ID.
- `jamf-pro-cli policies delete`  -  Deletes a policy by ID
- `jamf-pro-cli policies get`  -  Finds policies by ID
- `jamf-pro-cli policies get-subset`  -  Subset values can also be appended using an ampersand to return multiple subsets (e.g. /subsets/General&Scope)
- `jamf-pro-cli policies list`  -  Finds all policies
- `jamf-pro-cli policies list-by-category`  -  Category may be specified by id or name, or 'None' for policies with no category.
- `jamf-pro-cli policies list-by-created-by`  -  The value 'casper' refers to Casper Remote. The value 'jss' refers to policies created in the GUI or via the API.
- `jamf-pro-cli policies update`  -  Updates an existing policy by ID from a Classic API XML body (`--file` or `--stdin`)

**policies_preview**  -  Manage policies preview

- `jamf-pro-cli policies-preview list`  -  Gets `Policy Properties` object. Required privilege: Read Policies
- `jamf-pro-cli policies-preview update`  -  Update Policy Properties object. Required privilege: Update Policies

**printers**  -  Manage printers

- `jamf-pro-cli printers get`  -  Finds printers by ID
- `jamf-pro-cli printers list`  -  Finds all printers

**restricted_software**  -  Manage restricted software

- `jamf-pro-cli restricted-software get`  -  Finds restricted software by ID
- `jamf-pro-cli restricted-software list`  -  Finds all restricted software

**scripts**  -  Manage scripts

- `jamf-pro-cli scripts add-history-note`  -  Adds specified Script history object notes. Required privilege: Update Scripts
- `jamf-pro-cli scripts create`  -  Creates a script. Required privilege: Create Scripts
- `jamf-pro-cli scripts delete`  -  Deletes a script at the specified id. Required privilege: Delete Scripts
- `jamf-pro-cli scripts get`  -  Retrieves a full script object. Required privilege: Read Scripts
- `jamf-pro-cli scripts get-download`  -  Download a text file of the script contents. Required privilege: Read Scripts
- `jamf-pro-cli scripts get-history`  -  Gets specified Script history object. Required privilege: Read Scripts
- `jamf-pro-cli scripts list`  -  Search for sorted and paged scripts. Required privilege: Read Scripts
- `jamf-pro-cli scripts update`  -  Replaces the script at the id with the supplied information. Required privilege: Update Scripts

**sites**  -  Manage sites

- `jamf-pro-cli sites get-objects`  -  Find site objects for Site ID, with the ability to filter out different object types and object IDs for the site ID.
- `jamf-pro-cli sites list`  -  Find all sites. Required privilege: Read Sites

**smart_computer_groups_preview**  -  Manage smart computer groups preview

- `jamf-pro-cli smart-computer-groups-preview recalculate`  -  Recalculates the smart group for the given id and then returns the ids for the computers in the smart group.
- `jamf-pro-cli smart-computer-groups-preview recalculate-smart-groups`  -  Recalculates a smart group for the given id and then returns the count of smart groups the computer falls into.

**smart_mobile_device_groups_preview**  -  Manage smart mobile device groups preview

- `jamf-pro-cli smart-mobile-device-groups-preview recalculate`  -  recalculates a smart group for the given id and then returns the ids for the devices in the smart group.
- `jamf-pro-cli smart-mobile-device-groups-preview recalculate-smart-groups`  -  Recalculates all smart groups for the given device id and then returns the count of smart groups the device falls into.

**startup_status**  -  Manage startup status

- `jamf-pro-cli startup-status`  -  Retrieves information about application startup.

**volume_purchasing_locations**  -  Manage volume purchasing locations

- `jamf-pro-cli volume-purchasing-locations add-history-note`  -  Adds specified Volume Purchasing Location history object notes. Required privilege: Update Volume Purchasing Locations
- `jamf-pro-cli volume-purchasing-locations create`  -  Creates a Volume Purchasing Location using an sToken. Required privilege: Create Volume Purchasing Locations
- `jamf-pro-cli volume-purchasing-locations delete`  -  Deletes a Volume Purchasing Location with the supplied id. Required privilege: Delete Volume Purchasing Locations
- `jamf-pro-cli volume-purchasing-locations get`  -  Retrieves a Volume Purchasing Location with the supplied id. Required privilege: Read Volume Purchasing Locations
- `jamf-pro-cli volume-purchasing-locations get-content`  -  Retrieves the Volume Purchasing Content for the Volume Purchasing Location with the supplied id.
- `jamf-pro-cli volume-purchasing-locations get-history`  -  Gets specified Volume Purchasing Location history object. Required privilege: Read Volume Purchasing Locations
- `jamf-pro-cli volume-purchasing-locations list`  -  Retrieves Volume Purchasing Locations. Required privilege: Read Volume Purchasing Locations
- `jamf-pro-cli volume-purchasing-locations reclaim`  -  Reclaims a Volume Purchasing Location with the supplied id. Required privilege: Update Volume Purchasing Locations
- `jamf-pro-cli volume-purchasing-locations revoke-licenses`  -  Revokes licenses for a Volume Purchasing Location with the supplied id.
- `jamf-pro-cli volume-purchasing-locations update`  -  Updates a Volume Purchasing Location. Required privilege: Update Volume Purchasing Locations

**volume_purchasing_subscriptions**  -  Manage volume purchasing subscriptions

- `jamf-pro-cli volume-purchasing-subscriptions add-history-note`  -  Adds Volume Purchasing Subscription history object notes. Required privilege: Update Volume Purchasing Locations
- `jamf-pro-cli volume-purchasing-subscriptions create`  -  Creates a Volume Purchasing Subscription. Required privilege: Create Volume Purchasing Locations
- `jamf-pro-cli volume-purchasing-subscriptions delete`  -  Deletes a Volume Purchasing Subscription with the supplied id. Required privilege: Delete Volume Purchasing Locations
- `jamf-pro-cli volume-purchasing-subscriptions get`  -  Retrieves a Volume Purchasing Subscription with the supplied id. Required privilege: Read Volume Purchasing Locations
- `jamf-pro-cli volume-purchasing-subscriptions get-history`  -  Gets specified Volume Purchasing Subscription history object. Required privilege: Read Volume Purchasing Locations
- `jamf-pro-cli volume-purchasing-subscriptions list`  -  Retrieves Volume Purchasing Subscriptions. Required privilege: Read Volume Purchasing Locations
- `jamf-pro-cli volume-purchasing-subscriptions update`  -  Updates a Volume Purchasing Subscription. Required privilege: Update Volume Purchasing Locations

**webhooks**  -  Manage webhooks

- `jamf-pro-cli webhooks get`  -  Finds webhooks by ID
- `jamf-pro-cli webhooks list`  -  Finds all webhooks


**fleet**


- `jamf-pro-cli fleet find`  -  Find which client's Jamf Pro instance has a Mac, iPad or iPhone, by serial, name, username or email, across every registered instance at once.

- `jamf-pro-cli fleet posture`  -  Per-client counts of stale check-ins, Macs below a macOS floor, FileVault off, firewall off and expiring MDM profiles, with Macs that inventory cannot fully judge reported as incomplete.

- `jamf-pro-cli fleet patch`  -  Patch titles behind latest and failing patch-policy installs (one patch_title row per client and title), plus failed managed OS updates (one os_updates row per client).

- `jamf-pro-cli fleet policy-hygiene`  -  Flags policies that are disabled, can never run, have no scope, are expired, uncategorized, or point at a missing script or package.

- `jamf-pro-cli fleet health`  -  ADE and VPP token expiry, failed enrollment syncs, health check and Jamf Pro version spread for every client.

- `jamf-pro-cli fleet exec`  -  Run any read-only Jamf Pro or Classic API GET on every client instance and get one merged table with an instance column; credential fields are redacted and credential endpoints refused.

- `jamf-pro-cli fleet app`  -  Which clients and computers have an installed application below a version floor.

- `jamf-pro-cli fleet baseline`  -  Compare every client instance against a golden instance: scripts and extension attributes by content; packages, policies, profiles and smart groups by name.


**instance**


- `jamf-pro-cli instance`  -  Register each client's Jamf Pro instance once (server and API client id; its secret goes in JAMF_PRO_FLEET_SECRETS), then target it with --instance or report on all of them with the fleet commands.




### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
jamf-pro-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Stale Macs at one client

```bash
jamf-pro-cli computer-inventory list --instance acme --filter 'general.lastContact<2026-09-01T00:00:00Z' --all --agent --select id,general.name,general.lastContact
```

RSQL filter plus --all reads every page; --select keeps the agent's context small.

### Patch titles behind latest, every client

```bash
jamf-pro-cli fleet patch --behind-only --agent
```

Rows of kind patch_title for every client and title that is not fully patched or has failed patch-policy installs, plus each client's os_updates row with failed managed OS updates.

### Broken policies across clients

```bash
jamf-pro-cli fleet policy-hygiene --agent
```

Disabled, unscoped, never-runnable or broken-reference policies, tagged by instance.

### Any read across all clients

```bash
jamf-pro-cli fleet exec /api/v3/computer-groups/smart-groups --agent
```

Merged smart-group list with an instance column.

## Auth Setup

Each Jamf Pro instance has its own API client (Settings > System > API roles and clients). The CLI exchanges the client ID and secret for a short-lived Bearer token at https://<server>/api/v1/oauth/token (form-encoded client_id and client_secret), and that token works for both the Jamf Pro API and the Classic API. For one instance, set JAMF_PRO_SERVER, JAMF_PRO_CLIENT_ID and JAMF_PRO_CLIENT_SECRET (an env-sourced secret is never written to disk), or run 'auth login --client-id <id> --client-secret <secret>', which stores the secret in the CLI's credentials file; 'auth status' shows what is in use. For many clients, run 'instance add <name> --server <host> --client-id <id>' once per client and put each client secret in JAMF_PRO_FLEET_SECRETS as name=secret entries separated by commas; the registry never stores secrets, and 'instance test' proves every registration.

Run `jamf-pro-cli doctor` to verify a single-instance setup (env vars), or `jamf-pro-cli instance test` when client instances are registered. The `fleet` commands read only instances registered with `jamf-pro-cli instance add`; with none registered they report that nothing was checked (`instances_in_scope: 0`), never an empty all-clear.

Credentials go only to the configured Jamf Pro server. Any output, file or request that repeats one, in any encoding, is replaced by a `credkit: withheld` notice: that is the program working, not an error, so narrow the query instead of retrying. Password, key, token and PIN fields print as `[redacted]`.

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
  jamf-pro-cli account-groups list --agent --select accessLevel,directoryGroupId,id
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

- Use `--home <dir>` for one invocation (not with `--instance`, which already gives each client its own home), or set `JAMF_PRO_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `JAMF_PRO_CONFIG_DIR`, `JAMF_PRO_DATA_DIR`, `JAMF_PRO_STATE_DIR`, `JAMF_PRO_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `JAMF_PRO_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `jamf-pro-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "jamf-pro": {
        "command": "jamf-pro-mcp",
        "env": {
          "JAMF_PRO_HOME": "/srv/jamf-pro"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `JAMF_PRO_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `JAMF_PRO_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are not cached on disk, and credential-named fields are redacted before a row reaches the store. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `JAMF_PRO_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `JAMF_PRO_DATA_SOURCE` sets the `--data-source` default.

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
jamf-pro-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "jamf-pro-cli learnings confirm 12"] }
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
       materially more, record the divergence via `jamf-pro-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `jamf-pro-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Acme" → the acme and acme-eu instances). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `jamf-pro-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
jamf-pro-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
jamf-pro-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
jamf-pro-cli teach-playbook \
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
jamf-pro-cli playbook amend \
  --query "$QUERY" \
  --add-note "$NOTE"
# (append shell `&` to background it)
```

What counts as worth amending: a behavior you OBSERVED this session that future-you would benefit from knowing. Examples worth amending:

- A workaround for a CLI surface that silently drops or misorders a flag.
- An undocumented endpoint shape (response wrapped in `{meta, results}`, payload nested two levels deeper than the docs claim).
- Observed schema drift (a field renamed, an index that shifted between Jamf Pro versions, a category label that the API now returns lower-cased).

What does NOT belong in notes:

- The year-specific or entity-specific answer to the user's question. That's the response, not a learning.
- Per-client / per-device / per-row data the playbook already retrieves at runtime.
- Statements that paraphrase what the existing notes already say.

The amend command appends to the family's existing notes with a timestamped marker (`[amend YYYY-MM-DDTHH:MMZ]: <text>`). Multiple amends accumulate; the audit trail is visible. If no playbook exists yet for the family, amend creates a notes-only one (so cold-start corrections still land).

#### PII discipline for amend notes

`playbook amend` notes are designed to potentially flow upstream as shared knowledge in future versions of the Printing Press. Keep them clean of user-identifying content so the upstream-contribution path stays open without retroactive scrubbing:

- **Do NOT embed** paths to user filesystems, personal API keys or tokens, user email addresses, user GitHub handles, or specific query histories tied to a single user.
- **Acceptable**: endpoint shapes, undocumented field names, API gotchas, observed schema drift, workarounds for CLI surfaces, generalizable pagination or retry tactics.

If a correction is only meaningful with user-specific context, it belongs in a personal note, not in the playbook amend.

### Measuring the loop

`jamf-pro-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `JAMF_PRO_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
jamf-pro-cli feedback "the --since flag is inclusive but docs say exclusive"
jamf-pro-cli feedback --stdin < notes.txt
jamf-pro-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `JAMF_PRO_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `JAMF_PRO_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
jamf-pro-cli profile save briefing --json
jamf-pro-cli --profile briefing account-groups list
jamf-pro-cli profile list --json
jamf-pro-cli profile show briefing
jamf-pro-cli profile delete briefing --yes
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
| 8 | Fleet problems found (`fleet health --fail-on`) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `jamf-pro-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add jamf-pro-mcp --env JAMF_PRO_SERVER=<server> --env JAMF_PRO_CLIENT_ID=<client-id> --env JAMF_PRO_CLIENT_SECRET=<client-secret> -- jamf-pro-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `jamf-pro-mcp --transport http` requires `JAMF_PRO_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `JAMF_PRO_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `JAMF_PRO_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which jamf-pro-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   jamf-pro-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `jamf-pro-cli <command> --help`.
