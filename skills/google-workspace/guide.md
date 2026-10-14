# Google Workspace Admin CLI

**Cross-client Google Workspace admin: 2SV gaps, licence waste, sharing drift, safe offboarding.**

Typed commands for the Admin SDK Directory, Reports, Enterprise License Manager, Data Transfer and Reseller methods, run per client tenant. Register each client once (a service account with domain-wide delegation impersonating that client's super admin), then ask fleet questions in one command: mfa-gaps, stale-licenses, sharing-drift, posture, leavers, oauth-apps and login-alerts fan out across every tenant. offboard runs a leaver's cleanup with suspend last (backup-code invalidation fails on a suspended account) and previews before it changes anything.

For the short install path and the MCP client wire-up see [README.md](./README.md) and [mcp-install.md](./mcp-install.md). This file is the command reference.

## Authentication

Each client tenant is a Google service account with domain-wide delegation that impersonates that client's super admin, the same model GAM uses. Register it with 'tenants add <name> --domain <d> --admin <email> --key <sa.json>' (the key is copied to keys/<name>.json in the config dir at 0600 on macOS and Linux (on Windows it inherits the folder's permissions), next to tenants.toml; both are secrets), paste the printed Client ID and scopes into that client's Admin console under Security > Access and data control > API controls > Domain-wide delegation, then run 'tenants test <name>' to mint one token per scope and name any missing delegation. Scope profiles (--scopes, comma-separated, raw scope names allowed): read is the default and covers every fan-out command, but two of its scopes have no read-only variant (admin.directory.user.security and apps.licensing), so it can also sign users out and change licences; full adds the offboarding writes (user, group membership, data transfer), and its admin.directory.user scope also allows every user write, including users delete and users make-admin; policy adds cloud-identity.policies.readonly for posture's policy columns (only full includes read, so register it as --scopes read,policy or full,policy; --scopes policy alone leaves the reports nothing to read); reseller is for your own partner domain only ('tenants add <name> --reseller --scopes reseller'), is read-only (Reseller writes also need apps.order), and is never included in --all-tenants. Schemas, calendar resources, Chrome printers and most group, OU, domain, role and device writes need extra raw scopes. Change a tenant's scopes by re-running tenants add with --yes, and print them again with 'tenants scopes <name>'. Pick a client per command with --tenant <name> or GOOGLE_WORKSPACE_TENANT, or fan out with --all-tenants. With no tenants registered, a raw bearer token in GOOGLE_WORKSPACE_TOKEN (or 'auth set-token') acts as the single tenant "default", including for --all-tenants and offboard. The MCP server reads the same tenant registry: register tenants with the CLI first (tenants add is not exposed over MCP), pass a tenant argument per tool call, and set GOOGLE_WORKSPACE_TENANT in the MCP server's env for the raw endpoint execute tool and the sql, search and context tools, which take no tenant argument; with two or more tenants registered and none selected (a single registered tenant is selected automatically), API commands and the sql, search and analytics reads refuse rather than guess a client; GOOGLE_WORKSPACE_TOKEN is not used once tenants are registered.

## Quick Start

```bash
# Confirm the binary runs (no credentials or network needed).
google-workspace-cli doctor --dry-run

# Register a client; prints the Client ID and exact scopes to authorize in that client's Admin console.
google-workspace-cli tenants add acme --domain example.com --admin admin@example.com --key ./acme-sa.json

# Mints a token per scope so a missing delegation is named precisely, and records the customer ID and domains.
google-workspace-cli tenants test acme

# One graded row per client to see where to look first.
google-workspace-cli posture --all-tenants

# The highest-severity finding in any Workspace audit: admins without 2SV.
google-workspace-cli mfa-gaps --all-tenants --admins-only

# Preview a leaver's offboarding plan with read scopes; applying it needs a tenant registered with --scopes full, then --yes.
google-workspace-cli offboard jane@example.com --successor manager@example.com --ou /Departed

```

## Unique Features

### Cross-client security
- **`mfa-gaps`**  -  Every account without 2-Step Verification across all your client tenants, ranked with unenrolled admins first.

  _Reach for this for any 'who lacks 2SV' question across more than one client; it already ranks admins as critical._

  ```bash
  google-workspace-cli mfa-gaps --all-tenants --admins-only --agent
  ```
