---
name: google-workspace
description: "Use when the user asks to audit or administer Google Workspace across several client domains: find accounts without 2-Step Verification, reclaim paid licenses on suspended or idle accounts, check that leavers were fully offboarded, offboard a leaver in the order Google requires, review Drive and Calendar sharing changes, inventory risky third-party OAuth apps, or pull a one-row-per-client posture summary for a QBR. Drives the Google Workspace Admin SDK (Directory, Reports, Enterprise License Manager, Data Transfer, Reseller) per client tenant via service accounts with domain-wide delegation. Trigger phrases: `who doesn't have 2SV across my clients`, `google workspace license waste`, `google workspace licence waste`, `offboard a google workspace user`, `did our google workspace leavers get cleaned up`, `external sharing changes in google workspace`, `risky oauth apps in google workspace`, `google workspace posture for every client`, `use google-workspace`, `run google-workspace-cli`."
author: "Damien Stevens"
license: "Apache-2.0"
vendor: "Google Workspace"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - google-workspace-cli
    install:
      - kind: script
        bins: [google-workspace-cli]
        sh: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/google-workspace/install.sh
        ps1: https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/google-workspace/install.ps1
---

# Google Workspace Claude Code Skill

## Prerequisites: Install the CLI

This skill drives the `google-workspace-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/google-workspace/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/google-workspace/install.ps1 | iex
   ```
