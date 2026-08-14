# Cork CLI

**Every Cork API operation, plus cross-client risk attribution, exploitability-first triage, and stale-connector detection that a stateless API mirror cannot do.**

Cork tells you a client's risk score moved but never why, and offers no way to ask a question across your whole book of business. This CLI mirrors the API into local SQLite, then answers the questions that need history and fan-out: score attribute explains what drove a score change, score regressions ranks every client by how far they slipped, vulnerabilities triage orders patching by what is actually being exploited rather than by CVSS, and integrations health catches the connector that reports healthy while its data quietly went stale.

## Install

The recommended path installs both the `cork-cli` binary and the `pp-cork` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install cork
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install cork --cli-only
```

For skill only  -  installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install cork --skill-only
```

To constrain the skill install to one or more specific agents (repeatable  -  agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install cork --agent claude-code
npx -y @mvanhorn/printing-press-library install cork --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.5 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/cork/cmd/cork-cli@latest
```

This installs the CLI only  -  no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/cork-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install cork --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-cork --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-cork --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install cork --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle  -  Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/cork-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `CORK_API_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/monitoring/cork/cmd/cork-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "cork": {
      "command": "cork-mcp",
      "env": {
        "CORK_API_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Cork uses a bearer API key. Mint one in the Cork platform under Admin then API Keys, choosing a name and an expiry. Set it as CORK_API_KEY, or store it with cork-cli auth set-token. One caveat worth knowing: an API key inherits the permissions of the user who created it, so a 403 on a distributor or integration endpoint usually means the key was minted by an operator without that scope, not that the key is wrong.

## Quick Start

```bash
# Confirm the binary, config path, and local database resolve before spending an API call
cork-cli doctor --dry-run

# Populate the local mirror; the client roster carries the score history and tenant map that cross-client commands read
cork-cli sync --resources clients,warranties

# The Monday question: which clients moved backwards this week
cork-cli score regressions --since 7d --agent

# Build the patch queue ordered by what is actually being exploited
cork-cli vulnerabilities triage --kev-only --limit 25 --agent

# Catch connectors reporting healthy while their data has gone stale
cork-cli integrations health --stale-after 24h --agent

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Risk that compounds locally
- **`score attribute`**  -  Explain why a client's Cork score moved, broken out by claims, compliance, coverage, and vulnerability impact.

  _Reach for this when someone asks why a client got worse, instead of pulling raw score points and eyeballing a trend._

  ```bash
  cork-cli score attribute 3f2a9c14-7b6d-4e21-9a8c-1d5e2f0b4c77 --since 30d --agent
  ```
- **`score regressions`**  -  Rank every client by score change over a window, worst movers first.

  _Use this to open a book-of-business review; it answers 'who moved backwards' in one call instead of one call per client._

  ```bash
  cork-cli score regressions --since 7d --min-drop 10 --agent
  ```

### Exploitability over CVSS
- **`vulnerabilities triage`**  -  Rank software products by exploitability (known-exploited first, then EPSS, then CVSS) with a blast-radius device count and client names resolved.

  _Pick this over a raw vulnerability list when deciding what to patch first. Live-only: Cork's vulnerability rows carry no id so they cannot be mirrored locally, --limit counts products rather than individual findings, and a capped scan sets scan_cap_hit._

  ```bash
  cork-cli vulnerabilities triage --kev-only --min-epss 0.3 --limit 25 --agent
  ```
- **`vulnerabilities exposure`**  -  List every affected client, device, product, and version for a single CVE id.

  _Use this the moment a specific CVE is named in an advisory or a client email; it is the only way to answer 'are we exposed'. Exit 3 means the scan succeeded and found no exposure, which is a negative answer rather than a failure. Live-only: vulnerability rows carry no id so there is no local mirror to read, and a capped scan sets scan_cap_hit, which is explicitly not a clean bill of health._

  ```bash
  cork-cli vulnerabilities exposure CVE-2026-1234 --agent
  ```

### Data you can actually trust
- **`compliance overdue`**  -  Surface compliance events that have blown their event type's remediation window, bucketed by age.

  _Choose this over listing a client's events when you need the ones that are actively costing score, ordered by how late they are._

  ```bash
  cork-cli compliance overdue --bucket --agent
  ```
- **`integrations health`**  -  Flag connectors that are down, degraded, or reporting healthy while their last sync has gone stale, and name the clients they feed.

  _Run this before trusting any risk number; a silently stale connector makes every downstream score fiction._

  ```bash
  cork-cli integrations health --stale-after 24h --agent
  ```
- **`coverage gaps`**  -  Diff the devices a connector reports against the devices attributed to the client to expose endpoints one tool sees and another misses.

  _Use this during onboarding verification or when coverage impact is dragging a score and you need the specific unmonitored endpoints. Exit 3 means there were no connector devices to diff, a negative answer rather than a failure; the command refuses outright rather than reporting gaps against a device baseline it could not read._

  ```bash
  cork-cli coverage gaps --client 3f2a9c14-7b6d-4e21-9a8c-1d5e2f0b4c77 --agent
  ```

### Commercial signal
- **`warranties exposure`**  -  Rank unwarranted or lapsed clients by current risk so coverage conversations start with the ones that need it most.

  _Reach for this when preparing commercial outreach, rather than listing warranties and cross-checking risk by hand._

  ```bash
  cork-cli warranties exposure --limit 20 --agent
  ```

## Recipes

### Explain a score drop before a QBR

```bash
cork-cli score attribute 3f2a9c14-7b6d-4e21-9a8c-1d5e2f0b4c77 --since 30d --agent
```

Returns the four score impact components differenced across the window so you can name the cause instead of describing the trend.

### Narrow a deeply nested vulnerability payload for an agent

```bash
cork-cli vulnerabilities get-software --agent --select sw_vendor,sw_product,cves.cve_id,cves.epss,cves.is_kev --page-size 20
```

Vulnerability rows nest a full CVE array per product; selecting dotted paths keeps the exploitability signals and drops the rest of the payload.

### Answer an advisory the moment a CVE is named

```bash
cork-cli vulnerabilities exposure CVE-2026-1234 --agent
```

Scans the live vulnerability collection page by page and matches the CVE locally, because no Cork endpoint accepts a CVE filter. Exit 3 means the scan found no exposure; check scan_cap_hit before reading that as a clean bill of health.

### Verify a new client is fully monitored after onboarding

```bash
cork-cli coverage gaps --client 3f2a9c14-7b6d-4e21-9a8c-1d5e2f0b4c77 --agent
```

Diffs connector-reported devices against client-attributed devices to list endpoints that one tool sees and another is missing.

### Find a client by name without knowing its UUID

```bash
cork-cli search "Northwind" --type clients --limit 10
```

Full-text search over the local mirror resolves a human name to the UUID every other command takes.

## Usage

Run `cork-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `CORK_CONFIG_DIR`, `CORK_DATA_DIR`, `CORK_STATE_DIR`, or `CORK_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `CORK_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export CORK_HOME=/srv/cork
cork-cli doctor
```

Under `CORK_HOME=/srv/cork`, the four dirs resolve to `/srv/cork/config`, `/srv/cork/data`, `/srv/cork/state`, and `/srv/cork/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "cork": {
      "command": "cork-mcp",
      "env": {
        "CORK_HOME": "/srv/cork"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `CORK_DATA_DIR` overrides an explicit `--home` for that kind. Use `CORK_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `CORK_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `cork-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### clients

Manage clients

- **`cork-cli clients`** - List clients with their financial protection status (`warranty_status`), associated integration tenants, and the 10 most recent Cork Cyber Scores (`score_history`, newest first). For older scores, such as quarter-over-quarter trends or filtering by a date range, use `Get Client Score History`. Client UUIDs from this response are required by `Get Client Devices`, `Get Client Inboxes`, `Get Client Domains`, `Get Compliance Events`, and vulnerability tools. _note: If API user is a distributor, use partner_uuid to scope results to a specific partner._

### compliance

Manage compliance

- **`cork-cli compliance get-event-notification-settings`** - List the notification and alerting rules configured for compliance events on client assets. Shows which event types trigger alerts and how they are routed.
- **`cork-cli compliance get-event-types`** - List all compliance event types with their descriptions and cure periods. Use this to discover valid event_type values before filtering `Get Compliance Events`.
- **`cork-cli compliance get-events`** - List policy violations and risk events detected for a client's assets. Filter by event_type (use `Get Compliance Event Types` for valid values), device, inbox, or domain UUID. Use at_risk=true to show only currently active risks. Resolved events are excluded by default; set show_resolved=true to include them.

### distributor

Manage distributor

- **`cork-cli distributor get-partners`** - List partner sub-accounts managed by this distributor. Returns partner UUIDs that can be passed as partner_uuid to `Get Clients` and other tools to scope results to a specific partner. _note: distributor accounts only_
- **`cork-cli distributor provision-partner`** - Provision a new Partner account in the system. _note: Requires distributor privileges._

### integrations

Manage integrations

- **`cork-cli integrations connect`** - Connect an API-based integration. This creates a new integration that will immediately begin syncing data. Requires valid API credentials.
- **`cork-cli integrations delete`** - Delete an integration and stop all data collection from it. _note: Only API-created integrations can be deleted._
- **`cork-cli integrations get-available`** - List integration types that can be connected to Cork, including required credential fields. Use this before `Connect an Integration` to determine which vendor keys and credential schemas are supported.
- **`cork-cli integrations get-connected`** - List integrations that have been connected to Cork, including their vendor, connection status, and sync details. RMM integrations also carry an `installer` block describing whether software installs can run through them (`capable`, `requires_manual_setup`, `authorized`, `configured_package_managers`). Use it with `Get Client Devices` (`associated_endpoints[].integration`) to decide which integration an `Install Software` call will route through and whether it needs setup. Use this to discover integration UUIDs needed by `Get Integration Devices`, `Get Integration Users`, and `Get Integration Tenants`.
- **`cork-cli integrations update`** - Update an API-created integration's display name and/or credentials. _note: Only API-created integrations can be updated. When updating credentials, all credential fields must be provided._

### invoices

Manage invoices

- **`cork-cli invoices`** - List billing invoices. Returns invoice UUIDs required by `Get Invoice Line Items`. _note: If API user is a distributor, use partner_uuid to scope results to a specific partner._

### me

Manage me

- **`cork-cli me`** - Information on the authenticated user

### software

Manage software

- **`cork-cli software get-installer-history`** - List past software install attempts (most recent first) with their dispatch state, target client/device, package, and any errors. Every `Install Software` call appears here once dispatched. Filter by `client_uuid` or `device_uuid` to scope to a single client or device. `state` is one of queued, running, success, partial, error; 'success' means the RMM accepted the job, not that the on-device install finished.
- **`cork-cli software get-installer-setup`** - Get the one-time setup steps for an RMM vendor that requires manual setup before software installs work. Returns the script to create in the RMM, the exact name to give it, the settings to match, and the variables to declare. Use this when a connected integration shows `installer.requires_manual_setup=true` (and the package manager is missing from `installer.configured_package_managers`), or when `Install Software` returns `setup_required`. Vendors that auto-provision their script (e.g. Intune) report `requires_manual_setup=false` and need no setup.
- **`cork-cli software get-packages`** - List software packages available to install across supported package managers (WinGet, Chocolatey). Filter by `package_manager_key` to scope to a single manager, or `search` to substring-match name/publisher. Returns package_id values to pass to `Install Software`.
- **`cork-cli software install`** - Install a software package on a single mapped device via the device's RMM integration. The install is dispatched asynchronously through the RMM (Intune, NinjaRMM, Datto RMM); check `Get Installer History` for completion state. The device must have `can_install_software=true` in `Get Client Devices`. Required inputs: `mapped_device_uuid`, `package_manager_key`, `package_id`, `interpreter_key` (use `PS` for PowerShell). If this returns `setup_required` or `not_authorized`, the routing RMM integration needs setup; see `Get Installer Setup Instructions`.

### vulnerabilities

Manage vulnerabilities

- **`cork-cli vulnerabilities get-software`** - List individual software vulnerabilities with full CVE details including CVSS score, EPSS score, KEV (known exploited) status, and impacted version. Filter by minimum_cvss_score, minimum_epss_score, minimum_priority, or only_known_exploited=true to focus on the highest-risk findings. Scope by client_uuid or device_uuid.
- **`cork-cli vulnerabilities get-software-vulnerability-summary`** - Get a rollup of CVEs grouped by software product, showing number of impacted devices, impacted versions, and highest severity rating. Use client_uuid to scope to a single client. Follow up with `Get Software Vulnerabilities` to drill into specific CVEs for a product.

### warranties

Manage warranties

- **`cork-cli warranties`** - List active cyber warranty packages. To identify which clients lack coverage, check the warranty_status field in `Get Clients` results - clients with 'unwarranted' status have no active warranty.


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`cork-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`cork-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`cork-cli learnings list`** - Inspect taught rows
- **`cork-cli learnings forget <query>`** - Undo a teach
- **`cork-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`cork-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`cork-cli teach-pattern`** - Install a query/resource template up front
- **`cork-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `CORK_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `cork-cli` opens the database, older binaries refuse it with a version error  -  upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
cork-cli clients

# JSON for scripting and agents
cork-cli clients --json
# Filter to specific fields
cork-cli clients --json --select associated_tenants,created_at,hidden

# Dry run  -  show the request without sending
cork-cli clients --dry-run

# Agent mode  -  JSON + compact + no prompts in one flag
cork-cli clients --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and add `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
cork-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `cork-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/cork-cli/config.toml`; `--home`, `CORK_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `CORK_API_KEY` | per_call | Yes | Set to your API credential. |

### agentcookie (optional)

If you use agentcookie to sync secrets across machines, this CLI auto-adopts agentcookie-managed credentials with no extra setup. When the daemon writes to this CLI's config, `cork-cli doctor` reports `agentcookie: detected` and `auth-status` labels the source as `agentcookie`. Skip this section if you don't use agentcookie - the CLI works the same as any other.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `cork-cli doctor` to check credentials
- Verify the environment variable is set: `echo $CORK_API_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **401 with "Missing Authorization header"**  -  Set CORK_API_KEY, or run cork-cli auth set-token, then re-check with cork-cli doctor
- **403 on distributor or integration endpoints while other commands work**  -  The key inherits its creator's permissions; mint a new key in Admin then API Keys as an operator who holds that scope
- **score regressions returns nothing**  -  Run cork-cli sync --resources clients first; score regressions reads only the local mirror. warranties exposure prefers the mirrored roster but falls back to a live fetch, so it works either way
- **A result looks complete but only covered part of the fleet**  -  Check scan_cap_hit and note in the JSON envelope. Capped commands stop at --max-scan-pages, --max-clients or --max-connectors and say so; raise the relevant cap to widen the sweep
- **vulnerabilities exposure or coverage gaps exits 3**  -  Exit 3 from these two commands means the scan succeeded and matched nothing, not that it failed. Treat it as a negative answer, and check scan_cap_hit before reading it as a clean bill of health
- **--data-source local appears to be ignored on vulnerabilities triage or exposure**  -  Expected: both commands are live-only because Cork's vulnerability rows carry no id and cannot be mirrored locally
- **sync --resources vulnerabilities reports zero records**  -  Expected: Cork's vulnerability rows carry no id, so they cannot be mirrored locally. Use cork-cli vulnerabilities triage or vulnerabilities exposure, which query the API directly
- **Results look stale or predate a recent change in Cork**  -  Re-run cork-cli sync --resources clients --full, and check connector freshness with cork-cli integrations health
- **HTTP 429 during a wide sync or scan**  -  Cork does not publish rate limits. Narrow the sync with cork-cli sync --max-pages 2, then retry
