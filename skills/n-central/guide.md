# N Central CLI

N-able N-central RMM REST API  -  manage devices, customers, sites, org units, active issues, custom properties, scheduled tasks, and maintenance windows across an MSP's N-central instance.

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `n-central-cli` binary and the `pp-n-central` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install n-central
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install n-central --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install n-central --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install n-central --agent claude-code
npx -y @mvanhorn/printing-press-library install n-central --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/developer-tools/n-central/cmd/n-central-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/n-central-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install n-central --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-n-central --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-n-central --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install n-central --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/n-central-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `NCENTRAL_JWT` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/developer-tools/n-central/cmd/n-central-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "n-central": {
      "command": "n-central-mcp",
      "env": {
        "NCENTRAL_JWT": "<your-key>"
      }
    }
  }
}
```

</details>

## Quick Start

### 1. Install

See [Install](#install) above.

### 2. Set Up Credentials

Get your access token from your API provider's developer portal, then store it:

```bash
n-central-cli auth set-token YOUR_TOKEN_HERE
```

Or set it via environment variable:

```bash
export NCENTRAL_JWT="your-token-here"
```

### 3. Verify Setup

```bash
n-central-cli doctor
```

This checks your configuration and credentials.

### 4. Try Your First Command

```bash
n-central-cli access-groups <id>
```

## Usage

Run `n-central-cli --help` for the full command reference and flag list.

## Commands

### access-groups

Access groups (device-type and org-unit-type).

- **`n-central-cli access-groups <accessGroupId>`** - Retrieve detailed information for an access group by ID.

### customers

Customers (client organizations) in N-central.

- **`n-central-cli customers get`** - Retrieve a single customer by ID.
- **`n-central-cli customers list`** - List all customers across the instance.
- **`n-central-cli customers registration-token`** - Retrieve the agent registration token for a customer (used to enroll new devices).

### device-filters

Saved device filters (reusable as filterId on device list calls).

- **`n-central-cli device-filters`** - List saved device filters for the API user.

### devices

Devices monitored by N-central (workstations, servers, network devices, probes).

- **`n-central-cli devices assets`** - Retrieve hardware/software asset inventory for a device.
- **`n-central-cli devices get`** - Retrieve a single device by ID.
- **`n-central-cli devices list`** - List all devices visible to the API user, across the org tree.
- **`n-central-cli devices maintenance`** - List patch maintenance windows configured for a device.
- **`n-central-cli devices properties`** - List custom property values for a device (the backbone of MSP automation/documentation).
- **`n-central-cli devices status`** - Retrieve the service-monitoring status (active issues / health) for a device.
- **`n-central-cli devices tasks`** - List scheduled/automation tasks targeting this device.

### org-units

Organization units  -  the unified tree of service orgs, customers, and sites.

- **`n-central-cli org-units access-groups`** - List access groups for an org unit.
- **`n-central-cli org-units active-issues`** - Fetch active monitoring issues for an org unit (the daily NOC triage feed).
- **`n-central-cli org-units children`** - List the direct children of an org unit.
- **`n-central-cli org-units devices`** - List devices scoped to a specific org unit.
- **`n-central-cli org-units get`** - Retrieve a single org unit by ID.
- **`n-central-cli org-units job-statuses`** - Fetch job statuses for an org unit.
- **`n-central-cli org-units list`** - List all organization units (SO, customer, and site nodes).
- **`n-central-cli org-units registration-token`** - Retrieve the agent registration token for an org unit.
- **`n-central-cli org-units user-roles`** - List user roles defined for an org unit.

### scheduled-tasks

Scheduled tasks  -  run scripts/automation policies on devices and track them.

- **`n-central-cli scheduled-tasks get`** - Retrieve general information for a scheduled task.
- **`n-central-cli scheduled-tasks run`** - Create a direct-support scheduled task (run an Automation Policy, Script, or MacScript on a device).
- **`n-central-cli scheduled-tasks status`** - Retrieve aggregated status for a scheduled task.

### server

Server info and health.

- **`n-central-cli server health`** - Return the start and current time of the server (lightweight reachability check).
- **`n-central-cli server info`** - Return version information for the N-central API service and systems.

### service-orgs

Service Organizations  -  the top level of the N-central org tree.

- **`n-central-cli service-orgs customers`** - List all customers under a service organization.
- **`n-central-cli service-orgs get`** - Retrieve a single service organization by ID.
- **`n-central-cli service-orgs list`** - List all service organizations.

### sites

Sites  -  the leaf org-unit level under customers.

- **`n-central-cli sites get`** - Retrieve a single site by ID.
- **`n-central-cli sites list`** - List all sites across the instance.

### users

N-central users.

- **`n-central-cli users`** - List N-central users.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
n-central-cli access-groups <id>

# JSON for scripting and agents
n-central-cli access-groups <id> --json

# Filter to specific fields
n-central-cli access-groups <id> --json --select id,name,status

# Dry run  -  show the request without sending
n-central-cli access-groups <id> --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
n-central-cli access-groups <id> --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
n-central-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/n-central-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `NCENTRAL_JWT` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `n-central-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `n-central-cli doctor` to check credentials
- Verify the environment variable is set: `echo $NCENTRAL_JWT`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

---

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
