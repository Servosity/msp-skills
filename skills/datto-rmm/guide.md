# Datto RMM CLI

**Every Datto RMM API operation, plus a local SQLite fleet store and fleet-wide analytics no other Datto tool has.**

datto-rmm-cli mirrors the full Datto RMM v2 API (sites, devices, alerts, jobs, audit, variables) and syncs your whole multi-site fleet into local SQLite. That unlocks questions no single API call can answer: stale devices, alert storms, software sprawl, warranty cliffs, patch and AV gaps, and one-shot QBR scorecards  -  all offline, agent-native, and composable with jq.

## Install

The recommended path installs both the `datto-rmm-cli` binary and the `pp-datto-rmm` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install datto-rmm
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install datto-rmm --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install datto-rmm --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install datto-rmm --agent claude-code
npx -y @mvanhorn/printing-press-library install datto-rmm --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/datto-rmm/cmd/datto-rmm-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/datto-rmm-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install datto-rmm --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-datto-rmm --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-datto-rmm --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install datto-rmm --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/datto-rmm-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `DATTO_RMM_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/datto-rmm/cmd/datto-rmm-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "datto-rmm": {
      "command": "datto-rmm-mcp",
      "env": {
        "DATTO_RMM_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Datto RMM uses OAuth 2.0. You have an API key and an API secret key (Setup > Users > generate API keys in the Datto RMM web UI). The CLI mints and auto-refreshes a bearer token for you from DATTO_RMM_API_KEY + DATTO_RMM_API_SECRET_KEY (no manual token handling). Pick your region with DATTO_RMM_PLATFORM (pinotage, merlot, concord, vidal, zinfandel, syrah) or set DATTO_RMM_API_URL directly. Tokens last ~100 hours and refresh automatically on 401.

## Quick Start

```bash
# Confirm your API key/secret and platform mint a valid token and the API is reachable.
datto-rmm-cli doctor

# Pull every site, device, and alert into the local SQLite store.
datto-rmm-cli sync

# First payoff: every device that hasn't checked in for 30 days, across all sites.
datto-rmm-cli fleet stale --days 30

# Every endpoint with antivirus disabled or not running, fleet-wide.
datto-rmm-cli fleet av-gaps --status not-running

# Full-text search the synced fleet for a site, device, or alert.
datto-rmm-cli search "acme"

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Fleet hygiene that compounds
- **`fleet stale`**  -  Lists every device across all customer sites that hasn't checked in within a chosen window, so you catch dead agents before the customer does.

  _Reach for this when you need the authoritative list of unreachable or abandoned endpoints across the entire fleet, not just one site._

  ```bash
  datto-rmm-cli fleet stale --days 30 --agent --select hostname,siteName,lastSeen,online
  ```
- **`fleet sprawl`**  -  Rolls up audited software across the whole fleet to show how many installs of an app exist and the spread of versions, exposing license waste and unpatched copies.

  _Reach for this when you need a license-true or vulnerability-true count of an application across every endpoint, not a per-device lookup._

  ```bash
  datto-rmm-cli fleet sprawl --name "Google Chrome" --agent
  ```
- **`fleet agent-drift`**  -  Shows which devices run out-of-date RMM agents across the fleet, ranked by how far behind the newest version they are.

  _Reach for this to find endpoints whose stale RMM agent may be missing monitors before troubleshooting visibility gaps._

  ```bash
  datto-rmm-cli fleet agent-drift --agent --select hostname,siteName,cagVersion,lastSeen
  ```

### Alert intelligence
- **`fleet storms`**  -  Ranks the noisiest devices and sites by alert volume over a time window so the NOC can tune monitors instead of drowning in tickets.

  _Reach for this to find the alert-fatigue offenders driving ticket volume before recommending monitor or threshold changes._

  ```bash
  datto-rmm-cli fleet storms --days 7 --top 20 --agent
  ```
- **`fleet av-gaps`**  -  Finds every device fleet-wide where antivirus is missing, disabled, or not running, so security gaps surface before an incident does.

  _Reach for this when you need the definitive list of unprotected endpoints across all customers for a security review or incident sweep._

  ```bash
  datto-rmm-cli fleet av-gaps --status not-running --agent --select hostname,siteName,antivirus.antivirusProduct,antivirus.antivirusStatus
  ```
- **`fleet resolve-storm`**  -  Bulk-resolves the open alerts on the noisiest devices and sites from the storm ranking, so Monday triage ends with a clean queue instead of a worklist.

  _Reach for this when a monitor misfire flooded the queue and you need to act on the ranking, not just look at it; defaults to a plan-only preview, and nothing resolves without --confirm._

  ```bash
  datto-rmm-cli fleet resolve-storm --days 7 --top 10 --agent
  ```
- **`fleet device-timeline`**  -  Stitches one device's alerts and activity-log entries (which surface job, audit, and user actions) into a single chronological timeline from the local store.

  _Reach for this during triage when a device keeps misbehaving and you need its full recent history in one stream, not four console tabs._

  ```bash
  datto-rmm-cli fleet device-timeline ACME-DC01 --days 30 --agent
  ```