- **`sharing-drift`**  -  Who changed Drive or Calendar settings (any setting, not only sharing) or an app's sharing settings in the Admin console, and how many files newly went external or public (up to 5 sample names per user), across clients.

  _Use it for 'what changed in sharing since last review' across clients; Drive audit needs Business Standard or higher, so run Business Starter tenants with --settings-only._

  ```bash
  google-workspace-cli sharing-drift --all-tenants --since 7d --agent
  ```
- **`posture`**  -  One row per client: 2SV coverage, admins without 2SV, stale licences and Drive, Calendar or app-sharing setting changes, plus current sharing and 2SV policy for tenants registered with the policy scope profile.

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
- **`offboard`**  -  Offboard a leaver with suspend last (backup-code invalidation fails on a suspended account), a read-only preview, dry-run, and a receipt per step.

  _Use it for any leaver: without --yes it only previews (read scopes are enough); applying needs a tenant registered with --scopes full. Over MCP it can only preview._

  ```bash
  google-workspace-cli offboard jane@example.com --successor manager@example.com --ou /Departed
  ```
- **`leavers`**  -  Suspended accounts that still hold a paid licence, groups, an admin role, OAuth tokens, or have no completed Drive transfer.

  _Run it weekly to catch offboards done by hand or in the Admin console._

  ```bash
  google-workspace-cli leavers --all-tenants --agent
  ```

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

Only Admin console setting changes (any Drive or Calendar setting, plus app sharing settings), old to new, with who made them.

### Check suspended leavers were fully closed

```bash
google-workspace-cli leavers --all-tenants --agent
```

Suspended accounts still holding licences, groups, admin roles, tokens or unfinished transfers.

### Offboard and apply

```bash
google-workspace-cli offboard jane@example.com --successor manager@example.com --ou /Departed --release-license --yes
```

Runs every step in order and returns a receipt per step. Licence release is skipped while the Drive transfer runs; the receipt's next field lists one `license-assignments delete` command per held licence, to run once `transfers list` shows the transfer complete. Use `--json` rather than `--agent` for offboard: `--agent` implies `--compact`, which drops each step's detail (the OU move, token and group names, and any error text).

## Usage

Run `google-workspace-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `GOOGLE_WORKSPACE_CONFIG_DIR`, `GOOGLE_WORKSPACE_DATA_DIR`, `GOOGLE_WORKSPACE_STATE_DIR`, or `GOOGLE_WORKSPACE_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `GOOGLE_WORKSPACE_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export GOOGLE_WORKSPACE_HOME=/srv/google-workspace
google-workspace-cli doctor
```

Under `GOOGLE_WORKSPACE_HOME=/srv/google-workspace`, the four dirs resolve to `/srv/google-workspace/config`, `/srv/google-workspace/data`, `/srv/google-workspace/state`, and `/srv/google-workspace/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `GOOGLE_WORKSPACE_DATA_DIR` overrides an explicit `--home` for that kind. Use `GOOGLE_WORKSPACE_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `GOOGLE_WORKSPACE_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `google-workspace-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

Every command below is listed with its required positional arguments. Run `google-workspace-cli <command> --help` for flags.

### activities

- **`google-workspace-cli activities list <applicationName>`** - Retrieves a list of activities for a specific customer's account and application such as the Admin console application
- **`google-workspace-cli activities watch <applicationName>`** - Start receiving notifications for account activities. For more information, see Receiving Push Notifications.

### admin-channels

- **`google-workspace-cli admin-channels`** - Stop watching resources through this channel.

### analytics

- **`google-workspace-cli analytics`** - Run analytics queries on locally synced data

### api

- **`google-workspace-cli api [interface]`** - Browse API resource interfaces by raw name

### applications

- **`google-workspace-cli applications get <applicationId>`** - Retrieves information about an application for the given application ID.
- **`google-workspace-cli applications list`** - Lists the applications available for data transfer for a customer.

### asps

- **`google-workspace-cli asps delete <userKey> <codeId>`** - Deletes an ASP issued by a user.
- **`google-workspace-cli asps get <userKey> <codeId>`** - Gets information about an ASP issued by a user.
- **`google-workspace-cli asps list <userKey>`** - Lists the ASPs issued by a user.

