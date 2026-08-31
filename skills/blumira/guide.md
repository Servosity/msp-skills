# Blumira CLI

**Every Blumira finding, detection, and agent across your direct org and every MSP sub-account  -  in one offline-searchable store with cross-account triage and over-time trends no single API call can answer.**

Blumira's API and the community MCP return per-account, point-in-time snapshots. This CLI syncs findings, evidence, detection rules, and agent devices from your org and every MSP client account into a local SQLite store, so you get one ranked cross-account triage queue (triage), what-changed-since-last-sync (drift), MTTR and velocity trends (velocity), SLA aging (sla), detection coverage drift vs the MSP basis (coverage), and domain-controller exposure (exposure). It also mints and auto-refreshes its own JWT from your Client ID and Secret instead of making you bring your own.

## Install

This CLI ships as a Claude Code Skill and MCP server in [Servosity/msp-skills](https://github.com/Servosity/msp-skills). The installer downloads the `blumira-cli` and `blumira-mcp` binaries into `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows). It does not register the skill with your agent and writes no MCP client config - see [mcp-install.md](./mcp-install.md) for that wire-up.

1. macOS / Linux:
   ```bash
   bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.sh)
   ```
2. Windows (PowerShell):
   ```powershell
   iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.ps1 | iex
   ```
3. Verify: `blumira-cli --version`
4. Ensure `~/.local/bin` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills` (Windows) is on `$PATH`.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed until verification succeeds.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=blumira). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install Servosity/msp-skills/skills/blumira --force
```

Inside a Hermes chat session:

```bash
/skills install Servosity/msp-skills/skills/blumira --force
```

Hermes [speaks MCP natively](https://hermes-agent.nousresearch.com), so it can also use the `blumira-mcp` server directly - same install path, same environment variables. Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

> Install the blumira skill from https://github.com/Servosity/msp-skills/tree/main/skills/blumira. The skill defines how its required CLI (`blumira-cli`) can be installed via the `openclaw:` frontmatter block.

OpenClaw isn't generally available yet; the frontmatter wiring is pre-shipped and will activate the moment OpenClaw launches.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/Servosity/msp-skills/releases?q=blumira).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `BLUMIRA_API_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. A bundle carries the five platform binaries the builder downloads - macOS (`darwin-arm64`, `darwin-amd64`), Linux (`linux-arm64`, `linux-amd64`) and Windows (`windows-amd64`). Windows on ARM is released as a standalone binary but is not bundled, so use the manual config below there.

> **Interim note:** check any `.mcpb` bundle before you trust it ([#287](https://github.com/Servosity/msp-skills/issues/287)). Its `manifest.json` launches `${__dirname}/bin/blumira-mcp`, while the builder stores the release binaries in `bin/` under their platform-suffixed names - `blumira-mcp-darwin-arm64`, `-darwin-amd64`, `-linux-arm64`, `-linux-amd64`, `-windows-amd64.exe`. Run `unzip -l <file>.mcpb | grep bin/`: if the name the manifest launches is not among them, Claude Desktop has nothing to run - use the installer above and the manual JSON config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.sh)          # macOS / Linux
```
```powershell
iwr -useb https://raw.githubusercontent.com/Servosity/msp-skills/main/skills/blumira/install.ps1 | iex            # Windows
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "blumira": {
      "command": "blumira-mcp",
      "env": {
        "BLUMIRA_API_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Blumira uses OAuth2 client-credentials. Generate a Client ID + Client Secret in Settings > Organization > Generate API Credentials, then run `auth login --client-id <id> --client-secret <secret>`  -  the CLI exchanges them at https://auth.blumira.com/oauth/token (audience=public-api) for a ~30-day JWT, caches it, and refreshes it automatically. You can also export a pre-minted token as BLUMIRA_API_TOKEN. Keys are read-write by default; scope them read-only in the Blumira UI for safe read access. MSP tenants address sub-orgs by account_id; nothing is baked in.

## Quick Start

```bash
# Mint and cache a JWT from your API credentials.
blumira-cli auth login --client-id "$BLUMIRA_CLIENT_ID" --client-secret "$BLUMIRA_CLIENT_SECRET"

# Confirm auth + API reachability before syncing.
blumira-cli doctor

# List your MSP client accounts (sub-orgs).
blumira-cli msp get-accounts --json

# Pull findings, detections, and agents from the org and every account into the local store.
blumira-cli sync --full

# See the ranked open-findings queue across all clients.
blumira-cli triage --priority high --status open

# See what changed since the last sync.
blumira-cli drift --since 24h

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-account & over-time intelligence
- **`triage`**  -  One globally-ranked open-findings queue across every MSP client account.

  _Reach for this to start MSP triage: it answers 'what is on fire across all my clients right now' in one ranked list instead of one account at a time._

  ```bash
  blumira-cli triage --priority high --status open --agent
  ```
- **`drift`**  -  New, status-changed, and newly-resolved findings since the last sync, per account.

  _The shift-handoff / morning-standup answer  -  pick this when the user asks what is new or changed, not the full current state._

  ```bash
  blumira-cli drift --since 24h --agent
  ```
- **`velocity`**  -  Mean-time-to-resolve and open-rate per account or overall.

  _Use for reporting and trend tracking  -  it is the only way to get response-time trends Blumira's API does not expose._

  ```bash
  blumira-cli velocity --window 30d --by account --agent
  ```
- **`sla`**  -  Findings about to breach an age-based SLA across all accounts, ranked.

  _Pick this to catch findings slipping past SLA before they breach, instead of discovering breaches after the fact._

  ```bash
  blumira-cli sla --breach-in 4h --priority high --agent
  ```
- **`coverage`**  -  Detection rules missing or disabled in your org versus the MSP basis ruleset.

  _Use to find detection rules that silently drifted away from the MSP basis baseline._

  ```bash
  blumira-cli coverage --against basis --agent
  ```
- **`exposure`**  -  Cross-account agent rollup that flags stale or unprotected domain controllers.

  _Reach for this for billing/coverage reviews and to spot unprotected domain controllers across the whole MSP fleet._

  ```bash
  blumira-cli exposure --flag-dc-stale --agent
  ```
- **`audit`**  -  Findings that were resolved and then re-fired (reopened).

  _Use to catch premature or low-quality resolutions that re-fired._

  ```bash
  blumira-cli audit --min-reopens 2 --agent
  ```
- **`recurring`**  -  The same detection firing repeatedly across accounts and time.

  _Pick this to find the noisy detection or unfixed root cause that keeps generating findings._

  ```bash
  blumira-cli recurring --window 90d --min-count 3 --agent
  ```
- **`overview`**  -  One-screen per-account rollup: open findings by priority, agent coverage, and detection-coverage drift across every client.

  _Run this first each morning: it answers 'which client needs attention' before triage answers 'which finding'._

  ```bash
  blumira-cli overview --agent
  ```
- **`reconcile`**  -  Flat finding-to-org-to-assignee-to-status table for diffing against ConnectWise, Jira, or Zendesk.

  _Use when reconciling Blumira findings against an external ticketing system - one command replaces the manual cross-account export._

  ```bash
  blumira-cli reconcile --status open --csv
  ```
- **`evidence-search`**  -  Full-text search over synced finding evidence to find which findings mention an IOC, hostname, or user.

  _Reach for this during threat hunting when you have an indicator but not a finding ID._

  ```bash
  blumira-cli evidence-search "rdp brute force" --agent
  ```
- **`dc-roster`**  -  Every domain controller across all accounts with last check-in and protected/stale state.

  _Use for fleet and billing reviews needing the full DC inventory; use exposure instead for the prioritized stale/unprotected action list._

  ```bash
  blumira-cli dc-roster --agent
  ```
- **`workload`**  -  Open assigned findings grouped by owner across all accounts, with age buckets.

  _Use when balancing analyst assignments or reviewing team load before sprint or shift planning._

  ```bash
  blumira-cli workload --agent
  ```

### Access that beats the incumbent
- **`auth login`**  -  Mints, caches, and auto-refreshes the Blumira JWT from a Client ID + Secret.

  _Use once to authenticate; every other command then just works without manually exchanging or refreshing tokens._

  ```bash
  blumira-cli auth login --client-id "$BLUMIRA_CLIENT_ID" --client-secret "$BLUMIRA_CLIENT_SECRET"
  ```

## Recipes


### Morning MSP triage

```bash
blumira-cli sync --full && blumira-cli triage --priority high --status open --json
```

Sync every account, then pull the ranked high-priority open queue across all clients as JSON for a dashboard or agent.

### Shift-handoff drift report

```bash
blumira-cli drift --since 12h --json --select account,name,change
```

Show only what changed since the last sync with a narrow field set so an agent does not parse full finding payloads.

### Monthly velocity by account

```bash
blumira-cli velocity --window 30d --by account --csv
```

Export per-account MTTR and open-rate for a 30-day report.

### Find unprotected domain controllers

```bash
blumira-cli exposure --flag-dc-stale --json
```

List domain controllers across all accounts whose agent check-in is stale or missing.

### Hunt an indicator across every client's evidence

```bash
blumira-cli evidence-search "rdp brute force" --agent --select matches.finding_id,matches.name
```

Greps the locally cached evidence corpus plus synced finding payloads for an IOC/hostname/term and returns just the fields an agent needs  -  impossible upstream, where evidence is only retrievable per known finding ID. Pass --fetch to populate the evidence cache live first (bounded by --max-fetch).

## Usage

Run `blumira-cli --help` for the full command reference and flag list.

## Commands

### health

Manage health

- **`blumira-cli health`** - Get API health

### msp

Manage msp

- **`blumira-cli msp add-account-finding-comment`** - Add a comment to a finding for your MSP sub-account
- **`blumira-cli msp get`** - Get a specific MSP sub-account by ID
- **`blumira-cli msp get-account-agents-devices`** - Get or search agents devices for your MSP sub-account
- **`blumira-cli msp get-account-agents-keys`** - Get or search agents keys for your MSP sub-account
- **`blumira-cli msp get-account-finding`** - Get a specific finding by ID for your MSP sub-account
- **`blumira-cli msp get-account-finding-comments`** - Get comments for a specific finding for your MSP sub-account
- **`blumira-cli msp get-account-finding-evidence`** - Returns evidence keys and a paginated first page of evidence rows for the finding in one call. Same contract as GET /org/findings/{finding_id}/evidence; finding must belong to the sub-account (account_id).
- **`blumira-cli msp get-account-findings`** - Get or search findings for your MSP sub-account
- **`blumira-cli msp get-accounts`** - Get or search sub-accounts for your MSP
- **`blumira-cli msp get-accounts-findings`** - Get or search findings for all your MSP sub-accounts
- **`blumira-cli msp get-agents-device`** - Get a specific agents device by ID for your MSP sub-account
- **`blumira-cli msp get-agents-key`** - Get a specific agents key by ID for your MSP sub-account
- **`blumira-cli msp get-detection-rule-by-account`** - Get a detection rule by ID for your MSP sub-account
- **`blumira-cli msp get-detection-rules-by-account`** - List detection rules for your MSP sub-account
- **`blumira-cli msp get-msp-detection-rule`** - Get a basis detection rule by ID (for MSP bulk detail)
- **`blumira-cli msp get-msp-detection-rules`** - List basis detection rules (catalog) for MSP bulk
- **`blumira-cli msp list-users`** - Get or search users for your MSP sub-account
- **`blumira-cli msp resolve-finding`** - Resolve a finding for your MSP sub-account
- **`blumira-cli msp set-finding-owners`** - Assign owners to a finding for your MSP sub-account

### org

Manage org

- **`blumira-cli org controller-direct-add-comment`** - Add a comment to a finding in the org
- **`blumira-cli org controller-direct-get`** - Get a specific finding by ID
- **`blumira-cli org controller-direct-get-agents-device`** - Get a specific agents device by ID
- **`blumira-cli org controller-direct-get-agents-devices`** - Get or search agents devices
- **`blumira-cli org controller-direct-get-agents-key`** - Get a specific agents key by ID
- **`blumira-cli org controller-direct-get-agents-keys`** - Get or search agents keys
- **`blumira-cli org controller-direct-get-by`** - Get or search findings for your organization
- **`blumira-cli org controller-direct-get-comments`** - Get comments for a specific finding
- **`blumira-cli org controller-direct-get-details`** - Get details for a specific finding
- **`blumira-cli org controller-direct-get-detection-rule`** - Get a detection rule by ID
- **`blumira-cli org controller-direct-get-detection-rules-by`** - List detection rules for your organization
- **`blumira-cli org controller-direct-get-evidence`** - Returns evidence keys (schema) and a paginated first page of evidence rows for the finding in one call. Resolves the finding to its match and orchestrates evidence_key + evidence from bdf-matches. Use query params for pagination (e.g. page=2 for subsequent pages).
- **`blumira-cli org controller-direct-list-users`** - Get or search users for your organization
- **`blumira-cli org controller-direct-resolve-finding`** - Resolve a finding
- **`blumira-cli org controller-direct-set-owners`** - Assign owners to a finding

### resolutions

Manage resolutions

- **`blumira-cli resolutions`** - Get resolution options for findings


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
blumira-cli health

# JSON for scripting and agents
blumira-cli health --json

# Filter to specific fields
blumira-cli health --json --select id,name,status

# Dry run  -  show the request without sending
blumira-cli health --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
blumira-cli health --agent
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
blumira-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/blumira-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `BLUMIRA_API_TOKEN` | per_call | No | Set to your API credential. |
| `BLUMIRA_CLIENT_ID` | auth_flow_input | No | Blumira API Client ID (Settings > Organization > Generate API Credentials). Used with BLUMIRA_CLIENT_SECRET by `auth login` to mint a JWT. |
| `BLUMIRA_CLIENT_SECRET` | auth_flow_input | No | Set during initial auth setup. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `blumira-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `blumira-cli doctor` to check credentials
- Verify the environment variable is set: `echo $BLUMIRA_API_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401/403 'Key not authorized' or 'Authorization field missing'**  -  Run `auth login` again  -  the JWT is missing or expired (tokens last ~30 days). Check `doctor`.
- **Empty results from triage/drift/velocity**  -  Run `sync --full` first; the cross-account and over-time commands read the local store, which needs at least one sync (drift/velocity need two).
- **429 / rate limited**  -  The client backs off automatically (MSP limit is 100 req/s); rerun, or narrow with finding filters like --priority or --status.
- **MSP command returns nothing for an account**  -  Confirm the account_id with `msp get-accounts`; per-account endpoints require a valid sub-org UUID.