### Lifecycle and compliance
- **`fleet patch-gaps`**  -  Ranks every device by missing-patch count across all sites so you remediate the most-exposed endpoints first.

  _Reach for this to prioritize patch remediation fleet-wide by exposure rather than checking one device at a time._

  ```bash
  datto-rmm-cli fleet patch-gaps --min-missing 5 --agent
  ```
- **`fleet warranty`**  -  Surfaces every device whose hardware warranty expires within a window, sorted by date and site, ready to drop into a QBR refresh plan.

  _Reach for this when preparing a hardware-refresh or QBR budget conversation that needs the expiring-warranty list across all customers._

  ```bash
  datto-rmm-cli fleet warranty --within 60 --agent --select hostname,siteName,warrantyDate,deviceType.type
  ```
- **`fleet scorecard`**  -  Produces a one-shot per-site health card fusing device counts, alert volume, patch coverage, AV coverage, warranty exposure, and agent drift for QBRs.

  _Reach for this when you or a vCIO need an instant customer-health summary to open a QBR or flag an at-risk account._

  ```bash
  datto-rmm-cli fleet scorecard "Acme Corporation" --agent
  ```
- **`fleet orphans`**  -  Cross-references long-stale, suspended, or deleted devices that still count against a site so you stop billing and monitoring phantom endpoints.

  _Reach for this when cleaning up a site's device roster or auditing billing accuracy against what is actually alive._

  ```bash
  datto-rmm-cli fleet orphans --stale-days 90 --agent
  ```

### Receipts and data trust
- **`fleet job-results`**  -  Fetches every device's result for one quick job and rolls them into a single pass/fail table, so a 200-device push is verified in one command.

  _Reach for this after any quick-job push to answer which devices errored or printed nothing; fetches live per device, so it needs credentials and a synced cohort (--devices, --site, or --all)._

  ```bash
  datto-rmm-cli fleet job-results 8d3f2c1a-4b6e-4f0a-9c2d-1a2b3c4d5e6f --devices 0a1b2c3d-uid1,4e5f6a7b-uid2 --failed-only --agent
  ```
- **`fleet snapshot`**  -  Freezes the current synced fleet state into a labeled, dated archive so any number you report can be reproduced later as a receipt.

  _Reach for this before a QBR or audit so a disputed number weeks later can be proven against the dated snapshot; run sync first so the receipt reflects fresh data._

  ```bash
  datto-rmm-cli fleet snapshot --label q2-acme --note "QBR baseline"
  ```
- **`fleet diff`**  -  Diffs two fleet snapshots (or a snapshot vs the live store) to show devices added or removed and warranty, patch, AV, and agent-version movement since the baseline.

  _Reach for this in QBR prep to lead with the delta story: what got better, what slipped, what arrived and left; both sides read the synced store, so sync before snapshotting._

  ```bash
  datto-rmm-cli fleet diff --from q1-acme --to q2-acme --agent
  ```
- **`fleet sync-gaps`**  -  Verifies every synced resource pulled all pages by comparing stored row counts against the API's reported totals, so you can trust fleet numbers before reporting them.

  _Reach for this after sync and before any fleet report; an incomplete devices table makes every downstream number quietly wrong._

  ```bash
  datto-rmm-cli fleet sync-gaps --agent
  ```

## Recipes


### Find unprotected endpoints for a security review

```bash
datto-rmm-cli fleet av-gaps --status not-running --agent --select hostname,siteName,antivirus.antivirusProduct,antivirus.antivirusStatus
```

Returns only the fields an agent needs from the deeply-nested device record (antivirus is a sub-object), so context stays small.

### Patch-remediation worklist by exposure

```bash
datto-rmm-cli fleet patch-gaps --min-missing 5 --agent
```

Every device missing 5+ patches across all sites, ranked, ready to feed a remediation job.

### QBR prep for one customer

```bash
datto-rmm-cli fleet scorecard "Acme Corporation" --agent
```

One health card fusing device counts, open alerts, patch %, AV %, warranty exposure, and agent drift.

### Hardware-refresh budget list

```bash
datto-rmm-cli fleet warranty --within 90 --agent --select hostname,siteName,warrantyDate
```

Devices whose warranty expires within 90 days, sorted by date  -  drop straight into a QBR.

## Usage

Run `datto-rmm-cli --help` for the full command reference and flag list.

## Commands

### account

Manage account

- **`datto-rmm-cli account create-variable`** - Creates an account variable
- **`datto-rmm-cli account delete-variable`** - Deletes the account variable identified by the given variable Id.
- **`datto-rmm-cli account get-components`** - Fetches the components records of the authenticated user's account.
- **`datto-rmm-cli account get-dnet-site-mappings`** - Fetches the sites records with its mapped dnet network id for the authenticated user's account.
- **`datto-rmm-cli account get-sites`** - Fetches the site records of the authenticated user's account.
- **`datto-rmm-cli account get-user`** - Fetches the authenticated user's account data.
- **`datto-rmm-cli account get-user-closed-alerts`** - If the muted parameter is not provided, both muted and umuted alerts will be queried.
- **`datto-rmm-cli account get-user-devices`** - Fetches the devices of the authenticated user's account.
- **`datto-rmm-cli account get-user-open-alerts`** - If the muted parameter is not provided, both muted and umuted alerts will be queried.
- **`datto-rmm-cli account get-users`** - Fetches the authentication users records of the authenticated user's account.
- **`datto-rmm-cli account get-variables`** - Fetches the account variables.
- **`datto-rmm-cli account update-variable`** - Updates the account variable identified by the given variable Id.

