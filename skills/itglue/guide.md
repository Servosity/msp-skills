# IT Glue CLI

**Every IT Glue resource, plus an offline SQLite mirror, fleet-wide cross-resource search, and documentation-hygiene analytics no other IT Glue tool offers.**

IT Glue's API is a thin per-call JSON:API surface guarded by a 3000-requests/5-minute rate ceiling, and every existing tool just wraps one endpoint at a time. This CLI mirrors your organizations, contacts, passwords, configurations, and documents into a local SQLite store, then answers the questions the live API can't: `search` finds anything across every client at once, `coverage` ranks clients by documentation completeness, and `passwords stale` audits credential rotation fleet-wide without burning your rate budget.

Learn more at [IT Glue](https://api.itglue.com/developer/).

Created by [@dstevens](https://github.com/dstevens) (Damien Stevens).
Contributors: [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `itglue-cli` binary and the `pp-itglue` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install itglue
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install itglue --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install itglue --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install itglue --agent claude-code
npx -y @mvanhorn/printing-press-library install itglue --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/productivity/itglue/cmd/itglue-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/itglue-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install itglue --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-itglue --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-itglue --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install itglue --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/itglue-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `ITGLUE_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/productivity/itglue/cmd/itglue-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "itglue": {
      "command": "itglue-mcp",
      "env": {
        "ITGLUE_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Authentication is a single IT Glue API key sent as the `x-api-key` header. Set `ITGLUE_API_KEY` in your environment, then pick your data region by setting `ITGLUE_BASE_URL` (or `base_url` in `~/.config/itglue-cli/config.toml`) to US `https://api.itglue.com`, EU `https://api.eu.itglue.com`, or AU `https://api.au.itglue.com`. Generate the key in IT Glue account settings; it needs read (and, for create/update commands, write) permissions. Verify with `doctor`.

## Quick Start

```bash
# confirm the API key, region base URL, and reachability before anything else
itglue-cli doctor

# mirror the tenant into the local SQLite store so reads stop hitting the rate ceiling
itglue-cli sync --full

# fleet-wide search across every synced resource and client
itglue-cli search "Fortinet"

# surface organizations missing a whole documentation category
itglue-cli coverage --below 1

# credential rotation audit across all clients, oldest-first
itglue-cli passwords stale --days 365

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Fleet-wide answers from the local mirror
- **`search`**  -  Search every synced organization, contact, password (metadata), configuration, and document at once, with the owning client shown for each hit.

  _When a ticket only gives you a serial number, device name, or contact, reach for this to find which client it belongs to without paging the live API across every tenant._

  ```bash
  itglue-cli search "Fortinet" --agent
  ```
- **`changes`**  -  Show every record updated since a timestamp across all five resources, newest first, optionally narrowed to one resource type.

  _Use for a weekly remediation sweep to see what changed across the fleet since your last review._

  ```bash
  itglue-cli changes --since 2026-05-01 --agent
  ```

### Documentation & credential hygiene
- **`coverage`**  -  Rank organizations by documentation completeness  -  counts of configurations, contacts, passwords, and documents per client, thinnest first.

  _Use before a QBR or onboarding review to surface clients with missing runbooks, contacts, or credentials before they complain._

  ```bash
  itglue-cli coverage --below 1 --agent
  ```
- **`passwords stale`**  -  List credentials whose last update is older than a threshold, grouped by client and sorted oldest-first  -  metadata only, the secret value is never read.

  _Use for SOC2 / cyber-insurance credential-hygiene attestation and to generate rotation tickets across all clients in one pass._

  ```bash
  itglue-cli passwords stale --days 365 --agent
  ```
- **`contacts dupes`**  -  Find contacts that share a normalized name or email within (or across) an organization  -  the duplicates that overlapping PSA syncs leave behind.

  _Run after a PSA contact sync to clean up duplicate people before they pollute client documentation._

  ```bash
  itglue-cli contacts dupes --org 12345 --agent
  ```
- **`orphans`**  -  Find configurations, contacts, passwords, and documents whose organization-id no longer resolves to a synced organization.

  _Run after client offboarding or org renames to catch documentation left dangling before it misleads a technician._

  ```bash
  itglue-cli orphans --agent
  ```

### One-call relational views
- **`org show`**  -  Assemble everything known about one client  -  contacts, configurations, password metadata, and documents  -  in a single offline read.

  _Use at the start of a client call or ticket to pull the full picture instantly instead of clicking through the web UI._

  ```bash
  itglue-cli org show 12345 --agent
  ```

## Recipes


### Mirror then audit credential hygiene

```bash
itglue-cli sync --full && itglue-cli passwords stale --days 180 --agent
```

Sync once, then list every password not rotated in six months across all clients without touching the live API again.

### Narrow a verbose configuration list for an agent

```bash
itglue-cli configurations list --agent --select name,serial-number,primary-ip,configuration-type-name
```

Configuration records carry dozens of attributes; --select with --agent returns only the fields an agent needs, keeping the payload small.

### Find which client a device belongs to

```bash
itglue-cli search "SN-ABC123" --agent
```

One FTS query maps a serial number to its owning organization and resource, no per-org clicking.

### Pre-QBR documentation sweep

```bash
itglue-cli coverage --below 1 --agent
```

List clients missing an entire documentation category so you can assign remediation before the review.

### Preview a write before it lands

```bash
itglue-cli contacts create --data-attributes-organization-id 12345 --data-attributes-first-name Jane --data-attributes-last-name Doe --dry-run
```

Print the exact JSON:API payload that would be sent without creating anything.

## Usage

Run `itglue-cli --help` for the full command reference and flag list.

## Commands

### configurations

Manage configurations

- **`itglue-cli configurations create`** - Create a configuration
- **`itglue-cli configurations get`** - Retrieve a configuration
- **`itglue-cli configurations list`** - Use stable fields such as organization id, name, hostname, serial number, or asset tag before creating records.
- **`itglue-cli configurations update`** - Update a configuration

### contacts

Manage contacts

- **`itglue-cli contacts create`** - Idempotent callers should first search by organization id and email address.
- **`itglue-cli contacts get`** - Retrieve a contact
- **`itglue-cli contacts list`** - Use filter[email] or filter[organization_id] before creating contacts to avoid duplicates.
- **`itglue-cli contacts update`** - Use PATCH for contact deactivation; this spec intentionally does not expose DELETE.

### documents

Manage documents

- **`itglue-cli documents create`** - Create a document
- **`itglue-cli documents get`** - Retrieve a document
- **`itglue-cli documents list`** - List documents
- **`itglue-cli documents update`** - Update a document

### organizations

Manage organizations

- **`itglue-cli organizations get`** - Retrieve one organization
- **`itglue-cli organizations list`** - List organizations

### passwords

Manage passwords

- **`itglue-cli passwords create`** - Creates either a general password or an embedded password when resource-id and resource-type are supplied.
- **`itglue-cli passwords get`** - Retrieve a password metadata record
- **`itglue-cli passwords list`** - Use filter[name] plus filter[organization_id] before creating password mirror records.
- **`itglue-cli passwords update`** - Update a password


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
itglue-cli configurations list

# JSON for scripting and agents
itglue-cli configurations list --json

# Filter to specific fields
itglue-cli configurations list --json --select id,name,status

# Dry run  -  show the request without sending
itglue-cli configurations list --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
itglue-cli configurations list --agent
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
itglue-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/it-glue-myglue-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `ITGLUE_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `itglue-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `itglue-cli doctor` to check credentials
- Verify the environment variable is set: `echo $ITGLUE_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **HTTP 401 / unauthorized**  -  Set a valid key: export ITGLUE_API_KEY=<your-key>, then re-run doctor.
- **HTTP 429 Too Many Requests**  -  You hit the 3000-per-5-minute ceiling; sync once then read from the local store instead of looping live calls.
- **Empty results from search or coverage**  -  Run sync --full first  -  those commands read the local store, not the live API.
- **404 / wrong region**  -  Point at your data centre with --base-url https://api.eu.itglue.com (or api.au.itglue.com).

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**Celerium/ITGlue-PowerShellWrapper**](https://github.com/Celerium/ITGlue-PowerShellWrapper)  -  PowerShell
- [**itglue/powershellwrapper**](https://github.com/itglue/powershellwrapper)  -  PowerShell
- [**wyre-technology/itglue-mcp**](https://github.com/wyre-technology/itglue-mcp)  -  TypeScript
- [**junto-platforms/itglue-mcp-server**](https://github.com/junto-platforms/itglue-mcp-server)  -  TypeScript
- [**b-loyola/itglue-py**](https://github.com/b-loyola/itglue-py)  -  Python
- [**KelvinTegelaar/AzGlue**](https://github.com/KelvinTegelaar/AzGlue)  -  PowerShell
- [**CalebAlbers/IT-Glue-API-PowerShell-Wrapper**](https://github.com/CalebAlbers/IT-Glue-API-PowerShell-Wrapper)  -  PowerShell

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