3. Verify: `google-workspace-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

The installer downloads the `google-workspace-cli` and `google-workspace-mcp` binaries into `~/.local/bin`
(macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not
register the skill with your agent and writes no MCP client config - see
[mcp-install.md](./mcp-install.md) for that wire-up.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

Typed commands for the Admin SDK Directory, Reports, Enterprise License Manager, Data Transfer and Reseller methods, run per client tenant. Register each client once (a service account with domain-wide delegation impersonating that client's super admin), then ask fleet questions in one command: mfa-gaps, stale-licenses, sharing-drift, posture, leavers, oauth-apps and login-alerts fan out across every tenant. offboard runs a leaver's cleanup with suspend last (backup-code and token cleanup fail on a suspended account) and previews before it changes anything.

## When to Use This CLI

Use this CLI when an MSP needs answers across several Google Workspace client tenants at once: 2SV coverage, licence waste, external-sharing changes, risky sign-ins, third-party app access, or a per-client posture row for a QBR. It is also the safe way to offboard a leaver in the order Google requires, and gives typed access to all 163 Admin SDK, Reports, Licensing, Data Transfer and Reseller methods per tenant.

## Anti-triggers

Do not use this CLI for:
- Reading or sending a user's Gmail, Drive files or Calendar events: use Google's Workspace MCP or the end-user APIs instead.
- Microsoft 365 tenants: use CIPP or a Graph-based connector instead.
- Changing Gmail forwarding, delegates or vacation responders: that is the Gmail API, which this CLI does not cover.
- Creating Google Cloud projects or service-account keys: do that in each client's Cloud console or with gcloud, then register the key with 'tenants add'.

## Unique Capabilities

### Cross-client security
- **`mfa-gaps`**  -  Every account without 2-Step Verification across all your client tenants, ranked with unenrolled admins first.

  _Reach for this for any 'who lacks 2SV' question across more than one client; it already ranks admins as critical._

  ```bash
  google-workspace-cli mfa-gaps --all-tenants --admins-only --agent
  ```
- **`sharing-drift`**  -  Who changed Drive or Calendar sharing settings, and which files newly went external or public, across clients.

  _Use it for 'what changed in sharing since last review' across clients; Drive audit needs Business Standard or higher, so run Business Starter tenants with --settings-only._

  ```bash
  google-workspace-cli sharing-drift --all-tenants --since 7d --agent
  ```
- **`posture`**  -  One row per client: 2SV coverage, admins without 2SV, stale licences and sharing changes, plus current sharing and 2SV policy for tenants registered with the policy scope profile.

  _Start here for a fleet overview or a QBR headline, then drill in with mfa-gaps, stale-licenses or sharing-drift._

  ```bash
  google-workspace-cli posture --all-tenants --csv
  ```
- **`oauth-apps`**  -  Third-party apps with access to client data across every active user and tenant, ranked by scope risk; --since switches to apps newly authorized in that window.

  _Use it to find risky apps across clients; on tenants above --max-users (default 1000 active users) use --since 30d, which reads the token audit log instead._

  ```bash
  google-workspace-cli oauth-apps --all-tenants --min-risk high --agent
  ```
- **`login-alerts`**  -  Suspicious sign-ins, accounts Google disabled, 2SV turned off, and out-of-domain mail forwarding across clients.

  _Use it for the weekly 'anything bad happen to a client account' check._

  ```bash
  google-workspace-cli login-alerts --all-tenants --since 7d --agent
  ```

### Licence and seat waste
- **`stale-licenses`**  -  Paid licences held by suspended, never-signed-in, or idle accounts, per client, ready to reclaim.

  _Use it before QBRs and renewals: suspended accounts bill like active ones._

  ```bash
  google-workspace-cli stale-licenses --all-tenants --inactive-days 60 --agent
  ```

### Offboarding
- **`offboard`**  -  Offboard a leaver with suspend last (backup-code and token cleanup fail on a suspended account), a read-only preview, dry-run, and a receipt per step.

  _Use it for any leaver: without --yes it only previews (read scopes are enough); applying needs a tenant registered with --scopes full. Over MCP it can only preview._

  ```bash
  google-workspace-cli offboard jane@example.com --successor manager@example.com --ou /Departed
  ```
- **`leavers`**  -  Suspended accounts that still hold a paid licence, groups, an admin role, OAuth tokens, or have no completed Drive transfer.

  _Run it weekly to catch offboards done by hand or in the Admin console._

  ```bash
  google-workspace-cli leavers --all-tenants --agent
  ```

## Command Reference

Every command below is listed with its required positional arguments. Run `google-workspace-cli <command> --help` for flags.

**activities**

- `google-workspace-cli activities list <applicationName>` - Retrieves a list of activities for a specific customer's account and application such as the Admin console application
- `google-workspace-cli activities watch <applicationName>` - Start receiving notifications for account activities. For more information, see Receiving Push Notifications.

**admin-channels**

- `google-workspace-cli admin-channels` - Stop watching resources through this channel.

**analytics**

- `google-workspace-cli analytics` - Run analytics queries on locally synced data

**api**

- `google-workspace-cli api [interface]` - Browse API resource interfaces by raw name

**applications**

- `google-workspace-cli applications get <applicationId>` - Retrieves information about an application for the given application ID.
- `google-workspace-cli applications list` - Lists the applications available for data transfer for a customer.

**asps**

- `google-workspace-cli asps delete <userKey> <codeId>` - Deletes an ASP issued by a user.
- `google-workspace-cli asps get <userKey> <codeId>` - Gets information about an ASP issued by a user.
- `google-workspace-cli asps list <userKey>` - Lists the ASPs issued by a user.

**auth**

- `google-workspace-cli auth logout` - Clear stored credentials
- `google-workspace-cli auth set-token` - Save an API token to the credentials file
- `google-workspace-cli auth setup` - Print steps for obtaining a credential (use --launch to open the URL)
- `google-workspace-cli auth status` - Show authentication status

**channels**

- `google-workspace-cli channels` - Stops watching resources through this channel.

**chromeosdevices**

- `google-workspace-cli chromeosdevices action <resourceId>` - Use [BatchChangeChromeOsDeviceStatus](https://developers.google.
- `google-workspace-cli chromeosdevices get <deviceId>` - Retrieves a Chrome OS device's properties.
- `google-workspace-cli chromeosdevices list` - Retrieves a paginated list of Chrome OS devices within an account.
- `google-workspace-cli chromeosdevices move-devices-to-ou` - Moves or inserts multiple Chrome OS devices to an organizational unit. You can move up to 50 devices at once.
- `google-workspace-cli chromeosdevices patch <deviceId>` - Updates a device's updatable properties, such as `annotatedUser`, `annotatedLocation`, `notes`, `orgUnitPath`
- `google-workspace-cli chromeosdevices update <deviceId>` - Updates a device's updatable properties, such as `annotatedUser`, `annotatedLocation`, `notes`, `orgUnitPath`

**customer**

- `google-workspace-cli customer devices-chromeos batch-change-status` - Changes the status of a batch of ChromeOS devices.
- `google-workspace-cli customer devices-chromeos count-chrome-os-devices` - Counts ChromeOS devices matching the request.
- `google-workspace-cli customer devices-chromeos issue-command <deviceId>` - Issues a command for the device to execute.
- `google-workspace-cli customer devices-chromeos-commands get <deviceId> <commandId>` - Gets command data a specific command issued to the device.

**customer-usage-reports**

- `google-workspace-cli customer-usage-reports <date>` - Retrieves a report which is a collection of properties and statistics for a specific customer's account.

**customers**

- `google-workspace-cli customers chrome-print-servers batch-create-print-servers <parent> <customersId>` - Creates multiple print servers.
- `google-workspace-cli customers chrome-print-servers batch-delete-print-servers <parent> <customersId>` - Deletes multiple print servers.
- `google-workspace-cli customers chrome-print-servers create <parent> <customersId>` - Creates a print server.
- `google-workspace-cli customers chrome-print-servers delete <name> <customersId> <printServersId>` - Deletes a print server.
- `google-workspace-cli customers chrome-print-servers get <name> <customersId> <printServersId>` - Returns a print server's configuration.
- `google-workspace-cli customers chrome-print-servers list <parent> <customersId>` - Lists print server configurations.
- `google-workspace-cli customers chrome-print-servers patch <name> <customersId> <printServersId>` - Updates a print server's configuration.
- `google-workspace-cli customers chrome-printers batch-create-printers <parent> <customersId>` - Creates printers under given Organization Unit.
- `google-workspace-cli customers chrome-printers batch-delete-printers <parent> <customersId>` - Deletes printers in batch.
- `google-workspace-cli customers chrome-printers create <parent> <customersId>` - Creates a printer under given Organization Unit.
- `google-workspace-cli customers chrome-printers delete <name> <customersId> <printersId>` - Deletes a `Printer`.
- `google-workspace-cli customers chrome-printers get <name> <customersId> <printersId>` - Returns a `Printer` resource (printer's config).
- `google-workspace-cli customers chrome-printers list <parent> <customersId>` - List printers configs.
- `google-workspace-cli customers chrome-printers list-printer-models <parent> <customersId>` - Lists the supported printer models.
- `google-workspace-cli customers chrome-printers patch <name> <customersId> <printersId>` - Updates a `Printer` resource.
- `google-workspace-cli customers get` - Retrieves a customer.
- `google-workspace-cli customers patch` - Patches a customer.
- `google-workspace-cli customers update` - Updates a customer.

**doctor**

- `google-workspace-cli doctor` - Check CLI health

**domain-aliases**

- `google-workspace-cli domain-aliases delete <domainAliasName>` - Deletes a domain Alias of the customer.
- `google-workspace-cli domain-aliases get <domainAliasName>` - Retrieves a domain alias of the customer.
- `google-workspace-cli domain-aliases insert` - Inserts a domain alias of the customer.
- `google-workspace-cli domain-aliases list` - Lists the domain aliases of the customer.

**domains**

- `google-workspace-cli domains delete <domainName>` - Deletes a domain of the customer.
- `google-workspace-cli domains get <domainName>` - Retrieves a domain of the customer.
- `google-workspace-cli domains insert` - Inserts a domain of the customer.
- `google-workspace-cli domains list` - Lists the domains of the customer.

**entity-usage-reports**

- `google-workspace-cli entity-usage-reports <entityType> <entityKey> <date>` - Retrieves a report which is a collection of properties and statistics for entities used by users within the account.

**export**

- `google-workspace-cli export <resource> [id]` - Export data to JSONL or JSON for backup, migration, or analysis

**feedback**

- `google-workspace-cli feedback list` - List recent feedback entries

**groups**

- `google-workspace-cli groups aliases delete <groupKey> <alias>` - Removes an alias.
- `google-workspace-cli groups aliases insert <groupKey>` - Adds an alias for the group.
- `google-workspace-cli groups aliases list <groupKey>` - Lists all aliases for a group.
- `google-workspace-cli groups delete <groupKey>` - Deletes a group.
- `google-workspace-cli groups get <groupKey>` - Retrieves a group's properties.
- `google-workspace-cli groups insert` - Creates a group.
- `google-workspace-cli groups list` - Retrieves all groups of a domain or of a user given a userKey (paginated).
- `google-workspace-cli groups patch <groupKey>` - Updates a group's properties. This method supports [patch semantics](https://developers.google.
- `google-workspace-cli groups update <groupKey>` - Updates a group's properties.

**import**

- `google-workspace-cli import <resource>` - Import data from JSONL file via API create/upsert calls

**learnings**

- `google-workspace-cli learnings candidates` - List auto-captured improvement candidates awaiting judgment
- `google-workspace-cli learnings confirm <id>` - Confirm an open candidate: print its full payload, then materialize it
- `google-workspace-cli learnings forget <query>` - Delete learnings matching a query (use --all to wipe every rule for that query)
- `google-workspace-cli learnings list` - List recorded learnings from the local google-workspace-cli search_learnings table
- `google-workspace-cli learnings purge` - Delete settled candidate rows (expired and confirmed; --tombstones adds rejected)
- `google-workspace-cli learnings reject <id>` - Reject a candidate and tombstone its derivation signature
- `google-workspace-cli learnings stats` - Report learn-loop effectiveness metrics from the local events table

**leavers**

- `google-workspace-cli leavers` - Suspended accounts still holding a licence, groups, admin roles, OAuth tokens, or no completed transfer

**license-assignments**

- `google-workspace-cli license-assignments delete <skuId> <userId>` - Revoke a license.
- `google-workspace-cli license-assignments get <skuId> <userId>` - Get a specific user's license by product SKU.
- `google-workspace-cli license-assignments insert <skuId>` - Assign a license.
- `google-workspace-cli license-assignments list-for-product` - List all users assigned licenses for a specific product SKU.
- `google-workspace-cli license-assignments list-for-product-and-sku <skuId>` - List all users assigned licenses for a specific product SKU.
- `google-workspace-cli license-assignments patch <skuId> <userId>` - Reassign a user's product SKU with a different SKU in the same product. This method supports patch semantics.
- `google-workspace-cli license-assignments update <skuId> <userId>` - Reassign a user's product SKU with a different SKU in the same product.

**login-alerts**

- `google-workspace-cli login-alerts` - Suspicious sign-ins, accounts Google disabled, 2SV turned off, and out-of-domain forwarding across clients

**members**

- `google-workspace-cli members delete <groupKey> <memberKey>` - Removes a member from a group.
- `google-workspace-cli members get <groupKey> <memberKey>` - Retrieves a group member's properties.
- `google-workspace-cli members has-member <groupKey> <memberKey>` - Checks whether the given user is a member of the group.
- `google-workspace-cli members insert <groupKey>` - Adds a user to the specified group.
- `google-workspace-cli members list <groupKey>` - Retrieves a paginated list of all members in a group. This method times out after 60 minutes.
- `google-workspace-cli members patch <groupKey> <memberKey>` - Updates the membership properties of a user in the specified group.
- `google-workspace-cli members update <groupKey> <memberKey>` - Updates the membership of a user in the specified group.

**mfa-gaps**

- `google-workspace-cli mfa-gaps` - Users without 2-Step Verification across client tenants, admins first

**mobiledevices**

- `google-workspace-cli mobiledevices action <resourceId>` - Takes an action that affects a mobile device. For example, remotely wiping a device.
- `google-workspace-cli mobiledevices delete <resourceId>` - Removes a mobile device.
- `google-workspace-cli mobiledevices get <resourceId>` - Retrieves a mobile device's properties.
- `google-workspace-cli mobiledevices list` - Retrieves a paginated list of all user-owned mobile devices for an account.

**oauth-apps**

- `google-workspace-cli oauth-apps` - Third-party OAuth apps with access to client data, across every user and tenant, ranked by scope risk

**offboard**

- `google-workspace-cli offboard <user-email>` - Offboard a leaver in the right order: revoke access, remove groups, transfer data, suspend last

**orgunits**

- `google-workspace-cli orgunits delete <orgUnitPath> <orgunitsId>` - Removes an organizational unit.
- `google-workspace-cli orgunits get <orgUnitPath> <orgunitsId>` - Retrieves an organizational unit.
- `google-workspace-cli orgunits insert` - Adds an organizational unit.
- `google-workspace-cli orgunits list` - Retrieves a list of all organizational units for an account.
- `google-workspace-cli orgunits patch <orgUnitPath> <orgunitsId>` - Updates an organizational unit. This method supports [patch semantics](https://developers.google.
- `google-workspace-cli orgunits update <orgUnitPath> <orgunitsId>` - Updates an organizational unit.

**playbook**

- `google-workspace-cli playbook amend` - Append a note to an existing playbook (LLM-fired self-correction, silent)
- `google-workspace-cli playbook list` - List stored playbooks (query_family, content presence, last observed)

**posture**

- `google-workspace-cli posture` - One row per client tenant: 2SV coverage, admins without 2SV, stale licences, sharing changes

**privileges**

- `google-workspace-cli privileges` - Retrieves a paginated list of all privileges for a customer.

**profile**

- `google-workspace-cli profile delete <name>` - Remove a profile
- `google-workspace-cli profile list` - List saved profiles
- `google-workspace-cli profile save <name>` - Save the current invocation's non-default flags as a named profile
- `google-workspace-cli profile show <name>` - Show a profile's values as JSON
- `google-workspace-cli profile use <name>` - Print the flag values a profile will apply (does not execute anything)

**recall**

- `google-workspace-cli recall <query>` - Check prior learnings for a query before running discovery (LLM-fired, pre-discovery)

**reseller-customers**

- `google-workspace-cli reseller-customers get <customerId>` - Gets a customer account.
- `google-workspace-cli reseller-customers insert` - Orders a new customer's account.
- `google-workspace-cli reseller-customers patch <customerId>` - Updates a customer account's settings. This method supports patch semantics.
- `google-workspace-cli reseller-customers update <customerId>` - Updates a customer account's settings.

**resellernotify**

- `google-workspace-cli resellernotify getwatchdetails` - Returns all the details of the watch corresponding to the reseller.
- `google-workspace-cli resellernotify register` - Registers a Reseller for receiving notifications.
- `google-workspace-cli resellernotify unregister` - Unregisters a Reseller for receiving notifications.

**resources**

- `google-workspace-cli resources buildings delete <buildingId>` - Deletes a building.
- `google-workspace-cli resources buildings get <buildingId>` - Retrieves a building.
- `google-workspace-cli resources buildings insert` - Inserts a building.
- `google-workspace-cli resources buildings list` - Retrieves a list of buildings for an account.
- `google-workspace-cli resources buildings patch <buildingId>` - Patches a building.
- `google-workspace-cli resources buildings update <buildingId>` - Updates a building.
- `google-workspace-cli resources calendars delete <calendarResourceId>` - Deletes a calendar resource.
- `google-workspace-cli resources calendars get <calendarResourceId>` - Retrieves a calendar resource.
- `google-workspace-cli resources calendars insert` - Inserts a calendar resource.
- `google-workspace-cli resources calendars list` - Retrieves a list of calendar resources for an account.
- `google-workspace-cli resources calendars patch <calendarResourceId>` - Patches a calendar resource.
- `google-workspace-cli resources calendars update <calendarResourceId>` - Updates a calendar resource.
- `google-workspace-cli resources features delete <featureKey>` - Deletes a feature.
- `google-workspace-cli resources features get <featureKey>` - Retrieves a feature.
- `google-workspace-cli resources features insert` - Inserts a feature.
- `google-workspace-cli resources features list` - Retrieves a list of features for an account.
- `google-workspace-cli resources features patch <featureKey>` - Patches a feature.
- `google-workspace-cli resources features rename <oldName>` - Renames a feature.
- `google-workspace-cli resources features update <featureKey>` - Updates a feature.

**role-assignments**

- `google-workspace-cli role-assignments delete <roleAssignmentId>` - Deletes a role assignment.
- `google-workspace-cli role-assignments get <roleAssignmentId>` - Retrieves a role assignment.
- `google-workspace-cli role-assignments insert` - Creates a role assignment.
- `google-workspace-cli role-assignments list` - Retrieves a paginated list of all roleAssignments.

**roles**

- `google-workspace-cli roles delete <roleId>` - Deletes a role.
- `google-workspace-cli roles get <roleId>` - Retrieves a role.
- `google-workspace-cli roles insert` - Creates a role.
- `google-workspace-cli roles list` - Retrieves a paginated list of all the roles in a domain.
- `google-workspace-cli roles patch <roleId>` - Patches a role.
- `google-workspace-cli roles update <roleId>` - Updates a role.

**schemas**

- `google-workspace-cli schemas delete <schemaKey>` - Deletes a schema.
- `google-workspace-cli schemas get <schemaKey>` - Retrieves a schema.
- `google-workspace-cli schemas insert` - Creates a schema.
- `google-workspace-cli schemas list` - Retrieves all schemas for a customer.
- `google-workspace-cli schemas patch <schemaKey>` - Patches a schema.
- `google-workspace-cli schemas update <schemaKey>` - Updates a schema.

**search**

- `google-workspace-cli search <query>` - Search locally synced data

**sharing-drift**

- `google-workspace-cli sharing-drift` - External-sharing changes across client tenants: Drive sharing settings, new external shares, public links

**stale-licenses**

- `google-workspace-cli stale-licenses` - Paid licences held by suspended, never-used, or inactive accounts across client tenants

**subscriptions**

- `google-workspace-cli subscriptions activate <customerId> <subscriptionId>` - Activates a subscription previously suspended by the reseller.
- `google-workspace-cli subscriptions change-plan <customerId> <subscriptionId>` - Updates a subscription plan.
- `google-workspace-cli subscriptions change-renewal-settings <customerId> <subscriptionId>` - Updates a user license's renewal settings. This is applicable for accounts with annual commitment plans only.
- `google-workspace-cli subscriptions change-seats <customerId> <subscriptionId>` - Updates a subscription's user license settings.
- `google-workspace-cli subscriptions delete <customerId> <subscriptionId>` - Cancels, suspends, or transfers a subscription to direct.
- `google-workspace-cli subscriptions get <customerId> <subscriptionId>` - Gets a specific subscription.
- `google-workspace-cli subscriptions insert <customerId>` - Creates or transfer a subscription.
- `google-workspace-cli subscriptions list` - Lists of subscriptions managed by the reseller.
- `google-workspace-cli subscriptions start-paid-service <customerId> <subscriptionId>` - Immediately move a 30-day free trial subscription to a paid service subscription.
- `google-workspace-cli subscriptions suspend <customerId> <subscriptionId>` - Suspends an active subscription.

**sync**

- `google-workspace-cli sync` - Sync API data to local SQLite for offline search and analysis

**tail**

- `google-workspace-cli tail [resource]` - Stream live changes by polling the API at regular intervals

**teach**

- `google-workspace-cli teach` - Record a query -> resource mapping for future recall (LLM-fired, silent)

**teach-lookup**

- `google-workspace-cli teach-lookup` - Install a manual entity-lookup row (kind, canonical, value)

**teach-pattern**

- `google-workspace-cli teach-pattern` - Install a manual generalization pattern (query_template, resource_template, entity_kind)

**teach-playbook**

- `google-workspace-cli teach-playbook` - Record a CLI playbook + free-text notes for a query family

**tenants**

- `google-workspace-cli tenants add <name>` - Register a client tenant (copies its service-account key into the config dir, mode 0600)
- `google-workspace-cli tenants list` - List registered client tenants
- `google-workspace-cli tenants remove <name>` - Remove a tenant and delete its stored service-account key copy
- `google-workspace-cli tenants scopes <name>` - Print the Client ID and OAuth scopes to authorize in the client's Admin console
- `google-workspace-cli tenants test [name]` - Verify each tenant's domain-wide delegation scope by scope, and record its customer ID and domains

**tokens**

- `google-workspace-cli tokens delete <userKey> <clientId>` - Deletes all access tokens issued by a user for an application.
- `google-workspace-cli tokens get <userKey> <clientId>` - Gets information about an access token issued by a user.
- `google-workspace-cli tokens list <userKey>` - Returns the set of tokens specified user has issued to 3rd party applications.

**transfers**

- `google-workspace-cli transfers get <dataTransferId>` - Retrieves a data transfer request by its resource ID.
- `google-workspace-cli transfers insert` - Inserts a data transfer request. See the [Transfer parameters](https://developers.google.
- `google-workspace-cli transfers list` - Lists the transfers for a customer by source user, destination user, or status.

**two-step-verification**

- `google-workspace-cli two-step-verification <userKey>` - Turns off 2-Step Verification for user.

**user-usage-report**

- `google-workspace-cli user-usage-report <date>` - Retrieves a report which is a collection of properties and statistics for a set of users with the account.

**users**

- `google-workspace-cli users aliases delete <userKey> <alias>` - Removes an alias.
- `google-workspace-cli users aliases insert <userKey>` - Adds an alias.
- `google-workspace-cli users aliases list <userKey>` - Lists all aliases for a user.
- `google-workspace-cli users aliases watch <userKey>` - Watches for changes in users list.
- `google-workspace-cli users create-guest` - Create a guest user with access to a [subset of Workspace capabilities](https://support.google.com/a/answer/16558545).
- `google-workspace-cli users delete <userKey>` - Deletes a user.
- `google-workspace-cli users get <userKey>` - Retrieves a user.
- `google-workspace-cli users insert` - Creates a user.
- `google-workspace-cli users list` - Retrieves a paginated list of either deleted users or all users in a domain.
- `google-workspace-cli users make-admin <userKey>` - Makes a user a super administrator.
- `google-workspace-cli users patch <userKey>` - Updates a user using patch semantics.
- `google-workspace-cli users photos delete <userKey>` - Removes the user's photo.
- `google-workspace-cli users photos get <userKey>` - Retrieves the user's photo.
- `google-workspace-cli users photos patch <userKey>` - Adds a photo for the user. This method supports [patch semantics](https://developers.google.
- `google-workspace-cli users photos update <userKey>` - Adds a photo for the user.
- `google-workspace-cli users sign-out <userKey>` - Signs a user out of all web and device sessions and reset their sign-in cookies.
- `google-workspace-cli users undelete <userKey>` - Undeletes a deleted user.
- `google-workspace-cli users update <userKey>` - Updates a user.
- `google-workspace-cli users watch` - Watches for changes in users list.

**verification-codes**

- `google-workspace-cli verification-codes generate <userKey>` - Generates new backup verification codes for the user.
- `google-workspace-cli verification-codes invalidate <userKey>` - Invalidates the current backup verification codes for the user.
- `google-workspace-cli verification-codes list <userKey>` - Returns the current set of valid backup verification codes for the specified user.

**version**

- `google-workspace-cli version` - Print version

**which**

- `google-workspace-cli which [query]` - Find the command that implements a capability

**workflow**

- `google-workspace-cli workflow archive` - Sync all resources to local store for offline access and search
- `google-workspace-cli workflow status` - Show local archive status and sync state for all resources

### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
google-workspace-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Check the match before running it: for 2-Step Verification questions `which` can rank `two-step-verification` (which turns 2SV off for one user) above `mfa-gaps` (the read-only cross-client 2SV report). Exit code `0` means at least one match; exit code `2` means no confident match  -  fall back to `--help` or use a narrower query. `--json` (and other machine formats) keep that exit-2 contract and write `{"matches":[]}` on stdout so agents can inspect the envelope without treating a miss as success.

## Recipes

### Weekly 2SV sweep, admins first

```bash
google-workspace-cli mfa-gaps --all-tenants --agent --select results.tenant,results.email,results.severity,results.admin_role,meta.fetch_failures
```

Pulls only the fields an agent needs from every client's unenrolled accounts, and keeps meta.fetch_failures so a broken tenant is not mistaken for a clean one.

### Licence waste for a QBR

```bash
google-workspace-cli stale-licenses --tenant acme --inactive-days 90 --csv
```

Named accounts holding a paid licence that nobody uses, as CSV for the deck.

### What changed in sharing this week

```bash
google-workspace-cli sharing-drift --all-tenants --since 7d --settings-only
```

Only Admin console sharing-setting changes, old to new, with who made them.

### Check last week's leavers were closed

```bash
google-workspace-cli leavers --all-tenants --agent
```

Suspended accounts still holding licences, groups, admin roles, tokens or unfinished transfers.

### Offboard and apply

```bash
google-workspace-cli offboard jane@example.com --successor manager@example.com --ou /Departed --release-license --yes
```

Runs every step in order and returns a receipt per step. Licence release is skipped while the Drive transfer runs; the receipt's next field lists one `license-assignments delete` command per held licence, to run once `transfers list` shows the transfer complete. Use `--json` rather than `--agent` for offboard: `--agent` implies `--compact`, which drops each step's detail (the OU move, token and group names, and any error text).

## Auth Setup

Each client tenant is a Google service account with domain-wide delegation that impersonates that client's super admin, the same model GAM uses. Register it with 'tenants add <name> --domain <d> --admin <email> --key <sa.json>' (the key is copied to keys/<name>.json in the config dir at 0600, next to tenants.toml; both are secrets), paste the printed Client ID and scopes into that client's Admin console under Security > Access and data control > API controls > Domain-wide delegation, then run 'tenants test <name>' to mint one token per scope and name any missing delegation. Scope profiles (--scopes, comma-separated, raw scope names allowed): read is the default and covers every fan-out command, but two of its scopes have no read-only variant (admin.directory.user.security and apps.licensing), so it can also sign users out and change licences; full adds the offboarding writes (user, group membership, data transfer); policy adds cloud-identity.policies.readonly for posture's policy columns (only full includes read, so register it as --scopes read,policy or full,policy; --scopes policy alone leaves the reports nothing to read); reseller is for your own partner domain only ('tenants add <name> --reseller --scopes reseller'), is read-only (Reseller writes also need apps.order), and is never included in --all-tenants. Schemas, calendar resources, Chrome printers and most group, OU, domain, role and device writes need extra raw scopes. Change a tenant's scopes by re-running tenants add with --yes, and print them again with 'tenants scopes <name>'. Pick a client per command with --tenant <name> or GOOGLE_WORKSPACE_TENANT, or fan out with --all-tenants. With no tenants registered, a raw bearer token in GOOGLE_WORKSPACE_TOKEN (or 'auth set-token') acts as the single tenant "default", including for --all-tenants and offboard. The MCP server reads the same tenant registry: register tenants with the CLI first (tenants add is not exposed over MCP), pass a tenant argument per tool call, and set GOOGLE_WORKSPACE_TENANT in the MCP server's env for the raw endpoint execute tool and the sql, search and context tools, which take no tenant argument. Unset GOOGLE_WORKSPACE_TOKEN once tenants are registered: with two or more registered and none selected, generated API commands fall back to that token and act on its organization.

Run `google-workspace-cli tenants test --all-tenants` to verify every client's delegation; it exits non-zero when any scope is missing. `doctor` exits 0 even on a rejected key unless you pass `--fail-on error`.

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
  google-workspace-cli activities list login --user-key all --agent
  ```