### auth

- **`google-workspace-cli auth logout`** - Clear stored credentials
- **`google-workspace-cli auth set-token`** - Save an API token to the credentials file
- **`google-workspace-cli auth setup`** - Print steps for obtaining a credential (use --launch to open the URL)
- **`google-workspace-cli auth status`** - Show authentication status

### channels

- **`google-workspace-cli channels`** - Stops watching resources through this channel.

### chromeosdevices

- **`google-workspace-cli chromeosdevices action <resourceId>`** - Use [BatchChangeChromeOsDeviceStatus](https://developers.google.
- **`google-workspace-cli chromeosdevices get <deviceId>`** - Retrieves a Chrome OS device's properties.
- **`google-workspace-cli chromeosdevices list`** - Retrieves a paginated list of Chrome OS devices within an account.
- **`google-workspace-cli chromeosdevices move-devices-to-ou`** - Moves or inserts multiple Chrome OS devices to an organizational unit. You can move up to 50 devices at once.
- **`google-workspace-cli chromeosdevices patch <deviceId>`** - Updates a device's updatable properties, such as `annotatedUser`, `annotatedLocation`, `notes`, `orgUnitPath`
- **`google-workspace-cli chromeosdevices update <deviceId>`** - Updates a device's updatable properties, such as `annotatedUser`, `annotatedLocation`, `notes`, `orgUnitPath`

### customer

- **`google-workspace-cli customer devices-chromeos batch-change-status`** - Changes the status of a batch of ChromeOS devices.
- **`google-workspace-cli customer devices-chromeos count-chrome-os-devices`** - Counts ChromeOS devices matching the request.
- **`google-workspace-cli customer devices-chromeos issue-command <deviceId>`** - Issues a command for the device to execute.
- **`google-workspace-cli customer devices-chromeos-commands get <deviceId> <commandId>`** - Gets command data a specific command issued to the device.

### customer-usage-reports

- **`google-workspace-cli customer-usage-reports <date>`** - Retrieves a report which is a collection of properties and statistics for a specific customer's account.

### customers

- **`google-workspace-cli customers chrome-print-servers batch-create-print-servers <parent>`** - Creates multiple print servers.
- **`google-workspace-cli customers chrome-print-servers batch-delete-print-servers <parent>`** - Deletes multiple print servers.
- **`google-workspace-cli customers chrome-print-servers create <parent>`** - Creates a print server.
- **`google-workspace-cli customers chrome-print-servers delete <name>`** - Deletes a print server.
- **`google-workspace-cli customers chrome-print-servers get <name>`** - Returns a print server's configuration.
- **`google-workspace-cli customers chrome-print-servers list <parent>`** - Lists print server configurations.
- **`google-workspace-cli customers chrome-print-servers patch <name>`** - Updates a print server's configuration.
- **`google-workspace-cli customers chrome-printers batch-create-printers <parent>`** - Creates printers under given Organization Unit.
- **`google-workspace-cli customers chrome-printers batch-delete-printers <parent>`** - Deletes printers in batch.
- **`google-workspace-cli customers chrome-printers create <parent>`** - Creates a printer under given Organization Unit.
- **`google-workspace-cli customers chrome-printers delete <name>`** - Deletes a `Printer`.
- **`google-workspace-cli customers chrome-printers get <name>`** - Returns a `Printer` resource (printer's config).
- **`google-workspace-cli customers chrome-printers list <parent>`** - List printers configs.
- **`google-workspace-cli customers chrome-printers list-printer-models <parent>`** - Lists the supported printer models.
- **`google-workspace-cli customers chrome-printers patch <name>`** - Updates a `Printer` resource.
- **`google-workspace-cli customers get`** - Retrieves a customer.
- **`google-workspace-cli customers patch`** - Patches a customer.
- **`google-workspace-cli customers update`** - Updates a customer.

### doctor

- **`google-workspace-cli doctor`** - Check CLI health

### domain-aliases

- **`google-workspace-cli domain-aliases delete <domainAliasName>`** - Deletes a domain Alias of the customer.
- **`google-workspace-cli domain-aliases get <domainAliasName>`** - Retrieves a domain alias of the customer.
- **`google-workspace-cli domain-aliases insert`** - Inserts a domain alias of the customer.
- **`google-workspace-cli domain-aliases list`** - Lists the domain aliases of the customer.

### domains

- **`google-workspace-cli domains delete <domainName>`** - Deletes a domain of the customer.
- **`google-workspace-cli domains get <domainName>`** - Retrieves a domain of the customer.
- **`google-workspace-cli domains insert`** - Inserts a domain of the customer.
- **`google-workspace-cli domains list`** - Lists the domains of the customer.

### entity-usage-reports

- **`google-workspace-cli entity-usage-reports <entityType> <entityKey> <date>`** - Retrieves a report which is a collection of properties and statistics for entities used by users within the account.

### export

- **`google-workspace-cli export <resource> [id]`** - Export data to JSONL or JSON for backup, migration, or analysis

### feedback

- **`google-workspace-cli feedback list`** - List recent feedback entries

### groups

- **`google-workspace-cli groups aliases delete <groupKey> <alias>`** - Removes an alias.
- **`google-workspace-cli groups aliases insert <groupKey>`** - Adds an alias for the group.
- **`google-workspace-cli groups aliases list <groupKey>`** - Lists all aliases for a group.
- **`google-workspace-cli groups delete <groupKey>`** - Deletes a group.
- **`google-workspace-cli groups get <groupKey>`** - Retrieves a group's properties.
- **`google-workspace-cli groups insert`** - Creates a group.
- **`google-workspace-cli groups list`** - Retrieves all groups of a domain or of a user given a userKey (paginated).
- **`google-workspace-cli groups patch <groupKey>`** - Updates a group's properties. This method supports [patch semantics](https://developers.google.
- **`google-workspace-cli groups update <groupKey>`** - Updates a group's properties.

### import

- **`google-workspace-cli import <resource>`** - Import data from JSONL file via API create/upsert calls

### learnings

- **`google-workspace-cli learnings candidates`** - List auto-captured improvement candidates awaiting judgment
- **`google-workspace-cli learnings confirm <id>`** - Confirm an open candidate: print its full payload, then materialize it
- **`google-workspace-cli learnings forget <query>`** - Delete learnings matching a query (use --all to wipe every rule for that query)
- **`google-workspace-cli learnings list`** - List recorded learnings from the local google-workspace-cli search_learnings table
- **`google-workspace-cli learnings purge`** - Delete settled candidate rows (expired and confirmed; --tombstones adds rejected)
- **`google-workspace-cli learnings reject <id>`** - Reject a candidate and tombstone its derivation signature
- **`google-workspace-cli learnings stats`** - Report learn-loop effectiveness metrics from the local events table

### leavers

- **`google-workspace-cli leavers`** - Suspended accounts still holding a licence, groups, admin roles, OAuth tokens, or no completed transfer

### license-assignments

- **`google-workspace-cli license-assignments delete <skuId> <userId>`** - Revoke a license.
- **`google-workspace-cli license-assignments get <skuId> <userId>`** - Get a specific user's license by product SKU.
- **`google-workspace-cli license-assignments insert <skuId>`** - Assign a license.
- **`google-workspace-cli license-assignments list-for-product`** - List all users assigned licenses for a specific product SKU.
- **`google-workspace-cli license-assignments list-for-product-and-sku <skuId>`** - List all users assigned licenses for a specific product SKU.
- **`google-workspace-cli license-assignments patch <skuId> <userId>`** - Reassign a user's product SKU with a different SKU in the same product. This method supports patch semantics.
- **`google-workspace-cli license-assignments update <skuId> <userId>`** - Reassign a user's product SKU with a different SKU in the same product.

### login-alerts

- **`google-workspace-cli login-alerts`** - Suspicious sign-ins, accounts Google disabled, 2SV turned off, and out-of-domain forwarding across clients

### members

- **`google-workspace-cli members delete <groupKey> <memberKey>`** - Removes a member from a group.
- **`google-workspace-cli members get <groupKey> <memberKey>`** - Retrieves a group member's properties.
- **`google-workspace-cli members has-member <groupKey> <memberKey>`** - Checks whether the given user is a member of the group.
- **`google-workspace-cli members insert <groupKey>`** - Adds a user to the specified group.
- **`google-workspace-cli members list <groupKey>`** - Retrieves a paginated list of all members in a group. This method times out after 60 minutes.
- **`google-workspace-cli members patch <groupKey> <memberKey>`** - Updates the membership properties of a user in the specified group.
- **`google-workspace-cli members update <groupKey> <memberKey>`** - Updates the membership of a user in the specified group.

### mfa-gaps

- **`google-workspace-cli mfa-gaps`** - Users without 2-Step Verification across client tenants, admins first

### mobiledevices

- **`google-workspace-cli mobiledevices action <resourceId>`** - Takes an action that affects a mobile device. For example, remotely wiping a device.
- **`google-workspace-cli mobiledevices delete <resourceId>`** - Removes a mobile device.
- **`google-workspace-cli mobiledevices get <resourceId>`** - Retrieves a mobile device's properties.
- **`google-workspace-cli mobiledevices list`** - Retrieves a paginated list of all user-owned mobile devices for an account.

### oauth-apps

- **`google-workspace-cli oauth-apps`** - Third-party OAuth apps with access to client data, across every user and tenant, ranked by scope risk

### offboard

- **`google-workspace-cli offboard <user-email>`** - Offboard a leaver in the right order: revoke access, remove groups, transfer data, suspend last

### orgunits

- **`google-workspace-cli orgunits delete <orgUnitPath>`** - Removes an organizational unit.
- **`google-workspace-cli orgunits get <orgUnitPath>`** - Retrieves an organizational unit.
- **`google-workspace-cli orgunits insert`** - Adds an organizational unit.
- **`google-workspace-cli orgunits list`** - Retrieves a list of all organizational units for an account.
- **`google-workspace-cli orgunits patch <orgUnitPath>`** - Updates an organizational unit. This method supports [patch semantics](https://developers.google.
- **`google-workspace-cli orgunits update <orgUnitPath>`** - Updates an organizational unit.

### playbook

- **`google-workspace-cli playbook amend`** - Append a note to an existing playbook (LLM-fired self-correction, silent)
- **`google-workspace-cli playbook list`** - List stored playbooks (query_family, content presence, last observed)

### posture

- **`google-workspace-cli posture`** - One row per client tenant: 2SV coverage, admins without 2SV, stale licences, Drive, Calendar and app-sharing setting changes

### privileges

- **`google-workspace-cli privileges`** - Retrieves a paginated list of all privileges for a customer.

### profile

- **`google-workspace-cli profile delete <name>`** - Remove a profile
- **`google-workspace-cli profile list`** - List saved profiles
- **`google-workspace-cli profile save <name>`** - Save the current invocation's non-default flags as a named profile
- **`google-workspace-cli profile show <name>`** - Show a profile's values as JSON
- **`google-workspace-cli profile use <name>`** - Print the flag values a profile will apply (does not execute anything)

### recall

- **`google-workspace-cli recall <query>`** - Check prior learnings for a query before running discovery (LLM-fired, pre-discovery)

### reseller-customers

- **`google-workspace-cli reseller-customers get <customerId>`** - Gets a customer account.
- **`google-workspace-cli reseller-customers insert`** - Orders a new customer's account.
- **`google-workspace-cli reseller-customers patch <customerId>`** - Updates a customer account's settings. This method supports patch semantics.
- **`google-workspace-cli reseller-customers update <customerId>`** - Updates a customer account's settings.

### resellernotify

- **`google-workspace-cli resellernotify getwatchdetails`** - Returns all the details of the watch corresponding to the reseller.
- **`google-workspace-cli resellernotify register`** - Registers a Reseller for receiving notifications.
- **`google-workspace-cli resellernotify unregister`** - Unregisters a Reseller for receiving notifications.

### resources

- **`google-workspace-cli resources buildings delete <buildingId>`** - Deletes a building.
- **`google-workspace-cli resources buildings get <buildingId>`** - Retrieves a building.
- **`google-workspace-cli resources buildings insert`** - Inserts a building.
- **`google-workspace-cli resources buildings list`** - Retrieves a list of buildings for an account.
- **`google-workspace-cli resources buildings patch <buildingId>`** - Patches a building.
- **`google-workspace-cli resources buildings update <buildingId>`** - Updates a building.
- **`google-workspace-cli resources calendars delete <calendarResourceId>`** - Deletes a calendar resource.
- **`google-workspace-cli resources calendars get <calendarResourceId>`** - Retrieves a calendar resource.
- **`google-workspace-cli resources calendars insert`** - Inserts a calendar resource.
- **`google-workspace-cli resources calendars list`** - Retrieves a list of calendar resources for an account.
- **`google-workspace-cli resources calendars patch <calendarResourceId>`** - Patches a calendar resource.
- **`google-workspace-cli resources calendars update <calendarResourceId>`** - Updates a calendar resource.
- **`google-workspace-cli resources features delete <featureKey>`** - Deletes a feature.
- **`google-workspace-cli resources features get <featureKey>`** - Retrieves a feature.
- **`google-workspace-cli resources features insert`** - Inserts a feature.
- **`google-workspace-cli resources features list`** - Retrieves a list of features for an account.
- **`google-workspace-cli resources features patch <featureKey>`** - Patches a feature.
- **`google-workspace-cli resources features rename <oldName>`** - Renames a feature.
- **`google-workspace-cli resources features update <featureKey>`** - Updates a feature.

### role-assignments

- **`google-workspace-cli role-assignments delete <roleAssignmentId>`** - Deletes a role assignment.
- **`google-workspace-cli role-assignments get <roleAssignmentId>`** - Retrieves a role assignment.
- **`google-workspace-cli role-assignments insert`** - Creates a role assignment.
- **`google-workspace-cli role-assignments list`** - Retrieves a paginated list of all roleAssignments.

### roles

- **`google-workspace-cli roles delete <roleId>`** - Deletes a role.
- **`google-workspace-cli roles get <roleId>`** - Retrieves a role.
- **`google-workspace-cli roles insert`** - Creates a role.
- **`google-workspace-cli roles list`** - Retrieves a paginated list of all the roles in a domain.
- **`google-workspace-cli roles patch <roleId>`** - Patches a role.
- **`google-workspace-cli roles update <roleId>`** - Updates a role.

### schemas

- **`google-workspace-cli schemas delete <schemaKey>`** - Deletes a schema.
- **`google-workspace-cli schemas get <schemaKey>`** - Retrieves a schema.
- **`google-workspace-cli schemas insert`** - Creates a schema.
- **`google-workspace-cli schemas list`** - Retrieves all schemas for a customer.
- **`google-workspace-cli schemas patch <schemaKey>`** - Patches a schema.
- **`google-workspace-cli schemas update <schemaKey>`** - Updates a schema.

### search

- **`google-workspace-cli search <query>`** - Search locally synced data

### sharing-drift

- **`google-workspace-cli sharing-drift`** - External-sharing changes across client tenants: Drive, Calendar and app-sharing setting changes, new external shares, public links

### stale-licenses

- **`google-workspace-cli stale-licenses`** - Paid licences held by suspended, never-used, or inactive accounts across client tenants

### subscriptions

- **`google-workspace-cli subscriptions activate <customerId> <subscriptionId>`** - Activates a subscription previously suspended by the reseller.
- **`google-workspace-cli subscriptions change-plan <customerId> <subscriptionId>`** - Updates a subscription plan.
- **`google-workspace-cli subscriptions change-renewal-settings <customerId> <subscriptionId>`** - Updates a user license's renewal settings. This is applicable for accounts with annual commitment plans only.
- **`google-workspace-cli subscriptions change-seats <customerId> <subscriptionId>`** - Updates a subscription's user license settings.
- **`google-workspace-cli subscriptions delete <customerId> <subscriptionId>`** - Cancels, suspends, or transfers a subscription to direct.
- **`google-workspace-cli subscriptions get <customerId> <subscriptionId>`** - Gets a specific subscription.
- **`google-workspace-cli subscriptions insert <customerId>`** - Creates or transfer a subscription.
- **`google-workspace-cli subscriptions list`** - Lists of subscriptions managed by the reseller.
- **`google-workspace-cli subscriptions start-paid-service <customerId> <subscriptionId>`** - Immediately move a 30-day free trial subscription to a paid service subscription.
- **`google-workspace-cli subscriptions suspend <customerId> <subscriptionId>`** - Suspends an active subscription.

### sync

- **`google-workspace-cli sync`** - Sync API data to local SQLite for offline search and analysis

### tail

- **`google-workspace-cli tail [resource]`** - Stream live changes by polling the API at regular intervals

### teach

- **`google-workspace-cli teach`** - Record a query -> resource mapping for future recall (LLM-fired, silent)

### teach-lookup

- **`google-workspace-cli teach-lookup`** - Install a manual entity-lookup row (kind, canonical, value)

### teach-pattern

- **`google-workspace-cli teach-pattern`** - Install a manual generalization pattern (query_template, resource_template, entity_kind)

### teach-playbook

- **`google-workspace-cli teach-playbook`** - Record a CLI playbook + free-text notes for a query family

### tenants

- **`google-workspace-cli tenants add <name>`** - Register a client tenant (copies its service-account key into the config dir, mode 0600 on macOS/Linux)
- **`google-workspace-cli tenants list`** - List registered client tenants
- **`google-workspace-cli tenants remove <name>`** - Remove a tenant and delete its stored service-account key copy
- **`google-workspace-cli tenants scopes <name>`** - Print the Client ID and OAuth scopes to authorize in the client's Admin console
- **`google-workspace-cli tenants test [name]`** - Verify each tenant's domain-wide delegation scope by scope, and record its customer ID and domains

### tokens

- **`google-workspace-cli tokens delete <userKey> <clientId>`** - Deletes all access tokens issued by a user for an application.
- **`google-workspace-cli tokens get <userKey> <clientId>`** - Gets information about an access token issued by a user.
- **`google-workspace-cli tokens list <userKey>`** - Returns the set of tokens specified user has issued to 3rd party applications.

### transfers

- **`google-workspace-cli transfers get <dataTransferId>`** - Retrieves a data transfer request by its resource ID.
- **`google-workspace-cli transfers insert`** - Inserts a data transfer request. See the [Transfer parameters](https://developers.google.
- **`google-workspace-cli transfers list`** - Lists the transfers for a customer by source user, destination user, or status.

### two-step-verification

- **`google-workspace-cli two-step-verification <userKey>`** - Turns off 2-Step Verification for user.

### user-usage-report

- **`google-workspace-cli user-usage-report <date>`** - Retrieves a report which is a collection of properties and statistics for a set of users with the account.

### users

- **`google-workspace-cli users aliases delete <userKey> <alias>`** - Removes an alias.
- **`google-workspace-cli users aliases insert <userKey>`** - Adds an alias.
- **`google-workspace-cli users aliases list <userKey>`** - Lists all aliases for a user.
- **`google-workspace-cli users aliases watch <userKey>`** - Watches for changes in users list.
- **`google-workspace-cli users create-guest`** - Create a guest user with access to a [subset of Workspace capabilities](https://support.google.com/a/answer/16558545).
- **`google-workspace-cli users delete <userKey>`** - Deletes a user.
- **`google-workspace-cli users get <userKey>`** - Retrieves a user.
- **`google-workspace-cli users insert`** - Creates a user.
- **`google-workspace-cli users list`** - Retrieves a paginated list of either deleted users or all users in a domain.
- **`google-workspace-cli users make-admin <userKey>`** - Makes a user a super administrator.
- **`google-workspace-cli users patch <userKey>`** - Updates a user using patch semantics.
- **`google-workspace-cli users photos delete <userKey>`** - Removes the user's photo.
- **`google-workspace-cli users photos get <userKey>`** - Retrieves the user's photo.
- **`google-workspace-cli users photos patch <userKey>`** - Adds a photo for the user. This method supports [patch semantics](https://developers.google.
- **`google-workspace-cli users photos update <userKey>`** - Adds a photo for the user.
- **`google-workspace-cli users sign-out <userKey>`** - Signs a user out of all web and device sessions and reset their sign-in cookies.
- **`google-workspace-cli users undelete <userKey>`** - Undeletes a deleted user.
- **`google-workspace-cli users update <userKey>`** - Updates a user.
- **`google-workspace-cli users watch`** - Watches for changes in users list.

### verification-codes

- **`google-workspace-cli verification-codes generate <userKey>`** - Generates new backup verification codes for the user.
- **`google-workspace-cli verification-codes invalidate <userKey>`** - Invalidates the current backup verification codes for the user.
- **`google-workspace-cli verification-codes list <userKey>`** - Returns the current set of valid backup verification codes for the specified user.

### version

- **`google-workspace-cli version`** - Print version

### which

- **`google-workspace-cli which [query]`** - Find the command that implements a capability

### workflow

- **`google-workspace-cli workflow archive`** - Sync all resources to local store for offline access and search
- **`google-workspace-cli workflow status`** - Show local archive status and sync state for all resources

### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`google-workspace-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`google-workspace-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`google-workspace-cli learnings list`** - Inspect taught rows
- **`google-workspace-cli learnings forget <query>`** - Undo a teach
- **`google-workspace-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`google-workspace-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`google-workspace-cli teach-pattern`** - Install a query/resource template up front
- **`google-workspace-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `GOOGLE_WORKSPACE_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `google-workspace-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
google-workspace-cli activities list login --user-key all

# JSON for scripting and agents
google-workspace-cli activities list login --user-key all --json
# Filter to specific fields by name
google-workspace-cli activities list login --user-key all --json --select <field>[,<field>...]

# Dry run  -  show the request without sending
google-workspace-cli activities list login --user-key all --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
google-workspace-cli activities list login --user-key all --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.


Cross-client commands exit 6 when some tenants fail but still print the other tenants' results with `meta.fetch_failures`; pass `--allow-partial-failure` (over MCP, `allow-partial-failure`) to exit 0 with those partial results.

## Health Check

```bash
google-workspace-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `google-workspace-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/google-workspace-cli/config.toml`; `--home`, `GOOGLE_WORKSPACE_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `GOOGLE_WORKSPACE_TENANT` | per_call | No | Client tenant to act as (a name from `tenants list`), same as `--tenant`. Required for single-tenant commands when more than one tenant is registered. |
| `GOOGLE_WORKSPACE_TOKEN` | per_call | No | Single-organization bearer token, only when no client tenants are registered. |
| `ADMIN_ACCESS_TOKEN` | per_call | No | Single-organization bearer token, only when no client tenants are registered. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `google-workspace-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `google-workspace-cli doctor` to check credentials
- Verify the environment variable is set without printing it: `[ -n "$GOOGLE_WORKSPACE_TOKEN" ] && echo set || echo unset`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **unauthorized_client: Client is unauthorized to retrieve access tokens using this method**  -  Run 'tenants scopes <tenant>' and add the printed Client ID and scopes under Admin console > Security > Access and data control > API controls > Domain-wide delegation, then 'tenants test <tenant>'.
- **invalid_grant: Not a valid email or user ID**  -  The --admin email must be an existing super admin in that client's domain; re-register with 'tenants add <name> ... --yes'.
- **doctor reports auth not configured, or a command exits 4**  -  Run 'tenants test <tenant>' (or 'tenants test --all-tenants'); it names the exact missing delegation per scope.
- **Creating a service-account key fails with iam.disableServiceAccountKeyCreation**  -  Google Cloud orgs created after 2024-05-03 block key creation by default; the client's org admin must override that policy for the project first.
- **sharing-drift fails a tenant on the Drive audit query**  -  Drive audit events need Business Standard or higher; run that tenant with --settings-only.
- **A fan-out exits 6 with 'N of M tenant(s) failed'**  -  Results cover the healthy tenants and meta.fetch_failures names the rest; fix the named tenant with 'tenants test <tenant>', or pass --allow-partial-failure to exit 0. Exit 5 means every tenant failed.
- **A tenant fails with 'scan capped' or 'has more than N item(s)'**  -  Raise --max-scan-pages, --max-events or --max-users for that command, or narrow --since.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**GAM7**](https://github.com/GAM-team/GAM)  -  Python (4337 stars)
- [**ScubaGoggles**](https://github.com/cisagov/ScubaGoggles)  -  Python (378 stars)
- [**Google Workspace offboarding scripts**](https://github.com/PaulOgier/GoogleWorkspaceScripts)  -  Python (28 stars)
- [**mcp-gam**](https://github.com/dbarks/mcp-gam)  -  TypeScript (5 stars)
- [**gws-admin-mcp**](https://github.com/antct11/gws-admin-mcp)  -  TypeScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
