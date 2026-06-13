# Microsoft Graph CLI

**The maintained single-binary successor to the retiring mgc  -  every MSP-relevant Microsoft Graph surface, plus an offline store that finds wasted licenses, privileged-access risks, and stale devices no single API call can.**

Microsoft is retiring the Microsoft Graph CLI (mgc) in August 2026, leaving M365 admins and MSPs without a lightweight, scriptable replacement scoped to the directory, security, licensing, and device core. This is that replacement: one cross-platform Go binary (no .NET or PowerShell runtime), with a local SQLite store that powers cross-entity answers  -  licenses waste, admins audit, security triage, managed-devices drift, tenant snapshot  -  that no single Graph endpoint returns.

## Install

The recommended path installs both the `microsoft-graph-cli` binary and the `pp-microsoft-graph` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install microsoft-graph
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install microsoft-graph --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install microsoft-graph --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install microsoft-graph --agent claude-code
npx -y @mvanhorn/printing-press-library install microsoft-graph --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/cloud/microsoft-graph/cmd/microsoft-graph-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/microsoft-graph-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install microsoft-graph --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-microsoft-graph --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-microsoft-graph --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install microsoft-graph --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/microsoft-graph-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `MICROSOFT_GRAPH_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/cloud/microsoft-graph/cmd/microsoft-graph-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

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

## Authentication

Microsoft Graph uses OAuth2 bearer tokens. For unattended MSP use, run `auth login --tenant <tenant-id> --client-id <app-id> --client-secret <secret>` to mint and cache an app-only token via the client-credentials flow. Alternatively, export a pre-minted token as `MICROSOFT_GRAPH_TOKEN` (for example from `az account get-access-token --scope https://graph.microsoft.com/.default --query accessToken -o tsv` or Graph Explorer). Read scopes such as User.Read.All, Directory.Read.All, RoleManagement.Read.Directory, SecurityAlert.Read.All, and DeviceManagementManagedDevices.Read.All must be granted and admin-consented on the app registration.

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

These capabilities aren't available in any other tool for this API.

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

### security

Microsoft Defender / Sentinel security alerts and incidents

- **`microsoft-graph-cli security alert`** - Get a security alert by id
- **`microsoft-graph-cli security alerts`** - List security alerts (alerts_v2)
- **`microsoft-graph-cli security incident`** - Get a security incident by id
- **`microsoft-graph-cli security incidents`** - List security incidents

### users

Entra ID (Azure AD) users  -  list, get, mail, and license details

- **`microsoft-graph-cli users get`** - Get a user by object id or userPrincipalName
- **`microsoft-graph-cli users licenses`** - List the SKUs/licenses assigned to a user
- **`microsoft-graph-cli users list`** - List users in the tenant
- **`microsoft-graph-cli users me`** - Get the signed-in user (delegated tokens only; app-only tokens have no /me)
- **`microsoft-graph-cli users messages`** - List a user's mail messages


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
microsoft-graph-cli devices list

# JSON for scripting and agents
microsoft-graph-cli devices list --json

# Filter to specific fields
microsoft-graph-cli devices list --json --select id,name,status

# Dry run  -  show the request without sending
microsoft-graph-cli devices list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
microsoft-graph-cli devices list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
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

Config file: `~/.config/microsoft-graph-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `MICROSOFT_GRAPH_TOKEN` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `microsoft-graph-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

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

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**cli-microsoft365 (m365)**](https://github.com/pnp/cli-microsoft365)  -  JavaScript (1300 stars)
- [**msgraph-sdk-powershell**](https://github.com/microsoftgraph/msgraph-sdk-powershell)  -  PowerShell (700 stars)
- [**msgraph-cli (mgc)**](https://github.com/microsoftgraph/msgraph-cli)  -  C# (600 stars)
- [**msgraph-sdk-go**](https://github.com/microsoftgraph/msgraph-sdk-go)  -  Go (600 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
