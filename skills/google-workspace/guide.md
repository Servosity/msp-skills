# Google Workspace Admin CLI

**Cross-client Google Workspace admin: 2SV gaps, licence waste, sharing drift, safe offboarding.**

Typed commands for the Admin SDK Directory, Reports, Enterprise License Manager, Data Transfer and Reseller methods, run per client tenant. Register each client once (a service account with domain-wide delegation impersonating that client's super admin), then ask fleet questions in one command: mfa-gaps, stale-licenses, sharing-drift, posture, leavers, oauth-apps and login-alerts fan out across every tenant. offboard runs a leaver in the order Google's API requires and previews before it changes anything.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `google-workspace-cli` and `google-workspace-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=google-workspace). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/google-workspace --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/google-workspace --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `google-workspace-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the google-workspace skill from https://github.com/Servosity/msp-skills/tree/main/skills/google-workspace. The skill defines how its required CLI (`google-workspace-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=google-workspace).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `GOOGLE_WORKSPACE_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/google-workspace/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/google-workspace/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "google-workspace": {
      "command": "google-workspace-mcp",
      "env": {
        "GOOGLE_WORKSPACE_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Each client tenant is a Google service account with domain-wide delegation that impersonates that client's super admin, the same model GAM uses. Register it with 'tenants add <name> --domain <d> --admin <email> --key <sa.json>' (the key is copied to keys/<name>.json in the config dir at 0600, next to tenants.toml; both are secrets), paste the printed Client ID and scopes into that client's Admin console under Security > Access and data control > API controls > Domain-wide delegation, then run 'tenants test <name>' to mint one token per scope and name any missing delegation. Scope profiles (--scopes, comma-separated, raw scope names allowed): read is the default and covers every fan-out command, but two of its scopes have no read-only variant (admin.directory.user.security and apps.licensing), so it can also sign users out and change licences; full adds the offboarding writes (user, group membership, data transfer); policy adds cloud-identity.policies.readonly for posture's policy columns; reseller is for your own partner domain only ('tenants add <name> --reseller --scopes reseller'), is read-only (Reseller writes also need apps.order), and is never included in --all-tenants. Schemas, calendar resources, Chrome printers and most group, OU, domain, role and device writes need extra raw scopes. Change a tenant's scopes by re-running tenants add with --yes, and print them again with 'tenants scopes <name>'. Pick a client per command with --tenant <name> or GOOGLE_WORKSPACE_TENANT, or fan out with --all-tenants. With no tenants registered, a raw bearer token in GOOGLE_WORKSPACE_TOKEN (or 'auth set-token') acts as the single tenant "default", including for --all-tenants and offboard. The MCP server reads the same tenant registry: register tenants with the CLI first (tenants add is not exposed over MCP), pass a tenant argument per tool call, and set GOOGLE_WORKSPACE_TENANT in the MCP server's env for raw endpoint execution.

## Quick Start

```bash
# Confirm the CLI is installed and wired, without credentials or network.
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
- **`offboard`**  -  Offboard a leaver in the order Google's API requires, with a read-only preview, dry-run, and a receipt per step.

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

Runs every step in order and returns a receipt per step. Licence release is skipped while the Drive transfer runs; the receipt's next field prints the follow-up command to release it once the transfer completes.

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

### activities



- **`google-workspace-cli activities list`** - Retrieves a list of activities for a specific customer's account and application such as the Admin console application or the Google Drive application. For more information, see the guides for administrator and Google Drive activity reports. For more information about the activity report's parameters, see the activity parameters reference guides. 
- **`google-workspace-cli activities watch`** - Start receiving notifications for account activities. For more information, see Receiving Push Notifications.

### admin-channels



- **`google-workspace-cli admin-channels`** - Stop watching resources through this channel.

### applications



- **`google-workspace-cli applications get`** - Retrieves information about an application for the given application ID.
- **`google-workspace-cli applications list`** - Lists the applications available for data transfer for a customer.

### asps



- **`google-workspace-cli asps delete`** - Deletes an ASP issued by a user.
- **`google-workspace-cli asps get`** - Gets information about an ASP issued by a user.
- **`google-workspace-cli asps list`** - Lists the ASPs issued by a user.

### channels



- **`google-workspace-cli channels`** - Stops watching resources through this channel.

### chromeosdevices



- **`google-workspace-cli chromeosdevices action`** - Use [BatchChangeChromeOsDeviceStatus](https://developers.google.com/workspace/admin/directory/reference/rest/v1/customer.devices.chromeos/batchChangeStatus) instead. Takes an action that affects a Chrome OS Device. This includes deprovisioning, disabling, and re-enabling devices. *Warning:* * Deprovisioning a device will stop device policy syncing and remove device-level printers. After a device is deprovisioned, it must be wiped before it can be re-enrolled. * Lost or stolen devices should use the disable action. * Re-enabling a disabled device will consume a device license. If you do not have sufficient licenses available when completing the re-enable action, you will receive an error. For more information about deprovisioning and disabling devices, visit the [help center](https://support.google.com/chrome/a/answer/3523633).
- **`google-workspace-cli chromeosdevices get`** - Retrieves a Chrome OS device's properties.
- **`google-workspace-cli chromeosdevices list`** - Retrieves a paginated list of Chrome OS devices within an account.
- **`google-workspace-cli chromeosdevices move-devices-to-ou`** - Moves or inserts multiple Chrome OS devices to an organizational unit. You can move up to 50 devices at once.
- **`google-workspace-cli chromeosdevices patch`** - Updates a device's updatable properties, such as `annotatedUser`, `annotatedLocation`, `notes`, `orgUnitPath`, or `annotatedAssetId`. This method supports [patch semantics](https://developers.google.com/workspace/admin/directory/v1/guides/performance#patch).
- **`google-workspace-cli chromeosdevices update`** - Updates a device's updatable properties, such as `annotatedUser`, `annotatedLocation`, `notes`, `orgUnitPath`, or `annotatedAssetId`.

### customer




### customerUsageReports



- **`google-workspace-cli customer-usage-reports <date>`** - Retrieves a report which is a collection of properties and statistics for a specific customer's account. For more information, see the Customers Usage Report guide. For more information about the customer report's parameters, see the Customers Usage parameters reference guides. 

### customers



- **`google-workspace-cli customers get`** - Retrieves a customer.
- **`google-workspace-cli customers patch`** - Patches a customer.
- **`google-workspace-cli customers update`** - Updates a customer.

### domainAliases



- **`google-workspace-cli domain-aliases delete`** - Deletes a domain Alias of the customer.
- **`google-workspace-cli domain-aliases get`** - Retrieves a domain alias of the customer.
- **`google-workspace-cli domain-aliases insert`** - Inserts a domain alias of the customer.
- **`google-workspace-cli domain-aliases list`** - Lists the domain aliases of the customer.

### domains



- **`google-workspace-cli domains delete`** - Deletes a domain of the customer.
- **`google-workspace-cli domains get`** - Retrieves a domain of the customer.
- **`google-workspace-cli domains insert`** - Inserts a domain of the customer.
- **`google-workspace-cli domains list`** - Lists the domains of the customer.

### entityUsageReports



- **`google-workspace-cli entity-usage-reports <entityType> <entityKey> <date>`** - Retrieves a report which is a collection of properties and statistics for entities used by users within the account. For more information, see the Entities Usage Report guide. For more information about the entities report's parameters, see the Entities Usage parameters reference guides.

### groups



- **`google-workspace-cli groups delete`** - Deletes a group.
- **`google-workspace-cli groups get`** - Retrieves a group's properties.
- **`google-workspace-cli groups insert`** - Creates a group.
- **`google-workspace-cli groups list`** - Retrieves all groups of a domain or of a user given a userKey (paginated).
- **`google-workspace-cli groups patch`** - Updates a group's properties. This method supports [patch semantics](https://developers.google.com/workspace/admin/directory/v1/guides/performance#patch).
- **`google-workspace-cli groups update`** - Updates a group's properties.

### licenseAssignments



- **`google-workspace-cli license-assignments delete`** - Revoke a license.
- **`google-workspace-cli license-assignments get`** - Get a specific user's license by product SKU.
- **`google-workspace-cli license-assignments insert`** - Assign a license.
- **`google-workspace-cli license-assignments list-for-product`** - List all users assigned licenses for a specific product SKU.
- **`google-workspace-cli license-assignments list-for-product-and-sku`** - List all users assigned licenses for a specific product SKU.
- **`google-workspace-cli license-assignments patch`** - Reassign a user's product SKU with a different SKU in the same product. This method supports patch semantics.
- **`google-workspace-cli license-assignments update`** - Reassign a user's product SKU with a different SKU in the same product.

### members



- **`google-workspace-cli members delete`** - Removes a member from a group.
- **`google-workspace-cli members get`** - Retrieves a group member's properties.
- **`google-workspace-cli members has-member`** - Checks whether the given user is a member of the group. Membership can be direct or nested, but if nested, the `memberKey` and `groupKey` must be entities in the same domain or an `Invalid input` error is returned. To check for nested memberships that include entities outside of the group's domain, use the [`checkTransitiveMembership()`](https://cloud.google.com/identity/docs/reference/rest/v1/groups.memberships/checkTransitiveMembership) method in the Cloud Identity Groups API.
- **`google-workspace-cli members insert`** - Adds a user to the specified group.
- **`google-workspace-cli members list`** - Retrieves a paginated list of all members in a group. This method times out after 60 minutes. For more information, see [Troubleshoot error codes](https://developers.google.com/workspace/admin/directory/v1/guides/troubleshoot-error-codes).
- **`google-workspace-cli members patch`** - Updates the membership properties of a user in the specified group. This method supports [patch semantics](https://developers.google.com/workspace/admin/directory/v1/guides/performance#patch).
- **`google-workspace-cli members update`** - Updates the membership of a user in the specified group.

### mobiledevices



- **`google-workspace-cli mobiledevices action`** - Takes an action that affects a mobile device. For example, remotely wiping a device.
- **`google-workspace-cli mobiledevices delete`** - Removes a mobile device.
- **`google-workspace-cli mobiledevices get`** - Retrieves a mobile device's properties.
- **`google-workspace-cli mobiledevices list`** - Retrieves a paginated list of all user-owned mobile devices for an account. To retrieve a list that includes company-owned devices, use the Cloud Identity [Devices API](https://cloud.google.com/identity/docs/concepts/overview-devices) instead. This method times out after 60 minutes. For more information, see [Troubleshoot error codes](https://developers.google.com/workspace/admin/directory/v1/guides/troubleshoot-error-codes).

### orgunits



- **`google-workspace-cli orgunits delete`** - Removes an organizational unit.
- **`google-workspace-cli orgunits get`** - Retrieves an organizational unit.
- **`google-workspace-cli orgunits insert`** - Adds an organizational unit.
- **`google-workspace-cli orgunits list`** - Retrieves a list of all organizational units for an account.
- **`google-workspace-cli orgunits patch`** - Updates an organizational unit. This method supports [patch semantics](https://developers.google.com/workspace/admin/directory/v1/guides/performance#patch)
- **`google-workspace-cli orgunits update`** - Updates an organizational unit.

### privileges



- **`google-workspace-cli privileges`** - Retrieves a paginated list of all privileges for a customer.

### reseller-customers



- **`google-workspace-cli reseller-customers get`** - Gets a customer account. Use this operation to see a customer account already in your reseller management, or to see the minimal account information for an existing customer that you do not manage. For more information about the API response for existing customers, see [retrieving a customer account](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_customers#get_customer).
- **`google-workspace-cli reseller-customers insert`** - Orders a new customer's account. Before ordering a new customer account, establish whether the customer account already exists using the [`customers.get`](https://developers.google.com/workspace/admin/reseller/v1/reference/customers/get) If the customer account exists as a direct Google account or as a resold customer account from another reseller, use the `customerAuthToken\` as described in [order a resold account for an existing customer](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_customers#create_existing_customer). For more information about ordering a new customer account, see [order a new customer account](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_customers#create_customer). After creating a new customer account, you must provision a user as an administrator. The customer's administrator is required to sign in to the Admin console and sign the G Suite via Reseller agreement to activate the account. Resellers are prohibited from signing the G Suite via Reseller agreement on the customer's behalf. For more information, see [order a new customer account](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_customers#tos).
- **`google-workspace-cli reseller-customers patch`** - Updates a customer account's settings. This method supports patch semantics. You cannot update `customerType` via the Reseller API, but a `"team"` customer can verify their domain and become `customerType = "domain"`. For more information, see [Verify your domain to unlock Essentials features](https://support.google.com/a/answer/9122284).
- **`google-workspace-cli reseller-customers update`** - Updates a customer account's settings. You cannot update `customerType` via the Reseller API, but a `"team"` customer can verify their domain and become `customerType = "domain"`. For more information, see [update a customer's settings](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_customers#update_customer).

### resellernotify



- **`google-workspace-cli resellernotify getwatchdetails`** - Returns all the details of the watch corresponding to the reseller.
- **`google-workspace-cli resellernotify register`** - Registers a Reseller for receiving notifications.
- **`google-workspace-cli resellernotify unregister`** - Unregisters a Reseller for receiving notifications.

### resources




### roleAssignments



- **`google-workspace-cli role-assignments delete`** - Deletes a role assignment.
- **`google-workspace-cli role-assignments get`** - Retrieves a role assignment.
- **`google-workspace-cli role-assignments insert`** - Creates a role assignment.
- **`google-workspace-cli role-assignments list`** - Retrieves a paginated list of all roleAssignments.

### roles



- **`google-workspace-cli roles delete`** - Deletes a role.
- **`google-workspace-cli roles get`** - Retrieves a role.
- **`google-workspace-cli roles insert`** - Creates a role.
- **`google-workspace-cli roles list`** - Retrieves a paginated list of all the roles in a domain.
- **`google-workspace-cli roles patch`** - Patches a role.
- **`google-workspace-cli roles update`** - Updates a role.

### schemas



- **`google-workspace-cli schemas delete`** - Deletes a schema.
- **`google-workspace-cli schemas get`** - Retrieves a schema.
- **`google-workspace-cli schemas insert`** - Creates a schema.
- **`google-workspace-cli schemas list`** - Retrieves all schemas for a customer.
- **`google-workspace-cli schemas patch`** - Patches a schema.
- **`google-workspace-cli schemas update`** - Updates a schema.

### subscriptions



- **`google-workspace-cli subscriptions activate`** - Activates a subscription previously suspended by the reseller. If you did not suspend the customer subscription and it is suspended for any other reason, such as for abuse or a pending ToS acceptance, this call will not reactivate the customer subscription.
- **`google-workspace-cli subscriptions change-plan`** - Updates a subscription plan. Use this method to update a plan for a 30-day trial or a flexible plan subscription to an annual commitment plan with monthly or yearly payments. How a plan is updated differs depending on the plan and the products. For more information, see the description in [manage subscriptions](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_subscriptions#update_subscription_plan).
- **`google-workspace-cli subscriptions change-renewal-settings`** - Updates a user license's renewal settings. This is applicable for accounts with annual commitment plans only. For more information, see the description in [manage subscriptions](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_subscriptions#update_renewal).
- **`google-workspace-cli subscriptions change-seats`** - Updates a subscription's user license settings. For more information about updating an annual commitment plan or a flexible plan subscription’s licenses, see [Manage Subscriptions](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_subscriptions#update_subscription_seat).
- **`google-workspace-cli subscriptions delete`** - Cancels, suspends, or transfers a subscription to direct.
- **`google-workspace-cli subscriptions get`** - Gets a specific subscription. The `subscriptionId` can be found using the [Retrieve all reseller subscriptions](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_subscriptions#get_all_subscriptions) method. For more information about retrieving a specific subscription, see the information descrived in [manage subscriptions](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_subscriptions#get_subscription).
- **`google-workspace-cli subscriptions insert`** - Creates or transfer a subscription. Create a subscription for a customer's account that you ordered using the [Order a new customer account](https://developers.google.com/workspace/admin/reseller/v1/reference/customers/insert.html) method. For more information about creating a subscription for different payment plans, see [manage subscriptions](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_subscriptions#create_subscription).\ If you did not order the customer's account using the customer insert method, use the customer's `customerAuthToken` when creating a subscription for that customer. If transferring a G Suite subscription with an associated Google Drive or Google Vault subscription, use the [batch operation](https://developers.google.com/workspace/admin/reseller/v1/how-tos/batch.html) to transfer all of these subscriptions. For more information, see how to [transfer subscriptions](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_subscriptions#transfer_a_subscription).
- **`google-workspace-cli subscriptions list`** - Lists of subscriptions managed by the reseller. The list can be all subscriptions, all of a customer's subscriptions, or all of a customer's transferable subscriptions. Optionally, this method can filter the response by a `customerNamePrefix`. For more information, see [manage subscriptions](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_subscriptions).
- **`google-workspace-cli subscriptions start-paid-service`** - Immediately move a 30-day free trial subscription to a paid service subscription. This method is only applicable if a payment plan has already been set up for the 30-day trial subscription. For more information, see [manage subscriptions](https://developers.google.com/workspace/admin/reseller/v1/how-tos/manage_subscriptions#paid_service).
- **`google-workspace-cli subscriptions suspend`** - Suspends an active subscription. You can use this method to suspend a paid subscription that is currently in the `ACTIVE` state. * For `FLEXIBLE` subscriptions, billing is paused. * For `ANNUAL_MONTHLY_PAY` or `ANNUAL_YEARLY_PAY` subscriptions: * Suspending the subscription does not change the renewal date that was originally committed to. * A suspended subscription does not renew. If you activate the subscription after the original renewal date, a new annual subscription will be created, starting on the day of activation. We strongly encourage you to suspend subscriptions only for short periods of time as suspensions over 60 days may result in the subscription being cancelled.

### tokens



- **`google-workspace-cli tokens delete`** - Deletes all access tokens issued by a user for an application.
- **`google-workspace-cli tokens get`** - Gets information about an access token issued by a user.
- **`google-workspace-cli tokens list`** - Returns the set of tokens specified user has issued to 3rd party applications.

### transfers



- **`google-workspace-cli transfers get`** - Retrieves a data transfer request by its resource ID.
- **`google-workspace-cli transfers insert`** - Inserts a data transfer request. See the [Transfer parameters](https://developers.google.com/workspace/admin/data-transfer/v1/parameters) reference for specific application requirements.
- **`google-workspace-cli transfers list`** - Lists the transfers for a customer by source user, destination user, or status.

### twoStepVerification



- **`google-workspace-cli two-step-verification <userKey>`** - Turns off 2-Step Verification for user.

### userUsageReport



- **`google-workspace-cli user-usage-report <date>`** - Retrieves a report which is a collection of properties and statistics for a set of users with the account. For more information, see the User Usage Report guide. For more information about the user report's parameters, see the Users Usage parameters reference guides.

### users



- **`google-workspace-cli users create-guest`** - Create a guest user with access to a [subset of Workspace capabilities](https://support.google.com/a/answer/16558545). This feature is currently in Open Beta.
- **`google-workspace-cli users delete`** - Deletes a user.
- **`google-workspace-cli users get`** - Retrieves a user.
- **`google-workspace-cli users insert`** - Creates a user. Mutate calls immediately following user creation might sometimes fail as the user isn't fully created due to propagation delay in our backends. Check the error details for the "User creation is not complete" message to see if this is the case. Retrying the calls after some time can help in this case. If `resolve_conflict_account` is set to `true`, the option selected for [Find and add unmanaged users](https://knowledge.workspace.google.com/admin/users/find-and-add-unmanaged-users) will apply to resolve conflicting accounts: - A `200` response code indicates the user was created (or replaced an existing unmanaged personal account). - A `202` response code means that a conflicting unmanaged personal account exists and was invited to join the organization. - A `409` response code means that a conflicting account exists so the user wasn't created (e.g. based on the option selected to preserve the account, or if the email conflicts with an unmanaged work account or existing managed user). For details on resolving duplicate account errors, see [Resolve duplicate account errors](https://knowledge.workspace.google.com/p/duplicate-account-errors) and [Transfer unmanaged work accounts](https://knowledge.workspace.google.com/p/unmanaged-work-accounts).
- **`google-workspace-cli users list`** - Retrieves a paginated list of either deleted users or all users in a domain.
- **`google-workspace-cli users make-admin`** - Makes a user a super administrator.
- **`google-workspace-cli users patch`** - Updates a user using patch semantics. The update method should be used instead, because it also supports patch semantics and has better performance. If you're mapping an external identity to a Google identity, use the [`update`](https://developers.google.com/workspace/admin/directory/v1/reference/users/update) method instead of the `patch` method. This method is unable to clear fields that contain repeated objects (`addresses`, `phones`, etc). Use the update method instead.
- **`google-workspace-cli users sign-out`** - Signs a user out of all web and device sessions and reset their sign-in cookies. User will have to sign in by authenticating again.
- **`google-workspace-cli users undelete`** - Undeletes a deleted user.
- **`google-workspace-cli users update`** - Updates a user. This method supports patch semantics, meaning that you only need to include the fields you wish to update. Fields that are not present in the request will be preserved, and fields set to `null` will be cleared. For repeating fields that contain arrays, individual items in the array can't be patched piecemeal; they must be supplied in the request body with the desired values for all items. See the [user accounts guide](https://developers.google.com/workspace/admin/directory/v1/guides/manage-users#update_user) for more information.
- **`google-workspace-cli users watch`** - Watches for changes in users list.

### verificationCodes



- **`google-workspace-cli verification-codes generate`** - Generates new backup verification codes for the user.
- **`google-workspace-cli verification-codes invalidate`** - Invalidates the current backup verification codes for the user.
- **`google-workspace-cli verification-codes list`** - Returns the current set of valid backup verification codes for the specified user.


### leavers


- **`google-workspace-cli leavers`** - Suspended accounts that still hold a paid licence, groups, an admin role, OAuth tokens, or have no completed Drive transfer.


### login-alerts


- **`google-workspace-cli login-alerts`** - Suspicious sign-ins, accounts Google disabled, 2SV turned off, and out-of-domain mail forwarding across clients.


### mfa-gaps


- **`google-workspace-cli mfa-gaps`** - Every account without 2-Step Verification across all your client tenants, ranked with unenrolled admins first.


### oauth-apps


- **`google-workspace-cli oauth-apps`** - Third-party apps with access to client data across every active user and tenant, ranked by scope risk; --since switches to apps newly authorized in that window.


### offboard


- **`google-workspace-cli offboard`** - Offboard a leaver in the order Google's API requires, with a read-only preview, dry-run, and a receipt per step.


### posture


- **`google-workspace-cli posture`** - One row per client: 2SV coverage, admins without 2SV, stale licences and sharing changes, plus current sharing and 2SV policy for tenants registered with the policy scope profile.


### sharing-drift


- **`google-workspace-cli sharing-drift`** - Who changed Drive or Calendar sharing settings, and which files newly went external or public, across clients.


### stale-licenses


- **`google-workspace-cli stale-licenses`** - Paid licences held by suspended, never-signed-in, or idle accounts, per client, ready to reclaim.




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
| `GOOGLE_WORKSPACE_TOKEN` | per_call | No | Set to your API credential. |
| `ADMIN_ACCESS_TOKEN` | per_call | No | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `google-workspace-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `google-workspace-cli doctor` to check credentials
- Verify the environment variable is set: `echo $GOOGLE_WORKSPACE_TOKEN`
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
