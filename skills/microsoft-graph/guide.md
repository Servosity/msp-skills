# Microsoft Graph CLI

**The maintained single-binary successor to the retiring mgc  -  every MSP-relevant Microsoft Graph surface, plus an offline store that finds wasted licenses, privileged-access risks, and stale devices no single API call can.**

Microsoft is retiring the Microsoft Graph CLI (mgc) in August 2026, leaving M365 admins and MSPs without a lightweight, scriptable replacement scoped to the directory, security, licensing, and device core. This is that replacement: one cross-platform Go binary (no .NET or PowerShell runtime), with a local SQLite store that powers cross-entity answers  -  licenses waste, admins audit, security triage, managed-devices drift, tenant snapshot  -  that no single Graph endpoint returns.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `microsoft-graph-cli` and `microsoft-graph-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.ps1 | iex
   ```
3. Verify: `microsoft-graph-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=microsoft-graph). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/microsoft-graph --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/microsoft-graph --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `microsoft-graph-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the microsoft-graph skill from https://github.com/Servosity/msp-skills/tree/main/skills/microsoft-graph. The skill defines how its required CLI (`microsoft-graph-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=microsoft-graph).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `MICROSOFT_GRAPH_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/microsoft-graph/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "microsoft-graph": {
      "command": "microsoft-graph-mcp",
      "env": {
        "MICROSOFT_GRAPH_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

### Remote MCP over HTTP

`microsoft-graph-mcp --transport http` (or `PP_MCP_TRANSPORT=http`) serves streamable HTTP at `http://127.0.0.1:7777/mcp`. It refuses to start until `MICROSOFT_GRAPH_MCP_HTTP_TOKEN` is set to at least 32 characters, and every request must send `Authorization: Bearer <token>`. The command below prints 64 hex characters and meets that minimum. Plain HTTP is allowed only on a loopback `--addr`; any other bind, including `:7777` (every interface), also needs `--tls-cert` and `--tls-key`. Requests with no `Origin` header are accepted, as are loopback origins (`localhost`, `127.0.0.1`, or `[::1]`, any port). Any other `Origin` is refused unless it is listed in `MICROSOFT_GRAPH_MCP_HTTP_ALLOWED_ORIGINS`, a comma-separated list of `scheme://host[:port]` values with no path. That list is not a CORS policy; the server does not emit CORS headers.

```bash
export MICROSOFT_GRAPH_MCP_HTTP_TOKEN="$(openssl rand -hex 32)"
microsoft-graph-mcp --transport http
```

Set `MICROSOFT_GRAPH_MCP_READ_ONLY=1` (exactly `1`) to drop every tool not marked read-only, including the local-write learn tools (`teach`, `teach_playbook`, `playbook_amend`, `learnings_confirm`) that write only this CLI's own learn store.

## Authentication

Microsoft Graph uses OAuth2 bearer tokens. For unattended MSP use, run `auth login --tenant <tenant-id> --client-id <app-id> --client-secret <secret>` to mint and cache an app-only token via the client-credentials flow. Alternatively, export a pre-minted token as `MICROSOFT_GRAPH_TOKEN` (for example from `az account get-access-token --scope https://graph.microsoft.com/.default --query accessToken -o tsv` or Graph Explorer). Read scopes such as User.Read.All, Directory.Read.All, RoleManagement.Read.Directory, SecurityAlert.Read.All, and DeviceManagementManagedDevices.Read.All must be granted and admin-consented on the app registration. Teams Phone reads need TeamsTelephoneNumber.Read.All and TeamsUserConfiguration.Read.All. The `transcripts` commands are own-tenant only, because an MSP must not read its clients' meetings: set MICROSOFT_GRAPH_HOME_TENANT to your own tenant id, and a token for any other tenant (a client tenant, or a GDAP or guest sign-in) is refused before a request is sent. They default to a delegated (signed-in user) token with OnlineMeetingTranscript.Read.All and read only meetings that user can already open; an app-only token is refused unless MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY=1 and a Teams application access policy (Grant-CsApplicationAccessPolicy) limits the app to named users. They keep nothing on disk, and MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS=1 removes them from the MCP server.

## Quick Start

```bash
# Confirm the token is present and Graph is reachable before anything else
microsoft-graph-cli doctor

# Mint and cache an app-only token (or export MICROSOFT_GRAPH_TOKEN instead)
microsoft-graph-cli auth login --tenant <tenant-id> --client-id <app-id> --client-secret <secret>

# Pull users, groups, roles, licenses, alerts, and devices into the local store, following @odata.nextLink to completion
microsoft-graph-cli pull

# See unused paid seats per SKU  -  usually the first MSP win
microsoft-graph-cli licenses waste --agent

# List who holds privileged directory roles right now
microsoft-graph-cli admins audit --agent

# Triage open alerts created in the last 24 hours by severity
microsoft-graph-cli security triage --since 24h --agent

```

## Unique Features

### License cost intelligence
- **`licenses waste`**  -  Surfaces every tenant SKU where you are paying for more seats than you use, ranked by unused seats.

  _Reach for this to find recoverable M365 license spend across a tenant in one call instead of exporting SKU CSVs from the admin center._

  ```bash
  microsoft-graph-cli licenses waste --agent
  ```
- **`licenses orphans`**  -  Lists disabled and guest accounts that still hold paid SKUs  -  licenses you are paying for but nobody is using.

  _Use before a license true-up to reclaim seats assigned to disabled or guest identities._

  ```bash
  microsoft-graph-cli licenses orphans --json
  ```
- **`licenses map`**  -  Lists every user consuming a specific SKU, with account-enabled state and guest flags, so you can plan seat reclamation and reassignment.

  _Reach for this when you need to know exactly who holds a given SKU before reclaiming or reassigning seats._

  ```bash
  microsoft-graph-cli licenses map ENTERPRISEPACK --agent
  ```

### Security & privileged-access
- **`admins audit`**  -  Lists every holder of a privileged directory role with the role name, account-enabled state, and guest/disabled risk flags.

  _Run this for the monthly privileged-access review  -  it is the fastest answer to 'who can administer this tenant right now'._

  ```bash
  microsoft-graph-cli admins audit --agent
  ```
- **`security triage`**  -  Counts the open security alerts created in a recent time window, grouped by severity and detection source.

  _Reach for this every morning to answer 'what is new and still open since yesterday' without portal pagination._

  ```bash
  microsoft-graph-cli security triage --since 24h --agent
  ```
- **`groups risk`**  -  Flags ownerless, empty, and guest-heavy groups across the tenant in one pass.

  _Use this for tenant governance reviews when no single Graph filter can surface risky groups._

  ```bash
  microsoft-graph-cli groups risk --agent
  ```
- **`apps consent`**  -  Inventories every non-Microsoft enterprise application consented into the tenant and the access it holds, risk-ranked with over-privileged, admin-consented, privilege-escalation and user-consented flags.

  _Run this for the quarterly app-consent review: it is the one-command answer to which third-party apps can read or change this tenant._

  ```bash
  microsoft-graph-cli apps consent --agent
  ```

### Device & tenant posture
- **`managed-devices drift`**  -  Flags Intune devices that are non-compliant, unencrypted, or have not checked in within a time window, attributed to their assigned user.

  _Use to build the weekly device-compliance ticket queue in one command instead of a portal-to-spreadsheet ETL._

  ```bash
  microsoft-graph-cli managed-devices drift --days 30 --json
  ```
- **`tenant snapshot`**  -  One agent-readable summary of the tenant: user and guest counts, license waste, admin count, open high-severity alerts, and non-compliant device count.

  _Reach for this first when you pick up a tenant  -  it is the 'where does this tenant stand' answer an MSP needs before drilling in._

  ```bash
  microsoft-graph-cli tenant snapshot --agent
  ```

### Teams meetings (own tenant)

Why own tenant only: an MSP's credentials can reach its clients' tenants (GDAP, multi-tenant apps), but a client's meetings are never the MSP's to read. So the `transcripts` commands need `MICROSOFT_GRAPH_HOME_TENANT` set to your own tenant id and refuse, before any request, a token for another tenant, a GDAP or guest sign-in, an unreadable token, and an app-only token unless `MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY=1` (with a Teams application access policy, `Grant-CsApplicationAccessPolicy`). They keep nothing on disk (no local store, response cache or learn journal). Through MCP they are read-only, return the text instead of writing a file, and `MICROSOFT_GRAPH_MCP_NO_TRANSCRIPTS=1` removes them.

- **`transcripts list`**  -  Lists the transcripts of one Teams meeting the signed-in user can already open, in the credential's own tenant only.

  _Use it to find the transcript of your own MSP's meeting; it refuses a client tenant by design._

  ```bash
  microsoft-graph-cli transcripts list --meeting <meeting-id> --agent
  ```
- **`transcripts download`**  -  Downloads one Teams meeting transcript as WebVTT to stdout or a named file, in the credential's own tenant only; nothing is kept on disk by the CLI.

  _Reach for this to pull your own meeting's transcript into notes or an agent without copying it out of Teams._

  ```bash
  microsoft-graph-cli transcripts download --meeting <meeting-id> --transcript <transcript-id> --out meeting.vtt
  ```
- **`transcripts all`**  -  Lists every transcript from the meetings one user organized, in the credential's own tenant only (Graph offers this to application tokens only, behind an explicit opt-in).

  _Use it to see which of your own team's recent meetings have transcripts, without opening each meeting._

  ```bash
  microsoft-graph-cli transcripts all --organizer <user-id> --since 7d --agent
  ```

## Recipes

### Find recoverable license spend

```bash
microsoft-graph-cli licenses waste --agent
```

Ranks SKUs by unused paid seats so you can right-size subscriptions at renewal.

### Monthly privileged-access review

```bash
microsoft-graph-cli admins audit --agent
```

Lists every directory-role holder with risk flags for guest or disabled admin accounts.

### Morning alert triage

```bash
microsoft-graph-cli security triage --since 24h --agent
```

Groups open alerts from the last day by severity and detection source.

### Device compliance ticket queue

```bash
microsoft-graph-cli managed-devices drift --days 30 --agent
```

Surfaces non-compliant, unencrypted, or stale-sync Intune devices mapped to their user.

### Trim a large user payload to just the fields you need

```bash
microsoft-graph-cli users list --top 50 --agent --select id,displayName,userPrincipalName,accountEnabled
```

Pairs --agent with --select to keep agent context small when a Graph user object would otherwise return dozens of properties.

## Usage

Run `microsoft-graph-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `MICROSOFT_GRAPH_CONFIG_DIR`, `MICROSOFT_GRAPH_DATA_DIR`, `MICROSOFT_GRAPH_STATE_DIR`, or `MICROSOFT_GRAPH_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `MICROSOFT_GRAPH_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export MICROSOFT_GRAPH_HOME=/srv/microsoft-graph
microsoft-graph-cli doctor
```

On Windows (PowerShell):

```powershell
$env:MICROSOFT_GRAPH_HOME = "C:\ProgramData\microsoft-graph"
microsoft-graph-cli doctor
```

Under `MICROSOFT_GRAPH_HOME=/srv/microsoft-graph`, the four dirs resolve to `/srv/microsoft-graph/config`, `/srv/microsoft-graph/data`, `/srv/microsoft-graph/state`, and `/srv/microsoft-graph/cache`.

On Windows, set the same variable in PowerShell:

```powershell
$env:MICROSOFT_GRAPH_HOME = "C:\srv\microsoft-graph"
microsoft-graph-cli doctor
```

Under `$env:MICROSOFT_GRAPH_HOME = "C:\srv\microsoft-graph"`, the four dirs resolve to `C:\srv\microsoft-graph\config`, `C:\srv\microsoft-graph\data`, `C:\srv\microsoft-graph\state`, and `C:\srv\microsoft-graph\cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "microsoft-graph": {
      "command": "microsoft-graph-mcp",
      "env": {
        "MICROSOFT_GRAPH_HOME": "/srv/microsoft-graph"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `MICROSOFT_GRAPH_DATA_DIR` overrides an explicit `--home` for that kind. Use `MICROSOFT_GRAPH_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `MICROSOFT_GRAPH_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `microsoft-graph-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

### Local data

Reads are saved locally by default: in `--data-source auto` mode every successful read is also written to the SQLite store (`data.db`, or `data-<hash>.db` per credential) in the data directory. GET responses are never cached on disk. Credentials live in `credentials.toml` in the data directory. The learn journal lives under `learn/` in the state directory. `microsoft-graph-cli doctor` prints the resolved paths, every store file, and the response-cache size.

None of it is encrypted. New files are created owner-only; beyond that the data relies on OS account permissions and full-disk encryption (FileVault, BitLocker, LUKS).

- **Erase it:** `microsoft-graph-cli local-data purge` lists the store files, response cache, and learn journal it would delete; add `--yes` to delete them, or `--only store|cache|journal` to narrow it. Config and credentials are kept.
- **Run without a local copy:** set `MICROSOFT_GRAPH_NO_LOCAL_DATA=1`. Reads go to the API (`--data-source live`; `--data-source local` still reads an existing store), the response cache is off (`--no-cache`), nothing new is written to the store, and `sync` and `workflow archive` refuse to run. Set it in an MCP host's `env` block and every command the server runs inherits it.
- **Choose the read source without flags:** `MICROSOFT_GRAPH_DATA_SOURCE=auto|live|local` sets the `--data-source` default; an explicit flag wins.

## Commands

### devices

Entra ID registered/joined device objects

- **`microsoft-graph-cli devices get`** - Get an Entra device by object id
- **`microsoft-graph-cli devices list`** - List Entra-registered devices

### directory-roles

Entra ID directory roles (admin roles) and their members

- **`microsoft-graph-cli directory-roles get`** - Get a directory role by object id
- **`microsoft-graph-cli directory-roles list`** - List activated directory roles in the tenant
- **`microsoft-graph-cli directory-roles members`** - List the members assigned to a directory role

### groups

Entra ID groups  -  list, get, members, and owners

- **`microsoft-graph-cli groups get`** - Get a group by object id
- **`microsoft-graph-cli groups list`** - List groups in the tenant
- **`microsoft-graph-cli groups members`** - List a group's members
- **`microsoft-graph-cli groups owners`** - List a group's owners

### licenses

Tenant commercial subscriptions (subscribedSkus)

- **`microsoft-graph-cli licenses sku`** - Get a single subscribed SKU by id
- **`microsoft-graph-cli licenses skus`** - List the commercial subscriptions (SKUs) the tenant owns

### managed-devices

Intune-managed devices and their compliance posture

- **`microsoft-graph-cli managed-devices get`** - Get an Intune-managed device by id
- **`microsoft-graph-cli managed-devices list`** - List Intune-managed devices (requires an Intune license)

### phone-numbers

Teams Phone telephone numbers in the tenant and what each is assigned to (read-only)

- **`microsoft-graph-cli phone-numbers get`** - Get one Teams telephone number assignment by its id (needs TeamsTelephoneNumber.Read.All)
- **`microsoft-graph-cli phone-numbers list`** - List the tenant's Teams telephone numbers with type, activation and assignment (needs TeamsTelephoneNumber.Read.All)

### security

Microsoft Defender / Sentinel security alerts and incidents

- **`microsoft-graph-cli security alert`** - Get a security alert by id
- **`microsoft-graph-cli security alerts`** - List security alerts (alerts_v2)
- **`microsoft-graph-cli security incident`** - Get a security incident by id
- **`microsoft-graph-cli security incidents`** - List security incidents

### teams-users

Teams user configuration: phone numbers, Enterprise Voice and effective Teams policy assignments per user (read-only)

- **`microsoft-graph-cli teams-users get`** - Get one user's Teams configuration: numbers, voice enablement and effective policy assignments (needs TeamsUserConfiguration.Read.All)
- **`microsoft-graph-cli teams-users list`** - List Teams user configurations: numbers, voice enablement and effective policy assignments (needs TeamsUserConfiguration.Read.All)

### users

Entra ID (Azure AD) users  -  list, get, mail, and license details

- **`microsoft-graph-cli users get`** - Get a user by object id or userPrincipalName
- **`microsoft-graph-cli users licenses`** - List the SKUs/licenses assigned to a user
- **`microsoft-graph-cli users list`** - List users in the tenant
- **`microsoft-graph-cli users me`** - Get the signed-in user (delegated tokens only; app-only tokens have no /me)
- **`microsoft-graph-cli users messages`** - List a user's mail messages


### admins


- **`microsoft-graph-cli admins audit`** - Lists every holder of a privileged directory role with the role name, account-enabled state, and guest/disabled risk flags.


### apps


- **`microsoft-graph-cli apps consent`** - Inventories every non-Microsoft enterprise application consented into the tenant and the access it holds, risk-ranked with over-privileged, admin-consented, privilege-escalation and user-consented flags.


### groups


- **`microsoft-graph-cli groups risk`** - Flags ownerless, empty, and guest-heavy groups across the tenant in one pass.


### licenses


- **`microsoft-graph-cli licenses waste`** - Surfaces every tenant SKU where you are paying for more seats than you use, ranked by unused seats.

- **`microsoft-graph-cli licenses orphans`** - Lists disabled and guest accounts that still hold paid SKUs  -  licenses you are paying for but nobody is using.

- **`microsoft-graph-cli licenses map`** - Lists every user consuming a specific SKU, with account-enabled state and guest flags, so you can plan seat reclamation and reassignment.


### managed-devices


- **`microsoft-graph-cli managed-devices drift`** - Flags Intune devices that are non-compliant, unencrypted, or have not checked in within a time window, attributed to their assigned user.


### security


- **`microsoft-graph-cli security triage`** - Counts the open security alerts created in a recent time window, grouped by severity and detection source.


### tenant


- **`microsoft-graph-cli tenant snapshot`** - One agent-readable summary of the tenant: user and guest counts, license waste, admin count, open high-severity alerts, and non-compliant device count.


### transcripts


- **`microsoft-graph-cli transcripts list`** - Lists the transcripts of one Teams meeting the signed-in user can already open, in the credential's own tenant only.

- **`microsoft-graph-cli transcripts download`** - Downloads one Teams meeting transcript as WebVTT to stdout or a named file, in the credential's own tenant only; nothing is kept on disk by the CLI.

- **`microsoft-graph-cli transcripts all`** - Lists every transcript from the meetings one user organized, in the credential's own tenant only (Graph offers this to application tokens only, behind an explicit opt-in).




### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`microsoft-graph-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`microsoft-graph-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`microsoft-graph-cli learnings list`** - Inspect taught rows
- **`microsoft-graph-cli learnings forget <query>`** - Undo a teach
- **`microsoft-graph-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`microsoft-graph-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`microsoft-graph-cli teach-pattern`** - Install a query/resource template up front
- **`microsoft-graph-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `MICROSOFT_GRAPH_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `microsoft-graph-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
microsoft-graph-cli devices list

# JSON for scripting and agents
microsoft-graph-cli devices list --json
# Filter to specific fields
microsoft-graph-cli devices list --json --select id,displayName,operatingSystem

# Dry run  -  show the request without sending
microsoft-graph-cli devices list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
microsoft-graph-cli devices list --agent
```

`--csv` prefixes a leading `'` on cells and header keys that start with `=`, `+`, `-`, `@`, a tab, or a line break (including fullwidth `＝＋－＠`) so a spreadsheet does not run them as formulas. Plain signed numbers such as `-5` and `+2.5e3` are left unchanged.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
microsoft-graph-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `microsoft-graph-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/microsoft-graph-cli/config.toml` on macOS/Linux and `%USERPROFILE%\.config\microsoft-graph-cli\config.toml` on Windows; `--home`, `MICROSOFT_GRAPH_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

`--ca-cert`, `MICROSOFT_GRAPH_CA_CERT`, or `ca_cert` adds a PEM CA bundle to the system trust roots. `--tls-server-name`, `MICROSOFT_GRAPH_TLS_SERVER_NAME`, or `tls_server_name` is the name checked against the certificate when the dialed address does not match it. Precedence is flag, then environment, then the config file. A config file other users can change does not supply these settings. Certificate verification failures are not retried.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `MICROSOFT_GRAPH_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `microsoft-graph-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Credential safety

- Requests go only to the scheme, host and port of `MICROSOFT_GRAPH_BASE_URL` (default `https://graph.microsoft.com/v1.0`): a redirect, or an `@odata.nextLink`, naming anywhere else is refused or fetched from the configured host, so the token never reaches another host. `auth login` sends the client secret only to `login.microsoftonline.com` and refuses a redirect.
- Any output line, file, local-store record, MCP result or outbound request that would carry the token, the client secret or a credential from a response is withheld whole, and "[credkit: withheld from <where>: it carried a credential this program holds]" prints in its place. Responses are never cached on disk.
- Only `credentials.toml` (mode 0600) holds a credential; `auth login` and `auth set-token` write it.
- MCP tools only read: any tool that could change the tenant previews and runs only with `confirm: true`, and a tool argument can never name a file to read or write.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `microsoft-graph-cli doctor` to check credentials
- Verify the environment variable is set: `echo $MICROSOFT_GRAPH_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  The token is missing or expired (Graph tokens last ~60-75 min). Run `auth login` again or re-export MICROSOFT_GRAPH_TOKEN.
- **403 Forbidden / Authorization_RequestDenied**  -  The app or user lacks the required read scope. Grant and admin-consent the scope (e.g. SecurityAlert.Read.All, DeviceManagementManagedDevices.Read.All) on the app registration.
- **managed-devices commands return empty**  -  Intune commands need an active Intune license on the tenant. Verify the tenant has Intune before expecting managed-device data.
- **advanced $filter or $search rejected (eventual consistency required)**  -  Keep --filter to eq/startsWith/date comparisons, or use the offline `search` and `analytics` commands on synced data instead of advanced Graph queries.
- **/me returns an error with an app-only token**  -  App-only (client-credentials) tokens have no signed-in user. Use `users get <id-or-upn>` instead of `users me`.
- **transcripts refuses with 'own tenant only'**  -  MICROSOFT_GRAPH_HOME_TENANT is unset, the token was issued for another tenant (a client tenant, or a GDAP or guest sign-in), or it is app-only without the opt-in. Set MICROSOFT_GRAPH_HOME_TENANT to your own tenant id and use a delegated token from a member account of your own tenant, or set MICROSOFT_GRAPH_TRANSCRIPTS_APP_ONLY=1 after granting a Teams application access policy.
- **transcripts returns 403 GraphAccessToTranscriptsDisabled or SpeakerAttributionNotAllowed**  -  A Teams administrator turned off Graph access to transcripts (no workaround) or speaker attribution (download retries without speaker names when you pass --no-speakers).

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**cli-microsoft365 (m365)**](https://github.com/pnp/cli-microsoft365)  -  JavaScript (1300 stars)
- [**msgraph-sdk-powershell**](https://github.com/microsoftgraph/msgraph-sdk-powershell)  -  PowerShell (700 stars)
- [**msgraph-cli (mgc)**](https://github.com/microsoftgraph/msgraph-cli)  -  C# (600 stars)
- [**msgraph-sdk-go**](https://github.com/microsoftgraph/msgraph-sdk-go)  -  Go (600 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
