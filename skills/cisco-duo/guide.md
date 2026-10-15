# Cisco Duo CLI

**Fix Duo MFA lockouts in one command and see every MSP subaccount at once, with signed Admin API calls.**

The Duo Admin API calls an MSP help desk uses (users, phones, bypass codes, groups, hardware tokens, admins, authentication logs and MSP subaccounts) signed with Duo v5 HMAC, plus read access to integrations, policies and settings. --account-id routes any command to one customer subaccount. On top: lockout triage and lockout fix for the 'locked out of MFA' ticket, phone-swap for lost phones, fleet summary and fleet risk across all customers, and local audits of who can skip MFA.

Learn more at [Cisco Duo](https://duo.com/docs/adminapi).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `cisco-duo-cli` and `cisco-duo-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-duo/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-duo/install.ps1 | iex
   ```
3. Verify: `cisco-duo-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=cisco-duo). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/cisco-duo --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/cisco-duo --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `cisco-duo-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the cisco-duo skill from https://github.com/Servosity/msp-skills/tree/main/skills/cisco-duo. The skill defines how its required CLI (`cisco-duo-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=cisco-duo).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `CISCO_DUO_IKEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-duo/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/cisco-duo/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "cisco-duo": {
      "command": "cisco-duo-mcp",
      "env": {
        "CISCO_DUO_API_HOST": "<api_host>",
        "CISCO_DUO_IKEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Duo signs every Admin API request with HMAC-SHA512. In the Duo Admin Panel create an Admin API application (in the MSP parent account, with subaccount permissions, to manage customers) and set three variables: CISCO_DUO_IKEY (integration key), CISCO_DUO_SKEY (secret key) and CISCO_DUO_API_HOST (the API hostname shown there, e.g. api-xxxxxxxx.duosecurity.com). The CLI computes a fresh signature for every request and never sends the secret key. Pass --account-id (an account ID or exact name from 'accounts list'), or set CISCO_DUO_ACCOUNT_ID, to run any command against one customer subaccount; the CLI looks up that subaccount's API host and keeps a separate local store for it.

## Quick Start

```bash
# Verify-safe first step; run 'cisco-duo-cli doctor' without --dry-run to check your keys and API host
cisco-duo-cli doctor --dry-run

# Lists customer subaccounts and their account_id values (MSP parent apps only)
cisco-duo-cli accounts list --agent

# Finds which customer tenant the caller is in
cisco-duo-cli fleet find-user jdoe@example.com --agent

# Diagnoses the lockout and names the fix
cisco-duo-cli lockout triage jdoe --account-id DA9VZOC5X63I2W72NRP9 --agent

# Previews the unlock plus a single-use bypass code; re-run with --apply to make the changes
cisco-duo-cli lockout fix jdoe --account-id DA9VZOC5X63I2W72NRP9 --bypass

