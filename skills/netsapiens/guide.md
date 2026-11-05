# NetSapiens CLI

**Every NetSapiens v2 endpoint plus a reseller-wide fleet view: seat limits, offline phones, unrouted numbers and E911 gaps across every client at once.**

Works against any white-label NetSapiens platform (Crexendo, OITVOIP, PacketDial and others) with one API key. Sync your reseller's domains, users, devices and numbers into a local SQLite store, then run fleet audits such as fleet seats, fleet offline, fleet dids and fleet e911, or trace a number to its client and phones in one call.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `netsapiens-cli` and `netsapiens-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.ps1 | iex
   ```
3. Verify: `netsapiens-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=netsapiens). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/netsapiens --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/netsapiens --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `netsapiens-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the netsapiens skill from https://github.com/Servosity/msp-skills/tree/main/skills/netsapiens. The skill defines how its required CLI (`netsapiens-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=netsapiens).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `NETSAPIENS_API_KEY` and `NETSAPIENS_SERVER` (your provider's platform host, for example pbx.yourprovider.example) when Claude Desktop prompts you. ns-api.com is only the public sandbox.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/netsapiens/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "netsapiens": {
      "command": "netsapiens-mcp",
      "env": {
        "NETSAPIENS_SERVER": "<server>",
        "NETSAPIENS_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`netsapiens-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `NETSAPIENS_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `NETSAPIENS_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export NETSAPIENS_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
netsapiens-mcp --transport http
```

Set `NETSAPIENS_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Every tool not marked read-only is dropped, including the learning tools that write only this CLI's own local store (`teach`, `teach-playbook` and the other local-write commands); the read-only learning tools (`recall`, `learnings list`) stay.

## Authentication

NetSapiens runs on each provider's own host, so set two variables before anything else: export NETSAPIENS_SERVER=<your platform host, e.g. pbx.yourprovider.example> and export NETSAPIENS_API_KEY=<API key or OAuth2 access token from your provider>. Without NETSAPIENS_SERVER the CLI talks to ns-api.com, which is only the public sandbox. NETSAPIENS_BASE_URL overrides the whole base URL (https://<host>/ns-api/v2) if your provider uses a different path. A reseller-scoped key sees every client domain; an office-manager key sees one domain. Keys can be read-only.

## Quick Start

```bash
# Confirm the CLI is installed (no network or key needed)
netsapiens-cli doctor --dry-run

# Shows the URL the CLI will call; if it names ns-api.com, set NETSAPIENS_SERVER to your provider's host first
netsapiens-cli apikeys my --dry-run

# Check your key's scope (Reseller, Office Manager) and whether it is read-only
netsapiens-cli apikeys my --agent

# Pull exactly the fleet set (domains plus each domain's users, devices, queues, attendants, billing and E911 addresses) into the local store
netsapiens-cli sync --resources domains,users,devices,callqueues,autoattendants,billing,domains_addresses,phonenumbers,phones

# See which clients are over or near their seat limits
netsapiens-cli fleet seats --agent

# Find phones that are not registered across every client
netsapiens-cli fleet offline --agent

```

## Unique Features

### Fleet audits across every tenant
- **`fleet seats`**  -  From the synced mirror (run sync first): each client domain's users, call queues, auto attendants and conferences against their limits, plus device, number, call and SMS counts, with over-limit and near-limit domains flagged and a flat true-up CSV.

  _Reach for this when asked which clients are over or near their seat limits or for monthly true-up numbers._

  ```bash
  netsapiens-cli fleet seats --agent
  ```
- **`fleet offline`**  -  From the synced mirror (run sync first): phones and softphones that are unregistered, expired, high-latency, never provisioned or not provisioned recently, sorted by client.

  _Use this for 'which phones are down' sweeps across all clients instead of checking one domain at a time._

  ```bash
  netsapiens-cli fleet offline --agent
  ```
- **`fleet dids`**  -  From the synced mirror (run sync first): where every phone number routes, flagging numbers whose target is missing for that routing type, disabled or parked numbers, unverifiable targets, and possible dead ends (a user with no registered phone and voicemail off; answer rules are not inspected).

  _Use this after onboarding or porting numbers to catch numbers that route nowhere._

  ```bash
  netsapiens-cli fleet dids --problems-only --agent
  ```
- **`fleet e911`**  -  From the synced mirror (run sync first): domains, users and devices with no emergency caller ID after inheritance, or with an emergency address ID that does not exist. A data-gap check to feed a Kari's Law / RAY BAUM's Act review, not a full compliance test.

  _Use this to find E911 data gaps across every client before a compliance review._

  ```bash
  netsapiens-cli fleet e911 --agent
  ```
- **`fleet changes`**  -  From the synced mirror: users, devices, numbers and domains added, removed or re-pointed since the baseline. Read-only; record or move the baseline with --rebaseline.

  _Use this for 'what changed across clients since the last check'; run with --rebaseline at each check to move the baseline._

  ```bash
  netsapiens-cli fleet changes --agent
  ```

### Ticket triage
- **`trace`**  -  From the synced mirror (run sync first): resolve one phone number, extension, MAC address or email to its client, routing, user, and the user's phones with their registration state.

  _Use this first on a support ticket that names only a number, extension or MAC._

  ```bash
  netsapiens-cli trace 6025550142 --agent
  ```

### Call-handling reviews
- **`fleet missed`**  -  Live (needs the API key): unanswered inbound calls per user and client over a window, read from CDRs; adds names and registered-phone counts when users and devices are synced.

  _Use this for weekly client reviews of missed direct calls._

  ```bash
  netsapiens-cli fleet missed --window 7d --agent
  ```
- **`fleet queues`**  -  Live (needs the API key): call queues across clients ranked by abandonment rate, with service level, available agents and agents whose phones are not registered. Checks up to --max-domains (25) domains and reports when it was capped.

  _Use this when asked which clients' queues are dropping callers and why._

  ```bash
  netsapiens-cli fleet queues --window 7d --agent
  ```

## Recipes

### Seat true-up for every client

```bash
netsapiens-cli fleet seats --csv
```

Writes one flat row per domain with a count, limit and status column per metric plus device, number, call and SMS usage for invoice reconciliation.

### Who owns this number?

```bash
netsapiens-cli trace 6025550142 --agent
```

Resolves a DID to its client, routing target, user, and the user's phones with their registration state.

### Lean user list for an agent

```bash
netsapiens-cli domains users list --domain acme.example --all --agent --select user,name-first-name,name-last-name,email,user-scope
```

Users return dozens of fields each; --select keeps only the ones the agent needs, and --all pages past the 100-row default.

### Numbers that route nowhere

```bash
netsapiens-cli fleet dids --problems-only --agent
```

Lists DIDs whose destination user, queue or auto attendant does not exist or cannot be reached.

### E911 data-gap pass

```bash
netsapiens-cli fleet e911 --agent
```

Flags domains, users and devices with no emergency caller ID after inheritance, or with an emergency address ID that does not exist.

## Usage

Run `netsapiens-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `NETSAPIENS_CONFIG_DIR`, `NETSAPIENS_DATA_DIR`, `NETSAPIENS_STATE_DIR`, or `NETSAPIENS_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `NETSAPIENS_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export NETSAPIENS_HOME=/srv/netsapiens
netsapiens-cli doctor
```

On Windows (PowerShell):

```powershell
$env:NETSAPIENS_HOME = "C:\ProgramData\netsapiens"
netsapiens-cli doctor
```

Under `NETSAPIENS_HOME=/srv/netsapiens`, the four dirs resolve to `/srv/netsapiens/config`, `/srv/netsapiens/data`, `/srv/netsapiens/state`, and `/srv/netsapiens/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:NETSAPIENS_HOME = "C:\srv\netsapiens"
netsapiens-cli doctor
```

Under `$env:NETSAPIENS_HOME = "C:\srv\netsapiens"`, the four dirs resolve to `C:\srv\netsapiens\config`, `C:\srv\netsapiens\data`, `C:\srv\netsapiens\state`, and `C:\srv\netsapiens\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "netsapiens": {
      "command": "netsapiens-mcp",
      "env": {
        "NETSAPIENS_HOME": "/srv/netsapiens"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `NETSAPIENS_DATA_DIR` overrides an explicit `--home` for that kind. Use `NETSAPIENS_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `NETSAPIENS_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `netsapiens-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are cached for five minutes under `http/` in the cache directory. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `netsapiens-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `netsapiens-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `NETSAPIENS_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `NETSAPIENS_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### accesslog

Manage accesslog

- **`netsapiens-cli accesslog`** - Read Accesslog

### apikeys

Manage apikeys

- **`netsapiens-cli apikeys create`** - This is a limited action and will require special access to create API keys.
- **`netsapiens-cli apikeys delete`** - This is a limited action and will require special access to revoke API keys. Revoking the apikey will remove the key from the DB and any Cache stoping access immediately.
- **`netsapiens-cli apikeys get`** - This action will show apikeys that have been generated by your current APIkey and that you have access to update or revoke as needed. 

> Note: # and anything after is NOT needed, its just to allow multiple examples for the same path/method.
- **`netsapiens-cli apikeys list`** - This action will show apikeys that have been generated by your current APIkey and that you have access to update or revoke as needed. 

> Note: # and anything after is NOT needed, its just to allow multiple examples for the same path/method.
- **`netsapiens-cli apikeys my`** - This parameter-less action will return the information on the API key being used for the request giving confirmation on access levels allowed.
- **`netsapiens-cli apikeys update`** - This is a limited action and will require special access to create API keys. The Update is even more limited only allowing the change of the description and IP restrictions. You will not be able to change the scope, access rights or any premissions. A new apikey would need to be created in those cases.

### auditlog

Manage auditlog

- **`netsapiens-cli auditlog`** - Read Audit Log

### auth-code

Manage auth code

- **`netsapiens-cli auth-code`** - Get Access Token From Auth Code

### backup

Manage backup

- **`netsapiens-cli backup`** - Request a Full System backup

### calls

Manage calls

- **`netsapiens-cli calls`** - Report Active Calls

### cdrs

Manage cdrs

- **`netsapiens-cli cdrs count`** - Count CDRs and SUM minutes
- **`netsapiens-cli cdrs list`** - Read CDRs

### certificates

Manage certificates

- **`netsapiens-cli certificates create`** - Create SSL certificate for CertManager
- **`netsapiens-cli certificates delete`** - Delete SSL certificate for CertManager
- **`netsapiens-cli certificates get`** - Read SSL certificate by Common Name
- **`netsapiens-cli certificates list`** - Read SSL certificates for CertManager
- **`netsapiens-cli certificates update`** - Update SSL certificate for CertManager

### config-definitions

Manage config definitions

- **`netsapiens-cli config-definitions create`** - Create a Configuration Definition
- **`netsapiens-cli config-definitions delete`** - This will delete the configuration definition and all configurations that match the name.
- **`netsapiens-cli config-definitions get`** - Read a Specific Configuration Definition
- **`netsapiens-cli config-definitions list`** - Read all Configuration Definitions
- **`netsapiens-cli config-definitions update`** - Update a Configuration Definition

### configurations

Manage configurations

- **`netsapiens-cli configurations count`** - Count Configurations
- **`netsapiens-cli configurations create`** - Create a Configuration
- **`netsapiens-cli configurations delete`** - Delete a Configuration
- **`netsapiens-cli configurations get`** - Read a Specific Configuration
- **`netsapiens-cli configurations list`** - Read all Configurations
- **`netsapiens-cli configurations update`** - Update a configurution, must already exist or will get a 404.

### connections

Manage connections

- **`netsapiens-cli connections count`** - Count All Conections
- **`netsapiens-cli connections create`** - Create a Connection
- **`netsapiens-cli connections list`** - Get All Connections

### deviceprofiles

Manage deviceprofiles

- **`netsapiens-cli deviceprofiles count`** - Count Device Profiles
- **`netsapiens-cli deviceprofiles get-by-model`** - Get specific Device Profile for Model of Phone
- **`netsapiens-cli deviceprofiles list`** - Read Device Profiles

### dialplans

Manage dialplans

- **`netsapiens-cli dialplans create`** - Create Dialplan Global
- **`netsapiens-cli dialplans list`** - Read Dialplans

### dialpolicy

Manage dialpolicy

- **`netsapiens-cli dialpolicy create`** - Create Dialpolicy Table
- **`netsapiens-cli dialpolicy delete`** - Delete Dialpolicy
- **`netsapiens-cli dialpolicy get`** - Read Specific Policy
- **`netsapiens-cli dialpolicy list`** - Read Policies
- **`netsapiens-cli dialpolicy update`** - Update Dialpolicy

### domains

Manage domains

- **`netsapiens-cli domains count`** - This API is the same for both Super User and Reseller. If using Reseller scopped access there territory/reseller will be used from the access rights for the filter.
- **`netsapiens-cli domains create`** - This API will allow a new domain to be created. Version 2 of the api will assist in creating some resources that previously were not auto created on api domain create inlcuding creating a "domain" subscriber  for the owner to hold the defaults and creating a dialplan with a name that matches the domain that is chained up to a system wide table.
- **`netsapiens-cli domains delete`** - Delete a Domain
- **`netsapiens-cli domains exists`** - Check if Domain Exists
- **`netsapiens-cli domains get`** - Get Specific Domain
- **`netsapiens-cli domains list`** - This API is the same for both Super User and Reseller. If using Reseller scopped access there territory/reseller will be used from the access rights for the filter.
- **`netsapiens-cli domains my`** - Get My Domain Info
- **`netsapiens-cli domains my-answerrules`** - Read Answerrules for my  User
- **`netsapiens-cli domains my-contacts`** - Get My Contacts
- **`netsapiens-cli domains my-user`** - Get My User
- **`netsapiens-cli domains update`** - Update a Domain

### firebase

Manage firebase

- **`netsapiens-cli firebase create`** - Add firebase service account
- **`netsapiens-cli firebase list`** - Read firebase service accounts

### holidays

Manage holidays

- **`netsapiens-cli holidays get-by-year`** - In order to create holidays in a Holiday timeframe or a Custom timeframe, it is necessary to supply a series of parameters for each holiday that can be obtained via this endpoint.
- **`netsapiens-cli holidays get-by-year-country`** - Read Holiday Information by Country and Region
- **`netsapiens-cli holidays list`** - Retrieve a list of supported country codes to be used when reading holiday information.
- **`netsapiens-cli holidays list-regions`** - Retrieve a list of supported region codes by country, to be used when reading holiday information.

### images

Manage images

- **`netsapiens-cli images create-with-filename`** - Create Image from Upload (JSON + Base64 File)
- **`netsapiens-cli images delete`** - Delete an image file
- **`netsapiens-cli images get`** - Read an image file
- **`netsapiens-cli images update`** - Update Image from Upload (Multipart/Mixed Post)

### insight

Manage insight

- **`netsapiens-cli insight`** - Query Data from iNSight

### jwt

Manage jwt

- **`netsapiens-cli jwt create`** - This API will accept username and password and generate a JWT token.
- **`netsapiens-cli jwt delete`** - This API requires a valid JWT and will revoke it by JTI of the current Token. It will then prevent its use moving forward.
- **`netsapiens-cli jwt delete-jti`** - > Note: # and anything after is NOT needed, its just to allow multiple examples for the same path/method.
- **`netsapiens-cli jwt delete-uid`** - Revoke JWT(s) by UID (user@domain)
- **`netsapiens-cli jwt list`** - Read Current JWT

### meetings

Manage meetings

- **`netsapiens-cli meetings <meeting_registration_id>`** - Register Meeting

### netsapiens-version

Manage netsapiens version

- **`netsapiens-cli netsapiens-version`** - Read API Version

### phonenumbers

Manage phonenumbers

- **`netsapiens-cli phonenumbers count`** - This path will give you all Phonenumbers (DIDs) that are accessable based on the access rights of the Access Token or API Key used to make the requests. Super User or Reseller both supported, but for per domain lookups you should use /domains/{domain}/phonenumbers.
- **`netsapiens-cli phonenumbers list`** - This path will give you all Phonenumbers (DIDs) that are accessable based on the access rights of the Access Token or API Key used to make the requests. Super User or Reseller both supported, but for per domain lookups you should use /domains/{domain}/phonenumbers.

### phones

Manage phones

- **`netsapiens-cli phones count`** - Count Mac Addresses
- **`netsapiens-cli phones create`** - Add MAC address
- **`netsapiens-cli phones delete`** - Remove MAC address
- **`netsapiens-cli phones get`** - Read Specific Mac Address
- **`netsapiens-cli phones get-servers`** - Read Provisionable Server Details
- **`netsapiens-cli phones list`** - Read Mac Addresses
- **`netsapiens-cli phones list-models`** - Get list of Supported/Provisionable Models
- **`netsapiens-cli phones list-servers`** - Get list of Provisionable Server Profiles
- **`netsapiens-cli phones update`** - Update MAC address

### resellers

Manage resellers

- **`netsapiens-cli resellers count`** - Count Resellers
- **`netsapiens-cli resellers create`** - Create Reseller
- **`netsapiens-cli resellers delete`** - Delete Reseller
- **`netsapiens-cli resellers get`** - Get Specific Reseller
- **`netsapiens-cli resellers list`** - Get Resellers
- **`netsapiens-cli resellers update`** - Update Reseller

### restore

Manage restore

- **`netsapiens-cli restore list`** - Read Available Restore Points
- **`netsapiens-cli restore update`** - Restore a Specifc Domain Backup

### routecon

Manage routecon

- **`netsapiens-cli routecon`** - Count All Route Connections

### routes

Manage routes

- **`netsapiens-cli routes count`** - Count All Routes
- **`netsapiens-cli routes create`** - Create a Route
- **`netsapiens-cli routes delete`** - Delete A Specific Route
- **`netsapiens-cli routes list`** - Read Routes
- **`netsapiens-cli routes update`** - Update A Specific Route

### sipflow

Manage sipflow

- **`netsapiens-cli sipflow`** - Get Call Trace (SIPFlow) For Call

### smsblocks

Manage smsblocks

- **`netsapiens-cli smsblocks`** - Read SMS Blocks for all Domains

### smsnumbers

Manage smsnumbers

- **`netsapiens-cli smsnumbers`** - Get All SMS Numbers for System

### sso-enroll

Manage sso enroll

- **`netsapiens-cli sso-enroll`** - SSO Enroll

### subscriptions

Manage subscriptions

- **`netsapiens-cli subscriptions create`** - Create a Event Subscription
- **`netsapiens-cli subscriptions delete`** - Delete a subscription
- **`netsapiens-cli subscriptions get`** - Read Event Subscription By Id
- **`netsapiens-cli subscriptions list`** - Read Event Subscriptions
- **`netsapiens-cli subscriptions update`** - Update an Event Subscription

### templates

Manage templates

- **`netsapiens-cli templates create-with-filename`** - Create Template from Upload (JSON + Base64 File)
- **`netsapiens-cli templates delete`** - Delete a template file
- **`netsapiens-cli templates get`** - Read a template file
- **`netsapiens-cli templates update`** - Update Template from Upload (JSON + Base64 File)

### tokens

Manage tokens

- **`netsapiens-cli tokens`** - This API will accept username and password along with a passcode generated via a authenticator application and generate a Access token

### video

Manage video

- **`netsapiens-cli video`** - Read Iotum Video Domain Resellers


### fleet


- **`netsapiens-cli fleet seats`** - From the synced mirror (run sync first): each client domain's users, call queues, auto attendants and conferences against their limits, plus device, number, call and SMS counts, with over-limit and near-limit domains flagged and a flat true-up CSV.

- **`netsapiens-cli fleet offline`** - From the synced mirror (run sync first): phones and softphones that are unregistered, expired, high-latency, never provisioned or not provisioned recently, sorted by client.

- **`netsapiens-cli fleet dids`** - From the synced mirror (run sync first): where every phone number routes, flagging numbers whose target is missing for that routing type, disabled or parked numbers, unverifiable targets, and possible dead ends (a user with no registered phone and voicemail off; answer rules are not inspected).

- **`netsapiens-cli fleet e911`** - From the synced mirror (run sync first): domains, users and devices with no emergency caller ID after inheritance, or with an emergency address ID that does not exist. A data-gap check to feed a Kari's Law / RAY BAUM's Act review, not a full compliance test.

- **`netsapiens-cli fleet missed`** - Live (needs the API key): unanswered inbound calls per user and client over a window, read from CDRs; adds names and registered-phone counts when users and devices are synced.

- **`netsapiens-cli fleet queues`** - Live (needs the API key): call queues across clients ranked by abandonment rate, with service level, available agents and agents whose phones are not registered. Checks up to --max-domains (25) domains and reports when it was capped.

- **`netsapiens-cli fleet changes`** - From the synced mirror: users, devices, numbers and domains added, removed or re-pointed since the baseline. Read-only; record or move the baseline with --rebaseline.


### trace


- **`netsapiens-cli trace`** - From the synced mirror (run sync first): resolve one phone number, extension, MAC address or email to its client, routing, user, and the user's phones with their registration state.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`netsapiens-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`netsapiens-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`netsapiens-cli learnings list`** - Inspect taught rows
- **`netsapiens-cli learnings forget <query>`** - Undo a teach
- **`netsapiens-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`netsapiens-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`netsapiens-cli teach-pattern`** - Install a query/resource template up front
- **`netsapiens-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `NETSAPIENS_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `netsapiens-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
netsapiens-cli accesslog

# JSON for scripting and agents
netsapiens-cli accesslog --json
# Filter to specific fields
netsapiens-cli accesslog --json --select action-type,auditlog-details,by-client

# Dry run  -  show the request without sending
netsapiens-cli accesslog --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
netsapiens-cli accesslog --agent
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

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `10` config error.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `NETSAPIENS_SERVER` resolves `{server}`

Base URL: `https://{server}/ns-api/v2`

## Health Check

```bash
netsapiens-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `netsapiens-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/netsapiens-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\netsapiens-cli\config.toml` on Windows; `--home`, `NETSAPIENS_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `NETSAPIENS_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `NETSAPIENS_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `NETSAPIENS_SERVER` | endpoint | Yes |  |
| `NETSAPIENS_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `netsapiens-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `netsapiens-cli doctor` to check credentials
- Check the variable is set without printing it: `[ -n "$NETSAPIENS_API_KEY" ] && echo set || echo missing`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized: the request is missing an access token**  -  export NETSAPIENS_API_KEY=<key from your provider>, then run netsapiens-cli doctor
- **Connection errors or 404 on every command**  -  export NETSAPIENS_SERVER=<your platform host>; the default ns-api.com is only the sandbox
- **A list stops at exactly 100 rows**  -  domains list, domains users list, phonenumbers list and cdrs list return 100 rows by default: add --all (or --limit up to 1000 with --start to page)
- **403 Forbidden on writes**  -  Your key is read-only or office-manager scoped; check with netsapiens-cli apikeys my
- **CDR commands fail with a missing datetime error**  -  Pass --datetime-start and --datetime-end; CDR reads require a window
- **A full sync makes thousands of calls on a large reseller**  -  Name the resources you need; an explicit list syncs exactly those: netsapiens-cli sync --resources domains,users,devices,callqueues,autoattendants,billing,domains_addresses,phonenumbers,phones
- **fleet seats, offline, dids, e911 or trace return nothing**  -  They read the local mirror; run netsapiens-cli sync --resources domains,users,devices,callqueues,autoattendants,billing,domains_addresses,phonenumbers,phones first