- **Previewable**  -  `--dry-run` shows the request without sending
- **Offline-friendly**  -  sync/search commands can use the local SQLite store when available
- **Non-interactive**  -  never prompts, every input is a flag
- **Explicit confirmation**  -  `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Partial fan-out** - when some tenants fail, a cross-client command still prints the other tenants' results with `meta.fetch_failures` but exits 6. Over MCP a non-zero exit returns only the error text, so pass `allow-partial-failure` (CLI: `--allow-partial-failure`) on fan-out calls to get the partial results, then report `meta.fetch_failures` instead of treating missing tenants as clean.
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

- Use `--home <dir>` for one invocation, or set `GOOGLE_WORKSPACE_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `GOOGLE_WORKSPACE_CONFIG_DIR`, `GOOGLE_WORKSPACE_DATA_DIR`, `GOOGLE_WORKSPACE_STATE_DIR`, `GOOGLE_WORKSPACE_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `GOOGLE_WORKSPACE_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `google-workspace-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "google-workspace": {
        "command": "google-workspace-mcp",
        "env": {
          "GOOGLE_WORKSPACE_HOME": "/srv/google-workspace"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `GOOGLE_WORKSPACE_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `GOOGLE_WORKSPACE_HOME`, or `doctor` will not find credentials left under the former root.

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
google-workspace-cli recall "$QUERY" --agent
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
      "next_action": ["<trial command>", "google-workspace-cli learnings confirm 12"] }
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
       materially more, record the divergence via `google-workspace-cli playbook amend`
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

