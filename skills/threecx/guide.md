# 3CX CLI

**Run every client's 3CX phone system from one CLI: trunk, version, licence and backup health across the whole fleet, plus the full Configuration API on any single PBX.**

Register each client PBX once with its own service-principal credential, then 'fleet health', 'fleet trunks', 'fleet licences', 'fleet versions' and 'fleet backups' answer for every phone system at once, and 'fleet changes' shows what moved since the last run. Any of the generated XAPI commands targets one PBX with --pbx, and 'fleet exec' fans a read-only query across all of them.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `threecx-cli` and `threecx-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

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

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=threecx). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/threecx --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/threecx --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `threecx-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the threecx skill from https://github.com/Servosity/msp-skills/tree/main/skills/threecx. The skill defines how its required CLI (`threecx-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=threecx).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `THREECX_CLIENT_ID` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threecx/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/threecx/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "threecx": {
      "command": "threecx-mcp",
      "env": {
        "THREECX_FQDN": "<fqdn>",
        "THREECX_CLIENT_ID": "<dn-number>",
        "THREECX_CLIENT_SECRET": "<your-api-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`threecx-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `THREECX_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `THREECX_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export THREECX_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
threecx-mcp --transport http
```

Set `THREECX_MCP_READ_ONLY=1` (exactly `1`) to drop mirrored commands that are not marked read-only and make the raw execute tool refuse state-changing calls. Commands marked local-write, which write only this CLI's own learn store, stay available.

## Authentication

Each 3CX phone system has its own Configuration API (XAPI) credential. In the PBX admin console go to Integrations > API, add a client, tick '3CX Configuration API Access', and give it a department and the least role that works: System Admin covers the fleet reports; only reports and call history need System Owner. The client ID is the route point's DN number; the API key is shown once. The CLI exchanges them at https://<pbx-fqdn>/connect/token for a 60-minute bearer token and re-mints automatically. For a fleet, run 'fleet add <name> --fqdn <host> --client-id <dn>' once per PBX and export each key as THREECX_<NAME>_CLIENT_SECRET (or <PREFIX>_CLIENT_SECRET with --env-prefix <PREFIX>); the fleet registry never stores keys, and 'fleet list' shows which variables are missing. For a single PBX without the registry, set THREECX_FQDN, THREECX_CLIENT_ID and THREECX_CLIENT_SECRET. Check one system with 'doctor --pbx <name>'. The Configuration API requires an 8SC or higher licence. Do not pass --client-secret to 'auth login': it saves the key to credentials.toml; use the env-var paths instead.

## Quick Start

```bash
# Confirm the CLI is installed and see which credentials it can find.
threecx-cli doctor --dry-run

# Register one client phone system; repeat per PBX. The secret stays in the named env var.
threecx-cli fleet add acme --fqdn acme.3cx.us --client-id 900 --env-prefix THREECX_ACME

# The morning check: one pass/warn/fail row per PBX.
threecx-cli fleet health --agent

# Drill into any PBX with a trunk problem.
threecx-cli fleet trunks --offline

# Run any Configuration API command against one registered PBX.
threecx-cli users list --pbx acme --json

```

## Managing a fleet

- **Registry.** `fleet add|list|remove` keep one entry per phone system in `fleet.json` in the config directory (`THREECX_HOME/config/fleet.json` when relocated). An entry holds the name, FQDN (`host` or `host:port`), client ID, optional tags, and the *name* of the environment variable that holds the API key. The key itself is never written to disk; `fleet list` shows whether each variable is set.
- **One PBX at a time.** `--pbx <name>` (or `THREECX_PBX=<name>`) points any command at that phone system. Each PBX gets its own home under `<config>/pbx/<name>/<identity>/`, where the identity is derived from its FQDN and client ID, so tokens, the local mirror and caches never mix between clients, and re-pointing a PBX at another host starts from a clean home. `--pbx` cannot be combined with `--home`, and it ignores `THREECX_FQDN`, `THREECX_BASE_URL` and the per-kind directory variables so a stray shell setting cannot redirect a client's credential.
- **Whole fleet.** The `fleet` reports and `audit` read every registered PBX in parallel (`--concurrency`, default 8), or a subset with `--only a,b` or `--tag gold` (an unknown name, or a tag no PBX carries, exits 2 and lists what is registered). A PBX that cannot be read (unreachable, missing secret, 401, 403 without System Owner, below the 8SC licence) becomes a row with an `error` field; the rest of the run continues.
- **Alerting.** `fleet health --fail-on warn|fail` and `fleet trunks --fail-on-offline` exit **8** when they find a problem, after printing the full report.
- **History.** `fleet health` and `fleet trunks` save each run to `fleet-history.json` next to the registry (last 400 runs; `--no-save` skips it). `fleet changes` compares saved runs.
- **Removing a PBX.** `fleet remove <name>` deletes its entry and every home for that name, including cached tokens. `fleet add` over an existing name removes homes left by its previous host or client ID.

## Credential safety

- **Nothing prints a credential.** Client secrets (`THREECX_CLIENT_SECRET`, every registered `<PREFIX>_CLIENT_SECRET`), access tokens the CLI mints, keys a minting function returns (`generate-api-key`), and passwords sent in a request are held for the whole run. Credential-named fields in a response (passwords, PINs, keys, tokens, provisioning links) print `[redacted]`, so a listing stays readable. Any other output, error, MCP result, exported or delivered file, local-store row or outbound request that repeats a held value, in any common encoding or split across lines, is withheld whole and replaced by `[credkit: withheld from <where>: it carried a credential this program holds]`. A `--stdin` body counts as a credential whole, so a preview or a result that repeats a piece of it prints that notice instead; confirm the change with a read.
- **A command that mints a key prints that notice instead of the key.** `conference-settings generate-api-key` and the other minting functions are held like any other credential, so read a minted key in the 3CX admin console. A `--dry-run` preview of a request that carries a password prints the same notice.
- **Requests stay on the phone system.** Every request, the token exchange included, goes only to the configured `https://<fqdn>`; a redirect to another host is refused before anything is sent there.
- **No credential on disk** except the private (0600) credential store; GET responses are not cached on disk.
- **MCP writes need confirmation.** `threecx_execute` and mirrored commands that can change a phone system return a preview until called again with `confirm: true` (a boolean).

## Unique Features

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

## Usage

Run `threecx-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `THREECX_CONFIG_DIR`, `THREECX_DATA_DIR`, `THREECX_STATE_DIR`, or `THREECX_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `THREECX_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export THREECX_HOME=/srv/threecx
threecx-cli doctor
```

On Windows (PowerShell):

```powershell
$env:THREECX_HOME = "C:\ProgramData\threecx"
threecx-cli doctor
```

Under `THREECX_HOME=/srv/threecx`, the four dirs resolve to `/srv/threecx/config`, `/srv/threecx/data`, `/srv/threecx/state`, and `/srv/threecx/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:THREECX_HOME = "C:\srv\threecx"
threecx-cli doctor
```

Under `$env:THREECX_HOME = "C:\srv\threecx"`, the four dirs resolve to `C:\srv\threecx\config`, `C:\srv\threecx\data`, `C:\srv\threecx\state`, and `C:\srv\threecx\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

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

Precedence matters in fleets: an ambient per-kind variable such as `THREECX_DATA_DIR` overrides an explicit `--home` for that kind. Use `THREECX_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `THREECX_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `threecx-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are not cached on disk (XAPI bodies carry SIP passwords and provisioning links), and credential-named fields are redacted before a row reaches the store. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `threecx-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `threecx-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `THREECX_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `THREECX_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### active-calls

Manage active calls

- **`threecx-cli active-calls`** - Get entities from ActiveCalls

### active-calls-id

Manage active calls id

- **`threecx-cli active-calls-id <Id>`** - Invoke action DropCall

### activity-log

Manage activity log

- **`threecx-cli activity-log get-filter`** - Invoke function GetFilter
- **`threecx-cli activity-log get-logs`** - Invoke function GetLogs
- **`threecx-cli activity-log purge-logs`** - Invoke action PurgeLogs

### aisettings

Manage aisettings

- **`threecx-cli aisettings add-vector-store-files`** - Invoke action AddVectorStoreFiles
- **`threecx-cli aisettings batch-delete-files`** - Invoke action BatchDeleteFiles
- **`threecx-cli aisettings create-vector-store`** - Invoke action CreateVectorStore
- **`threecx-cli aisettings delete-open-ai-file`** - Invoke action DeleteOpenAiFile
- **`threecx-cli aisettings delete-vector-store`** - Invoke action DeleteVectorStore
- **`threecx-cli aisettings delete-vector-store-file`** - Invoke action DeleteVectorStoreFile
- **`threecx-cli aisettings get`** - Get AISettings
- **`threecx-cli aisettings get-airesources`** - Invoke function GetAIResources
- **`threecx-cli aisettings get-aitemplate-contents`** - Invoke function GetAITemplateContents
- **`threecx-cli aisettings get-current-airesources`** - Invoke function GetCurrentAIResources
- **`threecx-cli aisettings get-vector-store`** - Invoke function GetVectorStore
- **`threecx-cli aisettings get-vector-store-files`** - Invoke function GetVectorStoreFiles
- **`threecx-cli aisettings get-vector-stores`** - Invoke function GetVectorStores
- **`threecx-cli aisettings update`** - Update AISettings
- **`threecx-cli aisettings update-vector-store`** - Invoke action UpdateVectorStore

### amazon-integration-settings

Manage amazon integration settings

- **`threecx-cli amazon-integration-settings generate-iam-policy-file`** - Invoke function GenerateIamPolicyFile
- **`threecx-cli amazon-integration-settings get`** - Get AmazonIntegrationSettings
- **`threecx-cli amazon-integration-settings update`** - Update AmazonIntegrationSettings

### anti-hacking-settings

Manage anti hacking settings

- **`threecx-cli anti-hacking-settings get`** - Get AntiHackingSettings
- **`threecx-cli anti-hacking-settings update`** - Update AntiHackingSettings

### backups

Manage backups

- **`threecx-cli backups backup`** - Invoke action Backup
- **`threecx-cli backups get-can-create`** - Invoke function GetCanCreateBackup
- **`threecx-cli backups get-failover-scripts`** - Invoke function GetFailoverScripts
- **`threecx-cli backups get-failover-settings`** - Invoke function GetBackupFailoverSettings
- **`threecx-cli backups get-local-size`** - Invoke function GetLocalBackupSize
- **`threecx-cli backups get-repository-settings`** - Invoke function GetBackupRepositorySettings
- **`threecx-cli backups get-restore-settings`** - Invoke function GetRestoreSettings
- **`threecx-cli backups get-settings`** - Invoke function GetBackupSettings
- **`threecx-cli backups get-sftp-host-key`** - Invoke action GetSftpHostKey
- **`threecx-cli backups list`** - Get entities from Backups
- **`threecx-cli backups purge-local`** - Invoke action PurgeLocalBackups
- **`threecx-cli backups set-failover-settings`** - Invoke action SetBackupFailoverSettings
- **`threecx-cli backups set-repository-settings`** - Invoke action SetBackupRepositorySettings
- **`threecx-cli backups set-restore-settings`** - Invoke action SetRestoreSettings
- **`threecx-cli backups set-settings`** - Invoke action SetBackupSettings

### backups-file-name

Manage backups file name

- **`threecx-cli backups-file-name delete-backups`** - Delete entity from Backups
- **`threecx-cli backups-file-name get-backup-extras`** - Invoke function GetBackupExtras
- **`threecx-cli backups-file-name restore`** - Invoke action Restore

### black-list-numbers

Manage black list numbers

- **`threecx-cli black-list-numbers bulk-numbers-delete`** - Invoke action BulkNumbersDelete
- **`threecx-cli black-list-numbers create`** - Add new entity to BlackListNumbers
- **`threecx-cli black-list-numbers list`** - Get entities from BlackListNumbers

### black-list-numbers-id

Manage black list numbers id

- **`threecx-cli black-list-numbers-id delete-black-list-number`** - Delete entity from BlackListNumbers
- **`threecx-cli black-list-numbers-id get-black-list-number`** - Get entity from BlackListNumbers by key
- **`threecx-cli black-list-numbers-id update-black-list-number`** - Update entity in BlackListNumbers

### blocklist

Manage blocklist

- **`threecx-cli blocklist bulk-ips-delete`** - Invoke action BulkIpsDelete
- **`threecx-cli blocklist create-addr`** - Add new entity to Blocklist
- **`threecx-cli blocklist list-addr`** - Get entities from Blocklist

### blocklist-id

Manage blocklist id

- **`threecx-cli blocklist-id delete-blocklist-addr`** - Delete entity from Blocklist
- **`threecx-cli blocklist-id get-blocklist-addr`** - Get entity from Blocklist by key
- **`threecx-cli blocklist-id update-blocklist-addr`** - Update entity in Blocklist

### call-cost-settings

Manage call cost settings

- **`threecx-cli call-cost-settings export-call-costs`** - Invoke function ExportCallCosts
- **`threecx-cli call-cost-settings list`** - Get entities from CallCostSettings
- **`threecx-cli call-cost-settings update-cost`** - Invoke action UpdateCost

### call-flow-apps

Manage call flow apps

- **`threecx-cli call-flow-apps create`** - Add new entity to CallFlowApps
- **`threecx-cli call-flow-apps list`** - Get entities from CallFlowApps

### call-flow-apps-id

Manage call flow apps id

- **`threecx-cli call-flow-apps-id delete-call-flow-app`** - Delete entity from CallFlowApps
- **`threecx-cli call-flow-apps-id delete-file`** - Invoke action DeleteFile
- **`threecx-cli call-flow-apps-id get-call-flow-app`** - Get entity from CallFlowApps by key
- **`threecx-cli call-flow-apps-id get-files`** - Invoke function GetFiles
- **`threecx-cli call-flow-apps-id update-call-flow-app`** - Update entity in CallFlowApps

### call-flow-scripts

Manage call flow scripts

- **`threecx-cli call-flow-scripts`** - Get entities from CallFlowScripts

### call-history-view

Manage call history view

- **`threecx-cli call-history-view download-call-history`** - Invoke function DownloadCallHistory
- **`threecx-cli call-history-view list`** - Get entities from CallHistoryView

### call-parking-settings

Manage call parking settings

- **`threecx-cli call-parking-settings get`** - Get CallParkingSettings
- **`threecx-cli call-parking-settings update`** - Update CallParkingSettings

### call-types-settings

Manage call types settings

- **`threecx-cli call-types-settings get`** - Get CallTypesSettings
- **`threecx-cli call-types-settings update`** - Update CallTypesSettings

### cdrsettings

Manage cdrsettings

- **`threecx-cli cdrsettings get`** - Get CDRSettings
- **`threecx-cli cdrsettings update`** - Update CDRSettings

### chat-history-view

Manage chat history view

- **`threecx-cli chat-history-view download-chat-history`** - Invoke function DownloadChatHistory
- **`threecx-cli chat-history-view list`** - Get entities from ChatHistoryView

### chat-log-settings

Manage chat log settings

- **`threecx-cli chat-log-settings get`** - Get ChatLogSettings
- **`threecx-cli chat-log-settings update`** - Update ChatLogSettings

### chat-messages-history-view

Manage chat messages history view

- **`threecx-cli chat-messages-history-view download-chat-messages-history`** - Invoke function DownloadChatMessagesHistory
- **`threecx-cli chat-messages-history-view list`** - Get entities from ChatMessagesHistoryView

### codecs-settings

Manage codecs settings

- **`threecx-cli codecs-settings get`** - Get CodecsSettings
- **`threecx-cli codecs-settings update`** - Update CodecsSettings

### conference-settings

Manage conference settings

- **`threecx-cli conference-settings delete-api-key`** - Invoke action DeleteApiKey
- **`threecx-cli conference-settings generate-api-key`** - Invoke function GenerateApiKey
- **`threecx-cli conference-settings get`** - Get ConferenceSettings
- **`threecx-cli conference-settings get-mcurequest-status`** - Invoke function GetMCURequestStatus
- **`threecx-cli conference-settings get-mcurow`** - Invoke function GetMCURow
- **`threecx-cli conference-settings get-mcurows`** - Invoke function GetMCURows
- **`threecx-cli conference-settings get-onboard-mcu-data`** - Invoke function GetOnboardMcuData
- **`threecx-cli conference-settings get-onboard-meetings`** - Invoke function GetOnboardMeetings
- **`threecx-cli conference-settings get-web-meeting-zones`** - Invoke function GetWebMeetingZones
- **`threecx-cli conference-settings update`** - Update ConferenceSettings
- **`threecx-cli conference-settings update-mcurequest-status`** - Invoke action UpdateMCURequestStatus

### console-restrictions

Manage console restrictions

- **`threecx-cli console-restrictions get`** - Get ConsoleRestrictions
- **`threecx-cli console-restrictions update`** - Update ConsoleRestrictions

### contacts

Manage contacts

- **`threecx-cli contacts create`** - Add new entity to Contacts
- **`threecx-cli contacts create-by-number`** - Invoke action CreateContactByNumber
- **`threecx-cli contacts delete-by-id`** - Invoke action DeleteContactsById
- **`threecx-cli contacts delete-by-type`** - Invoke action DeleteContactsByType
- **`threecx-cli contacts delete-department-by-type`** - Invoke action DeleteDepartmentContactsByType
- **`threecx-cli contacts delete-personal-by-type`** - Invoke action DeletePersonalContactsByType
- **`threecx-cli contacts get-by-number`** - Invoke function GetContactsByNumber
- **`threecx-cli contacts get-dir-search-settings`** - Invoke function GetDirSearchSettings
- **`threecx-cli contacts list`** - Get entities from Contacts
- **`threecx-cli contacts set-dir-search-settings`** - Invoke action SetDirSearchSettings

### contacts-id

Manage contacts id

- **`threecx-cli contacts-id delete-contact`** - Delete entity from Contacts
- **`threecx-cli contacts-id get-contact`** - Get entity from Contacts by key
- **`threecx-cli contacts-id update-contact`** - Update entity in Contacts

### countries

Manage countries

- **`threecx-cli countries`** - Get entities from Countries

### country-codes

Manage country codes

- **`threecx-cli country-codes get`** - Get CountryCodes
- **`threecx-cli country-codes update`** - Update CountryCodes

### crm-integration

Manage crm integration

- **`threecx-cli crm-integration delete-crm-contacts`** - Invoke action DeleteCrmContacts
- **`threecx-cli crm-integration get`** - Get CrmIntegration
- **`threecx-cli crm-integration get-crm-template-source`** - Invoke function GetCrmTemplateSource
- **`threecx-cli crm-integration get-oauth`** - Invoke function GetOAuth
- **`threecx-cli crm-integration set-oauth-state`** - Invoke action SetOAuthState
- **`threecx-cli crm-integration test`** - Invoke action Test
- **`threecx-cli crm-integration update`** - Update CrmIntegration

### crm-templates

Manage crm templates

- **`threecx-cli crm-templates`** - Invoke function GeCrmtTemplates

### crm-templates-name

Manage crm templates name

- **`threecx-cli crm-templates-name delete-crm-template`** - Delete entity from CrmTemplates
- **`threecx-cli crm-templates-name get-crm-template`** - Get entity from CrmTemplates by key

### custom-prompts

Manage custom prompts

- **`threecx-cli custom-prompts list`** - Get entities from CustomPrompts
- **`threecx-cli custom-prompts make-call-record`** - Invoke action MakeCallRecordCustomPrompt

### custom-prompts-filename

Manage custom prompts filename

- **`threecx-cli custom-prompts-filename <Filename>`** - Delete entity from CustomPrompts

### data-connector-settings

Manage data connector settings

- **`threecx-cli data-connector-settings get`** - Get DataConnectorSettings
- **`threecx-cli data-connector-settings synchronize`** - Invoke action Synchronize
- **`threecx-cli data-connector-settings test-data-connector`** - Invoke action TestDataConnector
- **`threecx-cli data-connector-settings update`** - Update DataConnectorSettings

### defs

Manage defs

- **`threecx-cli defs get`** - Get Defs
- **`threecx-cli defs get-license-parameters`** - Invoke function GetLicenseParameters
- **`threecx-cli defs get-routes`** - Invoke action GetRoutes
- **`threecx-cli defs get-system-parameters`** - Invoke function GetSystemParameters
- **`threecx-cli defs has-system-owner`** - Invoke function HasSystemOwner
- **`threecx-cli defs list-codecs`** - Get Codecs from Defs
- **`threecx-cli defs list-gateway-parameter-values`** - Get GatewayParameterValues from Defs
- **`threecx-cli defs list-gateway-parameters`** - Get GatewayParameters from Defs
- **`threecx-cli defs list-time-zones`** - Get TimeZones from Defs
- **`threecx-cli defs report-log-event`** - Invoke action ReportLogEvent
- **`threecx-cli defs send-email`** - Invoke action SendEmail

### device-infos

Manage device infos

- **`threecx-cli device-infos`** - Get entities from DeviceInfos

### device-infos-id

Manage device infos id

- **`threecx-cli device-infos-id delete-device-info`** - Delete entity from DeviceInfos
- **`threecx-cli device-infos-id get-device-info`** - Get entity from DeviceInfos by key
- **`threecx-cli device-infos-id provision`** - Invoke action Provision

### dial-code-settings

Manage dial code settings

- **`threecx-cli dial-code-settings get`** - Get DialCodeSettings
- **`threecx-cli dial-code-settings update`** - Update DialCodeSettings

### did-numbers

Manage did numbers

- **`threecx-cli did-numbers`** - Get entities from DidNumbers

### dnproperties

Manage dnproperties

- **`threecx-cli dnproperties create-dnproperty`** - Invoke action CreateDNProperty
- **`threecx-cli dnproperties delete-dnproperty`** - Invoke action DeleteDNProperty
- **`threecx-cli dnproperties get-dnproperty-by-name`** - Invoke function GetDNPropertyByName
- **`threecx-cli dnproperties get-properties-by-dn`** - Invoke function GetPropertiesByDn
- **`threecx-cli dnproperties update-dnproperty`** - Invoke action UpdateDNProperty

### e164-settings

Manage e164 settings

- **`threecx-cli e164-settings get`** - Get E164Settings
- **`threecx-cli e164-settings update`** - Update E164Settings

### email-template

Manage email template

- **`threecx-cli email-template`** - Get entities from EmailTemplate

### email-template-template-path

Manage email template template path

- **`threecx-cli email-template-template-path get-email-template`** - Get entity from EmailTemplate by key
- **`threecx-cli email-template-template-path set-default`** - Invoke action SetDefault
- **`threecx-cli email-template-template-path update-email-template`** - Update entity in EmailTemplate

### emergency-geo-locations

Manage emergency geo locations

- **`threecx-cli emergency-geo-locations list`** - Get entities from EmergencyGeoLocations
- **`threecx-cli emergency-geo-locations update`** - Invoke action Update

### emergency-notifications-settings

Manage emergency notifications settings

- **`threecx-cli emergency-notifications-settings get`** - Get EmergencyNotificationsSettings
- **`threecx-cli emergency-notifications-settings update`** - Update EmergencyNotificationsSettings

### event-logs

Manage event logs

- **`threecx-cli event-logs download`** - Invoke function DownloadEventLogs
- **`threecx-cli event-logs list`** - Get entities from EventLogs
- **`threecx-cli event-logs purge`** - Invoke action PurgeEventLog

### fax

Manage fax

- **`threecx-cli fax bulk-delete`** - Invoke action BulkFaxDelete
- **`threecx-cli fax create`** - Add new entity to Fax
- **`threecx-cli fax init`** - Invoke function InitFax
- **`threecx-cli fax list`** - Get entities from Fax

### fax-id

Manage fax id

- **`threecx-cli fax-id delete-fax`** - Delete entity from Fax
- **`threecx-cli fax-id get-fax`** - Get entity from Fax by key
- **`threecx-cli fax-id update-fax`** - Update entity in Fax

### fax-number-number

Manage fax number number

- **`threecx-cli fax-number-number <Number>`** - Get entity from Fax by key (Number)

### fax-server-settings

Manage fax server settings

- **`threecx-cli fax-server-settings clean-up-fax`** - Invoke action CleanUpFax
- **`threecx-cli fax-server-settings get`** - Get FaxServerSettings
- **`threecx-cli fax-server-settings get-fax-files-size`** - Invoke function GetFaxFilesSize
- **`threecx-cli fax-server-settings update`** - Update FaxServerSettings

### firewall

Manage firewall

- **`threecx-cli firewall get-last-result`** - Invoke function GetLastResult
- **`threecx-cli firewall get-state`** - Get Firewall
- **`threecx-cli firewall start-check`** - Invoke action StartCheck
- **`threecx-cli firewall stop-check`** - Invoke action StopCheck

### firmwares

Manage firmwares

- **`threecx-cli firmwares clean-up`** - Invoke action CleanUp
- **`threecx-cli firmwares get-state`** - Invoke function GetFirmwareState
- **`threecx-cli firmwares list`** - Get entities from Firmwares

### firmwares-id

Manage firmwares id

- **`threecx-cli firmwares-id delete-firmware`** - Delete entity from Firmwares
- **`threecx-cli firmwares-id push-firmware-for-phones`** - Invoke action PushFirmwareForPhones
- **`threecx-cli firmwares-id set-as-default`** - Invoke action SetAsDefault

### fxs

Manage fxs

- **`threecx-cli fxs create`** - Add new entity to Fxs
- **`threecx-cli fxs list`** - Get entities from Fxs

### fxs-mac-address

Manage fxs mac address

- **`threecx-cli fxs-mac-address delete-fxs`** - Delete entity from Fxs
- **`threecx-cli fxs-mac-address get-fxs`** - Get entity from Fxs by key
- **`threecx-cli fxs-mac-address regenerate-web-credentials`** - Invoke action RegenerateWebCredentials
- **`threecx-cli fxs-mac-address update-fxs`** - Update entity in Fxs

### fxs-templates

Manage fxs templates

- **`threecx-cli fxs-templates create`** - Add new entity to FxsTemplates
- **`threecx-cli fxs-templates list`** - Get entities from FxsTemplates

### fxs-templates-id

Manage fxs templates id

- **`threecx-cli fxs-templates-id delete-fxs-template`** - Delete entity from FxsTemplates
- **`threecx-cli fxs-templates-id get-fxs-template`** - Get entity from FxsTemplates by key
- **`threecx-cli fxs-templates-id update-fxs-template`** - Update entity in FxsTemplates

### general-settings-for-apps

Manage general settings for apps

- **`threecx-cli general-settings-for-apps get`** - Get GeneralSettingsForApps
- **`threecx-cli general-settings-for-apps update`** - Update GeneralSettingsForApps

### general-settings-for-pbx

Manage general settings for pbx

- **`threecx-cli general-settings-for-pbx get`** - Get GeneralSettingsForPbx
- **`threecx-cli general-settings-for-pbx update`** - Update GeneralSettingsForPbx

### get-directory-info

Manage get directory info

- **`threecx-cli get-directory-info`** - Invoke actionImport GetDirectoryInfo

### get-prompt-set-updates

Manage get prompt set updates

- **`threecx-cli get-prompt-set-updates`** - Invoke functionImport GetPromptSetUpdates

### get-server-crm-updates

Manage get server crm updates

- **`threecx-cli get-server-crm-updates`** - Invoke functionImport GetServerCrmUpdates

### get-update-details-key-key-id-id

Manage get update details key key id id

- **`threecx-cli get-update-details-key-key-id-id <key> <id>`** - Invoke functionImport GetUpdateDetails

### get-update-settings

Manage get update settings

- **`threecx-cli get-update-settings`** - Invoke functionImport GetUpdateSettings

### get-updates

Manage get updates

- **`threecx-cli get-updates`** - Invoke functionImport GetUpdates

### get-updates-stats

Manage get updates stats

- **`threecx-cli get-updates-stats`** - Invoke functionImport GetUpdatesStats

### google-settings

Manage google settings

- **`threecx-cli google-settings get`** - Get GoogleSettings
- **`threecx-cli google-settings get-google-users`** - Invoke action GetGoogleUsers
- **`threecx-cli google-settings update`** - Update GoogleSettings

### groups

Manage groups

- **`threecx-cli groups create`** - Add new entity to Groups
- **`threecx-cli groups delete-company-by-id`** - Invoke action DeleteCompanyById
- **`threecx-cli groups delete-company-by-number`** - Invoke action DeleteCompanyByNumber
- **`threecx-cli groups link-partner`** - Invoke action LinkGroupPartner
- **`threecx-cli groups list`** - Get entities from Groups
- **`threecx-cli groups replace-license-key`** - Invoke action ReplaceGroupLicenseKey
- **`threecx-cli groups unlink-partner`** - Invoke action UnlinkGroupPartner

### groups-id

Manage groups id

- **`threecx-cli groups-id delete-group`** - Delete entity from Groups
- **`threecx-cli groups-id get-group`** - Get entity from Groups by key
- **`threecx-cli groups-id get-restrictions`** - Invoke function GetRestrictions
- **`threecx-cli groups-id list-members`** - Get Members from Groups
- **`threecx-cli groups-id list-rights`** - Get Rights from Groups
- **`threecx-cli groups-id update-group`** - Update entity in Groups

### holidays

Manage holidays

- **`threecx-cli holidays create`** - Add new entity to Holidays
- **`threecx-cli holidays list`** - Get entities from Holidays

### holidays-id

Manage holidays id

- **`threecx-cli holidays-id delete-holiday`** - Delete entity from Holidays
- **`threecx-cli holidays-id get-holiday`** - Get entity from Holidays by key
- **`threecx-cli holidays-id update-holiday`** - Update entity in Holidays

### hotel-rooms-id

Manage hotel rooms id

- **`threecx-cli hotel-rooms-id assign`** - Invoke action Assign
- **`threecx-cli hotel-rooms-id clear`** - Invoke action Clear

### hotel-services

Manage hotel services

- **`threecx-cli hotel-services get`** - Get HotelServices
- **`threecx-cli hotel-services update`** - Update HotelServices

### inbound-rules

Manage inbound rules

- **`threecx-cli inbound-rules bulk-delete`** - Invoke action BulkInboundRulesDelete
- **`threecx-cli inbound-rules create`** - Add new entity to InboundRules
- **`threecx-cli inbound-rules export-caller-id-rules`** - Invoke function ExportCallerIdRules
- **`threecx-cli inbound-rules list`** - Get entities from InboundRules

### inbound-rules-id

Manage inbound rules id

- **`threecx-cli inbound-rules-id delete-inbound-rule`** - Delete entity from InboundRules
- **`threecx-cli inbound-rules-id get-inbound-rule`** - Get entity from InboundRules by key
- **`threecx-cli inbound-rules-id update-inbound-rule`** - Update entity in InboundRules

### install-updates

Manage install updates

- **`threecx-cli install-updates`** - Invoke actionImport InstallUpdates

### last-cdr-and-chat-message-timestamp

Manage last cdr and chat message timestamp

- **`threecx-cli last-cdr-and-chat-message-timestamp`** - Get entities from LastCdrAndChatMessageTimestamp

### license-status

Manage license status

- **`threecx-cli license-status edit-license-info`** - Invoke action EditLicenseInfo
- **`threecx-cli license-status get`** - Get LicenseStatus
- **`threecx-cli license-status link-partner`** - Invoke action LinkPartner
- **`threecx-cli license-status partner-info`** - Invoke function PartnerInfo
- **`threecx-cli license-status refresh`** - Invoke action RefreshLicenseStatus
- **`threecx-cli license-status replace-license-key`** - Invoke action ReplaceLicenseKey
- **`threecx-cli license-status unlink-partner`** - Invoke action UnlinkPartner

### logging-settings

Manage logging settings

- **`threecx-cli logging-settings get`** - Get LoggingSettings
- **`threecx-cli logging-settings update`** - Update LoggingSettings

### mail-settings

Manage mail settings

- **`threecx-cli mail-settings get`** - Get MailSettings
- **`threecx-cli mail-settings test-email`** - Invoke action TestEmail
- **`threecx-cli mail-settings update`** - Update MailSettings

### mcp-servers

Manage mcp servers

- **`threecx-cli mcp-servers exchange`** - Invoke action Exchange
- **`threecx-cli mcp-servers get-server-metadata`** - Invoke function GetServerMetadata
- **`threecx-cli mcp-servers register`** - Invoke action Register

### microsoft365-integration

Manage microsoft365 integration

- **`threecx-cli microsoft365-integration authorize-presence`** - Invoke action AuthorizePresence
- **`threecx-cli microsoft365-integration deauthorize-presence`** - Invoke action DeauthorizePresence
- **`threecx-cli microsoft365-integration get`** - Get Microsoft365Integration
- **`threecx-cli microsoft365-integration get-m365-groups`** - Invoke action GetM365Groups
- **`threecx-cli microsoft365-integration get-m365-users`** - Invoke action GetM365Users
- **`threecx-cli microsoft365-integration get-microsoft-access-token`** - Invoke function GetMicrosoftAccessToken
- **`threecx-cli microsoft365-integration get-microsoft365-directory`** - Invoke function GetMicrosoft365Directory
- **`threecx-cli microsoft365-integration get-users-by-principal-names`** - Invoke action GetUsersByPrincipalNames
- **`threecx-cli microsoft365-integration test-presence`** - Invoke action TestPresence
- **`threecx-cli microsoft365-integration test-subscription`** - Invoke function TestSubscription
- **`threecx-cli microsoft365-integration update`** - Update Microsoft365Integration

### microsoft365-teams-integration

Manage microsoft365 teams integration

- **`threecx-cli microsoft365-teams-integration check-fqdn-record`** - Invoke function CheckFqdnRecord
- **`threecx-cli microsoft365-teams-integration check-map-users-script`** - Invoke function CheckMapUsersScript
- **`threecx-cli microsoft365-teams-integration get`** - Get Microsoft365TeamsIntegration
- **`threecx-cli microsoft365-teams-integration get-dial-plan-script`** - Invoke function GetDialPlanScript
- **`threecx-cli microsoft365-teams-integration get-map-users-script`** - Invoke function GetMapUsersScript
- **`threecx-cli microsoft365-teams-integration update`** - Update Microsoft365TeamsIntegration

### music-on-hold-settings

Manage music on hold settings

- **`threecx-cli music-on-hold-settings get`** - Get MusicOnHoldSettings
- **`threecx-cli music-on-hold-settings update`** - Update MusicOnHoldSettings

### my-group

Manage my group

- **`threecx-cli my-group get`** - Get MyGroup
- **`threecx-cli my-group get-partner-info`** - Invoke function GetMyGroupPartnerInfo
- **`threecx-cli my-group get-restrictions`** - Invoke function GetRestrictions
- **`threecx-cli my-group link-partner`** - Invoke action LinkMyGroupPartner
- **`threecx-cli my-group list-members`** - Get Members from MyGroup
- **`threecx-cli my-group list-rights`** - Get Rights from MyGroup
- **`threecx-cli my-group replace-license-key`** - Invoke action ReplaceMyGroupLicenseKey
- **`threecx-cli my-group unlink-partner`** - Invoke action UnlinkMyGroupPartner
- **`threecx-cli my-group update`** - Update MyGroup

### my-tokens

Manage my tokens

- **`threecx-cli my-tokens`** - Get entities from MyTokens

### my-tokens-id

Manage my tokens id

- **`threecx-cli my-tokens-id <Id>`** - Invoke action RevokeToken

### my-user

Manage my user

- **`threecx-cli my-user generate-prov-link`** - Invoke function GenerateProvLink
- **`threecx-cli my-user get`** - Get MyUser
- **`threecx-cli my-user issue-download-ticket`** - Invoke action IssueDownloadTicket
- **`threecx-cli my-user list-forwarding-profiles`** - Get ForwardingProfiles from MyUser
- **`threecx-cli my-user list-greetings`** - Get Greetings from MyUser
- **`threecx-cli my-user list-groups`** - Get Groups from MyUser
- **`threecx-cli my-user update`** - Update MyUser

### network-interfaces

Manage network interfaces

- **`threecx-cli network-interfaces`** - Get entities from NetworkInterfaces

### network-settings

Manage network settings

- **`threecx-cli network-settings get`** - Get NetworkSettings
- **`threecx-cli network-settings get-ifaces`** - Invoke function GetIfaces
- **`threecx-cli network-settings update`** - Update NetworkSettings

### notification-settings

Manage notification settings

- **`threecx-cli notification-settings get`** - Get NotificationSettings
- **`threecx-cli notification-settings update`** - Update NotificationSettings

### oauth-clients

Manage oauth clients

- **`threecx-cli oauth-clients`** - Get entities from OAuthClients

### oauth-clients-id

Manage oauth clients id

- **`threecx-cli oauth-clients-id delete-oauth-client`** - Delete entity from OAuthClients
- **`threecx-cli oauth-clients-id get-oauth-client`** - Get entity from OAuthClients by key
- **`threecx-cli oauth-clients-id refresh-client-metadata`** - Invoke action RefreshClientMetadata

### office-hours

Manage office hours

- **`threecx-cli office-hours get`** - Get OfficeHours
- **`threecx-cli office-hours update`** - Update OfficeHours

### outbound-rules

Manage outbound rules

- **`threecx-cli outbound-rules create`** - Add new entity to OutboundRules
- **`threecx-cli outbound-rules get-emergency`** - Invoke function GetEmergencyOutboundRules
- **`threecx-cli outbound-rules list`** - Get entities from OutboundRules
- **`threecx-cli outbound-rules move-up-down`** - Invoke action MoveUpDown
- **`threecx-cli outbound-rules purge`** - Invoke action Purge

### outbound-rules-id

Manage outbound rules id

- **`threecx-cli outbound-rules-id delete-outbound-rule`** - Delete entity from OutboundRules
- **`threecx-cli outbound-rules-id get-outbound-rule`** - Get entity from OutboundRules by key
- **`threecx-cli outbound-rules-id update-outbound-rule`** - Update entity in OutboundRules

### parameters

Manage parameters

- **`threecx-cli parameters create`** - Add new entity to Parameters
- **`threecx-cli parameters get-by-name`** - Invoke function GetParameterByName
- **`threecx-cli parameters list`** - Get entities from Parameters

### parameters-id

Manage parameters id

- **`threecx-cli parameters-id delete-parameter`** - Delete entity from Parameters
- **`threecx-cli parameters-id get-parameter`** - Get entity from Parameters by key
- **`threecx-cli parameters-id update-parameter`** - Update entity in Parameters

### parkings

Manage parkings

- **`threecx-cli parkings create`** - Add new entity to Parkings
- **`threecx-cli parkings list`** - Get entities from Parkings

### parkings-id

Manage parkings id

- **`threecx-cli parkings-id delete-parking`** - Delete entity from Parkings
- **`threecx-cli parkings-id get-parking`** - Get entity from Parkings by key
- **`threecx-cli parkings-id list-parking-groups`** - Get Groups from Parkings
- **`threecx-cli parkings-id update-parking`** - Update entity in Parkings

### parkings-number-number

Manage parkings number number

- **`threecx-cli parkings-number-number <Number>`** - Get entity from Parkings by key (Number)

### peers

Manage peers

- **`threecx-cli peers get-report`** - Invoke function GetReportPeers
- **`threecx-cli peers list`** - Get entities from Peers
- **`threecx-cli peers retreive-by-numbers`** - Invoke action RetreivePeersByNumbers

### peers-number-number

Manage peers number number

- **`threecx-cli peers-number-number <Number>`** - Get entity from Peers by key (Number)

### phone-book-settings

Manage phone book settings

- **`threecx-cli phone-book-settings get`** - Get PhoneBookSettings
- **`threecx-cli phone-book-settings update`** - Update PhoneBookSettings

### phone-logos

Manage phone logos

- **`threecx-cli phone-logos`** - Get entities from PhoneLogos

### phone-logos-filename

Manage phone logos filename

- **`threecx-cli phone-logos-filename <Filename>`** - Delete entity from PhoneLogos

### phone-templates

Manage phone templates

- **`threecx-cli phone-templates create`** - Add new entity to PhoneTemplates
- **`threecx-cli phone-templates list`** - Get entities from PhoneTemplates

### phone-templates-id

Manage phone templates id

- **`threecx-cli phone-templates-id delete-phone-template`** - Delete entity from PhoneTemplates
- **`threecx-cli phone-templates-id get-phone-template`** - Get entity from PhoneTemplates by key
- **`threecx-cli phone-templates-id update-phone-template`** - Update entity in PhoneTemplates

### phones-settings

Manage phones settings

- **`threecx-cli phones-settings get`** - Get PhonesSettings
- **`threecx-cli phones-settings update`** - Update PhonesSettings

### playlists

Manage playlists

- **`threecx-cli playlists create`** - Add new entity to Playlists
- **`threecx-cli playlists delete-file`** - Invoke action DeletePlaylistFile
- **`threecx-cli playlists download-file`** - Invoke function DownloadPlaylistFile
- **`threecx-cli playlists list`** - Get entities from Playlists

### playlists-name

Manage playlists name

- **`threecx-cli playlists-name delete-playlist`** - Delete entity from Playlists
- **`threecx-cli playlists-name get-playlist`** - Get entity from Playlists by key
- **`threecx-cli playlists-name update-playlist`** - Update entity in Playlists

### prompt-sets

Manage prompt sets

- **`threecx-cli prompt-sets get-active`** - Invoke function GetActive
- **`threecx-cli prompt-sets list`** - Get entities from PromptSets

### prompt-sets-id

Manage prompt sets id

- **`threecx-cli prompt-sets-id copy`** - Invoke action Copy
- **`threecx-cli prompt-sets-id delete-prompt-set`** - Delete entity from PromptSets
- **`threecx-cli prompt-sets-id get-prompt-set`** - Get entity from PromptSets by key
- **`threecx-cli prompt-sets-id list-prompts`** - Get Prompts from PromptSets
- **`threecx-cli prompt-sets-id play-prompt`** - Invoke action PlayPrompt
- **`threecx-cli prompt-sets-id record-prompt`** - Invoke action RecordPrompt
- **`threecx-cli prompt-sets-id set-active`** - Invoke action SetActive
- **`threecx-cli prompt-sets-id set-alternate-pronunciation`** - Invoke action SetAlternatePronunciation
- **`threecx-cli prompt-sets-id update-prompt-set`** - Update entity in PromptSets

### purge-all-logs

Manage purge all logs

- **`threecx-cli purge-all-logs`** - Invoke actionImport PurgeAllLogs

### purge-calls

Manage purge calls

- **`threecx-cli purge-calls`** - Invoke actionImport PurgeCalls

### purge-chats

Manage purge chats

- **`threecx-cli purge-chats`** - Invoke actionImport PurgeChats

### queues

Manage queues

- **`threecx-cli queues create`** - Add new entity to Queues
- **`threecx-cli queues get-first-available-number`** - Invoke function GetFirstAvailableQueueNumber
- **`threecx-cli queues list`** - Get entities from Queues

### queues-id

Manage queues id

- **`threecx-cli queues-id delete-queue`** - Delete entity from Queues
- **`threecx-cli queues-id get-queue`** - Get entity from Queues by key
- **`threecx-cli queues-id list-agents`** - Get Agents from Queues
- **`threecx-cli queues-id list-managers`** - Get Managers from Queues
- **`threecx-cli queues-id reset-queue-statistics`** - Invoke action ResetQueueStatistics
- **`threecx-cli queues-id update-queue`** - Update entity in Queues

### queues-number-number

Manage queues number number

- **`threecx-cli queues-number-number <Number>`** - Get entity from Queues by key (Number)

### receptionists

Manage receptionists

- **`threecx-cli receptionists create`** - Add new entity to Receptionists
- **`threecx-cli receptionists get-first-available-number`** - Invoke function GetFirstAvailableReceptionistNumber
- **`threecx-cli receptionists list`** - Get entities from Receptionists

### receptionists-id

Manage receptionists id

- **`threecx-cli receptionists-id delete-receptionist`** - Delete entity from Receptionists
- **`threecx-cli receptionists-id get-receptionist`** - Get entity from Receptionists by key
- **`threecx-cli receptionists-id list-forwards`** - Get Forwards from Receptionists
- **`threecx-cli receptionists-id update-receptionist`** - Update entity in Receptionists

### receptionists-number-number

Manage receptionists number number

- **`threecx-cli receptionists-number-number <Number>`** - Get entity from Receptionists by key (Number)

### recordings

Manage recordings

- **`threecx-cli recordings archive`** - Invoke action Archive
- **`threecx-cli recordings bulk-archive`** - Invoke action BulkRecordingsArchive
- **`threecx-cli recordings bulk-delete`** - Invoke action BulkRecordingsDelete
- **`threecx-cli recordings download`** - Invoke function DownloadRecording
- **`threecx-cli recordings get-repository-settings`** - Invoke function GetRecordingRepositorySettings
- **`threecx-cli recordings list`** - Get entities from Recordings
- **`threecx-cli recordings purge-archive`** - Invoke action PurgeArchive
- **`threecx-cli recordings purge-local`** - Invoke action PurgeLocal
- **`threecx-cli recordings set-repository-settings`** - Invoke action SetRecordingRepositorySettings
- **`threecx-cli recordings transcribe`** - Invoke action TranscribeRecordings

### remote-archiving-settings

Manage remote archiving settings

- **`threecx-cli remote-archiving-settings archive-chats`** - Invoke action ArchiveChats
- **`threecx-cli remote-archiving-settings archive-faxes`** - Invoke action ArchiveFaxes
- **`threecx-cli remote-archiving-settings archive-recordings`** - Invoke action ArchiveRecordings
- **`threecx-cli remote-archiving-settings archive-voicemail`** - Invoke action ArchiveVoicemail
- **`threecx-cli remote-archiving-settings get`** - Get RemoteArchivingSettings
- **`threecx-cli remote-archiving-settings update`** - Update RemoteArchivingSettings

### report-abandoned-chats-statistics

Manage report abandoned chats statistics

- **`threecx-cli report-abandoned-chats-statistics download-abandoned-chats-statistics`** - Invoke function DownloadAbandonedChatsStatistics
- **`threecx-cli report-abandoned-chats-statistics get-abandoned-chats-statistics-data`** - Invoke function GetAbandonedChatsStatisticsData

### report-abandoned-queue-calls

Manage report abandoned queue calls

- **`threecx-cli report-abandoned-queue-calls download-abandoned-queue-calls`** - Invoke function DownloadAbandonedQueueCalls
- **`threecx-cli report-abandoned-queue-calls get-abandoned-queue-calls-data`** - Invoke function GetAbandonedQueueCallsData

### report-agent-login-history

Manage report agent login history

- **`threecx-cli report-agent-login-history download-agent-login-history`** - Invoke function DownloadAgentLoginHistory
- **`threecx-cli report-agent-login-history get-agent-login-history-data`** - Invoke function GetAgentLoginHistoryData

### report-agents-in-queue-statistics

Manage report agents in queue statistics

- **`threecx-cli report-agents-in-queue-statistics download-agents-in-queue-statistics`** - Invoke function DownloadAgentsInQueueStatistics
- **`threecx-cli report-agents-in-queue-statistics get-agents-in-queue-statistics-data`** - Invoke function GetAgentsInQueueStatisticsData

### report-audit-log

Manage report audit log

- **`threecx-cli report-audit-log download-audit-log`** - Invoke function DownloadAuditLog
- **`threecx-cli report-audit-log get-audit-log-data`** - Invoke function GetAuditLogData

### report-average-queue-waiting-time

Manage report average queue waiting time

- **`threecx-cli report-average-queue-waiting-time download-average-queue-waiting-time-report`** - Invoke function DownloadAverageQueueWaitingTimeReport
- **`threecx-cli report-average-queue-waiting-time get-average-queue-waiting-time-data`** - Invoke function GetAverageQueueWaitingTimeData

### report-breaches-sla

Manage report breaches sla

- **`threecx-cli report-breaches-sla download-breaches-sla`** - Invoke function DownloadBreachesSla
- **`threecx-cli report-breaches-sla get-breaches-sla-data`** - Invoke function GetBreachesSlaData

### report-call-cost-by-extension-group

Manage report call cost by extension group

- **`threecx-cli report-call-cost-by-extension-group download-call-cost-by-extension-group`** - Invoke function DownloadCallCostByExtensionGroup
- **`threecx-cli report-call-cost-by-extension-group get-call-cost-by-extension-group-data`** - Invoke function GetCallCostByExtensionGroupData

### report-call-distribution

Manage report call distribution

- **`threecx-cli report-call-distribution download-get-get-call-distribution`** - Invoke function DownloadGetGetCallDistribution
- **`threecx-cli report-call-distribution get-call-distribution`** - Invoke function GetCallDistribution

### report-call-log-data

Manage report call log data

- **`threecx-cli report-call-log-data download-call-log`** - Invoke function DownloadCallLog
- **`threecx-cli report-call-log-data get-call-log-data`** - Invoke function GetCallLogData
- **`threecx-cli report-call-log-data get-call-quality-report`** - Invoke function GetCallQualityReport
- **`threecx-cli report-call-log-data get-old-call-log-data`** - Invoke function GetOldCallLogData
- **`threecx-cli report-call-log-data get-old-call-quality-report`** - Invoke function GetOldCallQualityReport

### report-chat-log

Manage report chat log

- **`threecx-cli report-chat-log download-chat-log`** - Invoke function DownloadChatLog
- **`threecx-cli report-chat-log get-chat-log`** - Invoke function GetChatLog

### report-detailed-queue-statistics

Manage report detailed queue statistics

- **`threecx-cli report-detailed-queue-statistics download-detailed-queue-statistics`** - Invoke function DownloadDetailedQueueStatistics
- **`threecx-cli report-detailed-queue-statistics get-detailed-queue-statistics-data`** - Invoke function GetDetailedQueueStatisticsData

### report-extension-statistics

Manage report extension statistics

- **`threecx-cli report-extension-statistics download-extension-statistics`** - Invoke function DownloadExtensionStatistics
- **`threecx-cli report-extension-statistics get-extension-statistics-data`** - Invoke function GetExtensionStatisticsData

### report-extension-statistics-by-group

Manage report extension statistics by group

- **`threecx-cli report-extension-statistics-by-group download-extension-statistics-by-group`** - Invoke function DownloadExtensionStatisticsByGroup
- **`threecx-cli report-extension-statistics-by-group get-extension-statistics-by-group-data`** - Invoke function GetExtensionStatisticsByGroupData

### report-extensions-statistics-by-ring-groups

Manage report extensions statistics by ring groups

- **`threecx-cli report-extensions-statistics-by-ring-groups download-extensions-statistics-by-ring-groups`** - Invoke function DownloadExtensionsStatisticsByRingGroups
- **`threecx-cli report-extensions-statistics-by-ring-groups get-extensions-statistics-by-ring-groups-data`** - Invoke function GetExtensionsStatisticsByRingGroupsData

### report-inbound-calls

Manage report inbound calls

- **`threecx-cli report-inbound-calls download-get-inbound-calls`** - Invoke function DownloadGetInboundCalls
- **`threecx-cli report-inbound-calls get-inbound-calls`** - Invoke function GetInboundCalls

### report-inbound-rules

Manage report inbound rules

- **`threecx-cli report-inbound-rules download-inbound-rules`** - Invoke function DownloadInboundRules
- **`threecx-cli report-inbound-rules get-inbound-rules-data`** - Invoke function GetInboundRulesData

### report-outbound-calls

Manage report outbound calls

- **`threecx-cli report-outbound-calls download-get-outbound-calls`** - Invoke function DownloadGetOutboundCalls
- **`threecx-cli report-outbound-calls get-outbound-calls`** - Invoke function GetOutboundCalls

### report-queue-agents-chat-statistics

Manage report queue agents chat statistics

- **`threecx-cli report-queue-agents-chat-statistics download-queue-agents-chat-statistics`** - Invoke function DownloadQueueAgentsChatStatistics
- **`threecx-cli report-queue-agents-chat-statistics get-queue-agents-chat-statistics-data`** - Invoke function GetQueueAgentsChatStatisticsData

### report-queue-agents-chat-statistics-totals

Manage report queue agents chat statistics totals

- **`threecx-cli report-queue-agents-chat-statistics-totals download-queue-agents-chat-statistics-totals`** - Invoke function DownloadQueueAgentsChatStatisticsTotals
- **`threecx-cli report-queue-agents-chat-statistics-totals get-queue-agents-chat-statistics-totals-data`** - Invoke function GetQueueAgentsChatStatisticsTotalsData

### report-queue-an-un-calls

Manage report queue an un calls

- **`threecx-cli report-queue-an-un-calls download-queue-an-un-calls-report`** - Invoke function DownloadQueueAnUnCallsReport
- **`threecx-cli report-queue-an-un-calls get-queue-an-un-calls-data`** - Invoke function GetQueueAnUnCallsData

### report-queue-answered-calls-by-wait-time

Manage report queue answered calls by wait time

- **`threecx-cli report-queue-answered-calls-by-wait-time download-queue-answered-calls-by-wait-time`** - Invoke function DownloadQueueAnsweredCallsByWaitTime
- **`threecx-cli report-queue-answered-calls-by-wait-time get-queue-answered-calls-by-wait-time-data`** - Invoke function GetQueueAnsweredCallsByWaitTimeData

### report-queue-callbacks

Manage report queue callbacks

- **`threecx-cli report-queue-callbacks download-queue-callbacks`** - Invoke function DownloadQueueCallbacks
- **`threecx-cli report-queue-callbacks get-queue-callbacks-data`** - Invoke function GetQueueCallbacksData

### report-queue-chat-performance

Manage report queue chat performance

- **`threecx-cli report-queue-chat-performance download-queue-chat-performance`** - Invoke function DownloadQueueChatPerformance
- **`threecx-cli report-queue-chat-performance get-queue-chat-performance-data`** - Invoke function GetQueueChatPerformanceData

### report-queue-failed-callbacks

Manage report queue failed callbacks

- **`threecx-cli report-queue-failed-callbacks download-queue-failed-callbacks`** - Invoke function DownloadQueueFailedCallbacks
- **`threecx-cli report-queue-failed-callbacks get-queue-failed-callbacks-data`** - Invoke function GetQueueFailedCallbacksData

### report-queue-performance-overview

Manage report queue performance overview

- **`threecx-cli report-queue-performance-overview download-queue-performance-overview`** - Invoke function DownloadQueuePerformanceOverview
- **`threecx-cli report-queue-performance-overview get-queue-performance-overview-data`** - Invoke function GetQueuePerformanceOverviewData

### report-queue-performance-totals

Manage report queue performance totals

- **`threecx-cli report-queue-performance-totals download-queue-performance-totals`** - Invoke function DownloadQueuePerformanceTotals
- **`threecx-cli report-queue-performance-totals get-queue-performance-totals-data`** - Invoke function GetQueuePerformanceTotalsData

### report-ring-group-statistics

Manage report ring group statistics

- **`threecx-cli report-ring-group-statistics download-ring-group-statistics`** - Invoke function DownloadRingGroupStatistics
- **`threecx-cli report-ring-group-statistics get-ring-group-statistics-data`** - Invoke function GetRingGroupStatisticsData

### report-statistic-sla

Manage report statistic sla

- **`threecx-cli report-statistic-sla download-statistic-sla`** - Invoke function DownloadStatisticSla
- **`threecx-cli report-statistic-sla get-statistic-sla-data`** - Invoke function GetStatisticSlaData

### report-team-queue-general-statistics

Manage report team queue general statistics

- **`threecx-cli report-team-queue-general-statistics download-team-queue-general-statistics`** - Invoke function DownloadTeamQueueGeneralStatistics
- **`threecx-cli report-team-queue-general-statistics get-team-queue-general-statistics-data`** - Invoke function GetTeamQueueGeneralStatisticsData

### report-user-activity

Manage report user activity

- **`threecx-cli report-user-activity download-get-user-activity`** - Invoke function DownloadGetUserActivity
- **`threecx-cli report-user-activity get-user-activity`** - Invoke function GetUserActivity

### ring-groups

Manage ring groups

- **`threecx-cli ring-groups create`** - Add new entity to RingGroups
- **`threecx-cli ring-groups get-first-available-number`** - Invoke function GetFirstAvailableRingGroupNumber
- **`threecx-cli ring-groups list`** - Get entities from RingGroups

### ring-groups-id

Manage ring groups id

- **`threecx-cli ring-groups-id delete-ring-group`** - Delete entity from RingGroups
- **`threecx-cli ring-groups-id get-ring-group`** - Get entity from RingGroups by key
- **`threecx-cli ring-groups-id list-ring-group-members`** - Get Members from RingGroups
- **`threecx-cli ring-groups-id update-ring-group`** - Update entity in RingGroups

### ring-groups-number-number

Manage ring groups number number

- **`threecx-cli ring-groups-number-number <Number>`** - Get entity from RingGroups by key (Number)

### sbcs

Manage sbcs

- **`threecx-cli sbcs create`** - Add new entity to Sbcs
- **`threecx-cli sbcs list`** - Get entities from Sbcs

### sbcs-name

Manage sbcs name

- **`threecx-cli sbcs-name delete-sbc`** - Delete entity from Sbcs
- **`threecx-cli sbcs-name get-sbc`** - Get entity from Sbcs by key
- **`threecx-cli sbcs-name push-config`** - Invoke action PushConfig
- **`threecx-cli sbcs-name update-sbc`** - Update entity in Sbcs

### scheduled-reports

Manage scheduled reports

- **`threecx-cli scheduled-reports create`** - Add new entity to ScheduledReports
- **`threecx-cli scheduled-reports list`** - Get entities from ScheduledReports

### scheduled-reports-id

Manage scheduled reports id

- **`threecx-cli scheduled-reports-id delete-scheduled-report`** - Delete entity from ScheduledReports
- **`threecx-cli scheduled-reports-id get-scheduled-report`** - Get entity from ScheduledReports by key
- **`threecx-cli scheduled-reports-id update-scheduled-report`** - Update entity in ScheduledReports

### secure-sip-settings

Manage secure sip settings

- **`threecx-cli secure-sip-settings get`** - Get SecureSipSettings
- **`threecx-cli secure-sip-settings update`** - Update SecureSipSettings

### security-tokens

Manage security tokens

- **`threecx-cli security-tokens`** - Get entities from SecurityTokens

### security-tokens-id

Manage security tokens id

- **`threecx-cli security-tokens-id <Id>`** - Invoke action RevokeToken

### service-principals

Manage service principals

- **`threecx-cli service-principals create`** - Add new entity to ServicePrincipals
- **`threecx-cli service-principals list`** - Get entities from ServicePrincipals

### service-principals-id

Manage service principals id

- **`threecx-cli service-principals-id delete-service-principal`** - Delete entity from ServicePrincipals
- **`threecx-cli service-principals-id generate-app-token`** - Invoke action GenerateAppToken
- **`threecx-cli service-principals-id get-service-principal`** - Get entity from ServicePrincipals by key
- **`threecx-cli service-principals-id update-service-principal`** - Update entity in ServicePrincipals

### services

Manage services

- **`threecx-cli services disable`** - Invoke action Disable
- **`threecx-cli services enable`** - Invoke action Enable
- **`threecx-cli services garbage-collect`** - Invoke action GarbageCollect
- **`threecx-cli services list-info`** - Get entities from Services
- **`threecx-cli services restart`** - Invoke action Restart
- **`threecx-cli services restart-operating-system`** - Invoke action RestartOperatingSystem
- **`threecx-cli services start`** - Invoke action Start
- **`threecx-cli services stop`** - Invoke action Stop

### set-update-settings

Manage set update settings

- **`threecx-cli set-update-settings`** - Invoke actionImport SetUpdateSettings

### sip-devices

Manage sip devices

- **`threecx-cli sip-devices bulk-switch-firmware`** - Invoke action BulkSwitchFirmware
- **`threecx-cli sip-devices bulk-update-firmware`** - Invoke action BulkUpdateFirmware
- **`threecx-cli sip-devices list`** - Get entities from SipDevices

### sip-devices-id

Manage sip devices id

- **`threecx-cli sip-devices-id clear-firmware`** - Invoke action ClearFirmware
- **`threecx-cli sip-devices-id move-phone`** - Invoke action MovePhone
- **`threecx-cli sip-devices-id push-firmware`** - Invoke action PushFirmware

### syslog-settings

Manage syslog settings

- **`threecx-cli syslog-settings get`** - Get SyslogSettings
- **`threecx-cli syslog-settings test-connection`** - Invoke action TestConnection
- **`threecx-cli syslog-settings update`** - Update SyslogSettings

### system-status

Manage system status

- **`threecx-cli system-status apitoken`** - Invoke function APIToken
- **`threecx-cli system-status get`** - Get SystemStatus
- **`threecx-cli system-status get-remote-access-status`** - Invoke function GetRemoteAccessStatus
- **`threecx-cli system-status get-request-help-link`** - Invoke function GetRequestHelpLink
- **`threecx-cli system-status get-version-type`** - Invoke function GetVersionType
- **`threecx-cli system-status is-request-help-enabled`** - Invoke function IsRequestHelpEnabled
- **`threecx-cli system-status my-phone-request-id-telemetry`** - Invoke function MyPhoneRequestIdTelemetry
- **`threecx-cli system-status network-telemetry`** - Invoke function NetworkTelemetry
- **`threecx-cli system-status request-help`** - Invoke action RequestHelp
- **`threecx-cli system-status revoke-remote-access`** - Invoke action RevokeRemoteAccess
- **`threecx-cli system-status service-telemetry`** - Invoke function ServiceTelemetry
- **`threecx-cli system-status set-chat-log-status`** - Invoke action SetChatLogStatus
- **`threecx-cli system-status set-multi-company-mode`** - Invoke action SetMultiCompanyMode
- **`threecx-cli system-status start-dbmaintenance`** - Invoke action StartDBMaintenance
- **`threecx-cli system-status system-database-information`** - Invoke function SystemDatabaseInformation
- **`threecx-cli system-status system-extensions`** - Invoke function SystemExtensions
- **`threecx-cli system-status system-health-status`** - Invoke function SystemHealthStatus
- **`threecx-cli system-status system-telemetry`** - Invoke function SystemTelemetry

### tenant-properties

Manage tenant properties

- **`threecx-cli tenant-properties create-property`** - Add new entity to TenantProperties
- **`threecx-cli tenant-properties list-property`** - Get entities from TenantProperties

### tenant-properties-name

Manage tenant properties name

- **`threecx-cli tenant-properties-name delete-property`** - Delete entity from TenantProperties
- **`threecx-cli tenant-properties-name get-property`** - Get entity from TenantProperties by key
- **`threecx-cli tenant-properties-name update-property`** - Update entity in TenantProperties

### trunk-templates

Manage trunk templates

- **`threecx-cli trunk-templates create`** - Add new entity to TrunkTemplates
- **`threecx-cli trunk-templates list`** - Get entities from TrunkTemplates

### trunk-templates-id

Manage trunk templates id

- **`threecx-cli trunk-templates-id delete-trunk-template`** - Delete entity from TrunkTemplates
- **`threecx-cli trunk-templates-id get-trunk-template`** - Get entity from TrunkTemplates by key
- **`threecx-cli trunk-templates-id update-trunk-template`** - Update entity in TrunkTemplates

### trunks

Manage trunks

- **`threecx-cli trunks create`** - Add new entity to Trunks
- **`threecx-cli trunks finalize-provisioning`** - Invoke action FinalizeTrunkProvisioning
- **`threecx-cli trunks get-first-available-number`** - Invoke function GetFirstAvailableTrunkNumber
- **`threecx-cli trunks get-provider-phones`** - Invoke action GetProviderPhones
- **`threecx-cli trunks init`** - Invoke function InitTrunk
- **`threecx-cli trunks init-master-bridge`** - Invoke function InitMasterBridge
- **`threecx-cli trunks init-slave-bridge`** - Invoke function InitSlaveBridge
- **`threecx-cli trunks list`** - Get entities from Trunks
- **`threecx-cli trunks refresh-registration`** - Invoke action RefreshRegistration
- **`threecx-cli trunks set-routes`** - Invoke action SetRoutes

### trunks-id

Manage trunks id

- **`threecx-cli trunks-id delete-trunk`** - Delete entity from Trunks
- **`threecx-cli trunks-id export-trunk`** - Invoke function ExportTrunk
- **`threecx-cli trunks-id get-trunk`** - Get entity from Trunks by key
- **`threecx-cli trunks-id run-analysis`** - Invoke action RunAnalysis
- **`threecx-cli trunks-id test-inbound-call`** - Invoke action TestInboundCall
- **`threecx-cli trunks-id test-outbound-call`** - Invoke action TestOutboundCall
- **`threecx-cli trunks-id update-trunk`** - Update entity in Trunks

### trunks-number-number

Manage trunks number number

- **`threecx-cli trunks-number-number <Number>`** - Get entity from Trunks by key (Number)

### users

Manage users

- **`threecx-cli users batch-delete`** - Invoke action BatchDelete
- **`threecx-cli users bulk-update`** - Invoke action BulkUpdate
- **`threecx-cli users create`** - Add new entity to Users
- **`threecx-cli users download-greeting`** - Invoke function DownloadGreeting
- **`threecx-cli users export-extensions`** - Invoke function ExportExtensions
- **`threecx-cli users get-duplicated-emails`** - Invoke action GetDuplicatedEmails
- **`threecx-cli users get-first-available-extension-number`** - Invoke function GetFirstAvailableExtensionNumber
- **`threecx-cli users get-first-available-hotdesking-number`** - Invoke function GetFirstAvailableHotdeskingNumber
- **`threecx-cli users get-multi-edit-greetings`** - Invoke action GetMultiEditGreetings
- **`threecx-cli users get-phone-registrar`** - Invoke function GetPhoneRegistrar
- **`threecx-cli users get-phone-registrars`** - Invoke action GetPhoneRegistrars
- **`threecx-cli users install-firmware`** - Invoke action InstallFirmware
- **`threecx-cli users list`** - Get entities from Users
- **`threecx-cli users make-call`** - Invoke action MakeCall
- **`threecx-cli users multi-delete-greeting`** - Invoke action MultiDeleteGreeting
- **`threecx-cli users multi-update`** - Invoke action MultiUserUpdate
- **`threecx-cli users reboot-phone`** - Invoke action RebootPhone
- **`threecx-cli users regenerate-passwords`** - Invoke action RegeneratePasswords
- **`threecx-cli users reprovision-all-phones`** - Invoke action ReprovisionAllPhones
- **`threecx-cli users reprovision-phone`** - Invoke action ReprovisionPhone

### users-id

Manage users id

- **`threecx-cli users-id delete-user`** - Delete entity from Users
- **`threecx-cli users-id generate-prov-link`** - Invoke function GenerateProvLink
- **`threecx-cli users-id get-user`** - Get entity from Users by key
- **`threecx-cli users-id has-duplicated-email`** - Invoke function HasDuplicatedEmail
- **`threecx-cli users-id list-forwarding-profiles`** - Get ForwardingProfiles from Users
- **`threecx-cli users-id list-greetings`** - Get Greetings from Users
- **`threecx-cli users-id list-groups`** - Get Groups from Users
- **`threecx-cli users-id make-call-user-record-greeting`** - Invoke action MakeCallUserRecordGreeting
- **`threecx-cli users-id regenerate`** - Invoke action Regenerate
- **`threecx-cli users-id resend-rps`** - Invoke action ResendRps
- **`threecx-cli users-id revoke-rps`** - Invoke action RevokeRps
- **`threecx-cli users-id send-welcome-email`** - Invoke action SendWelcomeEmail
- **`threecx-cli users-id set-monitor-status`** - Invoke action SetMonitorStatus
- **`threecx-cli users-id update-user`** - Update entity in Users

### users-number-number

Manage users number number

- **`threecx-cli users-number-number <Number>`** - Get entity from Users by key (Number)

### voicemail-settings

Manage voicemail settings

- **`threecx-cli voicemail-settings create-converter-configuration`** - Invoke action CreateConverterConfiguration
- **`threecx-cli voicemail-settings delete-all-user-voicemails`** - Invoke action DeleteAllUserVoicemails
- **`threecx-cli voicemail-settings delete-converter-configuration`** - Invoke action DeleteConverterConfiguration
- **`threecx-cli voicemail-settings get`** - Get VoicemailSettings
- **`threecx-cli voicemail-settings get-configured-converters`** - Invoke function GetConfiguredConverters
- **`threecx-cli voicemail-settings get-connected-converters-data`** - Invoke function GetConnectedConvertersData
- **`threecx-cli voicemail-settings get-converter-request-status`** - Invoke function GetConverterRequestStatus
- **`threecx-cli voicemail-settings get-transcribe-languages`** - Invoke function GetTranscribeLanguages
- **`threecx-cli voicemail-settings update`** - Update VoicemailSettings

### website-links

Manage website links

- **`threecx-cli website-links bulk-links-delete`** - Invoke action BulkLinksDelete
- **`threecx-cli website-links create-weblink`** - Add new entity to WebsiteLinks
- **`threecx-cli website-links list-weblink`** - Get entities from WebsiteLinks
- **`threecx-cli website-links validate-link`** - Invoke action ValidateLink

### website-links-link

Manage website links link

- **`threecx-cli website-links-link delete-weblink`** - Delete entity from WebsiteLinks
- **`threecx-cli website-links-link get-weblink`** - Get entity from WebsiteLinks by key
- **`threecx-cli website-links-link update-weblink`** - Update entity in WebsiteLinks


### audit


- **`threecx-cli audit`** - Find ring groups, queues, IVRs and inbound rules that route to deleted, disabled or missing extensions, on one phone system or the whole fleet.


### fleet


- **`threecx-cli fleet health`** - One row per client phone system with pass, warn, fail, unknown (not reported) or error (unreadable) for version, licence and maintenance expiry, last backup, trunks and extensions registered, disk, stopped services and capacity.

- **`threecx-cli fleet trunks`** - Every SIP trunk on every client phone system with state (online, offline, issue, unknown or error), certificate days left and configuration issues, problems first.

- **`threecx-cli fleet licences`** - Licence and maintenance expiry, edition, seat headroom and reseller of record for every client phone system, soonest expiry first.

- **`threecx-cli fleet versions`** - Each phone system's 3CX build against the newest build in your fleet, with auto-update state, pending updates and expired maintenance that blocks updates.

- **`threecx-cli fleet backups`** - Backup age, schedule, rotation and newest backup file for every client phone system, flagged never, stale, unscheduled, no files or shrunk.

- **`threecx-cli fleet exec`** - Run any read-only Configuration API query on every client phone system and get one merged table with a pbx column; credential fields are redacted and token-minting or download functions are refused.

- **`threecx-cli fleet changes`** - What changed across the fleet between two saved health or trunk runs: trunks offline or back, version bumped, licence renewed, backup gone stale, PBX unreachable.




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`threecx-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`threecx-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`threecx-cli learnings list`** - Inspect taught rows
- **`threecx-cli learnings forget <query>`** - Undo a teach
- **`threecx-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`threecx-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`threecx-cli teach-pattern`** - Install a query/resource template up front
- **`threecx-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `THREECX_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `threecx-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
threecx-cli active-calls

# JSON for scripting and agents
threecx-cli active-calls --json
# Filter to specific fields by name
threecx-cli active-calls --json --select <field>[,<field>...]

# Dry run  -  show the request without sending
threecx-cli active-calls --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
threecx-cli active-calls --agent
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

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `6` partial failure, `7` rate limited, `8` fleet problem found (`fleet health --fail-on`, `fleet trunks --fail-on-offline`), `10` config error.

## Runtime Endpoint

This CLI resolves endpoint placeholders at runtime, so one installed binary can target different tenants or API versions without regeneration.

Endpoint environment variables:
- `THREECX_FQDN` resolves `{fqdn}`

Base URL: `https://{fqdn}/xapi/v1`

## Health Check

```bash
threecx-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `threecx-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/threecx-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\threecx-cli\config.toml` on Windows; `--home`, `THREECX_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `THREECX_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `THREECX_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `THREECX_FQDN` | endpoint | Single PBX without the registry | Phone system host (`host` or `host:port`); resolves `{fqdn}` in `https://{fqdn}/xapi/v1`. Ignored under `--pbx`. |
| `THREECX_CLIENT_ID` | per_call | Single PBX without the registry | Client ID of the Configuration API client (the route point's DN number). |
| `THREECX_CLIENT_SECRET` | per_call | Single PBX without the registry | API key shown once when the client was created in Admin Console > Integrations > API. |
| `<PREFIX>_CLIENT_SECRET` | fleet | One per registered PBX | API key for a registered phone system; `<PREFIX>` is what `fleet add --env-prefix` recorded (default `THREECX_<NAME>`, so `THREECX_ACME_CLIENT_SECRET`). `fleet list` shows which are unset. |
| `THREECX_PBX` | fleet | No | Registered phone system to target, same as `--pbx <name>`. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `threecx-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `threecx-cli doctor` to check credentials
- Verify the environment variable is set: `echo $THREECX_CLIENT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **A PBX row shows 'OAuth client_credentials mint: HTTP 400' or 401**  -  Check the client ID is the service principal's DN number and that the env var named by 'fleet list' (secret_env) holds its API key.
- **HTTP 403 on reports, call history or some settings**  -  Give the service principal the System Owner role in Admin Console > Integrations > API.
- **A PBX errors at /connect/token or every XAPI call fails although credentials are right**  -  The Configuration API needs an 8SC or higher licence; exclude that PBX with --only or upgrade its licence.
- **On-premise PBX not reachable on 443**  -  Register it with its port: 'fleet add <name> --fqdn pbx.example.com:5001 ...'.
- **'fleet changes' says it needs two runs**  -  Run 'threecx-cli fleet health' now and again later; each run saves a snapshot.
- **A fleet report exits 5 with 'none of the PBX(es) in scope could be read'**  -  Every PBX failed; read each row's error field, then check the secret env vars with 'fleet list --json'.
- **The MCP server returns error rows for every PBX**  -  Pass each THREECX_<NAME>_CLIENT_SECRET variable (and THREECX_HOME if relocated) in the MCP server's env block.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**xapi-tutorial**](https://github.com/3cx/xapi-tutorial)  -  TypeScript (33 stars)
- [**3CX-XAPI_examples**](https://github.com/luxzg/3CX-XAPI_examples)  -  PowerShell (19 stars)
- [**3CX (PowerShell)**](https://github.com/O-IT/3CX)  -  PowerShell (3 stars)
- [**3CX_XAPI**](https://github.com/werddomain/3CX_XAPI)  -  C# (3 stars)
- [**3cx-xapi (printing-press-library)**](https://github.com/mvanhorn/printing-press-library/tree/main/library/social-and-messaging/3cx-xapi)  -  Go
- [**3cx-mcp-server**](https://github.com/SSIG-IT/3cx-mcp-server)  -  TypeScript
- [**3cx-cli**](https://github.com/getrav/3cx-cli)  -  Python

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
