# Afi CLI

**The first CLI for Afi SaaS backup  -  full public-API coverage plus the fleet-wide coverage, staleness, and offboarding answers the rate-limited API can't serve live.**

Afi has no other CLI, SDK, or MCP server  -  the only alternative is the web portal, and the vendor explicitly discourages API polling. This CLI walks the whole hierarchy into local SQLite once (fleet-sync), then answers fleet-wide questions (coverage-gaps, fleet-health, backup-stale, reconcile-licenses) offline. A guarded offboard command runs the vendor's own archive-then-release sequence with a verification gate the portal lacks.

Learn more at [Afi](https://afi.ai/docs/).

Created by [@DamienStevens](https://github.com/DamienStevens) (Damien Stevens).

## Install

The recommended path installs both the `afi-cli` binary and the `pp-afi` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install afi
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install afi --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install afi --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install afi --agent claude-code
npx -y @mvanhorn/printing-press-library install afi --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/afi/cmd/afi-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/afi-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install afi --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-afi --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-afi --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install afi --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/afi-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `AFI_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/afi/cmd/afi-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "afi": {
      "command": "afi-mcp",
      "env": {
        "AFI_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Create an Application and API key in the Afi portal (org level: Configuration → Apps; tenant level: Service → Settings → Apps), then set AFI_API_KEY. The key is sent as the raw Authorization header value (Afi keys look like appkey-...; no Bearer prefix). Each Application supports two keys for rotation. Keys inherit the Application's installation scope  -  if a tenant or org is missing from results, check where the Application is installed.

## Quick Start

```bash
# Verify the binary, config, and connectivity plan before touching the API
afi-cli doctor --dry-run

# One respectful hierarchy walk pulls orgs, tenants, resources, protections, policies, archives, quotas, and task stats into local SQLite  -  then every question below runs offline
afi-cli fleet-sync

# The Monday sweep: task failures and quota breaches across every tenant in one table
afi-cli fleet-health --json

# List resources with no backup protection  -  the blind spots
afi-cli coverage-gaps --json

# Catch protected resources whose backups quietly stopped landing
afi-cli backup-stale --max-age 48h --json

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`fleet-sync`**  -  Walk the whole Afi hierarchy  -  installations, orgs, tenants, then each tenant's resources, protections, policies, archives, quotas, and task stats  -  into local SQLite in one respectful pass.

  _Run this first (and on a schedule): every fleet question afterward  -  coverage, staleness, licensing, scorecards  -  answers offline without tripping Afi's rate limits._

  ```bash
  afi-cli fleet-sync
  ```
- **`coverage-gaps`**  -  Find resources in a tenant that have no backup protection applied  -  the backup blind spots.

  _Reach for this when asked whether every mailbox, site, or drive is actually being backed up  -  it answers fleet-coverage questions one API can't._

  ```bash
  afi-cli coverage-gaps --tenant 01F000000000000411Z1101G1Y --agent
  ```
- **`fleet-health`**  -  One table across all synced tenants: task success/failure counts, failed-task list, and quota breaches.

  _Use this for the Monday-morning 'is the whole fleet green' sweep instead of walking tenants one portal tab at a time._

  ```bash
  afi-cli fleet-health --since 24h --agent
  ```
- **`resolve`**  -  Map a Microsoft 365 / Google Workspace ID, email, or name to the canonical Afi resource or tenant, including Multi-Geo fan-out.

  _Run this before any offboard or audit to be certain which Afi object corresponds to an external-system identity._

  ```bash
  afi-cli resolve jane.doe@example.com --agent
  ```
- **`reconcile-licenses`**  -  Compare purchased subscription quantities against actually-protected resource counts per tenant to flag over- and under-provisioning.

  _Use this for the weekly licensing drift check that today requires manual spreadsheet stitching across two portal screens._

  ```bash
  afi-cli reconcile-licenses --org 01F0ORG0000000000000000000 --json
  ```
- **`backup-stale`**  -  Flag protected resources whose most recent backup archive is older than a threshold  -  protected-but-silently-failing resources.

  _Use this to catch the silent-failure class: resources that have a policy attached but whose backups quietly stopped landing._

  ```bash
  afi-cli backup-stale --max-age 48h --agent
  ```
- **`tenant-scorecard`**  -  One tenant's full backup posture: coverage percentage, last task status, newest/oldest archive age, and quota state.

  _Generate this card when prepping a customer ticket or QBR  -  the per-tenant deep dive that complements fleet-health._

  ```bash
  afi-cli tenant-scorecard 01F000000000000411Z1101G1Y --agent --select tenant_id,coverage_pct,quotas_exceeded
  ```

### Guarded workflows
- **`offboard`**  -  Safely back up a departing user's resource, verify the archive landed, then release the protection  -  refusing the irreversible step until the backup is confirmed.

  _Use this when offboarding an employee so the final backup is verified before the M365/GWS seat is released  -  never lose a leaver's data._

  ```bash
  afi-cli offboard 01F0RESOURCE0000000000000A --tenant 01F0TENANT00000000000000B --policy 01F0POLICY000000000000000C --reason "employee departure"
  ```

## Recipes


### Monday fleet sweep

```bash
afi-cli fleet-sync --skip-archives && afi-cli fleet-health --failed-only --agent
```

Refresh the fleet store (skipping the heavy archives walk), then list only the tenants with failed tasks or quota breaches.

### Find backup blind spots in one tenant

```bash
afi-cli coverage-gaps --tenant 01F000000000000411Z1101G1Y --agent --select items.name,items.kind,items.external_id
```

LEFT JOIN of resources against protections, narrowed to the three fields an agent needs to open tickets.

### Offboard a departed employee safely

```bash
afi-cli resolve jane.doe@example.com && afi-cli offboard 01F0RESOURCE0000000000000A --tenant 01F0TENANT00000000000000B --policy 01F0POLICY000000000000000C --reason "employee departure" --dry-run
```

Resolve the ambiguous email to the canonical Afi resource, then preview the guarded final-backup-verify-release sequence before running it for real.

### Catch silently failing backups

```bash
afi-cli backup-stale --max-age 48h --json
```

Protected resources whose newest archive is older than two days  -  the failure class no portal screen surfaces.

### Licensing drift check

```bash
afi-cli reconcile-licenses --org 01F0ORG0000000000000000000 --json
```

Purchased subscription quantities vs actually-protected resource counts, per tenant.

## Usage

Run `afi-cli --help` for the full command reference and flag list.

## Commands

### applications

Manage applications

- **`afi-cli applications`** - Lists installations for the application identified by the authentication key.
Use the `limit` and `page_token` query parameters to paginate through all installations.

### orgs

Manage orgs

- **`afi-cli orgs create`** - Creates a new child organization.
- **`afi-cli orgs get`** - Retrieves an organization by its ID.

### tenants

Manage tenants

- **`afi-cli tenants <id>`** - Retrieves a tenant by its ID.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
afi-cli orgs get <org-id>

# JSON for scripting and agents
afi-cli orgs get <org-id> --json

# Filter to specific fields
afi-cli orgs get <org-id> --json --select id,name,status

# Dry run  -  show the request without sending
afi-cli orgs get <org-id> --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
afi-cli orgs get <org-id> --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
afi-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/afi-calls-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `AFI_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `afi-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `afi-cli doctor` to check credentials
- Verify the environment variable is set: `echo $AFI_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Set AFI_API_KEY to a valid Application key (looks like appkey-...). The value is sent raw in the Authorization header  -  do not add a Bearer prefix.
- **429 Too Many Requests during fleet-sync**  -  Afi rate-limits aggressively and asks for exponential backoff. Re-run 'fleet-sync' later, scope it with --tenants, or add --skip-archives; prefer local queries over repeated live calls.
- **A tenant or org is missing from fleet-sync output**  -  The Application only sees orgs/tenants where it is installed. Check installations with 'afi-cli applications' and install the app in the missing org via the Afi portal.
- **coverage-gaps, fleet-health, or resolve returns nothing**  -  These commands read the local store. Run 'afi-cli fleet-sync' first; the stderr hints will tell you when the store is unsynced or stale.