Candidate judgment details: `learnings confirm <id>` prints the candidate's full payload before materializing it - check that the printed payload matches the behavior you verified. `learnings reject <id>` tombstones the derivation signature so the same candidate does not resurface. The envelope carries only the few candidates worth acting on now; `google-workspace-cli learnings candidates` lists the full open set.

Graceful degradation: if `learnings confirm` is an unknown command, you are driving an older binary - ignore the candidates guidance and follow the rest of the protocol.

### Step 3: always read `warnings`

- `low_confidence`: row exists at `confidence<2`. Treat as a hint, not a skip-discovery hit.
- `resource_not_in_store`: the local store doesn't have the resource the learning points at. The match validator couldn't classify entities  -  direct-fetch and re-evaluate.
- `cross_alias_match` (per-result): the row was taught under a different alias and matched the live query's canonical via `entity_lookups` (e.g., a "USA" teach satisfying a "United States" recall). Trust the resource_id.
- `similar_shape_different_entity:<canonical>` (top-level): a structurally matching row exists but its canonical entity differs from the live query's. Treated as cold start; the warning carries the conflicting canonical as a hint, but the row is NOT promoted into Results.
- `ambiguous_alias` (top-level): a single query entity resolved to multiple canonicals (e.g., "Cards" → Arizona Cardinals + St. Louis Cardinals). Surface the ambiguity from context before committing to a resource.
- `candidates_present` (top-level): the envelope carries a `candidates` section. Handle it via the candidates branch in Step 2 before anything else.
- `lookup_refresh_available` (top-level): an entity in the query has no lookup row yet, but synced data could provide one. Run `google-workspace-cli sync` to refresh entity lookups.
- Top-level `no_learnings_for_query_family`: the table had no rows above the Jaccard floor. Pure cold start.