### activity-logs

Manage activity logs

- **`datto-rmm-cli activity-logs`** - Fetches the activity logs.

### alert

Manage alert

- **`datto-rmm-cli alert <alertUid>`** - Fetches data of the alert identified by the given alert Uid.

### audit

Manage audit

- **`datto-rmm-cli audit get-device`** - Fetches audit data of the generic device identified the given device Uid.
- **`datto-rmm-cli audit get-device-by-mac-address`** - Fetches audit data of the generic device(s) identified the given MAC address in format: XXXXXXXXXXXX
- **`datto-rmm-cli audit get-device-software`** - Fetches audited software of the generic device identified the given device Uid.
- **`datto-rmm-cli audit get-esxi-host`** - Fetches audit data of the ESXi host identified the given device Uid.
- **`datto-rmm-cli audit get-printer`** - Fetches audit data of the printer identified the given device Uid.

### device

Manage device

- **`datto-rmm-cli device get-by-id`** - Fetches data of the device identified by the given device Id.
- **`datto-rmm-cli device get-by-mac-address`** - Fetches data of the device(s) identified by the given MAC address in format: XXXXXXXXXXXX
- **`datto-rmm-cli device get-by-uid`** - Fetches data of the device identified by the given device Uid.

### filter

Manage filter

- **`datto-rmm-cli filter get-custom`** - Fetches the custom device filters for the user (using administrator role).
- **`datto-rmm-cli filter get-defaults`** - Fetches the default device filters.

### job

Manage job

- **`datto-rmm-cli job <jobUid>`** - Fetches data of the job identified by the given job Uid.

### site

Manage site

- **`datto-rmm-cli site create`** - Creates a new site in the authenticated user's account.
- **`datto-rmm-cli site get`** - Fetches data of the site (including total number of devices) identified by the given site Uid.
- **`datto-rmm-cli site update`** - Updates the site identified by the given site Uid.

### system

Manage system

- **`datto-rmm-cli system get`** - Fetches the request rate status for the authenticated user's account.
- **`datto-rmm-cli system get-pagination-configurations`** - Fetches the pagination configurations.
- **`datto-rmm-cli system get-status`** - Fetches the system status (start date, status and version).

### user

Manage user

- **`datto-rmm-cli user`** - Resets the authenticated user's API access and secret keys.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
datto-rmm-cli alert <id>

# JSON for scripting and agents
datto-rmm-cli alert <id> --json

# Filter to specific fields
datto-rmm-cli alert <id> --json --select id,name,status

# Dry run  -  show the request without sending
datto-rmm-cli alert <id> --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
datto-rmm-cli alert <id> --agent
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
datto-rmm-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/datto-rmm-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `DATTO_RMM_API_KEY` | auth_flow_input | Yes | Set during initial auth setup. |
| `DATTO_RMM_API_SECRET_KEY` | auth_flow_input | Yes | Set during initial auth setup. |
| `DATTO_RMM_TOKEN` | per_call | No | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `datto-rmm-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `datto-rmm-cli doctor` to check credentials
- Verify the environment variable is set: `echo $DATTO_RMM_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 Unauthorized on every call**  -  Re-check DATTO_RMM_API_KEY and DATTO_RMM_API_SECRET_KEY, and confirm DATTO_RMM_PLATFORM matches your account region (run: datto-rmm-cli doctor).
- **Token works but all lists are empty**  -  Run 'datto-rmm-cli sync' first  -  fleet analytics read the local store, which starts empty.
- **Wrong region / host not found**  -  Set DATTO_RMM_PLATFORM to your platform (pinotage, merlot, concord, vidal, zinfandel, syrah) or DATTO_RMM_API_URL to the full https://<platform>-api.centrastage.net base.
- **429 / request-rate errors during a big sync**  -  The sync backs off and retries automatically; if it persists, wait for the rate window to reset or sync fewer resources at once.

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**aaronengels/DattoRMM**](https://github.com/aaronengels/DattoRMM)  -  PowerShell (85 stars)
- [**josh-fisher/datto-rmm**](https://github.com/josh-fisher/datto-rmm)  -  TypeScript
- [**wyre-technology/node-datto-rmm**](https://github.com/wyre-technology/node-datto-rmm)  -  TypeScript
- [**wyre-technology/datto-rmm-mcp**](https://github.com/wyre-technology/datto-rmm-mcp)  -  TypeScript
- [**pncit/datto-rmm-api-client**](https://github.com/pncit/datto-rmm-api-client)  -  TypeScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
