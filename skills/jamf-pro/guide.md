# Jamf Pro CLI

**Every client's Jamf Pro in one CLI: register each instance once, then find devices, check posture and patch status, and audit policies across all of them.**

Covers the Jamf Pro API and the Classic API, including script, package and policy CRUD; list commands read every page with --all instead of stopping at the default 100. 'fleet find', 'fleet posture', 'fleet patch' and 'fleet policy-hygiene' answer cross-client questions that otherwise mean logging into each instance in turn; they need each client registered once with 'instance add'.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `jamf-pro-cli` and `jamf-pro-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=jamf-pro). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/jamf-pro --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/jamf-pro --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `jamf-pro-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the jamf-pro skill from https://github.com/Servosity/msp-skills/tree/main/skills/jamf-pro. The skill defines how its required CLI (`jamf-pro-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=jamf-pro).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `JAMF_PRO_SERVER`, `JAMF_PRO_CLIENT_ID` and `JAMF_PRO_CLIENT_SECRET` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/jamf-pro/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "jamf-pro": {
      "command": "jamf-pro-mcp",
      "env": {
        "JAMF_PRO_SERVER": "<server>",
        "JAMF_PRO_CLIENT_ID": "<client-id>",
        "JAMF_PRO_CLIENT_SECRET": "<client-secret>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`jamf-pro-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `JAMF_PRO_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `JAMF_PRO_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export JAMF_PRO_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
jamf-pro-mcp --transport http
```

On Windows (PowerShell):

```powershell
$env:JAMF_PRO_MCP_HTTP_TOKEN = [guid]::NewGuid().ToString('N') + [guid]::NewGuid().ToString('N')
jamf-pro-mcp --transport http
```

Set `JAMF_PRO_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Each Jamf Pro instance has its own API client (Settings > System > API roles and clients). The CLI exchanges the client ID and secret for a short-lived Bearer token at https://<server>/api/v1/oauth/token (form-encoded client_id and client_secret), and that token works for both the Jamf Pro API and the Classic API. For one instance, set JAMF_PRO_SERVER, JAMF_PRO_CLIENT_ID and JAMF_PRO_CLIENT_SECRET (an env-sourced secret is never written to disk), or run 'auth login --client-id <id> --client-secret <secret>', which stores the secret in the CLI's credentials file; 'auth status' shows what is in use. For many clients, run 'instance add <name> --server <host> --client-id <id>' once per client and put each client secret in JAMF_PRO_FLEET_SECRETS as name=secret entries separated by commas; the registry never stores secrets, and 'instance test' proves every registration.

## Quick Start

```bash
# Confirm the binary runs; drop --dry-run to check auth and connectivity
jamf-pro-cli doctor --dry-run

# Register a client instance; then add acme=<client secret> to JAMF_PRO_FLEET_SECRETS
jamf-pro-cli instance add acme --server acme.jamfcloud.com --client-id 6b1a0d2e-1111-4c3e-9f00-000000000001

# Prove every registered API client can get a token and read the Jamf Pro version
jamf-pro-cli instance test --agent

# Compliance rollup across every registered client
jamf-pro-cli fleet posture --stale-days 14 --min-os 14.6 --agent

# Locate a device by serial without knowing the client
jamf-pro-cli fleet find C02XK1ABJG5H --agent

```

## Unique Features

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

## What output withholds

Every command's output (JSON, table, CSV, plain), every MCP result, every exported or delivered file and every local-store row passes through one redaction step, the same one for fleet commands and single-instance commands. It withholds, whole, the credentials Jamf Pro stores in its own credential fields (LAPS and recovery-lock passwords, FileVault and recovery keys, API client secrets, tokens, PINs), the values of extension attributes or other name/value records labelled as credentials, and credentials written plainly or in a standard encoding (URL, HTML or JSON escapes, base64, XML elements) inside any value, including URLs that carry credentials. Script bodies, configuration profile payloads, app configuration preferences, a policy's run command and script parameters 4-11 print as `[redacted sha256:...]`, a digest that never shows the content but still compares between instances. A downloaded document (a script, CSV or XML file) that carries a credential is withheld whole. Token-endpoint errors print only the HTTP status and a standard OAuth error code. It does not claim to catch a credential deliberately obfuscated to hide it: anyone who can do that can equally store a password in an extension attribute named "Notes", which no redactor can recognise. Keep credentials out of inventory fields.

## Credential safety

- **Nothing prints a credential.** Client secrets (`JAMF_PRO_CLIENT_SECRET`, every entry in `JAMF_PRO_FLEET_SECRETS`), access tokens the CLI mints, and passwords sent in a request or carried in a response are held for the whole run. Any output, error, MCP result, exported or delivered file, or local-store row that repeats one, in any common encoding or split across lines or values, prints `[text withheld: it repeated a secret value this program holds]` in its place; JSON output stays JSON. Credential-named fields (passwords, PINs, keys, tokens, secrets) print `[redacted]`.
- **Requests stay on the Jamf Pro server.** Every request, the token exchange included, goes only to the configured server; a redirect is refused before anything is sent.
- **No credential on disk** except the private (0600) credential store; GET responses are not cached on disk.
- **Previews send nothing.** Under `--dry-run` no request leaves the CLI.
- **MCP writes need confirmation.** `jamf-pro_execute` and mirrored commands that can change a Jamf Pro instance return a preview until called again with `confirm: true` (a boolean).

## Usage

Run `jamf-pro-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `JAMF_PRO_CONFIG_DIR`, `JAMF_PRO_DATA_DIR`, `JAMF_PRO_STATE_DIR`, or `JAMF_PRO_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `JAMF_PRO_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export JAMF_PRO_HOME=/srv/jamf-pro
jamf-pro-cli doctor
```

On Windows (PowerShell):

```powershell
$env:JAMF_PRO_HOME = "C:\ProgramData\jamf-pro"
jamf-pro-cli doctor
```

Under `JAMF_PRO_HOME=/srv/jamf-pro`, the four dirs resolve to `/srv/jamf-pro/config`, `/srv/jamf-pro/data`, `/srv/jamf-pro/state`, and `/srv/jamf-pro/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:JAMF_PRO_HOME = "C:\srv\jamf-pro"
jamf-pro-cli doctor
```

Under `$env:JAMF_PRO_HOME = "C:\srv\jamf-pro"`, the four dirs resolve to `C:\srv\jamf-pro\config`, `C:\srv\jamf-pro\data`, `C:\srv\jamf-pro\state`, and `C:\srv\jamf-pro\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `JAMF_PRO_DATA_DIR` overrides an explicit `--home` for that kind. Use `JAMF_PRO_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `JAMF_PRO_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `jamf-pro-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are not cached on disk, and credential-named fields are redacted before a row reaches the store. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `jamf-pro-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `jamf-pro-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `JAMF_PRO_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `JAMF_PRO_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### account_groups

Manage account groups

- **`jamf-pro-cli account-groups get`** - Gets the account group for the given id.


Required privilege: Read Account Groups
- **`jamf-pro-cli account-groups list`** - Get all account groups with pagination, sorting, and filtering support.

Required privilege: Read Account Groups

### accounts

Manage accounts

- **`jamf-pro-cli accounts get`** - Gets the user account for the given id.


Required privilege: Read Accounts
- **`jamf-pro-cli accounts list`** - Get all user accounts with pagination, sorting, and filtering support.

Required privilege: Read Accounts

### advanced_computer_searches

Manage advanced computer searches

- **`jamf-pro-cli advanced-computer-searches get`** - Additional display fields are returned within the `Computer` object
- **`jamf-pro-cli advanced-computer-searches list`** - Finds all advanced computer searches

### api_integrations

Manage api integrations

- **`jamf-pro-cli api-integrations create`** - Create API integration object.

Required privilege: Create API Integrations
- **`jamf-pro-cli api-integrations delete`** - Removes specified API integration.

Required privilege: Delete API Integrations
- **`jamf-pro-cli api-integrations get`** - Gets specified API integration object.

Required privilege: Read API Integrations
- **`jamf-pro-cli api-integrations list`** - Get Jamf Pro API Integrations with Search Criteria.

Required privilege: Read API Integrations
- **`jamf-pro-cli api-integrations update`** - Update specified API integration object.

Required privilege: Update API Integrations

### api_role_privileges

Manage api role privileges

- **`jamf-pro-cli api-role-privileges`** - Get role privileges.

Required privilege: Read API Roles

### api_roles

Manage api roles

- **`jamf-pro-cli api-roles create`** - Post to create new Role.

Required privilege: Create API Roles
- **`jamf-pro-cli api-roles delete`** - Delete specific Role.

Required privilege: Delete API Roles
- **`jamf-pro-cli api-roles get`** - Get specific Role.

Required privilege: Read API Roles
- **`jamf-pro-cli api-roles list`** - Get roles with Search Criteria.

Required privilege: Read API Roles
- **`jamf-pro-cli api-roles update`** - Update specific Role.

Required privilege: Update API Roles

### buildings

Manage buildings

- **`jamf-pro-cli buildings add-history-note`** - Adds specified Building history object notes.


Required privilege: Update Buildings
- **`jamf-pro-cli buildings create`** - Create building record.


Required privilege: Create Buildings
- **`jamf-pro-cli buildings delete`** - Removes specified building record.


Required privilege: Delete Buildings
- **`jamf-pro-cli buildings delete-multiple`** - multiple many Buildings by their ids.

Required privilege: Delete Buildings
- **`jamf-pro-cli buildings get`** - Gets specified Building object.


Required privilege: Read Buildings
- **`jamf-pro-cli buildings get-history`** - Gets specified Building history object.


Required privilege: Read Buildings
- **`jamf-pro-cli buildings list`** - Search for sorted and paged buildings.

Required privilege: Read Buildings
- **`jamf-pro-cli buildings update`** - Update specified building object.


Required privilege: Update Buildings

### categories

Manage categories

- **`jamf-pro-cli categories add-history-note`** - Adds specified Category history object notes.


Required privilege: Update Categories
- **`jamf-pro-cli categories create`** - Create category record.


Required privilege: Create Categories
- **`jamf-pro-cli categories delete`** - Removes specified category record.


Required privilege: Delete Categories
- **`jamf-pro-cli categories delete-multiple`** - Delete multiple Categories by their IDs.

Required privilege: Delete Categories
- **`jamf-pro-cli categories get`** - Gets specified Category object.


Required privilege: Read Categories
- **`jamf-pro-cli categories get-history`** - Gets specified Category history object.


Required privilege: Read Categories
- **`jamf-pro-cli categories list`** - Gets `Category` objects.


Required privileges:
> - Read Categories
> - Read Self Service
- **`jamf-pro-cli categories update`** - Update specified category object.


Required privilege: Update Categories

### computer_applications

Manage computer applications

- **`jamf-pro-cli computer-applications get-by-application`** - Finds computer applications by name
- **`jamf-pro-cli computer-applications get-by-application-inventory`** - Valid values for inventory are comma separated Display Fields for computers. Display Fields with spaces must be percent encoded (e.g. /computerapplications/application/Safari.app/inventory/Operating%20System,Last%20Check-in)
- **`jamf-pro-cli computer-applications get-by-version`** - Finds computer applications by name and version
- **`jamf-pro-cli computer-applications get-by-version-inventory`** - Generates additional inventory data for the devices matching the search criteria. Inventory values may be displayed in addition to computer identification information. Use Default to view the same fields as those checked for Inventory Display, otherwise list display fields separated by commas.

### computer_extension_attributes

Manage computer extension attributes

- **`jamf-pro-cli computer-extension-attributes add-history-note`** - Add specified Computer Extension Attribute history object notes.

Required privilege: Update Computer Extension Attributes
- **`jamf-pro-cli computer-extension-attributes create`** - Create Computer Extension Attribute to collect extra inventory information.

Required privilege: Create Computer Extension Attributes
- **`jamf-pro-cli computer-extension-attributes delete`** - ID of the Computer Extension Attribute to be deleted.

Required privilege: Delete Computer Extension Attributes
- **`jamf-pro-cli computer-extension-attributes delete-multiple`** - IDs of the Computer Extension Attribute to be deleted.

Required privilege: Delete Computer Extension Attributes
- **`jamf-pro-cli computer-extension-attributes get`** - Gets specified Computer Extension Attribute object.

Required privilege: Read Computer Extension Attributes
- **`jamf-pro-cli computer-extension-attributes get-data-dependency`** - Get smart group/advance search dependent objects for a specified computer extension attribute.

Required privilege: Read Computer Extension Attributes
- **`jamf-pro-cli computer-extension-attributes get-download`** - Retrieves the specified Computer Extension Attribute in XML format based on the provided unique ID.

Required privilege: Read Computer Extension Attributes
- **`jamf-pro-cli computer-extension-attributes get-history`** - Get specified Computer Extension Attribute history object.

Required privilege: Read Computer Extension Attributes
- **`jamf-pro-cli computer-extension-attributes get-templates`** - Gets specified Computer Extension Attribute Template object.

Required privilege: Read Computer Extension Attributes
- **`jamf-pro-cli computer-extension-attributes list`** - Retrieves All Computer Extension Attributes Configuration.

Required privilege: Read Computer Extension Attributes
- **`jamf-pro-cli computer-extension-attributes list-templates`** - Retrieves All Computer Extension Attributes Templates.

Required privilege: Read Computer Extension Attributes
- **`jamf-pro-cli computer-extension-attributes update`** - Update specified Computer Extension Attribute object.

Required privilege: Update Computer Extension Attributes
- **`jamf-pro-cli computer-extension-attributes upload`** - Uploads a Computer Extension Attribute.

Required privileges:
> - Create Computer Extension Attributes
> - Read Computer Extension Attributes

### computer_groups

Manage computer groups

- **`jamf-pro-cli computer-groups create-smart-groups`** - Creates a Smart Computer Group with strict andOr enum validation.


Required privilege: Create Smart Computer Groups
- **`jamf-pro-cli computer-groups create-static-groups`** - Create membership of a static computer group.


Required privilege: Create Static Computer Groups
- **`jamf-pro-cli computer-groups delete-smart-groups`** - Remove specified Smart Computer Group. Returns 422 if the group has dependencies.


Required privilege: Delete Smart Computer Groups
- **`jamf-pro-cli computer-groups delete-static-groups`** - Remove Static Computer Group by Id. Returns 422 if the group has dependencies.


Required privilege: Delete Static Computer Groups
- **`jamf-pro-cli computer-groups get-smart-group-membership`** - Gets the membership of a Smart Computer Group.


Required privilege: Read Smart Computer Groups
- **`jamf-pro-cli computer-groups get-smart-groups`** - Get Smart Computer Group by Id.


Required privilege: Read Smart Computer Groups
- **`jamf-pro-cli computer-groups get-static-groups`** - Get Static Computer Group by Id.


Required privilege: Read Static Computer Groups
- **`jamf-pro-cli computer-groups list`** - Use it to get the list of all computer groups.


Required privileges:
> - Read Smart Computer Groups
> - Read Static Computer Groups
- **`jamf-pro-cli computer-groups list-smart-groups`** - Search for Smart Computer Groups.


Required privilege: Read Smart Computer Groups
- **`jamf-pro-cli computer-groups list-static-groups`** - Search for Static Computer Groups.


Required privilege: Read Static Computer Groups
- **`jamf-pro-cli computer-groups update-smart-groups`** - Updates a Smart Computer Group with strict andOr enum validation.


Required privilege: Update Smart Computer Groups
- **`jamf-pro-cli computer-groups update-static-groups`** - Update membership of a static computer group.


Required privilege: Update Static Computer Groups

### computer_history

Manage computer history

- **`jamf-pro-cli computer-history get`** - Finds computer history by ID
- **`jamf-pro-cli computer-history get-subset`** - Subset values can also be appended using an ampersand to return multiple subsets (e.g. /subsets/General&Commands)

### computer_inventory

Manage computer inventory

- **`jamf-pro-cli computer-inventory create`** - Creates Computer Inventory record.

Required privilege: Create Computers
- **`jamf-pro-cli computer-inventory delete`** - Remove specified Computer record.

Required privilege: Delete Computers
- **`jamf-pro-cli computer-inventory erase`** - Erase a computer.

Required privilege: Send Computer Remote Wipe Command
- **`jamf-pro-cli computer-inventory get`** - Return General section of a Computer.

Required privilege: Read Computers
- **`jamf-pro-cli computer-inventory get-detail`** - Return all sections of a computer.

Required privilege: Read Computers
- **`jamf-pro-cli computer-inventory list`** - Return paginated Computer Inventory records.

Required privilege: Read Computers
- **`jamf-pro-cli computer-inventory remove-mdm-profile`** - Remove a computer's MDM profile.

Required privilege: Send Computer Unmanage Command
- **`jamf-pro-cli computer-inventory update`** - Update specific fields on a computer, then return the updated computer object.

Required privilege: Update Computers

### computer_management

Manage computer management

- **`jamf-pro-cli computer-management get`** - Finds computer management information by ID
- **`jamf-pro-cli computer-management get-subset`** - Subset values can also be appended using an ampersand to return multiple subsets (e.g. /subsets/General&Policies)

### computer_prestages

Manage computer prestages

- **`jamf-pro-cli computer-prestages create`** - Create a computer prestage.

Required privilege: Create Computer PreStage Enrollments
- **`jamf-pro-cli computer-prestages create-scope`** - Add device scope for a specific computer prestage.

Required privilege: Update Computer PreStage Enrollments
- **`jamf-pro-cli computer-prestages delete`** - Deletes a Computer Prestage with the supplied id.

Required privilege: Delete Computer PreStage Enrollments
- **`jamf-pro-cli computer-prestages get`** - Retrieves a Computer Prestage with the supplied id.

Required privilege: Read Computer PreStage Enrollments
- **`jamf-pro-cli computer-prestages get-scope`** - Get device scope for a specific computer prestage.

Required privilege: Read Computer PreStage Enrollments
- **`jamf-pro-cli computer-prestages list`** - Gets sorted and paged computer prestages.

Required privilege: Read Computer PreStage Enrollments
- **`jamf-pro-cli computer-prestages list-scope`** - Get all device scope for all computer prestages.

Required privilege: Read Computer PreStage Enrollments
- **`jamf-pro-cli computer-prestages scope-delete-multiple`** - Remove device scope for a specific computer prestage.

Required privilege: Update Computer PreStage Enrollments
- **`jamf-pro-cli computer-prestages update`** - Updates a Computer Prestage.

Required privilege: Update Computer PreStage Enrollments
- **`jamf-pro-cli computer-prestages update-scope`** - Replace device scope for a specific computer prestage.

Required privilege: Update Computer PreStage Enrollments

### computers_preview

Manage computers preview

- **`jamf-pro-cli computers-preview`** - Returns a list of computers.

Required privilege: Read Computers

### departments

Manage departments

- **`jamf-pro-cli departments add-history-note`** - Adds specified Department history object notes.


Required privilege: Update Departments
- **`jamf-pro-cli departments create`** - Create department record.


Required privilege: Create Departments
- **`jamf-pro-cli departments delete`** - Removes specified department record.


Required privilege: Delete Departments
- **`jamf-pro-cli departments delete-multiple`** - Deletes all departments by ids passed in body.


Required privilege: Delete Departments
- **`jamf-pro-cli departments get`** - Gets specified Department object.


Required privilege: Read Departments
- **`jamf-pro-cli departments get-history`** - Gets specified Department history object.


Required privilege: Read Departments
- **`jamf-pro-cli departments list`** - Search for Departments.


Required privilege: Read Departments
- **`jamf-pro-cli departments update`** - Update specified department object.


Required privilege: Update Departments

### device_enrollments

Manage device enrollments

- **`jamf-pro-cli device-enrollments add-history-note`** - Adds device enrollment history object notes.

Required privilege: Update Device Enrollment Program Instances
- **`jamf-pro-cli device-enrollments delete`** - Deletes a Device Enrollment Instance with the supplied id.

Required privilege: Delete Device Enrollment Program Instances
- **`jamf-pro-cli device-enrollments disown`** - Disowns devices from the given device enrollment instance.

Required privilege: Update Device Enrollment Program Instances
- **`jamf-pro-cli device-enrollments get`** - Retrieves a Device Enrollment Instance with the supplied id.

Required privilege: Read Device Enrollment Program Instances
- **`jamf-pro-cli device-enrollments get-history`** - Gets sorted and paged device enrollment history objects.

Required privilege: Read Device Enrollment Program Instances
- **`jamf-pro-cli device-enrollments get-syncs`** - Get all instance sync states for a single instance.

Required privilege: Read Device Enrollment Program Instances
- **`jamf-pro-cli device-enrollments get-syncs-latest`** - Get the latest sync state for a single instance.

Required privilege: Read Device Enrollment Program Instances
- **`jamf-pro-cli device-enrollments list`** - Search for sorted and paged device enrollment instances.

Required privilege: Read Device Enrollment Program Instances
- **`jamf-pro-cli device-enrollments list-public-key`** - Retrieve the Jamf Pro device enrollment public key.

Required privilege: Read Device Enrollment Program Instances
- **`jamf-pro-cli device-enrollments list-syncs`** - Get all instance sync states for all instances.

Required privilege: Read Device Enrollment Program Instances
- **`jamf-pro-cli device-enrollments update`** - Updates a Device Enrollment Instance with the supplied id.

Required privilege: Update Device Enrollment Program Instances

### device_enrollments_devices

Manage device enrollments devices

- **`jamf-pro-cli device-enrollments-devices <id>`** - Retrieves a list of devices assigned to the supplied id.

Required privilege: Read Device Enrollment Program Instances

### device_users

Manage device users

- **`jamf-pro-cli device-users get`** - Finds users by ID
- **`jamf-pro-cli device-users list`** - Finds all users

### disk_encryption_configurations

Manage disk encryption configurations

- **`jamf-pro-cli disk-encryption-configurations get`** - Finds disk encryption configurations by ID
- **`jamf-pro-cli disk-encryption-configurations list`** - Finds all disk encryption configurations

### groups

Manage groups

- **`jamf-pro-cli groups delete`** - Deletes a group by its platform UUID. Returns a 422 error if the group is being used as a dependency. Requires appropriate DELETE privileges.
- **`jamf-pro-cli groups get`** - Returns group information for the given platform UUID. Dependent upon the returned group type the corresponding READ privilege for that group type will be needed.
- **`jamf-pro-cli groups list`** - Returns group information for all Mobile Device and Computer groups. The type of groups returned will be dependent upon the corresponding group type READ privileges. Results can be sorted by name, description, group type, or isSmart. Default sorting is by group name in ascending order.
- **`jamf-pro-cli groups update`** - Updates a group by its platform UUID. For both smart and static groups, groupName and groupDescription can be updated. For smart groups, criteria can also be updated. For static groups, assignments can also be updated. Requires appropriate UPDATE privileges.

### health_check

Manage health check

- **`jamf-pro-cli health-check list`** - Get Jamf Pro API status. Which response codes might be returned in error states will depend on the specific state encountered.
- **`jamf-pro-cli health-check list-status`** - Returns metrics representing the request acceptance ratio for each concurrency group and time window on this Jamf Pro node. The acceptance ratio is a decimal value between 0 and 1, where 1 means all requests were accepted and 0 means all were denied. Health status metrics are only available in Jamf Cloud. This API will return a 404 if the Jamf Pro node does not support health status metrics.

### inventory_information

Manage inventory information

- **`jamf-pro-cli inventory-information`** - Gets statistics about managed/unmanaged devices and computers in the inventory.

### jamf_pro_information

Read basic information about the Jamf Pro server

- **`jamf-pro-cli jamf-pro-information`** - Get basic information about the Jamf Pro Server

### jamf_pro_version

Read the Jamf Pro version

- **`jamf-pro-cli jamf-pro-version`** - Returns information about the Jamf Pro including the current version.

### local_admin_password

Manage local admin password

- **`jamf-pro-cli local-admin-password get-account-history`** - Get the full history of all for a specific username on a target device. History will include date created, date last seen, expiration time, and rotational status. Get audit history by using the client management id and username as the path parameters.

Required privilege: View Local Admin Password Audit History
- **`jamf-pro-cli local-admin-password get-account-history-by-guid`** - Get the full history of all for a specific user guid on a target device. History will include date created, date last seen, expiration time, and rotational status. Get audit history by using the client management id, username, and user guid as the path parameters.

Required privilege: View Local Admin Password Audit History
- **`jamf-pro-cli local-admin-password get-accounts`** - Get a full list of admin accounts that are LAPS capable. Capable accounts are returned in the AutoSetupAdminAccounts from QueryResponses.

Required privilege: View Local Admin Password
- **`jamf-pro-cli local-admin-password get-history`** - Get the full history of all local admin passwords for all accounts for a specific management ID. History will include password, who viewed the password and when it was viewed. This will include rotation history as well.

Required privilege: View Local Admin Password Audit History
- **`jamf-pro-cli local-admin-password list-pending-rotations`** - Return information about all devices and usernames currently in the state of a pending LAPS rotation.

Required privilege: View Local Admin Password
- **`jamf-pro-cli local-admin-password list-settings`** - Return information about the current LAPS settings.

Required privileges:
> - Read User-Initiated Enrollment
> - Update Local Admin Password Settings
- **`jamf-pro-cli local-admin-password update-settings`** - Update settings for LAPS.

Required privilege: Update Local Admin Password Settings

### macos_configuration_profiles

Manage macos configuration profiles

- **`jamf-pro-cli macos-configuration-profiles get`** - Finds OS X configuration profiles by ID
- **`jamf-pro-cli macos-configuration-profiles get-subset`** - Subset values can also be appended using an ampersand to return multiple subsets (e.g. /subsets/General&Scope)
- **`jamf-pro-cli macos-configuration-profiles list`** - Finds all OS X configuration profiles

### managed_software_updates

Manage managed software updates

- **`jamf-pro-cli managed-software-updates create-plans`** - Creates a Managed Software Update Plan.


Required privileges:
> - Create Managed Software Updates
> - Read Computers
> - Read Mobile Devices
> - Send Computer Remote Command to Download and Install OS X Update
> - Send Mobile Device Remote Command to Download and Install iOS Update
- **`jamf-pro-cli managed-software-updates create-plans-group`** - Creates Managed Software Update Plans for a Group.


Required privileges:
> - Create Managed Software Updates
> - Read Computers
> - Read Mobile Devices
> - Read Smart Computer Groups
> - Read Static Computer Groups
> - Read Smart Mobile Device Groups
> - Read Static Mobile Device Groups
> - Send Computer Remote Command to Download and Install OS X Update
> - Send Mobile Device Remote Command to Download and Install iOS Update
- **`jamf-pro-cli managed-software-updates get-plans`** - Retrieves a Managed Software Update Plan.


Required privileges:
> - Read Managed Software Updates
> - Read Computers
> - Read Mobile Devices
- **`jamf-pro-cli managed-software-updates get-plans-declarations`** - Retrieves all Declarations associated with a Managed Software Update Plan.


Required privileges:
> - Read Managed Software Updates
> - Read Computers
> - Read Mobile Devices
- **`jamf-pro-cli managed-software-updates get-plans-events`** - Retrieves a Managed Software Update Plan Event Store.


Required privileges:
> - Read Managed Software Updates
> - Read Computers
> - Read Mobile Devices
- **`jamf-pro-cli managed-software-updates get-plans-group`** - Retrieves Managed Software Update Plans for a Group.


Required privileges:
> - Read Managed Software Updates
> - Read Computers
> - Read Smart Computer Groups
> - Read Static Computer Groups
> - Read Mobile Devices
> - Read Smart Mobile Device Groups
> - Read Static Mobile Device Groups
- **`jamf-pro-cli managed-software-updates get-update-statuses-computer-groups`** - Retrieve Managed Software Update Statuses for Computer Groups.


Required privileges:
> - Read Computers
> - Read Smart Computer Groups
> - Read Static Computer Groups
- **`jamf-pro-cli managed-software-updates get-update-statuses-computers`** - Retrieve Managed Software Update Statuses for Computers.


Required privilege: Read Computers
- **`jamf-pro-cli managed-software-updates get-update-statuses-mobile-device-groups`** - Retrieve Managed Software Update Statuses for Mobile Device Groups.


Required privileges:
> - Read Mobile Devices
> - Read Smart Mobile Device Groups
> - Read Static Mobile Device Groups
- **`jamf-pro-cli managed-software-updates get-update-statuses-mobile-devices`** - Retrieve Managed Software Update Statuses for Mobile Devices.


Required privilege: Read Mobile Devices
- **`jamf-pro-cli managed-software-updates list-available-updates`** - Retrieves available macOS and iOS Managed Software Updates
- **`jamf-pro-cli managed-software-updates list-plans`** - Retrieve Managed Software Update Plans.


Required privileges:
> - Read Managed Software Updates
> - Read Computers
> - Read Mobile Devices
- **`jamf-pro-cli managed-software-updates list-plans-feature-toggle`** - Retrieves current value of the Feature Toggle.


Required privilege: Read Managed Software Updates
- **`jamf-pro-cli managed-software-updates list-plans-feature-toggle-status`** - Retrieves background status of the Feature Toggle.


Required privilege: Read Managed Software Updates
- **`jamf-pro-cli managed-software-updates list-update-statuses`** - Retrieve Managed Software Update Statuses.


Required privileges:
> - Read Computers
> - Read Mobile Devices
- **`jamf-pro-cli managed-software-updates plans-feature-toggle-abandon`** - "Break Glass" endpoint, not for nominal usage.
Use this endpoint to forcefully abandon the feature-toggle background process if the status of the feature-toggle is 'stuck' or has reached an non-restartable failed state.
Usage of this endpoint under nominal conditions is undefined and unsupported.


Required privileges:
> - Read Managed Software Updates
> - Create Managed Software Updates
> - Update Managed Software Updates
- **`jamf-pro-cli managed-software-updates update-plans-feature-toggle`** - Updates the value of the Feature Toggle - This endpoint is asynchronous, the provided value will not be immediately updated. Please use the following endpoint to track the status of your toggle request. /v1/managed-software-updates/plans/feature-toggle/status:


Required privileges:
> - Read Managed Software Updates
> - Create Managed Software Updates
> - Update Managed Software Updates

### mdm

Manage mdm

- **`jamf-pro-cli mdm blank-push`** - Accepts a list of client management IDs and sends a blank push notification to each. Returns a list of UUIDs that encountered errors.

Required privilege: View MDM command information in Jamf Pro API
- **`jamf-pro-cli mdm deploy-package`** - Deploys packages to macOS devices using the InstallEnterpriseApplication MDM command.

Required privilege: Send Computer Remote Command to Install Package
- **`jamf-pro-cli mdm list-commands`** - Get information about mdm commands made by Jamf Pro.

Required privilege: View MDM command information in Jamf Pro API
- **`jamf-pro-cli mdm renew-profile`** - Renews the device's MDM Profile, including the device identity certificate within the MDM Profile.


Required privilege: Send Command to Renew MDM Profile
- **`jamf-pro-cli mdm send-command`** - Provided an MDM command type and appropriate information, will create and then queue said command. A separate privilege is required for each device type and MDM command you want to view or send.

Required privilege: View MDM command information in Jamf Pro API

### mobile_device_applications

Manage mobile device applications

- **`jamf-pro-cli mobile-device-applications get`** - Finds mobile device applications by ID
- **`jamf-pro-cli mobile-device-applications list`** - Finds all mobile device applications

### mobile_device_configuration_profiles

Manage mobile device configuration profiles

- **`jamf-pro-cli mobile-device-configuration-profiles get`** - Finds mobile device configuration profiles by ID
- **`jamf-pro-cli mobile-device-configuration-profiles list`** - Finds all mobile device configuration profiles

### mobile_device_extension_attributes

Manage mobile device extension attributes

- **`jamf-pro-cli mobile-device-extension-attributes add-history-note`** - Add specified Mobile Device Extension Attribute history object notes.

Required privilege: Update Mobile Device Extension Attributes
- **`jamf-pro-cli mobile-device-extension-attributes create`** - Create Mobile Device Extension Attribute to collect extra inventory information.

Required privilege: Create Mobile Device Extension Attributes
- **`jamf-pro-cli mobile-device-extension-attributes delete`** - Deletes the Mobile Device Extension Attribute identified by the provided ID.<\br>
In addition to removing the attribute itself, this operation will also delete any related dependent data, including:<\br>
  - Associated popup menu choices.<\br>
  - Fields used in saved search displays.<\br>
  - User preferences that reference the attribute.


Required privilege: Delete Mobile Device Extension Attributes
- **`jamf-pro-cli mobile-device-extension-attributes get`** - Gets specified Mobile Device Extension Attribute object.

Required privilege: Read Mobile Device Extension Attributes
- **`jamf-pro-cli mobile-device-extension-attributes get-data-dependency`** - Get smart group dependent object for a specified mobile device extension attribute.

Required privilege: Read Mobile Device Extension Attributes
- **`jamf-pro-cli mobile-device-extension-attributes get-history`** - Get specified Mobile Device Extension Attribute history object.

Required privilege: Read Mobile Device Extension Attributes
- **`jamf-pro-cli mobile-device-extension-attributes list`** - Retrieves all mobile device extension attributes configuration.

Required privilege: Read Mobile Device Extension Attributes
- **`jamf-pro-cli mobile-device-extension-attributes update`** - Update specified Mobile Device Extension Attribute object.

Required privilege: Update Mobile Device Extension Attributes

### mobile_device_groups

Manage mobile device groups

- **`jamf-pro-cli mobile-device-groups create-smart-groups`** - Create a smart group with strict andOr enum validation.


Required privilege: Create Smart Mobile Device Groups
- **`jamf-pro-cli mobile-device-groups create-static-groups`** - Create a static group.


Required privilege: Create Static Mobile Device Groups
- **`jamf-pro-cli mobile-device-groups delete-smart-groups`** - Remove Smart Group by Id. Returns 422 if the group has dependencies.


Required privilege: Delete Smart Mobile Device Groups
- **`jamf-pro-cli mobile-device-groups delete-static-groups`** - Remove Static Group by Id. Returns 422 if the group has dependencies.


Required privilege: Delete Static Mobile Device Groups
- **`jamf-pro-cli mobile-device-groups get-smart-group-membership`** - Get Smart Group Membership by Id.


Required privileges:
> - Read Smart Mobile Device Groups
> - Read Mobile Devices
- **`jamf-pro-cli mobile-device-groups get-smart-groups`** - Get Smart Group by Id.


Required privilege: Read Smart Mobile Device Groups
- **`jamf-pro-cli mobile-device-groups get-static-group-membership`** - Get Static Group Membership by Id.


Required privileges:
> - Read Static Mobile Device Groups
> - Read Mobile Devices
- **`jamf-pro-cli mobile-device-groups get-static-groups`** - Get Static Group by Id.


Required privilege: Read Static Mobile Device Groups
- **`jamf-pro-cli mobile-device-groups list`** - Returns the list of all mobile device groups filtered by site and permissions.


Required privileges:
> - Read Smart Mobile Device Groups
> - Read Static Mobile Device Groups
- **`jamf-pro-cli mobile-device-groups list-smart-groups`** - Get Smart Groups with pagination, sorting, and filtering support.


Required privilege: Read Smart Mobile Device Groups
- **`jamf-pro-cli mobile-device-groups list-static-groups`** - Get Static Groups with pagination, sorting, and filtering support.


Required privilege: Read Static Mobile Device Groups
- **`jamf-pro-cli mobile-device-groups update-smart-groups`** - Update a smart group with strict andOr enum validation.


Required privilege: Update Smart Mobile Device Groups
- **`jamf-pro-cli mobile-device-groups update-static-groups`** - Partially update a static group. Only provided fields will be updated.


Required privilege: Update Static Mobile Device Groups

### mobile_device_prestages

Manage mobile device prestages

- **`jamf-pro-cli mobile-device-prestages add-history-note`** - Adds mobile device prestage history object notes.

Required privilege: Update Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages create`** - Create a mobile device prestage.

Required privilege: Create Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages create-scope`** - Add device scope for a specific mobile device prestage.

Required privilege: Update Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages delete`** - Deletes a Mobile Device Prestage with the supplied id.

Required privilege: Delete Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages get`** - Retrieves a Mobile Device Prestage with the supplied id.

Required privilege: Read Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages get-history`** - Gets sorted and paged mobile device prestage history objects.

Required privilege: Read Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages get-scope`** - Get device scope for a specific mobile device prestage.

Required privilege: Read Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages get-syncs`** - Get all prestage sync states for a single prestage.

Required privilege: Read Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages get-syncs-latest`** - Get the latest sync state for a single prestage.

Required privilege: Read Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages list`** - Gets sorted and paged mobile device prestages.

Required privilege: Read Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages list-scope`** - Get all device scope for all mobile device prestages.

Required privilege: Read Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages list-syncs`** - Get all prestage sync states for all prestages.

Required privilege: Read Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages scope-delete-multiple`** - Remove device scope for a specific mobile device prestage.

Required privilege: Update Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages update`** - Updates a Mobile Device Prestage.

Required privilege: Update Mobile Device PreStage Enrollments
- **`jamf-pro-cli mobile-device-prestages update-scope`** - Replace device scope for a specific mobile device prestage.

Required privilege: Update Mobile Device PreStage Enrollments

### mobile_devices

Manage mobile devices

- **`jamf-pro-cli mobile-devices erase`** - Erase a Mobile Device.

Required privilege: Send Mobile Device Remote Wipe Command
- **`jamf-pro-cli mobile-devices get`** - Get MobileDevice.

Required privilege: Read Mobile Devices
- **`jamf-pro-cli mobile-devices get-detail`** - Get MobileDevice.

Required privilege: Read Mobile Devices
- **`jamf-pro-cli mobile-devices get-paired-devices`** - Return paginated Mobile Device Inventory records of all paired devices for the device.

Required privilege: Read Mobile Devices
- **`jamf-pro-cli mobile-devices list`** - Gets Mobile Device objects.


Required privilege: Read Mobile Devices
- **`jamf-pro-cli mobile-devices list-detail`** - Return paginated Mobile Device Inventory records.

Required privilege: Read Mobile Devices
- **`jamf-pro-cli mobile-devices unmanage`** - Unmanage a Mobile Device.

Required privilege: Unmanage Mobile Devices
- **`jamf-pro-cli mobile-devices update`** - Updates fields on a mobile device that are allowed to be modified by users.

Required privilege: Update Mobile Devices

### network_segments

Manage network segments

- **`jamf-pro-cli network-segments get`** - Finds network segments by ID
- **`jamf-pro-cli network-segments list`** - Finds all network segments

### packages

Manage packages

- **`jamf-pro-cli packages add-history-note`** - Adds specified Package history object notes.


Required privilege: Update Packages
- **`jamf-pro-cli packages create`** - Create package.

Required privilege: Create Packages
- **`jamf-pro-cli packages create-manifest`** - Add a manifest to a package.

Required privileges:
> - Update Packages
> - Read Packages
- **`jamf-pro-cli packages delete`** - Removes specified package.


Required privilege: Delete Packages
- **`jamf-pro-cli packages delete-manifest`** - Delete the manifest file for a specified package.


Required privileges:
> - Update Packages
> - Read Packages
- **`jamf-pro-cli packages delete-multiple`** - IDs of the packages to be deleted.

Required privilege: Delete Packages
- **`jamf-pro-cli packages get`** - Gets specified Package object.


Required privilege: Read Packages
- **`jamf-pro-cli packages get-history`** - Gets specified Package history object.


Required privilege: Read Packages
- **`jamf-pro-cli packages list`** - Retrieves packages.

Required privilege: Read Packages
- **`jamf-pro-cli packages update`** - Update specified package object.


Required privilege: Update Packages
- **`jamf-pro-cli packages upload`** - Uploads a package.

Required privileges:
> - Update Packages
> - Read Packages

### patch_management

Manage patch management

- **`jamf-pro-cli patch-management`** - Accept Patch Management disclaimer.

Required privilege: Update Patch Management Software Titles

### patch_policies

Manage patch policies

- **`jamf-pro-cli patch-policies create-dashboard`** - Adds a patch policy to the dashboard.

Required privilege: Read Patch Policies
- **`jamf-pro-cli patch-policies delete-dashboard`** - Removes a patch policy from the dashboard.

Required privilege: Read Patch Policies
- **`jamf-pro-cli patch-policies get-dashboard`** - Returns whether or not the requested patch policy is on the dashboard.

Required privilege: Read Patch Policies
- **`jamf-pro-cli patch-policies list`** - Retrieves a list of patch policies.

Required privilege: Read Patch Policies
- **`jamf-pro-cli patch-policies list-policy-details`** - Retrieves a list of patch policies.

Required privilege: Read Patch Policies

### patch_policy_logs

Manage patch policy logs

- **`jamf-pro-cli patch-policy-logs get-device`** - Retrieves a single Patch Policy Log.

Required privilege: Read Patch Policies
- **`jamf-pro-cli patch-policy-logs get-device-details`** - Return attempt details for a specific log.

Required privilege: Read Patch Policies
- **`jamf-pro-cli patch-policy-logs get-eligible-retry-count`** - return the count of the patch policy logs for the patch policy id that  are eligible for a retry attempt.

Required privilege: Read Patch Policies
- **`jamf-pro-cli patch-policy-logs list`** - Retrieves Patch Policy Logs.

Required privilege: Read Patch Policies
- **`jamf-pro-cli patch-policy-logs retry`** - Send retry attempts for specific devices.

Required privilege: Update Patch Policies
- **`jamf-pro-cli patch-policy-logs retry-all`** - Send retry attempts for all devices.

Required privilege: Update Patch Policies

### patch_software_title_configurations

Manage patch software title configurations

- **`jamf-pro-cli patch-software-title-configurations add-history-note`** - Adds Patch Software Title Configuration history object notes.

Required privilege: Update Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations create`** - Creates Patch Software Title Configurations using sToken.

Required privilege: Create Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations create-dashboard`** - Adds a software title configuration to the dashboard.

Required privilege: Read Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations delete`** - Deletes Patch Software Title Configurations with the supplied id.

Required privilege: Delete Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations delete-dashboard`** - Removes a software title configuration from the dashboard.

Required privilege: Read Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations get`** - Retrieves Patch Software Title Configurations with the supplied id.

Required privilege: Read Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations get-dashboard`** - Returns whether or not the requested software title configuration is on the dashboard.

Required privilege: Read Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations get-definitions`** - Retrieves patch software title definitions with the supplied id.

Required privilege: Read Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations get-dependencies`** - Retrieve list of Patch Software Title Configuration Dependencies.

Required privilege: Read Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations get-export-report`** - Export Patch Reporting Data.

Required privilege: Read Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations get-extension-attributes`** - Retrieves software title extension attributes with the supplied id.

Required privilege: Read Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations get-history`** - Gets specified Patch Software Title Configuration history object.


Required privilege: Read Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations get-patch-report`** - Retrieve Patch Software Title Configuration Patch Report.

Required privilege: Read Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations get-patch-summary`** - Returns active patch summary.

Required privilege: Read Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations get-patch-summary-versions`** - Returns patch versions.

Required privilege: Read Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations list`** - Retrieves patch software title configurations.

Required privilege: Read Patch Management Software Titles
- **`jamf-pro-cli patch-software-title-configurations update`** - Updates Patch Software Title Configurations.

Required privilege: Update Patch Management Software Titles

### policies

Manage policies

- **`jamf-pro-cli policies create`** - Creates a new policy by ID. Pass id 0 so Jamf Pro assigns the next free ID; Jamf's own SDK creates Classic objects this way (resource_path="categories/id/0" in https://github.com/macadmins/jamf-pro-sdk-python/blob/main/src/jamf_pro_sdk/clients/classic_api.py).
- **`jamf-pro-cli policies delete`** - Deletes a policy by ID
- **`jamf-pro-cli policies get`** - Finds policies by ID
- **`jamf-pro-cli policies get-subset`** - Subset values can also be appended using an ampersand to return multiple subsets (e.g. /subsets/General&Scope)
- **`jamf-pro-cli policies list`** - Finds all policies
- **`jamf-pro-cli policies list-by-category`** - Category may be specified by id or name, or 'None' for policies with no category.
- **`jamf-pro-cli policies list-by-created-by`** - The value 'casper' refers to Casper Remote. The value 'jss' refers to policies created in the GUI or via the API.
- **`jamf-pro-cli policies update`** - Updates an existing policy by ID

### policies_preview

Manage policies preview

- **`jamf-pro-cli policies-preview list`** - Gets `Policy Properties` object.


Required privilege: Read Policies
- **`jamf-pro-cli policies-preview update`** - Update Policy Properties object.


Required privilege: Update Policies

### printers

Manage printers

- **`jamf-pro-cli printers get`** - Finds printers by ID
- **`jamf-pro-cli printers list`** - Finds all printers

### restricted_software

Manage restricted software

- **`jamf-pro-cli restricted-software get`** - Finds restricted software by ID
- **`jamf-pro-cli restricted-software list`** - Finds all restricted software

### scripts

Manage scripts

- **`jamf-pro-cli scripts add-history-note`** - Adds specified Script history object notes.


Required privilege: Update Scripts
- **`jamf-pro-cli scripts create`** - Creates a script.

Required privilege: Create Scripts
- **`jamf-pro-cli scripts delete`** - Deletes a script at the specified id.

Required privilege: Delete Scripts
- **`jamf-pro-cli scripts get`** - Retrieves a full script object.

Required privilege: Read Scripts
- **`jamf-pro-cli scripts get-download`** - Download a text file of the script contents.

Required privilege: Read Scripts
- **`jamf-pro-cli scripts get-history`** - Gets specified Script history object.


Required privilege: Read Scripts
- **`jamf-pro-cli scripts list`** - Search for sorted and paged scripts.

Required privilege: Read Scripts
- **`jamf-pro-cli scripts update`** - Replaces the script at the id with the supplied information.

Required privilege: Update Scripts

### sites

Manage sites

- **`jamf-pro-cli sites get-objects`** - Find site objects for Site ID, with the ability to filter out different object types and object IDs for the site ID.


Required privilege: Read Sites
- **`jamf-pro-cli sites list`** - Find all sites.


Required privilege: Read Sites

### smart_computer_groups_preview

Manage smart computer groups preview

- **`jamf-pro-cli smart-computer-groups-preview recalculate`** - Recalculates the smart group for the given id and then
returns the ids for the computers in the smart group.


Required privilege: Update Smart Computer Groups
- **`jamf-pro-cli smart-computer-groups-preview recalculate-smart-groups`** - Recalculates a smart group for the given id and then
returns the count of smart groups the computer falls into.


Required privilege: Update Smart Computer Groups

### smart_mobile_device_groups_preview

Manage smart mobile device groups preview

- **`jamf-pro-cli smart-mobile-device-groups-preview recalculate`** - recalculates a smart group for the given id and then
returns the ids for the devices in the smart group.


Required privilege: Update Smart Mobile Device Groups
- **`jamf-pro-cli smart-mobile-device-groups-preview recalculate-smart-groups`** - Recalculates all smart groups for the given device id and then
returns the count of smart groups the device falls into.


Required privilege: Update Smart Mobile Device Groups

### startup_status

Manage startup status

- **`jamf-pro-cli startup-status`** - Retrieves information about application startup. Current startup operation taking place (if any) and overall startup completion percentage.

### volume_purchasing_locations

Manage volume purchasing locations

- **`jamf-pro-cli volume-purchasing-locations add-history-note`** - Adds specified Volume Purchasing Location history object notes.


Required privilege: Update Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-locations create`** - Creates a Volume Purchasing Location using an sToken.

Required privilege: Create Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-locations delete`** - Deletes a Volume Purchasing Location with the supplied id.

Required privilege: Delete Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-locations get`** - Retrieves a Volume Purchasing Location with the supplied id.

Required privilege: Read Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-locations get-content`** - Retrieves the Volume Purchasing Content for the Volume Purchasing Location with the supplied id.

Required privilege: Read Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-locations get-history`** - Gets specified Volume Purchasing Location history object.


Required privilege: Read Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-locations list`** - Retrieves Volume Purchasing Locations.

Required privilege: Read Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-locations reclaim`** - Reclaims a Volume Purchasing Location with the supplied id.

Required privilege: Update Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-locations revoke-licenses`** - Revokes licenses for a Volume Purchasing Location with the supplied id. The licenses must be revokable - any asset whose licenses are irrevocable will not be revoked.

Required privilege: Update Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-locations update`** - Updates a Volume Purchasing Location.

Required privilege: Update Volume Purchasing Locations

### volume_purchasing_subscriptions

Manage volume purchasing subscriptions

- **`jamf-pro-cli volume-purchasing-subscriptions add-history-note`** - Adds Volume Purchasing Subscription history object notes.

Required privilege: Update Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-subscriptions create`** - Creates a Volume Purchasing Subscription.

Required privilege: Create Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-subscriptions delete`** - Deletes a Volume Purchasing Subscription with the supplied id.

Required privilege: Delete Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-subscriptions get`** - Retrieves a Volume Purchasing Subscription with the supplied id.

Required privilege: Read Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-subscriptions get-history`** - Gets specified Volume Purchasing Subscription history object.


Required privilege: Read Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-subscriptions list`** - Retrieves Volume Purchasing Subscriptions.

Required privilege: Read Volume Purchasing Locations
- **`jamf-pro-cli volume-purchasing-subscriptions update`** - Updates a Volume Purchasing Subscription.

Required privilege: Update Volume Purchasing Locations

### webhooks

Manage webhooks

- **`jamf-pro-cli webhooks get`** - Finds webhooks by ID
- **`jamf-pro-cli webhooks list`** - Finds all webhooks


### fleet


- **`jamf-pro-cli fleet find`** - Find which client's Jamf Pro instance has a Mac, iPad or iPhone, by serial, name, username or email, across every registered instance at once.

- **`jamf-pro-cli fleet posture`** - Per-client counts of stale check-ins, Macs below a macOS floor, FileVault off, firewall off and expiring MDM profiles, with Macs that inventory cannot fully judge reported as incomplete.

- **`jamf-pro-cli fleet patch`** - Patch titles behind latest and failing patch-policy installs (one patch_title row per client and title), plus failed managed OS updates (one os_updates row per client).

- **`jamf-pro-cli fleet policy-hygiene`** - Flags policies that are disabled, can never run, have no scope, are expired, uncategorized, or point at a missing script or package.

- **`jamf-pro-cli fleet health`** - ADE and VPP token expiry, failed enrollment syncs, health check and Jamf Pro version spread for every client.

- **`jamf-pro-cli fleet exec`** - Run any read-only Jamf Pro or Classic API GET on every client instance and get one merged table with an instance column; credential fields are redacted and credential endpoints refused.

- **`jamf-pro-cli fleet app`** - Which clients and computers have an installed application below a version floor.

- **`jamf-pro-cli fleet baseline`** - Compare every client instance against a golden instance: scripts and extension attributes by content; packages, policies, profiles and smart groups by name.


### instance


- **`jamf-pro-cli instance`** - Register each client's Jamf Pro instance once (server and API client id; its secret goes in JAMF_PRO_FLEET_SECRETS), then target it with --instance or report on all of them with the fleet commands.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`jamf-pro-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`jamf-pro-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`jamf-pro-cli learnings list`** - Inspect taught rows
- **`jamf-pro-cli learnings forget <query>`** - Undo a teach
- **`jamf-pro-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`jamf-pro-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`jamf-pro-cli teach-pattern`** - Install a query/resource template up front
- **`jamf-pro-cli teach-lookup`** - Add an entity mapping (e.g. a client nickname or a Mac model alias) for pattern substitution

Pass `--no-learn` or set `JAMF_PRO_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `jamf-pro-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
jamf-pro-cli account-groups list

# JSON for scripting and agents
jamf-pro-cli account-groups list --json
# Filter to specific fields
jamf-pro-cli account-groups list --json --select accessLevel,directoryGroupId,id

# Dry run  -  show the request without sending
jamf-pro-cli account-groups list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
jamf-pro-cli account-groups list --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

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

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `8` fleet problems found (`fleet health --fail-on`), `10` config error.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `JAMF_PRO_SERVER` resolves `{server}`

Base URL: `https://{server}`

## Health Check

```bash
jamf-pro-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `jamf-pro-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/jamf-pro-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\jamf-pro-cli\config.toml` on Windows; `--home`, `JAMF_PRO_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `JAMF_PRO_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `JAMF_PRO_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `JAMF_PRO_SERVER` | endpoint | Yes | Jamf Pro host, e.g. `acme.jamfcloud.com` or `jamf.example.com:8443`. |
| `JAMF_PRO_FLEET_SECRETS` | per_call | For `--instance`/`fleet` | Client secrets of registered instances, as `name=secret` entries separated by commas (`acme=SECRET1,globex=SECRET2`). |
| `JAMF_PRO_INSTANCE` | selector | No | Same as the global `--instance` flag. |
| `JAMF_PRO_CLIENT_ID` | per_call | Single instance | Client ID of the Jamf Pro API client (Settings > System > API roles and clients). Registered instances carry their own, set by `instance add --client-id`. |
| `JAMF_PRO_CLIENT_SECRET` | per_call | Single instance | Client secret of that API client. Registered instances read their `JAMF_PRO_FLEET_SECRETS` entry instead. |
| `JAMF_PRO_CONFIG` | path | No | Config file to use instead of the default under the CLI's home. |
| `JAMF_PRO_MCP_HTTP_TOKEN` | secret | For `--transport http` | Bearer token HTTP callers must present to the MCP server. |
| `JAMF_PRO_MCP_READ_ONLY` | flag | No | `1` exposes only read tools in the MCP server; kept by `--instance` pinning and fleet children. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `jamf-pro-cli doctor` reports `agentcookie: detected` and `jamf-pro-cli auth status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `jamf-pro-cli doctor` to check credentials
- Verify the environment variable is set: `echo $JAMF_PRO_CLIENT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Check the API client is enabled and its secret env var is exported: jamf-pro-cli instance list --json shows secret_set per instance
- **403 Forbidden on one command**  -  Add the missing privilege (named in the command's help, e.g. Read Computers) to the API role assigned to that API client
- **unknown instance "acme"**  -  Register it first: jamf-pro-cli instance add acme --server acme.jamfcloud.com --client-id <id>
- **A list stops at 100 rows**  -  Add --all to read every page, or set --page-size; fleet commands read whole collections, except fleet find (--limit per instance) and fleet policy-hygiene (--max-policies)
- **Classic policy create or update returns 400 or 409**  -  Send the policy as Classic API XML (root element <policy>) with --file policy.xml or --stdin; pass --id 0 to create. The vendor sample body is on https://developer.jamf.com/jamf-pro/reference/createpolicybyid
- **A fleet command returns an empty result**  -  Register instances first: jamf-pro-cli instance list --json must show them; fleet commands only read registered instances
- **search returns nothing**  -  search reads the local store only: run jamf-pro-cli sync --resources computer_inventory first, or use fleet find for a live lookup

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**terraform-provider-jamfpro**](https://github.com/deploymenttheory/terraform-provider-jamfpro)  -  Go (126 stars)
- [**jamf-pro-sdk-python**](https://github.com/macadmins/jamf-pro-sdk-python)  -  Python (69 stars)
- [**jamf-mcp-server**](https://github.com/dbankscard/jamf-mcp-server)  -  TypeScript (38 stars)
- [**go-api-sdk-jamfpro**](https://github.com/deploymenttheory/go-api-sdk-jamfpro)  -  Go (37 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