### Step 4: `teach &` after finalizing your response - always

Teaching is unconditional. After resolving a query the store could not answer, background-teach the final resource mapping - no call-count threshold, no judging whether it was "worth" learning. The teach is the anchor of the loop: it triggers playbook synthesis for a family without a playbook, and same-referent phrasings fold into one family so near-duplicate teaches do not fragment the store. Fire it after assembling your user-facing response but BEFORE emitting it, with a shell `&` so the call returns immediately. Pass the query the same way as recall  -  argv/MCP, or file-then-`$QUERY`. Do not splice the question into the command text:

```bash
QUERY=$(cat /path/to/question.txt)
google-workspace-cli teach --query "$QUERY" --resource-type <type> --resource <id1> --resource <id2>
# (append shell `&` to background it)
```

Silent on success. Errors only land in `teach.log` under the resolved state dir. Teach the **most specific** resource - if the user asked a broad question and you walked through parent records to find the specific answer, teach the leaf id, not the parent. The CLI uses seeded `entity_lookups` for cross-alias resolution at recall time, so a teach under one alias (e.g., "Niners") satisfies future queries under another alias (e.g., "49ers", "San Francisco") automatically.

PII rule: teach the structural question with identifiers stripped - never include names, emails, phone numbers, account ids, or other personal identifiers in taught queries or notes. The CLI scans teach queries for obvious email/phone shapes and warns, but does not block; strip before teaching rather than relying on the warning.

