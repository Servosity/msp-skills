---
name: zoom-phone
description: "Every Zoom Phone admin operation plus E911, routing and footprint audits, across every client account. Trigger phrases: `audit zoom phone e911`, `what does this zoom phone user have`, `which call queues is this user in`, `where does this zoom phone number ring`, `zoom phone missed calls callback list`, `offboard a zoom phone user`, `zoom phone unassigned numbers`, `offline zoom desk phones`, `broken zoom phone call routing`, `run a zoom phone audit across all client accounts`, `use zoom-phone`, `run zoom-phone`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Zoom"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - zoom-phone-cli
    install:
      - kind: script
        bins: [zoom-phone-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zoom-phone/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zoom-phone/install.ps1
---

# Zoom Phone - Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `zoom-phone-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zoom-phone/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zoom-phone/install.ps1 | iex
   ```
3. Verify: `zoom-phone-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `zoom-phone-cli` and `zoom-phone-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Covers the whole Zoom Phone admin API as commands and MCP tools, with a local SQLite store for offline search. On top it joins answers the API spreads across many endpoints: e911 audit for emergency address gaps, footprint for everything one user holds, routing trace for where a number rings, and callbacks for missed callers nobody called back. MSPs save one profile per client account with accounts connect and run any read-only audit across all of them with fleet.

## When to Use This CLI

Use this CLI for Zoom Phone administration: looking up users, numbers, call queues, auto receptionists, devices and emergency addresses; auditing E911, routing and device health; following up missed calls; and running the same audit across many client Zoom accounts. It is the right tool when an agent needs joined answers (what a user holds, where a number rings) rather than one raw endpoint.

## Anti-triggers

Do not use this CLI for:
- Zoom Meetings, webinars, chat or Zoom Rooms scheduling (use a Zoom Meetings tool)
- Placing or answering live phone calls (the API administers the PBX; it does not carry audio)
- Billing or license purchases in the Zoom web portal
- Accounts without a Zoom Phone license (every Phone endpoint returns 403)

## Unique Capabilities

### Account health audits
- **`e911 audit`**  -  From the locally synced store: users, common areas and sites whose emergency address is missing or unverified, plus BYOC numbers needing a carrier update.

  _Reach for this before any E911 compliance review; run sync first (the BYOC check also needs numbers synced with --resource-param numbers:type=byoc), and treat assessed:false as 'no data', not 'clean'._

  ```bash
  zoom-phone-cli e911 audit --agent
  ```
- **`numbers inventory`**  -  From the locally synced store: phone numbers by site, source and assignee, with unassigned, pending and inactive-assignee lists.

  _Use it to find a free number per site or numbers still assigned to departed users; run sync first._

  ```bash
  zoom-phone-cli numbers inventory --agent
  ```
- **`devices health`**  -  From the locally synced store: offline or unassigned desk phones, firmware drift per model, devices assigned to inactive users, grouped by site.

  _Use it in a weekly health sweep to spot dead or outdated phones; run sync first (devices sync as the assigned and unassigned resources)._

  ```bash
  zoom-phone-cli devices health --agent
  ```
- **`routing audit`**  -  Dead IVR targets, empty call queues, orphaned auto receptionists and auto receptionists with no upcoming holiday hours.

  _Run it after routing changes to catch calls that ring nowhere; it reads routing live._

  ```bash
  zoom-phone-cli routing audit --agent
  ```

### User lifecycle
- **`footprint`**  -  Everything one phone user holds: extension, numbers, calling plans, call queue membership, shared lines, pickup and monitoring groups, desk phones and emergency address.

  _Use it to answer 'what does this person have?' or 'which queues is this user in?' in one call; base data comes from the synced store and group membership is read live._

  ```bash
  zoom-phone-cli footprint jane@example.com --agent
  ```
- **`offboard`**  -  Ordered removal plan for a departing user across queues, shared lines, groups, devices, numbers and calling plan; --apply executes it.

  _Use it to plan an offboarding without missing a queue or shared line; re-sync and review the plan before --apply, which sends the DELETE calls immediately. Families it cannot remove are listed under not_covered._

  ```bash
  zoom-phone-cli offboard jane@example.com --agent
  ```
- **`routing trace`**  -  Follow one inbound number or extension through auto receptionists, IVR keys and call queues to where it rings, for business and closed hours.

  _Use it for 'where does this number ring?' or 'where do calls go after hours?' tickets; it reads routing live, so no sync is needed._

  ```bash
  zoom-phone-cli routing trace +14155550123 --agent
  ```

### Call follow-up
- **`callbacks`**  -  External callers whose calls were missed or abandoned and who have not been called back yet.

  _Use it for the daily or weekly callback list instead of exporting call history to a spreadsheet; it reads call history live, so no sync is needed._

  ```bash
  zoom-phone-cli callbacks --since 7d --agent
  ```

### MSP fleet
- **`fleet`**  -  Run one read-only command (or sync) across every saved client account and get one merged result with an account column; writes need --allow-writes.

  _Use it when an MSP needs the same audit across all client Zoom accounts; sync each account first with fleet -- sync --full --resources ..._

  ```bash
  zoom-phone-cli fleet -- e911 audit
  ```
- **`accounts connect`**  -  Save one profile per client Zoom account (its own credentials, token cache and local store) so you can switch tenants without re-entering credentials.

  _Use it once per client account; then select it with eval "$(zoom-phone-cli accounts path acme --env)" or run across accounts with fleet. Do not point ZOOM_PHONE_CONFIG at the file alone._

  ```bash
  printf '%s' "$ACME_SECRET" | zoom-phone-cli accounts connect acme --account-id abc123XYZ --client-id cid123 --client-secret-stdin
  ```

## Command Reference

**account_settings**  -  Manage account settings

- `zoom-phone-cli account-settings`  -  Returns an account's Zoom Phone settings.

**alert_settings**  -  Manage alert settings

- `zoom-phone-cli alert-settings add-an`  -  Adds an alert setting.
- `zoom-phone-cli alert-settings delete-an`  -  Deletes an alert setting.
- `zoom-phone-cli alert-settings get-details`  -  Returns detailed information about a specific alert setting.
- `zoom-phone-cli alert-settings list-with-paging-query`  -  Gets alert settings for an account with paging query.
- `zoom-phone-cli alert-settings update-an`  -  Updates information of an Alert setting.

**audios**  -  Manage audios

- `zoom-phone-cli audios delete-item`  -  Deletes an audio item. Only the admin or user can delete your audio.
- `zoom-phone-cli audios get-item`  -  Returns an audio item. Only the admin or user can access your audio.
- `zoom-phone-cli audios update-item`  -  Updates an audio item. Only the admin or user can update your audio.

**auto_receptionists**  -  Manage auto receptionists

- `zoom-phone-cli auto-receptionists add`  -  Adds an auto receptionist to a Zoom Phone.
- `zoom-phone-cli auto-receptionists add-policy`  -  Adds a policy subsetting for a specific auto receptionist according to the `policyType`.
- `zoom-phone-cli auto-receptionists add-setting`  -  Adds the auto receptionist setting according to the setting type.
- `zoom-phone-cli auto-receptionists assign-phone-numbers`  -  Assigns available phone numbers to an auto receptionist.
- `zoom-phone-cli auto-receptionists delete`  -  Deletes a non-primary auto receptionist.
- `zoom-phone-cli auto-receptionists delete-policy`  -  Removes the policy subsetting for a specific auto receptionist according to the `policyType`.
- `zoom-phone-cli auto-receptionists delete-setting`  -  Delete the auto receptionist setting according to the setting type.
- `zoom-phone-cli auto-receptionists get-call-handling-settings`  -  Returns the call handling setting of an auto receptionist for the specified hour type.
- `zoom-phone-cli auto-receptionists get-detail`  -  Returns information on a specific auto receptionist.
- `zoom-phone-cli auto-receptionists get-ivr`  -  Returns an interactive voice response (IVR) system of the specified auto receptionist.
- `zoom-phone-cli auto-receptionists get-policy`  -  Returns the policy setting of a specific auto receptionist.
- `zoom-phone-cli auto-receptionists get-settings`  -  Returns auto receptionist settings. Prerequisites: * A Pro or higher account with a Zoom Phone license.
- `zoom-phone-cli auto-receptionists list`  -  Returns a list of auto receptionists. Prerequisites: * Pro or a higher account with Zoom Phone license.
- `zoom-phone-cli auto-receptionists unassign-all-phone-nums`  -  Unassigns all phone numbers that were previously assigned to an auto receptionist.
- `zoom-phone-cli auto-receptionists unassign-aphone-num`  -  Unassigns a specific phone number that was previously assigned to an auto receptionist.
- `zoom-phone-cli auto-receptionists update`  -  Changes information such as the display name and the extension number assigned to the main auto receptionist.
- `zoom-phone-cli auto-receptionists update-call-handling-settings`  -  Update the call handling setting of an auto receptionist for the specified hour type.
- `zoom-phone-cli auto-receptionists update-ivr`  -  Updates the interactive voice response (IVR) system of the specified auto receptionist.
- `zoom-phone-cli auto-receptionists update-policy`  -  Updates the policy setting of a specific auto receptionist.
- `zoom-phone-cli auto-receptionists update-policy-phone`  -  Updates the policy subsetting of a specific auto receptionist according to the `policyType`.
- `zoom-phone-cli auto-receptionists update-setting`  -  Updates the auto receptionist setting according to the setting type.

**batch_locations**  -  Manage batch locations

- `zoom-phone-cli batch-locations`  -  Adds emergency service locations in batch.

**billing_accounts**  -  Manage billing accounts

- `zoom-phone-cli billing-accounts get-abilling-account`  -  A Zoom account owner or a user with admin privilege can use this API to get information about a billing account.
- `zoom-phone-cli billing-accounts list`  -  Retrieves a list of available billing accounts for a Zoom account owner or a user with admin privileges.

**blocked_list**  -  Manage blocked list

- `zoom-phone-cli blocked-list add-anumber-to`  -  Creates a block list and add a number to the list.
- `zoom-phone-cli blocked-list delete-ablocked-list`  -  Deletes a blocked list, which removes the associated number from the blocked list.
- `zoom-phone-cli blocked-list get-ablocked-list`  -  Returns the information about a specific blocked list.
- `zoom-phone-cli blocked-list list`  -  Returns all the blocked lists in an acccount.
- `zoom-phone-cli blocked-list update`  -  Updates the information in the blocked list.

**byoc_numbers**  -  Manage byoc numbers

- `zoom-phone-cli byoc-numbers`  -  Adds BYOC (Bring Your Own Carrier) phone numbers to Zoom Phone.

**call_element**  -  Manage call element

- `zoom-phone-cli call-element get`  -  Returns the call element for a given call element ID.
- `zoom-phone-cli call-element get-phone-recording-by-id`  -  Returns an account's call recording by the call element's `callElementId`.
- `zoom-phone-cli call-element get-voicemail-details-by-id`  -  Returns the detailed information on a voicemail by the call element's `callElementId`.

**call_history**  -  Manage call history

- `zoom-phone-cli call-history account`  -  Returns an account's new edition of call logs.
- `zoom-phone-cli call-history add-client-code-to`  -  Adds a client code to a call log. You can track call logs with a client code.
- `zoom-phone-cli call-history get-call-path`  -  Returns information about a call log.
- `zoom-phone-cli call-history get-detail`  -  Returns the call history details for a given call log ID call history.

**call_logs**  -  Manage call logs

- `zoom-phone-cli call-logs account`  -  Returns an account's call logs.
- `zoom-phone-cli call-logs add-client-code-to`  -  Adds a client code to a call log. You can track call logs with a client code.
- `zoom-phone-cli call-logs get-details`  -  Returns information about a call log.
- `zoom-phone-cli call-logs get-phone-recordings-by-call-id-or-id`  -  Returns an account's call recording by the recording's `callId` or `callLogId`.

**call_queue_analytics**  -  Manage call queue analytics

- `zoom-phone-cli call-queue-analytics`  -  Returns the call queue analytics overview.

**call_queues**  -  Manage call queues

- `zoom-phone-cli call-queues add-cqpolicy-sub-setting`  -  Adds the policy subsetting for a specific call queue according to the `policyType`.
- `zoom-phone-cli call-queues add-members-to`  -  Adds phone users or common areas as members to a specific call queue. Prerequisites: * Pro or higher account plan.
- `zoom-phone-cli call-queues add-setting`  -  Adds a call queue setting according to the setting type.
- `zoom-phone-cli call-queues addmemberstoacustomgroup`  -  Adds call queue members to a custom group.
- `zoom-phone-cli call-queues assign-phone-to`  -  Assigns numbers to a call queue.
- `zoom-phone-cli call-queues create`  -  Creates a call queue. Call queues allow you to route incoming calls to a group of users.
- `zoom-phone-cli call-queues createacustomgroupinacallqueue`  -  Scopes: `phone:write:admin` Granular Scopes: `phone:write:call_queue_custom_group:admin` Rate Limit Label: `LIGHT`
- `zoom-phone-cli call-queues delete-acall-queue`  -  Call queues allow you to route incoming calls to a group of users.
- `zoom-phone-cli call-queues delete-setting`  -  Delete the call queue setting by the setting type. Prerequisites * A Pro or higher account with a Zoom Phone license.
- `zoom-phone-cli call-queues deleteacustomgroup`  -  Delete a custom group.
- `zoom-phone-cli call-queues get-acall-queue`  -  Routes incoming calls to a group of users and returns information on a specific call queue.
- `zoom-phone-cli call-queues get-call-handling-setting`  -  Returns the call handling setting of a call queue for the specified hour type.
- `zoom-phone-cli call-queues get-policy`  -  Returns the policy setting of a specific [call queue].
- `zoom-phone-cli call-queues get-recordings`  -  Use this API to view call recordings from the call queue.
- `zoom-phone-cli call-queues get-settings`  -  Returns call queue settings. Prerequisites: * Pro or higher account with Zoom Phone license.
- `zoom-phone-cli call-queues getacustomgroup`  -  Scopes: `phone:read:admin`,`phone:master` Granular Scopes: `phone:read:call_queue_custom_group:admin`,`phone:read
- `zoom-phone-cli call-queues list`  -  Call queues allow you to route incoming calls to a group of users.
- `zoom-phone-cli call-queues list-members`  -  Returns a list of call queue members.
- `zoom-phone-cli call-queues listcustomgroupsofacallqueue`  -  Returns a list of custom groups for a call queue.
- `zoom-phone-cli call-queues remove-cqpolicy-sub-setting`  -  Use this API to remove the policy sub-setting for a specific call queue according to the `policyType`.
- `zoom-phone-cli call-queues removeamemberfromacustomgroup`  -  Scopes: `phone:write:admin` Granular Scopes: `phone:delete:call_queue_custom_group_member:admin` Rate Limit Label
- `zoom-phone-cli call-queues un-assign-phone-num`  -  Unbinds a phone number from a call queue.
- `zoom-phone-cli call-queues unassign-all-members`  -  Removes all members from a call queue who were previously assigned to that call queue.
- `zoom-phone-cli call-queues unassign-aphone-num`  -  Unbinds all phone numbers that are assigned to a call queue.
- `zoom-phone-cli call-queues unassign-member-from`  -  Removes a member who was previously added to a call queue. The member could be a phone user or common area.
- `zoom-phone-cli call-queues update`  -  Updates information of a specific call queue. Call queues allow you to route incoming calls to a group of users.
- `zoom-phone-cli call-queues update-call-handling-setting`  -  Updates the call handling setting of a call queue for the specified hour type.
- `zoom-phone-cli call-queues update-cqpolicy-sub-setting`  -  Updates the policy subsetting for a specific call queue according to the `policyType`.
- `zoom-phone-cli call-queues update-policy`  -  Updates a call queue's policy settings.
- `zoom-phone-cli call-queues update-setting`  -  Update the call queue setting according to the setting type.
- `zoom-phone-cli call-queues updateacustomgroup`  -  Update a custom group.

**calling_plans**  -  Manage calling plans

- `zoom-phone-cli calling-plans`  -  Returns all of an account's Zoom Phone calling plans.

**carrier_peering**  -  Manage carrier peering

- `zoom-phone-cli carrier-peering`  -  Returns phone numbers that the carrier pushed to different customers.

**carrier_reseller**  -  Manage carrier reseller

- `zoom-phone-cli carrier-reseller active-crphone-numbers`  -  Changes the phone number status to `active` in a carrier reseller's master account.
- `zoom-phone-cli carrier-reseller create-crphone-numbers`  -  Adds phone numbers to a carrier reseller (master) account. Up to 200 numbers at a time.
- `zoom-phone-cli carrier-reseller delete-crphone-number`  -  Deletes or unassigns a phone number from a carrier reseller account.
- `zoom-phone-cli carrier-reseller list-crphone-numbers`  -  Lists phone numbers in a carrier reseller (master) account that can be pushed to its subaccounts.

**common_areas**  -  Manage common areas

- `zoom-phone-cli common-areas add`  -  Adds an instance of a common area. You can configure devices shared by users and deployed in shared spaces.
- `zoom-phone-cli common-areas add-outbound-calling-exception-rule`  -  Adds the common area level outbound calling policy exception rule for the country region.
- `zoom-phone-cli common-areas add-setting`  -  Adds the common area setting according to the setting type.
- `zoom-phone-cli common-areas apply-templateto`  -  Applies a template to common areas. You can add up to 50 common areas at a time.
- `zoom-phone-cli common-areas assign-calling-plans-to`  -  Assigns calling plans to a common area.
- `zoom-phone-cli common-areas assign-phone-numbers-to`  -  Assigns phone numbers to a common area.
- `zoom-phone-cli common-areas delete`  -  Removes the common area. Prerequisites: * Pro or a higher account with Zoom Phone license.
- `zoom-phone-cli common-areas delete-outbound-calling-exception-rule`  -  Deletes the common area level outbound calling exception rule.
- `zoom-phone-cli common-areas delete-setting`  -  Removes the common area subsetting from desk phones.
- `zoom-phone-cli common-areas generateactivationcodesforcommonareas`  -  Generates activation codes for common areas. You can add up to 50 common areas at a time.
- `zoom-phone-cli common-areas get-acommon-area`  -  Returns detailed information on the common area.
- `zoom-phone-cli common-areas get-call-handling-setting`  -  Returns the call handling setting of a common area for the specified hour type.
- `zoom-phone-cli common-areas get-outbound-calling-countries-and-regions`  -  Returns the common area level outbound calling countries and regions.
- `zoom-phone-cli common-areas get-settings`  -  Returns common area settings. Prerequisites: * Pro or a higher account with Zoom Phone license.
- `zoom-phone-cli common-areas list`  -  Lists common areas under an account.
- `zoom-phone-cli common-areas list-activation-codes`  -  Returns a list of activation code information of the common areas under an account.
- `zoom-phone-cli common-areas list-outbound-calling-exception-rule`  -  Lists the common area level outbound calling policy exception rules.
- `zoom-phone-cli common-areas unassign-calling-plans-from`  -  Use this API to unassign a calling plan from the common area.
- `zoom-phone-cli common-areas unassign-phone-numbers-from`  -  Unassigns a phone number from a common area.
- `zoom-phone-cli common-areas update`  -  Updates the common area information.
- `zoom-phone-cli common-areas update-call-handling-setting`  -  Update the call handling setting of a common area for the specified hour type.
- `zoom-phone-cli common-areas update-outbound-calling-countries-or-regions`  -  Updates the common area level outbound calling policy country or region.
- `zoom-phone-cli common-areas update-outbound-calling-exception-rule`  -  Updates the common area level outbound calling policy for the country region exception rule.
- `zoom-phone-cli common-areas update-pin-code`  -  Updates the common area pin code.
- `zoom-phone-cli common-areas update-setting`  -  Updates the common area setting according to the setting type.

**devices**  -  Manage devices

- `zoom-phone-cli devices add-extensions-to-adevice`  -  By default, all Zoom Phone users can make and receive calls using the Zoom desktop and mobile applications.
- `zoom-phone-cli devices add-phone`  -  Adds an unassigned desk phone or an assigned (to a user or a common area) desk phone.
- `zoom-phone-cli devices batch-update-line-key-setting`  -  Use this API to batch update the Zoom Phone device line key position information.
- `zoom-phone-cli devices delete-adevice`  -  Remove a desk phone device or ATA (Analog Telephone Adapter) from the Zoom Phone System Management.
- `zoom-phone-cli devices delete-extension-from-adevice`  -  Use this API to unassign a specific assignee from the device.
- `zoom-phone-cli devices get-adevice`  -  Returns detailed information about a specific desk phone device.
- `zoom-phone-cli devices list-line-key-setting`  -  Use this API to get information on the Zoom Phone device line keys settings and position.
- `zoom-phone-cli devices list-phone`  -  Lists all the desk phone devices that are configured with Zoom Phone on an account.
- `zoom-phone-cli devices reboot-phone`  -  Use this API to reboot an online zero-touch or assisted-provisioning device. You can only send one request every second.
- `zoom-phone-cli devices sync-phone`  -  Use this API to resync all online zero-touch or assisted-provisioning devices in an account or a site.
- `zoom-phone-cli devices update-adevice`  -  Updates the information of a desk phone device.
- `zoom-phone-cli devices update-provision-template-to`  -  Use this API to assign a provision template to a device or remove a provision template from the device by leaving

**dial_by_name_directory**  -  Manage dial by name directory

- `zoom-phone-cli dial-by-name-directory add-users-to-directory`  -  Use this API to add users to a directory.
- `zoom-phone-cli dial-by-name-directory delete-users-from-directory`  -  Use this API to delete users from the directory.
- `zoom-phone-cli dial-by-name-directory list-users-from-directory`  -  Use this API to get users that are in or not in a directory.

**emergency_addresses**  -  Manage emergency addresses

- `zoom-phone-cli emergency-addresses add-emergency-address`  -  Adds an emergency address.
- `zoom-phone-cli emergency-addresses delete-emergency-address`  -  Removes an emergency address.
- `zoom-phone-cli emergency-addresses get-emergency-address`  -  Gets the emergency address information.
- `zoom-phone-cli emergency-addresses list`  -  Lists the emergency addresses.
- `zoom-phone-cli emergency-addresses update-emergency-address`  -  Updates emergency address information.

**emergency_number_pools**  -  Manage emergency number pools

- `zoom-phone-cli emergency-number-pools assign-phone-number-to`  -  Assigns phone numbers to the Emergency Number Pool.
- `zoom-phone-cli emergency-number-pools list-account-numbers`  -  Returns phone numbers in the account-level Emergency Number Pool.
- `zoom-phone-cli emergency-number-pools unassign-phone-number-from`  -  Unassigns phone numbers from the Emergency Number Pool.

**emergency_service_settings**  -  Manage emergency service settings

- `zoom-phone-cli emergency-service-settings get`  -  Returns the complete emergency service configuration in a single response
- `zoom-phone-cli emergency-service-settings update`  -  Updates one emergency service setting group per call.

**emergency_services**  -  Manage emergency services

- `zoom-phone-cli emergency-services add-account-emergency-routing-rule`  -  Adds an emergency routing rule for an account.
- `zoom-phone-cli emergency-services delete-account-emergency-routing-rule`  -  Removes an account's emergency routing rule.
- `zoom-phone-cli emergency-services list-account-emergency-routing-rules`  -  Lists all of an account's emergency routing rules (both Zoom default and customized) in a single response.
- `zoom-phone-cli emergency-services update-account-emergency-routing-rule`  -  Updates an account's emergency routing rule.

**extension**  -  Manage extension

- `zoom-phone-cli extension add-call-handling`  -  Adds Zoom Phone call handling subsettings for your phone system.
- `zoom-phone-cli extension add-extensiont-level-inbound-block-rules`  -  Adds the given extension's block rule for inbound calls and messaging.
- `zoom-phone-cli extension batch-update-line-key-setting`  -  Use this API to batch update the Zoom Phone line key settings information.
- `zoom-phone-cli extension delete-call-handling`  -  Deletes a Zoom Phone call handling settings.
- `zoom-phone-cli extension delete-extensiont-level-inbound-block-rules`  -  Deletes the given extension's blocked rule for inbound calls and messaging.
- `zoom-phone-cli extension delete-line-key`  -  Use this API to delete the Zoom Phone line key settings information.
- `zoom-phone-cli extension get-call-handling`  -  Returns information about a Zoom Phone call handling settings.
- `zoom-phone-cli extension getuser-sfaxlogs`  -  Returns the extension's fax logs. Prerequisites * User must belong to a Business or Enterprise account.
- `zoom-phone-cli extension list-level-inbound-block-rules`  -  Returns a list of the given extension's block rule for inbound calls and messaging.
- `zoom-phone-cli extension list-line-key-setting`  -  Use this API to get the Zoom Phone line key settings information.
- `zoom-phone-cli extension update-call-handling`  -  Updates a Zoom Phone call handling setting.

**external_contacts**  -  Manage external contacts

- `zoom-phone-cli external-contacts add`  -  Adds an external contact.
- `zoom-phone-cli external-contacts delete-aexternal-contact`  -  Removes an external contact.
- `zoom-phone-cli external-contacts get-aexternal-contact`  -  Returns an external contact's information.
- `zoom-phone-cli external-contacts list`  -  Lists the external contacts.
- `zoom-phone-cli external-contacts update`  -  Updates an external contact's information.

**fax**  -  Manage fax

- `zoom-phone-cli fax delete-log`  -  Deletes a specific fax log. Prerequisites * User must belong to a Business or Enterprise account.
- `zoom-phone-cli fax download-cover-preview-image`  -  Downloads the preview image for a specific fax cover page.
- `zoom-phone-cli fax downloadfaxfile`  -  Downloads the fax file. Prerequisites* * User must belong to a Business or Enterprise account.
- `zoom-phone-cli fax get-account-s-logs`  -  Returns the account's fax logs. Prerequisites * User must belong to a Business or Enterprise account.
- `zoom-phone-cli fax get-log-details`  -  Returns the fax log's details. Prerequisites * User must belong to a Business or Enterprise account.
- `zoom-phone-cli fax list-covers`  -  Returns a list of fax cover pages for the account. Prerequisites * User must belong to a Business or Enterprise account.
- `zoom-phone-cli fax send-efax`  -  Sends a fax to receivers. Prerequisites * User must belong to a Business or Enterprise account.
- `zoom-phone-cli fax update-log-read-status`  -  Updates the read status of a specific fax log. Prerequisites * User must belong to a Business or Enterprise account.
- `zoom-phone-cli fax upload-files`  -  Uploads fax file Note: * Please use `https://fileapi.zoom.us` instead of `https://api.zoom.

**firmware_update_rules**  -  Manage firmware update rules

- `zoom-phone-cli firmware-update-rules add-firmware-rule`  -  Use this API to add a firmware update rule.
- `zoom-phone-cli firmware-update-rules delete`  -  Use this API to delete the firmware update rule.
- `zoom-phone-cli firmware-update-rules get-firmware-rule-detail`  -  Use this API to get the firmware update rule information.
- `zoom-phone-cli firmware-update-rules list-firmware-rules`  -  Use this API to get firmware update rules.
- `zoom-phone-cli firmware-update-rules update-firmware-rule`  -  Use this API to update a specific firmware update rule.

**firmwares**  -  Manage firmwares

- `zoom-phone-cli firmwares`  -  Use this API to get updatable firmwares.

**group_call_pickup**  -  Manage group call pickup

- `zoom-phone-cli group-call-pickup add-gcp`  -  Use this API to add a Group Call Pickup instance to the account that has the Zoom Phone license assigned.
- `zoom-phone-cli group-call-pickup add-gcpmembers`  -  Use this API to add members to the specified Group Call Pickup object.
- `zoom-phone-cli group-call-pickup delete-gcp`  -  Use this API to remove a Group Call Pickup object.
- `zoom-phone-cli group-call-pickup get-gcp`  -  Use this API to retrieve information on a specific Group Call Pickup object in an account.
- `zoom-phone-cli group-call-pickup list-gcp`  -  Use this API to retrieve a list of Group Call Pickup objects in an account.
- `zoom-phone-cli group-call-pickup list-gcpmembers`  -  Use this API to retrieve members of a call pickup group in an account.
- `zoom-phone-cli group-call-pickup remove-gcpmembers`  -  Use this API to remove member from the Group Call Pickup object.
- `zoom-phone-cli group-call-pickup update-gcp`  -  Use this API to update a specific Group Call Pickup object information.

**groups**  -  Manage groups

- `zoom-phone-cli groups get-phone-settings`  -  Returns group phone settings. Prerequisites Account must have a Pro or a higher plan with Zoom Phone license.
- `zoom-phone-cli groups get-policy-details`  -  Returns the group policy details.
- `zoom-phone-cli groups update-policy`  -  Updates the group policy.

**inbound_blocked**  -  Manage inbound blocked

- `zoom-phone-cli inbound-blocked add-account-level-inbound-block-rules`  -  Adds an account level block rule for inbound calls and messaging from a phone number.
- `zoom-phone-cli inbound-blocked delete-account-level-inbound-block-rules`  -  Deletes the account level blocked rule for inbound calls and messaging.
- `zoom-phone-cli inbound-blocked delete-account-level-statistics`  -  Deletes the statistic of extensions blocked rule for inbound calls and messaging. (e.g.
- `zoom-phone-cli inbound-blocked list-account-level-inbound-block-rules`  -  Returns a list of account level inbound block rule for inbound calls and messaging.
- `zoom-phone-cli inbound-blocked list-account-level-statistics`  -  Returns the list of the statistics of the extensions blocked rule for inbound calls and messaging. (e.g.
- `zoom-phone-cli inbound-blocked mark-phone-number-as-blocked-for-all-extensions`  -  Promotes and applies the phone number blocked from the extension level blocked rule to the account level blocked rule.
- `zoom-phone-cli inbound-blocked update-account-level-inbound-block-rule`  -  Updates the account level block rule for inbound calls and messaging.

**locations**  -  Manage locations

- `zoom-phone-cli locations add`  -  Adds an emergency service location.
- `zoom-phone-cli locations delete`  -  Removes an emergency location.
- `zoom-phone-cli locations get`  -  Returns an emergency service location's information.
- `zoom-phone-cli locations list`  -  Returns emergency service locations. Note: When you enable multiple sites, the `site_id` parameter is required.
- `zoom-phone-cli locations update`  -  Updates an emergency location's information.

**metrics**  -  Manage metrics

- `zoom-phone-cli metrics get-call-log-details`  -  Returns call log details of a specific call.
- `zoom-phone-cli metrics get-call-qo-s`  -  Gets the call quality of service (QoS) data for a call made or received by a Zoom phone user in the account.
- `zoom-phone-cli metrics list-call-logs`  -  Returns monthly call logs metrics.
- `zoom-phone-cli metrics list-past-call`  -  Returns all the call logs metrics of the account from the selected time period.
- `zoom-phone-cli metrics list-phone-realtimelocation`  -  Returns currently detected location of IP phones.
- `zoom-phone-cli metrics list-tracked-locations`  -  Lists the tracked locations.
- `zoom-phone-cli metrics list-user-default-emergency-address`  -  Returns the users who provide a default personal emergency address rather than a default site address or default
- `zoom-phone-cli metrics list-user-detectable-personal-location`  -  Returns the users who created one or more detectable personal locations with associated network data.
- `zoom-phone-cli metrics list-user-location-sharing-permission`  -  Returns users that have enabled location services and allowed location sharing with Zoom applications on client devices.
- `zoom-phone-cli metrics list-user-nomadic-emergency-services`  -  Returns users who have been enabled for nomadic emergency services.
- `zoom-phone-cli metrics list-user-realtime-location`  -  Returns the user's currently detected location. (Excludes users who didn't enable location permission on the client.

**monitoring_groups**  -  Manage monitoring groups

- `zoom-phone-cli monitoring-groups add-members`  -  Use this API to add members to a Monitoring Group.
- `zoom-phone-cli monitoring-groups create`  -  Creates a monitoring group.
- `zoom-phone-cli monitoring-groups delete`  -  Use this API to delete a Monitoring Group.
- `zoom-phone-cli monitoring-groups get-by-id`  -  Returns a monitoring group for the specified ID.
- `zoom-phone-cli monitoring-groups list`  -  Returns an account's monitoring group list.
- `zoom-phone-cli monitoring-groups list-members`  -  Use this API to return members list of a Monitoring Group.
- `zoom-phone-cli monitoring-groups remove-member`  -  Use this API to remove a member from a Monitoring Group.
- `zoom-phone-cli monitoring-groups remove-members`  -  Use this API to remove all monitor or monitored members from a Monitoring Group.
- `zoom-phone-cli monitoring-groups update`  -  Use this API to update a Monitoring Group.

**numbers**  -  Manage numbers

- `zoom-phone-cli numbers delete-unassigned-phone`  -  Deletes unassigned phone numbers. Up to 20 phone numbers can be removed in a single request.
- `zoom-phone-cli numbers get-phone-details`  -  Returns information about an account's Zoom Phone number.
- `zoom-phone-cli numbers list-account-phone`  -  Returns a list all Zoom Phone numbers in a Zoom account.
- `zoom-phone-cli numbers update-phone-details`  -  Updates a Zoom Phone number's information.
- `zoom-phone-cli numbers update-site-for-unassigned-phone`  -  Updates a site's unassigned phone numbers. Up to 20 phone numbers can be updated in a single request.

**outbound_caller_id**  -  Manage outbound caller id

- `zoom-phone-cli outbound-caller-id add-outbound-caller-numbers`  -  Adds the `account-level` customized outbound caller ID phone numbers.
- `zoom-phone-cli outbound-caller-id delete-outbound-caller-numbers`  -  Deletes the `account-level` customized outbound caller ID phone numbers.
- `zoom-phone-cli outbound-caller-id list-customize-outbound-caller-numbers`  -  Retrieves phone numbers that can be used as the `account-level` customized outbound caller ID.

**outbound_calling**  -  Manage outbound calling

- `zoom-phone-cli outbound-calling add-account-exception-rule`  -  Adds the account level outbound calling policy exception rule for country region.
- `zoom-phone-cli outbound-calling delete-account-exception-rule`  -  Deletes the account level outbound calling exception rule.
- `zoom-phone-cli outbound-calling get-account-countries-and-regions`  -  Returns the account level outbound calling countries and regions.
- `zoom-phone-cli outbound-calling list-account-exception-rule`  -  Lists the account level outbound calling policy exception rules.
- `zoom-phone-cli outbound-calling update-account-countries-or-regions`  -  Updates the account level outbound calling policy country or region.
- `zoom-phone-cli outbound-calling update-account-exception-rule`  -  Updates the account level outbound calling policy for the country region exception rule.

**peering**  -  Manage peering

- `zoom-phone-cli peering add-phone-numbers`  -  Adds phone numbers to Zoom through the Provider Exchange.
- `zoom-phone-cli peering delete-phone-numbers`  -  Removes phone numbers added to Zoom through the provider exchange.
- `zoom-phone-cli peering list-phone-numbers`  -  Returns phone numbers to Zoom through the Provider Exchange.
- `zoom-phone-cli peering update-phone-numbers`  -  Updates phone numbers to Zoom through the Provider Exchange.

**plans**  -  Manage plans

- `zoom-phone-cli plans`  -  Returns all of an account's Zoom Phone plan package, phone number usage, and availability.

**policies**  -  Manage policies

- `zoom-phone-cli policies get-account-policy-details`  -  Returns the account policy details.
- `zoom-phone-cli policies update-account-policy`  -  Updates the account's Zoom Phone policy.

**ported_numbers**  -  Manage ported numbers

- `zoom-phone-cli ported-numbers get-details`  -  Returns details on the ported numbers by specifying `order_id`.
- `zoom-phone-cli ported-numbers list`  -  Use this API to list ported numbers in a Zoom account.

**private_directory**  -  Manage private directory

- `zoom-phone-cli private-directory add-members-to-aprivate-directory`  -  Adds members to a private directory.
- `zoom-phone-cli private-directory list-members`  -  Returns the member list of a private directory.
- `zoom-phone-cli private-directory remove-amember-from-aprivate-directory`  -  Removes a member from a private directory.
- `zoom-phone-cli private-directory update-aprivate-directory-member`  -  Updates a member of a private directory.

**provision_templates**  -  Manage provision templates

- `zoom-phone-cli provision-templates add`  -  Use this API to create a provision template in a Zoom account.
- `zoom-phone-cli provision-templates delete`  -  Use this API to delete a provision template in a Zoom account.
- `zoom-phone-cli provision-templates get`  -  Use this API to get a specific provision template.
- `zoom-phone-cli provision-templates list-account`  -  Use this API to list all provision templates in a Zoom account.
- `zoom-phone-cli provision-templates update`  -  Use this API to update a provision template in a Zoom account.

**push_to_talk**  -  Manage push to talk

- `zoom-phone-cli push-to-talk add-members-to-apush-to-talk-channel`  -  Adds members to a push to talk (PTT) channel.
- `zoom-phone-cli push-to-talk create-apush-to-talk-channel`  -  Creates a push to talk (PTT) channel.
- `zoom-phone-cli push-to-talk get-channel-details`  -  Returns push to talk (PTT) channel details.
- `zoom-phone-cli push-to-talk list-channel-members`  -  Returns a list of push to talk (PTT) members.
- `zoom-phone-cli push-to-talk list-channels`  -  Returns a list of push to talk (PTT) channels.
- `zoom-phone-cli push-to-talk remove-amember-from-apush-to-talk-channel`  -  Removes a member from a push to talk (PTT) channel.
- `zoom-phone-cli push-to-talk remove-apush-to-talk-channel`  -  Removes a push to talk (PTT) channel.
- `zoom-phone-cli push-to-talk update-channel-details`  -  Updates the push to talk (PTT) channel details.

**recording_transcript**  -  Manage recording transcript

- `zoom-phone-cli recording-transcript <recordingId>`  -  Downloads the phone recording transcript.

**recordings**  -  Manage recordings

- `zoom-phone-cli recordings delete-call`  -  Deletes a call recording.
- `zoom-phone-cli recordings get-phone`  -  Returns an account's call recordings.
- `zoom-phone-cli recordings phone-download-file`  -  Downloads the phone recording.
- `zoom-phone-cli recordings update-auto-delete-field`  -  Updates the auto delete field for recording.
- `zoom-phone-cli recordings update-status`  -  Updates the status of a single recording in account.

**reports**  -  Manage reports

- `zoom-phone-cli reports get-call-charges-usage`  -  Retrieves the Phone Call Charges Report.
- `zoom-phone-cli reports get-psoperation-logs`  -  Retrieves the phone system operation logs report.
- `zoom-phone-cli reports get-smscharges-usage`  -  Retrieves the SMS/MMS Charges Report.
- `zoom-phone-cli reports getfaxchargesusagereport`  -  Retrieves the Phone Fax Charges Report.

**roles**  -  Manage roles

- `zoom-phone-cli roles add-members`  -  Use this API to add members to roles.
- `zoom-phone-cli roles add-phone-targets`  -  Adds targets to phone roles.
- `zoom-phone-cli roles del-members`  -  Use this API to delete member(s) in a role.
- `zoom-phone-cli roles delete-phone`  -  Use this API to delete a phone role.
- `zoom-phone-cli roles delete-phone-targets`  -  Deletes the targets of a phone role.
- `zoom-phone-cli roles duplicate-phone`  -  Use this API to duplicate a phone role.
- `zoom-phone-cli roles get-information`  -  Use this API to get information on a phone role.
- `zoom-phone-cli roles list-members`  -  Use this API to get members (not) in a role.
- `zoom-phone-cli roles list-phone`  -  Returns the phone roles.
- `zoom-phone-cli roles list-phone-targets`  -  Targets of a phone role.
- `zoom-phone-cli roles update-phone`  -  Use this API to update a role.

**rooms**  -  Manage rooms

- `zoom-phone-cli rooms add-zoom`  -  Associates a Zoom Room with a Zoom Phone license.
- `zoom-phone-cli rooms assign-calling-plan-to`  -  Assigns calling plans to a Zoom Room with a maximum of 200 numbers at a time.
- `zoom-phone-cli rooms assign-phone-number-to-zoom`  -  Assigns phone numbers to a Zoom Room. Up to 200 numbers at a time.
- `zoom-phone-cli rooms get-zoom`  -  Specifies Zoom Room in an account with an assigned Zoom Phone license.
- `zoom-phone-cli rooms list-unassigned-zoom`  -  Returns Zoom Rooms that are not assigned a Zoom Phone.
- `zoom-phone-cli rooms list-zoom`  -  Retrieves a list of Zoom Rooms under the account that has the Zoom Phone license assigned.
- `zoom-phone-cli rooms remove-zoom`  -  Use this API to remove Zoom Room from a Zoom Phone account.
- `zoom-phone-cli rooms unassign-calling-plan-from`  -  Use this API to unassign a calling plan from a Zoom Room.
- `zoom-phone-cli rooms unassign-phone-number-from-zoom`  -  Use this API to unassign a phone number from a Zoom Room.
- `zoom-phone-cli rooms update-zoom`  -  Use this API to update a Zoom Room in an account that has the Zoom Phone license assigned.

**routing_rules**  -  Manage routing rules

- `zoom-phone-cli routing-rules add`  -  Creates a directory backup routing rule.
- `zoom-phone-cli routing-rules delete`  -  Deletes the directory backup routing rule.
- `zoom-phone-cli routing-rules get`  -  Returns the directory backup routing rule.
- `zoom-phone-cli routing-rules list`  -  Returns a list of directory backup routing rules.
- `zoom-phone-cli routing-rules update`  -  Updates the directory backup routing rule.

**setting_templates**  -  Manage setting templates

- `zoom-phone-cli setting-templates add`  -  Creates a Zoom Phone setting template for an account.
- `zoom-phone-cli setting-templates get`  -  Returns information about an account's phone template.
- `zoom-phone-cli setting-templates list`  -  Gets a list of all the created phone template settings.
- `zoom-phone-cli setting-templates update`  -  Updates or modifies a phone template's settings.

**settings**  -  Manage settings

- `zoom-phone-cli settings phone`  -  Returns an account's settings.
- `zoom-phone-cli settings update-phone`  -  Updates Zoom Phone account settings.

**shared_line_appearances**  -  Manage shared line appearances

- `zoom-phone-cli shared-line-appearances`  -  Returns a list of shared line appearance instances.

**shared_line_groups**  -  Manage shared line groups

- `zoom-phone-cli shared-line-groups add-members-to`  -  Adds members to a shared line group.
- `zoom-phone-cli shared-line-groups add-setting`  -  Adds the shared line group setting according to the setting type.
- `zoom-phone-cli shared-line-groups add-slgpolicy-sub-setting`  -  Adds the policy sub-setting for a specific shared line group according to the `policyType`.
- `zoom-phone-cli shared-line-groups assign-phone-numbers-slg`  -  Assigns phone numbers to a shared line groups.
- `zoom-phone-cli shared-line-groups create-ashared-line-group`  -  Creates a shared line group.
- `zoom-phone-cli shared-line-groups delete-amember-slg`  -  Unassigns a specific member from a shared line group.
- `zoom-phone-cli shared-line-groups delete-aphone-number-slg`  -  Unassigns a specific phone number that was assigned to the shared line group.
- `zoom-phone-cli shared-line-groups delete-ashared-line-group`  -  Deletes a shared line group.
- `zoom-phone-cli shared-line-groups delete-members-of-slg`  -  Unassigns all existing members from a shared line group.
- `zoom-phone-cli shared-line-groups delete-phone-numbers-slg`  -  Unassigns all the phone numbers that have been assigned to the shared line group.
- `zoom-phone-cli shared-line-groups delete-setting`  -  Deletes the shared line group setting according to the setting type.
- `zoom-phone-cli shared-line-groups get-ashared-line-group`  -  Returns a list of all the shared line groups.
- `zoom-phone-cli shared-line-groups get-call-handling-setting`  -  Returns the call handling setting of a shared line group for the specified hour type.
- `zoom-phone-cli shared-line-groups get-policy`  -  Returns the policy setting of a specific shared line group.
- `zoom-phone-cli shared-line-groups get-settings`  -  Returns shared line group settings. Prerequisites: * A Pro or higher account with a Zoom Phone license.
- `zoom-phone-cli shared-line-groups list`  -  Lists all the shared line groups.
- `zoom-phone-cli shared-line-groups remove-slgpolicy-sub-setting`  -  Removes the policy sub-setting for a specific shared line group according to the `policyType`.
- `zoom-phone-cli shared-line-groups update-ashared-line-group`  -  Updates the information about a shared line group.
- `zoom-phone-cli shared-line-groups update-call-handling-setting`  -  Update the call handling setting of a shared line group for the specified hour type.
- `zoom-phone-cli shared-line-groups update-policy`  -  Updates the policy setting of a specific shared line group.
- `zoom-phone-cli shared-line-groups update-setting`  -  Update the shared line group setting according to the setting type.
- `zoom-phone-cli shared-line-groups update-slgpolicy-sub-setting`  -  Updates the policy sub-setting for a specific shared line group according to the `policyType`.

**sip_groups**  -  Manage sip groups

- `zoom-phone-cli sip-groups`  -  Returns a list of SIP (Session Initiation Protocol) groups.

**sip_trunk**  -  Manage sip trunk

- `zoom-phone-cli sip-trunk`  -  Returns a list of an account's assigned BYOC (Bring Your Own Carrier) SIP (Session Initiation Protocol) trunks.

**sites**  -  Manage sites

- `zoom-phone-cli sites add-emergency-routing-rule`  -  Add a site's emergency routing rule.
- `zoom-phone-cli sites add-outbound-caller-numbers`  -  Use this API to add the `site-level` customized outbound caller ID phone numbers. * Multiple sites must be enabled.
- `zoom-phone-cli sites add-outbound-calling-exception-rule`  -  Adds the site level outbound calling policy exception rule for the country region.
- `zoom-phone-cli sites add-setting`  -  Sites allow you to organize Zoom Phone users in your organization.
- `zoom-phone-cli sites add-users-to-directory-by`  -  Use this API to add users to a directory of the specified site.
- `zoom-phone-cli sites create-phone`  -  Creates a site that allows you to organize the Zoom Phone users in your organization.
- `zoom-phone-cli sites delete-emergency-routing-rule`  -  Removes a site's emergency routing rule.
- `zoom-phone-cli sites delete-outbound-caller-numbers`  -  Use this API to remove the `site-level` customized outbound caller ID phone numbers. * Multiple sites must be enabled.
- `zoom-phone-cli sites delete-outbound-calling-exception-rule`  -  Deletes the site level outbound calling exception rule.
- `zoom-phone-cli sites delete-phone`  -  Use this API to delete a specific site in a Zoom account.
- `zoom-phone-cli sites delete-setting`  -  Sites allow you to organize Zoom Phone users in your organization.
- `zoom-phone-cli sites delete-users-from-directory-by`  -  Use this API to delete users from a directory of the specified site.
- `zoom-phone-cli sites get-asite`  -  Returns information on a specific site. Sites allow you to organize Zoom Phone users in your organization.
- `zoom-phone-cli sites get-outbound-calling-countries-and-regions`  -  Returns the site level outbound calling countries and regions.
- `zoom-phone-cli sites get-setting-for-type`  -  Gets the site setting about a specific site according to the setting type.
- `zoom-phone-cli sites list-customize-outbound-caller-numbers`  -  Use this API to retrieve phone numbers that can be used as the `site-level` customized outbound caller ID.
- `zoom-phone-cli sites list-emergency-number-pool-numbers`  -  Returns phone numbers in the site-level Emergency Number Pool.
- `zoom-phone-cli sites list-emergency-routing-rules`  -  Lists all of a site's emergency routing rules (both Zoom default and customized) in a single response.
- `zoom-phone-cli sites list-outbound-calling-exception-rule`  -  Returns a list of site level outbound calling policy exception rules.
- `zoom-phone-cli sites list-phone`  -  Sites allow you to organize Zoom Phone users in your organization.
- `zoom-phone-cli sites list-users-from-directory-by`  -  Use this API to get users that are in or not in a directory of the specified site.
- `zoom-phone-cli sites update-details`  -  Updates information about a specific site. It allows you to organize Zoom Phone users in your organization.
- `zoom-phone-cli sites update-emergency-routing-rule`  -  Updates a site's emergency routing rule. Prerequisites * Pro or higher account with Zoom Phone license.
- `zoom-phone-cli sites update-outbound-calling-countries-or-regions`  -  Updates the site level outbound calling policy country or region.
- `zoom-phone-cli sites update-outbound-calling-exception-rule`  -  Updates the site level outbound calling policy for the country region exception rule.
- `zoom-phone-cli sites update-setting`  -  Allows you to organize Zoom Phone users in your organization.

**smartphones**  -  Manage smartphones

- `zoom-phone-cli smartphones`  -  Returns a list of all the smartphones configured with Zoom Phone on an account.

**sms**  -  Manage sms

- `zoom-phone-cli sms account-session`  -  Returns details about SMS sessions for an account.
- `zoom-phone-cli sms by-message-id`  -  Returns details about a specific message in an SMS session.
- `zoom-phone-cli sms post-message`  -  Sends SMS messages to Zoom Phone accounts.
- `zoom-phone-cli sms session-details`  -  Returns details about an SMS session.
- `zoom-phone-cli sms session-sync`  -  Syncs SMS messages in a session. Service Endpoint SMS * This API now supports syncing a Service Endpoint SMS session.

**sms_campaigns**  -  Manage sms campaigns

- `zoom-phone-cli sms-campaigns assign-campaign-phone-numbers`  -  Assigns a phone number to the SMS campaign.
- `zoom-phone-cli sms-campaigns get`  -  Returns a specific SMS campaign.
- `zoom-phone-cli sms-campaigns get-number-campaign-opt-status`  -  Returns the opt statuses of phone numbers that are assigned to SMS the campaign.
- `zoom-phone-cli sms-campaigns list-account`  -  Returns a list of all SMS campaigns in a Zoom account.
- `zoom-phone-cli sms-campaigns unassign-campaign-phone-number`  -  Unassigns a phone number from the SMS campaign. Remove the assigned phone number from the campaign.
- `zoom-phone-cli sms-campaigns update-number-campaign-opt-status`  -  Updates opt statuses of phone numbers that are assigned to the SMS campaign.

**sms_consent**  -  Manage sms consent

- `zoom-phone-cli sms-consent <consentId>`  -  Returns the opt statuses of phone numbers that are assigned to SMS consent.

**users**  -  Manage users

- `zoom-phone-cli users add-an-audio`  -  Adds an audio item for text-to-speech conversion.
- `zoom-phone-cli users add-audio-item`  -  Adds the audio items. You can only upload voice files at this time.
- `zoom-phone-cli users add-call-forward-setting`  -  Adds Zoom Phone call forwarding numbers for a user under a specific hour type.
- `zoom-phone-cli users add-outbound-caller-numbers`  -  Adds users' customized outbound caller ID phone numbers.
- `zoom-phone-cli users add-outbound-calling-exception-rule`  -  Adds an user level outbound calling policy exception rule for the country region.
- `zoom-phone-cli users add-profile-setting`  -  Adds the user setting according to the setting type. Prerequisites * Pro or a higher account with Zoom Phone license.
- `zoom-phone-cli users add-setting`  -  Adds the user setting according to the setting type, specifically for delegation
- `zoom-phone-cli users assign-calling-plan`  -  Assigns a calling plan to a Zoom Phone user. For user-level apps, pass the `me` value instead of the `userId` parameter.
- `zoom-phone-cli users assign-phone-number`  -  Assigns a phone number to a user who has already enabled Zoom Phone.
- `zoom-phone-cli users batch-add`  -  Adds phone users in batch. You can add up to 10 users at a time.
- `zoom-phone-cli users delete-call-forward-setting`  -  Delete Zoom Phone call forwarding numbers for a user under a specific hour type.
- `zoom-phone-cli users delete-call-history`  -  Deletes a user's call history. For user-level apps, pass the `me` value instead of the `userId` parameter.
- `zoom-phone-cli users delete-call-log`  -  Deletes a user's call log. For user-level apps, pass the `me` value instead of the `userId` parameter.
- `zoom-phone-cli users delete-outbound-caller-numbers`  -  Removes the users' customized outbound caller ID phone numbers.
- `zoom-phone-cli users delete-outbound-calling-exception-rule`  -  Deletes the user level outbound calling exception rule.
- `zoom-phone-cli users delete-profile-setting`  -  Deletes a user setting based on the setting type. Prerequisites * Pro or higher account with Zoom Phone license.
- `zoom-phone-cli users delete-setting`  -  Removes the user setting according to the setting type, specifically for delegation
- `zoom-phone-cli users get-aicall-summary`  -  Returns AI call summary details. For user-level apps, pass the `me` value instead of the `userId` parameter.
- `zoom-phone-cli users get-call-handling-setting`  -  Returns the call handling setting of a user for the specified hour type.
- `zoom-phone-cli users get-number-campaign-opt-status`  -  Returns the opt statuses of a user’s phone numbers.
- `zoom-phone-cli users get-outbound-calling-countries-and-regions`  -  Returns the user level outbound calling countries and regions.
- `zoom-phone-cli users get-policy-details`  -  Returns the user policy details.
- `zoom-phone-cli users get-sms-sessions`  -  Retrieves the user's SMS sessions in descending order. Mirrors the ZP client behavior with the most recent on top.
- `zoom-phone-cli users get-voicemail-details-by-call-id-or-call-log-id`  -  Returns the detailed information on a voicemail associated with a call log ID.
- `zoom-phone-cli users list-audio-items`  -  Returns personal audios.
- `zoom-phone-cli users list-customize-outbound-caller-numbers`  -  Retrieves phone numbers that can be the `user-level` customized outbound caller ID.
- `zoom-phone-cli users list-outbound-calling-exception-rule`  -  Returns a list of the user level outbound calling policy exception rules.
- `zoom-phone-cli users list-phone`  -  Returns a list of all of an account's users who are assigned a Zoom Phone license.
- `zoom-phone-cli users phone`  -  Returns a user's Zoom phone profile. For user-level apps, pass the `me` value instead of the `userId` parameter.
- `zoom-phone-cli users phone-call-history`  -  Returns a user's Zoom phone call history. For user-level apps, pass the `me` value instead of the `userId` parameter.
- `zoom-phone-cli users phone-call-logs`  -  Returns a user's Zoom phone call logs. For user-level apps, pass the `me` value instead of the `userId` parameter.
- `zoom-phone-cli users phone-recordings`  -  Gets a user's Zoom Phone recordings. For user-level apps, pass the `me` value instead of the `userId` parameter.
- `zoom-phone-cli users phone-settings`  -  Returns the Zoom Phone profile settings of a user.
- `zoom-phone-cli users phone-voice-mails`  -  Returns a user's Zoom Phone voicemails. For user-level apps, pass the `me` value instead of the `userId` parameter.
- `zoom-phone-cli users sms-session`  -  Returns details about SMS sessions for a user.
- `zoom-phone-cli users sync-call-history`  -  Syncs a user's Zoom phone call history. For user-level apps, pass the `me` value instead of the `userId` parameter.
- `zoom-phone-cli users sync-call-logs`  -  Syncs a user's Zoom phone call logs. For user-level apps, pass the `me` value instead of the `userId` parameter.
- `zoom-phone-cli users unassign-calling-plan`  -  Unassigns a Zoom Phone user's calling plan. For user-level apps, pass the `me` value instead of the `userId` parameter.
- `zoom-phone-cli users unassign-phone-number`  -  Unassigns Zoom Phone user's phone number.
- `zoom-phone-cli users update-call-handling-setting`  -  Updates a user's call handling settings for the specified hour type.
- `zoom-phone-cli users update-calling-plan`  -  Switches the calling plans of a Zoom Phone user.
- `zoom-phone-cli users update-outbound-calling-countries-or-regions`  -  Updates the user level outbound calling policy country or region.
- `zoom-phone-cli users update-outbound-calling-exception-rule`  -  Updates the user level outbound calling policy for the country region exception rule.
- `zoom-phone-cli users update-policy`  -  Updates a user's Zoom Phone policy.
- `zoom-phone-cli users update-profile`  -  Updates a user's Zoom Phone profile. For user-level apps, pass the `me` value instead of the `userId` parameter.
- `zoom-phone-cli users update-properties-in-batch`  -  Updates multiple users' properties in batch.
- `zoom-phone-cli users update-setting`  -  Updates the user setting according to the setting type, specifically for delegation
- `zoom-phone-cli users update-settings`  -  Updates the Zoom Phone profile settings of a user.
- `zoom-phone-cli users whatsapp-session`  -  Returns details about WhatsApp sessions for a user.

**voice_mails**  -  Manage voice mails

- `zoom-phone-cli voice-mails account`  -  Gets a user's Zoom Phone voicemails.
- `zoom-phone-cli voice-mails delete`  -  Deletes an account's voicemail message.
- `zoom-phone-cli voice-mails get-details`  -  Returns the information about a voicemail message.
- `zoom-phone-cli voice-mails phone-download-file`  -  Downloads the Zoom phone voicemail.
- `zoom-phone-cli voice-mails update-read-status`  -  Updates the status of voicemail message.

**whatsapp**  -  Manage whatsapp

- `zoom-phone-cli whatsapp by-message-id`  -  Returns details about a specific message in a WhatsApp session.
- `zoom-phone-cli whatsapp list-whats-app-sessions`  -  Returns details about WhatsApp sessions.
- `zoom-phone-cli whatsapp post-whats-app-message`  -  Sends a WhatsApp message through a Zoom Phone WhatsApp Business Account (WABA).
- `zoom-phone-cli whatsapp session-details`  -  Returns details about a WhatsApp session.


**accounts**


- `zoom-phone-cli accounts connect`  -  Save one profile per client Zoom account (its own credentials, token cache and local store) so you can switch tenants without re-entering credentials.


**callbacks**


- `zoom-phone-cli callbacks`  -  External callers whose calls were missed or abandoned and who have not been called back yet.


**devices**


- `zoom-phone-cli devices health`  -  From the locally synced store: offline or unassigned desk phones, firmware drift per model, devices assigned to inactive users, grouped by site.


**e911**


- `zoom-phone-cli e911 audit`  -  From the locally synced store: users, common areas and sites whose emergency address is missing or unverified, plus BYOC numbers needing a carrier update.


**fleet**


- `zoom-phone-cli fleet`  -  Run one read-only command (or sync) across every saved client account and get one merged result with an account column; writes need --allow-writes.


**footprint**


- `zoom-phone-cli footprint`  -  Everything one phone user holds: extension, numbers, calling plans, call queue membership, shared lines, pickup and monitoring groups, desk phones and emergency address.


**numbers**


- `zoom-phone-cli numbers inventory`  -  From the locally synced store: phone numbers by site, source and assignee, with unassigned, pending and inactive-assignee lists.


**offboard**


- `zoom-phone-cli offboard`  -  Ordered removal plan for a departing user across queues, shared lines, groups, devices, numbers and calling plan; --apply executes it.


**routing**


- `zoom-phone-cli routing trace`  -  Follow one inbound number or extension through auto receptionists, IVR keys and call queues to where it rings, for business and closed hours.

- `zoom-phone-cli routing audit`  -  Dead IVR targets, empty call queues, orphaned auto receptionists and auto receptionists with no upcoming holiday hours.




## Freshness Contract

This printed CLI owns bounded freshness only for registered store-backed read command paths. In `--data-source auto` mode, those paths check `sync_state` and may run a bounded refresh before reading local data. `--data-source local` never refreshes. `--data-source live` reads the API and does not mutate the local store. Set `ZOOM_PHONE_NO_AUTO_REFRESH=1` to skip the freshness hook without changing source selection.

Covered commands: the list and get commands of resources that `sync` stores, for example `users list-phone`, `call-queues list`, `numbers list-account-phone`, `devices list-phone`, `sites list` and `emergency-addresses list` (run `zoom-phone-cli sync --help` for the synced resource names). The audits (`e911 audit`, `numbers inventory`, `devices health`, `footprint`) read only the local store and report `assessed: false` until every resource they need has a completed sync.

When JSON output uses the generated provenance envelope, freshness metadata appears at `meta.freshness`. Treat it as current-cache freshness for the covered command path, not a guarantee of complete historical backfill or API-specific enrichment.

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
zoom-phone-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Narrow the user list to the fields that matter

```bash
zoom-phone-cli users list-phone --agent --select name,email,extension_number,phone_numbers.number
```

The user list is verbose; --select keeps only names, emails, extensions and numbers.

### E911 sweep

```bash
zoom-phone-cli e911 audit --agent
```

Lists every user, common area and site whose effective emergency address is missing or unverified.

### Who needs a callback

```bash
zoom-phone-cli callbacks --since 7d --agent
```

External callers missed in the last week who were never called back.

### Trace the main number after hours

```bash
zoom-phone-cli routing trace +14155550123 --agent
```

Follows the number through auto receptionist IVR keys and queues for business and closed hours.

### Same audit across every client

```bash
zoom-phone-cli fleet -- e911 audit
```

Runs the E911 audit against each saved client account and merges the results; sync each account first with fleet -- sync.

## Auth Setup

Zoom Phone uses a Server-to-Server OAuth app (Zoom App Marketplace, Develop > Build App > Server-to-Server OAuth) with phone admin scopes such as phone:read:admin and phone:write:admin. Set ZOOM_PHONE_ACCOUNT_ID, ZOOM_PHONE_CLIENT_ID and ZOOM_PHONE_CLIENT_SECRET; the CLI mints a one-hour bearer token with the account_credentials grant and re-mints automatically. There is no refresh token. Each Zoom account needs its own app, so MSPs save one profile per client account with 'accounts connect' and select it with eval "$(zoom-phone-cli accounts path <name> --env)" (sets ZOOM_PHONE_CONFIG and ZOOM_PHONE_HOME together) or run across accounts with fleet. The --profile and --client-profile flags do not select a Zoom account.

Run `zoom-phone-cli doctor` to verify setup.

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
  zoom-phone-cli auto-receptionists list --agent --select audio_prompt_language,cost_center,department
  ```
- **Previewable**  -  `--dry-run` shows the request without sending; it is opt-in, so a write without it is sent at once (MCP writes instead always need `confirm: true`)
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear. `--yes` answers this CLI's own local prompts (profile and local-data deletion); it does not gate API writes
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

- Use `--home <dir>` for one invocation, or set `ZOOM_PHONE_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `ZOOM_PHONE_CONFIG_DIR`, `ZOOM_PHONE_DATA_DIR`, `ZOOM_PHONE_STATE_DIR`, `ZOOM_PHONE_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `ZOOM_PHONE_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `zoom-phone-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "zoom-phone": {
        "command": "zoom-phone-mcp",
        "env": {
          "ZOOM_PHONE_HOME": "/srv/zoom-phone"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `ZOOM_PHONE_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `ZOOM_PHONE_HOME`, or `doctor` will not find credentials left under the former root.

Local data is unencrypted and relies on OS account permissions plus disk encryption. Reads in the default `--data-source auto` mode are written to the SQLite store; GET responses are cached for five minutes under the cache dir. `local-data purge` previews what it would erase (store files, response cache, learn journal) and deletes only with `--yes`; it is not exposed over MCP. `ZOOM_PHONE_NO_LOCAL_DATA=1` disables the response cache and store writes, turns `auto` reads into live reads, and blocks `sync` and `workflow archive`; `--data-source local` still reads an existing store, and `ZOOM_PHONE_DATA_SOURCE` sets the `--data-source` default.

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
zoom-phone-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "zoom-phone-cli learnings confirm 12"] }
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
       materially more, record the divergence via `zoom-phone-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `zoom-phone-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `zoom-phone-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
zoom-phone-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
zoom-phone-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
zoom-phone-cli teach-playbook \
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
zoom-phone-cli playbook amend \
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

`zoom-phone-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `ZOOM_PHONE_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
zoom-phone-cli feedback "the --since flag is inclusive but docs say exclusive"
zoom-phone-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `ZOOM_PHONE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `ZOOM_PHONE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
zoom-phone-cli profile save briefing --json
zoom-phone-cli --profile briefing auto-receptionists list
zoom-phone-cli profile list --json
zoom-phone-cli profile show briefing
zoom-phone-cli profile delete briefing --yes
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
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `zoom-phone-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add zoom-phone-mcp -e ZOOM_PHONE_ACCOUNT_ID=<account-id> -e ZOOM_PHONE_CLIENT_ID=<client-id> -e ZOOM_PHONE_CLIENT_SECRET=<client-secret> -- zoom-phone-mcp
   ```
3. Verify: `claude mcp list`

For a remote host, `zoom-phone-mcp --transport http` requires `ZOOM_PHONE_MCP_HTTP_TOKEN` set to at least 32 characters (callers send `Authorization: Bearer <token>`; `openssl rand -hex 32` prints 64 hex characters and meets that minimum), serves `http://127.0.0.1:7777/mcp` by default, and needs `--tls-cert`/`--tls-key` for any non-loopback `--addr`. Requests with no `Origin` header are accepted. Loopback origins are accepted; any other `Origin` is refused unless listed in `ZOOM_PHONE_MCP_HTTP_ALLOWED_ORIGINS` (comma-separated `scheme://host[:port]` values). `ZOOM_PHONE_MCP_READ_ONLY=1` drops mirrored write commands (local-write commands that only touch the CLI's own learn store stay) and makes the raw execute tool refuse state-changing calls.

## Direct Use

1. Check if installed: `which zoom-phone-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   zoom-phone-cli e911 audit --agent
   ```
4. If ambiguous, drill into subcommand help: `zoom-phone-cli <command> --help`.
