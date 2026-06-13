# SkyKick CLI

**Fleet-wide M365 backup assurance for SkyKick Cloud Backup - posture, stale snapshots, and coverage gaps no portal or wrapper can show.**

Every evidenced SkyKick (ConnectWise Cloud Services) Backup API operation as typed commands, plus a local SQLite fleet store that answers the questions the per-tenant API can't: which customers aren't fully protected (fleet-health), which mailboxes silently stopped snapshotting (stale-snapshots), and what changed since last review (drift). Built for the current apis.cloudservices.connectwise.com host - the only CLI that works post-migration.

## Install

The recommended path installs both the `skykick-cli` binary and the `pp-skykick` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install skykick
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install skykick --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install skykick --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install skykick --agent claude-code
npx -y @mvanhorn/printing-press-library install skykick --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/skykick/cmd/skykick-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/skykick-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install skykick --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-skykick --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-skykick --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install skykick --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

The bundle reuses your local OAuth tokens  -  authenticate first if you haven't:

```bash
skykick-cli auth login
```

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/skykick-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `SKYKICK_CLIENT_ID` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/skykick/cmd/skykick-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "skykick": {
      "command": "skykick-mcp",
      "env": {
        "SKYKICK_CLIENT_ID": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

SkyKick uses OAuth2 client-credentials behind Azure API Management with a twist: the token request needs HTTP Basic auth (API user ID + subscription key) AND an Ocp-Apim-Subscription-Key header, and every API call carries both the Bearer token and the subscription key. Set SKYKICK_CLIENT_ID to your API user ID and SKYKICK_CLIENT_SECRET to your subscription key (Partner Portal -> Settings -> User Profile -> Developer API Access; click Show on the Partner Subscription). The CLI mints and caches tokens automatically - SkyKick rate-limits the token endpoint aggressively, so cached reuse matters. Set SKYKICK_OAUTH_SCOPE=Distributor for distributor accounts (default Partner).

## Quick Start

```bash
# Verify config, credentials wiring, and API reachability before anything else
skykick-cli doctor --dry-run

# List every backup subscription order across your customers
skykick-cli backup list --agent

# Pull subscriptions plus per-tenant settings, mailboxes, sites, snapshot stats, and alerts into the local fleet store
skykick-cli fleet-sync

# The headline: one protection-posture row per customer tenant, gaps flagged
skykick-cli fleet-health --agent

# Find every mailbox that silently stopped snapshotting
skykick-cli stale-snapshots --hours 48 --agent

# Find tenants where autodiscover is off - new mailboxes there will silently never enroll
skykick-cli autodiscover-audit --only-off --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Fleet posture
- **`fleet-sync`**  -  One command pulls every subscription plus per-tenant settings, retention, autodiscover, snapshot stats, mailboxes, sites, and alerts into the local SQLite fleet store.

  _Run this first; every fleet-posture and backup-integrity command reads the store it builds._

  ```bash
  skykick-cli fleet-sync --agent
  ```
- **`fleet-health`**  -  One cross-tenant protection posture table - every subscription's Exchange/SharePoint enablement, retention, autodiscover, and last-backup age with gap flags.

  _The single command an MSP owner runs to see fleet-wide backup posture without per-tenant portal crawling._

  ```bash
  skykick-cli fleet-health --flag-gaps --agent
  ```
- **`retention-audit`**  -  Grades each tenant's retention period against a compliance floor you set.

  _Turns scattered retention numbers into a pass/under-floor compliance attestation for QBRs._

  ```bash
  skykick-cli retention-audit --floor-days 365 --agent
  ```
- **`autodiscover-audit`**  -  Fleet table of autodiscover on/off state per tenant.

  _Surfaces tenants where new hires will silently go unprotected because autodiscover is off._

  ```bash
  skykick-cli autodiscover-audit --only-off --agent
  ```
- **`partner-rollup`**  -  Protection posture aggregated by partner for distributor oversight.

  _Gives a distributor a per-partner protection scorecard the API can't assemble._

  ```bash
  skykick-cli partner-rollup --agent
  ```

### Backup integrity
- **`stale-snapshots`**  -  Every mailbox not snapshotted within N hours, fleet-wide.

  _Silently-stale mailboxes are the top MSP backup liability; this finds them all in one call._

  ```bash
  skykick-cli stale-snapshots --hours 48 --agent
  ```
- **`coverage-gaps`**  -  Discovered-but-unprotected mailboxes and SharePoint sites per tenant.

  _Catches new hires and sites that exist but aren't actually backed up after onboarding or churn._

  ```bash
  skykick-cli coverage-gaps --type mailboxes --agent
  ```
- **`drift`**  -  Diffs two sync snapshots and reports protection-state changes since last sync.

  _Surfaces newly-stale mailboxes, dropped subscriptions, and enablement flips between reviews._

  ```bash
  skykick-cli drift --agent
  ```

### Alert ops
- **`alert-sweep`**  -  One ranked list of open alerts across the whole fleet, with optional bulk mark-complete.

  _Replaces N per-id portal lookups with one cross-fleet triage view plus bulk closure._

  ```bash
  skykick-cli alert-sweep --agent
  ```

### Async control
- **`watch-operation`**  -  Polls an async operation to a terminal state with backoff in one command.

  _Lets onboarding discovery complete unattended instead of hand-polling operation status._

  ```bash
  skykick-cli watch-operation 1a2b3c4d-0000-0000-0000-000000000000 --timeout 300
  ```

## Recipes


### Morning fleet protection check

```bash
skykick-cli fleet-sync && skykick-cli fleet-health --flag-gaps --agent
```

Refresh the fleet store then emit one posture row per tenant with gap flags - the daily is-everyone-protected loop.

### Find silently-failing backups

```bash
skykick-cli stale-snapshots --hours 48 --agent --select mailbox,subscription_id,last_snapshot
```

Mailboxes with no snapshot in 48h, narrowed to the three fields an agent needs to open tickets.

### Post-onboarding coverage reconciliation

```bash
skykick-cli coverage-gaps --type mailboxes --agent
```

After running 'backup discover-mailboxes <subscriptionId>' and 'watch-operation <operationId>' (the discover response carries the id), list anything discovered but not protected.

### Cross-fleet alert triage

```bash
skykick-cli alert-sweep --agent
```

Fan /Alerts across every stored subscription and return one ranked open-alert list.

### QBR retention attestation

```bash
skykick-cli retention-audit --floor-days 365 --agent --select company,exchange_retention_days,status
```

Grade every tenant against a 1-year retention floor and keep only the columns the QBR deck needs.

## Usage

Run `skykick-cli --help` for the full command reference and flag list.

## Commands

### alerts

Alerts for backup services and email migration orders

- **`skykick-cli alerts complete`** - Mark a specific alert as complete
- **`skykick-cli alerts list`** - List alerts for a backup service or email migration order (max 500; the API does not support skip paging)

### backup

Cloud Backup subscriptions - M365 Exchange and SharePoint protection per customer tenant

- **`skykick-cli backup autodiscover`** - Auto-discover state (enabled/disabled) for Exchange and SharePoint
- **`skykick-cli backup by-partner`** - List backup subscription orders for a specific partner
- **`skykick-cli backup datacenters`** - List the Azure data centers available for backup storage
- **`skykick-cli backup discover-mailboxes`** - Trigger Exchange mailbox discovery (async; poll the returned operation with watch-operation)
- **`skykick-cli backup discover-sites`** - Trigger SharePoint site discovery (async; poll the returned operation with watch-operation)
- **`skykick-cli backup jobs`** - Active backup jobs for the subscription (known upstream defect: may return Unknown error, community-reported since 2024)
- **`skykick-cli backup last-snapshot-stats`** - Last snapshot statistics for all mailboxes in the subscription
- **`skykick-cli backup list`** - List all placed backup subscription orders across your customers
- **`skykick-cli backup mailbox`** - Details of a specific Exchange mailbox in the subscription
- **`skykick-cli backup mailboxes`** - Exchange mailboxes and their backup enabled/disabled status (IndividualMailboxes array)
- **`skykick-cli backup retention-period`** - Data retention periods for Exchange and SharePoint (response field ExchangeRentionPeriodInDays is the upstream spelling)
- **`skykick-cli backup sites`** - SharePoint site URLs and their backup enabled/disabled status
- **`skykick-cli backup sku`** - SKU and promotional details for a backup subscription
- **`skykick-cli backup storage-settings`** - Storage settings for a backup subscription
- **`skykick-cli backup subscription-settings`** - Subscription settings: Exchange/SharePoint backup state, enabled counts, customer info

### identity

Authenticated caller identity

- **`skykick-cli identity`** - Show the identity and context of the authenticated API user

### operations

Async operation tracking and work queue

- **`skykick-cli operations status`** - Poll the status of an async operation (e.g. a discovery run)
- **`skykick-cli operations workqueue`** - Retrieve the work queue for the authenticated account


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
skykick-cli alerts list <id>

# JSON for scripting and agents
skykick-cli alerts list <id> --json

# Filter to specific fields
skykick-cli alerts list <id> --json --select id,name,status

# Dry run  -  show the request without sending
skykick-cli alerts list <id> --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
skykick-cli alerts list <id> --agent
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
skykick-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: ``

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `SKYKICK_CLIENT_ID` | per_call | Yes | Set to your API credential. |
| `SKYKICK_CLIENT_SECRET` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `skykick-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `skykick-cli doctor` to check credentials
- Verify the environment variable is set: `echo $SKYKICK_CLIENT_ID`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Authorization has been denied for this request**  -  Both headers are required on every call. Confirm SKYKICK_CLIENT_ID and SKYKICK_CLIENT_SECRET are set, then run skykick-cli doctor; the subscription key must be the one shown under Developer API Access for the matching scope.
- **400 invalid_request: Missing required parameter: username on token mint**  -  The token call requires Basic auth carrying your API user ID. Set SKYKICK_CLIENT_ID (user ID) - an empty client id produces exactly this error.
- **Token requests failing repeatedly / rate-limited**  -  SkyKick strictly rate-limits /auth/token. The CLI caches tokens; avoid parallel cold-start invocations, and wait 60s before retrying after a burst.
- **Calls against apis.skykick.com fail with TLS or 404 errors**  -  That host died 2025-09-16 (ConnectWise migration). This CLI already targets apis.cloudservices.connectwise.com; remove any SKYKICK_BASE_URL override pointing at the old domain.
- **backup jobs returns an Unknown error**  -  The /Backup/{id}/jobs endpoint has a known upstream defect (community-reported since 2024). Use last-snapshot-stats or stale-snapshots for backup-recency evidence instead.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**BoSen29/SkykickAPI**](https://github.com/BoSen29/SkykickAPI)  -  PowerShell (4 stars)
- [**jancotanis/skykick**](https://github.com/jancotanis/skykick)  -  Ruby

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