```

## Unique Features

### Lockout desk
- **`lockout triage`**  -  See why one user cannot complete Duo MFA: status, lockout reason, the group status that overrides it, phones, outstanding bypass codes, last 24h failures, and the exact next command to run.

  _Reach for this first on any 'locked out of Duo' ticket; it is read-only and tells you which fix applies._

  ```bash
  cisco-duo-cli lockout triage jdoe --account-id DA9VZOC5X63I2W72NRP9 --agent
  ```
- **`lockout fix`**  -  Unlock a locked-out user, and with --bypass issue a single-use one-hour bypass code or with --reactivate-phone re-send Duo Mobile activation; a preview by default, --apply makes the changes and prints a before/after receipt.

  _Use after lockout triage when the user is locked out but still has their phone or needs a temporary bypass code._

  ```bash
  cisco-duo-cli lockout fix jdoe --account-id DA9VZOC5X63I2W72NRP9 --bypass --reactivate-phone
  ```
- **`phone-swap`**  -  Move a user to a new phone number: reuse or create the phone, attach it and text the activation link; a preview by default, --apply makes the changes. Old phones are retired on a second run once the new phone is activated, and a phone shared with another user is never deleted.

  _Use for lost, replaced or re-numbered phones instead of hand-sequencing phone create, associate and activation calls._

  ```bash
  cisco-duo-cli phone-swap jdoe --number +15551234567 --remove-old
  ```
- **`failures`**  -  Roll up failed and fraud authentications per user, reason, factor and application over a window, with first and last seen and source IPs.

  _Use for weekly failure reviews and 'I am getting pushes I did not send' investigations._

  ```bash
  cisco-duo-cli failures --since 7d --top 20 --agent
  ```

### MSP fleet
- **`fleet summary`**  -  One row per customer subaccount: edition, users against user limit, telephony credits, admin and integration counts. Needs an Admin API app in the MSP parent account; covers every subaccount and ignores --account-id.

  _Use for the weekly MSP capacity and credit sweep or before a customer review._

  ```bash
  cisco-duo-cli fleet summary --agent
  ```
- **`fleet risk`**  -  Per-customer counts of users in bypass, locked out, disabled, pending deletion or unenrolled, members of Bypass or Disabled groups, and standing bypass codes. Needs an MSP parent Admin API app; ignores --account-id.

  _Use to spot which customers have MFA gaps before drilling into one tenant._

  ```bash
  cisco-duo-cli fleet risk --agent
  ```
- **`fleet find-user`**  -  Find which customer tenants (and the MSP parent) contain a username, alias or email, with status and last login for each hit. Needs an MSP parent Admin API app; ignores --account-id.

  _Use when a ticket arrives without the customer name, before lockout triage._

  ```bash
  cisco-duo-cli fleet find-user jdoe@example.com --agent
  ```

### Tenant hygiene
- **`audit bypass`**  -  List every synced user who can skip MFA: bypass status, a Bypass group, or a bypass code that never expires or has unlimited reuse, with the admin who issued it. Needs 'sync --resources users,bypass_codes' first; JSON reports complete=false when data is missing.

  _Use for security reviews and weekly hygiene in one tenant._

  ```bash
  cisco-duo-cli audit bypass --older-than 30d --agent
  ```
- **`audit stale`**  -  Find unenrolled users heading for a lockout, inactive and pending-deletion users, expired, inactive or never-logged-in admins, and phones not seen recently. Needs 'sync --resources users,admins,phones' first; JSON reports complete=false when data is missing.

  _Use for offboarding and onboarding follow-up passes._

  ```bash
  cisco-duo-cli audit stale --days 90 --agent
  ```
- **`enrollment chase`**  -  List users still unenrolled after a given age (by default) and, with --apply, re-send their Duo enrollment emails, oldest first, up to --limit.

  _Use to head off 'Not enrolled' lockouts for new hires._

  ```bash
  cisco-duo-cli enrollment chase --older-than 72h --limit 25
  ```

## Recipes

### Locked-out caller, end to end

```bash
cisco-duo-cli lockout fix jdoe --account-id DA9VZOC5X63I2W72NRP9 --bypass --reactivate-phone
```

Prints the unlock, bypass code and SMS reactivation plan; add --apply to make the changes and get a before/after receipt.

### Narrow a user lookup for an agent

```bash
cisco-duo-cli lockout triage jdoe --agent --select effective_status,lockout_reason,next_actions
```

Returns only the diagnosis fields an agent needs to choose the fix.

### Monday MSP sweep

```bash
cisco-duo-cli fleet summary --agent
```

Edition, user-limit pressure and telephony credits for every customer subaccount.

### Who can skip MFA

```bash
cisco-duo-cli audit bypass --older-than 30d --agent
```

Run 'cisco-duo-cli sync --resources users,bypass_codes' first; lists bypass-status users, Bypass-group members and standing bypass codes, and reports complete=false if data is missing.

### Last week's failures

```bash
cisco-duo-cli failures --since 7d --top 20 --agent
```

Per-user failure counts by reason, factor and application.

## Usage

Run `cisco-duo-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `CISCO_DUO_CONFIG_DIR`, `CISCO_DUO_DATA_DIR`, `CISCO_DUO_STATE_DIR`, or `CISCO_DUO_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `CISCO_DUO_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export CISCO_DUO_HOME=/srv/cisco-duo
cisco-duo-cli doctor
```

Under `CISCO_DUO_HOME=/srv/cisco-duo`, the four dirs resolve to `/srv/cisco-duo/config`, `/srv/cisco-duo/data`, `/srv/cisco-duo/state`, and `/srv/cisco-duo/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "cisco-duo": {
      "command": "cisco-duo-mcp",
      "env": {
        "CISCO_DUO_HOME": "/srv/cisco-duo"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `CISCO_DUO_DATA_DIR` overrides an explicit `--home` for that kind. Use `CISCO_DUO_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `CISCO_DUO_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `cisco-duo-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### account_info

Manage account info

- **`cisco-duo-cli account-info get-auth-attempts`** - Authentication attempt counts by result (default last 30 days). Wire: GET /admin/v1/info/authentication_attempts signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Authentication Attempts Report'. Docs: Retrieve counts of authentication attempts for a given time period (not to exceed 180 days), broken down by result
- **`cisco-duo-cli account-info get-summary`** - Account summary: users, admins, integrations, telephony credits, edition. Wire: GET /admin/v1/info/summary signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Summary'. Docs: Returns a summary of account utilization information
- **`cisco-duo-cli account-info get-telephony-credits-used`** - Telephony credits used in a window (default last 30 days). Wire: GET /admin/v1/info/telephony_credits_used signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Telephony Credits Used Report'. Docs: This v1 endpoint will be deprecated in the future. You will need to migrate your use of this endpoint to the equivalent v2 endpoint when it becomes available to avoid service disruption
- **`cisco-duo-cli account-info get-user-auth-attempts`** - Number of users with authentication attempts (default last 30 days). Wire: GET /admin/v1/info/user_authentication_attempts signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Users with Authentication Attempts Report'. Docs: Retrieve counts of users with authentication attempts for a given time period (not to exceed 180 days), broken down by result. Each count is the number of users who had at least one authentication attempt ending with that result

### accounts

Manage accounts

- **`cisco-duo-cli accounts create`** - Create a subaccount. Wire: POST /accounts/v1/account/create signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Create Subaccounts'. Docs: Create a new subaccount
- **`cisco-duo-cli accounts delete`** - Delete a subaccount. Wire: POST /accounts/v1/account/delete signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Delete Subaccounts'. Docs: Delete the account with ID account_id from the system
- **`cisco-duo-cli accounts list`** - List subaccounts of the MSP parent account. Wire: POST /accounts/v1/account/list signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Subaccounts'. Docs: Returns a list of subaccounts

### admin_roles

Manage admin roles

- **`cisco-duo-cli admin-roles get`** - Get an administrator role. Wire: GET /admin/v1/admin_roles/{role_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Admin Role by ID'. Docs: Returns assignment detail for a standard admin role or a custom admin role identified by role_id. GET /admin/v1/admin_roles/[role_id] Parameters
- **`cisco-duo-cli admin-roles list`** - List administrator roles. Wire: GET /admin/v1/admin_roles signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Admin Roles'. Docs: Returns all administrative roles, both the eight standard roles and any custom roles

### admin_units

Manage admin units

- **`cisco-duo-cli admin-units get`** - Get an administrative unit. Wire: GET /admin/v1/administrative_units/{admin_unit_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Administrative Unit Details'. Docs: Returns details for a single administrative unit with admin_unit_id. Docs note: the docs describe one unit while the example wraps it in an array; modeled as one object
- **`cisco-duo-cli admin-units list`** - List administrative units. Wire: GET /admin/v1/administrative_units signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Administrative Units'. Docs: Returns a paged list of all administrative units if no parameters specified. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value

### admins

Manage admins

- **`cisco-duo-cli admins clear-inactivity`** - Clear an administrator's inactivity expiration. Wire: POST /admin/v1/admins/{admin_id}/clear_inactivity signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Clear Administrator Expiration'. Docs: Clear expiration for the administrator with admin_id after the admin has been expired due to inactivity. The administrator's status changes from "Expired" to the status applied to that admin before inactive expiration, and restores access to the Duo Admin Panel if the effective status is "Active"
- **`cisco-duo-cli admins create`** - Create an administrator (activation link flow). Wire: POST /admin/v1/admins signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Create Administrator'. Docs: Create a new administrator
- **`cisco-duo-cli admins delete`** - Delete an administrator. Wire: DELETE /admin/v1/admins/{admin_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Delete Administrator'. Docs: Delete the administrator with administrator ID admin_id from the system. Administrators managed by directory sync can not be deleted via API
- **`cisco-duo-cli admins get`** - Get an administrator. Wire: GET /admin/v1/admins/{admin_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Administrator by ID'. Docs: Return the single administrator with the administrator ID admin_id
- **`cisco-duo-cli admins list`** - List administrators. Wire: GET /admin/v1/admins signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Administrators'. Docs: Returns a paged list of administrators. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value
- **`cisco-duo-cli admins list-activations`** - List pending administrator activations. Wire: GET /admin/v1/admins/activations signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Pending Administrator Activations'. Docs: Returns a paged list of pending administrator activations. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value. Docs note: the docs prose says a paged list while its example shows a single object; modeled as the documented list
- **`cisco-duo-cli admins reset`** - Reset an administrator's failed authentication attempt count (admin lockout). Wire: POST /admin/v1/admins/{admin_id}/reset signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Reset Administrator Authentication Attempts'. Docs: Clear the number of failed login attempts for the administrator with admin_id. Re-enables an administrator who has been disabled due to too many failed authentication attempts
- **`cisco-duo-cli admins update`** - Modify an administrator. Wire: POST /admin/v1/admins/{admin_id} signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Modify Administrator'. Docs: Change the name, phone number, or other properties of the administrator with the administrator ID admin_id

### billing

Manage billing

- **`cisco-duo-cli billing get-edition`** - Get a subaccount's Duo edition. Wire: GET /admin/v1/billing/edition signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Get Subaccount Edition'. Docs: Returns the Duo edition information for a subaccount
- **`cisco-duo-cli billing get-telephony-credits`** - Get a subaccount's telephony credits. Wire: GET /admin/v1/billing/telephony_credits signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Get Subaccount Telephony Credits'. Docs: Returns the available telephony credits for a subaccount
- **`cisco-duo-cli billing get-user-limit`** - Get a subaccount's user limit and user count. Wire: GET /admin/v1/billing/user_limit signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Get Subaccount User Limit'. Docs: Returns the current user count and the hard user limit for a subaccount. Contact your MSP Partner Manager to have the user limit feature enabled for your account
- **`cisco-duo-cli billing set-edition`** - Set a subaccount's Duo edition. Wire: POST /admin/v1/billing/edition signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Set Subaccount Edition'. Docs: Sets the effective Duo edition for a subaccount
- **`cisco-duo-cli billing set-telephony-credits`** - Set a subaccount's telephony credit total (moved from the parent). Wire: POST /admin/v1/billing/telephony_credits signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Set Subaccount Telephony Credits'. Docs: Sets the telephony credits for a subaccount
- **`cisco-duo-cli billing set-user-limit`** - Set or remove a subaccount's user limit. Wire: POST /admin/v1/billing/user_limit signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Set Subaccount User Limit'. Docs: Sets or removes the hard user limit on a subaccount. Contact your MSP Partner Manager to have the user limit feature enabled for your account

### bypass_codes

Manage bypass codes

- **`cisco-duo-cli bypass-codes delete`** - Delete (revoke) a bypass code. Wire: DELETE /admin/v1/bypass_codes/{bypass_code_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Delete Bypass Code'. Docs: Delete the bypass code with ID bypass_code_id from the system
- **`cisco-duo-cli bypass-codes get`** - Get bypass code metadata. Wire: GET /admin/v1/bypass_codes/{bypass_code_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Bypass Code by ID'. Docs: Return information about a single bypass code with bypass_code_id. Output does not include the actual bypass code
- **`cisco-duo-cli bypass-codes list`** - List bypass code metadata across all users. Wire: GET /admin/v1/bypass_codes signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Bypass Codes'. Docs: Returns a paged list of information about all bypass codes. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value. Output does not include the actual bypass codes

### desktop_authenticators

Manage desktop authenticators

- **`cisco-duo-cli desktop-authenticators delete`** - Delete a Duo Desktop authenticator. Wire: DELETE /admin/v1/desktop_authenticators/{dakey} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Delete Desktop Authenticator'. Docs: Delete the desktop authenticator with key dakey from the system
- **`cisco-duo-cli desktop-authenticators get`** - Get a Duo Desktop authenticator. Wire: GET /admin/v1/desktop_authenticators/{dakey} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Desktop Authenticator by Key'. Docs: Return the single desktop authenticator with dakey
- **`cisco-duo-cli desktop-authenticators list`** - List Duo Desktop authenticators. Wire: GET /admin/v1/desktop_authenticators signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Desktop Authenticators'. Docs: Returns a paged list of desktop authenticators. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset

### directory_sync

Manage directory sync

- **`cisco-duo-cli directory-sync list`** - List external directory syncs. Wire: GET /admin/v1/users/directorysync signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'List External Directory Syncs'. Docs: Returns a list of user external directory syncs with basic information
- **`cisco-duo-cli directory-sync sync-user`** - Create or update one user from an external directory. Wire: POST /admin/v1/users/directorysync/{directory_key}/syncuser signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Synchronize User from External Directory'. Docs: Initiate a sync to create, update, or mark for deletion the user specified by username against the directory specified by the directory_key. The directory_key for a directory can be found by navigating to Users → Directory Sync in the Duo Admin Panel, and then clicking on the configured directory. Learn more about syncing individual users from Active Directory, OpenLDAP, or Entra ID

### endpoints

Manage endpoints

- **`cisco-duo-cli endpoints get`** - Get an endpoint. Wire: GET /admin/v1/endpoints/{epkey} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Endpoint by ID'. Docs: Return information for an individual endpoint with epkey
- **`cisco-duo-cli endpoints list`** - List endpoints (devices that accessed Duo apps). Wire: GET /admin/v1/endpoints signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Endpoints'. Docs: Returns a paged list of endpoints. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value. When no limit is specified the response is limited to 100 results

### groups

Manage groups

- **`cisco-duo-cli groups create`** - Create a group. Wire: POST /admin/v1/groups signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Create Group'. Docs: Create a new group
- **`cisco-duo-cli groups delete`** - Delete a group. Wire: DELETE /admin/v1/groups/{group_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Delete Group'. Docs: Delete a group
- **`cisco-duo-cli groups get`** - Get a group. Wire: GET /admin/v2/groups/{group_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Get Group Info'. Docs: See the Admin API v1 legacy instructions for more information about the Get Group Info v1 endpoint
- **`cisco-duo-cli groups list`** - List groups. Wire: GET /admin/v1/groups signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Groups'. Docs: Returns a paged list of groups. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value
- **`cisco-duo-cli groups list-members`** - List a group's members. Wire: GET /admin/v2/groups/{group_id}/users signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Get Group Members'
- **`cisco-duo-cli groups update`** - Update a group. Wire: POST /admin/v1/groups/{group_id} signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Update Group'. Docs: Update information about a group

### identity_verification

Manage identity verification

- **`cisco-duo-cli identity-verification cancel`** - Cancel an identity verification. Wire: POST /admin/v2/identity_verification/{user_id}/cancel signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Cancel Identity Verification'. Docs: Cancels the identity verification process for a specified user. Requires the "Grant identity verification - Write" API permission
- **`cisco-duo-cli identity-verification get`** - Get identity verification status for a user. Wire: GET /admin/v2/identity_verification/{user_id}/status signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Get Identity Verification Status'. Docs: Retrieves current status of the user's recent identity verification. Requires the "Grant identity verification - Read" API permission
- **`cisco-duo-cli identity-verification start`** - Start a Duo identity verification for a user. Wire: POST /admin/v2/identity_verification/{user_id}/start signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Start Identity Verification'. Docs: Initiates the Duo Identity Verification process by generating an access code. This code is used to redirect the user to Persona for verification. User is identified by the user_id. Requires the "Grant identity verification - Write" API permission

### integrations

Manage integrations

- **`cisco-duo-cli integrations get`** - Get an integration. Wire: GET /admin/v3/integrations/{integration_key} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Integration by Integration Key'. Docs: Return the single integration with integration_key. Returns SSO parameters for generic Duo Single Sign-On applications only
- **`cisco-duo-cli integrations list`** - List integrations (applications); secret keys are not requested. Wire: GET /admin/v3/integrations signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Integrations'. Docs: Returns a paged list of integrations. Returns SSO parameters for generic Duo Single Sign-On applications only. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value

### logs

Manage logs

- **`cisco-duo-cli logs get-activity`** - Activity log events (v2). Wire: GET /admin/v2/logs/activity signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Activity Logs'. Docs: Returns a paged list of activity log events ranging from the last 180 days up to as recently as two minutes before the API request. The events returned are subject to log retention, if set up in the account, as described here. To fetch all results, call repeatedly with the next_offset paging parameter as long as the result metadata has next_offset values
- **`cisco-duo-cli logs get-admin`** - Administrator action log (v1, 1000 per call). Wire: GET /admin/v1/logs/administrator signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Administrator Logs'. Docs: This v1 endpoint will be deprecated in the future. Migrate your use of this logging endpoint to the Activity Logs v2 endpoint before then to avoid service disruption
- **`cisco-duo-cli logs get-auth`** - Authentication log events (v2, filterable). Wire: GET /admin/v2/logs/authentication signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Authentication Logs'. Docs: See the Admin API v1 legacy instructions for more information about the Authentication Logs v1 endpoint
- **`cisco-duo-cli logs get-offline-enrollment`** - Offline enrollment log (v1). Wire: GET /admin/v1/logs/offline_enrollment signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Offline Enrollment Logs'. Docs: This v1 endpoint will be deprecated in the future. Migrate your use of this logging endpoint to the Activity Logs v2 endpoint before then to avoid service disruption
- **`cisco-duo-cli logs get-telephony`** - Telephony log events (v2). Wire: GET /admin/v2/logs/telephony signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Telephony Logs'. Docs: See the Admin API v1 legacy instructions for more information about the Telephony Logs v1 endpoint

### passport

Manage passport

- **`cisco-duo-cli passport`** - Get Duo Passport configuration. Wire: GET /admin/v2/passport/config signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Passport Configuration'. Docs: Returns the configuration for Duo Passport

### phones

Manage phones

- **`cisco-duo-cli phones create`** - Create a phone. Wire: POST /admin/v1/phones signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Create Phone'. Docs: Create a new phone with a specified phone number or other parameters
- **`cisco-duo-cli phones create-activation-url`** - Create a Duo Mobile activation code/URL for a phone (reactivation). Wire: POST /admin/v1/phones/{phone_id}/activation_url signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Create Activation Code'. Docs: Generate a Duo Mobile activation code. This method will fail if the phone's type or platform are Unknown
- **`cisco-duo-cli phones delete`** - Delete a phone. Wire: DELETE /admin/v1/phones/{phone_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Delete Phone'. Docs: Delete the phone with ID phone_id from the system
- **`cisco-duo-cli phones get`** - Get a phone. Wire: GET /admin/v1/phones/{phone_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Phone by ID'. Docs: Return the single phone with phone_id
- **`cisco-duo-cli phones list`** - List phones, or look one up by number. Wire: GET /admin/v1/phones signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Phones'. Docs: Returns a paged list of phones. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value. If no number or extension parameters are provided, the list will contain all phones. Otherwise, the list will contain either a single phone (if a match was found), or no phones
- **`cisco-duo-cli phones send-sms-activation`** - Text a Duo Mobile activation link to a phone. Wire: POST /admin/v1/phones/{phone_id}/send_sms_activation signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Send Activation Code via SMS'. Docs: The recommended maximum length for activation_msg and installation_msg is 80 characters
- **`cisco-duo-cli phones send-sms-installation`** - Text Duo Mobile install instructions to a phone. Wire: POST /admin/v1/phones/{phone_id}/send_sms_installation signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Send Installation URL via SMS'. Docs: The recommended maximum length for installation_msg is 80 characters
- **`cisco-duo-cli phones send-sms-passcodes`** - Text a new batch of SMS passcodes to a phone. Wire: POST /admin/v1/phones/{phone_id}/send_sms_passcodes signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Send Passcodes via SMS'. Docs: Generate a new batch of SMS passcodes and send them to the phone in a single SMS message
- **`cisco-duo-cli phones set-quarantine-status`** - Quarantine or release a device. Wire: POST /admin/v2/phones/{phone_id}/quarantine_status signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Set Quarantine Status'. Docs: Quarantine Status is available to Duo Premier and Duo Advantage plan customers
- **`cisco-duo-cli phones update`** - Modify a phone. Wire: POST /admin/v1/phones/{phone_id} signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Modify Phone'. Docs: Change the details of the phone with ID phone_id

### policies

Manage policies

- **`cisco-duo-cli policies calculate-policy`** - Show the effective policy for a user in an application. Wire: GET /admin/v2/policies/calculate signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Resulting Policy'. Docs: Returns the resulting policy for a given user/application pair. The resulting policy is constructed from the top-most sections from the entire stack of policies applied for a given ikey
- **`cisco-duo-cli policies get-global-policy`** - Get the global policy. Wire: GET /admin/v2/policies/global signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Policy by ID (global)'. Docs: Return the single policy with the specified policy_key. Use global as a shortcut for the global policy. Includs all policy section data. Docs note: the docs say a single policy object while the example starts with an array; modeled as one object
- **`cisco-duo-cli policies get-policy`** - Get a policy. Wire: GET /admin/v2/policies/{policy_key} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Policy by ID'. Docs: Return the single policy with the specified policy_key. Use global as a shortcut for the global policy. Includs all policy section data. Docs note: the docs say a single policy object while the example starts with an array; modeled as one object
- **`cisco-duo-cli policies list`** - List policies with all sections. Wire: GET /admin/v2/policies signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Policies'. Docs: Retrieve a complete set of all policies. Includes all policy section data for each policy
- **`cisco-duo-cli policies summarize`** - Summarize policies and where they apply. Wire: GET /admin/v2/policies/summary signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Summarize Policies'. Docs: Returns policy names, keys, and applications or groups to which a policy is applied

### registered_devices

Manage registered devices

- **`cisco-duo-cli registered-devices block-device`** - Block a registered device. Wire: POST /admin/v1/registered_devices/blocked/{compkey} signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Block Device by ID'. Docs: Block a device from accessing any applications protected by a Duo policy that requires device registration with compkey
- **`cisco-duo-cli registered-devices list`** - List Duo Passport / registered devices. Wire: GET /admin/v1/registered_devices signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Registered Devices'. Docs: Returns a paged list of Duo Desktop registered devices. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset. If no number or extension parameters are provided, the list will contain all registered devices
- **`cisco-duo-cli registered-devices list-blocked-devices`** - List blocked registered devices. Wire: GET /admin/v1/registered_devices/blocked signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Blocked Devices'. Docs: Returns a paged list of blocked devices. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset. If no number or extension parameters are provided, the list will contain all registered devices
- **`cisco-duo-cli registered-devices unblock-device`** - Unblock a registered device. Wire: DELETE /admin/v1/registered_devices/blocked/{compkey} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Unblock Device by ID'. Docs: Unblock a device with compkey. Previously registered devices will retain their original registration data

### settings

Manage settings

- **`cisco-duo-cli settings`** - Get account settings. Wire: GET /admin/v1/settings signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Settings'. Docs: Returns global Duo settings. These settings can also be viewed and set in the Duo Admin Panel

### tokens

Manage tokens

- **`cisco-duo-cli tokens delete`** - Delete a hardware token. Wire: DELETE /admin/v1/tokens/{token_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Delete Hardware Token'. Docs: Delete the hardware token with ID token_id from the system
- **`cisco-duo-cli tokens get`** - Get a hardware token. Wire: GET /admin/v1/tokens/{token_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Hardware Token by ID'. Docs: Return the single hardware token with token_id
- **`cisco-duo-cli tokens list`** - List hardware tokens. Wire: GET /admin/v1/tokens signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Hardware Tokens'. Docs: Returns a paged list of OTP hardware tokens. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value. If no type and serial parameters are provided, the list will contain all hardware tokens. Otherwise, the list will contain either a single hardware token (if a match was found) or no hardware tokens
- **`cisco-duo-cli tokens resync`** - Resynchronize an HOTP or Duo-D100 token with three successive codes. Wire: POST /admin/v1/tokens/{token_id}/resync signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Resync Hardware Token'. Docs: Resynchronize the hardware token with ID token_id by providing three successive codes from the token. Only HOTP and Duo-D100 tokens can be resynchronized. YubiKey tokens operating in their native AES mode do not need resynchronization

### trust_monitor

Manage trust monitor

- **`cisco-duo-cli trust-monitor`** - Trust Monitor events. Wire: GET /admin/v1/trust_monitor/events signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Events'. Docs: Returns a paged list of events surfaced by Trust Monitor from the last 180 days. To fetch all results, call repeatedly with the next_offset paging parameter as long as the result metadata has next_offset values

### users

Manage users

- **`cisco-duo-cli users add-group`** - Add a user to a group. Wire: POST /admin/v1/users/{user_id}/groups signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Associate Group with User'. Docs: Associate a group with ID group_id with the user with ID user_id
- **`cisco-duo-cli users add-phone`** - Associate a phone with a user. Wire: POST /admin/v1/users/{user_id}/phones signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Associate Phone with User'. Docs: Associate a phone with the user with ID user_id
- **`cisco-duo-cli users add-token`** - Associate a hardware token with a user. Wire: POST /admin/v1/users/{user_id}/tokens signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Associate Hardware Token with User'. Docs: Associate a hardware token with the user with ID user_id
- **`cisco-duo-cli users bulk-create`** - Create up to 100 users in one call. Wire: POST /admin/v1/users/bulk_create signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Create Multiple Users'. Docs: Create multiple users at once. If one user fails to add, the entire operation fails. You can create a maximum of 100 users per request at a rate limit of 50 calls per minute
- **`cisco-duo-cli users bulk-restore`** - Restore up to 100 users from the Trash. Wire: POST /admin/v1/users/bulk_restore signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Restore Multiple Users from Trash'. Docs: Restore multiple users from the Trash at once. If one user fails to restore, the entire operation fails. You can restore a maximum of 100 users per request at a rate limit of 50 calls per minute. Restoring users from the Trash doesn't change the user's status. Restored users still have a "disabled" status and can't log in. Refer to Restoring Users from Trash for more information
- **`cisco-duo-cli users bulk-trash`** - Send up to 100 users to the Trash. Wire: POST /admin/v1/users/bulk_send_to_trash signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Send Multiple Users to Trash'. Docs: Send multiple users to Trash. Once a user is in the Trash, the user is pending deletion for 7 days before being permanently deleted. Refer to Deleting and Restoring Users for more information
- **`cisco-duo-cli users create`** - Create a user. Wire: POST /admin/v1/users signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Create User'. Docs: Create a new user with the specified username
- **`cisco-duo-cli users create-bypass-codes`** - Generate bypass codes for a user (replaces existing codes unless --preserve-existing). Wire: POST /admin/v1/users/{user_id}/bypass_codes signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Create Bypass Codes for User'. Docs: Clear all existing bypass codes for the user with ID user_id and return a list of count newly generated bypass codes, or specify codes that expire after valid_secs seconds, or reuse_count uses. To preserve existing bypass codes instead of clearing them the request must specify preserve_existing=true. Does not trigger email or Duo Push notifications to users
- **`cisco-duo-cli users delete`** - Permanently delete a user (API deletes skip the Trash; use users bulk-trash for a 7-day restore window). Wire: DELETE /admin/v1/users/{user_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Delete User'. Docs: Delete the user with ID user_id from the system. The API will not delete phones associated only with that user right away; remove them immediately with Delete Phone. This method returns 200 if the user was found or if no such user exists
- **`cisco-duo-cli users enroll`** - Email an enrollment link to a new user. Wire: POST /admin/v1/users/enroll signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Enroll User'. Docs: Enroll a 2FA-only user with user name username and email address email, and send them an enrollment email with a single-use enrollment code that expires after valid_secs seconds. Enrollment codes expire immediately upon changing the user's email address after enrollment code creation. This endpoint does not create codes valid for Duo Directory user enrollment at sso.duosecurity.com portals
- **`cisco-duo-cli users get`** - Get a user by ID (with phones, tokens, groups). Wire: GET /admin/v1/users/{user_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve User by ID'. Docs: Return the single user with user_id
- **`cisco-duo-cli users get-verification-push-response`** - Check the result of a verification push. Wire: GET /admin/v1/users/{user_id}/verification_push_response signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Verification Push Response'. Docs: Retrieves the verification push result for the user with ID user_id. Push response information will be available for 120 seconds after the push was sent, after which this endpoint will return a 404. If no success or failure response was returned by this endpoint during these 120 seconds, it can be assumed that the push has timed out
- **`cisco-duo-cli users list`** - List users, or look one up by username or email. Wire: GET /admin/v1/users signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Users'. Docs: Returns a paged list of users. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value. If username is not provided, the list will contain all users. If username is provided, the list will either contain a single user (if a match was found) or no users
- **`cisco-duo-cli users list-bypass-codes`** - List bypass code metadata for a user (codes themselves are never returned). Wire: GET /admin/v1/users/{user_id}/bypass_codes signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Bypass Codes by User ID'. Docs: Returns a paged list of bypass code metadata associated with the user with ID user_id. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value. Does not return the actual bypass codes
- **`cisco-duo-cli users list-desktop-authenticators`** - List a user's Duo Desktop authenticators. Wire: GET /admin/v1/users/{user_id}/desktopauthenticators signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Desktop Authenticators by User ID'. Docs: Returns a paged list of desktop authenticators associated with the user with ID user_id. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value
- **`cisco-duo-cli users list-groups`** - List a user's groups. Wire: GET /admin/v1/users/{user_id}/groups signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Groups by User ID'. Docs: Returns a paged list of groups associated with the user with ID user_id. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value
- **`cisco-duo-cli users list-phones`** - List a user's phones. Wire: GET /admin/v1/users/{user_id}/phones signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Phones by User ID'. Docs: Returns a paged list of phones associated with the user with ID user_id. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value
- **`cisco-duo-cli users list-tokens`** - List a user's hardware tokens. Wire: GET /admin/v1/users/{user_id}/tokens signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve Hardware Tokens by User ID'. Docs: Returns a paged list of OTP hardware tokens associated with the user with ID user_id. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value
- **`cisco-duo-cli users list-webauthn`** - List a user's WebAuthn credentials. Wire: GET /admin/v1/users/{user_id}/webauthncredentials signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve WebAuthn Credentials by User ID'. Docs: Returns a list of WebAuthn credentials associated with the user with ID user_id
- **`cisco-duo-cli users remove-group`** - Remove a user from a group. Wire: DELETE /admin/v1/users/{user_id}/groups/{group_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Disassociate Group from User'. Docs: Disassociate a group from the user with ID user_id. This method will return 200 if the group was found or if no such group exists
- **`cisco-duo-cli users remove-phone`** - Disassociate a phone from a user (the phone itself is kept). Wire: DELETE /admin/v1/users/{user_id}/phones/{phone_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Disassociate Phone from User'. Docs: Disassociate a phone from the user with ID user_id. The API will not automatically delete the phone after removing the last user association; remove it permanently with Delete Phone. This method returns 200 if the phone was found or if no such phone exists
- **`cisco-duo-cli users remove-token`** - Disassociate a hardware token from a user. Wire: DELETE /admin/v1/users/{user_id}/tokens/{token_id} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Disassociate Hardware Token from User'. Docs: Disassociate a hardware token from the user with ID user_id. This method will return 200 if the hardware token was found or if no such hardware token exists
- **`cisco-duo-cli users send-verification-push`** - Send a Duo Push to confirm a user's identity (returns a confirmation code). Wire: POST /admin/v1/users/{user_id}/send_verification_push signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Send Verification Push'. Docs: Sends a verification Duo Push to the user with ID user_id. Verification pushes can also be sent from the Duo Admin Panel
- **`cisco-duo-cli users update`** - Modify a user; --status active unlocks a locked-out user. Wire: POST /admin/v1/users/{user_id} signed with Duo v5 HMAC, parameters in a JSON body; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Modify User'. Docs: Change the username, username aliases, full name, status, and/or notes section of the user with ID user_id

### webauthn

Manage webauthn

- **`cisco-duo-cli webauthn delete`** - Delete a WebAuthn credential. Wire: DELETE /admin/v1/webauthncredentials/{webauthnkey} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Delete WebAuthn Credential'. Docs: Delete the WebAuthn credential with key webauthnkey from the system
- **`cisco-duo-cli webauthn get`** - Get a WebAuthn credential. Wire: GET /admin/v1/webauthncredentials/{webauthnkey} signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve WebAuthn Credentials by Key'. Docs: Return the single WebAuthn credential with webauthnkey
- **`cisco-duo-cli webauthn list`** - List WebAuthn credentials. Wire: GET /admin/v1/webauthncredentials signed with Duo v5 HMAC; success responses arrive as JSON with stat OK and the result under response; documented in the Duo Admin API reference section 'Retrieve WebAuthn Credentials'. Docs: Returns a paged list of all registered WebAuthn credentials. To fetch all results, call repeatedly with the offset parameter as long as the result metadata has a next_offset value


### audit


- **`cisco-duo-cli audit bypass`** - List every synced user who can skip MFA: bypass status, a Bypass group, or a bypass code that never expires or has unlimited reuse, with the admin who issued it. Needs 'sync --resources users,bypass_codes' first; JSON reports complete=false when data is missing.

- **`cisco-duo-cli audit stale`** - Find unenrolled users heading for a lockout, inactive and pending-deletion users, expired, inactive or never-logged-in admins, and phones not seen recently. Needs 'sync --resources users,admins,phones' first; JSON reports complete=false when data is missing.


### enrollment


- **`cisco-duo-cli enrollment chase`** - List users still unenrolled after a given age (by default) and, with --apply, re-send their Duo enrollment emails, oldest first, up to --limit.


### failures


- **`cisco-duo-cli failures`** - Roll up failed and fraud authentications per user, reason, factor and application over a window, with first and last seen and source IPs.


### fleet


- **`cisco-duo-cli fleet summary`** - One row per customer subaccount: edition, users against user limit, telephony credits, admin and integration counts. Needs an Admin API app in the MSP parent account; covers every subaccount and ignores --account-id.

- **`cisco-duo-cli fleet risk`** - Per-customer counts of users in bypass, locked out, disabled, pending deletion or unenrolled, members of Bypass or Disabled groups, and standing bypass codes. Needs an MSP parent Admin API app; ignores --account-id.

- **`cisco-duo-cli fleet find-user`** - Find which customer tenants (and the MSP parent) contain a username, alias or email, with status and last login for each hit. Needs an MSP parent Admin API app; ignores --account-id.


### lockout


- **`cisco-duo-cli lockout triage`** - See why one user cannot complete Duo MFA: status, lockout reason, the group status that overrides it, phones, outstanding bypass codes, last 24h failures, and the exact next command to run.

- **`cisco-duo-cli lockout fix`** - Unlock a locked-out user, and with --bypass issue a single-use one-hour bypass code or with --reactivate-phone re-send Duo Mobile activation; a preview by default, --apply makes the changes and prints a before/after receipt.


### phone-swap


- **`cisco-duo-cli phone-swap`** - Move a user to a new phone number: reuse or create the phone, attach it and text the activation link; a preview by default, --apply makes the changes. Old phones are retired on a second run once the new phone is activated, and a phone shared with another user is never deleted.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`cisco-duo-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`cisco-duo-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`cisco-duo-cli learnings list`** - Inspect taught rows
- **`cisco-duo-cli learnings forget <query>`** - Undo a teach
- **`cisco-duo-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`cisco-duo-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`cisco-duo-cli teach-pattern`** - Install a query/resource template up front
- **`cisco-duo-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `CISCO_DUO_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `cisco-duo-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
cisco-duo-cli accounts list

# JSON for scripting and agents
cisco-duo-cli accounts list --json
# Filter to specific fields
cisco-duo-cli accounts list --json --select account_id,api_hostname,name

# Dry run  -  show the request without sending
cisco-duo-cli accounts list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
cisco-duo-cli accounts list --agent
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

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `CISCO_DUO_API_HOST` resolves `{api_host}`

Base URL: `https://{api_host}`

## Health Check

```bash
cisco-duo-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `cisco-duo-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/cisco-duo-cli/config.toml`; `--home`, `CISCO_DUO_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `CISCO_DUO_API_HOST` | endpoint | Yes |  |
| `CISCO_DUO_IKEY` | per_call | Yes | Admin API integration key (ikey), e.g. DIWJ8X6AEYOR5OMC6TQ1. |
| `CISCO_DUO_SKEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `cisco-duo-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `cisco-duo-cli doctor` to check credentials
- Verify the environment variable is set: `echo $CISCO_DUO_IKEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **CISCO_DUO_API_HOST not set**  -  export CISCO_DUO_API_HOST=api-xxxxxxxx.duosecurity.com (the API hostname from your Duo Admin API application)
- **401 code 40101 Missing request credentials**  -  export CISCO_DUO_IKEY and CISCO_DUO_SKEY, then run cisco-duo-cli doctor
- **401 code 40102 Invalid identity in request credentials**  -  CISCO_DUO_IKEY is not an Admin API integration key for this account; copy it from the Admin API application in the Duo Admin Panel
- **401 code 40103 Invalid signature**  -  Check CISCO_DUO_SKEY and that CISCO_DUO_API_HOST is exactly the API hostname shown in the Admin Panel; also sync your system clock
- **403 This integration is not authorized for this endpoint**  -  Grant the Admin API application the permission the endpoint needs, e.g. 'Grant resource - Read', 'Grant resource - Write', 'Grant read log', 'Grant read information' or 'Grant accounts - Read'
- **subaccount ... not found under this parent account**  -  List the parent's subaccounts with 'cisco-duo-cli accounts list' and reuse one of the account_id values it prints
- **A get command's fields are under response**  -  Duo wraps results as {stat, response}; select nested fields, e.g. --select response.username,response.status
- **429 Too many requests**  -  Retry later; query logs at most once per minute and narrow fleet commands with --only

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**duo_client_python**](https://github.com/duosecurity/duo_client_python)  -  Python (150 stars)
- [**Duo-PSModule**](https://github.com/mbegan/Duo-PSModule)  -  PowerShell (57 stars)
- [**duo_api_golang**](https://github.com/duosecurity/duo_api_golang)  -  Go (55 stars)
- [**duocli**](https://github.com/bensallen/duocli)  -  Go
- [**terraform-provider-duo**](https://github.com/emanuelbesliu/terraform-provider-duo)  -  Go

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