### Step 5: playbooks - optional flags, automatic synthesis

You do not need to decide whether a session "deserves" a playbook: a teach on a family without one auto-synthesizes a `playbook_candidate` from the session's journal, and the next session judges it via confirm/reject. Attach explicit playbook flags only when you already hold choreography worth recording verbatim - workarounds the CLI didn't surface (silently-dropped flags, undocumented params, pagination tricks, payload gotchas). Prefer the **integrated one-call form** - record the resource learning and the playbook in the same `teach` invocation:

```bash
# Common case: record both the resource learning AND the playbook in one call.
QUERY=$(cat /path/to/question.txt)
google-workspace-cli teach \
  --query "$QUERY" \
  --resource <id> \
  --playbook-file ~/playbooks/<shape>.json \
  --playbook-notes-file ~/playbooks/<shape>-notes.md
# (append shell `&` to background it)

# Alternate: playbook-only (no resource to record alongside).
QUERY=$(cat /path/to/question.txt)
google-workspace-cli teach-playbook \
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
google-workspace-cli playbook amend \
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

`google-workspace-cli learnings stats` reports recall hit rate, teach-to-reuse, playbook resolution rate, and candidate confirm/reject counts from the local `learn_events` table. Rates are null until they have a denominator; everything stays on this machine. Use it to check whether the loop is earning its keep for this CLI.

### Disabling learning

- `--no-learn` on a single command short-circuits both `recall` and the `teach` write path. Use for deterministic agent flows or tests that must not be affected by accumulated learnings.
- `GOOGLE_WORKSPACE_NO_LEARN=true` in the environment globally disables the pipeline.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
google-workspace-cli feedback "the --since flag is inclusive but docs say exclusive"
google-workspace-cli feedback --stdin < notes.txt
google-workspace-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `GOOGLE_WORKSPACE_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `GOOGLE_WORKSPACE_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

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
google-workspace-cli profile save briefing --json
google-workspace-cli --profile briefing activities list login --user-key all
google-workspace-cli profile list --json
google-workspace-cli profile show briefing
google-workspace-cli profile delete briefing --yes
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

1. **Empty, `help`, or `--help`** → show `google-workspace-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP binary (run the install script from the Prerequisites section, or see [mcp-install.md](./mcp-install.md) for per-agent wire-up).
2. Register with Claude Code:
   ```bash
   claude mcp add google-workspace-mcp -- google-workspace-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which google-workspace-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   google-workspace-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `google-workspace-cli <command> --help`.
