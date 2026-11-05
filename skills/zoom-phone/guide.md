# Zoom Phone CLI

**Every Zoom Phone admin operation plus E911, routing and footprint audits, across every client account.**

Covers the whole Zoom Phone admin API as commands and MCP tools, with a local SQLite store for offline search. On top it joins answers the API spreads across many endpoints: e911 audit for emergency address gaps, footprint for everything one user holds, routing trace for where a number rings, and callbacks for missed callers nobody called back. MSPs save one profile per client account with accounts connect and run any read-only audit across all of them with fleet.

Learn more at [Zoom Phone](https://developer.zoom.us/).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `zoom-phone-cli` and `zoom-phone-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=zoom-phone). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/zoom-phone --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/zoom-phone --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `zoom-phone-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the zoom-phone skill from https://github.com/Servosity/msp-skills/tree/main/skills/zoom-phone. The skill defines how its required CLI (`zoom-phone-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=zoom-phone).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `ZOOM_PHONE_ACCOUNT_ID`, `ZOOM_PHONE_CLIENT_ID` and `ZOOM_PHONE_CLIENT_SECRET` (from your Zoom Server-to-Server OAuth app) when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zoom-phone/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/zoom-phone/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "zoom-phone": {
      "command": "zoom-phone-mcp",
      "env": {
        "ZOOM_PHONE_ACCOUNT_ID": "<account-id>",
        "ZOOM_PHONE_CLIENT_ID": "<client-id>",
        "ZOOM_PHONE_CLIENT_SECRET": "<client-secret>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`zoom-phone-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `ZOOM_PHONE_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `ZOOM_PHONE_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export ZOOM_PHONE_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
zoom-phone-mcp --transport http
```

Set `ZOOM_PHONE_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Zoom Phone uses a Server-to-Server OAuth app (Zoom App Marketplace, Develop > Build App > Server-to-Server OAuth) with phone admin scopes such as phone:read:admin and phone:write:admin. Set ZOOM_PHONE_ACCOUNT_ID, ZOOM_PHONE_CLIENT_ID and ZOOM_PHONE_CLIENT_SECRET; the CLI mints a one-hour bearer token with the account_credentials grant and re-mints automatically. There is no refresh token. Each Zoom account needs its own app, so MSPs save one profile per client account with 'accounts connect' and select it with eval "$(zoom-phone-cli accounts path <name> --env)" (sets ZOOM_PHONE_CONFIG and ZOOM_PHONE_HOME together) or run across accounts with fleet. The --profile and --client-profile flags do not select a Zoom account.

## Quick Start

```bash
# Confirm the install without touching the network
zoom-phone-cli doctor --dry-run

# First live call: proves the token mint and the phone:read:admin scope
zoom-phone-cli users list-phone --agent --select name,email,extension_number

# Pull the core admin objects into the local store the audits read (--full also removes rows Zoom no longer returns)
zoom-phone-cli sync --full --resources users,numbers,call_queues,assigned,unassigned,sites,locations,common_areas,shared_line_groups,group_call_pickup,monitoring_groups,emergency_addresses

# Find users and sites without a verified emergency address
zoom-phone-cli e911 audit --agent

# See everything one user holds before a change or offboarding
zoom-phone-cli footprint jane@example.com --agent

```

## Unique Features

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

## Usage

Run `zoom-phone-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `ZOOM_PHONE_CONFIG_DIR`, `ZOOM_PHONE_DATA_DIR`, `ZOOM_PHONE_STATE_DIR`, or `ZOOM_PHONE_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `ZOOM_PHONE_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export ZOOM_PHONE_HOME=/srv/zoom-phone
zoom-phone-cli doctor
```

On Windows (PowerShell):

```powershell
$env:ZOOM_PHONE_HOME = "C:\ProgramData\zoom-phone"
zoom-phone-cli doctor
```

Under `ZOOM_PHONE_HOME=/srv/zoom-phone`, the four dirs resolve to `/srv/zoom-phone/config`, `/srv/zoom-phone/data`, `/srv/zoom-phone/state`, and `/srv/zoom-phone/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:ZOOM_PHONE_HOME = "C:\srv\zoom-phone"
zoom-phone-cli doctor
```

Under `$env:ZOOM_PHONE_HOME = "C:\srv\zoom-phone"`, the four dirs resolve to `C:\srv\zoom-phone\config`, `C:\srv\zoom-phone\data`, `C:\srv\zoom-phone\state`, and `C:\srv\zoom-phone\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `ZOOM_PHONE_DATA_DIR` overrides an explicit `--home` for that kind. Use `ZOOM_PHONE_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `ZOOM_PHONE_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `zoom-phone-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `zoom-phone-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `zoom-phone-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `ZOOM_PHONE_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `ZOOM_PHONE_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### account_settings

Manage account settings

- **`zoom-phone-cli account-settings`** - Returns an account's Zoom Phone settings.

Prerequisites
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_account_settings:admin`

Rate Limit Label: `LIGHT`

### alert_settings

Manage alert settings

- **`zoom-phone-cli alert-settings add-an`** - Adds an alert setting.

Prerequisites:  
 
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:alert_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli alert-settings delete-an`** - Deletes an alert setting.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions.

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:alert_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli alert-settings get-details`** - Returns detailed information about a specific alert setting.

Prerequisites:

* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:alert_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli alert-settings list-with-paging-query`** - Gets alert settings for an account with paging query.

Prerequisites
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_alert_settings:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli alert-settings update-an`** - Updates information of an Alert setting.

Prerequisites:  
 
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:patch:alert_setting:admin`

Rate Limit Label: `LIGHT`

### audios

Manage audios

- **`zoom-phone-cli audios delete-item`** - Deletes an audio item. Only the admin or user can delete your audio.

Prerequisites:
* Business, or Education account
* Zoom Phone license 



Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:delete:audio`,`phone:delete:audio:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli audios get-item`** - Returns an audio item. Only the admin or user can access your audio.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:audio`,`phone:read:audio:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli audios update-item`** - Updates an audio item. Only the admin or user can update your audio.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:update:audio`,`phone:update:audio:admin`

Rate Limit Label: `LIGHT`

### auto_receptionists

Manage auto receptionists

- **`zoom-phone-cli auto-receptionists add`** - Adds an auto receptionist to a Zoom Phone. Auto receptionists answer calls with a personalized recording and routes calls to a phone user, call queue, common area, voicemail or an IVR system. 

Prerequisites:
* Pro or higher account with Zoom Phone license.
* Account owner or admin privileges 



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:auto_receptionist:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli auto-receptionists add-policy`** - Adds a policy subsetting for a specific auto receptionist according to the `policyType`. For example, you can use this API to set up shared access members.<br><br>Prerequisites:
* Pro or higher account plan with Zoom Phone License
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:auto_receptionist_policy:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli auto-receptionists add-setting`** - Adds the auto receptionist setting according to the setting type.

Prerequisites
* A Pro or higher account with a Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:auto_receptionist_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli auto-receptionists assign-phone-numbers`** - Assigns available phone numbers to an auto receptionist. The available numbers can be retrieved using the List Phone Numbers API with `type` query parameter set to &quot;unassigned&quot;.

Prerequisites:
* Pro or higher account plan with Zoom Phone License
* Account owner or admin permissions  
 


Scopes: `phone:write:admin`

Granular Scopes: `phone:write:auto_receptionist_number:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli auto-receptionists delete`** - Deletes a non-primary auto receptionist.

An auto receptionist answers calls with a personalized recording and routes calls to a phone user, call queue, common area (phone), or to a voicemail. An auto receptionist can also be set up so that it routes calls to an interactive voice response (IVR) system to allow callers to select the routing options.


Prerequisites:
* Pro or higher account with Zoom Phone license.



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:auto_receptionist:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli auto-receptionists delete-policy`** - Removes the policy subsetting for a specific auto receptionist according to the `policyType`. For example, you can use this API to remove shared access members.

Prerequisites:
* Pro or higher account plan with Zoom Phone License
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:auto_receptionist_policy:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli auto-receptionists delete-setting`** - Delete the auto receptionist setting according to the setting type.

Prerequisites
* A Pro or higher account with a Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:auto_receptionist_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli auto-receptionists get-call-handling-settings`** - Returns the call handling setting of an auto receptionist for the specified hour type.

Prerequisites
* A Business or Enterprise account
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:auto_receptionist_call_handling_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli auto-receptionists get-detail`** - Returns information on a specific auto receptionist.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:auto_receptionist:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli auto-receptionists get-ivr`** - Returns an interactive voice response (IVR) system of the specified auto receptionist.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:auto_receptionist_ivr:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli auto-receptionists get-policy`** - Returns the policy setting of a specific auto receptionist.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:auto_receptionist_policy:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli auto-receptionists get-settings`** - Returns auto receptionist settings.

Prerequisites:
* A Pro or higher account with a Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:auto_receptionist_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli auto-receptionists list`** - Returns a list of auto receptionists.

Prerequisites:
* Pro or a higher account with Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_auto_receptionists:admin`

Rate Limit Label: `Medium`
- **`zoom-phone-cli auto-receptionists unassign-all-phone-nums`** - Unassigns all phone numbers that were previously assigned to an auto receptionist. 

Prerequisites:
* Pro or higher account plan with Zoom Phone License
* Account owner or admin permissions  
 


Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:auto_receptionist_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli auto-receptionists unassign-aphone-num`** - Unassigns a specific phone number that was previously assigned to an auto receptionist. 

Prerequisites:
* Pro or higher account plan with Zoom Phone License
* Account owner or admin permissions  
 


Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:auto_receptionist_number:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli auto-receptionists update`** - Changes information such as the display name and the extension number assigned to the main auto receptionist.

An auto receptionist answers calls with a personalized recording. And it routes calls to a phone user, call queue, common area, or voicemail. An auto receptionist can also be set up so that it routes calls to an interactive voice response (IVR) system to allow callers to select the routing options.


Prerequisites:
* Pro or higher account with Zoom Phone license.



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:auto_receptionist:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli auto-receptionists update-call-handling-settings`** - Update the call handling setting of an auto receptionist for the specified hour type.

Prerequisites:
* A Business or Enterprise account
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:auto_receptionist_call_handling_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli auto-receptionists update-ivr`** - Updates the interactive voice response (IVR) system of the specified auto receptionist.

Prerequisites:
 * A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:auto_receptionist_ivr:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli auto-receptionists update-policy`** - Updates the policy setting of a specific auto receptionist. 

Prerequisites:
* Pro or higher account plan with Zoom Phone License
* Account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:auto_receptionist_policy:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli auto-receptionists update-policy-phone`** - Updates the policy subsetting of a specific auto receptionist according to the `policyType`. For example, you can use this API to update shared access members.

Prerequisites
* Pro or higher account plan with Zoom Phone License
* Account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:auto_receptionist_policy:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli auto-receptionists update-setting`** - Updates the auto receptionist setting according to the setting type.

Prerequisites
* Pro or a higher account with Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:auto_receptionist_setting:admin`

Rate Limit Label: `LIGHT`

### batch_locations

Manage batch locations

- **`zoom-phone-cli batch-locations`** - Adds emergency service locations in batch. 

Prerequisites:
* Pro or higher account plan with Zoom phone license
* Account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:batch_emergency_locations:admin`

Rate Limit Label: `LIGHT`

### billing_accounts

Manage billing accounts

- **`zoom-phone-cli billing-accounts get-abilling-account`** - A Zoom account owner or a user with admin privilege can use this API to get information about a billing account.

Prerequisites:
* Pro or higher account plan with Zoom phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:billing_account:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli billing-accounts list`** - Retrieves a list of available billing accounts for a Zoom account owner or a user with admin privileges. 

If the multiple sites option has been enabled for the account, then `field(site_id)` is required.

Prerequisites:
* Pro or higher account plan with Zoom phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_billing_accounts:admin`

Rate Limit Label: `MEDIUM`

### blocked_list

Manage blocked list

- **`zoom-phone-cli blocked-list add-anumber-to`** - Creates a block list and add a number to the list. 

A Zoom account owner or a user with the admin privilege can block phone numbers for phone users in an account. 

Blocked numbers can be inbound (numbers will be blocked from calling in) and outbound (phone users in your account won't be able to dial those numbers). 

Blocked callers will hear a generic message stating that the person they are calling is not available. 

Prerequisites:
* Pro or higher account plan with Zoom phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:blocked_list:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli blocked-list delete-ablocked-list`** - Deletes a blocked list, which removes the associated number from the blocked list. 

The number will be unblocked after the deletion. A Zoom account owner or a user with admin privilege can block phone numbers for phone users in an account. Blocked numbers can be inbound (numbers will be blocked from calling in) and outbound (phone users in your account won't be able to dial those numbers).

Prerequisites:
* Pro or higher account plan with Zoom phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:blocked_list:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli blocked-list get-ablocked-list`** - Returns the information about a specific blocked list. 

A Zoom account owner or a user with admin privilege can block phone numbers for phone users in an account. Blocked numbers can be inbound (numbers will be blocked from calling in) and outbound (phone users in your account won't be able to dial those numbers). Blocked callers will hear a generic message stating that the person they are calling is not available.

Prerequisites:
* Pro or higher account plan with Zoom phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:blocked_list:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli blocked-list list`** - Returns all the blocked lists in an acccount. A Zoom account owner or a user with admin privilege can block phone numbers for phone users in an account. Blocked numbers can be inbound (numbers will be blocked from calling in) and outbound (phone users in your account won't be able to dial those numbers). Blocked callers will hear a generic message stating that the person they are calling is not available. 

Prerequisites:
* Pro or higher account plan with Zoom phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_blocked_lists:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli blocked-list update`** - Updates the information in the blocked list.

A Zoom account owner or a user with admin privilege can block phone numbers for phone users in an account. Blocked numbers can be inbound (numbers will be blocked from calling in) and outbound (phone users in your account won't be able to dial those numbers). Blocked callers hear a generic message stating that the person they are calling is not available.

Prerequisites:
* Pro or higher account plan with Zoom phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:blocked_list:admin`

Rate Limit Label: `LIGHT`

### byoc_numbers

Manage byoc numbers

- **`zoom-phone-cli byoc-numbers`** - Adds BYOC (Bring Your Own Carrier) phone numbers to Zoom Phone.

Prerequisites 
* A Business or Enterprise plan 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:byo_carrier_number:admin`

Rate Limit Label: `LIGHT`

### call_element

Manage call element

- **`zoom-phone-cli call-element get`** - Returns the call element for a given call element ID.
 
 Prerequisites 
 * A Business or Enterprise account 
 * A Zoom Phone license

Scopes: `phone_call_log:read`,`phone_call_log:read:admin`,`phone:read`

Granular Scopes: `phone:read:call_log:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-element get-phone-recording-by-id`** - Returns an account's call recording by the call element's `callElementId`.
Prerequisites 
* A Pro or higher account with Zoom Phone license 
* Account owner or admin privileges

Scopes: `phone:read:admin`,`phone_recording:read`,`phone_recording:read:admin`,`phone:read`

Granular Scopes: `phone:read:call_recording`,`phone:read:call_recording:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-element get-voicemail-details-by-id`** - Returns the detailed information on a voicemail by the call element's `callElementId`.
Prerequisites 
* A Pro or higher account with Zoom Phone license 
* Account owner or admin privileges

Scopes: `phone:read`,`phone:read:admin`,`phone_voicemail:read`,`phone_voicemail:read:admin`

Granular Scopes: `phone:read:voicemail`,`phone:read:voicemail:admin`

Rate Limit Label: `LIGHT`

### call_history

Manage call history

- **`zoom-phone-cli call-history account`** - Returns an account's new edition of call logs.

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license 
* Account owner or a role with Zoom Phone management

Scopes: `phone:read:admin`,`phone_call_log:read:admin`

Granular Scopes: `phone:read:list_call_logs:admin`

Rate Limit Label: `HEAVY`
- **`zoom-phone-cli call-history add-client-code-to`** - Adds a client code to a call log. You can track call logs with a client code. 

Prerequisites
* Business or Education account
* Zoom Phone license 

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:call_log:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-history get-call-path`** - Returns information about a call log.

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`,`phone_call_log:read:admin`

Granular Scopes: `phone:read:call_log:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-history get-detail`** - Returns the call history details for a given call log ID call history. 
 
 Prerequisites 
 * A Business or Enterprise account 
 * A Zoom Phone license

Scopes: `phone_call_log:read`,`phone_call_log:read:admin`,`phone:read`

Granular Scopes: `phone:read:call_log:admin`

Rate Limit Label: `LIGHT`

### call_logs

Manage call logs

- **`zoom-phone-cli call-logs account`** - Returns an account's call logs.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license 
* Account owner or a role with Zoom Phone management



Scopes: `phone:read:admin`,`phone_call_log:read:admin`

Granular Scopes: `phone:read:list_call_logs:admin`

Rate Limit Label: `HEAVY`
- **`zoom-phone-cli call-logs add-client-code-to`** - Adds a client code to a call log. You can track call logs with a client code. 

Prerequisites:
* Business or Education account
* Zoom Phone license 





Scopes: `phone:write:admin`

Granular Scopes: `phone:update:call_log:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-logs get-details`** - Returns information about a call log.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`,`phone_call_log:read:admin`,`phone:read`,`phone_call_log:read`

Granular Scopes: `phone:read:call_log:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-logs get-phone-recordings-by-call-id-or-id`** - Returns an account's call recording by the recording's `callId` or `callLogId`. 

Note: Returns the `file_url` in the JSON query results.

Prerequisites 
* A Pro or higher account with Zoom Phone license 
* Account owner or admin privileges

Scopes: `phone:read:admin`,`phone_recording:read`,`phone_recording:read:admin`

Granular Scopes: `phone:read:call_recording`,`phone:read:call_recording:admin`

Rate Limit Label: `LIGHT`

### call_queue_analytics

Manage call queue analytics

- **`zoom-phone-cli call-queue-analytics`** - Returns the call queue analytics overview.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_call_queues:admin`

Rate Limit Label: `MEDIUM`

### call_queues

Manage call queues

- **`zoom-phone-cli call-queues add-cqpolicy-sub-setting`** - Adds the policy subsetting for a specific call queue according to the `policyType`. For example, you can use set up shared access members. 

Prerequisites 
* Pro or higher account with Zoom Phone license.
* Account owner or admin privileges 



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:call_queue_policy:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues add-members-to`** - Adds phone users or common areas as members to a specific call queue.

Prerequisites:
* Pro or higher account plan.
* Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:call_queue_member:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues add-setting`** - Adds a call queue setting according to the setting type.

Prerequisites
* A Pro or higher account with a Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:call_queue_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues addmemberstoacustomgroup`** - Adds call queue members to a custom group.

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:call_queue_custom_group_member:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues assign-phone-to`** - Assigns numbers to a call queue. After buying phone number(s), you can assign it and allow callers to directly dial a number to reach a call queue.

Prerequisites:
- Pro or higher account plan
* Account owner or admin permissions
- Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:call_queue_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues create`** - Creates a call queue. Call queues allow you to route incoming calls to a group of users. For instance, you can use call queues to route calls to various departments in your organization such as sales, engineering, billing, customer service, and so on.
 You can add phone users or common areas to call queues.

Prerequisites:

* Pro, Business, or Education account
* Account owner or admin permissions
* Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:call_queue:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues createacustomgroupinacallqueue`** - Scopes: `phone:write:admin`

Granular Scopes: `phone:write:call_queue_custom_group:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues delete-acall-queue`** - Call queues allow you to route incoming calls to a group of users. For instance, you can use call queues to route calls to various departments in your organization such as sales, engineering, billing, customer service etc.
 Use this API to delete a Call Queue.
 
Prerequisites:

* Pro, Business, or Education account
* Account owner or admin permissions
* Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:call_queue:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli call-queues delete-setting`** - Delete the call queue setting by the setting type.

Prerequisites
* A Pro or higher account with a Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:call_queue_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues deleteacustomgroup`** - Delete a custom group.

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:call_queue_custom_group:admin`,`phone:delete:call_queue_custom_group:master`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues get-acall-queue`** - Routes incoming calls to a group of users and returns information on a specific call queue. 

For instance, you can use call queues to route calls to various departments in your organization such as sales, engineering, billing, customer service, and so on. 


 
Prerequisites:

* Pro, Business, or Education account
* Account owner or admin permissions
* Zoom Phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:call_queue:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues get-call-handling-setting`** - Returns the call handling setting of a call queue for the specified hour type.

Prerequisites
* A Business or Enterprise account
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:call_queue_call_handling_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues get-policy`** - Returns the policy setting of a specific [call queue].

Prerequisites:
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:call_queue_policy:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues get-recordings`** - Use this API to view call recordings from the call queue.


Prerequisites:

* Pro or higher account with Zoom Phone license.
* Automatic call recordings must be enabled in the Policy Settings for call queues. 
 

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_call_queue_recordings:admin`

Rate Limit Label: `Medium`
- **`zoom-phone-cli call-queues get-settings`** - Returns call queue settings.

Prerequisites:
* Pro or higher account with Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:call_queue_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues getacustomgroup`** - Scopes: `phone:read:admin`,`phone:master`

Granular Scopes: `phone:read:call_queue_custom_group:admin`,`phone:read:call_queue_custom_group:master`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues list`** - Call queues allow you to route incoming calls to a group of users. For instance, you can use call queues to route calls to various departments in your organization such as sales, engineering, billing, customer service etc.
 Use this API to list Call queues.


Prerequisites:

* Pro, Business, or Education account
* Account owner or admin permissions
* Zoom Phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_call_queues:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli call-queues list-members`** - Returns a list of call queue members.


Prerequisites:

* Pro, Business, or Education account
* Account owner or admin permissions
* Zoom Phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_call_queue_members:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli call-queues listcustomgroupsofacallqueue`** - Returns a list of custom groups for a call queue.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:call_queue_custom_group:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues remove-cqpolicy-sub-setting`** - Use this API to remove the policy sub-setting for a specific call queue according to the `policyType`. For example, you can use this API to remove shared access members. 

Prerequisites: 
* Pro or higher account with Zoom Phone license.
* Account owner or admin privileges 



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:call_queue_policy:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues removeamemberfromacustomgroup`** - Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:call_queue_custom_group_member:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues un-assign-phone-num`** - Unbinds a phone number from a call queue. After assigning a phone number, you can unbind it if you don't want it to be assigned to a call queue.  After successful unbinding, the number will appear in the Unassigned tab.


Prerequisites:
* Pro or higher account palan
* Account owner or admin permissions
* Zoom Phone license 

 

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:call_queue_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues unassign-all-members`** - Removes all members from a call queue who were previously assigned to that call queue. The members could be phone users or common areas.

Prerequisites:

* Pro or higher account plan.
* Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:call_queue_member:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli call-queues unassign-aphone-num`** - Unbinds all phone numbers that are assigned to a call queue.  After successful unbinding, the numbers will appear in the Unassigned tab.
 
 If you only need to unassign a specific phone number, use the Unassign a Phone Number API instead. 

Prerequisites:
* Pro or higher account palan
* Account owner or admin permissions
* Zoom Phone license 

 

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:call_queue_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues unassign-member-from`** - Removes a member who was previously added to a call queue. The member could be a phone user or common area. Note that you cannot use this API to unassign a call queue manager. 

Prerequisites:

* Pro or higher account plan.
* Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:call_queue_member:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli call-queues update`** - Updates information of a specific call queue. Call queues allow you to route incoming calls to a group of users. For instance, you can use call queues to route calls to various departments in your organization such as sales, engineering, billing, customer service, and so on.
  
Prerequisites:

* Pro, Business, or Education account
* Account owner or admin permissions
* Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:call_queue:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues update-call-handling-setting`** - Updates the call handling setting of a call queue for the specified hour type. Only the fields included in the request body are updated; omitted fields are left unchanged. Display-only fields (extension number/name, audio name, external contact name/email, Zoom Contact Center / Microsoft Teams display names) are derived on the server side and must not be sent in the request body.

Prerequisites:
* A Business or Enterprise account
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:call_queue_call_handling_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues update-cqpolicy-sub-setting`** - Updates the policy subsetting for a specific call queue according to the `policyType`. For example, you can use this API to update shared access members.

Prerequisites: 
* Pro or higher account with Zoom Phone license.
* Account owner or admin privileges 



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:call_queue_policy:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues update-policy`** - Updates a call queue's policy settings.

Prerequisites
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:call_queue_policy:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues update-setting`** - Update the call queue setting according to the setting type.

Prerequisites
* A Pro or higher account with a Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:call_queue_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli call-queues updateacustomgroup`** - Update a custom group.

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:call_queue_custom_group:admin`

Rate Limit Label: `LIGHT`

### calling_plans

Manage calling plans

- **`zoom-phone-cli calling-plans`** - Returns all of an account's Zoom Phone calling plans.

Prerequisites 
* A Pro or a higher account 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_calling_plans:admin`

Rate Limit Label: `MEDIUM`

### carrier_peering

Manage carrier peering

- **`zoom-phone-cli carrier-peering`** - Returns phone numbers that the carrier pushed to different customers.
To become a peering provider/ carrier, submit your request.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`,`phone_peering:read:admin`

Granular Scopes: `phone:read:list_peering_numbers:admin`

Rate Limit Label: `LIGHT`

### carrier_reseller

Manage carrier reseller

- **`zoom-phone-cli carrier-reseller active-crphone-numbers`** - Changes the phone number status to `active` in a carrier reseller's master account. It can change up to 200 numbers at a time.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:carrier_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli carrier-reseller create-crphone-numbers`** - Adds phone numbers to a carrier reseller (master) account. Up to 200 numbers at a time. If this API is called in MA mode, it also has functions of distribution.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:carrier_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli carrier-reseller delete-crphone-number`** - Deletes or unassigns a phone number from a carrier reseller account.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:carrier_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli carrier-reseller list-crphone-numbers`** - Lists phone numbers in a carrier reseller (master) account that can be pushed to its subaccounts.Prerequisites: * A Pro or higher account plan * A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_carrier_numbers:admin`

Rate Limit Label: `MEDIUM`

### common_areas

Manage common areas

- **`zoom-phone-cli common-areas add`** - Adds an instance of a common area. You can configure devices shared by users and deployed in shared spaces.

Prerequisites
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:common_area:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas add-outbound-calling-exception-rule`** - Adds the common area level outbound calling policy exception rule for the country region.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:common_area_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas add-setting`** - Adds the common area setting according to the setting type.

Prerequisites
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:common_area_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas apply-templateto`** - Applies a template to common areas.  You can add up to 50 common areas at a time.

Prerequisites
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions


Scopes: `phone:write:admin`

Granular Scopes: `phone:write:apply_template_to_common_areas:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas assign-calling-plans-to`** - Assigns calling plans to a common area.

Prerequisites
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:common_area_calling_plan:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas assign-phone-numbers-to`** - Assigns phone numbers to a common area.

Prerequisites
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:common_area_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas delete`** - Removes the common area. 

Prerequisites:
* Pro or a higher account with Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:common_area:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas delete-outbound-calling-exception-rule`** - Deletes the common area level outbound calling exception rule.
Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:common_area_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas delete-setting`** - Removes the common area subsetting from desk phones. 

Prerequisites
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:common_area_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas generateactivationcodesforcommonareas`** - Generates activation codes for common areas. You can add up to 50 common areas at a time.

Prerequisites
- Pro or a higher account with Zoom Phone license
- Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:common_area:admin`

Rate Limit Label: `LIGHT`

Not supported in Gov cluster
- **`zoom-phone-cli common-areas get-acommon-area`** - Returns detailed information on the common area.

Prerequisites  
 
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:common_area:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas get-call-handling-setting`** - Returns the call handling setting of a common area for the specified hour type.

Prerequisites:
* A Business or Enterprise account
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:common_area_call_handling_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas get-outbound-calling-countries-and-regions`** - Returns the common area level outbound calling countries and regions.

Prerequisites:
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:common_area_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas get-settings`** - Returns common area settings.

Prerequisites:
* Pro or a higher account with Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_common_area_settings:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas list`** - Lists common areas under an account.

Prerequisites

* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:common_area:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli common-areas list-activation-codes`** - Returns a list of activation code information of the common areas under an account.

Prerequisites
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_common_area_activation_codes:admin`

Rate Limit Label: `MEDIUM`

Not supported in Gov cluster
- **`zoom-phone-cli common-areas list-outbound-calling-exception-rule`** - Lists the common area level outbound calling policy exception rules.

Prerequisite:
* Pro or higher account plan with Zoom phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:common_area_outbound_calling_rule:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli common-areas unassign-calling-plans-from`** - Use this API to unassign a calling plan from the common area.

Prerequisites: 
* A Pro or higher account with a Zoom Phone license 
* An account owner or admin permissions 



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:common_area_calling_plan:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas unassign-phone-numbers-from`** - Unassigns a phone number from a common area.

Prerequisites
* A Pro or a higher account with a Zoom Phone license
* An account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:common_area_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas update`** - Updates the common area information.

Prerequisites

* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:common_area:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas update-call-handling-setting`** - Update the call handling setting of a common area for the specified hour type.

Prerequisites
* A Business or Enterprise account
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:common_area_call_handling_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas update-outbound-calling-countries-or-regions`** - Updates the common area level outbound calling policy country or region.

Prerequisite:
* Account must have a Pro or a higher plan with a Zoom Phone license.

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:common_area_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas update-outbound-calling-exception-rule`** - Updates the common area level outbound calling policy for the country region exception rule.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:common_area_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas update-pin-code`** - Updates the common area pin code.

Prerequisites

* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions, This depends on whether The PIN cannot be viewed by the admin is enabled.  



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:common_area:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli common-areas update-setting`** - Updates the common area setting according to the setting type.

Prerequisites
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:common_area_setting:admin`

Rate Limit Label: `LIGHT`

### devices

Manage devices

- **`zoom-phone-cli devices add-extensions-to-adevice`** - By default, all Zoom Phone users can make and receive calls using the Zoom desktop and mobile applications. Additionally, if a desk phone is required, use this API to add a desk phone and assign it to a user.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions
* Supported device



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:device_extension:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli devices add-phone`** - Adds an unassigned desk phone or an assigned (to a user or a common area) desk phone.

Prerequisites:  
 
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions
* Supported device  
 


Scopes: `phone:write:admin`

Granular Scopes: `phone:write:device:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli devices batch-update-line-key-setting`** - Use this API to batch update the Zoom Phone device line key position information.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license



Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:update:device_line_keys`,`phone:update:device_line_keys:admin`

Rate Limit Label: `Heavy`
- **`zoom-phone-cli devices delete-adevice`** - Remove a desk phone device or ATA (Analog Telephone Adapter) from the Zoom Phone System Management.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions
* Device must not have been assigned to a user.



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:device:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli devices delete-extension-from-adevice`** - Use this API to unassign a specific assignee from the device.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions. 



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:device_extension:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli devices get-adevice`** - Returns detailed information about a specific desk phone device.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:device:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli devices list-line-key-setting`** - Use this API to get information on the Zoom Phone device line keys settings and position.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license



Scopes: `phone:read:admin`,`phone:read`

Granular Scopes: `phone:read:device_line_keys`,`phone:read:device_line_keys:admin`

Rate Limit Label: `Heavy`
- **`zoom-phone-cli devices list-phone`** - Lists all the desk phone devices that are configured with Zoom Phone on an account.

Prerequisites:  
 
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_devices:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli devices reboot-phone`** - Use this API to reboot an online zero-touch or assisted-provisioning device. You can only send one request every second.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions. 



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:reboot_device:admin`

Rate Limit Label: `Heavy`
- **`zoom-phone-cli devices sync-phone`** - Use this API to resync all online zero-touch or assisted-provisioning devices in an account or a site. Only allows sending one request every 15 minutes.

Prerequisites:

* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions


Scopes: `phone:write:admin`

Granular Scopes: `phone:write:sync_device:admin`

Rate Limit Label: `Heavy`
- **`zoom-phone-cli devices update-adevice`** - Updates the information of a desk phone device.

Prerequisites:
* Pro or a higher account with Zoom Phone license* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:device:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli devices update-provision-template-to`** - Use this API to assign a provision template to a device or remove a provision template from the device by leaving `templateId` empty.

Prerequisites:  
 
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions
* Supported device  
 


Scopes: `phone:write:admin`

Granular Scopes: `phone:update:device_provision_template:admin`

Rate Limit Label: `Light`

### dial_by_name_directory

Manage dial by name directory

- **`zoom-phone-cli dial-by-name-directory add-users-to-directory`** - Use this API to add users to a directory.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:directory:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli dial-by-name-directory delete-users-from-directory`** - Use this API to delete users from the directory.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:directory:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli dial-by-name-directory list-users-from-directory`** - Use this API to get users that are in or not in a directory.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:directory:admin`

Rate Limit Label: `Medium`

### emergency_addresses

Manage emergency addresses

- **`zoom-phone-cli emergency-addresses add-emergency-address`** - Adds an emergency address. If the address provided is not an exact match, the system generated corrected address will be used. 

Prerequisites: 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:emergency_address:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli emergency-addresses delete-emergency-address`** - Removes an emergency address.

Prerequisites: 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:emergency_address:admin`

Rate Limit Label: `Heavy`
- **`zoom-phone-cli emergency-addresses get-emergency-address`** - Gets the emergency address information.

Prerequisites: 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions  
 

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:emergency_address:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli emergency-addresses list`** - Lists the emergency addresses.

Prerequisites 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_emergency_addresses:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli emergency-addresses update-emergency-address`** - Updates emergency address information. If the provided address does not exactly match, the system uses the corrected address it generates.

Prerequisites 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:emergency_address:admin`

Rate Limit Label: `LIGHT`

### emergency_number_pools

Manage emergency number pools

- **`zoom-phone-cli emergency-number-pools assign-phone-number-to`** - Assigns phone numbers to the Emergency Number Pool.

Only full PSTN numbers (numbers that support both incoming and outgoing calls) with a stable (`normal`) status can be assigned. 

You can assign up to 300 numbers per request (`phone_numbers`), and up to a total of 1,000 numbers per pool per assignment operation.

Prerequisites
* A Business or Enterprise account
* A Zoom Phone license
* Route emergency calls to PSAP must be enabled, and the Caller ID method for Emergency Service Routing  -  for the site if Multiple Sites is enabled, or for the account if Multiple Sites is disabled  -  must already be set to Temporary number from local emergency pool.

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:emergency_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli emergency-number-pools list-account-numbers`** - Returns phone numbers in the account-level Emergency Number Pool.

This endpoint can only be used for accounts where Multiple Sites is disabled. For accounts with Multiple Sites enabled, use List site emergency number pool phone numbers instead.

Prerequisites:
* A Business or Enterprise account
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:emergency_number:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli emergency-number-pools unassign-phone-number-from`** - Unassigns phone numbers from the Emergency Number Pool.

Prerequisites
* A Business or Enterprise account
* A Zoom Phone license
* If the Caller ID method for Emergency Service Routing  -  for the site if Multiple Sites is enabled, or for the account if Multiple Sites is disabled  -  is currently set to Temporary number from local emergency pool, at least one number must remain assigned to the pool; unassigning the last remaining number fails with a `400` error (see Responses below).

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:emergency_number:admin`

Rate Limit Label: `LIGHT`

### emergency_service_settings

Manage emergency service settings

- **`zoom-phone-cli emergency-service-settings get`** - Returns the complete emergency service configuration in a single response: "Route Emergency Calls to Public Safety Answering Point (PSAP)", "Route Emergency Calls to Internal Safety Response Team", "Nomadic Emergency Services", and "Emergency Address Management", each with its sub-settings. 

This single endpoint serves both account-level and site-level configuration: 
* When Multiple Sites is enabled for the account, `site_id` is required and the specified site's configuration is returned. 
* When Multiple Sites is disabled, `site_id` must be omitted and the account-level configuration is returned. 

Prerequisites: 
* Pro or a higher account with Zoom Phone license. 
* Account owner or admin permissions.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:emergency_service_setting:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli emergency-service-settings update`** - Updates one emergency service setting group per call. Exactly one of `psap`, `internal_safety_response_team`, `nomadic_emergency_service` or `emergency_address_management` must be present in the request body; supplying none or more than one returns a validation error. Within the supplied group, omitted fields keep their current values. 

This single endpoint serves both account-level and site-level configuration: 
* When Multiple Sites is enabled for the account, `site_id` is required in the request body and the specified site's configuration is updated. 
* When Multiple Sites is disabled, `site_id` must be omitted and the account-level configuration is updated. 

Prerequisites: 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:emergency_service_setting:admin`

Rate Limit Label: `HEAVY`

### emergency_services

Manage emergency services

- **`zoom-phone-cli emergency-services add-account-emergency-routing-rule`** - Adds an emergency routing rule for an account. You can define additional emergency service numbers, special service numbers, and call handling settings associated with the account. 

Prerequisites 

* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions 
* This is an account-level API. It can only be used when Multiple Sites is disabled for the account. Use the corresponding site-level emergency routing rule API instead when Multiple Sites is enabled.

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:emergency_services_routing_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli emergency-services delete-account-emergency-routing-rule`** - Removes an account's emergency routing rule.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions
* This is an account-level API. It can only be used when Multiple Sites is disabled for the account. Use the corresponding site-level emergency routing rule API instead when Multiple Sites is enabled.

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:emergency_services_routing_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli emergency-services list-account-emergency-routing-rules`** - Lists all of an account's emergency routing rules (both Zoom default and customized) in a single response.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions
* This is an account-level API. It can only be used when Multiple Sites is disabled for the account. Use the corresponding site-level emergency routing rule list API instead when Multiple Sites is enabled.

Note: The response does not include a routing rule ID. Use the returned `number_pattern` value as the `routingRuleIdentifier` when calling the update or delete emergency routing rule APIs.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_emergency_services_routing_rules:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli emergency-services update-account-emergency-routing-rule`** - Updates an account's emergency routing rule.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions
* This is an account-level API. It can only be used when Multiple Sites is disabled for the account. Use the corresponding site-level emergency routing rule API instead when Multiple Sites is enabled.

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:emergency_services_routing_rule:admin`

Rate Limit Label: `MEDIUM`

### extension

Manage extension

- **`zoom-phone-cli extension add-call-handling`** - Adds Zoom Phone call handling subsettings for your phone system. Call handling settings allow you to control how your system routes calls during business, closed, or holiday hours. For more information, see our API guide or Zoom support article Customizing call handling settings.

Applicable to user, call queue, auto receptionist, or shared line group call handling at this time.

Prerequisites:
* A Pro or higher account with Zoom Phone enabled

Deprecation notice: This API is planned for deprecation.

Use the resource-specific APIs instead:
* To add user call forwarding settings, use `POST /phone/users/{userId}/call_forward_settings/{hourType}`.
* To add holiday hour settings for users, use `POST /phone/users/{userId}/profile_settings/{settingType}`.
* To add holiday hour or break hour settings for common areas, use `POST /phone/common_areas/{commonAreaId}/settings/{settingType}`.
* To add holiday hour or break hour settings for auto receptionists, use `POST /phone/auto_receptionists/{autoReceptionistId}/settings/{settingType}`.
* To add holiday hour or break hour settings for call queues, use `POST /phone/call_queues/{callQueueId}/settings/{settingType}`.
* To add holiday hour or break hour settings for shared line groups, use `POST /phone/shared_line_groups/{sharedLineGroupId}/settings/{settingType}`.


Scopes: `phone:write:admin`

Granular Scopes: `phone:write:call_handling_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli extension add-extensiont-level-inbound-block-rules`** - Adds the given extension's block rule for inbound calls and messaging. 

It adds the inbound block rule for the given Call Queue, Auto Receptionist, Shared Line Group, Common Area, Zoom Room or User.

Prerequisites:
* Pro or a higher account with Zoom Phone license

Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:write:extension_inbound_block_rule:admin`,`phone:write:extension_inbound_block_rule`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli extension batch-update-line-key-setting`** - Use this API to batch update the Zoom Phone line key settings information.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license



Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:update:line_keys`,`phone:update:line_keys:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli extension delete-call-handling`** - Deletes a Zoom Phone call handling settings. Call handling settings let you control how your system routes calls during business, closed, or holiday hours. For more information, read our API guide or Zoom support article Customizing call handling settings.

Applicable to user, call queue, auto receptionist, or shared line group call handling at this time.

Prerequisites:
* Pro or a higher account with Zoom Phone enabled

Deprecation notice: This API is planned for deprecation.

Use the resource-specific APIs instead:
* To delete user call forwarding settings, use `DELETE /phone/users/{userId}/call_forward_settings/{hourType}`.
* To delete holiday hour settings for users, use `DELETE /phone/users/{userId}/profile_settings/{settingType}`.
* To delete holiday hour or break hour settings for common areas, use `DELETE /phone/common_areas/{commonAreaId}/settings/{settingType}`.
* To delete holiday hour or break hour settings for auto receptionists, use `DELETE /phone/auto_receptionists/{autoReceptionistId}/settings/{settingType}`.
* To delete holiday hour or break hour settings for call queues, use `DELETE /phone/call_queues/{callQueueId}/settings/{settingType}`.
* To delete holiday hour or break hour settings for shared line groups, use `DELETE /phone/shared_line_groups/{sharedLineGroupId}/settings/{settingType}`.


Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:call_handling_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli extension delete-extensiont-level-inbound-block-rules`** - Deletes the given extension's blocked rule for inbound calls and messaging. 

Use this API to delete inbound blocked rule for the given Call Queue, Auto Receptionist, Shared Line Group, Common Area, Zoom Room or User.


Prerequisites:
* Pro or a higher account with Zoom Phone license

Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:delete:extension_inbound_block_rule:admin`,`phone:delete:extension_inbound_block_rule`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli extension delete-line-key`** - Use this API to delete the Zoom Phone line key settings information.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license



Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:delete:line_keys`,`phone:delete:line_keys:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli extension get-call-handling`** - Returns information about a Zoom Phone call handling settings.

Call handling settings let you control how your system routes calls during business, closed, or holiday hours. For more information, read our API guide or Zoom support article Customizing call handling settings.

Applicable to user, call queue, auto receptionist, or shared line group call handling at this time.

Prerequisites:
* Pro or a higher account with Zoom Phone enabled

Deprecation notice: This API is planned for deprecation.
Use the resource-specific APIs instead:
* For user call handling settings, use `GET /phone/users/{userId}/call_handling/settings`.
* For common area call handling settings, use `GET /phone/common_areas/{commonAreaId}/call_handling/settings`.
* For auto receptionist call handling settings, use `GET /phone/auto_receptionists/{autoReceptionistId}/call_handling/settings`.
* For call queue call handling settings, use `GET /phone/call_queues/{callQueueId}/call_handling/settings`.
* For shared line group call handling settings, use `GET /phone/shared_line_groups/{sharedLineGroupId}/call_handling/settings`.
* For user profile, holiday hour, custom hour, or other user setting data, use `GET /phone/users/{userId}/settings`.
* For common area holiday hour, custom hour, or other setting data, use `GET /phone/common_areas/{commonAreaId}/settings`.
* For auto receptionist holiday hour, custom hour, or other setting data, use `GET /phone/auto_receptionists/{autoReceptionistId}/settings`.
* For call queue holiday hour, custom hour, or other setting data, use `GET /phone/call_queues/{callQueueId}/settings`.
* For shared line group holiday hour, custom hour, or other setting data, use `GET /phone/shared_line_groups/{sharedLineGroupId}/settings`.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_call_handling_settings:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli extension getuser-sfaxlogs`** - Returns the extension's fax logs.

Prerequisites
* User must belong to a Business or Enterprise account.
* User must have a Zoom Phone license.

Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:list_fax_log`,`phone:read:list_fax_log:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli extension list-level-inbound-block-rules`** - Returns a list of the given extension's block rule for inbound calls and messaging. 

it lists inbound block rule for the given Call Queue, Auto Receptionist, Shared Line Group, Common Area, Zoom Room, or User.

Prerequisites:
* Pro or a higher account with Zoom Phone license

Scopes: `phone:read:admin`,`phone:read`

Granular Scopes: `phone:read:list_extension_inbound_block_rules:admin`,`phone:read:list_extension_inbound_block_rules`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli extension list-line-key-setting`** - Use this API to get the Zoom Phone line key settings information.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license



Scopes: `phone:read:admin`,`phone:read`

Granular Scopes: `phone:read:line_keys`,`phone:read:line_keys:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli extension update-call-handling`** - Updates a Zoom Phone call handling setting.

Call handling settings allow you to control how your system routes calls during business, closed, or holiday hours. For more information, read our Call Handling API guide or Zoom support article Customizing call handling settings.

Applicable to user, call queue, auto receptionist, or shared line group call handling at this time.

Prerequisites:
* Pro or a higher account with Zoom Phone enabled

Deprecation notice: This API is planned for deprecation.
Use the resource-specific APIs instead:
* For user call handling settings, including user call forwarding settings, use `PATCH /phone/users/{userId}/call_handling/settings/{hourType}`.
* For common area call handling settings, use `PATCH /phone/common_areas/{commonAreaId}/call_handling/settings/{hourType}`.
* For auto receptionist call handling settings, use `PATCH /phone/auto_receptionists/{autoReceptionistId}/call_handling/settings/{hourType}`.
* For call queue call handling settings, use `PATCH /phone/call_queues/{callQueueId}/call_handling/settings/{hourType}`.
* For shared line group call handling settings, use `PATCH /phone/shared_line_groups/{sharedLineGroupId}/call_handling/settings/{hourType}`.
* For user profile, holiday hour, custom hour, or other settings, use `PATCH /phone/users/{userId}/settings`.
* For common area holiday hour, custom hour, or other settings, use `PATCH /phone/common_areas/{commonAreaId}/settings/{settingType}`.
* For auto receptionist holiday hour, custom hour, or other settings, use `PATCH /phone/auto_receptionists/{autoReceptionistId}/settings/{settingType}`.
* For call queue holiday hour, custom hour, or other settings, use `PATCH /phone/call_queues/{callQueueId}/settings/{settingType}`.
* For shared line group holiday hour, custom hour, or other settings, use `PATCH /phone/shared_line_groups/{sharedLineGroupId}/settings/{settingType}`.

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:call_handling_setting:admin`

Rate Limit Label: `LIGHT`

### external_contacts

Manage external contacts

- **`zoom-phone-cli external-contacts add`** - Adds an external contact.

Prerequisites 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:external_contact:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli external-contacts delete-aexternal-contact`** - Removes an external contact.

Prerequisites: 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:external_contact:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli external-contacts get-aexternal-contact`** - Returns an external contact's information.

Prerequisites 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions  
 

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:external_contact:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli external-contacts list`** - Lists the external contacts.

Prerequisites 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_external_contacts:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli external-contacts update`** - Updates an external contact's information.

Prerequisites 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:external_contact:admin`

Rate Limit Label: `LIGHT`

### fax

Manage fax

- **`zoom-phone-cli fax delete-log`** - Deletes a specific fax log.

Prerequisites
* User must belong to a Business or Enterprise account.
* User must have a Zoom Phone license.

Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:delete:fax_log`,`phone:delete:fax_log:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli fax download-cover-preview-image`** - Downloads the preview image for a specific fax cover page.

Prerequisites
* User must belong to a Business or Enterprise account.
* User must have a Zoom Phone license.

Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:fax_cover_page`,`phone:read:fax_cover_page:admin`

Rate Limit Label: `RESOURCE-INTENSIVE`
- **`zoom-phone-cli fax downloadfaxfile`** - Downloads the fax file.

Prerequisites*
* User must belong to a Business or Enterprise account.
* User must have a Zoom Phone license.

Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:fax_log`,`phone:read:fax_log:admin`

Rate Limit Label: `RESOURCE-INTENSIVE`
- **`zoom-phone-cli fax get-account-s-logs`** - Returns the account's fax logs.

Prerequisites
* User must belong to a Business or Enterprise account.
* User must have a Zoom Phone license.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_fax_log:admin`

Rate Limit Label: `HEAVY`
- **`zoom-phone-cli fax get-log-details`** - Returns the fax log's details.

Prerequisites
* User must belong to a Business or Enterprise account.
* User must have a Zoom Phone license.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_fax_log:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli fax list-covers`** - Returns a list of fax cover pages for the account.

Prerequisites
* User must belong to a Business or Enterprise account.
* User must have a Zoom Phone license.

Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:fax_cover_page`,`phone:read:fax_cover_page:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli fax send-efax`** - Sends a fax to receivers. 

Prerequisites 
* User must belong to a Business or Enterprise account. 
* User must have a Zoom Phone license.

Note:
Faxes sent through the API will first use the account-level page pool, if available. 

If no page pool is available, pages meter individually. Unlimited faxing does not apply to faxes sent through the API.

Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:write:send_fax`,`phone:write:send_fax:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli fax update-log-read-status`** - Updates the read status of a specific fax log.

Prerequisites
* User must belong to a Business or Enterprise account.
* User must have a Zoom Phone license.

Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:fax_log:admin`,`phone:read:fax_log`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli fax upload-files`** - Uploads fax file

Note:
* Please use `https://fileapi.zoom.us` instead of `https://api.zoom.us`, the full path is `https://fileapi.zoom.us/v2/fax/files`
* Supported file formats: `.zip`
* The maximum size of file is 50MB

Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:write:send_fax`,`phone:write:send_fax:admin`

Rate Limit Label: `RESOURCE-INTENSIVE`

### firmware_update_rules

Manage firmware update rules

- **`zoom-phone-cli firmware-update-rules add-firmware-rule`** - Use this API to add a firmware update rule.

Prerequisites:
* Business, or Education account
* Zoom Phone license 



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:firmware_update_rule:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli firmware-update-rules delete`** - Use this API to delete the firmware update rule.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:firmware_update_rule:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli firmware-update-rules get-firmware-rule-detail`** - Use this API to get the firmware update rule information.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:firmware_update_rule:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli firmware-update-rules list-firmware-rules`** - Use this API to get firmware update rules.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_firmware_update_rules:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli firmware-update-rules update-firmware-rule`** - Use this API to update a specific firmware update rule.

Prerequisites:
* Business, or Education account
* Zoom Phone license 



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:firmware_update_rule:admin`

Rate Limit Label: `Light`

### firmwares

Manage firmwares

- **`zoom-phone-cli firmwares`** - Use this API to get updatable firmwares.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_firmwares:admin`

Rate Limit Label: `Light`

### group_call_pickup

Manage group call pickup

- **`zoom-phone-cli group-call-pickup add-gcp`** - Use this API to add a Group Call Pickup instance to the account that has the Zoom Phone license assigned. 

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:call_pickup_group:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli group-call-pickup add-gcpmembers`** - Use this API to add members to the specified Group Call Pickup object. 

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:call_pickup_group_member:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli group-call-pickup delete-gcp`** - Use this API to remove a Group Call Pickup object.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:call_pickup_group:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli group-call-pickup get-gcp`** - Use this API to retrieve information on a specific Group Call Pickup object in an account. 

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:call_pickup_group:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli group-call-pickup list-gcp`** - Use this API to retrieve a list of Group Call Pickup objects in an account.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_call_pickup_groups:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli group-call-pickup list-gcpmembers`** - Use this API to retrieve members of a call pickup group in an account.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:call_pickup_group_member:admin`

Rate Limit Label: `Medium`
- **`zoom-phone-cli group-call-pickup remove-gcpmembers`** - Use this API to remove member from the Group Call Pickup object. 

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:call_pickup_group_member:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli group-call-pickup update-gcp`** - Use this API to update a specific Group Call Pickup object information.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:call_pickup_group:admin`

Rate Limit Label: `Light`

### groups

Manage groups

- **`zoom-phone-cli groups get-phone-settings`** - Returns group phone settings.

Prerequisites
 Account must have a Pro or a higher plan with Zoom Phone license.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:group_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli groups get-policy-details`** - Returns the group policy details.

Prerequisites
Pro or higher account plan with Zoom phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:group_policy:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli groups update-policy`** - Updates the group policy.

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:group_policy:admin`

Rate Limit Label: `HEAVY`

### inbound_blocked

Manage inbound blocked

- **`zoom-phone-cli inbound-blocked add-account-level-inbound-block-rules`** - Adds an account level block rule for inbound calls and messaging from a phone number. As a result, all extensions block calls and messages to the phone number.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:inbound_block_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli inbound-blocked delete-account-level-inbound-block-rules`** - Deletes the account level blocked rule for inbound calls and messaging.


Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:inbound_block_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli inbound-blocked delete-account-level-statistics`** - Deletes the statistic of extensions blocked rule for inbound calls and messaging. (e.g. Call Queue, Auto Receptionist, Shared Line Group, Common Area, Zoom Room and User)

Prerequisites:
* Pro or higher account plan with Zoom phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:extension_inbound_block_rule_stat:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli inbound-blocked list-account-level-inbound-block-rules`** - Returns a list of account level inbound block rule for inbound calls and messaging.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_inbound_block_rules:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli inbound-blocked list-account-level-statistics`** - Returns the list of the statistics of the extensions blocked rule for inbound calls and messaging. (e.g. Call Queue, Auto Receptionist, Shared Line Group, Common Area, Zoom Room and User)

Prerequisites:
* Pro or higher account plan with Zoom phone license
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_extension_inbound_block_rules_stat:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli inbound-blocked mark-phone-number-as-blocked-for-all-extensions`** - Promotes and applies the phone number blocked from the extension level blocked rule to the account level blocked rule. This action is contingent on the statistics of extensions blocked rule. All extensions under the current account block this phone number.

Prerequisites:
* Pro or higher account plan with Zoom phone license
* Account owner or admin permissions


Scopes: `phone:write:admin`

Granular Scopes: `phone:update:inbound_blocked_for_all:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli inbound-blocked update-account-level-inbound-block-rule`** - Updates the account level block rule for inbound calls and messaging.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:inbound_block_rule:admin`

Rate Limit Label: `LIGHT`

### locations

Manage locations

- **`zoom-phone-cli locations add`** - Adds an emergency service location.

Prerequisites 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:emergency_location:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli locations delete`** - Removes an emergency location.

Prerequisites: 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:emergency_location:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli locations get`** - Returns an emergency service location's information.

Prerequisites: 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:emergency_location:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli locations list`** - Returns emergency service locations. 
Note: When you enable multiple sites, the `site_id` parameter is required.

Prerequisites: 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_emergency_locations:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli locations update`** - Updates an emergency location's information.

Prerequisites 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:emergency_location:admin`

Rate Limit Label: `LIGHT`

### metrics

Manage metrics

- **`zoom-phone-cli metrics get-call-log-details`** - Returns call log details of a specific call. 

The call logs provide a record of all incoming and outgoing calls over Zoom Phone in an account.

 Prerequisites:
 * Business, or Education account
 * Zoom Phone license 

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:call_log:admin`
- **`zoom-phone-cli metrics get-call-qo-s`** - Gets the call quality of service (QoS) data for a call made or received by a Zoom phone user in the account. 

Prerequisites:
* Business, or Education account
* Zoom Phone license   
   
 


Scopes: `phone:read:admin`

Granular Scopes: `phone:read:call_qos:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli metrics list-call-logs`** - Returns monthly call logs metrics.

The call logs that provide a record of all incoming and outgoing calls over Zoom Phone in an account. 

You can use query parameters to filter the response by date, site and MOS(Mean Opinion Score) of the call. 

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_call_logs:admin`

Rate Limit Label: `HEAVY`
- **`zoom-phone-cli metrics list-past-call`** - Returns all the call logs metrics of the account from the selected time period. The call logs provide a record of all incoming and outgoing calls over Zoom Phone in an account. You can use query parameters to filter the response by metrics of the call (such as date, phone number, extension number and quality type). 

Prerequisites:
* Business, or Education account
* Zoom Phone license 



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_call_logs:admin`

Rate Limit Label: `Heavy`
- **`zoom-phone-cli metrics list-phone-realtimelocation`** - Returns currently detected location of IP phones.

Prerequisites: 
* Pro or higher account plan with Zoom phone license 
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:realtime_location_devices:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli metrics list-tracked-locations`** - Lists the tracked locations. 

Prerequisites:
* Pro or higher account plan with Zoom phone license
* Account owner or admin permissions



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_tracked_locations:admin`

Rate Limit Label: `Medium`
- **`zoom-phone-cli metrics list-user-default-emergency-address`** - Returns the users who provide a default personal emergency address rather than a default site address or default account address.   

Prerequisites: 
* Pro or higher account plan with Zoom phone license 
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:default_emergency_address:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli metrics list-user-detectable-personal-location`** - Returns the users who created one or more detectable personal locations with associated network data. (Exclude users didn't enable Location Permission on client.) 

Prerequisites: 
* Pro or higher account plan with Zoom phone license 
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:detectable_personal_location:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli metrics list-user-location-sharing-permission`** - Returns users that have enabled location services and allowed location sharing with Zoom applications on client devices. 

(Excludes users who are unable to use Nomadic Emergency Services, or users on client versions lower than 5.4.0.) 

Prerequisites: 
* Pro or higher account plan with Zoom phone license 
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:location_sharing_permission:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli metrics list-user-nomadic-emergency-services`** - Returns users who have been enabled for nomadic emergency services.

Prerequisites: 
* Pro or higher account plan with Zoom phone license 
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:nomadic_emergency_services:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli metrics list-user-realtime-location`** - Returns the user's currently detected location. (Excludes users who didn't enable location permission on the client.)  

Prerequisites: 
* Pro or higher account plan with Zoom phone license 
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:realtime_location_users:admin`

Rate Limit Label: `MEDIUM`

### monitoring_groups

Manage monitoring groups

- **`zoom-phone-cli monitoring-groups add-members`** - Use this API to add members to a Monitoring Group.

Prerequisites:
 * A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:monitoring_group_member:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli monitoring-groups create`** - Creates a monitoring group.

Prerequisites:
 * A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:monitoring_group:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli monitoring-groups delete`** - Use this API to delete a Monitoring Group.

Prerequisites:
 * A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:monitoring_group:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli monitoring-groups get-by-id`** - Returns a monitoring group for the specified ID.

Prerequisites: 
* A Business or Enterprise account
 * A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:monitoring_group:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli monitoring-groups list`** - Returns an account's monitoring group list.

Prerequisites:
 * A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_monitoring_groups:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli monitoring-groups list-members`** - Use this API to return members list of a Monitoring Group.

Prerequisites:
 * A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_monitoring_group_members:admin`

Rate Limit Label: `Medium`
- **`zoom-phone-cli monitoring-groups remove-member`** - Use this API to remove a member from a Monitoring Group.

Prerequisites:
 * A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:monitoring_group_member:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli monitoring-groups remove-members`** - Use this API to remove all monitor or monitored members from a Monitoring Group.

Prerequisites:
 * A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:monitoring_group_member:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli monitoring-groups update`** - Use this API to update a Monitoring Group.

Prerequisites:
 * A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:monitoring_group:admin`

Rate Limit Label: `Light`

### numbers

Manage numbers

- **`zoom-phone-cli numbers delete-unassigned-phone`** - Deletes unassigned phone numbers. Up to 20 phone numbers can be removed in a single request.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license 
* The user must have been previously assigned a Zoom Phone number



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:number:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli numbers get-phone-details`** - Returns information about an account's Zoom Phone number.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:numbers:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli numbers list-account-phone`** - Returns a list all Zoom Phone numbers in a Zoom account.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_numbers:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli numbers update-phone-details`** - Updates a Zoom Phone number's information.

Prerequisites: 
* A Paid account

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli numbers update-site-for-unassigned-phone`** - Updates a site's unassigned phone numbers. Up to 20 phone numbers can be updated in a single request. 

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:site_number:admin`

Rate Limit Label: `Medium`

### outbound_caller_id

Manage outbound caller id

- **`zoom-phone-cli outbound-caller-id add-outbound-caller-numbers`** - Adds the `account-level` customized outbound caller ID phone numbers. Note that when multiple sites policy is enabled, users cannot manage the `account-level` configuration. The system will throw an exception.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license.



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:customized_number:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli outbound-caller-id delete-outbound-caller-numbers`** - Deletes the `account-level` customized outbound caller ID phone numbers. Note that when multiple sites policy is enabled, users cannot manage the `account-level` configuration. The system will throw an exception.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license.



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:customized_number:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli outbound-caller-id list-customize-outbound-caller-numbers`** - Retrieves phone numbers that can be used as the `account-level` customized outbound caller ID. Note that when multiple sites policy is enabled, users cannot manage the `account-level` configuration. The system will throw an exception. 

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license.



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_customized_number:admin`

Rate Limit Label: `Light`

### outbound_calling

Manage outbound calling

- **`zoom-phone-cli outbound-calling add-account-exception-rule`** - Adds the account level outbound calling policy exception rule for country region.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli outbound-calling delete-account-exception-rule`** - Deletes the account level outbound calling exception rule.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli outbound-calling get-account-countries-and-regions`** - Returns the account level outbound calling countries and regions.

Prerequisites:
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_outbound_calling_rules:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli outbound-calling list-account-exception-rule`** - Lists the account level outbound calling policy exception rules.

Prerequisite:
* Pro or higher account plan with Zoom phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_outbound_calling_rules:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli outbound-calling update-account-countries-or-regions`** - Updates the account level outbound calling policy country or region.

Prerequisite:
* Account must have a Pro or a higher plan with a Zoom Phone license.

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli outbound-calling update-account-exception-rule`** - Updates the account level outbound calling policy for the country region exception rule.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`

### peering

Manage peering

- **`zoom-phone-cli peering add-phone-numbers`** - Adds phone numbers to Zoom through the Provider Exchange.

Note: Phone peering API and events are for partners who have completed the MoU to peer with Zoom. To become a peering provider/ carrier, submit your request.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`,`phone_peering:write:admin`

Granular Scopes: `phone:write:peering_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli peering delete-phone-numbers`** - Removes phone numbers added to Zoom through the provider exchange.

Note: Phone peering API and events are for partners who have completed the MoU to peer with Zoom. To become a peering provider or carrier, submit your request.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`,`phone_peering:write:admin`

Granular Scopes: `phone:delete:peering_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli peering list-phone-numbers`** - Returns phone numbers to Zoom through the Provider Exchange.

Note: Phone peering API and events are for partners who have completed the MoU to peer with Zoom. To become a peering provider/ carrier, submit your request.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`,`phone_peering:read:admin`

Granular Scopes: `phone:read:list_peering_numbers:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli peering update-phone-numbers`** - Updates phone numbers to Zoom through the Provider Exchange.

Note: Phone peering API and events are for partners that have completed the MoU to peer with Zoom. To become a peering provider/ carrier, submit your request.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`,`phone_peering:write:admin`

Granular Scopes: `phone:update:peering_number:admin`

Rate Limit Label: `LIGHT`

### plans

Manage plans

- **`zoom-phone-cli plans`** - Returns all of an account's Zoom Phone plan package, phone number usage, and availability.

Prerequisites 
* A Pro or a higher account 
* A Zoom Phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_calling_plans:admin`

Rate Limit Label: `MEDIUM`

### policies

Manage policies

- **`zoom-phone-cli policies get-account-policy-details`** - Returns the account policy details.

Prerequisites
* Pro or higher account plan with Zoom phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:policy:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli policies update-account-policy`** - Updates the account's Zoom Phone policy.

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:policy:admin`

Rate Limit Label: `RESOURCE-INTENSIVE`

### ported_numbers

Manage ported numbers

- **`zoom-phone-cli ported-numbers get-details`** - Returns details on the ported numbers by specifying `order_id`.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom phone license

Scopes: `phone:read:admin`,`phone_peering:read:admin`

Granular Scopes: `phone:read:ported_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli ported-numbers list`** - Use this API to list ported numbers in a Zoom account.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license

Scopes: `phone:read:admin`,`phone_peering:read:admin`

Granular Scopes: `phone:read:list_ported_numbers:admin`

Rate Limit Label: `MEDIUM`

### private_directory

Manage private directory

- **`zoom-phone-cli private-directory add-members-to-aprivate-directory`** - Adds members to a private directory.

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:private_directory_member:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli private-directory list-members`** - Returns the member list of a private directory.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_private_directory_members:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli private-directory remove-amember-from-aprivate-directory`** - Removes a member from a private directory.

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:private_directory_member:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli private-directory update-aprivate-directory-member`** - Updates a member of a private directory.

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:private_directory_member:admin`

Rate Limit Label: `LIGHT`

### provision_templates

Manage provision templates

- **`zoom-phone-cli provision-templates add`** - Use this API to create a provision template in a Zoom account.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:provision_template:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli provision-templates delete`** - Use this API to delete a provision template in a Zoom account.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:provision_template:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli provision-templates get`** - Use this API to get a specific provision template.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:provision_template:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli provision-templates list-account`** - Use this API to list all provision templates in a Zoom account.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_provision_templates:admin`

Rate Limit Label: `Medium`
- **`zoom-phone-cli provision-templates update`** - Use this API to update a provision template in a Zoom account.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:provision_template:admin`

Rate Limit Label: `Light`

### push_to_talk

Manage push to talk

- **`zoom-phone-cli push-to-talk add-members-to-apush-to-talk-channel`** - Adds members to a push to talk (PTT) channel.

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:push_to_talk_channel_member:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli push-to-talk create-apush-to-talk-channel`** - Creates a push to talk (PTT) channel.

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:push_to_talk_channel:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli push-to-talk get-channel-details`** - Returns push to talk (PTT) channel details.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:push_to_talk_channel:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli push-to-talk list-channel-members`** - Returns a list of push to talk (PTT) members.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_push_to_talk_channel_members:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli push-to-talk list-channels`** - Returns a list of push to talk (PTT) channels.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_push_to_talk_channels:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli push-to-talk remove-amember-from-apush-to-talk-channel`** - Removes a member from a push to talk (PTT) channel.

Scopes: `phone:read:admin`

Granular Scopes: `phone:delete:push_to_talk_channel_member:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli push-to-talk remove-apush-to-talk-channel`** - Removes a push to talk (PTT) channel.

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:push_to_talk_channel:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli push-to-talk update-channel-details`** - Updates the push to talk (PTT) channel details.

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:push_to_talk_channel:admin`

Rate Limit Label: `LIGHT`

Not supported in Gov cluster

### recording_transcript

Manage recording transcript

- **`zoom-phone-cli recording-transcript <recordingId>`** - Downloads the phone recording transcript. 

Prerequisites

* A Business or Enterprise account
* A Zoom Phone license



Scopes: `phone:read`,`phone:read:admin`,`phone_recording:read`,`phone_recording:read:admin`

Granular Scopes: `phone:read:recording_transcript`,`phone:read:recording_transcript:admin`

Rate Limit Label: `LIGHT`

### recordings

Manage recordings

- **`zoom-phone-cli recordings delete-call`** - Deletes a call recording.

Prerequisites: 
* User must belong to a Business or Enterprise account 
* User must have a Zoom Phone license

Scopes: `phone:write:admin`,`phone:write`,`phone_recording:write`,`phone_recording:write:admin`

Granular Scopes: `phone:delete:call_recording`,`phone:delete:call_recording:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli recordings get-phone`** - Returns an account's call recordings. 

Prerequisites: 
* A Pro or higher account plan 
 * A Zoom Phone license 
 * Account owner or admin privileges

Scopes: `phone:read:admin`,`phone_recording:read:admin`

Granular Scopes: `phone:read:list_call_recordings:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli recordings phone-download-file`** - Downloads the phone recording. 

Prerequisites:

* A Business or Enterprise account
* A Zoom Phone license



Scopes: `phone:read`,`phone:read:admin`,`phone_recording:read`,`phone_recording:read:admin`

Granular Scopes: `phone:read:call_recording`,`phone:read:call_recording:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli recordings update-auto-delete-field`** - Updates the auto delete field for recording. It only updates if you enable the Auto Delete Data After Retention Duration setting in the account settings for recordings. 

Prerequisites: 
* User must belong to a Business or Enterprise account 
* User must have a Zoom Phone license.

Scopes: `phone:write:admin`,`phone:write`,`phone_recording:write`,`phone_recording:write:admin`

Granular Scopes: `phone:update:call_recording`,`phone:update:call_recording:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli recordings update-status`** - Updates the status of a single recording in account. 

Prerequisites: 
* User must belong to a Business or Enterprise account. 
* User must have a Zoom Phone license.

Scopes: `phone:write:admin`,`phone:write`,`phone_recording:write`,`phone_recording:write:admin`

Granular Scopes: `phone:update:call_recording`,`phone:update:call_recording:admin`

Rate Limit Label: `LIGHT`

### reports

Manage reports

- **`zoom-phone-cli reports get-call-charges-usage`** - Retrieves the Phone Call Charges Report. 

The Call charges usage report allows account owners and admins to view monthly Zoom phone call charges. Account owners and admins can also access this information when they log into their Zoom accounts and navigate to Call Charges Usage Report.

 Prerequisites: 

* Account must be enrollled in Pro or a higher plan
* Account must be enrolled in a Zoom Phone plan



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:call_charges:admin`

Rate Limit Label: `HEAVY`
- **`zoom-phone-cli reports get-psoperation-logs`** - Retrieves the phone system operation logs report.

The phone system operation logs report allows account owners and admins to view monthly Zoom phone related admin operation details. 

 Account owners and admins can also access this information by logging into their Zoom accounts and navigating to Phone System Operation Logs.  
   
  Prerequisites:   
 
* Account must be enrollled in Pro or a higher plan
* Account must be enrolled in a Zoom Phone plan



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:operation_logs:admin`

Rate Limit Label: `HEAVY`
- **`zoom-phone-cli reports get-smscharges-usage`** - Retrieves the SMS/MMS Charges Report.  The SMS/MMS charges usage report allows account owners and admins to view monthly Zoom phone SMS charges. Account owners and admins can also access this information by when they log into their Zoom accounts and navigate to  SMS/MMS Charges Usage Report.  Prerequisites  * Account must be enrolled in Pro or a higher plan * Account must be enrolled in a Zoom Phone plan

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:sms_charges:admin`

Rate Limit Label: `HEAVY`
- **`zoom-phone-cli reports getfaxchargesusagereport`** - Retrieves the Phone Fax Charges Report. The fax charges usage report allows account owners and admins to view monthly Zoom phone fax charges. 

Account owners and admins can also access this information when they log into their Zoom accounts and navigate to Fax Charges Usage Report. 

Prerequisites 
* Account must be enrollled in Pro or a higher plan
* Account must be enrolled in a Zoom Phone plan

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:fax_charges:admin`

Rate Limit Label: `HEAVY`

### roles

Manage roles

- **`zoom-phone-cli roles add-members`** - Use this API to add members to roles.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:role_member:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli roles add-phone-targets`** - Adds targets to phone roles.

Prerequisites:
* Business or Education account
* Zoom Phone license 

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:role:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli roles del-members`** - Use this API to delete member(s) in a role.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:role_member:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli roles delete-phone`** - Use this API to delete a phone role.

Prerequisites:
* Business, or Education account
* Zoom Phone license 



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:role:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli roles delete-phone-targets`** - Deletes the targets of a phone role.

Prerequisites:
* Business or Education account
* Zoom Phone license 

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:role:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli roles duplicate-phone`** - Use this API to duplicate a phone role.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:role:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli roles get-information`** - Use this API to get information on a phone role.

Prerequisites:
* Business, or Education account
* Zoom Phone license 



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:role:admin`

Rate Limit Label: `Medium`
- **`zoom-phone-cli roles list-members`** - Use this API to get members (not) in a role.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:role_member:admin`

Rate Limit Label: `Medium`
- **`zoom-phone-cli roles list-phone`** - Returns the phone roles.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_roles:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli roles list-phone-targets`** - Targets of a phone role. 

Prerequisites: 
* Business or Education account 
* Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_roles:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli roles update-phone`** - Use this API to update a role.

Prerequisites:
* Business, or Education account
* Zoom Phone license 



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:role:admin`

Rate Limit Label: `Light`

### rooms

Manage rooms

- **`zoom-phone-cli rooms add-zoom`** - Associates a Zoom Room with a Zoom Phone license. 

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:room:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli rooms assign-calling-plan-to`** - Assigns calling plans to a Zoom Room with a maximum of 200 numbers at a time.

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:room_calling_plan:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli rooms assign-phone-number-to-zoom`** - Assigns phone numbers to a Zoom Room. Up to 200 numbers at a time.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:room_phone_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli rooms get-zoom`** - Specifies Zoom Room  in an account with an assigned Zoom Phone license.

Prerequisites 
* A Pro or higher account plan 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:room:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli rooms list-unassigned-zoom`** - Returns Zoom Rooms that are not assigned a Zoom Phone.Prerequisites: * A Pro or higher account plan * A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_rooms:admin`

Rate Limit Label: `Medium`
- **`zoom-phone-cli rooms list-zoom`** - Retrieves a list of Zoom Rooms under the account that has the Zoom Phone license assigned.Prerequisites: * A Pro or higher account plan * A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_rooms:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli rooms remove-zoom`** - Use this API to remove Zoom Room from a Zoom Phone account.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:room:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli rooms unassign-calling-plan-from`** - Use this API to unassign a calling plan from a Zoom Room.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:room_calling_plan:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli rooms unassign-phone-number-from-zoom`** - Use this API to unassign a phone number from a Zoom Room.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license 
* The Zoom Room must have been previously assigned a Zoom Phone number

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:room_phone_number:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli rooms update-zoom`** - Use this API to update a Zoom Room in an account that has the Zoom Phone license assigned.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:room:admin`

Rate Limit Label: `Light`

### routing_rules

Manage routing rules

- **`zoom-phone-cli routing-rules add`** - Creates a directory backup routing rule. 

The directory backup routing rules are a series of predefined regular expressions. These rules are used to route outgoing calls. If a dialed number does not match a Zoom Phone user, and does not match a defined external contact, these rules are tested next. If a dialed number does not match any rules, the call will be routed through the PSTN.

Prerequisites:
 * A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:routing_rule:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli routing-rules delete`** - Deletes the directory backup routing rule. 

The directory backup routing rules are a series of predefined Regular Expressions. These rules are used to route outgoing calls. If a dialed number does not match a Zoom Phone user, and does not match a defined External Contact, these rules are tested next. If a dialed number does not match any rules, the call will be routed via the PSTN.

Prerequisites:
 * A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:routing_rule:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli routing-rules get`** - Returns the directory backup routing rule. The directory backup routing rules are a series of predefined Regular Expressions. These rules are used to route outgoing calls. If a dialed number does not match a Zoom Phone user, and does not match a defined External Contact, these rules are tested next. If a dialed number does not match any rules, the call will be routed via the PSTN.Prerequisites: * A Business or Enterprise account * A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:routing_rule:admin`

Rate Limit Label: `Medium`
- **`zoom-phone-cli routing-rules list`** - Returns a list of directory backup routing rules. The directory backup routing rules are a series of predefined Regular Expressions. These rules are used to route outgoing calls. If a dialed number does not match a Zoom Phone user, and does not match a defined External Contact, these rules are tested next. If a dialed number does not match any rules, the call will be routed via the PSTN. Prerequisites: * A Business or Enterprise account * A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_routing_rules:admin`

Rate Limit Label: `Medium`
- **`zoom-phone-cli routing-rules update`** - Updates the directory backup routing rule. 

The directory backup routing rules are a series of predefined regular expressions. These rules are used to route outgoing calls. If a dialed number does not match a Zoom Phone user, and does not match a defined external contact, these rules are tested next. If a dialed number does not match any rules, the call will be routed through the PSTN.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:routing_rule:admin`

Rate Limit Label: `Light`

### setting_templates

Manage setting templates

- **`zoom-phone-cli setting-templates add`** - Creates a Zoom Phone setting template for an account. After creating a phone template, the defined settings will become the default settings for an account.

Prerequisites: 
* A Business or enterprise Zoom account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:setting_template:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli setting-templates get`** - Returns information about an account's phone template.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:setting_template:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli setting-templates list`** - Gets a list of all the created phone template settings.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_setting_templates:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli setting-templates update`** - Updates or modifies a phone template's settings.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:setting_template:admin`

Rate Limit Label: `LIGHT`

### settings

Manage settings

- **`zoom-phone-cli settings phone`** - Returns an account's settings.

Prerequisites:
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:settings:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli settings update-phone`** - Updates Zoom Phone account settings.

Prerequisites: 
* A Business or Enterprise account

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:settings:admin`

### shared_line_appearances

Manage shared line appearances

- **`zoom-phone-cli shared-line-appearances`** - Returns a list of shared line appearance instances.

Prerequisites 
* Pro or higher account with Zoom Phone license 
* Account owner or admin privileges  



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_shared_line_appearances:admin`

Rate Limit Label: `MEDIUM`

### shared_line_groups

Manage shared line groups

- **`zoom-phone-cli shared-line-groups add-members-to`** - Adds members to a shared line group. A shared line group allows Zoom Phone admins to share a phone number and extension with a group of phone users or common areas. This gives members of the shared line group access to the group's direct phone number and voicemail.  Note that a member can only be added to one shared line group. 

Prerequisites:   
 
* Pro or higher account with Zoom Phone license.
* A valid Shared Line Group
* Account owner or admin privileges 



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:shared_line_member:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups add-setting`** - Adds the shared line group setting according to the setting type.

Prerequisites
* A Pro or higher account with a Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:shared_line_group_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups add-slgpolicy-sub-setting`** - Adds the policy sub-setting for a specific shared line group according to the `policyType`. For example, you can use this API to set up shared access members. Prerequisites: * Pro or higher account with Zoom Phone license.* Account owner or admin privileges 

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:shared_line_group_policy:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli shared-line-groups assign-phone-numbers-slg`** - Assigns phone numbers to a shared line groups. These direct phone numbers will be shared among members of the shared line group.

Prerequisites:   
 * Pro or higher account with Zoom Phone license.
 * A valid Shared Line Group
 * Account owner or admin privileges

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:shared_line_group_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups create-ashared-line-group`** - Creates a shared line group. A shared line group allows Zoom Phone admins to share a phone number and extension with a group of phone users or common areas. This gives members of the shared line group access to the group's direct phone number and voicemail. Prerequisites:    * Pro or higher account with Zoom Phone license.* Account owner or admin privileges 

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:shared_line_group:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups delete-amember-slg`** - Unassigns a specific member from a shared line group. Members of the shared line group have access to the group's phone number and voicemail. 

Prerequisites:   
 
* Pro or higher account with Zoom Phone license.
* A valid Shared Line Group
* Account owner or admin privileges



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:shared_line_member:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups delete-aphone-number-slg`** - Unassigns a specific phone number that was assigned to the shared line group.

Prerequisites:   
 
* Pro or higher account with Zoom Phone license.
* A valid shared line group
* Account owner or admin privileges



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:shared_line_group_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups delete-ashared-line-group`** - Deletes a shared line group. A shared line group allows Zoom Phone admins to share a phone number and extension with a group of phone users or common areas. 

Prerequisites:   
 * Pro or higher account with Zoom Phone license.
 * Account owner or admin privileges 

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:shared_line_group:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups delete-members-of-slg`** - Unassigns all existing members from a shared line group. Members of the shared line group have access to the group's phone number and voicemail. 

Prerequisites: 

* Pro or higher account with Zoom Phone license.
* A valid Shared Line Group
* Account owner or admin privileges



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:shared_line_member:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups delete-phone-numbers-slg`** - Unassigns all the phone numbers that have been assigned to the shared line group.

Prerequisites: 

* Pro or higher account with Zoom Phone license.
* A valid Shared Line Group
* Account owner or admin privileges



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:shared_line_group_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups delete-setting`** - Deletes the shared line group setting according to the setting type.

Prerequisites
* A Pro or higher account with a Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:shared_line_group_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups get-ashared-line-group`** - Returns a list of all the shared line groups. 
 
 A shared line group allows Zoom Phone admins to share a phone number and extension with a group of phone users or common areas. This gives members of the shared line group access to the group's direct phone number and voicemail. 

Prerequisites:   
 
* Pro or higher account with Zoom Phone license.
* Account owner or admin privileges 



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:shared_line_group:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups get-call-handling-setting`** - Returns the call handling setting of a shared line group for the specified hour type.

Prerequisites:
* A Pro or higher account with a Zoom Phone license.
* Account owner or admin permissions.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:shared_line_group_call_handling_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups get-policy`** - Returns the policy setting of a specific shared line group.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:shared_line_group_policy:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups get-settings`** - Returns shared line group settings.

Prerequisites:
* A Pro or higher account with a Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:shared_line_group_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups list`** - Lists all the shared line groups. A shared line group allows Zoom Phone admins to share a phone number and extension with a group of phone users or common areas. This capability gives members of the shared line group access to the group's direct phone number and voicemail. 

Prerequisites:   
 
* Pro or higher account with Zoom Phone license.
* Account owner or admin privileges    
 



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_shared_line_groups:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli shared-line-groups remove-slgpolicy-sub-setting`** - Removes the policy sub-setting for a specific shared line group according to the `policyType`. For example, you can use this API to remove shared access members. Prerequisites: * Pro or higher account with Zoom Phone license.* Account owner or admin privileges 

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:shared_line_group_policy:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups update-ashared-line-group`** - Updates the information about a shared line group.  
 
 A shared line group allows Zoom Phone admins to share a phone number and extension with a group of phone users or common areas. Members of the shared line group can access to the group's direct phone number and voicemail. 

Prerequisites   
 
* Pro or higher account with Zoom Phone license
* Account owner or admin privileges 



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:shared_line_group:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups update-call-handling-setting`** - Update the call handling setting of a shared line group for the specified hour type.

At least one of `call_distribution_setting` or `call_handling_setting` must be provided in the request body. `call_distribution_setting` is only allowed when `hour_type` is `business_hour`.

Prerequisites:
* A Pro or higher account with a Zoom Phone license.
* Account owner or admin permissions.

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:shared_line_group_call_handling_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups update-policy`** - Updates the policy setting of a specific shared line group. 

Prerequisites:
* Pro or higher account plan with Zoom Phone License
* Account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:shared_line_group_policy:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups update-setting`** - Update the shared line group setting according to the setting type.

Prerequisites
* A Pro or higher account with a Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:shared_line_group_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli shared-line-groups update-slgpolicy-sub-setting`** - Updates the policy sub-setting for a specific shared line group according to the `policyType`. For example, you can use this API to update shared access members. 

Prerequisites: 
* Pro or higher account with Zoom Phone license.
* Account owner or admin privileges 



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:shared_line_group_policy:admin`

Rate Limit Label: `LIGHT`

### sip_groups

Manage sip groups

- **`zoom-phone-cli sip-groups`** - Returns a list of SIP (Session Initiation Protocol) groups.

Prerequisites: 
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_sip_groups:admin`

Rate Limit Label: `LIGHT`

### sip_trunk

Manage sip trunk

- **`zoom-phone-cli sip-trunk`** - Returns a list of an account's assigned BYOC (Bring Your Own Carrier) SIP (Session Initiation Protocol) trunks.

Prerequisites 
* A Business or Enterprise account

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_sip_trunks:admin`

Rate Limit Label: `LIGHT`

### sites

Manage sites

- **`zoom-phone-cli sites add-emergency-routing-rule`** - Add a site's emergency routing rule. You can define additional emergency service numbers, special service numbers, and call handling settings associated with it.

Prerequisites:
* A Pro or higher account with a Zoom Phone license.
* Account owner or admin permissions.
* This is a site-level API. It can only be used when Multiple Sites is enabled for the account; if Multiple Sites is disabled, this API returns a 400 error ("Multiple Sites option has been disabled. Try again after enabling it."). Use the corresponding account-level emergency routing rule API instead when Multiple Sites is disabled.

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:site_emergency_service_routing_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sites add-outbound-caller-numbers`** - Use this API to add the `site-level` customized outbound caller ID phone numbers. 
* Multiple sites must be enabled. 

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license.



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:site_customized_number:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli sites add-outbound-calling-exception-rule`** - Adds the site level outbound calling policy exception rule for the country region.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:site_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sites add-setting`** - Sites allow you to organize Zoom Phone users in your organization. Use this API to add a site setting to a specific site according to the setting type.

Prerequisites: 
* Account must have a Pro or a higher plan with Zoom Phone license.
* Multiple sites must be enabled.



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:site_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sites add-users-to-directory-by`** - Use this API to add users to a directory of the specified site.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:directory:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli sites create-phone`** - Creates a site that allows you to organize the Zoom Phone users in your organization.

Prerequisites:
* Multiple sites must be enabled. 
* Pro or a higher account with Zoom Phone enabled.



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:site:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sites delete-emergency-routing-rule`** - Removes a site's emergency routing rule.

Prerequisites 
* Pro or a higher account with Zoom Phone license 
* Account owner or admin permissions 
* This is a site-level API. It can only be used when Multiple Sites is enabled for the account; if Multiple Sites is disabled, this API returns a 400 error ("Multiple Sites option has been disabled. Try again after enabling it."). Use the corresponding account-level emergency routing rule API instead when Multiple Sites is disabled.

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:site_emergency_service_routing_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sites delete-outbound-caller-numbers`** - Use this API to remove the `site-level` customized outbound caller ID phone numbers. 
* Multiple sites must be enabled. 

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license.



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:site_customized_number:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli sites delete-outbound-calling-exception-rule`** - Deletes the site level outbound calling exception rule.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:site_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sites delete-phone`** - Use this API to delete a specific site in a Zoom account. To delete a site, in the query parameter, you must provide the site ID of another site where the assets of current site (users, numbers and phones) can be transferred to.  You cannot use this API to delete the main site.

Prerequisites: 

* Account must have a Pro or a higher plan with Zoom Phone license. 
* Multiple sites must be enabled.



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:site:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli sites delete-setting`** - Sites allow you to organize Zoom Phone users in your organization. Use this API to delete the site setting of a specific site.

Prerequisites: 
* Account must have a Pro or a higher plan with Zoom Phone license.
* Multiple sites must be enabled.



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:site_setting:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli sites delete-users-from-directory-by`** - Use this API to delete users from a directory of the specified site.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:directory:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli sites get-asite`** - Returns information on a specific site. 

Sites allow you to organize Zoom Phone users in your organization. 

Prerequisites 
* Account must have a Pro or a higher plan with Zoom Phone license.
* Multiple sites must be enabled.



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:site:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sites get-outbound-calling-countries-and-regions`** - Returns the site level outbound calling countries and regions.

Prerequisites:
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:site_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sites get-setting-for-type`** - Gets the site setting about a specific site according to the setting type. Sites allow you to organize Zoom Phone users in your organization.

Prerequisites: 

* Account must have a Pro or a higher plan with Zoom Phone license.
* Multiple sites must be enabled.



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:site_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sites list-customize-outbound-caller-numbers`** - Use this API to retrieve phone numbers that can be used as the `site-level` customized outbound caller ID. 
* Multiple sites must be enabled. 

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license.



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_site_customized_number:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli sites list-emergency-number-pool-numbers`** - Returns phone numbers in the site-level Emergency Number Pool.

This endpoint can only be used for accounts where Multiple Sites is enabled. For accounts with Multiple Sites disabled, use List account emergency number pool phone numbers instead. 

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_sites_emergency_numbers:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli sites list-emergency-routing-rules`** - Lists all of a site's emergency routing rules (both Zoom default and customized) in a single response.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions
* This is a site-level API. It can only be used when Multiple Sites is enabled for the account; if Multiple Sites is disabled, this API returns a 400 error ("Multiple Sites option has been disabled. Try again after enabling it."). Use the corresponding account-level emergency routing rule list API instead when Multiple Sites is disabled.

Note: The response does not include a routing rule ID. Use the returned `number_pattern` value as the `routingRuleIdentifier` when calling the update or delete emergency routing rule APIs.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_site_emergency_service_routing_rules:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli sites list-outbound-calling-exception-rule`** - Returns a list of site level outbound calling policy exception rules.

Prerequisite:
* Pro or higher account plan with Zoom phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:site_outbound_calling_rule:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli sites list-phone`** - Sites allow you to organize Zoom Phone users in your organization. Use this API to list all the sites that have been created for an account.  
 
Prerequisites:  
 
* Multiple Sites must be enabled.
* Pro or a higher account with Zoom Phone enabled.




Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_sites:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli sites list-users-from-directory-by`** - Use this API to get users that are in or not in a directory of the specified site.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:directory:admin`

Rate Limit Label: `Medium`
- **`zoom-phone-cli sites update-details`** - Updates information about a specific site. 

It allows you to organize Zoom Phone users in your organization. 


Prerequisites 
An account must have a Pro or a higher plan with Zoom Phone license.


Scopes: `phone:write:admin`

Granular Scopes: `phone:update:site:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sites update-emergency-routing-rule`** - Updates a site's emergency routing rule.

Prerequisites
* Pro or higher account with Zoom Phone license.
* Account owner or admin permissions.
* This is a site-level API. It can only be used when Multiple Sites is enabled for the account; if Multiple Sites is disabled, this API returns a 400 error ("Multiple Sites option has been disabled. Try again after enabling it."). Use the corresponding account-level emergency routing rule API instead when Multiple Sites is disabled.

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:site_emergency_service_routing_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sites update-outbound-calling-countries-or-regions`** - Updates the site level outbound calling policy country or region.

Prerequisites:
* Account must have a Pro or a higher plan with a Zoom Phone license.

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:site_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sites update-outbound-calling-exception-rule`** - Updates the site level outbound calling policy for the country region exception rule.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:site_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sites update-setting`** - Allows you to organize Zoom Phone users in your organization. You can update the site setting of a specific site according to the setting type.

Prerequisites 
* Account must have a Pro or a higher plan with Zoom Phone license.
* Multiple sites must be enabled.



Scopes: `phone:write:admin`

Granular Scopes: `phone:update:site_setting:admin`

Rate Limit Label: `LIGHT`

### smartphones

Manage smartphones

- **`zoom-phone-cli smartphones`** - Returns a list of all the smartphones configured with Zoom Phone on an account.

Prerequisites  
 
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_devices:admin`

Rate Limit Label: `LIGHT`

Not supported in Gov cluster

### sms

Manage sms

- **`zoom-phone-cli sms account-session`** - Returns details about SMS sessions for an account.

---

Service Endpoint SMS
* This API now supports retrieving Service Endpoint SMS sessions.
* Non-Zoom Phone users can access only Service Endpoint SMS sessions. An account-level app is required for non-Zoom Phone users to access Service Endpoint SMS.

---

Prerequisites
* Paid account
* Zoom Phone enabled for the account
* For Zoom Phone SMS: a Zoom Phone-enabled user
* For Service Endpoint SMS: an account-level app and a Service Endpoint phone number enabled for SMS

Scopes: `phone:read:admin`,`phone_sms:read`,`phone_sms:read:admin`

Granular Scopes: `phone:read:list_sms_sessions`,`phone:read:list_sms_sessions:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli sms by-message-id`** - Returns details about a specific message in an SMS session.

Service Endpoint SMS
* This API supports retrieving a specific message from a Service Endpoint SMS session.
* The `sessionId` must belong to a Service Endpoint SMS session, and the `messageId` must belong to that session.
* Non-Zoom Phone users can only access messages in Service Endpoint SMS sessions. An account-level app is required for non-Zoom Phone users to access Service Endpoint SMS.

Prerequisites
* A paid account
* Zoom Phone enabled for the account
* For Zoom Phone SMS: a Zoom Phone-enabled user
* For Service Endpoint SMS: an account-level app and a Service Endpoint phone number enabled for SMS




Scopes: `phone:read:admin`,`phone:read`,`phone_sms:read`,`phone_sms:read:admin`

Granular Scopes: `phone:read:sms_message`,`phone:read:sms_message:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli sms post-message`** - Sends SMS messages to Zoom Phone accounts. 

Service Endpoint SMS
* This API now supports sending a Service Endpoint SMS or MMS message.
* The sender phone number must be assigned to a Service Endpoint and enabled for SMS.
* Non-Zoom Phone users can send only Service Endpoint SMS or MMS messages. An account-level app is required for non-Zoom Phone users to send Service Endpoint SMS.

Prerequisites
* A Zoom Phone license
* SMS enabled for the account or admin privileges
* For Zoom Phone SMS: a Zoom Phone-enabled user
* For Service Endpoint SMS: an account-level app, SMS enabled for the account, and a Service Endpoint phone number enabled for SMS

SMS and attachments - Size and limits
* SMS size with no media attachments: 2048 bytes
* SMS size with attachments: 1000 bytes
* Total attachments: 10
* Total attachment file size: 2 MB

Scopes: `phone:write:admin`,`phone:write`,`phone_sms:write`,`phone_sms:write:admin`

Granular Scopes: `phone:read:sms_message`,`phone:read:sms_message:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sms session-details`** - Returns details about an SMS session.

Service Endpoint SMS
* This API now supports retrieving a Service Endpoint SMS session.
* The `sessionId` must belong to a Service Endpoint SMS session.
* Non-Zoom Phone users can access only Service Endpoint SMS sessions. An account-level app is required for non-Zoom Phone users to access Service Endpoint SMS.

Prerequisites
* A paid account
* Zoom Phone enabled for the account
* For Zoom Phone SMS: a Zoom Phone-enabled user
* For Service Endpoint SMS: an account-level app and a Service Endpoint phone number enabled for SMS




Scopes: `phone:read:admin`,`phone:read`,`phone_sms:read`,`phone_sms:read:admin`

Granular Scopes: `phone:read:sms_session`,`phone:read:sms_session:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli sms session-sync`** - Syncs SMS messages in a session. 

Service Endpoint SMS
* This API now supports syncing a Service Endpoint SMS session.
* The `sessionId` must belong to a Service Endpoint SMS session.
* Non-Zoom Phone users can sync only Service Endpoint SMS sessions. An account-level app is required for non-Zoom Phone users to access Service Endpoint SMS.

Prerequisites
* Paid account
* Zoom Phone enabled for the account
* For Zoom Phone SMS: a Zoom Phone-enabled user
* For Service Endpoint SMS: an account-level app and a Service Endpoint phone number enabled for SMS

Scopes: `phone:read:admin`,`phone:read`,`phone_sms:read`,`phone_sms:read:admin`

Granular Scopes: `phone:read:sms_session`,`phone:read:sms_session:admin`

Rate Limit Label: `MEDIUM`

### sms_campaigns

Manage sms campaigns

- **`zoom-phone-cli sms-campaigns assign-campaign-phone-numbers`** - Assigns a phone number to the SMS campaign.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:sms_campaign_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sms-campaigns get`** - Returns a specific SMS campaign.

Prerequisites: 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:sms_campaign:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sms-campaigns get-number-campaign-opt-status`** - Returns the opt statuses of phone numbers that are assigned to SMS the campaign.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:sms_campaign_number_opt_status:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sms-campaigns list-account`** - Returns a list of all SMS campaigns in a Zoom account.

Prerequisites 
* A Pro or higher account plan 
* A Zoom Phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_sms_campaigns:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli sms-campaigns unassign-campaign-phone-number`** - Unassigns a phone number from the SMS campaign.
Remove the assigned phone number from the campaign.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license 
* The campaign must have been previously assigned a Zoom Phone number



Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:sms_campaign_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli sms-campaigns update-number-campaign-opt-status`** - Updates opt statuses of phone numbers that are assigned to the SMS campaign.

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:sms_campaign_number_opt_status:admin`

Rate Limit Label: `LIGHT`

### sms_consent

Manage sms consent

- **`zoom-phone-cli sms-consent <consentId>`** - Returns the opt statuses of phone numbers that are assigned to SMS consent.

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:sms_consent_number_opt_status:admin`

Rate Limit Label: `LIGHT`

### users

Manage users

- **`zoom-phone-cli users add-an-audio`** - Adds an audio item for text-to-speech conversion. Only the admin or user can add your audio and directly pass the `me` value instead of the `userId` parameter.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:write:audio`,`phone:write:audio:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users add-audio-item`** - Adds the audio items. You can only upload voice files at this time. Only the admin or user can add your audio and directly pass the `me` value instead of the `userId` parameter.

Prerequisites:
* Business or Education account
* Zoom Phone license 

Size and quantity limits for audio attachments:
* Up to 5 attachments
* Each file size should be no more than 1MB

Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:write:batch_audios`,`phone:write:batch_audios:admin`

Rate Limit Label: `HEAVY`
- **`zoom-phone-cli users add-call-forward-setting`** - Adds Zoom Phone call forwarding numbers for a user under a specific hour type. Call forwarding settings let users route their inbound calls to one or more external phone numbers or external contacts during business, closed, holiday, or break hours.

Prerequisites:
* A Business or Enterprise account
* A Zoom Phone license

Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:write:user_call_handling_setting`,`phone:write:user_call_handling_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users add-outbound-caller-numbers`** - Adds users' customized outbound caller ID phone numbers. 

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license.



Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:write:user_customized_number`,`phone:write:user_customized_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users add-outbound-calling-exception-rule`** - Adds an user level outbound calling policy exception rule for the country region.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:write:user_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users add-profile-setting`** - Adds the user setting according to the setting type.

Prerequisites
* Pro or a higher account with Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:write:user_setting:admin`,`phone:write:user_setting`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users add-setting`** - Adds the user setting according to the setting type, specifically for delegation, intercom and shared access for voicemail, and call recordings. For user-level apps, pass the `me` value instead of the `userId` parameter.

To see the shared access settings in the Zoom web portal, go to Admin &gt; Phone System Management &gt; Users &amp; Rooms . Choose Users and select User Policy. Go to Voicemail, Automatic Call Recording and Ad Hoc Call Recording.

 To view the delegation and intercom setting in your Zoom web portal, navigate to Admin &gt; Phone System Management &gt; Users &amp; Rooms. Choose the Users tab and select User Settings

 Prerequisites: 
* A Business or Enterprise account

Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:write:shared_setting`,`phone:write:shared_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users assign-calling-plan`** - Assigns a calling plan to a Zoom Phone user. For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:write:calling_plan`,`phone:write:calling_plan:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users assign-phone-number`** - Assigns a phone number to a user who has already enabled Zoom Phone.Prerequisites: * A Business or Enterprise account * A Zoom Phone license

Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:write:user_number`,`phone:write:user_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users batch-add`** - Adds phone users in batch. You can add up to 10 users at a time.

Prerequisites
* The users must be active in your Zoom account. 
* Pro or higher account plan with Zoom phone license
* Account owner or admin permissions



Scopes: `phone:write:admin`

Granular Scopes: `phone:write:batch_users:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli users delete-call-forward-setting`** - Delete Zoom Phone call forwarding numbers for a user under a specific hour type. Call forwarding settings let users route their inbound calls to one or more external phone numbers or external contacts during business, closed, holiday, or break hours.

Prerequisites:
* A Business or Enterprise account
* A Zoom Phone license


Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:delete:user_call_handling_setting`,`phone:delete:user_call_handling_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users delete-call-history`** - Deletes a user's call history. 

For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites 

* Belong to a Business or Enterprise account 
* Have a Zoom Phone license

Scopes: `phone:write`,`phone:write:admin`,`phone_call_log:write`,`phone_call_log:write:admin`

Granular Scopes: `phone:delete:call_log`,`phone:delete:call_log:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users delete-call-log`** - Deletes a user's call log. For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites: 
* User must belong to a Business or Enterprise account 
* User must have a Zoom Phone license

Scopes: `phone:write`,`phone:write:admin`,`phone_call_log:write`,`phone_call_log:write:admin`

Granular Scopes: `phone:delete:call_log`,`phone:delete:call_log:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users delete-outbound-caller-numbers`** - Removes the users' customized outbound caller ID phone numbers. 

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license.



Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:delete:user_customized_number`,`phone:delete:user_customized_number:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli users delete-outbound-calling-exception-rule`** - Deletes the user level outbound calling exception rule.

Prerequisites:
* Pro or a higher account with Zoom Phone license
* Account owner or admin permissions

Scopes: `phone:write:admin`

Granular Scopes: `phone:delete:user_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users delete-profile-setting`** - Deletes a user setting based on the setting type.

Prerequisites
* Pro or higher account with Zoom Phone license.
* Account owner or admin permissions.



Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:delete:user_setting`,`phone:delete:user_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users delete-setting`** - Removes the user setting according to the setting type, specifically for delegation, intercom and shared access for voicemail and call recordings. For user-level apps, pass the `me` value instead of the `userId` parameter.

To see the shared access settings in the Zoom web portal, go to Admin &gt; Phone System Management &gt; Users &amp; Rooms . Click Users and select User Policy. Go to Voicemail, Automatic Call Recording and Ad Hoc Call Recording.

 To view the delegation and intercom setting in your Zoom web portal, navigate to Admin &gt; Phone System Management &gt; Users &amp; Rooms. Click the Users tab and select User Settings

 Prerequisites: 
* A Business or Enterprise account

Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:delete:shared_setting`,`phone:delete:shared_setting:admin`

Rate Limit Label: `Light`
- **`zoom-phone-cli users get-aicall-summary`** - Returns AI call summary details. For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`,`phone:read`

Granular Scopes: `phone:read:ai_call_summary`,`phone:read:ai_call_summary:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli users get-call-handling-setting`** - Returns the call handling setting of a user for the specified hour type.

Prerequisites:
* A Business or Enterprise account
* A Zoom Phone license

Scopes: `phone:read:admin`,`phone:read`

Granular Scopes: `phone:read:user_call_handling_setting:admin`,`phone:read:user_call_handling_setting`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users get-number-campaign-opt-status`** - Returns the opt statuses of a user’s phone numbers. For user-level apps, pass the <a href="https://developers.zoom.us/docs/api/using-zoom-apis/#the-me-keyword" target="_blank">me</a> value instead of the userId parameter.

Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:sms_campaign_number_opt_status`,`phone:read:sms_campaign_number_opt_status:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users get-outbound-calling-countries-and-regions`** - Returns the user level outbound calling countries and regions.

Prerequisites:
* A Business or Enterprise account
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:user_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users get-policy-details`** - Returns the user policy details.

Prerequisites
* Pro or higher account plan with Zoom phone license



Scopes: `phone:read:admin`

Granular Scopes: `phone:read:user_policy:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli users get-sms-sessions`** - Retrieves the user's SMS sessions in descending order. Mirrors the ZP client behavior with the most recent on top.

Prerequisites:
* Paid account 
* User-enabled Zoom phone   
   
 



Scopes: `phone:read:admin`,`phone:read`,`phone_sms:read`,`phone_sms:read:admin`

Granular Scopes: `phone:read:sms_session`,`phone:read:sms_session:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli users get-voicemail-details-by-call-id-or-call-log-id`** - Returns the detailed information on a voicemail associated with a call log ID. 

For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites 
* User must belong to a Business or Enterprise account 
* User must have a Zoom Phone license

Scopes: `phone:read`,`phone:read:admin`,`phone_voicemail:read`,`phone_voicemail:read:admin`

Granular Scopes: `phone:read:voicemail`,`phone:read:voicemail:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users list-audio-items`** - Returns personal audios. 

Only the admin or user can query your audios and directly pass the `me` value instead of the `userId` parameter.

Prerequisites:
* Business or Education account
* Zoom Phone license 



Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:list_audios`,`phone:read:list_audios:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users list-customize-outbound-caller-numbers`** - Retrieves phone numbers that can be the `user-level` customized outbound caller ID. 

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license.



Scopes: `phone:read:admin`,`phone:read`

Granular Scopes: `phone:read:list_user_customized_number`,`phone:read:list_user_customized_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users list-outbound-calling-exception-rule`** - Returns a list of the user level outbound calling policy exception rules.

Prerequisite:
* Pro or higher account plan with Zoom phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:user_outbound_calling_rule:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli users list-phone`** - Returns a list of all of an account's users who are assigned a Zoom Phone license.

Prerequisites 
* A Pro or higher account plan 
* A Zoom Phone license

Scopes: `phone:read:admin`

Granular Scopes: `phone:read:list_users:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli users phone`** - Returns a user's Zoom phone profile. For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites
 * A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:user`,`phone:read:user:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users phone-call-history`** - Returns a user's Zoom phone call history. 

For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`,`phone_call_log:read`

Granular Scopes: `phone:read:list_call_logs:admin`,`phone:read:list_call_logs`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli users phone-call-logs`** - Returns a user's Zoom phone call logs. For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read`,`phone:read:admin`,`phone_call_log:read`,`phone_call_log:read:admin`

Granular Scopes: `phone:read:list_call_logs`,`phone:read:list_call_logs:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli users phone-recordings`** - Gets a user's Zoom Phone recordings. For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read`,`phone:read:admin`,`phone_recording:read`,`phone_recording:read:admin`

Granular Scopes: `phone:read:list_recordings`,`phone:read:list_recordings:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli users phone-settings`** - Returns the Zoom Phone profile settings of a user. For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:user_setting:admin`,`phone:read:user_setting`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users phone-voice-mails`** - Returns a user's Zoom Phone voicemails. For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read`,`phone:read:admin`,`phone_voicemail:read`,`phone_voicemail:read:admin`

Granular Scopes: `phone:read:list_voicemails`,`phone:read:list_voicemails:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli users sms-session`** - Returns details about SMS sessions for a user. 

For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites: 
* Paid account 
* User-enabled Zoom phone

Scopes: `phone:read:admin`,`phone:read`,`phone_sms:read`,`phone_sms:read:admin`

Granular Scopes: `phone:read:list_sms_sessions`,`phone:read:list_sms_sessions:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli users sync-call-history`** - Syncs a user's Zoom phone call history. 

For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites 
* A Business or Enterprise account
 * A Zoom Phone license

Scopes: `phone:read:admin`,`phone:read`

Granular Scopes: `phone:read:list_call_logs`,`phone:read:list_call_logs:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli users sync-call-logs`** - Syncs a user's Zoom phone call logs. For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`,`phone:read`

Granular Scopes: `phone:read:list_call_logs`,`phone:read:list_call_logs:admin`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli users unassign-calling-plan`** - Unassigns a Zoom Phone user's calling plan. For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:delete:users_calling_plan`,`phone:delete:users_calling_plan:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users unassign-phone-number`** - Unassigns Zoom Phone user's phone number.

After assigning a phone number, you can remove it if you do not want it to be assigned to anyone.

Prerequisites: 
* A Business or Enterprise account 
* A Zoom Phone license 
* The user must have been previously assigned a Zoom Phone number

Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:delete:user_number`,`phone:delete:user_number:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users update-call-handling-setting`** - Updates a user's call handling settings for the specified hour type.

Prerequisites
* A Business or Enterprise account
* A Zoom Phone license

Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:update:user_call_handling_setting:admin`,`phone:update:user_call_handling_setting`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users update-calling-plan`** - Switches the calling plans of a Zoom Phone user. For user-level apps, pass the `me` value instead of the `userId` parameter. 

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license



Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:update:calling_plan`,`phone:update:calling_plan:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users update-outbound-calling-countries-or-regions`** - Updates the user level outbound calling policy country or region.

Prerequisite:
* Account must have a Pro or a higher plan with a Zoom Phone license.

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:user_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users update-outbound-calling-exception-rule`** - Updates the user level outbound calling policy for the country region exception rule.

Prerequisites:

* Pro or a higher account with Zoom Phone license.
* Account owner or admin permissions.

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:user_outbound_calling_rule:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users update-policy`** - Updates a user's Zoom Phone policy.

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`

Granular Scopes: `phone:update:user_policy:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users update-profile`** - Updates a user's Zoom Phone profile. For user-level apps, pass the `me` value instead of the `userId` parameter.

To add, update or remove the shared access members for voicemail and call recordings, use the Add/Update/Delete a user's shared access setting API. 

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:update:user`,`phone:update:user:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users update-properties-in-batch`** - Updates multiple users' properties in batch. For example, you can update the users' site when `batchType` is equal to `move_site`. You can update 10 users at a time. 

Prerequisites:
* Business, or Education account
* Zoom Phone license 





Scopes: `phone:write:admin`

Granular Scopes: `phone:update:batch_users:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users update-setting`** - Updates the user setting according to the setting type, specifically for delegation, intercom and shared access for voicemail and call recordings. For user-level apps, pass the `me` value instead of the `userId` parameter.

To see the shared access settings in the Zoom web portal, go to Admin &gt; Phone System Management &gt; Users &amp; Rooms . Choose Users and select User Policy. Go to Voicemail, Automatic Call Recording and Ad Hoc Call Recording.

 To view the delegation and intercom setting in your Zoom web portal, navigate to Admin &gt; Phone System Management &gt; Users &amp; Rooms. Choose the Users tab and select User Settings

 Prerequisites: 
* A Business or Enterprise account

Scopes: `phone:write:admin`,`phone:write`

Granular Scopes: `phone:update:shared_setting`,`phone:update:shared_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users update-settings`** - Updates the Zoom Phone profile settings of a user. For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:update:user_setting`,`phone:update:user_setting:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli users whatsapp-session`** - Returns details about WhatsApp sessions for a user. 

For user-level apps, pass the `me` value instead of the `userId` parameter.

Prerequisites: 
* Paid account 
* User-enabled Zoom phone

Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:list_whatsapp_sessions`,`phone:read:list_whatsapp_sessions:admin`

Rate Limit Label: `LIGHT`

### voice_mails

Manage voice mails

- **`zoom-phone-cli voice-mails account`** - Gets a user's Zoom Phone voicemails. 

Prerequisites 
* A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:read:admin`,`phone_voicemail:read:admin`

Granular Scopes: `phone:read:list_voicemails:admin`

Rate Limit Label: `HEAVY`
- **`zoom-phone-cli voice-mails delete`** - Deletes an account's voicemail message.

Prerequisites: 
* A Zoom Phone license

Scopes: `phone:write:admin`,`phone:write`,`phone_voicemail:write`,`phone_voicemail:write:admin`

Granular Scopes: `phone:delete:voicemail`,`phone:delete:voicemail:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli voice-mails get-details`** - Returns the information about a voicemail message. 

Prerequisites 
* Zoom Phone license

Scopes: `phone:read`,`phone:read:admin`,`phone_voicemail:read:admin`,`phone_voicemail:read`

Granular Scopes: `phone:read:voicemail`,`phone:read:voicemail:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli voice-mails phone-download-file`** - Downloads the Zoom phone voicemail. 

Prerequisites:

* A Business or Enterprise account
* A Zoom Phone license

Scopes: `phone:read`,`phone:read:admin`,`phone_voicemail:read`,`phone_voicemail:read:admin`

Granular Scopes: `phone:read:voicemail:admin`,`phone:read:voicemail`

Rate Limit Label: `MEDIUM`
- **`zoom-phone-cli voice-mails update-read-status`** - Updates the status of voicemail message.

Prerequisites:
 * A Business or Enterprise account 
* A Zoom Phone license

Scopes: `phone:write:admin`,`phone:write`,`phone_voicemail:write`,`phone_voicemail:write:admin`

Granular Scopes: `phone:update:voicemail`,`phone:update:voicemail:admin`

Rate Limit Label: `LIGHT`

### whatsapp

Manage whatsapp

- **`zoom-phone-cli whatsapp by-message-id`** - Returns details about a specific message in a WhatsApp session.

Prerequisites
* A paid account
* Zoom Phone enabled for the account
* For Zoom Phone WhatsApp: a Zoom Phone-enabled user



Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:whatsapp_message`,`phone:read:whatsapp_message:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli whatsapp list-whats-app-sessions`** - Returns details about WhatsApp sessions.

Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:list_whatsapp_sessions`,`phone:read:list_whatsapp_sessions:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli whatsapp post-whats-app-message`** - Sends a WhatsApp message through a Zoom Phone WhatsApp Business Account (WABA). The sender phone number must be registered as a Meta WhatsApp Business phone number and linked to a WABA within the Zoom account.

Supported Outbound Message Types
* `text`  -  Free-text message. Only allowed within a 24-hour session window opened by an inbound message from the recipient.
* `template`  -  Highly Structured Message (HSM) template pre-approved by Meta. Can be sent at any time, including outside the 24-hour window.
* `image` / `document`  -  Rich media messages. Allowed only within the 24-hour session window. Media is delivered inline as base64-encoded content through `attachments`.

24-hour Session Window
WhatsApp enforces a customer-initiated messaging policy. After the recipient sends a message, a 24-hour window opens during which the business can reply with any message type. Once the window expires, only pre-approved `template` messages may be sent.

Prerequisites
* A Zoom Phone license
* A WhatsApp Business Account (WABA) connected to the Zoom account
* At least one Meta-registered phone number linked to the WABA
* For `template` messages: a Meta-approved message template

Scopes: `phone:write`,`phone:write:admin`

Granular Scopes: `phone:write:whatsapp_message`,`phone:write:whatsapp_message:admin`

Rate Limit Label: `LIGHT`
- **`zoom-phone-cli whatsapp session-details`** - Returns details about a WhatsApp session.

Prerequisites
* A paid account
* Zoom Phone enabled for the account
* A Zoom Phone-enabled user with WhatsApp messaging enabled



Scopes: `phone:read`,`phone:read:admin`

Granular Scopes: `phone:read:whatsapp_session`,`phone:read:whatsapp_session:admin`

Rate Limit Label: `LIGHT`


### accounts


- **`zoom-phone-cli accounts connect`** - Save one profile per client Zoom account (its own credentials, token cache and local store) so you can switch tenants without re-entering credentials.


### callbacks


- **`zoom-phone-cli callbacks`** - External callers whose calls were missed or abandoned and who have not been called back yet.


### devices


- **`zoom-phone-cli devices health`** - From the locally synced store: offline or unassigned desk phones, firmware drift per model, devices assigned to inactive users, grouped by site.


### e911


- **`zoom-phone-cli e911 audit`** - From the locally synced store: users, common areas and sites whose emergency address is missing or unverified, plus BYOC numbers needing a carrier update.


### fleet


- **`zoom-phone-cli fleet`** - Run one read-only command (or sync) across every saved client account and get one merged result with an account column; writes need --allow-writes.


### footprint


- **`zoom-phone-cli footprint`** - Everything one phone user holds: extension, numbers, calling plans, call queue membership, shared lines, pickup and monitoring groups, desk phones and emergency address.


### numbers


- **`zoom-phone-cli numbers inventory`** - From the locally synced store: phone numbers by site, source and assignee, with unassigned, pending and inactive-assignee lists.


### offboard


- **`zoom-phone-cli offboard`** - Ordered removal plan for a departing user across queues, shared lines, groups, devices, numbers and calling plan; --apply executes it.


### routing


- **`zoom-phone-cli routing trace`** - Follow one inbound number or extension through auto receptionists, IVR keys and call queues to where it rings, for business and closed hours.

- **`zoom-phone-cli routing audit`** - Dead IVR targets, empty call queues, orphaned auto receptionists and auto receptionists with no upcoming holiday hours.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`zoom-phone-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`zoom-phone-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`zoom-phone-cli learnings list`** - Inspect taught rows
- **`zoom-phone-cli learnings forget <query>`** - Undo a teach
- **`zoom-phone-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`zoom-phone-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`zoom-phone-cli teach-pattern`** - Install a query/resource template up front
- **`zoom-phone-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `ZOOM_PHONE_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `zoom-phone-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
zoom-phone-cli auto-receptionists list

# JSON for scripting and agents
zoom-phone-cli auto-receptionists list --json
# Filter to specific fields
zoom-phone-cli auto-receptionists list --json --select audio_prompt_language,cost_center,department

# Dry run  -  show the request without sending
zoom-phone-cli auto-receptionists list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
zoom-phone-cli auto-receptionists list --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending; it is opt-in, so a write without it is sent at once (MCP writes instead always need `confirm: true`)
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Explicit confirmation** - `--agent` does not imply `--yes`; pass `--yes` separately only after the target, arguments, and side effects are clear. `--yes` answers this CLI's own local prompts (profile and local-data deletion); it does not gate API writes
- **Piped input** - write commands can accept structured input when their help lists `--stdin`. The credential fields of the body (a PIN, a password) are kept out of everything the program prints; a body that is not JSON is refused.
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Freshness

This CLI owns bounded freshness for registered store-backed read command paths. In `--data-source auto` mode, covered commands check the local SQLite store before serving results; stale or missing resources trigger a bounded refresh, and refresh failures fall back to the existing local data with a warning. `--data-source local` never refreshes, and `--data-source live` reads the API without mutating the local store.

Set `ZOOM_PHONE_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered commands: the list and get commands of resources that `sync` stores, for example `users list-phone`, `call-queues list`, `numbers list-account-phone`, `devices list-phone`, `sites list` and `emergency-addresses list` (run `zoom-phone-cli sync --help` for the synced resource names). The audits (`e911 audit`, `numbers inventory`, `devices health`, `footprint`) read only the local store and report `assessed: false` until every resource they need has a completed sync.

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
zoom-phone-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `zoom-phone-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/zoom-phone-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\zoom-phone-cli\config.toml` on Windows; `--home`, `ZOOM_PHONE_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `ZOOM_PHONE_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `ZOOM_PHONE_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `ZOOM_PHONE_ACCOUNT_ID` | auth_flow_input | Yes | Account ID of the Zoom Server-to-Server OAuth app (sent as account_id on the token request). |
| `ZOOM_PHONE_CLIENT_ID` | auth_flow_input | Yes | Client ID of the Zoom Server-to-Server OAuth app. |
| `ZOOM_PHONE_CLIENT_SECRET` | auth_flow_input | Yes | Client Secret of the Zoom Server-to-Server OAuth app. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `zoom-phone-cli doctor` reports `agentcookie: detected` and `auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `zoom-phone-cli doctor` to check credentials
- Verify all three are set: `for v in ZOOM_PHONE_ACCOUNT_ID ZOOM_PHONE_CLIENT_ID ZOOM_PHONE_CLIENT_SECRET; do printenv $v >/dev/null && echo "$v set" || echo "$v MISSING"; done`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **HTTP 400 invalid_client when minting a token**  -  Check ZOOM_PHONE_CLIENT_ID and ZOOM_PHONE_CLIENT_SECRET come from a Server-to-Server OAuth app (not a General app) and that the app is activated
- **HTTP 400 'Invalid account_id' or token mint fails with a valid client**  -  Set ZOOM_PHONE_ACCOUNT_ID to the Account ID shown on the S2S app's App Credentials page
- **HTTP 403 or code 104/124 on Phone endpoints**  -  Add the phone:read:admin and phone:write:admin scopes (or the granular phone:*:admin scopes) to the S2S app; accounts without a Zoom Phone license return 403
- **HTTP 429 Too Many Requests**  -  Zoom Phone limits are per account and shared across apps; lower sync --concurrency to 1 or 2 and retry later (heavy endpoints allow 15k-30k calls per day)
- **call-logs commands return errors or nothing**  -  Account call logs were sunset in June 2025; use call-history account instead
- **An audit returns no findings right after install**  -  Check assessed and missing_reasons in the output; run the sync --full command from the Quick Start first, since e911 audit, numbers inventory and devices health read the local store and only trust a complete, unfiltered, full sync
- **`[credkit: withheld from ...]` in place of output**  -  By design: that output carried a credential this process holds, in some encoding, so the whole unit was replaced.
- **Exit 78, `refusing to start`**  -  A credential on this run equals fixed protocol text the program must always be able to print: a `--pin-code` that is also an HTTP status code (404) or another reserved number. Use a different PIN.
- **Device SIP passwords, desk-phone or port PINs, smartphone activation codes or Team Chat channel tokens show as <redacted>**  -  By design: this CLI never outputs or stores credential fields; view them in the Zoom web portal

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**zoom-phone-cli**](https://github.com/ku/zoom-phone-cli)  -  Go (1 stars)
- [**zoom-phone-mcp**](https://github.com/FujitsuPolycom/zoom-phone-mcp)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
